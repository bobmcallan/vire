# /vire-develop - Vire Development Workflow

Develop and test Vire MCP server features using an agent team for parallel, high-quality development.

## Usage
```
/vire-develop <feature-description>
```

**Examples:**
- `/vire-develop add support for stop-loss alerts`
- `/vire-develop fix RSI calculation for low-volume stocks`
- `/vire-develop add sector filtering to market snipe`

## Workflow

When this skill is invoked, create an agent team to develop the feature collaboratively. The team should have the following structure:

### Team Structure

Create an agent team with the following teammates:

1. **implementer** - The primary developer who writes the feature code and tests
   - Owns all code changes across `internal/`, `cmd/`, and `test/`
   - Follows test-driven development: writes failing tests first, then implements
   - Runs tests after each change to verify correctness
   - Uses Sonnet model for cost efficiency

2. **reviewer** - A code reviewer focused on quality and correctness
   - Reviews the implementer's changes for bugs, edge cases, and design issues
   - Validates test coverage is adequate
   - Checks for adherence to existing patterns in the codebase
   - Reports findings back to the team
   - Uses Sonnet model for cost efficiency

3. **devils-advocate** - A critical challenger who stress-tests every decision
   - Actively challenges the implementer's design choices and assumptions
   - Proposes alternative approaches and asks "what if" questions
   - Looks for failure modes, performance issues, and missed edge cases
   - Questions whether the feature scope is right (too broad? too narrow?)
   - Tries to poke holes in the test strategy (what's NOT being tested?)
   - Plays the role of a skeptical user or hostile input source
   - Must be convinced before the team considers a task complete
   - Uses Sonnet model for cost efficiency

### Team Coordination

The lead (you) should:

1. **Analyse the feature request** and break it into tasks:
   - Requirements analysis task
   - Architecture/design task
   - Test creation task(s)
   - Implementation task(s)
   - Integration testing task
   - Skills documentation update task

2. **Assign tasks** to teammates with clear context:
   - Give the implementer the coding and testing tasks
   - Give the reviewer tasks to review after implementation
   - Give the devils-advocate tasks to challenge the design and test strategy at each phase

3. **Require plan approval** for the implementer before they start coding, so the devils-advocate and reviewer can weigh in on the approach first.

4. **Wait for teammates** to complete their work - do not implement code yourself. Use delegate mode.

5. **Synthesise findings** from all three teammates before considering each phase complete.

### Phase 1: Requirements & Design

1. The implementer investigates the codebase and proposes an approach
2. The devils-advocate challenges the approach:
   - Are there simpler alternatives?
   - What assumptions are being made?
   - What could go wrong?
   - Is this the right abstraction level?
3. The reviewer checks if the approach follows existing patterns
4. Iterate until all three teammates reach consensus

### Phase 2: Test-First Development

1. The implementer writes failing tests that define expected behavior:
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

2. The devils-advocate challenges the test strategy:
   - What edge cases are missing?
   - What happens with nil/empty/malformed input?
   - Are error paths tested?
   - Could these tests pass with a broken implementation?

3. The implementer runs tests to confirm they fail:
   ```bash
   cd /home/bobmc/development/vire
   VIRE_TEST_DOCKER=true go test -v ./test/api/... -run TestNewFeature
   ```

### Phase 3: Implementation

1. The implementer writes code to pass tests
2. The implementer runs tests after each change:
   ```bash
   go test -v ./test/api/... -run TestNewFeature
   ```
3. The reviewer checks the implementation for:
   - Bugs and logic errors
   - Code quality and readability
   - Consistency with existing codebase patterns
4. The devils-advocate stress-tests the implementation:
   - Could this break existing functionality?
   - What happens under load or with unexpected data?
   - Are there race conditions or resource leaks?

### Phase 4: Integration Testing

1. The implementer rebuilds and tests:
   ```bash
   cd /home/bobmc/development/vire
   docker compose -f docker/docker-compose.yml build
   docker compose -f docker/docker-compose.yml up -d
   touch docker/.last_build
   ```

2. Run full test suite:
   ```bash
   VIRE_TEST_DOCKER=true go test ./...
   ```

3. The reviewer validates MCP tool integration works end-to-end
4. The devils-advocate verifies nothing was missed

### Phase 5: Update Skills & Cleanup

After implementation, update affected skills:
- `.claude/skills/vire-portfolio-review/SKILL.md`
- `.claude/skills/vire-market-snipe/SKILL.md`
- `.claude/skills/vire-collect/SKILL.md`

The reviewer verifies documentation matches the implementation.

**Clean up the team** when all tasks are complete.

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
- [ ] Devils-advocate has signed off on the approach

## Files Reference

| Component | Location |
|-----------|----------|
| MCP Server | `cmd/vire-mcp/` |
| Services | `internal/services/` |
| Strategy Service | `internal/services/strategy/` |
| Strategy Model | `internal/models/strategy.go` |
| Clients | `internal/clients/` |
| Models | `internal/models/` |
| Config | `internal/common/config.go` |
| Storage | `internal/storage/` |
| Strategy Storage | `internal/storage/badger.go` (strategyStorage) |
| Tests | `test/` |
| Strategy Tests | `internal/services/strategy/service_test.go`, `internal/storage/strategy_test.go`, `internal/models/strategy_test.go` |
| Docker | `docker/` |
| Skills | `.claude/skills/vire-*/` |
