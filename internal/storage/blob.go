// Package storage provides blob-based persistence with pluggable backends.
package storage

import (
	"context"
	"errors"
	"io"
	"time"
)

// Common errors for blob storage operations.
var (
	ErrBlobNotFound = errors.New("blob not found")
	ErrBlobExists   = errors.New("blob already exists")
)

// BlobMetadata contains metadata about a stored blob.
type BlobMetadata struct {
	Key          string    `json:"key"`
	Size         int64     `json:"size"`
	ContentType  string    `json:"content_type,omitempty"`
	LastModified time.Time `json:"last_modified"`
	ETag         string    `json:"etag,omitempty"` // For conditional operations
}

// ListOptions configures blob listing behavior.
type ListOptions struct {
	Prefix    string // Only return keys with this prefix
	Delimiter string // Group keys by delimiter (e.g., "/" for directories)
	MaxKeys   int    // Maximum number of keys to return (0 = no limit)
	Cursor    string // Pagination cursor from previous ListResult
}

// ListResult contains the results of a list operation.
type ListResult struct {
	Blobs      []BlobMetadata `json:"blobs"`
	NextCursor string         `json:"next_cursor,omitempty"` // Empty if no more results
	Truncated  bool           `json:"truncated"`             // True if more results available
}

// BlobStore defines a provider-agnostic interface for blob storage.
// Implementations: FileBlobStore (local), GCSBlobStore (Google Cloud), S3BlobStore (AWS).
type BlobStore interface {
	// Get retrieves a blob by key. Returns ErrBlobNotFound if not found.
	Get(ctx context.Context, key string) ([]byte, error)

	// GetReader returns a reader for streaming large blobs.
	// Caller must close the reader when done.
	GetReader(ctx context.Context, key string) (io.ReadCloser, error)

	// Put stores a blob. Overwrites if exists.
	Put(ctx context.Context, key string, data []byte) error

	// PutReader stores a blob from a reader for streaming large blobs.
	PutReader(ctx context.Context, key string, r io.Reader, size int64) error

	// Delete removes a blob. No error if not found.
	Delete(ctx context.Context, key string) error

	// Exists checks if a blob exists.
	Exists(ctx context.Context, key string) (bool, error)

	// Metadata returns metadata for a blob. Returns ErrBlobNotFound if not found.
	Metadata(ctx context.Context, key string) (*BlobMetadata, error)

	// List returns blobs matching the given options.
	List(ctx context.Context, opts ListOptions) (*ListResult, error)

	// Close releases any resources held by the store.
	Close() error
}

// BlobStoreConfig holds configuration for creating a blob store.
type BlobStoreConfig struct {
	// Backend type: "file", "gcs", "s3"
	Backend string `toml:"backend"`

	// File backend configuration
	File FileBlobConfig `toml:"file"`

	// GCS backend configuration (future)
	GCS GCSBlobConfig `toml:"gcs"`

	// S3 backend configuration (future)
	S3 S3BlobConfig `toml:"s3"`
}

// FileBlobConfig holds file-based blob store configuration.
type FileBlobConfig struct {
	BasePath string `toml:"base_path"`
}

// GCSBlobConfig holds Google Cloud Storage configuration.
type GCSBlobConfig struct {
	Bucket          string `toml:"bucket"`
	Prefix          string `toml:"prefix"`           // Optional key prefix
	CredentialsFile string `toml:"credentials_file"` // Path to service account JSON
}

// S3BlobConfig holds AWS S3 configuration.
type S3BlobConfig struct {
	Bucket    string `toml:"bucket"`
	Prefix    string `toml:"prefix"`   // Optional key prefix
	Region    string `toml:"region"`   // AWS region
	Endpoint  string `toml:"endpoint"` // Custom endpoint for S3-compatible stores (MinIO, R2)
	AccessKey string `toml:"access_key"`
	SecretKey string `toml:"secret_key"`
}
