// Package engine is the PURE domain core: it computes every derived figure of
// functional/03-calculation-rules.md (envelope states, balances, residual,
// cascade, low point, PEA net, net-worth totals, banker's-rounding money) and
// the pure reconciliation decision (reconcile.go), shared with the future DSP2
// import.
//
// Purity is load-bearing and enforced at build time by depguard (G4): this
// package may import ONLY the standard library and internal/domain. It must not
// import repo, services, i18n, view, net/http, any template or SQL package, nor
// any source of time, randomness, or locale. The clock is an injected `today`
// parameter, never time.Now(). See technical/09-testability-seam.md.
//
// The engine and reconciliation logic land in increment 2; this file only
// declares the package so the scaffold compiles and depguard is active.
package engine
