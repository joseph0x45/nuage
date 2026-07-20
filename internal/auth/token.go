package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// SessionTTL is how long a login stays valid before the browser must
// re-authenticate.
const SessionTTL = 7 * 24 * time.Hour

// NewSessionToken returns a signed token encoding username and an expiry
// time. It's stateless — verifying it requires only secret, not a
// server-side session store — so the only way to revoke a token early is
// rotating secret (which invalidates every session at once).
func NewSessionToken(secret []byte, username string) string {
	expiry := time.Now().Add(SessionTTL).Unix()
	return signToken(secret, username, expiry)
}

// VerifySessionToken reports the username encoded in token if it was signed
// by secret and has not expired.
func VerifySessionToken(secret []byte, token string) (username string, ok bool) {
	parts := strings.SplitN(token, ".", 3)
	if len(parts) != 3 {
		return "", false
	}
	userB64, expiryStr, sigB64 := parts[0], parts[1], parts[2]

	expiry, err := strconv.ParseInt(expiryStr, 10, 64)
	if err != nil {
		return "", false
	}
	if time.Now().Unix() > expiry {
		return "", false
	}

	gotSig, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil {
		return "", false
	}
	payload := userB64 + "." + expiryStr
	wantSig := sign(secret, payload)
	if !hmac.Equal(gotSig, wantSig) {
		return "", false
	}

	userBytes, err := base64.RawURLEncoding.DecodeString(userB64)
	if err != nil {
		return "", false
	}
	return string(userBytes), true
}

func signToken(secret []byte, username string, expiry int64) string {
	userB64 := base64.RawURLEncoding.EncodeToString([]byte(username))
	expiryStr := strconv.FormatInt(expiry, 10)
	payload := userB64 + "." + expiryStr
	sig := sign(secret, payload)
	return fmt.Sprintf("%s.%s", payload, base64.RawURLEncoding.EncodeToString(sig))
}

func sign(secret []byte, payload string) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payload))
	return mac.Sum(nil)
}
