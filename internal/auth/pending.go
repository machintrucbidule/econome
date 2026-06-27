package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"strconv"
	"strings"
	"time"
)

// The login 2FA step (functional/01 §3): once the password is correct on a
// 2FA-enabled account, the server does NOT open a session — it hands the browser
// a short-lived signed token carrying the user id + the "remember me" choice. The
// TOTP step posts that token back with the code; the session is opened only after
// the code verifies. The token is stateless (HMAC under the server secret), so no
// half-authenticated session is ever persisted.

// pendingTTL bounds how long the 2FA step stays valid.
const pendingTTL = 10 * time.Minute

// SignPending returns a signed token "userID.remember.expiryUnix.<mac>".
func SignPending(secret []byte, userID int64, remember bool, now time.Time) string {
	rb := "0"
	if remember {
		rb = "1"
	}
	payload := strconv.FormatInt(userID, 10) + "." + rb + "." + strconv.FormatInt(now.Add(pendingTTL).Unix(), 10)
	return payload + "." + pendingMAC(secret, payload)
}

// VerifyPending validates a pending-2FA token and returns the carried user id +
// remember flag. An expired, malformed, or tampered token yields ok=false.
func VerifyPending(secret []byte, token string, now time.Time) (userID int64, remember, ok bool) {
	i := strings.LastIndex(token, ".")
	if i <= 0 {
		return 0, false, false
	}
	payload, mac := token[:i], token[i+1:]
	if subtle.ConstantTimeCompare([]byte(pendingMAC(secret, payload)), []byte(mac)) != 1 {
		return 0, false, false
	}
	parts := strings.Split(payload, ".")
	if len(parts) != 3 {
		return 0, false, false
	}
	uid, err1 := strconv.ParseInt(parts[0], 10, 64)
	exp, err2 := strconv.ParseInt(parts[2], 10, 64)
	if err1 != nil || err2 != nil {
		return 0, false, false
	}
	if now.Unix() > exp {
		return 0, false, false
	}
	return uid, parts[1] == "1", true
}

func pendingMAC(secret []byte, payload string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte("pending2fa:"))
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
