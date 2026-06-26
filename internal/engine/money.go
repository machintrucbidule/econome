package engine

// Money is a monetary amount in integer minor units (e.g. euro cents). The
// engine works exclusively in minor units — never floats — so every figure is
// exact (functional/03 C6, technical/03 §1). Formatting to a localised string
// such as "−635,00 €" is the job of internal/i18n, never the engine.
type Money int64

// BasisPoints is a rate expressed in basis points: 1 % = 100 bps, 100 % = 10000.
// Rates are stored and computed as integers (functional/03 C6; technical/03
// §4.5 pea_social_charge_rate / near_cap_threshold).
type BasisPoints int64

const basisPointsDivisor int64 = 10000

// RoundHalfEvenDiv divides num by den and rounds the quotient to the nearest
// integer using banker's rounding (round-half-to-even): a result exactly halfway
// between two integers rounds to the even one. This is the single rounding rule
// for every derived figure (functional/03 C6). den must be non-zero; it panics
// on a zero divisor (a programming error, never user input).
//
// The rounding is symmetric about zero: RoundHalfEvenDiv(-x, d) == -RoundHalfEvenDiv(x, d).
func RoundHalfEvenDiv(num, den int64) int64 {
	if den == 0 {
		panic("engine: RoundHalfEvenDiv by zero")
	}
	// Normalise so den > 0; this keeps the floored-remainder reasoning below valid.
	if den < 0 {
		num, den = -num, -den
	}

	q := num / den
	r := num % den
	// Go's % takes the sign of num; shift to a floored remainder in [0, den)
	// so that num == q*den + r with 0 <= r < den.
	if r < 0 {
		q--
		r += den
	}

	switch twice := r * 2; {
	case twice < den:
		// Closer to q; round down (towards negative infinity at the floor).
	case twice > den:
		q++
	default:
		// Exactly halfway: round to the even neighbour.
		if q%2 != 0 {
			q++
		}
	}
	return q
}

// ApplyRate applies a basis-point rate to an amount in minor units, returning
// the result in minor units, banker's-rounded to the nearest minor unit
// (functional/03 C6 — e.g. the PEA social-charge deduction, rules §12).
func ApplyRate(amount Money, rate BasisPoints) Money {
	return Money(RoundHalfEvenDiv(int64(amount)*int64(rate), basisPointsDivisor))
}
