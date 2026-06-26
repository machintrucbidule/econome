package server_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// Month-initialisation assistant e2e backbone (increment 5). Drives the real HTTP
// surface: configure → draft → recompute → create → redirect, plus the
// negative-residual state, the rail scope, and the already-created guard.

// mkAccountID creates a current account over HTTP and returns the id of the
// most recently created account (last in DOM order on the Comptes card).
func mkAccountID(t *testing.T, base string, client *http.Client, name, typ, policy string) string {
	t.Helper()
	tok := csrfToken(t, client, base, "/config/parameters")
	resp := formReq(t, client, http.MethodPost, base+"/config/accounts", url.Values{
		"_csrf": {tok}, "name": {name}, "type": {typ}, "month_end_policy": {policy},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create account %s = %d", name, resp.StatusCode)
	}
	_ = bodyOf(t, resp)
	page := getBody(t, client, base, "/config/parameters")
	ids := extractAllAccountIDs(t, page)
	if len(ids) == 0 {
		t.Fatalf("no account id after creating %s", name)
	}
	return ids[len(ids)-1]
}

func mkEnvHTTP(t *testing.T, base string, client *http.Client, form url.Values) {
	t.Helper()
	tok := csrfToken(t, client, base, "/config/envelopes")
	form.Set("_csrf", tok)
	resp := formReq(t, client, http.MethodPost, base+"/config/envelopes", form)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create envelope %q = %d: %s", form.Get("name"), resp.StatusCode, bodyOf(t, resp))
	}
	_ = bodyOf(t, resp)
}

// firstAmtID returns the first amt_<id> input id on the draft page (the
// lowest-id post; posts are sorted ascending → the first-created envelope).
func firstAmtID(t *testing.T, page string) string {
	t.Helper()
	const marker = `name="amt_`
	j := strings.Index(page, marker)
	if j < 0 {
		t.Fatalf("no amt_ input on draft page")
	}
	start := j + len(marker)
	end := start
	for end < len(page) && page[end] >= '0' && page[end] <= '9' {
		end++
	}
	return page[start:end]
}

func TestMonthInitDraftCreateFlow(t *testing.T) {
	ts, client := setupOwner(t)
	base := ts.URL

	fID := mkAccountID(t, base, client, "Fortuneo", "current", "sweep")
	// Income first (lowest envelope id → first post), then the expense.
	mkEnvHTTP(t, base, client, url.Values{
		"name": {"Salaire"}, "flow_type": {"income"}, "account_id": {fID},
		"mode": {"fixed_recurring"}, "default_amount": {"2600,00"}, "frequency": {"monthly"}, "expected_day": {"27"},
	})
	mkEnvHTTP(t, base, client, url.Values{
		"name": {"Loyers"}, "flow_type": {"expense"}, "account_id": {fID},
		"mode": {"fixed_recurring"}, "default_amount": {"1050,00"}, "frequency": {"monthly"}, "expected_day": {"5"},
	})

	// Draft renders: posts + residual encart + create affordance.
	draft := getBody(t, client, base, "/month-init?period=2026-06")
	for _, want := range []string{"Salaire", "Loyers", "Épargne prévisionnelle", "Prévu", "Créer le mois"} {
		if !strings.Contains(draft, want) {
			t.Errorf("draft page missing %q", want)
		}
	}

	// Recompute: drop the income (first post) to 0 → the residual goes negative.
	amtID := firstAmtID(t, draft)
	tok := csrfToken(t, client, base, "/config/parameters")
	resp := formReq(t, client, http.MethodPatch, base+"/month-init/draft?period=2026-06&scope=all", url.Values{
		"_csrf": {tok}, "amt_" + amtID: {"0,00"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("draft PATCH = %d", resp.StatusCode)
	}
	if b := bodyOf(t, resp); !strings.Contains(b, "négatif") {
		t.Errorf("negative residual not reflected in recompute fragment: %s", b)
	}

	// Create the month → 303 redirect to the budget landing.
	tok = csrfToken(t, client, base, "/config/parameters")
	resp = formReq(t, client, http.MethodPost, base+"/month-init?period=2026-06", url.Values{"_csrf": {tok}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create month = %d, want 303", resp.StatusCode)
	}

	// The assistant is now unavailable for that month → GET redirects away.
	resp = getNoFollow(t, client, base+"/month-init?period=2026-06")
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("GET created month = %d, want 303 redirect", resp.StatusCode)
	}
}

func TestMonthInitScopeCarryNote(t *testing.T) {
	ts, client := setupOwner(t)
	base := ts.URL
	fID := mkAccountID(t, base, client, "Fortuneo", "current", "sweep")
	bID := mkAccountID(t, base, client, "Boursorama", "current", "carry")
	// A post on the sweep account so the month is non-empty (the assistant then
	// shows the real draft, not the "nothing to generate" state).
	mkEnvHTTP(t, base, client, url.Values{
		"name": {"Courses"}, "flow_type": {"expense"}, "account_id": {fID},
		"mode": {"variable"}, "default_amount": {"300,00"},
	})

	// Scoped to the sweep account: a residual savings band (the label "résidu"
	// appears in both the positive and the negative-residual variants).
	sweepBody := getBody(t, client, base, "/month-init?period=2026-06&scope="+fID)
	if !strings.Contains(sweepBody, "résidu") {
		t.Errorf("sweep scope should show a residual band, got: %s", sweepBody)
	}
	// Scoped to the carry account: the "no savings — carried" note.
	carryBody := getBody(t, client, base, "/month-init?period=2026-06&scope="+bID)
	if !strings.Contains(carryBody, "Pas d'épargne") {
		t.Errorf("carry-account scope should show the carry note, got: %s", carryBody)
	}
}

func getNoFollow(t *testing.T, client *http.Client, urlStr string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, urlStr, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", urlStr, err)
	}
	return resp
}
