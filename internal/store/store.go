package store

import (
	"context"
	"errors"
)

// Meta contains metadata for an object in the store.
type Meta struct {
	ETag string
}

// Sentinel errors returned by the Store operations.
var (
	ErrNotFound           = errors.New("store: key not found")
	ErrPreconditionFailed = errors.New("store: precondition failed")
	ErrAlreadyExists      = errors.New("store: key already exists")
)

// Store defines the interface for a CAS-enabled key-value store.
type Store interface {
	// Get retrieves the data and metadata for the given key.
	// Returns ErrNotFound if the key does not exist.
	Get(ctx context.Context, key string) (data []byte, meta Meta, err error)

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
	Put(ctx context.Context, key string, data []byte, ifMatch string) (meta Meta, err error)

	// Delete removes the key from the store.
	//
	// If ifMatch is empty (""), the operation is unconditional. If the key does not
	// exist, Delete returns ErrNotFound.
	//
	// If ifMatch is a non-empty string, the key must exist and its current ETag
	// must match ifMatch. If the key does not exist, Delete returns ErrNotFound.
	// If the key exists but the ETag does not match, Delete returns ErrPreconditionFailed.
	Delete(ctx context.Context, key string, ifMatch string) error

	// List retrieves all keys starting with the given prefix.
	// The returned keys are relative to the store (i.e. without the S3 prefix, if configured).
	List(ctx context.Context, prefix string) (keys []string, err error)
}
