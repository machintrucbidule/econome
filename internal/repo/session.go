package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"econome/internal/domain"
)

type sessionRepo struct{}

const selectSession = `SELECT id, user_id, token_hash, kind, expires_at, created_at,
	last_seen_at, user_agent, ip FROM session`

func (sessionRepo) Create(ctx context.Context, q DBTX, s *domain.Session) (int64, error) {
	res, err := q.ExecContext(ctx,
		`INSERT INTO session (user_id, token_hash, kind, expires_at, created_at, last_seen_at, user_agent, ip)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		s.UserID, s.TokenHash, string(s.Kind), formatTime(s.ExpiresAt), formatTime(s.CreatedAt),
		formatTime(s.LastSeenAt), nullString(s.UserAgent), nullString(s.IP))
	if err != nil {
		if isUniqueViolation(err) {
			return 0, domain.ErrDuplicate
		}
		return 0, fmt.Errorf("repo: create session: %w", err)
	}
	return res.LastInsertId()
}

func (sessionRepo) GetByTokenHash(ctx context.Context, q DBTX, tokenHash string) (*domain.Session, error) {
	return scanSession(q.QueryRowContext(ctx, selectSession+` WHERE token_hash = ?`, tokenHash))
}

func (sessionRepo) ListByUser(ctx context.Context, q DBTX, userID int64) ([]domain.Session, error) {
	rows, err := q.QueryContext(ctx, selectSession+` WHERE user_id = ? ORDER BY last_seen_at DESC, id DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("repo: list sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []domain.Session
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

func (sessionRepo) Touch(ctx context.Context, q DBTX, id int64, lastSeen, expires time.Time) error {
	_, err := q.ExecContext(ctx,
		`UPDATE session SET last_seen_at = ?, expires_at = ? WHERE id = ?`,
		formatTime(lastSeen), formatTime(expires), id)
	if err != nil {
		return fmt.Errorf("repo: touch session: %w", err)
	}
	return nil
}

func (sessionRepo) Delete(ctx context.Context, q DBTX, id int64) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM session WHERE id = ?`, id); err != nil {
		return fmt.Errorf("repo: delete session: %w", err)
	}
	return nil
}

func (sessionRepo) DeleteByUserScoped(ctx context.Context, q DBTX, userID, id int64) error {
	res, err := q.ExecContext(ctx, `DELETE FROM session WHERE user_id = ? AND id = ?`, userID, id)
	if err != nil {
		return fmt.Errorf("repo: delete session scoped: %w", err)
	}
	return notFoundIfNoRows(res)
}

func (sessionRepo) DeleteByUser(ctx context.Context, q DBTX, userID int64) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM session WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("repo: delete sessions by user: %w", err)
	}
	return nil
}

func (sessionRepo) DeleteByUserExcept(ctx context.Context, q DBTX, userID, keepID int64) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM session WHERE user_id = ? AND id <> ?`, userID, keepID); err != nil {
		return fmt.Errorf("repo: delete sessions except: %w", err)
	}
	return nil
}

func scanSession(row rowScanner) (*domain.Session, error) {
	var (
		s                              domain.Session
		expiresAt, createdAt, lastSeen string
		userAgent, ip                  sql.NullString
	)
	err := row.Scan(&s.ID, &s.UserID, &s.TokenHash, &s.Kind, &expiresAt, &createdAt, &lastSeen, &userAgent, &ip)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("repo: scan session: %w", err)
	}
	s.UserAgent = ptrString(userAgent)
	s.IP = ptrString(ip)
	if s.ExpiresAt, err = parseTime(expiresAt); err != nil {
		return nil, fmt.Errorf("repo: parse expires_at: %w", err)
	}
	if s.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, fmt.Errorf("repo: parse created_at: %w", err)
	}
	if s.LastSeenAt, err = parseTime(lastSeen); err != nil {
		return nil, fmt.Errorf("repo: parse last_seen_at: %w", err)
	}
	return &s, nil
}
