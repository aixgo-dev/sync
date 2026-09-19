package store_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/aixgo-dev/sync/internal/store"
)

func TestMemoryStore_GetMiss(t *testing.T) {
	t.Parallel()
	s := store.NewMemoryStore()
	ctx := context.Background()

	_, _, err := s.Get(ctx, "non-existent")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestMemoryStore_PutCreate_GetRoundTrip(t *testing.T) {
	t.Parallel()
	s := store.NewMemoryStore()
	ctx := context.Background()
	key := "my-key"
	val := []byte("hello world")

	// 1. Create-only (empty ifMatch) should succeed
	meta1, err := s.Put(ctx, key, val, "")
	if err != nil {
		t.Fatalf("unexpected error on Put: %v", err)
	}
	if meta1.ETag == "" {
		t.Fatal("expected non-empty ETag")
	}

	// 2. Put with empty ifMatch again should fail with ErrAlreadyExists
	_, err = s.Put(ctx, key, []byte("new data"), "")
	if !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}

	// 3. Get should retrieve correct data and etag
	gotVal, meta2, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf("unexpected error on Get: %v", err)
	}
	if !bytes.Equal(gotVal, val) {
		t.Fatalf("expected %s, got %s", val, gotVal)
	}
	if meta2.ETag != meta1.ETag {
		t.Fatalf("expected etag %s, got %s", meta1.ETag, meta2.ETag)
	}
}

func TestMemoryStore_PutCAS(t *testing.T) {
	t.Parallel()
	s := store.NewMemoryStore()
	ctx := context.Background()
	key := "cas-key"
	val1 := []byte("v1")
	val2 := []byte("v2")

	// Create
	meta1, err := s.Put(ctx, key, val1, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// CAS with wrong etag should fail
	_, err = s.Put(ctx, key, val2, "wrong-etag")
	if !errors.Is(err, store.ErrPreconditionFailed) {
		t.Fatalf("expected ErrPreconditionFailed, got %v", err)
	}

	// Verify value remains val1
	gotVal, metaAfterFail, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(gotVal, val1) {
		t.Fatalf("value modified after failed CAS")
	}
	if metaAfterFail.ETag != meta1.ETag {
		t.Fatalf("etag modified after failed CAS")
	}

	// CAS with correct etag should succeed and rotate ETag
	meta2, err := s.Put(ctx, key, val2, meta1.ETag)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta2.ETag == meta1.ETag {
		t.Fatalf("expected new etag to be different from old etag")
	}

	// Verify Get returns val2 and new ETag
	gotVal2, meta3, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(gotVal2, val2) {
		t.Fatalf("expected %s, got %s", val2, gotVal2)
	}
	if meta3.ETag != meta2.ETag {
		t.Fatalf("expected etag %s, got %s", meta2.ETag, meta3.ETag)
	}
}

func TestMemoryStore_Delete(t *testing.T) {
	t.Parallel()
	s := store.NewMemoryStore()
	ctx := context.Background()
	key := "del-key"
	val := []byte("delete-me")

	// Delete non-existent
	err := s.Delete(ctx, key, "")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for non-existent key, got %v", err)
	}

	// Create
	meta, err := s.Put(ctx, key, val, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Delete with wrong etag should fail
	err = s.Delete(ctx, key, "wrong-etag")
	if !errors.Is(err, store.ErrPreconditionFailed) {
		t.Fatalf("expected ErrPreconditionFailed, got %v", err)
	}

	// Delete with correct etag should succeed
	err = s.Delete(ctx, key, meta.ETag)
	if err != nil {
		t.Fatalf("unexpected error on Delete: %v", err)
	}

	// Verify key is gone
	_, _, err = s.Get(ctx, key)
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}

	// Unconditional delete (empty ifMatch) of existing key
	_, err = s.Put(ctx, key, val, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = s.Delete(ctx, key, "")
	if err != nil {
		t.Fatalf("unexpected error on unconditional Delete: %v", err)
	}

	// Verify key is gone
	_, _, err = s.Get(ctx, key)
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after unconditional delete, got %v", err)
	}
}

func TestMemoryStore_ConcurrentPutCAS(t *testing.T) {
	t.Parallel()
	s := store.NewMemoryStore()
	ctx := context.Background()
	key := "concurrent-key"

	// Create initial
	meta, err := s.Put(ctx, key, []byte("init"), "")
	if err != nil {
		t.Fatalf("failed to write initial: %v", err)
	}

	const numWorkers = 50
	var wg sync.WaitGroup
	errs := make([]error, numWorkers)

	wg.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go func(id int) {
			defer wg.Done()
			_, putErr := s.Put(ctx, key, []byte(fmt.Sprintf("worker-%d", id)), meta.ETag)
			errs[id] = putErr
		}(i)
	}
	wg.Wait()

	// Analyze outcomes: exactly one worker must succeed (nil error), and all others must fail with ErrPreconditionFailed
	successCount := 0
	conflictCount := 0
	otherCount := 0

	for _, e := range errs {
		if e == nil {
			successCount++
		} else if errors.Is(e, store.ErrPreconditionFailed) {
			conflictCount++
		} else {
			otherCount++
		}
	}

	if successCount != 1 {
		t.Errorf("expected exactly 1 winner, got %d", successCount)
	}
	if conflictCount != numWorkers-1 {
		t.Errorf("expected %d conflicts, got %d", numWorkers-1, conflictCount)
	}
	if otherCount > 0 {
		t.Errorf("got %d unexpected errors", otherCount)
	}
}

func TestMemoryStore_ContextCancellation(t *testing.T) {
	t.Parallel()
	s := store.NewMemoryStore()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, _, err := s.Get(ctx, "any-key")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	_, err = s.Put(ctx, "any-key", []byte("data"), "")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	err = s.Delete(ctx, "any-key", "")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}
