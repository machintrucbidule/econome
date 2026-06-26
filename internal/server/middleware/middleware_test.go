package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRecoverConvertsPanicTo500(t *testing.T) {
	h := Recover(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	h := SecurityHeaders(true)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	for _, hdr := range []string{"X-Content-Type-Options", "X-Frame-Options", "Content-Security-Policy", "Strict-Transport-Security"} {
		if rec.Header().Get(hdr) == "" {
			t.Errorf("missing header %s", hdr)
		}
	}
}

func TestParseAcceptLanguage(t *testing.T) {
	if parseAcceptLanguage("en-US,en;q=0.9") != "en" {
		t.Error("want en")
	}
	if parseAcceptLanguage("fr-FR,fr;q=0.9") != "fr" {
		t.Error("want fr")
	}
	if parseAcceptLanguage("") != "fr" {
		t.Error("empty should default fr")
	}
}
