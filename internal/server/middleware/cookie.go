package middleware

import "net/http"

// setCookie writes an HttpOnly, SameSite=Lax cookie. Secure is set behind TLS
// (technical/05 §2). maxAge 0 leaves it a session cookie (no Max-Age).
func setCookie(w http.ResponseWriter, name, value string, behindTLS bool, maxAge int) {
	//nolint:gosec // Secure is gated on behindTLS by design: local http dev needs it off (technical/07 §3)
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   behindTLS,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	})
}

func clearCookie(w http.ResponseWriter, name string) {
	//nolint:gosec // deletion cookie (MaxAge<0); Secure is irrelevant for removal
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// SetSessionCookie sets the session cookie with the opaque token (handlers call
// it on login/setup).
func SetSessionCookie(w http.ResponseWriter, value string, behindTLS bool, maxAge int) {
	setCookie(w, SessionCookie, value, behindTLS, maxAge)
}

// ClearSessionCookie removes the session cookie (logout).
func ClearSessionCookie(w http.ResponseWriter) {
	clearCookie(w, SessionCookie)
}
