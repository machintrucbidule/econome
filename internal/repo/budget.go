package repo

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"econome/internal/domain"
)

// Budget repository interfaces (consumer-side). Every method is user_id-scoped
// (technical/01 §4): a row owned by another user is indistinguishable from absent
// (cross-tenant ⇒ ErrNotFound / empty), never 403.

// AccountRepo persists accounts.
type AccountRepo interface {
	Create(ctx context.Context, q DBTX, a *domain.Account) (int64, error)
	Get(ctx context.Context, q DBTX, userID, id int64) (*domain.Account, error)
	Update(ctx context.Context, q DBTX, a *domain.Account) error
	Delete(ctx context.Context, q DBTX, userID, id int64) error
	ListByUser(ctx context.Context, q DBTX, userID int64) ([]domain.Account, error)
}

// CategoryRepo persists categories.
type CategoryRepo interface {
	Create(ctx context.Context, q DBTX, c *domain.Category) (int64, error)
	Get(ctx context.Context, q DBTX, userID, id int64) (*domain.Category, error)
	Update(ctx context.Context, q DBTX, c *domain.Category) error
	Delete(ctx context.Context, q DBTX, userID, id int64) error
	ListByUser(ctx context.Context, q DBTX, userID int64) ([]domain.Category, error)
}

// EnvelopeRepo persists envelopes.
type EnvelopeRepo interface {
	Create(ctx context.Context, q DBTX, e *domain.Envelope) (int64, error)
	Get(ctx context.Context, q DBTX, userID, id int64) (*domain.Envelope, error)
	Update(ctx context.Context, q DBTX, e *domain.Envelope) error
	Delete(ctx context.Context, q DBTX, userID, id int64) error
	ListByUser(ctx context.Context, q DBTX, userID int64) ([]domain.Envelope, error)
	ListByAccount(ctx context.Context, q DBTX, userID, accountID int64) ([]domain.Envelope, error)
}

// AllocationRepo persists allocations.
type AllocationRepo interface {
	Create(ctx context.Context, q DBTX, a *domain.Allocation) (int64, error)
	Update(ctx context.Context, q DBTX, a *domain.Allocation) error
	Delete(ctx context.Context, q DBTX, userID, id int64) error
	ByEnvelopePeriod(ctx context.Context, q DBTX, userID, envelopeID int64, period string) (*domain.Allocation, error)
	ListByPeriod(ctx context.Context, q DBTX, userID int64, period string) ([]domain.Allocation, error)
}

// TransactionRepo persists transactions.
type TransactionRepo interface {
	Create(ctx context.Context, q DBTX, t *domain.Transaction) (int64, error)
	Get(ctx context.Context, q DBTX, userID, id int64) (*domain.Transaction, error)
	Update(ctx context.Context, q DBTX, t *domain.Transaction) error
	Delete(ctx context.Context, q DBTX, userID, id int64) error
	ListByPeriod(ctx context.Context, q DBTX, userID int64, period string) ([]domain.Transaction, error)
	ListByAccountPeriod(ctx context.Context, q DBTX, userID, accountID int64, period string) ([]domain.Transaction, error)
}

// --- shared mapping helpers ---

func formatNullDate(d *domain.Date) sql.NullString {
	if d == nil || d.IsZero() {
		return sql.NullString{}
	}
	return sql.NullString{String: d.String(), Valid: true}
}

func parseNullDate(ns sql.NullString) (*domain.Date, error) {
	if !ns.Valid || ns.String == "" {
		return nil, nil
	}
	var y, m, dd int
	if _, err := fmt.Sscanf(ns.String, "%d-%d-%d", &y, &m, &dd); err != nil {
		return nil, fmt.Errorf("repo: parse date %q: %w", ns.String, err)
	}
	d := domain.NewDate(y, m, dd)
	return &d, nil
}

func formatDueMonths(months []int) sql.NullString {
	if len(months) == 0 {
		return sql.NullString{}
	}
	parts := make([]string, len(months))
	for i, m := range months {
		parts[i] = strconv.Itoa(m)
	}
	return sql.NullString{String: strings.Join(parts, ","), Valid: true}
}

func parseDueMonths(ns sql.NullString) ([]int, error) {
	if !ns.Valid || ns.String == "" {
		return nil, nil
	}
	parts := strings.Split(ns.String, ",")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return nil, fmt.Errorf("repo: parse due_months %q: %w", ns.String, err)
		}
		out = append(out, n)
	}
	return out, nil
}

func nullStatus(s domain.ArchiveStatus) string {
	if s == "" {
		return string(domain.ArchiveActive)
	}
	return string(s)
}

// notFoundIfNoRows turns a delete/update that affected no rows into ErrNotFound
// (also the cross-tenant outcome — a row owned by another user is absent here).
func notFoundIfNoRows(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("repo: rows affected: %w", err)
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func nullIntP(p *int) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*p), Valid: true}
}

func ptrIntP(ni sql.NullInt64) *int {
	if !ni.Valid {
		return nil
	}
	v := int(ni.Int64)
	return &v
}
