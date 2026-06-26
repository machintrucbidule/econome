// Package assets exposes the embedded static frontend assets (technical/02 §3,
// 01 §1). econome.css and econome.js are the validated design system, reused
// VERBATIM from specifications/mockups/assets (G5, mockups/README.md); they are
// byte-for-byte copies and must only be re-synced from that source, never
// hand-edited here.
//
// htmx, web fonts, and icons are vendored alongside these in increment 1, when
// the three-pane shell template first loads them and the exact htmx version can
// be pinned; the embed directive is widened to include them then.
package assets

import "embed"

// FS holds the embedded static frontend assets (design system + vendored libs).
//
//go:embed econome.css econome.js
var FS embed.FS
