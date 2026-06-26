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

**Increment 5 (month-initialisation assistant) — DONE & MERGED** (PR #22). The first
feature to **consume the pure engine through a screen**: `/month-init?period=&scope=` computes the editable,
**non-persisted** draft (start cards, posts table, residual encart) recomputed **server-side by the engine**
on each leaf edit (no client computation, I-025); "Créer le mois" (`POST /month-init`) materialises
**allocations + awaited transactions + the `period` row** (`state=active`) + a `create` `period_event` in
**one transaction** (refuses an already-created period, I-027) then redirects. Materialisation: fixed
expense/income → allocation + awaited txn; variable → allocation only; fixed transfer → awaited transfer only
(dest set, no allocation); residual → nothing. `engineInputs`/`startBalances` (I-018/I-026) is the reusable
engine-assembly seam (inc 6/7 reuse it). **T11**: `envelope.dest_account_id` added (migration `0007`,
additive) so a transfer envelope stores its destination — the Enveloppes config gained a dest picker (current
account ≠ source, service-validated). M26 rail scope filters the draft. `Service` now wires `PeriodEvents`.
Decisions **T11**, **I-025..I-028**. See `docs/progress/0005-month-init.md`.

**D2 checkpoint held** (running build, default port now **`:8765`**). Post-merge fixes shipped while the user
tested, each its own merged PR: **#23** default listen port `:8080`→`:8765` (**I-029**); **#24** restored the
`/setup`+`/login` card styling (the auth-layout CSS was never ported from the `login.html` mockup into
`web/assets/econome.css` — regression test added); **#25** password **min length 12→8** (**M27**, supersedes
A8's length) + wired the home shell's dead "Configuration" nav link to `/config/parameters`; **#26** the money
parser accepts a `.` as decimal in fr-FR when unambiguous, fixing a ×100 (**I-030**). Added
`scripts/clean.bat` (fresh DB, with confirmation).

**Increment 6 = Forecast + Journal + reconciliation orchestration** (Milestone M2) is being delivered as
**4 small sequential PRs**; demo **D3** follows 6d.

**6a (Forecast read-only) — DONE** (all gates green; awaiting the user's go-ahead before 6b). The forecast is
now the **budget landing** at `GET /{$}` (replacing the retired `home.html` placeholder — **O-20 resolved**):
the envelope hierarchy with **5-state leaf badges + rolled-up parent badges** (M2), the right insights panel
(figures + savings encart + à surveiller), the **server-rendered treasury-timeline SVG** (M17), the 3 scope
variants (sweep/carry/aggregated), the read-only transaction drill-down (D2), and the not-created/empty/locked
states. New: `services/forecast.go` (read-model via the `engineInputs` seam), `view/forecast.go` (+ the SVG
renderer), `handlers/forecast.go`, `web/templates/forecast.html` (the real CSP-clean budget shell). Fixed a
latent increment-5 sign bug (**I-031**): internal-transfer txns are now stored source-signed (negative) so the
engine's balances/timeline/carry-funding figure are correct. Independent spec review of the read-model + sign
fix: **no correctness bugs**. See `docs/progress/0006-budget-core.md`.

**6b (Forecast inline edit) — DONE** (all gates green; awaiting go-ahead before 6c). The forecast is now
interactive: the inline `Prévu` edit (`PATCH /allocations/{env}`, envelope-keyed upsert **I-032**) with live
server-side recompute (the screen is decomposed into id-stable OOB fragments — `fc-row`/`fc-total`/
`fc-figures`/`fc-panel`/`fc-timeline`; a PATCH returns the edited row + OOB parent/total/panel/figures per the
recalc matrix, figures included for the §4a residual-negative → red Point bas coupling, timeline correctly
omitted); the "Virer en fin de mois" sweep (`POST /transfers/end-of-month` → a cleared sweep→cascade-target
transfer of `to_save`, source-signed negative I-031, disabled when to_save≤0/cascade-full/locked); and the
locked-month guard on every mutation via `ensureEditable` (→409). See `docs/progress/0006-budget-core.md` (6b).

**Next: 6c** Journal (quick-entry, whole-cell inline edit, sort/filter, transfer rows, atomic delete L8;
`functional/06`, `04` §3.5, `technical/04` §3.3). Then **6d** reconciliation orchestration via the pure
`engine.Reconcile`/`PairTransfer` (edit-in-place, no duplicate L6, variance→residual) + `label_mapping`
autocomplete + `ui_preference` expand — **mandatory subagent review on the reconciliation path**, then
close-out + D3. Open points **O-16** (no opening-balance column), **O-17** (snapshots-at-init for
cascade-full), **O-18** (sweep start≈0 depends on the close increment's sweep txn — the 6b "Virer" is the
manual sweep; inc 8's close reuses it), **O-19** `e2e chrome smoke` flaky, **O-21** (savings accounts not yet
in the forecast rail — deferred to inc 7), **O-22** (inline `Prévu` edit is per-account scope only; aggregated
rows read-only).
(Increments 0–4 done: scaffold; the walking skeleton — owner setup → login → shell → logout, sessions/
lockout/CSRF, migrations-with-backup, htmx, `money.go` → `−635,00 €`; the sealed pure engine + reconciliation
at 91.7 %; the full budget schema + `user_id`-scoped repos + fakes; and both configuration screens
(Paramètres + Enveloppes, combined category+envelope CRUD I-021, SortableJS cascade, I-021..I-024).)

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
