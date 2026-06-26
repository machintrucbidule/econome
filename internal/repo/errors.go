package repo

import (
	"errors"

	"modernc.org/sqlite"
)

// SQLite extended result codes for constraint violations (sqlite3.h).
const (
	sqliteConstraintUnique     = 2067 // SQLITE_CONSTRAINT_UNIQUE
	sqliteConstraintPrimaryKey = 1555 // SQLITE_CONSTRAINT_PRIMARYKEY
	sqliteConstraintForeignKey = 787  // SQLITE_CONSTRAINT_FOREIGNKEY
)

// isForeignKeyViolation reports whether err is a SQLite FK constraint violation
// (e.g. deleting a row with dependents under ON DELETE RESTRICT, L4), surfaced as
// domain.ErrConflict.
func isForeignKeyViolation(err error) bool {
	var se *sqlite.Error
	if errors.As(err, &se) {
		return se.Code() == sqliteConstraintForeignKey
	}
	return false
}

// isUniqueViolation reports whether err is a SQLite UNIQUE/PRIMARY KEY constraint
// violation, so the repository can surface it as domain.ErrDuplicate (mapped to
// 409/422 by the transport layer, G3) instead of a raw driver error.
func isUniqueViolation(err error) bool {
	var se *sqlite.Error
	if errors.As(err, &se) {
		switch se.Code() {
		case sqliteConstraintUnique, sqliteConstraintPrimaryKey:
			return true
		}
	}
	return false
}
