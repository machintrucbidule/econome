package services

import (
	"context"
	"errors"
	"testing"

	"econome/internal/domain"
)

// Journal use-case tests (increment 6c, functional/06). Real SQLite via
// newService, clock pinned. Cover create (signed amount, period derivation,
// validation, locked guard), inline update (date↔status, transfer scope),
// delete, sort/filter, and the month summary.

func catIDByName(t *testing.T, s *Service, uid int64, name string) int64 {
	t.Helper()
	_, c, err := s.GetEnvelope(context.Background(), uid, envelopeIDByName(t, s, uid, name))
	if err != nil {
		t.Fatalf("cat %q: %v", name, err)
	}
	return c.ID
}

func date(y, m, d int) *domain.Date { dt := domain.NewDate(y, m, d); return &dt }

func mkTxn(t *testing.T, s *Service, uid int64, in TxnInput) *domain.Transaction {
	t.Helper()
	tx, err := s.CreateTransaction(context.Background(), uid, in)
	if err != nil {
		t.Fatalf("create txn %q: %v", in.Label, err)
	}
	return tx
}

// journalFixture: a sweep account with income + two expense categories.
func journalFixture(t *testing.T, s *Service) (uid, sweep, carry, salaire, loyers, courses int64) {
	t.Helper()
	fixedClock(s)
	uid = miOwner(t, s)
	sweep = mkAccount(t, s, uid, "Fortuneo", "current", "sweep")
	carry = mkAccount(t, s, uid, "Boursorama", "current", "carry")
	mkEnv(t, s, uid, EnvelopeInput{Name: "Salaire", FlowType: "income", AccountID: sweep, Mode: "fixed_recurring", DefaultAmount: amt(260000), Frequency: "monthly", ExpectedDay: day(1)})
	mkEnv(t, s, uid, EnvelopeInput{Name: "Loyers", FlowType: "expense", AccountID: sweep, Mode: "fixed_recurring", DefaultAmount: amt(105000), Frequency: "monthly", ExpectedDay: day(5)})
	mkEnv(t, s, uid, EnvelopeInput{Name: "Courses", FlowType: "expense", AccountID: sweep, Mode: "variable", DefaultAmount: amt(60000)})
	salaire = catIDByName(t, s, uid, "Salaire")
	loyers = catIDByName(t, s, uid, "Loyers")
	courses = catIDByName(t, s, uid, "Courses")
	// An empty active period (no materialised rows — the journal starts blank).
	now := s.now().UTC()
	if _, err := s.periods.Create(context.Background(), s.tx.DB(), &domain.Period{UserID: uid, Period: "2026-06", State: domain.PeriodActive, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("period: %v", err)
	}
	return uid, sweep, carry, salaire, loyers, courses
}

func TestJournal_CreateSignsAndSummary(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	uid, sweep, carry, salaire, loyers, courses := journalFixture(t, s)
	const period = "2026-06"

	inc := mkTxn(t, s, uid, TxnInput{Label: "Salaire", CategoryID: &salaire, AccountID: sweep, Magnitude: 260000, Status: domain.StatusCleared, OpDate: date(2026, 6, 1)})
	if inc.Amount != 260000 || inc.FlowType != domain.FlowIncome || inc.BudgetPeriod != period {
		t.Errorf("income txn = %+v, want +260000 income period 2026-06", inc)
	}
	exp := mkTxn(t, s, uid, TxnInput{Label: "Loyer", CategoryID: &loyers, AccountID: sweep, Magnitude: 105000, Status: domain.StatusCleared, OpDate: date(2026, 6, 5)})
	if exp.Amount != -105000 || exp.FlowType != domain.FlowExpense {
		t.Errorf("expense txn = %+v, want -105000 expense", exp)
	}
	mkTxn(t, s, uid, TxnInput{Label: "Courses", CategoryID: &courses, AccountID: sweep, Magnitude: 5000, Status: domain.StatusPending, OpDate: date(2026, 6, 8)})
	mkTxn(t, s, uid, TxnInput{Label: "Vacances", CategoryID: &courses, AccountID: sweep, Magnitude: 2000, Status: domain.StatusAwaited, BudgetPeriod: "2026-06"})
	// A transfer is excluded from the summary (neutralised).
	mkTxn(t, s, uid, TxnInput{Label: "Alim", AccountID: sweep, DestAccountID: &carry, Magnitude: 24000, Status: domain.StatusCleared, FlowType: domain.FlowTransfer, OpDate: date(2026, 6, 2)})

	d, err := s.Journal(ctx, uid, period, ScopeAll, "", "", JournalFilters{IncludeTransfers: true})
	if err != nil {
		t.Fatalf("Journal: %v", err)
	}
	sm := d.Summary
	if sm.IncomeReceived != 260000 || sm.RealExpenses != 110000 {
		t.Errorf("summary income/real = %d/%d, want 260000/110000", sm.IncomeReceived, sm.RealExpenses)
	}
	if sm.Pending != 5000 || sm.PendingCount != 1 || sm.Awaited != 2000 || sm.AwaitedCount != 1 {
		t.Errorf("summary pending/awaited = %d(%d)/%d(%d), want 5000(1)/2000(1)", sm.Pending, sm.PendingCount, sm.Awaited, sm.AwaitedCount)
	}
	if sm.NetBalance != 150000 {
		t.Errorf("net = %d, want 150000", sm.NetBalance)
	}
	if len(d.Rows) != 5 {
		t.Errorf("rows = %d, want 5", len(d.Rows))
	}
}

func TestJournal_CreateValidationAndLock(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	uid, sweep, carry, _, loyers, _ := journalFixture(t, s)
	var ve *domain.ValidationError

	// amount = 0
	if _, err := s.CreateTransaction(ctx, uid, TxnInput{CategoryID: &loyers, AccountID: sweep, Magnitude: 0, Status: domain.StatusCleared}); !errors.As(err, &ve) {
		t.Errorf("amount 0 err = %v, want ValidationError", err)
	}
	// non-transfer without a category
	if _, err := s.CreateTransaction(ctx, uid, TxnInput{AccountID: sweep, Magnitude: 100, Status: domain.StatusCleared}); !errors.As(err, &ve) {
		t.Errorf("no category err = %v, want ValidationError", err)
	}
	// transfer to the same account
	if _, err := s.CreateTransaction(ctx, uid, TxnInput{AccountID: sweep, DestAccountID: &sweep, Magnitude: 100, Status: domain.StatusCleared, FlowType: domain.FlowTransfer}); !errors.As(err, &ve) {
		t.Errorf("transfer self err = %v, want ValidationError", err)
	}
	_ = carry
	// locked month
	if err := s.periods.UpdateState(ctx, s.tx.DB(), uid, "2026-06", domain.PeriodLocked, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateTransaction(ctx, uid, TxnInput{Label: "x", CategoryID: &loyers, AccountID: sweep, Magnitude: 100, Status: domain.StatusCleared, OpDate: date(2026, 6, 5)}); !errors.Is(err, domain.ErrLocked) {
		t.Errorf("locked create err = %v, want ErrLocked", err)
	}
}

func TestJournal_InlineEditDateStatusAndTransferScope(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	uid, sweep, carry, _, _, courses := journalFixture(t, s)

	awaited := mkTxn(t, s, uid, TxnInput{Label: "Abo", CategoryID: &courses, AccountID: sweep, Magnitude: 2000, Status: domain.StatusAwaited, BudgetPeriod: "2026-06"})
	// Filling the date reconciles awaited → cleared (§4).
	if err := s.UpdateTransaction(ctx, uid, awaited.ID, "op_date", "28/06"); err != nil {
		t.Fatalf("date fill: %v", err)
	}
	got, _ := s.transactions.Get(ctx, s.tx.DB(), uid, awaited.ID)
	if got.Status != domain.StatusCleared || got.OpDate == nil || got.OpDate.Day != 28 {
		t.Errorf("after date fill: %+v, want cleared 28/06", got)
	}
	// Clearing the date reverts cleared → awaited.
	if err := s.UpdateTransaction(ctx, uid, awaited.ID, "op_date", ""); err != nil {
		t.Fatalf("date clear: %v", err)
	}
	got, _ = s.transactions.Get(ctx, s.tx.DB(), uid, awaited.ID)
	if got.Status != domain.StatusAwaited || got.OpDate != nil {
		t.Errorf("after date clear: %+v, want awaited no date", got)
	}
	// Direct status edit.
	if err := s.UpdateTransaction(ctx, uid, awaited.ID, "status", "pending"); err != nil {
		t.Fatalf("status edit: %v", err)
	}
	// Amount edit re-signs by flow (expense stays negative).
	if err := s.UpdateTransaction(ctx, uid, awaited.ID, "amount", "3000"); err != nil {
		t.Fatalf("amount edit: %v", err)
	}
	got, _ = s.transactions.Get(ctx, s.tx.DB(), uid, awaited.ID)
	if got.Amount != -3000 {
		t.Errorf("amount after edit = %d, want -3000", got.Amount)
	}

	// Transfer inline scope: category/account are fixed (M23) → 409.
	xfer := mkTxn(t, s, uid, TxnInput{Label: "Alim", AccountID: sweep, DestAccountID: &carry, Magnitude: 24000, Status: domain.StatusCleared, FlowType: domain.FlowTransfer, OpDate: date(2026, 6, 2)})
	if err := s.UpdateTransaction(ctx, uid, xfer.ID, "category_id", "1"); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("transfer category edit err = %v, want ErrConflict", err)
	}
	// But its amount/label/status remain editable.
	if err := s.UpdateTransaction(ctx, uid, xfer.ID, "amount", "30000"); err != nil {
		t.Errorf("transfer amount edit: %v", err)
	}
	got, _ = s.transactions.Get(ctx, s.tx.DB(), uid, xfer.ID)
	if got.Amount != -30000 {
		t.Errorf("transfer amount = %d, want -30000 (source-signed)", got.Amount)
	}
}

func TestJournal_DeleteAndSortFilter(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	uid, sweep, _, _, loyers, courses := journalFixture(t, s)
	const period = "2026-06"

	a := mkTxn(t, s, uid, TxnInput{Label: "Loyer", CategoryID: &loyers, AccountID: sweep, Magnitude: 105000, Status: domain.StatusCleared, OpDate: date(2026, 6, 5)})
	mkTxn(t, s, uid, TxnInput{Label: "Leclerc", CategoryID: &courses, AccountID: sweep, Magnitude: 8430, Status: domain.StatusCleared, OpDate: date(2026, 6, 12)})
	mkTxn(t, s, uid, TxnInput{Label: "Acompte", CategoryID: &courses, AccountID: sweep, Magnitude: 15000, Status: domain.StatusAwaited, BudgetPeriod: "2026-06"})

	// Default sort = date descending; the undated awaited row sorts last.
	d, _ := s.Journal(ctx, uid, period, ScopeAll, "", "", JournalFilters{IncludeTransfers: true})
	if len(d.Rows) != 3 || d.Rows[0].Txn.Label != "Leclerc" || d.Rows[2].Txn.OpDate != nil {
		t.Errorf("default sort wrong: %v", labelsOf(d.Rows))
	}
	// Filter by category (Courses) → 2 rows.
	d, _ = s.Journal(ctx, uid, period, ScopeAll, "", "", JournalFilters{IncludeTransfers: true, CategoryID: &courses})
	if len(d.Rows) != 2 {
		t.Errorf("category filter = %d rows, want 2", len(d.Rows))
	}
	// Filter by status (awaited only) → 1 row.
	d, _ = s.Journal(ctx, uid, period, ScopeAll, "", "", JournalFilters{IncludeTransfers: true, Statuses: []domain.TransactionStatus{domain.StatusAwaited}})
	if len(d.Rows) != 1 || d.Rows[0].Txn.Label != "Acompte" {
		t.Errorf("status filter wrong: %v", labelsOf(d.Rows))
	}
	// Search.
	d, _ = s.Journal(ctx, uid, period, ScopeAll, "", "", JournalFilters{IncludeTransfers: true, Q: "lecl"})
	if len(d.Rows) != 1 || d.Rows[0].Txn.Label != "Leclerc" {
		t.Errorf("search wrong: %v", labelsOf(d.Rows))
	}

	// Delete the loyer row.
	if err := s.DeleteTransaction(ctx, uid, a.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	d, _ = s.Journal(ctx, uid, period, ScopeAll, "", "", JournalFilters{IncludeTransfers: true})
	if len(d.Rows) != 2 {
		t.Errorf("after delete = %d rows, want 2", len(d.Rows))
	}
}

func labelsOf(rows []JournalRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Txn.Label
	}
	return out
}
