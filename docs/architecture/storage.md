# Storage Architecture

3-area layout with separate backends per concern.

## Stores

| Store | Backend | Package | Path | Contents |
|-------|---------|---------|------|----------|
| InternalStore | SurrealDB | `internal/storage/surrealdb/internalstore.go` | SurrealDB | User accounts, per-user config KV, system KV |
| UserDataStore | SurrealDB | `internal/storage/surrealdb/` | SurrealDB | Generic UserRecord — all user domain data |
| MarketFS | File-based JSON | `internal/storage/marketfs/` | `data/market/` | Market data, signals, charts |

## InternalStore

SurrealDB-backed user store keyed by user_id. Stores `InternalUser` (user_id, email, name, password_hash, provider, role, created_at, modified_at) and `UserKeyValue` (user_id, key, value, version, datetime).

Interface: `GetUser`, `GetUserByEmail`, `SaveUser`, `DeleteUser`, `ListUsers`, `GetUserKV`, `SetUserKV`, `DeleteUserKV`, `ListUserKV`, `GetSystemKV`, `SetSystemKV`.

`GetUserByEmail` performs case-insensitive email lookup and rejects empty email input.

## UserDataStore

Generic `UserRecord` (user_id, subject, key, value, version, datetime). Services marshal/unmarshal domain types to/from the `value` field as JSON.

Interface: `Get`, `Put`, `Delete`, `List`, `Query`, `DeleteBySubject`.

Subjects: `portfolio`, `strategy`, `plan`, `watchlist`, `report`, `search`, `cashflow`.

## MarketFS

File-based JSON with atomic writes (temp file + rename). Implements `MarketDataStorage` and `SignalStorage` interfaces.

## StockIndexStore

SurrealDB-backed registry of all tracked stocks (`internal/storage/surrealdb/stockindex.go`). Each `StockIndexEntry` has ticker, code, exchange, name, source, and per-component freshness timestamps.

Interface: `Upsert`, `Get`, `List`, `UpdateTimestamp`, `Delete`. Upsert preserves existing timestamps. Ticker dots replaced with underscores for record IDs via `tickerToID()`.

## JobQueueStore

Persistent priority job queue (`internal/storage/surrealdb/jobqueue.go`). Atomic dequeue via `UPDATE ... WHERE status = 'pending' ORDER BY priority DESC, created_at ASC LIMIT 1 RETURN AFTER`.

Interface: `Enqueue`, `Dequeue`, `Complete`, `Cancel`, `SetPriority`, `GetMaxPriority`, `ListPending`, `ListAll`, `ListByTicker`, `CountPending`, `HasPendingJob`, `PurgeCompleted`, `CancelByTicker`.

## FeedbackStore

SurrealDB-backed (`internal/storage/surrealdb/feedbackstore.go`). Uses explicit `feedback_id` field with `SELECT feedback_id as id` aliasing.

Categories: data_anomaly, sync_delay, calculation_error, missing_data, schema_change, tool_error, observation.

Feedback records carry identity fields set from the authenticated `UserContext` at request time (not from the request body):
- `user_id`, `user_name`, `user_email` — identity of the user who submitted the feedback (set on `Create`)
- `updated_by_user_id`, `updated_by_user_name`, `updated_by_user_email` — identity of the user who last updated the feedback (set on `Update`)

`FeedbackStore.Update()` accepts user identity parameters directly; handlers extract them from `common.UserContextFromContext(r.Context())` and look up name/email via `InternalStore.GetUser()`.

## OAuthStore

SurrealDB-backed (`internal/storage/surrealdb/oauthstore.go`). Tables: `oauth_client`, `oauth_code`, `oauth_refresh_token`. Client secrets bcrypt-hashed. Refresh tokens stored as SHA-256 hashes.

## Migration

On first startup, `MigrateOldLayout` reads from old single-BadgerDB layout and splits into the 3-area layout. Old directories renamed to `.migrated-{timestamp}`.

## Adding New Data

- **User domain data:** `UserDataStore.Put` with a new `subject` string. No new storage files needed.
- **Market/signal data:** Follow `MarketFS` pattern — file-based JSON with `FileStore` wrapper.

## Schema Version

`SchemaVersion` in `internal/common/version.go`. Bumped when model changes invalidate cached data. Portfolio records include `DataVersion`; stale versions trigger re-sync.
