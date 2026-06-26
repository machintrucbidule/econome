package engine

import (
	"testing"

	"econome/internal/domain"
)

func mv(account int64, sign Sign, amount int64, d domain.Date) Movement {
	return Movement{Account: account, Sign: sign, Amount: amount, Date: d}
}

func cand(id, account int64, sign Sign, amount int64, d domain.Date) Candidate {
	return Candidate{TxnID: id, Account: account, Sign: sign, Amount: amount, ExpectedDate: d, Period: d.Period()}
}

func TestReconcileMatrix(t *testing.T) {
	exact := Tolerance{Amount: 0, DateWindowDays: 3}
	d10 := domain.NewDate(2026, 6, 10)
	d12 := domain.NewDate(2026, 6, 12)
	d20 := domain.NewDate(2026, 6, 20)
	base := mv(1, -1, 10000, d10)

	d13 := domain.NewDate(2026, 6, 13) // exactly 3 days from d10 (window boundary)
	cases := []struct {
		name       string
		m          Movement
		candidates []Candidate
		tol        Tolerance
		wantKind   DecisionKind
		wantTxn    int64
		wantAmbig  []int64
	}{
		{"zero candidates", base, nil, exact, CreateNew, 0, nil},
		{"one exact match", base, []Candidate{cand(7, 1, -1, 10000, d12)}, exact, ReconcileInPlace, 7, nil},
		{"amount mismatch (exact)", base, []Candidate{cand(7, 1, -1, 10001, d12)}, exact, CreateNew, 0, nil},
		{"amount within tolerance", base, []Candidate{cand(7, 1, -1, 10003, d12)}, Tolerance{Amount: 5, DateWindowDays: 3}, ReconcileInPlace, 7, nil},
		{"amount at tolerance boundary", base, []Candidate{cand(7, 1, -1, 10005, d12)}, Tolerance{Amount: 5, DateWindowDays: 3}, ReconcileInPlace, 7, nil},
		{"amount just past tolerance", base, []Candidate{cand(7, 1, -1, 10006, d12)}, Tolerance{Amount: 5, DateWindowDays: 3}, CreateNew, 0, nil},
		{"date at window boundary", base, []Candidate{cand(7, 1, -1, 10000, d13)}, exact, ReconcileInPlace, 7, nil},
		{"date outside window", base, []Candidate{cand(7, 1, -1, 10000, d20)}, exact, CreateNew, 0, nil},
		{"wrong sign", base, []Candidate{cand(7, 1, 1, 10000, d12)}, exact, CreateNew, 0, nil},
		{"wrong account", base, []Candidate{cand(7, 2, -1, 10000, d12)}, exact, CreateNew, 0, nil},
		{"two matches ambiguous", base, []Candidate{cand(7, 1, -1, 10000, d12), cand(8, 1, -1, 10000, d10)}, exact, Ambiguous, 0, []int64{7, 8}},
		{"one of two matches", base, []Candidate{cand(7, 1, -1, 10000, d12), cand(8, 2, -1, 10000, d10)}, exact, ReconcileInPlace, 7, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Reconcile(c.m, c.candidates, c.tol)
			if got.Kind != c.wantKind {
				t.Fatalf("kind = %v, want %v", got.Kind, c.wantKind)
			}
			if c.wantKind == ReconcileInPlace && got.TxnID != c.wantTxn {
				t.Errorf("txnID = %d, want %d", got.TxnID, c.wantTxn)
			}
			if c.wantKind == Ambiguous {
				if len(got.AmbiguousIDs) != len(c.wantAmbig) {
					t.Fatalf("ambiguous ids = %v, want %v", got.AmbiguousIDs, c.wantAmbig)
				}
				for i, id := range c.wantAmbig {
					if got.AmbiguousIDs[i] != id {
						t.Errorf("ambiguous[%d] = %d, want %d", i, got.AmbiguousIDs[i], id)
					}
				}
			}
		})
	}
}

func TestPairTransfer(t *testing.T) {
	tol := Tolerance{Amount: 0, DateWindowDays: 2}
	d10 := domain.NewDate(2026, 6, 10)
	d11 := domain.NewDate(2026, 6, 11)
	// An outflow leg on account 1 pairs with an inflow leg on account 2.
	leg := mv(1, -1, 50000, d10)

	if got := PairTransfer(leg, []Candidate{cand(9, 2, 1, 50000, d11)}, tol); got.Kind != ReconcileInPlace || got.TxnID != 9 {
		t.Errorf("opposite leg should pair: %+v", got)
	}
	// Same sign ⇒ not a pair.
	if got := PairTransfer(leg, []Candidate{cand(9, 2, -1, 50000, d11)}, tol); got.Kind != CreateNew {
		t.Errorf("same-sign leg should not pair: %+v", got)
	}
	// Same account ⇒ not a pair.
	if got := PairTransfer(leg, []Candidate{cand(9, 1, 1, 50000, d11)}, tol); got.Kind != CreateNew {
		t.Errorf("same-account leg should not pair: %+v", got)
	}
}

func TestDateDiffDays(t *testing.T) {
	cases := []struct {
		a, b domain.Date
		want int
	}{
		{domain.NewDate(2026, 6, 10), domain.NewDate(2026, 6, 15), 5},
		{domain.NewDate(2026, 6, 15), domain.NewDate(2026, 6, 10), 5}, // symmetric
		{domain.NewDate(2026, 2, 28), domain.NewDate(2026, 3, 1), 1},  // non-leap
		{domain.NewDate(2024, 2, 28), domain.NewDate(2024, 3, 1), 2},  // leap
		{domain.NewDate(2025, 12, 31), domain.NewDate(2026, 1, 1), 1}, // year boundary
		{domain.NewDate(2026, 6, 10), domain.NewDate(2026, 6, 10), 0},
	}
	for _, c := range cases {
		if got := dateDiffDays(c.a, c.b); got != c.want {
			t.Errorf("dateDiffDays(%v,%v) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
