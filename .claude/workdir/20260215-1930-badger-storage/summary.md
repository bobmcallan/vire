# Summary: Migrate user data store from file-based JSON to BadgerHold

**Date:** 2026-02-15
**Status:** Completed

## What Changed

| File | Change |
|------|--------|
| `internal/storage/badger/store.go` | NEW: BadgerHold connection wrapper |
| `internal/storage/badger/user_storage.go` | NEW: UserStorage via BadgerHold |
| `internal/storage/badger/portfolio_storage.go` | NEW: PortfolioStorage via BadgerHold |
| `internal/storage/badger/strategy_storage.go` | NEW: StrategyStorage via BadgerHold (version increment preserved) |
| `internal/storage/badger/plan_storage.go` | NEW: PlanStorage via BadgerHold (version increment preserved) |
| `internal/storage/badger/watchlist_storage.go` | NEW: WatchlistStorage via BadgerHold (version increment preserved) |
| `internal/storage/badger/report_storage.go` | NEW: ReportStorage via BadgerHold |
| `internal/storage/badger/search_storage.go` | NEW: SearchHistoryStorage via BadgerHold (query filtering, auto-pruning) |
| `internal/storage/badger/kv_storage.go` | NEW: KeyValueStorage via BadgerHold |
| `internal/storage/badger/migrate.go` | NEW: One-time file-to-BadgerDB migration |
| `internal/storage/badger/store_test.go` | NEW: 14 tests covering all domains |
| `internal/storage/badger/stress_test.go` | NEW: Concurrent access, large payloads, key injection stress tests |
| `internal/storage/file.go` | Removed all 8 user-domain implementations (-464 lines) |
| `internal/storage/manager.go` | Wired BadgerHold for user stores, kept FileStore for data stores |
| `internal/storage/file_test.go` | Adapted tests for BadgerDB backend |
| `internal/common/config.go` | Default user data path: `data/user/badger` |
| `go.mod` / `go.sum` | Added `badgerhold/v4` dependency |
| `config/vire-service.toml.example` | Updated storage config |
| `tests/docker/vire-blank.toml` | Updated test config |
| `README.md` | Storage architecture updated for BadgerDB |
| `.claude/skills/develop/SKILL.md` | Storage docs updated for BadgerHold pattern |

## Tests
- `internal/storage/badger/store_test.go` — 14 tests: CRUD for all domains, version increment, search filtering, pruning, KV, migration
- `internal/storage/badger/stress_test.go` — concurrent access, large payloads, key injection
- All existing tests pass: `go test ./internal/...` (0 failures)
- `go vet ./...` clean
- Server builds, runs, health endpoint responds `{"status":"ok"}`

## Documentation Updated
- `README.md` — storage architecture section describes BadgerDB for user data, file-based for market data
- `.claude/skills/develop/SKILL.md` — storage table, "adding a new domain storage" instructions, migration notes

## Devils-Advocate Findings
- Migration bug: empty key when migrating users (username field needed as explicit key) — fixed
- Version increment race condition noted (read-before-write not atomic) — assessed as not a regression since file-based had the same pattern; acceptable for single-instance deployment
- Concurrent access stress tests passed with no panics or data corruption

## Notes
- **Net code reduction**: -464 lines from file.go, +new badger package — cleaner separation of concerns
- **No interface changes**: all 10 storage interfaces remain exactly as-is
- **File-based data store preserved**: market data + signals stay as JSON files (different access pattern)
- **Migration is automatic**: first startup with BadgerDB detects old JSON directories and imports them
- **Future NoSQL migration**: interfaces are the contract — swap BadgerHold implementations for MongoDB/DynamoDB without touching services or handlers
