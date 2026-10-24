package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aixgo-dev/sync/internal/server"
	"github.com/aixgo-dev/sync/internal/store"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func do(t *testing.T, h http.Handler, method, path, token, body string, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	return rr
}

func TestBuildHandler_RefusesWithoutKeys(t *testing.T) {
	for name, e := range map[string]map[string]string{
		"unset":              {},
		"empty":              {"SYNC_API_KEYS": ""},
		"only separators":    {"SYNC_API_KEYS": " , "},
		"opt-out not truthy": {"SYNC_INSECURE_NO_AUTH": "false"},
	} {
		t.Run(name, func(t *testing.T) {
			h, err := buildHandler(store.NewMemoryStore(), env(e))
			if err == nil || h != nil {
				t.Fatalf("expected startup refusal, got handler=%v err=%v", h, err)
			}
			if !strings.Contains(err.Error(), "SYNC_API_KEYS") || !strings.Contains(err.Error(), "SYNC_INSECURE_NO_AUTH") {
				t.Errorf("error should name both env vars, got: %v", err)
			}
		})
	}
}

func TestBuildHandler_InvalidKeys(t *testing.T) {
	if _, err := buildHandler(store.NewMemoryStore(), env(map[string]string{"SYNC_API_KEYS": "not-valid"})); err == nil {
		t.Fatal("expected error for malformed SYNC_API_KEYS")
	}
}

func TestBuildHandler_InsecureOptOut(t *testing.T) {
	for _, v := range []string{"1", "true"} {
		h, err := buildHandler(store.NewMemoryStore(), env(map[string]string{"SYNC_INSECURE_NO_AUTH": v}))
		if err != nil {
			t.Fatalf("opt-out %q: unexpected error: %v", v, err)
		}
		rr := do(t, h, "POST", "/v1/orgs/o/projects/p/jobs", "", `{"kind":"custom"}`, nil)
		if rr.Code != http.StatusCreated {
			t.Errorf("opt-out %q: expected unauthenticated job create to succeed (201), got %d", v, rr.Code)
		}
	}
}

func TestBuildHandler_KeysEnableAuthAndOptOutDoesNotOverride(t *testing.T) {
	// Even with the opt-out set, configured keys keep authentication on.
	h, err := buildHandler(store.NewMemoryStore(), env(map[string]string{
		"SYNC_API_KEYS":         "orgA:projA:tokA",
		"SYNC_INSECURE_NO_AUTH": "1",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if rr := do(t, h, "GET", "/v1/orgs/orgA/projects/projA/sessions/s1", "", "", nil); rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with keys configured, got %d", rr.Code)
	}
}

// TestLiveHandler_AuthAndRoutes exercises the handler the binary serves.
func TestLiveHandler_AuthAndRoutes(t *testing.T) {
	h, err := buildHandler(store.NewMemoryStore(), env(map[string]string{
		"SYNC_API_KEYS": "orgA:projA:tokA,orgB:projB:tokB",
	}))
	if err != nil {
		t.Fatal(err)
	}
	a := "/v1/orgs/orgA/projects/projA"

	// /healthz stays open.
	if rr := do(t, h, "GET", "/healthz", "", "", nil); rr.Code != http.StatusOK {
		t.Errorf("/healthz: expected 200 without key, got %d", rr.Code)
	}

	// 401: missing and invalid keys, on every /v1 route family.
	routes := []struct{ method, path, body string }{
		{"POST", a + "/jobs", `{"kind":"custom"}`},
		{"GET", a + "/jobs/j1", ""},
		{"POST", a + "/jobs/j1/claim", `{"worker_id":"w"}`},
		{"GET", a + "/sessions/s1", ""},
		{"PUT", a + "/sessions/s1", `{"status":"open","thread_id":"t"}`},
		{"POST", a + "/threads/t1/messages", `{"role":"user","content":"hi"}`},
		{"GET", a + "/threads/t1/messages", ""},
	}
	for _, rt := range routes {
		if rr := do(t, h, rt.method, rt.path, "", rt.body, nil); rr.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without key: expected 401, got %d", rt.method, rt.path, rr.Code)
		}
		if rr := do(t, h, rt.method, rt.path, "bogus", rt.body, nil); rr.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with bad key: expected 401, got %d", rt.method, rt.path, rr.Code)
		}
		// 403: key for another tenant.
		if rr := do(t, h, rt.method, rt.path, "tokB", rt.body, nil); rr.Code != http.StatusForbidden {
			t.Errorf("%s %s with other tenant's key: expected 403, got %d", rt.method, rt.path, rr.Code)
		}
	}

	// 403: same org, different project, via a key scoped to orgA/projA.
	if rr := do(t, h, "GET", "/v1/orgs/orgA/projects/projZ/sessions/s1", "tokA", "", nil); rr.Code != http.StatusForbidden {
		t.Errorf("wrong project: expected 403, got %d", rr.Code)
	}

	// 403: paths without org/project are denied even for a valid key.
	for _, p := range []string{"/v1/agents", "/v1/jobs", "/v1/sessions/s1", "/v1/orgs/orgA/jobs", "/v1/orgs/orgA/projects"} {
		if rr := do(t, h, "GET", p, "tokA", "", nil); rr.Code != http.StatusForbidden {
			t.Errorf("GET %s with valid key: expected 403, got %d", p, rr.Code)
		}
	}

	// Jobs: create -> get -> claim -> second claim conflicts.
	rr := do(t, h, "POST", a+"/jobs", "tokA", `{"kind":"command","payload":{"cmd":"echo hi"}}`, nil)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create job: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var job server.Job
	if err := json.Unmarshal(rr.Body.Bytes(), &job); err != nil {
		t.Fatal(err)
	}
	if job.OrgID != "orgA" || job.ProjectID != "projA" || job.Status != "queued" {
		t.Errorf("unexpected job: %+v", job)
	}

	if rr := do(t, h, "GET", a+"/jobs/"+job.ID, "tokA", "", nil); rr.Code != http.StatusOK {
		t.Fatalf("get job: expected 200, got %d", rr.Code)
	}
	// Another tenant cannot read it.
	if rr := do(t, h, "GET", a+"/jobs/"+job.ID, "tokB", "", nil); rr.Code != http.StatusForbidden {
		t.Errorf("cross-tenant get job: expected 403, got %d", rr.Code)
	}

	rr = do(t, h, "POST", a+"/jobs/"+job.ID+"/claim", "tokA", `{"worker_id":"worker-1"}`, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("claim job: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var claimed server.Job
	if err := json.Unmarshal(rr.Body.Bytes(), &claimed); err != nil {
		t.Fatal(err)
	}
	if claimed.Status != "claimed" || claimed.AssigneeWorker != "worker-1" {
		t.Errorf("unexpected claimed job: %+v", claimed)
	}
	if rr := do(t, h, "POST", a+"/jobs/"+job.ID+"/claim", "tokA", `{"worker_id":"worker-2"}`, nil); rr.Code != http.StatusConflict {
		t.Errorf("second claim: expected 409, got %d", rr.Code)
	}

	// Sessions: PUT then GET keeps worker_id and labels.
	rr = do(t, h, "PUT", a+"/sessions/s1", "tokA", `{"status":"open","thread_id":"t1","worker_id":"worker-1","labels":{"k":"v"}}`, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("put session: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	rr = do(t, h, "GET", a+"/sessions/s1", "tokA", "", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("get session: expected 200, got %d", rr.Code)
	}
	var sess server.Session
	if err := json.Unmarshal(rr.Body.Bytes(), &sess); err != nil {
		t.Fatal(err)
	}
	if sess.WorkerID != "worker-1" || sess.Labels["k"] != "v" || sess.ThreadID != "t1" {
		t.Errorf("unexpected session: %+v", sess)
	}

	// Messages round-trip through the authenticated handler.
	if rr := do(t, h, "POST", a+"/threads/t1/messages", "tokA", `{"role":"user","content":"hi"}`, nil); rr.Code != http.StatusCreated {
		t.Fatalf("post message: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	if rr := do(t, h, "GET", a+"/threads/t1/messages", "tokA", "", nil); rr.Code != http.StatusOK {
		t.Fatalf("list messages: expected 200, got %d", rr.Code)
	}
}
