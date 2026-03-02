package blob

import (
	"fmt"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
)

// NewFileStore creates a FileStore backed by the configured blob storage type.
// Default is filesystem unless a bucket is configured, which implies S3.
func NewFileStore(cfg common.BlobConfig, logger *common.Logger) (interfaces.FileStore, error) {
	storeType := cfg.Type

	// Infer type from bucket: if bucket is set and type isn't explicit, use S3
	if storeType == "" && cfg.Bucket != "" {
		storeType = "s3"
	}

	switch storeType {
	case "s3":
		if cfg.Bucket == "" {
			return nil, fmt.Errorf("blob store type 's3' requires a bucket name ([storage.blob] bucket or VIRE_BLOB_BUCKET)")
		}
		return NewS3Store(cfg, logger)
	case "file", "":
		return NewFileSystemStore(cfg.Path, logger)
	default:
		return nil, fmt.Errorf("unknown blob store type: %q (expected 'file' or 's3')", cfg.Type)
	}
}
