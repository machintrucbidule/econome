package server_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// Net-worth e2e backbone (increment 7, functional/07). Drives the real HTTP
// surface: render the Synthèse, enter a gross value → recomputed table, edit a
// past snapshot → delta on the Registre, the shared comment, the curve + range
// re-render, and the empty states.

func snapshotPost(t *testing.T, base string, client *http.Client, accID, period, gross string) *http.Response {
	t.Helper()
	tok := csrfToken(t, client, base, "/networth?period="+period)
	return formReq(t, client, http.MethodPost, base+"/snapshots?period="+period, url.Values{
		"_csrf": {tok}, "account_id": {accID}, "gross_value": {gross},
	})
}

func TestNetWorthRendersAndRecomputes(t *testing.T) {
	ts, client := setupOwner(t)
	base := ts.URL
	la := mkAccountID(t, base, client, "Livret A", "passbook", "none")
	pea := mkAccountID(t, base, client, "PEA", "securities", "none")

	page := getBody(t, client, base, "/networth?period=2026-06")
	for _, want := range []string{"Patrimoine total", "Commentaire du mois", "Livret A", "PEA net", "Synthèse"} {
		if !strings.Contains(page, want) {
			t.Errorf("synthèse missing %q", want)
		}
	}

	// Enter a gross value → table recomputes (fragment, 200).
	if resp := snapshotPost(t, base, client, la, "2026-06", "14200"); resp.StatusCode != http.StatusOK {
		t.Fatalf("POST snapshot = %d", resp.StatusCode)
	} else {
		body := bodyOf(t, resp)
		if !strings.Contains(body, "nw-table") || !strings.Contains(body, "nw-cards") {
			t.Errorf("snapshot response missing table/cards fragments")
		}
	}

	// A prior month → the focus month gains a delta (1 420 − 1 400 = +200,00).
	if resp := snapshotPost(t, base, client, la, "2026-05", "14000"); resp.StatusCode != http.StatusOK {
		t.Fatalf("POST prior snapshot = %d", resp.StatusCode)
	} else {
		_ = bodyOf(t, resp)
	}
	page = getBody(t, client, base, "/networth?period=2026-06")
	if !strings.Contains(page, "200,00") {
		t.Errorf("synthèse missing the +200,00 month delta")
	}
	// A second support → the "Le reste" card appears (Total + livret + le reste),
	// and editable rows carry the explicit delete ✕ (I-037/I-035, D4 choices).
	_ = bodyOf(t, snapshotPost(t, base, client, pea, "2026-06", "12000"))
	page = getBody(t, client, base, "/networth?period=2026-06")
	if !strings.Contains(page, "Le reste") {
		t.Errorf("synthèse missing the \"Le reste\" card")
	}
	if !strings.Contains(page, "snapdel") {
		t.Errorf("editable snapshot row missing the delete ✕")
	}

	// The ✕ deletes the snapshot (L7) and recomputes. CSRF rides the header (the
	// htmx hx-delete inherits the .app hx-headers; Go does not parse a DELETE body).
	tok := csrfToken(t, client, base, "/networth?period=2026-06")
	req, _ := http.NewRequest(http.MethodDelete, base+"/snapshots/1?period=2026-06", nil)
	req.Header.Set("X-CSRF-Token", tok)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("DELETE snapshot: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE snapshot = %d", resp.StatusCode)
	}
	_ = bodyOf(t, resp)
}

func TestNetWorthCommentSharedWithRegister(t *testing.T) {
	ts, client := setupOwner(t)
	base := ts.URL
	la := mkAccountID(t, base, client, "Livret A", "passbook", "none")
	_ = bodyOf(t, snapshotPost(t, base, client, la, "2026-06", "14200"))

	tok := csrfToken(t, client, base, "/networth?period=2026-06")
	resp := formReq(t, client, http.MethodPut, base+"/networth/2026-06/comment", url.Values{
		"_csrf": {tok}, "comment": {"Versement Livret A ++"},
	})
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT comment = %d, want 204", resp.StatusCode)
	}
	_ = bodyOf(t, resp)

	// The same per-month comment shows on both surfaces.
	if syn := getBody(t, client, base, "/networth?period=2026-06"); !strings.Contains(syn, "Versement Livret A ++") {
		t.Errorf("synthèse comment box missing the saved comment")
	}
	if reg := getBody(t, client, base, "/register?period=2026-06"); !strings.Contains(reg, "Versement Livret A ++") {
		t.Errorf("registre cell missing the shared comment")
	}
}

func TestRegisterCurveAndStates(t *testing.T) {
	ts, client := setupOwner(t)
	base := ts.URL

	// No history yet → empty state.
	if reg := getBody(t, client, base, "/register?period=2026-06"); !strings.Contains(reg, "Aucun mois enregistré") {
		t.Errorf("registre empty state missing")
	}

	la := mkAccountID(t, base, client, "Livret A", "passbook", "none")
	_ = bodyOf(t, snapshotPost(t, base, client, la, "2026-05", "14000"))
	_ = bodyOf(t, snapshotPost(t, base, client, la, "2026-06", "14200"))

	reg := getBody(t, client, base, "/register?period=2026-06")
	for _, want := range []string{"Évolution du patrimoine", "<svg", "Total", "12 mois"} {
		if !strings.Contains(reg, want) {
			t.Errorf("registre missing %q", want)
		}
	}
	// Range re-render returns just the chart fragment.
	chart := getBody(t, client, base, "/register/chart?range=6&period=2026-06")
	if !strings.Contains(chart, "<svg") || !strings.Contains(chart, "nw-chart") {
		t.Errorf("range chart fragment missing svg/nw-chart")
	}
}

// TestNetworthStylesheetDefinesClasses guards the Patrimoine screens against the
// #24 class regression: every promoted patrimoine selector must be in the served
// stylesheet.
func TestNetworthStylesheetDefinesClasses(t *testing.T) {
	ts, client := newTestServer(t)
	resp, err := client.Get(ts.URL + "/assets/econome.css")
	if err != nil {
		t.Fatalf("GET econome.css: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	css := bodyOf(t, resp)
	for _, sel := range []string{
		".metrics", ".ptable", ".sub-row", ".tot-row", ".ann", ".commentbox",
		".dot", ".pos2", ".neg2", ".nul2", ".rtable", "tr.cur", ".rfoot", ".rangeseg",
	} {
		if !strings.Contains(css, sel) {
			t.Errorf("econome.css missing patrimoine selector %q", sel)
		}
	}
}
