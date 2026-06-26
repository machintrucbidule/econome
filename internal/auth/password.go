// Package auth holds EconoMe's security primitives: Argon2id password hashing,
// opaque session tokens (hashed at rest), CSRF tokens, and the login lockout /
// throttle. crypto/rand is the only randomness source (gosec); math/rand is
// never used for security material. Parameters follow technical/05.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2Params are the Argon2id cost parameters (technical/05 §1).
type Argon2Params struct {
	Memory      uint32 // KiB
	Iterations  uint32
	Parallelism uint8
	SaltLen     uint32
	KeyLen      uint32
}

// DefaultArgon2 is the launch default: 64 MiB, 3 iterations, parallelism 2,
// 16-byte salt, 32-byte key (technical/05 §1).
var DefaultArgon2 = Argon2Params{Memory: 64 * 1024, Iterations: 3, Parallelism: 2, SaltLen: 16, KeyLen: 32}

// ErrInvalidHash is returned when a stored PHC string cannot be parsed.
var ErrInvalidHash = errors.New("auth: invalid password hash")

// HashPassword hashes pw with the default Argon2id parameters and returns a PHC
// encoded string: $argon2id$v=19$m=...,t=...,p=...$<b64 salt>$<b64 hash>.
func HashPassword(pw string) (string, error) {
	return hashPasswordWith(pw, DefaultArgon2)
}

func hashPasswordWith(pw string, p Argon2Params) (string, error) {
	salt := make([]byte, p.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: salt: %w", err)
	}
	key := argon2.IDKey([]byte(pw), salt, p.Iterations, p.Memory, p.Parallelism, p.KeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.Memory, p.Iterations, p.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword reports whether pw matches the PHC-encoded hash, using a
// constant-time comparison. needsRehash is true when the stored parameters
// differ from the current defaults, so the caller can transparently re-hash on a
// successful login (technical/05 §1).
func VerifyPassword(phc, pw string) (ok, needsRehash bool, err error) {
	p, salt, hash, err := parsePHC(phc)
	if err != nil {
		return false, false, err
	}
	computed := argon2.IDKey([]byte(pw), salt, p.Iterations, p.Memory, p.Parallelism, uint32(len(hash))) //nolint:gosec // hash length is small and bounded by the PHC format
	ok = subtle.ConstantTimeCompare(computed, hash) == 1
	needsRehash = ok && p != DefaultArgon2
	return ok, needsRehash, nil
}

func parsePHC(phc string) (Argon2Params, []byte, []byte, error) {
	parts := strings.Split(phc, "$")
	// ["", "argon2id", "v=19", "m=..,t=..,p=..", "<salt>", "<hash>"]
	if len(parts) != 6 || parts[1] != "argon2id" {
		return Argon2Params{}, nil, nil, ErrInvalidHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return Argon2Params{}, nil, nil, ErrInvalidHash
	}
	var p Argon2Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Iterations, &p.Parallelism); err != nil {
		return Argon2Params{}, nil, nil, ErrInvalidHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return Argon2Params{}, nil, nil, ErrInvalidHash
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return Argon2Params{}, nil, nil, ErrInvalidHash
	}
	p.SaltLen = uint32(len(salt)) //nolint:gosec // salt length is small and bounded
	p.KeyLen = uint32(len(hash))  //nolint:gosec // key length is small and bounded
	return p, salt, hash, nil
}
