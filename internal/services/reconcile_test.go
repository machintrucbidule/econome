package services

import (
	"context"
	"testing"

	"econome/internal/domain"
	"econome/internal/engine"
)

// Reconciliation orchestration tests (increment 6d, functional/04 §7,
// technical/09 §3) — the DB-write side of the pure engine.Reconcile decision.
// Real SQLite, clock pinned. This is the mandatory-review surface.

// awaitedTxn inserts an awaited transaction directly (the materialised shape).
func awaitedTxn(t *testing.T, s *Service, uid, acct int64, catID *int64, flow domain.FlowType, signedAmt int64) int64 {
	t.Helper()
	now := s.now().UTC()
	id, err := s.transactions.Create(context.Background(), s.tx.DB(), &domain.Transaction{
		UserID: uid, AccountID: acct, CategoryID: catID, FlowType: flow, Amount: signedAmt,
		BudgetPeriod: "2026-06", Status: domain.StatusAwaited, Source: domain.SourceManual,
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("awaited txn: %v", err)
	}
	return id
}

func countTxns(t *testing.T, s *Service, uid int64) int {
	t.Helper()
	txns, _ := s.transactions.ListByPeriod(context.Background(), s.tx.DB(), uid, "2026-06")
	return len(txns)
}

func TestReconcile_InPlaceCreateAmbiguous(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	uid, sweep, _, _, loyers, courses := journalFixture(t, s) // Loyers has expected_day 5

	// One awaited fixed expense (Loyers, −1050, expected day 5 from its envelope).
	awa := awaitedTxn(t, s, uid, sweep, &loyers, domain.FlowExpense, -105000)
	before := countTxns(t, s, uid)

	// (1) Exactly one match → ReconcileInPlace (the awaited row updated, no new row).
	mov := MovementInput{AccountID: sweep, Magnitude: 105000, Sign: -1, Date: domain.NewDate(2026, 6, 6), Period: "2026-06", FlowType: domain.FlowExpense}
	out, err := s.ReconcileCleared(ctx, uid, mov, ManualTolerance())
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if out.Kind != engine.ReconcileInPlace || out.TxnID != awa {
		t.Fatalf("outcome = %+v, want ReconcileInPlace txn %d", out, awa)
	}
	got, _ := s.transactions.Get(ctx, s.tx.DB(), uid, awa)
	if got.Status != domain.StatusCleared || got.OpDate == nil || got.OpDate.Day != 6 || got.Amount != -105000 {
		t.Errorf("reconciled row = %+v, want cleared 06/06 -105000", got)
	}
	if countTxns(t, s, uid) != before {
		t.Errorf("ReconcileInPlace must not create a duplicate (count %d → %d)", before, countTxns(t, s, uid))
	}

	// (2) Zero match → CreateNew (a cleared row inserted).
	none := MovementInput{AccountID: sweep, Magnitude: 4200, Sign: -1, Date: domain.NewDate(2026, 6, 12), Period: "2026-06", CategoryID: &courses, FlowType: domain.FlowExpense, Label: "Café"}
	out, err = s.ReconcileCleared(ctx, uid, none, ManualTolerance())
	if err != nil || out.Kind != engine.CreateNew || out.TxnID == 0 {
		t.Fatalf("zero-match outcome = %+v err=%v, want CreateNew", out, err)
	}
	created, _ := s.transactions.Get(ctx, s.tx.DB(), uid, out.TxnID)
	if created.Status != domain.StatusCleared || created.Amount != -4200 {
		t.Errorf("created row = %+v, want cleared -4200", created)
	}

	// (3) Several matches → Ambiguous (no write, ids returned).
	a1 := awaitedTxn(t, s, uid, sweep, &courses, domain.FlowExpense, -2000)
	a2 := awaitedTxn(t, s, uid, sweep, &courses, domain.FlowExpense, -2000)
	cnt := countTxns(t, s, uid)
	out, err = s.ReconcileCleared(ctx, uid, MovementInput{AccountID: sweep, Magnitude: 2000, Sign: -1, Date: domain.NewDate(2026, 6, 30), Period: "2026-06", FlowType: domain.FlowExpense}, ManualTolerance())
	if err != nil || out.Kind != engine.Ambiguous || len(out.AmbiguousIDs) != 2 {
		t.Fatalf("ambiguous outcome = %+v err=%v, want Ambiguous(2)", out, err)
	}
	if countTxns(t, s, uid) != cnt {
		t.Error("Ambiguous must not write")
	}
	_, _ = a1, a2
}

func TestReconcile_ToleranceAdoptsVariance(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	uid, sweep, _, _, loyers, _ := journalFixture(t, s)
	awa := awaitedTxn(t, s, uid, sweep, &loyers, domain.FlowExpense, -105000)

	// A small amount variance within tolerance → match, and the real amount is
	// adopted (the awaited 1050 clears at 1052; the extra reduces the residual on
	// read — the allocation is not raised).
	tol := engine.Tolerance{Amount: 500, DateWindowDays: 5}
	out, err := s.ReconcileCleared(ctx, uid, MovementInput{AccountID: sweep, Magnitude: 105200, Sign: -1, Date: domain.NewDate(2026, 6, 5), Period: "2026-06", FlowType: domain.FlowExpense}, tol)
	if err != nil || out.Kind != engine.ReconcileInPlace {
		t.Fatalf("outcome = %+v err=%v, want ReconcileInPlace", out, err)
	}
	got, _ := s.transactions.Get(ctx, s.tx.DB(), uid, awa)
	if got.Amount != -105200 {
		t.Errorf("adopted amount = %d, want -105200 (actual wins)", got.Amount)
	}
}

func TestReconcile_PairInternalTransfer(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	uid, sweep, carry, _, _, _ := journalFixture(t, s)

	// Two awaited transfer legs: out of sweep (−240) and into carry (+240).
	legOut := awaitedTxn(t, s, uid, sweep, nil, domain.FlowTransfer, -24000)
	legIn := awaitedTxn(t, s, uid, carry, nil, domain.FlowTransfer, 24000)

	out, err := s.PairInternalTransfer(ctx, uid, legOut, MovementInput{
		AccountID: sweep, Magnitude: 24000, Sign: -1, Date: domain.NewDate(2026, 6, 30), Period: "2026-06",
	}, engine.Tolerance{Amount: 0, DateWindowDays: 5})
	if err != nil || out.Kind != engine.ReconcileInPlace || out.TxnID != legIn {
		t.Fatalf("pair outcome = %+v err=%v, want ReconcileInPlace leg %d", out, err, legIn)
	}
	a, _ := s.transactions.Get(ctx, s.tx.DB(), uid, legOut)
	b, _ := s.transactions.Get(ctx, s.tx.DB(), uid, legIn)
	if a.PairedTransactionID == nil || *a.PairedTransactionID != legIn || b.PairedTransactionID == nil || *b.PairedTransactionID != legOut {
		t.Errorf("legs not linked: out=%+v in=%+v", a.PairedTransactionID, b.PairedTransactionID)
	}
}
