# Increment 1 — Walking skeleton (Demo D1)

**Date.** 2026-06-26 · **Milestone.** M0 · **Status.** DONE (merged to `main`; CI green; subagent reviews
passed). Built as three PRs: [#5](https://github.com/machintrucbidule/econome/pull/5) pure core,
[#6](https://github.com/machintrucbidule/econome/pull/6) persistence,
[#7](https://github.com/machintrucbidule/econome/pull/7) auth + transport + screens.

## What was built

The thinnest end-to-end vertical slice exercising every layer + the production rails:

- **Engine (pure):** `internal/engine/money.go` — `Money`/`BasisPoints` minor-unit types, `RoundHalfEvenDiv`
  (banker's rounding C6, correct for negatives + exact halves), `ApplyRate`. 100 % covered.
- **Domain:** `User`/`Session`/`Settings`, English-code enums (exhaustive `Valid()`), password policy (§9),
  email validation, `ValidationError` + sentinels (`G3`).
- **Persistence:** `0001_init` (`user`/`session`/`settings`); hand-rolled migration runner over `embed.FS`
  (pre-migration `VACUUM INTO` backup → per-migration transaction → abort-on-failure; bootstraps
  `schema_migrations`); `user_id`-scoped SQLite repositories over `modernc.org/sqlite` (WAL/FK/busy_timeout)
  + in-memory fakes; on-volume session-secret bootstrap + data-dir writability check.
- **Auth:** Argon2id (m=64MiB/t=3/p=2, PHC, constant-time, re-hash-on-login); opaque sessions (SHA-256 at
  rest); HMAC signed double-submit CSRF; progressive lockout (5 → 1/30/300 s) + per-IP throttle; **dummy
  Argon2 verify on unknown email** (anti-enumeration timing fix from the subagent review).
- **Services:** setup (owner+settings+session atomic, zero-users guard, no partial write), login (generic
  error, lockout), logout (instant revoke), session resolution (idle expiry, deactivated-user purge).
- **i18n + view:** embedded FR/EN TOML catalogs (parity-tested) + exact manual money formatter (no float,
  U+2212 / U+202F → `−635,00 €`); renderer + per-request view-models (`T`/`Money`/`CSRF`).
- **Transport:** full middleware chain (Recover → SecurityHeaders → RequestContext → Session → SetupGuard →
  [AuthGuard → TenantContext] → CSRF → Locale), security headers; handlers + templates (signed-out
  setup/login + empty three-pane shell reproducing the mockups); routes incl. `GET /healthz`; **vendored
  htmx 2.0.7**. `cmd/econome` wires repo → service, catalog → renderer, server; migrates on startup.

## Specs satisfied

`functional/01` §1.3/§2/§3/§7/§9, `functional/02` §1/§9 (shell), `functional/03` C6 (money),
`technical/01` §2–§4, `technical/03` §2.1/§2.2/§4.5/§5.3, `technical/04` §1–§2, `technical/05` §1–§4/§6/§10,
`technical/06` §3–§4, `technical/07` §2/§4/§7, `technical/08` §1–§3, `guardrails/01` §2/§4/§5.
Decisions **I-009..I-013** (htmx vendor, CSRF HMAC, settings FK, dep pins, money formatting).

## Tests passing

Verified locally (Go 1.26.4) and in CI (`-race` on Linux):
- **Engine:** `money` table + `rapid` property tests; **100 % coverage** (gate active).
- **Integration (real SQLite):** migration runner forward-from-empty + backup + abort; repo CRUD + tenant
  scoping + FK/cascade + tx rollback; fakes parity; service use-cases (setup/login/lockout/logout/expiry).
- **Security regression (subset):** Argon2id PHC round-trip + re-hash; session hashed-at-rest + revocation;
  cookie flags; generic error + progressive lockout.
- **Middleware:** Recover panic→500; security headers; Accept-Language.
- **e2e backbone (`httptest`+`goquery`):** setup-guard redirect, CSRF 403, owner creation → authenticated
  shell (with `−635,00 €` + email + logout), setup→login redirect, logout revocation, generic login
  failure, htmx AuthGuard `HX-Redirect`.
- **Binary smoke:** boots, migrates, serves `/healthz`/`/setup`/assets, redirects `/`→`/setup`, creates
  DB + secret on the volume.
- `gofumpt`/`golangci-lint` (depguard active) green; `govulncheck` clean.

**Subagent reviews (G8 §3.2):** engine (money) — conformant, nits only; auth/security — conformant after
fixing the timing oracle (#1) and reordering CSRF-before-Locale (#2); I-log gap (#3) closed.

## Exact next step (next run)

**Increment 2 — Engine + reconciliation (full coverage)** (`development-plan/01-phased-plan.md`), after the
user's go-ahead. Build the pure `internal/engine` for all of `functional/03` (five envelope states C2/C8,
`real=cleared+pending` C7, balances C5/C9, residual/secured C1/C4, low point C3/C9, PEA net §12, net-worth
§13, aggregation §14) and the pure `internal/engine/reconcile.go` (zero/one/many + tolerance + transfer
pairing, `functional/04` §7, `technical/09` §3). Property (`rapid`) + golden (`testdata/`) + reconciliation
matrix tests; the ~90 % engine coverage gate stays green. **Mandatory subagent review** (engine +
reconciliation). No UI. Depends on increment 1 (`money.go`) + the `domain` types.

## Open points

- **O-2 — RESOLVED.** htmx 2.0.7 vendored; no fonts/icons needed (I-009).
- **O-5 — RESOLVED.** Dockerfile copies `go.sum`.
- **O-7 (deferred).** `chromedp` smoke not added: it needs a headless Chrome the CI does not yet provision.
  The `httptest`+`goquery` e2e backbone + the binary smoke cover "login renders the shell". Add a
  chrome-provisioned CI job + the `chromedp` smoke before release (or in a follow-up within M0). Flagged
  for the user.
- **O-8 (note).** Engine review nits (theoretical int64 overflow at extremes; a stronger to-even property
  test) — not reachable with realistic values; optional hardening.
