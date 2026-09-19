package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// S3API defines the subset of S3 client operations used by S3Store.
type S3API interface {
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
}

// S3Store implements the Store interface using an S3-compatible backend.
type S3Store struct {
	client S3API
	bucket string
	prefix string
}

// Ensure S3Store implements Store interface at compile time.
var _ Store = (*S3Store)(nil)

// NewS3Store creates a new S3Store with an existing S3 client.
func NewS3Store(client S3API, bucket string, prefix string) *S3Store {
	return &S3Store{
		client: client,
		bucket: bucket,
		prefix: prefix,
	}
}

// NewS3StoreFromEnv initializes and configures an S3Store based on environment variables.
func NewS3StoreFromEnv(ctx context.Context) (*S3Store, error) {
	endpoint := os.Getenv("SYNC_S3_ENDPOINT")
	region := os.Getenv("SYNC_S3_REGION")
	bucket := os.Getenv("SYNC_S3_BUCKET")
	accessKeyID := os.Getenv("SYNC_S3_ACCESS_KEY_ID")
	secretAccessKey := os.Getenv("SYNC_S3_SECRET_ACCESS_KEY")
	prefix := os.Getenv("SYNC_S3_PREFIX")

	if bucket == "" {
		return nil, fmt.Errorf("SYNC_S3_BUCKET is required")
	}

	var opts []func(*config.LoadOptions) error
	if region != "" {
		opts = append(opts, config.WithRegion(region))
	} else {
		opts = append(opts, config.WithRegion("us-east-1"))
	}

	if accessKeyID != "" && secretAccessKey != "" {
		opts = append(opts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, ""),
		))
	}

	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load S3 SDK config: %w", err)
	}

	usePathStyle := true
	if val, ok := os.LookupEnv("SYNC_S3_USE_PATH_STYLE"); ok {
		if val == "false" || val == "0" {
			usePathStyle = false
		}
	}

	s3Client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
		}
		o.UsePathStyle = usePathStyle
	})

	return &S3Store{
		client: s3Client,
		bucket: bucket,
		prefix: prefix,
	}, nil
}

// resolveKey joins the prefix and the key.
func (s *S3Store) resolveKey(key string) string {
	if s.prefix == "" {
		return key
	}
	return s.prefix + key
}

// Get retrieves the data and metadata for the given key.
// Returns ErrNotFound if the key does not exist.
func (s *S3Store) Get(ctx context.Context, key string) ([]byte, Meta, error) {
	if err := ctx.Err(); err != nil {
		return nil, Meta{}, err
	}

	skey := s.resolveKey(key)
	resp, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(skey),
	})
	if err != nil {
		if isNotFound(err) {
			return nil, Meta{}, ErrNotFound
		}
		return nil, Meta{}, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, Meta{}, err
	}

	meta := Meta{}
	if resp.ETag != nil {
		meta.ETag = sanitizeETag(*resp.ETag)
	}

	return data, meta, nil
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
func (s *S3Store) Put(ctx context.Context, key string, data []byte, ifMatch string) (Meta, error) {
	if err := ctx.Err(); err != nil {
		return Meta{}, err
	}

	skey := s.resolveKey(key)
	input := &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(skey),
		Body:   bytes.NewReader(data),
	}

	if ifMatch == "" {
		// Create-only: prevent overwriting existing keys
		input.IfNoneMatch = aws.String("*")
	} else {
		// CAS update: only update if the ETag matches ifMatch
		input.IfMatch = aws.String(formatETag(ifMatch))
	}

	resp, err := s.client.PutObject(ctx, input)
	if err != nil {
		if ifMatch == "" {
			if isPreconditionFailed(err) {
				return Meta{}, ErrAlreadyExists
			}
		} else {
			if isPreconditionFailed(err) || isNotFound(err) {
				return Meta{}, ErrPreconditionFailed
			}
		}
		return Meta{}, err
	}

	meta := Meta{}
	if resp.ETag != nil {
		meta.ETag = sanitizeETag(*resp.ETag)
	}

	return meta, nil
}

// Delete removes the key from the store.
//
// If ifMatch is empty (""), the operation is unconditional. If the key does not
// exist, Delete returns ErrNotFound.
//
// If ifMatch is a non-empty string, the key must exist and its current ETag
// must match ifMatch. If the key does not exist, Delete returns ErrNotFound.
// If the key exists but the ETag does not match, Delete returns ErrPreconditionFailed.
func (s *S3Store) Delete(ctx context.Context, key string, ifMatch string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	skey := s.resolveKey(key)

	// Since S3 DeleteObject is traditionally idempotent (returns 204 even if key doesn't exist),
	// and the Store interface requires returning ErrNotFound if key does not exist,
	// we first verify key existence and check the ETag using HeadObject.
	headResp, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(skey),
	})
	if err != nil {
		if isNotFound(err) {
			return ErrNotFound
		}
		return err
	}

	if ifMatch != "" {
		var currentETag string
		if headResp.ETag != nil {
			currentETag = sanitizeETag(*headResp.ETag)
		}
		if currentETag != sanitizeETag(ifMatch) {
			return ErrPreconditionFailed
		}
	}

	input := &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(skey),
	}
	if ifMatch != "" {
		input.IfMatch = aws.String(formatETag(ifMatch))
	}

	_, err = s.client.DeleteObject(ctx, input)
	if err != nil {
		if isNotFound(err) {
			return ErrNotFound
		}
		if isPreconditionFailed(err) {
			return ErrPreconditionFailed
		}
		return err
	}

	return nil
}

// sanitizeETag strips the surrounding double quotes from an ETag.
func sanitizeETag(etag string) string {
	return strings.Trim(etag, `"`)
}

// formatETag wraps an ETag in double quotes if it is not already wrapped.
func formatETag(etag string) string {
	if etag == "" {
		return ""
	}
	if !strings.HasPrefix(etag, `"`) {
		etag = `"` + etag + `"`
	}
	return etag
}

// isNotFound returns true if the error indicates that the key was not found.
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	var nsk *types.NoSuchKey
	var nf *types.NotFound
	var apiErr smithy.APIError
	if errors.As(err, &nsk) || errors.As(err, &nf) {
		return true
	}
	if errors.As(err, &apiErr) {
		code := apiErr.ErrorCode()
		return code == "NoSuchKey" || code == "NotFound" || code == "404"
	}
	return false
}

// isPreconditionFailed returns true if the error indicates a precondition failure.
func isPreconditionFailed(err error) bool {
	if err == nil {
		return false
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code := apiErr.ErrorCode()
		return code == "PreconditionFailed" || code == "412"
	}
	return false
}
