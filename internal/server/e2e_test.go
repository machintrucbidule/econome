package server_test

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"

	"econome/internal/config"
	"econome/internal/i18n"
	"econome/internal/repo"
	"econome/internal/server"
	"econome/internal/services"
	"econome/internal/view"
	"econome/migrations"
)

func newTestServer(t *testing.T) (*httptest.Server, *http.Client) {
	t.Helper()
	dir := t.TempDir()
	db, err := repo.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := repo.Migrate(context.Background(), db, migrations.FS, filepath.Join(dir, "backups")); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store := repo.New(db)
	catalog, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	rdr, err := view.New(catalog)
	if err != nil {
		t.Fatal(err)
	}
	svc := services.New(services.Deps{
		Users:          store.Users,
		Sessions:       store.Sessions,
		Settings:       store.Settings,
		Accounts:       store.Accounts,
		Categories:     store.Categories,
		Envelopes:      store.Envelopes,
		Allocations:    store.Allocations,
		Transactions:   store.Transactions,
		Snapshots:      store.Snapshots,
		NetworthMonths: store.NetworthMonths,
		Periods:        store.Periods,
		PeriodEvents:   store.PeriodEvents,
		Labels:         store.Labels,
		UIPreferences:  store.UIPreferences,
		Tx:             store,
		Secret:         []byte("secret-0123456789abcdef0123456789"),
	})
	cfg := &config.Config{Listen: ":0", BehindTLS: false, DefaultLocale: "fr"}
	ts := httptest.NewServer(server.New(cfg, svc, rdr).Handler)

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	t.Cleanup(func() { ts.Close(); _ = db.Close() })
	return ts, client
}

func csrfToken(t *testing.T, client *http.Client, base, path string) string {
	t.Helper()
	resp, err := client.Get(base + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	tok, ok := doc.Find(`input[name="_csrf"]`).First().Attr("value")
	if !ok || tok == "" {
		t.Fatalf("no _csrf token on %s", path)
	}
	return tok
}

func postForm(t *testing.T, client *http.Client, base, path string, form url.Values) *http.Response {
	t.Helper()
	resp, err := client.PostForm(base+path, form)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

func TestWalkingSkeletonFlow(t *testing.T) {
	ts, client := newTestServer(t)
	base := ts.URL

	// Healthz works on a fresh, empty instance.
	if resp, _ := client.Get(base + "/healthz"); resp.StatusCode != http.StatusOK {
		t.Fatalf("/healthz = %d, want 200", resp.StatusCode)
	}

	// Zero users: every route redirects to /setup.
	resp, _ := client.Get(base + "/")
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/setup" {
		t.Fatalf("GET / on empty instance = %d -> %q, want 303 /setup", resp.StatusCode, resp.Header.Get("Location"))
	}

	// CSRF protects setup: a POST without a token is rejected.
	bad := postForm(t, client, base, "/setup", url.Values{"email": {"owner@example.org"}})
	if bad.StatusCode != http.StatusForbidden {
		t.Fatalf("setup without csrf = %d, want 403", bad.StatusCode)
	}

	// Create the owner.
	tok := csrfToken(t, client, base, "/setup")
	form := url.Values{
		"_csrf": {tok}, "email": {"owner@example.org"},
		"password": {"Tr0ub4dour&3xtra"}, "password_confirm": {"Tr0ub4dour&3xtra"},
		"language": {"fr"}, "currency": {"EUR"},
	}
	resp = postForm(t, client, base, "/setup", form)
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/" {
		t.Fatalf("setup = %d -> %q, want 303 /", resp.StatusCode, resp.Header.Get("Location"))
	}

	// The forecast (budget landing) renders, authenticated: with no month yet
	// created it shows the "month not created" state offering the assistant.
	home := getBody(t, client, base, "/")
	for _, want := range []string{"Prévisionnel", "Ce mois n'est pas encore créé", "Préparer"} {
		if !strings.Contains(home, want) {
			t.Errorf("forecast landing missing %q", want)
		}
	}
	if !strings.Contains(home, "owner@example.org") || !strings.Contains(home, "Se déconnecter") {
		t.Error("shell missing email or logout control")
	}

	// Once an owner exists, /setup redirects to /login.
	resp, _ = client.Get(base + "/setup")
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/login" {
		t.Fatalf("GET /setup after owner = %d -> %q, want 303 /login", resp.StatusCode, resp.Header.Get("Location"))
	}

	// Logout revokes the session.
	tok = csrfToken(t, client, base, "/login")
	// (login GET while authenticated still serves a form with a token via the seed cookie)
	resp = postForm(t, client, base, "/logout", url.Values{"_csrf": {tok}})
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/login" {
		t.Fatalf("logout = %d -> %q, want 303 /login", resp.StatusCode, resp.Header.Get("Location"))
	}
	resp, _ = client.Get(base + "/")
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/login" {
		t.Fatalf("GET / after logout = %d -> %q, want 303 /login", resp.StatusCode, resp.Header.Get("Location"))
	}

	// Wrong password -> generic error; correct password -> shell.
	tok = csrfToken(t, client, base, "/login")
	resp = postForm(t, client, base, "/login", url.Values{"_csrf": {tok}, "email": {"owner@example.org"}, "password": {"nope"}})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad login = %d, want 401", resp.StatusCode)
	}
	tok = csrfToken(t, client, base, "/login")
	resp = postForm(t, client, base, "/login", url.Values{"_csrf": {tok}, "email": {"owner@example.org"}, "password": {"Tr0ub4dour&3xtra"}})
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/" {
		t.Fatalf("good login = %d -> %q, want 303 /", resp.StatusCode, resp.Header.Get("Location"))
	}
}

func TestAuthGuardHTMXRedirect(t *testing.T) {
	ts, _ := newTestServer(t)
	// A fresh client (no session). Seed an owner so SetupGuard does not intercept.
	seedOwner(t, ts)

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/", nil)
	req.Header.Set("HX-Request", "true")
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized || resp.Header.Get("HX-Redirect") != "/login" {
		t.Fatalf("htmx unauth = %d HX-Redirect=%q, want 401 /login", resp.StatusCode, resp.Header.Get("HX-Redirect"))
	}
}

// TestAuthStylesheetDefinesLayoutClasses guards against the regression where the
// setup/login templates reference auth-layout classes (.authstage, .auth-card, …)
// that were never ported from the mockup page styles into the shared econome.css,
// leaving the first-run page unstyled. Every class the auth templates depend on
// must be defined in the served stylesheet.
func TestAuthStylesheetDefinesLayoutClasses(t *testing.T) {
	ts, client := newTestServer(t)
	resp, err := client.Get(ts.URL + "/assets/econome.css")
	if err != nil {
		t.Fatalf("GET econome.css: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("econome.css = %d, want 200", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read css: %v", err)
	}
	css := string(body)
	for _, sel := range []string{
		".authstage", ".authwrap", ".auth-card", ".brand", ".tagline",
		".fld", ".pwrules", ".ferr", ".btn-block", ".errbox", ".warnbox",
	} {
		if !strings.Contains(css, sel) {
			t.Errorf("econome.css missing auth-layout selector %q (auth pages would render unstyled)", sel)
		}
	}
}

// TestForecastStylesheetDefinesClasses guards the forecast screen against the
// same regression as the auth pages (#24): every design-system class the
// forecast template depends on must be defined in the served stylesheet.
func TestForecastStylesheetDefinesClasses(t *testing.T) {
	ts, client := newTestServer(t)
	resp, err := client.Get(ts.URL + "/assets/econome.css")
	if err != nil {
		t.Fatalf("GET econome.css: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read css: %v", err)
	}
	css := string(body)
	for _, sel := range []string{
		".tog", ".chev", ".child", ".drill", ".di", ".dh", ".jlink", ".bar",
		".pill", ".agg", ".pill-acc", ".tl", ".chart", ".leg", ".figs", ".fig",
		".save", ".watch", ".mnav", ".mp", ".tabs", ".lockbar",
	} {
		if !strings.Contains(css, sel) {
			t.Errorf("econome.css missing forecast selector %q", sel)
		}
	}
}

// TestJournalStylesheetDefinesClasses guards the journal screen against the #24
// regression: every journal class must be in the served stylesheet (they were
// ported from the mockup's page <style>).
func TestJournalStylesheetDefinesClasses(t *testing.T) {
	ts, client := newTestServer(t)
	resp, err := client.Get(ts.URL + "/assets/econome.css")
	if err != nil {
		t.Fatalf("GET econome.css: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	css := string(body)
	for _, sel := range []string{
		".jtable", ".statpill", ".catpill", ".srt", ".panel-card", ".flab",
		".vtext", ".actcol", ".chip-period", ".xfer", ".ltext", ".sk-row",
	} {
		if !strings.Contains(css, sel) {
			t.Errorf("econome.css missing journal selector %q", sel)
		}
	}
}

func getBody(t *testing.T, client *http.Client, base, path string) string {
	t.Helper()
	resp, err := client.Get(base + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	doc, _ := goquery.NewDocumentFromReader(resp.Body)
	return doc.Text() + docHTML(doc)
}

func docHTML(doc *goquery.Document) string {
	h, _ := doc.Html()
	return h
}

func seedOwner(t *testing.T, ts *httptest.Server) {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	tok := csrfToken(t, c, ts.URL, "/setup")
	postForm(t, c, ts.URL, "/setup", url.Values{
		"_csrf": {tok}, "email": {"owner@example.org"},
		"password": {"Tr0ub4dour&3xtra"}, "password_confirm": {"Tr0ub4dour&3xtra"},
		"language": {"fr"}, "currency": {"EUR"},
	})
}
