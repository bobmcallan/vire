# /test-create-review - Create or Review Tests

Create new tests or update existing tests to match the required structure. This skill consolidates test creation and compliance review into a single workflow.

**Mandatory rules are defined in `/test-common`. Read them first.**

## Usage
```
/test-create-review <action> <layer> <target>
```

**Actions:**
- `create` — Scaffold a new test file
- `review` — Review existing tests for compliance and fix issues
- `audit` — Review all tests without making changes (report only)

**Examples:**
- `/test-create-review create unit internalstore` — Create storage unit tests
- `/test-create-review create data marketstore` — Create data layer tests
- `/test-create-review create api user` — Create API integration tests
- `/test-create-review review unit internalstore` — Review and fix storage unit tests
- `/test-create-review review api` — Review and fix all API tests
- `/test-create-review audit` — Audit all tests for compliance (no changes)

## Prerequisites

Read `/test-common` for:
- **Mandatory Rules** (compliance requirements for all tests)
- Test environment setup patterns
- Key components and helpers

## Workflow

### Step 1: Determine Test Layer and Location

| Layer | Location | Package | Dependencies |
|-------|----------|---------|--------------|
| Unit (storage) | `internal/storage/surrealdb/{store}_test.go` | `surrealdb` | SurrealDB container |
| Data (integration) | `tests/data/{store}_test.go` | `data` | SurrealDB container |
| API (end-to-end) | `tests/api/{service}_test.go` | `api` | Docker |
| Connectivity | `tests/api/connectivity_test.go` | `api` | External API keys |

### Step 2: For `create` — Scaffold Using Template

Create the test file using the appropriate template below. Ensure compliance with all mandatory rules from `/test-common`.

### Step 3: For `review` — Check and Fix Compliance

Read the target test files and check each mandatory rule:

#### Compliance Checklist

| # | Rule | Check | Fix |
|---|------|-------|-----|
| 1 | Independent of Claude | No Claude/AI imports or dependencies | Remove any Claude-specific code |
| 2 | Common containerized setup | Uses `StartSurrealDB(t)` or `NewEnv(t)` from `tests/common/` | Replace custom setup with common helpers |
| 2b | Clean per test file | Setup at file level, unique DB per test | Add `testDB(t)` or `testManager(t)` pattern |
| 3 | Results output | Uses `TestOutputGuard` or `SaveResult()` | Add results directory and output saving |
| 3b | Complete test log | Results include full test context | Add log capture and `SaveResult()` calls |
| 4 | Structural patterns | Table-driven, `t.Helper()`, proper assertions | Restructure tests to match patterns |

For each non-compliant item: fix the test file directly, then document what was changed.

### Step 4: For `audit` — Report Only

Same checks as `review`, but do NOT modify any files. Output a compliance report:

```
# Test Compliance Audit

## Summary
- Files checked: N
- Compliant: N
- Non-compliant: N

## Non-Compliant Files

### path/to/file_test.go
- [ ] Rule 2: Missing common setup — uses custom SurrealDB connection
- [ ] Rule 3: No results output — missing TestOutputGuard

## Recommendation
Run `/test-create-review review <layer> <target>` to fix.
```

## Templates

### Unit Test (Storage Layer)

```go
package surrealdb

import (
    "context"
    "testing"
    "time"

    "github.com/bobmcallan/vire/internal/models"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestFeature(t *testing.T) {
    db := testDB(t)  // from testhelper_test.go — shared SurrealDB, unique DB
    store := NewInternalStore(db, testLogger())
    ctx := context.Background()

    // Table-driven tests
    tests := []struct {
        name string
        // fields...
    }{
        {"case1", /* ... */},
        {"case2", /* ... */},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // test logic
        })
    }
}
```

### Data Layer Test (Interface-Based)

```go
package data

import (
    "testing"
    "time"

    "github.com/bobmcallan/vire/internal/models"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestFeatureLifecycle(t *testing.T) {
    mgr := testManager(t)  // from helpers_test.go — unique DB per test
    store := mgr.InternalStore()
    ctx := testContext()

    // Create -> Read -> Update -> Delete
    // ...
}
```

### API Test (HTTP End-to-End)

```go
package api

import (
    "encoding/json"
    "io"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"

    "github.com/bobmcallan/vire/tests/common"
)

func TestFeatureEndpoint(t *testing.T) {
    env := common.NewEnv(t)
    if env == nil {
        return
    }
    defer env.Cleanup()

    guard := env.OutputGuard()

    resp, err := env.HTTPGet("/api/endpoint")
    require.NoError(t, err)
    defer resp.Body.Close()

    body, _ := io.ReadAll(resp.Body)
    guard.SaveResult("response", string(body))

    assert.Equal(t, 200, resp.StatusCode)
}
```

### MCP Test (JSON-RPC)

```go
func TestMCPTool(t *testing.T) {
    env := common.NewEnvWithOptions(t, common.EnvOptions{
        ConfigFile: "vire.toml",
    })
    if env == nil {
        return
    }
    defer env.Cleanup()

    guard := env.OutputGuard()

    result, err := env.MCPRequest("tools/call", map[string]interface{}{
        "name":      "tool_name",
        "arguments": map[string]interface{}{"key": "value"},
    })
    require.NoError(t, err)

    guard.SaveResult("result", common.FormatMCPContent(result))
}
```

## Test Helpers

### Storage test helper (`testhelper_test.go`)

Each storage package has a `testDB(t)` that:
- Starts shared SurrealDB container via `tcommon.StartSurrealDB(t)`
- Connects and authenticates (root/root)
- Uses unique database per test for isolation
- Registers cleanup to close connection

### Data test helper (`helpers_test.go`)

`testManager(t)` creates a full `interfaces.StorageManager` with unique database per test.

## Coverage Requirements

| Package | Minimum Coverage |
|---------|------------------|
| `internal/storage/surrealdb` | 70% |
| `internal/signals` | 80% |
| `internal/server` | 60% |

## Test Layer Requirements

### Unit Tests (`internal/storage/surrealdb/`)
- [ ] InternalStore: CRUD users, KV, SystemKV
- [ ] UserStore: CRUD records, query ordering, delete by subject
- [ ] MarketStore: CRUD market data, signals, batch, purge
- [ ] Manager: NewManager, WriteRaw, PurgeDerivedData, Close

### Data Layer Tests (`tests/data/`)
- [ ] Lifecycle tests (create -> read -> update -> delete)
- [ ] Multiple subjects (portfolio, strategy, plan, watchlist, report, search)
- [ ] Query ordering (datetime_asc, datetime_desc)
- [ ] Batch operations
- [ ] Concurrent access

### API Tests (`tests/api/`)
- [ ] Health and version endpoints
- [ ] User CRUD via HTTP
- [ ] Username availability check
- [ ] Upsert semantics

## Structural Checklist

Before completing any create or review action:

- [ ] Uses `testify/require` for setup failures, `testify/assert` for assertions
- [ ] Table-driven tests where multiple cases exist
- [ ] `t.Helper()` on all helper functions
- [ ] Test isolation (unique database per test)
- [ ] Both success and error/not-found cases covered
- [ ] Proper cleanup via `t.Cleanup()` or `defer`
- [ ] Module path is `github.com/bobmcallan/vire`
- [ ] Results output to `tests/results/{datetime}-{TestName}/`
- [ ] Executable via `go test` without Claude
- [ ] Uses common containerized setup from `tests/common/`

## Standards Reference

Tests should follow:
- Go testing best practices
- Table-driven test pattern
- `testify/assert` and `testify/require` for assertions
- `testcontainers-go` for container management
- Module path: `github.com/bobmcallan/vire`
