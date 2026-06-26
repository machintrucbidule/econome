package server_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// makeAccount creates a current account and returns its id (parsed from the
// parameters page edit link).
func makeAccount(t *testing.T, client *http.Client, base, csrf, name string) string {
	t.Helper()
	bodyOf(t, formReq(t, client, http.MethodPost, base+"/config/accounts", url.Values{
		"_csrf": {csrf}, "name": {name}, "type": {"current"}, "month_end_policy": {"sweep"},
	}))
	return extractAccountID(t, getBody(t, client, base, "/config/parameters"))
}

func TestEnvelopesRendersEmpty(t *testing.T) {
	ts, client := setupOwner(t)
	body := getBody(t, client, ts.URL, "/config/envelopes")
	if !strings.Contains(body, "Commencez par ajouter") {
		t.Errorf("envelopes page missing empty state")
	}
	if !strings.Contains(body, `id="env-list"`) {
		t.Errorf("envelopes page missing the list container")
	}
}

func TestEnvelopeCreateAndHierarchy(t *testing.T) {
	ts, client := setupOwner(t)
	tok := csrfToken(t, client, ts.URL, "/config/parameters")
	acc := makeAccount(t, client, ts.URL, tok, "Fortuneo")

	// Create a variable envelope under a new parent "Assurance".
	resp := formReq(t, client, http.MethodPost, ts.URL+"/config/envelopes", url.Values{
		"_csrf": {tok}, "name": {"Habitation"}, "flow_type": {"expense"}, "mode": {"variable"},
		"account_id": {acc}, "parent_id": {"__new__"}, "new_parent_name": {"Assurance"}, "default_amount": {"28,40"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create envelope = %d, want 200", resp.StatusCode)
	}
	if b := bodyOf(t, resp); !strings.Contains(b, "Habitation") || !strings.Contains(b, "Assurance") || !strings.Contains(b, `id="env-list"`) {
		t.Errorf("create response missing rows / OOB list: %s", b)
	}

	// Duplicate (same category × account) → 422.
	resp = formReq(t, client, http.MethodPost, ts.URL+"/config/envelopes", url.Values{
		"_csrf": {tok}, "name": {"Habitation"}, "flow_type": {"expense"}, "mode": {"variable"},
		"account_id": {acc}, "parent_id": {"__new__"}, "new_parent_name": {"Assurance"},
	})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("duplicate envelope = %d, want 422", resp.StatusCode)
	}
}

func TestEnvelopeValidationE2E(t *testing.T) {
	ts, client := setupOwner(t)
	tok := csrfToken(t, client, ts.URL, "/config/parameters")
	acc := makeAccount(t, client, ts.URL, tok, "Fortuneo")

	// fixed_recurring without a frequency → 422 with an inline error.
	resp := formReq(t, client, http.MethodPost, ts.URL+"/config/envelopes", url.Values{
		"_csrf": {tok}, "name": {"Loyer"}, "flow_type": {"expense"}, "mode": {"fixed_recurring"},
		"account_id": {acc}, "frequency": {""},
	})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("fixed without frequency = %d, want 422", resp.StatusCode)
	}
	if b := bodyOf(t, resp); !strings.Contains(b, "requise") {
		t.Errorf("422 response missing frequency error: %s", b)
	}
}
