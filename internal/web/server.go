// Package web exposes Nuage's core engine over a JSON HTTP API. It has no
// upload/download/index logic of its own — every handler is a thin
// translation between HTTP and internal/core.
package web

import (
	"net/http"

	"github.com/joseph0x45/nuage/internal/core"
)

// Server is the JSON API for Nuage: upload, list, and download files
// through the shared core engine. It implements http.Handler.
type Server struct {
	engine *core.Engine
	mux    *http.ServeMux
}

// NewServer builds a Server backed by engine.
func NewServer(engine *core.Engine) *Server {
	s := &Server{engine: engine, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.mux.HandleFunc("POST /api/files", s.handleUpload)
	s.mux.HandleFunc("GET /api/files", s.handleList)
	s.mux.HandleFunc("GET /api/files/{id}", s.handleDownload)
}
