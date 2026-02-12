// Package storage provides blob-based persistence with pluggable backends.
package storage

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bobmcallan/vire/internal/common"
)

// FileBlobStore implements BlobStore using the local filesystem.
// Keys are mapped to file paths under the base directory.
// Key format: "portfolios/SMSF.json" -> "{basePath}/portfolios/SMSF.json"
type FileBlobStore struct {
	basePath string
	logger   *common.Logger
}

// NewFileBlobStore creates a new file-based blob store.
func NewFileBlobStore(logger *common.Logger, config *FileBlobConfig) (*FileBlobStore, error) {
	if config.BasePath == "" {
		return nil, fmt.Errorf("file blob store base_path is required")
	}

	// Ensure base directory exists
	if err := os.MkdirAll(config.BasePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base directory %s: %w", config.BasePath, err)
	}

	fb := &FileBlobStore{
		basePath: config.BasePath,
		logger:   logger,
	}

	logger.Debug().Str("path", config.BasePath).Msg("FileBlobStore initialized")
	return fb, nil
}

// sanitizeKey converts a key to a safe filesystem path.
// Prevents path traversal attacks while allowing "/" for subdirectories.
func (fb *FileBlobStore) sanitizeKey(key string) string {
	// Clean the path to remove ".." and other traversal attempts
	clean := filepath.Clean(key)
	// Remove leading slashes to prevent absolute paths
	clean = strings.TrimPrefix(clean, "/")
	// Reject any remaining ".." segments
	if strings.Contains(clean, "..") {
		// Replace with safe alternative
		clean = strings.ReplaceAll(clean, "..", "__")
	}
	return clean
}

// keyToPath converts a key to an absolute filesystem path.
func (fb *FileBlobStore) keyToPath(key string) string {
	return filepath.Join(fb.basePath, fb.sanitizeKey(key))
}

// Get retrieves a blob by key.
func (fb *FileBlobStore) Get(ctx context.Context, key string) ([]byte, error) {
	path := fb.keyToPath(key)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrBlobNotFound
		}
		return nil, fmt.Errorf("failed to read blob %s: %w", key, err)
	}
	return data, nil
}

// GetReader returns a reader for streaming large blobs.
func (fb *FileBlobStore) GetReader(ctx context.Context, key string) (io.ReadCloser, error) {
	path := fb.keyToPath(key)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrBlobNotFound
		}
		return nil, fmt.Errorf("failed to open blob %s: %w", key, err)
	}
	return f, nil
}

// Put stores a blob atomically using temp file + rename.
func (fb *FileBlobStore) Put(ctx context.Context, key string, data []byte) error {
	return fb.PutReader(ctx, key, bytes.NewReader(data), int64(len(data)))
}

// PutReader stores a blob from a reader atomically.
func (fb *FileBlobStore) PutReader(ctx context.Context, key string, r io.Reader, size int64) error {
	path := fb.keyToPath(key)
	dir := filepath.Dir(path)

	// Ensure parent directory exists
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Atomic write: temp file + rename
	tmpFile, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Copy data to temp file
	if _, err := io.Copy(tmpFile, r); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// Delete removes a blob. No error if not found.
func (fb *FileBlobStore) Delete(ctx context.Context, key string) error {
	path := fb.keyToPath(key)
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete blob %s: %w", key, err)
	}
	return nil
}

// Exists checks if a blob exists.
func (fb *FileBlobStore) Exists(ctx context.Context, key string) (bool, error) {
	path := fb.keyToPath(key)
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("failed to check blob %s: %w", key, err)
}

// Metadata returns metadata for a blob.
func (fb *FileBlobStore) Metadata(ctx context.Context, key string) (*BlobMetadata, error) {
	path := fb.keyToPath(key)
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrBlobNotFound
		}
		return nil, fmt.Errorf("failed to stat blob %s: %w", key, err)
	}

	// Compute ETag from file content (MD5 hash)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read blob for etag %s: %w", key, err)
	}
	hash := md5.Sum(data)
	etag := hex.EncodeToString(hash[:])

	return &BlobMetadata{
		Key:          key,
		Size:         info.Size(),
		LastModified: info.ModTime(),
		ETag:         etag,
	}, nil
}

// List returns blobs matching the given options.
func (fb *FileBlobStore) List(ctx context.Context, opts ListOptions) (*ListResult, error) {
	searchDir := fb.basePath
	prefix := opts.Prefix

	// If prefix contains directory components, start search from that directory
	if prefix != "" {
		prefixDir := filepath.Dir(prefix)
		if prefixDir != "." {
			searchDir = filepath.Join(fb.basePath, prefixDir)
		}
	}

	var blobs []BlobMetadata
	maxKeys := opts.MaxKeys
	if maxKeys <= 0 {
		maxKeys = 1000 // Default limit
	}

	err := filepath.Walk(searchDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip inaccessible paths
		}
		if info.IsDir() {
			return nil // Skip directories
		}
		if strings.HasPrefix(info.Name(), ".tmp-") {
			return nil // Skip temp files
		}

		// Convert path back to key
		relPath, err := filepath.Rel(fb.basePath, path)
		if err != nil {
			return nil
		}
		key := filepath.ToSlash(relPath) // Normalize to forward slashes

		// Apply prefix filter
		if prefix != "" && !strings.HasPrefix(key, prefix) {
			return nil
		}

		// Check limit
		if len(blobs) >= maxKeys {
			return filepath.SkipAll
		}

		blobs = append(blobs, BlobMetadata{
			Key:          key,
			Size:         info.Size(),
			LastModified: info.ModTime(),
		})
		return nil
	})

	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to list blobs: %w", err)
	}

	return &ListResult{
		Blobs:     blobs,
		Truncated: len(blobs) >= maxKeys,
	}, nil
}

// Close releases resources (no-op for file storage).
func (fb *FileBlobStore) Close() error {
	return nil
}
