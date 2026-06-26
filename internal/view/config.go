package view

// View-models for the Configuration screens (Parameters, Envelopes). The
// handler precomputes every display string (labels via the catalog, amounts via
// the formatters) so the templates stay logic-free (G5).

// SelectOption is one choice in a custom select / native option list.
type SelectOption struct {
	Value string
	Label string
}

// AccountRow is one row of the Comptes table.
type AccountRow struct {
	ID         int64
	Name       string
	TypeLabel  string
	IsCurrent  bool
	PolicyCode string // sweep / carry / none
	ChipClass  string // fin-sweep / fin-carry ("" when not current)
	PolicyText string // chip label, or "—"
	CeilingStr string // formatted ceiling, or "—"
	Archived   bool
}

// CascadeRow is one savings vehicle in the fill-priority cascade.
type CascadeRow struct {
	ID    int64
	Name  string
	Order int
}

// SettingsVM holds the prefilled values of the Épargne / Localisation /
// Préférences cards.
type SettingsVM struct {
	DefaultAccountID int64
	PEAInitialStr    string
	PEARateStr       string
	NearCapStr       string
	Basis            string // all_planned / fixed_only
	Comment          bool
	Language         string
	LanguageLabel    string
	Currency         string
	CurrencyLabel    string
	ThemeDark        bool
}

// ParametersView backs GET /config/parameters.
type ParametersView struct {
	Base
	Email           string
	Nav             string
	IsAdmin         bool
	Accounts        []AccountRow
	ArchivedCount   int
	Settings        SettingsVM
	Cascade         []CascadeRow
	CurrentOptions  []SelectOption // default-account (salary) choices
	CascadeAddable  []SelectOption // savings accounts not yet in the cascade
	LangOptions     []SelectOption
	CurrencyOptions []SelectOption
	FieldErrors     map[string]string // settings field errors (after a 422)
}

// FieldError returns the localised settings error for a field, or "".
func (v ParametersView) FieldError(field string) string { return v.FieldErrors[field] }

// AccountFormView backs the create/edit account modal fragment.
type AccountFormView struct {
	Base
	IsEdit      bool
	ID          int64
	Name        string
	Type        string
	TypeLabel   string
	Policy      string
	PolicyLabel string
	CeilingStr  string
	IsCurrent   bool
	IsSavings   bool
	TypeOptions []SelectOption
	FieldErrors map[string]string
	FormError   string
}

// FieldError returns the localised error for a field, or "".
func (v AccountFormView) FieldError(field string) string { return v.FieldErrors[field] }
