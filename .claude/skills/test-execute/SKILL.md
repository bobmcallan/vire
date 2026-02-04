# /test-execute - Test Execution

Run Vire tests and report results.

## Usage
```
/test-execute [scope] [options]
```

**Examples:**
- `/test-execute` - Run all tests
- `/test-execute unit` - Run unit tests only
- `/test-execute api` - Run API integration tests
- `/test-execute -v TestPortfolioReview` - Run specific test verbosely

## Workflow

### Step 1: Determine Test Scope

| Scope | Command | Description |
|-------|---------|-------------|
| `all` | `go test ./...` | All tests |
| `unit` | `go test ./internal/...` | Unit tests only |
| `api` | `go test ./test/api/...` | API integration tests |
| `signals` | `go test ./internal/signals/...` | Signal computation tests |

### Step 2: Execute Tests

```bash
# Basic execution
go test {scope} -v

# With coverage
go test {scope} -coverprofile=coverage.out

# Specific test
go test {scope} -run {TestName}
```

### Step 3: Report Results

Parse test output and report:
- Total tests run
- Passed/Failed/Skipped counts
- Duration
- Any failures with details

## Output Format

```
# Test Results

**Scope:** unit
**Duration:** 2.3s

| Status | Count |
|--------|-------|
| ✅ Passed | 45 |
| ❌ Failed | 2 |
| ⏭️ Skipped | 3 |

## Failures

### TestSignalComputation
```
signals/computer_test.go:45
Expected RSI 30.5, got 31.2
```
```

## Coverage Report

If `--coverage` flag:

```bash
go tool cover -html=coverage.out -o coverage.html
```

Report coverage percentage by package.
