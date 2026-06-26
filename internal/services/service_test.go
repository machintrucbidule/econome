package services

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"econome/internal/domain"
	"econome/internal/repo"
	"econome/migrations"
)

func newService(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	db, err := repo.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := repo.Migrate(context.Background(), db, migrations.FS, filepath.Join(dir, "backups")); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store := repo.New(db)
	return New(store.Users, store.Sessions, store.Settings, store, []byte("test-secret-0123456789abcdef0123"))
}

const goodPassword = "Tr0ub4dour&3xtra"

func validSetup() SetupInput {
	return SetupInput{Email: "owner@example.org", Password: goodPassword, PasswordConfirm: goodPassword, Language: "fr", Currency: "EUR"}
}

func TestSetupCreatesOwnerAndSession(t *testing.T) {
	s := newService(t)
	ctx := context.Background()

	if zero, _ := s.ZeroUsers(ctx); !zero {
		t.Fatal("fresh instance should have zero users")
	}
	res, err := s.Setup(ctx, validSetup())
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if res.User.ID == 0 || !res.User.IsAdmin || res.Token == "" {
		t.Fatalf("unexpected setup result: %+v", res.User)
	}
	if zero, _ := s.ZeroUsers(ctx); zero {
		t.Error("after setup there should be a user")
	}
	// Settings row was created with defaults.
	st, err := s.Settings(ctx, res.User.ID)
	if err != nil || st.PEASocialChargeRate != 1720 || st.SecuredSavingsBasis != domain.BasisAllPlanned {
		t.Fatalf("settings: %+v err=%v", st, err)
	}
	// Session resolves.
	u, _, err := s.ResolveSession(ctx, res.Token)
	if err != nil || u.ID != res.User.ID {
		t.Fatalf("ResolveSession: %v", err)
	}
}

func TestSetupValidationAndGuard(t *testing.T) {
	s := newService(t)
	ctx := context.Background()

	// Bad input -> typed ValidationError, no user created.
	bad := SetupInput{Email: "x", Password: "short", PasswordConfirm: "nope", Language: "fr"}
	_, err := s.Setup(ctx, bad)
	var ve *domain.ValidationError
	if !errors.As(err, &ve) || len(ve.Fields) == 0 {
		t.Fatalf("want ValidationError, got %v", err)
	}
	if zero, _ := s.ZeroUsers(ctx); !zero {
		t.Error("no user should be persisted after invalid setup")
	}

	// First owner ok; a second setup is refused (guard).
	if _, err := s.Setup(ctx, validSetup()); err != nil {
		t.Fatalf("first setup: %v", err)
	}
	if _, err := s.Setup(ctx, SetupInput{Email: "two@example.org", Password: goodPassword, PasswordConfirm: goodPassword, Language: "fr", Currency: "EUR"}); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("second setup err = %v, want ErrConflict", err)
	}
}

func TestLoginSuccessAndGenericFailure(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	if _, err := s.Setup(ctx, validSetup()); err != nil {
		t.Fatal(err)
	}

	res, err := s.Login(ctx, LoginInput{Email: "owner@example.org", Password: goodPassword, IP: "1.2.3.4"})
	if err != nil || res.Token == "" {
		t.Fatalf("login success: %v", err)
	}

	_, err = s.Login(ctx, LoginInput{Email: "owner@example.org", Password: "wrong", IP: "1.2.3.4"})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("wrong password err = %v, want ErrInvalidCredentials", err)
	}
	// Unknown email gives the same generic error (no user enumeration).
	_, err = s.Login(ctx, LoginInput{Email: "nobody@example.org", Password: goodPassword, IP: "1.2.3.4"})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("unknown email err = %v, want ErrInvalidCredentials", err)
	}
}

func TestLoginLockoutBackoff(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	base := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return base }
	if _, err := s.Setup(ctx, validSetup()); err != nil {
		t.Fatal(err)
	}

	// 5 failures: still just invalid credentials (no lock yet).
	for i := 0; i < 5; i++ {
		if _, err := s.Login(ctx, LoginInput{Email: "owner@example.org", Password: "wrong", IP: "ip"}); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("failure %d err = %v", i+1, err)
		}
	}
	// 6th failure triggers the lock; the next attempt is throttled.
	if _, err := s.Login(ctx, LoginInput{Email: "owner@example.org", Password: "wrong", IP: "ip"}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("6th failure err = %v", err)
	}
	var locked *LockedError
	if _, err := s.Login(ctx, LoginInput{Email: "owner@example.org", Password: goodPassword, IP: "ip"}); !errors.As(err, &locked) {
		t.Fatalf("after 6 failures err = %v, want LockedError", err)
	}
	if locked.RetrySeconds() < 1 {
		t.Errorf("RetrySeconds = %d, want >=1", locked.RetrySeconds())
	}

	// After the lock window, a correct password succeeds and resets the counter.
	s.now = func() time.Time { return base.Add(2 * time.Second) }
	if _, err := s.Login(ctx, LoginInput{Email: "owner@example.org", Password: goodPassword, IP: "ip"}); err != nil {
		t.Fatalf("login after lock window: %v", err)
	}
}

func TestLogoutAndSessionExpiry(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	base := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return base }
	res, err := s.Setup(ctx, validSetup())
	if err != nil {
		t.Fatal(err)
	}

	// Logout revokes immediately.
	if err := s.Logout(ctx, res.Token); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, _, err := s.ResolveSession(ctx, res.Token); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("resolve after logout err = %v, want ErrNotFound", err)
	}

	// A new session expires after its short lifetime.
	res2, _ := s.Login(ctx, LoginInput{Email: "owner@example.org", Password: goodPassword, IP: "ip"})
	s.now = func() time.Time { return base.Add(25 * time.Hour) }
	if _, _, err := s.ResolveSession(ctx, res2.Token); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("resolve expired err = %v, want ErrNotFound", err)
	}
}
