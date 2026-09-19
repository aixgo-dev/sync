package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// mockAPIError implements smithy.APIError for testing.
type mockAPIError struct {
	code    string
	message string
}

func (e *mockAPIError) Error() string { return e.message }
func (e *mockAPIError) ErrorCode() string { return e.code }
func (e *mockAPIError) ErrorMessage() string { return e.message }
func (e *mockAPIError) ErrorFault() smithy.ErrorFault { return smithy.FaultClient }

type mockObj struct {
	data []byte
	etag string
}

type mockS3Client struct {
	mu      sync.Mutex
	objects map[string]mockObj
	etagSeq int
}

func newMockS3Client() *mockS3Client {
	return &mockS3Client{
		objects: make(map[string]mockObj),
	}
}

func (m *mockS3Client) generateETag() string {
	m.etagSeq++
	return fmt.Sprintf("etag-%d", m.etagSeq)
}

// Implement S3API for mockS3Client
func (m *mockS3Client) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := *params.Key
	obj, ok := m.objects[key]
	if !ok {
		return nil, &types.NoSuchKey{Message: aws.String("NoSuchKey")}
	}

	return &s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader(obj.data)),
		ETag: aws.String(`"` + obj.etag + `"`),
	}, nil
}

func (m *mockS3Client) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := *params.Key
	obj, exists := m.objects[key]

	if params.IfNoneMatch != nil && *params.IfNoneMatch == "*" {
		if exists {
			return nil, &mockAPIError{code: "PreconditionFailed", message: "object already exists"}
		}
	}

	if params.IfMatch != nil {
		expectedETag := strings.Trim(*params.IfMatch, `"`)
		if !exists || obj.etag != expectedETag {
			return nil, &mockAPIError{code: "PreconditionFailed", message: "etag mismatch"}
		}
	}

	data, err := io.ReadAll(params.Body)
	if err != nil {
		return nil, err
	}

	newEtag := m.generateETag()
	m.objects[key] = mockObj{
		data: data,
		etag: newEtag,
	}

	return &s3.PutObjectOutput{
		ETag: aws.String(`"` + newEtag + `"`),
	}, nil
}

func (m *mockS3Client) HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := *params.Key
	obj, ok := m.objects[key]
	if !ok {
		return nil, &types.NotFound{Message: aws.String("NotFound")}
	}

	return &s3.HeadObjectOutput{
		ETag: aws.String(`"` + obj.etag + `"`),
	}, nil
}

func (m *mockS3Client) DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := *params.Key
	obj, exists := m.objects[key]

	if params.IfMatch != nil {
		expectedETag := strings.Trim(*params.IfMatch, `"`)
		if !exists || obj.etag != expectedETag {
			return nil, &mockAPIError{code: "PreconditionFailed", message: "etag mismatch"}
		}
	}

	if !exists {
		// Native S3 returns success on DeleteObject even if key does not exist.
		// However, our implementation checks existence via HeadObject first,
		// so if DeleteObject is reached, we can simulate S3's normal behavior.
		return &s3.DeleteObjectOutput{}, nil
	}

	delete(m.objects, key)
	return &s3.DeleteObjectOutput{}, nil
}

// We need fmt and strings in this file too

func TestS3Store_Unit(t *testing.T) {
	ctx := context.Background()
	mockClient := newMockS3Client()
	store := NewS3Store(mockClient, "test-bucket", "test-prefix/")

	// 1. Get non-existent
	_, _, err := store.Get(ctx, "k1")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// 2. Put create-only success
	meta1, err := store.Put(ctx, "k1", []byte("data1"), "")
	if err != nil {
		t.Fatalf("expected nil error on Put, got %v", err)
	}
	if meta1.ETag == "" {
		t.Fatal("expected ETag to be returned")
	}

	// 3. Put create-only conflict
	_, err = store.Put(ctx, "k1", []byte("data1-new"), "")
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}

	// 4. Get success
	data, metaGet, err := store.Get(ctx, "k1")
	if err != nil {
		t.Fatalf("expected Get success, got %v", err)
	}
	if string(data) != "data1" {
		t.Fatalf("expected 'data1', got %s", data)
	}
	if metaGet.ETag != meta1.ETag {
		t.Fatalf("expected ETag %s, got %s", meta1.ETag, metaGet.ETag)
	}

	// 5. Put CAS update conflict due to bad ETag
	_, err = store.Put(ctx, "k1", []byte("data1-bad"), "wrong-etag")
	if !errors.Is(err, ErrPreconditionFailed) {
		t.Fatalf("expected ErrPreconditionFailed, got %v", err)
	}

	// 6. Put CAS update conflict due to non-existent key
	_, err = store.Put(ctx, "k2", []byte("data2"), "any-etag")
	if !errors.Is(err, ErrPreconditionFailed) {
		t.Fatalf("expected ErrPreconditionFailed, got %v", err)
	}

	// 7. Put CAS update success
	meta2, err := store.Put(ctx, "k1", []byte("data1-updated"), meta1.ETag)
	if err != nil {
		t.Fatalf("expected Put CAS success, got %v", err)
	}
	if meta2.ETag == meta1.ETag {
		t.Fatal("expected a new ETag after successful write")
	}

	// 8. Delete CAS conflict due to bad ETag
	err = store.Delete(ctx, "k1", "wrong-etag")
	if !errors.Is(err, ErrPreconditionFailed) {
		t.Fatalf("expected ErrPreconditionFailed, got %v", err)
	}

	// 9. Delete CAS success
	err = store.Delete(ctx, "k1", meta2.ETag)
	if err != nil {
		t.Fatalf("expected Delete CAS success, got %v", err)
	}

	// 10. Get after delete should be ErrNotFound
	_, _, err = store.Get(ctx, "k1")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}

	// 11. Delete non-existent key (unconditional)
	err = store.Delete(ctx, "k1", "")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for non-existent key, got %v", err)
	}
}

func TestS3Store_Integration(t *testing.T) {
	if os.Getenv("SYNC_S3_TEST") != "1" {
		t.Skip("Skipping integration test; SYNC_S3_TEST is not set")
	}

	ctx := context.Background()
	store, err := NewS3StoreFromEnv(ctx)
	if err != nil {
		t.Fatalf("failed to initialize S3 store from env: %v", err)
	}

	key := fmt.Sprintf("test-key-%s", generateTestSuffix())

	// Clean up if left over
	_ = store.Delete(ctx, key, "")

	// 1. Get non-existent
	_, _, err = store.Get(ctx, key)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// 2. Create-only success
	meta1, err := store.Put(ctx, key, []byte("hello-integration"), "")
	if err != nil {
		t.Fatalf("expected Put success, got %v", err)
	}
	if meta1.ETag == "" {
		t.Fatal("expected returned ETag to be non-empty")
	}

	// 3. Create-only conflict
	_, err = store.Put(ctx, key, []byte("conflict"), "")
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}

	// 4. Get success
	data, metaGet, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("failed to get: %v", err)
	}
	if string(data) != "hello-integration" {
		t.Fatalf("expected 'hello-integration', got %s", data)
	}
	if metaGet.ETag != meta1.ETag {
		t.Fatalf("expected ETag %s, got %s", meta1.ETag, metaGet.ETag)
	}

	// 5. Update CAS conflict
	_, err = store.Put(ctx, key, []byte("fail-cas"), "invalid-etag")
	if !errors.Is(err, ErrPreconditionFailed) {
		t.Fatalf("expected ErrPreconditionFailed, got %v", err)
	}

	// 6. Update CAS success
	meta2, err := store.Put(ctx, key, []byte("hello-integration-updated"), meta1.ETag)
	if err != nil {
		t.Fatalf("expected Put success, got %v", err)
	}

	// 7. Delete CAS conflict
	err = store.Delete(ctx, key, "invalid-etag")
	if !errors.Is(err, ErrPreconditionFailed) {
		t.Fatalf("expected ErrPreconditionFailed, got %v", err)
	}

	// 8. Delete CAS success
	err = store.Delete(ctx, key, meta2.ETag)
	if err != nil {
		t.Fatalf("expected Delete success, got %v", err)
	}

	// 9. Verify deleted
	_, _, err = store.Get(ctx, key)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func generateTestSuffix() string {
	// A simple helper to generate unique keys for integration tests to avoid collisions
	importBytes := []byte("0123456789abcdef")
	importRand := make([]byte, 8)
	// Fallback to basic pseudo-random if crypto rand fails, but let's try reading
	importSeq := os.Getpid()
	for i := range importRand {
		importRand[i] = importBytes[(importSeq+i)%len(importBytes)]
	}
	return string(importRand)
}
