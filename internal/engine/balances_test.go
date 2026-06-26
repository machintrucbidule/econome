package engine

import (
	"testing"

	"econome/internal/domain"
)

func incomeTxn(acc int64, status domain.TransactionStatus, amt int64, op *domain.Date) domain.Transaction {
	c := catIncome
	return domain.Transaction{AccountID: acc, CategoryID: &c, FlowType: domain.FlowIncome, Amount: amt, BudgetPeriod: "2026-06", Status: status, OpDate: op}
}

func expenseTxn(acc int64, status domain.TransactionStatus, magnitude int64, op *domain.Date) domain.Transaction {
	c := catExpense
	return domain.Transaction{AccountID: acc, CategoryID: &c, FlowType: domain.FlowExpense, Amount: -magnitude, BudgetPeriod: "2026-06", Status: status, OpDate: op}
}

func balInputs(today domain.Date, start map[int64]int64, txns ...domain.Transaction) Inputs {
	return Inputs{
		Period:        "2026-06",
		Accounts:      []domain.Account{{ID: accCurrent, Type: domain.AccountCurrent, MonthEndPolicy: domain.PolicySweep}, {ID: 2, Type: domain.AccountPassbook, MonthEndPolicy: domain.PolicyNone}},
		Categories:    []domain.Category{{ID: catExpense, FlowType: domain.FlowExpense}, {ID: catIncome, FlowType: domain.FlowIncome}},
		Txns:          txns,
		Params:        Params{Today: today},
		StartBalances: start,
	}
}

func TestAccountBalances(t *testing.T) {
	today := domain.NewDate(2026, 6, 15)
	d10 := domain.NewDate(2026, 6, 10)
	in := balInputs(
		today, map[int64]int64{accCurrent: 100000},
		incomeTxn(accCurrent, domain.StatusCleared, 200000, &d10), // +200000 real
		expenseTxn(accCurrent, domain.StatusPending, 50000, &d10), // −50000 real + in_progress
		expenseTxn(accCurrent, domain.StatusAwaited, 30000, nil),  // only projected_end
	)
	b := in.AccountBalances(accCurrent)
	if b.Start != 100000 {
		t.Errorf("start = %d", b.Start)
	}
	if b.RealBalance != 250000 { // 100000 + 200000 − 50000
		t.Errorf("real_balance = %d, want 250000", b.RealBalance)
	}
	if b.InProgress != -50000 {
		t.Errorf("in_progress = %d, want -50000", b.InProgress)
	}
	if b.ClearedBalance != 300000 { // real − in_progress = 250000 − (−50000)
		t.Errorf("cleared_balance = %d, want 300000", b.ClearedBalance)
	}
	if b.ProjectedEnd != 220000 { // 100000 + 200000 − 50000 − 30000
		t.Errorf("projected_end = %d, want 220000", b.ProjectedEnd)
	}
}

func TestFutureDatedClearedExcludedFromReal(t *testing.T) {
	today := domain.NewDate(2026, 6, 15)
	future := domain.NewDate(2026, 6, 20)
	in := balInputs(today, nil, incomeTxn(accCurrent, domain.StatusCleared, 100000, &future))
	b := in.AccountBalances(accCurrent)
	if b.RealBalance != 0 {
		t.Errorf("future-dated cleared should not be in real_balance; got %d", b.RealBalance)
	}
	if b.ProjectedEnd != 100000 {
		t.Errorf("projected_end should include it; got %d", b.ProjectedEnd)
	}
}

func TestTransferAffectsBothLegs(t *testing.T) {
	today := domain.NewDate(2026, 6, 15)
	d := domain.NewDate(2026, 6, 10)
	dest := int64(2)
	transfer := domain.Transaction{
		AccountID: accCurrent, DestAccountID: &dest, FlowType: domain.FlowTransfer,
		Amount: -50000, BudgetPeriod: "2026-06", Status: domain.StatusCleared, OpDate: &d,
	}
	in := balInputs(today, nil, transfer)
	if b := in.AccountBalances(accCurrent); b.RealBalance != -50000 {
		t.Errorf("source leg = %d, want -50000", b.RealBalance)
	}
	if b := in.AccountBalances(2); b.RealBalance != 50000 {
		t.Errorf("dest leg = %d, want 50000", b.RealBalance)
	}
}
