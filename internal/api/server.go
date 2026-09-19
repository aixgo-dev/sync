package api

import (
	"encoding/json"
	"net/http"

	"github.com/aixgo-dev/sync/internal/store"
)

// Server holds the dependencies for the sync API server.
type Server struct {
	store store.Store
}

// NewServer creates a new API Server.
func NewServer(s store.Store) *Server {
	return &Server{
		store: s,
	}
}

// Handler returns the HTTP handler for the server.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Register routes using Go 1.22+ wildcard patterns.
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /v1/orgs/{org}/projects/{project}/sessions/{id}", s.handleGetSession)
	mux.HandleFunc("PUT /v1/orgs/{org}/projects/{project}/sessions/{id}", s.handlePutSession)

	return mux
}

// sendJSONError helper sends a structured JSON error response.
func sendJSONError(w http.ResponseWriter, statusCode int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}
