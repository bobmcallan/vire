# Requirements: Refactor Data Layer to SurrealDB

**Date:** 2026-02-20
**Requested:** Refactor the entire data layer (BadgerHold and file-based JSON) to SurrealDB (currently on port 8000). Major refactor, no backward compatibility needed, all data moves to SurrealDB.

## Scope
- IN SCOPE: Replace `InternalStore` with a SurrealDB implementation.
- IN SCOPE: Replace `UserDataStore` with a SurrealDB implementation.
- IN SCOPE: Replace `MarketFS` with a SurrealDB implementation (`MarketDataStorage`, `SignalStorage`).
- IN SCOPE: Update `StorageManager` to connect to SurrealDB and provide these interfaces.
- IN SCOPE: Update tests/configuration to support SurrealDB.
- IN SCOPE: Remove old Badger/BadgerHold and File-based storage packages/dependencies.
- OUT OF SCOPE: Backward compatibility / data migration scripts (explicitly not needed).

## Approach
1.  **SurrealDB Connection**: Add a new configuration section `[database]` to configure the SurrealDB address, namespace, database name, and credentials.
2.  **Implementation**: Create a single package `internal/storage/surrealdb` that implements all interfaces (`StorageManager`, `InternalStore`, `UserDataStore`, `MarketDataStorage`, `SignalStorage`).
3.  **Data Structures**: 
    - SurrealDB uses `<table_name>:<id>` for record IDs. 
    - `InternalUser` -> `user` table. ID: `user:<username>` or `user:<user_id>`.
    - `UserKeyValue` -> `user_kv` table. ID: `user_kv:<user_id>_<key>`.
    - `SystemKV` -> `system_kv` table. ID: `system_kv:<key>`.
    - `UserRecord` -> `user_data` table. ID: `user_data:<user_id>_<subject>_<key>`. We can structure the JSON natively or keep the generic struct. Since SurrealDB is a document database, it is best to serialize the `Value` field into JSON string or raw nested object. We'll stick to the existing `UserRecord` format (where `Value []byte` or JSON string) to minimize changes to `internal/services/` serialization, OR we can store the raw object if it simplifies queries. Storing `Value` as string/bytes is safest for now.
    - `MarketData` -> `market_data` table. ID: `market_data:<ticker>`.
    - `TickerSignals` -> `signals` table. ID: `signals:<ticker>`.
4.  **Dependencies**: We added `github.com/surrealdb/surrealdb.go`. We will remove `github.com/timshannon/badgerhold/v4` and `github.com/dgraph-io/badger/v4`.

## Files Expected to Change
- `go.mod`, `go.sum`
- `config/vire-service.toml` & `internal/common/config.go`
- `internal/storage/manager.go` (Rewritten to init SurrealDB)
- `internal/storage/surrealdb/manager.go` (New)
- `internal/storage/surrealdb/internalstore.go` (New)
- `internal/storage/surrealdb/userstore.go` (New)
- `internal/storage/surrealdb/marketstore.go` (New)
- `internal/storage/internaldb/*` (Deleted)
- `internal/storage/userdb/*` (Deleted)
- `internal/storage/marketfs/*` (Deleted)
- `internal/storage/migrate.go` (Deleted or emptied)
- Relevant tests that initialized BadgerHold/MarketFS.
