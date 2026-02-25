# Pi Development Pipeline Skill

Multi-phase development pipeline with automated testing, code review, and security analysis.

## Quick Start

```bash
# Start the pipeline with agent-team extension
pi -e extensions/agent-team.ts

# Use the pipeline skill
/pipeline Add user authentication with JWT tokens
```

## What It Does

The pipeline skill orchestrates 5 specialized AI agents to build features with comprehensive testing and review:

1. **implementer** — Writes code and unit tests
2. **reviewer** — Reviews code quality and consistency
3. **devil's-advocate** — Stress-tests for security and edge cases
4. **test-creator** — Creates Docker integration tests
5. **test-executor** — Runs tests with automated feedback loops

## Pipeline Phases

```
┌─────────────────────────────────────────────────────┐
│ Phase 1: Implement + Review + Stress-test          │
│  ├─ implementer (code + unit tests)                │
│  ├─ reviewer (code quality) ← blocks               │
│  └─ devil's-advocate (security) ← blocks           │
└─────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────┐
│ Phase 2: Docker Integration Tests                   │
│  ├─ test-creator (write tests)                     │
│  └─ test-executor (run tests)                      │
│      └─ feedback loop to implementer on failures   │
└─────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────┐
│ Phase 3: Verify                                     │
│  ├─ implementer (build + test locally)             │
│  └─ reviewer (validate deployment)                  │
└─────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────┐
│ Phase 4: Document                                   │
│  ├─ implementer (update docs)                      │
│  └─ reviewer (verify docs)                          │
└─────────────────────────────────────────────────────┘
```

## Features

### Structured Development
- Automatic work directory creation with timestamps
- Requirements documentation template
- Approach planning before implementation

### Automated Review
- Code quality checks
- Pattern consistency verification
- Test coverage validation

### Security Testing
- Vulnerability scanning
- Edge case testing
- Hostile input validation
- Injection attack prevention

### Docker Integration Tests
- Containerized test environments
- Test isolation (unique namespaces)
- Parallel-safe execution
- Automatic cleanup

### Feedback Loops
- Automated test failure reporting
- Max 3 retry rounds for test fixes
- Failure details sent to implementer
- Progress tracking across phases

### Documentation
- Automatic documentation updates
- Verification against implementation
- Complete audit trail in workdir

## Usage Examples

### Basic Feature

```bash
/pipeline Add rate limiting to API endpoints
```

### Complex Feature with Specifics

```bash
/pipeline Implement OAuth2 authentication with Google and GitHub providers, including token refresh and session management
```

### Feature with Constraints

```bash
/pipeline Add real-time notifications using WebSockets. Requirements: Must work behind load balancer, support reconnection, handle 10k concurrent connections
```

## Output Structure

Each pipeline run creates a work directory:

```
.claude/workdir/20250116-1430-oauth-auth/
├── requirements.md     # What was requested, approach, files to change
└── summary.md          # What was built, tests, security review, metrics
```

## Test Results

Integration test results are saved to:

```
tests/logs/
├── 20250116-143000-unit.json         # Unit test results
└── 20250116-143015-integration.json  # Integration test results
```

## Configuration

### Model Assignments

| Agent | Model | Reason |
|-------|-------|--------|
| implementer | opus | Complex implementation work |
| devil's-advocate | opus | Deep security analysis |
| reviewer | sonnet | Code review |
| test-creator | sonnet | Test writing |
| test-executor | sonnet | Test execution |

### Timeouts

| Phase | Timeout |
|-------|---------|
| Implement | 600s |
| Review | 300s |
| Stress-test | 450s |
| Test-create | 600s |
| Test-execute | 300s |
| Build/test | 300s |
| Validate | 200s |
| Update docs | 300s |
| Verify docs | 200s |

### Max Retries

- Test feedback loop: **3 rounds**
- After 3 rounds, remaining failures are documented

## Agent Definitions

Agents are defined in `.pi/agents/`:

- `implementer.md` — Code generation and unit tests
- `reviewer.md` — Code quality and pattern review
- `devil's-advocate.md` — Security and edge case testing
- `test-creator.md` — Docker integration test creation
- `test-executor.md` — Test execution and feedback loops

## Teams Configuration

The dev-team is defined in `.pi/agents/teams.yaml`:

```yaml
dev-team:
  - implementer
  - reviewer
  - devil's-advocate
  - test-creator
  - test-executor
```

Other teams available:
- `build-team` — Quick implementation without extensive testing
- `security-team` — Focused on security review
- `test-team` — Focused on testing only

## Switching Teams

```bash
# Switch to security-focused team
/agents-team

# Select "security-team" from the list
```

## Files Structure

```
.pi/
├── skills/
│   └── pipeline/
│       ├── SKILL.md                      # Main skill definition
│       ├── README.md                     # This file
│       ├── references/
│       │   ├── task-dependencies.md      # Phase/blocking structure
│       │   ├── docker-test-templates.md  # Test creation patterns
│       │   └── agent-communication.md    # Inter-agent messaging
│       └── templates/
│           ├── workdir-requirements.md   # Requirements template
│           └── workdir-summary.md        # Summary template
├── agents/
│   ├── implementer.md
│   ├── reviewer.md
│   ├── devil's-advocate.md
│   ├── test-creator.md
│   ├── test-executor.md
│   └── teams.yaml
└── extensions.tmp/
    └── agent-team.ts                     # Agent dispatch extension
```

## Coordination Guidelines

As the primary agent (you, the user), you can:

1. **Monitor progress** — Each agent reports status
2. **Resolve conflicts** — Make final decisions when agents disagree
3. **Apply trivial fixes** — Directly fix typos and minor issues
4. **Stop the pipeline** — If critical issues arise

## Best Practices

### When to Use
- Complex features requiring multiple perspectives
- Features with security implications
- Changes affecting multiple files
- Features requiring comprehensive testing

### When NOT to Use
- Simple typo fixes
- Single-line changes
- Documentation-only updates
- Quick experiments

### Tips

1. **Be specific** — Detailed feature descriptions lead to better results
2. **Provide context** — Mention existing patterns to follow
3. **Set constraints** — Specify requirements upfront
4. **Review progress** — Check in after each phase
5. **Trust the process** — Let agents complete their phases

## Troubleshooting

### "Agent X not found"

```bash
# Verify agents are loaded
/agents-list

# Expected output:
implementer (idle, new, runs: 0): Implementation and code generation
reviewer (idle, new, runs: 0): Code review and quality checks
...
```

### "Docker containers not starting"

```bash
# Check Docker is running
docker ps

# Check docker-compose file exists
ls tests/docker/docker-compose.yml

# Start containers manually
docker-compose -f tests/docker/docker-compose.yml up -d
```

### "Tests timeout"

Increase timeouts in the skill execution or reduce test complexity:

```bash
# Run specific test with longer timeout
go test ./tests/integration/... -v -timeout 600s
```

### "Too many feedback rounds"

After 3 rounds, remaining failures are documented. Review the failure details:

```bash
cat .claude/workdir/YYYYMMDD-HHMM-*/summary.md
```

## Advanced Usage

### Custom Test Patterns

Add custom test patterns to `references/docker-test-templates.md`:

```go
func TestCustomPattern(t *testing.T) {
    // Your custom test pattern
}
```

### Additional Agents

Create new agent definitions:

```bash
# Create custom agent
cat > .pi/agents/custom-reviewer.md << 'EOF'
---
name: custom-reviewer
description: Custom review agent for specific patterns
tools: read,bash,grep,find,ls
---
[Custom instructions...]
EOF

# Add to team
echo "  - custom-reviewer" >> .pi/agents/teams.yaml
```

### Pipeline Customization

Modify phases in `SKILL.md`:

```markdown
### Phase 2.5: Performance Testing
- Dispatch performance-tester agent
- Load test the implementation
- Report metrics and bottlenecks
```

## Contributing

To improve the pipeline:

1. **Update templates** — Improve `workdir-requirements.md` or `workdir-summary.md`
2. **Add test patterns** — Extend `docker-test-templates.md`
3. **Refine agent prompts** — Edit agent `.md` files
4. **Document patterns** — Add to `agent-communication.md`

## License

MIT
