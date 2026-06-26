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

## Increment 0 — DONE

- Repository **published**: https://github.com/machintrucbidule/econome (**public**; `specifications/`
  gitignored and confirmed not staged — I-006).
- **CI green on `main`** (run on commit `d55d9da`): format · vet · lint (depguard active) · test
  (race+coverage) · govulncheck · build, **and** the multi-arch image build (linux/amd64 + arm64). The
  GHCR publish job correctly skipped (no release tag).
- **Branch protection on `main`** (G13): PR required (0 approvals — solo self-merge), required checks
  `format · vet · lint · test · vuln · build` + `multi-arch image build (no push)` (strict/up-to-date),
  linear history, no force-push, no deletions, enforced on admins.
- One follow-up CI fix landed (`ci:`): removed an empty `misspell:` settings key that failed
  `golangci-lint config verify`.

## Exact next step (next run)

**Increment 1 — Walking skeleton** (`development-plan/02-walking-skeleton.md`), after the user's
go-ahead. Specs: `functional/01` §2/§3/§7, `02` §1/§9, `technical/04` §1–§2, `05` §1–§4/§6, `08` §1–§3,
`06` §3–§4. Deliver: the hand-rolled migration runner (pre-migration `VACUUM INTO` backup → transactional
apply → abort-on-failure) + `0001_init` (`user`, `session`, `settings`, `schema_migrations`); first-run
owner bootstrap + password login + opaque sessions + logout; lockout/throttle; the full middleware chain
(Recover→RequestContext→Session→AuthGuard→TenantContext→CSRF→Locale); the empty three-pane shell reusing
`econome.css/js`; the `money.go` banker's-rounding engine call rendered via view/i18n as `−635,00 €`;
`GET /healthz`. **Also vendor htmx + fonts/icons** into `web/assets` and widen the embed (O-2). Demo **D1**.

> First task next run: add `go.sum` to the Dockerfile `COPY` line when the first dependency
> (`modernc.org/sqlite`) is introduced (O-5).

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
