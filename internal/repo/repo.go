package repo

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"econome/internal/domain"
)

// DBTX is the subset of *sql.DB / *sql.Tx the repositories use, so every method
// runs either standalone or inside a service-owned transaction. Both *sql.DB and
// *sql.Tx satisfy it.
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// UserRepo persists tenant identities. GetByEmail is the documented pre-auth
// exception to user_id scoping (login has no user_id yet); every other
// user-owned query is user_id-scoped (technical/01 §4).
type UserRepo interface {
	CountUsers(ctx context.Context, q DBTX) (int, error)
	GetByEmail(ctx context.Context, q DBTX, email string) (*domain.User, error)
	GetByID(ctx context.Context, q DBTX, id int64) (*domain.User, error)
	Create(ctx context.Context, q DBTX, u *domain.User) (int64, error)
	UpdateLoginState(ctx context.Context, q DBTX, u *domain.User) error
	UpdatePasswordHash(ctx context.Context, q DBTX, id int64, hash string) error
}

// SessionRepo persists opaque sessions (token stored only as its SHA-256).
type SessionRepo interface {
	Create(ctx context.Context, q DBTX, s *domain.Session) (int64, error)
	GetByTokenHash(ctx context.Context, q DBTX, tokenHash string) (*domain.Session, error)
	Touch(ctx context.Context, q DBTX, id int64, lastSeen, expires time.Time) error
	Delete(ctx context.Context, q DBTX, id int64) error
	DeleteByUser(ctx context.Context, q DBTX, userID int64) error
}

// SettingsRepo persists the single per-user settings row.
type SettingsRepo interface {
	Create(ctx context.Context, q DBTX, s *domain.Settings) error
	Get(ctx context.Context, q DBTX, userID int64) (*domain.Settings, error)
	Update(ctx context.Context, q DBTX, s *domain.Settings) error
}

// Txer exposes the transaction seam so a service can drive a multi-write
// use-case atomically (owner creation) or run a single query on the pool.
type Txer interface {
	WithTx(ctx context.Context, fn func(q DBTX) error) error
	DB() DBTX
}

// Store is the SQLite-backed implementation of the repositories + Txer.
type Store struct {
	db           *sql.DB
	Users        UserRepo
	Sessions     SessionRepo
	Settings     SettingsRepo
	Accounts     AccountRepo
	Categories   CategoryRepo
	Envelopes    EnvelopeRepo
	Allocations  AllocationRepo
	Transactions TransactionRepo
}

// New wires the SQLite repositories over db.
func New(db *sql.DB) *Store {
	return &Store{
		db:           db,
		Users:        userRepo{},
		Sessions:     sessionRepo{},
		Settings:     settingsRepo{},
		Accounts:     accountRepo{},
		Categories:   categoryRepo{},
		Envelopes:    envelopeRepo{},
		Allocations:  allocationRepo{},
		Transactions: transactionRepo{},
	}
}

// DB returns the connection pool as a DBTX for non-transactional queries.
func (s *Store) DB() DBTX { return s.db }

// WithTx runs fn inside a transaction, committing on success and rolling back on
// error or panic (the "no partial write" rule).
func (s *Store) WithTx(ctx context.Context, fn func(q DBTX) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("repo: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op after a successful commit
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// --- shared time<->TEXT helpers (ISO-8601 UTC; technical/03 §1) ---

const tsLayout = time.RFC3339

func nowUTC() time.Time { return time.Now().UTC() }

func formatTime(t time.Time) string { return t.UTC().Format(tsLayout) }

func formatNullTime(t *time.Time) sql.NullString {
	if t == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: formatTime(*t), Valid: true}
}

func parseTime(s string) (time.Time, error) {
	return time.Parse(tsLayout, s)
}

func parseNullTime(ns sql.NullString) (*time.Time, error) {
	if !ns.Valid {
		return nil, nil
	}
	t, err := parseTime(ns.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func nullString(p *string) sql.NullString {
	if p == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *p, Valid: true}
}

func ptrString(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	v := ns.String
	return &v
}

func nullInt64(p *int64) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *p, Valid: true}
}

func ptrInt64(ni sql.NullInt64) *int64 {
	if !ni.Valid {
		return nil
	}
	v := ni.Int64
	return &v
}

// rowScanner is satisfied by *sql.Row and *sql.Rows, so scan helpers serve both
// single-row and iterated reads.
type rowScanner interface {
	Scan(dest ...any) error
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
