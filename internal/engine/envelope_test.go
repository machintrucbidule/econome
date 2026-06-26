package engine

import (
	"testing"

	"pgregory.net/rapid"

	"econome/internal/domain"
)

const (
	catExpense int64 = 10
	catIncome  int64 = 11
	accCurrent int64 = 1
)

// expenseEnvInputs builds Inputs with one expense envelope (id 100) on the
// current account, a planned allocation, and the given transactions.
func expenseEnvInputs(planned int64, txns ...domain.Transaction) Inputs {
	return Inputs{
		Period:      "2026-06",
		Accounts:    []domain.Account{{ID: accCurrent, Type: domain.AccountCurrent, MonthEndPolicy: domain.PolicySweep}},
		Categories:  []domain.Category{{ID: catExpense, FlowType: domain.FlowExpense}, {ID: catIncome, FlowType: domain.FlowIncome}},
		Envelopes:   []domain.Envelope{{ID: 100, CategoryID: catExpense, AccountID: accCurrent, Mode: domain.ModeVariable}},
		Allocations: []domain.Allocation{{EnvelopeID: 100, Period: "2026-06", PlannedAmount: planned}},
		Txns:        txns,
	}
}

// exp builds a cleared/pending/awaited expense transaction of the given positive
// magnitude (stored as a negative signed amount).
func exp(status domain.TransactionStatus, magnitude int64) domain.Transaction {
	c := catExpense
	return domain.Transaction{AccountID: accCurrent, CategoryID: &c, FlowType: domain.FlowExpense, Amount: -magnitude, BudgetPeriod: "2026-06", Status: status}
}

func TestEnvelopeFiveStates(t *testing.T) {
	cases := []struct {
		name    string
		planned int64
		txns    []domain.Transaction
		state   domain.EnvelopeState
	}{
		{"none", 30000, nil, domain.StateNone},
		{"expected", 30000, []domain.Transaction{exp(domain.StatusAwaited, 10000)}, domain.StateExpected},
		{"partial", 30000, []domain.Transaction{exp(domain.StatusCleared, 10000)}, domain.StatePartial},
		{"paid", 30000, []domain.Transaction{exp(domain.StatusCleared, 30000)}, domain.StatePaid},
		{"overrun", 30000, []domain.Transaction{exp(domain.StatusCleared, 32000)}, domain.StateOverrun},
		{"planned0 none", 0, nil, domain.StateNone},
		{"planned0 expected", 0, []domain.Transaction{exp(domain.StatusAwaited, 5000)}, domain.StateExpected},
		{"planned0 overrun", 0, []domain.Transaction{exp(domain.StatusCleared, 5000)}, domain.StateOverrun},
		{"pending counts as real", 30000, []domain.Transaction{exp(domain.StatusPending, 30000)}, domain.StatePaid},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := expenseEnvInputs(c.planned, c.txns...).EnvelopeView(100)
			if v.State != c.state {
				t.Errorf("state = %q, want %q (real=%d awaited=%d planned=%d)", v.State, c.state, v.Real, v.Awaited, v.Planned)
			}
		})
	}
}

func TestEnvelopeRealRemainingPercent(t *testing.T) {
	in := expenseEnvInputs(30000, exp(domain.StatusCleared, 10000), exp(domain.StatusPending, 5000), exp(domain.StatusAwaited, 8000))
	v := in.EnvelopeView(100)
	if v.Real != 15000 { // cleared 10000 + pending 5000 (C7)
		t.Errorf("real = %d, want 15000", v.Real)
	}
	if v.InProgress != 5000 {
		t.Errorf("in_progress = %d, want 5000", v.InProgress)
	}
	if v.Awaited != 8000 {
		t.Errorf("awaited = %d, want 8000", v.Awaited)
	}
	if v.Remaining != 15000 { // 30000 - 15000
		t.Errorf("remaining = %d, want 15000", v.Remaining)
	}
	if v.Percent != 50 { // 100*15000/30000
		t.Errorf("percent = %d, want 50", v.Percent)
	}
	if v.PlannedOverrun { // 15000 + 8000 = 23000 <= 30000
		t.Errorf("PlannedOverrun should be false (23000 <= 30000)")
	}
}

// Overrun preserves the flag: clearing an awaited 100 at 102 yields real 102,
// overrun vs planned 100, remaining −2; the allocation is not raised.
func TestOverrunPreservesFlag(t *testing.T) {
	in := expenseEnvInputs(10000, exp(domain.StatusCleared, 10200))
	v := in.EnvelopeView(100)
	if v.State != domain.StateOverrun || v.Real != 10200 || v.Remaining != -200 || v.Planned != 10000 {
		t.Errorf("got state=%q real=%d remaining=%d planned=%d", v.State, v.Real, v.Remaining, v.Planned)
	}
}

func TestIncomeEnvelopeNoFiveState(t *testing.T) {
	c := catIncome
	in := Inputs{
		Period:      "2026-06",
		Accounts:    []domain.Account{{ID: accCurrent, Type: domain.AccountCurrent}},
		Categories:  []domain.Category{{ID: catIncome, FlowType: domain.FlowIncome}},
		Envelopes:   []domain.Envelope{{ID: 200, CategoryID: catIncome, AccountID: accCurrent, Mode: domain.ModeVariable}},
		Allocations: []domain.Allocation{{EnvelopeID: 200, Period: "2026-06", PlannedAmount: 200000}},
		Txns:        []domain.Transaction{{AccountID: accCurrent, CategoryID: &c, FlowType: domain.FlowIncome, Amount: 200000, BudgetPeriod: "2026-06", Status: domain.StatusCleared}},
	}
	v := in.EnvelopeView(200)
	if v.State != "" {
		t.Errorf("income envelope should have no five-state, got %q", v.State)
	}
	if v.Real != 200000 { // received (positive)
		t.Errorf("income real = %d, want 200000", v.Real)
	}
}

// Property: real = cleared + pending; inProgress = pending; awaited separate.
func TestProp_PendingInReal(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		var txns []domain.Transaction
		var cleared, pending, awaited int64
		n := rapid.IntRange(0, 8).Draw(t, "n")
		for i := 0; i < n; i++ {
			mag := rapid.Int64Range(1, 100000).Draw(t, "mag")
			st := []domain.TransactionStatus{domain.StatusCleared, domain.StatusPending, domain.StatusAwaited}[rapid.IntRange(0, 2).Draw(t, "st")]
			txns = append(txns, exp(st, mag))
			switch st {
			case domain.StatusCleared:
				cleared += mag
			case domain.StatusPending:
				pending += mag
			case domain.StatusAwaited:
				awaited += mag
			}
		}
		v := expenseEnvInputs(50000, txns...).EnvelopeView(100)
		if v.Real != cleared+pending {
			t.Fatalf("real = %d, want %d", v.Real, cleared+pending)
		}
		if v.InProgress != pending {
			t.Fatalf("inProgress = %d, want %d", v.InProgress, pending)
		}
		if v.Awaited != awaited {
			t.Fatalf("awaited = %d, want %d", v.Awaited, awaited)
		}
	})
}

// Property: exactly one valid five-state for any expense inputs.
func TestProp_FiveStateTotality(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		planned := rapid.Int64Range(0, 100000).Draw(t, "planned")
		realAmt := rapid.Int64Range(-50000, 200000).Draw(t, "real")
		awaited := rapid.Int64Range(0, 100000).Draw(t, "awaited")
		s := envelopeState(planned, realAmt, awaited)
		if !s.Valid() {
			t.Fatalf("invalid state %q for planned=%d real=%d awaited=%d", s, planned, realAmt, awaited)
		}
	})
}
