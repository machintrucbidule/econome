# Increment 4 — Configuration (Paramètres + Enveloppes)

**Date.** 2026-06-26 · **Milestone.** M1 · **Status.** DONE (both sub-PRs merged; CI green). Delivered as
two sub-PRs (I-023): PR-a Paramètres (#19), PR-b Enveloppes (#20).

## PR-a — Paramètres (`/config/parameters`)

The first configuration screen: account CRUD, settings (Épargne / Localisation / Préférences), the savings
cascade, with service-layer validation and the central error→fragment mapping. **No new behaviour or
schema** — all upstream.

### What was built

- **i18n money/rate boundary** (`internal/i18n/parse.go`): `ParseMoney`/`ParsePercent` — locale-aware,
  **integer-only** (no float), inverse of `FormatMoney`; `FormatAmount`/`FormatRate` for symbol-less input
  prefills. Errors `ErrEmptyAmount`/`ErrBadAmount`/`ErrRateRange` → field 422 (the rate-bound 422 of
  `functional/10` §3, never a DB 500). Property/table tested.
- **Services** (`internal/services/accounts.go`, `settings.go`; `Service` now takes a `Deps` struct):
  `ListAccounts`/`GetAccount`/`CreateAccount`/`UpdateAccount`/`ArchiveAccount`/`UnarchiveAccount`/
  `DeleteAccount`/`ReorderCascade`/`UpdateSettings`. Validation = typed `domain.ValidationError`, no partial
  write: name required + unique (UNIQUE→field 422, not raw 409), cross-column policy rule (current⇒sweep/
  carry, savings⇒none), ceiling ≥ 0, rate ∈ [0,1), amounts ≥ 0, cascade members are savings with unique
  `fill_priority` (two-phase clear→assign so the partial UNIQUE index never collides), default-account ref
  check, basis/theme/lang/currency enum checks. **L3 forward-only** `month_end_policy`: the `ensureEditable`
  locked-month guard (real, queries `period`; load-bearing from inc 5) refuses a policy change effective on
  a locked month. **L4/L10 archive-vs-delete**: hard delete when no dependents, else FK-RESTRICT→ErrConflict
  →soft-archive (history kept).
- **Handlers + central error mapper** (`internal/handlers/config.go`): `mutationError` maps
  ValidationError→422 (+ localized inline field errors), ErrNotFound→404, ErrLocked→409, ErrConflict/
  ErrDuplicate→409 (G3). Money/rate parsed at the boundary into the typed service inputs.
- **Templates** (`web/templates/parameters.html` + shared `rail`/`confhead`/`confscripts` partials):
  reproduce `mockups/parameters.html` — Comptes table (sweep/carry chips, archived toggle, edit/restore),
  Épargne (PEA fields, secured-basis option cards, drag cascade), Localisation, Préférences, DSP2-disabled,
  auth-stub placeholders. htmx fragments: `comptes-card`/`comptes-oob`, `cascade-card`, `card-epargne`/
  `card-localisation`/`card-preferences`, `account-form` modal.
- **Assets**: vendored `sortable.min.js` (I-022) + new `app.js` (CSP-clean delegation, htmx 422/409 swap
  shim, `htmx.process` on swapped content, cascade drag) + `config.css` (screen styles, kept out of the
  verbatim `econome.css`). Shell nav **Configuration** → `/config/parameters`.
- **Routes** wired into the protected chain; **i18n** keys added to both FR/EN catalogs (parity test green).

### Specs satisfied

`functional/10` (§2 Comptes, §3 Épargne, §4 Localisation, §5 Préférences, §6 DSP2-disabled),
`functional/04` §3.1 (account CRUD, L3) / §3.7 (settings) / §5 (L4/L10 archive-vs-delete) / §6 (recalc rows
are latent for config edits), `technical/04` §3.5/§3.7 (routes) + §4 (payloads, basis-points conversion) +
§1.1 (422/404/409 conventions), `technical/03` §3.1/§4.5, `technical/06` §2 (no-float parsing boundary),
`guardrails/01` §2/§4/§5, `G3` (one error mapper). Decisions **I-021..I-024**.

### Tests passing

- **Service integration** (real SQLite, `internal/services/accounts_test.go`): create/update/archive/delete
  validation (typed 422, no partial write), duplicate-name→field 422, cross-column policy rule, cross-tenant
  ⇒ ErrNotFound (never 403), L3 forward-only refuses a locked month (ErrLocked), archive-when-dependents,
  cascade reorder (assign/shrink, current-account rejected), settings update + negative/unknown/bad-enum
  422s. i18n parse/format tests.
- **e2e backbone** (`internal/server/config_e2e_test.go`, httptest+goquery): page renders all panels; account
  create→200 OOB card / empty-name→422 / duplicate→422; archive→archived badge; settings epargne valid→200,
  out-of-range rate→422, locale→EN reflected; cascade reorder→200.
- **chromedp smoke** (`-tags chromedp`): account modal opens (htmx, CSP-clean), native submit, OOB Comptes
  card swap shows the new account. (Login-shell smoke still green.)
- Full suite green; `gofumpt`/`golangci-lint` clean; engine coverage gate 91.7%.

## PR-b — Enveloppes (`/config/envelopes`)

The combined category+envelope CRUD screen (I-021, user-validated inline-parent path), reusing every PR-a
foundation (shared `rail`/`confhead`/`confscripts`, `config.css`, `app.js`, `#modal-host` choreography,
`mutationError` mapper).

### What was built

- **Service** (`internal/services/envelopes.go`): `EnvelopesOverview` (hierarchical list: parent groups +
  read-only **exact integer** default-sums + top-level rows), `ParentOptions`, `GetEnvelope`,
  `CreateEnvelope`/`UpdateEnvelope` (one tx: resolve/find-or-create parent → find-or-create **leaf category
  by (name, parent, flow_type)** so a category may pair with several accounts → create/update envelope),
  `ArchiveEnvelope`/`UnarchiveEnvelope`/`DeleteEnvelope`. **Inline new parent** (`NewParentName` find-or-
  create; `default_expanded` seeded only at parent creation, never silently flipped on a child edit).
  Validation (typed 422, no partial write): name required; (category×account) unique → field 422;
  `fixed_recurring`⇒frequency; non-monthly⇒due_months; `expected_day` 1..31; `residual` ⇒ no amount +
  **never deletable/archivable** (structural); `default_amount ≥ 0`; **flow_type must match the parent
  branch** (MsgFlowTypeConflict). Archive-vs-delete by dependents (FK-RESTRICT→archive).
- **Handlers** (`internal/handlers/envelopes.go`): full screen, modal form (field adaptation flags set
  server-side + `app.js` delegated `change`), create/update/archive/unarchive/delete. Mode/flow **badge**
  (residual > income/transfer flow > mode) + frequency label (with due-month names) computed in the handler.
- **Templates** (`web/templates/envelopes.html`): hierarchical `.etable` (parent `.tog` rows with the
  CSP-clean `data-action="toggle-group"` chevron + `.child`/`data-c` rows seeded open by
  `category.default_expanded`, mode badges `.mb.*`, `auto` for residual, archived rows behind the toggle),
  the `envelope-form` modal (native `<select>`s, `.off`/`hidden` adaptation, inline new-parent input, the L2
  note), empty-state. The Enveloppes header tab is now a **live link** (both screens). `config.css` gained
  the envelopes/badge styles; `app.js` gained `adaptEnvelope` + the group toggle.
- **i18n**: full Enveloppes + month + `validation.envelope.*`/`validation.category.*` keys in both catalogs.

### Tests passing

- **Service integration**: inline new-parent + reuse; **shared category across two accounts** (one category,
  two envelopes) + (category×account) duplicate 422; fixed/frequency/due-months validation; residual no-amount
  + non-deletable; flow-type-vs-parent conflict; archive-when-dependents (allocation FK); cross-tenant ⇒
  ErrNotFound; **parent read-only sum = Σ children** (exact).
- **e2e backbone**: empty state; create-with-new-parent → OOB list shows the row under its parent; duplicate
  422; fixed-without-frequency 422 with inline error.
- **chromedp smoke**: envelope modal opens (htmx, CSP-clean), mode change un-dims the frequency field
  (`adaptEnvelope`), submit → OOB list shows the new row. (PR-a smokes still green.)
- Full suite + lint + vet + govulncheck clean; engine coverage gate 91.7% holds.

## Exact next step

**Increment 5 — Month-initialisation assistant** (`development-plan/01-phased-plan.md`): compute the editable
non-persisted draft (starting balances, posts table, residual encart), `POST /month-init` materialising
allocations + awaited transactions + the `period` row in one tx; recurring generation from the envelopes
built here; the locked-month guard (`ensureEditable`, already in place from PR-a) becomes load-bearing. Specs
`functional/09`, `functional/04` §3.4/§4, `rules` §5/§7/§8/§11.1, `technical/04` §3.6, `technical/03`
§3.4/§4.1. Demo **D2** follows. Depends on inc 2 (residual engine), 3 (repos+period), 4 (config to generate
from).

## Open points

- **O-12.** Per-period forward-only `month_end_policy` (L3) is delivered by the lock-freeze of past months
  (the engine recomputes only non-locked periods) — the data model has **no per-period policy column**
  (`technical/03` §3.1 single `account.month_end_policy`). The service accepts `effective_period` and the
  `ensureEditable` guard blocks a locked target, but a true per-period policy history is **not** modeled. If
  inc 5/6 reveals a need for past *unlocked* months to keep an old policy, that is a `technical/03` change
  (raise it there first), not an implementation detail. No reshape needed for the manual flow.
- **O-13.** The existing `home.html` shell uses inline `onclick` (pre-CSP-clean), non-functional under the
  CSP for the theme toggle. The new shared `rail` partial is CSP-clean (`data-action`); `home.html` is left
  as-is for now (the landing shell is replaced by the real Budget screen in inc 6).
- The custom `emMenu` selects (mockup visual) are deferred (I-024); native controls are used. `app.js` keeps
  the drag/toggle/modal behaviours.
- **O-14 (envelope form / transfer).** A **transfer** envelope is a category with `flow_type='transfer'` +
  one account; the `envelope` table has **no destination column** (`technical/03` §3.3 — "the envelope
  inherits flow via its category"). The transfer destination is therefore set when a transfer transaction is
  created (journal/month-init, inc 5/6), not on the template. If inc 5's recurring transfer generation needs
  a stored destination, that is a `technical/03` change (raise it there first). No reshape now.
- **O-15 (default_expanded edit).** In PR-b, `category.default_expanded` is seeded only when a **new** parent
  is created (not flipped from a child's form, to avoid a surprising side effect). There is no direct UI to
  toggle an existing parent's seed; this is acceptable because the per-user persisted open/closed state (M4,
  `PUT /ui/expand`) lands in inc 6 and overrides the seed anyway.
