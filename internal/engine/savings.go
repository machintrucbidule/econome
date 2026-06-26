package engine

import (
	"sort"

	"econome/internal/domain"
)

// SavingsView is the residual-savings figures for a sweep account
// (functional/03 §7/§9/§11.1).
type SavingsView struct {
	Projected        int64  // start + Σ planned(income) − Σ planned(expense)
	Secured          int64  // real_balance − Σ_included remaining_committed
	ToSave           int64  // realised residual (cleared only)
	CascadeTargetID  *int64 // lowest fill_priority non-full vehicle; nil if all full / none
	CascadeFull      bool   // every cascade vehicle is full (C4)
	ResidualNegative bool   // projected < 0 (the negative-residual alert, §11.1)
}

// Savings computes the residual-savings figures for a sweep account.
func (in Inputs) Savings(sweepAccID int64) SavingsView {
	projected := in.savingsProjected(sweepAccID)
	target, full := in.cascadeTarget()
	return SavingsView{
		Projected:        projected,
		Secured:          in.savingsSecured(sweepAccID),
		ToSave:           in.toSave(sweepAccID),
		CascadeTargetID:  target,
		CascadeFull:      full,
		ResidualNegative: projected < 0,
	}
}

// savingsProjected = start(sweep) + Σ planned(income) − Σ planned(expense) over
// the sweep account's envelopes.
func (in Inputs) savingsProjected(sweep int64) int64 {
	total := in.startOfMonth(sweep)
	for _, e := range in.Envelopes {
		if e.AccountID != sweep {
			continue
		}
		planned := in.PlannedAmount(e.ID, in.Period)
		switch in.EnvelopeFlow(e) {
		case domain.FlowIncome:
			total += planned
		case domain.FlowExpense:
			total -= planned
		case domain.FlowTransfer:
			// transfers are not budget lines
		}
	}
	return total
}

// savingsSecured = real_balance(sweep) − Σ_included max(planned−real, 0). The
// included set depends on secured_savings_basis (C1).
func (in Inputs) savingsSecured(sweep int64) int64 {
	secured := in.AccountBalances(sweep).RealBalance
	for _, e := range in.Envelopes {
		if e.AccountID != sweep || in.EnvelopeFlow(e) != domain.FlowExpense {
			continue
		}
		if in.Params.SecuredSavingsBasis == domain.BasisFixedOnly && e.Mode != domain.ModeFixedRecurring {
			continue
		}
		v := in.EnvelopeView(e.ID)
		if rc := v.Planned - v.Real; rc > 0 { // remaining_committed
			secured -= rc
		}
	}
	return secured
}

// toSave is the realised residual: start + cleared movements on the sweep
// account (cleared only, not pending).
func (in Inputs) toSave(sweep int64) int64 {
	total := in.startOfMonth(sweep)
	for _, t := range in.Txns {
		if t.BudgetPeriod != in.Period || t.Status != domain.StatusCleared {
			continue
		}
		if amt, ok := txnAmountFor(t, sweep); ok {
			total += amt
		}
	}
	return total
}

// cascadeTarget returns the lowest-fill_priority non-full savings vehicle, or
// (nil, true) when vehicles exist but are all full, or (nil, false) when there
// are no cascade vehicles (C4 — never auto-exceed a ceiling).
func (in Inputs) cascadeTarget() (*int64, bool) {
	var vehicles []domain.Account
	for _, a := range in.Accounts {
		if a.IsSavings() && a.FillPriority != nil && a.Status != domain.ArchiveArchived {
			vehicles = append(vehicles, a)
		}
	}
	sort.SliceStable(vehicles, func(i, j int) bool { return *vehicles[i].FillPriority < *vehicles[j].FillPriority })

	for _, a := range vehicles {
		if !in.vehicleFull(a) {
			id := a.ID
			return &id, false
		}
	}
	return nil, len(vehicles) > 0
}

// vehicleFull reports whether a savings vehicle has reached its ceiling. Its
// balance is the latest snapshot value plus cleared transfer inflows in the
// period (functional/03 §9).
func (in Inputs) vehicleFull(a domain.Account) bool {
	if a.Ceiling == nil {
		return false
	}
	return in.vehicleBalance(a.ID) >= *a.Ceiling
}

func (in Inputs) vehicleBalance(accID int64) int64 {
	balance := in.latestSnapshotValue(accID)
	for _, t := range in.Txns {
		if t.BudgetPeriod != in.Period || t.FlowType != domain.FlowTransfer || t.Status != domain.StatusCleared {
			continue
		}
		if t.DestAccountID != nil && *t.DestAccountID == accID {
			balance += -t.Amount // inflow (amount is source-signed)
		}
	}
	return balance
}

// latestSnapshotValue returns the gross value of the snapshot for in.Period, or
// the most recent earlier snapshot, or 0.
func (in Inputs) latestSnapshotValue(accID int64) int64 {
	best := ""
	var value int64
	for _, s := range in.Snapshots {
		if s.AccountID != accID || s.Period > in.Period {
			continue
		}
		if s.Period > best {
			best = s.Period
			value = s.GrossValue
		}
	}
	return value
}
