package middleware

import "net/http"

// Middleware wraps an http.Handler.
type Middleware = func(http.Handler) http.Handler

// Chain composes middleware so the first listed runs outermost.
func Chain(mws ...Middleware) Middleware {
	return func(final http.Handler) http.Handler {
		for i := len(mws) - 1; i >= 0; i-- {
			final = mws[i](final)
		}
		return final
	}
}
