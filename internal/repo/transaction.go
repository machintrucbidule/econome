package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"econome/internal/domain"
)

type transactionRepo struct{}

const selectTxn = `SELECT id, user_id, account_id, dest_account_id, category_id, flow_type, amount,
	op_date, budget_period, status, label, note, source, external_ref, paired_transaction_id,
	created_at, updated_at FROM "transaction"`

func (transactionRepo) Create(ctx context.Context, q DBTX, t *domain.Transaction) (int64, error) {
	res, err := q.ExecContext(ctx,
		`INSERT INTO "transaction" (user_id, account_id, dest_account_id, category_id, flow_type, amount,
			op_date, budget_period, status, label, note, source, external_ref, paired_transaction_id,
			created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.UserID, t.AccountID, nullInt64(t.DestAccountID), nullInt64(t.CategoryID), string(t.FlowType), t.Amount,
		formatNullDate(t.OpDate), t.BudgetPeriod, string(t.Status), t.Label, nullString(t.Note), nullSource(t.Source),
		nullString(t.ExternalRef), nullInt64(t.PairedTransactionID), formatTime(t.CreatedAt), formatTime(t.UpdatedAt))
	if err != nil {
		return 0, fmt.Errorf("repo: create transaction: %w", err)
	}
	return res.LastInsertId()
}

func (transactionRepo) Get(ctx context.Context, q DBTX, userID, id int64) (*domain.Transaction, error) {
	return scanTxn(q.QueryRowContext(ctx, selectTxn+` WHERE user_id = ? AND id = ?`, userID, id))
}

func (transactionRepo) Update(ctx context.Context, q DBTX, t *domain.Transaction) error {
	_, err := q.ExecContext(ctx,
		`UPDATE "transaction" SET account_id = ?, dest_account_id = ?, category_id = ?, flow_type = ?, amount = ?,
			op_date = ?, budget_period = ?, status = ?, label = ?, note = ?, source = ?, external_ref = ?,
			paired_transaction_id = ?, updated_at = ?
		 WHERE user_id = ? AND id = ?`,
		t.AccountID, nullInt64(t.DestAccountID), nullInt64(t.CategoryID), string(t.FlowType), t.Amount,
		formatNullDate(t.OpDate), t.BudgetPeriod, string(t.Status), t.Label, nullString(t.Note), nullSource(t.Source),
		nullString(t.ExternalRef), nullInt64(t.PairedTransactionID), formatTime(nowUTC()), t.UserID, t.ID)
	if err != nil {
		return fmt.Errorf("repo: update transaction: %w", err)
	}
	return nil
}

func (transactionRepo) Delete(ctx context.Context, q DBTX, userID, id int64) error {
	res, err := q.ExecContext(ctx, `DELETE FROM "transaction" WHERE user_id = ? AND id = ?`, userID, id)
	if err != nil {
		return fmt.Errorf("repo: delete transaction: %w", err)
	}
	return notFoundIfNoRows(res)
}

func (r transactionRepo) ListByPeriod(ctx context.Context, q DBTX, userID int64, period string) ([]domain.Transaction, error) {
	return r.list(ctx, q, selectTxn+` WHERE user_id = ? AND budget_period = ? ORDER BY id`, userID, period)
}

func (r transactionRepo) ListByAccountPeriod(ctx context.Context, q DBTX, userID, accountID int64, period string) ([]domain.Transaction, error) {
	return r.list(ctx, q, selectTxn+` WHERE user_id = ? AND account_id = ? AND budget_period = ? ORDER BY id`, userID, accountID, period)
}

func (transactionRepo) list(ctx context.Context, q DBTX, query string, args ...any) ([]domain.Transaction, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("repo: list transactions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []domain.Transaction
	for rows.Next() {
		t, err := scanTxn(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

func scanTxn(row rowScanner) (*domain.Transaction, error) {
	var (
		t                                   domain.Transaction
		destAccountID, categoryID, pairedID sql.NullInt64
		opDate, note, externalRef           sql.NullString
		createdAt, updatedAt                string
	)
	err := row.Scan(&t.ID, &t.UserID, &t.AccountID, &destAccountID, &categoryID, &t.FlowType, &t.Amount,
		&opDate, &t.BudgetPeriod, &t.Status, &t.Label, &note, &t.Source, &externalRef, &pairedID, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("repo: scan transaction: %w", err)
	}
	t.DestAccountID = ptrInt64(destAccountID)
	t.CategoryID = ptrInt64(categoryID)
	t.PairedTransactionID = ptrInt64(pairedID)
	t.Note = ptrString(note)
	t.ExternalRef = ptrString(externalRef)
	if t.OpDate, err = parseNullDate(opDate); err != nil {
		return nil, err
	}
	if t.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, err
	}
	if t.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return nil, err
	}
	return &t, nil
}

func nullSource(s domain.TxnSource) string {
	if s == "" {
		return string(domain.SourceManual)
	}
	return string(s)
}
