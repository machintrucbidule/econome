// Package domain holds EconoMe's shared value types and enums (English internal
// codes) plus pure validation helpers. It is the only application package the
// pure engine (internal/engine) is allowed to import.
//
// Enums use English codes; FR/EN display strings live in internal/i18n. The
// enum sets the `exhaustive` linter guards (five envelope states, transaction
// status, mode, flow_type, account.type, month_end_policy) are declared here as
// the increments that own them land.
package domain
