// Package templates exposes the embedded html/template files (technical/02 §3).
//
// At increment 0 no templates exist yet, so the embed pattern points at the
// placeholder README.md (a zero-match //go:embed pattern is a compile error).
// Increment 1 adds the shell + signed-out layouts and switches the directive to
// embed the real *.html templates.
package templates

import "embed"

// FS holds the embedded html/template files.
//
//go:embed README.md
var FS embed.FS
