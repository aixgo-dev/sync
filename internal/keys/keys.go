// Package keys provides pure function helpers to build and parse object keys
// for Sync entities, enforcing safety rules and tenant separation.
//
// Dynamic ID segments (org_id, project_id, session_id, thread_id, msg_id, job_id, name, device_id)
// must conform to a safe charset: ASCII alphanumeric, dot (.), underscore (_), and hyphen (-).
// That is, they must match the regular expression [A-Za-z0-9._-]+.
// In addition, IDs cannot be empty, ".", or "..", and cannot contain any slashes (including URL-encoded "%2f").
package keys

import (
	"fmt"
	"strings"
)

// KeyKind represents the specific type of resource key.
type KeyKind string

const (
	KindProjectMeta KeyKind = "project_meta"
	KindSession     KeyKind = "session"
	KindThreadMeta  KeyKind = "thread_meta"
	KindMessage     KeyKind = "message"
	KindJob         KeyKind = "job"
	KindDesired     KeyKind = "desired"
	KindDevice      KeyKind = "device"
)

// KeyInfo holds the typed components parsed from a valid object key.
type KeyInfo struct {
	Kind      KeyKind
	OrgID     string
	ProjectID string
	SessionID string
	ThreadID  string
	MessageID string
	JobID     string
	Name      string
	DeviceID  string
}

// isValidID checks if the given string is a valid dynamic ID segment.
// A valid ID must be non-empty, consist only of characters in the safe charset:
// letters (A-Z, a-z), digits (0-9), dot (.), underscore (_), and hyphen (-).
// It must not be "." or "..".
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

// ProjectMetaKey builds the key for project metadata:
// tenants/{org_id}/projects/{project_id}/meta.json
func ProjectMetaKey(orgID, projectID string) (string, error) {
	if !isValidID(orgID) {
		return "", fmt.Errorf("invalid org ID %q", orgID)
	}
	if !isValidID(projectID) {
		return "", fmt.Errorf("invalid project ID %q", projectID)
	}
	return fmt.Sprintf("tenants/%s/projects/%s/meta.json", orgID, projectID), nil
}

// SessionKey builds the key for a session:
// tenants/{org_id}/projects/{project_id}/sessions/{session_id}.json
func SessionKey(orgID, projectID, sessionID string) (string, error) {
	if !isValidID(orgID) {
		return "", fmt.Errorf("invalid org ID %q", orgID)
	}
	if !isValidID(projectID) {
		return "", fmt.Errorf("invalid project ID %q", projectID)
	}
	if !isValidID(sessionID) {
		return "", fmt.Errorf("invalid session ID %q", sessionID)
	}
	return fmt.Sprintf("tenants/%s/projects/%s/sessions/%s.json", orgID, projectID, sessionID), nil
}

// ThreadMetaKey builds the key for thread metadata:
// tenants/{org_id}/projects/{project_id}/threads/{thread_id}/meta.json
func ThreadMetaKey(orgID, projectID, threadID string) (string, error) {
	if !isValidID(orgID) {
		return "", fmt.Errorf("invalid org ID %q", orgID)
	}
	if !isValidID(projectID) {
		return "", fmt.Errorf("invalid project ID %q", projectID)
	}
	if !isValidID(threadID) {
		return "", fmt.Errorf("invalid thread ID %q", threadID)
	}
	return fmt.Sprintf("tenants/%s/projects/%s/threads/%s/meta.json", orgID, projectID, threadID), nil
}

// MessageKey builds the key for a message:
// tenants/{org_id}/projects/{project_id}/threads/{thread_id}/messages/{msg_id}.json
func MessageKey(orgID, projectID, threadID, msgID string) (string, error) {
	if !isValidID(orgID) {
		return "", fmt.Errorf("invalid org ID %q", orgID)
	}
	if !isValidID(projectID) {
		return "", fmt.Errorf("invalid project ID %q", projectID)
	}
	if !isValidID(threadID) {
		return "", fmt.Errorf("invalid thread ID %q", threadID)
	}
	if !isValidID(msgID) {
		return "", fmt.Errorf("invalid message ID %q", msgID)
	}
	return fmt.Sprintf("tenants/%s/projects/%s/threads/%s/messages/%s.json", orgID, projectID, threadID, msgID), nil
}

// JobKey builds the key for a job:
// tenants/{org_id}/projects/{project_id}/jobs/{job_id}.json
func JobKey(orgID, projectID, jobID string) (string, error) {
	if !isValidID(orgID) {
		return "", fmt.Errorf("invalid org ID %q", orgID)
	}
	if !isValidID(projectID) {
		return "", fmt.Errorf("invalid project ID %q", projectID)
	}
	if !isValidID(jobID) {
		return "", fmt.Errorf("invalid job ID %q", jobID)
	}
	return fmt.Sprintf("tenants/%s/projects/%s/jobs/%s.json", orgID, projectID, jobID), nil
}

// DesiredKey builds the key for a desired state document:
// tenants/{org_id}/projects/{project_id}/desired/{name}.json
func DesiredKey(orgID, projectID, name string) (string, error) {
	if !isValidID(orgID) {
		return "", fmt.Errorf("invalid org ID %q", orgID)
	}
	if !isValidID(projectID) {
		return "", fmt.Errorf("invalid project ID %q", projectID)
	}
	if !isValidID(name) {
		return "", fmt.Errorf("invalid desired document name %q", name)
	}
	return fmt.Sprintf("tenants/%s/projects/%s/desired/%s.json", orgID, projectID, name), nil
}

// DeviceKey builds the key for a device:
// tenants/{org_id}/projects/{project_id}/devices/{device_id}.json
func DeviceKey(orgID, projectID, deviceID string) (string, error) {
	if !isValidID(orgID) {
		return "", fmt.Errorf("invalid org ID %q", orgID)
	}
	if !isValidID(projectID) {
		return "", fmt.Errorf("invalid project ID %q", projectID)
	}
	if !isValidID(deviceID) {
		return "", fmt.Errorf("invalid device ID %q", deviceID)
	}
	return fmt.Sprintf("tenants/%s/projects/%s/devices/%s.json", orgID, projectID, deviceID), nil
}

// Parse parses a key into its typed components (KeyInfo), validating all schemas,
// segments, charsets, and safety prefixes to prevent escapes.
func Parse(key string) (*KeyInfo, error) {
	// Reject URL-encoded slash attempts in the key.
	if strings.Contains(strings.ToLower(key), "%2f") {
		return nil, fmt.Errorf("key contains URL-encoded slash %q", "%2f")
	}

	segments := strings.Split(key, "/")

	// Reject key if it contains empty segments (which detects leading, trailing,
	// or double slashes, e.g. "tenants//org", "/tenants/org", or "tenants/org/").
	for _, seg := range segments {
		if seg == "" {
			return nil, fmt.Errorf("empty segment found in key %q", key)
		}
	}

	// All valid keys must have at least 5 segments (minimum: tenants/{org_id}/projects/{project_id}/meta.json).
	// Maximum allowed segments is 8 (maximum: tenants/{org_id}/projects/{project_id}/threads/{thread_id}/messages/{msg_id}.json).
	if len(segments) < 5 || len(segments) > 8 {
		return nil, fmt.Errorf("invalid key segment count %d for key %q", len(segments), key)
	}

	// Validate prefix layout: tenants/{org_id}/projects/{project_id}/
	if segments[0] != "tenants" {
		return nil, fmt.Errorf("invalid top-level prefix segment %q for key %q", segments[0], key)
	}

	orgID := segments[1]
	if !isValidID(orgID) {
		return nil, fmt.Errorf("invalid org ID %q in key %q", orgID, key)
	}

	if segments[2] != "projects" {
		return nil, fmt.Errorf("invalid prefix segment %q for key %q", segments[2], key)
	}

	projectID := segments[3]
	if !isValidID(projectID) {
		return nil, fmt.Errorf("invalid project ID %q in key %q", projectID, key)
	}

	info := &KeyInfo{
		OrgID:     orgID,
		ProjectID: projectID,
	}

	switch len(segments) {
	case 5:
		// Expecting: tenants/{org_id}/projects/{project_id}/meta.json
		if segments[4] != "meta.json" {
			return nil, fmt.Errorf("invalid leaf segment %q for 5-segment key %q", segments[4], key)
		}
		info.Kind = KindProjectMeta
		return info, nil

	case 6:
		// Expecting:
		// - tenants/{org_id}/projects/{project_id}/sessions/{session_id}.json
		// - tenants/{org_id}/projects/{project_id}/jobs/{job_id}.json
		// - tenants/{org_id}/projects/{project_id}/desired/{name}.json
		// - tenants/{org_id}/projects/{project_id}/devices/{device_id}.json
		category := segments[4]
		leaf := segments[5]

		if !strings.HasSuffix(leaf, ".json") {
			return nil, fmt.Errorf("invalid leaf file extension in key %q", key)
		}
		id := strings.TrimSuffix(leaf, ".json")
		if !isValidID(id) {
			return nil, fmt.Errorf("invalid resource ID %q in key %q", id, key)
		}

		switch category {
		case "sessions":
			info.Kind = KindSession
			info.SessionID = id
		case "jobs":
			info.Kind = KindJob
			info.JobID = id
		case "desired":
			info.Kind = KindDesired
			info.Name = id
		case "devices":
			info.Kind = KindDevice
			info.DeviceID = id
		default:
			return nil, fmt.Errorf("invalid category segment %q for 6-segment key %q", category, key)
		}
		return info, nil

	case 7:
		// Expecting: tenants/{org_id}/projects/{project_id}/threads/{thread_id}/meta.json
		if segments[4] != "threads" {
			return nil, fmt.Errorf("expected 'threads' segment, got %q in key %q", segments[4], key)
		}
		threadID := segments[5]
		if !isValidID(threadID) {
			return nil, fmt.Errorf("invalid thread ID %q in key %q", threadID, key)
		}
		if segments[6] != "meta.json" {
			return nil, fmt.Errorf("expected 'meta.json' segment, got %q in key %q", segments[6], key)
		}
		info.Kind = KindThreadMeta
		info.ThreadID = threadID
		return info, nil

	case 8:
		// Expecting: tenants/{org_id}/projects/{project_id}/threads/{thread_id}/messages/{msg_id}.json
		if segments[4] != "threads" {
			return nil, fmt.Errorf("expected 'threads' segment, got %q in key %q", segments[4], key)
		}
		threadID := segments[5]
		if !isValidID(threadID) {
			return nil, fmt.Errorf("invalid thread ID %q in key %q", threadID, key)
		}
		if segments[6] != "messages" {
			return nil, fmt.Errorf("expected 'messages' segment, got %q in key %q", segments[6], key)
		}
		leaf := segments[7]
		if !strings.HasSuffix(leaf, ".json") {
			return nil, fmt.Errorf("invalid leaf file extension in key %q", key)
		}
		msgID := strings.TrimSuffix(leaf, ".json")
		if !isValidID(msgID) {
			return nil, fmt.Errorf("invalid message ID %q in key %q", msgID, key)
		}
		info.Kind = KindMessage
		info.ThreadID = threadID
		info.MessageID = msgID
		return info, nil
	}

	return nil, fmt.Errorf("unhandled key layout for %q", key)
}
