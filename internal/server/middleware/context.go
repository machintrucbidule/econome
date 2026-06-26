package middleware

import (
	"context"

	"econome/internal/domain"
)

type ctxKeyType struct{}

var ctxKey = ctxKeyType{}

// Ctx is the per-request context the middleware chain populates and handlers /
// view-models read. Tenant scoping uses Ctx.User.ID exclusively — never a
// request parameter (technical/04 §2).
type Ctx struct {
	RequestID string
	User      *domain.User
	Session   *domain.Session
	Lang      domain.Language
	Currency  string
	Theme     domain.Theme
	IsAdmin   bool
	CSRFToken string
}

func newCtx() *Ctx {
	// Safe defaults so a view-model never renders an empty locale/theme.
	return &Ctx{Lang: domain.LangFR, Currency: "EUR", Theme: domain.ThemeLight}
}

// From returns the request Ctx, or a default one if the chain has not attached
// it (defensive; the real one is set by RequestContext).
func From(ctx context.Context) *Ctx {
	if c, ok := ctx.Value(ctxKey).(*Ctx); ok && c != nil {
		return c
	}
	return newCtx()
}

func withCtx(ctx context.Context, c *Ctx) context.Context {
	return context.WithValue(ctx, ctxKey, c)
}
