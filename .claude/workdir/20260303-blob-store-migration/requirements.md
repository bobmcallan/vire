# Blob Store Migration — Requirements

**Source Document**: `docs/architecture/26-03-03-blob-store-migration.md`
**Requested**: Move binary file storage (PDFs, charts) from SurrealDB `files` table to pluggable blob storage (filesystem or S3-compatible). Default to filesystem for local dev; S3 for production (Tigris on Fly.io).

## Scope

**In scope:**
- `BlobConfig` struct + TOML config + env overrides + defaults
- Filesystem blob store implementation (`internal/storage/blob/file.go`)
- S3-compatible blob store implementation (`internal/storage/blob/s3.go`)
- Factory function to select backend (`internal/storage/blob/factory.go`)
- Wire blob store into Manager, replacing internal `surrealdb.FileStore`
- Remove `maxFileStoreBytes` size limit from filings (S3 has no 10MB limit; filesystem doesn't either)
- Unit tests for both implementations
- Update TOML example config

**Out of scope:**
- Data migration from existing `files` table (PDFs re-download on next collection cycle)
- Dropping the `files` table (manual post-deploy step)
- GCS backend (future)
- Schema version bump (no derived data change — just storage backend swap)

---

## Files to Change

### 1. `internal/common/config.go` — Add BlobConfig

**What changes:** Add `BlobConfig` struct, nest under `StorageConfig`, set defaults, add env overrides.

Replace the existing unused `S3Config` and `GCSConfig` structs (lines 122-137) with `BlobConfig`:

```go
// BlobConfig holds blob/object storage configuration.
// Default type is "file" for local development; set to "s3" for production.
type BlobConfig struct {
	Type      string `toml:"type"`       // "file" (default) or "s3"
	Path      string `toml:"path"`       // filesystem base path (type=file)
	Bucket    string `toml:"bucket"`     // S3 bucket name (type=s3)
	Prefix    string `toml:"prefix"`     // S3 key prefix (type=s3)
	Region    string `toml:"region"`     // S3 region (type=s3)
	Endpoint  string `toml:"endpoint"`   // custom S3 endpoint for Tigris/MinIO/R2 (type=s3)
	AccessKey string `toml:"access_key"` // S3 access key (type=s3, prefer env vars)
	SecretKey string `toml:"secret_key"` // S3 secret key (type=s3, prefer env vars)
}
```

Add `Blob` field to `StorageConfig` (line 113-120):

```go
type StorageConfig struct {
	Address   string     `toml:"address"`
	Namespace string     `toml:"namespace"`
	Database  string     `toml:"database"`
	Username  string     `toml:"username"`
	Password  string     `toml:"password"`
	DataPath  string     `toml:"data_path"`
	Blob      BlobConfig `toml:"blob"` // NEW
}
```

Add defaults in `NewDefaultConfig()` (inside `Storage:` block, after DataPath, ~line 313):

```go
Blob: BlobConfig{
	Type:   "file",
	Path:   "data/blobs",
	Prefix: "vire",
	Region: "auto",
},
```

Add env overrides in `applyEnvOverrides()` (after the existing storage overrides, ~line 428):

```go
// Blob store overrides
if v := os.Getenv("VIRE_BLOB_TYPE"); v != "" {
	config.Storage.Blob.Type = v
}
if v := os.Getenv("VIRE_BLOB_PATH"); v != "" {
	config.Storage.Blob.Path = v
}
if v := os.Getenv("VIRE_BLOB_BUCKET"); v != "" {
	config.Storage.Blob.Bucket = v
}
if v := os.Getenv("VIRE_BLOB_PREFIX"); v != "" {
	config.Storage.Blob.Prefix = v
}
if v := os.Getenv("VIRE_BLOB_REGION"); v != "" {
	config.Storage.Blob.Region = v
}
if v := os.Getenv("VIRE_BLOB_ENDPOINT"); v != "" {
	config.Storage.Blob.Endpoint = v
}
if v := os.Getenv("AWS_ACCESS_KEY_ID"); v != "" {
	config.Storage.Blob.AccessKey = v
}
if v := os.Getenv("AWS_SECRET_ACCESS_KEY"); v != "" {
	config.Storage.Blob.SecretKey = v
}
```

### 2. `config/vire-service.toml.example` — Add blob section

Add after the existing `[storage]` block (after line 71):

```toml
[storage.blob]
type = 'file'                              # 'file' (default, local dev) or 's3' (production)
path = 'data/blobs'                        # filesystem path (type=file only)
# S3-compatible settings (type=s3 only):
# bucket   = 'vire-filings'
# prefix   = 'vire'
# region   = 'auto'                        # AWS region, or 'auto' for Tigris/MinIO
# endpoint = ''                            # custom endpoint (Tigris, MinIO, R2)
# Credentials: prefer AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY env vars
# access_key = ''
# secret_key = ''
```

### 3. `internal/storage/blob/file.go` — NEW: Filesystem Implementation

Implements `interfaces.FileStore`. This is the **default** backend for local dev.

```go
package blob

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
)

// FileSystemStore implements interfaces.FileStore using local filesystem.
type FileSystemStore struct {
	basePath string
	logger   *common.Logger
}

// Compile-time check
var _ interfaces.FileStore = (*FileSystemStore)(nil)

func NewFileSystemStore(basePath string, logger *common.Logger) (*FileSystemStore, error) {
	if basePath == "" {
		basePath = "data/blobs"
	}
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create blob base path %s: %w", basePath, err)
	}
	return &FileSystemStore{basePath: basePath, logger: logger}, nil
}
```

**Key design:**
- File path: `{basePath}/{category}/{key}` — sanitize to prevent path traversal
- Sidecar metadata: `{basePath}/{category}/{key}.meta` — JSON `{"content_type":"application/pdf"}`
- `SaveFile`: create parent dirs, write data file + `.meta` sidecar atomically (write to `.tmp`, rename)
- `GetFile`: read data file + parse `.meta` for content type
- `DeleteFile`: remove both data file and `.meta` sidecar
- `HasFile`: `os.Stat` on data file

**Path sanitization** (critical): reject keys containing `..` to prevent directory traversal. Follow the pattern from `surrealdb/filestore.go:40-42` for key sanitization but do NOT replace slashes (they're used for `TICKER/filename.pdf` structure — create subdirs instead).

```go
// blobPath returns the safe filesystem path for a category/key pair.
// Returns error if the key attempts path traversal.
func (s *FileSystemStore) blobPath(category, key string) (string, error) {
	if strings.Contains(category, "..") || strings.Contains(key, "..") {
		return "", fmt.Errorf("invalid path: directory traversal not allowed")
	}
	p := filepath.Join(s.basePath, category, key)
	// Verify resolved path is within basePath
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	baseAbs, err := filepath.Abs(s.basePath)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(abs, baseAbs+string(os.PathSeparator)) && abs != baseAbs {
		return "", fmt.Errorf("invalid path: resolved outside base directory")
	}
	return p, nil
}
```

**Sidecar metadata struct:**
```go
type fileMeta struct {
	ContentType string `json:"content_type"`
}
```

### 4. `internal/storage/blob/s3.go` — NEW: S3-Compatible Implementation

Implements `interfaces.FileStore` using `aws-sdk-go-v2`.

```go
package blob

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
)

// S3Store implements interfaces.FileStore using any S3-compatible object storage.
type S3Store struct {
	client *s3.Client
	bucket string
	prefix string
	logger *common.Logger
}

// Compile-time check
var _ interfaces.FileStore = (*S3Store)(nil)
```

**Constructor:**
```go
func NewS3Store(cfg common.BlobConfig, logger *common.Logger) (*S3Store, error) {
	ctx := context.Background()

	// Build AWS SDK config with optional custom endpoint
	var opts []func(*config.LoadOptions) error

	if cfg.Region != "" && cfg.Region != "auto" {
		opts = append(opts, config.WithRegion(cfg.Region))
	} else {
		opts = append(opts, config.WithRegion("us-east-1")) // default for S3-compatible
	}

	// Static credentials if provided (fallback — prefer env vars)
	if cfg.AccessKey != "" && cfg.SecretKey != "" {
		opts = append(opts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		))
	}

	awsCfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Build S3 client with optional custom endpoint (Tigris, MinIO, R2)
	var s3Opts []func(*s3.Options)
	if cfg.Endpoint != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			o.UsePathStyle = true // Required for MinIO and most S3-compatible stores
		})
	}

	client := s3.NewFromConfig(awsCfg, s3Opts...)

	return &S3Store{
		client: client,
		bucket: cfg.Bucket,
		prefix: cfg.Prefix,
		logger: logger,
	}, nil
}
```

**Object key format:**
```go
// objectKey builds the S3 key: {prefix}/{category}/{key}
func (s *S3Store) objectKey(category, key string) string {
	if s.prefix != "" {
		return s.prefix + "/" + category + "/" + key
	}
	return category + "/" + key
}
```

**Method implementations:**
- `SaveFile` → `PutObject` with `ContentType` metadata
- `GetFile` → `GetObject`, read body, return data + content type from response
- `DeleteFile` → `DeleteObject`
- `HasFile` → `HeadObject`, check for `types.NoSuchKey` error → return false

**Error handling for HasFile:**
```go
func (s *S3Store) HasFile(ctx context.Context, category, key string) (bool, error) {
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.objectKey(category, key)),
	})
	if err != nil {
		var nsk *types.NoSuchKey
		var notFound *types.NotFound
		if errors.As(err, &nsk) || errors.As(err, &notFound) {
			return false, nil
		}
		// HeadObject may also return a generic 404 — check for HTTP 404
		var respErr interface{ HTTPStatusCode() int }
		if errors.As(err, &respErr) && respErr.HTTPStatusCode() == 404 {
			return false, nil
		}
		return false, fmt.Errorf("failed to check blob %s/%s: %w", category, key, err)
	}
	return true, nil
}
```

### 5. `internal/storage/blob/factory.go` — NEW: Backend Factory

```go
package blob

import (
	"fmt"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
)

// NewFileStore creates a FileStore backed by the configured blob storage type.
func NewFileStore(cfg common.BlobConfig, logger *common.Logger) (interfaces.FileStore, error) {
	switch cfg.Type {
	case "s3":
		if cfg.Bucket == "" {
			return nil, fmt.Errorf("blob store type 's3' requires a bucket name ([storage.blob] bucket)")
		}
		return NewS3Store(cfg, logger)
	case "file", "":
		return NewFileSystemStore(cfg.Path, logger)
	default:
		return nil, fmt.Errorf("unknown blob store type: %q (expected 'file' or 's3')", cfg.Type)
	}
}
```

### 6. `internal/storage/surrealdb/manager.go` — Accept External FileStore

**Change the Manager struct** (line 24): change `fileStore` type from `*FileStore` to `interfaces.FileStore`:
```go
// Before:
fileStore       *FileStore
// After:
fileStore       interfaces.FileStore
```

**Change NewManager signature** (line 31) to accept an external FileStore:
```go
// Before:
func NewManager(logger *common.Logger, config *common.Config) (*Manager, error) {
// After:
func NewManager(logger *common.Logger, config *common.Config, fileStore interfaces.FileStore) (*Manager, error) {
```

**Remove internal FileStore creation** (line ~86): delete the line:
```go
// DELETE THIS:
m.fileStore = NewFileStore(db, logger)
```

**Set from parameter instead** (after the `m := &Manager{...}` block):
```go
m.fileStore = fileStore
```

**Remove `"files"` from the tables list** (line 54) — the `files` table is no longer needed for new data. (Keep it for now so existing data can still be read during transition; remove in a follow-up.)

Actually, **keep the `files` table** in the list for now — `PurgeCharts()` in marketstore still uses `DELETE FROM files`. We'll clean that up separately.

### 7. `internal/storage/manager.go` — Pass Blob Store Through

Update the wrapper to create the blob store and pass it:

```go
package storage

import (
	"fmt"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/storage/blob"
	"github.com/bobmcallan/vire/internal/storage/surrealdb"
)

func NewManager(logger *common.Logger, config *common.Config) (interfaces.StorageManager, error) {
	// Create blob-backed FileStore from config
	fileStore, err := blob.NewFileStore(config.Storage.Blob, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create blob store: %w", err)
	}

	manager, err := surrealdb.NewManager(logger, config, fileStore)
	if err != nil {
		return nil, fmt.Errorf("failed to create surrealdb storage manager: %w", err)
	}

	return manager, nil
}
```

### 8. `internal/services/market/filings.go` — Remove Size Limit

**Remove `maxFileStoreBytes` constant** (line 558):
```go
// DELETE:
const maxFileStoreBytes int64 = 7_500_000
```

**Remove the oversized-PDF skip block** (lines 598-609):
```go
// DELETE this entire block:
if fileSize > maxFileStoreBytes {
	os.Remove(tmpPath)
	s.logger.Warn().
		Str("headline", f.Headline).
		Int64("size_bytes", fileSize).
		Int64("limit_bytes", maxFileStoreBytes).
		Msg("Skipping oversized PDF — exceeds SurrealDB CBOR limit after base64 encoding")
	continue
}
```

### 9. `internal/app/app.go` — Resolve Relative Blob Path

Add after the existing `DataPath` resolution (after line 108):

```go
// Resolve relative blob path to binary directory
if config.Storage.Blob.Path != "" && !filepath.IsAbs(config.Storage.Blob.Path) {
	config.Storage.Blob.Path = filepath.Join(binDir, config.Storage.Blob.Path)
}
```

### 10. `go.mod` — Add AWS SDK

Run:
```bash
go get github.com/aws/aws-sdk-go-v2
go get github.com/aws/aws-sdk-go-v2/config
go get github.com/aws/aws-sdk-go-v2/credentials
go get github.com/aws/aws-sdk-go-v2/service/s3
go mod tidy
```

### 11. Test File Mock Updates

Every mock `StorageManager` in test files that currently creates an internal FileStore or returns nil needs no change — they return `interfaces.FileStore` already. But the `surrealdb.NewManager` calls in test helpers need the new parameter.

**`internal/storage/surrealdb/testhelper_test.go`** — no change needed (tests use `testDB()` which returns raw `*surreal.DB`, not `Manager`)

**`tests/data/helpers_test.go`** — update `testManager()` to pass a FileStore:

```go
func testManager(t *testing.T) interfaces.StorageManager {
	t.Helper()
	sc := tcommon.StartSurrealDB(t)
	dataPath := t.TempDir()
	blobPath := t.TempDir()

	cfg := &common.Config{
		Environment: "test",
		Storage: common.StorageConfig{
			Address:   sc.Address(),
			Namespace: "vire_data_test",
			Database:  fmt.Sprintf("d_%s_%d", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()), time.Now().UnixNano()%100000),
			Username:  "root",
			Password:  "root",
			DataPath:  dataPath,
			Blob: common.BlobConfig{
				Type: "file",
				Path: blobPath,
			},
		},
	}

	logger := common.NewSilentLogger()

	// Create blob-backed file store for tests
	fileStore, err := blob.NewFileSystemStore(blobPath, logger)
	if err != nil {
		t.Fatalf("create file store: %v", err)
	}

	mgr, err := surrealdb.NewManager(logger, cfg, fileStore)
	if err != nil {
		t.Fatalf("create storage manager: %v", err)
	}

	t.Cleanup(func() { mgr.Close() })
	return mgr
}
```

**`internal/storage/surrealdb/manager_test.go`** — update any `NewManager()` calls to pass a file store. Follow the pattern: create a `FileSystemStore` with `t.TempDir()` and pass it.

---

## Unit Tests

### `internal/storage/blob/file_test.go` — Filesystem Store Tests

| Test Name | Verifies |
|---|---|
| `TestFileSystemStore_SaveAndGet` | Save file, get it back, verify data + content type |
| `TestFileSystemStore_HasFile` | HasFile returns true for existing, false for missing |
| `TestFileSystemStore_DeleteFile` | Delete removes both data file and .meta sidecar |
| `TestFileSystemStore_OverwriteExisting` | SaveFile with same key overwrites previous data |
| `TestFileSystemStore_NestedKeys` | Keys like `BHP/20250101-doc.pdf` create subdirectories |
| `TestFileSystemStore_PathTraversal` | Keys containing `..` are rejected |
| `TestFileSystemStore_MissingFile` | GetFile on non-existent key returns error |

### `internal/storage/blob/s3_test.go` — S3 Store Tests

Unit tests using a mock S3 client (not a real endpoint). Test:
| Test Name | Verifies |
|---|---|
| `TestS3Store_ObjectKey` | Key format: `{prefix}/{category}/{key}` |
| `TestS3Store_ObjectKeyNoPrefix` | Without prefix: `{category}/{key}` |

Full S3 integration tests belong in `tests/data/` using MinIO testcontainer (Phase 3 — test-creator).

### `internal/storage/blob/factory_test.go` — Factory Tests

| Test Name | Verifies |
|---|---|
| `TestNewFileStore_DefaultFile` | Empty type defaults to filesystem |
| `TestNewFileStore_ExplicitFile` | `type=file` creates FileSystemStore |
| `TestNewFileStore_S3MissingBucket` | `type=s3` without bucket returns error |
| `TestNewFileStore_UnknownType` | Unknown type returns descriptive error |

---

## Integration Tests (test-creator scope)

### `tests/data/blobstore_test.go` — Filesystem Blob Store Integration

Uses `t.TempDir()` (no containers needed for filesystem backend):

| Test Name | Verifies |
|---|---|
| `TestBlobStore_Lifecycle` | Full CRUD: save, has, get, delete, has-after-delete |
| `TestBlobStore_LargeFile` | 20MB file round-trips correctly |
| `TestBlobStore_ContentType` | Content-type preserved in .meta sidecar |
| `TestBlobStore_CategoryIsolation` | Same key in different categories are independent |
| `TestBlobStore_ConcurrentWrites` | 10 goroutines writing different keys |
| `TestBlobStore_NestedKeyStructure` | `TICKER/date-doc.pdf` creates proper directory structure |

---

## Verification Checklist

- [ ] `go build ./cmd/vire-server/` — server compiles
- [ ] `go vet ./...` — clean
- [ ] `go test ./internal/storage/blob/...` — all blob store tests pass
- [ ] `go test ./internal/...` — all existing unit tests pass (mock updates)
- [ ] `go test ./tests/data/...` — integration tests pass
- [ ] Default config (`type=file`) works without any S3 credentials
- [ ] TOML example has correct `[storage.blob]` section
