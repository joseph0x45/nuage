package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/joseph0x45/nuage/internal/auth"
)

const sessionCookieName = "nuage_session"

// unknownUserHash is a fixed bcrypt hash (of an arbitrary password no login
// will ever send) checked when the submitted username doesn't exist, so a
// bad-username response costs the same bcrypt comparison as a bad-password
// one instead of returning near-instantly.
const unknownUserHash = "$2a$10$arU1bQYYu.RdVLlkcknA3Oq4B1NwrIpSSy02biKiF5srhXT4awwfC"

type contextKey int

const usernameContextKey contextKey = iota

// usernameFromContext returns the logged-in profile's username, as set by
// requireAuth. Only meaningful inside handlers wrapped by requireAuth.
func usernameFromContext(ctx context.Context) string {
	username, _ := ctx.Value(usernameContextKey).(string)
	return username
}

// maxLoginAttempts is how many failed logins a single IP gets within
// loginLockoutWindow before being locked out — brute-force protection for
// the single shared password, per CLAUDE.md's requirement that the login
// endpoint have rate limiting once this is internet-reachable.
const (
	maxLoginAttempts   = 5
	loginLockoutWindow = 15 * time.Minute
)

type loginLimiter struct {
	mu       sync.Mutex
	failures map[string][]time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{failures: make(map[string][]time.Time)}
}

// allow reports whether ip is currently permitted to attempt a login.
func (l *loginLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.prune(ip)
	return len(l.failures[ip]) < maxLoginAttempts
}

// recordFailure logs a failed attempt from ip.
func (l *loginLimiter) recordFailure(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.prune(ip)
	l.failures[ip] = append(l.failures[ip], time.Now())
}

// recordSuccess clears ip's failure history.
func (l *loginLimiter) recordSuccess(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, ip)
}

// prune must be called with l.mu held.
func (l *loginLimiter) prune(ip string) {
	cutoff := time.Now().Add(-loginLockoutWindow)
	attempts := l.failures[ip]
	kept := attempts[:0]
	for _, t := range attempts {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(l.failures, ip)
	} else {
		l.failures[ip] = kept
	}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.loginLimiter.allow(ip) {
		writeError(w, http.StatusTooManyRequests, fmt.Errorf("too many failed login attempts, try again later"))
		return
	}

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body"))
		return
	}

	hash, exists := s.users[req.Username]
	// Run CheckPassword even when the username doesn't exist, against a
	// fixed placeholder hash, so a nonexistent-username response takes
	// roughly as long as a wrong-password one — a valid username shouldn't
	// be distinguishable by response time.
	if !exists {
		hash = unknownUserHash
	}
	if !exists || !auth.CheckPassword(hash, req.Password) {
		s.loginLimiter.recordFailure(ip)
		writeError(w, http.StatusUnauthorized, fmt.Errorf("incorrect username or password"))
		return
	}
	s.loginLimiter.recordSuccess(ip)

	token := auth.NewSessionToken(s.sessionSecret, req.Username)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  time.Now().Add(auth.SessionTTL),
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteStrictMode,
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteStrictMode,
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// requireAuth wraps a handler so it 401s unless the request carries a valid
// session cookie, and makes the logged-in profile's username available to
// next via usernameFromContext.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			writeError(w, http.StatusUnauthorized, fmt.Errorf("not logged in"))
			return
		}
		username, ok := auth.VerifySessionToken(s.sessionSecret, cookie.Value)
		if !ok {
			writeError(w, http.StatusUnauthorized, fmt.Errorf("not logged in"))
			return
		}
		ctx := context.WithValue(r.Context(), usernameContextKey, username)
		next(w, r.WithContext(ctx))
	}
}

// isHTTPS reports whether the request appears to have arrived over HTTPS,
// either directly or (the expected case in production) via the
// X-Forwarded-Proto header cloudflared sets when proxying from the tunnel.
// Cookies only get the Secure attribute when this is true, so local
// http://127.0.0.1 testing during development still works.
func isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// clientIP returns the originating client address for rate limiting.
// Behind cloudflared, r.RemoteAddr is the local tunnel connection, so the
// real client IP is read from the header Cloudflare sets first.
func clientIP(r *http.Request) string {
	if ip := r.Header.Get("Cf-Connecting-Ip"); ip != "" {
		return ip
	}
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		return strings.TrimSpace(strings.Split(fwd, ",")[0])
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
