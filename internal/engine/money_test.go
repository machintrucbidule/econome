package engine

import (
	"testing"

	"pgregory.net/rapid"
)

func TestRoundHalfEvenDiv_Table(t *testing.T) {
	cases := []struct {
		name     string
		num, den int64
		want     int64
	}{
		{"exact", 10, 2, 5},
		{"round down below half", 5, 4, 1}, // 1.25 -> 1
		{"round up above half", 7, 4, 2},   // 1.75 -> 2
		{"half to even (down)", 2, 4, 0},   // 0.5 -> 0 (even)
		{"half to even (up)", 6, 4, 2},     // 1.5 -> 2 (even)
		{"half to even 2.5 -> 2", 5, 2, 2}, // 2.5 -> 2
		{"half to even 3.5 -> 4", 7, 2, 4}, // 3.5 -> 4
		{"neg exact", -10, 2, -5},          // -5
		{"neg half to even -2.5 -> -2", -5, 2, -2},
		{"neg half to even -3.5 -> -4", -7, 2, -4},
		{"neg below half", -5, 4, -1},                  // -1.25 -> -1
		{"neg above half", -7, 4, -2},                  // -1.75 -> -2
		{"negative denominator normalised", 5, -2, -2}, // 5/-2 = -2.5 -> -2
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := RoundHalfEvenDiv(c.num, c.den); got != c.want {
				t.Errorf("RoundHalfEvenDiv(%d, %d) = %d, want %d", c.num, c.den, got, c.want)
			}
		})
	}
}

func TestRoundHalfEvenDiv_PanicsOnZero(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on zero divisor")
		}
	}()
	RoundHalfEvenDiv(1, 0)
}

func TestApplyRate(t *testing.T) {
	cases := []struct {
		amount Money
		rate   BasisPoints
		want   Money
	}{
		{10000, 1720, 1720}, // 100.00 € * 17.20 % = 17.20 €
		{12345, 0, 0},
		{10000, 10000, 10000},  // * 100 %
		{-63500, 1720, -10922}, // -635.00 € * 17.20 % = -109.22 € (banker's rounded)
	}
	for _, c := range cases {
		if got := ApplyRate(c.amount, c.rate); got != c.want {
			t.Errorf("ApplyRate(%d, %d) = %d, want %d", c.amount, c.rate, got, c.want)
		}
	}
}

// Property: an exact multiple divides back to the original quotient.
func TestRoundHalfEvenDiv_ExactMultiples(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		q := rapid.Int64Range(-1_000_000_000, 1_000_000_000).Draw(t, "q")
		den := rapid.Int64Range(1, 1_000_000).Draw(t, "den")
		if got := RoundHalfEvenDiv(q*den, den); got != q {
			t.Fatalf("RoundHalfEvenDiv(%d*%d, %d) = %d, want %d", q, den, den, got, q)
		}
	})
}

// Property: the rounded quotient is within half a unit of the true ratio, i.e.
// 2*|q*den - num| <= den.
func TestRoundHalfEvenDiv_WithinHalf(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		num := rapid.Int64Range(-1_000_000_000, 1_000_000_000).Draw(t, "num")
		den := rapid.Int64Range(1, 1_000_000).Draw(t, "den")
		q := RoundHalfEvenDiv(num, den)
		diff := q*den - num
		if diff < 0 {
			diff = -diff
		}
		if 2*diff > den {
			t.Fatalf("RoundHalfEvenDiv(%d, %d) = %d not within half: 2*%d > %d", num, den, q, diff, den)
		}
	})
}

// Property: rounding is symmetric about zero.
func TestRoundHalfEvenDiv_Symmetric(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		num := rapid.Int64Range(-1_000_000_000, 1_000_000_000).Draw(t, "num")
		den := rapid.Int64Range(1, 1_000_000).Draw(t, "den")
		if RoundHalfEvenDiv(-num, den) != -RoundHalfEvenDiv(num, den) {
			t.Fatalf("not symmetric for num=%d den=%d", num, den)
		}
	})
}
