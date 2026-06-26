package engine

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"econome/internal/domain"
)

var update = flag.Bool("update", false, "update golden files in testdata/")

// goldenCheck compares a computed output against testdata/<name>.json. The
// inputs are built in Go (clearer than JSON); only the expected output is the
// golden artefact (I-016). `go test -run Golden -update` regenerates them.
func goldenCheck(t *testing.T, name string, got any) {
	t.Helper()
	path := filepath.Join("testdata", name+".json")
	data, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatalf("marshal %s: %v", name, err)
	}
	data = append(data, '\n')
	if *update {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatalf("write golden %s: %v", path, err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s (run with -update first): %v", path, err)
	}
	if string(want) != string(data) {
		t.Errorf("golden %s mismatch:\n got: %s\nwant: %s", name, data, want)
	}
}

// securedInputs reproduces functional/03 §7: sweep real balance 2 000 €, a fixed
// expense with 130 € remaining and a variable expense with 210 € remaining.
func securedInputs(basis domain.SecuredSavingsBasis) Inputs {
	const fixedCat, varCat int64 = 20, 21
	return Inputs{
		Period:   "2026-06",
		Accounts: []domain.Account{{ID: 1, Type: domain.AccountCurrent, MonthEndPolicy: domain.PolicySweep}},
		Categories: []domain.Category{
			{ID: fixedCat, FlowType: domain.FlowExpense},
			{ID: varCat, FlowType: domain.FlowExpense},
		},
		Envelopes: []domain.Envelope{
			{ID: 1, CategoryID: fixedCat, AccountID: 1, Mode: domain.ModeFixedRecurring},
			{ID: 2, CategoryID: varCat, AccountID: 1, Mode: domain.ModeVariable},
		},
		Allocations: []domain.Allocation{
			{EnvelopeID: 1, Period: "2026-06", PlannedAmount: 13000}, // 130 € remaining (no spend)
			{EnvelopeID: 2, Period: "2026-06", PlannedAmount: 21000}, // 210 € remaining
		},
		StartBalances: map[int64]int64{1: 200000}, // 2 000 € real balance, no txns
		Params:        Params{SecuredSavingsBasis: basis, Today: domain.NewDate(2026, 6, 15)},
	}
}

func TestGolden_SecuredAllPlanned(t *testing.T) {
	got := securedInputs(domain.BasisAllPlanned).Savings(1)
	if got.Secured != 166000 {
		t.Errorf("all_planned secured = %d, want 166000 (1 660 €)", got.Secured)
	}
	goldenCheck(t, "secured_all_planned", got)
}

func TestGolden_SecuredFixedOnly(t *testing.T) {
	got := securedInputs(domain.BasisFixedOnly).Savings(1)
	if got.Secured != 187000 {
		t.Errorf("fixed_only secured = %d, want 187000 (1 870 €)", got.Secured)
	}
	goldenCheck(t, "secured_fixed_only", got)
}

func TestGolden_SweepMonthToSave(t *testing.T) {
	in := balInputs(
		domain.NewDate(2026, 6, 28), map[int64]int64{accCurrent: 0},
		incomeTxn(accCurrent, domain.StatusCleared, 200000, ptrDate(2026, 6, 2)),
		expenseTxn(accCurrent, domain.StatusCleared, 50000, ptrDate(2026, 6, 10)),
	)
	got := in.Savings(accCurrent)
	if got.ToSave != 150000 {
		t.Errorf("to_save = %d, want 150000", got.ToSave)
	}
	goldenCheck(t, "sweep_month_to_save", got)
}

func TestGolden_CarryMonthProjectedEnd(t *testing.T) {
	in := Inputs{
		Period:        "2026-06",
		Accounts:      []domain.Account{{ID: 3, Type: domain.AccountCurrent, MonthEndPolicy: domain.PolicyCarry}},
		Categories:    []domain.Category{{ID: catExpense, FlowType: domain.FlowExpense}},
		Txns:          []domain.Transaction{expenseTxn(3, domain.StatusAwaited, 30000, nil)},
		StartBalances: map[int64]int64{3: 100000},
		Params:        Params{Today: domain.NewDate(2026, 6, 15)},
	}
	got := in.AccountBalances(3)
	if got.ProjectedEnd != 70000 {
		t.Errorf("projected_end = %d, want 70000", got.ProjectedEnd)
	}
	goldenCheck(t, "carry_month_balances", got)
}

func TestGolden_PEAGainAndLoss(t *testing.T) {
	mk := func(gross int64) Inputs {
		return Inputs{
			Period:    "2026-06",
			Accounts:  []domain.Account{{ID: 4, Type: domain.AccountSecurities, MonthEndPolicy: domain.PolicyNone}},
			Snapshots: []domain.Snapshot{{AccountID: 4, Period: "2026-06", GrossValue: gross}},
			Params:    Params{PEAInitialDeposit: 1000000, PEASocialChargeRate: 1720, Today: domain.NewDate(2026, 6, 15)},
		}
	}
	gain := mk(1100000).NetWorth()
	if gain.Total != 1082800 { // 11000 − 172 charge on the 1000 gain
		t.Errorf("pea gain total = %d, want 1082800", gain.Total)
	}
	goldenCheck(t, "pea_gain", gain)

	loss := mk(900000).NetWorth()
	if loss.Total != 900000 { // loss guard: gross unchanged
		t.Errorf("pea loss total = %d, want 900000", loss.Total)
	}
	goldenCheck(t, "pea_loss", loss)
}

func ptrDate(y, m, d int) *domain.Date {
	dt := domain.NewDate(y, m, d)
	return &dt
}
