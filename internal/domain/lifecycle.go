package domain

import "time"

// Lifecycle / net-worth / UI / auth-extra value types + enums (technical/03
// §4.1–§4.4, §5.1–§5.2, §2.3–§2.4). Persistence fields included; the engine does
// not consume these.

// PeriodState is the month-lifecycle state (no `draft` — the draft is not
// persisted, L1).
type PeriodState string

// Period states.
const (
	PeriodActive PeriodState = "active"
	PeriodLocked PeriodState = "locked"
)

// Valid reports whether s is a known state.
func (s PeriodState) Valid() bool {
	switch s {
	case PeriodActive, PeriodLocked:
		return true
	default:
		return false
	}
}

// PeriodAction is an audited lifecycle action (L1).
type PeriodAction string

// Period actions.
const (
	ActionCreate PeriodAction = "create"
	ActionLock   PeriodAction = "lock"
	ActionUnlock PeriodAction = "unlock"
)

// Valid reports whether a is a known action.
func (a PeriodAction) Valid() bool {
	switch a {
	case ActionCreate, ActionLock, ActionUnlock:
		return true
	default:
		return false
	}
}

// NodeType identifies a hierarchy node for a UI preference (M4).
type NodeType string

// Node types.
const (
	NodeCategory NodeType = "category"
	NodeEnvelope NodeType = "envelope"
)

// Valid reports whether n is a known node type.
func (n NodeType) Valid() bool {
	switch n {
	case NodeCategory, NodeEnvelope:
		return true
	default:
		return false
	}
}

// Period is a month-lifecycle row; it exists only once "Créer le mois" is
// validated (technical/03 §4.1).
type Period struct {
	ID        int64
	UserID    int64
	Period    string // "YYYY-MM"
	State     PeriodState
	LockedAt  *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// PeriodEvent is an append-only lifecycle audit entry (technical/03 §4.2, L1).
type PeriodEvent struct {
	ID          int64
	UserID      int64
	Period      string
	Action      PeriodAction
	At          time.Time
	ActorUserID int64
}

// NetworthMonth is the single per-month comment shared by Synthèse + Registre
// (technical/03 §4.4).
type NetworthMonth struct {
	ID        int64
	UserID    int64
	Period    string
	Comment   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// LabelMapping powers autocomplete + learned categorisation (technical/03 §5.1).
type LabelMapping struct {
	ID         int64
	UserID     int64
	Label      string
	LabelKey   string
	CategoryID *int64
	AccountID  *int64
	UsageCount int
	LastUsedAt time.Time
}

// UIPreference is a per-user persisted expand/collapse state (technical/03 §5.2).
type UIPreference struct {
	ID        int64
	UserID    int64
	NodeType  NodeType
	NodeID    int64
	Expanded  bool
	UpdatedAt time.Time
}

// Invitation is a single-use, expiring account invitation (technical/03 §2.3).
type Invitation struct {
	ID             int64
	Email          *string
	TokenHash      string
	InvitedIsAdmin bool
	CreatedBy      int64
	ExpiresAt      time.Time
	ConsumedAt     *time.Time
	RevokedAt      *time.Time
}

// TOTPBackupCode is a single-use 2FA recovery code (technical/03 §2.4).
type TOTPBackupCode struct {
	ID         int64
	UserID     int64
	CodeHash   string
	ConsumedAt *time.Time
}
