# /test-common - Shared Test Infrastructure

Documentation for Vire's test infrastructure patterns.

## Test Environment Setup

All integration tests follow this pattern:

```go
func TestSomething(t *testing.T) {
    dockerEnv := common.SetupDockerTestEnvironment(t)
    if dockerEnv == nil {
        return // Skip if Docker not available
    }
    defer dockerEnv.Cleanup()

    // Test code here
}
```

## Key Components

### DockerTestEnvironment

Provides:
- Isolated file-based storage per test
- Mock API servers for EODHD, Navexa, Gemini
- Automatic cleanup on test completion

### TestOutputGuard

Validates test outputs:

```go
guard := common.NewTestOutputGuard(t)
guard.AssertContains(output, "expected text")
guard.AssertNotContains(output, "error")
guard.SaveResult("test_name", output)
```

### Test Results Directory

Results saved to: `tests/results/{api|ui}/{timestamp}-{TestName}/`

Structure:
```
tests/results/
├── api/
│   └── 20240115-143022-TestPortfolioReview/
│       ├── output.md
│       ├── signals.json
│       └── summary.txt
└── ui/
    └── ...
```

## Mock Data

Test fixtures in `tests/fixtures/`:
- `portfolio_smsf.json` - Sample portfolio
- `market_data_bhp.json` - Sample market data
- `signals_bhp.json` - Sample signals

## Environment Variables

For tests:
```
VIRE_TEST_DOCKER=true      # Enable Docker tests
VIRE_TEST_EODHD_KEY=demo   # Use demo API key
VIRE_TEST_TIMEOUT=60s      # Test timeout
```

## Running Tests

```bash
# Unit tests only
go test ./internal/...

# Integration tests (requires Docker)
go test ./tests/api/...

# With verbose output
go test -v ./tests/api/... -run TestPortfolioReview
```
