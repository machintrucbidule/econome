package engine

import (
	"strconv"

	"econome/internal/domain"
)

// Params are the per-user calculation parameters (technical/09 §2). today is an
// injected Date — the engine never reads a clock.
type Params struct {
	PEAInitialDeposit   int64                      // minor units
	PEASocialChargeRate int                        // basis points (e.g. 1720 = 17.20 %)
	NearCapThreshold    int                        // basis points (e.g. 9000 = 90 %)
	SecuredSavingsBasis domain.SecuredSavingsBasis // all_planned | fixed_only
	Today               domain.Date
}

// Inputs is the complete, deterministic input set the engine derives figures
// from (technical/09 §2). All money is integer minor units; nothing here is a
// derived figure. The engine reads only this struct — no I/O, clock, or locale.
type Inputs struct {
	Period      string // the "YYYY-MM" focus period
	Accounts    []domain.Account
	Categories  []domain.Category
	Envelopes   []domain.Envelope
	Allocations []domain.Allocation
	Txns        []domain.Transaction
	Snapshots   []domain.Snapshot
	Params      Params
}

// AccountByID returns the account with id, or false.
func (in Inputs) AccountByID(id int64) (domain.Account, bool) {
	for _, a := range in.Accounts {
		if a.ID == id {
			return a, true
		}
	}
	return domain.Account{}, false
}

// CategoryByID returns the category with id, or false.
func (in Inputs) CategoryByID(id int64) (domain.Category, bool) {
	for _, c := range in.Categories {
		if c.ID == id {
			return c, true
		}
	}
	return domain.Category{}, false
}

// EnvelopeByID returns the envelope with id, or false.
func (in Inputs) EnvelopeByID(id int64) (domain.Envelope, bool) {
	for _, e := range in.Envelopes {
		if e.ID == id {
			return e, true
		}
	}
	return domain.Envelope{}, false
}

// EnvelopeFlow returns the flow type an envelope inherits from its category.
func (in Inputs) EnvelopeFlow(e domain.Envelope) domain.FlowType {
	if c, ok := in.CategoryByID(e.CategoryID); ok {
		return c.FlowType
	}
	return ""
}

// PlannedAmount returns the planned allocation for an envelope in a period, or 0
// if none exists.
func (in Inputs) PlannedAmount(envelopeID int64, period string) int64 {
	for _, a := range in.Allocations {
		if a.EnvelopeID == envelopeID && a.Period == period {
			return a.PlannedAmount
		}
	}
	return 0
}

// parsePeriod splits "YYYY-MM" into year and month.
func parsePeriod(period string) (year, month int, ok bool) {
	if len(period) != 7 || period[4] != '-' {
		return 0, 0, false
	}
	y, err1 := strconv.Atoi(period[:4])
	m, err2 := strconv.Atoi(period[5:])
	if err1 != nil || err2 != nil || m < 1 || m > 12 {
		return 0, 0, false
	}
	return y, m, true
}

// endOfPeriod returns the last calendar day of a "YYYY-MM" period.
func endOfPeriod(period string) domain.Date {
	y, m, ok := parsePeriod(period)
	if !ok {
		return domain.Date{}
	}
	return domain.NewDate(y, m, daysInMonth(y, m))
}

func daysInMonth(year, month int) int {
	switch month {
	case 1, 3, 5, 7, 8, 10, 12:
		return 31
	case 4, 6, 9, 11:
		return 30
	case 2:
		if isLeap(year) {
			return 29
		}
		return 28
	default:
		return 30
	}
}

func isLeap(y int) bool { return y%4 == 0 && (y%100 != 0 || y%400 == 0) }
