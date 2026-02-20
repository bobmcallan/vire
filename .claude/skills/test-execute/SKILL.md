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
- `/test-execute -v TestHealthEndpoint` - Run specific test verbosely

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

| Scope | Command | Description | Requirements |
|-------|---------|-------------|--------------|
| `all` | `go test ./...` | All tests | Docker (SurrealDB container) |
| `unit` | `go test ./internal/storage/surrealdb/...` | Storage unit tests | Docker (SurrealDB container) |
| `data` | `go test ./tests/data/...` | Data layer integration tests | Docker (SurrealDB container) |
| `api` | `VIRE_TEST_DOCKER=true go test ./tests/api/...` | API end-to-end tests | Docker + VIRE_TEST_DOCKER=true |
| `vet` | `go vet ./...` | Static analysis | None |

### Step 3: Execute Tests

```bash
# Storage unit tests (SurrealDB container auto-started via testcontainers)
go test ./internal/storage/surrealdb/... -v

# Data layer tests (SurrealDB container auto-started via testcontainers)
go test ./tests/data/... -v

# API tests (requires Docker image build + SurrealDB)
VIRE_TEST_DOCKER=true go test ./tests/api/... -v

# With coverage
go test ./internal/storage/surrealdb/... -coverprofile=coverage.out

# Specific test
go test ./internal/storage/surrealdb/... -run TestGetUser -v

# With timeout
go test ./internal/storage/surrealdb/... -timeout 180s
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
<full test output saved to tests/results/{datetime}-test-execute/>
```

The complete test log MUST be saved to `tests/results/` regardless of pass/fail.

## Notes

- SurrealDB container is shared per test process via `sync.Once`
- Each test gets a unique database name for isolation
- First test run pulls `surrealdb/surrealdb:v2.2.1` image (may be slow)
- API tests build `vire-server:test` Docker image (cached after first build)

## Coverage Report

```bash
go test ./internal/storage/surrealdb/... -coverprofile=coverage.out
go tool cover -func=coverage.out
go tool cover -html=coverage.out -o coverage.html
```
