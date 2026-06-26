module econome

go 1.26.4

// No external runtime dependencies at increment 0 (stdlib-only compiling stubs).
// modernc.org/sqlite is added in increment 1 when internal/repo + the migration
// runner first import it (I-001, I-003). Test/dev tools (golangci-lint, gofumpt,
// rapid, goquery, chromedp) are not module requirements of the binary.
