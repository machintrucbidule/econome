package engine

import "econome/internal/domain"

// AccountBalances is an account's derived balances for the focus period
// (functional/03 §5). real includes pending (C7).
type AccountBalances struct {
	Start          int64 // start-of-month (C5)
	RealBalance    int64 // start + Σ(cleared+pending up to today)
	InProgress     int64 // Σ pending (subset of RealBalance)
	ClearedBalance int64 // RealBalance − InProgress (bank-finalised)
	ProjectedEnd   int64 // start + Σ(cleared+pending+awaited)
}

// AccountBalances computes an account's balances for in.Period. Transfers affect
// balances (both legs), unlike the budget.
func (in Inputs) AccountBalances(accID int64) AccountBalances {
	start := in.startOfMonth(accID)
	var realDelta, inProgress, projDelta int64

	for _, t := range in.Txns {
		if t.BudgetPeriod != in.Period {
			continue
		}
		amt, ok := txnAmountFor(t, accID)
		if !ok {
			continue
		}
		projDelta += amt // projected_end counts all statuses
		switch t.Status {
		case domain.StatusCleared:
			if !futureDated(t, in.Params.Today) {
				realDelta += amt
			}
		case domain.StatusPending:
			if !futureDated(t, in.Params.Today) {
				realDelta += amt
				inProgress += amt
			}
		case domain.StatusAwaited:
			// awaited is not part of real_balance (only projected_end)
		}
	}

	realBal := start + realDelta
	return AccountBalances{
		Start:          start,
		RealBalance:    realBal,
		InProgress:     inProgress,
		ClearedBalance: realBal - inProgress,
		ProjectedEnd:   start + projDelta,
	}
}

// futureDated reports whether a dated movement lies strictly after today (so it
// does not yet count towards real_balance, "up to today").
func futureDated(t domain.Transaction, today domain.Date) bool {
	return t.OpDate != nil && t.OpDate.After(today)
}
