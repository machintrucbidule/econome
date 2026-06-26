package repo

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Migration is one discovered forward migration.
type Migration struct {
	Version int
	Name    string
	SQL     string
}

// Migrate applies every pending forward migration from fsys (the embedded
// migrations.FS), in version order, each inside its own transaction. Before
// applying any pending migration it writes a VACUUM INTO snapshot under
// backupDir; on any failure it aborts with the backup intact and the schema
// unchanged past the last good migration (technical/08 §1–§2, I-003).
//
// backupDir is created if missing. The schema_migrations tracking table is
// bootstrapped here (a tracking table cannot migrate itself).
func Migrate(ctx context.Context, db *sql.DB, fsys fs.FS, backupDir string) error {
	if _, err := db.ExecContext(ctx,
		`CREATE TABLE IF NOT EXISTS schema_migrations (
		   version    INTEGER PRIMARY KEY,
		   applied_at TEXT NOT NULL
		 )`); err != nil {
		return fmt.Errorf("repo: bootstrap schema_migrations: %w", err)
	}

	all, err := discoverMigrations(fsys)
	if err != nil {
		return err
	}

	applied, err := appliedVersions(ctx, db)
	if err != nil {
		return err
	}

	var pending []Migration
	for _, m := range all {
		if !applied[m.Version] {
			pending = append(pending, m)
		}
	}
	if len(pending) == 0 {
		return nil
	}

	from := maxVersion(applied)
	to := pending[len(pending)-1].Version
	if err := backup(ctx, db, backupDir, from, to); err != nil {
		return err
	}

	for _, m := range pending {
		if err := applyOne(ctx, db, m); err != nil {
			// Abort: the failed migration's tx is rolled back; earlier ones and
			// the pre-migration backup remain intact.
			return fmt.Errorf("repo: migration %04d_%s failed (aborting, backup intact): %w", m.Version, m.Name, err)
		}
	}
	return nil
}

// discoverMigrations reads and sorts every NNNN_name.up.sql in fsys.
func discoverMigrations(fsys fs.FS) ([]Migration, error) {
	entries, err := fs.Glob(fsys, "*.up.sql")
	if err != nil {
		return nil, fmt.Errorf("repo: glob migrations: %w", err)
	}
	out := make([]Migration, 0, len(entries))
	for _, name := range entries {
		base := strings.TrimSuffix(name, ".up.sql")
		numStr, rest, found := strings.Cut(base, "_")
		if !found {
			return nil, fmt.Errorf("repo: malformed migration filename %q (want NNNN_name.up.sql)", name)
		}
		v, err := strconv.Atoi(numStr)
		if err != nil {
			return nil, fmt.Errorf("repo: bad version in %q: %w", name, err)
		}
		body, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("repo: read migration %q: %w", name, err)
		}
		out = append(out, Migration{Version: v, Name: rest, SQL: string(body)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

func appliedVersions(ctx context.Context, db *sql.DB) (map[int]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("repo: read applied versions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	applied := map[int]bool{}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("repo: scan version: %w", err)
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo: iterate versions: %w", err)
	}
	return applied, nil
}

func applyOne(ctx context.Context, db *sql.DB, m Migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }() // no-op after a successful commit

	if _, err := tx.ExecContext(ctx, m.SQL); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`,
		m.Version, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	return tx.Commit()
}

// backup writes a consistent pre-migration snapshot via VACUUM INTO. VACUUM
// cannot run inside a transaction, so it runs on the connection directly.
func backup(ctx context.Context, db *sql.DB, backupDir string, from, to int) error {
	if err := os.MkdirAll(backupDir, 0o750); err != nil {
		return fmt.Errorf("repo: create backup dir: %w", err)
	}
	ts := time.Now().UTC().Format("20060102-150405")
	target := filepath.Join(backupDir, fmt.Sprintf("econome-premigrate-%d-%d-%s.db", from, to, ts))

	// VACUUM INTO takes a string literal, not a bound parameter; target is
	// app-controlled (never user input). Escape single quotes defensively.
	literal := "'" + strings.ReplaceAll(target, "'", "''") + "'"
	if _, err := db.ExecContext(ctx, "VACUUM INTO "+literal); err != nil { //nolint:gosec // target is app-controlled and quote-escaped; VACUUM INTO cannot be parameterised
		return fmt.Errorf("repo: pre-migration backup failed: %w", err)
	}
	return nil
}

func maxVersion(applied map[int]bool) int {
	maxV := 0
	for v := range applied {
		if v > maxV {
			maxV = v
		}
	}
	return maxV
}
