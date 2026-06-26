package i18n

import (
	"strconv"
	"strings"

	"econome/internal/domain"
)

// Money formatting is done manually from integer minor units (I-013): this
// honours the load-bearing "no float ever touches money" invariant while
// producing exactly the functional convention's output. The fr-FR separators
// are built from numeric code points (unambiguous in source):
//
//	U+2212 true minus, U+202F narrow no-break space (grouping),
//	U+00A0 no-break space before a trailing symbol — e.g. "MINUS635,00 EUR".
var (
	trueMinus  = string(rune(0x2212))
	narrowNBSP = string(rune(0x202f))
	nbsp       = string(rune(0x00a0))
	euroSymbol = string(rune(0x20ac))
)

type moneyFormat struct {
	decimal       string
	group         string // thousands separator
	minus         string
	symbolSpace   string // between amount and symbol
	symbolLeading bool
}

func formatFor(lang domain.Language) moneyFormat {
	switch lang {
	case domain.LangEN:
		return moneyFormat{decimal: ".", group: ",", minus: "-", symbolSpace: "", symbolLeading: true}
	case domain.LangFR:
		return moneyFormat{decimal: ",", group: narrowNBSP, minus: trueMinus, symbolSpace: nbsp, symbolLeading: false}
	default:
		return moneyFormat{decimal: ",", group: narrowNBSP, minus: trueMinus, symbolSpace: nbsp, symbolLeading: false}
	}
}

func currencySymbol(currency string) string {
	switch currency {
	case "EUR":
		return euroSymbol
	default:
		return currency
	}
}

// FormatMoney renders an amount in minor units as a localised currency string.
// It is exact: no float is ever involved.
func FormatMoney(minor int64, lang domain.Language, currency string) string {
	f := formatFor(lang)

	negative := minor < 0
	abs := minor
	if negative {
		abs = -abs
	}
	euros := abs / 100
	cents := abs % 100

	var b strings.Builder
	if negative {
		b.WriteString(f.minus)
	}

	body := groupDigits(strconv.FormatInt(euros, 10), f.group) + f.decimal + twoDigits(cents)
	sym := currencySymbol(currency)
	if f.symbolLeading {
		b.WriteString(sym)
		b.WriteString(body)
	} else {
		b.WriteString(body)
		b.WriteString(f.symbolSpace)
		b.WriteString(sym)
	}
	return b.String()
}

// FormatAmount renders minor units as a localised number WITHOUT a currency
// symbol — for editable form inputs (the parsing inverse of ParseMoney). It is
// exact: no float. Example: 900000 -> "9 000,00" (FR), "9,000.00" (EN).
func FormatAmount(minor int64, lang domain.Language) string {
	f := formatFor(lang)
	negative := minor < 0
	abs := minor
	if negative {
		abs = -abs
	}
	body := groupDigits(strconv.FormatInt(abs/100, 10), f.group) + f.decimal + twoDigits(abs%100)
	if negative {
		return f.minus + body
	}
	return body
}

// FormatRate renders basis points as a localised percentage number without the
// percent sign, trimming a redundant trailing zero — for the rate input fields
// (the inverse of ParsePercent). Example: 1720 -> "17,2", 9000 -> "90",
// 1725 -> "17,25" (FR).
func FormatRate(bp int, lang domain.Language) string {
	f := formatFor(lang)
	whole := bp / 100
	frac := bp % 100
	s := strconv.Itoa(whole)
	switch {
	case frac == 0:
		return s
	case frac%10 == 0:
		return s + f.decimal + strconv.Itoa(frac/10)
	default:
		return s + f.decimal + twoDigits(int64(frac))
	}
}

// groupDigits inserts sep every three digits from the right.
func groupDigits(digits, sep string) string {
	n := len(digits)
	if n <= 3 {
		return digits
	}
	var b strings.Builder
	lead := n % 3
	if lead > 0 {
		b.WriteString(digits[:lead])
	}
	for i := lead; i < n; i += 3 {
		if b.Len() > 0 {
			b.WriteString(sep)
		}
		b.WriteString(digits[i : i+3])
	}
	return b.String()
}

func twoDigits(n int64) string {
	if n < 10 {
		return "0" + strconv.FormatInt(n, 10)
	}
	return strconv.FormatInt(n, 10)
}
