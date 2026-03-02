package blob

import (
	"testing"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFileStore_DefaultFile(t *testing.T) {
	cfg := common.BlobConfig{
		Type: "",
		Path: t.TempDir(),
	}
	store, err := NewFileStore(cfg, testLogger())
	require.NoError(t, err)
	assert.NotNil(t, store)
	_, ok := store.(*FileSystemStore)
	assert.True(t, ok, "expected FileSystemStore for empty type")
}

func TestNewFileStore_ExplicitFile(t *testing.T) {
	cfg := common.BlobConfig{
		Type: "file",
		Path: t.TempDir(),
	}
	store, err := NewFileStore(cfg, testLogger())
	require.NoError(t, err)
	assert.NotNil(t, store)
	_, ok := store.(*FileSystemStore)
	assert.True(t, ok, "expected FileSystemStore for type=file")
}

func TestNewFileStore_S3MissingBucket(t *testing.T) {
	cfg := common.BlobConfig{
		Type:   "s3",
		Bucket: "",
	}
	_, err := NewFileStore(cfg, testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bucket")
}

func TestNewFileStore_BucketInfersS3(t *testing.T) {
	cfg := common.BlobConfig{
		Bucket:    "test-bucket",
		Endpoint:  "http://localhost:9000",
		AccessKey: "minioadmin",
		SecretKey: "minioadmin",
	}
	store, err := NewFileStore(cfg, testLogger())
	require.NoError(t, err)
	_, ok := store.(*S3Store)
	assert.True(t, ok, "expected S3Store when bucket is set without explicit type")
}

func TestNewFileStore_EmptyBucketDefaultsToFile(t *testing.T) {
	cfg := common.BlobConfig{
		Path: t.TempDir(),
	}
	store, err := NewFileStore(cfg, testLogger())
	require.NoError(t, err)
	_, ok := store.(*FileSystemStore)
	assert.True(t, ok, "expected FileSystemStore when no bucket and no type")
}

func TestNewFileStore_UnknownType(t *testing.T) {
	cfg := common.BlobConfig{
		Type: "gcs",
	}
	_, err := NewFileStore(cfg, testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown blob store type")
}
