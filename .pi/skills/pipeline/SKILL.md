---
name: pipeline
description: |
  Multi-phase development pipeline orchestrating 6 specialized agents (investigator,
  implementer, reviewer, devil's-advocate, test-creator, test-executor) with Docker test
  creation and execution. Use for complex features requiring code review, security analysis,
  and comprehensive testing. Keywords: pipeline, multi-agent, parallel, review, test, docker.
license: MIT
compatibility: Requires agent-team extension
metadata:
  phases: 5
  agents: 6
  parallel_workers: 2
allowed-tools: Bash dispatch_agent
---

# Multi-Phase Development Pipeline

Orchestrates 6 specialized agents across 5 phases with dependency management,
Docker test creation, and automated test execution with feedback loops.

## Usage

```
/pipeline <feature-description>
```

## Setup

Ensure the agent-team extension is installed:

```bash
pi -e extensions/agent-team.ts
```

Verify team is loaded (should show at session start):

```
Team: dev-team (investigator, implementer, reviewer, devil's-advocate, test-creator, test-executor)
```

## Agent Roles

| Agent | Role | Phase | Model |
|-------|------|-------|-------|
| investigator | Investigate codebase, gather context, create task specs | 0 | sonnet |
| implementer | Write unit tests + implementation | 1, 3, 4 | opus |
| reviewer | Code quality, pattern consistency, docs | 1, 3, 4 | sonnet |
| devil's-advocate | Security, edge cases, hostile inputs | 1 | opus |
| test-creator | Create Docker integration tests | 2 | sonnet |
| test-executor | Run tests, feedback loop | 2 | sonnet |

## Phase Structure

```
Phase 0: Investigate (pre-implementation)
         └── investigator ──→ requirements.md with detailed approach

Phase 1: Implement + Review + Stress-test (parallel)
         └── implementer ──┬── reviewer (blocked)
                           └── devil's-advocate (blocked)

Phase 2: Integration Tests (sequential, depends on Phase 1)
         └── test-creator ──→ test-executor

Phase 3: Verify (depends on Phase 2)
         └── implementer (build/test) ──→ reviewer (validate)

Phase 4: Document (depends on Phase 3)
         └── implementer (update docs) ──→ reviewer (verify docs)
```

## Procedure

### Step 1: Create Work Directory and requirements.md

Generate the work directory path using the current datetime and a short task slug:

```
.claude/workdir/YYYYMMDD-HHMM-<task-slug>/
```

Example: `.claude/workdir/20250116-1430-docker-test-framework/`

Create the directory and write `requirements.md`:

```markdown
# Requirements: <feature-description>

**Date:** <date>
**Requested:** <what the user asked for>

## Scope
- <what's in scope>
- <what's out of scope>

## Approach
<chosen approach and rationale - UPDATED AFTER INVESTIGATION>

## Files Expected to Change
- <file list>
```

### Step 2: Investigate and Plan

Dispatch the **investigator** agent to gather context and create detailed task specifications:

```json
{
  "agent": "investigator",
  "task": "Investigate codebase for: <feature-description>.\n\nWorking directory: <cwd>\n\nInvestigation goals:\n1. Understand relevant files, patterns, and existing implementations\n2. Determine approach, files to change, and risks\n3. Write findings into requirements.md under Approach section\n4. Create detailed task descriptions so teammates don't need to re-investigate\n\nOutput:\n- Updated requirements.md with comprehensive Approach section\n- Detailed task breakdown for downstream agents\n- Risk assessment with mitigation strategies\n- File change list with specific modifications needed\n\nThe key principle: thorough investigation happens UPFRONT so downstream agents receive all context they need."
}
```

The investigator will:
1. Explore the codebase to understand context
2. Identify relevant files and patterns
3. Assess risks and dependencies
4. Write comprehensive findings to requirements.md
5. Provide detailed task descriptions for downstream agents

**Critical**: Teammates should NOT need to re-investigate — all context must be gathered upfront.

### Step 3: Execute Phase 1 (Parallel Implementation)

**3a. Dispatch implementer first:**

```json
{
  "agent": "implementer",
  "task": "Write unit tests and implement: <feature-description>.\n\nWorking directory: <cwd>\nRead requirements.md for approach and files to change.\n\nConventions:\n- Unit tests go alongside code (*_test.go or *.test.ts)\n- Follow existing code patterns\n- Keep changes minimal and focused\n- Test edge cases\n\nReport what was changed and test results."
}
```

Wait for implementer to complete before proceeding.

**3b. Dispatch reviewer AND devil's-advocate in parallel:**

```json
{
  "agent": "reviewer",
  "task": "Review implementation of: <feature-description>.\n\nWorking directory: <cwd>\nRead requirements.md for approach.\n\nFiles changed: <list from implementer output>\n\nCheck:\n- Code quality and bugs\n- Pattern consistency with existing code\n- Test coverage adequacy\n- Documentation if public API\n\nReport findings. Send fixes to implementer if critical issues found."
}
```

```json
{
  "agent": "devil's-advocate",
  "task": "Stress-test implementation of: <feature-description>.\n\nWorking directory: <cwd>\nRead requirements.md for approach.\n\nFocus on:\n- Security vulnerabilities (injection, auth bypass)\n- Edge cases (null, empty, max values)\n- Failure modes (network errors, timeouts)\n- Hostile inputs (malformed data, attacks)\n\nWrite stress tests if appropriate. Report findings with severity ratings."
}
```

**Coordination:** If both reviewer and devil's-advocate find issues:
- Critical blockers: dispatch implementer again with consolidated fixes
- Minor issues: note for future work, proceed to Phase 2

### Step 4: Execute Phase 2 (Docker Integration Tests)

**4a. Dispatch test-creator:**

```json
{
  "agent": "test-creator",
  "task": "Create Docker integration tests for: <feature-description>.\n\nWorking directory: <cwd>\nRead requirements.md for approach.\n\nCreate tests following Docker test patterns:\n1. Containerized test environment\n2. Test isolation (unique namespaces/DBs)\n3. Cleanup in t.Cleanup() or defer\n4. Parallel-safe via unique identifiers\n5. Output results to tests/logs/\n\nTest structure:\n- tests/docker/ — Docker Compose files\n- tests/common/ — Test helpers and fixtures\n- tests/api/ or tests/integration/ — Integration tests\n\nWrite comprehensive tests covering:\n- Happy path\n- Error cases\n- Edge cases from devil's-advocate findings\n- Integration with existing components\n\nReport created test files and structure."
}
```

Wait for test-creator to complete.

**4b. Dispatch test-executor:**

```json
{
  "agent": "test-executor",
  "task": "Execute Docker integration tests for: <feature-description>.\n\nWorking directory: <cwd>\n\nIMPORTANT: You are read-only for test code. Do NOT modify test files.\n\nTest execution:\n1. Start Docker containers (docker-compose up -d)\n2. Wait for services to be ready\n3. Run integration tests\n4. Run unit tests to catch regressions\n5. Collect and report results\n6. Save results to tests/logs/\n7. Shutdown containers (docker-compose down)\n\nCommands:\n- docker-compose -f tests/docker/docker-compose.yml up -d\n- go test ./tests/integration/... -v -timeout 300s\n- go test ./internal/... -timeout 120s\n- docker-compose -f tests/docker/docker-compose.yml down\n\nFEEDBACK LOOP:\n- If tests PASS: report success with test count\n- If tests FAIL: send failure details to implementer with:\n  * Which tests failed\n  * Error output\n  * Which files likely need fixing\n  Then WAIT for implementer to fix. Re-run after confirmation.\n  Max 3 rounds.\n\nReport final test results."
}
```

**Coordination:** Monitor test-executor output:
- On failure: dispatch implementer with failure details
- After implementer fixes: dispatch test-executor again
- Repeat until pass or 3 rounds reached

### Step 5: Execute Phase 3 (Verify)

**5a. Dispatch implementer (build/test):**

```json
{
  "agent": "implementer",
  "task": "Build, test, and run locally for: <feature-description>.\n\nWorking directory: <cwd>\n\nVerify:\n1. Code compiles/builds\n2. All unit tests pass\n3. All integration tests pass\n4. Application starts and runs\n5. Feature works end-to-end\n\nRun:\n- go build ./... or npm run build\n- go test ./... or npm test\n- Manual smoke test if applicable\n\nReport verification results."
}
```

Wait for implementer to complete.

**5b. Dispatch reviewer (validate deployment):**

```json
{
  "agent": "reviewer",
  "task": "Validate deployment for: <feature-description>.\n\nWorking directory: <cwd>\n\nImplementation verified by implementer:\n<previous output>\n\nCheck:\n1. Health endpoints respond (if API)\n2. Key routes work correctly\n3. No new linter warnings\n4. No security regressions\n5. Performance acceptable\n\nReport validation results."
}
```

### Step 6: Execute Phase 4 (Document)

**6a. Dispatch implementer (update docs):**

```json
{
  "agent": "implementer",
  "task": "Update documentation for: <feature-description>.\n\nWorking directory: <cwd>\n\nUpdate:\n1. README.md if user-facing behavior changed\n2. API documentation if endpoints changed\n3. Skill files if workflows changed\n4. Inline code comments if complex logic\n\nKeep documentation:\n- Accurate (matches implementation)\n- Concise (avoid redundancy)\n- Complete (all public APIs documented)\n\nReport updated files."
}
```

Wait for implementer to complete.

**6b. Dispatch reviewer (verify docs):**

```json
{
  "agent": "reviewer",
  "task": "Verify documentation matches implementation for: <feature-description>.\n\nWorking directory: <cwd>\n\nDocumentation updated by implementer:\n<previous output>\n\nCheck:\n1. Examples in docs actually work\n2. API docs match actual endpoints\n3. No outdated information\n4. New features are documented\n5. Breaking changes are noted\n\nReport verification results."
}
```

### Step 7: Completion

When all phases complete:

1. **Run quality checklist:**
   - [ ] All new code has unit tests
   - [ ] Docker integration tests created
   - [ ] All tests pass
   - [ ] No new linter warnings
   - [ ] Application builds
   - [ ] Feature verified end-to-end
   - [ ] Documentation updated
   - [ ] Devil's-advocate signed off
   - [ ] Test-executor signed off

2. **Write `summary.md` in work directory:**

```markdown
# Summary: <feature-description>

**Date:** <date>
**Status:** <completed | partial | blocked>

## What Changed

| File | Change |
|------|--------|
| `path/to/file.ext` | <brief description> |

## Tests
- Unit tests: <count> tests, <pass/fail>
- Integration tests: <count> tests, <pass/fail>
- Docker tests: <count> containers, <pass/fail>
- Test feedback rounds: <N> (if failures required fixes)

## Security Review
- Devil's-advocate findings: <count> issues
- Critical: <count> (resolved/pending)
- High: <count> (resolved/pending)
- Medium: <count> (resolved/pending)

## Documentation Updated
- <list of docs/skills/README changes>

## Notes
- <anything notable: trade-offs, follow-up work, risks>
```

3. **Report results to user**

## Coordination Guidelines

As pipeline lead (the primary agent running this skill):

1. **Relay information** — Forward findings between agents when relevant
2. **Resolve conflicts** — Make final call on disagreements
3. **Apply trivial fixes directly** — Don't round-trip typos through agents
4. **Monitor feedback loops** — Ensure test failures get addressed
5. **Track progress** — Know which phase/agent is active
6. **Enforce max rounds** — Stop after 3 test retry rounds, document remaining issues

## Docker Test Patterns

### Container Structure

```yaml
# tests/docker/docker-compose.yml
version: '3.8'
services:
  db:
    image: surrealdb/surrealdb:latest
    ports:
      - "8000:8000"
    environment:
      - SURREALDB_USER=root
      - SURREALDB_PASS=root
  api:
    build: ../../
    ports:
      - "8080:8080"
    depends_on:
      - db
    environment:
      - DATABASE_URL=ws://db:8000/rpc
```

### Test Isolation

```go
func TestFeature(t *testing.T) {
    // Unique namespace for parallel safety
    ns := fmt.Sprintf("test_%s", t.Name())
    db := connectDB(t, ns)
    defer db.Close()

    // Cleanup on exit
    t.Cleanup(func() {
        db.Query("REMOVE NAMESPACE $ns", map[string]any{"ns": ns})
    })

    // Run test...
}
```

### Results Output

```go
func TestMain(m *testing.M) {
    // Setup Docker containers
    cmd := exec.Command("docker-compose", "up", "-d")
    cmd.Run()

    // Wait for services
    time.Sleep(5 * time.Second)

    // Run tests
    code := m.Run()

    // Save results
    results := fmt.Sprintf("tests/logs/%s.json", time.Now().Format("20060102-150405"))
    os.WriteFile(results, testResults, 0644)

    // Cleanup
    exec.Command("docker-compose", "down").Run()

    os.Exit(code)
}
```

## Reference

- [Task Dependencies](references/task-dependencies.md) — Detailed blocking structure
- [Docker Test Templates](references/docker-test-templates.md) — Test creation patterns
- [Agent Communication](references/agent-communication.md) — How agents send messages
