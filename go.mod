module econome

go 1.26.4

// Runtime deps stay minimal (technical/01 §1). modernc.org/sqlite + golang.org/x/*
// are added in PR-b/PR-c of increment 1 when repo/auth/i18n first import them
// (I-001). pgregory.net/rapid is a test-only dependency (engine property tests).
// Other dev tools (golangci-lint, gofumpt, goquery, chromedp) are not module
// requirements of the binary.

require pgregory.net/rapid v1.3.0
