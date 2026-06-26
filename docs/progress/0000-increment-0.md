# Increment 0 — Environment & repository setup

**Date.** 2026-06-26 · **Milestone.** M0 (Foundations & rails) · **Status.** Scaffold authored **and
locally verified green** (Go 1.26.4); git/GitHub publish next.

## What was built

The full repository scaffold per `technical/02` §1, as compiling stubs + configuration — **no
application behaviour**:

- `go.mod` (module `econome`, Go 1.24 pinned; **no external runtime deps at inc 0** — `modernc.org/sqlite`
  added in inc 1 when `repo`/the runner import it, else `go mod tidy` would drop it).
- `cmd/econome` (config load → structured `slog` → runnable stub server → graceful shutdown; `-ldflags`
  version stamp) and `cmd/econome-admin` (recovery-CLI stub, subcommands declared).
- `internal/{config,server,server/middleware,handlers,services,engine,repo,auth,i18n,view,domain}` — each
  a `doc.go` documenting its boundary; `config.go` reads every `ECONOME_*` var with the `technical/07` §3
  defaults; `server/routes.go` a minimal stdlib-mux stub (I-002).
- `migrations/`, `web/templates/`, `web/assets/` `embed.FS` homes (migrations/templates embed a
  placeholder README until real files land — a zero-match `//go:embed` is a compile error).
- **Design system imported verbatim**: `web/assets/econome.css` (23 834 B) + `econome.js` (15 228 B),
  byte-for-byte from `mockups/assets/` (copied via PowerShell / native NTFS, lengths verified).
- Tooling: `.golangci.yml` (v2; `G2` set + `G4` depguard purity rules — engine deny-list, SQL-only-in-repo,
  services-no-transport), `.editorconfig`, `.gitignore`, `.env.example` (every `ECONOME_*` documented).
- CI `.github/workflows/ci.yml` (format→vet→lint→test(-race)+coverage→govulncheck→build; multi-arch image
  build on PR/main; GHCR publish on `v*` tag only), `.github/dependabot.yml` (I-005).
- `Dockerfile` (multi-stage, multi-arch, `CGO_ENABLED=0` static → distroless), `docker-compose.yml`
  (named volume + external proxy network), `scripts/start.bat` + `stop.bat`.
- Context artefacts: `CLAUDE.md`, this log, `specifications/implementation/decision-log.md` (I-001..I-005).
- Smoke test `internal/config/config_test.go` (defaults / locale reject / overrides) so `go test -race`
  has a target and the harness is proven.

## Specs satisfied

`technical/01` T1 (stack pins), `technical/02` (whole — layout + boundaries), `technical/07` §1/§3/§7
(artefacts, env config, local loop), `guardrails/01` §1/§3 (style baseline, depguard boundaries),
`guardrails/04` §1/§3/§5/§6 (CI scaffold, publish-on-tag, Dependabot, secret hygiene), `guardrails/02`
§2 (`CLAUDE.md` + progress log created). Decisions: **I-001..I-005**.

## Tests passing

Verified locally with **Go 1.26.4** (all green):
- `go build ./...` ✓ · `go vet ./...` ✓ · `go test ./...` ✓ (`internal/config` smoke test: defaults /
  locale reject / overrides).
- `gofumpt -l .` ✓ clean · `golangci-lint run` ✓ **0 issues** · **depguard proven active** (a temporary
  `net/http` import in `internal/engine` was rejected with the G4 message, then removed).
- `govulncheck ./...` ✓ no vulnerabilities.
- **`go test -race`** not run locally — the race detector needs cgo + a C compiler (absent on this
  Windows box); it runs in **CI on ubuntu-latest** where cgo is available. The multi-arch image build is
  likewise a CI gate.

## Exact next step

1. **(Blocker) Install Go** (latest stable) locally, then from the repo root run, in order:
   `go mod tidy` · `gofumpt -l .` · `go vet ./...` · `golangci-lint run` · `go test -race ./...` ·
   `go build ./cmd/econome ./cmd/econome-admin`. Fix any scaffold issue surfaced (esp. the `.golangci.yml`
   v2 schema and the depguard globs — verify depguard is active by temporarily adding a forbidden import
   to `internal/engine` and confirming lint fails, then revert).
2. **Only after step 1 is green** (user chose verify-then-publish): `git init`; initial commit
   `chore: scaffold repository (increment 0)`; then create a **public** repo `econome` under
   `machintrucbidule` (`gh repo create econome --public --source=. --remote=origin`) — `specifications/`
   is gitignored (I-006), so the dossier is not pushed; push; set branch protection on `main` (no
   direct/force push, linear history, require green CI) via `gh api`. Confirm `git status` shows
   `specifications/` untracked and the app builds without it.
3. Confirm CI is green on the scaffold → increment 0 **done**; stop for the user's go-ahead before
   **increment 1 (walking skeleton)**: specs `functional/01` §2/§3/§7, `02` §1/§9, `technical/04` §1–§2,
   `05` §1–§4/§6, `08` §1–§3, `06` §3–§4; deliver the migration runner + `0001_init`, owner bootstrap +
   login + sessions + lockout, the full middleware chain, the empty three-pane shell, the `money.go`
   banker's-rounding engine call rendered as `−635,00 €`, `/healthz`. Demo **D1**.

## Open points

- **O-1 — RESOLVED.** Go 1.26.4 installed; scaffold verified green locally (see Tests passing).
- **O-3 — RESOLVED.** `go.mod` pinned to `go 1.26.4`; Dockerfile `GO_VERSION=1.26` (I-007).
- **O-7 (note).** Linter/formatter config settled during verification (I-007): gci-only formatter +
  standalone gofumpt gate; `misspell` locale unpinned (British spelling); `http.Server` timeouts for
  gosec G112.
- **O-2.** `htmx` + web fonts + icons **not yet vendored** into `web/assets` — deferred to increment 1,
  where the shell template first loads htmx and its exact version can be pinned. The assets embed
  directive widens then. (DoD impact: none for inc 0; the verbatim design-system files that were required
  — econome.css/js — are in place.)
- **O-3.** `go.mod` declares `go 1.24` without a specific patch `toolchain` line; pin the exact installed
  patch version during step 1.
- **O-4 — RESOLVED.** CI pins `golangci-lint` to `v2.12.2` (the locally-verified version); Dependabot
  can bump it.
- **O-5.** Dockerfile `COPY go.mod ./` (no `go.sum` yet); add `go.sum` to the copy line once inc 1
  introduces the first dependency.
- **O-6.** Repo will be **public**; `specifications/` is gitignored and stays local only (I-006). A fresh
  clone of the public repo will not contain the specs / decision logs — intended. The local working tree
  retains them for the resume protocol.
- None of the above is a silent assumption; each is tracked here for the next run.
