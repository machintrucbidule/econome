# Increment 7 — Net worth (Synthèse + Registre) — Milestone M3

**Date.** 2026-06-27 · **Status.** complete, all gates green; **demo D4** to be presented; awaiting the
user's go-ahead before increment 8. Delivered as **one PR** (user's choice, `G15`).

The Patrimoine group end to end: a **Synthèse** (selected-month asset snapshot) and a **Registre**
(full month-by-month history + evolution curve), reproducing `mockups/networth.html` /
`mockups/register.html`. Independent of increments 4–6 (depends on inc 1 shell, inc 2 net-worth engine,
inc 3 snapshot/`networth_month` repos). Derived-not-stored: only `gross_value` snapshots and the
per-month comment are inputs; PEA net, subtotals, the total and every delta are the **pure engine's**
output, recomputed on read. **L7 — snapshots and comments are always editable, independent of the budget
month lock** (the one path that bypasses the lock).

## What was built

- **Service read-models + mutations** (`internal/services/networth.go`).
  - `NetWorthSynthesis(userID, period)` → the per-support table (each savings account's gross, the PEA
    gross + derived net, the livrets subtotal, the total, every Δ), the four-card data, and the month
    comment (+ the M25 auto-prefill movements when enabled). `Register(userID, range, period)` → the full
    history table (one `engine.NetWorth()` pass per recorded month, most-recent-first) + the range-clipped
    multi-series evolution curve (M24/D3). Both build a dedicated **`networthInputs`** (accounts + settings
    + **all** snapshots) and vary `in.Period` — the engine is consumed unchanged.
  - Mutations (**all L7 — no `ensureEditable`**): `UpsertSnapshot` (validate `gross ≥ 0` 422; account must
    be the user's savings account else 404/422; upsert by (account, period)), `DeleteSnapshot` (next
    month's Δ recomputes on read), `SaveComment` (upsert `networth_month`). `SavingsAccounts` /
    `RailAccounts` for the rails.
  - M25 prefill ranking (`movements`, I-036): per-support Δ ranked, intensity by relative magnitude.
- **Full-history repo readers (I-035, additive — no migration).** `SnapshotRepo.ListByUser` +
  `NetworthMonthRepo.ListByUser` (real `lifecycle.go` + fakes); `NetworthMonthRepo` wired into
  `services.Service`/`Deps`/`New` + `cmd/econome` + both test harnesses.
- **View + curve SVG** (`internal/view/networth.go`). `NetWorthView` / `RegisterView` + cards/lines/rows
  structs; `RenderNetWorthChart` — the server-built multi-series evolution curve (grid + k€ Y labels +
  one polyline per support + the emphasised total + end dot/label + sparse month X labels), same pattern
  as `RenderTimeline`. Money stays integer; geometry is presentation float.
- **Handlers + routes** (`internal/handlers/networth.go`, `server/routes.go`). `NetWorthGet`,
  `SnapshotUpsert` (`POST /snapshots`), `SnapshotDelete` (`DELETE /snapshots/{id}`), `CommentPut`
  (`PUT /networth/{period}/comment` → 204), `RegisterGet`, `RegisterChart` (`GET /register/chart` →
  `frag:nw-chart`). A snapshot edit returns `frag:nw-table` + OOB `frag:nw-cards` (I-035). The four-card
  selection (I-037) + livret cap subtext live here.
- **Templates** (`web/templates/networth.html`, `register.html`). Reproduce the mockups (metrics,
  `.ptable` with dashed-underline editable cells, `.commentbox`, `.rtable`, the curve, empty/no-savings/
  no-history states). Shared `nw-rail` partial. id-stable fragments `nw-cards`/`nw-table`/`nw-chart`.
  CSP-clean: snapshot whole-cell edit + Registre comment cell via `app.js` `data-action` delegation
  (`nw-edit`/`nw-comment`); the Synthèse comment textarea + the range seg use htmx attributes directly.
- **Rail O-21** (I-038). Forecast + journal rails gain an **Épargne** section (savings → `/networth`,
  name + link, no extra query); the Patrimoine nav now points to `/networth` on every screen (was `/`).
- **Assets.** `app.js` `nw-edit`/`nw-comment` (CSP-clean, CSRF via `_csrf`/`X-CSRF-Token`). `econome.css`
  gains the promoted patrimoine classes (`.metrics`/`.ptable`/`.sub-row`/`.tot-row`/`.ann`/`.commentbox`/
  `.dot`/`.pos2`/`.neg2`/`.nul2`/`.rtable`/`tr.cur`/`.rfoot`/`.rangeseg`).
- **i18n.** `networth.*` + `register.*` keys in both catalogs (FR/EN parity green — `TestCatalogParity`).

## Decisions (this run)

**I-035** (full-history readers + (account, period)-keyed snapshot upsert; empty value deletes;
`nw-table` + OOB cards; L7 no lock guard), **I-036** (M25 intensity bucketing — relative magnitude,
presentation layer, retune at D4), **I-037** (metric-card selection + livret cap subtext, retune at D4),
**I-038** (O-21 rail Épargne section, name + link). See `specifications/implementation/decision-log.md`.

## Specs satisfied

`functional/07` (whole — Synthèse A, Registre B, cross-notes C), `functional/04` §3.6 (L7 snapshot
create/edit/delete, always editable), `rules` §12 (PEA net) / §13 (net-worth total, livrets subtotal,
deltas), `technical/04` §3.4 (the six routes), `technical/03` §4.3/§4.4 (`savings_snapshot` /
`networth_month`, consumed; no schema change). The engine is consumed unchanged.

## Tests passing

- **Service integration** (`internal/services/networth_test.go`, real SQLite, pinned clock): figures +
  deltas match the engine (PEA net 1 076 400, subtotal, total 2 496 400, total Δ 102 800); earliest month
  → deltas undefined; **delete recomputes the following month's delta (L7)**; **snapshot editable while
  the budget month is LOCKED (L7)**; one comment shared by Synthèse + Registre; validation (gross < 0 →
  422, current account → 422, cross-tenant → 404, bad period → 422); Register ordering + curve series +
  range; **M25 prefill** (PEA +++ / Livret A ++ ranking; off-by-default → no movements).
- **e2e backbone** (`internal/server/networth_e2e_test.go`, httptest+goquery): `/networth` renders
  cards+table+comment; `POST /snapshots` → `nw-table`+`nw-cards` fragments; a prior month → the +200,00 Δ;
  comment saved on Synthèse appears on the Registre (shared); `/register` renders the curve+table+range;
  `/register/chart?range=6` → the chart fragment; Registre empty state. CSS regression
  (`TestNetworthStylesheetDefinesClasses`).
- **chromedp smoke** (`-tags chromedp`, **passes locally**): the Synthèse whole-cell snapshot edit opens
  an input (app.js), commits via htmx, and the metric cards recompute live; the Registre curve renders an
  `<svg>`.
- Full suite + `gofumpt -l` + `go vet` (incl. `-tags chromedp`) + `golangci-lint` (depguard/gosec/
  exhaustive — **0 issues**) + `govulncheck` (clean); engine-coverage gate **91.7 %** holds (engine
  untouched).

## Verification (G8)

Conformance checklist passed: derived-not-stored (only gross snapshots + the comment are written; every
figure recomputed on read); tenant scoping (`user_id`-scoped repos; cross-tenant account → 404); integer
minor units + banker's rounding, no float on money (SVG geometry floats are presentation only); exhaustive
enum `switch` (account type, with the no-card current/employee case explicit); **recalc matrix**: a
snapshot edit re-renders the table + cards and the Registre Δ chain updates on read; delete recomputes the
next month's Δ; **L7 — no lock guard on any net-worth mutation** (asserted under a locked period); DSP2 seam
untouched (no transaction/reconciliation change). Engine/reconcile/auth are **not** touched → the mandatory
targeted subagent review is **not** required for increment 7; an optional review of the read-models vs
`rules §12–§13` + L7 may be run.

## Exact next step

**Increment 8 — Lifecycle, full auth, hardening (Milestone M4, release-ready)** per
`development-plan/01-phased-plan.md`: month close/unlock (L1) with the pre-close `to_save` sweep (O-18),
regenerate-missing-recurring (L9), the remaining auth surface (2FA enable/disable + backup codes, password
change, active sessions, admin users/invitations — `functional/01` §4–§8), the security regression suite,
and the pre-release hardening pass. Demo **D5** + the M4 pre-release pass follow; then Stage 7's final
deliverable — author `specifications/prompts/stage-8-dsp2-import-spec.md`. **Awaiting the user's go-ahead**
(`G15`) and the **D4** running-build demo.

## Open points

- **O-21 RESOLVED** (I-038): savings accounts gain the Patrimoine destination; the budget rails list them.
- **O-25 (M25 thresholds + card rule provisional).** The intensity bands (I-036) and the metric-card
  selection (I-037) are chosen defaults — **to confirm/retune with the user at demo D4**.
- **No Synthèse per-row delete button.** Deletion (L7) is reachable by **clearing a value cell to empty**
  (→ `DELETE /snapshots/{id}`); a number incl. `0` is a valid gross. The mockup has no delete affordance,
  so none was invented; the `DELETE` route is service- + e2e-tested.
- The Registre curve assigns securities a dashed stroke and cycles a small palette for other supports
  (the mockup's exact per-support colours are illustrative); legend reflects the actual series.
- **O-16/O-17/O-18/O-19/O-24** carry forward unchanged (opening-balance column; snapshots-at-init for
  cascade-full; the close increment's sweep txn — now due in inc 8; chrome-smoke flake mitigated;
  `PairInternalTransfer` one-row-vs-two-leg, DSP2-only).
