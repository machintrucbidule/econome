# Increment 3 — Data model + repositories

**Date.** 2026-06-26 · **Milestone.** M0 · **Status.** DONE (merged; CI green). Built as three PRs:
[#15](https://github.com/machintrucbidule/econome/pull/15) migrations,
[#16](https://github.com/machintrucbidule/econome/pull/16) budget-core repos,
[#17](https://github.com/machintrucbidule/econome/pull/17) lifecycle/networth/ui/auth-extra repos.

## What was built

The persistence foundation every screen reuses — **no engine change, no UI**:

- **Migrations** (`migrations/0002–0006.up.sql`, I-019): the full budget schema per `technical/03` §3–§5 —
  account/category/envelope/allocation/`transaction` (0002), period/period_event (0003), savings_snapshot/
  networth_month (0004), label_mapping/ui_preference (0005), invitation/totp_backup_code (0006). Exact
  columns, indexes (incl. partial `UNIQUE(user_id,fill_priority)`, the five transaction composite indexes),
  CHECK/UNIQUE/FK, ON DELETE RESTRICT default + CASCADE on totp_backup_code, the anticipatory DSP2 fields
  (`source`/`external_ref`/`op_date`/`paired_transaction_id`, `account.external_ref`).
- **Domain entities** extended with persistence fields (UserID/timestamps/Note/ExternalRef on the budget
  value types; the engine ignores them) + new types/enums (Period/PeriodEvent/NetworthMonth/LabelMapping/
  UIPreference/Invitation/TOTPBackupCode; PeriodState/PeriodAction/NodeType).
- **Repositories** (`internal/repo`): `user_id`-scoped SQLite repos for all 13 tables, reusing the
  increment-1 seam (`DBTX`/`Store`/`Txer`/scan helpers). CRUD + scoped reads + upserts; `due_months`
  CSV↔`[]int`, `op_date` `domain.Date` mapping, UNIQUE→`ErrDuplicate`, FK-RESTRICT→`ErrConflict`,
  cross-tenant→`ErrNotFound`; parameterised queries only. **In-memory fakes** for every repo, wired into
  `repotest.Store`.

## Specs satisfied

`technical/03` (whole — §2.3/§2.4 auth-extra, §3 budget core, §4 lifecycle/net-worth, §5 ui/learning, §6
relationships, §7 DSP2 checklist), `technical/08` §1/§4 (forward migrations, migration testing),
`technical/09` §4 (repo interfaces + fakes), `guardrails/01` §5 (SQL style). Decisions **I-019/I-020**
(reaffirms **I-011**).

## Tests passing

- **Migration:** full-schema forward-from-empty (17 tables, version 6) + **production-shaped** (DB at
  version 1 with an owner+settings row → migrate to 6, no data loss, pre-migration backups taken) +
  abort-on-failure (from inc 1).
- **Integration (real SQLite):** budget-core contract (CRUD + scoped reads) run against **both** the SQLite
  store and the in-memory fake; CHECK (amount<>0, enum) + FK-RESTRICT fire; tenant scoping (cross-`user_id`
  ⇒ `ErrNotFound`); lifecycle/networth/ui/auth-extra repos (period lock/audit, snapshot upsert, networth
  comment upsert, label learning + ranked search, ui-preference toggle, invitation consume, totp single-use
  + cascade) + a fake smoke.
- CI green incl. `-race`, multi-arch, `e2e-chrome`; `gofumpt`/`golangci-lint`/`govulncheck` clean.

## Exact next step (next run)

**Increment 4 — Configuration (Parameters + Envelopes)** (`development-plan/01-phased-plan.md`), after the
user's go-ahead. The first screens: Paramètres (account CRUD incl. forward-only `month_end_policy` L3,
archive-vs-delete L4/L10; Épargne & fiscalité; Localisation; Préférences; DSP2 card disabled) and Enveloppes
(category + envelope CRUD with field adaptation by mode/frequency, hierarchy, show-archived). Service-layer
validation (typed 422, no partial write, rate-bound 422, uniqueness, no-cyclic-parent, flow_type edit
legality) + the error→(status,fragment) mapping (G3). Specs `functional/08` + `functional/10`,
`functional/04` §3.1–§3.3/§3.7, `technical/04` §3.5/§3.7. Depends on increments 1 (shell), 2 (engine sums),
3 (repos). Demo **D2** follows increment 5.

## Open points

- **O-10 (note).** The lifecycle/networth/ui/auth-extra fakes have a smoke test, not a full dual-parity
  contract (the budget-core repos do). Parity for those will be exercised when the consuming services land
  (inc 5–8); add a parity test there if a divergence is suspected.
- **O-11 (note).** `label_mapping` has no `UNIQUE(user_id,label_key)`; `Upsert` does update-or-insert. If a
  race ever duplicates a key, `Search` still works (ranked) — acceptable for a single-family instance.
