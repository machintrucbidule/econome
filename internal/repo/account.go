package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"econome/internal/domain"
)

type accountRepo struct{}

const selectAccount = `SELECT id, user_id, name, type, month_end_policy, fill_priority, ceiling,
	status, external_ref, created_at, updated_at FROM account`

func (accountRepo) Create(ctx context.Context, q DBTX, a *domain.Account) (int64, error) {
	res, err := q.ExecContext(ctx,
		`INSERT INTO account (user_id, name, type, month_end_policy, fill_priority, ceiling,
			status, external_ref, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.UserID, a.Name, string(a.Type), string(a.MonthEndPolicy), nullIntP(a.FillPriority), nullInt64(a.Ceiling),
		nullStatus(a.Status), nullString(a.ExternalRef), formatTime(a.CreatedAt), formatTime(a.UpdatedAt))
	if err != nil {
		if isUniqueViolation(err) {
			return 0, domain.ErrDuplicate
		}
		return 0, fmt.Errorf("repo: create account: %w", err)
	}
	return res.LastInsertId()
}

func (accountRepo) Get(ctx context.Context, q DBTX, userID, id int64) (*domain.Account, error) {
	return scanAccount(q.QueryRowContext(ctx, selectAccount+` WHERE user_id = ? AND id = ?`, userID, id))
}

func (accountRepo) Update(ctx context.Context, q DBTX, a *domain.Account) error {
	_, err := q.ExecContext(ctx,
		`UPDATE account SET name = ?, type = ?, month_end_policy = ?, fill_priority = ?, ceiling = ?,
			status = ?, external_ref = ?, updated_at = ?
		 WHERE user_id = ? AND id = ?`,
		a.Name, string(a.Type), string(a.MonthEndPolicy), nullIntP(a.FillPriority), nullInt64(a.Ceiling),
		nullStatus(a.Status), nullString(a.ExternalRef), formatTime(nowUTC()), a.UserID, a.ID)
	if err != nil {
		if isUniqueViolation(err) {
			return domain.ErrDuplicate
		}
		return fmt.Errorf("repo: update account: %w", err)
	}
	return nil
}

func (accountRepo) Delete(ctx context.Context, q DBTX, userID, id int64) error {
	res, err := q.ExecContext(ctx, `DELETE FROM account WHERE user_id = ? AND id = ?`, userID, id)
	if err != nil {
		if isForeignKeyViolation(err) {
			return domain.ErrConflict // has dependents (RESTRICT, L4)
		}
		return fmt.Errorf("repo: delete account: %w", err)
	}
	return notFoundIfNoRows(res)
}

func (accountRepo) ListByUser(ctx context.Context, q DBTX, userID int64) ([]domain.Account, error) {
	rows, err := q.QueryContext(ctx, selectAccount+` WHERE user_id = ? ORDER BY id`, userID)
	if err != nil {
		return nil, fmt.Errorf("repo: list accounts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []domain.Account
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

func scanAccount(row rowScanner) (*domain.Account, error) {
	var (
		a            domain.Account
		fillPriority sql.NullInt64
		ceiling      sql.NullInt64
		externalRef  sql.NullString
		createdAt    string
		updatedAt    string
	)
	err := row.Scan(&a.ID, &a.UserID, &a.Name, &a.Type, &a.MonthEndPolicy, &fillPriority, &ceiling,
		&a.Status, &externalRef, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("repo: scan account: %w", err)
	}
	a.FillPriority = ptrIntP(fillPriority)
	a.Ceiling = ptrInt64(ceiling)
	a.ExternalRef = ptrString(externalRef)
	if a.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, err
	}
	if a.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return nil, err
	}
	return &a, nil
}
