# Increment 5 — Month-initialisation assistant

**Date.** 2026-06-26 · **Milestone.** M1 · **Status.** DONE & MERGED (PR #22).
**Demo.** D2 held (running-build checkpoint, `G15`); user testing the build. See the **D2 checkpoint** section
at the foot of this file for the post-merge fixes, then the **exact next step** for increment 6.

The first feature that **consumes the pure engine through a screen**. The user prepares a not-yet-created
month as an editable **DRAFT** (per-account starting-balance cards, a posts table with editable leaf amounts,
a residual savings encart), recomputed **server-side by the engine** on every adjustment (T4/T5 — no
client-side figure computation), and on "Créer le mois" materialises **allocations + awaited transactions +
the `period` row** (`state=active`, `period_event=create`) in **one transaction**, then redirects to the
budget landing.

## User decision (this run)

**O-14 resolved — recurring internal transfers.** The user chose to **add a destination to the envelope
template** (vs. deferring or routing to the default account). Logged as **`T11`** in
`technical/decision-log.md` (additive nullable `envelope.dest_account_id`) **before** coding, per governance.

## What was built

- **Schema + repo (T11).** Migration `0007_envelope_dest_account` adds `envelope.dest_account_id INTEGER NULL`
  (additive, forward-only; no DB FK on `ALTER`, service-enforced — cf. I-011/I-020). `domain.Envelope.DestAccountID`;
  the SQLite `envelopeRepo` and the in-memory fake round-trip it.
- **Envelopes config extension.** A destination `<select>` appears for transfer-flow envelopes (CSP-clean
  `app.js` `adaptEnvelope` toggling `#w-dest` off `e-flow` change, I-024). Service validation (typed 422, no
  partial write): a transfer envelope **requires** a destination that is a **current** account `≠` source,
  same tenant; a non-transfer envelope **must** have none (`resolveDest` + the basic shape check in
  `validateEnvelope`). i18n keys (`env.f.dest*`, `validation.envelope.dest_*`) in both catalogs.
- **Engine inputs builder (`internal/services/engineinputs.go`).** The first place persisted data crosses
  into the pure engine: assembles `engine.Inputs` (accounts/categories/envelopes + the period's
  allocations/transactions/snapshots + `Params` from settings + `Today` = the injected clock as a clock-free
  `domain.Date`). `StartBalances` = the immediately-preceding **created** period's `projected_end` (carry
  semantics, sweep ≈ 0), else 0 (I-018/I-026); recursion bounded by the contiguous chain of created periods.
- **Month-init service (`internal/services/monthinit.go`).** `BuildDraft` (synthetic, unpersisted
  allocations/awaited txns over current config + overrides → engine figures), `CreateMonth` (one-tx
  materialisation + `period` + `period_event`; refuses an already-created period, I-027), `CurrentPeriod`,
  `IsCreated`. Materialisation rule: fixed expense/income → allocation + awaited txn; variable → allocation
  only; fixed transfer → awaited transfer only (no allocation, no category, dest set); residual → nothing.
  Signed amounts per I-017. Due-this-month: monthly always; non-monthly only on `due_months`. Archived
  envelopes/accounts excluded (L4). `Service` now also wires `PeriodEvents`.
- **Handlers + routes (`internal/handlers/monthinit.go`).** `GET /month-init?period=&scope=` (draft, or 303
  to `/` when already created, I-027), `PATCH /month-init/draft` (recompute → the `#mi-figures` fragment),
  `POST /month-init` (create → 303). Money parsed at the boundary (`i18n.ParseMoney`); central `mutationError`
  mapper (G3). M26 rail scope (aggregated / per current account) filters posts, start cards and the encart.
- **Templates + assets.** `web/templates/month-init.html` reproduces `mockups/month-init.html` (start cards,
  `.itable` posts table with `.gen-prevu`/`.gen-alloc` chips and the neutralised transfer row, the `.save`
  residual band + `-neg`/`-full`/carry-`note` variants, empty state) with its own scope rail. CSP-clean leaf
  edit (native input → htmx PATCH, no inline JS). Draft styles in `config.css`; `mi.*` i18n keys both
  catalogs (parity green).

## Specs satisfied

`functional/09` (whole), `functional/04` §3.3 (L2)/§3.4/§4 (month create, `period`+event), `rules` §5 (C5
start), §6 (income), §7 (residual projected/secured/negative), §8 (sweep/carry), §10 (transfer
neutralisation), §11.1 (negative-residual alert), `technical/04` §3.6, `technical/03` §3.3 (**T11**
`dest_account_id`)/§3.4/§4.1/§4.2. Decisions **T11**, **I-025..I-028**.

## Tests passing

- **Service integration** (`internal/services/monthinit_test.go`, real SQLite): create materialises 3
  allocations + 2 awaited txns (awaited/manual/no-date, signed) + the active `period` row + one `create`
  event in one tx; nothing persisted before create; already-created ⇒ `ErrConflict`; transfer generates an
  awaited transfer with the stored dest and **no** allocation; non-monthly only on a due month; archived
  envelope excluded; adjusted amount flows to both allocation and awaited txn. **Engine reuse**: draft
  residual = 95000 (2 600 − 1 050 − 600), override → −45000 negative. **Dest validation**:
  required/self/savings/on-expense → field 422; valid transfer round-trips the dest.
- **e2e backbone** (`internal/server/monthinit_e2e_test.go`, httptest+goquery): configure → draft renders →
  leaf PATCH recompute → negative-residual fragment → create 303 → already-created GET 303; rail scope shows
  the residual band (sweep) vs the carry note (carry).
- **chromedp smoke** (`-tags chromedp`): live residual recompute — dropping the income to 0 flips the
  `#mi-figures` band to negative (CSP-clean htmx PATCH). Compiles; runs in CI's e2e-chrome job.
- **Migration**: `0007` applies forward-from-empty and against a production-shaped DB copy; the new column is
  queryable. Schema version 6 → 7 (both migration tests updated).
- Full suite + `gofumpt -l` + `go vet` + `golangci-lint` (depguard/gosec/exhaustive) clean; `govulncheck`
  clean; engine-coverage gate **91.7 %** holds.

## Verification (G8)

Conformance checklist passed: derived-not-stored (draft persists nothing, T3i); tenant scoping on every new
query (`user_id` via the repos); integer minor units + banker's rounding, no float on money; exhaustive enum
`switch`; recalculation honoured (single `#mi-figures` OOB swap, I-028); locked-month guard intact
(`ensureEditable` present; CreateMonth refuses an existing/locked period); DSP2 seam intact (awaited
`source=manual`, nullable `op_date`). **Targeted subagent review** of the residual/start-balance/materialisation
path against `rules` §5/§7/§8/§10/§11.1: **all 7 checks PASS, no correctness bugs**.

## Exact next step

**Increment 6 — Forecast + Journal + reconciliation orchestration** (`development-plan/01-phased-plan.md`):
the Budget group end to end, including the DSP2-shared reconciliation orchestration calling the pure
`engine/reconcile.go`. The forecast becomes the screen that reads the shared month/scope context (the
month-init redirect target). Specs `functional/05`, `functional/06`, `functional/04` §3.4/§3.5/§6/§7, `rules`
§2–§11/§14, `technical/04` §3.2–§3.3. Mandatory subagent review on the reconciliation path. Demo **D3**
follows. Depends on inc 2 (engine+reconcile), inc 3 (repos), inc 5 (a created month with data).

## Open points

- **O-16 (opening balances).** No `account.opening_balance` column: a brand-new install cannot seed a
  non-zero start for a `carry` account before history accumulates (start = prior `projected_end`, else 0). If
  needed, that is a `technical/03` change (raise it there first). Out of scope for inc 5.
- **O-17 (snapshots-at-init for cascade-full).** `engineInputs` loads only the focus period's snapshots
  (`SnapshotRepo.ListByPeriod`). At month-init a new month usually has no snapshots, so cascade-full
  detection reads 0 balances and a vehicle at its ceiling in a *prior* month is not seen as full. Acceptable
  for the manual flow (savings balances are entered on the net-worth screen, inc 7); revisit if inc 6/7 needs
  a "latest snapshot ≤ period" read (a new repo method, not a schema change).
- **O-18 (sweep start≈0 depends on the close-increment's sweep txn).** `startBalances` carries the prior
  period's `projected_end`. For a **sweep** account this is ≈ 0 only once a sweep (residual→savings)
  transaction exists in the prior period; increment 5 has **no** month-close / sweep-transaction generation,
  so a multi-month chain of created sweep months would carry the prior month's full residual (salary
  included) into the next start. **Latent, not active** (no close flow yet ⇒ such chains can't arise through
  normal use). The **close increment (8)** must emit the sweep transaction that zeroes `projected_end`; it
  must not assume start-is-already-zero. The engine math here is correct; it depends on an input a later
  increment supplies. Surfaced by the subagent review.
- **Reaching the assistant.** Until the forecast (inc 6) offers "Préparer le mois" for a not-created month,
  `/month-init` is reached by URL; the e2e/demo navigate there directly. No new behaviour — the trigger
  affordance lands with the forecast.

## D2 checkpoint — held; post-merge fixes (all merged to `main`)

The increment-5 work was merged (PR #22) and the **D2 running-build demo** was presented. While the user
exercised the build, several issues surfaced and were each fixed in their own merged PR (trunk discipline,
CI green, no behaviour smuggled in — owning-layer logs updated first):

- **#23 — default listen port `:8080` → `:8765`** (**I-029**). `:8080` collided with another local service.
  Changed `config.defaultListen` + all references (`start.bat`, `.env.example`, `docker-compose.yml`,
  `Dockerfile`, `README`, `technical/07` §3). `ECONOME_LISTEN` override unchanged.
- **#24 — restored `/setup` + `/login` card styling.** The auth-layout classes (`.authstage`, `.auth-card`,
  `.fld`, `.pwrules`, …) lived in the `mockups/login.html` page `<style>` and were never ported into the
  shared `web/assets/econome.css` at increment 0, so the first-run page rendered unstyled. Ported them
  (CSP-clean) + added a regression test asserting the served CSS defines every auth selector the templates use.
- **#25 — password min length 12 → 8** (**M27**, supersedes A8's length) **+ home nav fix.** User's choice
  (A8 was flagged revisable). Updated the single validator, FR/EN strings, boundary tests, `functional/01` §9,
  `technical/05` §1. Also: `home.html` hard-coded all three nav links to `/` — wired "Configuration" to
  `/config/parameters` (Paramètres was unreachable from the landing shell).
- **#26 — money parser accepts `.` as decimal in fr-FR when unambiguous** (**I-030**). A French user typing
  `12.50` parsed as `1250` (×100) because `.` was dropped as grouping. `ParseMoney`/`ParsePercent` now infer
  the separator; ambiguous input still rejected; still exact integer minor units. `technical/06` §2 updated.
- **`scripts/clean.bat`** added — reset to a fresh DB (deletes `./data`, with a typed `YES` confirmation),
  alongside `start.bat`/`stop.bat`.

**New open points** (carried into `CLAUDE.md`): **O-19** `e2e chrome smoke` is **flaky** (Chrome
websocket-launch timeout; failed-then-passed on re-run for #24 and #25 — harden the launch/timeout or add a
retry). **O-20** the increment-1 placeholder `home.html` shell still uses inline `onclick` toggles that CSP
blocks (theme/panel buttons inert) — to be superseded when **6a** builds the real budget shell.

## Exact next step (updated)

**Increment 6 — Forecast + Journal + reconciliation orchestration**, delivered as **4 small sequential PRs**
(agreed with the user at the D2 checkpoint): **6a** Forecast read-only · **6b** Forecast inline edit + recalc
matrix + end-of-month transfer + locked-month guard · **6c** Journal (entry/edit/sort/filter/transfer/atomic
delete) · **6d** reconciliation orchestration (pure `engine.Reconcile`) + `label_mapping` + `ui_preference`,
**mandatory subagent review on the reconciliation path**, then close-out (`0006-budget-core.md`) + demo **D3**.
Specs: `functional/05`, `functional/06`, `functional/04` §3.4/§3.5/§6/§7, `rules` §2–§11/§14, `technical/04`
§3.2–§3.3, `technical/03` §3.4/§3.5/§5.1/§5.2. **Awaiting the user's go-ahead before starting 6a** (`G15`).
