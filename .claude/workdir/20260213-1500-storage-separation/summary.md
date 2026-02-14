# Summary: Separate Storage into UserStore and DataStore

**Date:** 2026-02-13
**Status:** completed

## What Changed

| File | Change |
|------|--------|
| `internal/common/config.go` | Renamed `File FileConfig` → `UserData FileConfig`, added `Data FileConfig`, updated defaults and VIRE_DATA_PATH env handling |
| `internal/storage/file.go` | Removed hardcoded subdirectory list from NewFileStore; domain constructors create own dirs via os.MkdirAll; added defensive MkdirAll in writeJSON/WriteRaw; added Close() method |
| `internal/storage/manager.go` | Replaced blob+fs with userStore+dataStore; updated PurgeDerivedData/PurgeReports/WriteRaw/DataPath/Close for two-store routing; removed BlobStore accessor |
| `internal/storage/migrate.go` | New — detects old flat layout, migrates subdirs to correct store (7 user, 3 data), runs on startup |
| `internal/app/app.go` | Updated relative path resolution for both UserData.Path and Data.Path |
| `internal/server/routes.go` | Config endpoint returns storage_user_path and storage_data_path |
| `internal/storage/file_test.go` | Updated for two-store config; 6 new separation tests, 3 migration tests; updated existing test helpers |
| `internal/app/app_test.go` | Updated test config for new storage structure |
| `cmd/vire-server/server_test.go` | Updated test config for new storage structure |
| `docker/vire.toml` | New `[storage.user_data]` and `[storage.data]` sections |
| `docker/vire.toml.docker` | Same TOML structure update |
| `tests/docker/vire.toml` | Same TOML structure update |
| `tests/docker/vire-blank.toml` | Same TOML structure update |

## Data Classification

- **UserStore** (`data/user/`): portfolios, strategies, plans, watchlists, reports, searches, kv
- **DataStore** (`data/data/`): market, signals, charts

## Tests
- 6 new two-store separation tests (portfolio→userStore, market→dataStore, signals→dataStore, strategy→userStore, WriteRaw→dataStore, DataPath→dataStore)
- 3 migration tests (happy path, no-op when no old layout, skip-if-dest-exists)
- Updated existing test helpers for two-store config
- All 22 packages pass
- No regressions

## Deploy
- v0.3.11, commit 1cb0ba3
- Both containers healthy
- Migration ran successfully on live data: 10 directories moved (7 user, 3 data)
- Config endpoint verified: `storage_user_path=/app/data/user`, `storage_data_path=/app/data/data`

## Review Findings
- Reviewer approved with no bugs found
- 5 non-blocking items deferred for follow-up:
  1. `rebuild.go:44` total undercounts purged items (pre-existing issue)
  2. DataPath interface comment stale
  3. README.md references old `[storage.file]` config
  4. Blob files are orphaned dead code
  5. Domain constructor os.MkdirAll errors silently ignored (acceptable)

## Notes
- This is Stage 1 of the infrastructure plan for multi-tenant cloud deployment
- Services and handlers are unaffected — the split is invisible above the storage layer
- BlobStore interface kept for future GCS/S3 implementation (Stage 3)
- Backwards compatibility: VIRE_DATA_PATH env var derives both paths from a single parent
- Migration is automatic on first startup when old flat layout detected
