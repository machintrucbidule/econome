package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// sessionTokenLen is the entropy of the opaque session token (technical/05 §2).
const sessionTokenLen = 32

// GenerateSessionToken returns a fresh opaque session token (32 bytes from
// crypto/rand, base64url). This raw value goes into the cookie and is never
// stored; only HashToken(token) is persisted.
func GenerateSessionToken() (string, error) {
	b := make([]byte, sessionTokenLen)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("auth: session token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashToken returns the SHA-256 of a token, base64-encoded — the value stored in
// session.token_hash (technical/05 §2).
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return base64.RawStdEncoding.EncodeToString(sum[:])
}
