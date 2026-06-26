package domain

import "testing"

func TestBudgetEnumsValid(t *testing.T) {
	valid := []interface{ Valid() bool }{
		FlowExpense, FlowIncome, FlowTransfer,
		AccountCurrent, AccountPassbook, AccountSecurities, AccountEmployeeSavings,
		PolicySweep, PolicyCarry, PolicyNone,
		ModeFixedRecurring, ModeVariable, ModeResidual,
		FreqMonthly, FreqQuarterly, FreqSemiannual, FreqAnnual,
		StatusAwaited, StatusPending, StatusCleared,
		SourceManual, SourceImport,
		ArchiveActive, ArchiveArchived,
	}
	for _, v := range valid {
		if !v.Valid() {
			t.Errorf("%v should be Valid", v)
		}
	}
	invalid := []interface{ Valid() bool }{
		FlowType("x"), AccountType("x"), MonthEndPolicy("x"), Mode("x"),
		Frequency("x"), TransactionStatus("x"), TxnSource("x"), ArchiveStatus("x"),
	}
	for _, v := range invalid {
		if v.Valid() {
			t.Errorf("%v should be invalid", v)
		}
	}
}

func TestStatusIsReal(t *testing.T) {
	if !StatusCleared.IsReal() || !StatusPending.IsReal() {
		t.Error("cleared and pending count as real (C7)")
	}
	if StatusAwaited.IsReal() {
		t.Error("awaited is not real")
	}
}

func TestAccountIsSavings(t *testing.T) {
	if (Account{Type: AccountCurrent}).IsSavings() {
		t.Error("current is not savings")
	}
	for _, ty := range []AccountType{AccountPassbook, AccountSecurities, AccountEmployeeSavings} {
		if !(Account{Type: ty}).IsSavings() {
			t.Errorf("%s should be savings", ty)
		}
	}
}
