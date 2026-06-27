package main

import (
	"context"
	"path/filepath"
	"testing"

	"econome/internal/repo"
	"econome/internal/services"
	"econome/migrations"
)

// CLI integration test (increment 8, technical/05 §8): drives the recovery
// commands against a real temp SQLite database seeded with an owner. The
// recovery logic itself is covered by the service tests; this proves the CLI
// wiring (open → migrate → service → command) end to end.

func seedDataDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("ECONOME_DATA_DIR", dir)
	db, err := repo.Open(filepath.Join(dir, "econome.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := repo.Migrate(context.Background(), db, migrations.FS, filepath.Join(dir, "backups")); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store := repo.New(db)
	svc := services.New(services.Deps{
		Users: store.Users, Sessions: store.Sessions, Settings: store.Settings,
		Accounts: store.Accounts, Categories: store.Categories, Envelopes: store.Envelopes,
		Allocations: store.Allocations, Transactions: store.Transactions, Snapshots: store.Snapshots,
		NetworthMonths: store.NetworthMonths, Periods: store.Periods, PeriodEvents: store.PeriodEvents,
		Labels: store.Labels, UIPreferences: store.UIPreferences,
		Invitations: store.Invitations, TOTPBackups: store.TOTPBackups,
		Tx: store, Secret: []byte("cli-test-secret-0123456789abcdef"),
	})
	if _, err := svc.Setup(context.Background(), services.SetupInput{
		Email: "owner@example.org", Password: "Tr0ub4dour&3xtra", PasswordConfirm: "Tr0ub4dour&3xtra",
		Language: "fr", Currency: "EUR",
	}); err != nil {
		t.Fatalf("seed owner: %v", err)
	}
	_ = db.Close()
	return dir
}

func TestCLIRecoveryCommands(t *testing.T) {
	seedDataDir(t)

	// user list / backup succeed.
	if err := dispatch("user", []string{"list"}); err != nil {
		t.Fatalf("user list: %v", err)
	}
	if err := dispatch("backup", nil); err != nil {
		t.Fatalf("backup: %v", err)
	}

	// reset-password / reset-2fa succeed for the seeded owner.
	if err := dispatch("reset-password", []string{"owner@example.org"}); err != nil {
		t.Fatalf("reset-password: %v", err)
	}
	if err := dispatch("reset-2fa", []string{"owner@example.org"}); err != nil {
		t.Fatalf("reset-2fa: %v", err)
	}

	// The last admin cannot be deactivated (shared rule).
	if err := dispatch("user", []string{"deactivate", "owner@example.org"}); err == nil {
		t.Fatal("deactivating the last admin should fail")
	}

	// Unknown user → clear error.
	if err := dispatch("reset-password", []string{"nobody@example.org"}); err == nil {
		t.Fatal("reset-password for unknown user should fail")
	}
}
