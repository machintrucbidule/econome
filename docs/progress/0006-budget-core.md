# Increment 6 — Forecast + Journal + reconciliation orchestration (Milestone M2)

Delivered as **4 small sequential PRs** (agreed at the D2 checkpoint, `G15`): **6a** Forecast read-only ·
**6b** Forecast inline edit + recalc + transfer + locked guard · **6c** Journal · **6d** reconciliation
orchestration. Demo **D3** follows 6d. This file is appended per sub-increment.

---

## 6a — Forecast (Prévisionnel) read-only — DONE

**Date.** 2026-06-27 · **Status.** complete, all gates green; awaiting the user's go-ahead before 6b.

The first screen to read the shared month + account scope and render the engine's full per-month picture.
Per `functional/02` §6 the forecast **is the budget landing**, so it replaces the increment-1 placeholder
`home.html` shell at `GET /{$}` — resolving **O-20** (the placeholder's CSP-blocked inline `onclick`
toggles) by building the real, CSP-clean budget shell. The not-created empty state offers "Préparer le mois"
→ `/month-init`, closing the "reaching the assistant by URL only" gap.

### What was built

- **Read-model (`internal/services/forecast.go`).** `Forecast(ctx, userID, period, scope)` assembles a
  non-persisted `ForecastData` by calling the **pure engine** through the existing `engineInputs` seam
  (I-018/I-026). Builds the envelope tree from the category hierarchy (reusing the `EnvelopesOverview`
  shape: top-level leaves + parent groups), computing `engine.EnvelopeView` per leaf and rolling up parents
  (exact integer sums + most-severe child state `overrun > partial > expected > paid > none`, badge
  "agrégé", M2). Per scope: figures (`AccountBalances`), savings encart (`Savings`), treasury (`LowPoint` /
  `AggregateLowPoint`), "à surveiller". Aggregated scope is flat with account pills; single-account scope is
  the collapsible hierarchy with a read-only transaction drill-down (D2). Transfer + residual envelopes are
  excluded from table rows (rules §10). Everything derived-not-stored.
- **View + timeline SVG (`internal/view/forecast.go`).** `ForecastView` + the formatted row/figure/encart/
  watch/timeline view structs. `RenderTimeline` builds the **server-rendered treasury-timeline SVG** (M17):
  running-balance polyline + area + grid + event dots coloured by kind (income/debit/awaited/overrun) + the
  low-point/end marker + axis labels, from `engine.LowPoint.OrderedPoints`. SVG coordinates are presentation
  floats; money stays integer minor units.
- **Handler + route (`internal/handlers/forecast.go`, `server/routes.go`).** `ForecastGet` at `GET /{$}`
  (replaces `Home`/`ShellView`/`home.html`, all retired). Reads `period`/`scope` via the existing helpers;
  the five-state badge mapping (income = received-vs-expected, no overrun red, §4); the CSP-clean month
  navigator + picker (prev/next links, picker `pick=1` query state, month cells as `data-action="goto"`
  buttons).
- **Template + shell (`web/templates/forecast.html`, `partials.html`).** The real three-pane budget shell
  (rail with the live current-account scope list + aggregated; pinned header with `Prévisionnel | Journal`
  tabs + month navigator; right insights panel), reproducing `mockups/forecast.html` for the three scope
  variants. CSP-clean: row expand (`data-action="toggle-row"` → toggles `data-c` children / `data-d` drill +
  rotates the chevron), picker, rail/panel toggles all delegated in `app.js`. New `appscripts` partial
  (htmx + econome.js + app.js, no SortableJS). The Journal tab links to `/journal` (404 until 6c — user's
  choice).
- **i18n.** `forecast.*`, `state.*`, `txn.status.*`, `shell.collapse_rail/toggle_panel` keys in both
  catalogs (FR/EN parity green).

### Decision (this run)

- **I-031 — transfer sign correction.** A latent increment-5 bug: `monthinit.signedAmount` stored
  internal-transfer transactions **positive**, but the sealed engine treats a transfer's stored amount as
  money leaving the source (source −X / dest +X). No screen displayed balances until the forecast, so it
  never surfaced. Fixed `signedAmount` (transfer now negative, like an expense) + a regression assertion.
  Logged in `specifications/implementation/decision-log.md`; conforms to `I-017` + `rules §10`, no spec
  change.

### Specs satisfied

`functional/05` (whole, read-only parts), `functional/02` §2–§7 (shell, rail scope, tabs, month navigator,
landing §6), `functional/03` §3 (five states), §4 (income/transfer), §7–§9 (residual/balances/cascade),
§11.2/§14 (low point, aggregation), `technical/04` §3.2 (forecast read route). No schema change; the engine
is consumed, not extended.

### Tests passing

- **Service integration** (`internal/services/forecast_test.go`, real SQLite, clock pinned 2026-06-15):
  not-created state; leaf state/real/remaining/percent match `engine.EnvelopeView`; parent rollup = Σ
  children + agrégé badge; footer total = expense-only; transfer excluded; watch surfaces the overrun;
  the three scope variants (sweep encart + low point · carry note + projected-end + incoming transfer =
  +24000 · aggregated flat-with-pills + masked transfers + one encart per sweep).
- **e2e backbone** (`internal/server/forecast_e2e_test.go`, httptest+goquery): not-created landing →
  "Préparer"/`/month-init`; configure + create → forecast renders the table, total, figures, encart, the
  `<svg>` timeline; sweep scope → residual band + low point + drill-down; carry/aggregated variants.
- **chromedp smoke** (`-tags chromedp`): login → forecast shell renders (replaces the retired `#demo-balance`
  assertion); a leaf row expands its drill-down client-side (CSP-clean).
- **CSS regression** (`TestForecastStylesheetDefinesClasses`): every forecast design-system selector is in
  the served `econome.css` (guards the #24-class regression).
- Full suite + `gofumpt -l` + `go vet` + `golangci-lint` (depguard/gosec/exhaustive/misspell) + `govulncheck`
  clean; engine-coverage gate **91.7 %** holds (engine untouched).

### Verification (G8)

Conformance checklist passed: derived-not-stored (the read-model persists nothing); tenant scoping via the
`user_id`-scoped repos through `engineInputs`; integer minor units + banker's rounding, no float on money
(SVG geometry floats are presentation only); exhaustive enum `switch` (states, flow, scope, account type,
period state); recalc matrix N/A (no mutation in 6a); locked-month guard respected (read-only screen +
lockbar; `period.state` read but not written); DSP2 seam untouched. The mandatory engine/reconcile/auth
subagent review is **not** required for 6a (no change to those surfaces); an **optional** targeted review of
the read-model + the I-031 sign fix against `rules §3/§4/§7/§8/§10/§11.2/§14` was run.

### Exact next step

**6b — Forecast inline `Prévu` edit + live recompute + end-of-month transfer + the locked-month guard.**
`PATCH /allocations/:id` recomputing residual/alerts/`projected_end`/low point and returning the edited row
**+ the OOB fragments** per the recalculation-trigger matrix (`functional/04` §6); "Virer en fin de mois"
generating the transfer (`functional/05` §5, lifecycle §3.5); the locked-month read-only guard wired to the
edit affordances. Specs `functional/05` §5, `functional/04` §3.4/§6, `rules` §7/§11, `technical/04` §3.2.
The "Virer"/transfer button currently renders **disabled** in 6a — 6b wires its action. **Awaiting the
user's go-ahead** (`G15`).

### Open points

- **O-20 RESOLVED.** The placeholder `home.html` shell is retired; the forecast is the CSP-clean budget
  landing.
- **O-21 (savings accounts not in the rail).** 6a's rail lists `Tous` + current-account scopes only;
  `functional/02` §4 also lists savings accounts "for context" (clicking → Patrimoine). Deferred until the
  net-worth screen exists (increment 7) so the link has a destination. Not a behaviour gap (savings are not
  budget scopes).
- **O-19 (chrome smoke flaky)** still open (Chrome websocket-launch timeout); 6a adds one more chromedp
  test under the same job.
- The treasury timeline starts at `today` (the engine's low point is forward-looking from the injected
  clock, C3/C9), so for the current month the curve covers today→EOM rather than the full month; faithful to
  the engine, slightly less than the full-month mockup. Acceptable for read-only.

---

## 6b — Forecast inline edit + recompute + end-of-month transfer + locked guard — DONE

**Date.** 2026-06-27 · **Status.** complete, all gates green; awaiting the user's go-ahead before 6c.

Makes the forecast interactive while staying derived-not-stored: the inline `Prévu` edit with live
server-side recompute, the "Virer en fin de mois" savings sweep, and the locked-month read-only guard.

### What was built

- **Inline `Prévu` edit.** `PATCH /allocations/{env}` (envelope-keyed upsert, **I-032**) →
  `Service.EditAllocation`: `ensureEditable` (locked guard) + `planned ≥ 0` (typed 422) + residual-envelope
  rejection, then upsert via `allocations.ByEnvelopePeriod`→`Update`/`Create` in one tx. Each leaf/child
  expense **and income** row renders an editable `.amt-inp` on an **active** month (read-only text on a
  locked month / parent rollup / aggregated flat row; the residual envelope is never a row).
- **Recalc + OOB fragments** (`functional/04` §6, `technical/04` §3.2). The handler decomposed the screen
  into id-stable reusable fragments (`fc-row` leaf/child/parent/flat, `fc-total`, `fc-figures`, `fc-panel`,
  `fc-timeline`) used by both the full page and the PATCH response. PATCH returns the **edited row**
  (primary swap into `#fc-row-e{id}`) + **OOB**: the **parent rollup row** when a child is edited, the
  **footer total**, the **savings panel** (encart + à surveiller), and the **figures** — the figures
  re-render because a `Prévu` edit that drives the residual negative turns the **Point bas card red** to stay
  coherent with the red encart (`functional/05` §4a, new in 6b). The **timeline is correctly not re-sent**
  for an allocation edit (planned doesn't change transactions).
- **End-of-month transfer.** `POST /transfers/end-of-month?account=&period=` → `Service.EndOfMonthTransfer`:
  `ensureEditable`, then a **cleared** transfer dated today from the sweep account to its **cascade target**
  of magnitude `to_save` (realised residual), stored source-signed **negative** (I-031). Refused (409) when
  `to_save ≤ 0`, the cascade is full / has no target, or the month is locked. Returns the savings panel +
  OOB figures + timeline (the cleared transfer shifts balances + low point). The "Virer" button is live only
  on an active month with `to_save > 0` (else `disabled`); 6a's negative/cascade bands keep their disabled
  button.
- **Locked-month guard.** Every mutation funnels through the existing `ensureEditable` choke point
  (`services/accounts.go:25`) → `ErrLocked` → 409; the central `mutationError` maps it and `app.js`
  `allowErrorSwap` already swaps 409. Inputs/buttons render `disabled` when the month is locked.

### Specs satisfied

`functional/05` §5 (inline edit, end-of-month transfer) + §4a (residual-negative → red Point bas),
`functional/04` §3.4 (allocation edit), §6 (recalc matrix), §4 (lock guard, the end-of-month transfer of
`to_save`), `rules` §7/§9 (residual/to_save/cascade), §10 (transfer sign), `technical/04` §1/§3.2 (the OOB
hypermedia model). Decisions **I-031** (reused), **I-032**. No schema change.

### Tests passing

- **Service integration** (`forecast_test.go`, pinned clock): `EditAllocation` upserts + the projected
  residual recomputes (76000 → 36000) and the row reflects it; `planned < 0` → 422; residual envelope → 422;
  **locked period → ErrLocked**; `EndOfMonthTransfer` with no cascade target → 409; with a cascade passbook →
  a cleared sweep→livret transfer of **−111000**, and `to_save` realises to **0** afterwards; locked → 409.
- **e2e backbone** (`forecast_e2e_test.go`): inline `Prévu` PATCH returns the edited row + OOB
  `#fc-total`/`#fc-figures`/`#fc-panel` with `hx-swap-oob`, and raising an expense past income flips the
  encart to **"Solde insuffisant"**; the end-of-month route is wired + guarded (409 with nothing to sweep).
- **chromedp smoke** (`-tags chromedp`): inline `Prévu` edit fires the htmx PATCH and the savings panel
  swaps live to the negative state without reload.
- Full suite + `gofumpt -l` + `go vet` (incl. `-tags chromedp`) + `golangci-lint` + `govulncheck` clean;
  engine-coverage gate **91.7 %** holds (engine untouched).

### Verification (G8)

Conformance checklist: derived-not-stored (only the allocation/transfer rows are written; every figure is
recomputed on read); tenant scoping via `user_id` repos; integer minor units + banker's rounding, no float
on money; exhaustive enum `switch`; **recalc matrix honoured** (edited row + parent + total + panel + figures
on an allocation edit; panel + figures + timeline on the transfer; timeline correctly omitted on an
allocation edit); **locked-month guard on every mutation** (409, inside the tx, before any write); DSP2 seam
intact (the transfer is `source=manual`, the reconciliation path untouched; the allocation upsert adds no
transaction). Optional targeted review of the mutations + locked guard run.

### Exact next step

**6c — Journal** (`functional/06`): creation-only quick-entry (`POST /transactions`, status default
`cleared`, account-from-category server-side), whole-cell inline edit (`PATCH /transactions/:id`), sortable
columns / default date-desc, the right-panel month summary + filters, transfer rows (one two-legged row,
inline-edit scope M23, atomic delete L8). Specs `functional/06`, `functional/04` §3.5/§6, `technical/04`
§3.3. The forecast drill-down "Ouvrir dans le journal" + the Journal tab already link to `/journal`.
**Awaiting the user's go-ahead** (`G15`).

### Open points

- **Inline edit scope.** The inline `Prévu` edit is enabled in **per-account** scope (sweep/carry) for
  leaf/child rows; the **aggregated** flat rows stay read-only (the overview, not the edit surface). Income
  rows are editable too (their planned feeds the residual). No spec conflict; revisit if the user wants
  aggregated-scope editing.
- **End-of-month transfer idempotency.** Clicking "Virer" twice would create a second transfer, but after the
  first the realised `to_save` drops to ~0 so the button disables — a double-click transfers ~0. No explicit
  idempotency key; acceptable for the manual flow. The full close flow (increment 8) supersedes this as the
  primary path (O-18).
- The `POST /transfers/end-of-month` happy path is **service-tested** (needs cleared movements); the e2e only
  asserts the route + guard, because cleared transactions require the Journal (6c). The chromedp/e2e happy
  path lands once the Journal can realise movements.

---

## 6c — Journal (entry journal) — DONE

**Date.** 2026-06-27 · **Status.** complete, all gates green; awaiting the user's go-ahead before 6d.

The flat data-entry screen that feeds **all** actuals (`functional/06`). Delivered as **one PR** (user's
choice). Derived-not-stored: only transaction rows are written; the summary + every figure derive on read.

### What was built

- **Quick-entry bar (create-only, M20).** `POST /transactions` → `Service.CreateTransaction`: `ensureEditable`
  + validation (`amount ≠ 0`; category required unless transfer; transfer `dest ≠ account`), `flow` from the
  category (or transfer), **signed amount** via `signedAmount` (expense/transfer negative, income positive,
  I-031), `budget_period` from the date (date/period decoupling), explicit status (default `cleared`),
  account-from-category server prefill (I-033). The custom selects reuse the validated `econome.js` widgets;
  `selSet` ported to `app.js`; option sets delivered as inert `<script type="application/json">` (CSP-safe).
- **Table + server-side sort/filter.** `GET /journal` / `GET /journal/rows` (htmx body re-render + OOB
  summary). Sort (date/period/label/cat/acct/amount/status; **default date desc, undated awaited last**, M19);
  filters (search, category, status chips, transfers toggle, M18) — `f`-prefixed params + a `filtered=1`
  sentinel (I-033); view-only, never mutating. Columns `Date | Période | Libellé | Catégorie | Compte |
  Montant | Statut` with the `~JJ/MM`/`—` date display and the `Période` highlight when it differs from the
  date's month.
- **Whole-cell inline edit (M22).** `PATCH /transactions/{id}` (one field) → `Service.UpdateTransaction`:
  `ensureEditable` (source **and** the new period on a `budget_period` change), the **date↔status**
  consistency (date set → `cleared`, date cleared → `awaited`, §4), direct status edit, re-sign on an
  amount/category change, and the **transfer inline scope** (M23 — category/account fixed → 409). Wired
  CSP-clean: `app.js` opens the right widget on `data-action="j-edit"` and fires the htmx PATCH (row + OOB
  summary).
- **Right panel (M18).** *Résumé du mois* (revenus reçus, dépenses réelles = cleared+pending C7, en attente +
  count, attendu à venir + count, solde net — **transfers excluded**, rules §10) + *Filtres*.
- **Delete (L8).** `DELETE /transactions/{id}` → a manual transfer is a single two-legged row; deleting
  removes it (both balance legs). Row removed via `hx-swap="delete"` + OOB summary.
- **States + locked guard.** Not-created / empty / locked; every mutation funnels through `ensureEditable`
  (→409); quick-entry + inline edits render only on an active month.
- **CSS port.** The journal-only classes from the mockup's page `<style>` (`.jtable`/`.statpill`/`.catpill`/
  `.srt`/`.panel-card`/`.flab`/`.vtext`/`.stcell`/`.actcol`/`.chip-period`/`.ltext`/`.sk-row`) promoted into
  `web/assets/econome.css` + a regression test (guards the #24 class regression).

### Specs satisfied

`functional/06` (whole, minus the autocomplete deferred to 6d), `functional/04` §3.5 (transaction CRUD), §6
(recalc matrix), §7 (single-row date-fill reconciliation), `rules` §10 (transfer neutralisation),
`technical/04` §1/§3.3/§4. Decisions **I-033** (reuses I-031). No schema change.

**Scope boundary vs 6d.** 6c does the full CRUD + the **single-row** date↔status consistency. The
`engine.Reconcile` matching orchestration (a new movement finding its awaited twin), label **autocomplete**
(`/api/labels` + `label_mapping`), and `ui_preference` expand are **6d** — so the label field is plain text
here.

### Tests passing

- **Service integration** (`journal_test.go`, pinned clock): create (signed amounts, period-from-date,
  summary = income/real(cleared+pending)/pending+count/awaited+count/net, transfers excluded); validation
  (amount 0 / no category / transfer-self → 422; locked → 409); inline edit (date-fill→cleared,
  clear-date→awaited, status, amount re-sign, transfer scope 409); delete; sort (date desc, undated last) +
  filters (category/status/search).
- **e2e backbone** (`journal_e2e_test.go`): not-created state; created month renders the quick-entry + table
  + summary; quick-entry POST appends a row + OOB summary; inline status PATCH → row + summary; `GET
  /journal/rows` re-render; DELETE removes the row (CSRF via header — Go does not parse a DELETE body).
- **chromedp smoke**: quick-entry via the **CSP-clean custom category select** (`emMenu`) + `[+]` htmx create
  appends the row.
- **CSS regression** (`TestJournalStylesheetDefinesClasses`).
- Full suite + `gofumpt -l` + `go vet` (incl. `-tags chromedp`) + `golangci-lint` + `govulncheck` clean;
  engine coverage 91.7 % holds (engine untouched).

### Verification (G8)

Conformance checklist: derived-not-stored (only transaction rows written; summary/figures derived); tenant
scoping (userID from context; `user_id`-scoped repos; cross-tenant → 404); integer minor units + banker's
rounding, no float on money; exhaustive enum `switch` (status); recalc matrix honoured (row + OOB summary; the
forecast recomputes on its next read); **locked-month guard on every mutation** (source + target period on a
`budget_period` move, 409); **DSP2 seam intact** (manual single-row transfer + `source=manual` + the
awaited↔cleared single-row path are exactly what import will drive; the `engine.Reconcile` matching +
`external_ref` dedup land in 6d, no reshape). Targeted subagent review not mandatory for 6c (no
engine/reconcile/auth change).

### Exact next step

**6d — reconciliation orchestration** (`functional/04` §7, `06` §4): the service calls the **pure
`engine.Reconcile`/`PairTransfer`** (increment 2) to match a new cleared movement to its awaited twin
(edit-in-place, no duplicate L6, amount variance → residual), internal-transfer auto-pairing; plus
**`label_mapping`** (account-from-category prefill refinement + `GET /api/labels` autocomplete, M21) and
**`ui_preference`** (`PUT /ui/expand` per-user expand persistence, M4). **Mandatory subagent review on the
reconciliation path.** Then close-out (`0006-budget-core.md` final) + demo **D3**. Specs `functional/04` §7,
`functional/06` §4/§5, `technical/04` §3.3/§3.5, `technical/03` §5.1/§5.2. **Awaiting the user's go-ahead**
(`G15`).

### Open points

- **O-23 (forecast chevron double-wired).** The validated `econome.js` wires `.tog`/`.chev` row toggling at
  load (its own element listeners); the forecast's `app.js` `toggle-row` delegation (needed for htmx-swapped
  rows) **also** fires, so clicking the **chevron** specifically is a no-op (econome.js `stopPropagation`s it
  with no `data-k`) and a second close-click glitches. Row-**body** clicks work (the path 6a/6b/6c smokes
  use). Pre-existing since 6a (not introduced by 6c); a focused forecast follow-up should make `app.js`
  delegation the sole toggler (e.g. give the chev `data-k` for econome.js, or move the rows off the `.tog`
  class). The Journal is unaffected (its rows are not `.tog`).
- **Autocomplete + reconciliation matching deferred to 6d** (per plan): the label field is plain text; a
  quick-entry create does not auto-match an existing awaited row.
- **Account-from-category prefill** is the category's first active envelope account, not usage-ranked (I-033).
