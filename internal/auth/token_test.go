package auth

import (
	"testing"
	"time"
)

func TestSessionTokenRoundTrip(t *testing.T) {
	secret := []byte("test-secret")
	token := NewSessionToken(secret, "joseph")

	username, ok := VerifySessionToken(secret, token)
	if !ok {
		t.Fatal("VerifySessionToken: expected ok=true")
	}
	if username != "joseph" {
		t.Errorf("username = %q, want %q", username, "joseph")
	}
}

func TestSessionTokenWrongSecret(t *testing.T) {
	token := NewSessionToken([]byte("secret-a"), "joseph")
	if _, ok := VerifySessionToken([]byte("secret-b"), token); ok {
		t.Fatal("VerifySessionToken with wrong secret: expected ok=false")
	}
}

func TestSessionTokenTampered(t *testing.T) {
	secret := []byte("test-secret")
	token := NewSessionToken(secret, "joseph")
	tampered := NewSessionToken(secret, "mom")[:len(token)-len(token)/3] + token[len(token)-len(token)/3:]
	if _, ok := VerifySessionToken(secret, tampered); ok {
		t.Fatal("VerifySessionToken with tampered payload: expected ok=false")
	}
}

func TestSessionTokenExpired(t *testing.T) {
	secret := []byte("test-secret")
	expired := signToken(secret, "joseph", time.Now().Add(-time.Minute).Unix())
	if _, ok := VerifySessionToken(secret, expired); ok {
		t.Fatal("VerifySessionToken with expired token: expected ok=false")
	}
}

func TestSessionTokenMalformed(t *testing.T) {
	secret := []byte("test-secret")
	for _, tok := range []string{"", "a", "a.b", "a.b.c.d"} {
		if _, ok := VerifySessionToken(secret, tok); ok {
			t.Errorf("VerifySessionToken(%q): expected ok=false", tok)
		}
	}
}
