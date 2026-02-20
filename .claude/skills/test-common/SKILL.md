# /test-common - Shared Test Infrastructure

Documentation for Vire's test infrastructure patterns and mandatory rules.

## Mandatory Rules

All Vire tests MUST comply with these rules. These are non-negotiable.

### Rule 1: Tests Are Independent of Claude

Tests MUST be executable via standard Go tooling. No test may depend on Claude, MCP, or any AI tooling to run. Every test must pass with:

```bash
go test ./path/to/package/...
```

Tests may be *created* or *reviewed* by Claude skills, but their execution must never require Claude.

### Rule 2: Common Containerized Setup, Clean Per Test File

All tests that require external services (SurrealDB, vire-server) MUST use the shared containerized setup from `tests/common/`. The environment MUST be clean for each test file (package-level or file-level setup). Individual tests within a file may share the environment if appropriate.

- Unit/data tests: Use `StartSurrealDB(t)` with a unique database per test
- API tests: Use `NewEnv(t)` which provides an isolated container per test file
- Never rely on state from a previous test file or package

### Rule 3: Test Results Output

All test files MUST create separate results in:

```
tests/logs/{datetime}-{TestName}/
```

Pattern: `YYYYMMDD-HHMMSS-TestFunctionName`

Every test MUST:
- Save the complete test log to the results directory
- Use `TestOutputGuard` or `Env.SaveResult()` for output persistence
- Include sufficient context to diagnose failures without re-running

When executed via `/test-execute`, a summary report is also generated. But the test itself must always write its own results regardless of how it's invoked.

### Rule 4: test-execute Is Read-Only

`/test-execute` MUST NEVER modify or update test files. Its role is:
1. Validate test structure compliance (Rules 1-3) before running
2. Execute the tests
3. Report results and any structural non-compliance

If structural issues are found, `/test-execute` documents them and advises using `/test-create-review` to fix. It does not fix them itself.

## Test Environment Setup

### Docker-Based API Tests

All Docker-based integration tests follow this pattern:

```go
func TestSomething(t *testing.T) {
    env := common.NewEnv(t)
    if env == nil {
        return
    }
    defer env.Cleanup()

    // HTTP API testing
    resp, err := env.HTTPGet("/api/health")
    // ...
}
```

### SurrealDB Unit/Data Tests

Storage-layer tests use a shared SurrealDB container via testcontainers:

```go
func TestSomething(t *testing.T) {
    sc := tcommon.StartSurrealDB(t)  // shared container via sync.Once
    db, _ := surreal.New(sc.Address())
    // sign in, select namespace/database, run tests
}
```

## Key Components

### `tests/common/containers.go` - Docker Test Environment

- `NewEnv(t)` / `NewEnvWithOptions(t, opts)` - Creates isolated Docker environment
- Builds `vire-server:test` image from `tests/docker/Dockerfile.server`
- Starts container with SurrealDB dependency
- Provides `HTTPGet`, `HTTPPost`, `HTTPPut`, `HTTPDelete` helpers
- `MCPRequest(method, params)` for MCP protocol tests
- `OutputGuard()` / `SaveResult()` for test output persistence
- Auto-collects container logs on cleanup

### `tests/common/surrealdb.go` - SurrealDB Container Helper

- `StartSurrealDB(t)` - Starts shared SurrealDB container (sync.Once)
- Uses image `surrealdb/surrealdb:v3.0.0`
- Exposes port 8000, auth: root/root
- `Address()` returns WebSocket RPC URL (`ws://host:port/rpc`)
- One container per test process, unique database per test for isolation

### TestOutputGuard

Validates test outputs:

```go
guard := common.NewTestOutputGuard(t)
guard.AssertContains(output, "expected text")
guard.AssertNotContains(output, "error")
guard.SaveResult("test_name", output)
```

### Test Results Directory

Results saved to: `tests/logs/{timestamp}-{TestName}/`

Structure:
```
tests/logs/
└── 20260220-120845-TestGetPortfolio/
    ├── 01_initialize_response.md
    ├── 02_sync_portfolio_response.md
    └── container.log
```

## Test Docker Infrastructure

### `tests/docker/vire.toml`
Main SurrealDB test config. Points to `ws://surrealdb:8000/rpc` (Docker network).

### `tests/docker/vire-blank.toml`
Blank SurrealDB config with separate `vire_blank` database.

### `tests/docker/docker-compose.yml`
Defines `surrealdb` (v3.0.0) and `vire-server` services with health checks.

### `tests/docker/Dockerfile.server`
Multi-stage build: Go 1.25 builder + Alpine runtime.

## Test Fixtures

Test fixtures in `tests/fixtures/`:
- `portfolio_smsf.json` - Sample portfolio
- `market_data_bhp.json` - Sample market data

## Environment Variables

```
VIRE_TEST_TIMEOUT=60s      # Test timeout (default 60s)
```

## Running Tests

```bash
# Unit tests (storage layer, requires Docker for SurrealDB container)
go test ./internal/storage/surrealdb/...

# Data layer integration tests (SurrealDB container via testcontainers)
go test ./tests/data/...

# API integration tests (Docker-based)
go test ./tests/api/...

# Specific test with verbose output
go test -v ./tests/api/... -run TestHealthEndpoint

# All tests
go test ./...
```
