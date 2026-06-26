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

// --- Envelopes ---

// EnvelopeRowVM is one envelope row in the configuration list.
type EnvelopeRowVM struct {
	ID          int64
	Name        string
	AccountName string
	BadgeClass  string // mb fixe / var / res / rev / xfer
	BadgeLabel  string
	FreqLabel   string
	DefaultStr  string // amount, "auto" (residual), or "—"
	DayStr      string
	Archived    bool
}

// ParentGroupVM is a parent category with its children (read-only sum).
type ParentGroupVM struct {
	Key        string // data-k for the expand chevron
	Name       string
	ChildCount int
	SumStr     string
	Expanded   bool // seeds the open/closed state (category.default_expanded, M4)
	Children   []EnvelopeRowVM
}

// EnvelopesView backs GET /config/envelopes.
type EnvelopesView struct {
	Base
	Email       string
	Nav         string
	Parents     []ParentGroupVM
	TopLevel    []EnvelopeRowVM
	HasArchived bool
	IsEmpty     bool
}

// EnvelopeFormView backs the create/edit envelope modal.
type EnvelopeFormView struct {
	Base
	IsEdit          bool
	ID              int64
	Name            string
	FlowType        string
	Mode            string
	AccountID       int64
	DestAccountID   int64 // transfer destination (T11); 0 = none
	DefaultStr      string
	Frequency       string
	DueMonth        string // single month value, "" if none
	ExpectedDayStr  string
	ParentID        int64 // 0 = none
	DefaultExpanded bool
	IsResidual      bool
	IsFixed         bool
	IsTransfer      bool // flow_type == transfer → reveal the destination select
	NonMonthly      bool
	AccountOptions  []SelectOption
	DestOptions     []SelectOption // current accounts the transfer may target
	ParentOptions   []SelectOption
	FlowOptions     []SelectOption
	ModeOptions     []SelectOption
	FreqOptions     []SelectOption
	MonthOptions    []SelectOption
	FieldErrors     map[string]string
	FormError       string
}

// FieldError returns the localised error for a field, or "".
func (v EnvelopeFormView) FieldError(field string) string { return v.FieldErrors[field] }
