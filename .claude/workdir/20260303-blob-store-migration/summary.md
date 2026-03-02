# Summary: Blob Store Migration

**Status:** completed

## Changes

| File | Change |
|------|--------|
| `internal/storage/blob/file.go` | **NEW** — Filesystem FileStore implementation with path traversal protection, sidecar .meta files |
| `internal/storage/blob/s3.go` | **NEW** — S3-compatible FileStore using aws-sdk-go-v2 (works with Tigris, MinIO, R2) |
| `internal/storage/blob/factory.go` | **NEW** — Backend selection factory: "file" (default) or "s3" |
| `internal/storage/blob/file_test.go` | **NEW** — Filesystem unit tests |
| `internal/storage/blob/s3_test.go` | **NEW** — S3 unit tests (object key formatting) |
| `internal/storage/blob/factory_test.go` | **NEW** — Factory tests (type switching, validation) |
| `internal/storage/blob/stress_test.go` | **NEW** — Filesystem stress tests (path traversal, symlinks, concurrency) |
| `internal/storage/blob/s3_stress_test.go` | **NEW** — S3 stress tests |
| `internal/storage/blob/factory_stress_test.go` | **NEW** — Factory stress tests |
| `internal/common/config.go` | BlobConfig struct replaces S3Config/GCSConfig, defaults, env overrides |
| `internal/storage/surrealdb/manager.go` | Accepts external `interfaces.FileStore` parameter, no longer creates internal FileStore |
| `internal/storage/manager.go` | Creates blob store from config, passes to surrealdb.NewManager |
| `internal/app/app.go` | Resolves relative blob path to binary directory |
| `internal/services/market/filings.go` | Removed `maxFileStoreBytes` (7.5MB) size limit |
| `config/vire-service.toml.example` | Added `[storage.blob]` section with documented defaults |
| `go.mod` / `go.sum` | Added aws-sdk-go-v2 dependencies |
| `tests/data/blobstore_test.go` | **NEW** — 20 integration tests for filesystem blob store |
| `tests/data/helpers_test.go` | Updated testManager() for new 3-arg NewManager |
| `tests/api/job_recovery_test.go` | Updated 5 call sites for new NewManager signature |
| `internal/storage/surrealdb/manager_test.go` | Refactored to testManagerWithBlob helper |

## Tests

- **Unit tests**: 70+ tests (file, s3, factory, stress) — all pass
- **Integration tests**: 20 tests (blobstore_test.go) — all pass
- **Full suite**: 1698/1714 pass (99.1%) — 16 failures all pre-existing
- **Fix rounds**: 1 (team lead fixed 9 test call sites for NewManager signature change)

## Architecture

- Architect reviewed: FULLY COMPLIANT
- `interfaces.FileStore` unchanged — zero-impact swap
- No new interfaces on StorageManager
- Config follows existing TOML + env override pattern
- docs/architecture/26-02-27-storage.md updated

## Devils-Advocate

- Symlink traversal vulnerability found and fixed in file.go
- Path traversal protection verified (.. rejection + abs path check)
- Concurrent write safety verified
- S3 error type handling verified (NoSuchKey, NotFound, HTTP 404)
- 42 stress tests pass

## Notes

- Default config (`type=file`) works with zero S3 credentials
- Switch to S3: set `type='s3'` + bucket + endpoint + credentials
- Existing `files` table data becomes orphaned — PDFs re-download on next collection cycle
- Drop `files` table post-deploy: `REMOVE TABLE files;`
- No schema version bump needed (storage backend swap, not data structure change)
