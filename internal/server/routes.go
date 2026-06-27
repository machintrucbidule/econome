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
// [AuthGuard → TenantContext] → CSRF → Locale (technical/04 §2, I-002 ServeMux).
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
		middleware.CSRF(secret, cfg.BehindTLS), middleware.Locale)...)
	protected := middleware.Chain(append(append([]middleware.Middleware{}, common...),
		middleware.AuthGuard, middleware.TenantContext(svc), middleware.ForcePasswordChange, middleware.CSRF(secret, cfg.BehindTLS), middleware.Locale)...)
	// Admin-only chain adds the admin gate (non-admin ⇒ 404) after tenant context.
	admin := middleware.Chain(append(append([]middleware.Middleware{}, common...),
		middleware.AuthGuard, middleware.TenantContext(svc), middleware.ForcePasswordChange, middleware.AdminGuard, middleware.CSRF(secret, cfg.BehindTLS), middleware.Locale)...)

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
	mux.Handle("GET /invite/{token}", public(http.HandlerFunc(h.InviteGet)))
	mux.Handle("POST /invite/{token}", public(http.HandlerFunc(h.InvitePost)))

	mux.Handle("POST /logout", protected(http.HandlerFunc(h.Logout)))

	// Forced password change (must_change_password) — full page, exempt from the
	// ForcePasswordChange redirect so the user can complete it.
	mux.Handle("GET /password", protected(http.HandlerFunc(h.ForcedPasswordGet)))

	// Self-service security (functional/01 §5–§7).
	mux.Handle("GET /security/2fa", protected(http.HandlerFunc(h.TOTPEnrolGet)))
	mux.Handle("POST /security/2fa", protected(http.HandlerFunc(h.TOTPConfirm)))
	mux.Handle("GET /security/2fa/disable", protected(http.HandlerFunc(h.TOTPDisableGet)))
	mux.Handle("POST /security/2fa/disable", protected(http.HandlerFunc(h.TOTPDisablePost)))
	mux.Handle("POST /security/2fa/backup", protected(http.HandlerFunc(h.BackupRegenerate)))
	mux.Handle("GET /security/password", protected(http.HandlerFunc(h.PasswordChangeGet)))
	mux.Handle("POST /security/password", protected(http.HandlerFunc(h.PasswordChangePost)))
	mux.Handle("GET /security/email", protected(http.HandlerFunc(h.EmailChangeGet)))
	mux.Handle("POST /security/email", protected(http.HandlerFunc(h.EmailChangePost)))
	mux.Handle("POST /security/sessions/{id}/revoke", protected(http.HandlerFunc(h.SessionRevoke)))
	mux.Handle("POST /security/sessions/revoke-all", protected(http.HandlerFunc(h.SessionsRevokeAll)))

	// Admin: invitations + user management (functional/01 §4/§8; admin-gated → 404).
	mux.Handle("GET /admin/invitations/new", admin(http.HandlerFunc(h.InviteFormGet)))
	mux.Handle("POST /admin/invitations", admin(http.HandlerFunc(h.InvitationCreate)))
	mux.Handle("POST /admin/invitations/{id}/revoke", admin(http.HandlerFunc(h.InvitationRevoke)))
	mux.Handle("GET /admin/users/{id}", admin(http.HandlerFunc(h.UserManageGet)))
	mux.Handle("POST /admin/users/{id}/deactivate", admin(http.HandlerFunc(h.UserDeactivate)))
	mux.Handle("POST /admin/users/{id}/reactivate", admin(http.HandlerFunc(h.UserReactivate)))
	mux.Handle("POST /admin/users/{id}/reset-2fa", admin(http.HandlerFunc(h.UserResetTOTP)))
	mux.Handle("POST /admin/users/{id}/reset-password", admin(http.HandlerFunc(h.UserResetPassword)))

	// Budget — Prévisionnel (forecast) is the landing screen (functional/02 §6).
	mux.Handle("GET /{$}", protected(http.HandlerFunc(h.ForecastGet)))
	mux.Handle("PATCH /allocations/{env}", protected(http.HandlerFunc(h.AllocationPatch)))
	mux.Handle("POST /transfers/end-of-month", protected(http.HandlerFunc(h.EndOfMonthTransfer)))

	// Month lifecycle controls (increment 8): close/unlock + regenerate (L1/L9).
	mux.Handle("POST /periods/{period}/lock", protected(http.HandlerFunc(h.PeriodLock)))
	mux.Handle("POST /periods/{period}/unlock", protected(http.HandlerFunc(h.PeriodUnlock)))
	mux.Handle("POST /periods/{period}/regenerate", protected(http.HandlerFunc(h.PeriodRegenerate)))

	// Budget — Journal (increment 6c).
	mux.Handle("GET /journal", protected(http.HandlerFunc(h.JournalGet)))
	mux.Handle("GET /journal/rows", protected(http.HandlerFunc(h.JournalRows)))
	mux.Handle("POST /transactions", protected(http.HandlerFunc(h.TransactionCreate)))
	mux.Handle("PATCH /transactions/{id}", protected(http.HandlerFunc(h.TransactionPatch)))
	mux.Handle("DELETE /transactions/{id}", protected(http.HandlerFunc(h.TransactionDelete)))
	mux.Handle("PUT /ui/expand", protected(http.HandlerFunc(h.UIExpand)))

	// Patrimoine — Net worth (increment 7): Synthèse + Registre. Snapshots and
	// comments are always editable, independent of the budget lock (L7).
	mux.Handle("GET /networth", protected(http.HandlerFunc(h.NetWorthGet)))
	mux.Handle("POST /snapshots", protected(http.HandlerFunc(h.SnapshotUpsert)))
	mux.Handle("DELETE /snapshots/{id}", protected(http.HandlerFunc(h.SnapshotDelete)))
	mux.Handle("PUT /networth/{period}/comment", protected(http.HandlerFunc(h.CommentPut)))
	mux.Handle("GET /register", protected(http.HandlerFunc(h.RegisterGet)))
	mux.Handle("GET /register/chart", protected(http.HandlerFunc(h.RegisterChart)))

	// Configuration — Paramètres (increment 4, PR-a).
	mux.Handle("GET /config/parameters", protected(http.HandlerFunc(h.ParametersGet)))
	mux.Handle("GET /config/accounts/new", protected(http.HandlerFunc(h.AccountFormGet)))
	mux.Handle("GET /config/accounts/{id}/edit", protected(http.HandlerFunc(h.AccountFormGet)))
	mux.Handle("POST /config/accounts", protected(http.HandlerFunc(h.AccountCreate)))
	mux.Handle("PATCH /config/accounts/{id}", protected(http.HandlerFunc(h.AccountUpdate)))
	mux.Handle("POST /config/accounts/{id}/archive", protected(http.HandlerFunc(h.AccountArchive)))
	mux.Handle("POST /config/accounts/{id}/unarchive", protected(http.HandlerFunc(h.AccountUnarchive)))
	mux.Handle("DELETE /config/accounts/{id}", protected(http.HandlerFunc(h.AccountDelete)))
	mux.Handle("POST /config/accounts/reorder", protected(http.HandlerFunc(h.CascadeReorder)))
	mux.Handle("PATCH /config/settings", protected(http.HandlerFunc(h.SettingsPatch)))

	// Month-initialisation assistant (increment 5).
	mux.Handle("GET /month-init", protected(http.HandlerFunc(h.MonthInitGet)))
	mux.Handle("PATCH /month-init/draft", protected(http.HandlerFunc(h.MonthInitDraft)))
	mux.Handle("POST /month-init", protected(http.HandlerFunc(h.MonthInitCreate)))

	// Configuration — Envelopes (increment 4, PR-b).
	mux.Handle("GET /config/envelopes", protected(http.HandlerFunc(h.EnvelopesGet)))
	mux.Handle("GET /config/envelopes/new", protected(http.HandlerFunc(h.EnvelopeFormGet)))
	mux.Handle("GET /config/envelopes/{id}/edit", protected(http.HandlerFunc(h.EnvelopeFormGet)))
	mux.Handle("POST /config/envelopes", protected(http.HandlerFunc(h.EnvelopeCreate)))
	mux.Handle("PATCH /config/envelopes/{id}", protected(http.HandlerFunc(h.EnvelopeUpdate)))
	mux.Handle("POST /config/envelopes/{id}/archive", protected(http.HandlerFunc(h.EnvelopeArchive)))
	mux.Handle("POST /config/envelopes/{id}/unarchive", protected(http.HandlerFunc(h.EnvelopeUnarchive)))
	mux.Handle("DELETE /config/envelopes/{id}", protected(http.HandlerFunc(h.EnvelopeDelete)))

	return &http.Server{
		Addr:              cfg.Listen,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}
