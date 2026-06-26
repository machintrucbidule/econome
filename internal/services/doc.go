// Package services owns the application use-cases: CRUD orchestration, input
// validation (typed 422, no partial write), the month-lifecycle / locked-month
// guard, reconciliation orchestration, and DB transactions. It depends on repo
// (via interfaces), engine, and domain — and must never import net/http or
// templates (depguard, G4).
//
// Services load inputs via repositories, invoke the pure engine to derive
// figures, apply validation and the lifecycle guard, write inside a single DB
// transaction, and return view models. Use-cases land from increment 4 onward.
package services
