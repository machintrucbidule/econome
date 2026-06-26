// Package migrations exposes the forward-only SQL migration files as an
// embedded filesystem so the binary is self-contained (technical/02 §3, 08).
//
// At increment 0 there are no .up.sql files yet, so the embed pattern points at
// the placeholder README.md (an //go:embed pattern that matches zero files is a
// compile error). Increment 1 adds 0001_init.up.sql and switches the directive
// to `//go:embed *.up.sql`; the hand-rolled runner globs *.up.sql (I-003).
package migrations

import "embed"

// FS holds the embedded forward-only migration files.
//
//go:embed README.md
var FS embed.FS
