package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"econome/internal/domain"
)

type settingsRepo struct{}

const selectSettings = `SELECT user_id, default_account_id, pea_initial_deposit, pea_social_charge_rate,
	near_cap_threshold, secured_savings_basis, comment_autoprefill, theme, language, currency,
	dsp2_enabled, updated_at FROM settings`

func (settingsRepo) Create(ctx context.Context, q DBTX, s *domain.Settings) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO settings (user_id, default_account_id, pea_initial_deposit, pea_social_charge_rate,
			near_cap_threshold, secured_savings_basis, comment_autoprefill, theme, language, currency,
			dsp2_enabled, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.UserID, nullInt64(s.DefaultAccountID), s.PEAInitialDeposit, s.PEASocialChargeRate,
		s.NearCapThreshold, string(s.SecuredSavingsBasis), boolToInt(s.CommentAutoprefill), string(s.Theme),
		string(s.Language), s.Currency, boolToInt(s.DSP2Enabled), formatTime(s.UpdatedAt))
	if err != nil {
		if isUniqueViolation(err) {
			return domain.ErrDuplicate
		}
		return fmt.Errorf("repo: create settings: %w", err)
	}
	return nil
}

func (settingsRepo) Get(ctx context.Context, q DBTX, userID int64) (*domain.Settings, error) {
	return scanSettings(q.QueryRowContext(ctx, selectSettings+` WHERE user_id = ?`, userID))
}

func (settingsRepo) Update(ctx context.Context, q DBTX, s *domain.Settings) error {
	_, err := q.ExecContext(ctx,
		`UPDATE settings SET default_account_id = ?, pea_initial_deposit = ?, pea_social_charge_rate = ?,
			near_cap_threshold = ?, secured_savings_basis = ?, comment_autoprefill = ?, theme = ?,
			language = ?, currency = ?, dsp2_enabled = ?, updated_at = ?
		 WHERE user_id = ?`,
		nullInt64(s.DefaultAccountID), s.PEAInitialDeposit, s.PEASocialChargeRate, s.NearCapThreshold,
		string(s.SecuredSavingsBasis), boolToInt(s.CommentAutoprefill), string(s.Theme), string(s.Language),
		s.Currency, boolToInt(s.DSP2Enabled), formatTime(s.UpdatedAt), s.UserID)
	if err != nil {
		return fmt.Errorf("repo: update settings: %w", err)
	}
	return nil
}

func scanSettings(row rowScanner) (*domain.Settings, error) {
	var (
		s                domain.Settings
		defaultAccountID sql.NullInt64
		commentPrefill   int
		dsp2Enabled      int
		updatedAt        string
	)
	err := row.Scan(&s.UserID, &defaultAccountID, &s.PEAInitialDeposit, &s.PEASocialChargeRate,
		&s.NearCapThreshold, &s.SecuredSavingsBasis, &commentPrefill, &s.Theme, &s.Language, &s.Currency,
		&dsp2Enabled, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("repo: scan settings: %w", err)
	}
	s.DefaultAccountID = ptrInt64(defaultAccountID)
	s.CommentAutoprefill = commentPrefill != 0
	s.DSP2Enabled = dsp2Enabled != 0
	if s.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return nil, fmt.Errorf("repo: parse updated_at: %w", err)
	}
	return &s, nil
}
