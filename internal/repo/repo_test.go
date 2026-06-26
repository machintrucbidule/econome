package repo_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"econome/internal/domain"
	"econome/internal/repo"
	"econome/internal/repo/repotest"
	"econome/migrations"
)

func newSQLiteStore(t *testing.T) *repo.Store {
	t.Helper()
	db, dir := openTestDB(t)
	if err := repo.Migrate(context.Background(), db, migrations.FS, filepath.Join(dir, "backups")); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return repo.New(db)
}

func newUser(email string) *domain.User {
	now := time.Now().UTC().Truncate(time.Second)
	return &domain.User{
		Email: email, PasswordHash: "phc", Status: domain.StatusActive,
		Language: domain.LangFR, Currency: "EUR", CreatedAt: now, UpdatedAt: now,
	}
}

func newSettings(userID int64) *domain.Settings {
	return &domain.Settings{
		UserID: userID, PEASocialChargeRate: 1720, NearCapThreshold: 9000,
		SecuredSavingsBasis: domain.BasisAllPlanned, Theme: domain.ThemeLight,
		Language: domain.LangFR, Currency: "EUR", UpdatedAt: time.Now().UTC().Truncate(time.Second),
	}
}

func newSession(userID int64, tokenHash string) *domain.Session {
	now := time.Now().UTC().Truncate(time.Second)
	return &domain.Session{
		UserID: userID, TokenHash: tokenHash, Kind: domain.SessionShort,
		ExpiresAt: now.Add(24 * time.Hour), CreatedAt: now, LastSeenAt: now,
	}
}

// contract exercises the repository behaviour both the SQLite store and the
// in-memory fake must satisfy identically (technical/09 §4 fakes parity).
func contract(t *testing.T, users repo.UserRepo, sessions repo.SessionRepo, settings repo.SettingsRepo, q repo.DBTX) {
	t.Helper()
	ctx := context.Background()

	if n, err := users.CountUsers(ctx, q); err != nil || n != 0 {
		t.Fatalf("CountUsers = %d, %v; want 0, nil", n, err)
	}

	id, err := users.Create(ctx, q, newUser("owner@example.org"))
	if err != nil || id == 0 {
		t.Fatalf("Create user = %d, %v", id, err)
	}
	if n, _ := users.CountUsers(ctx, q); n != 1 {
		t.Fatalf("CountUsers = %d, want 1", n)
	}
	if _, err := users.Create(ctx, q, newUser("owner@example.org")); !errors.Is(err, domain.ErrDuplicate) {
		t.Errorf("duplicate email Create err = %v, want ErrDuplicate", err)
	}

	got, err := users.GetByEmail(ctx, q, "owner@example.org")
	if err != nil || got.ID != id || got.Status != domain.StatusActive {
		t.Fatalf("GetByEmail = %+v, %v", got, err)
	}
	if _, err := users.GetByEmail(ctx, q, "nobody@example.org"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("GetByEmail(missing) err = %v, want ErrNotFound", err)
	}
	if g, err := users.GetByID(ctx, q, id); err != nil || g.Email != "owner@example.org" {
		t.Fatalf("GetByID = %+v, %v", g, err)
	}

	// Settings round-trip + tenant scoping.
	if err := settings.Create(ctx, q, newSettings(id)); err != nil {
		t.Fatalf("settings Create: %v", err)
	}
	if s, err := settings.Get(ctx, q, id); err != nil || s.PEASocialChargeRate != 1720 {
		t.Fatalf("settings Get = %+v, %v", s, err)
	}
	if _, err := settings.Get(ctx, q, 99999); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("settings Get(other tenant) err = %v, want ErrNotFound", err)
	}

	// Session round-trip.
	sid, err := sessions.Create(ctx, q, newSession(id, "hash-abc"))
	if err != nil || sid == 0 {
		t.Fatalf("session Create = %d, %v", sid, err)
	}
	if s, err := sessions.GetByTokenHash(ctx, q, "hash-abc"); err != nil || s.UserID != id {
		t.Fatalf("GetByTokenHash = %+v, %v", s, err)
	}
	newExpiry := time.Now().UTC().Truncate(time.Second).Add(48 * time.Hour)
	if err := sessions.Touch(ctx, q, sid, time.Now().UTC().Truncate(time.Second), newExpiry); err != nil {
		t.Fatalf("Touch: %v", err)
	}
	if s, _ := sessions.GetByTokenHash(ctx, q, "hash-abc"); !s.ExpiresAt.Equal(newExpiry) {
		t.Errorf("Touch did not update expiry: %v vs %v", s.ExpiresAt, newExpiry)
	}
	if err := sessions.Delete(ctx, q, sid); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := sessions.GetByTokenHash(ctx, q, "hash-abc"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("after Delete err = %v, want ErrNotFound", err)
	}

	// Login-state + password update.
	u, _ := users.GetByID(ctx, q, id)
	u.FailedLoginCount = 3
	if err := users.UpdateLoginState(ctx, q, u); err != nil {
		t.Fatalf("UpdateLoginState: %v", err)
	}
	if g, _ := users.GetByID(ctx, q, id); g.FailedLoginCount != 3 {
		t.Errorf("FailedLoginCount = %d, want 3", g.FailedLoginCount)
	}
	if err := users.UpdatePasswordHash(ctx, q, id, "phc2"); err != nil {
		t.Fatalf("UpdatePasswordHash: %v", err)
	}
	if g, _ := users.GetByID(ctx, q, id); g.PasswordHash != "phc2" {
		t.Errorf("PasswordHash = %q, want phc2", g.PasswordHash)
	}
}

func TestContract_SQLite(t *testing.T) {
	st := newSQLiteStore(t)
	contract(t, st.Users, st.Sessions, st.Settings, st.DB())
}

func TestContract_Fake(t *testing.T) {
	st := repotest.NewStore()
	contract(t, st.Users, st.Sessions, st.Settings, st.DB())
}

// SQLite-specific: foreign keys enforced, cascade on user delete, tx rollback.
func TestSQLite_ForeignKeysAndTx(t *testing.T) {
	st := newSQLiteStore(t)
	ctx := context.Background()

	// FK: a session for a non-existent user is rejected.
	if _, err := st.Sessions.Create(ctx, st.DB(), newSession(4242, "orphan")); err == nil {
		t.Error("expected FK violation creating session for missing user")
	}

	// Tx rollback: a failing use-case persists nothing ("no partial write").
	wantErr := errors.New("boom")
	err := st.WithTx(ctx, func(q repo.DBTX) error {
		if _, e := st.Users.Create(ctx, q, newUser("tx@example.org")); e != nil {
			return e
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("WithTx err = %v, want boom", err)
	}
	if n, _ := st.Users.CountUsers(ctx, st.DB()); n != 0 {
		t.Errorf("after rollback CountUsers = %d, want 0", n)
	}

	// Cascade: deleting a user removes their sessions.
	id, _ := st.Users.Create(ctx, st.DB(), newUser("casc@example.org"))
	if _, err := st.Sessions.Create(ctx, st.DB(), newSession(id, "casc-tok")); err != nil {
		t.Fatalf("session create: %v", err)
	}
	if _, err := st.DB().ExecContext(ctx, `DELETE FROM user WHERE id = ?`, id); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	if _, err := st.Sessions.GetByTokenHash(ctx, st.DB(), "casc-tok"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("session should cascade-delete; err = %v", err)
	}
}
