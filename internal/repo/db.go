package repo

import (
	"database/sql"
	"fmt"
	"net/url"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, registered as "sqlite" (I-001)
)

// Open opens (creating if absent) the SQLite database at path with WAL,
// foreign-key enforcement, and a busy timeout — the load-bearing pragmas of
// technical/07 §4. It is the only place the SQLite driver is opened (the
// depguard "sql-only-in-repo" rule keeps the driver out of every other package).
func Open(path string) (*sql.DB, error) {
	// modernc.org/sqlite applies pragmas passed as _pragma DSN parameters on
	// every new connection in the pool, so WAL/foreign_keys/busy_timeout hold
	// regardless of which pooled connection serves a query.
	dsn := "file:" + path + "?" + url.Values{
		"_pragma": {"journal_mode(WAL)", "foreign_keys(ON)", "busy_timeout(5000)"},
	}.Encode()

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("repo: open sqlite: %w", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("repo: ping sqlite: %w", err)
	}
	return db, nil
}
