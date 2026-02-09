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

**Phase 1 — Investigate** (no dependencies):
- "Investigate codebase and propose approach" — owner: implementer
- "Challenge the proposed approach" — owner: devils-advocate, blockedBy: [investigate task]
- "Review approach for pattern consistency" — owner: reviewer, blockedBy: [investigate task]

**Phase 2 — Test** (blockedBy all Phase 1):
- "Write failing tests for the feature" — owner: implementer
- "Challenge test strategy and coverage" — owner: devils-advocate, blockedBy: [write tests task]

**Phase 3 — Implement** (blockedBy all Phase 2):
- "Implement feature to pass tests" — owner: implementer
- "Review implementation for bugs and quality" — owner: reviewer, blockedBy: [implement task]
- "Stress-test implementation" — owner: devils-advocate, blockedBy: [implement task]

**Phase 4 — Verify** (blockedBy all Phase 3):
- "Rebuild Docker and run full test suite" — owner: implementer
- "Validate end-to-end MCP tool integration" — owner: reviewer, blockedBy: [rebuild task]

**Phase 5 — Document** (blockedBy all Phase 4):
- "Update affected skills and README documentation" — owner: implementer
- "Verify documentation matches implementation" — owner: reviewer, blockedBy: [update docs task]

### Step 3: Spawn Teammates

Spawn all three teammates in parallel using the `Task` tool:

**implementer:**
```
name: "implementer"
subagent_type: "general-purpose"
model: "sonnet"
mode: "plan"
team_name: "vire-develop"
run_in_background: true
prompt: |
  You are the implementer on a development team. You write code and tests.

  Your team name is "vire-develop". Read your tasks from the task list with TaskList.
  Claim tasks assigned to you (owner: "implementer") by setting status to "in_progress".
  Work through tasks in ID order. Mark each completed before moving to the next.

  When you enter plan mode, write your plan and call ExitPlanMode. The team lead
  will review and approve before you proceed with implementation.

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
model: "sonnet"
team_name: "vire-develop"
run_in_background: true
prompt: |
  You are the reviewer on a development team. You review code for bugs, quality,
  and consistency with existing codebase patterns.

  Your team name is "vire-develop". Read your tasks from the task list with TaskList.
  Claim tasks assigned to you (owner: "reviewer") by setting status to "in_progress".
  Work through tasks in ID order. Mark each completed before moving to the next.

  Working directory: /home/bobmc/development/vire

  When reviewing:
  - Read the changed files and surrounding context
  - Check for bugs, logic errors, and edge cases
  - Verify consistency with existing patterns in the codebase
  - Validate test coverage is adequate
  - Report findings via SendMessage to "implementer" (for fixes) and to the team lead (for status)

  When reviewing documentation:
  - Check that README.md accurately reflects new/changed functionality
  - Check that affected skill files match the implementation
  - Verify examples and usage instructions work

  After completing each task, check TaskList for your next task.
  If all your tasks are done or blocked, send a message to the team lead.
```

**devils-advocate:**
```
name: "devils-advocate"
subagent_type: "general-purpose"
model: "sonnet"
team_name: "vire-develop"
run_in_background: true
prompt: |
  You are the devils-advocate on a development team. You critically challenge
  every decision to catch problems early.

  Your team name is "vire-develop". Read your tasks from the task list with TaskList.
  Claim tasks assigned to you (owner: "devils-advocate") by setting status to "in_progress".
  Work through tasks in ID order. Mark each completed before moving to the next.

  Working directory: /home/bobmc/development/vire

  Your job is to:
  - Challenge design choices: Are there simpler alternatives? What assumptions are being made?
  - Poke holes in test strategy: What edge cases are missing? Could tests pass with a broken implementation?
  - Stress-test implementation: Race conditions? Resource leaks? Unexpected data? Breaking existing functionality?
  - Question scope: Too broad? Too narrow? Right abstraction level?
  - Play the role of a hostile input source

  You must be convinced before any task is considered complete.
  Send findings via SendMessage to "implementer" (for action) and to the team lead (for awareness).

  After completing each task, check TaskList for your next task.
  If all your tasks are done or blocked, send a message to the team lead.
```

### Step 4: Coordinate

As team lead, your job is coordination only:

1. **Approve plans** — When the implementer submits a plan via ExitPlanMode, review it and use `SendMessage` with `type: "plan_approval_response"` to approve or reject with feedback.
2. **Relay information** — If one teammate's findings affect another, forward via `SendMessage`.
3. **Resolve conflicts** — If the devils-advocate and implementer disagree, make the call.
4. **Unblock tasks** — When a phase completes, verify all tasks in that phase are done before confirming teammates can proceed.

### Step 5: Completion

When all tasks are complete:

1. Verify the code quality checklist:
   - All new code has tests
   - All tests pass
   - No new linter warnings (`golangci-lint run`)
   - Docker container builds successfully
   - README.md updated if user-facing behaviour changed
   - Affected skill files updated
   - Devils-advocate has signed off

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

## Devils-Advocate Findings
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
| MCP Server | `cmd/vire-mcp/` |
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
