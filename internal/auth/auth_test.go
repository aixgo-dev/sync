package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aixgo-dev/sync/internal/auth"
)

func TestNewKeyStore(t *testing.T) {
	t.Parallel()

	// Valid bootstrap keys
	bootstrap := "orgA:projA:secrettoken1, orgB:projB:secrettoken2"
	ks, err := auth.NewKeyStore(bootstrap)
	if err != nil {
		t.Fatalf("unexpected error creating KeyStore: %v", err)
	}

	// Verify token lookup works and scope is correct
	scope, ok := ks.Lookup("secrettoken1")
	if !ok {
		t.Fatal("expected secrettoken1 to be found")
	}
	if scope.OrgID != "orgA" || scope.ProjectID != "projA" {
		t.Errorf("expected scope orgA:projA, got %s:%s", scope.OrgID, scope.ProjectID)
	}

	scope, ok = ks.Lookup("secrettoken2")
	if !ok {
		t.Fatal("expected secrettoken2 to be found")
	}
	if scope.OrgID != "orgB" || scope.ProjectID != "projB" {
		t.Errorf("expected scope orgB:projB, got %s:%s", scope.OrgID, scope.ProjectID)
	}

	// Verify non-existent token is not found
	_, ok = ks.Lookup("non-existent")
	if ok {
		t.Fatal("expected non-existent token to be missing")
	}

	// Invalid bootstrap formats
	invalidFormats := []string{
		"orgA:projA",             // missing token
		"orgA::token",            // empty project
		":projA:token",            // empty org
		"orgA:projA:token,badkey", // malformed key in list
	}

	for _, invalid := range invalidFormats {
		_, err := auth.NewKeyStore(invalid)
		if err == nil {
			t.Errorf("expected error for bootstrap %q, but got nil", invalid)
		}
	}
}

func TestContextHelpers(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	if auth.OrgIDFromContext(ctx) != "" {
		t.Error("expected empty org ID from empty context")
	}
	if auth.ProjectIDFromContext(ctx) != "" {
		t.Error("expected empty project ID from empty context")
	}

	ctx = context.WithValue(ctx, auth.ContextKeyOrgID, "org1")
	ctx = context.WithValue(ctx, auth.ContextKeyProjectID, "proj1")

	if auth.OrgIDFromContext(ctx) != "org1" {
		t.Errorf("expected org1, got %q", auth.OrgIDFromContext(ctx))
	}
	if auth.ProjectIDFromContext(ctx) != "proj1" {
		t.Errorf("expected proj1, got %q", auth.ProjectIDFromContext(ctx))
	}
}

func TestAuthMiddleware(t *testing.T) {
	t.Parallel()

	bootstrap := "orgA:projA:tokenA,orgB:projB:tokenB"
	ks, err := auth.NewKeyStore(bootstrap)
	if err != nil {
		t.Fatalf("failed to create keystore: %v", err)
	}

	// Define a dummy API handler that responds with the authenticated org/project from context
	dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		org := auth.OrgIDFromContext(r.Context())
		project := auth.ProjectIDFromContext(r.Context())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"success","org":"` + org + `","project":"` + project + `"}`))
	})

	// Wrap the dummy handler in the middleware
	handler := ks.Middleware(dummyHandler)

	// Helper to send requests
	runRequest := func(method, path string, headers map[string]string) *httptest.ResponseRecorder {
		req, err := http.NewRequest(method, path, nil)
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	// 1. healthz no auth
	t.Run("healthz no auth", func(t *testing.T) {
		rec := runRequest("GET", "/healthz", nil)
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200 for healthz, got %d", rec.Code)
		}
	})

	// 2. sessions GET without header -> 401
	t.Run("sessions GET without header", func(t *testing.T) {
		rec := runRequest("GET", "/v1/orgs/orgA/projects/projA/sessions", nil)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401 for unauthenticated request, got %d", rec.Code)
		}
	})

	// 3. invalid / wrong key -> 401
	t.Run("wrong key auth", func(t *testing.T) {
		headers := map[string]string{"Authorization": "Bearer wrongtoken"}
		rec := runRequest("GET", "/v1/orgs/orgA/projects/projA/sessions", headers)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401 for wrong key, got %d", rec.Code)
		}
	})

	t.Run("malformed auth header", func(t *testing.T) {
		headers := map[string]string{"Authorization": "tokenA"}
		rec := runRequest("GET", "/v1/orgs/orgA/projects/projA/sessions", headers)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401 for malformed auth header, got %d", rec.Code)
		}
	})

	// 4. valid key success
	t.Run("valid key success", func(t *testing.T) {
		headers := map[string]string{"Authorization": "Bearer tokenA"}
		rec := runRequest("GET", "/v1/orgs/orgA/projects/projA/sessions", headers)
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200 for valid key, got %d", rec.Code)
		}
		expectedBody := `{"status":"success","org":"orgA","project":"projA"}`
		if rec.Body.String() != expectedBody {
			t.Errorf("expected body %q, got %q", expectedBody, rec.Body.String())
		}
	})

	// 5. cross-tenant 403
	t.Run("cross-tenant 403", func(t *testing.T) {
		// tokenA is authorized for orgA/projA, but we are requesting orgB/projB paths
		headers := map[string]string{"Authorization": "Bearer tokenA"}
		rec := runRequest("GET", "/v1/orgs/orgB/projects/projB/sessions", headers)
		if rec.Code != http.StatusForbidden {
			t.Errorf("expected status 403 for cross-tenant access, got %d", rec.Code)
		}
	})
}
