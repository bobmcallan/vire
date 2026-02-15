# Requirements: Migrate user data store from file-based JSON to BadgerHold

**Date:** 2026-02-15
**Requested:** Replace the file-based JSON storage for all `data/user/` domain stores with BadgerDB/BadgerHold. Keep file-based storage for `data/data/` (market data, signals, charts). Design for future migration to a NoSQL/document database. No backward compatibility required.

## Scope

### In scope
- Add `badgerhold/v4` dependency
- Create `internal/storage/badger/` package with BadgerHold implementations for all 8 user-domain stores
- Update Manager to wire BadgerHold stores for user data, keep FileStore for data store
- Add BadgerDB config to StorageConfig
- One-time migration from existing file-based JSON to BadgerDB
- Update purge operations for BadgerDB
- Remove file-based user-domain implementations from file.go
- Update tests
- Update documentation

### Out of scope
- Changing storage interfaces (they stay exactly as-is)
- Moving `data/data/` stores (market, signals) to BadgerDB
- Multi-tenant / per-user isolation (separate concern)
- Cloud backends (GCS/S3) — future work

## Approach

### Why BadgerHold

1. **Performance**: User data is accessed every request (middleware user lookup). File I/O per request is slow. BadgerDB's LSM-tree provides fast reads with built-in caching.
2. **Concurrency**: File-based storage has no locking. BadgerDB handles concurrent access natively.
3. **Query support**: SearchHistory currently scans all files into memory for filtering. BadgerHold provides type-safe queries with optional indexes.
4. **Document-oriented**: BadgerHold stores Go structs as serialized documents — natural stepping stone to MongoDB/DynamoDB. The existing interfaces remain unchanged, only the implementation swaps.
5. **Proven pattern**: vire-portal already uses badgerhold/v4 for KV storage. Same library, same patterns.

### Why NOT for data store

Market data and signals stay file-based because:
- High-volume, frequently overwritten derived data (not user-authored)
- Large payloads (OHLCV history arrays, fundamentals)
- File-per-ticker is simple and allows easy inspection/debugging
- No concurrent access concerns (single scheduler writes)

### Architecture

**Before:**
```
data/user/  ← FileStore (JSON files)
  portfolios/, strategies/, plans/, watchlists/,
  users/, reports/, searches/, kv/

data/data/  ← FileStore (JSON files)
  market/, signals/, charts/
```

**After:**
```
data/user/badger/  ← BadgerDB (single embedded DB)
  All 8 domain types stored by BadgerHold type separation

data/data/  ← FileStore (unchanged)
  market/, signals/, charts/
```

### 1. New package: `internal/storage/badger/`

**store.go** — BadgerDB connection wrapper (based on portal pattern):
```go
type Store struct {
    db     *badgerhold.Store
    logger *common.Logger
}
func NewStore(logger *common.Logger, path string) (*Store, error)
func (s *Store) Close() error
```

**8 domain storage files** — each implements the corresponding interface:
- `user_storage.go` → `interfaces.UserStorage`
- `portfolio_storage.go` → `interfaces.PortfolioStorage`
- `strategy_storage.go` → `interfaces.StrategyStorage`
- `plan_storage.go` → `interfaces.PlanStorage`
- `watchlist_storage.go` → `interfaces.WatchlistStorage`
- `report_storage.go` → `interfaces.ReportStorage`
- `search_storage.go` → `interfaces.SearchHistoryStorage`
- `kv_storage.go` → `interfaces.KeyValueStorage`

Each follows the same pattern:
```go
type userStorage struct {
    store  *Store
    logger *common.Logger
}

func (s *userStorage) GetUser(ctx context.Context, username string) (*models.User, error) {
    var user models.User
    err := s.store.db.Get(username, &user)
    if err == badgerhold.ErrNotFound {
        return nil, fmt.Errorf("user '%s' not found", username)
    }
    return &user, err
}

func (s *userStorage) SaveUser(ctx context.Context, user *models.User) error {
    return s.store.db.Upsert(user.Username, user)
}

func (s *userStorage) ListUsers(ctx context.Context) ([]string, error) {
    var users []models.User
    err := s.store.db.Find(&users, nil)
    // extract usernames from results
}
```

**Version increment logic** preserved in strategy/plan/watchlist implementations — same read-before-write pattern, just using BadgerHold Get/Upsert instead of readJSON/writeJSON.

**SearchHistory queries** use BadgerHold's query API:
```go
query := &badgerhold.Query{}
if options.Type != "" {
    query = badgerhold.Where("Type").Eq(options.Type)
}
if options.Exchange != "" {
    if query != nil { query = query.And("Exchange").Eq(options.Exchange) }
}
store.db.Find(&records, query)
```

**migrate.go** — one-time migration:
- Check if `data/user/users/` directory exists (old file layout indicator)
- Read all JSON files from each subdirectory, insert into BadgerDB
- Rename `data/user/{portfolios,strategies,...}` to `data/user/.migrated-{timestamp}/`
- Log counts per domain type
- Called from manager initialization

### 2. Config changes — `internal/common/config.go`

Replace `FileConfig` for user data with BadgerDB path:
```toml
[storage]
backend = "badger"  # default changes from "file" to "badger"

[storage.user_data]
path = "data/user/badger"  # BadgerDB directory

[storage.data]
path = "data/data"  # File-based (unchanged)
```

StorageConfig.UserData stays as FileConfig (just the path field is used). No structural change needed — BadgerDB just needs a directory path.

### 3. Manager changes — `internal/storage/manager.go`

- Create `badger.Store` for user data
- Wire all 8 user-domain stores from `badger` package
- Keep `FileStore` + file-based implementations for data store
- Update `PurgeDerivedData()` to use BadgerHold delete-by-type for portfolios, reports, searches
- Update `PurgeReports()` similarly
- Update `Close()` to close BadgerDB

### 4. Clean up file.go

Remove all user-domain implementations from file.go:
- `userStorage`, `portfolioStorage`, `strategyStorage`, `planStorage`, `watchlistStorage`, `reportStorage`, `searchHistoryStorage`, `kvStorage`

Keep:
- `FileStore` struct and base methods (readJSON, writeJSON, etc.)
- `marketDataStorage`, `signalStorage` (data store implementations)
- `WriteRaw` (used for charts)
- Purge methods that apply to file-based data store

### 5. File-based versioning

Drop file-based `.vN` versioning for user data. BadgerDB handles durability. Models with version tracking (Strategy, Plan, Watchlist) already have `Version` fields that auto-increment on save. The `.vN` file rotation was a safety net — BadgerDB's WAL provides crash recovery instead.

The `FileConfig.Versions` field in config still applies to the data store (which stays file-based).

## Files Expected to Change

### New files
- `internal/storage/badger/store.go` — BadgerDB connection wrapper
- `internal/storage/badger/user_storage.go` — UserStorage implementation
- `internal/storage/badger/portfolio_storage.go` — PortfolioStorage implementation
- `internal/storage/badger/strategy_storage.go` — StrategyStorage implementation
- `internal/storage/badger/plan_storage.go` — PlanStorage implementation
- `internal/storage/badger/watchlist_storage.go` — WatchlistStorage implementation
- `internal/storage/badger/report_storage.go` — ReportStorage implementation
- `internal/storage/badger/search_storage.go` — SearchHistoryStorage implementation
- `internal/storage/badger/kv_storage.go` — KeyValueStorage implementation
- `internal/storage/badger/migrate.go` — File-to-BadgerDB migration
- `internal/storage/badger/store_test.go` — BadgerHold storage tests

### Modified files
- `go.mod` / `go.sum` — add badgerhold/v4 dependency
- `internal/common/config.go` — default backend change
- `internal/storage/manager.go` — wire BadgerHold stores, update purge ops
- `internal/storage/file.go` — remove user-domain implementations (keep FileStore base + data stores)
- `internal/storage/file_test.go` — update for removed implementations
- `README.md` — storage architecture section
- `.claude/skills/develop/SKILL.md` — storage section update

### Deleted (code removal from file.go)
- `userStorage`, `portfolioStorage`, `strategyStorage`, `planStorage`
- `watchlistStorage`, `reportStorage`, `searchHistoryStorage`, `kvStorage`
- All their constructors (`newUserStorage`, etc.)
