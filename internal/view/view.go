// Package view builds the view-models templates render and owns template
// parsing/execution. It turns the engine's minor-unit integers and English codes
// into localised strings via internal/i18n; templates contain no business logic
// and do no formatting beyond calling these helpers (G5). The engine never
// formats.
package view

import (
	"fmt"
	"html/template"
	"io"
	"strconv"

	"econome/internal/domain"
	"econome/internal/i18n"
	"econome/web/templates"
)

// funcs are the central, pure template functions (G5).
var funcs = template.FuncMap{
	"itoa": strconv.Itoa,
}

// Renderer parses the embedded templates once and renders a named page.
type Renderer struct {
	tmpl    *template.Template
	catalog *i18n.Catalog
}

// New parses every embedded template.
func New(catalog *i18n.Catalog) (*Renderer, error) {
	t, err := template.New("econome").Funcs(funcs).ParseFS(templates.FS, "*.html")
	if err != nil {
		return nil, fmt.Errorf("view: parse templates: %w", err)
	}
	return &Renderer{tmpl: t, catalog: catalog}, nil
}

// Render writes the named template to w with the given data.
func (r *Renderer) Render(w io.Writer, name string, data any) error {
	if err := r.tmpl.ExecuteTemplate(w, name, data); err != nil {
		return fmt.Errorf("view: render %q: %w", name, err)
	}
	return nil
}

// Catalog exposes the message catalog (used to build view-models).
func (r *Renderer) Catalog() *i18n.Catalog { return r.catalog }

// Base carries the per-request context every page needs. Page view-models embed
// it, so templates can call {{.T "key"}}, {{.Money 12345}}, {{.CSRF}}.
type Base struct {
	catalog   *i18n.Catalog
	Lang      domain.Language
	Currency  string
	Theme     domain.Theme
	CSRFToken string
}

// NewBase builds a Base for a request.
func (r *Renderer) NewBase(lang domain.Language, currency string, theme domain.Theme, csrf string) Base {
	return Base{catalog: r.catalog, Lang: lang, Currency: currency, Theme: theme, CSRFToken: csrf}
}

// T localises a message key with optional positional arguments.
func (b Base) T(key string, args ...string) string { return b.catalog.T(b.Lang, key, args...) }

// Money formats an amount in minor units for the active locale + currency.
func (b Base) Money(minor int64) string { return i18n.FormatMoney(minor, b.Lang, b.Currency) }

// Amount formats minor units as a localised number without a currency symbol —
// for editable form inputs (the inverse of i18n.ParseMoney).
func (b Base) Amount(minor int64) string { return i18n.FormatAmount(minor, b.Lang) }

// Rate formats basis points as a localised percentage number without the percent
// sign — for the rate input fields (the inverse of i18n.ParsePercent).
func (b Base) Rate(bp int) string { return i18n.FormatRate(bp, b.Lang) }

// CSRF returns the per-request CSRF token (for the hidden form field / hx-headers).
func (b Base) CSRF() string { return b.CSRFToken }

// LangCode returns the raw language code for the html lang attribute.
func (b Base) LangCode() string { return string(b.Lang) }

// ThemeCode returns the raw theme code for the data-theme attribute.
func (b Base) ThemeCode() string { return string(b.Theme) }

// --- page view-models ---

// AuthView backs the signed-out setup/login pages.
type AuthView struct {
	Base
	Title         string
	GenericError  string            // non-field banner (e.g. login failure)
	LockedSeconds int               // >0 shows the lockout countdown
	FieldErrors   map[string]string // field -> localised message
	Email         string            // sticky value on re-render
	Remember      bool
	TOTPStep      bool   // login: show the 2FA code step instead of the credentials form
	Pending       string // signed pending-2FA token carried into the TOTP step
}

// FieldError returns the localised error for a field, or "".
func (v AuthView) FieldError(field string) string { return v.FieldErrors[field] }
