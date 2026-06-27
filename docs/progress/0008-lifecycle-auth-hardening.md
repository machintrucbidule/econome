# Increment 8 — Lifecycle controls + rest of auth + hardening — Milestone M4 (release-ready)

**Date.** 2026-06-27 · **Status.** complete, all gates green; **mandatory auth subagent review done**
(findings fixed in-PR); demos **D4 + D5** to be presented; awaiting the user's go-ahead. Delivered as
**one PR** (`feat/increment-8-lifecycle-auth`, user's choice `G15`, I-040) off an up-to-date `main`
(inc 7 merged as PR #32 first, I-045).

Makes the build release-ready: the month lifecycle controls, the full auth surface, the offline recovery
CLI, and the security regression suite. **Additive** — the walking skeleton (inc 1) already had the auth
core (Argon2id, sessions, CSRF, login+lockout+throttle, setup, headers, /healthz) and the lifecycle
substrate (`period`/`period_event`, the `ensureEditable`→409 guard wired into every budget mutation,
read-only lockbars). **No new migration** — every column already existed (`0001`/`0003`/`0006`).

## What was built

### A. Month lifecycle (functional/04 §4, technical/04 §3.7, L1/L9 — I-043)
- `internal/services/lifecycle.go`: `LockMonth`/`UnlockMonth` (state + `period_event` audit in one tx;
  not-created → 404, already-in-state → 409), `RegenerateMissingRecurring` (reuses monthinit
  `buildDraftPosts`/`dueThisMonth`; additive + idempotent — the per-envelope allocation / awaited transfer
  is the presence marker; refused on a locked month).
- Routes `POST /periods/{period}/lock|unlock|regenerate` (`internal/handlers/lifecycle.go`, sanitised
  app-relative redirect, gosec-clean). Forecast + journal headers gain the close / unlock / "Régénérer les
  récurrents manquants" controls (`lifebar`; lock confirms with the O-18 sweep reminder; the residual
  encart already surfaces `to_save`).

### B. Rest of auth (functional/01 §4–§8, technical/05 — I-041/I-042/I-044)
- **TOTP 2FA** — `internal/auth/totp.go` (secret + otpauth URL via `pquerna/otp`; `VerifyTOTP` ±1 step;
  Argon2id-hashed single-use backup codes; `RandomPassword` for temp passwords; `TOTPURLFromSecret` for
  stable re-render). `internal/auth/pending.go` — the stateless signed pending-2FA token (HMAC, 10-min
  TTL). `internal/services/security.go` — `BeginTOTPEnrolment`/`CurrentTOTPEnrolment`/`ConfirmTOTP`/
  `DisableTOTP`/`RegenerateBackupCodes`/`BackupCodesRemaining`/`CompleteTOTPLogin` (per-IP throttled).
  `Login` defers the session to the 2FA step when enabled. Handlers: `LoginPost` 2FA branch + `LoginTOTP`;
  the login template's inline TOTP step. QR PNG via `skip2/go-qrcode` → `data:` URI.
- **Invitations** — `internal/services/admin.go`: `IssueInvitation` (7-day single-use; only the SHA-256
  hash stored), `CheckInvitation`, `AcceptInvitation` (user + settings + token-consume atomic),
  `RevokeInvitation` (issuer-scoped → 404). Public `GET/POST /invite/{token}` + `web/templates/auth_extra.html`
  `accept` page (form / invalid state).
- **Admin user management** — `ListUsers`/`DeactivateUser`(revokes sessions)/`ReactivateUser`/
  `AdminResetTOTP`/`AdminResetPassword`(temp + `must_change_password`, revokes sessions)/`SetAdmin`, all
  sharing `ensureNotLastAdmin`. Routes behind `AdminGuard` (→ 404). `internal/handlers/admin.go` + the
  Parameters Utilisateurs panel + invite/user modals.
- **Active sessions** — `ListSessions`(marks current)/`RevokeSession`(scoped)/`RevokeOtherSessions`.
  Routes `GET`(panel)/`POST /security/sessions/{id}/revoke` + `/revoke-all`.
- **Profile** — `ChangePassword` (clears `must_change`), `ChangeEmail` (password re-auth, unique).
  `ForcePasswordChange` middleware + the full-page `/password` form.
- **Parameters screen** — new **Profil / Sécurité / Utilisateurs(admin)** cards replacing the stub
  (`web/templates/parameters.html`); CSP-clean (htmx + `data-action`; QR is a `data:` img).
- **`econome-admin` CLI** (technical/05 §8) — `reset-password`/`reset-2fa`/`user list|deactivate|
  reactivate`/`backup` (`repo.BackupTo` VACUUM INTO), reusing the services + the shared last-admin rule.

### C. Hardening
- Security regression suite extended (below). i18n FR/EN parity holds (`TestCatalogParity`) for ~95 new
  keys. Security headers + `/healthz` unchanged (verified). Backup via the CLI; the Watchtower/volume
  upgrade path is the existing deployment guarantee (data on the mounted volume; migrations auto-apply
  with a pre-migration VACUUM-INTO backup).

## Repo plumbing
- `UserRepo`: +`CountActiveAdmins`/`ListAll`/`SetPassword`/`UpdateEmail`/`UpdateTOTP`/`UpdateStatus`/
  `SetAdmin`. `SessionRepo`: +`ListByUser`/`DeleteByUserScoped`/`DeleteByUserExcept`. `InvitationRepo`:
  +`ByID`. Real + `repotest` fakes both updated. `invitations`/`totpBackups` wired into
  `services.Service`/`Deps`/`New` + `cmd/econome` + both test harnesses (the repos existed since inc 6;
  now consumed).

## Decisions (this run)
**I-040** one PR · **I-041** pquerna/otp + skip2/go-qrcode · **I-042** Argon2id backup codes ·
**I-043** lifecycle placement + within-contract controls (O-18 via the encart) · **I-044** AdminGuard 404
+ pending-2FA token + must_change middleware · **I-045** D4 deferred to D5 · **I-046** security-review
fixes. See `specifications/implementation/decision-log.md`.

## Specs satisfied
`functional/01` §3–§9 (whole auth surface), `functional/04` §4 (L1 lock/unlock + L9 regenerate),
`technical/04` §3.1/§3.7, `technical/05` (whole), `guardrails/03` §8, `guardrails/04` §3. DSP2 seam
untouched.

## Tests passing
- **auth unit** (`internal/auth`): TOTP round-trip + ±1 skew + two-step rejection; backup-code single-use
  hashed + dash-insensitive; pending-token (existing CSRF/session/lockout suites still green).
- **service integration** (`internal/services`): lock guards budget writes (409) + logs create/lock/unlock;
  unlock re-enables; regenerate additive + idempotent + refused-on-locked; 2FA enrol→confirm→login-step→
  backup-code-single-use→disable; invitation single-use/expiry/revoke + invited-admin + duplicate-email
  422; **last-admin** rule (deactivate + demote); admin reset forces `must_change` + revokes sessions;
  change-password/email; revoke-other-sessions keeps current; **deactivated user cannot log in**;
  case-insensitive login.
- **e2e backbone** (`internal/server`, httptest+goquery): invitation issue→accept→single-use; admin gate
  404 for a member + no Users panel; forced-password-change full flow (admin reset → re-login → /password
  redirect → change → back in).
- **CLI integration** (`cmd/econome-admin`): user list / backup / reset-password / reset-2fa against a
  real seeded DB; last-admin deactivation refused; unknown-user error.
- Full suite green; engine coverage **91.7 %** holds (engine untouched); `gofumpt -l` clean; `go vet`
  (incl. `-tags chromedp`); `golangci-lint` **0 issues**; `govulncheck` clean.

## Verification (G8)
Conformance checklist passed: tenant scoping (AdminGuard → 404; session/invitation revokes user-scoped;
admin ops never touch another user's financial data); derived-not-stored (auth is identity, not budget
figures — unaffected); money/rounding untouched; exhaustive enums; lifecycle guard (lock/unlock/regenerate
honour + extend `ensureEditable`); DSP2 seam untouched. **Mandatory auth/security subagent review run**
(`G8` §3.2): found H1 deactivated-login bypass, H2 two CSP inline handlers, M1 email casing, M2 unthrottled
2FA step, L1 secret-rotation-on-wrong-code — **all fixed in-PR** (I-046) + regression tests added; sound
areas (pending token, ±1 skew, backup single-use, invitations, last-admin, CSRF, generic errors)
confirmed.

## Exact next step
Increment 8 is the last build increment. **Stop for the user's go-ahead**, then present **D4 + D5** (the
deferred running build + the M4 release-readiness demo) and run the M4 pre-release hardening pass. After
that, Stage 7's final deliverable: author `specifications/prompts/stage-8-dsp2-import-spec.md` (DSP2
import **specification & dev plan** in Cowork — not implementation) and confirm where it was written.

## Open points
- **O-26** (carried from review M3): `CompleteTOTPLogin` consumes a backup code then issues the session in
  two statements, not one tx — the consume is itself atomic (SQL `consumed_at IS NULL` guard, no
  double-spend), but a crash between consume and issue burns a code. Low impact; wrap in one tx later.
- **O-27** (review L2): `services.SetAdmin` (+ last-admin demote guard) has no UI route yet — promote/
  demote is service/CLI-reachable only. The owner-cannot-be-demoted rule is enforced; the UI control is a
  follow-up.
- Per-IP throttle keyed on the resolved client IP collapses to the proxy IP unless `ECONOME_TRUSTED_PROXY`
  is set (operator config, documented).
- Carried from inc 7: **O-16** (no opening-balance column), **O-17** (snapshots-at-init for cascade-full),
  **O-19** (chrome-smoke flake, mitigated), **O-24** (`PairInternalTransfer` one-row-vs-two-leg, DSP2-only).
- `chromedp` smoke for the inline 2FA UI / theme runs locally with `-tags chromedp` + local Chrome (the
  flows are covered by the httptest e2e + service tests).
