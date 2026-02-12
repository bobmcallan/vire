// Package storage provides blob-based persistence with pluggable backends.
package storage

import (
	"fmt"

	"github.com/bobmcallan/vire/internal/common"
)

// Backend type constants.
const (
	BackendFile = "file"
	BackendGCS  = "gcs"
	BackendS3   = "s3"
)

// NewBlobStore creates a blob store based on the configuration.
// Supported backends: "file" (default), "gcs", "s3".
func NewBlobStore(logger *common.Logger, config *BlobStoreConfig) (BlobStore, error) {
	backend := config.Backend
	if backend == "" {
		backend = BackendFile // Default to file backend
	}

	switch backend {
	case BackendFile:
		return NewFileBlobStore(logger, &config.File)

	case BackendGCS:
		return nil, fmt.Errorf("GCS blob store not yet implemented (coming in Phase 2)")

	case BackendS3:
		return nil, fmt.Errorf("S3 blob store not yet implemented (coming in Phase 2)")

	default:
		return nil, fmt.Errorf("unknown storage backend: %s (supported: file, gcs, s3)", backend)
	}
}
