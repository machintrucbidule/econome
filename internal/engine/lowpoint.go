package engine

import (
	"sort"

	"econome/internal/domain"
)

// LowPointEntry is one cumulative balance point for the treasury timeline.
type LowPointEntry struct {
	Date    domain.Date
	Balance int64
}

// LowPoint is the intra-month treasury low point for an account (functional/03
// §11.2, C3/C9). It is built from transactions only — unspent variable budget is
// deliberately excluded (C9).
type LowPoint struct {
	Value         int64
	AtDate        domain.Date
	BreachesZero  bool
	OrderedPoints []LowPointEntry
}

// LowPoint computes the minimum cumulative balance from today to end-of-period.
func (in Inputs) LowPoint(accID int64) LowPoint {
	running := in.AccountBalances(accID).RealBalance
	points := []LowPointEntry{{Date: in.Params.Today, Balance: running}}
	minVal := running
	at := in.Params.Today

	type move struct {
		date domain.Date
		amt  int64
	}
	var moves []move
	for _, t := range in.Txns {
		if t.BudgetPeriod != in.Period {
			continue
		}
		amt, ok := txnAmountFor(t, accID)
		if !ok {
			continue
		}
		include := false
		switch t.Status {
		case domain.StatusAwaited:
			include = true
		case domain.StatusPending, domain.StatusCleared:
			include = futureDated(t, in.Params.Today) // not yet counted in real_balance
		}
		if include {
			moves = append(moves, move{date: in.movementOrderingDate(t), amt: amt})
		}
	}
	sort.SliceStable(moves, func(i, j int) bool { return moves[i].date.Before(moves[j].date) })

	for _, m := range moves {
		running += m.amt
		points = append(points, LowPointEntry{Date: m.date, Balance: running})
		if running < minVal {
			minVal = running
			at = m.date
		}
	}
	return LowPoint{Value: minVal, AtDate: at, BreachesZero: minVal < 0, OrderedPoints: points}
}

// movementOrderingDate places a not-yet-completed movement on the timeline:
// op_date if present; else the fixed_recurring envelope's expected_day; else
// end-of-period for undated one-offs (C3).
func (in Inputs) movementOrderingDate(t domain.Transaction) domain.Date {
	if t.OpDate != nil {
		return *t.OpDate
	}
	if e, ok := in.envelopeForTxn(t); ok && e.Mode == domain.ModeFixedRecurring && e.ExpectedDay != nil {
		if y, m, parsed := parsePeriod(t.BudgetPeriod); parsed {
			day := *e.ExpectedDay
			if dim := daysInMonth(y, m); day > dim {
				day = dim
			}
			if day < 1 {
				day = 1
			}
			return domain.NewDate(y, m, day)
		}
	}
	return endOfPeriod(t.BudgetPeriod)
}

func (in Inputs) envelopeForTxn(t domain.Transaction) (domain.Envelope, bool) {
	if t.CategoryID == nil {
		return domain.Envelope{}, false
	}
	for _, e := range in.Envelopes {
		if e.CategoryID == *t.CategoryID && e.AccountID == t.AccountID {
			return e, true
		}
	}
	return domain.Envelope{}, false
}
