module econome

go 1.26.4

// Runtime deps stay minimal (technical/01 §1). modernc.org/sqlite + golang.org/x/*
// are added in PR-b/PR-c of increment 1 when repo/auth/i18n first import them
// (I-001). pgregory.net/rapid is a test-only dependency (engine property tests).
// Other dev tools (golangci-lint, gofumpt, goquery, chromedp) are not module
// requirements of the binary.

require (
	github.com/BurntSushi/toml v1.6.0
	github.com/PuerkitoBio/goquery v1.12.0
	golang.org/x/crypto v0.53.0
	modernc.org/sqlite v1.53.0
	pgregory.net/rapid v1.3.0
)

require (
	github.com/andybalholm/cascadia v1.3.3 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/sync v0.21.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
	modernc.org/libc v1.73.4 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)
