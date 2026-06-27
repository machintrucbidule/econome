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
	github.com/chromedp/chromedp v0.15.1
	golang.org/x/crypto v0.53.0
	modernc.org/sqlite v1.53.0
	pgregory.net/rapid v1.3.0
)

require (
	github.com/andybalholm/cascadia v1.3.3 // indirect
	github.com/boombuler/barcode v1.0.1-0.20190219062509-6c824513bacc // indirect
	github.com/chromedp/cdproto v0.0.0-20260321001828-e3e3800016bc // indirect
	github.com/chromedp/sysutil v1.1.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/go-json-experiment/json v0.0.0-20260214004413-d219187c3433 // indirect
	github.com/gobwas/httphead v0.1.0 // indirect
	github.com/gobwas/pool v0.2.1 // indirect
	github.com/gobwas/ws v1.4.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/pquerna/otp v1.5.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/skip2/go-qrcode v0.0.0-20200617195104-da1b6568686e // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/sync v0.21.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
	modernc.org/libc v1.73.4 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)
