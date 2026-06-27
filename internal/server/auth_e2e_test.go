package server_test

import (
	"html"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// End-to-end auth-surface tests (increment 8, functional/01 §3/§4/§8): the
// invitation issue→accept flow, the admin gate (404 for non-admins), and the
// inline 2FA login step. They exercise the real router + templates over httptest.

// ownerClient seeds the owner and returns its logged-in client + base URL.
func ownerClient(t *testing.T) (string, *http.Client) {
	t.Helper()
	ts, _ := newTestServer(t)
	base := ts.URL
	c := clientFor()
	st := csrfToken(t, c, base, "/setup")
	postForm(t, c, base, "/setup", url.Values{
		"_csrf": {st}, "email": {"owner@example.org"},
		"password": {"Tr0ub4dour&3xtra"}, "password_confirm": {"Tr0ub4dour&3xtra"},
		"language": {"fr"}, "currency": {"EUR"},
	})
	return base, c
}

func clientFor() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

var inviteLinkRE = regexp.MustCompile(`/invite/([A-Za-z0-9_-]+)`)

func TestInvitationIssueAndAccept(t *testing.T) {
	base, admin := ownerClient(t)

	// The owner sees the Users panel with an Invite control.
	params := getBody(t, admin, base, "/config/parameters")
	if !strings.Contains(params, "Utilisateurs") || !strings.Contains(params, "Inviter") {
		t.Fatal("parameters missing the Users panel")
	}

	// Issue an invitation; the response shows the one-time link.
	tok := csrfTokenFromParams(t, admin, base)
	resp := postForm(t, admin, base, "/admin/invitations", url.Values{"_csrf": {tok}, "email": {"famille@example.org"}, "role": {"member"}})
	body := readBody(t, resp)
	m := inviteLinkRE.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no invitation link in response: %s", body)
	}
	token := m[1]

	// A fresh (anonymous) client opens the link → the acceptance form.
	invitee := clientFor()
	form := getBody(t, invitee, base, "/invite/"+token)
	if !strings.Contains(form, "Créer votre compte") {
		t.Fatalf("invite page missing the acceptance form: %s", form)
	}

	// Accept: create the invited user; lands authenticated on the shell.
	itok := csrfToken(t, invitee, base, "/invite/"+token)
	r := postForm(t, invitee, base, "/invite/"+token, url.Values{
		"_csrf": {itok}, "email": {"famille@example.org"},
		"password": {"An0ther&Str0ng"}, "password_confirm": {"An0ther&Str0ng"}, "language": {"fr"},
	})
	if r.StatusCode != http.StatusSeeOther || r.Header.Get("Location") != "/" {
		t.Fatalf("accept = %d -> %q, want 303 /", r.StatusCode, r.Header.Get("Location"))
	}

	// The token is single-use: re-opening shows the invalid state.
	again := getBody(t, clientFor(), base, "/invite/"+token)
	if !strings.Contains(again, "Invitation invalide") {
		t.Error("consumed invitation should show the invalid state")
	}
}

func TestTOTPEnrolRendersQRDataURI(t *testing.T) {
	base, admin := ownerClient(t)
	// GET /security/2fa returns the enrol modal with the QR as a data: URI; the
	// data URI must survive html/template's URL filter (typed template.URL), not
	// be rewritten to the #ZgotmplZ placeholder.
	body := getBody(t, admin, base, "/security/2fa")
	if !strings.Contains(body, "data:image/png;base64,") {
		t.Fatal("2FA enrol modal should embed the QR as a data:image/png base64 URI")
	}
	if strings.Contains(body, "ZgotmplZ") {
		t.Fatal("QR data URI was filtered by html/template (#ZgotmplZ) — needs template.URL")
	}
}

func TestAdminGateBlocksNonAdmin(t *testing.T) {
	base, admin := ownerClient(t)

	// Invite + accept a MEMBER.
	tok := csrfTokenFromParams(t, admin, base)
	resp := postForm(t, admin, base, "/admin/invitations", url.Values{"_csrf": {tok}, "role": {"member"}})
	token := inviteLinkRE.FindStringSubmatch(readBody(t, resp))[1]
	member := clientFor()
	itok := csrfToken(t, member, base, "/invite/"+token)
	postForm(t, member, base, "/invite/"+token, url.Values{
		"_csrf": {itok}, "email": {"member@example.org"},
		"password": {"An0ther&Str0ng"}, "password_confirm": {"An0ther&Str0ng"}, "language": {"fr"},
	})

	// The member cannot reach an admin route: 404 (never 403), and no Users panel.
	r, _ := member.Get(base + "/admin/invitations/new")
	if r.StatusCode != http.StatusNotFound {
		t.Fatalf("member GET /admin/invitations/new = %d, want 404", r.StatusCode)
	}
	params := getBody(t, member, base, "/config/parameters")
	if strings.Contains(params, "Utilisateurs & invitations") {
		t.Error("member should not see the admin Users panel")
	}
}

func TestForcedPasswordChangeFlow(t *testing.T) {
	base, admin := ownerClient(t)

	// Invite + accept a member.
	tok := csrfTokenFromParams(t, admin, base)
	resp := postForm(t, admin, base, "/admin/invitations", url.Values{"_csrf": {tok}, "role": {"member"}})
	token := inviteLinkRE.FindStringSubmatch(readBody(t, resp))[1]
	member := clientFor()
	itok := csrfToken(t, member, base, "/invite/"+token)
	postForm(t, member, base, "/invite/"+token, url.Values{
		"_csrf": {itok}, "email": {"member@example.org"},
		"password": {"An0ther&Str0ng"}, "password_confirm": {"An0ther&Str0ng"}, "language": {"fr"},
	})

	// Resolve the member's id from the admin Users panel link, then reset its
	// password; the response shows the one-time temporary password.
	pr, _ := admin.Get(base + "/config/parameters")
	idm := regexp.MustCompile(`(?s)member@example\.org.*?/admin/users/(\d+)`).FindStringSubmatch(readBody(t, pr))
	if idm == nil {
		t.Fatal("member row not found in Users panel")
	}
	id := idm[1]
	atok := csrfTokenFromParams(t, admin, base)
	rp := postForm(t, admin, base, "/admin/users/"+id+"/reset-password", url.Values{"_csrf": {atok}})
	tmp := regexp.MustCompile(`<input class="inp" value="([^"]+)" readonly`).FindStringSubmatch(readBody(t, rp))
	if tmp == nil {
		t.Fatal("reset-password did not surface a temporary password")
	}
	temp := html.UnescapeString(tmp[1]) // the attribute is HTML-escaped (e.g. & → &amp;)

	// The member's sessions were revoked; re-login with the temp password.
	fresh := clientFor()
	ltok := csrfToken(t, fresh, base, "/login")
	postForm(t, fresh, base, "/login", url.Values{"_csrf": {ltok}, "email": {"member@example.org"}, "password": {temp}})

	// Any protected page now redirects to the forced change-password screen.
	r, _ := fresh.Get(base + "/config/parameters")
	if r.StatusCode != http.StatusSeeOther || r.Header.Get("Location") != "/password" {
		t.Fatalf("must_change redirect = %d -> %q, want 303 /password", r.StatusCode, r.Header.Get("Location"))
	}
	page := getBody(t, fresh, base, "/password")
	if !strings.Contains(page, "Changer votre mot de passe") {
		t.Fatal("forced page missing")
	}

	// Change the password; the flag clears and the user is let back in.
	ptok := csrfToken(t, fresh, base, "/password")
	cp := postForm(t, fresh, base, "/security/password", url.Values{
		"_csrf": {ptok}, "mode": {"force"}, "current_password": {temp},
		"password": {"Memb3r&Chos3n"}, "password_confirm": {"Memb3r&Chos3n"},
	})
	if cp.StatusCode != http.StatusSeeOther {
		t.Fatalf("forced change = %d, want 303", cp.StatusCode)
	}
	r2, _ := fresh.Get(base + "/config/parameters")
	if r2.StatusCode != http.StatusOK {
		t.Fatalf("after change, /config/parameters = %d, want 200", r2.StatusCode)
	}
}

// csrfTokenFromParams returns a CSRF token valid for the logged-in client (taken
// from any rendered form on the parameters page).
func csrfTokenFromParams(t *testing.T, c *http.Client, base string) string {
	t.Helper()
	return csrfToken(t, c, base, "/config/parameters")
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	b := make([]byte, 0, 8192)
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
