# Requirements: Separate Storage into UserStore and DataStore

**Date:** 2026-02-13
**Requested:** Split the storage layer into two distinct stores within StorageManager — UserStore for per-user data (encrypted, versioned) and DataStore for shared reference data (market prices, signals, no encryption). This is Stage 1 of the infrastructure plan for multi-tenant cloud deployment.

## Problem

The current `StorageManager` treats everything as a single store with one `FileStore` and one `BlobStore`. Market data (stock prices, fundamentals, technical signals) is identical for all users and doesn't need encryption. User data (portfolios, strategies, plans, watchlists) is personal and must be scoped per-user and encrypted at rest. The cloud deployment requires multi-tenant operation where user data is isolated but market data is shared.

## Scope

### In scope
- Split Manager struct into userStore and dataStore FileStore instances
- Update StorageConfig with UserData and Data paths (rename File → UserData, add Data)
- Update NewStorageManager to create two stores and route domain types appropriately
- Update PurgeDerivedData, WriteRaw, DataPath, Close for two-store operation
- Remove blob and fs fields from Manager (keep BlobStore interface for future GCS)
- Update TOML config files (docker/vire.toml)
- Data migration function (detect old flat layout, move files to new structure)
- Update tests
- Each domain storage constructor creates its own subdirectory (remove hardcoded list)

### Out of scope
- GCS/S3 backend implementation (Stage 3)
- Multi-tenant middleware (later stage)
- Per-request storage prefix (later stage)
- Encryption at rest (later stage)
- Service layer or handler changes (storage split is invisible to them)

## Data Classification

**UserStore** (per-user, `data/user/`): portfolios, strategies, plans, watchlists, reports, searches, kv
**DataStore** (shared, `data/data/`): market, signals, charts

## Approach

Follow the 14-step plan in `docs/storage-separation.md`. Key principles:
- Domain storage types unchanged internally — they already accept a `*FileStore` and directory name
- Two FileStore instances with different base paths naturally separate data
- Remove hardcoded subdirectory list from NewFileStore; each domain constructor creates its own dir
- Migration runs automatically on startup if old flat layout detected

## Files Expected to Change
- `internal/storage/manager.go` — split into userStore/dataStore, update PurgeDerivedData/WriteRaw/DataPath/Close
- `internal/storage/file.go` — remove hardcoded subdirectories, domain constructors create own dirs
- `internal/common/config.go` — StorageConfig with UserData and Data paths
- `internal/interfaces/storage.go` — remove BlobStore accessor, keep domain accessors
- `docker/vire.toml` — new TOML structure with storage.user_data and storage.data
- `internal/storage/migrate.go` — new file for data migration
- `internal/storage/file_test.go` — update for two-store testing
- `internal/storage/blob_test.go` — update if needed

## Reference
- Detailed plan: `docs/storage-separation.md`
- Architecture context: `/home/bobmc/development/vire-infra/docs/architecture-per-user-deployment.md`
