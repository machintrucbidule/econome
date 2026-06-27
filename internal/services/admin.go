package services

import (
	"context"
	"errors"
	"time"

	"econome/internal/auth"
	"econome/internal/domain"
	"econome/internal/repo"
)

// Admin use-cases (functional/01 §4/§8, technical/05 §7/§8): invitations
// (issue/revoke/accept) and user management (list/deactivate/reactivate/reset).
// The last-admin rule is enforced here so both the UI and the CLI share it.
// Admin endpoints operate on user *accounts*, never on another user's financial
// data; transport gates admin-only access (non-admin ⇒ 404, never 403).

// invitationTTL is the single-use link's validity (functional/01 §1.3).
const invitationTTL = 7 * 24 * time.Hour

// ErrLastAdmin is returned when an action would remove the last active admin.
var ErrLastAdmin = errors.New("services: last admin")

// IssuedInvitation is a freshly created invitation plus the one-time raw token
// (shown to the admin once; only its hash is stored).
type IssuedInvitation struct {
	Invitation domain.Invitation
	RawToken   string
}

// IssueInvitation creates a single-use invitation valid 7 days, returning the raw
// token for the one-time link. email is optional (the invitee may set their own).
func (s *Service) IssueInvitation(ctx context.Context, adminID int64, email *string, invitedAdmin bool) (*IssuedInvitation, error) {
	raw, err := auth.GenerateSessionToken()
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	if email != nil {
		e := normaliseEmail(*email)
		if e == "" {
			email = nil
		} else {
			email = &e
		}
	}
	inv := &domain.Invitation{
		Email: email, TokenHash: auth.HashToken(raw), InvitedIsAdmin: invitedAdmin,
		CreatedBy: adminID, ExpiresAt: now.Add(invitationTTL),
	}
	id, err := s.invitations.Create(ctx, s.tx.DB(), inv)
	if err != nil {
		return nil, err
	}
	inv.ID = id
	return &IssuedInvitation{Invitation: *inv, RawToken: raw}, nil
}

// ListInvitations returns the invitations the admin issued (newest handling left
// to the view). Pending vs consumed/revoked/expired is derived per row.
func (s *Service) ListInvitations(ctx context.Context, adminID int64) ([]domain.Invitation, error) {
	return s.invitations.ListByCreator(ctx, s.tx.DB(), adminID)
}

// RevokeInvitation invalidates a pending invitation the admin issued (scoped to
// the issuer; another admin's invitation is a 404).
func (s *Service) RevokeInvitation(ctx context.Context, adminID, invID int64) error {
	return s.tx.WithTx(ctx, func(q repo.DBTX) error {
		inv, err := s.invitations.ByID(ctx, q, invID)
		if err != nil {
			return err
		}
		if inv.CreatedBy != adminID {
			return domain.ErrNotFound // not the issuer — never reveal another tenant's row
		}
		if inv.ConsumedAt != nil || inv.RevokedAt != nil {
			return domain.ErrConflict
		}
		now := s.now().UTC()
		inv.RevokedAt = &now
		return s.invitations.Update(ctx, q, inv)
	})
}

// invitationValid reports whether an invitation is currently acceptable.
func (s *Service) invitationValid(inv *domain.Invitation, now time.Time) bool {
	return inv.ConsumedAt == nil && inv.RevokedAt == nil && now.Before(inv.ExpiresAt)
}

// CheckInvitation validates a raw token for the acceptance screen, returning the
// invitation (so the form can pre-fill the email) or ErrNotFound if invalid.
func (s *Service) CheckInvitation(ctx context.Context, rawToken string) (*domain.Invitation, error) {
	inv, err := s.invitations.ByTokenHash(ctx, s.tx.DB(), auth.HashToken(rawToken))
	if err != nil {
		return nil, err
	}
	if !s.invitationValid(inv, s.now().UTC()) {
		return nil, domain.ErrNotFound
	}
	return inv, nil
}

// AcceptInput is the invitation-acceptance form.
type AcceptInput struct {
	Email, Password, PasswordConfirm string
	Language, Currency               string
}

// AcceptInvitation creates the invited user (with the invited role) + its
// settings, consumes the token, and opens a session — all atomically
// (functional/01 §4.2, technical/05 §7). An invalid/expired token is ErrNotFound;
// a duplicate email is a typed 422.
func (s *Service) AcceptInvitation(ctx context.Context, rawToken string, in AcceptInput) (*AuthResult, error) {
	if err := s.validateAccept(in); err != nil {
		return nil, err
	}
	now := s.now().UTC()
	lang := normaliseLang(in.Language)
	currency := normaliseCurrency(in.Currency)
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return nil, err
	}
	user := &domain.User{
		Email: normaliseEmail(in.Email), PasswordHash: hash, Status: domain.StatusActive,
		Language: lang, Currency: currency, CreatedAt: now, UpdatedAt: now,
	}
	err = s.tx.WithTx(ctx, func(q repo.DBTX) error {
		inv, err := s.invitations.ByTokenHash(ctx, q, auth.HashToken(rawToken))
		if err != nil {
			return err
		}
		if !s.invitationValid(inv, now) {
			return domain.ErrNotFound
		}
		user.IsAdmin = inv.InvitedIsAdmin
		id, err := s.users.Create(ctx, q, user)
		if err != nil {
			return err // ErrDuplicate ⇒ email collision
		}
		user.ID = id
		settings := defaultSettings(lang, currency, now)
		settings.UserID = id
		if err := s.settings.Create(ctx, q, settings); err != nil {
			return err
		}
		inv.ConsumedAt = &now
		return s.invitations.Update(ctx, q, inv)
	})
	if err != nil {
		if errors.Is(err, domain.ErrDuplicate) {
			v := &domain.ValidationError{}
			v.Add("email", domain.MsgEmailDuplicate)
			return nil, v
		}
		return nil, err
	}
	return s.issueSession(ctx, user, false)
}

func (s *Service) validateAccept(in AcceptInput) error {
	v := &domain.ValidationError{}
	mergeValidation(v, domain.ValidateEmail(in.Email))
	mergeValidation(v, domain.ValidatePassword(in.Password))
	if in.Password != in.PasswordConfirm {
		v.Add("password_confirm", domain.MsgPasswordMismatch)
	}
	return v.OrNil()
}

// --- user management (functional/01 §8) ---

// ListUsers returns every account for the admin users panel.
func (s *Service) ListUsers(ctx context.Context) ([]domain.User, error) {
	return s.users.ListAll(ctx, s.tx.DB())
}

// DeactivateUser disables an account (cannot log in; sessions revoked; data
// retained). The last active admin cannot be deactivated (ErrLastAdmin → 409).
func (s *Service) DeactivateUser(ctx context.Context, targetID int64) error {
	return s.tx.WithTx(ctx, func(q repo.DBTX) error {
		u, err := s.users.GetByID(ctx, q, targetID)
		if err != nil {
			return err
		}
		if u.IsAdmin {
			if err := s.ensureNotLastAdmin(ctx, q); err != nil {
				return err
			}
		}
		if err := s.users.UpdateStatus(ctx, q, targetID, domain.StatusDeactivated); err != nil {
			return err
		}
		return s.sessions.DeleteByUser(ctx, q, targetID)
	})
}

// ReactivateUser re-enables a deactivated account.
func (s *Service) ReactivateUser(ctx context.Context, targetID int64) error {
	return s.tx.WithTx(ctx, func(q repo.DBTX) error {
		if _, err := s.users.GetByID(ctx, q, targetID); err != nil {
			return err
		}
		return s.users.UpdateStatus(ctx, q, targetID, domain.StatusActive)
	})
}

// AdminResetTOTP disables a user's 2FA so they can log in and re-enrol
// (technical/05 §5 recovery). Clears the secret and backup codes.
func (s *Service) AdminResetTOTP(ctx context.Context, targetID int64) error {
	return s.tx.WithTx(ctx, func(q repo.DBTX) error {
		if _, err := s.users.GetByID(ctx, q, targetID); err != nil {
			return err
		}
		if err := s.users.UpdateTOTP(ctx, q, targetID, false, nil); err != nil {
			return err
		}
		return s.totpBackups.DeleteByUser(ctx, q, targetID)
	})
}

// AdminResetPassword sets a temporary password (policy-checked) forcing a change
// on next login, and revokes the user's sessions (technical/05 §8). Shared by the
// UI and the CLI.
func (s *Service) AdminResetPassword(ctx context.Context, targetID int64, temp string) error {
	if v := domain.ValidatePassword(temp); v != nil {
		return v
	}
	hash, err := auth.HashPassword(temp)
	if err != nil {
		return err
	}
	return s.tx.WithTx(ctx, func(q repo.DBTX) error {
		if _, err := s.users.GetByID(ctx, q, targetID); err != nil {
			return err
		}
		if err := s.users.SetPassword(ctx, q, targetID, hash, true); err != nil {
			return err
		}
		return s.sessions.DeleteByUser(ctx, q, targetID)
	})
}

// SetAdmin promotes/demotes a user; demoting the last admin is refused.
func (s *Service) SetAdmin(ctx context.Context, targetID int64, isAdmin bool) error {
	return s.tx.WithTx(ctx, func(q repo.DBTX) error {
		u, err := s.users.GetByID(ctx, q, targetID)
		if err != nil {
			return err
		}
		if u.IsAdmin && !isAdmin {
			if err := s.ensureNotLastAdmin(ctx, q); err != nil {
				return err
			}
		}
		return s.users.SetAdmin(ctx, q, targetID, isAdmin)
	})
}

// FindUserByEmail resolves a user by email for the admin CLI recovery paths
// (technical/05 §8). ErrNotFound if no such account.
func (s *Service) FindUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	return s.users.GetByEmail(ctx, s.tx.DB(), normaliseEmail(email))
}

// GenerateTempPassword returns a fresh policy-compliant temporary password for
// an admin/CLI reset (shown once to the operator).
func (s *Service) GenerateTempPassword() (string, error) {
	return auth.RandomPassword()
}

// ensureNotLastAdmin refuses an action when only one active admin remains.
func (s *Service) ensureNotLastAdmin(ctx context.Context, q repo.DBTX) error {
	n, err := s.users.CountActiveAdmins(ctx, q)
	if err != nil {
		return err
	}
	if n <= 1 {
		return ErrLastAdmin
	}
	return nil
}
