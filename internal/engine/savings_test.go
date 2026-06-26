package engine

import (
	"testing"

	"pgregory.net/rapid"

	"econome/internal/domain"
)

// Residual identity: savings_projected = start + Σ planned(income) − Σ planned(expense).
func TestProp_ResidualIdentity(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		start := rapid.Int64Range(-100000, 500000).Draw(t, "start")
		var envs []domain.Envelope
		var allocs []domain.Allocation
		cats := []domain.Category{{ID: 1, FlowType: domain.FlowIncome}, {ID: 2, FlowType: domain.FlowExpense}}
		var wantIncome, wantExpense int64
		n := rapid.IntRange(0, 6).Draw(t, "n")
		for i := 0; i < n; i++ {
			income := rapid.Bool().Draw(t, "income")
			planned := rapid.Int64Range(0, 80000).Draw(t, "planned")
			catID := int64(2)
			if income {
				catID = 1
				wantIncome += planned
			} else {
				wantExpense += planned
			}
			id := int64(100 + i)
			envs = append(envs, domain.Envelope{ID: id, CategoryID: catID, AccountID: 1, Mode: domain.ModeVariable})
			allocs = append(allocs, domain.Allocation{EnvelopeID: id, Period: "2026-06", PlannedAmount: planned})
		}
		in := Inputs{
			Period: "2026-06", Accounts: []domain.Account{{ID: 1, Type: domain.AccountCurrent, MonthEndPolicy: domain.PolicySweep}},
			Categories: cats, Envelopes: envs, Allocations: allocs,
			StartBalances: map[int64]int64{1: start},
		}
		got := in.Savings(1).Projected
		if got != start+wantIncome-wantExpense {
			t.Fatalf("projected = %d, want %d", got, start+wantIncome-wantExpense)
		}
	})
}

// Transfer neutrality (budget): adding a transfer leaves every envelope and
// savings_projected unchanged, while changing account balances.
func TestProp_TransferNeutralityOnBudget(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		base := expenseEnvInputs(30000, exp(domain.StatusCleared, 12000), exp(domain.StatusAwaited, 5000))
		base.Accounts = append(base.Accounts, domain.Account{ID: 2, Type: domain.AccountPassbook, MonthEndPolicy: domain.PolicyNone})
		base.StartBalances = map[int64]int64{accCurrent: 100000}
		base.Params.Today = domain.NewDate(2026, 6, 15)

		before := base.EnvelopeView(100)
		beforeProjected := base.Savings(accCurrent).Projected
		beforeBal := base.AccountBalances(accCurrent).RealBalance

		amt := rapid.Int64Range(1, 50000).Draw(t, "amt")
		dest := int64(2)
		d := domain.NewDate(2026, 6, 10)
		withTransfer := base
		withTransfer.Txns = append(append([]domain.Transaction{}, base.Txns...),
			domain.Transaction{AccountID: accCurrent, DestAccountID: &dest, FlowType: domain.FlowTransfer, Amount: -amt, BudgetPeriod: "2026-06", Status: domain.StatusCleared, OpDate: &d})

		if withTransfer.EnvelopeView(100) != before {
			t.Fatal("transfer changed an envelope view")
		}
		if withTransfer.Savings(accCurrent).Projected != beforeProjected {
			t.Fatal("transfer changed savings_projected")
		}
		if withTransfer.AccountBalances(accCurrent).RealBalance == beforeBal {
			t.Fatal("transfer should change the source balance")
		}
	})
}

func TestSecuredBasisAndLEProjected(t *testing.T) {
	all := securedInputs(domain.BasisAllPlanned)
	allView := all.Savings(1)
	if allView.Secured != 166000 || allView.Projected < allView.Secured {
		t.Errorf("all_planned: secured=%d projected=%d (want secured 166000 <= projected)", allView.Secured, allView.Projected)
	}
	fixed := securedInputs(domain.BasisFixedOnly).Savings(1)
	if fixed.Secured != 187000 {
		t.Errorf("fixed_only secured = %d, want 187000", fixed.Secured)
	}
	if fixed.Secured <= allView.Secured {
		t.Error("fixed_only should be >= all_planned")
	}
}

func TestCascadeTarget(t *testing.T) {
	p := func(n int) *int { return &n }
	cap1 := int64(1000000)
	base := func(accounts []domain.Account, snaps []domain.Snapshot) Inputs {
		return Inputs{Period: "2026-06", Accounts: accounts, Snapshots: snaps, Params: Params{Today: domain.NewDate(2026, 6, 15)}}
	}

	// Two vehicles; the lower fill_priority non-full one is the target.
	accs := []domain.Account{
		{ID: 1, Type: domain.AccountCurrent, MonthEndPolicy: domain.PolicySweep},
		{ID: 2, Type: domain.AccountPassbook, MonthEndPolicy: domain.PolicyNone, FillPriority: p(1), Ceiling: &cap1},
		{ID: 3, Type: domain.AccountPassbook, MonthEndPolicy: domain.PolicyNone, FillPriority: p(2)},
	}
	// Vehicle 2 is full (snapshot at ceiling) ⇒ target is vehicle 3.
	full := base(accs, []domain.Snapshot{{AccountID: 2, Period: "2026-06", GrossValue: 1000000}})
	got := full.Savings(1)
	if got.CascadeTargetID == nil || *got.CascadeTargetID != 3 {
		t.Errorf("target = %v, want 3", got.CascadeTargetID)
	}
	if got.CascadeFull {
		t.Error("not all full")
	}

	// All full ⇒ no target + flag (never exceed a ceiling, C4).
	cap3 := int64(500)
	accsFull := []domain.Account{
		{ID: 1, Type: domain.AccountCurrent, MonthEndPolicy: domain.PolicySweep},
		{ID: 2, Type: domain.AccountPassbook, FillPriority: p(1), Ceiling: &cap1},
		{ID: 3, Type: domain.AccountPassbook, FillPriority: p(2), Ceiling: &cap3},
	}
	allFull := base(accsFull, []domain.Snapshot{
		{AccountID: 2, Period: "2026-06", GrossValue: 1000000},
		{AccountID: 3, Period: "2026-06", GrossValue: 500},
	})
	gf := allFull.Savings(1)
	if gf.CascadeTargetID != nil || !gf.CascadeFull {
		t.Errorf("all full: target=%v full=%v, want nil/true", gf.CascadeTargetID, gf.CascadeFull)
	}

	// No cascade vehicles ⇒ no target, not flagged full.
	none := base([]domain.Account{{ID: 1, Type: domain.AccountCurrent, MonthEndPolicy: domain.PolicySweep}}, nil)
	gn := none.Savings(1)
	if gn.CascadeTargetID != nil || gn.CascadeFull {
		t.Error("no vehicles should give nil target, not full")
	}
}

func TestResidualNegativeFlag(t *testing.T) {
	in := Inputs{
		Period: "2026-06", Accounts: []domain.Account{{ID: 1, Type: domain.AccountCurrent, MonthEndPolicy: domain.PolicySweep}},
		Categories:    []domain.Category{{ID: 2, FlowType: domain.FlowExpense}},
		Envelopes:     []domain.Envelope{{ID: 1, CategoryID: 2, AccountID: 1, Mode: domain.ModeVariable}},
		Allocations:   []domain.Allocation{{EnvelopeID: 1, Period: "2026-06", PlannedAmount: 50000}},
		StartBalances: map[int64]int64{1: 10000}, // 100 € start, 500 € planned spend ⇒ projected −400 €
	}
	if !in.Savings(1).ResidualNegative {
		t.Error("projected should be negative ⇒ residual-negative flag")
	}
}
