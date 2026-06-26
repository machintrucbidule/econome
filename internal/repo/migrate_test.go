package repo_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"econome/internal/repo"
	"econome/migrations"
)

func openTestDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	dir := t.TempDir()
	db, err := repo.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, dir
}

func tableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&n)
	if err != nil {
		t.Fatalf("tableExists(%s): %v", name, err)
	}
	return n > 0
}

func backupCount(t *testing.T, dir string) int {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "econome-premigrate-*.db"))
	if err != nil {
		t.Fatalf("glob backups: %v", err)
	}
	return len(matches)
}

// The real 0001_init applies forward-from-empty and is idempotent.
func TestMigrate_RealInitForwardFromEmpty(t *testing.T) {
	db, dir := openTestDB(t)
	backups := filepath.Join(dir, "backups")
	ctx := context.Background()

	if err := repo.Migrate(ctx, db, migrations.FS, backups); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	for _, tbl := range []string{"user", "session", "settings", "schema_migrations"} {
		if !tableExists(t, db, tbl) {
			t.Errorf("table %q not created", tbl)
		}
	}
	var version int
	if err := db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatalf("read version: %v", err)
	}
	if version != 1 {
		t.Errorf("schema version = %d, want 1", version)
	}

	// Re-running is a no-op (no pending ⇒ no backup taken).
	if err := repo.Migrate(ctx, db, migrations.FS, backups); err != nil {
		t.Fatalf("re-migrate: %v", err)
	}
}

// A multi-migration run takes exactly one pre-migration backup and records both.
func TestMigrate_BackupAndOrdering(t *testing.T) {
	db, dir := openTestDB(t)
	backups := filepath.Join(dir, "backups")
	ctx := context.Background()

	fsys := fstest.MapFS{
		"0002_b.up.sql": {Data: []byte(`CREATE TABLE b (id INTEGER PRIMARY KEY);`)},
		"0001_a.up.sql": {Data: []byte(`CREATE TABLE a (id INTEGER PRIMARY KEY);`)},
	}
	if err := repo.Migrate(ctx, db, fsys, backups); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !tableExists(t, db, "a") || !tableExists(t, db, "b") {
		t.Fatal("both migrations should have applied in version order")
	}
	if got := backupCount(t, backups); got != 1 {
		t.Errorf("backup count = %d, want 1", got)
	}
}

// A failing migration aborts: its tx rolls back, the backup is intact, and the
// schema stops at the last good version.
func TestMigrate_AbortOnFailure(t *testing.T) {
	db, dir := openTestDB(t)
	backups := filepath.Join(dir, "backups")
	ctx := context.Background()

	fsys := fstest.MapFS{
		"0001_good.up.sql": {Data: []byte(`CREATE TABLE good (id INTEGER PRIMARY KEY);`)},
		"0002_bad.up.sql":  {Data: []byte(`CREATE TABLE bad (this is not valid sql`)},
	}
	if err := repo.Migrate(ctx, db, fsys, backups); err == nil {
		t.Fatal("expected migrate to fail on the bad migration")
	}
	if !tableExists(t, db, "good") {
		t.Error("0001 should have committed before 0002 failed")
	}
	if tableExists(t, db, "bad") {
		t.Error("0002 must have rolled back")
	}
	var version int
	if err := db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatalf("read version: %v", err)
	}
	if version != 1 {
		t.Errorf("schema version = %d, want 1 (0002 not recorded)", version)
	}
	if backupCount(t, backups) != 1 {
		t.Error("pre-migration backup must be intact after abort")
	}
	if _, err := os.Stat(backups); err != nil {
		t.Errorf("backup dir missing: %v", err)
	}
}
