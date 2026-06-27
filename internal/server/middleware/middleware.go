package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"econome/internal/auth"
	"econome/internal/domain"
)

// Cookie names.
const (
	SessionCookie = "econome_session"
	csrfCookie    = "econome_csrf"
)

// Deps is what the chain needs from the service layer (kept narrow for testing).
type Deps interface {
	ResolveSession(ctx context.Context, rawToken string) (*domain.User, *domain.Session, error)
	Settings(ctx context.Context, userID int64) (*domain.Settings, error)
	ZeroUsers(ctx context.Context) (bool, error)
}

// Recover converts a panic into a 500 so the process stays up (technical/04 §2).
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic recovered", "err", rec, "path", r.URL.Path)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// RequestContext attaches a fresh Ctx with a request id and logs the request.
func RequestContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := newCtx()
		c.RequestID = randomID()
		ctx := withCtx(r.Context(), c)
		slog.Debug("request", "id", c.RequestID, "method", r.Method, "path", r.URL.Path)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// SecurityHeaders sets the baseline headers (technical/05 §10). HSTS is only sent
// when served behind TLS.
func SecurityHeaders(behindTLS bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "same-origin")
			h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'")
			if behindTLS {
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Session resolves the session cookie into Ctx.User/Session, clearing an
// invalid/expired cookie. No cookie ⇒ anonymous.
func Session(d Deps) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := From(r.Context())
			if cookie, err := r.Cookie(SessionCookie); err == nil && cookie.Value != "" {
				user, sess, err := d.ResolveSession(r.Context(), cookie.Value)
				switch {
				case err == nil:
					c.User, c.Session = user, sess
				case errors.Is(err, domain.ErrNotFound):
					clearCookie(w, SessionCookie)
				default:
					slog.Error("resolve session", "err", err, "id", c.RequestID)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// SetupGuard redirects every route to /setup while no user exists, and away from
// /setup once an owner exists (functional/01 §2). Applied to public routes.
func SetupGuard(d Deps) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			zero, err := d.ZeroUsers(r.Context())
			if err != nil {
				slog.Error("zero-users check", "err", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			switch {
			case zero && r.URL.Path != "/setup":
				http.Redirect(w, r, "/setup", http.StatusSeeOther)
				return
			case !zero && r.URL.Path == "/setup":
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// AuthGuard requires an authenticated context; otherwise it redirects (or sends
// HX-Redirect for htmx) to /login (technical/04 §1.1/§2).
func AuthGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if From(r.Context()).User == nil {
			if r.Header.Get("HX-Request") == "true" {
				w.Header().Set("HX-Redirect", "/login")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// AdminGuard requires the authenticated user to be an admin; a non-admin gets a
// 404 (never 403 — the isolation invariant: never disclose that the route
// exists). Applied to /admin/* after TenantContext has set Ctx.IsAdmin.
func AdminGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !From(r.Context()).IsAdmin {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ForcePasswordChange redirects a user flagged must_change_password to the forced
// change-password page until it is cleared (technical/05 §8). The change endpoint,
// logout, and assets are exempt so the user can complete the change or leave.
func ForcePasswordChange(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := From(r.Context())
		if c.User != nil && c.User.MustChangePassword && !passwordChangeExempt(r.URL.Path) {
			if r.Header.Get("HX-Request") == "true" {
				w.Header().Set("HX-Redirect", "/password")
				w.WriteHeader(http.StatusOK)
				return
			}
			http.Redirect(w, r, "/password", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func passwordChangeExempt(path string) bool {
	switch path {
	case "/password", "/security/password", "/logout":
		return true
	}
	return strings.HasPrefix(path, "/assets/")
}

// TenantContext injects the locale/currency/admin flag from the authenticated
// user + settings. Downstream code reads user_id only from here.
func TenantContext(d Deps) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := From(r.Context())
			if c.User != nil {
				c.Lang = c.User.Language
				c.Currency = c.User.Currency
				c.IsAdmin = c.User.IsAdmin
				if st, err := d.Settings(r.Context(), c.User.ID); err == nil {
					c.Lang = st.Language
					c.Currency = st.Currency
					c.Theme = st.Theme
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Locale resolves the signed-out locale from Accept-Language (default FR);
// authenticated requests already have their locale from TenantContext.
func Locale(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := From(r.Context())
		if c.User == nil {
			c.Lang = parseAcceptLanguage(r.Header.Get("Accept-Language"))
		}
		next.ServeHTTP(w, r)
	})
}

// CSRF implements the signed double-submit token (I-010): a per-browser seed
// cookie + a token = HMAC(secret, seed). It ensures the seed exists, exposes the
// token via Ctx, and rejects mutating requests with a bad token (403).
func CSRF(secret []byte, behindTLS bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := From(r.Context())
			seed := ""
			if cookie, err := r.Cookie(csrfCookie); err == nil {
				seed = cookie.Value
			}
			if seed == "" {
				s, err := auth.GenerateCSRFSeed()
				if err != nil {
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
					return
				}
				seed = s
				setCookie(w, csrfCookie, seed, behindTLS, 0)
			}
			c.CSRFToken = auth.CSRFToken(secret, seed)

			if isMutating(r.Method) {
				token := r.PostFormValue("_csrf")
				if token == "" {
					token = r.Header.Get("X-CSRF-Token")
				}
				if !auth.ValidCSRF(secret, seed, token) {
					http.Error(w, "Forbidden", http.StatusForbidden)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func isMutating(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func parseAcceptLanguage(header string) domain.Language {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(header)), "en") {
		return domain.LangEN
	}
	return domain.LangFR
}

func randomID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "req"
	}
	return hex.EncodeToString(b)
}
