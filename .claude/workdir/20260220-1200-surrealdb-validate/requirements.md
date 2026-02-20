# Requirements: Validate SurrealDB Refactor & Implement Tests

**Date:** 2026-02-20
**Requested:** Validate the BadgerHold-to-SurrealDB refactor, implement unit tests for the storage layer, and add data/API integration tests with SurrealDB Docker support.

## Scope

### In Scope
- Validate the SurrealDB refactor compiles and is structurally sound
- Fix stale test config (`tests/docker/vire-blank.toml` still references BadgerDB)
- Create missing `tests/docker/vire.toml` (referenced by Dockerfile but doesn't exist)
- Add SurrealDB container to test Docker infrastructure
- Write unit tests for `internal/storage/surrealdb/` (internalstore, userstore, marketstore, manager)
- Create `tests/data/` with data layer integration tests
- Update `tests/api/` with API tests that use the built binary from `./bin/`
- Update test infrastructure (`tests/common/containers.go`) to support SurrealDB
- Update test-* skill files to reflect new patterns
- Create test Docker Compose with SurrealDB service

### Out of Scope
- Changing the storage implementation itself (only testing it)
- Adding new features
- GCS/S3 storage backends

## Issues Found During Investigation

1. **SurrealDB dependency is `indirect`** — `go.mod` has `github.com/surrealdb/surrealdb.go v1.3.0 // indirect` but it's used directly. Needs `go mod tidy`.
2. **Test config stale** — `tests/docker/vire-blank.toml` still has BadgerDB config (`backend = "badger"`, `storage.user_data.path`). Must be updated to SurrealDB config.
3. **Missing `tests/docker/vire.toml`** — Referenced in `tests/docker/Dockerfile.server` line 23 (`COPY tests/docker/vire.toml`) but file doesn't exist.
4. **No SurrealDB in test Docker** — `tests/docker/docker-compose.yml` only has vire-server, no SurrealDB service.
5. **No unit tests** — Old BadgerHold tests were deleted in commit `5a089fc` but no SurrealDB tests were added.
6. **No `tests/data/` directory** — Doesn't exist yet.
7. **Test infrastructure lacks multi-container support** — `containers.go` only manages one container (vire-server). Needs SurrealDB container support.
8. **Skill files outdated** — `test-common/SKILL.md` references `SetupDockerTestEnvironment` which doesn't match actual code (`NewEnv`).

## Approach

### Test Architecture

```
tests/
├── api/                     # Integration tests (HTTP → built binary → SurrealDB)
│   ├── health_test.go       # Health/version endpoint tests
│   ├── user_test.go         # User CRUD API tests
│   └── auth_test.go         # Auth API tests
├── data/                    # Data layer tests (Go → SurrealDB directly)
│   ├── internalstore_test.go
│   ├── userstore_test.go
│   └── marketstore_test.go
├── common/
│   ├── containers.go        # Updated: SurrealDB container + binary runner
│   └── surrealdb.go         # NEW: SurrealDB test helper (container setup)
├── docker/
│   ├── docker-compose.yml   # Updated: add SurrealDB service
│   ├── Dockerfile.server    # Updated: SurrealDB config
│   ├── vire.toml            # NEW: Full test config with SurrealDB
│   └── vire-blank.toml      # Updated: SurrealDB config (no API keys)
└── fixtures/                # Test data
```

### Unit Tests (`internal/storage/surrealdb/`)
- Direct tests of each store against a real SurrealDB via testcontainers
- Table-driven tests for all CRUD operations
- Error cases (not found, duplicate, invalid input)
- Tests for retry logic, purge operations

### Data Tests (`tests/data/`)
- Higher-level storage integration tests
- Test through `interfaces.StorageManager`
- Cross-store operations (e.g., purge derived data)
- Concurrent access patterns

### API Tests (`tests/api/`)
- Build binary to `./bin/`, start SurrealDB in Docker, run binary against it
- Test user CRUD, auth, health endpoints
- Use existing `TestOutputGuard` pattern

### SurrealDB Test Infrastructure
- Use `testcontainers-go` to spin up `surrealdb/surrealdb:latest`
- Expose port 8000 (WebSocket RPC)
- Wait for health check before tests run
- Isolated namespace per test to avoid cross-contamination
- Shared SurrealDB container across test suite (per package)

## Files Expected to Change

### New Files
- `tests/common/surrealdb.go` — SurrealDB container helper
- `tests/docker/vire.toml` — Test config with SurrealDB
- `tests/data/internalstore_test.go` — InternalStore data tests
- `tests/data/userstore_test.go` — UserStore data tests
- `tests/data/marketstore_test.go` — MarketStore data tests
- `tests/api/health_test.go` — Health/version API tests
- `tests/api/user_test.go` — User CRUD API tests
- `internal/storage/surrealdb/internalstore_test.go` — Unit tests
- `internal/storage/surrealdb/userstore_test.go` — Unit tests
- `internal/storage/surrealdb/marketstore_test.go` — Unit tests
- `internal/storage/surrealdb/manager_test.go` — Unit tests

### Modified Files
- `tests/docker/vire-blank.toml` — Update to SurrealDB config
- `tests/docker/docker-compose.yml` — Add SurrealDB service
- `tests/docker/Dockerfile.server` — Update for SurrealDB
- `tests/common/containers.go` — Add SurrealDB support, binary runner
- `.claude/skills/test-common/SKILL.md` — Update to reflect new patterns
- `.claude/skills/test-create/SKILL.md` — Update templates
- `.claude/skills/test-execute/SKILL.md` — Update commands
- `.claude/skills/test-review/SKILL.md` — Update coverage targets
- `go.mod` / `go.sum` — `go mod tidy` to fix indirect dep
