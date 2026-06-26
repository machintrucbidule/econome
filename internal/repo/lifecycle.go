package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"econome/internal/domain"
)

// PeriodRepo persists month-lifecycle rows.
type PeriodRepo interface {
	Create(ctx context.Context, q DBTX, p *domain.Period) (int64, error)
	ByPeriod(ctx context.Context, q DBTX, userID int64, period string) (*domain.Period, error)
	UpdateState(ctx context.Context, q DBTX, userID int64, period string, state domain.PeriodState, lockedAt *time.Time) error
}

// PeriodEventRepo is the append-only lifecycle audit log.
type PeriodEventRepo interface {
	Append(ctx context.Context, q DBTX, e *domain.PeriodEvent) (int64, error)
	ListByPeriod(ctx context.Context, q DBTX, userID int64, period string) ([]domain.PeriodEvent, error)
}

// SnapshotRepo persists savings snapshots.
type SnapshotRepo interface {
	Upsert(ctx context.Context, q DBTX, s *domain.Snapshot) error
	ByAccountPeriod(ctx context.Context, q DBTX, userID, accountID int64, period string) (*domain.Snapshot, error)
	ListByPeriod(ctx context.Context, q DBTX, userID int64, period string) ([]domain.Snapshot, error)
	Delete(ctx context.Context, q DBTX, userID, id int64) error
}

// NetworthMonthRepo persists the per-month comment.
type NetworthMonthRepo interface {
	Get(ctx context.Context, q DBTX, userID int64, period string) (*domain.NetworthMonth, error)
	Upsert(ctx context.Context, q DBTX, userID int64, period, comment string) error
}

type periodRepo struct{}

func (periodRepo) Create(ctx context.Context, q DBTX, p *domain.Period) (int64, error) {
	res, err := q.ExecContext(ctx,
		`INSERT INTO period (user_id, period, state, locked_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		p.UserID, p.Period, string(p.State), formatNullTime(p.LockedAt), formatTime(p.CreatedAt), formatTime(p.UpdatedAt))
	if err != nil {
		if isUniqueViolation(err) {
			return 0, domain.ErrDuplicate
		}
		return 0, fmt.Errorf("repo: create period: %w", err)
	}
	return res.LastInsertId()
}

func (periodRepo) ByPeriod(ctx context.Context, q DBTX, userID int64, period string) (*domain.Period, error) {
	var (
		p                    domain.Period
		lockedAt             sql.NullString
		createdAt, updatedAt string
	)
	err := q.QueryRowContext(ctx,
		`SELECT id, user_id, period, state, locked_at, created_at, updated_at FROM period WHERE user_id = ? AND period = ?`,
		userID, period).Scan(&p.ID, &p.UserID, &p.Period, &p.State, &lockedAt, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("repo: scan period: %w", err)
	}
	if p.LockedAt, err = parseNullTime(lockedAt); err != nil {
		return nil, err
	}
	if p.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, err
	}
	if p.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return nil, err
	}
	return &p, nil
}

func (periodRepo) UpdateState(ctx context.Context, q DBTX, userID int64, period string, state domain.PeriodState, lockedAt *time.Time) error {
	res, err := q.ExecContext(ctx,
		`UPDATE period SET state = ?, locked_at = ?, updated_at = ? WHERE user_id = ? AND period = ?`,
		string(state), formatNullTime(lockedAt), formatTime(nowUTC()), userID, period)
	if err != nil {
		return fmt.Errorf("repo: update period state: %w", err)
	}
	return notFoundIfNoRows(res)
}

type periodEventRepo struct{}

func (periodEventRepo) Append(ctx context.Context, q DBTX, e *domain.PeriodEvent) (int64, error) {
	res, err := q.ExecContext(ctx,
		`INSERT INTO period_event (user_id, period, action, at, actor_user_id) VALUES (?, ?, ?, ?, ?)`,
		e.UserID, e.Period, string(e.Action), formatTime(e.At), e.ActorUserID)
	if err != nil {
		return 0, fmt.Errorf("repo: append period event: %w", err)
	}
	return res.LastInsertId()
}

func (periodEventRepo) ListByPeriod(ctx context.Context, q DBTX, userID int64, period string) ([]domain.PeriodEvent, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, user_id, period, action, at, actor_user_id FROM period_event WHERE user_id = ? AND period = ? ORDER BY id`,
		userID, period)
	if err != nil {
		return nil, fmt.Errorf("repo: list period events: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []domain.PeriodEvent
	for rows.Next() {
		var (
			e  domain.PeriodEvent
			at string
		)
		if err := rows.Scan(&e.ID, &e.UserID, &e.Period, &e.Action, &at, &e.ActorUserID); err != nil {
			return nil, fmt.Errorf("repo: scan period event: %w", err)
		}
		if e.At, err = parseTime(at); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

type snapshotRepo struct{}

func (snapshotRepo) Upsert(ctx context.Context, q DBTX, s *domain.Snapshot) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO savings_snapshot (user_id, account_id, period, gross_value, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(account_id, period) DO UPDATE SET gross_value = excluded.gross_value, updated_at = excluded.updated_at`,
		s.UserID, s.AccountID, s.Period, s.GrossValue, formatTime(nowUTC()), formatTime(nowUTC()))
	if err != nil {
		return fmt.Errorf("repo: upsert snapshot: %w", err)
	}
	return nil
}

func (snapshotRepo) ByAccountPeriod(ctx context.Context, q DBTX, userID, accountID int64, period string) (*domain.Snapshot, error) {
	return scanSnapshot(q.QueryRowContext(ctx,
		`SELECT id, user_id, account_id, period, gross_value, created_at, updated_at FROM savings_snapshot
		 WHERE user_id = ? AND account_id = ? AND period = ?`, userID, accountID, period))
}

func (snapshotRepo) ListByPeriod(ctx context.Context, q DBTX, userID int64, period string) ([]domain.Snapshot, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, user_id, account_id, period, gross_value, created_at, updated_at FROM savings_snapshot
		 WHERE user_id = ? AND period = ? ORDER BY id`, userID, period)
	if err != nil {
		return nil, fmt.Errorf("repo: list snapshots: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []domain.Snapshot
	for rows.Next() {
		s, err := scanSnapshot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

func (snapshotRepo) Delete(ctx context.Context, q DBTX, userID, id int64) error {
	res, err := q.ExecContext(ctx, `DELETE FROM savings_snapshot WHERE user_id = ? AND id = ?`, userID, id)
	if err != nil {
		return fmt.Errorf("repo: delete snapshot: %w", err)
	}
	return notFoundIfNoRows(res)
}

func scanSnapshot(row rowScanner) (*domain.Snapshot, error) {
	var (
		s                    domain.Snapshot
		createdAt, updatedAt string
	)
	err := row.Scan(&s.ID, &s.UserID, &s.AccountID, &s.Period, &s.GrossValue, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("repo: scan snapshot: %w", err)
	}
	if s.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, err
	}
	if s.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return nil, err
	}
	return &s, nil
}

type networthMonthRepo struct{}

func (networthMonthRepo) Get(ctx context.Context, q DBTX, userID int64, period string) (*domain.NetworthMonth, error) {
	var (
		m                    domain.NetworthMonth
		createdAt, updatedAt string
	)
	err := q.QueryRowContext(ctx,
		`SELECT id, user_id, period, comment, created_at, updated_at FROM networth_month WHERE user_id = ? AND period = ?`,
		userID, period).Scan(&m.ID, &m.UserID, &m.Period, &m.Comment, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("repo: scan networth_month: %w", err)
	}
	if m.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, err
	}
	if m.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return nil, err
	}
	return &m, nil
}

func (networthMonthRepo) Upsert(ctx context.Context, q DBTX, userID int64, period, comment string) error {
	now := formatTime(nowUTC())
	_, err := q.ExecContext(ctx,
		`INSERT INTO networth_month (user_id, period, comment, created_at, updated_at) VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(user_id, period) DO UPDATE SET comment = excluded.comment, updated_at = excluded.updated_at`,
		userID, period, comment, now, now)
	if err != nil {
		return fmt.Errorf("repo: upsert networth_month: %w", err)
	}
	return nil
}
