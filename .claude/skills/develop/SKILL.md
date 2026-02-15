# /vire-develop - Vire Development Workflow

Develop and test Vire server features using an agent team.

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

### Step 2: Investigate and Plan

Before creating the team, the team lead investigates the codebase directly:

1. Use the Explore agent to understand relevant files, patterns, and existing implementations
2. Determine the approach, files to change, and any risks
3. Write this into `requirements.md` (created in Step 1) under the Approach section
4. Use this knowledge to write detailed task descriptions — teammates should NOT need to re-investigate

### Step 3: Create Team and Tasks

Call `TeamCreate`:
```
team_name: "vire-develop"
description: "Developing: <feature-description>"
```

Create 7 tasks across 3 phases using `TaskCreate`. Set `blockedBy` dependencies via `TaskUpdate`.

**Phase 1 — Implement** (no dependencies):
- "Write tests and implement <feature>" — owner: implementer
  Task description includes: approach, files to change, test strategy, and acceptance criteria.
- "Review implementation and tests" — owner: reviewer, blockedBy: [implement task]
  Scope: code quality, pattern consistency, test coverage.
- "Stress-test implementation" — owner: devils-advocate, blockedBy: [implement task]
  Scope: security, failure modes, edge cases, hostile inputs.

**Phase 2 — Verify** (blockedBy: review + stress-test):
- "Build, test, and run locally" — owner: implementer
- "Validate deployment" — owner: reviewer, blockedBy: [build task]

**Phase 3 — Document** (blockedBy: validate):
- "Update affected documentation" — owner: implementer
- "Verify documentation matches implementation" — owner: reviewer, blockedBy: [update docs task]

### Step 4: Spawn Teammates

Spawn all three teammates in parallel using the `Task` tool:

**implementer:**
```
name: "implementer"
subagent_type: "general-purpose"
model: "sonnet"
mode: "bypassPermissions"
team_name: "vire-develop"
run_in_background: true
prompt: |
  You are the implementer on a development team. You write tests and code.

  Team: "vire-develop". Working directory: /home/bobmc/development/vire
  Read .claude/skills/develop/SKILL.md Reference section for conventions, tests, and deploy details.

  Workflow:
  1. Read TaskList, claim your tasks (owner: "implementer") by setting status to "in_progress"
  2. Work through tasks in ID order, mark each completed before moving to the next
  3. After each task, check TaskList for your next available task

  For implement tasks: write tests first, then implement to pass them.
  For verify tasks: run tests and deploy:
    go test ./internal/...
    VIRE_TEST_DOCKER=true go test ./...
    go vet ./...
    golangci-lint run
    ./scripts/run.sh restart
    curl -s http://localhost:4242/api/health
  For documentation tasks: update affected files in README.md and .claude/skills/vire-*/SKILL.md.

  Do NOT send status messages. Only message teammates for: blocking issues, review findings, or questions.
  Mark tasks completed via TaskUpdate — the system handles notifications.
```

**reviewer:**
```
name: "reviewer"
subagent_type: "general-purpose"
model: "haiku"
team_name: "vire-develop"
run_in_background: true
prompt: |
  You are the reviewer on a development team. You review for code quality, pattern
  consistency, test coverage, and documentation accuracy.

  Team: "vire-develop". Working directory: /home/bobmc/development/vire
  Read .claude/skills/develop/SKILL.md Reference section for conventions, tests, and deploy details.

  Workflow:
  1. Read TaskList, claim your tasks (owner: "reviewer") by setting status to "in_progress"
  2. Work through tasks in ID order, mark each completed before moving to the next
  3. After each task, check TaskList for your next available task

  When reviewing code: read changed files and surrounding context, check for bugs, verify
  consistency with existing patterns, validate test coverage is adequate.
  When reviewing docs: check accuracy against implementation, verify examples work.
  When validating deployment: confirm health endpoint responds, test key routes.

  Send findings to "implementer" via SendMessage only if fixes are needed.
  Do NOT send status messages. Mark tasks completed via TaskUpdate — the system handles notifications.
```

**devils-advocate:**
```
name: "devils-advocate"
subagent_type: "general-purpose"
model: "sonnet"
team_name: "vire-develop"
run_in_background: true
prompt: |
  You are the devils-advocate on a development team. Your scope: security vulnerabilities,
  failure modes, edge cases, and hostile inputs.

  Team: "vire-develop". Working directory: /home/bobmc/development/vire
  Read .claude/skills/develop/SKILL.md Reference section for conventions, tests, and deploy details.

  Workflow:
  1. Read TaskList, claim your tasks (owner: "devils-advocate") by setting status to "in_progress"
  2. Work through tasks in ID order, mark each completed before moving to the next
  3. After each task, check TaskList for your next available task

  Stress-test the implementation: input validation, injection attacks, broken auth flows,
  missing error states, race conditions, resource leaks, crash recovery. Write stress tests
  where appropriate. Play the role of a hostile input source.

  Send findings to "implementer" via SendMessage only if fixes are needed.
  Do NOT send status messages. Mark tasks completed via TaskUpdate — the system handles notifications.
```

### Step 5: Coordinate

As team lead, your job is lightweight coordination:

1. **Relay information** — If one teammate's findings affect another, forward via `SendMessage`.
2. **Resolve conflicts** — If the devils-advocate and implementer disagree, make the call.
3. **Apply direct fixes** — For trivial issues (typos, missing imports), fix them directly rather than round-tripping through the implementer.

### Step 6: Completion

When all tasks are complete:

1. Verify the code quality checklist:
   - All new code has tests
   - All tests pass (`go test ./...`, `VIRE_TEST_DOCKER=true go test ./...`)
   - No new linter warnings (`golangci-lint run`)
   - Go vet is clean (`go vet ./...`)
   - Server builds and runs (`./scripts/run.sh restart`)
   - Health endpoint responds (`curl -s http://localhost:4242/api/health`)
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
| Entry Point | `cmd/vire-server/` |
| Application | `internal/app/` |
| Services | `internal/services/` |
| Clients | `internal/clients/` |
| Models | `internal/models/` (includes `user.go`) |
| Config (code) | `internal/common/config.go` |
| Config (files) | `config/` |
| Signals | `internal/signals/` |
| HTTP Server | `internal/server/` (includes `handlers_user.go` for user/auth endpoints) |
| Storage | `internal/storage/` (BadgerHold for user data, file-based for market/signals) |
| Storage (badger) | `internal/storage/badger/` (BadgerHold domain stores) |
| Interfaces | `internal/interfaces/` |
| User Context | `internal/common/userctx.go` (X-Vire-* header resolution) |
| Tests | `tests/` |
| Docker | `docker/` |
| Scripts | `scripts/` |
| Skills | `.claude/skills/vire-*/` |

### Test Architecture
```
tests/
├── api/           # Integration tests
├── common/        # Test infra (containers.go, mocks.go)
├── docker/        # Docker test helpers
├── fixtures/      # Test data
└── results/       # Test output (gitignored)
```

### Test Commands

| Command | Scope |
|---------|-------|
| `go test ./internal/...` | Unit tests only |
| `VIRE_TEST_DOCKER=true go test -v ./tests/api/... -run TestName` | Single integration test |
| `VIRE_TEST_DOCKER=true go test ./...` | Full suite (unit + integration) |
| `go vet ./...` | Static analysis |
| `golangci-lint run` | Linter |

### Storage Architecture

The storage layer uses a split layout with domain-specific interfaces:

| Store | Backend | Path | Contents |
|-------|---------|------|----------|
| `badgerStore` | BadgerHold | `data/user/badger/` | Portfolios, strategies, plans, watchlists, reports, searches, KV, users |
| `dataStore` | File-based JSON | `data/data/` | Market data, signals, charts |

**User data** is stored in BadgerDB via [BadgerHold](https://github.com/timshannon/badgerhold) (`internal/storage/badger/`). Each domain has its own file (e.g., `user_storage.go`, `portfolio_storage.go`). Import the badger package as `bstore` in manager.go to avoid naming conflicts.

**User storage** (`internal/storage/badger/user_storage.go`): BadgerHold keyed by username. Accessed every request via `userContextMiddleware` when `X-Vire-User-ID` header is present — resolves all user preferences (`navexa_key`, `display_currency`, `portfolios`) from the stored profile. Headers override profile values for backward compatibility. The `UserStorage` interface provides `GetUser`, `SaveUser`, `DeleteUser`, `ListUsers`.

**Migration:** On first startup, `MigrateFromFiles` in `internal/storage/badger/migrate.go` reads old file-based JSON from `data/user/{domain}/` and inserts into BadgerDB. Old directories are renamed to `.migrated-{timestamp}`.

**Adding a new user-domain storage:** Create a new file in `internal/storage/badger/` following the existing pattern (e.g., `user_storage.go`). Key operations: `store.db.Get(key, &dest)`, `store.db.Upsert(key, &value)`, `store.db.Delete(key, Type{})`, `store.db.Find(&slice, nil)`. Check `badgerhold.ErrNotFound` for not-found errors. Wire into `Manager` in `manager.go` and add the accessor to the `StorageManager` interface in `internal/interfaces/storage.go`.

**Adding a new data-domain storage (market/signals):** Follow the `marketDataStorage` pattern in `internal/storage/file.go` — create a struct wrapping `FileStore`, implement the interface methods.

### User & Auth Endpoints

| Endpoint | Method | Handler File |
|----------|--------|-------------|
| `/api/users` | POST | `handlers_user.go` — create user (bcrypt password) |
| `/api/users/{id}` | GET/PUT/DELETE | `handlers_user.go` — CRUD via `routeUsers` dispatch |
| `/api/users/import` | POST | `handlers_user.go` — bulk import (idempotent) |
| `/api/auth/login` | POST | `handlers_user.go` — credential verification |

User model includes `display_currency`, `default_portfolio`, and `portfolios` fields. Passwords are bcrypt-hashed (cost 10, 72-byte truncation). GET responses mask `password_hash` entirely and return `navexa_key_set` (bool) + `navexa_key_preview` (last 4 chars) instead of the raw key. Login response includes preference fields.

### Middleware — User Context Resolution

`userContextMiddleware` in `internal/server/middleware.go` extracts `X-Vire-*` headers into a `UserContext` stored in request context. When `X-Vire-User-ID` is present, the middleware resolves all user preferences from the stored profile (navexa_key, display_currency, portfolios). Individual headers override profile values for backward compatibility.

### Dev Mode Auto-Import

In non-production mode, `import/users.json` is imported on startup (idempotent). The shared `ImportUsersFromFile` function in `internal/app/import.go` handles both file import and is available for reuse.

### Documentation to Update

When the feature affects user-facing behaviour, update:
- `README.md` — if new tools, changed tool behaviour, or new capabilities
- `.claude/skills/vire-*/SKILL.md` — affected skill files
