package engine

import (
	"testing"

	"econome/internal/domain"
)

func TestPEANet(t *testing.T) {
	cases := []struct {
		name           string
		gross, initial int64
		rateBP         int
		want           int64
	}{
		{"loss returns gross", 900000, 1000000, 1720, 900000},
		{"exact initial no gain", 1000000, 1000000, 1720, 1000000},
		{"gain charged", 1100000, 1000000, 1720, 1082800}, // gain 1000 € * 17.2% = 172 € charge
		{"zero rate", 1100000, 1000000, 0, 1100000},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := PEANet(c.gross, c.initial, c.rateBP); got != c.want {
				t.Errorf("PEANet(%d,%d,%d) = %d, want %d", c.gross, c.initial, c.rateBP, got, c.want)
			}
		})
	}
}

func TestNetWorthTotalsAndDeltas(t *testing.T) {
	in := Inputs{
		Period: "2026-06",
		Accounts: []domain.Account{
			{ID: 1, Type: domain.AccountCurrent, MonthEndPolicy: domain.PolicySweep}, // not savings, ignored
			{ID: 2, Type: domain.AccountPassbook, MonthEndPolicy: domain.PolicyNone},
			{ID: 3, Type: domain.AccountSecurities, MonthEndPolicy: domain.PolicyNone},
		},
		Snapshots: []domain.Snapshot{
			{AccountID: 2, Period: "2026-05", GrossValue: 500000},
			{AccountID: 2, Period: "2026-06", GrossValue: 550000},  // +50 000
			{AccountID: 3, Period: "2026-05", GrossValue: 1000000}, // prior PEA at initial
			{AccountID: 3, Period: "2026-06", GrossValue: 1100000}, // gain ⇒ net 1 082 800
		},
		Params: Params{PEAInitialDeposit: 1000000, PEASocialChargeRate: 1720, Today: domain.NewDate(2026, 6, 15)},
	}
	nw := in.NetWorth()

	if len(nw.Supports) != 2 {
		t.Fatalf("supports = %d, want 2 (current ignored)", len(nw.Supports))
	}
	if nw.LivretsSubtotal != 550000 {
		t.Errorf("livrets_subtotal = %d, want 550000", nw.LivretsSubtotal)
	}
	if nw.Total != 550000+1082800 {
		t.Errorf("total = %d, want %d", nw.Total, 550000+1082800)
	}
	// Deltas: passbook +50 000; PEA net 1 082 800 − 1 000 000 (prior net, gross==initial) = 82 800.
	if nw.TotalDelta != 50000+82800 {
		t.Errorf("total_delta = %d, want %d", nw.TotalDelta, 50000+82800)
	}
}

func TestNetWorthFirstMonthDeltaFromZero(t *testing.T) {
	in := Inputs{
		Period:    "2026-06",
		Accounts:  []domain.Account{{ID: 2, Type: domain.AccountPassbook, MonthEndPolicy: domain.PolicyNone}},
		Snapshots: []domain.Snapshot{{AccountID: 2, Period: "2026-06", GrossValue: 500000}},
		Params:    Params{Today: domain.NewDate(2026, 6, 15)},
	}
	nw := in.NetWorth()
	if nw.TotalDelta != 500000 { // no prior snapshot ⇒ delta from 0
		t.Errorf("first-month delta = %d, want 500000", nw.TotalDelta)
	}
}
