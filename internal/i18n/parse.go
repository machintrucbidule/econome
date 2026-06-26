package i18n

import (
	"errors"
	"strconv"
	"strings"
	"unicode"

	"econome/internal/domain"
)

// Money/rate parsing is the inverse of FormatMoney: it turns a locale-entered
// string ("22 950,00", "17,2 %") into integer minor units / basis points at the
// HTTP boundary, so the engine and the database never see a float (the
// "no float ever touches money" invariant, technical/06 §2, technical/04 §4).
//
// Parsing is forgiving of the separators the formatter emits (grouping spaces of
// any width, a leading/trailing currency symbol or percent sign, ASCII '-' or the
// true minus U+2212) but exact about precision: more than two fractional digits is
// rejected rather than silently rounded.

// Parse errors. The service layer maps any of these to a field-level
// ValidationError (422) with a message key it chooses (G3); they are not
// user-facing strings themselves.
var (
	// ErrEmptyAmount is returned for a blank or sign-only input.
	ErrEmptyAmount = errors.New("i18n: empty amount")
	// ErrBadAmount is returned for an unparseable amount (stray letters, two
	// decimal separators, or more than two fractional digits).
	ErrBadAmount = errors.New("i18n: invalid amount")
	// ErrRateRange is returned by ParsePercent when the rate is outside [0, 100).
	ErrRateRange = errors.New("i18n: rate out of range")
)

func decimalRune(lang domain.Language) rune {
	if lang == domain.LangEN {
		return '.'
	}
	return ','
}

// chooseDecimal picks the decimal separator actually present in s. EconoMe is
// forgiving at the input boundary (technical/06 §2, I-030): when only the
// "foreign" separator appears (a '.' in fr-FR, a ',' in en-) it is taken as the
// decimal point, so a French user typing "12.50" still means 12,50 — not 1250.
// When both separators appear, the locale's own separator is the decimal and the
// other is grouping. Truly ambiguous input (two of the chosen separator, e.g.
// "1,2,3") is still rejected downstream by parseFixed2.
func chooseDecimal(s string, lang domain.Language) rune {
	hasComma := strings.ContainsRune(s, ',')
	hasDot := strings.ContainsRune(s, '.')
	if hasComma != hasDot {
		if hasComma {
			return ','
		}
		return '.'
	}
	return decimalRune(lang)
}

// parseFixed2 parses s as a fixed-point number with at most two fractional
// digits, returning the value in hundredths (so "17,2" -> 1720, "9 000,5" ->
// 90050). Every rune that is not a digit, the locale decimal separator, or a sign
// is ignored (grouping spaces, a currency symbol, a percent sign), except a
// Unicode letter, which is rejected so "12x" never parses to 12.
func parseFixed2(s string, decimal rune) (int64, error) {
	var intD, fracD []rune
	neg := false
	sawDecimal := false
	sawDigit := false
	for _, r := range s {
		switch {
		case r == '-' || r == rune(0x2212):
			neg = true
		case r >= '0' && r <= '9':
			sawDigit = true
			if sawDecimal {
				fracD = append(fracD, r)
			} else {
				intD = append(intD, r)
			}
		case r == decimal:
			if sawDecimal {
				return 0, ErrBadAmount
			}
			sawDecimal = true
		case unicode.IsLetter(r):
			return 0, ErrBadAmount
		default:
			// grouping separators, currency symbol, percent sign, whitespace
		}
	}
	if !sawDigit {
		return 0, ErrEmptyAmount
	}
	if len(fracD) > 2 {
		return 0, ErrBadAmount
	}
	intVal := int64(0)
	if len(intD) > 0 {
		v, err := strconv.ParseInt(string(intD), 10, 64)
		if err != nil {
			return 0, ErrBadAmount
		}
		intVal = v
	}
	frac := int64(0)
	switch len(fracD) {
	case 1:
		frac = int64(fracD[0]-'0') * 10
	case 2:
		frac = int64(fracD[0]-'0')*10 + int64(fracD[1]-'0')
	}
	out := intVal*100 + frac
	if neg {
		out = -out
	}
	return out, nil
}

// ParseMoney parses a locale-entered amount into signed integer minor units
// (cents). It is exact: no float is ever involved.
func ParseMoney(s string, lang domain.Language) (int64, error) {
	return parseFixed2(s, chooseDecimal(s, lang))
}

// ParsePercent parses a locale-entered percentage into basis points (17,2 % ->
// 1720), bound-checked to [0, 100) % i.e. [0, 10000) basis points — the
// rate-bound 422 of functional/10 §3 (never a DB 500). A negative rate is
// rejected as out of range.
func ParsePercent(s string, lang domain.Language) (int, error) {
	// A percentage carries two decimal places of precision exactly: 1 basis
	// point = 0.01 %, so the hundredths returned by parseFixed2 are basis points.
	bp, err := parseFixed2(s, chooseDecimal(s, lang))
	if err != nil {
		return 0, err
	}
	if bp < 0 || bp >= 10000 {
		return 0, ErrRateRange
	}
	return int(bp), nil
}
