// Package handlers is part of the HTTP/transport layer: one file per
// screen/domain. A handler parses the request, resolves session/user/locale,
// calls exactly one service use-case, and renders a template (full page or
// htmx fragment). It contains no business logic and computes no derived figure.
//
// Together with internal/server it is one of the only packages that know htmx,
// cookies, CSRF, sessions, and templates. Screen handlers land from increment 1
// (shell) onward.
package handlers
