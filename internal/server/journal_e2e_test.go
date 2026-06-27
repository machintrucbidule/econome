package server_test

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// Journal e2e backbone (increment 6c). Drives the real HTTP surface: states,
// quick-entry create, server-side sort/filter, inline edit, delete.

var reCatID = regexp.MustCompile(`"value":"(\d+)"`)
var reRowID = regexp.MustCompile(`id="jrow-(\d+)"`)

func firstMatch(t *testing.T, re *regexp.Regexp, s, what string) string {
	t.Helper()
	m := re.FindStringSubmatch(s)
	if m == nil {
		t.Fatalf("no %s found", what)
	}
	return m[1]
}

func TestJournalNotCreated(t *testing.T) {
	ts, client := setupOwner(t)
	body := getBody(t, client, ts.URL, "/journal?period=2099-01")
	for _, want := range []string{"Journal", "Ce mois n'est pas encore créé", "Préparer"} {
		if !strings.Contains(body, want) {
			t.Errorf("not-created journal missing %q", want)
		}
	}
}

func TestJournalCreateEditDelete(t *testing.T) {
	ts, client := setupOwner(t)
	base := ts.URL
	fID := mkAccountID(t, base, client, "Fortuneo", "current", "sweep")
	mkEnvHTTP(t, base, client, url.Values{
		"name": {"Courses"}, "flow_type": {"expense"}, "account_id": {fID},
		"mode": {"variable"}, "default_amount": {"600,00"},
	})
	// Create the month so the journal accepts entry.
	tok := csrfToken(t, client, base, "/config/parameters")
	if resp := formReq(t, client, http.MethodPost, base+"/month-init?period=2026-06", url.Values{"_csrf": {tok}}); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create month = %d", resp.StatusCode)
	}

	page := getBody(t, client, base, "/journal?period=2026-06&scope=all")
	for _, want := range []string{"Journal", "Résumé du mois", "Filtres", `id="qform"`, "Solde net du mois"} {
		if !strings.Contains(page, want) {
			t.Errorf("journal page missing %q", want)
		}
	}
	cat := firstMatch(t, reCatID, page, "category option")

	// Quick-entry create → re-rendered rows + OOB summary.
	tok = csrfToken(t, client, base, "/config/parameters")
	resp := formReq(t, client, http.MethodPost, base+"/transactions?period=2026-06&scope=all", url.Values{
		"_csrf": {tok}, "label": {"Café du coin"}, "category_id": {cat}, "account_id": {fID},
		"amount": {"3,50"}, "status": {"cleared"}, "op_date": {"15/06"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("quick-entry = %d, want 200", resp.StatusCode)
	}
	body := bodyOf(t, resp)
	for _, want := range []string{"Café du coin", `id="jbody"`, `id="month-summary"`, "hx-swap-oob"} {
		if !strings.Contains(body, want) {
			t.Errorf("create response missing %q", want)
		}
	}

	// Re-read, grab the new row id, inline-edit its status → row + OOB summary.
	page = getBody(t, client, base, "/journal?period=2026-06&scope=all")
	rowID := firstMatch(t, reRowID, page, "row id")
	tok = csrfToken(t, client, base, "/config/parameters")
	resp = formReq(t, client, http.MethodPatch, base+"/transactions/"+rowID+"?period=2026-06&scope=all", url.Values{
		"_csrf": {tok}, "status": {"pending"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("inline status edit = %d", resp.StatusCode)
	}
	if b := bodyOf(t, resp); !strings.Contains(b, "En cours") || !strings.Contains(b, `id="month-summary"`) {
		t.Errorf("status edit response missing pill / summary: %s", b)
	}

	// Server-side sort/filter re-render (raw body — a bare <tbody> fragment).
	rowsResp, _ := client.Get(base + "/journal/rows?period=2026-06&scope=all&filtered=1&sort=amount&dir=asc")
	if rows := bodyOf(t, rowsResp); !strings.Contains(rows, `id="jbody"`) {
		t.Errorf("rows fragment missing tbody")
	}

	// Delete the row → OOB summary (CSRF via header; Go does not parse a DELETE body).
	tok = csrfToken(t, client, base, "/config/parameters")
	delReq, _ := http.NewRequest(http.MethodDelete, base+"/transactions/"+rowID+"?period=2026-06&scope=all", nil)
	delReq.Header.Set("X-CSRF-Token", tok)
	resp, err := client.Do(delReq)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete = %d", resp.StatusCode)
	}
	_ = bodyOf(t, resp)
	page = getBody(t, client, base, "/journal?period=2026-06&scope=all")
	if strings.Contains(page, "Café du coin") {
		t.Error("deleted transaction still present")
	}
}
