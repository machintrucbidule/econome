package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"econome/internal/domain"
)

// 2FA lifecycle tests (increment 8, functional/01 §3/§5, technical/05 §5).

func fixedClockTime(s *Service) time.Time {
	now := time.Date(2026, 6, 15, 12, 0, 30, 0, time.UTC)
	s.now = func() time.Time { return now }
	return now
}

func TestTOTPEnrolConfirmAndLoginStep(t *testing.T) {
	s := newService(t)
	now := fixedClockTime(s)
	ctx := context.Background()
	uid := seedUser(t, s)

	// Enrol → a secret is stored, 2FA not yet enabled.
	enr, err := s.BeginTOTPEnrolment(ctx, uid)
	if err != nil {
		t.Fatalf("begin enrol: %v", err)
	}
	u, _ := s.users.GetByID(ctx, s.tx.DB(), uid)
	if u.TOTPEnabled || u.TOTPSecret == nil {
		t.Fatal("enrolment should store a pending secret, disabled")
	}

	// A wrong confirmation code is a 422.
	if _, err := s.ConfirmTOTP(ctx, uid, "000000"); err == nil {
		t.Fatal("wrong confirm code should fail")
	} else {
		var ve *domain.ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("want ValidationError, got %v", err)
		}
	}

	// Confirm with a live code → enabled + backup codes issued.
	code, _ := totp.GenerateCode(enr.Secret, now)
	codes, err := s.ConfirmTOTP(ctx, uid, code)
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if len(codes) != 10 {
		t.Fatalf("got %d backup codes, want 10", len(codes))
	}
	u, _ = s.users.GetByID(ctx, s.tx.DB(), uid)
	if !u.TOTPEnabled {
		t.Fatal("2FA should be enabled after confirm")
	}

	// Login now returns the 2FA step (no session).
	res, err := s.Login(ctx, LoginInput{Email: "owner@example.org", Password: goodPassword})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if !res.TwoFactor || res.Pending == "" || res.Token != "" {
		t.Fatalf("login should require 2FA, got %+v", res)
	}

	// Completing with a fresh TOTP opens the session.
	step, _ := totp.GenerateCode(enr.Secret, now)
	done, err := s.CompleteTOTPLogin(ctx, res.Pending, step, "1.2.3.4")
	if err != nil {
		t.Fatalf("complete totp: %v", err)
	}
	if done.Token == "" {
		t.Fatal("session token expected after 2FA")
	}

	// A backup code works once, then is consumed.
	res2, _ := s.Login(ctx, LoginInput{Email: "owner@example.org", Password: goodPassword})
	if _, err := s.CompleteTOTPLogin(ctx, res2.Pending, codes[0], "1.2.3.4"); err != nil {
		t.Fatalf("backup-code login: %v", err)
	}
	res3, _ := s.Login(ctx, LoginInput{Email: "owner@example.org", Password: goodPassword})
	if _, err := s.CompleteTOTPLogin(ctx, res3.Pending, codes[0], "1.2.3.4"); !errors.Is(err, ErrTOTPRequired) {
		t.Fatalf("reused backup code = %v, want ErrTOTPRequired", err)
	}
	if rem, _ := s.BackupCodesRemaining(ctx, uid); rem != 9 {
		t.Fatalf("remaining backup codes = %d, want 9", rem)
	}
}

func TestTOTPDisableRequiresPassword(t *testing.T) {
	s := newService(t)
	now := fixedClockTime(s)
	ctx := context.Background()
	uid := seedUser(t, s)
	enr, _ := s.BeginTOTPEnrolment(ctx, uid)
	code, _ := totp.GenerateCode(enr.Secret, now)
	if _, err := s.ConfirmTOTP(ctx, uid, code); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	// Wrong password is rejected.
	if err := s.DisableTOTP(ctx, uid, "wrong-password", ""); err == nil {
		t.Fatal("disable with wrong password should fail")
	}
	// Correct password disables and clears codes.
	if err := s.DisableTOTP(ctx, uid, goodPassword, ""); err != nil {
		t.Fatalf("disable: %v", err)
	}
	u, _ := s.users.GetByID(ctx, s.tx.DB(), uid)
	if u.TOTPEnabled || u.TOTPSecret != nil {
		t.Fatal("2FA should be off with no secret after disable")
	}
	if rem, _ := s.BackupCodesRemaining(ctx, uid); rem != 0 {
		t.Fatalf("backup codes after disable = %d, want 0", rem)
	}
}
