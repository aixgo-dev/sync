package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"path"
	"strings"
)

type contextKey string

const (
	ContextKeyOrgID     contextKey = "org_id"
	ContextKeyProjectID contextKey = "project_id"
)

// KeyScope holds the authorized scope of an API key.
type KeyScope struct {
	OrgID     string
	ProjectID string
}

// KeyStore manages authenticated API keys, storing only hashes of the plaintext keys.
type KeyStore struct {
	hashes map[string]KeyScope
}

// OrgIDFromContext extracts the authenticated org_id from the context.
func OrgIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ContextKeyOrgID).(string); ok {
		return v
	}
	return ""
}

// ProjectIDFromContext extracts the authenticated project_id from the context.
func ProjectIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ContextKeyProjectID).(string); ok {
		return v
	}
	return ""
}

// NewKeyStore parses bootstrap keys from a comma-separated string: "org:project:token,..."
// It stores only the SHA-256 hashes of the plaintext tokens in process memory.
func NewKeyStore(bootstrapStr string) (*KeyStore, error) {
	hashes := make(map[string]KeyScope)
	if bootstrapStr == "" {
		return &KeyStore{hashes: hashes}, nil
	}
	pairs := strings.Split(bootstrapStr, ",")
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, ":", 3)
		if len(parts) != 3 {
			return nil, errors.New("invalid bootstrap key format: expected orgID:projectID:token")
		}
		orgID, projectID, token := parts[0], parts[1], parts[2]
		if orgID == "" || projectID == "" || token == "" {
			return nil, errors.New("orgID, projectID, and token must all be non-empty")
		}
		// Hash token and store
		hash := sha256.Sum256([]byte(token))
		hexHash := hex.EncodeToString(hash[:])
		hashes[hexHash] = KeyScope{
			OrgID:     orgID,
			ProjectID: projectID,
		}
	}
	return &KeyStore{hashes: hashes}, nil
}

// Lookup retrieves the scope for a plaintext token, if it exists.
func (s *KeyStore) Lookup(token string) (KeyScope, bool) {
	hash := sha256.Sum256([]byte(token))
	hexHash := hex.EncodeToString(hash[:])
	scope, ok := s.hashes[hexHash]
	return scope, ok
}

// Len returns the number of configured API keys.
func (s *KeyStore) Len() int {
	return len(s.hashes)
}

// parseOrgAndProject parses org and project from a tenant-scoped path of the
// form `/v1/orgs/{org}/projects/{project}/...`. ok is false when the path is
// not tenant-scoped, or when either segment is empty or contains characters
// that could be interpreted differently by the router (e.g. percent-encoded
// slashes).
func parseOrgAndProject(escapedPath string) (org, project string, ok bool) {
	parts := strings.Split(escapedPath, "/")
	// parts[0] is "" (before first slash), parts[1] "v1", parts[2] "orgs",
	// parts[3] the {org}, parts[4] "projects", parts[5] the {project}.
	if len(parts) < 6 || parts[0] != "" || parts[1] != "v1" || parts[2] != "orgs" || parts[4] != "projects" {
		return "", "", false
	}
	org, project = parts[3], parts[5]
	if org == "" || project == "" || strings.Contains(org, "%") || strings.Contains(project, "%") {
		return "", "", false
	}
	return org, project, true
}

// Middleware returns an HTTP middleware that performs API-key authentication.
//
// The policy is deny-by-default:
//   - /healthz (exact path, any method) is the only unauthenticated route.
//   - Every other path requires `Authorization: Bearer <token>` with a known
//     key; otherwise the response is 401.
//   - After authentication, the request path must be tenant-scoped, i.e. of the
//     form /v1/orgs/{org}/projects/{project}/..., and {org}/{project} must equal
//     the key's scope. A path that is not tenant-scoped (no org or project, or
//     any non-/v1 path) is rejected with 403, as is a mismatching org or
//     project. Finally, a tenant-scoped path that is not canonical (e.g.
//     containing "..", "//" or a trailing slash) is rejected with 400.
//   - On success, the context carries org_id and project_id.
func (s *KeyStore) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Leave /healthz open (does not need authentication)
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Unauthorized: missing Authorization header", http.StatusUnauthorized)
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			http.Error(w, "Unauthorized: invalid Authorization header format", http.StatusUnauthorized)
			return
		}

		token := strings.TrimSpace(parts[1])
		scope, exists := s.Lookup(token)
		if !exists {
			http.Error(w, "Unauthorized: invalid API key", http.StatusUnauthorized)
			return
		}

		escaped := r.URL.EscapedPath()

		// Deny by default: the path must carry an org and project, and they must
		// match the authenticated scope.
		urlOrg, urlProject, ok := parseOrgAndProject(escaped)
		if !ok {
			http.Error(w, "Forbidden: route is not tenant-scoped", http.StatusForbidden)
			return
		}
		if urlOrg != scope.OrgID {
			http.Error(w, "Forbidden: tenant organization mismatch", http.StatusForbidden)
			return
		}
		if urlProject != scope.ProjectID {
			http.Error(w, "Forbidden: tenant project mismatch", http.StatusForbidden)
			return
		}

		// Reject paths the router would clean or redirect, so the segments checked
		// here are exactly the ones the handlers will see.
		if path.Clean(escaped) != escaped {
			http.Error(w, "Bad Request: non-canonical path", http.StatusBadRequest)
			return
		}

		// Add scope to context
		ctx := context.WithValue(r.Context(), ContextKeyOrgID, scope.OrgID)
		ctx = context.WithValue(ctx, ContextKeyProjectID, scope.ProjectID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
