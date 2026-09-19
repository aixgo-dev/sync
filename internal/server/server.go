package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aixgo-dev/sync/internal/keys"
	"github.com/aixgo-dev/sync/internal/store"
)

// Message represents a thread message.
type Message struct {
	ID            string         `json:"id"`
	ThreadID      string         `json:"thread_id"`
	SenderActorID string         `json:"sender_actor_id"`
	Role          string         `json:"role"` // user|assistant|system|tool|agent
	Content       string         `json:"content"`
	Refs          map[string]any `json:"refs,omitempty"`
	CreatedAt     string         `json:"created_at"`
}

// Session represents a coordination session.
type Session struct {
	ID        string            `json:"id"`
	Status    string            `json:"status"` // open|closed
	WorkerID  string            `json:"worker_id,omitempty"`
	ThreadID  string            `json:"thread_id"`
	Labels    map[string]string `json:"labels,omitempty"`
	CreatedAt string            `json:"created_at"`
	UpdatedAt string            `json:"updated_at"`
}

// Server defines the HTTP server that wraps a Store and routes coordination APIs.
type Server struct {
	store store.Store
}

// NewServer initializes a Server with the given CAS Store.
func NewServer(s store.Store) *Server {
	return &Server{store: s}
}

// Handler returns the HTTP handler containing all the routed API endpoints.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("GET /healthz", s.handleHealthz)

	// Messages endpoints
	mux.HandleFunc("POST /v1/orgs/{org}/projects/{project}/threads/{thread_id}/messages", s.handlePostMessage)
	mux.HandleFunc("GET /v1/orgs/{org}/projects/{project}/threads/{thread_id}/messages", s.handleGetMessages)

	// Sessions endpoints (CAS support)
	mux.HandleFunc("GET /v1/orgs/{org}/projects/{project}/sessions/{session_id}", s.handleGetSession)
	mux.HandleFunc("PUT /v1/orgs/{org}/projects/{project}/sessions/{session_id}", s.handlePutSession)

	return mux
}

// handleHealthz responds with 200 OK and basic health status.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// handlePostMessage handles message appends (POST).
func (s *Server) handlePostMessage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org := r.PathValue("org")
	project := r.PathValue("project")
	threadID := r.PathValue("thread_id")

	// Validate path parameters
	if !isValidID(org) || !isValidID(project) || !isValidID(threadID) {
		s.errorResponse(w, http.StatusBadRequest, "Invalid path parameters")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.errorResponse(w, http.StatusBadRequest, "Failed to read request body")
		return
	}
	defer r.Body.Close()

	// We decode input data into a temporary struct
	var input struct {
		SenderActorID string         `json:"sender_actor_id"`
		Role          string         `json:"role"`
		Content       string         `json:"content"`
		Refs          map[string]any `json:"refs,omitempty"`
	}
	if err := json.Unmarshal(body, &input); err != nil {
		s.errorResponse(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	// Validate fields
	if input.Role == "" {
		s.errorResponse(w, http.StatusBadRequest, "Missing required field 'role'")
		return
	}
	if !isValidRole(input.Role) {
		s.errorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid role %q", input.Role))
		return
	}

	// Assign ID and timestamp
	msgID := generateID("msg")
	createdAt := time.Now().UTC().Format(time.RFC3339Nano)

	msg := Message{
		ID:            msgID,
		ThreadID:      threadID,
		SenderActorID: input.SenderActorID,
		Role:          input.Role,
		Content:       input.Content,
		Refs:          input.Refs,
		CreatedAt:     createdAt,
	}

	msgData, err := json.Marshal(msg)
	if err != nil {
		s.errorResponse(w, http.StatusInternalServerError, "Failed to serialize message")
		return
	}

	// Build exact key
	key, err := keys.MessageKey(org, project, threadID, msgID)
	if err != nil {
		s.errorResponse(w, http.StatusBadRequest, fmt.Sprintf("Failed to construct key: %v", err))
		return
	}

	// Write create-only (empty ifMatch) to store
	meta, err := s.store.Put(ctx, key, msgData, "")
	if err != nil {
		if errors.Is(err, store.ErrAlreadyExists) {
			s.errorResponse(w, http.StatusConflict, "Message already exists")
			return
		}
		s.errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Storage error: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("ETag", fmt.Sprintf(`"%s"`, meta.ETag))
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write(msgData)
}

// handleGetMessages lists messages in stable order under a thread (GET).
func (s *Server) handleGetMessages(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org := r.PathValue("org")
	project := r.PathValue("project")
	threadID := r.PathValue("thread_id")

	// Validate path parameters
	if !isValidID(org) || !isValidID(project) || !isValidID(threadID) {
		s.errorResponse(w, http.StatusBadRequest, "Invalid path parameters")
		return
	}

	// Construct message prefix for this thread
	prefix := fmt.Sprintf("tenants/%s/projects/%s/threads/%s/messages/", org, project, threadID)

	// List keys matching prefix
	allKeys, err := s.store.List(ctx, prefix)
	if err != nil {
		s.errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Storage list error: %v", err))
		return
	}

	// Load each message object
	var messages []Message
	for _, key := range allKeys {
		// Just to be fully safe, parse key to ensure it is actually a valid KindMessage key
		info, err := keys.Parse(key)
		if err != nil || info.Kind != keys.KindMessage || info.OrgID != org || info.ProjectID != project || info.ThreadID != threadID {
			continue // skip mismatched keys (e.g. if we somehow listed non-matching structures)
		}

		data, _, err := s.store.Get(ctx, key)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				continue // could be deleted concurrently
			}
			s.errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Storage get error: %v", err))
			return
		}

		var msg Message
		if err := json.Unmarshal(data, &msg); err != nil {
			s.errorResponse(w, http.StatusInternalServerError, "Malformed stored message JSON")
			return
		}
		messages = append(messages, msg)
	}

	// Sort messages deterministically: Ascending created_at, then Ascending id
	sort.Slice(messages, func(i, j int) bool {
		if messages[i].CreatedAt != messages[j].CreatedAt {
			return messages[i].CreatedAt < messages[j].CreatedAt
		}
		return messages[i].ID < messages[j].ID
	})

	// Handle cursor/after_id pagination
	afterID := r.URL.Query().Get("after_id")
	if afterID == "" {
		afterID = r.URL.Query().Get("cursor")
	}

	if afterID != "" {
		foundIdx := -1
		for i, msg := range messages {
			if msg.ID == afterID {
				foundIdx = i
				break
			}
		}
		if foundIdx != -1 {
			messages = messages[foundIdx+1:]
		} else {
			// If cursor was provided but not found, represent an empty page
			messages = []Message{}
		}
	}

	// Apply limit
	limitStr := r.URL.Query().Get("limit")
	limit := 100 // standard default limit
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	if len(messages) > limit {
		messages = messages[:limit]
	}

	// Render response
	response := struct {
		Messages []Message `json:"messages"`
	}{
		Messages: messages,
	}

	respData, err := json.Marshal(response)
	if err != nil {
		s.errorResponse(w, http.StatusInternalServerError, "Failed to marshal response")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(respData)
}

// handleGetSession retrieves session metadata (GET).
func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org := r.PathValue("org")
	project := r.PathValue("project")
	sessionID := r.PathValue("session_id")

	if !isValidID(org) || !isValidID(project) || !isValidID(sessionID) {
		s.errorResponse(w, http.StatusBadRequest, "Invalid path parameters")
		return
	}

	key, err := keys.SessionKey(org, project, sessionID)
	if err != nil {
		s.errorResponse(w, http.StatusBadRequest, fmt.Sprintf("Failed to construct session key: %v", err))
		return
	}

	data, meta, err := s.store.Get(ctx, key)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.errorResponse(w, http.StatusNotFound, "Session not found")
			return
		}
		s.errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Storage error: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("ETag", fmt.Sprintf(`"%s"`, meta.ETag))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// handlePutSession writes or updates session metadata with CAS check (PUT).
func (s *Server) handlePutSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org := r.PathValue("org")
	project := r.PathValue("project")
	sessionID := r.PathValue("session_id")

	if !isValidID(org) || !isValidID(project) || !isValidID(sessionID) {
		s.errorResponse(w, http.StatusBadRequest, "Invalid path parameters")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.errorResponse(w, http.StatusBadRequest, "Failed to read body")
		return
	}
	defer r.Body.Close()

	var sess Session
	if err := json.Unmarshal(body, &sess); err != nil {
		s.errorResponse(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	// Ensure fields are assigned
	sess.ID = sessionID
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if sess.CreatedAt == "" {
		sess.CreatedAt = now
	}
	sess.UpdatedAt = now

	sessData, err := json.Marshal(sess)
	if err != nil {
		s.errorResponse(w, http.StatusInternalServerError, "Failed to serialize session")
		return
	}

	key, err := keys.SessionKey(org, project, sessionID)
	if err != nil {
		s.errorResponse(w, http.StatusBadRequest, fmt.Sprintf("Failed to construct session key: %v", err))
		return
	}

	// CAS If-Match header processing
	ifMatch := r.Header.Get("If-Match")
	ifMatch = strings.Trim(ifMatch, `"`) // strip surrounding quotes if present

	meta, err := s.store.Put(ctx, key, sessData, ifMatch)
	if err != nil {
		if errors.Is(err, store.ErrAlreadyExists) {
			s.errorResponse(w, http.StatusConflict, "Session already exists (cannot create-only over existing key)")
			return
		}
		if errors.Is(err, store.ErrPreconditionFailed) {
			s.errorResponse(w, http.StatusPreconditionFailed, "Precondition failed (CAS mismatch)")
			return
		}
		s.errorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Storage error: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("ETag", fmt.Sprintf(`"%s"`, meta.ETag))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(sessData)
}

// errorResponse writes a standard JSON error payload.
func (s *Server) errorResponse(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	response := map[string]string{"error": msg}
	data, _ := json.Marshal(response)
	_, _ = w.Write(data)
}

// Helper: isValidRole checks role constraints.
func isValidRole(role string) bool {
	switch role {
	case "user", "assistant", "system", "tool", "agent":
		return true
	default:
		return false
	}
}

// Helper: isValidID validates charset safety [A-Za-z0-9._-]+.
func isValidID(id string) bool {
	if id == "" || id == "." || id == ".." {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '.' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

// Helper: generateID generates clean random IDs matching character constraints.
func generateID(prefix string) string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		log.Printf("rand.Read error: %v", err)
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(b))
}
