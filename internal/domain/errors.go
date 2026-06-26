package domain

import (
	"errors"
	"fmt"
	"strings"
)

// Cross-cutting sentinel errors. The transport layer maps these to HTTP status
// codes in one place (guardrails/01 §2, G3):
//
//	ErrNotFound  -> 404 (also the cross-tenant outcome: a row scoped to another
//	                user_id is indistinguishable from absent — never 403)
//	ErrLocked    -> 409 (locked-month guard; "unlock to edit")
//	ErrConflict  -> 409 (state conflict)
//	ErrDuplicate -> 409/422 (unique-constraint violation: email, etc.)
var (
	ErrNotFound  = errors.New("not found")
	ErrLocked    = errors.New("locked")
	ErrConflict  = errors.New("conflict")
	ErrDuplicate = errors.New("duplicate")
)

// FieldError is a single field-level validation failure. MsgKey is an i18n
// catalog key (resolved to a localised message by the view layer), never a
// user-facing string itself.
type FieldError struct {
	Field  string
	MsgKey string
}

// ValidationError aggregates field-level validation failures. Handlers
// errors.As() it and re-render the fragment with inline field errors and no
// partial write (technical/04 §1.1; guardrails/01 §2). It maps to HTTP 422.
type ValidationError struct {
	Fields []FieldError
}

// Error implements error.
func (e *ValidationError) Error() string {
	parts := make([]string, len(e.Fields))
	for i, f := range e.Fields {
		parts[i] = fmt.Sprintf("%s: %s", f.Field, f.MsgKey)
	}
	return "validation error: " + strings.Join(parts, "; ")
}

// Add appends a field error.
func (e *ValidationError) Add(field, msgKey string) {
	e.Fields = append(e.Fields, FieldError{Field: field, MsgKey: msgKey})
}

// HasErrors reports whether any field error has been recorded.
func (e *ValidationError) HasErrors() bool {
	return len(e.Fields) > 0
}

// OrNil returns the receiver as an error when it has failures, or nil when it is
// empty — so callers can `return v.OrNil()` after accumulating checks.
func (e *ValidationError) OrNil() error {
	if e.HasErrors() {
		return e
	}
	return nil
}
