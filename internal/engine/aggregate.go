package engine

import "econome/internal/domain"

// SweepAccounts returns the ids of accounts whose month-end policy is sweep
// (one savings band each in the aggregated view, functional/03 §14).
func (in Inputs) SweepAccounts() []int64 {
	var ids []int64
	for _, a := range in.Accounts {
		if a.MonthEndPolicy == domain.PolicySweep && a.Status != domain.ArchiveArchived {
			ids = append(ids, a.ID)
		}
	}
	return ids
}

// AggregateLowPoint returns the worst (lowest) single-account low point across
// the given accounts — an overdraft is per account, never on the aggregate
// (functional/03 §14).
func (in Inputs) AggregateLowPoint(accountIDs []int64) (LowPoint, int64) {
	var worst LowPoint
	var worstAcc int64
	first := true
	for _, id := range accountIDs {
		lp := in.LowPoint(id)
		if first || lp.Value < worst.Value {
			worst, worstAcc, first = lp, id, false
		}
	}
	return worst, worstAcc
}
