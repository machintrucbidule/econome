package engine

import (
	"testing"

	"econome/internal/domain"
)

// fixedRecurringTxn builds an awaited transaction tied to a fixed_recurring
// envelope with the given expected_day (so it orders mid-month).
func lowPointInputs(start int64, today domain.Date, envs []domain.Envelope, txns []domain.Transaction) Inputs {
	return Inputs{
		Period:   "2026-06",
		Accounts: []domain.Account{{ID: accCurrent, Type: domain.AccountCurrent, MonthEndPolicy: domain.PolicySweep}},
		Categories: []domain.Category{
			{ID: 30, FlowType: domain.FlowExpense},
			{ID: 31, FlowType: domain.FlowIncome},
			{ID: catExpense, FlowType: domain.FlowExpense},
		},
		Envelopes:     envs,
		Txns:          txns,
		StartBalances: map[int64]int64{accCurrent: start},
		Params:        Params{Today: today},
	}
}

func day(n int) *int { return &n }

// A mid-month awaited expense dips below zero even though the month ends positive.
func TestLowPointMidMonthDip(t *testing.T) {
	c30, c31 := int64(30), int64(31)
	envs := []domain.Envelope{
		{ID: 1, CategoryID: 30, AccountID: accCurrent, Mode: domain.ModeFixedRecurring, ExpectedDay: day(10)},
		{ID: 2, CategoryID: 31, AccountID: accCurrent, Mode: domain.ModeFixedRecurring, ExpectedDay: day(20)},
	}
	txns := []domain.Transaction{
		{AccountID: accCurrent, CategoryID: &c30, FlowType: domain.FlowExpense, Amount: -50000, BudgetPeriod: "2026-06", Status: domain.StatusAwaited},
		{AccountID: accCurrent, CategoryID: &c31, FlowType: domain.FlowIncome, Amount: 60000, BudgetPeriod: "2026-06", Status: domain.StatusAwaited},
	}
	lp := lowPointInputs(10000, domain.NewDate(2026, 6, 1), envs, txns).LowPoint(accCurrent)
	if lp.Value != -40000 || !lp.BreachesZero { // 10000 − 50000 at day 10
		t.Errorf("low point = %d breaches=%v, want -40000/true", lp.Value, lp.BreachesZero)
	}
	if lp.AtDate.Compare(domain.NewDate(2026, 6, 10)) != 0 {
		t.Errorf("low point at %v, want 2026-06-10", lp.AtDate)
	}
}

// An undated one-off awaited weighs only at end-of-period (C3), creating no
// spurious mid-month dip.
func TestLowPointUndatedOneOffAtEnd(t *testing.T) {
	c31 := int64(31)
	cExp := catExpense
	envs := []domain.Envelope{
		{ID: 2, CategoryID: 31, AccountID: accCurrent, Mode: domain.ModeFixedRecurring, ExpectedDay: day(15)},
		{ID: 3, CategoryID: catExpense, AccountID: accCurrent, Mode: domain.ModeVariable}, // one-off, no expected_day
	}
	txns := []domain.Transaction{
		{AccountID: accCurrent, CategoryID: &c31, FlowType: domain.FlowIncome, Amount: 60000, BudgetPeriod: "2026-06", Status: domain.StatusAwaited},
		{AccountID: accCurrent, CategoryID: &cExp, FlowType: domain.FlowExpense, Amount: -50000, BudgetPeriod: "2026-06", Status: domain.StatusAwaited},
	}
	lp := lowPointInputs(10000, domain.NewDate(2026, 6, 1), envs, txns).LowPoint(accCurrent)
	// Income at day 15 (→ 70000), one-off expense at end (→ 20000). Min = 10000 (start), no dip.
	if lp.Value != 10000 || lp.BreachesZero {
		t.Errorf("low point = %d breaches=%v, want 10000/false (one-off at end)", lp.Value, lp.BreachesZero)
	}
}

// Treasury excludes unspent variable budget (C9): an allocation with no
// transaction does not affect the low point.
func TestLowPointExcludesUnspentVariable(t *testing.T) {
	in := Inputs{
		Period:        "2026-06",
		Accounts:      []domain.Account{{ID: accCurrent, Type: domain.AccountCurrent, MonthEndPolicy: domain.PolicySweep}},
		Categories:    []domain.Category{{ID: catExpense, FlowType: domain.FlowExpense}},
		Envelopes:     []domain.Envelope{{ID: 1, CategoryID: catExpense, AccountID: accCurrent, Mode: domain.ModeVariable}},
		Allocations:   []domain.Allocation{{EnvelopeID: 1, Period: "2026-06", PlannedAmount: 99999900}}, // huge planned, no txn
		StartBalances: map[int64]int64{accCurrent: 10000},
		Params:        Params{Today: domain.NewDate(2026, 6, 1)},
	}
	lp := in.LowPoint(accCurrent)
	if lp.Value != 10000 || lp.BreachesZero {
		t.Errorf("unspent variable budget must not move the low point; got %d/%v", lp.Value, lp.BreachesZero)
	}
}

func TestAggregateLowPointWorstAccount(t *testing.T) {
	c30 := int64(30)
	in := Inputs{
		Period: "2026-06",
		Accounts: []domain.Account{
			{ID: 1, Type: domain.AccountCurrent, MonthEndPolicy: domain.PolicySweep},
			{ID: 2, Type: domain.AccountCurrent, MonthEndPolicy: domain.PolicyCarry},
		},
		Categories:    []domain.Category{{ID: 30, FlowType: domain.FlowExpense}},
		Envelopes:     []domain.Envelope{{ID: 1, CategoryID: 30, AccountID: 2, Mode: domain.ModeFixedRecurring, ExpectedDay: day(10)}},
		Txns:          []domain.Transaction{{AccountID: 2, CategoryID: &c30, FlowType: domain.FlowExpense, Amount: -80000, BudgetPeriod: "2026-06", Status: domain.StatusAwaited}},
		StartBalances: map[int64]int64{1: 50000, 2: 30000},
		Params:        Params{Today: domain.NewDate(2026, 6, 1)},
	}
	lp, acc := in.AggregateLowPoint([]int64{1, 2})
	if acc != 2 || lp.Value != -50000 { // account 2: 30000 − 80000
		t.Errorf("worst = acct %d value %d, want acct 2 / -50000", acc, lp.Value)
	}
}
