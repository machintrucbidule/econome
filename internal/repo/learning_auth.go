package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"econome/internal/domain"
)

// LabelMappingRepo persists learned label→category/account mappings (M21).
type LabelMappingRepo interface {
	Search(ctx context.Context, q DBTX, userID int64, prefix string, limit int) ([]domain.LabelMapping, error)
	Upsert(ctx context.Context, q DBTX, m *domain.LabelMapping) error
}

// UIPreferenceRepo persists per-user expand/collapse state (M4).
type UIPreferenceRepo interface {
	Upsert(ctx context.Context, q DBTX, p *domain.UIPreference) error
	ListByUser(ctx context.Context, q DBTX, userID int64) ([]domain.UIPreference, error)
}

// InvitationRepo persists invitations.
type InvitationRepo interface {
	Create(ctx context.Context, q DBTX, inv *domain.Invitation) (int64, error)
	ByTokenHash(ctx context.Context, q DBTX, tokenHash string) (*domain.Invitation, error)
	ByID(ctx context.Context, q DBTX, id int64) (*domain.Invitation, error)
	Update(ctx context.Context, q DBTX, inv *domain.Invitation) error
	ListByCreator(ctx context.Context, q DBTX, createdBy int64) ([]domain.Invitation, error)
}

// TOTPBackupRepo persists single-use 2FA backup codes.
type TOTPBackupRepo interface {
	Create(ctx context.Context, q DBTX, c *domain.TOTPBackupCode) (int64, error)
	ListByUser(ctx context.Context, q DBTX, userID int64) ([]domain.TOTPBackupCode, error)
	MarkConsumed(ctx context.Context, q DBTX, userID, id int64) error
	DeleteByUser(ctx context.Context, q DBTX, userID int64) error
}

type labelMappingRepo struct{}

func (labelMappingRepo) Search(ctx context.Context, q DBTX, userID int64, prefix string, limit int) ([]domain.LabelMapping, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, user_id, label, label_key, category_id, account_id, usage_count, last_used_at
		 FROM label_mapping WHERE user_id = ? AND label_key LIKE ? ESCAPE '\' ORDER BY usage_count DESC, label LIMIT ?`,
		userID, escapeLike(prefix)+"%", limit)
	if err != nil {
		return nil, fmt.Errorf("repo: search labels: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []domain.LabelMapping
	for rows.Next() {
		m, err := scanLabel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	return out, rows.Err()
}

func (labelMappingRepo) Upsert(ctx context.Context, q DBTX, m *domain.LabelMapping) error {
	// No UNIQUE(user_id, label_key); update the existing row if present, else
	// insert. Learned mappings are treated as one per key.
	res, err := q.ExecContext(ctx,
		`UPDATE label_mapping SET label = ?, category_id = ?, account_id = ?, usage_count = usage_count + 1, last_used_at = ?
		 WHERE user_id = ? AND label_key = ?`,
		m.Label, nullInt64(m.CategoryID), nullInt64(m.AccountID), formatTime(nowUTC()), m.UserID, m.LabelKey)
	if err != nil {
		return fmt.Errorf("repo: update label: %w", err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	if _, err := q.ExecContext(ctx,
		`INSERT INTO label_mapping (user_id, label, label_key, category_id, account_id, usage_count, last_used_at)
		 VALUES (?, ?, ?, ?, ?, 1, ?)`,
		m.UserID, m.Label, m.LabelKey, nullInt64(m.CategoryID), nullInt64(m.AccountID), formatTime(nowUTC())); err != nil {
		return fmt.Errorf("repo: insert label: %w", err)
	}
	return nil
}

func scanLabel(row rowScanner) (*domain.LabelMapping, error) {
	var (
		m                     domain.LabelMapping
		categoryID, accountID sql.NullInt64
		lastUsed              string
	)
	err := row.Scan(&m.ID, &m.UserID, &m.Label, &m.LabelKey, &categoryID, &accountID, &m.UsageCount, &lastUsed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("repo: scan label: %w", err)
	}
	m.CategoryID = ptrInt64(categoryID)
	m.AccountID = ptrInt64(accountID)
	if m.LastUsedAt, err = parseTime(lastUsed); err != nil {
		return nil, err
	}
	return &m, nil
}

type uiPreferenceRepo struct{}

func (uiPreferenceRepo) Upsert(ctx context.Context, q DBTX, p *domain.UIPreference) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO ui_preference (user_id, node_type, node_id, expanded, updated_at) VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(user_id, node_type, node_id) DO UPDATE SET expanded = excluded.expanded, updated_at = excluded.updated_at`,
		p.UserID, string(p.NodeType), p.NodeID, boolToInt(p.Expanded), formatTime(nowUTC()))
	if err != nil {
		return fmt.Errorf("repo: upsert ui_preference: %w", err)
	}
	return nil
}

func (uiPreferenceRepo) ListByUser(ctx context.Context, q DBTX, userID int64) ([]domain.UIPreference, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, user_id, node_type, node_id, expanded, updated_at FROM ui_preference WHERE user_id = ? ORDER BY id`, userID)
	if err != nil {
		return nil, fmt.Errorf("repo: list ui_preferences: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []domain.UIPreference
	for rows.Next() {
		var (
			p         domain.UIPreference
			expanded  int
			updatedAt string
		)
		if err := rows.Scan(&p.ID, &p.UserID, &p.NodeType, &p.NodeID, &expanded, &updatedAt); err != nil {
			return nil, fmt.Errorf("repo: scan ui_preference: %w", err)
		}
		p.Expanded = expanded != 0
		if p.UpdatedAt, err = parseTime(updatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

type invitationRepo struct{}

func (invitationRepo) Create(ctx context.Context, q DBTX, inv *domain.Invitation) (int64, error) {
	res, err := q.ExecContext(ctx,
		`INSERT INTO invitation (email, token_hash, invited_is_admin, created_by, expires_at, consumed_at, revoked_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		nullString(inv.Email), inv.TokenHash, boolToInt(inv.InvitedIsAdmin), inv.CreatedBy,
		formatTime(inv.ExpiresAt), formatNullTime(inv.ConsumedAt), formatNullTime(inv.RevokedAt))
	if err != nil {
		if isUniqueViolation(err) {
			return 0, domain.ErrDuplicate
		}
		return 0, fmt.Errorf("repo: create invitation: %w", err)
	}
	return res.LastInsertId()
}

func (invitationRepo) ByTokenHash(ctx context.Context, q DBTX, tokenHash string) (*domain.Invitation, error) {
	return scanInvitation(q.QueryRowContext(ctx,
		`SELECT id, email, token_hash, invited_is_admin, created_by, expires_at, consumed_at, revoked_at
		 FROM invitation WHERE token_hash = ?`, tokenHash))
}

func (invitationRepo) ByID(ctx context.Context, q DBTX, id int64) (*domain.Invitation, error) {
	return scanInvitation(q.QueryRowContext(ctx,
		`SELECT id, email, token_hash, invited_is_admin, created_by, expires_at, consumed_at, revoked_at
		 FROM invitation WHERE id = ?`, id))
}

func (invitationRepo) Update(ctx context.Context, q DBTX, inv *domain.Invitation) error {
	_, err := q.ExecContext(ctx,
		`UPDATE invitation SET consumed_at = ?, revoked_at = ? WHERE id = ?`,
		formatNullTime(inv.ConsumedAt), formatNullTime(inv.RevokedAt), inv.ID)
	if err != nil {
		return fmt.Errorf("repo: update invitation: %w", err)
	}
	return nil
}

func (invitationRepo) ListByCreator(ctx context.Context, q DBTX, createdBy int64) ([]domain.Invitation, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, email, token_hash, invited_is_admin, created_by, expires_at, consumed_at, revoked_at
		 FROM invitation WHERE created_by = ? ORDER BY id`, createdBy)
	if err != nil {
		return nil, fmt.Errorf("repo: list invitations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []domain.Invitation
	for rows.Next() {
		inv, err := scanInvitation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *inv)
	}
	return out, rows.Err()
}

func scanInvitation(row rowScanner) (*domain.Invitation, error) {
	var (
		inv                   domain.Invitation
		email                 sql.NullString
		isAdmin               int
		expiresAt             string
		consumedAt, revokedAt sql.NullString
	)
	err := row.Scan(&inv.ID, &email, &inv.TokenHash, &isAdmin, &inv.CreatedBy, &expiresAt, &consumedAt, &revokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("repo: scan invitation: %w", err)
	}
	inv.Email = ptrString(email)
	inv.InvitedIsAdmin = isAdmin != 0
	if inv.ExpiresAt, err = parseTime(expiresAt); err != nil {
		return nil, err
	}
	if inv.ConsumedAt, err = parseNullTime(consumedAt); err != nil {
		return nil, err
	}
	if inv.RevokedAt, err = parseNullTime(revokedAt); err != nil {
		return nil, err
	}
	return &inv, nil
}

type totpBackupRepo struct{}

func (totpBackupRepo) Create(ctx context.Context, q DBTX, c *domain.TOTPBackupCode) (int64, error) {
	res, err := q.ExecContext(ctx,
		`INSERT INTO totp_backup_code (user_id, code_hash, consumed_at) VALUES (?, ?, ?)`,
		c.UserID, c.CodeHash, formatNullTime(c.ConsumedAt))
	if err != nil {
		return 0, fmt.Errorf("repo: create totp backup: %w", err)
	}
	return res.LastInsertId()
}

func (totpBackupRepo) ListByUser(ctx context.Context, q DBTX, userID int64) ([]domain.TOTPBackupCode, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, user_id, code_hash, consumed_at FROM totp_backup_code WHERE user_id = ? ORDER BY id`, userID)
	if err != nil {
		return nil, fmt.Errorf("repo: list totp backups: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []domain.TOTPBackupCode
	for rows.Next() {
		var (
			c          domain.TOTPBackupCode
			consumedAt sql.NullString
		)
		if err := rows.Scan(&c.ID, &c.UserID, &c.CodeHash, &consumedAt); err != nil {
			return nil, fmt.Errorf("repo: scan totp backup: %w", err)
		}
		if c.ConsumedAt, err = parseNullTime(consumedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (totpBackupRepo) MarkConsumed(ctx context.Context, q DBTX, userID, id int64) error {
	res, err := q.ExecContext(ctx,
		`UPDATE totp_backup_code SET consumed_at = ? WHERE user_id = ? AND id = ? AND consumed_at IS NULL`,
		formatTime(nowUTC()), userID, id)
	if err != nil {
		return fmt.Errorf("repo: consume totp backup: %w", err)
	}
	return notFoundIfNoRows(res)
}

func (totpBackupRepo) DeleteByUser(ctx context.Context, q DBTX, userID int64) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM totp_backup_code WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("repo: delete totp backups: %w", err)
	}
	return nil
}

// escapeLike escapes LIKE wildcards in a user-supplied prefix.
func escapeLike(s string) string {
	r := make([]rune, 0, len(s))
	for _, c := range s {
		if c == '%' || c == '_' || c == '\\' {
			r = append(r, '\\')
		}
		r = append(r, c)
	}
	return string(r)
}
