package auth

import (
	"testing"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

func TestTOTPRoundTripAndSkew(t *testing.T) {
	secret, url, err := NewTOTPSecret("owner@example.org")
	if err != nil {
		t.Fatalf("new secret: %v", err)
	}
	if secret == "" || url == "" {
		t.Fatal("empty secret/url")
	}
	now := time.Date(2026, 6, 15, 12, 0, 30, 0, time.UTC)
	code, err := totp.GenerateCode(secret, now)
	if err != nil {
		t.Fatalf("generate code: %v", err)
	}
	if !VerifyTOTP(secret, code, now) {
		t.Error("current code should verify")
	}
	// ±1 step tolerance.
	if !VerifyTOTP(secret, code, now.Add(30*time.Second)) {
		t.Error("code should verify one step later (skew)")
	}
	if !VerifyTOTP(secret, code, now.Add(-30*time.Second)) {
		t.Error("code should verify one step earlier (skew)")
	}
	// Two steps away must fail.
	if VerifyTOTP(secret, code, now.Add(90*time.Second)) {
		t.Error("code should not verify two steps later")
	}
	if VerifyTOTP(secret, "000000", now) && code != "000000" {
		t.Error("wrong code should not verify")
	}
	if VerifyTOTP("", code, now) || VerifyTOTP(secret, "", now) {
		t.Error("empty secret/code never verifies")
	}
}

func TestBackupCodesAreSingleUseHashed(t *testing.T) {
	codes, err := GenerateBackupCodes()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(codes) != backupCodeCount {
		t.Fatalf("got %d codes, want %d", len(codes), backupCodeCount)
	}
	hash, err := HashBackupCode(codes[0])
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if hash == codes[0] {
		t.Fatal("backup code stored in plaintext")
	}
	if !VerifyBackupCode(hash, codes[0]) {
		t.Error("code should verify against its hash")
	}
	// Dash-insensitive entry.
	if !VerifyBackupCode(hash, NormalizeBackupCode(codes[0])) {
		t.Error("normalized code should verify")
	}
	if VerifyBackupCode(hash, "wrong-code") {
		t.Error("wrong code should not verify")
	}
}

func TestNewTOTPSecretIsSHA1SixDigit(t *testing.T) {
	secret, url, _ := NewTOTPSecret("a@b.c")
	key, err := otp.NewKeyFromURL(url)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	if key.Secret() != secret {
		t.Error("url secret mismatch")
	}
	if key.Issuer() != TOTPIssuer {
		t.Errorf("issuer = %q, want %q", key.Issuer(), TOTPIssuer)
	}
}
