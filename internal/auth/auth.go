package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
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

// parseOrgAndProject parses org and project from path `/v1/orgs/{org}/projects/{project}/...`
func parseOrgAndProject(path string) (string, string) {
	if !strings.HasPrefix(path, "/v1/orgs/") {
		return "", ""
	}
	parts := strings.Split(path, "/")
	// parts[0] is "" (before first slash)
	// parts[1] is "v1"
	// parts[2] is "orgs"
	// parts[3] is the {org}
	// parts[4] is "projects"
	// parts[5] is the {project}
	if len(parts) >= 6 && parts[4] == "projects" {
		return parts[3], parts[5]
	}
	return "", ""
}

// Middleware returns an HTTP middleware that performs API-key authentication.
// - Header: Authorization: Bearer <token>
// - On success, context carries org_id and project_id
// - URL org/project (retrieved from path) must match authenticated scope -> else 403
func (s *KeyStore) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Leave /healthz open (does not need authentication)
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}

		// Reject missing/invalid keys on /v1/... routes
		if !strings.HasPrefix(r.URL.Path, "/v1/") {
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

		token := parts[1]
		scope, exists := s.Lookup(token)
		if !exists {
			http.Error(w, "Unauthorized: invalid API key", http.StatusUnauthorized)
			return
		}

		// URL org/project must match authenticated scope -> else 403
		urlOrg, urlProject := parseOrgAndProject(r.URL.Path)
		if urlOrg != "" && urlOrg != scope.OrgID {
			http.Error(w, "Forbidden: tenant organization mismatch", http.StatusForbidden)
			return
		}
		if urlProject != "" && urlProject != scope.ProjectID {
			http.Error(w, "Forbidden: tenant project mismatch", http.StatusForbidden)
			return
		}

		// Add scope to context
		ctx := context.WithValue(r.Context(), ContextKeyOrgID, scope.OrgID)
		ctx = context.WithValue(ctx, ContextKeyProjectID, scope.ProjectID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
