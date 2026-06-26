package engine

import "econome/internal/domain"

// EnvelopeView is an envelope's derived budget figures for the focus period
// (functional/03 §3). Money is in the envelope's positive consumption direction
// (expense spend, income received). The five-state status applies to expenses
// only; income/transfer envelopes leave State empty.
type EnvelopeView struct {
	EnvelopeID     int64
	Flow           domain.FlowType
	Planned        int64
	Real           int64 // cleared + pending (C7)
	Awaited        int64
	InProgress     int64 // pending subset of Real
	Remaining      int64 // planned − real
	Percent        int   // round(100·real/planned); 0 when planned = 0
	State          domain.EnvelopeState
	PlannedOverrun bool // real + awaited > planned (forward warning)
}

// EnvelopeView computes an envelope's figures for in.Period.
func (in Inputs) EnvelopeView(envID int64) EnvelopeView {
	e, _ := in.EnvelopeByID(envID)
	flow := in.EnvelopeFlow(e)
	planned := in.PlannedAmount(envID, in.Period)
	realAmt, awaited, inProgress := in.envelopeSums(e, flow)

	v := EnvelopeView{
		EnvelopeID: envID, Flow: flow, Planned: planned,
		Real: realAmt, Awaited: awaited, InProgress: inProgress,
		Remaining:      planned - realAmt,
		PlannedOverrun: realAmt+awaited > planned,
	}
	if planned > 0 {
		v.Percent = int(RoundHalfEvenDiv(100*realAmt, planned))
	}
	if flow == domain.FlowExpense {
		v.State = envelopeState(planned, realAmt, awaited)
	}
	return v
}

func (in Inputs) envelopeSums(e domain.Envelope, flow domain.FlowType) (realAmt, awaited, inProgress int64) {
	for _, t := range in.Txns {
		if t.BudgetPeriod != in.Period || t.CategoryID == nil || *t.CategoryID != e.CategoryID || t.AccountID != e.AccountID {
			continue
		}
		amt := consumptionAmount(flow, t.Amount)
		switch t.Status {
		case domain.StatusCleared:
			realAmt += amt
		case domain.StatusPending:
			realAmt += amt
			inProgress += amt
		case domain.StatusAwaited:
			awaited += amt
		}
	}
	return realAmt, awaited, inProgress
}

// envelopeState applies the five-state rule (C2/C8), including the planned = 0
// case. Expenses only.
func envelopeState(planned, realAmt, awaited int64) domain.EnvelopeState {
	if planned == 0 { // C8: unbudgeted envelope
		switch {
		case realAmt > 0:
			return domain.StateOverrun
		case awaited > 0:
			return domain.StateExpected
		default:
			return domain.StateNone
		}
	}
	switch {
	case realAmt == 0 && awaited == 0:
		return domain.StateNone
	case realAmt == 0 && awaited > 0:
		return domain.StateExpected
	case realAmt < planned:
		return domain.StatePartial
	case realAmt == planned:
		return domain.StatePaid
	default:
		return domain.StateOverrun
	}
}
