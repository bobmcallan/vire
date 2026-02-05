# /test-create - Create New Test

Scaffold a new integration test following the Vire test architecture.

## Usage
```
/test-create <service_name> [test_name]
```

**Examples:**
- `/test-create signal` - Create signal service tests
- `/test-create portfolio SyncWithRetry` - Create specific test

## Prerequisites

See `/test-common` for test infrastructure documentation.

## Workflow

### Step 1: Determine Test Location

| Service Type | Location | Package |
|-------------|----------|---------|
| API/Integration | `test/api/{service}_test.go` | `api` |
| Unit | `internal/{pkg}/{file}_test.go` | same as source |

### Step 2: Create Test File

Create `test/api/{service_name}_test.go` using the template below.

### Step 3: Add Test Functions

Each test function should:
1. Setup Docker environment (skip if unavailable)
2. Create mock clients with test data
3. Initialize service under test
4. Execute and assert

### Step 4: Add Helper Functions

Place test-specific helpers at bottom of file (unexported).

## Template

```go
package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmccarthy/vire/internal/interfaces"
	"github.com/bobmccarthy/vire/internal/models"
	"github.com/bobmccarthy/vire/internal/services/{service}"
	"github.com/bobmccarthy/vire/test/common"
)

func Test{Feature}(t *testing.T) {
	// 1. Setup environment (skips if Docker unavailable)
	env := common.SetupDockerTestEnvironment(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	ctx := env.Context()

	// 2. Setup mocks with test data
	mockEODHD := common.NewMockEODHDClient()
	mockNavexa := common.NewMockNavexaClient()
	mockGemini := common.NewMockGeminiClient()

	// 3. Configure mock responses
	// mockEODHD.EODData["BHP.AU"] = ...
	// mockNavexa.Portfolios = ...

	// 4. Create service
	svc := {service}.NewService(
		env.StorageManager,
		mockEODHD,
		mockGemini,
		env.Logger,
	)

	// 5. Execute
	result, err := svc.{Method}(ctx, {params})

	// 6. Assert
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func Test{Feature}Error(t *testing.T) {
	env := common.SetupDockerTestEnvironment(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	ctx := env.Context()

	// Test error case
	svc := {service}.NewService(env.StorageManager, nil, nil, env.Logger)

	_, err := svc.{Method}(ctx, {invalid_params})
	assert.Error(t, err)
}
```

## Mock Configuration

### EODHD Mock
```go
mock := common.NewMockEODHDClient()
mock.EODData["BHP.AU"] = &models.EODResponse{Data: bars}
mock.Symbols["AU"] = []*models.Symbol{{Code: "BHP", Name: "BHP Group"}}
```

### Navexa Mock
```go
mock := common.NewMockNavexaClient()
mock.Portfolios = []models.NavexaPortfolio{{ID: "1", Name: "Test"}}
mock.Holdings["1"] = []*models.NavexaHolding{{Ticker: "BHP", Units: 100}}
```

### Gemini Mock
```go
mock := common.NewMockGeminiClient()
mock.Responses["analyze"] = "Mock analysis response"
```

## Test Data Helpers

Add at bottom of test file:

```go
// generateBars creates price bars from close prices
func generateBars(closes []float64) []models.EODBar {
	bars := make([]models.EODBar, len(closes))
	for i, close := range closes {
		bars[i] = models.EODBar{
			Open:     close - 0.5,
			High:     close + 1.0,
			Low:      close - 1.0,
			Close:    close,
			AdjClose: close,
			Volume:   500000,
		}
	}
	return bars
}
```

## Checklist

Before submitting test:

- [ ] Docker environment setup with cleanup
- [ ] Mocks configured (no real API calls)
- [ ] Both success and error cases covered
- [ ] Assertions use `require` for fatal, `assert` for non-fatal
- [ ] Test runs with `VIRE_TEST_DOCKER=true go test ./test/api/...`
