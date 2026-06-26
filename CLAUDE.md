# CLAUDE.md — EconoMe standing brief

> The implementing agent's documentation-of-record (`G7`, `guardrails/02` §2.1). Kept **accurate**, not
> appended to. Read this first every run, then the latest `docs/progress/` entry, then re-ground in the
> specific spec sections the current increment touches. This file is working memory — the authoritative
> record of any decision is the relevant `M`/`T`/`G`/`P`/`I` log.

## What EconoMe is

Self-hosted, multi-tenant, internationalised (FR/EN), single-currency (default EUR) personal-accounts
web app. Replaces two spreadsheets: a **monthly forecast adjusted against actuals** (envelope budgeting
with a residual-savings adjustment variable + pending-transaction handling) and a **monthly net-worth /
savings tracker**. Go SSR + htmx monolith, SQLite, single deployable. Bank import (DSP2/PSD2 via
GoCardless) is **specified in Stage 8, built in Stage 9 — not now**; keep it forward-compatible.

## Resume protocol (every run, before writing code)

1. Read this file.
2. Read the latest entry under `docs/progress/`.
3. Read `specifications/development-plan/01-phased-plan.md` (the increment list) and inspect git state to
   find the last completed and next increment. **Git state + progress log are authoritative; if they
   disagree, trust git and reconcile the log.**
4. Re-ground: re-read only the **specific** functional/technical sections the next increment touches;
   restate its goal + the invariants it protects.
5. Confirm to the user which increment you are resuming, then build it to its Definition of Done.
6. **Stop at the increment boundary** for the user's explicit go-ahead (`G15`). Never run two increments
   unattended.

## Spec map (precedence: technical > functional > foundation; mockups = visual only; guardrails govern
process; development-plan governs order)

| Area | Owner |
|---|---|
| Behaviour, calc rules, data-model draft, glossary | `specifications/foundations/` (fallback only) |
| Roadmap / stage governance | `foundations/roadmap.md` |
| Detailed behaviour (per screen + cross-cutting) | `specifications/functional/00..10` + `decision-log.md` (`A/N/S/C/L/D/M`) |
| **The calculation engine (normative)** | `functional/03-calculation-rules.md` (states C2/C7/C8, residual C1/C4, low point C3/C9, rounding C6) |
| Entities + lifecycle + reconciliation path | `functional/04` (L1–L10, §6 recalc matrix, §7 reconcile) |
| Visual reference (reproduce verbatim) | `specifications/mockups/` + `README.md` design system |
| Technical shape (authoritative for the build) | `specifications/technical/00..09` + `decision-log.md` (`T`) |
| Stack / pins | `technical/01` · Repo layout / boundaries | `technical/02` · Data model | `technical/03` |
| API contracts + middleware | `technical/04` · Auth/security | `technical/05` · i18n/currency | `technical/06` |
| Deployment | `technical/07` · Migrations | `technical/08` · **Testability seam** | `technical/09` |
| Process/quality | `specifications/guardrails/00..04` + `decision-log.md` (`G`, incl. `G15` pacing) |
| Build order (the execution script) | `specifications/development-plan/00..05` + `decision-log.md` (`P`) |
| **Implementation decisions (this stage)** | `specifications/implementation/decision-log.md` (`I`) |

## Load-bearing invariants (never let these drift)

- **Multi-tenant isolation.** Every entity carries `user_id`; scoping enforced centrally (middleware)
  **and** at the repository layer (defence in depth). Cross-tenant ⇒ **404, never 403**.
- **Derived-not-stored.** Allocations, transactions, snapshots are the only inputs. Every figure
  (envelope real/remaining/%/status, balances, savings projected/secured, to_save, cascade, both
  overdraft alerts, pea_net, net-worth totals/deltas, low point) is computed in the **pure engine** on
  read — never stored, never cached.
- **Money.** Integer **minor units** stored; derived values rounded to the cent with **banker's rounding**
  (round-half-to-even); rates as **basis points**; single active currency (default EUR); amounts never
  converted on a currency change. **No float ever touches money.**
- **Domain rules.** Residual-savings model; transfer neutralisation; date/period decoupling; **five
  envelope states (expenses only)**; `real = cleared + pending` (C7); secured basis setting (C1);
  treasury excludes unspent variable budget (C9).
- **Codes vs display.** English internal codes for every enum; only labels localised (FR/EN); regulated
  proper nouns verbatim (Livret A, LDDS, PEA).
- **Purity.** `internal/engine` + `reconcile.go` are pure — no I/O, clock, locale, randomness; the clock
  is the injected `today`. Enforced by `depguard` (`.golangci.yml`).
- **DSP2 forward-compat.** The reconciliation path the manual flow uses is the exact path import will
  drive; anticipatory schema fields stay; nothing forces import rework (`development-plan/05`).

## Architecture & boundaries (inward-only; `depguard`-enforced)

`handlers/server` (HTTP/htmx/templates) → `services` (use-cases, DB tx, lifecycle guard, reconciliation
orchestration) → `engine` (PURE) ← `domain`; `repo` (only SQLite importer, every method `user_id`-scoped).
`view`+`i18n` own all formatting; the engine never formats. See `internal/*/doc.go` for each boundary.

## Conventions

- Style/linters/error-handling/templates/SQL: `guardrails/01` (gofumpt, golangci-lint v2 with gosec /
  exhaustive / depguard; typed `422` validation, sentinels `ErrNotFound`→404 / `ErrLocked`→409; one
  central error→(status,fragment) mapper).
- Tests: `guardrails/03` (engine property `rapid` + golden `testdata/`; reconciliation matrix; service
  integration on real SQLite; middleware; httptest+goquery e2e + chromedp smoke; ~90% engine coverage;
  security regression suite).
- Commits: Conventional Commits, trunk-based on protected `main` (`G13`). Code/docs/commits in **English**;
  discussion with the user in **French**; example data fictional only.
- **Pacing (`G15`).** A user question ⇒ answer and stop; nothing decided until the user explicitly
  chooses. Every question in plain language **with the concrete impact** of each option. Work in visible
  increments; stop at each boundary for go-ahead.

## Commands

> Requires a local Go toolchain (latest stable, pinned in `go.mod`). Install: `winget install GoLang.Go`.

| Task | Command |
|---|---|
| Format | `gofumpt -w .` (CI checks `gofumpt -l .`) |
| Vet | `go vet ./...` |
| Lint (incl. depguard/gosec/exhaustive) | `golangci-lint run` |
| Test (race + coverage) | `go test -race -coverprofile=coverage.out ./...` |
| Engine coverage (gate from inc. 2) | `go test -cover ./internal/engine/...` (~90%) |
| Vuln scan | `govulncheck ./...` |
| Build local (Windows) | `go build -o econome.exe ./cmd/econome` then `scripts\start.bat` |
| Build admin CLI | `go build -o econome-admin.exe ./cmd/econome-admin` |
| Image (multi-arch, local) | `docker buildx build --platform linux/amd64,linux/arm64 .` |
| Targeted subagent review | run an independent agent over `internal/engine` / `reconcile` / `internal/auth` against the spec text (`G8` §3.2) — mandatory on those three surfaces |

## Current state

**Increment 0 (environment & repository setup) — DONE.** Scaffold, tooling, CI, and context artefacts
created and verified; repo published (public) at https://github.com/machintrucbidule/econome with
`main` protected and CI green; Go pinned 1.26.4. Decisions I-001..I-007. See
`docs/progress/0000-increment-0.md`. **Next: increment 1 = walking skeleton**
(`development-plan/02-walking-skeleton.md`) — awaiting the user's go-ahead. Note: `main` is protected, so
all further changes land via **PR** (branch → CI green → merge).

> **Repo note (I-006).** The published GitHub repo is **public** but `specifications/` is **gitignored**
> (local-only) — the design dossier + decision logs are not pushed. They remain on this working tree, so
> this resume protocol still reads them from disk; `git status` will correctly show `specifications/` as
> untracked. The app has no runtime dependency on it (design system copied into `web/assets/`).
