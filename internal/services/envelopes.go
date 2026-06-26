package services

import (
	"context"
	"errors"
	"strings"

	"econome/internal/domain"
	"econome/internal/repo"
)

// Envelope & category use-cases (functional/08, functional/04 §3.2–§3.3). The
// validated UI is one combined form: a leaf **category** (name, parent, flow_type)
// plus its **envelope** (account, mode, amounts, recurrence) are written in one
// transaction (I-021). A category is shared across accounts — "Divers" on two
// accounts is one category paired with two accounts (foundation §5), so the leaf
// category is resolved find-or-create by (name, parent, flow_type); uniqueness is
// enforced on (category × account) at the envelope.

// EnvelopeRow is one leaf envelope joined with its category + account name.
type EnvelopeRow struct {
	Envelope    domain.Envelope
	Category    domain.Category
	AccountName string
}

// ParentGroup is a parent category and its child envelope rows (read-only sum of
// the children's default amounts — exact integer addition of minor units).
type ParentGroup struct {
	Category   domain.Category
	Children   []EnvelopeRow
	SumDefault int64
}

// EnvelopesOverview is the hierarchical configuration list.
type EnvelopesOverview struct {
	Parents     []ParentGroup // categories that have child envelopes
	TopLevel    []EnvelopeRow // envelopes whose category has no parent
	HasArchived bool
}

// EnvelopeInput is the combined create/edit form. Money is already parsed to
// minor units at the HTTP boundary (DefaultAmount nil ⇒ residual / none).
type EnvelopeInput struct {
	Name            string
	ParentID        *int64 // existing parent category
	NewParentName   string // "+ Nouvelle catégorie…" — find-or-create a parent
	FlowType        string
	DefaultExpanded bool // seeds the parent node's forecast expansion (M4)
	AccountID       int64
	Mode            string
	DefaultAmount   *int64
	Frequency       string
	DueMonths       []int
	ExpectedDay     *int
}

// EnvelopesOverview returns the hierarchical list; archived rows are included
// only when includeArchived (the handler still renders them hidden behind the
// toggle, but excluding them server-side keeps the default view clean).
func (s *Service) EnvelopesOverview(ctx context.Context, userID int64, includeArchived bool) (*EnvelopesOverview, error) {
	q := s.tx.DB()
	cats, err := s.categories.ListByUser(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	envs, err := s.envelopes.ListByUser(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	accts, err := s.accounts.ListByUser(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	catByID := make(map[int64]domain.Category, len(cats))
	for _, c := range cats {
		catByID[c.ID] = c
	}
	acctName := make(map[int64]string, len(accts))
	for _, a := range accts {
		acctName[a.ID] = a.Name
	}

	ov := &EnvelopesOverview{}
	groups := map[int64]*ParentGroup{} // parent category id -> group
	for _, e := range envs {
		if e.Status == domain.ArchiveArchived {
			ov.HasArchived = true
			if !includeArchived {
				continue
			}
		}
		cat, ok := catByID[e.CategoryID]
		if !ok {
			continue
		}
		row := EnvelopeRow{Envelope: e, Category: cat, AccountName: acctName[e.AccountID]}
		if cat.ParentID == nil {
			ov.TopLevel = append(ov.TopLevel, row)
			continue
		}
		pid := *cat.ParentID
		g := groups[pid]
		if g == nil {
			pc := catByID[pid]
			g = &ParentGroup{Category: pc}
			groups[pid] = g
		}
		g.Children = append(g.Children, row)
		if e.Status == domain.ArchiveActive && e.DefaultAmount != nil {
			g.SumDefault += *e.DefaultAmount
		}
	}
	for _, c := range cats {
		if g := groups[c.ID]; g != nil {
			ov.Parents = append(ov.Parents, *g)
		}
	}
	return ov, nil
}

// ParentOptions returns the categories usable as a parent in the form select —
// those that already group at least one envelope — so the user reuses them; brand
// new parents come from the "+ Nouvelle catégorie…" path (find-or-create).
func (s *Service) ParentOptions(ctx context.Context, userID int64) ([]domain.Category, error) {
	q := s.tx.DB()
	cats, err := s.categories.ListByUser(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	hasChild := map[int64]bool{}
	for _, c := range cats {
		if c.ParentID != nil {
			hasChild[*c.ParentID] = true
		}
	}
	var out []domain.Category
	for _, c := range cats {
		if c.ParentID == nil && hasChild[c.ID] {
			out = append(out, c)
		}
	}
	return out, nil
}

// GetEnvelope returns an envelope + its category, scoped to the user.
func (s *Service) GetEnvelope(ctx context.Context, userID, id int64) (domain.Envelope, domain.Category, error) {
	e, err := s.envelopes.Get(ctx, s.tx.DB(), userID, id)
	if err != nil {
		return domain.Envelope{}, domain.Category{}, err
	}
	c, err := s.categories.Get(ctx, s.tx.DB(), userID, e.CategoryID)
	if err != nil {
		return domain.Envelope{}, domain.Category{}, err
	}
	return *e, *c, nil
}

// CreateEnvelope writes the leaf category (+ optional new parent) and the envelope
// in one transaction (non-retroactive, L2).
func (s *Service) CreateEnvelope(ctx context.Context, userID int64, in EnvelopeInput) (*domain.Envelope, error) {
	if err := validateEnvelope(in); err != nil {
		return nil, err
	}
	var created *domain.Envelope
	err := s.tx.WithTx(ctx, func(q repo.DBTX) error {
		parentID, err := s.resolveParent(ctx, q, userID, in)
		if err != nil {
			return err
		}
		catID, err := s.resolveLeafCategory(ctx, q, userID, in, parentID, 0)
		if err != nil {
			return err
		}
		now := s.now().UTC()
		e := &domain.Envelope{
			UserID: userID, CategoryID: catID, AccountID: in.AccountID,
			Mode: domain.Mode(in.Mode), DefaultAmount: amountForMode(in),
			Status: domain.ArchiveActive, CreatedAt: now, UpdatedAt: now,
		}
		applyRecurrence(e, in)
		id, err := s.envelopes.Create(ctx, q, e)
		if err != nil {
			return envelopeWriteError(err)
		}
		e.ID = id
		created = e
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// UpdateEnvelope updates the leaf category and the envelope in one transaction.
func (s *Service) UpdateEnvelope(ctx context.Context, userID, id int64, in EnvelopeInput) (*domain.Envelope, error) {
	if err := validateEnvelope(in); err != nil {
		return nil, err
	}
	var updated *domain.Envelope
	err := s.tx.WithTx(ctx, func(q repo.DBTX) error {
		cur, err := s.envelopes.Get(ctx, q, userID, id)
		if err != nil {
			return err
		}
		parentID, err := s.resolveParent(ctx, q, userID, in)
		if err != nil {
			return err
		}
		// Reuse/relocate the envelope's own leaf category in place (rename/reparent).
		leaf, err := s.categories.Get(ctx, q, userID, cur.CategoryID)
		if err != nil {
			return err
		}
		now := s.now().UTC()
		leaf.Name = strings.TrimSpace(in.Name)
		leaf.ParentID = parentID
		leaf.FlowType = domain.FlowType(in.FlowType)
		leaf.UpdatedAt = now
		if err := s.categories.Update(ctx, q, leaf); err != nil {
			return err
		}
		cur.AccountID = in.AccountID
		cur.Mode = domain.Mode(in.Mode)
		cur.DefaultAmount = amountForMode(in)
		applyRecurrence(cur, in)
		cur.UpdatedAt = now
		if err := s.envelopes.Update(ctx, q, cur); err != nil {
			return envelopeWriteError(err)
		}
		updated = cur
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// resolveParent returns the parent category id for the input: a new parent is
// find-or-created by name (so the same name is never duplicated), otherwise the
// selected existing parent is used (nil ⇒ top level). The parent's flow_type must
// match the leaf's branch (single-type branch, functional/04 §3.2). Cyclic
// parenting is impossible because a parent is always top-level (parent_id NULL).
func (s *Service) resolveParent(ctx context.Context, q repo.DBTX, userID int64, in EnvelopeInput) (*int64, error) {
	flow := domain.FlowType(in.FlowType)
	name := strings.TrimSpace(in.NewParentName)
	if name == "" {
		if in.ParentID == nil {
			return nil, nil
		}
		pc, err := s.categories.Get(ctx, q, userID, *in.ParentID)
		if err != nil {
			return nil, err
		}
		if pc.FlowType != flow {
			return nil, flowConflict()
		}
		// An existing parent's default_expanded is not silently flipped from a
		// child's form (it would surprise; "Ouvert par défaut" only seeds a *new*
		// parent here — the per-user override is M4/inc 6).
		return in.ParentID, nil
	}
	cats, err := s.categories.ListByUser(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	for _, c := range cats {
		if c.ParentID == nil && strings.EqualFold(c.Name, name) {
			if c.FlowType != flow {
				return nil, flowConflict()
			}
			id := c.ID
			return &id, nil
		}
	}
	now := s.now().UTC()
	pc := &domain.Category{
		UserID: userID, Name: name, FlowType: flow,
		DefaultExpanded: in.DefaultExpanded, Status: domain.ArchiveActive,
		CreatedAt: now, UpdatedAt: now,
	}
	id, err := s.categories.Create(ctx, q, pc)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func flowConflict() error {
	v := &domain.ValidationError{}
	v.Add("flow_type", domain.MsgFlowTypeConflict)
	return v
}

// resolveLeafCategory find-or-creates the leaf category by (name, parent, flow):
// the same category may pair with several accounts (one envelope each). excludeID
// lets an edit skip its own row (0 = none).
func (s *Service) resolveLeafCategory(ctx context.Context, q repo.DBTX, userID int64, in EnvelopeInput, parentID *int64, excludeID int64) (int64, error) {
	cats, err := s.categories.ListByUser(ctx, q, userID)
	if err != nil {
		return 0, err
	}
	name := strings.TrimSpace(in.Name)
	for _, c := range cats {
		if c.ID == excludeID {
			continue
		}
		if strings.EqualFold(c.Name, name) && samePtr(c.ParentID, parentID) && c.FlowType == domain.FlowType(in.FlowType) {
			return c.ID, nil
		}
	}
	now := s.now().UTC()
	c := &domain.Category{
		UserID: userID, Name: name, ParentID: parentID, FlowType: domain.FlowType(in.FlowType),
		Status: domain.ArchiveActive, CreatedAt: now, UpdatedAt: now,
	}
	return s.categories.Create(ctx, q, c)
}

// ArchiveEnvelope / UnarchiveEnvelope soft-archive an envelope.
func (s *Service) ArchiveEnvelope(ctx context.Context, userID, id int64) error {
	return s.setEnvelopeStatus(ctx, userID, id, domain.ArchiveArchived)
}

// UnarchiveEnvelope restores an archived envelope.
func (s *Service) UnarchiveEnvelope(ctx context.Context, userID, id int64) error {
	return s.setEnvelopeStatus(ctx, userID, id, domain.ArchiveActive)
}

func (s *Service) setEnvelopeStatus(ctx context.Context, userID, id int64, status domain.ArchiveStatus) error {
	e, err := s.envelopes.Get(ctx, s.tx.DB(), userID, id)
	if err != nil {
		return err
	}
	if e.Mode == domain.ModeResidual && status == domain.ArchiveArchived {
		v := &domain.ValidationError{}
		v.Add("mode", domain.MsgResidualNotDeletable)
		return v
	}
	e.Status = status
	e.UpdatedAt = s.now().UTC()
	return s.envelopes.Update(ctx, s.tx.DB(), e)
}

// DeleteEnvelope hard-deletes an envelope when it has no dependents (allocations/
// transactions), else archives it (L4/L10). A residual envelope is structural and
// never deletable.
func (s *Service) DeleteEnvelope(ctx context.Context, userID, id int64) (archived bool, err error) {
	e, err := s.envelopes.Get(ctx, s.tx.DB(), userID, id)
	if err != nil {
		return false, err
	}
	if e.Mode == domain.ModeResidual {
		v := &domain.ValidationError{}
		v.Add("mode", domain.MsgResidualNotDeletable)
		return false, v
	}
	err = s.envelopes.Delete(ctx, s.tx.DB(), userID, id)
	if errors.Is(err, domain.ErrConflict) {
		if aerr := s.setEnvelopeStatus(ctx, userID, id, domain.ArchiveArchived); aerr != nil {
			return false, aerr
		}
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return false, nil
}

func validateEnvelope(in EnvelopeInput) error {
	v := &domain.ValidationError{}
	if strings.TrimSpace(in.Name) == "" {
		v.Add("name", domain.MsgNameRequired)
	}
	if !domain.FlowType(in.FlowType).Valid() {
		v.Add("flow_type", domain.MsgFlowTypeInvalid)
	}
	mode := domain.Mode(in.Mode)
	if !mode.Valid() {
		v.Add("mode", domain.MsgModeInvalid)
	}
	if in.AccountID == 0 {
		v.Add("account_id", domain.MsgAccountRequired)
	}
	switch mode {
	case domain.ModeFixedRecurring:
		freq := domain.Frequency(in.Frequency)
		if !freq.Valid() {
			v.Add("frequency", domain.MsgFrequencyRequired)
		} else if freq != domain.FreqMonthly && len(in.DueMonths) == 0 {
			v.Add("due_months", domain.MsgDueMonthsRequired)
		}
		if in.ExpectedDay != nil && (*in.ExpectedDay < 1 || *in.ExpectedDay > 31) {
			v.Add("expected_day", domain.MsgExpectedDayInvalid)
		}
		if in.DefaultAmount != nil && *in.DefaultAmount < 0 {
			v.Add("default_amount", domain.MsgAmountNegative)
		}
	case domain.ModeVariable:
		if in.DefaultAmount != nil && *in.DefaultAmount < 0 {
			v.Add("default_amount", domain.MsgAmountNegative)
		}
	case domain.ModeResidual:
		if in.DefaultAmount != nil {
			v.Add("default_amount", domain.MsgResidualNoAmount)
		}
	}
	for _, m := range in.DueMonths {
		if m < 1 || m > 12 {
			v.Add("due_months", domain.MsgDueMonthsInvalid)
			break
		}
	}
	return v.OrNil()
}

// amountForMode drops the default amount for a residual envelope (computed).
func amountForMode(in EnvelopeInput) *int64 {
	if domain.Mode(in.Mode) == domain.ModeResidual {
		return nil
	}
	return in.DefaultAmount
}

// applyRecurrence sets the recurrence fields only for fixed_recurring; other
// modes carry none.
func applyRecurrence(e *domain.Envelope, in EnvelopeInput) {
	if domain.Mode(in.Mode) != domain.ModeFixedRecurring {
		e.Frequency = nil
		e.DueMonths = nil
		e.ExpectedDay = nil
		return
	}
	freq := domain.Frequency(in.Frequency)
	e.Frequency = &freq
	e.ExpectedDay = in.ExpectedDay
	if freq == domain.FreqMonthly {
		e.DueMonths = nil
	} else {
		e.DueMonths = in.DueMonths
	}
}

func envelopeWriteError(err error) error {
	if errors.Is(err, domain.ErrDuplicate) {
		v := &domain.ValidationError{}
		v.Add("account_id", domain.MsgEnvelopeDuplicate)
		return v
	}
	return err
}

func samePtr(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
