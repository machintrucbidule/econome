package services

import (
	"context"
	"errors"

	"econome/internal/auth"
	"econome/internal/domain"
	"econome/internal/repo"
)

// Self-service security use-cases (functional/01 §5–§7, technical/05): TOTP
// two-factor (enrol → confirm → backup codes → disable/regenerate), the inline
// 2FA login completion, the password change, and active-session management.
// Every method is scoped to the authenticated user_id.

// ErrTOTPRequired signals that a TOTP/backup code is needed but missing/invalid.
var ErrTOTPRequired = errors.New("services: totp required")

// TOTPEnrolment is the pending secret + the otpauth URL the handler renders as a
// QR code. The secret is stored on the user with totp_enabled still false until
// ConfirmTOTP verifies a live code.
type TOTPEnrolment struct {
	Secret     string
	OTPAuthURL string
}

// BeginTOTPEnrolment generates a fresh secret, stores it (disabled), and returns
// it for the QR display. Re-running before confirmation rotates the secret. It is
// refused if 2FA is already enabled (disable first).
func (s *Service) BeginTOTPEnrolment(ctx context.Context, userID int64) (*TOTPEnrolment, error) {
	u, err := s.users.GetByID(ctx, s.tx.DB(), userID)
	if err != nil {
		return nil, err
	}
	if u.TOTPEnabled {
		return nil, domain.ErrConflict
	}
	secret, url, err := auth.NewTOTPSecret(u.Email)
	if err != nil {
		return nil, err
	}
	if err := s.users.UpdateTOTP(ctx, s.tx.DB(), userID, false, &secret); err != nil {
		return nil, err
	}
	return &TOTPEnrolment{Secret: secret, OTPAuthURL: url}, nil
}

// CurrentTOTPEnrolment rebuilds the QR for the pending (not-yet-enabled) secret
// without rotating it — used to re-render the enrol modal after a wrong code so
// the user's already-scanned secret stays valid.
func (s *Service) CurrentTOTPEnrolment(ctx context.Context, userID int64) (*TOTPEnrolment, error) {
	u, err := s.users.GetByID(ctx, s.tx.DB(), userID)
	if err != nil {
		return nil, err
	}
	if u.TOTPEnabled || u.TOTPSecret == nil {
		return nil, domain.ErrConflict
	}
	url, err := auth.TOTPURLFromSecret(*u.TOTPSecret, u.Email)
	if err != nil {
		return nil, err
	}
	return &TOTPEnrolment{Secret: *u.TOTPSecret, OTPAuthURL: url}, nil
}

// ConfirmTOTP verifies a live code against the pending secret, enables 2FA, and
// issues a fresh set of single-use backup codes (returned once, stored hashed).
// A wrong code is a typed 422; no secret pending is a 409.
func (s *Service) ConfirmTOTP(ctx context.Context, userID int64, code string) ([]string, error) {
	var codes []string
	err := s.tx.WithTx(ctx, func(q repo.DBTX) error {
		u, err := s.users.GetByID(ctx, q, userID)
		if err != nil {
			return err
		}
		if u.TOTPEnabled || u.TOTPSecret == nil {
			return domain.ErrConflict
		}
		if !auth.VerifyTOTP(*u.TOTPSecret, code, s.now().UTC()) {
			v := &domain.ValidationError{}
			v.Add("code", domain.MsgTOTPInvalid)
			return v
		}
		if err := s.users.UpdateTOTP(ctx, q, userID, true, u.TOTPSecret); err != nil {
			return err
		}
		codes, err = s.issueBackupCodes(ctx, q, userID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return codes, nil
}

// DisableTOTP turns 2FA off after re-authenticating with the password (and a
// current code if one is supplied, technical/05 §5). It clears the secret and all
// backup codes.
func (s *Service) DisableTOTP(ctx context.Context, userID int64, password, code string) error {
	return s.tx.WithTx(ctx, func(q repo.DBTX) error {
		u, err := s.users.GetByID(ctx, q, userID)
		if err != nil {
			return err
		}
		if !u.TOTPEnabled {
			return domain.ErrConflict
		}
		if ok, _, _ := auth.VerifyPassword(u.PasswordHash, password); !ok {
			v := &domain.ValidationError{}
			v.Add("password", domain.MsgPasswordWrong)
			return v
		}
		// If a code is supplied it must be valid; an empty code is accepted (the
		// password already re-authenticated the user).
		if code != "" && u.TOTPSecret != nil && !auth.VerifyTOTP(*u.TOTPSecret, code, s.now().UTC()) {
			v := &domain.ValidationError{}
			v.Add("code", domain.MsgTOTPInvalid)
			return v
		}
		if err := s.users.UpdateTOTP(ctx, q, userID, false, nil); err != nil {
			return err
		}
		return s.totpBackups.DeleteByUser(ctx, q, userID)
	})
}

// RegenerateBackupCodes invalidates the old set and issues a new one (shown once).
func (s *Service) RegenerateBackupCodes(ctx context.Context, userID int64) ([]string, error) {
	var codes []string
	err := s.tx.WithTx(ctx, func(q repo.DBTX) error {
		u, err := s.users.GetByID(ctx, q, userID)
		if err != nil {
			return err
		}
		if !u.TOTPEnabled {
			return domain.ErrConflict
		}
		if err := s.totpBackups.DeleteByUser(ctx, q, userID); err != nil {
			return err
		}
		codes, err = s.issueBackupCodes(ctx, q, userID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return codes, nil
}

// BackupCodesRemaining counts the user's unconsumed backup codes.
func (s *Service) BackupCodesRemaining(ctx context.Context, userID int64) (int, error) {
	all, err := s.totpBackups.ListByUser(ctx, s.tx.DB(), userID)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, c := range all {
		if c.ConsumedAt == nil {
			n++
		}
	}
	return n, nil
}

func (s *Service) issueBackupCodes(ctx context.Context, q repo.DBTX, userID int64) ([]string, error) {
	codes, err := auth.GenerateBackupCodes()
	if err != nil {
		return nil, err
	}
	for _, c := range codes {
		h, err := auth.HashBackupCode(c)
		if err != nil {
			return nil, err
		}
		if _, err := s.totpBackups.Create(ctx, q, &domain.TOTPBackupCode{UserID: userID, CodeHash: h}); err != nil {
			return nil, err
		}
	}
	return codes, nil
}

// CompleteTOTPLogin verifies the inline 2FA step: it validates the pending token,
// loads the user, accepts a current TOTP code OR an unconsumed backup code (which
// it then consumes), and opens the session (functional/01 §3). A bad/expired
// pending token or a wrong code yields ErrTOTPRequired (no enumeration of which).
func (s *Service) CompleteTOTPLogin(ctx context.Context, pending, code, ip string) (*AuthResult, error) {
	now := s.now().UTC()
	// The 2FA step is the second half of the login flow: apply the same per-IP
	// throttle so the 6-digit code cannot be brute-forced within the pending
	// window (technical/05 §6).
	if !s.throttle.Allow(ip, now) {
		return nil, ErrTOTPRequired
	}
	userID, remember, ok := auth.VerifyPending(s.secret, pending, now)
	if !ok {
		return nil, ErrTOTPRequired
	}
	u, err := s.users.GetByID(ctx, s.tx.DB(), userID)
	if err != nil {
		return nil, ErrTOTPRequired
	}
	if u.Status != domain.StatusActive || !u.TOTPEnabled || u.TOTPSecret == nil {
		return nil, ErrTOTPRequired
	}
	if auth.VerifyTOTP(*u.TOTPSecret, code, now) {
		return s.issueSession(ctx, u, remember)
	}
	if s.consumeBackupCode(ctx, userID, code) {
		return s.issueSession(ctx, u, remember)
	}
	return nil, ErrTOTPRequired
}

// --- password & profile (functional/01 §6/§8) ---

// ChangePassword changes the user's own password after re-authenticating with the
// current one, enforcing the policy + confirmation, and clears any
// must_change_password flag (technical/05 §1).
func (s *Service) ChangePassword(ctx context.Context, userID int64, current, next, confirm string) error {
	return s.tx.WithTx(ctx, func(q repo.DBTX) error {
		u, err := s.users.GetByID(ctx, q, userID)
		if err != nil {
			return err
		}
		if ok, _, _ := auth.VerifyPassword(u.PasswordHash, current); !ok {
			v := &domain.ValidationError{}
			v.Add("current_password", domain.MsgPasswordWrong)
			return v
		}
		v := &domain.ValidationError{}
		mergeValidation(v, domain.ValidatePassword(next))
		if next != confirm {
			v.Add("password_confirm", domain.MsgPasswordMismatch)
		}
		if err := v.OrNil(); err != nil {
			return err
		}
		hash, err := auth.HashPassword(next)
		if err != nil {
			return err
		}
		return s.users.SetPassword(ctx, q, userID, hash, false)
	})
}

// ChangeEmail updates the user's email after re-authenticating with the password
// (functional/01 §8). A taken email is a typed 422.
func (s *Service) ChangeEmail(ctx context.Context, userID int64, password, newEmail string) error {
	if v := domain.ValidateEmail(newEmail); v != nil {
		return v
	}
	email := normaliseEmail(newEmail)
	return s.tx.WithTx(ctx, func(q repo.DBTX) error {
		u, err := s.users.GetByID(ctx, q, userID)
		if err != nil {
			return err
		}
		if ok, _, _ := auth.VerifyPassword(u.PasswordHash, password); !ok {
			vv := &domain.ValidationError{}
			vv.Add("password", domain.MsgPasswordWrong)
			return vv
		}
		if err := s.users.UpdateEmail(ctx, q, userID, email); err != nil {
			if errors.Is(err, domain.ErrDuplicate) {
				vv := &domain.ValidationError{}
				vv.Add("email", domain.MsgEmailDuplicate)
				return vv
			}
			return err
		}
		return nil
	})
}

// --- active sessions (functional/01 §7) ---

// SessionView is one row of the active-sessions list with the current marker.
type SessionView struct {
	Session domain.Session
	Current bool
}

// ListSessions returns the user's active sessions, flagging the current one (by
// the cookie's token hash).
func (s *Service) ListSessions(ctx context.Context, userID int64, currentTokenHash string) ([]SessionView, error) {
	rows, err := s.sessions.ListByUser(ctx, s.tx.DB(), userID)
	if err != nil {
		return nil, err
	}
	out := make([]SessionView, 0, len(rows))
	for _, r := range rows {
		out = append(out, SessionView{Session: r, Current: r.TokenHash == currentTokenHash})
	}
	return out, nil
}

// RevokeSession revokes one of the user's sessions (scoped; another user's
// session id is a 404).
func (s *Service) RevokeSession(ctx context.Context, userID, sessionID int64) error {
	return s.sessions.DeleteByUserScoped(ctx, s.tx.DB(), userID, sessionID)
}

// RevokeOtherSessions logs out everywhere except the current session (kept by its
// token hash). If the current session is unknown, all are revoked.
func (s *Service) RevokeOtherSessions(ctx context.Context, userID int64, currentTokenHash string) error {
	rows, err := s.sessions.ListByUser(ctx, s.tx.DB(), userID)
	if err != nil {
		return err
	}
	keep := int64(0)
	for _, r := range rows {
		if r.TokenHash == currentTokenHash {
			keep = r.ID
			break
		}
	}
	return s.sessions.DeleteByUserExcept(ctx, s.tx.DB(), userID, keep)
}

// consumeBackupCode marks the first matching unconsumed backup code consumed and
// reports success.
func (s *Service) consumeBackupCode(ctx context.Context, userID int64, candidate string) bool {
	if candidate == "" {
		return false
	}
	all, err := s.totpBackups.ListByUser(ctx, s.tx.DB(), userID)
	if err != nil {
		return false
	}
	for _, c := range all {
		if c.ConsumedAt != nil {
			continue
		}
		if auth.VerifyBackupCode(c.CodeHash, candidate) {
			return s.totpBackups.MarkConsumed(ctx, s.tx.DB(), userID, c.ID) == nil
		}
	}
	return false
}
