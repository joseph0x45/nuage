// Package web exposes Nuage's core engine over a JSON HTTP API. It has no
// upload/download/index logic of its own — every handler is a thin
// translation between HTTP and internal/core.
package web

import (
	"net/http"

	"github.com/joseph0x45/nuage/internal/core"
)

// Server is the JSON API for Nuage: upload, list, and download files
// through the shared core engine, gated by a shared-password login. It
// implements http.Handler.
type Server struct {
	engine        *core.Engine
	mux           *http.ServeMux
	passwordHash  string
	sessionSecret []byte
	loginLimiter  *loginLimiter
}

// NewServer builds a Server backed by engine. passwordHash is the bcrypt
// hash checked at login (from config.WebPasswordHash) and sessionSecret
// signs session cookies (from config.SessionSecret) — both are required;
// callers should refuse to start the server without them configured
// (see `nuage password`).
func NewServer(engine *core.Engine, passwordHash string, sessionSecret []byte) *Server {
	s := &Server{
		engine:        engine,
		mux:           http.NewServeMux(),
		passwordHash:  passwordHash,
		sessionSecret: sessionSecret,
		loginLimiter:  newLoginLimiter(),
	}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.mux.HandleFunc("POST /api/login", s.handleLogin)
	s.mux.HandleFunc("POST /api/logout", s.handleLogout)

	s.mux.HandleFunc("POST /api/files", s.requireAuth(s.handleUpload))
	s.mux.HandleFunc("GET /api/files", s.requireAuth(s.handleList))
	s.mux.HandleFunc("GET /api/files/{id}", s.requireAuth(s.handleDownload))
	s.mux.HandleFunc("DELETE /api/files/{id}", s.requireAuth(s.handleDelete))

	// The HTML/JS/CSS shell is not itself sensitive — it's a static asset
	// that renders a login form. The frontend JS decides what to show
	// based on whether the API calls it makes come back 401. Actual file
	// data only ever flows through the /api/* routes above, which are
	// gated.
	s.mux.Handle("/", http.FileServerFS(staticFS()))
}
