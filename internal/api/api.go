package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/aixgo-dev/sync/internal/keys"
	"github.com/aixgo-dev/sync/internal/store"
)

// Job represents a background execution task, conforming to both the
// general PRD envelope schema and the specific Job field definitions.
type Job struct {
	ID             string      `json:"id"`
	OrgID          string      `json:"org_id"`
	ProjectID      string      `json:"project_id"`
	Type           string      `json:"type"`
	Kind           string      `json:"kind"`
	Status         string      `json:"status"`
	Payload        interface{} `json:"payload,omitempty"`
	AssigneeWorker string      `json:"assignee_worker,omitempty"`
	Result         interface{} `json:"result,omitempty"`
	CreatedAt      string      `json:"created_at"`
	UpdatedAt      string      `json:"updated_at"`
}

// Server handles the Sync HTTP API.
type Server struct {
	store store.Store
}

// NewServer creates a new API Server instance with the given backing Store.
func NewServer(s store.Store) *Server {
	return &Server{store: s}
}

// NewHandler returns an http.Handler with all API routes registered.
// Uses Go 1.22+ standard library pattern routing.
func (s *Server) NewHandler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /v1/orgs/{org}/projects/{project}/jobs", s.handleCreateJob)
	mux.HandleFunc("GET /v1/orgs/{org}/projects/{project}/jobs/{id}", s.handleGetJob)
	mux.HandleFunc("POST /v1/orgs/{org}/projects/{project}/jobs/{id}/claim", s.handleClaimJob)

	return mux
}

// CreateJobRequest is the request body for POST /jobs.
type CreateJobRequest struct {
	Kind    string      `json:"kind"`
	Payload interface{} `json:"payload"`
}

// ClaimJobRequest is the request body for POST /jobs/{id}/claim.
type ClaimJobRequest struct {
	WorkerID string `json:"worker_id"`
}

func (s *Server) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	project := r.PathValue("project")

	var req CreateJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Validate job kind: (code_offload|command|schedule|custom)
	switch req.Kind {
	case "code_offload", "command", "schedule", "custom":
		// valid
	default:
		writeJSONError(w, "invalid job kind: must be 'code_offload', 'command', 'schedule', or 'custom'", http.StatusBadRequest)
		return
	}

	// Generate a unique Job ID
	jobID, err := generateJobID()
	if err != nil {
		writeJSONError(w, "failed to generate job ID", http.StatusInternalServerError)
		return
	}

	// Validate the IDs by building the Store key
	key, err := keys.JobKey(org, project, jobID)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	job := Job{
		ID:        jobID,
		OrgID:     org,
		ProjectID: project,
		Type:      "job",
		Kind:      req.Kind,
		Status:    "queued",
		Payload:   req.Payload,
		CreatedAt: now,
		UpdatedAt: now,
	}

	data, err := json.Marshal(job)
	if err != nil {
		writeJSONError(w, "failed to serialize job", http.StatusInternalServerError)
		return
	}

	// Put in store with empty ifMatch (create-only)
	meta, err := s.store.Put(r.Context(), key, data, "")
	if err != nil {
		if errors.Is(err, store.ErrAlreadyExists) {
			writeJSONError(w, "job ID conflict", http.StatusConflict)
			return
		}
		writeJSONError(w, "failed to store job: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("ETag", meta.ETag)
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write(data)
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	project := r.PathValue("project")
	id := r.PathValue("id")

	key, err := keys.JobKey(org, project, id)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	data, meta, err := s.store.Get(r.Context(), key)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeJSONError(w, "job not found", http.StatusNotFound)
			return
		}
		writeJSONError(w, "failed to retrieve job: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("ETag", meta.ETag)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) handleClaimJob(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	project := r.PathValue("project")
	id := r.PathValue("id")

	var req ClaimJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.WorkerID == "" {
		writeJSONError(w, "worker_id is required", http.StatusBadRequest)
		return
	}

	key, err := keys.JobKey(org, project, id)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 1. Get the current job from the store
	data, meta, err := s.store.Get(r.Context(), key)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeJSONError(w, "job not found", http.StatusNotFound)
			return
		}
		writeJSONError(w, "failed to retrieve job: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var job Job
	if err := json.Unmarshal(data, &job); err != nil {
		writeJSONError(w, "failed to parse job data", http.StatusInternalServerError)
		return
	}

	// 2. Validate state (must be queued)
	if job.Status != "queued" {
		writeJSONError(w, "job is already claimed or not in queued state", http.StatusConflict)
		return
	}

	// 3. Mutate job to claimed state
	job.Status = "claimed"
	job.AssigneeWorker = req.WorkerID
	job.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	updatedData, err := json.Marshal(job)
	if err != nil {
		writeJSONError(w, "failed to serialize job update", http.StatusInternalServerError)
		return
	}

	// 4. Perform CAS update in the Store using current ETag
	newMeta, err := s.store.Put(r.Context(), key, updatedData, meta.ETag)
	if err != nil {
		if errors.Is(err, store.ErrPreconditionFailed) {
			writeJSONError(w, "concurrent modification conflict", http.StatusPreconditionFailed)
			return
		}
		writeJSONError(w, "failed to update job state: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("ETag", newMeta.ETag)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(updatedData)
}

func writeJSONError(w http.ResponseWriter, errMsg string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": errMsg})
}

func generateJobID() (string, error) {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
