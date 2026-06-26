// Package server wires the router, the middleware chain, and the HTTP server.
// With internal/handlers it is one of the only packages that know net/http,
// htmx, cookies, CSRF, sessions, and templates (G4).
//
// At increment 0 this is a minimal runnable stub: an empty ServeMux (I-002 —
// stdlib net/http routing) behind a placeholder root handler, enough to prove
// the cmd → config → server wiring. The full middleware chain (Recover →
// RequestContext → Session → AuthGuard → TenantContext → CSRF → Locale), the
// three-pane shell, and GET /healthz land in increment 1 (the walking skeleton).
package server

import (
	"net/http"
	"time"

	"econome/internal/config"
)

// New builds the HTTP server for the given configuration. The routing table and
// middleware chain are filled in by later increments; for now the mux only
// answers the root with a placeholder so the binary is runnable end to end.
func New(cfg *config.Config) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("EconoMe — scaffold (increment 0). No application behaviour yet.\n"))
	})

	return &http.Server{
		Addr:              cfg.Listen,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}
