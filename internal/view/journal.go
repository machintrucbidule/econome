package view

import "html/template"

// View-models for the Journal (functional/06). The handler precomputes every
// display string; the template stays logic-free (G5).

// JOption is one custom-select option (category / account / status), serialised
// to JSON for the CSP-clean econome.js widgets.
type JOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Icon  string `json:"ic,omitempty"`
	Flow  string `json:"flow,omitempty"` // category options: expense|income|transfer
	Acct  string `json:"acct,omitempty"` // category options: the prefill account id
}

// JRow is one rendered journal row.
type JRow struct {
	ID              int64
	CategoryID      int64  // current value for the inline category widget
	AccountID       int64  // current value for the inline account widget
	Period          string // page period (for the hx urls)
	Scope           string
	DateStr         string
	DateApprox      bool
	BudgetPeriod    string
	PeriodLabel     string
	PeriodHighlight bool
	Label           string
	CategoryName    string
	AccountName     string
	AccountDisplay  string // "Source → Dest" for a transfer, else AccountName
	DestName        string
	DelTitle        string
	IsTransfer      bool
	AmountStr       string
	AmountPos       bool
	AmountMuted     bool // transfers
	Status          string
	StatusLabel     string
	StatusClass     string // ok | warn
	StatusIcon      string
	Editable        bool
}

// JSummary is the right-panel month summary (M18).
type JSummary struct {
	IncomeStr    string
	RealStr      string
	PendingStr   string
	PendingCount int
	AwaitedStr   string
	AwaitedCount int
	NetStr       string
	NetPos       bool
}

// JFilterStatus is one status filter chip's on/off state.
type JFilterStatus struct {
	Code  string
	Label string
	Icon  string
	On    bool
}

// JournalView backs GET /journal.
type JournalView struct {
	Base
	Email      string
	Nav        string
	Period     string
	MonthLabel string
	PrevPeriod string
	NextPeriod string
	YearLabel  string
	MonthIndex int

	PickerOpen     bool
	PrevYearPeriod string
	NextYearPeriod string
	MonthCells     []MonthCell // reused from the forecast view

	Scope  string
	Scopes []FScope // reused from the forecast view
	Sort   string
	Dir    string

	NotCreated bool
	Locked     bool
	Empty      bool
	Editable   bool
	OOB        bool // panel/rows rendered out-of-band (htmx response)

	Rows    []JRow
	Summary JSummary

	// quick-entry + selector data (JSON consumed by app.js, CSP-safe)
	CatsJSON   template.JS
	AcctsJSON  template.JS
	StatusJSON template.JS
	TodayDDMM  string

	// filter state (reflected back into the controls)
	FQ         string
	FCategory  string
	FTransfers bool
	FStatuses  []JFilterStatus
}
