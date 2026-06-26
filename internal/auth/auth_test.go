package auth

import (
	"strings"
	"testing"
	"time"
)

func TestHashAndVerify(t *testing.T) {
	hash, err := HashPassword("Tr0ub4dour&3xtra")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=65536,t=3,p=2$") {
		t.Errorf("unexpected PHC prefix: %q", hash)
	}
	ok, rehash, err := VerifyPassword(hash, "Tr0ub4dour&3xtra")
	if err != nil || !ok {
		t.Fatalf("verify correct: ok=%v rehash=%v err=%v", ok, rehash, err)
	}
	if rehash {
		t.Error("default-param hash should not need rehash")
	}
	bad, _, err := VerifyPassword(hash, "wrong-password")
	if err != nil || bad {
		t.Errorf("verify wrong: ok=%v err=%v, want false", bad, err)
	}
}

func TestVerifyNeedsRehashOnWeakerParams(t *testing.T) {
	weak := Argon2Params{Memory: 16 * 1024, Iterations: 1, Parallelism: 1, SaltLen: 16, KeyLen: 32}
	hash, err := hashPasswordWith("Tr0ub4dour&3xtra", weak)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	ok, rehash, err := VerifyPassword(hash, "Tr0ub4dour&3xtra")
	if err != nil || !ok {
		t.Fatalf("verify: %v %v", ok, err)
	}
	if !rehash {
		t.Error("weaker-param hash should report needsRehash")
	}
}

func TestVerifyRejectsMalformed(t *testing.T) {
	for _, bad := range []string{"", "notphc", "$argon2id$v=19$bad$x$y", "$argon2i$v=19$m=1,t=1,p=1$AA$BB"} {
		if _, _, err := VerifyPassword(bad, "x"); err == nil {
			t.Errorf("VerifyPassword(%q) err = nil, want error", bad)
		}
	}
}

func TestSessionToken(t *testing.T) {
	a, err := GenerateSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := GenerateSessionToken()
	if a == "" || a == b {
		t.Error("tokens should be non-empty and unique")
	}
	if HashToken(a) == a {
		t.Error("stored hash must differ from the raw token")
	}
	h1, h2 := HashToken(a), HashToken(a)
	if h1 != h2 {
		t.Error("HashToken must be deterministic")
	}
}

func TestCSRF(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	seed, err := GenerateCSRFSeed()
	if err != nil {
		t.Fatal(err)
	}
	tok := CSRFToken(secret, seed)
	if !ValidCSRF(secret, seed, tok) {
		t.Error("valid token rejected")
	}
	if ValidCSRF(secret, seed, tok+"x") {
		t.Error("tampered token accepted")
	}
	if ValidCSRF(secret, "other-seed", tok) {
		t.Error("token accepted under wrong seed")
	}
	if ValidCSRF(secret, "", "") {
		t.Error("empty seed/token accepted")
	}
}

func TestBackoff(t *testing.T) {
	cases := map[int]time.Duration{
		1: 0, 5: 0,
		6: 1 * time.Second, 7: 30 * time.Second, 8: 300 * time.Second, 100: 300 * time.Second,
	}
	for n, want := range cases {
		if got := BackoffFor(n); got != want {
			t.Errorf("BackoffFor(%d) = %v, want %v", n, got, want)
		}
	}
}

func TestThrottle(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	th := NewThrottle(2, time.Minute)
	if !th.Allow("ip", now) {
		t.Fatal("first attempt should pass")
	}
	if !th.Allow("ip", now) {
		t.Fatal("second attempt should pass")
	}
	if th.Allow("ip", now) {
		t.Error("third attempt within window should be blocked")
	}
	if !th.Allow("ip", now.Add(2*time.Minute)) {
		t.Error("attempt after window should pass")
	}
	if !th.Allow("other", now) {
		t.Error("a different key is independent")
	}
}
