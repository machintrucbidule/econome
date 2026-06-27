package auth

import (
	"crypto/rand"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// TOTP two-factor primitives (technical/05 §5, A2/A10): RFC 6238 (30 s step,
// 6 digits, SHA-1), a ±1-step verification skew, and single-use backup codes
// hashed with the same Argon2id family as passwords. crypto/rand is the only
// randomness source.

// TOTPIssuer is the label authenticator apps show for the account.
const TOTPIssuer = "EconoMe"

// totpSkew accepts the adjacent ±1 time steps to tolerate small clock drift
// (technical/05 §5).
const totpSkew = 1

// backupCodeCount is the number of single-use recovery codes issued per set.
const backupCodeCount = 10

// NewTOTPSecret generates a fresh TOTP secret for an account and returns the
// base32 secret (stored on the user) and the otpauth:// URL the handler renders
// as a QR code. The account is the user's email; the issuer is fixed.
func NewTOTPSecret(account string) (secret, otpauthURL string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      TOTPIssuer,
		AccountName: account,
		Period:      30,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1,
	})
	if err != nil {
		return "", "", fmt.Errorf("auth: generate totp: %w", err)
	}
	return key.Secret(), key.URL(), nil
}

// TOTPURLFromSecret rebuilds the otpauth:// URL for an existing secret (so a
// re-render after a wrong confirmation code shows the SAME QR, not a rotated one).
func TOTPURLFromSecret(secret, account string) (string, error) {
	key, err := otp.NewKeyFromURL("otpauth://totp/" + url.PathEscape(TOTPIssuer+":"+account) +
		"?secret=" + secret + "&issuer=" + url.QueryEscape(TOTPIssuer) + "&algorithm=SHA1&digits=6&period=30")
	if err != nil {
		return "", fmt.Errorf("auth: totp url: %w", err)
	}
	return key.URL(), nil
}

// VerifyTOTP reports whether code is a valid 6-digit TOTP for secret at time t,
// accepting the adjacent ±1 step. A blank/secret-less input is never valid.
func VerifyTOTP(secret, code string, t time.Time) bool {
	code = strings.TrimSpace(code)
	if secret == "" || code == "" {
		return false
	}
	ok, err := totp.ValidateCustom(code, secret, t.UTC(), totp.ValidateOpts{
		Period:    30,
		Skew:      totpSkew,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	return err == nil && ok
}

// GenerateBackupCodes returns a fresh set of human-friendly single-use recovery
// codes (e.g. "3f9a-1c2e"), shown once to the user. Hash each with HashBackupCode
// before storing; never persist the plaintext.
func GenerateBackupCodes() ([]string, error) {
	codes := make([]string, 0, backupCodeCount)
	for i := 0; i < backupCodeCount; i++ {
		c, err := randomBackupCode()
		if err != nil {
			return nil, err
		}
		codes = append(codes, c)
	}
	return codes, nil
}

// HashBackupCode hashes a backup code with the password Argon2id family
// (technical/05 §5 — Argon2id preferred for consistency).
func HashBackupCode(code string) (string, error) {
	return HashPassword(NormalizeBackupCode(code))
}

// VerifyBackupCode reports whether a candidate matches a stored backup-code hash
// (constant-time via the Argon2id verify).
func VerifyBackupCode(hash, candidate string) bool {
	ok, _, err := VerifyPassword(hash, NormalizeBackupCode(candidate))
	return err == nil && ok
}

// NormalizeBackupCode lowercases and strips spacing/dashes so a code entered with
// or without its display dash verifies the same.
func NormalizeBackupCode(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	code = strings.ReplaceAll(code, "-", "")
	code = strings.ReplaceAll(code, " ", "")
	return code
}

// RandomPassword returns a 16-character password guaranteed to satisfy the §9
// policy (lower, upper, digit, symbol) — used for admin/CLI temporary passwords.
func RandomPassword() (string, error) {
	const (
		lower  = "abcdefghijkmnpqrstuvwxyz"
		upper  = "ABCDEFGHJKMNPQRSTUVWXYZ"
		digit  = "23456789"
		symbol = "!@#$%&*+?"
	)
	classes := []string{lower, upper, digit, symbol}
	all := lower + upper + digit + symbol
	out := make([]byte, 16)
	b := make([]byte, len(out))
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("auth: random password: %w", err)
	}
	// First four positions guarantee one of each class; the rest are uniform.
	for i := range out {
		if i < len(classes) {
			set := classes[i]
			out[i] = set[int(b[i])%len(set)]
		} else {
			out[i] = all[int(b[i])%len(all)]
		}
	}
	// Rotate so the guaranteed-class characters are not always in the same slots.
	shift := int(b[len(b)-1]) % len(out)
	rotated := make([]byte, 0, len(out))
	rotated = append(rotated, out[shift:]...)
	rotated = append(rotated, out[:shift]...)
	return string(rotated), nil
}

const backupAlphabet = "23456789abcdefghjkmnpqrstuvwxyz" // no 0/1/o/l/i ambiguity

// randomBackupCode builds an 8-character code grouped as "xxxx-xxxx".
func randomBackupCode() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("auth: backup code: %w", err)
	}
	out := make([]byte, 0, 9)
	for i, v := range b {
		if i == 4 {
			out = append(out, '-')
		}
		out = append(out, backupAlphabet[int(v)%len(backupAlphabet)])
	}
	return string(out), nil
}
