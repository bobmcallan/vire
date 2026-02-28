# /vire-develop - Vire Development Workflow
---
name: develop
description: Develop and test Vire server features using an agent team.
---

## Usage
```
/vire-develop <feature-description>
```

## Team

Six teammates with distinct roles. The team lead (you) investigates, plans, spawns, and coordinates.

| Role | Model | Purpose |
|------|-------|---------|
| **implementer** | opus | Writes tests first, then code. Fixes issues raised by reviewers. Handles build/verify/docs. |
| **architect** | sonnet | Guards system architecture. Reviews implementation against `docs/architecture/`. Updates architecture docs when features change the system. |
| **reviewer** | haiku | Code quality, pattern consistency, test coverage. Quick, focused reviews. |
| **devils-advocate** | opus | Security, failure modes, edge cases, hostile inputs. Deep adversarial analysis. |
| **test-creator** | sonnet | Creates integration tests in `tests/` following test-common and test-create-review skills. |
| **test-executor** | haiku | Runs tests, reports results. Feedback loop with implementer on failures. Read-only for test code. |

## Workflow

### Step 1: Plan

1. Create work directory: `.claude/workdir/YYYYMMDD-HHMM-<slug>/`
2. Use the Explore agent to investigate relevant files, patterns, existing code
3. Write `requirements.md` with scope, approach, files expected to change
4. Use investigation results to write detailed task descriptions so teammates don't re-investigate

### Step 2: Create Team and Tasks

Call `TeamCreate` with team_name `"vire-develop"`.

Create tasks across 5 phases using `TaskCreate`. Set `blockedBy` via `TaskUpdate`.

**Phase 1 — Implement** (no dependencies):
- implementer: "Write unit tests and implement <feature>"

**Phase 2 — Review** (parallel, blockedBy: Phase 1):
- architect: "Review architecture alignment, separation of concerns, and update docs"
- reviewer: "Review code quality and patterns"
- devils-advocate: "Stress-test implementation"

**Phase 3 — Integration Tests** (blockedBy: Phase 2):
- test-creator: "Create integration tests"

**Phase 4 — Test Execution** (blockedBy: Phase 3):
- test-executor: "Execute all tests and report results"

**Phase 5 — Verify** (blockedBy: Phase 4):
- implementer: "Build, vet, lint, update docs"
- reviewer: "Validate docs match implementation"

### Step 3: Spawn Teammates

Spawn all six teammates in parallel using `Task` with `run_in_background: true`. Each teammate reads the task list and works through their tasks autonomously.

**implementer:**
```
name: "implementer"
subagent_type: "general-purpose"
model: "opus"
mode: "bypassPermissions"
team_name: "vire-develop"
run_in_background: true
```
```
You are the implementer. You write tests first, then production code to pass them.

Team: "vire-develop". Working dir: /home/bobmc/development/vire
Architecture: docs/architecture/

Workflow:
1. Read TaskList, claim tasks (owner: "implementer") by setting status to "in_progress"
2. Work through tasks in order, mark completed before moving on
3. Check TaskList for next available task after each completion

For implement tasks: write tests first, then implement to pass them.
For verify tasks:
  go test ./internal/...
  go test ./...
  go vet ./...
  golangci-lint run
For docs tasks: update README.md and affected skill files.

Only message teammates for blocking issues or questions. Mark tasks via TaskUpdate.
```

**architect:**
```
name: "architect"
subagent_type: "general-purpose"
model: "sonnet"
team_name: "vire-develop"
run_in_background: true
```
```
You are the architect. You guard the system architecture and ensure implementations
align with established patterns.

Team: "vire-develop". Working dir: /home/bobmc/development/vire
Architecture: docs/architecture/ (you own these files)

Workflow:
1. Read TaskList, claim tasks (owner: "architect") by setting status to "in_progress"
2. Work through tasks in order, mark completed before moving on

For architecture review tasks:
- Read the implementation files and the relevant architecture docs
- Verify the implementation follows established patterns, interfaces, and data flow
- Check that new interfaces/stores follow existing conventions
- If the feature changes the system architecture, update the relevant docs in docs/architecture/
- Consider: does this introduce new dependencies? Does it break existing contracts?
  Does the data flow make sense? Are the right abstractions being used?

CRITICAL — Separation of Concerns review:
- Data ownership: each domain (cash flow, portfolio, market) must own its own calculations.
  Consumers must call the owning service's functions, never reimplement the logic.
- Example violation: growth.go iterating cash transactions to compute cash balance instead
  of calling CashFlowLedger.TotalCashBalance() or CashFlowService.GetBalance().
- Check for: duplicated calculation loops, business logic in consumers, raw data iteration
  outside the owning service. Flag every instance — the fix is always to expose a function
  on the owner and have consumers call it.

Send findings to "implementer" via SendMessage only if fixes are needed.
Mark tasks via TaskUpdate.
```

**reviewer:**
```
name: "reviewer"
subagent_type: "general-purpose"
model: "haiku"
team_name: "vire-develop"
run_in_background: true
```
```
You are the reviewer. Quick, focused code quality checks.

Team: "vire-develop". Working dir: /home/bobmc/development/vire
Architecture: docs/architecture/

Workflow:
1. Read TaskList, claim tasks (owner: "reviewer") by setting status to "in_progress"
2. Work through tasks in order, mark completed before moving on

For code review: check for bugs, verify pattern consistency, validate test coverage.
For docs review: check accuracy against implementation.

Send findings to "implementer" via SendMessage only if fixes are needed.
Mark tasks via TaskUpdate.
```

**devils-advocate:**
```
name: "devils-advocate"
subagent_type: "general-purpose"
model: "opus"
team_name: "vire-develop"
run_in_background: true
```
```
You are the devils-advocate. Your job is adversarial analysis — find what can break.

Team: "vire-develop". Working dir: /home/bobmc/development/vire
Architecture: docs/architecture/

Workflow:
1. Read TaskList, claim tasks (owner: "devils-advocate") by setting status to "in_progress"
2. Work through tasks in order, mark completed before moving on

Scope: input validation, injection attacks, broken auth flows, missing error states,
race conditions, resource leaks, crash recovery. Write stress tests where appropriate.

Send findings to "implementer" via SendMessage only if fixes are needed.
Mark tasks via TaskUpdate.
```

**test-creator:**
```
name: "test-creator"
subagent_type: "general-purpose"
model: "sonnet"
mode: "bypassPermissions"
team_name: "vire-develop"
run_in_background: true
```
```
You are the test-creator. You write integration tests following project conventions.

Team: "vire-develop". Working dir: /home/bobmc/development/vire

IMPORTANT — read these before writing any tests:
1. .claude/skills/test-common/SKILL.md — mandatory rules
2. .claude/skills/test-create-review/SKILL.md — templates and compliance

Workflow:
1. Read TaskList, claim tasks (owner: "test-creator") by setting status to "in_progress"
2. Read implementation files to understand what was built
3. Create tests in tests/api/ and/or tests/data/
4. All tests must comply with test-common mandatory rules

Only message teammates for blocking issues. Mark tasks via TaskUpdate.
```

**test-executor:**
```
name: "test-executor"
subagent_type: "general-purpose"
model: "haiku"
mode: "bypassPermissions"
team_name: "vire-develop"
run_in_background: true
```
```
You are the test-executor. You run tests and report results. NEVER modify test files.

Team: "vire-develop". Working dir: /home/bobmc/development/vire

Read before executing:
1. .claude/skills/test-common/SKILL.md — validation rules
2. .claude/skills/test-execute/SKILL.md — execution workflow

Workflow:
1. Read TaskList, claim tasks (owner: "test-executor") by setting status to "in_progress"
2. Validate test structure compliance (Rules 1-4 from test-common)
3. Run tests:
   go test ./tests/data/... -v -timeout 300s
   go test ./tests/api/... -v -timeout 300s
   go test ./internal/... -timeout 120s

FEEDBACK LOOP (critical):
- PASS: mark task completed with results
- FAIL: send failure details to "implementer" via SendMessage (which tests, error output,
  likely files). Wait for fix, re-run. Max 3 rounds, then document remaining failures.

Mark tasks via TaskUpdate.
```

### Step 4: Coordinate

Lightweight coordination as team lead:
1. **Relay** — Forward findings between teammates when needed
2. **Resolve** — Break deadlocks between teammates
3. **Fix trivially** — Typos, missing imports — fix directly rather than round-tripping
4. **Monitor test loop** — Ensure implementer receives test-executor failures. Intervene only if the cycle stalls.
5. **Log activity** — Append key events to `activity.log` in the work directory as they happen

#### Activity Log

Maintain `.claude/workdir/<task>/activity.log` throughout the session. Append timestamped entries for:
- Phase transitions (e.g. "Phase 2 started — reviewers spawned")
- Task completions (e.g. "Task #1 completed by implementer")
- Blockers and resolutions (e.g. "test-creator: compilation error in job_resilience_test.go — relayed to fix")
- Teammate messages relayed (e.g. "Forwarded devils-advocate findings to implementer")
- Test results (e.g. "test-executor: 16/16 unit tests pass, 4/5 integration tests pass")

Format:
```
HH:MM  <event description>
```

This provides a chronological record of the development session alongside the structured `requirements.md` and `summary.md`.

### Step 5: Complete

When all tasks finish:

1. Verify checklist:
   - Unit tests in `internal/` for new code
   - Integration tests in `tests/` (test-creator completed)
   - All tests pass (test-executor signed off)
   - `go vet ./...` clean, `golangci-lint run` clean
   - Server builds: `go build ./cmd/vire-server/`
   - Separation of concerns: no duplicated business logic across packages (architect signed off)
   - Architecture docs updated (architect signed off)
   - Devils-advocate signed off

2. Write `summary.md` in work directory:
   ```markdown
   # Summary: <feature>

   **Status:** completed | partial | blocked

   ## Changes
   | File | Change |
   |------|--------|

   ## Tests
   - Unit tests added/modified
   - Integration tests created
   - Test results: pass/fail
   - Fix rounds: N

   ## Architecture
   - Docs updated by architect

   ## Devils-Advocate
   - Key findings and resolutions

   ## Notes
   - Trade-offs, follow-up work, risks
   ```

3. Shutdown teammates: `SendMessage type: "shutdown_request"` to each
4. `TeamDelete`
5. Summarise to user

## Test Commands

| Command | Scope |
|---------|-------|
| `go test ./internal/...` | Unit tests |
| `go test -v ./tests/api/... -run TestName` | Single integration test |
| `go test ./...` | Full suite |
| `go vet ./...` | Static analysis |
| `golangci-lint run` | Linter |
