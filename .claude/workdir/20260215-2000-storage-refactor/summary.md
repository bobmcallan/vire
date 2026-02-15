# Summary: Refactor storage into 3 areas — internal, user, market

**Date:** 2026-02-15
**Status:** Completed

## What Changed

### New packages
| File | Change |
|------|--------|
| `internal/models/storage.go` | NEW: InternalUser, UserKeyValue, UserRecord models |
| `internal/storage/internaldb/store.go` | NEW: BadgerDB store for auth + user config + system KV |
| `internal/storage/internaldb/store_test.go` | NEW: Tests for internal store |
| `internal/storage/internaldb/stress_test.go` | NEW: Stress tests (concurrent access, key isolation) |
| `internal/storage/userdb/store.go` | NEW: BadgerDB generic document store (userid, subject, key, value, version, datetime) |
| `internal/storage/userdb/store_test.go` | NEW: Tests for user data store |
| `internal/storage/userdb/stress_test.go` | NEW: Stress tests (composite key injection, cross-subject contamination) |
| `internal/storage/marketfs/store.go` | NEW: FileStore for market data + signals |

### Modified files
| File | Change |
|------|--------|
| `internal/interfaces/storage.go` | Replaced 8 domain interfaces with InternalStore + UserDataStore |
| `internal/storage/manager.go` | Wires 3 stores, updated purge operations |
| `internal/storage/migrate.go` | Rewritten: old BadgerDB → 3-area layout migration |
| `internal/common/config.go` | 3-area StorageConfig (internal, user, market) |
| `internal/common/userctx.go` | Added ResolveUserID(ctx) helper |
| `internal/server/middleware.go` | Uses InternalStore for user resolution |
| `internal/server/handlers_user.go` | Uses InternalStore for user CRUD + UserKeyValue for prefs |
| `internal/server/handlers.go` | Uses UserDataStore via services |
| `internal/server/routes.go` | Updated handler constructors |
| `internal/app/app.go` | Uses InternalStore for system KV, API keys |
| `internal/app/import.go` | Uses InternalStore for user import |
| `internal/services/portfolio/service.go` | Uses UserDataStore (JSON marshal/unmarshal) |
| `internal/services/strategy/service.go` | Uses UserDataStore |
| `internal/services/plan/service.go` | Uses UserDataStore |
| `internal/services/watchlist/service.go` | Uses UserDataStore |
| `internal/services/report/service.go` | Uses UserDataStore |
| `config/vire-service.toml.example` | 3-area config format |
| `README.md` | Updated storage architecture |
| `.claude/skills/develop/SKILL.md` | Updated storage docs |

### Deleted files
| File | Reason |
|------|--------|
| `internal/storage/badger/` (10 files) | Replaced by internaldb + userdb |
| `internal/storage/file.go` | Replaced by marketfs |
| `internal/storage/blob.go`, `file_blob.go`, `factory.go` | Unused |
| `internal/storage/blob_test.go`, `file_test.go` | Tests moved to new packages |
| `internal/models/user.go` | Replaced by InternalUser + UserKeyValue in storage.go |

## Architecture

```
data/
├── internal/    # BadgerDB — users (auth), user config (KV), system config
├── user/        # BadgerDB — generic table: {userid, subject, key, value, version, datetime}
└── market/      # FileStore — market data, signals, charts
```

**Net code change:** -5,957 lines (1,435 added, 7,392 removed)

## Tests
- `internal/storage/internaldb/store_test.go` — user CRUD, UserKV CRUD, system KV, isolation
- `internal/storage/internaldb/stress_test.go` — concurrent access, system KV isolation
- `internal/storage/userdb/store_test.go` — UserRecord CRUD, list by subject, query, delete by subject
- `internal/storage/userdb/stress_test.go` — composite key injection, cross-subject contamination
- All updated service/handler tests pass
- `go test ./internal/...` — all packages pass
- `go vet ./...` — clean

## Documentation Updated
- `README.md` — 3-area storage architecture
- `.claude/skills/develop/SKILL.md` — storage table, interfaces, config, migration docs
- `config/vire-service.toml.example` — 3-area config

## Devils-Advocate Findings
- System KV isolation: system keys were accessible as user keys — fixed with separate prefix
- Composite key injection: tested with colons and special characters — handled correctly
- Cross-subject contamination: verified portfolio ops don't affect strategy data
- Purge safety: confirmed strategies, plans, watchlists preserved during PurgeDerivedData

## Notes
- **Massive simplification**: 8 typed domain stores → 1 generic document table
- **User model split**: auth/identity in InternalStore, preferences as UserKeyValue entries
- **Future-ready**: UserRecord table maps directly to a NoSQL/document database collection
- **Services handle serialization**: each service marshals/unmarshals its own models to UserRecord.Value
- **user_id field**: structural (for future multi-tenant), defaults to "default" when no user context
