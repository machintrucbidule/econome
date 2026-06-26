package services

import (
	"context"
	"errors"
	"sort"

	"econome/internal/domain"
	"econome/internal/repo"
)

// Month-initialisation assistant (functional/09, functional/04 §3.4/§4, rules
// §5/§7/§8/§11.1). The draft is computed on the fly and **never persisted** until
// "Créer le mois" (T3i); its only mutable state is the user's per-leaf amount
// overrides, carried in the request (I-025). Materialisation rule (uniform):
//   - fixed_recurring expense/income due this month → allocation (planned =
//     amount) + an awaited transaction (amount = ±magnitude by flow sign);
//   - variable expense/income → allocation only;
//   - fixed_recurring transfer due this month → an awaited transfer transaction
//     only (no allocation — transfers are excluded from the budget, rules §10);
//   - residual envelope → nothing (computed).

// DraftPost is one leaf line of the month-init draft (a generated allocation
// and/or awaited transaction). Amount is an unsigned magnitude in minor units.
type DraftPost struct {
	EnvelopeID    int64
	CategoryID    int64
	Name          string
	AccountID     int64
	AccountName   string
	DestAccountID *int64
	Flow          domain.FlowType
	Mode          domain.Mode
	Amount        int64 // default_amount or the user override (≥ 0)
	ExpectedDay   *int
	Recurring     bool // true → "Prévu" chip; false → "Allocation" chip
	HasTxn        bool // generates an awaited transaction
	HasAllocation bool // generates an allocation (planned amount)
}

// DraftSavings is the residual figures for one sweep account, projected for the
// view layer so handlers need not import the engine (rules §7/§9/§11.1).
type DraftSavings struct {
	Projected        int64  // residual savings (start + Σ planned income − Σ planned expense)
	ResidualNegative bool   // projected < 0 → red overdraft alert (§11.1)
	CascadeFull      bool   // every cascade vehicle is full (§9/C4)
	CascadeTargetID  *int64 // the vehicle the residual fills, or nil
}

// MonthDraft is the computed, non-persisted preview of a new month.
type MonthDraft struct {
	Period         string
	Accounts       []domain.Account // active accounts (for scope + labels)
	Posts          []DraftPost
	StartByAccount map[int64]int64        // start_of_month per account (C5)
	Sweeps         []int64                // sweep account ids (one band each)
	SavingsBySweep map[int64]DraftSavings // residual encart per sweep account
}

// IsCreated reports whether the period already has a row (the assistant is then
// unavailable, functional/09 §5 / I-027).
func (s *Service) IsCreated(ctx context.Context, userID int64, period string) (bool, error) {
	_, err := s.periods.ByPeriod(ctx, s.tx.DB(), userID, period)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, domain.ErrNotFound) {
		return false, nil
	}
	return false, err
}

// CurrentPeriod returns the current "YYYY-MM" under the service clock (the
// default month for the assistant).
func (s *Service) CurrentPeriod() string {
	return s.today().Period()
}

// BuildDraft computes the editable draft for a not-yet-created period, applying
// the user's per-envelope amount overrides, and returns it with every figure
// derived by the pure engine over synthetic (unpersisted) inputs.
func (s *Service) BuildDraft(ctx context.Context, userID int64, period string, overrides map[int64]int64) (*MonthDraft, error) {
	if _, _, ok := parsePeriodYM(period); !ok {
		v := &domain.ValidationError{}
		v.Add("period", domain.MsgPeriodInvalid)
		return nil, v
	}
	q := s.tx.DB()
	in, err := s.engineInputs(ctx, q, userID, period)
	if err != nil {
		return nil, err
	}
	posts := buildDraftPosts(in.Envelopes, in.Categories, in.Accounts, period, overrides)
	in.Allocations, in.Txns = syntheticInputs(posts, userID, period)

	d := &MonthDraft{
		Period:         period,
		Posts:          posts,
		StartByAccount: make(map[int64]int64),
		SavingsBySweep: make(map[int64]DraftSavings),
	}
	for _, a := range in.Accounts {
		if a.Status == domain.ArchiveActive {
			d.Accounts = append(d.Accounts, a)
		}
		d.StartByAccount[a.ID] = in.AccountBalances(a.ID).Start
	}
	d.Sweeps = in.SweepAccounts()
	for _, id := range d.Sweeps {
		sv := in.Savings(id)
		d.SavingsBySweep[id] = DraftSavings{
			Projected:        sv.Projected,
			ResidualNegative: sv.ResidualNegative,
			CascadeFull:      sv.CascadeFull,
			CascadeTargetID:  sv.CascadeTargetID,
		}
	}
	return d, nil
}

// CreateMonth materialises the draft (allocations + awaited transactions) and the
// period row (state=active) + a `create` audit event in one transaction, then the
// month is ACTIVE (functional/04 §4). It refuses a period that already exists
// (I-027). Overrides carry the adjusted draft amounts the user validated.
func (s *Service) CreateMonth(ctx context.Context, userID int64, period string, overrides map[int64]int64) error {
	if _, _, ok := parsePeriodYM(period); !ok {
		v := &domain.ValidationError{}
		v.Add("period", domain.MsgPeriodInvalid)
		return v
	}
	now := s.now().UTC()
	return s.tx.WithTx(ctx, func(q repo.DBTX) error {
		if _, err := s.periods.ByPeriod(ctx, q, userID, period); err == nil {
			return domain.ErrConflict // already created — not re-initialisable (functional/09 §5)
		} else if !errors.Is(err, domain.ErrNotFound) {
			return err
		}
		accounts, err := s.accounts.ListByUser(ctx, q, userID)
		if err != nil {
			return err
		}
		categories, err := s.categories.ListByUser(ctx, q, userID)
		if err != nil {
			return err
		}
		envelopes, err := s.envelopes.ListByUser(ctx, q, userID)
		if err != nil {
			return err
		}
		posts := buildDraftPosts(envelopes, categories, accounts, period, overrides)
		for _, p := range posts {
			if p.HasAllocation {
				if _, err := s.allocations.Create(ctx, q, &domain.Allocation{
					UserID: userID, EnvelopeID: p.EnvelopeID, Period: period,
					PlannedAmount: p.Amount, CreatedAt: now, UpdatedAt: now,
				}); err != nil {
					return err
				}
			}
			if p.HasTxn {
				if _, err := s.transactions.Create(ctx, q, &domain.Transaction{
					UserID: userID, AccountID: p.AccountID, DestAccountID: p.DestAccountID,
					CategoryID: postCategoryID(p), FlowType: p.Flow,
					Amount: signedAmount(p.Flow, p.Amount), BudgetPeriod: period,
					Status: domain.StatusAwaited, Source: domain.SourceManual,
					CreatedAt: now, UpdatedAt: now,
				}); err != nil {
					return err
				}
			}
		}
		if _, err := s.periods.Create(ctx, q, &domain.Period{
			UserID: userID, Period: period, State: domain.PeriodActive,
			CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			return err
		}
		_, err = s.periodEvents.Append(ctx, q, &domain.PeriodEvent{
			UserID: userID, Period: period, Action: domain.ActionCreate,
			At: now, ActorUserID: userID,
		})
		return err
	})
}

// buildDraftPosts derives the leaf posts from the current configuration (the
// envelope/account config at creation time, non-retroactive L2), applying the
// per-envelope amount overrides. Archived envelopes/accounts are excluded (L4).
func buildDraftPosts(envelopes []domain.Envelope, categories []domain.Category, accounts []domain.Account, period string, overrides map[int64]int64) []DraftPost {
	catByID := make(map[int64]domain.Category, len(categories))
	for _, c := range categories {
		catByID[c.ID] = c
	}
	acctByID := make(map[int64]domain.Account, len(accounts))
	for _, a := range accounts {
		acctByID[a.ID] = a
	}

	var posts []DraftPost
	for _, e := range envelopes {
		if e.Status != domain.ArchiveActive || e.Mode == domain.ModeResidual {
			continue
		}
		acc, ok := acctByID[e.AccountID]
		if !ok || acc.Status != domain.ArchiveActive {
			continue
		}
		cat, ok := catByID[e.CategoryID]
		if !ok {
			continue
		}
		if !dueThisMonth(e, period) {
			continue
		}
		amount := defaultMagnitude(e)
		if ov, has := overrides[e.ID]; has && ov >= 0 {
			amount = ov
		}
		recurring := e.Mode == domain.ModeFixedRecurring
		p := DraftPost{
			EnvelopeID:    e.ID,
			CategoryID:    e.CategoryID,
			Name:          cat.Name,
			AccountID:     e.AccountID,
			AccountName:   acc.Name,
			DestAccountID: e.DestAccountID,
			Flow:          cat.FlowType,
			Mode:          e.Mode,
			Amount:        amount,
			ExpectedDay:   e.ExpectedDay,
			Recurring:     recurring,
		}
		switch cat.FlowType {
		case domain.FlowExpense, domain.FlowIncome:
			p.HasAllocation = true // planned drives savings_projected/secured (rules §7)
			p.HasTxn = recurring   // fixed → awaited txn; variable → allocation only
		case domain.FlowTransfer:
			if !recurring {
				continue // a variable transfer generates nothing
			}
			p.HasTxn = true // awaited transfer only (excluded from the budget)
		}
		posts = append(posts, p)
	}
	sort.SliceStable(posts, func(i, j int) bool { return posts[i].EnvelopeID < posts[j].EnvelopeID })
	return posts
}

// syntheticInputs turns the draft posts into in-memory allocations + awaited
// transactions so the engine recomputes the residual without persisting anything.
func syntheticInputs(posts []DraftPost, userID int64, period string) ([]domain.Allocation, []domain.Transaction) {
	var allocs []domain.Allocation
	var txns []domain.Transaction
	for i, p := range posts {
		id := int64(i + 1)
		if p.HasAllocation {
			allocs = append(allocs, domain.Allocation{
				ID: id, UserID: userID, EnvelopeID: p.EnvelopeID, Period: period, PlannedAmount: p.Amount,
			})
		}
		if p.HasTxn {
			txns = append(txns, domain.Transaction{
				ID: id, UserID: userID, AccountID: p.AccountID, DestAccountID: p.DestAccountID,
				CategoryID: postCategoryID(p), FlowType: p.Flow, Amount: signedAmount(p.Flow, p.Amount),
				BudgetPeriod: period, Status: domain.StatusAwaited, Source: domain.SourceManual,
			})
		}
	}
	return allocs, txns
}

// dueThisMonth reports whether an envelope generates this month: variable always
// (an allocation); fixed_recurring monthly always; non-monthly only when the
// month is in due_months.
func dueThisMonth(e domain.Envelope, period string) bool {
	if e.Mode != domain.ModeFixedRecurring {
		return e.Mode == domain.ModeVariable
	}
	if e.Frequency == nil || *e.Frequency == domain.FreqMonthly {
		return true
	}
	_, m, ok := parsePeriodYM(period)
	if !ok {
		return false
	}
	for _, dm := range e.DueMonths {
		if dm == m {
			return true
		}
	}
	return false
}

// defaultMagnitude is the envelope's default amount as an unsigned magnitude (0
// when unset).
func defaultMagnitude(e domain.Envelope) int64 {
	if e.DefaultAmount == nil {
		return 0
	}
	return *e.DefaultAmount
}

// signedAmount applies the signed-amount convention (I-017): expense negative,
// income/transfer positive.
func signedAmount(flow domain.FlowType, magnitude int64) int64 {
	if flow == domain.FlowExpense {
		return -magnitude
	}
	return magnitude
}

// postCategoryID returns the transaction's category id: nil for transfers,
// otherwise the post's category (transactions store their own category, §3.5).
func postCategoryID(p DraftPost) *int64 {
	if p.Flow == domain.FlowTransfer {
		return nil
	}
	id := p.CategoryID
	return &id
}
