// Package migrations exposes the forward-only SQL migration files as an
// embedded filesystem so the binary is self-contained (technical/02 §3, 08).
// The hand-rolled runner (internal/repo) globs *.up.sql in version order (I-003).
package migrations

import "embed"

// FS holds the embedded forward-only migration files (NNNN_name.up.sql).
//
//go:embed *.up.sql
var FS embed.FS
