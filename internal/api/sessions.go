package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/aixgo-dev/sync/internal/keys"
	"github.com/aixgo-dev/sync/internal/store"
)

// Session represents a live or hibernated agent conversation.
type Session struct {
	ID        string `json:"id"`
	OrgID     string `json:"org_id"`
	ProjectID string `json:"project_id"`
	Status    string `json:"status"` // "open" | "closed"
	ThreadID  string `json:"thread_id"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// handleGetSession handles GET /v1/orgs/{org}/projects/{project}/sessions/{id}
func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	project := r.PathValue("project")
	id := r.PathValue("id")

	key, err := keys.SessionKey(org, project, id)
	if err != nil {
		sendJSONError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	data, meta, err := s.store.Get(r.Context(), key)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			sendJSONError(w, http.StatusNotFound, "not_found", "session not found")
			return
		}
		sendJSONError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("ETag", meta.ETag)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// handlePutSession handles PUT /v1/orgs/{org}/projects/{project}/sessions/{id}
func (s *Server) handlePutSession(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	project := r.PathValue("project")
	id := r.PathValue("id")

	key, err := keys.SessionKey(org, project, id)
	if err != nil {
		sendJSONError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	var session Session
	if err := json.NewDecoder(r.Body).Decode(&session); err != nil {
		sendJSONError(w, http.StatusBadRequest, "bad_request", "invalid JSON body: "+err.Error())
		return
	}

	// Validate path org/project/id mismatch against body if fields are non-empty.
	if session.OrgID != "" && session.OrgID != org {
		sendJSONError(w, http.StatusBadRequest, "bad_request", "org_id in body does not match path")
		return
	}
	if session.ProjectID != "" && session.ProjectID != project {
		sendJSONError(w, http.StatusBadRequest, "bad_request", "project_id in body does not match path")
		return
	}
	if session.ID != "" && session.ID != id {
		sendJSONError(w, http.StatusBadRequest, "bad_request", "id in body does not match path")
		return
	}

	// Enforce fields to match path.
	session.ID = id
	session.OrgID = org
	session.ProjectID = project

	// Default status if empty, and validate status.
	if session.Status == "" {
		session.Status = "open"
	} else if session.Status != "open" && session.Status != "closed" {
		sendJSONError(w, http.StatusBadRequest, "bad_request", "status must be 'open' or 'closed'")
		return
	}

	// Set/override updated_at with current RFC3339 timestamp.
	session.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	data, err := json.Marshal(session)
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, "internal_error", "failed to marshal session: "+err.Error())
		return
	}

	// Get If-Match header and normalize (trim quotes if present).
	ifMatch := r.Header.Get("If-Match")
	ifMatch = strings.Trim(ifMatch, `"`)

	meta, err := s.store.Put(r.Context(), key, data, ifMatch)
	if err != nil {
		if errors.Is(err, store.ErrPreconditionFailed) || errors.Is(err, store.ErrAlreadyExists) {
			sendJSONError(w, http.StatusPreconditionFailed, "precondition_failed", "CAS conflict: stale or missing ETag")
			return
		}
		sendJSONError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("ETag", meta.ETag)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
