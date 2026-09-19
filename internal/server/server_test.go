package server_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aixgo-dev/sync/internal/server"
	"github.com/aixgo-dev/sync/internal/store"
)

func TestServer_Healthz(t *testing.T) {
	s := store.NewMemoryStore()
	srv := server.NewServer(s)
	handler := srv.Handler()

	req := httptest.NewRequest("GET", "/healthz", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	expected := `{"status":"ok"}`
	if rr.Body.String() != expected {
		t.Fatalf("expected %s, got %s", expected, rr.Body.String())
	}
}

func TestServer_Messages_OrderAndIsolation(t *testing.T) {
	s := store.NewMemoryStore()
	srv := server.NewServer(s)
	handler := srv.Handler()

	// 1. Post messages to Thread A
	org := "org1"
	project := "proj1"
	threadA := "threadA"

	msg1Body := map[string]string{
		"sender_actor_id": "human1",
		"role":            "user",
		"content":         "Hello World 1",
	}
	msg1JSON, _ := json.Marshal(msg1Body)

	// Post first message
	req := httptest.NewRequest("POST", fmt.Sprintf("/v1/orgs/%s/projects/%s/threads/%s/messages", org, project, threadA), bytes.NewReader(msg1JSON))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	var m1 server.Message
	if err := json.Unmarshal(rr.Body.Bytes(), &m1); err != nil {
		t.Fatalf("failed to unmarshal message: %v", err)
	}

	// Wait slightly to ensure a different timestamp for sorting if sorting relies on creation time
	time.Sleep(2 * time.Millisecond)

	msg2Body := map[string]string{
		"sender_actor_id": "agent1",
		"role":            "assistant",
		"content":         "Response 2",
	}
	msg2JSON, _ := json.Marshal(msg2Body)

	// Post second message
	req = httptest.NewRequest("POST", fmt.Sprintf("/v1/orgs/%s/projects/%s/threads/%s/messages", org, project, threadA), bytes.NewReader(msg2JSON))
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rr.Code)
	}

	var m2 server.Message
	if err := json.Unmarshal(rr.Body.Bytes(), &m2); err != nil {
		t.Fatalf("failed to unmarshal message: %v", err)
	}

	// 2. Get messages from Thread A -> check correct order
	req = httptest.NewRequest("GET", fmt.Sprintf("/v1/orgs/%s/projects/%s/threads/%s/messages", org, project, threadA), nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var getResp struct {
		Messages []server.Message `json:"messages"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("failed to parse get response: %v", err)
	}

	if len(getResp.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(getResp.Messages))
	}

	// Verify chronological stable ordering (ascending created_at)
	if getResp.Messages[0].ID != m1.ID {
		t.Fatalf("expected first message to be %s, got %s", m1.ID, getResp.Messages[0].ID)
	}
	if getResp.Messages[1].ID != m2.ID {
		t.Fatalf("expected second message to be %s, got %s", m2.ID, getResp.Messages[1].ID)
	}

	// 3. Post to Thread B and verify complete isolation
	threadB := "threadB"
	msg3Body := map[string]string{
		"sender_actor_id": "human1",
		"role":            "user",
		"content":         "Hello World B",
	}
	msg3JSON, _ := json.Marshal(msg3Body)

	req = httptest.NewRequest("POST", fmt.Sprintf("/v1/orgs/%s/projects/%s/threads/%s/messages", org, project, threadB), bytes.NewReader(msg3JSON))
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rr.Code)
	}

	// Get Thread B
	req = httptest.NewRequest("GET", fmt.Sprintf("/v1/orgs/%s/projects/%s/threads/%s/messages", org, project, threadB), nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	var getRespB struct {
		Messages []server.Message `json:"messages"`
	}
	json.Unmarshal(rr.Body.Bytes(), &getRespB)

	if len(getRespB.Messages) != 1 {
		t.Fatalf("expected 1 message in Thread B, got %d", len(getRespB.Messages))
	}

	// Get Thread A again -> verify still contains only its own 2 messages (Thread B message not leaked!)
	req = httptest.NewRequest("GET", fmt.Sprintf("/v1/orgs/%s/projects/%s/threads/%s/messages", org, project, threadA), nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	json.Unmarshal(rr.Body.Bytes(), &getResp)
	if len(getResp.Messages) != 2 {
		t.Fatalf("expected Thread A to still isolate and contain only 2 messages, got %d", len(getResp.Messages))
	}

	// 4. Cross-tenant isolation (different org)
	org2 := "org2"
	req = httptest.NewRequest("GET", fmt.Sprintf("/v1/orgs/%s/projects/%s/threads/%s/messages", org2, project, threadA), nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	var getRespCross struct {
		Messages []server.Message `json:"messages"`
	}
	json.Unmarshal(rr.Body.Bytes(), &getRespCross)
	if len(getRespCross.Messages) != 0 {
		t.Fatalf("expected 0 messages for different org, got %d", len(getRespCross.Messages))
	}
}

func TestServer_Messages_Validation(t *testing.T) {
	s := store.NewMemoryStore()
	srv := server.NewServer(s)
	handler := srv.Handler()

	org := "org1"
	project := "proj1"
	threadA := "threadA"

	// 1. Invalid role
	invalidRoleBody := map[string]string{
		"sender_actor_id": "human1",
		"role":            "admin", // "admin" is not valid
		"content":         "Hello",
	}
	invalidJSON, _ := json.Marshal(invalidRoleBody)

	req := httptest.NewRequest("POST", fmt.Sprintf("/v1/orgs/%s/projects/%s/threads/%s/messages", org, project, threadA), bytes.NewReader(invalidJSON))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for invalid role, got %d", rr.Code)
	}

	// 2. Missing role
	missingRoleBody := map[string]string{
		"sender_actor_id": "human1",
		"content":         "Hello",
	}
	missingJSON, _ := json.Marshal(missingRoleBody)

	req = httptest.NewRequest("POST", fmt.Sprintf("/v1/orgs/%s/projects/%s/threads/%s/messages", org, project, threadA), bytes.NewReader(missingJSON))
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for missing role, got %d", rr.Code)
	}

	// 3. Valid roles (user|assistant|system|tool|agent)
	validRoles := []string{"user", "assistant", "system", "tool", "agent"}
	for _, role := range validRoles {
		body := map[string]string{
			"sender_actor_id": "tester",
			"role":            role,
			"content":         "test content",
		}
		bJSON, _ := json.Marshal(body)
		req = httptest.NewRequest("POST", fmt.Sprintf("/v1/orgs/%s/projects/%s/threads/%s/messages", org, project, threadA), bytes.NewReader(bJSON))
		rr = httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusCreated {
			t.Fatalf("expected 201 StatusCreated for role %s, got %d. Body: %s", role, rr.Code, rr.Body.String())
		}
	}
}

func TestServer_Messages_Pagination(t *testing.T) {
	s := store.NewMemoryStore()
	srv := server.NewServer(s)
	handler := srv.Handler()

	org := "org1"
	project := "proj1"
	thread := "thread-pag"

	// Create 5 messages
	var ids []string
	for i := 1; i <= 5; i++ {
		body := map[string]string{
			"sender_actor_id": "user1",
			"role":            "user",
			"content":         fmt.Sprintf("Message %d", i),
		}
		bJSON, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", fmt.Sprintf("/v1/orgs/%s/projects/%s/threads/%s/messages", org, project, thread), bytes.NewReader(bJSON))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		var m server.Message
		json.Unmarshal(rr.Body.Bytes(), &m)
		ids = append(ids, m.ID)
		time.Sleep(2 * time.Millisecond) // separate timestamps slightly
	}

	// 1. Retrieve first page with limit=2
	req := httptest.NewRequest("GET", fmt.Sprintf("/v1/orgs/%s/projects/%s/threads/%s/messages?limit=2", org, project, thread), nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	var getResp1 struct {
		Messages []server.Message `json:"messages"`
	}
	json.Unmarshal(rr.Body.Bytes(), &getResp1)

	if len(getResp1.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(getResp1.Messages))
	}
	if getResp1.Messages[0].ID != ids[0] || getResp1.Messages[1].ID != ids[1] {
		t.Fatalf("unexpected message IDs on first page")
	}

	// 2. Retrieve second page using after_id=ids[1] with limit=2
	req = httptest.NewRequest("GET", fmt.Sprintf("/v1/orgs/%s/projects/%s/threads/%s/messages?after_id=%s&limit=2", org, project, thread, ids[1]), nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	var getResp2 struct {
		Messages []server.Message `json:"messages"`
	}
	json.Unmarshal(rr.Body.Bytes(), &getResp2)

	if len(getResp2.Messages) != 2 {
		t.Fatalf("expected 2 messages on second page, got %d", len(getResp2.Messages))
	}
	if getResp2.Messages[0].ID != ids[2] || getResp2.Messages[1].ID != ids[3] {
		t.Fatalf("unexpected message IDs on second page")
	}

	// 3. Retrieve third page using after_id=ids[3] (contains remaining 1 message)
	req = httptest.NewRequest("GET", fmt.Sprintf("/v1/orgs/%s/projects/%s/threads/%s/messages?after_id=%s&limit=2", org, project, thread, ids[3]), nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	var getResp3 struct {
		Messages []server.Message `json:"messages"`
	}
	json.Unmarshal(rr.Body.Bytes(), &getResp3)

	if len(getResp3.Messages) != 1 {
		t.Fatalf("expected 1 message on third page, got %d", len(getResp3.Messages))
	}
	if getResp3.Messages[0].ID != ids[4] {
		t.Fatalf("unexpected message ID on third page")
	}

	// 4. Retrieve fourth page using after_id=ids[4] (should return an empty page at end)
	req = httptest.NewRequest("GET", fmt.Sprintf("/v1/orgs/%s/projects/%s/threads/%s/messages?after_id=%s&limit=2", org, project, thread, ids[4]), nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	var getResp4 struct {
		Messages []server.Message `json:"messages"`
	}
	json.Unmarshal(rr.Body.Bytes(), &getResp4)

	if len(getResp4.Messages) != 0 {
		t.Fatalf("expected empty page at end, got %d messages", len(getResp4.Messages))
	}
}

func TestServer_Sessions_CAS(t *testing.T) {
	s := store.NewMemoryStore()
	srv := server.NewServer(s)
	handler := srv.Handler()

	org := "org1"
	project := "proj1"
	sessionID := "sess1"

	// 1. PUT create session
	sessBody := map[string]any{
		"status":    "open",
		"thread_id": "thread-s1",
	}
	sessJSON, _ := json.Marshal(sessBody)

	req := httptest.NewRequest("PUT", fmt.Sprintf("/v1/orgs/%s/projects/%s/sessions/%s", org, project, sessionID), bytes.NewReader(sessJSON))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for session PUT creation, got %d", rr.Code)
	}

	etag1 := rr.Header().Get("ETag")
	if etag1 == "" {
		t.Fatal("expected non-empty ETag in session response header")
	}

	// 2. GET session
	req = httptest.NewRequest("GET", fmt.Sprintf("/v1/orgs/%s/projects/%s/sessions/%s", org, project, sessionID), nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for session GET, got %d", rr.Code)
	}

	etagGet := rr.Header().Get("ETag")
	if etagGet != etag1 {
		t.Fatalf("expected GET ETag %s to match PUT ETag %s", etag1, etagGet)
	}

	// 3. PUT update session with wrong ETag -> Precondition Failed
	req = httptest.NewRequest("PUT", fmt.Sprintf("/v1/orgs/%s/projects/%s/sessions/%s", org, project, sessionID), bytes.NewReader(sessJSON))
	req.Header.Set("If-Match", `"wrong-etag"`)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusPreconditionFailed {
		t.Fatalf("expected 412 Precondition Failed for wrong ETag, got %d", rr.Code)
	}

	// 4. PUT update session with correct ETag -> success
	req = httptest.NewRequest("PUT", fmt.Sprintf("/v1/orgs/%s/projects/%s/sessions/%s", org, project, sessionID), bytes.NewReader(sessJSON))
	req.Header.Set("If-Match", etag1)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for valid CAS update, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	etag2 := rr.Header().Get("ETag")
	if etag2 == etag1 {
		t.Fatal("expected new ETag to be rotated after successful CAS update")
	}
}
