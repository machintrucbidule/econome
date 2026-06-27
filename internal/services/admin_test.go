package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"econome/internal/auth"
	"econome/internal/domain"
)

// Admin / invitation / profile tests (increment 8, functional/01 §4/§6/§8,
// technical/05 §7/§8) — part of the security regression suite.

func TestInvitationSingleUseAndExpiry(t *testing.T) {
	s := newService(t)
	now := fixedClockTime(s)
	ctx := context.Background()
	admin := seedUser(t, s)

	issued, err := s.IssueInvitation(ctx, admin, strp("famille@example.org"), false)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	// Valid → acceptance creates the user.
	res, err := s.AcceptInvitation(ctx, issued.RawToken, AcceptInput{
		Email: "famille@example.org", Password: "An0ther&Str0ng", PasswordConfirm: "An0ther&Str0ng", Language: "fr",
	})
	if err != nil || res.User.ID == 0 || res.User.IsAdmin {
		t.Fatalf("accept: res=%+v err=%v", res, err)
	}
	// Single-use: the token no longer validates.
	if _, err := s.CheckInvitation(ctx, issued.RawToken); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("consumed token check = %v, want ErrNotFound", err)
	}
	if _, err := s.AcceptInvitation(ctx, issued.RawToken, AcceptInput{
		Email: "x@example.org", Password: "An0ther&Str0ng", PasswordConfirm: "An0ther&Str0ng", Language: "fr",
	}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("reuse = %v, want ErrNotFound", err)
	}

	// Expiry: a token 8 days old is invalid.
	exp, _ := s.IssueInvitation(ctx, admin, nil, false)
	s.now = func() time.Time { return now.Add(8 * 24 * time.Hour) }
	if _, err := s.CheckInvitation(ctx, exp.RawToken); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expired token = %v, want ErrNotFound", err)
	}
}

func TestInvitationRevoke(t *testing.T) {
	s := newService(t)
	fixedClockTime(s)
	ctx := context.Background()
	admin := seedUser(t, s)
	issued, _ := s.IssueInvitation(ctx, admin, nil, false)
	if err := s.RevokeInvitation(ctx, admin, issued.Invitation.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := s.CheckInvitation(ctx, issued.RawToken); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("revoked token = %v, want ErrNotFound", err)
	}
	// Re-revoking is a conflict.
	if err := s.RevokeInvitation(ctx, admin, issued.Invitation.ID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("double revoke = %v, want ErrConflict", err)
	}
}

func TestInvitedAdminAndDuplicateEmail(t *testing.T) {
	s := newService(t)
	fixedClockTime(s)
	ctx := context.Background()
	admin := seedUser(t, s)

	// Duplicate email → typed 422.
	dup, _ := s.IssueInvitation(ctx, admin, nil, false)
	if _, err := s.AcceptInvitation(ctx, dup.RawToken, AcceptInput{
		Email: "owner@example.org", Password: "An0ther&Str0ng", PasswordConfirm: "An0ther&Str0ng", Language: "fr",
	}); err == nil {
		t.Fatal("duplicate email should fail")
	} else {
		var ve *domain.ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("want ValidationError, got %v", err)
		}
	}

	// An invited admin is created with the admin role.
	inv, _ := s.IssueInvitation(ctx, admin, nil, true)
	res, err := s.AcceptInvitation(ctx, inv.RawToken, AcceptInput{
		Email: "second-admin@example.org", Password: "An0ther&Str0ng", PasswordConfirm: "An0ther&Str0ng", Language: "fr",
	})
	if err != nil || !res.User.IsAdmin {
		t.Fatalf("invited admin: res=%+v err=%v", res, err)
	}
}

func TestLastAdminCannotBeDeactivated(t *testing.T) {
	s := newService(t)
	fixedClockTime(s)
	ctx := context.Background()
	owner := seedUser(t, s)

	// Sole admin: deactivation is refused.
	if err := s.DeactivateUser(ctx, owner); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("deactivate last admin = %v, want ErrLastAdmin", err)
	}

	// Add a second admin; now the first can be deactivated.
	inv, _ := s.IssueInvitation(ctx, owner, nil, true)
	res, _ := s.AcceptInvitation(ctx, inv.RawToken, AcceptInput{
		Email: "admin2@example.org", Password: "An0ther&Str0ng", PasswordConfirm: "An0ther&Str0ng", Language: "fr",
	})
	if err := s.DeactivateUser(ctx, owner); err != nil {
		t.Fatalf("deactivate with two admins: %v", err)
	}
	// Now admin2 is the last admin → cannot be demoted.
	if err := s.SetAdmin(ctx, res.User.ID, false); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("demote last admin = %v, want ErrLastAdmin", err)
	}
}

func TestAdminResetPasswordForcesChangeAndRevokes(t *testing.T) {
	s := newService(t)
	fixedClockTime(s)
	ctx := context.Background()
	admin := seedUser(t, s)
	inv, _ := s.IssueInvitation(ctx, admin, nil, false)
	res, _ := s.AcceptInvitation(ctx, inv.RawToken, AcceptInput{
		Email: "member@example.org", Password: "An0ther&Str0ng", PasswordConfirm: "An0ther&Str0ng", Language: "fr",
	})
	memberID := res.User.ID
	// The member has an open session (from acceptance).
	if got, _ := s.sessions.ListByUser(ctx, s.tx.DB(), memberID); len(got) == 0 {
		t.Fatal("expected an open session after acceptance")
	}

	temp, _ := s.GenerateTempPassword()
	if err := s.AdminResetPassword(ctx, memberID, temp); err != nil {
		t.Fatalf("reset password: %v", err)
	}
	u, _ := s.users.GetByID(ctx, s.tx.DB(), memberID)
	if !u.MustChangePassword {
		t.Error("reset should set must_change_password")
	}
	if ok, _, _ := auth.VerifyPassword(u.PasswordHash, temp); !ok {
		t.Error("temp password should verify")
	}
	if got, _ := s.sessions.ListByUser(ctx, s.tx.DB(), memberID); len(got) != 0 {
		t.Errorf("sessions should be revoked on reset, got %d", len(got))
	}
}

func TestChangePasswordAndEmail(t *testing.T) {
	s := newService(t)
	fixedClockTime(s)
	ctx := context.Background()
	uid := seedUser(t, s)

	// Wrong current password is rejected.
	if err := s.ChangePassword(ctx, uid, "wrong", "Br4nd&New1", "Br4nd&New1"); err == nil {
		t.Fatal("wrong current password should fail")
	}
	// Mismatched confirmation is rejected.
	if err := s.ChangePassword(ctx, uid, goodPassword, "Br4nd&New1", "different"); err == nil {
		t.Fatal("mismatch should fail")
	}
	// Valid change.
	if err := s.ChangePassword(ctx, uid, goodPassword, "Br4nd&New1", "Br4nd&New1"); err != nil {
		t.Fatalf("change password: %v", err)
	}
	u, _ := s.users.GetByID(ctx, s.tx.DB(), uid)
	if ok, _, _ := auth.VerifyPassword(u.PasswordHash, "Br4nd&New1"); !ok {
		t.Error("new password should verify")
	}

	// Email change requires the (new) password and rejects a taken address.
	if err := s.ChangeEmail(ctx, uid, "Br4nd&New1", "fresh@example.org"); err != nil {
		t.Fatalf("change email: %v", err)
	}
	u, _ = s.users.GetByID(ctx, s.tx.DB(), uid)
	if u.Email != "fresh@example.org" {
		t.Errorf("email = %q, want fresh@example.org", u.Email)
	}
}

func TestRevokeOtherSessionsKeepsCurrent(t *testing.T) {
	s := newService(t)
	fixedClockTime(s)
	ctx := context.Background()
	uid := seedUser(t, s)
	// Open three sessions for the user.
	r1, _ := s.issueSession(ctx, &domain.User{ID: uid}, false)
	_, _ = s.issueSession(ctx, &domain.User{ID: uid}, false)
	_, _ = s.issueSession(ctx, &domain.User{ID: uid}, true)

	if err := s.RevokeOtherSessions(ctx, uid, auth.HashToken(r1.Token)); err != nil {
		t.Fatalf("revoke others: %v", err)
	}
	got, _ := s.sessions.ListByUser(ctx, s.tx.DB(), uid)
	if len(got) != 1 || got[0].TokenHash != auth.HashToken(r1.Token) {
		t.Fatalf("expected only the current session to survive, got %d", len(got))
	}
}

func TestDeactivatedUserCannotLogIn(t *testing.T) {
	s := newService(t)
	fixedClockTime(s)
	ctx := context.Background()
	admin := seedUser(t, s)
	inv, _ := s.IssueInvitation(ctx, admin, nil, false)
	res, _ := s.AcceptInvitation(ctx, inv.RawToken, AcceptInput{
		Email: "member@example.org", Password: "An0ther&Str0ng", PasswordConfirm: "An0ther&Str0ng", Language: "fr",
	})
	// Active member logs in fine.
	if _, err := s.Login(ctx, LoginInput{Email: "member@example.org", Password: "An0ther&Str0ng"}); err != nil {
		t.Fatalf("active login: %v", err)
	}
	// Deactivate, then login is refused with the generic error (no disclosure).
	if err := s.DeactivateUser(ctx, res.User.ID); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	if _, err := s.Login(ctx, LoginInput{Email: "member@example.org", Password: "An0ther&Str0ng"}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("deactivated login = %v, want ErrInvalidCredentials", err)
	}
}

func TestLoginEmailIsCaseInsensitive(t *testing.T) {
	s := newService(t)
	fixedClockTime(s)
	ctx := context.Background()
	// Setup stores the email normalised; logging in with different casing works.
	if _, err := s.Setup(ctx, SetupInput{Email: "Owner@Example.ORG", Password: goodPassword, PasswordConfirm: goodPassword, Language: "fr", Currency: "EUR"}); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if _, err := s.Login(ctx, LoginInput{Email: "owner@example.org", Password: goodPassword}); err != nil {
		t.Fatalf("case-insensitive login: %v", err)
	}
}

func strp(s string) *string { return &s }
