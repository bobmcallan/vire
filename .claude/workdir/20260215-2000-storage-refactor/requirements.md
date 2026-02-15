# Requirements: Refactor storage into 3 areas — internal, user, market

**Date:** 2026-02-15
**Requested:** Refactor storage into 3 separate areas (internal/BadgerDB, user/BadgerDB, market/FileStore). Simplify user data into a single generic table: `{userid, subject, key, value, version, datetime}`. Split current User model — auth in internal, preferences as key-value pairs.

## Scope

### In scope
- 3 storage areas with separate directories and backends
- Generic user data table replacing 6+ typed domain stores
- Internal store: users (auth/identity) + user_key_value (per-user config) + system KV
- Market store: file-based (current data store, renamed)
- Migration from current BadgerDB + FileStore layout
- Update all consumers (services, handlers, middleware, app)
- Update interfaces

### Out of scope
- Multi-tenant user isolation (user_id field is structural, not enforced)
- Service logic changes (version increment, validation stay in services)
- New API endpoints
- Cloud backends (GCS/S3)

## Approach

### Architecture: 3 Storage Areas

```
data/
├── internal/    # BadgerDB — auth, user config, system config
├── user/        # BadgerDB — generic document table for all user domain data
└── market/      # FileStore — market data, signals, charts
```

### 1. Internal Store — `data/internal/` (BadgerDB)

Two models plus system KV:

**InternalUser** — user accounts/identity:
```go
type InternalUser struct {
    UserID       string    `json:"user_id"`      // primary key
    Email        string    `json:"email"`
    PasswordHash string    `json:"password_hash"` // bcrypt
    Role         string    `json:"role"`
    CreatedAt    time.Time `json:"created_at"`
    ModifiedAt   time.Time `json:"modified_at"`
}
```
BadgerHold key: `user_id`

**UserKeyValue** — per-user configuration:
```go
type UserKeyValue struct {
    UserID   string    `json:"user_id"`
    Key      string    `json:"key"`       // "navexa_key", "display_currency", "default_portfolio", "portfolios"
    Value    string    `json:"value"`     // string value (JSON for complex types like portfolios)
    Version  int       `json:"version"`
    DateTime time.Time `json:"datetime"`
}
```
BadgerHold key: `user_id:key` (composite)

**System KV** — system-level config (schema version, API keys):
Uses same KVEntry pattern but with no user_id (or user_id="system"). Keys: `vire_schema_version`, `vire_build_timestamp`, `eodhd_api_key`, `gemini_api_key`, `default_portfolio`.

### 2. User Data Store — `data/user/` (BadgerDB)

Single generic table replaces 6 domain stores (portfolio, strategy, plan, watchlist, report, search):

**UserRecord** — universal document:
```go
type UserRecord struct {
    UserID   string    `json:"user_id"`   // user who owns this data
    Subject  string    `json:"subject"`   // "portfolio", "strategy", "plan", "watchlist", "report", "search"
    Key      string    `json:"key"`       // portfolio name, search ID, etc.
    Value    string    `json:"value"`     // JSON-serialized domain object
    Version  int       `json:"version"`   // auto-incremented by service layer
    DateTime time.Time `json:"datetime"`  // last modified
}
```
BadgerHold key: `user_id:subject:key` (composite, e.g. `dev_user:portfolio:SMSF`)

**Subjects and their key patterns:**

| Subject | Key | Value (JSON) |
|---------|-----|-------------|
| `portfolio` | portfolio name (e.g., "SMSF") | Full Portfolio struct |
| `strategy` | portfolio name | Full PortfolioStrategy struct |
| `plan` | portfolio name | Full PortfolioPlan struct |
| `watchlist` | portfolio name | Full PortfolioWatchlist struct |
| `report` | portfolio name | Full PortfolioReport struct |
| `search` | search ID (auto-generated) | Full SearchRecord struct |

### 3. Market Store — `data/market/` (FileStore)

Renamed from `data/data/`. Unchanged backend. Contains:
- `market/` — ticker JSON files (MarketData)
- `signals/` — signal JSON files (TickerSignals)
- `charts/` — chart images (binary)

### Package Structure

```
internal/storage/
├── internaldb/         # NEW: data/internal/ — BadgerDB
│   ├── store.go        # Store struct + InternalUser CRUD + UserKeyValue CRUD + System KV
│   └── store_test.go
├── userdb/             # NEW: data/user/ — BadgerDB
│   ├── store.go        # Store struct + UserRecord CRUD + query/purge
│   └── store_test.go
├── marketfs/           # REFACTORED: data/market/ — FileStore
│   ├── store.go        # FileStore base + MarketDataStorage + SignalStorage + WriteRaw
│   └── store_test.go
├── manager.go          # Wires all 3 stores, adapts to domain interfaces
├── migrate.go          # Migration from old layout
├── badger/             # DELETED entirely (replaced by internaldb + userdb)
├── file.go             # DELETED (split into marketfs/ and base methods moved)
```

### Interface Changes — `internal/interfaces/storage.go`

**New interfaces:**

```go
// InternalStore manages user accounts and per-user config
type InternalStore interface {
    // User accounts
    GetUser(ctx context.Context, userID string) (*models.InternalUser, error)
    SaveUser(ctx context.Context, user *models.InternalUser) error
    DeleteUser(ctx context.Context, userID string) error
    ListUsers(ctx context.Context) ([]string, error)

    // Per-user key-value config
    GetUserKV(ctx context.Context, userID, key string) (*models.UserKeyValue, error)
    SetUserKV(ctx context.Context, userID, key, value string) error
    DeleteUserKV(ctx context.Context, userID, key string) error
    ListUserKV(ctx context.Context, userID string) ([]*models.UserKeyValue, error)

    // System key-value (non-user-scoped)
    GetSystemKV(ctx context.Context, key string) (string, error)
    SetSystemKV(ctx context.Context, key, value string) error

    Close() error
}

// UserDataStore manages all user domain data via generic records
type UserDataStore interface {
    Get(ctx context.Context, userID, subject, key string) (*models.UserRecord, error)
    Put(ctx context.Context, record *models.UserRecord) error
    Delete(ctx context.Context, userID, subject, key string) error
    List(ctx context.Context, userID, subject string) ([]*models.UserRecord, error)
    Query(ctx context.Context, userID, subject string, opts QueryOptions) ([]*models.UserRecord, error)
    DeleteBySubject(ctx context.Context, subject string) (int, error) // for purge
    Close() error
}

type QueryOptions struct {
    Limit   int
    OrderBy string // "datetime_desc" (default), "datetime_asc"
}
```

**Removed interfaces:** `UserStorage`, `PortfolioStorage`, `StrategyStorage`, `PlanStorage`, `WatchlistStorage`, `ReportStorage`, `SearchHistoryStorage`, `KeyValueStorage`

**Kept interfaces:** `MarketDataStorage`, `SignalStorage` (unchanged, backed by marketfs)

**StorageManager changes:**
```go
type StorageManager interface {
    InternalStore() InternalStore
    UserDataStore() UserDataStore
    MarketDataStorage() MarketDataStorage
    SignalStorage() SignalStorage
    DataPath() string
    WriteRaw(subdir, key string, data []byte) error
    PurgeDerivedData(ctx context.Context) (map[string]int, error)
    PurgeReports(ctx context.Context) (int, error)
    Close() error
}
```

### Model Changes — `internal/models/`

**New models** (in `internal/models/storage.go`):
- `InternalUser` — user identity (fields listed above)
- `UserKeyValue` — per-user config pair
- `UserRecord` — generic document record

**Removed from `user.go`**: The current `User` struct is replaced by `InternalUser` + `UserKeyValue` entries. Delete `internal/models/user.go`.

### Service Changes

Services that use typed storage interfaces change to use `UserDataStore` directly. Each service handles its own JSON serialization:

```go
// Example: portfolio service
func (s *PortfolioService) GetPortfolio(ctx context.Context, name string) (*models.Portfolio, error) {
    userID := common.ResolveUserID(ctx)
    record, err := s.userStore.Get(ctx, userID, "portfolio", name)
    if err != nil { return nil, err }
    var portfolio models.Portfolio
    if err := json.Unmarshal([]byte(record.Value), &portfolio); err != nil {
        return nil, fmt.Errorf("failed to deserialize portfolio: %w", err)
    }
    return &portfolio, nil
}

func (s *PortfolioService) SavePortfolio(ctx context.Context, portfolio *models.Portfolio) error {
    data, _ := json.Marshal(portfolio)
    userID := common.ResolveUserID(ctx)
    return s.userStore.Put(ctx, &models.UserRecord{
        UserID:   userID,
        Subject:  "portfolio",
        Key:      portfolio.Name,
        Value:    string(data),
        Version:  1,
        DateTime: time.Now(),
    })
}
```

**Services that change:**
- `portfolio/service.go` — use UserDataStore for portfolio CRUD
- `strategy/service.go` — use UserDataStore, keep version increment logic
- `plan/service.go` — use UserDataStore, keep version increment logic
- `watchlist/service.go` — use UserDataStore
- `report/service.go` — use UserDataStore
- `market/service.go` — search save uses UserDataStore (subject="search")

**Helper function** in `internal/common/userctx.go`:
```go
func ResolveUserID(ctx context.Context) string {
    if uc := GetUserContext(ctx); uc != nil && uc.UserID != "" {
        return uc.UserID
    }
    return "default"
}
```

### Handler Changes — `internal/server/handlers_user.go`

User CRUD handlers change to use InternalStore:
- `handleUserCreate` → `InternalStore().SaveUser()` + `SetUserKV()` for preferences
- `handleUserGet` → `InternalStore().GetUser()` + `ListUserKV()` for full profile
- `handleUserUpdate` → `InternalStore().GetUser()` + `SetUserKV()` for changed prefs
- `handleUserDelete` → `InternalStore().DeleteUser()` + delete all UserKV entries
- `handleAuthLogin` → `InternalStore().GetUser()` (password) + `ListUserKV()` (prefs for response)
- `handleUserImport` → both InternalStore operations

The `userResponse()` helper reconstructs the composite response from InternalUser + UserKeyValue entries.

### Middleware Changes — `internal/server/middleware.go`

`userContextMiddleware` changes from `UserStorage.GetUser()` to:
1. `InternalStore().GetUser(ctx, userID)` — verify user exists
2. `InternalStore().ListUserKV(ctx, userID)` — load all preferences
3. Build UserContext from KV entries (navexa_key, display_currency, portfolios)
4. Individual X-Vire-* headers still override

### App Startup Changes — `internal/app/app.go`

- Schema version check: `InternalStore().GetSystemKV(ctx, "vire_schema_version")`
- Build timestamp: `InternalStore().SetSystemKV(ctx, "vire_build_timestamp", ...)`
- API key resolution: `InternalStore().GetSystemKV(ctx, "eodhd_api_key")`
- Dev mode import: `ImportUsersFromFile()` uses InternalStore instead of UserStorage

### Config Changes — `internal/common/config.go`

```go
type StorageConfig struct {
    Internal FileConfig `toml:"internal"` // data/internal (BadgerDB)
    User     FileConfig `toml:"user"`     // data/user (BadgerDB)
    Market   FileConfig `toml:"market"`   // data/market (FileStore)
}
```

Default paths: `data/internal`, `data/user`, `data/market`. FileConfig just needs the `Path` field (Versions only relevant for market store).

### Purge Operations

- `PurgeDerivedData()`: delete by subject — `UserDataStore.DeleteBySubject("portfolio")`, `DeleteBySubject("report")`, `DeleteBySubject("search")` + file purge for market/signals/charts
- `PurgeReports()`: `UserDataStore.DeleteBySubject("report")`
- Preserved across purge: strategy, plan, watchlist (not included in purge)

### Migration — `internal/storage/migrate.go`

From current layout (`data/user/badger/` + `data/data/`) to new layout (`data/internal/` + `data/user/` + `data/market/`):

1. Read all records from old BadgerDB at `data/user/badger/`
2. Split by type:
   - Users → InternalStore.SaveUser() + SetUserKV() for preferences
   - KV entries → InternalStore.SetSystemKV() / SetUserKV()
   - All others → UserDataStore.Put() with appropriate subject
3. Move `data/data/market/` → `data/market/market/`
4. Move `data/data/signals/` → `data/market/signals/`
5. Move `data/data/charts/` → `data/market/charts/`
6. Rename old directories to `.migrated-{timestamp}/`

## Files Expected to Change

### New files
- `internal/models/storage.go` — InternalUser, UserKeyValue, UserRecord models
- `internal/storage/internaldb/store.go` — Internal store implementation
- `internal/storage/internaldb/store_test.go`
- `internal/storage/userdb/store.go` — User data store implementation
- `internal/storage/userdb/store_test.go`
- `internal/storage/marketfs/store.go` — Market store implementation
- `internal/storage/marketfs/store_test.go`
- `internal/storage/migrate.go` — Migration logic (rewritten)

### Modified files
- `internal/interfaces/storage.go` — replace 8 domain interfaces with InternalStore + UserDataStore
- `internal/storage/manager.go` — wire 3 new stores
- `internal/common/config.go` — 3-area StorageConfig
- `internal/common/userctx.go` — add ResolveUserID helper
- `internal/server/middleware.go` — use InternalStore for user resolution
- `internal/server/handlers_user.go` — use InternalStore for user CRUD
- `internal/server/handlers.go` — use UserDataStore for domain operations
- `internal/server/server.go` — pass InternalStore to middleware
- `internal/server/routes.go` — update handler constructors if needed
- `internal/app/app.go` — use InternalStore for system KV + imports
- `internal/app/import.go` — use InternalStore instead of UserStorage
- `internal/services/portfolio/service.go` — use UserDataStore
- `internal/services/strategy/service.go` — use UserDataStore
- `internal/services/plan/service.go` — use UserDataStore
- `internal/services/watchlist/service.go` — use UserDataStore
- `internal/services/report/service.go` — use UserDataStore
- `internal/services/market/service.go` — use UserDataStore for search history
- `config/vire-service.toml.example` — 3-area config
- `README.md` — storage architecture
- `.claude/skills/develop/SKILL.md` — storage docs

### Deleted files
- `internal/storage/badger/` — entire directory (10 files)
- `internal/storage/file.go` — replaced by marketfs/store.go
- `internal/models/user.go` — replaced by storage.go models
- Various test files that move to new packages
