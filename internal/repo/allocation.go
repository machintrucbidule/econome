package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"econome/internal/domain"
)

type allocationRepo struct{}

const selectAllocation = `SELECT id, user_id, envelope_id, period, planned_amount, created_at, updated_at FROM allocation`

func (allocationRepo) Create(ctx context.Context, q DBTX, a *domain.Allocation) (int64, error) {
	res, err := q.ExecContext(ctx,
		`INSERT INTO allocation (user_id, envelope_id, period, planned_amount, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		a.UserID, a.EnvelopeID, a.Period, a.PlannedAmount, formatTime(a.CreatedAt), formatTime(a.UpdatedAt))
	if err != nil {
		if isUniqueViolation(err) {
			return 0, domain.ErrDuplicate // one allocation per (envelope, period)
		}
		return 0, fmt.Errorf("repo: create allocation: %w", err)
	}
	return res.LastInsertId()
}

func (allocationRepo) Update(ctx context.Context, q DBTX, a *domain.Allocation) error {
	_, err := q.ExecContext(ctx,
		`UPDATE allocation SET planned_amount = ?, updated_at = ? WHERE user_id = ? AND id = ?`,
		a.PlannedAmount, formatTime(nowUTC()), a.UserID, a.ID)
	if err != nil {
		return fmt.Errorf("repo: update allocation: %w", err)
	}
	return nil
}

func (allocationRepo) Delete(ctx context.Context, q DBTX, userID, id int64) error {
	res, err := q.ExecContext(ctx, `DELETE FROM allocation WHERE user_id = ? AND id = ?`, userID, id)
	if err != nil {
		return fmt.Errorf("repo: delete allocation: %w", err)
	}
	return notFoundIfNoRows(res)
}

func (allocationRepo) ByEnvelopePeriod(ctx context.Context, q DBTX, userID, envelopeID int64, period string) (*domain.Allocation, error) {
	return scanAllocation(q.QueryRowContext(ctx,
		selectAllocation+` WHERE user_id = ? AND envelope_id = ? AND period = ?`, userID, envelopeID, period))
}

func (allocationRepo) ListByPeriod(ctx context.Context, q DBTX, userID int64, period string) ([]domain.Allocation, error) {
	rows, err := q.QueryContext(ctx, selectAllocation+` WHERE user_id = ? AND period = ? ORDER BY id`, userID, period)
	if err != nil {
		return nil, fmt.Errorf("repo: list allocations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []domain.Allocation
	for rows.Next() {
		a, err := scanAllocation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

func scanAllocation(row rowScanner) (*domain.Allocation, error) {
	var (
		a                    domain.Allocation
		createdAt, updatedAt string
	)
	err := row.Scan(&a.ID, &a.UserID, &a.EnvelopeID, &a.Period, &a.PlannedAmount, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("repo: scan allocation: %w", err)
	}
	if a.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, err
	}
	if a.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return nil, err
	}
	return &a, nil
}
