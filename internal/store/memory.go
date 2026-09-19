package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
)

type record struct {
	data []byte
	etag string
}

// MemoryStore implements the Store interface in-memory.
// It is fully concurrency-safe and isolates stored data from mutation via copy-on-read and copy-on-write.
type MemoryStore struct {
	mu      sync.RWMutex
	records map[string]record
}

// NewMemoryStore creates a new in-memory implementation of Store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		records: make(map[string]record),
	}
}

// Ensure MemoryStore implements Store interface at compile time.
var _ Store = (*MemoryStore)(nil)

// Get retrieves the data and metadata for the given key.
// Returns ErrNotFound if the key does not exist.
func (s *MemoryStore) Get(ctx context.Context, key string) ([]byte, Meta, error) {
	if err := ctx.Err(); err != nil {
		return nil, Meta{}, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	rec, ok := s.records[key]
	if !ok {
		return nil, Meta{}, ErrNotFound
	}

	// copy-on-read to prevent external mutations from affecting stored data.
	dataCopy := make([]byte, len(rec.data))
	copy(dataCopy, rec.data)

	return dataCopy, Meta{ETag: rec.etag}, nil
}

// Put writes data to the given key.
//
// If ifMatch is empty (""), the operation is "create-only". If the key already
// exists, Put returns ErrAlreadyExists.
//
// If ifMatch is a non-empty string, the operation is a conditional update (CAS).
// The key must exist and its current ETag must match ifMatch. If the key does
// not exist or the ETag does not match, Put returns ErrPreconditionFailed.
//
// On success, Put returns the new metadata with a newly generated ETag.
func (s *MemoryStore) Put(ctx context.Context, key string, data []byte, ifMatch string) (Meta, error) {
	if err := ctx.Err(); err != nil {
		return Meta{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok := s.records[key]

	if ifMatch == "" {
		if ok {
			return Meta{}, ErrAlreadyExists
		}
	} else {
		if !ok {
			return Meta{}, ErrPreconditionFailed
		}
		if rec.etag != ifMatch {
			return Meta{}, ErrPreconditionFailed
		}
	}

	// copy-on-write to prevent external mutations from affecting stored data.
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)

	newETag, err := generateETag()
	if err != nil {
		return Meta{}, err
	}

	s.records[key] = record{
		data: dataCopy,
		etag: newETag,
	}

	return Meta{ETag: newETag}, nil
}

// Delete removes the key from the store.
//
// If ifMatch is empty (""), the operation is unconditional. If the key does not
// exist, Delete returns ErrNotFound.
//
// If ifMatch is a non-empty string, the key must exist and its current ETag
// must match ifMatch. If the key does not exist, Delete returns ErrNotFound.
// If the key exists but the ETag does not match, Delete returns ErrPreconditionFailed.
func (s *MemoryStore) Delete(ctx context.Context, key string, ifMatch string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok := s.records[key]
	if !ok {
		return ErrNotFound
	}

	if ifMatch != "" {
		if rec.etag != ifMatch {
			return ErrPreconditionFailed
		}
	}

	delete(s.records, key)
	return nil
}

func generateETag() (string, error) {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
