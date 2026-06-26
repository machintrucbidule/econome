package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"econome/internal/domain"
)

// Forecast read-model tests (increment 6a, functional/05). Real SQLite via
// newService; the clock is pinned mid-period so balances/low-point are
// deterministic. They prove the read-model matches the pure engine and that the
// hierarchy / scope variants / states are assembled correctly.

func fixedClock(s *Service) {
	s.now = func() time.Time { return time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC) }
}

// clearTxn inserts a cleared transaction for an envelope's (category, account)
// dated within the period (so it counts towards real_balance).
func clearTxn(t *testing.T, s *Service, uid, envID, amount int64, dom int, flow domain.FlowType) {
	t.Helper()
	ctx := context.Background()
	e, _, err := s.GetEnvelope(ctx, uid, envID)
	if err != nil {
		t.Fatalf("get envelope %d: %v", envID, err)
	}
	d := domain.NewDate(2026, 6, dom)
	cat := e.CategoryID
	now := s.now().UTC()
	if _, err := s.transactions.Create(ctx, s.tx.DB(), &domain.Transaction{
		UserID: uid, AccountID: e.AccountID, CategoryID: &cat, FlowType: flow,
		Amount: amount, OpDate: &d, BudgetPeriod: "2026-06", Status: domain.StatusCleared,
		Label: "test", Source: domain.SourceManual, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create txn: %v", err)
	}
}

// forecastFixture builds a sweep + carry pair with income, fixed/variable
// expenses, a parent category with two children, a carry expense and an internal
// transfer, then creates the month and clears a few movements.
func forecastFixture(t *testing.T, s *Service) (uid, sweep, carry, courses, divers int64) {
	t.Helper()
	ctx := context.Background()
	uid = miOwner(t, s)
	sweep = mkAccount(t, s, uid, "Fortuneo", "current", "sweep")
	carry = mkAccount(t, s, uid, "Boursorama", "current", "carry")

	mkEnv(t, s, uid, EnvelopeInput{Name: "Salaire", FlowType: "income", AccountID: sweep, Mode: "fixed_recurring", DefaultAmount: amt(260000), Frequency: "monthly", ExpectedDay: day(1)})
	mkEnv(t, s, uid, EnvelopeInput{Name: "Loyers", FlowType: "expense", AccountID: sweep, Mode: "fixed_recurring", DefaultAmount: amt(105000), Frequency: "monthly", ExpectedDay: day(5)})
	courses = mkEnv(t, s, uid, EnvelopeInput{Name: "Courses", FlowType: "expense", AccountID: sweep, Mode: "variable", DefaultAmount: amt(60000)})
	divers = mkEnv(t, s, uid, EnvelopeInput{Name: "Divers", FlowType: "expense", AccountID: sweep, Mode: "variable", DefaultAmount: amt(9000)})
	// Parent "Assurance" with two child envelopes.
	mkEnv(t, s, uid, EnvelopeInput{Name: "Habitation", NewParentName: "Assurance", FlowType: "expense", AccountID: sweep, Mode: "fixed_recurring", DefaultAmount: amt(3000), Frequency: "monthly", ExpectedDay: day(8)})
	mkEnv(t, s, uid, EnvelopeInput{Name: "Auto", NewParentName: "Assurance", FlowType: "expense", AccountID: sweep, Mode: "fixed_recurring", DefaultAmount: amt(7000), Frequency: "monthly", ExpectedDay: day(8)})
	// Carry-account expense.
	mkEnv(t, s, uid, EnvelopeInput{Name: "Restaurant", FlowType: "expense", AccountID: carry, Mode: "variable", DefaultAmount: amt(12000)})
	// Internal transfer sweep → carry.
	mkEnv(t, s, uid, EnvelopeInput{Name: "Alimentation CC", FlowType: "transfer", AccountID: sweep, DestAccountID: &carry, Mode: "fixed_recurring", DefaultAmount: amt(24000), Frequency: "monthly", ExpectedDay: day(2)})

	if err := s.CreateMonth(ctx, uid, "2026-06", nil); err != nil {
		t.Fatalf("CreateMonth: %v", err)
	}
	// Realise some movements: salary received, rent paid, courses partial, divers overrun.
	clearTxn(t, s, uid, envIDByName(t, s, uid, "Salaire"), 260000, 1, domain.FlowIncome)
	clearTxn(t, s, uid, envIDByName(t, s, uid, "Loyers"), -105000, 5, domain.FlowExpense)
	clearTxn(t, s, uid, courses, -33000, 10, domain.FlowExpense)
	clearTxn(t, s, uid, divers, -11000, 12, domain.FlowExpense)
	return uid, sweep, carry, courses, divers
}

func envIDByName(t *testing.T, s *Service, uid int64, name string) int64 {
	return envelopeIDByName(t, s, uid, name)
}

func rowByName(rows []ForecastRow, name string) (ForecastRow, bool) {
	for _, r := range rows {
		if r.Name == name {
			return r, true
		}
		if child, ok := rowByName(r.Children, name); ok {
			return child, true
		}
	}
	return ForecastRow{}, false
}

func TestForecast_NotCreatedState(t *testing.T) {
	s := newService(t)
	fixedClock(s)
	ctx := context.Background()
	uid := miOwner(t, s)
	mkAccount(t, s, uid, "Fortuneo", "current", "sweep")

	d, err := s.Forecast(ctx, uid, "2026-06", ScopeAll)
	if err != nil {
		t.Fatalf("Forecast: %v", err)
	}
	if d.Exists {
		t.Error("Exists should be false for a not-created month")
	}
	if len(d.Accounts) != 1 {
		t.Errorf("Accounts = %d, want 1 (rail still populated)", len(d.Accounts))
	}
	if len(d.Rows) != 0 {
		t.Errorf("not-created month should have no rows, got %d", len(d.Rows))
	}
}

func TestForecast_RowsMatchEngineAndRollup(t *testing.T) {
	s := newService(t)
	fixedClock(s)
	ctx := context.Background()
	uid, sweep, _, courses, divers := forecastFixture(t, s)

	d, err := s.Forecast(ctx, uid, "2026-06", idStr(sweep))
	if err != nil {
		t.Fatalf("Forecast: %v", err)
	}
	if !d.Exists || d.Locked {
		t.Fatalf("expected an active, created month: %+v", d)
	}

	// Courses: partial (330 of 600).
	cr, ok := rowByName(d.Rows, "Courses")
	if !ok {
		t.Fatal("Courses row missing")
	}
	if cr.State != domain.StatePartial || cr.Real != 33000 || cr.Remaining != 27000 || cr.Percent != 55 {
		t.Errorf("Courses = %+v, want partial real 33000 remaining 27000 55%%", cr)
	}
	// Divers: overrun (110 of 90).
	dv, _ := rowByName(d.Rows, "Divers")
	if dv.State != domain.StateOverrun || dv.Real != 11000 || dv.Remaining != -2000 {
		t.Errorf("Divers = %+v, want overrun real 11000 remaining -2000", dv)
	}
	// Salaire: income row, received.
	sal, _ := rowByName(d.Rows, "Salaire")
	if !sal.Income || sal.Real != 260000 {
		t.Errorf("Salaire = %+v, want income received 260000", sal)
	}

	// The leaf figures equal the pure engine's EnvelopeView (no re-derivation).
	in, err := s.engineInputs(ctx, s.tx.DB(), uid, "2026-06")
	if err != nil {
		t.Fatal(err)
	}
	ev := in.EnvelopeView(courses)
	if cr.Real != ev.Real || cr.Remaining != ev.Remaining || cr.State != ev.State {
		t.Errorf("Courses row != engine view: row=%+v view=%+v", cr, ev)
	}
	_ = divers

	// Parent rollup: Assurance = sum of its two children (30 + 70), badge agrégé.
	as, ok := rowByName(d.Rows, "Assurance")
	if !ok || !as.IsParent {
		t.Fatalf("Assurance parent missing/not parent: %+v", as)
	}
	if as.Planned != 10000 || len(as.Children) != 2 {
		t.Errorf("Assurance rollup planned = %d (children %d), want 10000 / 2", as.Planned, len(as.Children))
	}

	// Footer total sums expense envelopes only (income excluded).
	// loyers 1050 + courses 600 + divers 90 + assurance 100 = 1840.
	if d.Total.Planned != 184000 {
		t.Errorf("total planned = %d, want 184000 (expense only)", d.Total.Planned)
	}

	// Transfer envelope is not a budget row.
	if _, ok := rowByName(d.Rows, "Alimentation CC"); ok {
		t.Error("transfer envelope must be excluded from the forecast table")
	}

	// Watch list surfaces the Divers overrun.
	var sawOverrun bool
	for _, wch := range d.Watch {
		if wch.Kind == "overrun" && wch.Label == "Divers" {
			sawOverrun = true
		}
	}
	if !sawOverrun {
		t.Errorf("watch should surface the Divers overrun, got %+v", d.Watch)
	}
}

func TestForecast_EditAllocationRecomputes(t *testing.T) {
	s := newService(t)
	fixedClock(s)
	ctx := context.Background()
	uid, sweep, _, courses, _ := forecastFixture(t, s)

	// Baseline projected residual (planned income − planned expenses).
	before, _ := s.Forecast(ctx, uid, "2026-06", idStr(sweep))
	if before.Encarts[0].Projected != 76000 {
		t.Fatalf("baseline projected = %d, want 76000", before.Encarts[0].Projected)
	}

	// Raise Courses 600 → 1000: the allocation upserts and the residual drops.
	if err := s.EditAllocation(ctx, uid, courses, "2026-06", 100000); err != nil {
		t.Fatalf("EditAllocation: %v", err)
	}
	a, err := s.allocations.ByEnvelopePeriod(ctx, s.tx.DB(), uid, courses, "2026-06")
	if err != nil || a.PlannedAmount != 100000 {
		t.Fatalf("allocation = %+v err=%v, want planned 100000", a, err)
	}
	after, _ := s.Forecast(ctx, uid, "2026-06", idStr(sweep))
	if after.Encarts[0].Projected != 36000 {
		t.Errorf("projected after edit = %d, want 36000", after.Encarts[0].Projected)
	}
	cr, _ := rowByName(after.Rows, "Courses")
	if cr.Planned != 100000 {
		t.Errorf("Courses planned row = %d, want 100000", cr.Planned)
	}
}

func TestForecast_EditAllocationValidationAndLock(t *testing.T) {
	s := newService(t)
	fixedClock(s)
	ctx := context.Background()
	uid, sweep, _, courses, _ := forecastFixture(t, s)

	// Negative amount → typed 422, no write.
	var ve *domain.ValidationError
	if err := s.EditAllocation(ctx, uid, courses, "2026-06", -1); !errors.As(err, &ve) {
		t.Errorf("negative amount err = %v, want ValidationError", err)
	}
	// Residual envelope → 422 (its amount is computed).
	res := mkEnv(t, s, uid, EnvelopeInput{Name: "Résidu", FlowType: "expense", AccountID: sweep, Mode: "residual"})
	if err := s.EditAllocation(ctx, uid, res, "2026-06", 1000); !errors.As(err, &ve) {
		t.Errorf("residual edit err = %v, want ValidationError", err)
	}
	// Locked month → ErrLocked (409).
	if err := s.periods.UpdateState(ctx, s.tx.DB(), uid, "2026-06", domain.PeriodLocked, nil); err != nil {
		t.Fatalf("lock: %v", err)
	}
	if err := s.EditAllocation(ctx, uid, courses, "2026-06", 50000); !errors.Is(err, domain.ErrLocked) {
		t.Errorf("locked edit err = %v, want ErrLocked", err)
	}
}

func TestForecast_EndOfMonthTransfer(t *testing.T) {
	s := newService(t)
	fixedClock(s)
	ctx := context.Background()
	uid, sweep, _, _, _ := forecastFixture(t, s)

	// No cascade vehicle yet → nothing to sweep into.
	if err := s.EndOfMonthTransfer(ctx, uid, sweep, "2026-06"); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("transfer with no cascade target err = %v, want ErrConflict", err)
	}

	// Add a passbook to the cascade, then sweep the realised residual (to_save).
	livret := mkAccount(t, s, uid, "Livret A", "passbook", "none")
	if err := s.ReorderCascade(ctx, uid, []int64{livret}); err != nil {
		t.Fatalf("cascade: %v", err)
	}
	if err := s.EndOfMonthTransfer(ctx, uid, sweep, "2026-06"); err != nil {
		t.Fatalf("EndOfMonthTransfer: %v", err)
	}
	txns, _ := s.transactions.ListByPeriod(ctx, s.tx.DB(), uid, "2026-06")
	var found bool
	for _, tx := range txns {
		if tx.FlowType == domain.FlowTransfer && tx.AccountID == sweep && tx.DestAccountID != nil && *tx.DestAccountID == livret &&
			tx.Amount == -111000 && tx.Status == domain.StatusCleared && tx.OpDate != nil {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a cleared sweep→livret transfer of -111000, got %+v", txns)
	}
	// After the sweep, to_save is realised away (≈ 0).
	d, _ := s.Forecast(ctx, uid, "2026-06", idStr(sweep))
	if d.Encarts[0].ToSave != 0 {
		t.Errorf("to_save after sweep = %d, want 0", d.Encarts[0].ToSave)
	}

	// A locked month refuses the transfer.
	if err := s.periods.UpdateState(ctx, s.tx.DB(), uid, "2026-06", domain.PeriodLocked, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.EndOfMonthTransfer(ctx, uid, sweep, "2026-06"); !errors.Is(err, domain.ErrLocked) {
		t.Errorf("locked transfer err = %v, want ErrLocked", err)
	}
}

func TestForecast_ScopeVariants(t *testing.T) {
	s := newService(t)
	fixedClock(s)
	ctx := context.Background()
	uid, sweep, carry, _, _ := forecastFixture(t, s)

	// Sweep scope: a residual savings encart, no carry note.
	sw, err := s.Forecast(ctx, uid, "2026-06", idStr(sweep))
	if err != nil {
		t.Fatal(err)
	}
	if sw.ScopeKind != "sweep" || len(sw.Encarts) != 1 || sw.CarryNote {
		t.Errorf("sweep scope: kind=%s encarts=%d carryNote=%v", sw.ScopeKind, len(sw.Encarts), sw.CarryNote)
	}
	if !sw.Figures.HasLowPoint {
		t.Error("sweep figures should include a low point")
	}

	// Carry scope: carry note, no savings encart, a projected-end figure.
	cy, err := s.Forecast(ctx, uid, "2026-06", idStr(carry))
	if err != nil {
		t.Fatal(err)
	}
	if cy.ScopeKind != "carry" || !cy.CarryNote || len(cy.Encarts) != 0 {
		t.Errorf("carry scope: kind=%s carryNote=%v encarts=%d", cy.ScopeKind, cy.CarryNote, len(cy.Encarts))
	}
	if !cy.Figures.HasProjectedEnd || !cy.Figures.HasIncomingXfer {
		t.Error("carry figures should include projected-end + incoming transfer")
	}
	// The internal transfer funds the carry account (+240 incoming).
	if cy.Figures.IncomingXfer != 24000 {
		t.Errorf("incoming transfer = %d, want 24000", cy.Figures.IncomingXfer)
	}

	// Aggregated scope: flat rows with account pills + masked-transfers footer.
	ag, err := s.Forecast(ctx, uid, "2026-06", ScopeAll)
	if err != nil {
		t.Fatal(err)
	}
	if ag.ScopeKind != "aggregated" || !ag.HasHiddenTransfers {
		t.Errorf("aggregated: kind=%s hiddenTransfers=%v", ag.ScopeKind, ag.HasHiddenTransfers)
	}
	var pilled bool
	for _, r := range ag.Rows {
		if r.ShowPill && r.AccountName != "" {
			pilled = true
		}
		if r.IsParent {
			t.Error("aggregated scope should be flat (no parent rows)")
		}
	}
	if !pilled {
		t.Error("aggregated rows should carry account pills")
	}
	// One savings encart for the single sweep account.
	if len(ag.Encarts) != 1 {
		t.Errorf("aggregated encarts = %d, want 1 (one sweep account)", len(ag.Encarts))
	}
}
