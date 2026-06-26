package i18n

import (
	"errors"
	"testing"

	"econome/internal/domain"
)

func TestParseMoney(t *testing.T) {
	nnbsp := string(rune(0x202f))
	nb := string(rune(0x00a0))
	eur := string(rune(0x20ac))
	minus := string(rune(0x2212))

	cases := []struct {
		in   string
		lang domain.Language
		want int64
	}{
		{"0", domain.LangFR, 0},
		{"0,00", domain.LangFR, 0},
		{"9 000,00", domain.LangFR, 900000},
		{"9" + nnbsp + "000,00", domain.LangFR, 900000},
		{"22 950,00", domain.LangFR, 2295000},
		{"1 279,27", domain.LangFR, 127927},
		{"0,05", domain.LangFR, 5},
		{"12,5", domain.LangFR, 1250},
		{"1234", domain.LangFR, 123400},
		// foreign decimal separator accepted when unambiguous (I-030)
		{"12.50", domain.LangFR, 1250},
		{"0.05", domain.LangFR, 5},
		{"1234.5", domain.LangFR, 123450},
		{"12,50", domain.LangEN, 1250},
		{minus + "635,00", domain.LangFR, -63500},
		{"-635,00", domain.LangFR, -63500},
		// round-trips of the formatter's own output
		{"1" + nnbsp + "279,27" + nb + eur, domain.LangFR, 127927},
		{eur + "1,279.27", domain.LangEN, 127927},
		{"1,234.56", domain.LangEN, 123456},
		{"635.00", domain.LangEN, 63500},
	}
	for _, c := range cases {
		got, err := ParseMoney(c.in, c.lang)
		if err != nil {
			t.Errorf("ParseMoney(%q, %s) error: %v", c.in, c.lang, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseMoney(%q, %s) = %d, want %d", c.in, c.lang, got, c.want)
		}
	}
}

func TestParseMoneyErrors(t *testing.T) {
	cases := []struct {
		in   string
		lang domain.Language
		want error
	}{
		{"", domain.LangFR, ErrEmptyAmount},
		{"  ", domain.LangFR, ErrEmptyAmount},
		{"-", domain.LangFR, ErrEmptyAmount},
		{"12,345", domain.LangFR, ErrBadAmount}, // 3 fractional digits
		{"1,2,3", domain.LangFR, ErrBadAmount},  // two decimals
		{"12.345", domain.LangFR, ErrBadAmount}, // 3 fractional via foreign dot
		{"1.2.3", domain.LangFR, ErrBadAmount},  // two foreign-dot decimals
		{"12x", domain.LangFR, ErrBadAmount},    // stray letter
		{"abc", domain.LangFR, ErrBadAmount},    // letters
	}
	for _, c := range cases {
		_, err := ParseMoney(c.in, c.lang)
		if !errors.Is(err, c.want) {
			t.Errorf("ParseMoney(%q) error = %v, want %v", c.in, err, c.want)
		}
	}
}

func TestParsePercent(t *testing.T) {
	cases := []struct {
		in   string
		lang domain.Language
		want int
	}{
		{"0", domain.LangFR, 0},
		{"17,2", domain.LangFR, 1720},
		{"17,2 %", domain.LangFR, 1720},
		{"17,25 %", domain.LangFR, 1725},
		{"90 %", domain.LangFR, 9000},
		{"17.2", domain.LangEN, 1720},
		{"17.2", domain.LangFR, 1720}, // foreign dot accepted (I-030)
		{"99,99", domain.LangFR, 9999},
	}
	for _, c := range cases {
		got, err := ParsePercent(c.in, c.lang)
		if err != nil {
			t.Errorf("ParsePercent(%q) error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParsePercent(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestParsePercentRange(t *testing.T) {
	for _, in := range []string{"100", "100 %", "150", "-5"} {
		if _, err := ParsePercent(in, domain.LangFR); !errors.Is(err, ErrRateRange) {
			t.Errorf("ParsePercent(%q) error = %v, want ErrRateRange", in, err)
		}
	}
}
