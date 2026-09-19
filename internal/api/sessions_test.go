package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aixgo-dev/sync/internal/api"
	"github.com/aixgo-dev/sync/internal/store"
)

func TestHealthz(t *testing.T) {
	memStore := store.NewMemoryStore()
	server := api.NewServer(memStore)
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	res, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatalf("failed to GET /healthz: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}

	if string(body) != "ok" {
		t.Errorf("expected body 'ok', got %q", string(body))
	}
}

func TestSessionLifecycle(t *testing.T) {
	memStore := store.NewMemoryStore()
	server := api.NewServer(memStore)
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	client := ts.Client()

	// 1. GET missing session -> should return 404
	{
		res, err := client.Get(ts.URL + "/v1/orgs/myorg/projects/myproj/sessions/sess1")
		if err != nil {
			t.Fatalf("GET failed: %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", res.StatusCode)
		}

		var errRes map[string]map[string]string
		if err := json.NewDecoder(res.Body).Decode(&errRes); err != nil {
			t.Fatalf("failed to decode error body: %v", err)
		}
		if errRes["error"]["code"] != "not_found" {
			t.Errorf("expected error code 'not_found', got %q", errRes["error"]["code"])
		}
	}

	// 2. PUT session with invalid ID/characters -> should return 400
	{
		sessionBody := `{"status":"open","thread_id":"thread1"}`
		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/v1/orgs/myorg!/projects/myproj/sessions/sess1", strings.NewReader(sessionBody))
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("PUT failed: %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 for invalid characters in org ID, got %d", res.StatusCode)
		}
	}

	// 3. PUT create session with mismatching org_id in body -> should return 400
	{
		sessionBody := `{"org_id":"otherorg","status":"open","thread_id":"thread1"}`
		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/v1/orgs/myorg/projects/myproj/sessions/sess1", strings.NewReader(sessionBody))
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("PUT failed: %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 for mismatched org_id, got %d", res.StatusCode)
		}
	}

	// 4. PUT create session with mismatching project_id in body -> should return 400
	{
		sessionBody := `{"project_id":"otherproj","status":"open","thread_id":"thread1"}`
		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/v1/orgs/myorg/projects/myproj/sessions/sess1", strings.NewReader(sessionBody))
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("PUT failed: %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 for mismatched project_id, got %d", res.StatusCode)
		}
	}

	// 5. PUT create session with mismatching id in body -> should return 400
	{
		sessionBody := `{"id":"otherid","status":"open","thread_id":"thread1"}`
		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/v1/orgs/myorg/projects/myproj/sessions/sess1", strings.NewReader(sessionBody))
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("PUT failed: %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 for mismatched session id, got %d", res.StatusCode)
		}
	}

	// 6. PUT create session (success)
	var etag1 string
	{
		sessionBody := `{"org_id":"myorg","project_id":"myproj","status":"open","thread_id":"thread1"}`
		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/v1/orgs/myorg/projects/myproj/sessions/sess1", strings.NewReader(sessionBody))
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("PUT failed: %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", res.StatusCode)
		}

		etag1 = res.Header.Get("ETag")
		if etag1 == "" {
			t.Errorf("expected ETag header in response")
		}

		var created api.Session
		if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if created.ID != "sess1" || created.OrgID != "myorg" || created.ProjectID != "myproj" || created.Status != "open" || created.ThreadID != "thread1" {
			t.Errorf("unexpected session body: %+v", created)
		}
		if created.UpdatedAt == "" {
			t.Errorf("expected updated_at to be populated by server")
		}
	}

	// 7. GET the newly created session -> should return the same payload + ETag
	{
		res, err := client.Get(ts.URL + "/v1/orgs/myorg/projects/myproj/sessions/sess1")
		if err != nil {
			t.Fatalf("GET failed: %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", res.StatusCode)
		}

		etagGet := res.Header.Get("ETag")
		if etagGet != etag1 {
			t.Errorf("expected ETag %q, got %q", etag1, etagGet)
		}

		var fetched api.Session
		if err := json.NewDecoder(res.Body).Decode(&fetched); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if fetched.ID != "sess1" || fetched.OrgID != "myorg" || fetched.ProjectID != "myproj" || fetched.Status != "open" || fetched.ThreadID != "thread1" {
			t.Errorf("unexpected fetched session body: %+v", fetched)
		}
	}

	// 8. PUT update with stale If-Match -> should return 412
	{
		sessionBody := `{"status":"closed","thread_id":"thread1"}`
		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/v1/orgs/myorg/projects/myproj/sessions/sess1", strings.NewReader(sessionBody))
		req.Header.Set("If-Match", "staleetag")
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("PUT failed: %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusPreconditionFailed {
			t.Errorf("expected 412, got %d", res.StatusCode)
		}

		var errRes map[string]map[string]string
		if err := json.NewDecoder(res.Body).Decode(&errRes); err != nil {
			t.Fatalf("failed to decode error body: %v", err)
		}
		if errRes["error"]["code"] != "precondition_failed" {
			t.Errorf("expected error code 'precondition_failed', got %q", errRes["error"]["code"])
		}
	}

	// 9. PUT update without If-Match when session exists -> should return 412 (per store create-only rules)
	{
		sessionBody := `{"status":"closed","thread_id":"thread1"}`
		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/v1/orgs/myorg/projects/myproj/sessions/sess1", strings.NewReader(sessionBody))
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("PUT failed: %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusPreconditionFailed {
			t.Errorf("expected 412 for missing If-Match when updating, got %d", res.StatusCode)
		}
	}

	// 10. PUT update with correct If-Match -> should succeed
	{
		sessionBody := `{"status":"closed","thread_id":"thread2"}`
		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/v1/orgs/myorg/projects/myproj/sessions/sess1", strings.NewReader(sessionBody))
		req.Header.Set("If-Match", etag1)
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("PUT failed: %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", res.StatusCode)
		}

		etag2 := res.Header.Get("ETag")
		if etag2 == "" || etag2 == etag1 {
			t.Errorf("expected new distinct ETag header in response, got %q", etag2)
		}

		var updated api.Session
		if err := json.NewDecoder(res.Body).Decode(&updated); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if updated.Status != "closed" || updated.ThreadID != "thread2" {
			t.Errorf("unexpected updated session body: %+v", updated)
		}
	}
}
