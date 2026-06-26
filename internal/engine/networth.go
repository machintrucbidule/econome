package engine

import "econome/internal/domain"

// PEANet applies the social-charge deduction to a PEA's gross value, charging
// the gain only (functional/03 §12): a loss returns the gross unchanged; a gain
// keeps initial + (gross−initial)·(1−rate), banker's-rounded.
func PEANet(gross, initial int64, rateBP int) int64 {
	if gross < initial {
		return gross // loss guard: no charge without a gain
	}
	gain := gross - initial
	charge := int64(ApplyRate(Money(gain), BasisPoints(rateBP)))
	return gross - charge
}

// NetWorthSupport is a single savings support's contribution (functional/03 §13).
type NetWorthSupport struct {
	AccountID int64
	Type      domain.AccountType
	Value     int64 // pea_net for securities, gross otherwise
	Delta     int64 // value − previous snapshot's value
}

// NetWorth is the net-worth synthesis for the focus period (functional/03 §13).
type NetWorth struct {
	Supports        []NetWorthSupport
	LivretsSubtotal int64 // Σ passbook gross
	Total           int64 // Σ support value
	TotalDelta      int64
}

// NetWorth computes the net-worth totals from the period's snapshots.
func (in Inputs) NetWorth() NetWorth {
	var nw NetWorth
	for _, a := range in.Accounts {
		if !a.IsSavings() || a.Status == domain.ArchiveArchived {
			continue
		}
		gross, ok := in.snapshotValue(a.ID, in.Period)
		if !ok {
			continue // no snapshot this month ⇒ not part of this month's net worth
		}
		value := in.supportValue(a, gross)
		support := NetWorthSupport{
			AccountID: a.ID, Type: a.Type, Value: value,
			Delta: value - in.prevSupportValue(a),
		}
		nw.Supports = append(nw.Supports, support)
		nw.Total += value
		nw.TotalDelta += support.Delta
		if a.Type == domain.AccountPassbook {
			nw.LivretsSubtotal += gross
		}
	}
	return nw
}

func (in Inputs) supportValue(a domain.Account, gross int64) int64 {
	if a.Type == domain.AccountSecurities {
		return PEANet(gross, in.Params.PEAInitialDeposit, in.Params.PEASocialChargeRate)
	}
	return gross
}

func (in Inputs) prevSupportValue(a domain.Account) int64 {
	best := ""
	var gross int64
	found := false
	for _, s := range in.Snapshots {
		if s.AccountID != a.ID || s.Period >= in.Period {
			continue
		}
		if !found || s.Period > best {
			best, gross, found = s.Period, s.GrossValue, true
		}
	}
	if !found {
		return 0
	}
	return in.supportValue(a, gross)
}

func (in Inputs) snapshotValue(accID int64, period string) (int64, bool) {
	for _, s := range in.Snapshots {
		if s.AccountID == accID && s.Period == period {
			return s.GrossValue, true
		}
	}
	return 0, false
}
