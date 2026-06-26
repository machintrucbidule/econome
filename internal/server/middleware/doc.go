// Package middleware holds the request-pipeline stages of the transport layer:
// Recover, RequestContext, Session, AuthGuard, TenantContext, CSRF, Locale, and
// AdminOnly (technical/04 §2). The chain is assembled in increment 1; this file
// only declares the package.
package middleware
