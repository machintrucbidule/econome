package handlers

import (
	"encoding/base64"
	"errors"
	"html/template"
	"net/http"
	"strconv"

	qrcode "github.com/skip2/go-qrcode"

	"econome/internal/domain"
	"econome/internal/server/middleware"
	"econome/internal/view"
)

// Self-service security handlers (functional/01 §5–§7): TOTP enrol/confirm/
// disable/regenerate, password + email change, active-session management. Each
// renders a modal fragment into #modal-host; a successful mutation triggers a
// full-page refresh (HX-Refresh) so the Parameters panels re-read live state.

// hxRefresh reloads the page so the panels re-read live state: an HX-Refresh for
// htmx requests, a 303 to the landing for a plain form post.
func hxRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Refresh", "true")
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// TOTPEnrolGet begins enrolment: it generates a secret and renders the QR + a
// confirmation-code form.
func (h *Handlers) TOTPEnrolGet(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	enr, err := h.svc.BeginTOTPEnrolment(r.Context(), c.User.ID)
	if err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	png, err := qrcode.Encode(enr.OTPAuthURL, qrcode.Medium, 256)
	if err != nil {
		ev := h.secModal(r)
		ev.Kind = "enrol"
		ev.FormError = h.t(r, "security.enrol.error")
		h.render(w, http.StatusInternalServerError, "security-modal", ev)
		return
	}
	v := h.secModal(r)
	v.Kind = "enrol"
	v.Secret = enr.Secret
	v.QRDataURI = qrDataURI(png)
	h.render(w, http.StatusOK, "security-modal", v)
}

// qrDataURI wraps a PNG as a template.URL data: URI (app-generated, so safe to
// emit verbatim past html/template's URL filter).
func qrDataURI(png []byte) template.URL {
	// The bytes are a self-generated QR PNG (never user input); base64 is
	// URL-attribute-safe, so emitting it verbatim is intentional.
	return template.URL("data:image/png;base64," + base64.StdEncoding.EncodeToString(png)) //nolint:gosec // app-generated PNG, not attacker-controlled
}

// TOTPConfirm verifies the enrolment code, enables 2FA, and shows the one-time
// backup codes.
func (h *Handlers) TOTPConfirm(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	_ = r.ParseForm()
	codes, err := h.svc.ConfirmTOTP(r.Context(), c.User.ID, r.PostFormValue("code"))
	if err != nil {
		var ve *domain.ValidationError
		if errors.As(err, &ve) {
			v := h.secModal(r)
			v.Kind = "enrol"
			v.FieldErrors = h.localizeFields(r, ve)
			// Re-render the SAME (unrotated) secret so the user's scanned QR stays
			// valid after a wrong code.
			if enr, e := h.svc.CurrentTOTPEnrolment(r.Context(), c.User.ID); e == nil {
				if png, e2 := qrcode.Encode(enr.OTPAuthURL, qrcode.Medium, 256); e2 == nil {
					v.Secret = enr.Secret
					v.QRDataURI = qrDataURI(png)
				}
			}
			h.render(w, http.StatusUnprocessableEntity, "security-modal", v)
			return
		}
		h.mutationError(w, r, err, nil)
		return
	}
	v := h.secModal(r)
	v.Kind = "backup"
	v.BackupCodes = codes
	h.render(w, http.StatusOK, "security-modal", v)
}

// TOTPDisableGet renders the disable-2FA modal (password + optional code).
func (h *Handlers) TOTPDisableGet(w http.ResponseWriter, r *http.Request) {
	v := h.secModal(r)
	v.Kind = "disable"
	h.render(w, http.StatusOK, "security-modal", v)
}

// TOTPDisablePost disables 2FA after re-auth.
func (h *Handlers) TOTPDisablePost(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	_ = r.ParseForm()
	err := h.svc.DisableTOTP(r.Context(), c.User.ID, r.PostFormValue("password"), r.PostFormValue("code"))
	if err != nil {
		var ve *domain.ValidationError
		if errors.As(err, &ve) {
			v := h.secModal(r)
			v.Kind = "disable"
			v.FieldErrors = h.localizeFields(r, ve)
			h.render(w, http.StatusUnprocessableEntity, "security-modal", v)
			return
		}
		h.mutationError(w, r, err, nil)
		return
	}
	hxRefresh(w, r)
}

// BackupRegenerate issues a fresh set of backup codes (shown once).
func (h *Handlers) BackupRegenerate(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	codes, err := h.svc.RegenerateBackupCodes(r.Context(), c.User.ID)
	if err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	v := h.secModal(r)
	v.Kind = "backup"
	v.BackupCodes = codes
	h.render(w, http.StatusOK, "security-modal", v)
}

// PasswordChangeGet renders the change-password modal.
func (h *Handlers) PasswordChangeGet(w http.ResponseWriter, r *http.Request) {
	v := h.secModal(r)
	v.Kind = "password"
	h.render(w, http.StatusOK, "security-modal", v)
}

// PasswordChangePost changes the user's password.
func (h *Handlers) PasswordChangePost(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	_ = r.ParseForm()
	forced := r.PostFormValue("mode") == "force"
	err := h.svc.ChangePassword(r.Context(), c.User.ID,
		r.PostFormValue("current_password"), r.PostFormValue("password"), r.PostFormValue("password_confirm"))
	if err != nil {
		var ve *domain.ValidationError
		if errors.As(err, &ve) {
			v := h.secModal(r)
			v.FieldErrors = h.localizeFields(r, ve)
			if forced {
				v.Kind = "force"
				h.render(w, http.StatusUnprocessableEntity, "force-password", v)
				return
			}
			v.Kind = "password"
			h.render(w, http.StatusUnprocessableEntity, "security-modal", v)
			return
		}
		h.mutationError(w, r, err, nil)
		return
	}
	hxRefresh(w, r)
}

// EmailChangePost changes the user's email after password re-auth.
func (h *Handlers) EmailChangePost(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	_ = r.ParseForm()
	err := h.svc.ChangeEmail(r.Context(), c.User.ID, r.PostFormValue("password"), r.PostFormValue("email"))
	if err != nil {
		var ve *domain.ValidationError
		if errors.As(err, &ve) {
			v := h.secModal(r)
			v.Kind = "email"
			v.FieldErrors = h.localizeFields(r, ve)
			h.render(w, http.StatusUnprocessableEntity, "security-modal", v)
			return
		}
		h.mutationError(w, r, err, nil)
		return
	}
	hxRefresh(w, r)
}

// EmailChangeGet renders the change-email modal.
func (h *Handlers) EmailChangeGet(w http.ResponseWriter, r *http.Request) {
	v := h.secModal(r)
	v.Kind = "email"
	h.render(w, http.StatusOK, "security-modal", v)
}

// SessionRevoke revokes one of the user's sessions.
func (h *Handlers) SessionRevoke(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := h.svc.RevokeSession(r.Context(), c.User.ID, id); err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	hxRefresh(w, r)
}

// SessionsRevokeAll logs out everywhere except the current session.
func (h *Handlers) SessionsRevokeAll(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	curHash := ""
	if c.Session != nil {
		curHash = c.Session.TokenHash
	}
	if err := h.svc.RevokeOtherSessions(r.Context(), c.User.ID, curHash); err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	hxRefresh(w, r)
}

// ForcedPasswordGet renders the full-page forced change-password form shown to a
// user with must_change_password set (technical/05 §8). It posts to the same
// /security/password endpoint; clearing the flag lets the user back in.
func (h *Handlers) ForcedPasswordGet(w http.ResponseWriter, r *http.Request) {
	v := h.secModal(r)
	v.Kind = "force"
	h.render(w, http.StatusOK, "force-password", v)
}

// secModal builds a fresh SecurityModalView with the request base.
func (h *Handlers) secModal(r *http.Request) view.SecurityModalView {
	return view.SecurityModalView{Base: h.base(r)}
}
