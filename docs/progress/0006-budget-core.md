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
