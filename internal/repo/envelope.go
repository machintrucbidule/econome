package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"econome/internal/domain"
)

type envelopeRepo struct{}

const selectEnvelope = `SELECT id, user_id, category_id, account_id, mode, default_amount, frequency,
	due_months, expected_day, status, created_at, updated_at FROM envelope`

func (envelopeRepo) Create(ctx context.Context, q DBTX, e *domain.Envelope) (int64, error) {
	res, err := q.ExecContext(ctx,
		`INSERT INTO envelope (user_id, category_id, account_id, mode, default_amount, frequency,
			due_months, expected_day, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.UserID, e.CategoryID, e.AccountID, string(e.Mode), nullInt64(e.DefaultAmount), nullFrequency(e.Frequency),
		formatDueMonths(e.DueMonths), nullIntP(e.ExpectedDay), nullStatus(e.Status), formatTime(e.CreatedAt), formatTime(e.UpdatedAt))
	if err != nil {
		if isUniqueViolation(err) {
			return 0, domain.ErrDuplicate
		}
		return 0, fmt.Errorf("repo: create envelope: %w", err)
	}
	return res.LastInsertId()
}

func (envelopeRepo) Get(ctx context.Context, q DBTX, userID, id int64) (*domain.Envelope, error) {
	return scanEnvelope(q.QueryRowContext(ctx, selectEnvelope+` WHERE user_id = ? AND id = ?`, userID, id))
}

func (envelopeRepo) Update(ctx context.Context, q DBTX, e *domain.Envelope) error {
	_, err := q.ExecContext(ctx,
		`UPDATE envelope SET category_id = ?, account_id = ?, mode = ?, default_amount = ?, frequency = ?,
			due_months = ?, expected_day = ?, status = ?, updated_at = ?
		 WHERE user_id = ? AND id = ?`,
		e.CategoryID, e.AccountID, string(e.Mode), nullInt64(e.DefaultAmount), nullFrequency(e.Frequency),
		formatDueMonths(e.DueMonths), nullIntP(e.ExpectedDay), nullStatus(e.Status), formatTime(nowUTC()), e.UserID, e.ID)
	if err != nil {
		if isUniqueViolation(err) {
			return domain.ErrDuplicate
		}
		return fmt.Errorf("repo: update envelope: %w", err)
	}
	return nil
}

func (envelopeRepo) Delete(ctx context.Context, q DBTX, userID, id int64) error {
	res, err := q.ExecContext(ctx, `DELETE FROM envelope WHERE user_id = ? AND id = ?`, userID, id)
	if err != nil {
		if isForeignKeyViolation(err) {
			return domain.ErrConflict
		}
		return fmt.Errorf("repo: delete envelope: %w", err)
	}
	return notFoundIfNoRows(res)
}

func (r envelopeRepo) ListByUser(ctx context.Context, q DBTX, userID int64) ([]domain.Envelope, error) {
	return r.list(ctx, q, selectEnvelope+` WHERE user_id = ? ORDER BY id`, userID)
}

func (r envelopeRepo) ListByAccount(ctx context.Context, q DBTX, userID, accountID int64) ([]domain.Envelope, error) {
	return r.list(ctx, q, selectEnvelope+` WHERE user_id = ? AND account_id = ? ORDER BY id`, userID, accountID)
}

func (envelopeRepo) list(ctx context.Context, q DBTX, query string, args ...any) ([]domain.Envelope, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("repo: list envelopes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []domain.Envelope
	for rows.Next() {
		e, err := scanEnvelope(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

func scanEnvelope(row rowScanner) (*domain.Envelope, error) {
	var (
		e                    domain.Envelope
		defaultAmount        sql.NullInt64
		frequency, dueMonths sql.NullString
		expectedDay          sql.NullInt64
		createdAt, updatedAt string
	)
	err := row.Scan(&e.ID, &e.UserID, &e.CategoryID, &e.AccountID, &e.Mode, &defaultAmount, &frequency,
		&dueMonths, &expectedDay, &e.Status, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("repo: scan envelope: %w", err)
	}
	e.DefaultAmount = ptrInt64(defaultAmount)
	e.ExpectedDay = ptrIntP(expectedDay)
	if frequency.Valid {
		f := domain.Frequency(frequency.String)
		e.Frequency = &f
	}
	if e.DueMonths, err = parseDueMonths(dueMonths); err != nil {
		return nil, err
	}
	if e.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, err
	}
	if e.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return nil, err
	}
	return &e, nil
}

func nullFrequency(f *domain.Frequency) sql.NullString {
	if f == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: string(*f), Valid: true}
}
