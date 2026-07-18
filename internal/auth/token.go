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

// NewSessionToken returns a signed token encoding an expiry time.
// It's stateless — verifying it requires only secret, not a server-side
// session store — so the only way to revoke a token early is rotating
// secret (which invalidates every session at once).
func NewSessionToken(secret []byte) string {
	expiry := time.Now().Add(SessionTTL).Unix()
	return signToken(secret, expiry)
}

// VerifySessionToken reports whether token was signed by secret and has not
// expired.
func VerifySessionToken(secret []byte, token string) bool {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return false
	}
	expiryStr, sigB64 := parts[0], parts[1]

	expiry, err := strconv.ParseInt(expiryStr, 10, 64)
	if err != nil {
		return false
	}
	if time.Now().Unix() > expiry {
		return false
	}

	gotSig, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil {
		return false
	}
	wantSig := sign(secret, expiryStr)
	return hmac.Equal(gotSig, wantSig)
}

func signToken(secret []byte, expiry int64) string {
	expiryStr := strconv.FormatInt(expiry, 10)
	sig := sign(secret, expiryStr)
	return fmt.Sprintf("%s.%s", expiryStr, base64.RawURLEncoding.EncodeToString(sig))
}

func sign(secret []byte, payload string) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payload))
	return mac.Sum(nil)
}
