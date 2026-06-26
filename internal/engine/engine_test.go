package engine

import (
	"testing"

	"econome/internal/domain"
)

func sampleInputs() Inputs {
	return Inputs{
		Period: "2026-06",
		Accounts: []domain.Account{
			{ID: 1, Name: "Courant", Type: domain.AccountCurrent, MonthEndPolicy: domain.PolicySweep},
			{ID: 2, Name: "LDDS", Type: domain.AccountPassbook, MonthEndPolicy: domain.PolicyNone},
		},
		Categories: []domain.Category{
			{ID: 10, FlowType: domain.FlowExpense},
			{ID: 11, FlowType: domain.FlowIncome},
		},
		Envelopes: []domain.Envelope{
			{ID: 100, CategoryID: 10, AccountID: 1, Mode: domain.ModeVariable},
		},
		Allocations: []domain.Allocation{
			{ID: 1, EnvelopeID: 100, Period: "2026-06", PlannedAmount: 30000},
		},
	}
}

func TestLookups(t *testing.T) {
	in := sampleInputs()
	if a, ok := in.AccountByID(1); !ok || a.Name != "Courant" {
		t.Error("AccountByID")
	}
	if _, ok := in.AccountByID(99); ok {
		t.Error("AccountByID missing should be false")
	}
	if c, ok := in.CategoryByID(11); !ok || c.FlowType != domain.FlowIncome {
		t.Error("CategoryByID")
	}
	e, ok := in.EnvelopeByID(100)
	if !ok {
		t.Fatal("EnvelopeByID")
	}
	if in.EnvelopeFlow(e) != domain.FlowExpense {
		t.Error("EnvelopeFlow should inherit category flow")
	}
	if in.PlannedAmount(100, "2026-06") != 30000 {
		t.Error("PlannedAmount")
	}
	if in.PlannedAmount(100, "2026-07") != 0 {
		t.Error("PlannedAmount missing period should be 0")
	}
}

func TestEndOfPeriod(t *testing.T) {
	cases := map[string]domain.Date{
		"2026-06": domain.NewDate(2026, 6, 30),
		"2026-07": domain.NewDate(2026, 7, 31),
		"2026-02": domain.NewDate(2026, 2, 28),
		"2024-02": domain.NewDate(2024, 2, 29), // leap year
		"2000-02": domain.NewDate(2000, 2, 29), // div by 400
		"1900-02": domain.NewDate(1900, 2, 28), // div by 100 not 400
	}
	for period, want := range cases {
		if got := endOfPeriod(period); got != want {
			t.Errorf("endOfPeriod(%s) = %v, want %v", period, got, want)
		}
	}
	if !endOfPeriod("bad").IsZero() {
		t.Error("malformed period should yield zero date")
	}
}
