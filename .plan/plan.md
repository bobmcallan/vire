# FileStorageManager Migration Proposal

## 1. Current State Analysis

### Storage Interfaces (9 sub-interfaces + 1 coordinator)
All defined in `internal/interfaces/storage.go`:

| Interface | Methods | Key Type |
|-----------|---------|----------|
| `StorageManager` | `PortfolioStorage()`, `MarketDataStorage()`, `SignalStorage()`, `KeyValueStorage()`, `ReportStorage()`, `StrategyStorage()`, `PlanStorage()`, `SearchHistoryStorage()`, `WatchlistStorage()`, `PurgeDerivedData()`, `Close()` | coordinator |
| `PortfolioStorage` | `GetPortfolio(name)`, `SavePortfolio(p)`, `ListPortfolios()`, `DeletePortfolio(name)` | `Portfolio.ID` (= Name) |
| `MarketDataStorage` | `GetMarketData(ticker)`, `SaveMarketData(d)`, `GetMarketDataBatch(tickers)`, `GetStaleTickers(exchange, maxAge)` | `MarketData.Ticker` |
| `SignalStorage` | `GetSignals(ticker)`, `SaveSignals(s)`, `GetSignalsBatch(tickers)` | `TickerSignals.Ticker` |
| `KeyValueStorage` | `Get(key)`, `Set(key, value)`, `Delete(key)`, `GetAll()` | arbitrary string key |
| `ReportStorage` | `GetReport(portfolio)`, `SaveReport(r)`, `ListReports()`, `DeleteReport(portfolio)` | `PortfolioReport.Portfolio` |
| `StrategyStorage` | `GetStrategy(portfolio)`, `SaveStrategy(s)`, `DeleteStrategy(portfolio)`, `ListStrategies()` | `PortfolioStrategy.PortfolioName` |
| `PlanStorage` | `GetPlan(portfolio)`, `SavePlan(p)`, `DeletePlan(portfolio)`, `ListPlans()` | `PortfolioPlan.PortfolioName` |
| `SearchHistoryStorage` | `SaveSearch(r)`, `GetSearch(id)`, `ListSearches(opts)`, `DeleteSearch(id)` | `SearchRecord.ID` |
| `WatchlistStorage` | `GetWatchlist(portfolio)`, `SaveWatchlist(w)`, `DeleteWatchlist(portfolio)`, `ListWatchlists()` | `PortfolioWatchlist.PortfolioName` |

### BadgerDB Dependencies to Remove
- `go.mod`: `github.com/timshannon/badgerhold/v4 v4.0.3` (direct), `github.com/dgraph-io/badger/v4 v4.9.0` (indirect)
- Files with badgerhold imports: `internal/storage/badger.go`, `internal/storage/manager.go`, `internal/storage/manager_test.go`, `internal/storage/strategy_test.go`
- `internal/storage/search_history_test.go` uses `NewBadgerDB` directly

### Gob Registrations to Remove
All `init()` functions with `gob.Register(...)` calls in:
- `internal/models/portfolio.go` (11 registrations)
- `internal/models/market.go` (23 registrations)
- `internal/models/signals.go` (7 registrations)
- `internal/models/strategy.go` (10 registrations)
- `internal/models/plan.go` (2 registrations)
- `internal/models/watchlist.go` (2 registrations)
- `internal/models/report.go` (2 registrations)
- `internal/models/navexa.go` (4 registrations)

### Badgerhold Struct Tags to Remove
Models with `badgerhold:"key"` or `badgerhold:"index"` tags:
- `models.Portfolio` (ID key, Name index)
- `models.MarketData` (Ticker key, Exchange index, LastUpdated index)
- `models.TickerSignals` (Ticker key)
- `models.PortfolioReport` (Portfolio key, GeneratedAt index)
- `models.PortfolioStrategy` (PortfolioName key)
- `models.PortfolioPlan` (PortfolioName key)
- `models.PortfolioWatchlist` (PortfolioName key)
- `models.SearchRecord` (ID key, Type index, Exchange index, CreatedAt index)

### Config References to Update
- `internal/common/config.go`: `StorageConfig.Badger BadgerConfig`, `BadgerConfig{Path}`, `NewDefaultConfig()`
- `cmd/vire-mcp/main.go`: `config.Storage.Badger.Path` (lines 58-59)
- `cmd/vire-mcp/handlers.go`: line 894 config display
- `config/vire.toml.example`: `[storage.badger]` section
- `tests/docker/vire.toml`: `[storage.badger]` section
- `internal/common/config.go`: `applyEnvOverrides` references `config.Storage.Badger.Path`

---

## 2. Proposed FileStorageManager Design

### Directory Structure
```
data/
  portfolios/      # One JSON file per portfolio: {name}.json
  market/          # One JSON file per ticker: {ticker}.json
  signals/         # One JSON file per ticker: {ticker}.json
  reports/         # One JSON file per portfolio: {portfolio}.json
  strategies/      # One JSON file per portfolio: {portfolio}.json
  plans/           # One JSON file per portfolio: {portfolio}.json
  watchlists/      # One JSON file per portfolio: {portfolio}.json
  searches/        # One JSON file per search: {id}.json
  kv/              # One JSON file per key: {key}.json
```

### File Naming Convention
- Keys are sanitized for filesystem safety: replace `/`, `\`, `:` etc. with `_`
- Dots in tickers (e.g., `BHP.AU`) are preserved (dots are safe in filenames)
- Files use `.json` extension
- Version files use `.json.v1`, `.json.v2`, etc. suffixes

### Versioning Strategy
- On each write, before overwriting, rotate existing file to a version backup
- Pattern: `{name}.json.v1` (oldest) through `{name}.json.v{N}` (newest backup)
- On save: shift versions up (v4->v5, v3->v4, v2->v3, v1->v2), current->v1, write new current
- Configurable retention: `[storage.file] versions = 5` in TOML (default 5)
- Version 0 means no versioning (just overwrite)
- **No external Go libraries needed** -- the suffix rotation pattern is simple enough to implement directly (afero adds unnecessary abstraction for this use case)

### Atomic Writes
- Write to `{name}.json.tmp.{pid}` (temp file in same directory)
- `os.Rename(tmp, target)` for atomic swap
- This works on local filesystems AND GCS FUSE mounts (which support rename within the same directory)
- Temp files include PID to avoid collisions between concurrent processes

### Concurrent Access
- No exclusive locks (solving the BadgerDB problem)
- Reads: `os.ReadFile()` -- always succeeds, returns consistent snapshot
- Writes: atomic rename ensures readers never see partial writes
- Multiple processes can read/write simultaneously
- Last-writer-wins semantics (acceptable for this use case -- same as current behavior)

### JSON Serialization
- `json.MarshalIndent(data, "", "  ")` for human-readable output
- All models already have `json` struct tags
- No gob encoding needed

### GetStaleTickers Implementation
- BadgerDB uses indexed queries: `WHERE Exchange = ? AND LastUpdated < ?`
- File-based: scan all files in `data/market/`, unmarshal each, filter in Go
- For the typical portfolio size (10-50 tickers), this is fast enough
- Optimization: read only metadata (unmarshal just ticker/exchange/lastUpdated fields) if performance becomes an issue

### ListSearches with Filters
- BadgerDB uses indexed queries with `WHERE Type = ? AND Exchange = ?`
- File-based: scan all files in `data/searches/`, unmarshal, filter in Go, sort by CreatedAt descending
- Auto-prune: same logic as current (cap at 50 records, delete oldest on save)

---

## 3. Config Changes

### Before (BadgerDB)
```toml
[storage.badger]
path = "data"
```

### After (File)
```toml
[storage.file]
path = "data"
versions = 5
```

### Go Config Structs
```go
type StorageConfig struct {
    File FileConfig `toml:"file"`
}

type FileConfig struct {
    Path     string `toml:"path"`
    Versions int    `toml:"versions"`
}
```

Default: `Path = "data"`, `Versions = 5`

### Env Override
`VIRE_DATA_PATH` continues to work, now sets `config.Storage.File.Path`.

---

## 4. Implementation Plan

### File: `internal/storage/file.go` (NEW)
Core FileStorageManager with:
- `readJSON(dir, key, dest)` -- read and unmarshal
- `writeJSON(dir, key, data)` -- marshal, rotate versions, atomic write
- `deleteJSON(dir, key)` -- remove file and all versions
- `listKeys(dir)` -- list all `.json` files, return keys
- `sanitizeKey(key)` -- make key filesystem-safe

### File: `internal/storage/manager.go` (MODIFY)
- Replace `BadgerDB` with `FileStore` reference
- `NewStorageManager` now creates `FileStore` from `FileConfig`
- Sub-storage constructors take `*FileStore` instead of `*BadgerDB`

### File: `internal/storage/badger.go` (DELETE)
- Entire file removed

### File: `internal/common/config.go` (MODIFY)
- Replace `BadgerConfig` with `FileConfig`
- Update `StorageConfig`, `NewDefaultConfig`, `applyEnvOverrides`

### File: `cmd/vire-mcp/main.go` (MODIFY)
- Update `config.Storage.Badger.Path` references to `config.Storage.File.Path`

### File: `cmd/vire-mcp/handlers.go` (MODIFY)
- Update config display from `storage.badger.path` to `storage.file.path` and add `storage.file.versions`

### Files: `internal/models/*.go` (MODIFY)
- Remove all `init()` functions with `gob.Register()` calls
- Remove `encoding/gob` imports
- Remove `badgerhold:"key"` and `badgerhold:"index"` struct tags

### Files: `config/vire.toml.example`, `tests/docker/vire.toml`, `docker/vire.toml` (MODIFY)
- Replace `[storage.badger]` with `[storage.file]` + `versions = 5`

### File: `go.mod` (MODIFY)
- Remove `github.com/timshannon/badgerhold/v4`
- Run `go mod tidy` to clean up transitive deps (badger/v4, ristretto, flatbuffers, etc.)

### Files: `internal/storage/*_test.go` (REWRITE)
- Tests now create `FileStore` with `t.TempDir()` instead of badgerhold stores
- Same test cases, different setup

---

## 5. Risk Assessment

| Risk | Mitigation |
|------|------------|
| Data loss during migration | Old BadgerDB data directory is untouched; file storage uses same base path but different structure |
| GCS FUSE compatibility | Using only `os.ReadFile`, `os.WriteFile`, `os.Rename` (same dir), `os.Remove`, `os.MkdirAll`, `os.ReadDir` -- all FUSE-safe |
| Concurrent write corruption | Atomic rename prevents partial reads; last-writer-wins is acceptable |
| Performance for scan operations | Portfolio size is small (10-50 tickers); scanning 50 JSON files is <10ms |
| Filename collisions | Key sanitization replaces unsafe chars; PID in temp filenames prevents write collisions |

## 6. Library Decision

**No external libraries needed.** The implementation uses only Go stdlib:
- `encoding/json` for serialization
- `os` for file operations
- `path/filepath` for path manipulation
- `sort` for list ordering
- `strings` for key sanitization

The afero library was considered but adds unnecessary abstraction -- we need real filesystem semantics (atomic rename) that virtual filesystems may not guarantee.
