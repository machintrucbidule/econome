// Package handlers is the HTTP transport layer: one handler per route. Each
// parses the request, calls exactly one service use-case, and renders a template
// or fragment. No business logic, no derived-figure computation.
package handlers

import (
	"errors"
	"log/slog"
	"net"
	"net/http"

	"econome/internal/domain"
	"econome/internal/server/middleware"
	"econome/internal/services"
	"econome/internal/view"
)

// demoBalanceMinor is the placeholder figure rendered in the empty shell to
// prove the engine→view→i18n path (−635,00 €). Real figures land in increment 6.
const demoBalanceMinor int64 = -63500

// Handlers holds the transport dependencies.
type Handlers struct {
	svc          *services.Service
	rdr          *view.Renderer
	behindTLS    bool
	trustedProxy string
}

// New builds the handler set.
func New(svc *services.Service, rdr *view.Renderer, behindTLS bool, trustedProxy string) *Handlers {
	return &Handlers{svc: svc, rdr: rdr, behindTLS: behindTLS, trustedProxy: trustedProxy}
}

func (h *Handlers) base(r *http.Request) view.Base {
	c := middleware.From(r.Context())
	return h.rdr.NewBase(c.Lang, c.Currency, c.Theme, c.CSRFToken)
}

func (h *Handlers) render(w http.ResponseWriter, status int, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := h.rdr.Render(w, name, data); err != nil {
		slog.Error("render", "template", name, "err", err)
	}
}

// Healthz is the liveness probe (DB ping).
func (h *Handlers) Healthz(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Health(r.Context()); err != nil {
		http.Error(w, "unhealthy", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok"))
}

// SetupGet renders the owner-creation wizard.
func (h *Handlers) SetupGet(w http.ResponseWriter, r *http.Request) {
	h.render(w, http.StatusOK, "setup", view.AuthView{Base: h.base(r), Title: "setup"})
}

// SetupPost creates the owner, opens a session, and lands on the shell.
func (h *Handlers) SetupPost(w http.ResponseWriter, r *http.Request) {
	in := services.SetupInput{
		Email:           r.PostFormValue("email"),
		Password:        r.PostFormValue("password"),
		PasswordConfirm: r.PostFormValue("password_confirm"),
		Language:        r.PostFormValue("language"),
		Currency:        r.PostFormValue("currency"),
	}
	res, err := h.svc.Setup(r.Context(), in)
	if err != nil {
		var ve *domain.ValidationError
		switch {
		case errors.As(err, &ve):
			h.render(w, http.StatusUnprocessableEntity, "setup", h.authViewWithErrors(r, in.Email, false, ve))
		case errors.Is(err, domain.ErrConflict), errors.Is(err, domain.ErrDuplicate):
			http.Redirect(w, r, "/login", http.StatusSeeOther)
		default:
			slog.Error("setup", "err", err)
			h.render(w, http.StatusInternalServerError, "setup", h.authViewError(r, in.Email, "setup.error"))
		}
		return
	}
	h.startSession(w, res)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// LoginGet renders the login form.
func (h *Handlers) LoginGet(w http.ResponseWriter, r *http.Request) {
	h.render(w, http.StatusOK, "login", view.AuthView{Base: h.base(r), Title: "login"})
}

// LoginPost verifies credentials and opens a session.
func (h *Handlers) LoginPost(w http.ResponseWriter, r *http.Request) {
	in := services.LoginInput{
		Email:     r.PostFormValue("email"),
		Password:  r.PostFormValue("password"),
		Remember:  r.PostFormValue("remember") == "1",
		IP:        h.clientIP(r),
		UserAgent: r.UserAgent(),
	}
	res, err := h.svc.Login(r.Context(), in)
	if err != nil {
		var locked *services.LockedError
		switch {
		case errors.As(err, &locked):
			v := view.AuthView{Base: h.base(r), Title: "login", Email: in.Email, Remember: in.Remember, LockedSeconds: locked.RetrySeconds()}
			h.render(w, http.StatusTooManyRequests, "login", v)
		case errors.Is(err, services.ErrInvalidCredentials):
			v := view.AuthView{Base: h.base(r), Title: "login", Email: in.Email, Remember: in.Remember, GenericError: h.rdr.Catalog().T(middleware.From(r.Context()).Lang, "login.error_generic")}
			h.render(w, http.StatusUnauthorized, "login", v)
		default:
			slog.Error("login", "err", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return
	}
	h.startSession(w, res)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// LoginTOTP is the stubbed 2FA step (built in increment 8).
func (h *Handlers) LoginTOTP(w http.ResponseWriter, r *http.Request) {
	lang := middleware.From(r.Context()).Lang
	v := view.AuthView{Base: h.base(r), Title: "login", GenericError: h.rdr.Catalog().T(lang, "login.totp_not_enabled")}
	h.render(w, http.StatusNotImplemented, "login", v)
}

// Logout revokes the current session.
func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(middleware.SessionCookie); err == nil && cookie.Value != "" {
		if err := h.svc.Logout(r.Context(), cookie.Value); err != nil {
			slog.Error("logout", "err", err)
		}
	}
	middleware.ClearSessionCookie(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// Home renders the empty three-pane shell.
func (h *Handlers) Home(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	v := view.ShellView{Base: h.base(r), Email: c.User.Email, DemoBalanceMinor: demoBalanceMinor}
	h.render(w, http.StatusOK, "home", v)
}

func (h *Handlers) startSession(w http.ResponseWriter, res *services.AuthResult) {
	maxAge := 0 // short session: a session cookie (cleared on browser close)
	if res.Kind == domain.SessionRemember {
		maxAge = int(rememberCookieSeconds)
	}
	middleware.SetSessionCookie(w, res.Token, h.behindTLS, maxAge)
}

func (h *Handlers) authViewWithErrors(r *http.Request, email string, remember bool, ve *domain.ValidationError) view.AuthView {
	c := middleware.From(r.Context())
	fe := map[string]string{}
	for _, f := range ve.Fields {
		fe[f.Field] = h.rdr.Catalog().T(c.Lang, f.MsgKey)
	}
	return view.AuthView{Base: h.base(r), Title: "setup", Email: email, Remember: remember, FieldErrors: fe}
}

func (h *Handlers) authViewError(r *http.Request, email, msgKey string) view.AuthView {
	c := middleware.From(r.Context())
	return view.AuthView{Base: h.base(r), Title: "setup", Email: email, GenericError: h.rdr.Catalog().T(c.Lang, msgKey)}
}

const rememberCookieSeconds = 365 * 24 * 60 * 60

// clientIP resolves the client IP, trusting X-Forwarded-For only when a trusted
// proxy is configured (technical/05 §6, 07 §2).
func (h *Handlers) clientIP(r *http.Request) string {
	if h.trustedProxy != "" {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := splitFirst(xff)
			if parts != "" {
				return parts
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func splitFirst(xff string) string {
	for i := 0; i < len(xff); i++ {
		if xff[i] == ',' {
			return trimSpace(xff[:i])
		}
	}
	return trimSpace(xff)
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && s[start] == ' ' {
		start++
	}
	for end > start && s[end-1] == ' ' {
		end--
	}
	return s[start:end]
}
