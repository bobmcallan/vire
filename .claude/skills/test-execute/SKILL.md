# /test-execute - Test Execution

Run Vire tests and report results.

**Mandatory rules are defined in `/test-common`. Read them first.**

**CRITICAL: This skill MUST NEVER modify or update test files. It is read-only.**

## Usage
```
/test-execute [scope] [options]
```

**Examples:**
- `/test-execute` - Run all tests
- `/test-execute unit` - Run storage unit tests
- `/test-execute data` - Run data layer integration tests
- `/test-execute api` - Run API integration tests
- `/test-execute TestHealthEndpoint` - Run a specific test by name
- `/test-execute TestPortfolioWorkflow` - Run portfolio workflow test

## Workflow

### Step 1: Validate Test Structure (Mandatory)

Before executing any tests, validate structural compliance. Check each test file in scope against the mandatory rules from `/test-common`:

| # | Rule | What to Check |
|---|------|---------------|
| 1 | Independent of Claude | No Claude/AI imports or runtime dependencies |
| 2 | Common containerized setup | Uses `StartSurrealDB(t)` or `NewEnv(t)` from `tests/common/` |
| 2b | Clean per test file | Unique DB per test, no cross-file state |
| 3 | Results output | Uses `TestOutputGuard` or `SaveResult()` |
| 4 | Standard Go patterns | Table-driven, `t.Helper()`, testify assertions |

**If non-compliant files are found:**
1. Document each violation in the output report
2. Advise the user to run `/test-create-review review <layer> <target>` to fix
3. Still execute the tests (non-compliance does not block execution)
4. **DO NOT modify the test files**

### Step 2: Determine Test Scope

Parse the argument to determine what to run:

| Argument | Command | Description |
|----------|---------|-------------|
| *(none)* or `all` | `go test ./... -timeout 300s` | All tests |
| `unit` | `go test ./internal/storage/surrealdb/... -v` | Storage unit tests |
| `data` | `go test ./tests/data/... -v` | Data layer integration tests |
| `api` | `go test ./tests/api/... -v -timeout 300s` | API end-to-end tests |
| `vet` | `go vet ./...` | Static analysis |
| `TestName` | *(see below)* | Run a specific test by name |

**Running a specific test by name:** When the argument starts with `Test` (e.g. `TestPortfolioWorkflow`, `TestHealthEndpoint`), search for the test across all packages and run it:

```bash
# Find which package contains the test
grep -rl 'func TestName' internal/ tests/

# Run it with verbose output and generous timeout
go test ./path/to/package/... -run TestName -v -timeout 300s
```

All tests use Docker containers — no env var gates are needed. API integration tests load `tests/docker/.env` automatically via `common.LoadEnvFile()` for secrets like `NAVEXA_API_KEY` and `DEFAULT_PORTFOLIO`.

### Step 3: Execute Tests

```bash
# All tests
go test ./... -timeout 300s

# Storage unit tests
go test ./internal/storage/surrealdb/... -v

# Data layer tests
go test ./tests/data/... -v

# API tests
go test ./tests/api/... -v -timeout 300s

# Specific test by name
go test ./tests/api/... -run TestPortfolioWorkflow -v -timeout 300s

# With coverage
go test ./internal/storage/surrealdb/... -coverprofile=coverage.out
```

### Step 4: Report Results

Output a structured report with:

```
# Test Execution Report

## Structure Validation
- Files checked: N
- Compliant: N
- Non-compliant: N (use /test-create-review to fix)

### Non-Compliant Files (if any)
- `path/to/file_test.go`: Rule 3 — missing results output

## Test Results

**Scope:** <scope>
**Duration:** <total duration>

| Status | Count |
|--------|-------|
| Passed | N |
| Failed | N |
| Skipped | N |

## Failures (if any)

### TestFunctionName
```
file_test.go:45
Expected X, got Y
```

## Coverage (if requested)
| Package | Coverage |
|---------|----------|
| internal/storage/surrealdb | 75% |

## Complete Test Log
<full test output saved to tests/logs/{datetime}-test-execute/>
```

The complete test log MUST be saved to `tests/logs/` regardless of pass/fail.

## Notes

- All tests use Docker — no env var gates required
- API tests load `tests/docker/.env` for secrets (Navexa key, default portfolio)
- SurrealDB container is shared per test process via `sync.Once`
- Each test gets a unique database name for isolation
- First test run pulls `surrealdb/surrealdb:v3.0.0` image (may be slow)
- API tests build `vire-server:test` Docker image (cached after first build)

## Coverage Report

```bash
go test ./internal/storage/surrealdb/... -coverprofile=coverage.out
go tool cover -func=coverage.out
go tool cover -html=coverage.out -o coverage.html
```
