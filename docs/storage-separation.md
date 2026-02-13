# Instruction: Separate Storage into UserStore and DataStore

## Goal

Split the storage layer into two distinct stores within `StorageManager`:

- **UserStore** — per-user data, encrypted at rest, versioned where applicable
- **DataStore** — shared reference data (market prices, signals), no encryption required, no per-user scoping

The separation happens at the storage layer inside vire-server. No new service is introduced. The existing single-user `docker-compose up` workflow must continue to work unchanged.

---

## Why

Market data (stock prices, fundamentals, technical signals) is identical regardless of which user requests it. BHP.AU's closing price is the same for everyone. This data should be fetched once, stored once, and read by all users. It has no privacy requirement and does not need encryption.

User data (portfolios, strategies, plans, watchlists) is personal. A user can have multiple portfolios, each with associated strategies, plans, and watchlists. This data must be scoped per-user and encrypted at rest.

The current `StorageManager` treats everything as a single store. The cloud deployment (documented in `docs/architecture-per-user-deployment.md`) requires multi-tenant operation where user data is isolated but market data is shared. This separation must exist in the storage layer to support that.

---

## Data Classification

### UserStore (per-user, encrypted, scoped to `users/{uid}/`)

| Domain | Storage Type | Key | Versioned | Notes |
|--------|-------------|-----|-----------|-------|
| Portfolios | `portfolioStorage` | portfolio name | Yes | A user can have multiple portfolios (e.g., "SMSF", "Personal") |
| Strategies | `strategyStorage` | portfolio name | Yes | One strategy per portfolio |
| Plans | `planStorage` | portfolio name | Yes | One plan per portfolio |
| Watchlists | `watchlistStorage` | portfolio name | Yes | One watchlist per portfolio |
| Reports | `reportStorage` | portfolio name | No | Cached analysis of a user's portfolio |
| Search History | `searchHistoryStorage` | search ID | No | Screen/snipe/funnel results, auto-pruned to 50 |
| KV Settings | `kvStorage` | key name | No | User preferences (default_portfolio, etc.) |

### DataStore (shared, unencrypted, scoped to `data/`)

| Domain | Storage Type | Key | Versioned | Notes |
|--------|-------------|-----|-----------|-------|
| Market Data | `marketDataStorage` | ticker symbol | No | OHLCV, fundamentals, fetched from EODHD |
| Signals | `signalStorage` | ticker symbol | No | Computed technical indicators (RSI, MACD, etc.) |
| Charts | via `WriteRaw` | filename | No | Generated PNG chart images |

---

## Current Architecture (what exists today)

### StorageManager (`internal/storage/manager.go`)

```go
type Manager struct {
    blob          BlobStore
    fs            *FileStore
    portfolio     interfaces.PortfolioStorage
    marketData    interfaces.MarketDataStorage
    signal        interfaces.SignalStorage
    kv            interfaces.KeyValueStorage
    report        interfaces.ReportStorage
    strategy      interfaces.StrategyStorage
    plan          interfaces.PlanStorage
    searchHistory interfaces.SearchHistoryStorage
    watchlist     interfaces.WatchlistStorage
    logger        *common.Logger
}
```

All domain storage types are constructed from a single `FileStore` in `NewStorageManager()` (line 36). Every domain type holds a reference to `*FileStore` and a subdirectory name.

### Domain storage types (`internal/storage/file.go`)

All nine domain types follow the same pattern:

```go
type portfolioStorage struct {
    fs     *FileStore
    dir    string           // e.g., "{basePath}/portfolios"
    logger *common.Logger
}
```

They call `fs.readJSON()` / `fs.writeJSON()` for persistence. The `writeJSON` method accepts a `versioned bool` parameter — user-authored data passes `true`, derived data passes `false`.

### FileStore (`internal/storage/file.go`)

Creates subdirectories on init: `portfolios`, `market`, `signals`, `reports`, `strategies`, `plans`, `watchlists`, `searches`, `kv`, `charts`.

### BlobStore (`internal/storage/blob.go`)

Interface with 9 methods: `Get`, `GetReader`, `Put`, `PutReader`, `Delete`, `Exists`, `Metadata`, `List`, `Close`. Only `FileBlobStore` is implemented. GCS/S3 return "not yet implemented" in the factory (`internal/storage/factory.go`).

### Config (`internal/common/config.go`)

```go
type StorageConfig struct {
    Backend string     // "file", "gcs", "s3"
    File    FileConfig
    GCS     GCSConfig
    S3      S3Config
}
```

### Interfaces (`internal/interfaces/storage.go`)

`StorageManager` interface (line 11) exposes accessors for all nine domain storage types plus `PurgeDerivedData()`, `PurgeReports()`, `WriteRaw()`, `DataPath()`, `Close()`.

### App (`internal/app/app.go`)

`App.Storage` (line 33) holds the `interfaces.StorageManager`. Services are constructed with the full storage manager:

```go
marketService := market.NewService(storageManager, eodhdClient, geminiClient, logger)
portfolioService := portfolio.NewService(storageManager, navexaClient, eodhdClient, geminiClient, logger)
```

Services access the storage types they need via `storageManager.MarketDataStorage()`, `storageManager.PortfolioStorage()`, etc.

---

## Required Changes

### 1. Split StorageManager into two backing stores

Modify `internal/storage/manager.go`:

```go
type Manager struct {
    userStore     *FileStore   // Per-user data (portfolios, strategies, plans, etc.)
    dataStore     *FileStore   // Shared data (market, signals, charts)

    // Domain storage — each backed by the appropriate store
    portfolio     interfaces.PortfolioStorage      // → userStore
    strategy      interfaces.StrategyStorage       // → userStore
    plan          interfaces.PlanStorage            // → userStore
    watchlist     interfaces.WatchlistStorage       // → userStore
    report        interfaces.ReportStorage          // → userStore
    searchHistory interfaces.SearchHistoryStorage   // → userStore
    kv            interfaces.KeyValueStorage        // → userStore

    marketData    interfaces.MarketDataStorage      // → dataStore
    signal        interfaces.SignalStorage           // → dataStore

    logger        *common.Logger
}
```

### 2. Update StorageConfig

Modify `internal/common/config.go` — the storage config needs two paths:

```go
type StorageConfig struct {
    Backend  string     // "file", "gcs", "s3"
    UserData FileConfig // Per-user encrypted data
    Data     FileConfig // Shared reference data
    GCS      GCSConfig
    S3       S3Config
}
```

Rename the existing `File` field to `UserData` and add `Data`. Update `NewDefaultConfig()`:

```go
Storage: StorageConfig{
    Backend: "file",
    UserData: FileConfig{
        Path:     "data/user",
        Versions: 5,
    },
    Data: FileConfig{
        Path:     "data/data",
        Versions: 0,
    },
},
```

### 3. Update FileStore subdirectory creation

The current `FileStore` creates all subdirectories in a flat list. Each `FileStore` instance should only create the subdirectories relevant to its role.

Option A (simpler): Let each `FileStore` create whatever subdirectories its domain types need — the domain types already pass the subdirectory name when calling `readJSON`/`writeJSON`, so no change is needed to `FileStore` itself. Just construct two instances with different base paths.

Option B: Pass the subdirectory list to the `FileStore` constructor.

Option A is recommended. The domain storage constructors (`newPortfolioStorage`, etc.) already call `filepath.Join(fs.basePath, "portfolios")` to set their `dir` field. Two `FileStore` instances with different `basePath` values will naturally separate the data.

Remove the hardcoded `subdirectories` slice from `NewFileStore()`. Instead, have each domain storage constructor create its own subdirectory via `os.MkdirAll` in its `new*Storage()` function if it doesn't exist.

### 4. Update NewStorageManager constructor

```go
func NewStorageManager(logger *common.Logger, config *common.Config) (interfaces.StorageManager, error) {
    userStore, err := NewFileStore(logger, &config.Storage.UserData)
    if err != nil {
        return nil, fmt.Errorf("user store: %w", err)
    }

    dataStore, err := NewFileStore(logger, &config.Storage.Data)
    if err != nil {
        return nil, fmt.Errorf("data store: %w", err)
    }

    manager := &Manager{
        userStore:     userStore,
        dataStore:     dataStore,

        // User data — backed by userStore
        portfolio:     newPortfolioStorage(userStore, logger),
        strategy:      newStrategyStorage(userStore, logger),
        plan:          newPlanStorage(userStore, logger),
        watchlist:     newWatchlistStorage(userStore, logger),
        report:        newReportStorage(userStore, logger),
        searchHistory: newSearchHistoryStorage(userStore, logger),
        kv:            newKVStorage(userStore, logger),

        // Shared data — backed by dataStore
        marketData:    newMarketDataStorage(dataStore, logger),
        signal:        newSignalStorage(dataStore, logger),

        logger:        logger,
    }

    return manager, nil
}
```

### 5. Update PurgeDerivedData

Modify `PurgeDerivedData()` in `internal/storage/manager.go` (line 141). Currently it purges across a single store. After the split:

- Purging user derived data (reports, searches): operates on `userStore`
- Purging shared derived data (market, signals, charts): operates on `dataStore`

Review what "purge derived data" means with two stores. The current implementation deletes portfolios, market data, signals, reports, search history, and charts. With the split:

- Portfolios are user-authored (versioned) — should purge logic still delete them? This is used for re-sync from Navexa. Keep this behaviour but scope it to `userStore`.
- Market/signals/charts are in `dataStore` — purge those from `dataStore`.

### 6. Update WriteRaw

`WriteRaw(subdir, key, data)` is currently used for charts. Charts are in the data store. Update to route through `dataStore`:

```go
func (m *Manager) WriteRaw(subdir, key string, data []byte) error {
    return m.dataStore.WriteRaw(subdir, key, data)
}
```

If user-scoped raw writes are needed later (e.g., user-specific charts), add a `WriteRawUser` method or accept a store selector.

### 7. Update DataPath

`DataPath()` currently returns the single base path. With two stores, decide what this should return. Options:

- Return `userStore.basePath` (most callers want the user data path)
- Add `UserDataPath()` and `DataPath()` separately
- Audit all callers and route appropriately

Audit callers first. If `DataPath()` is only used for chart output paths, it should return `dataStore.basePath`.

### 8. Update Close

Close both stores:

```go
func (m *Manager) Close() error {
    var errs []error
    if err := m.userStore.Close(); err != nil {
        errs = append(errs, err)
    }
    if err := m.dataStore.Close(); err != nil {
        errs = append(errs, err)
    }
    return errors.Join(errs...)
}
```

### 9. Remove the `blob` and `fs` fields from Manager

The current `Manager` holds both `blob BlobStore` and `fs *FileStore`. The `blob` field was intended for future cloud storage. With the two-store split, the `BlobStore` interface is not yet needed — both stores use `FileStore`. When GCS is implemented later, `userStore` and `dataStore` will each get their own `BlobStore` implementation (GCS with encryption for user data, GCS without for shared data).

Remove the `blob` field. Remove the `BlobStore()` accessor from the `StorageManager` interface. Keep the `BlobStore` interface itself — it will be used when GCS is implemented.

### 10. Update TOML config

Update `docker/vire.toml` (and any other config files) to reflect the new structure:

```toml
[storage]
backend = "file"

[storage.user_data]
path = "data/user"
versions = 5

[storage.data]
path = "data/data"
versions = 0
```

### 11. Backwards compatibility for single-user local mode

When both `UserData.Path` and `Data.Path` are empty or not configured, fall back to the current behaviour: use a single path (`data/`) for everything. This is a config-level concern — `NewStorageManager` can detect the legacy config and construct both stores pointing at the same base path.

Alternatively, just update the default config and Docker compose to use the new paths. Since this is a breaking change to the data directory layout, provide a migration note or script that moves files from the old flat layout to the new split layout.

### 12. Data migration

Existing single-user deployments have data in:

```
data/
├── portfolios/
├── market/
├── signals/
├── reports/
├── strategies/
├── plans/
├── watchlists/
├── searches/
├── kv/
└── charts/
```

After the change, data lives in:

```
data/
├── user/
│   ├── portfolios/
│   ├── strategies/
│   ├── plans/
│   ├── watchlists/
│   ├── reports/
│   ├── searches/
│   └── kv/
└── data/
    ├── market/
    ├── signals/
    └── charts/
```

Write a migration function in `internal/storage/migrate.go` that:

1. Detects the old flat layout (checks if `data/portfolios/` exists at the old location)
2. Creates the new directory structure
3. Moves files to the correct store
4. Logs what was moved
5. Runs automatically on startup if the old layout is detected

### 13. Update StorageManager interface

Modify `internal/interfaces/storage.go`:

- Remove `BlobStore() BlobStore` if it exists as an accessor
- Keep all domain storage accessors unchanged — callers don't need to know about the split
- Consider adding `UserDataPath()` and `DataPath()` if both are needed by callers

### 14. Update tests

Update `internal/storage/file_test.go` and `internal/storage/blob_test.go`:

- Tests should construct two separate `FileStore` instances
- Verify that user data and shared data are written to different directories
- Verify that the migration function works correctly

---

## What NOT to Change

- **Domain storage types** (`portfolioStorage`, `marketDataStorage`, etc.) — their internal logic stays the same. They already accept a `*FileStore` and a directory name. The only change is which `FileStore` instance they receive.
- **Service layer** (`internal/services/`) — services access storage through `StorageManager` accessors. The split is invisible to them.
- **Handlers** (`internal/server/`) — handlers access storage through `s.app.Storage`. No changes needed.
- **MCP proxy** (`cmd/vire-mcp/`) — no storage changes, it's stateless.
- **BlobStore interface** — keep it for future GCS/S3 implementation, but don't wire it into the new split yet.

---

## Verification

After implementation:

1. `docker-compose up` works with the new default config
2. Existing data is migrated automatically on first startup
3. User data (portfolios, strategies, plans, watchlists, reports, searches, kv) is written under `data/user/`
4. Shared data (market, signals, charts) is written under `data/data/`
5. All existing tests pass
6. A new test verifies the two-store separation: saving a portfolio writes to `userStore`, saving market data writes to `dataStore`
7. `PurgeDerivedData` clears the correct data from the correct store
