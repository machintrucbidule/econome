package view

import "html/template"

// View-models for the security/admin modals (functional/01 §4–§8). They back the
// htmx fragments swapped into #modal-host on the Parameters screen and the
// invitation-acceptance page.

// SecurityModalView backs the 2FA-enrol / backup-codes / password / disable
// modals.
type SecurityModalView struct {
	Base
	Kind string // "enrol" | "backup" | "password" | "disable"
	// QRDataURI is a self-generated PNG data: URI; typed template.URL so
	// html/template emits it verbatim (a plain string is filtered to #ZgotmplZ
	// because data: URLs are treated as unsafe in a src attribute).
	QRDataURI   template.URL
	Secret      string   // manual-entry secret (enrol)
	BackupCodes []string // shown once (enrol confirm / regenerate)
	FieldErrors map[string]string
	FormError   string
}

// FieldError returns the localised error for a field, or "".
func (v SecurityModalView) FieldError(field string) string { return v.FieldErrors[field] }

// InviteModalView backs the admin "invite a user" modal: the form, then the
// one-time link shown after creation.
type InviteModalView struct {
	Base
	Created     bool   // true ⇒ show the link, not the form
	Link        string // full one-time invitation link (shown once)
	Email       string
	FieldErrors map[string]string
	FormError   string
}

// FieldError returns the localised error for a field, or "".
func (v InviteModalView) FieldError(field string) string { return v.FieldErrors[field] }

// UserManageView backs the admin per-user management modal.
type UserManageView struct {
	Base
	ID          int64
	Email       string
	IsAdmin     bool
	Deactivated bool
	TOTPEnabled bool
	IsSelf      bool
	TempPass    string // shown once after a password reset
	Notice      string // localised result notice
	FormError   string
}

// AcceptView backs the invitation-acceptance page (functional/01 §4.2).
type AcceptView struct {
	Base
	Title        string
	Token        string
	Email        string
	Invalid      bool // expired/used/revoked → the "no longer valid" state
	FieldErrors  map[string]string
	GenericError string
	LangOptions  []SelectOption
}

// FieldError returns the localised error for a field, or "".
func (v AcceptView) FieldError(field string) string { return v.FieldErrors[field] }
