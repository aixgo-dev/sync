package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
		s.errorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate job kind: (code_offload|command|schedule|custom)
	switch req.Kind {
	case "code_offload", "command", "schedule", "custom":
		// valid
	default:
		s.errorResponse(w, http.StatusBadRequest, "invalid job kind: must be 'code_offload', 'command', 'schedule', or 'custom'")
		return
	}

	// Generate a unique Job ID
	jobID, err := generateJobID()
	if err != nil {
		s.errorResponse(w, http.StatusInternalServerError, "failed to generate job ID")
		return
	}

	// Validate the IDs by building the Store key
	key, err := keys.JobKey(org, project, jobID)
	if err != nil {
		s.errorResponse(w, http.StatusBadRequest, err.Error())
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
		s.errorResponse(w, http.StatusInternalServerError, "failed to serialize job")
		return
	}

	// Put in store with empty ifMatch (create-only)
	meta, err := s.store.Put(r.Context(), key, data, "")
	if err != nil {
		if errors.Is(err, store.ErrAlreadyExists) {
			s.errorResponse(w, http.StatusConflict, "job ID conflict")
			return
		}
		s.errorResponse(w, http.StatusInternalServerError, "failed to store job: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("ETag", fmt.Sprintf(`"%s"`, meta.ETag))
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write(data)
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	project := r.PathValue("project")
	id := r.PathValue("id")

	key, err := keys.JobKey(org, project, id)
	if err != nil {
		s.errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	data, meta, err := s.store.Get(r.Context(), key)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.errorResponse(w, http.StatusNotFound, "job not found")
			return
		}
		s.errorResponse(w, http.StatusInternalServerError, "failed to retrieve job: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("ETag", fmt.Sprintf(`"%s"`, meta.ETag))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) handleClaimJob(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	project := r.PathValue("project")
	id := r.PathValue("id")

	var req ClaimJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.errorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.WorkerID == "" {
		s.errorResponse(w, http.StatusBadRequest, "worker_id is required")
		return
	}

	key, err := keys.JobKey(org, project, id)
	if err != nil {
		s.errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	// 1. Get the current job from the store
	data, meta, err := s.store.Get(r.Context(), key)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.errorResponse(w, http.StatusNotFound, "job not found")
			return
		}
		s.errorResponse(w, http.StatusInternalServerError, "failed to retrieve job: "+err.Error())
		return
	}

	var job Job
	if err := json.Unmarshal(data, &job); err != nil {
		s.errorResponse(w, http.StatusInternalServerError, "failed to parse job data")
		return
	}

	// 2. Validate state (must be queued)
	if job.Status != "queued" {
		s.errorResponse(w, http.StatusConflict, "job is already claimed or not in queued state")
		return
	}

	// 3. Mutate job to claimed state
	job.Status = "claimed"
	job.AssigneeWorker = req.WorkerID
	job.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	updatedData, err := json.Marshal(job)
	if err != nil {
		s.errorResponse(w, http.StatusInternalServerError, "failed to serialize job update")
		return
	}

	// 4. Perform CAS update in the Store using current ETag
	newMeta, err := s.store.Put(r.Context(), key, updatedData, meta.ETag)
	if err != nil {
		if errors.Is(err, store.ErrPreconditionFailed) {
			s.errorResponse(w, http.StatusPreconditionFailed, "concurrent modification conflict")
			return
		}
		s.errorResponse(w, http.StatusInternalServerError, "failed to update job state: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("ETag", fmt.Sprintf(`"%s"`, newMeta.ETag))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(updatedData)
}

// generateJobID returns a random 128-bit hex job ID.
func generateJobID() (string, error) {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
