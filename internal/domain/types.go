package domain

import "time"

// Enums use English internal codes; FR/EN display labels live in internal/i18n,
// keyed by these codes (technical/06 §4). Every switch over an enum must be
// exhaustive (the `exhaustive` lint gate).

// Status is a user account's lifecycle state (functional/01 §1.2, A8).
type Status string

// Status codes.
const (
	StatusActive      Status = "active"
	StatusDeactivated Status = "deactivated"
)

// Valid reports whether s is a known status code.
func (s Status) Valid() bool {
	switch s {
	case StatusActive, StatusDeactivated:
		return true
	default:
		return false
	}
}

// Language is the UI locale (technical/06).
type Language string

// Language codes.
const (
	LangFR Language = "fr"
	LangEN Language = "en"
)

// Valid reports whether l is a supported language.
func (l Language) Valid() bool {
	switch l {
	case LangFR, LangEN:
		return true
	default:
		return false
	}
}

// SessionKind distinguishes a short (idle-expiring) session from a long-lived
// "remember me" session (technical/05 §2, A12).
type SessionKind string

// Session kinds.
const (
	SessionShort    SessionKind = "short"
	SessionRemember SessionKind = "remember"
)

// Valid reports whether k is a known session kind.
func (k SessionKind) Valid() bool {
	switch k {
	case SessionShort, SessionRemember:
		return true
	default:
		return false
	}
}

// SecuredSavingsBasis selects which planned amounts count towards secured
// savings (functional/03 C1).
type SecuredSavingsBasis string

// Secured-savings bases.
const (
	BasisAllPlanned SecuredSavingsBasis = "all_planned"
	BasisFixedOnly  SecuredSavingsBasis = "fixed_only"
)

// Valid reports whether b is a known basis.
func (b SecuredSavingsBasis) Valid() bool {
	switch b {
	case BasisAllPlanned, BasisFixedOnly:
		return true
	default:
		return false
	}
}

// Theme is the persisted colour-scheme choice (M12).
type Theme string

// Theme codes.
const (
	ThemeLight Theme = "light"
	ThemeDark  Theme = "dark"
)

// Valid reports whether t is a known theme.
func (t Theme) Valid() bool {
	switch t {
	case ThemeLight, ThemeDark:
		return true
	default:
		return false
	}
}

// User is a tenant identity (technical/03 §2.1). Money/rate fields are integer
// minor units / basis points; the domain never imports the engine.
type User struct {
	ID                 int64
	Email              string
	PasswordHash       string // Argon2id PHC string (technical/05 §1)
	IsAdmin            bool
	Status             Status
	Language           Language
	Currency           string
	TOTPEnabled        bool
	TOTPSecret         *string // base32, set when 2FA is enabled (inc 8)
	MustChangePassword bool
	FailedLoginCount   int
	LastFailedLoginAt  *time.Time
	LockedUntil        *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Session is an opaque server-side session (technical/03 §2.2). Only the SHA-256
// of the cookie token is stored (TokenHash); the raw token never persists.
type Session struct {
	ID         int64
	UserID     int64
	TokenHash  string
	Kind       SessionKind
	ExpiresAt  time.Time
	CreatedAt  time.Time
	LastSeenAt time.Time
	UserAgent  *string
	IP         *string
}

// Settings is the single per-user configuration row (technical/03 §4.5). Money
// is in minor units; rates are basis points. DefaultAccountID has no DB-level
// foreign key until the account table exists (I-011); integrity is enforced in
// the service layer.
type Settings struct {
	UserID              int64
	DefaultAccountID    *int64
	PEAInitialDeposit   int64 // minor units
	PEASocialChargeRate int   // basis points (default 1720 = 17.20 %)
	NearCapThreshold    int   // basis points (default 9000 = 90 %)
	SecuredSavingsBasis SecuredSavingsBasis
	CommentAutoprefill  bool
	Theme               Theme
	Language            Language
	Currency            string
	DSP2Enabled         bool
	UpdatedAt           time.Time
}
