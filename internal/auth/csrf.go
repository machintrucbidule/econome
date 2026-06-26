package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
)

// CSRF uses a signed double-submit token (I-010): a per-browser random seed is
// stored in a cookie, and the form/htmx token is HMAC(secret, seed). This is
// stateless (no DB column) and works for both anonymous forms (setup/login) and
// authenticated requests, since the binding is the seed cookie, not a session id.

const csrfSeedLen = 18

// GenerateCSRFSeed returns a fresh random seed for the CSRF cookie.
func GenerateCSRFSeed() (string, error) {
	b := make([]byte, csrfSeedLen)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("auth: csrf seed: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// CSRFToken derives the form token from the secret and the cookie seed.
func CSRFToken(secret []byte, seed string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(seed))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// ValidCSRF reports whether token matches the seed under the secret, in constant
// time. Empty seed or token never validates.
func ValidCSRF(secret []byte, seed, token string) bool {
	if seed == "" || token == "" {
		return false
	}
	expected := CSRFToken(secret, seed)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(token)) == 1
}
