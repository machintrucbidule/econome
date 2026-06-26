package server_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// Forecast read-only e2e backbone (increment 6a). Drives the real HTTP surface:
// the not-created landing state, then a configured + created month rendering the
// envelope table, the right-panel figures + savings encart, the treasury-timeline
// SVG, the read-only drill-down, and the scope variants.

func TestForecastNotCreatedLanding(t *testing.T) {
	ts, client := setupOwner(t)
	base := ts.URL

	// A fresh owner with no month: the landing offers the initialisation assistant.
	landing := getBody(t, client, base, "/")
	for _, want := range []string{"Prévisionnel", "Ce mois n'est pas encore créé", "Préparer", "/month-init"} {
		if !strings.Contains(landing, want) {
			t.Errorf("not-created landing missing %q", want)
		}
	}
}

func TestForecastRendersCreatedMonth(t *testing.T) {
	ts, client := setupOwner(t)
	base := ts.URL

	fID := mkAccountID(t, base, client, "Fortuneo", "current", "sweep")
	mkEnvHTTP(t, base, client, url.Values{
		"name": {"Salaire"}, "flow_type": {"income"}, "account_id": {fID},
		"mode": {"fixed_recurring"}, "default_amount": {"2600,00"}, "frequency": {"monthly"}, "expected_day": {"1"},
	})
	mkEnvHTTP(t, base, client, url.Values{
		"name": {"Loyers"}, "flow_type": {"expense"}, "account_id": {fID},
		"mode": {"fixed_recurring"}, "default_amount": {"1050,00"}, "frequency": {"monthly"}, "expected_day": {"5"},
	})
	mkEnvHTTP(t, base, client, url.Values{
		"name": {"Courses"}, "flow_type": {"expense"}, "account_id": {fID},
		"mode": {"variable"}, "default_amount": {"600,00"},
	})

	// Create the month.
	tok := csrfToken(t, client, base, "/config/parameters")
	resp := formReq(t, client, http.MethodPost, base+"/month-init?period=2026-06", url.Values{"_csrf": {tok}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create month = %d, want 303", resp.StatusCode)
	}
	_ = bodyOf(t, resp)

	// The forecast renders the table, badges, total, figures, savings encart,
	// the treasury-timeline SVG and the read-only drill-down.
	page := getBody(t, client, base, "/?period=2026-06&scope=all")
	for _, want := range []string{
		"Salaire", "Loyers", "Courses",
		"Total dépenses", "Réel", "Prévu",
		"Balance compte", "À épargner", // figures + savings encart
		"<svg", "Trésorerie", // treasury timeline
	} {
		if !strings.Contains(page, want) {
			t.Errorf("forecast page missing %q", want)
		}
	}

	// Scoped to the sweep account: the residual band + the read-only drill-down
	// affordance (per-account scope renders the collapsible hierarchy).
	sweep := getBody(t, client, base, "/?period=2026-06&scope="+fID)
	for _, want := range []string{"À épargner", "Point bas", "Ouvrir dans le journal"} {
		if !strings.Contains(sweep, want) {
			t.Errorf("sweep scope missing %q", want)
		}
	}

	// The journal tab links to /journal (built in 6c).
	if !strings.Contains(page, "/journal?period=2026-06") {
		t.Error("Journal tab link missing")
	}
}

func TestForecastScopeCarryAndAggregated(t *testing.T) {
	ts, client := setupOwner(t)
	base := ts.URL

	fID := mkAccountID(t, base, client, "Fortuneo", "current", "sweep")
	bID := mkAccountID(t, base, client, "Boursorama", "current", "carry")
	mkEnvHTTP(t, base, client, url.Values{
		"name": {"Courses"}, "flow_type": {"expense"}, "account_id": {fID},
		"mode": {"variable"}, "default_amount": {"600,00"},
	})
	mkEnvHTTP(t, base, client, url.Values{
		"name": {"Restaurant"}, "flow_type": {"expense"}, "account_id": {bID},
		"mode": {"variable"}, "default_amount": {"120,00"},
	})
	mkEnvHTTP(t, base, client, url.Values{
		"name": {"Alimentation CC"}, "flow_type": {"transfer"}, "account_id": {fID},
		"dest_account_id": {bID}, "mode": {"fixed_recurring"}, "default_amount": {"240,00"}, "frequency": {"monthly"}, "expected_day": {"2"},
	})
	tok := csrfToken(t, client, base, "/config/parameters")
	resp := formReq(t, client, http.MethodPost, base+"/month-init?period=2026-06", url.Values{"_csrf": {tok}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create month = %d, want 303", resp.StatusCode)
	}
	_ = bodyOf(t, resp)

	// Carry scope: the carry note, no savings encart, an end-of-month figure.
	carry := getBody(t, client, base, "/?period=2026-06&scope="+bID)
	if !strings.Contains(carry, "Pas de virement d'épargne") || !strings.Contains(carry, "Fin de mois") {
		t.Errorf("carry scope missing carry note / end-of-month figure")
	}
	// Aggregated: account pills + the masked-internal-transfers footer.
	agg := getBody(t, client, base, "/?period=2026-06&scope=all")
	if !strings.Contains(agg, "internes masqués") {
		t.Error("aggregated scope missing the masked-transfers footer")
	}
	if !strings.Contains(agg, "pill-acc") {
		t.Error("aggregated rows should carry account pills")
	}
}
