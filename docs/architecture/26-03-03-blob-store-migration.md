# Blob Store Migration — SurrealDB to S3-Compatible Object Storage

## Problem

Binary files (filing PDFs, chart images) are stored as base64-encoded blobs inside SurrealDB records via the `files` table. This causes:

1. **CBOR size limit** — SurrealDB's 10MB document limit restricts raw PDF size to ~7.5MB (after base64 overhead). Larger filings (e.g. FY24 annual presentations at 17MB) are silently skipped.
2. **Database bloat** — Binary data dominates disk usage. A 10GB SurrealDB volume fills quickly when each PDF consumes 1.3× its raw size in-database.
3. **No disk reclaim** — `DELETE FROM files` marks space as reusable but doesn't shrink the data file. Schema-bump purges cause full rebuild load spikes without freeing disk.

## Solution

Replace the `FileStore` implementation with a pluggable `BlobStore` interface backed by:

| Environment | Backend | Notes |
|---|---|---|
| **Production** (Fly.io) | Tigris | S3-compatible, co-located with Fly machines |
| **Local dev** | Filesystem | `data/blobs/` directory, zero dependencies |
| **CI / GitHub Actions** | MinIO | S3-compatible testcontainer (Docker) |
| **Future** | AWS S3, R2, GCS | Change endpoint + credentials only |

The existing `FileStore` interface is preserved — only the implementation changes. No callers need modification.

## Design

### Interface (unchanged)

The existing `interfaces.FileStore` already defines the correct abstraction:

```go
type FileStore interface {
    SaveFile(ctx context.Context, category, key string, data []byte, contentType string) error
    GetFile(ctx context.Context, category, key string) ([]byte, string, error)
    DeleteFile(ctx context.Context, category, key string) error
    HasFile(ctx context.Context, category, key string) (bool, error)
}
```

No interface changes needed. The `StorageManager.FileStore()` accessor continues to return `interfaces.FileStore`.

### Implementations

Two new implementations replace `internal/storage/surrealdb/filestore.go`:

#### 1. `internal/storage/blob/s3.go` — S3-Compatible Store

```go
type S3Store struct {
    client *s3.Client
    bucket string
    prefix string
    logger *common.Logger
}
```

- Uses `aws-sdk-go-v2` — works with any S3-compatible endpoint (Tigris, MinIO, AWS, R2)
- Object key format: `{prefix}/{category}/{key}` (e.g. `vire/filing_pdf/BHP/20250101-03063826.pdf`)
- Content-Type stored as S3 object metadata (native, no encoding overhead)
- No size limit (S3 supports up to 5TB per object)
- `HasFile` uses `HeadObject` (no data transfer)

#### 2. `internal/storage/blob/file.go` — Local Filesystem Store

```go
type FileSystemStore struct {
    basePath string
    logger   *common.Logger
}
```

- Stores files at `{basePath}/{category}/{key}` on local disk
- Content-Type tracked via `.meta` sidecar files (JSON: `{"content_type": "application/pdf"}`)
- Used for local development and as the default when no S3 config is provided
- `HasFile` uses `os.Stat`

### Configuration

#### TOML Config

```toml
[storage.blob]
type = 'file'                              # 'file' (default) or 's3'
path = 'data/blobs'                        # filesystem path (type=file only)

# S3-compatible settings (type=s3 only)
bucket   = ''                              # e.g. 'vire-filings'
prefix   = 'vire'                          # key prefix within bucket
region   = 'auto'                          # AWS region or 'auto' for Tigris/MinIO
endpoint = ''                              # custom endpoint (Tigris, MinIO, R2)
# Credentials: prefer env vars (AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY)
# These TOML fields are fallback only:
access_key = ''
secret_key = ''
```

#### Config Struct

```go
// BlobConfig holds blob/object storage configuration.
type BlobConfig struct {
    Type      string `toml:"type"`       // "file" (default) or "s3"
    Path      string `toml:"path"`       // filesystem base path (type=file)
    Bucket    string `toml:"bucket"`     // S3 bucket name (type=s3)
    Prefix    string `toml:"prefix"`     // S3 key prefix (type=s3)
    Region    string `toml:"region"`     // S3 region (type=s3)
    Endpoint  string `toml:"endpoint"`   // custom S3 endpoint (type=s3)
    AccessKey string `toml:"access_key"` // S3 access key (type=s3, prefer env)
    SecretKey string `toml:"secret_key"` // S3 secret key (type=s3, prefer env)
}
```

Location in `Config`: `Storage.Blob BlobConfig` — nested under `[storage.blob]`.

#### Defaults

```go
Storage: StorageConfig{
    // ... existing SurrealDB fields ...
    Blob: BlobConfig{
        Type:   "file",
        Path:   "data/blobs",
        Prefix: "vire",
        Region: "auto",
    },
},
```

**Default is `file`** — zero-config local dev. Adding `type = 's3'` + credentials switches to object storage. No other changes needed.

#### Environment Variable Overrides

```
VIRE_BLOB_TYPE=s3                           # override type
VIRE_BLOB_BUCKET=vire-filings              # override bucket
VIRE_BLOB_ENDPOINT=fly.storage.tigris.dev  # override endpoint
AWS_ACCESS_KEY_ID=...                       # standard AWS SDK env var
AWS_SECRET_ACCESS_KEY=...                   # standard AWS SDK env var
```

The S3 store uses the standard AWS SDK credential chain: env vars → shared credentials → IAM role. The TOML `access_key`/`secret_key` fields are used only as fallback via `credentials.NewStaticCredentialsProvider`.

### Backend Selection (Factory)

```go
// internal/storage/blob/factory.go

func NewFileStore(cfg common.BlobConfig, logger *common.Logger) (interfaces.FileStore, error) {
    switch cfg.Type {
    case "s3":
        return NewS3Store(cfg, logger)
    case "file", "":
        return NewFileSystemStore(cfg.Path, logger)
    default:
        return nil, fmt.Errorf("unknown blob store type: %q", cfg.Type)
    }
}
```

### Wiring

In `cmd/vire-server/main.go` or server setup:

```go
// Before: FileStore was internal to SurrealDB Manager
// After: FileStore created independently, passed to Manager

blobStore, err := blob.NewFileStore(config.Storage.Blob, logger)
// ...
manager, err := surrealdb.NewManager(logger, config, blobStore)
```

The `Manager` struct replaces its internal `*FileStore` with an `interfaces.FileStore` parameter, removing the SurrealDB `files` table dependency for binary data.

## File Changes

| File | Change |
|---|---|
| `internal/common/config.go` | Add `BlobConfig` struct, nest under `StorageConfig.Blob`, add defaults, add env overrides |
| `config/vire-service.toml.example` | Add `[storage.blob]` section with defaults |
| `internal/storage/blob/factory.go` | **NEW** — backend selection factory |
| `internal/storage/blob/s3.go` | **NEW** — S3-compatible implementation |
| `internal/storage/blob/file.go` | **NEW** — local filesystem implementation |
| `internal/storage/surrealdb/manager.go` | Accept `interfaces.FileStore` parameter instead of creating internal `FileStore` |
| `internal/storage/surrealdb/filestore.go` | **RETAIN** — keep as fallback / migration source, but no longer default |
| `internal/services/market/filings.go` | Remove `maxFileStoreBytes` limit (no longer needed with S3) |
| `go.mod` | Add `github.com/aws/aws-sdk-go-v2` + sub-packages |

## Testing

### Unit Tests

- `internal/storage/blob/file_test.go` — filesystem CRUD, sidecar metadata, concurrent writes, missing dirs
- `internal/storage/blob/s3_test.go` — mock S3 client, error paths, key formatting

### Integration Tests

- `tests/data/blobstore_test.go` — MinIO testcontainer:
  - Full CRUD lifecycle
  - Large file storage (50MB+)
  - Content-Type round-trip
  - Concurrent access
  - Key isolation (category/key namespace)

MinIO testcontainer setup:

```go
container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
    ContainerRequest: testcontainers.ContainerRequest{
        Image:        "minio/minio:latest",
        ExposedPorts: []string{"9000/tcp"},
        Cmd:          []string{"server", "/data"},
        Env: map[string]string{
            "MINIO_ROOT_USER":     "minioadmin",
            "MINIO_ROOT_PASSWORD": "minioadmin",
        },
        WaitingFor: wait.ForHTTP("/minio/health/live").WithPort("9000"),
    },
    Started: true,
})
```

### Existing Tests

No changes to existing mock `StorageManager` implementations — they already return `nil` from `FileStore()`. The `FileStore` interface is unchanged.

## Migration

### Phase 1: Dual-Write (Optional)

Not needed. Filing PDFs are re-downloadable from ASX. Charts are regenerated on demand.

### Phase 2: Cutover

1. Deploy with `type = 's3'` in config
2. Existing `files` table data becomes orphaned (PDFs re-downloaded on next collection cycle)
3. After confirming S3 storage works, drop the `files` table to reclaim disk:
   ```sql
   REMOVE TABLE files;
   ```

### Disk Impact

- Current: ~8GB of base64 PDFs + charts in SurrealDB `files` table
- After: ~6GB in S3 (no base64 overhead) + ~2GB freed from SurrealDB volume
- Net SurrealDB reduction: ~80% of current disk usage

## No Lock-in

- **Tigris** is S3-compatible — code uses standard `aws-sdk-go-v2`, not Tigris SDK
- **MinIO** is S3-compatible — same code works for CI and self-hosted
- **Local dev** uses filesystem — no cloud dependency at all
- Switching providers = changing `endpoint` + credentials in config
- The `GCSConfig` struct (already in config.go) can be implemented later as a third backend if needed
