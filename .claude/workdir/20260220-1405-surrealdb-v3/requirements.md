# Requirements: Update SurrealDB to v3.0.0

**Date:** 2026-02-20
**Requested:** Update SurrealDB version from v2.2.1 to v3.0.0

## Scope

**In scope:**
- Update Docker image references from `surrealdb/surrealdb:v2.2.1` to `surrealdb/surrealdb:v3.0.0`
- Add DEFINE TABLE IF NOT EXISTS for all 6 tables at manager init (v3 errors on querying non-existent tables)
- Verify/update test container startup log wait strategy for v3
- Run all unit and stress tests against v3
- Update vire-infra docker/vire-stack.yml
- Update test docker-compose.yml

**Out of scope:**
- Go SDK version change (v1.3.0 already supports SurrealDB v3)
- Query syntax changes (UPSERT, DELETE RETURN BEFORE, SELECT WHERE all work in v3)
- New v3 features (sessions, interactive transactions, bearer access)

## Approach

### 1. Non-existent table handling (CRITICAL)
SurrealDB v3 returns errors instead of empty arrays when querying non-existent tables. Vire relies on tables being auto-created by the first UPSERT, but SELECT queries on a fresh database would fail before any writes.

**Solution:** Add table definitions after connecting in `NewManager`:
```surql
DEFINE TABLE IF NOT EXISTS user SCHEMALESS;
DEFINE TABLE IF NOT EXISTS user_kv SCHEMALESS;
DEFINE TABLE IF NOT EXISTS system_kv SCHEMALESS;
DEFINE TABLE IF NOT EXISTS user_data SCHEMALESS;
DEFINE TABLE IF NOT EXISTS market_data SCHEMALESS;
DEFINE TABLE IF NOT EXISTS signals SCHEMALESS;
```

### 2. Docker image updates
Three locations:
- `tests/common/surrealdb.go:36` — `"surrealdb/surrealdb:v2.2.1"` → `"surrealdb/surrealdb:v3.0.0"`
- `tests/docker/docker-compose.yml:5` — `surrealdb/surrealdb:v2.2.1` → `surrealdb/surrealdb:v3.0.0`
- vire-infra `docker/vire-stack.yml:61` — `surrealdb/surrealdb:v2.2.1` → `surrealdb/surrealdb:v3.0.0`

### 3. Test container startup
The wait strategy `wait.ForLog("Started web server")` may differ in v3. Need to verify.

### 4. Validation
Run all tests: unit, stress, data, and API tests.

## Files Expected to Change

| File | Change |
|------|--------|
| `internal/storage/surrealdb/manager.go` | Add DEFINE TABLE statements after connection |
| `tests/common/surrealdb.go` | Update Docker image to v3.0.0 |
| `tests/docker/docker-compose.yml` | Update image to v3.0.0 |
| vire-infra `docker/vire-stack.yml` | Update image to v3.0.0 |

## Risk Assessment

- **Low risk**: Go SDK v1.3.0 already supports v3, query syntax is compatible
- **Medium risk**: Non-existent table errors need DEFINE TABLE mitigation
- **Low risk**: `surrealkv://` storage backend still supported in v3
- **Low risk**: `start --user root --pass root` command syntax unchanged
