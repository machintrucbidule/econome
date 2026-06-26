package view

// View-models for the month-initialisation assistant (functional/09). The handler
// precomputes every display string; the template stays logic-free (G5). The draft
// is non-persisted (T3i) — these are a pure projection of the engine's output over
// the current configuration plus the user's amount overrides (I-025).

// MIScope is one rail scope entry (aggregated, or a single current account, M26).
type MIScope struct {
	Key   string // "all" or the account id as a string
	Name  string
	Note  string // policy note ("sweep → épargne" / "report" / "agrégé")
	On    bool
	IsAll bool
}

// MIStartCard is a per-current-account starting-balance card (C5).
type MIStartCard struct {
	AccountID int64
	Name      string
	ValueStr  string
	Note      string
}

// MIPost is one editable leaf line of the draft (Poste | Compte | Montant | Génération).
type MIPost struct {
	EnvelopeID  int64
	Name        string
	AccountID   int64
	AccountName string
	AmountStr   string // editable magnitude, symbol-less (i18n.Amount)
	GenClass    string // gen-prevu | gen-alloc
	GenLabel    string // "Prévu · 27" or "Allocation"
	IsTransfer  bool
}

// MIEncart is the residual savings band (and its variants) for one sweep/carry
// account (rules §7/§9/§11.1).
type MIEncart struct {
	Kind        string // residual | negative | cascade | carry
	AccountName string
	TargetName  string
	AmountStr   string
}

// MonthInitView backs GET /month-init.
type MonthInitView struct {
	Base
	Email      string
	Nav        string
	Period     string
	MonthLabel string
	Scope      string
	Scopes     []MIScope
	StartCards []MIStartCard
	Posts      []MIPost
	Figures    MIFigures
	Empty      bool
}

// MIFigures is the recomputable block (residual encarts + footer total) swapped
// on every draft adjustment (PATCH /month-init/draft).
type MIFigures struct {
	Encarts    []MIEncart
	TotalLabel string
	TotalStr   string
}
