// Package server wires the router, the middleware chain, and the HTTP server.
// With internal/handlers it is one of the only packages that know net/http,
// htmx, cookies, CSRF, sessions, and templates (G4).
package server

import (
	"net/http"
	"time"

	"econome/internal/config"
	"econome/internal/handlers"
	"econome/internal/server/middleware"
	"econome/internal/services"
	"econome/internal/view"
	"econome/web/assets"
)

// New builds the HTTP server: the static asset handler, the public chain
// (setup/login), and the protected chain (shell/logout) — the full middleware
// order Recover → SecurityHeaders → RequestContext → Session → SetupGuard →
// [AuthGuard → TenantContext] → Locale → CSRF (technical/04 §2, I-002 ServeMux).
func New(cfg *config.Config, svc *services.Service, rdr *view.Renderer) *http.Server {
	h := handlers.New(svc, rdr, cfg.BehindTLS, cfg.TrustedProxy)
	secret := svc.Secret()
	mux := http.NewServeMux()

	common := []middleware.Middleware{
		middleware.Recover,
		middleware.SecurityHeaders(cfg.BehindTLS),
		middleware.RequestContext,
		middleware.Session(svc),
		middleware.SetupGuard(svc),
	}
	public := middleware.Chain(append(append([]middleware.Middleware{}, common...),
		middleware.Locale, middleware.CSRF(secret, cfg.BehindTLS))...)
	protected := middleware.Chain(append(append([]middleware.Middleware{}, common...),
		middleware.AuthGuard, middleware.TenantContext(svc), middleware.Locale, middleware.CSRF(secret, cfg.BehindTLS))...)

	// Static assets bypass the auth chain (Recover + security headers only).
	assetChain := middleware.Chain(middleware.Recover, middleware.SecurityHeaders(cfg.BehindTLS))
	mux.Handle("GET /assets/", assetChain(http.StripPrefix("/assets/", http.FileServer(http.FS(assets.FS)))))

	// Liveness bypasses SetupGuard so it works on a fresh, empty instance.
	mux.Handle("GET /healthz", assetChain(http.HandlerFunc(h.Healthz)))

	mux.Handle("GET /setup", public(http.HandlerFunc(h.SetupGet)))
	mux.Handle("POST /setup", public(http.HandlerFunc(h.SetupPost)))
	mux.Handle("GET /login", public(http.HandlerFunc(h.LoginGet)))
	mux.Handle("POST /login", public(http.HandlerFunc(h.LoginPost)))
	mux.Handle("POST /login/totp", public(http.HandlerFunc(h.LoginTOTP)))

	mux.Handle("POST /logout", protected(http.HandlerFunc(h.Logout)))
	mux.Handle("GET /{$}", protected(http.HandlerFunc(h.Home)))

	return &http.Server{
		Addr:              cfg.Listen,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}
