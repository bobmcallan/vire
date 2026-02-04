# /test-review - Test Compliance Review

Review test coverage and compliance with testing standards.

## Usage
```
/test-review [scope]
```

**Examples:**
- `/test-review` - Full compliance review
- `/test-review services` - Review service tests
- `/test-review clients` - Review client tests

## Checklist

### Required Test Patterns

| Pattern | Description | Required |
|---------|-------------|----------|
| Table-driven tests | Use test tables for multiple cases | ✅ |
| Error cases | Test error handling paths | ✅ |
| Edge cases | Test boundary conditions | ✅ |
| Mock interfaces | Mock external dependencies | ✅ |
| Cleanup | Proper resource cleanup | ✅ |

### Coverage Requirements

| Package | Minimum Coverage |
|---------|------------------|
| `internal/signals` | 80% |
| `internal/services` | 70% |
| `internal/clients` | 60% |
| `internal/storage` | 70% |

### Integration Test Requirements

- [ ] Docker environment setup
- [ ] Isolated data per test
- [ ] Result persistence
- [ ] Timeout handling

## Review Process

### Step 1: Check Test Files Exist

```bash
find . -name "*_test.go" | head -20
```

### Step 2: Analyze Coverage

```bash
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

### Step 3: Review Test Quality

For each test file:
1. Check for table-driven tests
2. Verify error case coverage
3. Confirm mocks are used for external deps
4. Ensure proper assertions

### Step 4: Generate Report

```
# Test Compliance Review

## Summary
- Test files: 15
- Total tests: 87
- Coverage: 72%

## By Package

| Package | Tests | Coverage | Status |
|---------|-------|----------|--------|
| signals | 25 | 85% | ✅ |
| services | 30 | 68% | ⚠️ |
| clients | 20 | 55% | ❌ |
| storage | 12 | 75% | ✅ |

## Recommendations

1. Add error case tests to `services/market`
2. Increase client test coverage
3. Add integration tests for portfolio sync
```

## Standards Reference

Tests should follow:
- Go testing best practices
- Table-driven test pattern
- testify/assert for assertions
- testcontainers-go for Docker tests
