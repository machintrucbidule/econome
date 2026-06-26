package services

import (
	"context"
	"errors"

	"econome/internal/domain"
	"econome/internal/repo"
)

// Account & savings-cascade use-cases (functional/04 §3.1, functional/10 §2–§3).
// Every method is tenant-scoped: the userID comes from TenantContext, never from
// the request, and the repository filters every query by it (defence in depth).

// currentPeriod is the YYYY-MM of the injected clock (the engine/lifecycle clock
// is always the injected `today`, never time.Now directly).
func (s *Service) currentPeriod() string {
	return s.now().UTC().Format("2006-01")
}

// ensureEditable enforces the locked-month guard (functional/04 §4): a write into
// a LOCKED period is refused with ErrLocked (409). A period with no row yet is
// not created and therefore not locked — editable. Built here so every later
// mutating use-case (inc 5–8) funnels through one choke point.
func (s *Service) ensureEditable(ctx context.Context, q repo.DBTX, userID int64, period string) error {
	p, err := s.periods.ByPeriod(ctx, q, userID, period)
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if p.State == domain.PeriodLocked {
		return domain.ErrLocked
	}
	return nil
}

// ListAccounts returns the user's accounts (active + archived), ordered by id.
func (s *Service) ListAccounts(ctx context.Context, userID int64) ([]domain.Account, error) {
	return s.accounts.ListByUser(ctx, s.tx.DB(), userID)
}

// GetAccount returns one account scoped to the user (ErrNotFound ⇒ 404, also the
// cross-tenant outcome).
func (s *Service) GetAccount(ctx context.Context, userID, id int64) (*domain.Account, error) {
	return s.accounts.Get(ctx, s.tx.DB(), userID, id)
}

// AccountInput is the create/edit form for an account. Ceiling is already parsed
// to minor units at the HTTP boundary (nil = no cap); EffectivePeriod is the
// forward-only effective month for a month_end_policy change (L3).
type AccountInput struct {
	Name            string
	Type            string
	MonthEndPolicy  string
	Ceiling         *int64
	EffectivePeriod string
}

// CreateAccount validates and inserts a new account (status = active).
func (s *Service) CreateAccount(ctx context.Context, userID int64, in AccountInput) (*domain.Account, error) {
	if err := validateAccount(in); err != nil {
		return nil, err
	}
	now := s.now().UTC()
	a := &domain.Account{
		UserID:         userID,
		Name:           trim(in.Name),
		Type:           domain.AccountType(in.Type),
		MonthEndPolicy: domain.MonthEndPolicy(in.MonthEndPolicy),
		Ceiling:        in.Ceiling,
		Status:         domain.ArchiveActive,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	id, err := s.accounts.Create(ctx, s.tx.DB(), a)
	if err != nil {
		return nil, accountWriteError(err)
	}
	a.ID = id
	return a, nil
}

// UpdateAccount validates and applies an edit. A month_end_policy change is
// forward-only (L3): the chosen effective period must not be a locked month.
func (s *Service) UpdateAccount(ctx context.Context, userID, id int64, in AccountInput) (*domain.Account, error) {
	if err := validateAccount(in); err != nil {
		return nil, err
	}
	cur, err := s.accounts.Get(ctx, s.tx.DB(), userID, id)
	if err != nil {
		return nil, err // ErrNotFound ⇒ 404 (also cross-tenant)
	}

	policyChanged := cur.MonthEndPolicy != domain.MonthEndPolicy(in.MonthEndPolicy)
	if policyChanged {
		eff := in.EffectivePeriod
		if eff == "" {
			eff = s.currentPeriod()
		}
		if !validPeriod(eff) {
			v := &domain.ValidationError{}
			v.Add("effective_period", domain.MsgEffectivePeriod)
			return nil, v
		}
		// Forward-only: a locked month is never rewritten (L3); refuse adopting a
		// new policy retroactively into a locked period.
		if err := s.ensureEditable(ctx, s.tx.DB(), userID, eff); err != nil {
			return nil, err
		}
	}

	cur.Name = trim(in.Name)
	cur.Type = domain.AccountType(in.Type)
	cur.MonthEndPolicy = domain.MonthEndPolicy(in.MonthEndPolicy)
	cur.Ceiling = in.Ceiling
	cur.UpdatedAt = s.now().UTC()
	if err := s.accounts.Update(ctx, s.tx.DB(), cur); err != nil {
		return nil, accountWriteError(err)
	}
	return cur, nil
}

// DeleteAccount hard-deletes an account when it has no dependents, otherwise
// archives it (L4/L10): a dependent record (envelope/transaction/snapshot) makes
// the FK RESTRICT fire, surfaced as ErrConflict, which we convert into a
// soft-archive — history is preserved. The bool reports whether it was archived.
func (s *Service) DeleteAccount(ctx context.Context, userID, id int64) (archived bool, err error) {
	err = s.accounts.Delete(ctx, s.tx.DB(), userID, id)
	if errors.Is(err, domain.ErrConflict) {
		if aerr := s.setAccountStatus(ctx, userID, id, domain.ArchiveArchived); aerr != nil {
			return false, aerr
		}
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return false, nil
}

// ArchiveAccount soft-archives an account (stops future generation, hides it from
// selectors/inits, keeps history). UnarchiveAccount reverses it.
func (s *Service) ArchiveAccount(ctx context.Context, userID, id int64) error {
	return s.setAccountStatus(ctx, userID, id, domain.ArchiveArchived)
}

// UnarchiveAccount restores an archived account to active.
func (s *Service) UnarchiveAccount(ctx context.Context, userID, id int64) error {
	return s.setAccountStatus(ctx, userID, id, domain.ArchiveActive)
}

func (s *Service) setAccountStatus(ctx context.Context, userID, id int64, status domain.ArchiveStatus) error {
	a, err := s.accounts.Get(ctx, s.tx.DB(), userID, id)
	if err != nil {
		return err
	}
	a.Status = status
	a.UpdatedAt = s.now().UTC()
	return s.accounts.Update(ctx, s.tx.DB(), a)
}

// ReorderCascade sets the savings cascade to exactly orderedIDs, assigning
// fill_priority 1..n in order (the single source of truth for cascade order,
// T3c). Accounts previously in the cascade but absent from the list are removed
// (fill_priority cleared). All ids must be the user's savings accounts. The clear
// pass runs before the assign pass so the partial UNIQUE(user_id, fill_priority)
// index never sees a transient collision.
func (s *Service) ReorderCascade(ctx context.Context, userID int64, orderedIDs []int64) error {
	return s.tx.WithTx(ctx, func(q repo.DBTX) error {
		all, err := s.accounts.ListByUser(ctx, q, userID)
		if err != nil {
			return err
		}
		byID := make(map[int64]domain.Account, len(all))
		for _, a := range all {
			byID[a.ID] = a
		}
		seen := make(map[int64]bool, len(orderedIDs))
		for _, id := range orderedIDs {
			a, ok := byID[id]
			if !ok {
				return domain.ErrNotFound
			}
			if !a.IsSavings() {
				v := &domain.ValidationError{}
				v.Add("cascade", domain.MsgCascadeNotSavings)
				return v
			}
			if seen[id] {
				return domain.ErrConflict
			}
			seen[id] = true
		}
		now := s.now().UTC()
		for _, a := range all {
			if a.FillPriority != nil {
				a.FillPriority = nil
				a.UpdatedAt = now
				if err := s.accounts.Update(ctx, q, &a); err != nil {
					return err
				}
			}
		}
		for i, id := range orderedIDs {
			a := byID[id]
			prio := i + 1
			a.FillPriority = &prio
			a.UpdatedAt = now
			if err := s.accounts.Update(ctx, q, &a); err != nil {
				return err
			}
		}
		return nil
	})
}

func validateAccount(in AccountInput) error {
	v := &domain.ValidationError{}
	if trim(in.Name) == "" {
		v.Add("name", domain.MsgNameRequired)
	}
	t := domain.AccountType(in.Type)
	if !t.Valid() {
		v.Add("type", domain.MsgAccountTypeInvalid)
	} else {
		// Cross-column rule (kept in the service, not a brittle table CHECK,
		// technical/03 §3.1): current ⇒ sweep/carry; savings ⇒ none.
		p := domain.MonthEndPolicy(in.MonthEndPolicy)
		okCurrent := t == domain.AccountCurrent && (p == domain.PolicySweep || p == domain.PolicyCarry)
		okSavings := t != domain.AccountCurrent && p == domain.PolicyNone
		if !okCurrent && !okSavings {
			v.Add("month_end_policy", domain.MsgAccountPolicyInvalid)
		}
	}
	if in.Ceiling != nil && *in.Ceiling < 0 {
		v.Add("ceiling", domain.MsgCeilingNegative)
	}
	return v.OrNil()
}

// accountWriteError maps a UNIQUE(user_id, name) violation to a field-level 422
// (not a raw 409) so the user sees an inline "name already used" error.
func accountWriteError(err error) error {
	if errors.Is(err, domain.ErrDuplicate) {
		v := &domain.ValidationError{}
		v.Add("name", domain.MsgAccountNameDuplicate)
		return v
	}
	return err
}

// validPeriod reports whether s is a well-formed YYYY-MM month.
func validPeriod(s string) bool {
	if len(s) != 7 || s[4] != '-' {
		return false
	}
	for i, r := range s {
		if i == 4 {
			continue
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	mm := int(s[5]-'0')*10 + int(s[6]-'0')
	return mm >= 1 && mm <= 12
}

func trim(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
