package domain

import "time"

// Budget-domain enums (English codes; FR/EN labels live in internal/i18n) and
// the value structs the pure engine consumes (technical/03 §3, technical/09 §2).
// These are value types only; their DB tables land in increment 3. Money fields
// are integer minor units.

// FlowType is a category/transaction flow (technical/03 §3.2/§3.5).
type FlowType string

// Flow types.
const (
	FlowExpense  FlowType = "expense"
	FlowIncome   FlowType = "income"
	FlowTransfer FlowType = "transfer"
)

// Valid reports whether f is a known flow type.
func (f FlowType) Valid() bool {
	switch f {
	case FlowExpense, FlowIncome, FlowTransfer:
		return true
	default:
		return false
	}
}

// AccountType is the account kind (technical/03 §3.1, M13).
type AccountType string

// Account types.
const (
	AccountCurrent         AccountType = "current"
	AccountPassbook        AccountType = "passbook"
	AccountSecurities      AccountType = "securities"
	AccountEmployeeSavings AccountType = "employee_savings"
)

// Valid reports whether t is a known account type.
func (t AccountType) Valid() bool {
	switch t {
	case AccountCurrent, AccountPassbook, AccountSecurities, AccountEmployeeSavings:
		return true
	default:
		return false
	}
}

// MonthEndPolicy is the end-of-month behaviour (current ⇒ sweep/carry; savings ⇒
// none) (technical/03 §3.1).
type MonthEndPolicy string

// Month-end policies.
const (
	PolicySweep MonthEndPolicy = "sweep"
	PolicyCarry MonthEndPolicy = "carry"
	PolicyNone  MonthEndPolicy = "none"
)

// Valid reports whether p is a known policy.
func (p MonthEndPolicy) Valid() bool {
	switch p {
	case PolicySweep, PolicyCarry, PolicyNone:
		return true
	default:
		return false
	}
}

// Mode is the envelope mode (M10) (technical/03 §3.3).
type Mode string

// Envelope modes.
const (
	ModeFixedRecurring Mode = "fixed_recurring"
	ModeVariable       Mode = "variable"
	ModeResidual       Mode = "residual"
)

// Valid reports whether m is a known mode.
func (m Mode) Valid() bool {
	switch m {
	case ModeFixedRecurring, ModeVariable, ModeResidual:
		return true
	default:
		return false
	}
}

// Frequency is a fixed-recurring envelope's frequency (technical/03 §3.3).
type Frequency string

// Frequencies.
const (
	FreqMonthly    Frequency = "monthly"
	FreqQuarterly  Frequency = "quarterly"
	FreqSemiannual Frequency = "semiannual"
	FreqAnnual     Frequency = "annual"
)

// Valid reports whether f is a known frequency.
func (f Frequency) Valid() bool {
	switch f {
	case FreqMonthly, FreqQuarterly, FreqSemiannual, FreqAnnual:
		return true
	default:
		return false
	}
}

// TransactionStatus is the awaited→pending→cleared lifecycle (M10, C7).
type TransactionStatus string

// Transaction statuses.
const (
	StatusAwaited TransactionStatus = "awaited"
	StatusPending TransactionStatus = "pending"
	StatusCleared TransactionStatus = "cleared"
)

// Valid reports whether s is a known status.
func (s TransactionStatus) Valid() bool {
	switch s {
	case StatusAwaited, StatusPending, StatusCleared:
		return true
	default:
		return false
	}
}

// IsReal reports whether the status counts towards `real` (cleared + pending, C7).
func (s TransactionStatus) IsReal() bool {
	return s == StatusCleared || s == StatusPending
}

// TxnSource distinguishes manual entry from DSP2 import (anticipated).
type TxnSource string

// Transaction sources.
const (
	SourceManual TxnSource = "manual"
	SourceImport TxnSource = "import"
)

// Valid reports whether s is a known source.
func (s TxnSource) Valid() bool {
	switch s {
	case SourceManual, SourceImport:
		return true
	default:
		return false
	}
}

// ArchiveStatus is the soft-archive lifecycle for account/category/envelope
// (active/archived, L4/L10) — distinct from the user Status (active/deactivated).
type ArchiveStatus string

// Archive statuses.
const (
	ArchiveActive   ArchiveStatus = "active"
	ArchiveArchived ArchiveStatus = "archived"
)

// Valid reports whether s is a known archive status.
func (s ArchiveStatus) Valid() bool {
	switch s {
	case ArchiveActive, ArchiveArchived:
		return true
	default:
		return false
	}
}

// EnvelopeState is the derived five-state status of an expense envelope
// (functional/03 §3, C2/C8). Income/transfer envelopes do not use it. The view
// localises these codes (state.none/expected/partial/paid/overrun).
type EnvelopeState string

// Envelope states (expenses only).
const (
	StateNone     EnvelopeState = "none"
	StateExpected EnvelopeState = "expected"
	StatePartial  EnvelopeState = "partial"
	StatePaid     EnvelopeState = "paid"
	StateOverrun  EnvelopeState = "overrun"
)

// Valid reports whether s is a known envelope state.
func (s EnvelopeState) Valid() bool {
	switch s {
	case StateNone, StateExpected, StatePartial, StatePaid, StateOverrun:
		return true
	default:
		return false
	}
}

// Account is a money account (technical/03 §3.1). Persistence fields (UserID,
// timestamps, ExternalRef) are ignored by the engine.
type Account struct {
	ID             int64
	UserID         int64
	Name           string
	Type           AccountType
	MonthEndPolicy MonthEndPolicy
	FillPriority   *int   // cascade order (savings only); nil if not in the cascade
	Ceiling        *int64 // minor units; regulatory/chosen cap; nil = no cap
	Status         ArchiveStatus
	ExternalRef    *string // DSP2 (anticipated)
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// IsSavings is derived (is_savings ⇔ type != current), never stored
// (technical/03 §3.1, foundation §4.2).
func (a Account) IsSavings() bool { return a.Type != AccountCurrent }

// Category is a budget category; budget is posted at the leaf (technical/03 §3.2).
type Category struct {
	ID              int64
	UserID          int64
	Name            string
	ParentID        *int64
	FlowType        FlowType
	DefaultExpanded bool
	Status          ArchiveStatus
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Envelope is a (category, account) budget line with a mode (technical/03 §3.3).
type Envelope struct {
	ID            int64
	UserID        int64
	CategoryID    int64
	AccountID     int64
	Mode          Mode
	DefaultAmount *int64
	Frequency     *Frequency
	DueMonths     []int // parsed from the stored CSV (1–12)
	ExpectedDay   *int
	Status        ArchiveStatus
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Allocation is the per-period planned amount for an envelope — the only budget
// input besides transactions (technical/03 §3.4, L2 non-retroactive).
type Allocation struct {
	ID            int64
	UserID        int64
	EnvelopeID    int64
	Period        string // "YYYY-MM"
	PlannedAmount int64  // minor units, >= 0
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Transaction is a money movement (technical/03 §3.5). Amount is signed minor
// units; a transfer is the canonical single two-legged row (dest_account_id set,
// category_id nil).
type Transaction struct {
	ID                  int64
	UserID              int64
	AccountID           int64
	DestAccountID       *int64
	CategoryID          *int64
	FlowType            FlowType
	Amount              int64
	OpDate              *Date // nil while awaited
	BudgetPeriod        string
	Status              TransactionStatus
	Label               string
	Note                *string
	Source              TxnSource
	ExternalRef         *string
	PairedTransactionID *int64
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// Snapshot is a monthly gross value for a savings account (technical/03 §4.3).
type Snapshot struct {
	ID         int64
	UserID     int64
	AccountID  int64
	Period     string // "YYYY-MM"
	GrossValue int64  // minor units, >= 0
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
