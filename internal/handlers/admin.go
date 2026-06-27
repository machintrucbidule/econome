package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"econome/internal/domain"
	"econome/internal/server/middleware"
	"econome/internal/services"
	"econome/internal/view"
)

// Admin handlers (functional/01 §4/§8): invitations + user management. The routes
// are gated by the AdminGuard middleware (non-admin ⇒ 404). Mutations refresh the
// Parameters page; the one-time invite link is shown in a modal.

// InviteFormGet renders the "invite a user" modal.
func (h *Handlers) InviteFormGet(w http.ResponseWriter, r *http.Request) {
	h.render(w, http.StatusOK, "invite-modal", view.InviteModalView{Base: h.base(r)})
}

// InvitationCreate issues a single-use invitation and shows its one-time link.
func (h *Handlers) InvitationCreate(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	_ = r.ParseForm()
	var email *string
	if e := r.PostFormValue("email"); e != "" {
		email = &e
	}
	invitedAdmin := r.PostFormValue("role") == "admin"
	issued, err := h.svc.IssueInvitation(r.Context(), c.User.ID, email, invitedAdmin)
	if err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	link := h.absoluteURL(r, "/invite/"+issued.RawToken)
	h.render(w, http.StatusOK, "invite-modal", view.InviteModalView{Base: h.base(r), Created: true, Link: link})
}

// InvitationRevoke invalidates a pending invitation.
func (h *Handlers) InvitationRevoke(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := h.svc.RevokeInvitation(r.Context(), c.User.ID, id); err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	hxRefresh(w, r)
}

// UserManageGet renders the per-user management modal (deactivate/reactivate,
// reset 2FA, reset password).
func (h *Handlers) UserManageGet(w http.ResponseWriter, r *http.Request) {
	c := middleware.From(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	users, err := h.svc.ListUsers(r.Context())
	if err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	for _, u := range users {
		if u.ID != id {
			continue
		}
		h.render(w, http.StatusOK, "user-modal", view.UserManageView{
			Base: h.base(r), ID: u.ID, Email: u.Email, IsAdmin: u.IsAdmin,
			Deactivated: u.Status == domain.StatusDeactivated, TOTPEnabled: u.TOTPEnabled,
			IsSelf: u.ID == c.User.ID,
		})
		return
	}
	http.NotFound(w, r)
}

// UserDeactivate / UserReactivate / UserResetTOTP toggle account state; the
// last-admin rule is enforced in the service (ErrLastAdmin → 409).
func (h *Handlers) UserDeactivate(w http.ResponseWriter, r *http.Request) {
	h.userAction(w, r, func(id int64) error { return h.svc.DeactivateUser(r.Context(), id) })
}

// UserReactivate re-enables a deactivated account.
func (h *Handlers) UserReactivate(w http.ResponseWriter, r *http.Request) {
	h.userAction(w, r, func(id int64) error { return h.svc.ReactivateUser(r.Context(), id) })
}

// UserResetTOTP disables a user's 2FA so they can re-enrol (admin recovery).
func (h *Handlers) UserResetTOTP(w http.ResponseWriter, r *http.Request) {
	h.userAction(w, r, func(id int64) error { return h.svc.AdminResetTOTP(r.Context(), id) })
}

// UserResetPassword sets a temporary password and shows it once.
func (h *Handlers) UserResetPassword(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	temp, err := h.svc.GenerateTempPassword()
	if err != nil {
		h.mutationError(w, r, err, nil)
		return
	}
	if err := h.svc.AdminResetPassword(r.Context(), id, temp); err != nil {
		h.adminActionError(w, r, err)
		return
	}
	users, _ := h.svc.ListUsers(r.Context())
	email := ""
	for _, u := range users {
		if u.ID == id {
			email = u.Email
		}
	}
	h.render(w, http.StatusOK, "user-modal", view.UserManageView{
		Base: h.base(r), ID: id, Email: email, TempPass: temp,
		Notice: h.t(r, "users.reset_pw.done"),
	})
}

func (h *Handlers) userAction(w http.ResponseWriter, r *http.Request, fn func(int64) error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := fn(id); err != nil {
		h.adminActionError(w, r, err)
		return
	}
	hxRefresh(w, r)
}

// adminActionError maps the last-admin rule to a 409 with a clear message.
func (h *Handlers) adminActionError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, services.ErrLastAdmin) {
		http.Error(w, h.t(r, "users.err.last_admin"), http.StatusConflict)
		return
	}
	h.mutationError(w, r, err, nil)
}

// absoluteURL builds a full URL for the one-time invitation link, honouring the
// reverse-proxy forwarded host/proto when present (technical/07 §2).
func (h *Handlers) absoluteURL(r *http.Request, path string) string {
	scheme := "http"
	if h.behindTLS {
		scheme = "https"
	}
	if xf := r.Header.Get("X-Forwarded-Proto"); xf != "" {
		scheme = xf
	}
	host := r.Host
	if xf := r.Header.Get("X-Forwarded-Host"); xf != "" {
		host = xf
	}
	return scheme + "://" + host + path
}
