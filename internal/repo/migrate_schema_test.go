package repo_test

import (
	"context"
	"io/fs"
	"path/filepath"
	"testing"
	"testing/fstest"

	"econome/internal/repo"
	"econome/migrations"
)

// allTables are every table the migrations must create (baseline 0001 +
// schema_migrations from the runner + the budget schema 0002–0006).
var allTables = []string{
	"user", "session", "settings", "schema_migrations",
	"account", "category", "envelope", "allocation", "transaction",
	"period", "period_event",
	"savings_snapshot", "networth_month",
	"label_mapping", "ui_preference",
	"invitation", "totp_backup_code",
}

func TestMigrate_FullSchemaForwardFromEmpty(t *testing.T) {
	db, dir := openTestDB(t)
	if err := repo.Migrate(context.Background(), db, migrations.FS, filepath.Join(dir, "backups")); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	for _, tbl := range allTables {
		if !tableExists(t, db, tbl) {
			t.Errorf("table %q not created", tbl)
		}
	}
	var version int
	if err := db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatalf("read version: %v", err)
	}
	if version != 6 {
		t.Errorf("schema version = %d, want 6", version)
	}
}

// Production-shaped: a DB already at version 1 with an owner+settings row migrates
// forward to 6 without data loss (the unattended-Watchtower upgrade path).
func TestMigrate_ProductionShaped(t *testing.T) {
	db, dir := openTestDB(t)
	ctx := context.Background()
	backups := filepath.Join(dir, "backups")

	// Bring the DB to version 1 with only the baseline migration.
	init0001, err := fs.ReadFile(migrations.FS, "0001_init.up.sql")
	if err != nil {
		t.Fatalf("read 0001: %v", err)
	}
	onlyV1 := fstest.MapFS{"0001_init.up.sql": {Data: init0001}}
	if err := repo.Migrate(ctx, db, onlyV1, backups); err != nil {
		t.Fatalf("migrate v1: %v", err)
	}

	// Seed production-shaped data (an owner + settings row).
	if _, err := db.ExecContext(ctx,
		`INSERT INTO user (email, password_hash, status, language, currency, created_at, updated_at)
		 VALUES ('owner@example.org', 'phc', 'active', 'fr', 'EUR', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO settings (user_id, updated_at) VALUES (1, '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("seed settings: %v", err)
	}

	// Migrate forward to 6 against the full set.
	if err := repo.Migrate(ctx, db, migrations.FS, backups); err != nil {
		t.Fatalf("migrate to 6: %v", err)
	}

	// Data preserved + new tables present.
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM user WHERE email = 'owner@example.org'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("owner row lost: n=%d err=%v", n, err)
	}
	for _, tbl := range []string{"account", "transaction", "savings_snapshot", "invitation"} {
		if !tableExists(t, db, tbl) {
			t.Errorf("table %q missing after forward migration", tbl)
		}
	}
	// A backup was taken before each run with pending migrations (v1 + the v2–6 batch).
	if backupCount(t, backups) < 2 {
		t.Errorf("expected >= 2 pre-migration backups, got %d", backupCount(t, backups))
	}
}
