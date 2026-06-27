package services

import (
	"context"

	"econome/internal/domain"
	"econome/internal/engine"
	"econome/internal/repo"
)

// Reconciliation orchestration (P8, functional/04 §7, technical/09 §3). It is the
// DB-write side of the pure engine.Reconcile / engine.PairTransfer decision: the
// exact path the DSP2 import (Stage 9) will drive. The manual journal flow
// reconciles by hand (the inline date-fill of 6c); this seam is built + tested +
// reviewed now and wired by import later — never an auto-match on manual entry.

// MovementInput is an observed cleared movement to reconcile (already parsed to
// minor units + a clock-free Date at the boundary).
type MovementInput struct {
	AccountID  int64
	Magnitude  int64 // positive
	Sign       engine.Sign
	Date       domain.Date
	Period     string
	Label      string
	CategoryID *int64
	FlowType   domain.FlowType
}

// ReconcileOutcome reports what the orchestration did.
type ReconcileOutcome struct {
	Kind         engine.DecisionKind
	TxnID        int64   // the reconciled or created row
	AmbiguousIDs []int64 // the candidate set when Ambiguous (no write)
}

// ManualTolerance is the exact-amount tolerance the manual/by-hand match uses; a
// date window absorbs the gap between an awaited row's forecast day and the real
// op_date. DSP2 may widen the amount tolerance later.
func ManualTolerance() engine.Tolerance { return engine.Tolerance{Amount: 0, DateWindowDays: 5} }

// ReconcileCleared matches a cleared movement against the period's awaited
// candidates on the same account and performs the engine's decision:
// ReconcileInPlace updates the awaited row in place (op_date + cleared + the real
// amount — variance flows to the residual on read, the allocation is not raised);
// CreateNew inserts a cleared row; Ambiguous writes nothing (no silent guess).
func (s *Service) ReconcileCleared(ctx context.Context, userID int64, mov MovementInput, tol engine.Tolerance) (ReconcileOutcome, error) {
	var out ReconcileOutcome
	err := s.tx.WithTx(ctx, func(q repo.DBTX) error {
		if err := s.ensureEditable(ctx, q, userID, mov.Period); err != nil {
			return err
		}
		cands, err := s.awaitedCandidates(ctx, q, userID, mov.Period, false)
		if err != nil {
			return err
		}
		d := engine.Reconcile(engine.Movement{Account: mov.AccountID, Sign: mov.Sign, Amount: mov.Magnitude, Date: mov.Date}, cands, tol)
		out.Kind = d.Kind
		switch d.Kind {
		case engine.ReconcileInPlace:
			out.TxnID = d.TxnID
			return s.reconcileInPlace(ctx, q, userID, d.TxnID, mov)
		case engine.CreateNew:
			id, err := s.createCleared(ctx, q, userID, mov)
			out.TxnID = id
			return err
		case engine.Ambiguous:
			out.AmbiguousIDs = d.AmbiguousIDs
			return nil
		default:
			return nil
		}
	})
	return out, err
}

// PairInternalTransfer matches one observed transfer leg against the period's
// awaited opposite legs (engine.PairTransfer): on a single match it links the two
// legs (paired_transaction_id both ways) for internal-transfer pairing (rules
// §10, L8). The legID is the observed leg's own row.
func (s *Service) PairInternalTransfer(ctx context.Context, userID, legID int64, leg MovementInput, tol engine.Tolerance) (ReconcileOutcome, error) {
	var out ReconcileOutcome
	err := s.tx.WithTx(ctx, func(q repo.DBTX) error {
		if err := s.ensureEditable(ctx, q, userID, leg.Period); err != nil {
			return err
		}
		cands, err := s.awaitedCandidates(ctx, q, userID, leg.Period, true)
		if err != nil {
			return err
		}
		// Exclude the leg's own row from its candidate set.
		cands = filterCandidates(cands, legID)
		d := engine.PairTransfer(engine.Movement{Account: leg.AccountID, Sign: leg.Sign, Amount: leg.Magnitude, Date: leg.Date}, cands, tol)
		out.Kind = d.Kind
		switch d.Kind {
		case engine.ReconcileInPlace:
			out.TxnID = d.TxnID
			return s.linkPair(ctx, q, userID, legID, d.TxnID)
		case engine.Ambiguous:
			out.AmbiguousIDs = d.AmbiguousIDs
			return nil
		case engine.CreateNew:
			return nil // nothing to pair
		default:
			return nil
		}
	})
	return out, err
}

// awaitedCandidates loads the period's awaited transactions as engine candidates.
// transfersOnly restricts to transfer legs (for pairing).
func (s *Service) awaitedCandidates(ctx context.Context, q repo.DBTX, userID int64, period string, transfersOnly bool) ([]engine.Candidate, error) {
	txns, err := s.transactions.ListByPeriod(ctx, q, userID, period)
	if err != nil {
		return nil, err
	}
	expected := s.expectedDayMap(ctx, q, userID)
	var out []engine.Candidate
	for _, t := range txns {
		if t.Status != domain.StatusAwaited {
			continue
		}
		if transfersOnly != (t.FlowType == domain.FlowTransfer) {
			continue
		}
		out = append(out, engine.Candidate{
			TxnID:        t.ID,
			Account:      t.AccountID,
			Sign:         signOf(t.Amount),
			Amount:       absAmount(t.Amount),
			ExpectedDate: candidateDate(t, expected),
			Period:       t.BudgetPeriod,
		})
	}
	return out, nil
}

// reconcileInPlace updates the matched awaited row to cleared, adopting the real
// op_date + amount (variance → residual on read; the allocation is untouched, §7).
func (s *Service) reconcileInPlace(ctx context.Context, q repo.DBTX, userID, txnID int64, mov MovementInput) error {
	t, err := s.transactions.Get(ctx, q, userID, txnID)
	if err != nil {
		return err
	}
	d := mov.Date
	t.OpDate = &d
	t.Status = domain.StatusCleared
	t.Amount = signedAmount(t.FlowType, mov.Magnitude) // actual wins; keep the row's flow/category/account
	t.UpdatedAt = s.now().UTC()
	return s.transactions.Update(ctx, q, t)
}

func (s *Service) createCleared(ctx context.Context, q repo.DBTX, userID int64, mov MovementInput) (int64, error) {
	d := mov.Date
	now := s.now().UTC()
	return s.transactions.Create(ctx, q, &domain.Transaction{
		UserID: userID, AccountID: mov.AccountID, CategoryID: mov.CategoryID, FlowType: mov.FlowType,
		Amount: signedAmount(mov.FlowType, mov.Magnitude), OpDate: &d, BudgetPeriod: mov.Period,
		Status: domain.StatusCleared, Label: mov.Label, Source: domain.SourceManual, CreatedAt: now, UpdatedAt: now,
	})
}

// linkPair sets paired_transaction_id on both legs (atomic pairing, L8).
func (s *Service) linkPair(ctx context.Context, q repo.DBTX, userID, aID, bID int64) error {
	a, err := s.transactions.Get(ctx, q, userID, aID)
	if err != nil {
		return err
	}
	b, err := s.transactions.Get(ctx, q, userID, bID)
	if err != nil {
		return err
	}
	now := s.now().UTC()
	a.PairedTransactionID, a.UpdatedAt = &b.ID, now
	b.PairedTransactionID, b.UpdatedAt = &a.ID, now
	if err := s.transactions.Update(ctx, q, a); err != nil {
		return err
	}
	return s.transactions.Update(ctx, q, b)
}

// --- helpers ---

func signOf(amount int64) engine.Sign {
	if amount < 0 {
		return -1
	}
	return 1
}

func absAmount(amount int64) int64 {
	if amount < 0 {
		return -amount
	}
	return amount
}

// candidateDate is an awaited row's expected date: its op_date if any (rare), else
// the fixed-recurring envelope's expected day, else end-of-period.
func candidateDate(t domain.Transaction, expected map[envKey]*int) domain.Date {
	if t.OpDate != nil {
		return *t.OpDate
	}
	y, m, ok := parsePeriodYM(t.BudgetPeriod)
	if !ok {
		return domain.Date{}
	}
	if t.CategoryID != nil {
		if d, has := expected[envKey{*t.CategoryID, t.AccountID}]; has && d != nil {
			day := *d
			if dim := daysIn(y, m); day > dim {
				day = dim
			}
			return domain.NewDate(y, m, day)
		}
	}
	return domain.NewDate(y, m, daysIn(y, m))
}

func filterCandidates(cands []engine.Candidate, excludeID int64) []engine.Candidate {
	out := cands[:0]
	for _, c := range cands {
		if c.TxnID != excludeID {
			out = append(out, c)
		}
	}
	return out
}
