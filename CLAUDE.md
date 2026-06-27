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

**Milestones M0–M2 DONE.** Increments 0–4: scaffold; walking skeleton (owner setup → login → shell → logout,
sessions/lockout/CSRF, migrations-with-backup, htmx); the **sealed pure engine + reconciliation** (91.7 %);
the full budget schema + `user_id`-scoped repos + fakes; both configuration screens (Paramètres + Enveloppes,
I-021..I-024). **Increment 5** (month-init assistant, PR #22): the non-persisted engine-computed draft →
"Créer le mois" materialises allocations + awaited txns + the `period` row in one tx; `engineInputs`/
`startBalances` (I-018/I-026) is the reusable engine seam (inc 6/7 reuse it); **T11** `envelope.dest_account_id`
(migration `0007`). D2 post-merge fixes #23–#26 (I-029/I-030, M27).

**Milestone M2 (Budget core) — COMPLETE; demo D3 held.** Increment 6 delivered as **4 PRs**:
- **6a** (#28) Forecast read-only — the **budget landing** at `GET /{$}` (retired `home.html`, **O-20**): the
  hierarchy with 5-state leaf + rolled-up parent badges (M2), the right panel, the server-rendered
  treasury-timeline SVG (M17), 3 scope variants, drill-down, states. Fixed a latent inc-5 transfer sign bug
  (**I-031**: internal transfers stored source-signed negative).
- **6b** (#29) Forecast inline `Prévu` edit (`PATCH /allocations/{env}`, **I-032**) with live OOB recompute
  (id-stable fragments `fc-row`/`fc-total`/`fc-figures`/`fc-panel`/`fc-timeline`; §4a residual-negative → red
  Point bas); "Virer en fin de mois" sweep (`POST /transfers/end-of-month`, `to_save` → cascade target).
- **6c** (#30) Journal — quick-entry (`POST /transactions`, `econome.js` custom selects, CSP-clean inert-JSON
  option blocks); whole-cell inline edit (`PATCH /transactions/{id}`, date↔status §4, M23 transfer scope);
  server-side sort/filter (`GET /journal/rows`, `f`-prefixed + `filtered=1`); month summary; atomic delete L8.
  Decisions **I-033**.
- **6d** reconciliation seam (`services/reconcile.go`: `ReconcileCleared`/`PairInternalTransfer` wrap the pure
  `engine.Reconcile`/`PairTransfer` — built + tested + **mandatory review**, **not** wired into manual
  auto-matching per the user/spec; DSP2 wires it later) + **label autocomplete** (M21, learned `label_mapping`
  + embedded top-N + `emAutocomplete`; `/api/labels` deferred) + **expand persistence** (M4, `PUT /ui/expand`;
  **O-23 resolved** — forecast toggle now `frow`/`fchev`, `app.js` sole toggler). Decisions **I-034**.
  See `docs/progress/0006-budget-core.md`.

**Next: increment 7 — Net worth (Synthèse + Registre)** (Milestone M3; `functional/07`, `04` §3.6 L7, `rules`
§12–§13, `technical/04` §3.4, `technical/03` §4.3/§4.4): metric cards, the editable snapshot table (PEA net /
subtotal / total / Δ derived live), the evolution curve + history, snapshots **always editable independent of
the budget lock** (L7), the per-month comment. Routes `GET /networth`, `PATCH/POST/DELETE /snapshots`,
`PUT /networth/:period/comment`, `GET /register`, `GET /register/chart`. **Independent of inc 4–6** (depends on
inc 1 shell, inc 2 net-worth engine, inc 3 snapshot/`networth_month` repos). Demo **D4** follows; resolves
**O-21** (savings gain the Patrimoine destination). **Awaiting the user's go-ahead** (`G15`). Carried open
points: **O-16** (no opening-balance column), **O-17** (snapshots-at-init for cascade-full), **O-18** (the
close increment's sweep txn), **O-19** (`e2e chrome smoke` flake, mitigated by `WSURLReadTimeout`), **O-22**
(inline `Prévu` edit per-account scope only).

> Reminders: `main` is protected — all changes via PR → CI green → merge; required checks now include
> `e2e chrome smoke` (O-7 resolved). Dependabot minor/patch auto-merge on green, majors manual (I-008).
> Local-dev: add `C:\Program Files\Go\bin` + `%USERPROFILE%\go\bin` to PATH; `go test -race` needs cgo (runs
> in CI on Linux) so locally use `go test ./...`; `chromedp` smoke needs `-tags chromedp` + local Chrome.
> Engine golden fixtures regenerate with `go test -run Golden -update ./internal/engine`.
> **Vendored JS is not scanned by `govulncheck`** (Go-only): `web/assets/sortable.min.js` (SortableJS,
> pinned 1.15.6, I-022) and `htmx.min.js` (I-009) must be bumped manually / via Dependabot, not relied on
> for vuln alerts. **CSP** (`technical/05` §10) forbids inline JS — all in-app screens are built CSP-clean
> (native controls + htmx attributes + `web/assets/app.js` delegation off `data-action`); never add inline
> `onclick`/`<script>` to a template (I-024).

> **Repo note (I-006).** The published GitHub repo is **public** but `specifications/` is **gitignored**
> (local-only) — the design dossier + decision logs are not pushed. They remain on this working tree, so
> this resume protocol still reads them from disk; `git status` will correctly show `specifications/` as
> untracked. The app has no runtime dependency on it (design system copied into `web/assets/`).
