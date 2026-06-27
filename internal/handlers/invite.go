package handlers

import (
	"errors"
	"net/http"

	"econome/internal/domain"
	"econome/internal/services"
	"econome/internal/view"
)

// Invitation acceptance (functional/01 §4.2). Public routes (no session yet):
// GET shows the account-creation form for a valid token or the "no longer valid"
// state; POST creates the invited user, consumes the token, and opens a session.

// InviteGet renders the acceptance form or the invalid-token state.
func (h *Handlers) InviteGet(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	base := h.base(r)
	inv, err := h.svc.CheckInvitation(r.Context(), token)
	if err != nil {
		h.render(w, http.StatusOK, "accept", view.AcceptView{Base: base, Title: "accept", Invalid: true})
		return
	}
	v := view.AcceptView{Base: base, Title: "accept", Token: token, LangOptions: langOptions(base)}
	if inv.Email != nil {
		v.Email = *inv.Email
	}
	h.render(w, http.StatusOK, "accept", v)
}

// InvitePost creates the invited user and opens a session.
func (h *Handlers) InvitePost(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	base := h.base(r)
	in := services.AcceptInput{
		Email:           r.PostFormValue("email"),
		Password:        r.PostFormValue("password"),
		PasswordConfirm: r.PostFormValue("password_confirm"),
		Language:        r.PostFormValue("language"),
		Currency:        r.PostFormValue("currency"),
	}
	res, err := h.svc.AcceptInvitation(r.Context(), token, in)
	if err != nil {
		var ve *domain.ValidationError
		switch {
		case errors.As(err, &ve):
			v := view.AcceptView{Base: base, Title: "accept", Token: token, Email: in.Email, LangOptions: langOptions(base), FieldErrors: h.localizeFields(r, ve)}
			h.render(w, http.StatusUnprocessableEntity, "accept", v)
		case errors.Is(err, domain.ErrNotFound):
			h.render(w, http.StatusOK, "accept", view.AcceptView{Base: base, Title: "accept", Invalid: true})
		default:
			h.render(w, http.StatusInternalServerError, "accept", view.AcceptView{Base: base, Title: "accept", Token: token, GenericError: h.t(r, "accept.error")})
		}
		return
	}
	h.startSession(w, res)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
