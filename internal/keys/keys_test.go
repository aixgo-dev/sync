package keys

import (
	"reflect"
	"testing"
)

// TestRoundTrip verifies that each resource kind builds correct keys and
// can be parsed back into the exact same components.
func TestRoundTrip(t *testing.T) {
	tests := []struct {
		name      string
		kind      KeyKind
		build     func() (string, error)
		expected  *KeyInfo
		wantedKey string
	}{
		{
			name: "project metadata",
			kind: KindProjectMeta,
			build: func() (string, error) {
				return ProjectMetaKey("my-org.1", "my-proj_2")
			},
			expected: &KeyInfo{
				Kind:      KindProjectMeta,
				OrgID:     "my-org.1",
				ProjectID: "my-proj_2",
			},
			wantedKey: "tenants/my-org.1/projects/my-proj_2/meta.json",
		},
		{
			name: "session key",
			kind: KindSession,
			build: func() (string, error) {
				return SessionKey("my-org.1", "my-proj_2", "sess-3.4")
			},
			expected: &KeyInfo{
				Kind:      KindSession,
				OrgID:     "my-org.1",
				ProjectID: "my-proj_2",
				SessionID: "sess-3.4",
			},
			wantedKey: "tenants/my-org.1/projects/my-proj_2/sessions/sess-3.4.json",
		},
		{
			name: "thread metadata",
			kind: KindThreadMeta,
			build: func() (string, error) {
				return ThreadMetaKey("my-org.1", "my-proj_2", "thread_5")
			},
			expected: &KeyInfo{
				Kind:      KindThreadMeta,
				OrgID:     "my-org.1",
				ProjectID: "my-proj_2",
				ThreadID:  "thread_5",
			},
			wantedKey: "tenants/my-org.1/projects/my-proj_2/threads/thread_5/meta.json",
		},
		{
			name: "message key",
			kind: KindMessage,
			build: func() (string, error) {
				return MessageKey("my-org.1", "my-proj_2", "thread_5", "msg-6")
			},
			expected: &KeyInfo{
				Kind:      KindMessage,
				OrgID:     "my-org.1",
				ProjectID: "my-proj_2",
				ThreadID:  "thread_5",
				MessageID: "msg-6",
			},
			wantedKey: "tenants/my-org.1/projects/my-proj_2/threads/thread_5/messages/msg-6.json",
		},
		{
			name: "job key",
			kind: KindJob,
			build: func() (string, error) {
				return JobKey("my-org.1", "my-proj_2", "job-abc")
			},
			expected: &KeyInfo{
				Kind:      KindJob,
				OrgID:     "my-org.1",
				ProjectID: "my-proj_2",
				JobID:     "job-abc",
			},
			wantedKey: "tenants/my-org.1/projects/my-proj_2/jobs/job-abc.json",
		},
		{
			name: "desired state document key",
			kind: KindDesired,
			build: func() (string, error) {
				return DesiredKey("my-org.1", "my-proj_2", "desired-doc")
			},
			expected: &KeyInfo{
				Kind:      KindDesired,
				OrgID:     "my-org.1",
				ProjectID: "my-proj_2",
				Name:      "desired-doc",
			},
			wantedKey: "tenants/my-org.1/projects/my-proj_2/desired/desired-doc.json",
		},
		{
			name: "device key",
			kind: KindDevice,
			build: func() (string, error) {
				return DeviceKey("my-org.1", "my-proj_2", "device-99")
			},
			expected: &KeyInfo{
				Kind:      KindDevice,
				OrgID:     "my-org.1",
				ProjectID: "my-proj_2",
				DeviceID:  "device-99",
			},
			wantedKey: "tenants/my-org.1/projects/my-proj_2/devices/device-99.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build the key
			key, err := tt.build()
			if err != nil {
				t.Fatalf("failed to build key: %v", err)
			}

			// Validate generated string structure
			if key != tt.wantedKey {
				t.Errorf("expected built key %q, got %q", tt.wantedKey, key)
			}

			// Parse back
			info, err := Parse(key)
			if err != nil {
				t.Fatalf("failed to parse key %q: %v", key, err)
			}

			// Deep equal verification
			if !reflect.DeepEqual(info, tt.expected) {
				t.Errorf("mismatch on parse:\nexpected: %+v\ngot:      %+v", tt.expected, info)
			}
		})
	}
}

// TestBuilderValidation checks that builders properly reject invalid arguments.
func TestBuilderValidation(t *testing.T) {
	invalidIDs := []string{
		"",                 // empty
		".",                // dot
		"..",               // double dot
		"../foo",           // path escape
		"foo/bar",          // slash
		"foo\\bar",         // backslash
		"foo%2fbar",        // URL-encoded slash
		"org!",             // invalid character
		"org name",         // spaces
		"org-with-url-%2f", // URL-encoded slash variation
	}

	for _, invalidID := range invalidIDs {
		t.Run("invalid_id_"+invalidID, func(t *testing.T) {
			if _, err := ProjectMetaKey(invalidID, "proj"); err == nil {
				t.Errorf("expected error for ProjectMetaKey with invalid orgID: %q", invalidID)
			}
			if _, err := ProjectMetaKey("org", invalidID); err == nil {
				t.Errorf("expected error for ProjectMetaKey with invalid projectID: %q", invalidID)
			}

			if _, err := SessionKey(invalidID, "proj", "sess"); err == nil {
				t.Errorf("expected error for SessionKey with invalid orgID: %q", invalidID)
			}
			if _, err := SessionKey("org", invalidID, "sess"); err == nil {
				t.Errorf("expected error for SessionKey with invalid projectID: %q", invalidID)
			}
			if _, err := SessionKey("org", "proj", invalidID); err == nil {
				t.Errorf("expected error for SessionKey with invalid sessionID: %q", invalidID)
			}

			if _, err := ThreadMetaKey("org", "proj", invalidID); err == nil {
				t.Errorf("expected error for ThreadMetaKey with invalid threadID: %q", invalidID)
			}

			if _, err := MessageKey("org", "proj", "thread", invalidID); err == nil {
				t.Errorf("expected error for MessageKey with invalid msgID: %q", invalidID)
			}

			if _, err := JobKey("org", "proj", invalidID); err == nil {
				t.Errorf("expected error for JobKey with invalid jobID: %q", invalidID)
			}

			if _, err := DesiredKey("org", "proj", invalidID); err == nil {
				t.Errorf("expected error for DesiredKey with invalid name: %q", invalidID)
			}

			if _, err := DeviceKey("org", "proj", invalidID); err == nil {
				t.Errorf("expected error for DeviceKey with invalid deviceID: %q", invalidID)
			}
		})
	}
}

// TestParserSafetyAndValidation checks that the Parse function rejects all forms
// of malformed, unsafe, truncated, or directory-escaping keys.
func TestParserSafetyAndValidation(t *testing.T) {
	invalidKeys := []struct {
		name string
		key  string
	}{
		{
			name: "empty key",
			key:  "",
		},
		{
			name: "leading slash",
			key:  "/tenants/org/projects/proj/meta.json",
		},
		{
			name: "trailing slash",
			key:  "tenants/org/projects/proj/meta.json/",
		},
		{
			name: "double slash",
			key:  "tenants//org/projects/proj/meta.json",
		},
		{
			name: "triple slash",
			key:  "tenants/org/projects/proj/sessions///sess.json",
		},
		{
			name: "directory escape double-dot in org",
			key:  "tenants/../projects/proj/meta.json",
		},
		{
			name: "directory escape double-dot in project",
			key:  "tenants/org/projects/../meta.json",
		},
		{
			name: "directory escape trailing double-dot",
			key:  "tenants/org/projects/proj/..",
		},
		{
			name: "directory escape mid double-dot",
			key:  "tenants/org/projects/proj/../../etc/passwd",
		},
		{
			name: "single dot in ID",
			key:  "tenants/./projects/proj/meta.json",
		},
		{
			name: "wrong top-level prefix",
			key:  "users/org/projects/proj/meta.json",
		},
		{
			name: "wrong intermediate prefix",
			key:  "tenants/org/workspaces/proj/meta.json",
		},
		{
			name: "truncated key - 1 segment",
			key:  "tenants",
		},
		{
			name: "truncated key - 2 segments",
			key:  "tenants/org",
		},
		{
			name: "truncated key - 3 segments",
			key:  "tenants/org/projects",
		},
		{
			name: "truncated key - 4 segments",
			key:  "tenants/org/projects/proj",
		},
		{
			name: "wrong leaf file name (project meta)",
			key:  "tenants/org/projects/proj/meta.yaml",
		},
		{
			name: "wrong leaf extension (session)",
			key:  "tenants/org/projects/proj/sessions/sess.yaml",
		},
		{
			name: "empty session ID",
			key:  "tenants/org/projects/proj/sessions/.json",
		},
		{
			name: "empty msg ID",
			key:  "tenants/org/projects/proj/threads/thread-1/messages/.json",
		},
		{
			name: "wrong category for 6-segment key",
			key:  "tenants/org/projects/proj/unknown/id.json",
		},
		{
			name: "wrong middle segment for 7-segment key",
			key:  "tenants/org/projects/proj/channels/thread-1/meta.json",
		},
		{
			name: "wrong end segment for 7-segment key",
			key:  "tenants/org/projects/proj/threads/thread-1/info.json",
		},
		{
			name: "wrong category segment for 8-segment key",
			key:  "tenants/org/projects/proj/threads/thread-1/posts/msg.json",
		},
		{
			name: "excessive segments",
			key:  "tenants/org/projects/proj/threads/thread-1/messages/msg.json/extra-segment",
		},
		{
			name: "URL-encoded slash in key (uppercase)",
			key:  "tenants/org%2Fid/projects/proj/meta.json",
		},
		{
			name: "URL-encoded slash in key (lowercase)",
			key:  "tenants/org%2fid/projects/proj/meta.json",
		},
		{
			name: "URL-encoded slash in leaf",
			key:  "tenants/org/projects/proj/sessions/sess%2fid.json",
		},
		{
			name: "invalid characters in ID (at-sign)",
			key:  "tenants/org@name/projects/proj/meta.json",
		},
		{
			name: "invalid characters in ID (backslash)",
			key:  "tenants/org\\name/projects/proj/meta.json",
		},
	}

	for _, tt := range invalidKeys {
		t.Run(tt.name, func(t *testing.T) {
			info, err := Parse(tt.key)
			if err == nil {
				t.Errorf("expected parsing to fail for invalid key %q, but it succeeded with: %+v", tt.key, info)
			}
		})
	}
}
