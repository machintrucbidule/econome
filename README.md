# EconoMe

Self-hosted, multi-tenant, internationalised (FR/EN), single-currency (default EUR) personal-accounts
web application. It replaces two spreadsheets with one tool that preserves a specific method: a **monthly
forecast adjusted against actuals** (envelope budgeting with a residual-savings adjustment variable and
pending-transaction handling) plus a **monthly net-worth / savings tracker**.

> **Status: under construction (Stage 7 — core implementation).** The build proceeds increment by
> increment per `specifications/development-plan/01-phased-plan.md`. Bank import (DSP2/PSD2 via
> GoCardless) is **not** part of this build — it is specified in Stage 8 and implemented in Stage 9.

## Tech stack

- **Go** SSR monolith (`html/template` + **htmx**), single static binary.
- **SQLite** (WAL) via the pure-Go `modernc.org/sqlite` driver (cgo-free → clean cross-compilation).
- Money stored as **integer minor units**; all derived figures computed in a **pure engine** with
  banker's rounding — never stored, never floated.
- Four-layer architecture (handlers → services → engine ← domain; repo is the only SQLite importer),
  with the inward-only dependency rule enforced at build time by `depguard`.

## Repository layout

See `specifications/technical/02-repository-structure.md`. In short: `cmd/` (server + admin CLI),
`internal/` (the layers), `migrations/` (embedded forward-only SQL), `web/` (templates + assets),
`testdata/` (engine golden fixtures), `.github/` (CI). The agent's standing brief is `CLAUDE.md`;
per-increment build notes are under `docs/progress/`.

## Local development (Windows 11)

Requires a local Go toolchain (latest stable): `winget install GoLang.Go`.

```bat
scripts\start.bat   :: builds econome.exe if needed, runs it on http://localhost:8765 (data in .\data)
scripts\stop.bat    :: stops it
```

`ECONOME_BEHIND_TLS=0` is set locally so cookies work over `http://localhost`. The offline recovery CLI
builds as `econome-admin.exe`.

## Quality gates

`gofumpt` · `go vet` · `golangci-lint` (incl. `gosec`, `exhaustive`, `depguard`) · `go test -race`
(unit + integration + e2e, ~90 % engine coverage) · `govulncheck` · multi-arch build. All run in CI on
every PR and on `main`; see `specifications/guardrails/`.

## Deployment (production)

Multi-arch image published to **GHCR** on a release tag (`vX.Y.Z` + a moving `stable` channel that
**Watchtower** follows). Deployed via Portainer/Watchtower with the SQLite database + secret on a
**mounted named volume** (the rule that keeps data safe across image updates). TLS is terminated by an
upstream reverse proxy; the app listens HTTP on `:8765`. Configuration is via `ECONOME_*` environment
variables (`.env.example`). On startup the app takes a pre-migration backup, applies migrations
transactionally, and aborts on failure with the backup intact. See
`specifications/technical/07-deployment.md`.

## Documentation

The full specification set lives under `specifications/` — foundations, functional specs, validated
mockups, technical specs, engineering guardrails, the development plan, and the decision logs
(`M`/`T`/`G`/`P`/`I`). Precedence on overlapping topics: technical > functional > foundation; mockups are
visual reference; guardrails govern process; the development plan governs build order.
