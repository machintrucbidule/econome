package server_test

import (
	"net/http"
	"net/url"
	"regexp"
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

// allocEnvID extracts an editable row's envelope id from the per-account page by
// finding the first /allocations/<id> input that follows the row's name.
func allocEnvID(t *testing.T, raw, name string) string {
	t.Helper()
	ni := strings.Index(raw, ">"+name+"<")
	if ni < 0 {
		t.Fatalf("row %q not found on the page", name)
	}
	const marker = "/allocations/"
	j := strings.Index(raw[ni:], marker)
	if j < 0 {
		t.Fatalf("no editable input after row %q", name)
	}
	start := ni + j + len(marker)
	end := start
	for end < len(raw) && raw[end] >= '0' && raw[end] <= '9' {
		end++
	}
	return raw[start:end]
}

func TestForecastInlineEditRecompute(t *testing.T) {
	ts, client := setupOwner(t)
	base := ts.URL

	fID := mkAccountID(t, base, client, "Fortuneo", "current", "sweep")
	mkEnvHTTP(t, base, client, url.Values{
		"name": {"Salaire"}, "flow_type": {"income"}, "account_id": {fID},
		"mode": {"fixed_recurring"}, "default_amount": {"2600,00"}, "frequency": {"monthly"}, "expected_day": {"1"},
	})
	mkEnvHTTP(t, base, client, url.Values{
		"name": {"Courses"}, "flow_type": {"expense"}, "account_id": {fID},
		"mode": {"variable"}, "default_amount": {"600,00"},
	})
	tok := csrfToken(t, client, base, "/config/parameters")
	resp := formReq(t, client, http.MethodPost, base+"/month-init?period=2026-06", url.Values{"_csrf": {tok}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create month = %d", resp.StatusCode)
	}
	_ = bodyOf(t, resp)

	// Read the per-account page (editable inputs) and find the Courses envelope id.
	pageResp, _ := client.Get(base + "/?period=2026-06&scope=" + fID)
	raw := bodyOf(t, pageResp)
	if !strings.Contains(raw, `hx-patch="/allocations/`) {
		t.Fatalf("active month should render inline Prévu inputs")
	}
	env := allocEnvID(t, raw, "Courses")

	// Raise Courses past income → the recompute fragment shows the negative state.
	tok = csrfToken(t, client, base, "/config/parameters")
	resp = formReq(t, client, http.MethodPatch, base+"/allocations/"+env+"?period=2026-06&scope="+fID, url.Values{
		"_csrf": {tok}, "planned": {"3000,00"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("allocation PATCH = %d, want 200", resp.StatusCode)
	}
	body := bodyOf(t, resp)
	for _, want := range []string{`id="fc-total"`, `id="fc-figures"`, `id="fc-panel"`, "hx-swap-oob", "Solde insuffisant"} {
		if !strings.Contains(body, want) {
			t.Errorf("PATCH response missing %q", want)
		}
	}

	// The end-of-month transfer route is wired and guarded: with no realised
	// residual (no cleared movements yet) it refuses with 409.
	tok = csrfToken(t, client, base, "/config/parameters")
	resp = formReq(t, client, http.MethodPost, base+"/transfers/end-of-month?period=2026-06&scope="+fID+"&account="+fID, url.Values{"_csrf": {tok}})
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("end-of-month transfer with nothing to sweep = %d, want 409", resp.StatusCode)
	}
	_ = bodyOf(t, resp)
}

func TestForecastExpandPersists(t *testing.T) {
	ts, client := setupOwner(t)
	base := ts.URL
	fID := mkAccountID(t, base, client, "Fortuneo", "current", "sweep")
	// Two envelopes under a parent category "Assurance".
	for _, name := range []string{"Habitation", "Auto"} {
		mkEnvHTTP(t, base, client, url.Values{
			"name": {name}, "parent_id": {"__new__"}, "new_parent_name": {"Assurance"}, "flow_type": {"expense"}, "account_id": {fID},
			"mode": {"fixed_recurring"}, "default_amount": {"30,00"}, "frequency": {"monthly"}, "expected_day": {"8"},
		})
	}
	tok := csrfToken(t, client, base, "/config/parameters")
	if resp := formReq(t, client, http.MethodPost, base+"/month-init?period=2026-06", url.Values{"_csrf": {tok}}); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create month = %d", resp.StatusCode)
	}

	// The parent row is collapsed by default; grab its category node id (raw body
	// — goquery would reorder the attributes).
	pr, _ := client.Get(base + "/?period=2026-06&scope=" + fID)
	page := bodyOf(t, pr)
	m := regexp.MustCompile(`data-node-id="(\d+)"[^>]*data-node-type="category"|data-node-type="category"[^>]*data-node-id="(\d+)"`).FindStringSubmatch(page)
	if m == nil {
		t.Fatalf("no parent category row found")
	}
	catID := m[1] + m[2]

	// Persist expanded.
	tok = csrfToken(t, client, base, "/config/parameters")
	resp := formReq(t, client, http.MethodPut, base+"/ui/expand", url.Values{
		"_csrf": {tok}, "node_type": {"category"}, "node_id": {catID}, "expanded": {"1"},
	})
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT /ui/expand = %d, want 204", resp.StatusCode)
	}
	_ = bodyOf(t, resp)

	// Re-render: the parent row is now open + its children visible.
	pr, _ = client.Get(base + "/?period=2026-06&scope=" + fID)
	page = bodyOf(t, pr)
	if !strings.Contains(page, `class="frow open"`) {
		t.Error("parent row should render expanded after PUT /ui/expand")
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
	// O-22: the aggregated flat rows are now inline-editable (each maps to one
	// envelope), so the Prévu cell renders an amount input wired to PATCH.
	if !strings.Contains(agg, `class="amt-inp" name="planned"`) {
		t.Error("aggregated scope should expose an editable Prévu input (O-22)")
	}
	if !strings.Contains(agg, "hx-patch=\"/allocations/") {
		t.Error("aggregated Prévu input should hx-patch the allocation (O-22)")
	}
}
