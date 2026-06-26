// Package repo is the persistence layer: the ONLY package allowed to import the
// SQLite driver (modernc.org/sqlite, I-001) — no SQL leaks above it. It exposes
// consumer-side interfaces (in repo.go) so services can be tested with
// in-memory fakes (technical/09 §4).
//
// Every public method is user_id-scoped: tenant isolation is enforced here as
// defence in depth, independently of the middleware (technical/01 §4). A row
// scoped to another user_id is indistinguishable from absent (cross-tenant ⇒
// 404, never 403). Queries are parameterised only (gosec); connections set
// PRAGMA foreign_keys=ON, WAL, busy_timeout (technical/07 §4).
//
// Interfaces + the SQLite implementation land in increment 3; this file only
// declares the package.
package repo
