package services

import (
	"context"
	"errors"
	"testing"

	"econome/internal/domain"
)

// Month-initialisation assistant service tests (increment 5, functional/09). Run
// against real SQLite via newService. Being in package services, they read the
// persisted state through the unexported repos to prove what CreateMonth wrote.

func miOwner(t *testing.T, s *Service) int64 {
	t.Helper()
	res, err := s.Setup(context.Background(), validSetup())
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	return res.User.ID
}

func mkAccount(t *testing.T, s *Service, uid int64, name, typ, policy string) int64 {
	t.Helper()
	a, err := s.CreateAccount(context.Background(), uid, AccountInput{Name: name, Type: typ, MonthEndPolicy: policy})
	if err != nil {
		t.Fatalf("create account %s: %v", name, err)
	}
	return a.ID
}

func mkEnv(t *testing.T, s *Service, uid int64, in EnvelopeInput) int64 {
	t.Helper()
	e, err := s.CreateEnvelope(context.Background(), uid, in)
	if err != nil {
		t.Fatalf("create envelope %s: %v", in.Name, err)
	}
	return e.ID
}

func amt(v int64) *int64 { return &v }
func day(v int) *int     { return &v }

// stdFixture: one sweep current account with a fixed income, a fixed expense and a
// variable expense. Projected residual = 2 600 − (1 050 + 600) = 950 €.
func stdFixture(t *testing.T, s *Service) (uid, sweep, coursesEnv int64) {
	t.Helper()
	uid = miOwner(t, s)
	sweep = mkAccount(t, s, uid, "Fortuneo", "current", "sweep")
	mkEnv(t, s, uid, EnvelopeInput{Name: "Salaire", FlowType: "income", AccountID: sweep, Mode: "fixed_recurring", DefaultAmount: amt(260000), Frequency: "monthly", ExpectedDay: day(27)})
	mkEnv(t, s, uid, EnvelopeInput{Name: "Loyers", FlowType: "expense", AccountID: sweep, Mode: "fixed_recurring", DefaultAmount: amt(105000), Frequency: "monthly", ExpectedDay: day(5)})
	coursesEnv = mkEnv(t, s, uid, EnvelopeInput{Name: "Courses", FlowType: "expense", AccountID: sweep, Mode: "variable", DefaultAmount: amt(60000)})
	return uid, sweep, coursesEnv
}

func TestMonthInit_CreateMaterialisesInOneTx(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	uid, sweep, _ := stdFixture(t, s)
	const period = "2026-06"

	// Nothing persisted before creation (the draft is non-persisted, T3i).
	if allocs, _ := s.allocations.ListByPeriod(ctx, s.tx.DB(), uid, period); len(allocs) != 0 {
		t.Fatalf("draft must persist nothing before create, got %d allocations", len(allocs))
	}
	if _, err := s.periods.ByPeriod(ctx, s.tx.DB(), uid, period); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("period must not exist before create, err=%v", err)
	}

	if err := s.CreateMonth(ctx, uid, period, nil); err != nil {
		t.Fatalf("CreateMonth: %v", err)
	}

	// 3 allocations (income + fixed expense + variable expense).
	allocs, _ := s.allocations.ListByPeriod(ctx, s.tx.DB(), uid, period)
	if len(allocs) != 3 {
		t.Errorf("allocations = %d, want 3", len(allocs))
	}
	// 2 awaited transactions (fixed income + fixed expense; the variable has none).
	txns, _ := s.transactions.ListByPeriod(ctx, s.tx.DB(), uid, period)
	if len(txns) != 2 {
		t.Fatalf("transactions = %d, want 2", len(txns))
	}
	for _, tx := range txns {
		if tx.Status != domain.StatusAwaited || tx.Source != domain.SourceManual || tx.OpDate != nil {
			t.Errorf("txn %q: want awaited/manual/no-date, got %s/%s/%v", tx.Label, tx.Status, tx.Source, tx.OpDate)
		}
	}
	// The fixed expense awaited amount is negative (expense), the income positive.
	var sawNeg, sawPos bool
	for _, tx := range txns {
		if tx.AccountID == sweep && tx.Amount < 0 {
			sawNeg = true
		}
		if tx.AccountID == sweep && tx.Amount > 0 {
			sawPos = true
		}
	}
	if !sawNeg || !sawPos {
		t.Errorf("want a negative expense and a positive income awaited txn (neg=%v pos=%v)", sawNeg, sawPos)
	}

	// Period row active + a `create` audit event.
	p, err := s.periods.ByPeriod(ctx, s.tx.DB(), uid, period)
	if err != nil || p.State != domain.PeriodActive {
		t.Fatalf("period row: %+v err=%v", p, err)
	}
	evs, _ := s.periodEvents.ListByPeriod(ctx, s.tx.DB(), uid, period)
	if len(evs) != 1 || evs[0].Action != domain.ActionCreate {
		t.Errorf("period events = %+v, want one create", evs)
	}
}

func TestMonthInit_AlreadyCreatedRejected(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	uid, _, _ := stdFixture(t, s)
	const period = "2026-06"
	if err := s.CreateMonth(ctx, uid, period, nil); err != nil {
		t.Fatalf("first create: %v", err)
	}
	// A second creation (an already-created — incl. a locked — month) is refused.
	if err := s.CreateMonth(ctx, uid, period, nil); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("second create err = %v, want ErrConflict", err)
	}
	if created, _ := s.IsCreated(ctx, uid, period); !created {
		t.Error("IsCreated must report true after creation")
	}
}

func TestMonthInit_DraftResidualMatchesEngine(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	uid, sweep, courses := stdFixture(t, s)
	const period = "2026-06"

	d, err := s.BuildDraft(ctx, uid, period, nil)
	if err != nil {
		t.Fatalf("BuildDraft: %v", err)
	}
	sv := d.SavingsBySweep[sweep]
	if sv.Projected != 95000 {
		t.Errorf("projected residual = %d, want 95000", sv.Projected)
	}
	if sv.ResidualNegative {
		t.Error("residual must be positive in the base draft")
	}

	// Raising the variable expense to 2 000 € drives the residual negative.
	neg, err := s.BuildDraft(ctx, uid, period, map[int64]int64{courses: 200000})
	if err != nil {
		t.Fatalf("BuildDraft override: %v", err)
	}
	sv = neg.SavingsBySweep[sweep]
	if sv.Projected != -45000 || !sv.ResidualNegative {
		t.Errorf("override residual = %d (neg=%v), want -45000 negative", sv.Projected, sv.ResidualNegative)
	}
}

func TestMonthInit_TransferGeneratesAwaitedWithDest(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	uid := miOwner(t, s)
	f := mkAccount(t, s, uid, "Fortuneo", "current", "sweep")
	b := mkAccount(t, s, uid, "Boursorama", "current", "carry")
	mkEnv(t, s, uid, EnvelopeInput{
		Name: "Alimentation CC", FlowType: "transfer", AccountID: f, DestAccountID: &b,
		Mode: "fixed_recurring", DefaultAmount: amt(24000), Frequency: "monthly", ExpectedDay: day(1),
	})
	const period = "2026-06"
	if err := s.CreateMonth(ctx, uid, period, nil); err != nil {
		t.Fatalf("CreateMonth: %v", err)
	}
	txns, _ := s.transactions.ListByPeriod(ctx, s.tx.DB(), uid, period)
	if len(txns) != 1 {
		t.Fatalf("transactions = %d, want 1 awaited transfer", len(txns))
	}
	tx := txns[0]
	if tx.FlowType != domain.FlowTransfer || tx.AccountID != f || tx.DestAccountID == nil || *tx.DestAccountID != b || tx.CategoryID != nil {
		t.Errorf("transfer txn wrong: flow=%s acc=%d dest=%v cat=%v", tx.FlowType, tx.AccountID, tx.DestAccountID, tx.CategoryID)
	}
	// A transfer is excluded from the budget → no allocation generated.
	if allocs, _ := s.allocations.ListByPeriod(ctx, s.tx.DB(), uid, period); len(allocs) != 0 {
		t.Errorf("transfer must not create an allocation, got %d", len(allocs))
	}
}

func TestMonthInit_NonMonthlyOnlyOnDueMonth(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	uid := miOwner(t, s)
	f := mkAccount(t, s, uid, "Fortuneo", "current", "sweep")
	mkEnv(t, s, uid, EnvelopeInput{
		Name: "Assurance auto", FlowType: "expense", AccountID: f, Mode: "fixed_recurring",
		DefaultAmount: amt(9600), Frequency: "quarterly", DueMonths: []int{1, 4, 7, 10}, ExpectedDay: day(12),
	})

	// June (6) is not a due month → empty draft.
	jun, _ := s.BuildDraft(ctx, uid, "2026-06", nil)
	if len(jun.Posts) != 0 {
		t.Errorf("June posts = %d, want 0 (not a due month)", len(jun.Posts))
	}
	// July (7) is a due month → one post.
	jul, _ := s.BuildDraft(ctx, uid, "2026-07", nil)
	if len(jul.Posts) != 1 {
		t.Errorf("July posts = %d, want 1 (due month)", len(jul.Posts))
	}
}

func TestMonthInit_ArchivedEnvelopeExcluded(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	uid := miOwner(t, s)
	f := mkAccount(t, s, uid, "Fortuneo", "current", "sweep")
	env := mkEnv(t, s, uid, EnvelopeInput{Name: "Vieux poste", FlowType: "expense", AccountID: f, Mode: "variable", DefaultAmount: amt(5000)})

	if d, _ := s.BuildDraft(ctx, uid, "2026-06", nil); len(d.Posts) != 1 {
		t.Fatalf("active posts = %d, want 1", len(d.Posts))
	}
	if err := s.ArchiveEnvelope(ctx, uid, env); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if d, _ := s.BuildDraft(ctx, uid, "2026-06", nil); len(d.Posts) != 0 {
		t.Errorf("archived envelope must be excluded from the draft, got %d posts", len(d.Posts))
	}
}

func TestMonthInit_FixedExpenseAllocationAndAwaitedAmountsMatch(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	uid, _, _ := stdFixture(t, s)
	const period = "2026-06"
	// Override the fixed expense via the draft amounts the user validated: the
	// adjusted amount must flow to both the allocation and the awaited txn.
	loyers := envelopeIDByName(t, s, uid, "Loyers")
	if err := s.CreateMonth(ctx, uid, period, map[int64]int64{loyers: 110000}); err != nil {
		t.Fatalf("CreateMonth: %v", err)
	}
	alloc, err := s.allocations.ByEnvelopePeriod(ctx, s.tx.DB(), uid, loyers, period)
	if err != nil || alloc.PlannedAmount != 110000 {
		t.Fatalf("loyers allocation = %+v err=%v, want planned 110000", alloc, err)
	}
	var found bool
	txns, _ := s.transactions.ListByPeriod(ctx, s.tx.DB(), uid, period)
	for _, tx := range txns {
		if tx.CategoryID != nil && tx.Amount == -110000 {
			found = true
		}
	}
	if !found {
		t.Error("awaited fixed-expense txn should carry the adjusted amount -110000")
	}
}

// --- transfer-destination validation (T11) ---

func TestEnvelope_DestValidation(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	uid := miOwner(t, s)
	f := mkAccount(t, s, uid, "Fortuneo", "current", "sweep")
	livret := mkAccount(t, s, uid, "Livret A", "passbook", "none")

	cases := []struct {
		name  string
		in    EnvelopeInput
		field string
	}{
		{"transfer without dest", EnvelopeInput{Name: "Vir", FlowType: "transfer", AccountID: f, Mode: "fixed_recurring", DefaultAmount: amt(1000), Frequency: "monthly"}, "dest_account_id"},
		{"transfer to self", EnvelopeInput{Name: "Vir", FlowType: "transfer", AccountID: f, DestAccountID: &f, Mode: "fixed_recurring", DefaultAmount: amt(1000), Frequency: "monthly"}, "dest_account_id"},
		{"transfer to savings", EnvelopeInput{Name: "Vir", FlowType: "transfer", AccountID: f, DestAccountID: &livret, Mode: "fixed_recurring", DefaultAmount: amt(1000), Frequency: "monthly"}, "dest_account_id"},
		{"dest on expense", EnvelopeInput{Name: "Courses", FlowType: "expense", AccountID: f, DestAccountID: &livret, Mode: "variable", DefaultAmount: amt(1000)}, "dest_account_id"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := s.CreateEnvelope(ctx, uid, c.in)
			var ve *domain.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("want ValidationError, got %v", err)
			}
			if ve.Fields[0].Field != c.field {
				t.Errorf("field = %q, want %q", ve.Fields[0].Field, c.field)
			}
		})
	}

	// A valid transfer to another current account succeeds and round-trips the dest.
	b := mkAccount(t, s, uid, "Boursorama", "current", "carry")
	e, err := s.CreateEnvelope(ctx, uid, EnvelopeInput{Name: "Alim", FlowType: "transfer", AccountID: f, DestAccountID: &b, Mode: "fixed_recurring", DefaultAmount: amt(24000), Frequency: "monthly", ExpectedDay: day(1)})
	if err != nil {
		t.Fatalf("valid transfer envelope: %v", err)
	}
	got, _, err := s.GetEnvelope(ctx, uid, e.ID)
	if err != nil || got.DestAccountID == nil || *got.DestAccountID != b {
		t.Errorf("dest not persisted: %+v err=%v", got.DestAccountID, err)
	}
}

func envelopeIDByName(t *testing.T, s *Service, uid int64, name string) int64 {
	t.Helper()
	ov, err := s.EnvelopesOverview(context.Background(), uid, true)
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	for _, r := range ov.TopLevel {
		if r.Category.Name == name {
			return r.Envelope.ID
		}
	}
	for _, g := range ov.Parents {
		for _, r := range g.Children {
			if r.Category.Name == name {
				return r.Envelope.ID
			}
		}
	}
	t.Fatalf("envelope %q not found", name)
	return 0
}
