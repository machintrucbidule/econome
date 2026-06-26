package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"econome/internal/domain"
)

type userRepo struct{}

const selectUser = `SELECT id, email, password_hash, is_admin, status, language, currency,
	totp_enabled, totp_secret, must_change_password, failed_login_count,
	last_failed_login_at, locked_until, created_at, updated_at FROM user`

func (userRepo) CountUsers(ctx context.Context, q DBTX) (int, error) {
	var n int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM user`).Scan(&n); err != nil {
		return 0, fmt.Errorf("repo: count users: %w", err)
	}
	return n, nil
}

func (userRepo) GetByEmail(ctx context.Context, q DBTX, email string) (*domain.User, error) {
	return scanUser(q.QueryRowContext(ctx, selectUser+` WHERE email = ?`, email))
}

func (userRepo) GetByID(ctx context.Context, q DBTX, id int64) (*domain.User, error) {
	return scanUser(q.QueryRowContext(ctx, selectUser+` WHERE id = ?`, id))
}

func (userRepo) Create(ctx context.Context, q DBTX, u *domain.User) (int64, error) {
	res, err := q.ExecContext(ctx,
		`INSERT INTO user (email, password_hash, is_admin, status, language, currency,
			totp_enabled, totp_secret, must_change_password, failed_login_count,
			last_failed_login_at, locked_until, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.Email, u.PasswordHash, boolToInt(u.IsAdmin), string(u.Status), string(u.Language), u.Currency,
		boolToInt(u.TOTPEnabled), nullString(u.TOTPSecret), boolToInt(u.MustChangePassword), u.FailedLoginCount,
		formatNullTime(u.LastFailedLoginAt), formatNullTime(u.LockedUntil), formatTime(u.CreatedAt), formatTime(u.UpdatedAt))
	if err != nil {
		if isUniqueViolation(err) {
			return 0, domain.ErrDuplicate
		}
		return 0, fmt.Errorf("repo: create user: %w", err)
	}
	return res.LastInsertId()
}

func (userRepo) UpdateLoginState(ctx context.Context, q DBTX, u *domain.User) error {
	_, err := q.ExecContext(ctx,
		`UPDATE user SET failed_login_count = ?, last_failed_login_at = ?, locked_until = ?, updated_at = ?
		 WHERE id = ?`,
		u.FailedLoginCount, formatNullTime(u.LastFailedLoginAt), formatNullTime(u.LockedUntil), formatTime(u.UpdatedAt), u.ID)
	if err != nil {
		return fmt.Errorf("repo: update login state: %w", err)
	}
	return nil
}

func (userRepo) UpdatePasswordHash(ctx context.Context, q DBTX, id int64, hash string) error {
	_, err := q.ExecContext(ctx,
		`UPDATE user SET password_hash = ?, updated_at = ? WHERE id = ?`,
		hash, formatTime(nowUTC()), id)
	if err != nil {
		return fmt.Errorf("repo: update password hash: %w", err)
	}
	return nil
}

func scanUser(row rowScanner) (*domain.User, error) {
	var (
		u                                   domain.User
		isAdmin, totpEnabled, mustChange    int
		totpSecret, lastFailed, lockedUntil sql.NullString
		createdAt, updatedAt                string
	)
	err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &isAdmin, &u.Status, &u.Language, &u.Currency,
		&totpEnabled, &totpSecret, &mustChange, &u.FailedLoginCount, &lastFailed, &lockedUntil, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("repo: scan user: %w", err)
	}

	u.IsAdmin = isAdmin != 0
	u.TOTPEnabled = totpEnabled != 0
	u.MustChangePassword = mustChange != 0
	u.TOTPSecret = ptrString(totpSecret)
	if u.LastFailedLoginAt, err = parseNullTime(lastFailed); err != nil {
		return nil, fmt.Errorf("repo: parse last_failed_login_at: %w", err)
	}
	if u.LockedUntil, err = parseNullTime(lockedUntil); err != nil {
		return nil, fmt.Errorf("repo: parse locked_until: %w", err)
	}
	if u.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, fmt.Errorf("repo: parse created_at: %w", err)
	}
	if u.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return nil, fmt.Errorf("repo: parse updated_at: %w", err)
	}
	return &u, nil
}
