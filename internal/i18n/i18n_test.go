package i18n

import (
	"strings"
	"testing"

	"econome/internal/domain"
)

func TestFormatMoney(t *testing.T) {
	minus := string(rune(0x2212))
	nnbsp := string(rune(0x202f))
	nb := string(rune(0x00a0))
	eur := string(rune(0x20ac))

	cases := []struct {
		minor int64
		lang  domain.Language
		want  string
	}{
		{-63500, domain.LangFR, minus + "635,00" + nb + eur},
		{127927, domain.LangFR, "1" + nnbsp + "279,27" + nb + eur},
		{0, domain.LangFR, "0,00" + nb + eur},
		{5, domain.LangFR, "0,05" + nb + eur},
		{-63500, domain.LangEN, "-" + eur + "635.00"},
		{127927, domain.LangEN, eur + "1,279.27"},
		{100000000, domain.LangFR, "1" + nnbsp + "000" + nnbsp + "000,00" + nb + eur},
	}
	for _, c := range cases {
		if got := FormatMoney(c.minor, c.lang, "EUR"); got != c.want {
			t.Errorf("FormatMoney(%d, %s) = %q, want %q", c.minor, c.lang, got, c.want)
		}
	}
}

func TestCatalog(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := c.T(domain.LangFR, "app.name"); got != "EconoMe" {
		t.Errorf("FR app.name = %q", got)
	}
	if got := c.T(domain.LangEN, "login.submit"); got != "Sign in" {
		t.Errorf("EN login.submit = %q", got)
	}
	// Missing key returns the key itself (logged, never crashes).
	if got := c.T(domain.LangFR, "does.not.exist"); got != "does.not.exist" {
		t.Errorf("missing key = %q, want the key", got)
	}
	// Positional argument substitution.
	if got := c.T(domain.LangEN, "login.locked", "30"); !strings.Contains(got, "30") {
		t.Errorf("login.locked = %q, want it to contain 30", got)
	}
}

// Every key present in FR must also exist in EN (catalog completeness).
func TestCatalogParity(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for key := range c.messages[domain.LangFR] {
		if _, ok := c.messages[domain.LangEN][key]; !ok {
			t.Errorf("key %q present in FR but missing in EN", key)
		}
	}
	for key := range c.messages[domain.LangEN] {
		if _, ok := c.messages[domain.LangFR][key]; !ok {
			t.Errorf("key %q present in EN but missing in FR", key)
		}
	}
}
