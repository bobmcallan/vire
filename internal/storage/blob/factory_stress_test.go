package blob

import (
	"testing"

	"github.com/bobmcallan/vire/internal/common"
)

// Devils-advocate stress tests for the blob store factory.
// Verifies edge cases in backend selection and config validation.

// ============================================================================
// FS-1. Unknown type returns descriptive error
// ============================================================================

func TestFactoryStress_UnknownType(t *testing.T) {
	logger := common.NewSilentLogger()

	badTypes := []string{"gcs", "azure", "minio", "  s3  ", "S3", "FILE", "s3\x00", "../file"}
	for _, typ := range badTypes {
		t.Run(typ, func(t *testing.T) {
			cfg := common.BlobConfig{Type: typ}
			_, err := NewFileStore(cfg, logger)
			if err == nil {
				t.Errorf("expected error for type %q, got nil", typ)
			}
		})
	}
}

// ============================================================================
// FS-2. S3 type without bucket returns error
// ============================================================================

func TestFactoryStress_S3NoBucket(t *testing.T) {
	logger := common.NewSilentLogger()
	cfg := common.BlobConfig{
		Type:   "s3",
		Bucket: "",
		Region: "us-east-1",
	}
	_, err := NewFileStore(cfg, logger)
	if err == nil {
		t.Error("expected error for S3 type without bucket")
	}
}

// ============================================================================
// FS-3. Empty type defaults to filesystem
// ============================================================================

func TestFactoryStress_EmptyTypeDefaultsFile(t *testing.T) {
	logger := common.NewSilentLogger()
	cfg := common.BlobConfig{
		Type: "",
		Path: t.TempDir(),
	}
	store, err := NewFileStore(cfg, logger)
	if err != nil {
		t.Fatalf("empty type should default to file: %v", err)
	}
	if _, ok := store.(*FileSystemStore); !ok {
		t.Errorf("expected *FileSystemStore, got %T", store)
	}
}

// ============================================================================
// FS-4. File type with empty path uses default
// ============================================================================

func TestFactoryStress_FileEmptyPath(t *testing.T) {
	logger := common.NewSilentLogger()
	cfg := common.BlobConfig{
		Type: "file",
		Path: "",
	}
	store, err := NewFileStore(cfg, logger)
	if err != nil {
		// Acceptable if default dir can't be created in test env
		return
	}
	fs, ok := store.(*FileSystemStore)
	if !ok {
		t.Fatalf("expected *FileSystemStore, got %T", store)
	}
	if fs.basePath == "" {
		t.Error("basePath should be set to default, not empty")
	}
}

// ============================================================================
// FS-5. File type with non-writable path
// ============================================================================

func TestFactoryStress_FileNonWritablePath(t *testing.T) {
	logger := common.NewSilentLogger()
	cfg := common.BlobConfig{
		Type: "file",
		Path: "/proc/non_writable_dir_for_test",
	}
	_, err := NewFileStore(cfg, logger)
	if err == nil {
		t.Error("expected error for non-writable path")
	}
}
