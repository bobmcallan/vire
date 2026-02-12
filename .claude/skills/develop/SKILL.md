# /vire-develop - Vire Development Workflow

Develop and test Vire MCP server features using an agent team.

## Usage
```
/vire-develop <feature-description>
```

## Outputs

Every invocation produces documentation in `.claude/workdir/<datetime>-<taskname>/`:
- `requirements.md` — what was requested, scope, approach chosen
- `summary.md` — what was built, files changed, tests added, outcome

## Procedure

### Step 1: Create Work Directory

Generate the work directory path using the current datetime and a short task slug:
```
.claude/workdir/YYYYMMDD-HHMM-<task-slug>/
```

Example: `.claude/workdir/20260210-1430-arbor-logging/`

Create the directory and write `requirements.md`:

```markdown
# Requirements: <feature-description>

**Date:** <date>
**Requested:** <what the user asked for>

## Scope
- <what's in scope>
- <what's out of scope>

## Approach
<chosen approach and rationale>

## Files Expected to Change
- <file list>
```

### Step 2: Create Team and Tasks

Call `TeamCreate`:
```
team_name: "vire-develop"
description: "Developing: <feature-description>"
```

Create tasks grouped by phase using `TaskCreate`. Set `blockedBy` dependencies via `TaskUpdate` so later phases cannot start until earlier ones complete.

**Phase 1 — Plan** (no dependencies):
- "Investigate codebase and propose approach" — owner: implementer
- "Challenge approach and review for pattern consistency" — owner: reviewer, blockedBy: [investigate task]

**Phase 2 — Build** (blockedBy all Phase 1):
- "Write tests and implement feature" — owner: implementer
- "Review implementation: bugs, quality, edge cases, and test coverage" — owner: reviewer, blockedBy: [implement task]

**Phase 3 — Verify** (blockedBy all Phase 2):
- "Deploy, rebuild Docker, and run full test suite" — owner: implementer
- "Validate integration, update docs, and final sign-off" — owner: reviewer, blockedBy: [deploy task]

The deploy task MUST include:
1. `./scripts/deploy.sh local` — rebuild and deploy Docker containers
2. Verify containers are healthy: `curl -s http://localhost:4242/health`
3. `go test ./...` — full test suite
4. `go vet ./...` — static analysis
5. Report container status, test results, and any failures

### Step 3: Spawn Teammates

Spawn both teammates in parallel using the `Task` tool:

**implementer:**
```
name: "implementer"
subagent_type: "general-purpose"
model: "opus"
team_name: "vire-develop"
run_in_background: true
prompt: |
  You are the implementer on a development team. You write code and tests.

  Your team name is "vire-develop". Read your tasks from the task list with TaskList.
  Claim tasks assigned to you (owner: "implementer") by setting status to "in_progress".
  Work through tasks in ID order. Mark each completed before moving to the next.

  Key conventions:
  - Working directory: /home/bobmc/development/vire
  - Unit tests: go test ./internal/...
  - Integration tests: VIRE_TEST_DOCKER=true go test -v ./test/api/... -run TestName
  - Full suite: VIRE_TEST_DOCKER=true go test ./...
  - Docker rebuild: ./scripts/deploy.sh local
  - Lint: golangci-lint run

  Documentation tasks: update affected skill files in .claude/skills/vire-*/SKILL.md
  and README.md to reflect the changes made.

  Send messages to teammates via SendMessage when you need input.
  After completing each task, check TaskList for your next task.
  If all your tasks are done or blocked, send a message to the team lead.
```

**reviewer:**
```
name: "reviewer"
subagent_type: "general-purpose"
model: "haiku"
team_name: "vire-develop"
run_in_background: true
prompt: |
  You are the reviewer on a development team. You review code for bugs, quality,
  consistency with existing codebase patterns, and critically challenge decisions
  to catch problems early.

  Your team name is "vire-develop". Read your tasks from the task list with TaskList.
  Claim tasks assigned to you (owner: "reviewer") by setting status to "in_progress".
  Work through tasks in ID order. Mark each completed before moving to the next.

  Working directory: /home/bobmc/development/vire

  When reviewing approach:
  - Challenge design choices: Are there simpler alternatives? What assumptions are being made?
  - Question scope: Too broad? Too narrow? Right abstraction level?
  - Verify consistency with existing patterns in the codebase

  When reviewing implementation:
  - Read the changed files and surrounding context
  - Check for bugs, logic errors, race conditions, resource leaks, and edge cases
  - Validate test coverage: What edge cases are missing? Could tests pass with a broken implementation?
  - Play the role of a hostile input source

  When reviewing documentation:
  - Check that README.md accurately reflects new/changed functionality
  - Check that affected skill files match the implementation

  Report findings via SendMessage to "implementer" (for fixes) and to the team lead (for status).
  You must be convinced before marking any review task as complete.

  After completing each task, check TaskList for your next task.
  If all your tasks are done or blocked, send a message to the team lead.
```

### Step 4: Coordinate

As team lead, your job is coordination only:

1. **Relay information** — If the reviewer's findings need implementer action, forward via `SendMessage`.
2. **Resolve conflicts** — If the reviewer and implementer disagree, make the call.
3. **Unblock tasks** — When a phase completes, verify all tasks in that phase are done before confirming teammates can proceed.

### Step 5: Completion

When all tasks are complete:

1. Verify the code quality checklist:
   - All new code has tests
   - All tests pass
   - No new linter warnings (`golangci-lint run`)
   - Docker container builds successfully
   - README.md updated if user-facing behaviour changed
   - Affected skill files updated
   - Reviewer has signed off

2. Write `summary.md` in the work directory:

```markdown
# Summary: <feature-description>

**Date:** <date>
**Status:** <completed | partial | blocked>

## What Changed

| File | Change |
|------|--------|
| `path/to/file.go` | <brief description> |

## Tests
- <tests added or modified>
- <test results: pass/fail>

## Documentation Updated
- <list of docs/skills/README changes>

## Review Findings
- <key issues raised and how they were resolved>

## Notes
- <anything notable: trade-offs, follow-up work, risks>
```

3. Shut down teammates:
   ```
   SendMessage type: "shutdown_request" to each teammate
   ```

4. Clean up:
   ```
   TeamDelete
   ```

5. Summarise what was built, changed, and tested.

## Reference

### Key Directories

| Component | Location |
|-----------|----------|
| HTTP Server | `cmd/vire-server/` |
| Stdio Proxy | `cmd/vire-mcp/` |
| Shared App | `internal/app/` |
| Services | `internal/services/` |
| Clients | `internal/clients/` |
| Models | `internal/models/` |
| Config | `internal/common/config.go` |
| Storage | `internal/storage/` |
| Tests | `test/` |
| Docker | `docker/` |
| Skills | `.claude/skills/vire-*/` |

### Test Architecture
```
test/
├── api/           # Integration tests
├── common/        # Test infra (containers.go, mocks.go)
├── fixtures/      # Test data
└── results/       # Test output (gitignored)
```

### Documentation to Update

When the feature affects user-facing behaviour, update:
- `README.md` — if new tools, changed tool behaviour, or new capabilities
- `.claude/skills/vire-*/SKILL.md` — affected skill files
