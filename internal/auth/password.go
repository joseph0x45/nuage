// Package auth implements Nuage's web UI login: named user profiles
// (bcrypt-hashed passwords) and signed, stateless session cookies that
// encode which profile is logged in.
package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword bcrypt-hashes password for storage in config.json.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

// CheckPassword reports whether password matches the bcrypt hash produced
// by HashPassword.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// GenerateSecret returns a fresh random key for signing session tokens, hex/
// base64-encoded for storage in config.json.
func GenerateSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate session secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
