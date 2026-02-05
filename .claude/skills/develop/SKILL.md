# /vire-develop - Vire Development Workflow

Develop and test Vire MCP server features with proper test-driven development.

## Usage
```
/vire-develop <feature-description>
```

**Examples:**
- `/vire-develop add support for stop-loss alerts`
- `/vire-develop fix RSI calculation for low-volume stocks`
- `/vire-develop add sector filtering to market snipe`

## Workflow

### Phase 1: Requirements Analysis

1. **Review the feature request** - Understand what's being asked
2. **Check existing code** - Search for related implementations:
   ```bash
   # Search for related code
   grep -r "relevant_term" internal/ cmd/
   ```
3. **Identify affected components**:
   - MCP tools (cmd/vire-mcp/)
   - Services (internal/services/)
   - Clients (internal/clients/)
   - Models (internal/models/)
   - Storage (internal/storage/)

### Phase 2: Architecture

1. **Document the approach** in a brief plan:
   - What changes are needed
   - Which files will be modified
   - Any new types or interfaces required

2. **Consider test strategy**:
   - Unit tests for pure functions
   - Integration tests for API interactions
   - End-to-end tests for MCP tool workflows

### Phase 3: Test-First Development

1. **Review existing tests**:
   ```bash
   ls test/api/
   cat test/api/*_test.go
   ```

2. **Create or update test cases** that define expected behavior:
   ```go
   func TestNewFeature(t *testing.T) {
       env := common.SetupDockerTestEnvironment(t)
       if env == nil {
           return
       }
       defer env.Cleanup()

       // Test implementation
   }
   ```

3. **Run tests to confirm they fail**:
   ```bash
   cd /home/bobmc/development/vire
   VIRE_TEST_DOCKER=true go test -v ./test/api/... -run TestNewFeature
   ```

### Phase 4: Implementation

1. **Implement the feature** to pass tests
2. **Run tests after each change**:
   ```bash
   go test -v ./test/api/... -run TestNewFeature
   ```
3. **Iterate until all tests pass**

### Phase 5: Integration Testing

1. **Rebuild Docker container**:
   ```bash
   cd /home/bobmc/development/vire
   docker compose -f docker/docker-compose.yml build
   docker compose -f docker/docker-compose.yml up -d
   touch docker/.last_build
   ```

2. **Test MCP tools manually** using the vire MCP tools

3. **Run full test suite**:
   ```bash
   VIRE_TEST_DOCKER=true go test ./...
   ```

### Phase 6: Update Skills

After implementation, update affected skills:
- `.claude/skills/vire-portfolio-review/SKILL.md`
- `.claude/skills/vire-market-snipe/SKILL.md`
- `.claude/skills/vire-collect/SKILL.md`

Update documentation to reflect:
- New parameters or options
- Changed behavior
- New output fields

## Test Architecture

### Current Structure
```
test/
├── api/                    # Integration tests
│   ├── portfolio_review_test.go
│   └── market_snipe_test.go
├── common/                 # Shared test infrastructure
│   ├── containers.go       # Test environment setup
│   └── mocks.go           # Mock implementations
├── fixtures/              # Test data
│   └── portfolio_smsf.json
└── results/               # Test output (gitignored)
```

### Docker Test Environment

Tests use `DockerTestEnvironment` which provides:
- Isolated BadgerDB instance (temp directory per test)
- Mock API servers for external services
- Automatic cleanup

**Note:** Current implementation uses temp directories, not actual Docker containers.
For true isolation, tests should use testcontainers-go.

### Running Tests

```bash
# Unit tests only
go test ./internal/...

# Integration tests
VIRE_TEST_DOCKER=true go test ./test/api/...

# Specific test with verbose output
VIRE_TEST_DOCKER=true go test -v ./test/api/... -run TestPortfolioReview

# All tests
VIRE_TEST_DOCKER=true go test ./...
```

## Code Quality Checklist

Before completing:
- [ ] All new code has tests
- [ ] All tests pass
- [ ] No new linter warnings: `golangci-lint run`
- [ ] Docker container builds successfully
- [ ] MCP tools work via Claude Code
- [ ] Skills documentation updated

## Files Reference

| Component | Location |
|-----------|----------|
| MCP Server | `cmd/vire-mcp/` |
| Services | `internal/services/` |
| Clients | `internal/clients/` |
| Models | `internal/models/` |
| Config | `internal/common/config.go` |
| Storage | `internal/storage/` |
| Tests | `test/` |
| Docker | `docker/` |
| Skills | `.claude/skills/vire-*/` |
