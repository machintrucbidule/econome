// Package auth owns the security primitives: Argon2id password hashing,
// opaque-token sessions (hashed at rest), TOTP enrolment/verification,
// invitations, and the login lockout/throttle. Parameters come from config /
// technical/05-authentication-security.md.
//
// crypto/rand is the only randomness source (gosec); math/rand is never used
// for security material. The mechanisms land across increments 1 (password
// login, sessions, lockout) and 8 (2FA, invitations, admin recovery).
package auth
