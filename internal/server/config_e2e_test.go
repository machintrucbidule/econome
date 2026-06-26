package server_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// setupOwner creates the owner and returns an authenticated client (the setup
// response opens a session stored in the client's jar).
func setupOwner(t *testing.T) (*httptest.Server, *http.Client) {
	t.Helper()
	ts, client := newTestServer(t)
	tok := csrfToken(t, client, ts.URL, "/setup")
	resp := postForm(t, client, ts.URL, "/setup", url.Values{
		"_csrf": {tok}, "email": {"owner@example.org"},
		"password": {"Tr0ub4dour&3xtra"}, "password_confirm": {"Tr0ub4dour&3xtra"},
		"language": {"fr"}, "currency": {"EUR"},
	})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("setup = %d", resp.StatusCode)
	}
	return ts, client
}

func formReq(t *testing.T, client *http.Client, method, urlStr string, form url.Values) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, urlStr, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, urlStr, err)
	}
	return resp
}

func bodyOf(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	b := make([]byte, 0, 4096)
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		b = append(b, buf[:n]...)
		if err != nil {
			break
		}
	}
	return string(b)
}

func TestParametersRenders(t *testing.T) {
	ts, client := setupOwner(t)
	body := getBody(t, client, ts.URL, "/config/parameters")
	for _, want := range []string{"Comptes", "Épargne & fiscalité", "Localisation", "Préférences", "Import bancaire (DSP2)"} {
		if !strings.Contains(body, want) {
			t.Errorf("parameters page missing %q", want)
		}
	}
}

func TestAccountCreateAndValidation(t *testing.T) {
	ts, client := setupOwner(t)
	tok := csrfToken(t, client, ts.URL, "/config/parameters")

	// Create a current account → 200, the row appears in the refreshed card.
	resp := formReq(t, client, http.MethodPost, ts.URL+"/config/accounts", url.Values{
		"_csrf": {tok}, "name": {"Fortuneo"}, "type": {"current"}, "month_end_policy": {"sweep"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create account = %d, want 200", resp.StatusCode)
	}
	if b := bodyOf(t, resp); !strings.Contains(b, "Fortuneo") || !strings.Contains(b, `id="comptes-card"`) {
		t.Errorf("create response missing account row / OOB card: %s", b)
	}

	// Invalid (empty name) → 422 with the inline field error.
	resp = formReq(t, client, http.MethodPost, ts.URL+"/config/accounts", url.Values{
		"_csrf": {tok}, "name": {""}, "type": {"current"}, "month_end_policy": {"sweep"},
	})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("empty name = %d, want 422", resp.StatusCode)
	}
	if b := bodyOf(t, resp); !strings.Contains(b, "requis") {
		t.Errorf("422 response missing field error: %s", b)
	}

	// Duplicate name → 422 (field error, not a 409).
	resp = formReq(t, client, http.MethodPost, ts.URL+"/config/accounts", url.Values{
		"_csrf": {tok}, "name": {"Fortuneo"}, "type": {"current"}, "month_end_policy": {"sweep"},
	})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("duplicate name = %d, want 422", resp.StatusCode)
	}
}

func TestAccountArchiveToggle(t *testing.T) {
	ts, client := setupOwner(t)
	tok := csrfToken(t, client, ts.URL, "/config/parameters")

	// Create then archive an account.
	resp := formReq(t, client, http.MethodPost, ts.URL+"/config/accounts", url.Values{
		"_csrf": {tok}, "name": {"Vieux"}, "type": {"current"}, "month_end_policy": {"carry"},
	})
	bodyOf(t, resp)

	// Find the id by parsing the parameters page edit link.
	page := getBody(t, client, ts.URL, "/config/parameters")
	id := extractAccountID(t, page)

	resp = formReq(t, client, http.MethodPost, ts.URL+"/config/accounts/"+id+"/archive", url.Values{"_csrf": {tok}})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("archive = %d, want 200", resp.StatusCode)
	}
	if b := bodyOf(t, resp); !strings.Contains(b, "archivé") {
		t.Errorf("archive response missing archived badge: %s", b)
	}
}

func TestSettingsPatch(t *testing.T) {
	ts, client := setupOwner(t)
	tok := csrfToken(t, client, ts.URL, "/config/parameters")

	// Épargne card: valid rate persists (200).
	resp := formReq(t, client, http.MethodPatch, ts.URL+"/config/settings", url.Values{
		"_csrf": {tok}, "card": {"epargne"}, "pea_social_charge_rate": {"17,2"}, "pea_initial_deposit": {"9 000,00"}, "secured_savings_basis": {"all_planned"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("settings epargne = %d, want 200", resp.StatusCode)
	}

	// Out-of-range rate → 422 (rate-bound, not a DB 500).
	resp = formReq(t, client, http.MethodPatch, ts.URL+"/config/settings", url.Values{
		"_csrf": {tok}, "card": {"epargne"}, "pea_social_charge_rate": {"150"},
	})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("bad rate = %d, want 422", resp.StatusCode)
	}

	// Localisation: switch to English, reflected on the next render.
	resp = formReq(t, client, http.MethodPatch, ts.URL+"/config/settings", url.Values{
		"_csrf": {tok}, "card": {"localisation"}, "language": {"en"}, "currency": {"EUR"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("settings localisation = %d, want 200", resp.StatusCode)
	}
	if page := getBody(t, client, ts.URL, "/config/parameters"); !strings.Contains(page, "Preferences") {
		t.Errorf("locale change to EN not reflected (no English labels)")
	}
}

func TestCascadeReorder(t *testing.T) {
	ts, client := setupOwner(t)
	tok := csrfToken(t, client, ts.URL, "/config/parameters")

	// Two savings accounts.
	for _, n := range []string{"Livret A", "LDDS"} {
		bodyOf(t, formReq(t, client, http.MethodPost, ts.URL+"/config/accounts", url.Values{
			"_csrf": {tok}, "name": {n}, "type": {"passbook"}, "month_end_policy": {"none"},
		}))
	}
	page := getBody(t, client, ts.URL, "/config/parameters")
	ids := extractAllAccountIDs(t, page)
	if len(ids) < 2 {
		t.Fatalf("want >=2 accounts, got %d", len(ids))
	}
	resp := formReq(t, client, http.MethodPost, ts.URL+"/config/accounts/reorder", url.Values{
		"_csrf": {tok}, "order": {ids[1] + "," + ids[0]},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reorder = %d, want 200", resp.StatusCode)
	}
	if b := bodyOf(t, resp); !strings.Contains(b, `id="cascade"`) {
		t.Errorf("reorder response missing cascade list: %s", b)
	}
}

// extractAccountID pulls the first account id from an edit link in the page.
func extractAccountID(t *testing.T, page string) string {
	t.Helper()
	ids := extractAllAccountIDs(t, page)
	if len(ids) == 0 {
		t.Fatal("no account id found in page")
	}
	return ids[0]
}

func extractAllAccountIDs(t *testing.T, page string) []string {
	t.Helper()
	var ids []string
	marker := "/config/accounts/"
	for i := 0; ; {
		j := strings.Index(page[i:], marker)
		if j < 0 {
			break
		}
		start := i + j + len(marker)
		end := start
		for end < len(page) && page[end] >= '0' && page[end] <= '9' {
			end++
		}
		if end > start && end < len(page) && (page[end] == '/') {
			ids = append(ids, page[start:end])
		}
		i = end
	}
	// de-dup preserving order
	seen := map[string]bool{}
	var out []string
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
