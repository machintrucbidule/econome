# Increment 2 — Engine + reconciliation (full coverage)

**Date.** 2026-06-26 · **Milestone.** M0 · **Status.** DONE (merged; CI green; mandatory subagent reviews
passed). Built as five PRs: [#9](https://github.com/machintrucbidule/econome/pull/9) domain+inputs,
[#10](https://github.com/machintrucbidule/econome/pull/10) envelope+balances,
[#11](https://github.com/machintrucbidule/econome/pull/11) savings+lowpoint+networth,
[#12](https://github.com/machintrucbidule/econome/pull/12) reconcile,
[#13](https://github.com/machintrucbidule/econome/pull/13) chromedp CI (O-7).

## What was built

The two highest-risk correctness surfaces, sealed before any screen consumes them — **pure, no UI/DB/HTTP**:

- **Domain value types** (`internal/domain`): budget enums (FlowType/AccountType/MonthEndPolicy/Mode/
  Frequency/TransactionStatus/TxnSource/ArchiveStatus/EnvelopeState, exhaustive `Valid()`) + structs
  (Account/Category/Envelope/Allocation/Transaction/Snapshot) per `technical/03` §3; `Account.IsSavings()`
  derived; clock-free `domain.Date` (I-015).
- **Engine** (`internal/engine`, depguard-sealed): `Inputs`/`Params`; `envelope.go` (real=cleared+pending
  C7, income received cleared-only §4, five states C2/C8, remaining/percent); `balances.go` (start C5 /
  real / in_progress / cleared / projected_end, transfers both legs); `lowpoint.go` (intra-month low point,
  undated one-off at end-of-period C3, excludes unspent variable budget C9); `savings.go` (projected /
  secured C1 / to_save / cascade C4 + full flag / residual-negative); `networth.go` (PEA net guard §12,
  support values, livrets subtotal, totals, deltas §13); `aggregate.go` (sweep bands, worst-account low
  point §14). Sign convention I-017.
- **Reconciliation** (`internal/engine/reconcile.go`): pure `Reconcile(movement, candidates, tol) →
  ReconcileInPlace/CreateNew/Ambiguous` (matching keys account+sign+amount±tol+date-window) + `PairTransfer`
  for internal-transfer auto-pairing — the **same** function the manual path and DSP2 import call
  (`technical/09` §3, `functional/04` §7). Decision is a tagged struct (I-014); O(1) date diff.
- **O-7 closed:** a `//go:build chromedp` smoke (login → shell, verified against real Chrome) + a
  Chrome-provisioned `e2e-chrome` CI job, now a required check on `main`.

## Specs satisfied

`functional/03` (whole: §1 rounding/sign, §3/§4 envelope, §5 balances, §7 savings, §9 cascade, §10
transfer neutralisation, §11 low point/alerts, §12 PEA, §13 net worth, §14 aggregation),
`functional/04` §7 (the pure decision), `technical/04` T4, `technical/09` §2–§3, `guardrails/01` §3,
`guardrails/03` §3–§4. Decisions **I-014..I-018**.

## Tests passing

- **Property (`rapid`):** residual identity, transfer neutrality (budget), pending-in-real, five-state
  totality.
- **Golden (`testdata/` + `-update`):** secured 1 660 €/1 870 € (basis switch), sweep → to_save, carry →
  projected_end, PEA gain/loss.
- **Unit:** five states (incl. planned=0), income received cleared-only, overrun-preserves-flag, balances
  (transfers/future-dated), low point (mid-month dip, end-of-period one-off C3, excludes-variable C9),
  cascade ceiling (lowest non-full / all-full flag / none), PEA table, net-worth deltas, aggregation
  worst-account.
- **Reconciliation matrix:** zero/one/many, amount within/at/past tolerance, date window/boundary, sign &
  account mismatch, transfer pairing (opposite/same-sign/same-account); date-diff leap/year boundaries.
- **Coverage:** `internal/engine` **91.7 %** (≥ ~90 % gate). CI green incl. `-race`, multi-arch, e2e-chrome.

**Subagent reviews (G8 §3.2):** engine — conformant (fixed: income `received` = cleared-only per §4);
reconciliation — conformant, no blockers (added boundary + exact-ambiguous-id tests).

## Exact next step (next run)

**Increment 3 — Data model + repositories** (`development-plan/01-phased-plan.md`), after the user's
go-ahead. Forward migrations for the core budget tables (account/category/envelope/allocation/transaction/
period/period_event/savings_snapshot/networth_month + the full settings/label_mapping/ui_preference/
totp_backup_code/invitation set, `technical/03` §3–§5, incl. the anticipatory DSP2 fields), the
`user_id`-scoped repositories + in-memory fakes (`technical/09` §4), with integration + migration tests.
Depends on increment 1 (runner) + increment 2 (domain types). No engine change.

## Open points

- **O-7 — RESOLVED.** chromedp smoke + Chrome CI job added and required on `main`.
- **O-9 (note).** Engine review nits not actioned (all non-correctness): `savingsSecured` recomputes a full
  `EnvelopeView` per envelope (O(env×txn) on a read path — optimise if it ever shows up); a `fixed_recurring`
  envelope missing `expected_day` degrades to end-of-period in the low point (defensive); `Candidate.Period`
  is reserved for future one-off period-window matching (I-014). Hardening candidates for a later pass.
