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

Create 9 tasks across 4 phases using `TaskCreate`. Set `blockedBy` dependencies via `TaskUpdate`.

**Phase 1 — Implement** (no dependencies):
- "Write unit tests and implement <feature>" — owner: implementer
  Task description includes: approach, files to change, test strategy, and acceptance criteria.
  Unit tests go in `internal/` alongside the code (e.g. `*_test.go`).
- "Review implementation and tests" — owner: reviewer, blockedBy: [implement task]
  Scope: code quality, pattern consistency, test coverage.
- "Stress-test implementation" — owner: devils-advocate, blockedBy: [implement task]
  Scope: security, failure modes, edge cases, hostile inputs.

**Phase 2 — Integration Tests** (blockedBy: review + stress-test):
- "Create API and data integration tests" — owner: test-creator, blockedBy: [review + stress-test tasks]
  Creates tests in `tests/api/` and/or `tests/data/` following `.claude/skills/test-create-review`.
  Must comply with `.claude/skills/test-common` mandatory rules.
- "Execute integration tests" — owner: test-executor, blockedBy: [test-create task]
  Runs tests following `.claude/skills/test-execute`.
  **Feedback loop:** If tests fail, sends failure details to "implementer" via SendMessage.
  The implementer fixes the issues. The test-executor re-runs until tests pass.
  Task is only marked complete when all tests pass (or failures are documented as known issues).

**Phase 3 — Verify** (blockedBy: test execution):
- "Build, test, and run locally" — owner: implementer, blockedBy: [test execution task]
- "Validate deployment" — owner: reviewer, blockedBy: [build task]

**Phase 4 — Document** (blockedBy: validate):
- "Update affected documentation" — owner: implementer
- "Verify documentation matches implementation" — owner: reviewer, blockedBy: [update docs task]

### Step 4: Spawn Teammates

Spawn all five teammates in parallel using the `Task` tool:

**implementer:**
```
name: "implementer"
subagent_type: "general-purpose"
model: "opus"
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
    go test ./...
    go vet ./...
    golangci-lint run
    ./scripts/run.sh restart
    curl -s http://localhost:8501/api/health
  For documentation tasks: update affected files in README.md and .claude/skills/vire-*/SKILL.md.

  Do NOT send status messages. Only message teammates for: blocking issues, review findings, or questions.
  Mark tasks completed via TaskUpdate — the system handles notifications.
```

**reviewer:**
```
name: "reviewer"
subagent_type: "general-purpose"
model: "sonnet"
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
model: "opus"
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

**test-creator:**
```
name: "test-creator"
subagent_type: "general-purpose"
model: "sonnet"
mode: "bypassPermissions"
team_name: "vire-develop"
run_in_background: true
prompt: |
  You are the test-creator on a development team. You create API and data integration
  tests in ./tests/ following the project's test conventions.

  Team: "vire-develop". Working directory: /home/bobmc/development/vire

  IMPORTANT: Before writing any tests, read these skills in order:
  1. .claude/skills/test-common/SKILL.md — mandatory rules all tests must follow
  2. .claude/skills/test-create-review/SKILL.md — templates, patterns, and compliance checklist

  Workflow:
  1. Read TaskList, claim your tasks (owner: "test-creator") by setting status to "in_progress"
  2. Work through tasks in ID order, mark each completed before moving to the next
  3. After each task, check TaskList for your next available task

  For test creation tasks:
  - Read the implementation files to understand what was built
  - Determine which test layers are needed (API tests in tests/api/, data tests in tests/data/)
  - Create tests following the templates in test-create-review SKILL.md
  - API tests: use common.NewEnv(t), HTTPGet/Post/Put/Delete helpers, OutputGuard
  - Data tests: use testManager(t), lifecycle patterns (Create -> Read -> Update -> Delete)
  - All tests must comply with test-common mandatory rules (independent, containerized, results output)
  - Use testify/require for setup, testify/assert for assertions
  - Table-driven tests where multiple cases exist

  Do NOT send status messages. Only message teammates for: blocking issues or questions.
  Mark tasks completed via TaskUpdate — the system handles notifications.
```

**test-executor:**
```
name: "test-executor"
subagent_type: "general-purpose"
model: "sonnet"
mode: "bypassPermissions"
team_name: "vire-develop"
run_in_background: true
prompt: |
  You are the test-executor on a development team. You run tests and report results.
  You MUST NOT modify test files — you are read-only for test code.

  Team: "vire-develop". Working directory: /home/bobmc/development/vire

  IMPORTANT: Before executing tests, read these skills:
  1. .claude/skills/test-common/SKILL.md — mandatory rules for validation
  2. .claude/skills/test-execute/SKILL.md — execution workflow and reporting format

  Workflow:
  1. Read TaskList, claim your tasks (owner: "test-executor") by setting status to "in_progress"
  2. Work through tasks in ID order, mark each completed before moving to the next
  3. After each task, check TaskList for your next available task

  For test execution tasks:
  - First validate test structure compliance (Rules 1-4 from test-common)
  - Run the tests created by the test-creator:
    go test ./tests/data/... -v -timeout 300s
    go test ./tests/api/... -v -timeout 300s
  - Also run unit tests to catch regressions:
    go test ./internal/... -timeout 120s

  FEEDBACK LOOP — this is critical:
  - If tests PASS: mark your task as completed with results in the description
  - If tests FAIL: send failure details to "implementer" via SendMessage with:
    - Which tests failed
    - The error output
    - Which files likely need fixing
    Then WAIT for the implementer to fix the issues. After receiving confirmation,
    re-run the failing tests. Repeat until all tests pass or you've done 3 rounds
    (then mark task complete with remaining failures documented).

  Do NOT modify test files. Do NOT send status messages.
  Mark tasks completed via TaskUpdate — the system handles notifications.
```

### Step 5: Coordinate

As team lead, your job is lightweight coordination:

1. **Relay information** — If one teammate's findings affect another, forward via `SendMessage`.
2. **Resolve conflicts** — If the devils-advocate and implementer disagree, make the call.
3. **Apply direct fixes** — For trivial issues (typos, missing imports), fix them directly rather than round-tripping through the implementer.
4. **Test feedback loop** — When the test-executor reports failures, ensure the implementer receives the details and fixes the issues. Monitor the fix-retest cycle (max 3 rounds). If the implementer and test-executor are communicating directly, let them work — only intervene if the cycle stalls.

### Step 6: Completion

When all tasks are complete:

1. Verify the code quality checklist:
   - All new code has unit tests (`internal/`)
   - Integration tests created in `tests/` (API and/or data layer)
   - All tests pass (`go test ./...`)
   - Integration test results saved to `tests/logs/`
   - No new linter warnings (`golangci-lint run`)
   - Go vet is clean (`go vet ./...`)
   - Server builds and runs (`./scripts/run.sh restart`)
   - Health endpoint responds (`curl -s http://localhost:8501/api/health`)
   - README.md updated if user-facing behaviour changed
   - Affected skill files updated
   - Devils-advocate has signed off
   - Test-executor has signed off (all integration tests pass)

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
- <unit tests added or modified>
- <integration tests created in tests/>
- <test results: pass/fail>
- <test feedback rounds: N (if any failures required fixes)>

## Documentation Updated
- <list of docs/skills/README changes>

## Devils-Advocate Findings
- <key issues raised and how they were resolved>

## Notes
- <anything notable: trade-offs, follow-up work, risks>
```

3. Shut down all five teammates:
   ```
   SendMessage type: "shutdown_request" to each teammate
   (implementer, reviewer, devils-advocate, test-creator, test-executor)
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
| Job Manager | `internal/services/jobmanager/` (queue-based background data collection) |
| Clients | `internal/clients/` |
| Models | `internal/models/` (includes `storage.go` for InternalUser, UserKeyValue, UserRecord; `jobs.go` for StockIndexEntry, Job, JobEvent) |
| Config (code) | `internal/common/config.go` (includes `JobManagerConfig`) |
| Config (files) | `config/` |
| Signals | `internal/signals/` |
| HTTP Server | `internal/server/` (includes `handlers_user.go` for user/auth, `handlers_auth.go` for OAuth/JWT, `handlers_admin.go` for admin API) |
| Storage | `internal/storage/` (manager, migration) |
| Storage (SurrealDB) | `internal/storage/surrealdb/` (all persistent data) |
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
├── docker/        # Docker test helpers (.env.example for required env vars)
├── fixtures/      # Test data
├── import/        # Import data (users.json)
└── results/       # Test output (gitignored)
```

### Test Commands

| Command | Scope |
|---------|-------|
| `go test ./internal/...` | Unit tests only |
| `go test -v ./tests/api/... -run TestName` | Single integration test |
| `go test ./...` | Full suite (unit + integration) |
| `go test ./tests/api/... -run TestPortfolioWorkflow -v -timeout 300s` | Portfolio workflow test (loads `tests/docker/.env`) |
| `go vet ./...` | Static analysis |
| `golangci-lint run` | Linter |

### Storage Architecture

The storage layer uses a 3-area layout with separate databases per concern:

| Store | Backend | Package | Path | Contents |
|-------|---------|---------|------|----------|
| `InternalStore` | BadgerHold | `internal/storage/internaldb/` | `data/internal/` | User accounts (`InternalUser`), per-user config KV (`UserKeyValue`), system KV |
| `UserDataStore` | BadgerHold | `internal/storage/userdb/` | `data/user/` | Generic `UserRecord` — all user domain data (portfolio, strategy, plan, watchlist, report, search) |
| `MarketFS` | File-based JSON | `internal/storage/marketfs/` | `data/market/` | Market data, signals, charts |

**InternalStore** (`internal/storage/internaldb/`): BadgerHold keyed by username. Stores `InternalUser` (user_id, email, password_hash, role, created_at, modified_at) and `UserKeyValue` (user_id, key, value, version, datetime). User preferences (`navexa_key`, `display_currency`, `portfolios`) are stored as KV entries. Accessed every request via `userContextMiddleware` when `X-Vire-User-ID` header is present. The `InternalStore` interface provides `GetUser`, `SaveUser`, `DeleteUser`, `ListUsers`, `GetUserKV`, `SetUserKV`, `DeleteUserKV`, `ListUserKV`, `GetSystemKV`, `SetSystemKV`.

**UserDataStore** (`internal/storage/userdb/`): BadgerHold storing generic `UserRecord` (user_id, subject, key, value, version, datetime). Services marshal/unmarshal domain types to/from the `value` field as JSON. The `UserDataStore` interface provides `Get`, `Put`, `Delete`, `List`, `Query`, `DeleteBySubject`. Subjects: `portfolio`, `strategy`, `plan`, `watchlist`, `report`, `search`.

**MarketFS** (`internal/storage/marketfs/`): File-based JSON with atomic writes (temp file + rename). Implements `MarketDataStorage` and `SignalStorage` interfaces.

**Migration:** On first startup, `MigrateOldLayout` in `internal/storage/migrate.go` reads data from the old single-BadgerDB layout and splits it into the 3-area layout. Old directories are renamed to `.migrated-{timestamp}`.

**Adding new user domain data:** Add records via `UserDataStore.Put` with a new `subject` string. No new storage files needed — just marshal your domain type to JSON and store as a `UserRecord`.

**Adding new market/signal data:** Follow the existing `MarketFS` pattern in `internal/storage/marketfs/` — file-based JSON with `FileStore` wrapper.

**StockIndexStore** (`internal/storage/surrealdb/stockindex.go`): SurrealDB-backed registry of all tracked stocks. Each `StockIndexEntry` contains ticker, code, exchange, name, source, and per-component freshness timestamps (eod_collected_at, fundamentals_collected_at, filings_collected_at, news_collected_at, filing_summaries_collected_at, timeline_collected_at, signals_collected_at). Keyed by ticker (dots replaced with underscores for SurrealDB record IDs via `tickerToID()`). The `StockIndexStore` interface provides `Upsert`, `Get`, `List`, `UpdateTimestamp`, `Delete`. Upsert preserves existing timestamps on re-insertion (only updates LastSeenAt and Source). UpdateTimestamp validates field names against a whitelist.

**JobQueueStore** (`internal/storage/surrealdb/jobqueue.go`): Persistent priority job queue backed by SurrealDB. Each `Job` has ID, job_type, ticker, priority, status, timestamps, error, attempt count. Dequeue uses atomic `UPDATE ... WHERE status = 'pending' ORDER BY priority DESC, created_at ASC LIMIT 1 RETURN AFTER` for concurrent-safe job claiming. The `JobQueueStore` interface provides `Enqueue`, `Dequeue`, `Complete`, `Cancel`, `SetPriority`, `GetMaxPriority`, `ListPending`, `ListAll`, `ListByTicker`, `CountPending`, `HasPendingJob`, `PurgeCompleted`, `CancelByTicker`.

### User & Auth Endpoints

| Endpoint | Method | Handler File |
|----------|--------|-------------|
| `/api/users` | POST | `handlers_user.go` — create user (bcrypt password) |
| `/api/users/upsert` | POST | `handlers_user.go` — create or update user (merge semantics) |
| `/api/users/check/{username}` | GET | `handlers_user.go` — check username availability |
| `/api/users/{id}` | GET/PUT/DELETE | `handlers_user.go` — CRUD via `routeUsers` dispatch |
| `/api/auth/login` | POST | `handlers_user.go` — credential verification (returns JWT token) |
| `/api/auth/password-reset` | POST | `handlers_user.go` — reset user password (bcrypt hash) |
| `/api/auth/oauth` | POST | `handlers_auth.go` — exchange OAuth code for JWT (providers: `dev`, `google`, `github`) |
| `/api/auth/validate` | POST | `handlers_auth.go` — validate JWT from `Authorization: Bearer` header |
| `/api/auth/login/google` | GET | `handlers_auth.go` — redirect to Google OAuth consent screen |
| `/api/auth/login/github` | GET | `handlers_auth.go` — redirect to GitHub OAuth consent screen |
| `/api/auth/callback/google` | GET | `handlers_auth.go` — Google OAuth callback, redirects to portal with JWT |
| `/api/auth/callback/github` | GET | `handlers_auth.go` — GitHub OAuth callback, redirects to portal with JWT |

User model (`InternalUser`) contains `user_id`, `email`, `password_hash`, `provider`, `role`, `created_at`, `modified_at`. The `provider` field tracks the authentication source: `"email"`, `"google"`, `"github"`, or `"dev"`. Preferences (`display_currency`, `portfolios`, `navexa_key`) are stored as per-user KV entries in InternalStore. Passwords are bcrypt-hashed (cost 10, 72-byte truncation). GET responses mask `password_hash` entirely and return `navexa_key_set` (bool) + `navexa_key_preview` (last 4 chars) instead of the raw key. Login response includes preference fields from KV and a JWT token.

### Auth Config

The `[auth]` section in `config/vire-service.toml` configures JWT signing and OAuth providers:

```toml
[auth]
jwt_secret = "change-me-in-production"
token_expiry = "24h"

[auth.google]
client_id = ""
client_secret = ""

[auth.github]
client_id = ""
client_secret = ""
```

Config types: `AuthConfig` (JWTSecret, TokenExpiry, Google, GitHub), `OAuthProvider` (ClientID, ClientSecret). Env overrides: `VIRE_AUTH_JWT_SECRET`, `VIRE_AUTH_TOKEN_EXPIRY`, `VIRE_AUTH_GOOGLE_CLIENT_ID`, `VIRE_AUTH_GOOGLE_CLIENT_SECRET`, `VIRE_AUTH_GITHUB_CLIENT_ID`, `VIRE_AUTH_GITHUB_CLIENT_SECRET`.

JWT tokens are HMAC-SHA256 signed using `github.com/golang-jwt/jwt/v5`. Claims include: `sub` (user_id), `email`, `name`, `provider`, `iss` ("vire-server"), `iat`, `exp`. The `dev` provider is blocked in production mode via `config.IsProduction()`.

OAuth state parameters use HMAC-signed base64 payloads with a 10-minute expiry for CSRF protection.

### Middleware — User Context Resolution

`userContextMiddleware` in `internal/server/middleware.go` takes an `InternalStore` and extracts `X-Vire-*` headers into a `UserContext` stored in request context. When `X-Vire-User-ID` is present, the middleware resolves all user preferences from `ListUserKV` (navexa_key, display_currency, portfolios). Individual headers override profile values for backward compatibility.

### Job Manager

The job manager (`internal/services/jobmanager/`) is a queue-driven background service with three components:

**Architecture:**
- **Watcher** (`watcher.go`): Scans the stock index on a configurable interval (default 1m). For each tracked ticker, checks per-component freshness timestamps against TTLs from `common.Freshness*` constants. Enqueues jobs for stale components using `HasPendingJob` for deduplication. New stocks (added < 5min ago) get elevated priority (`PriorityNewStock = 15`). EOD collection is grouped per-exchange: instead of individual `collect_eod` jobs per ticker, the watcher collects exchanges with stale EOD tickers and enqueues one `collect_eod_bulk` job per exchange.
- **Processor Pool** (`manager.go`): N concurrent goroutines (configurable via `max_concurrent`, default 5) that continuously dequeue jobs from the priority queue and execute them.
- **Executor** (`executor.go`): Dispatches jobs by type to the corresponding `MarketService` method (CollectEOD, CollectBulkEOD, CollectFundamentals, CollectFilings, etc.). On completion, updates the stock index freshness timestamp. Bulk EOD jobs pass the exchange code (stored in `job.Ticker`) to `CollectBulkEOD`; timestamp updates are handled per-ticker internally by `CollectBulkEOD` rather than by the executor.
- **Queue** (`queue.go`): Thin wrappers around `JobQueueStore` that broadcast `JobEvent` messages via WebSocket on enqueue/start/complete/fail. Provides `PushToTop` (sets priority to max + 1) and `enqueueIfNeeded` (dedup check + enqueue).
- **WebSocket Hub** (`websocket.go`): gorilla/websocket-based hub broadcasting `JobEvent` (queued/started/completed/failed) to connected admin clients. Served at `/api/admin/ws/jobs`.

**Constructor:** `NewJobManager(market, signal, storage, logger, config)` — no longer takes a portfolio service parameter. The job manager operates on the stock index, not on portfolios directly.

**Flow:**
1. Portfolio sync upserts tickers to the stock index (`portfolio/service.go`)
2. Watcher scans stock index, enqueues jobs for stale data
3. Processor pool dequeues by priority (highest first), executes via MarketService
4. Admin API allows manual enqueue, priority changes, and cancellation
5. WebSocket broadcasts real-time job events to admin clients

**Legacy compat:** `LastJobRun()` in `jobs.go` still supports the `/api/jobs/status` endpoint by reading from system KV.

Config (`config/vire-service.toml`):
```toml
[jobmanager]
enabled = true
watcher_interval = "1m"
max_concurrent = 5
max_retries = 3
purge_after = "24h"
```

Config type: `JobManagerConfig` in `internal/common/config.go` with `Enabled`, `WatcherInterval` (string duration), `MaxConcurrent`, `MaxRetries`, `PurgeAfter` (string duration). Methods: `GetWatcherInterval()`, `GetMaxRetries()`, `GetPurgeAfter()`.

**Job Types** (defined in `internal/models/jobs.go`):
| Constant | Value | Default Priority |
|----------|-------|-----------------|
| `JobTypeCollectEOD` | `collect_eod` | 10 |
| `JobTypeCollectEODBulk` | `collect_eod_bulk` | 10 |
| `JobTypeCollectFundamentals` | `collect_fundamentals` | 8 |
| `JobTypeCollectFilings` | `collect_filings` | 5 |
| `JobTypeCollectNews` | `collect_news` | 5 |
| `JobTypeCollectFilingSummaries` | `collect_filing_summaries` | 3 |
| `JobTypeCollectTimeline` | `collect_timeline` | 3 |
| `JobTypeCollectNewsIntelligence` | `collect_news_intelligence` | 3 |
| `JobTypeComputeSignals` | `compute_signals` | 7 |

**Priority Constants:**
| Constant | Value | Usage |
|----------|-------|-------|
| `PriorityNewStock` | 15 | New stocks added to index (< 5min old) |
| `PriorityManual` | 20 | Manually enqueued via admin API |
| `PriorityUrgent` | 50 | Urgent/pushed-to-top jobs |

### Report Generation Pipeline

`GenerateReport` uses a fast path: Navexa sync + `CollectCoreMarketData` (EOD + fundamentals only) + portfolio review + build report. No filings, news, or AI summaries. Detailed data collection happens in the background via the job manager.

`GenerateTickerReport` (single-ticker refresh) also uses `CollectCoreMarketData` (EOD + fundamentals only) for the targeted ticker, consistent with the `GenerateReport` fast path.

**Report Markdown Structure:** Stock and ETF reports wrap EODHD-sourced data (fundamentals, fund metrics, technical signals) under a `## EODHD Market Analysis` parent section. Fundamentals sub-sections (Valuation, Profitability, etc.) use `####` headings. Technical Signals uses `###`. Non-EODHD sections (News Intelligence, Risk Flags, etc.) remain at `##` level.

### Portfolio Review Response

The `POST /api/portfolios/{name}/review` handler returns a slim response that strips heavy analysis data. `ReviewPortfolio` still computes everything internally (needed for action/compliance determination), but the API response only includes position-level fields.

**Kept fields per holding:** `holding`, `overnight_move`, `overnight_pct`, `news_impact`, `action_required`, `action_reason`, `compliance`.

**Stripped fields:** `signals`, `fundamentals`, `news_intelligence`, `filings_intelligence`, `filing_summaries`, `timeline`.

The conversion is handled by `toSlimReview()` in `internal/server/handlers.go`, which maps `PortfolioReview` to `slimPortfolioReview`. Portfolio-level fields (totals, alerts, summary, recommendations, balance) are preserved.

### MarketService — Collection Methods

The MarketService interface (`internal/interfaces/services.go`) provides both composite and individual collection methods:

**Composite methods** (unchanged):

| Method | Scope | Used By |
|--------|-------|---------|
| `CollectMarketData` | Full: EOD + fundamentals + filings + news + AI | Job manager, manual collection |
| `CollectCoreMarketData` | Fast: EOD (bulk) + fundamentals only | `GenerateReport`, `GenerateTickerReport` |

**Individual methods** (`internal/services/market/collect.go`):

| Method | Data Collected | Used By |
|--------|---------------|---------|
| `CollectEOD(ctx, ticker)` | EOD bars (incremental merge) + signal computation | Job manager (fallback for new tickers) |
| `CollectBulkEOD(ctx, exchange, force)` | Last-day EOD bars for all tickers on an exchange via bulk API, with per-ticker merge, signal computation, and stock index timestamp updates. Falls back to `CollectEOD` for tickers with no existing EOD history. | Job manager (`collect_eod_bulk`) |
| `CollectFundamentals(ctx, ticker)` | Company fundamentals | Job manager |
| `CollectFilings(ctx, ticker)` | ASX announcements / filings | Job manager |
| `CollectNews(ctx, ticker)` | News articles | Job manager |
| `CollectFilingSummaries(ctx, ticker)` | AI-generated filing summaries (Gemini) | Job manager |
| `CollectTimeline(ctx, ticker)` | Structured company timeline | Job manager |
| `CollectNewsIntelligence(ctx, ticker)` | AI-generated news sentiment (Gemini) | Job manager |

Each individual method loads existing MarketData, checks component freshness, fetches from external API if stale, and saves. This decomposition allows the job queue to execute specific collection tasks independently with different priorities and scheduling. `CollectBulkEOD` operates at the exchange level: it lists all stock index entries for the exchange, calls `GetBulkEOD` to fetch last-day bars in a single API request, then merges each bar into the corresponding ticker's existing EOD history. Tickers with no existing EOD data fall back to individual `CollectEOD` for full 3-year history.

**GetStockData caching:** `GetStockData` serves filing summaries, company timeline, and quality assessment directly from cached `MarketData`. It does not invoke Gemini for summarization — that is handled by the job manager via `CollectFilingSummaries` and `CollectTimeline`. Quality assessment is computed on demand if fundamentals exist but no assessment is cached.

**Filing Summary Prompt Versioning:** `CollectFilingSummaries` tracks a SHA-256 hash of the prompt template (`FilingSummaryPromptHash` on `MarketData`). When the prompt changes, all summaries are regenerated automatically.

**FilingSummary struct** includes `financial_summary` (one-line financial performance with key numbers) and `performance_commentary` (notable management commentary on performance/outlook).

**QualityAssessment struct** (`internal/models/market.go`): Computed from fundamentals data, stored on both `MarketData` and `StockData`. Contains 7 scored metrics (`ROE`, `GrossMargin`, `FCFConversion`, `NetDebtToEBITDA`, `EarningsStability`, `RevenueGrowth`, `MarginTrend`), each with `Value`, `Benchmark`, `Rating` ("excellent"/"good"/"average"/"poor"), and `Score` (0-100). Also includes `RedFlags`, `Strengths`, `OverallRating` ("High Quality"/"Quality"/"Average"/"Below Average"/"Speculative"), `OverallScore` (0-100 weighted average), and `AssessedAt`. Recomputed when fundamentals are refreshed via `CollectFundamentals` or on demand in `GetStockData`.

**Filing summaries endpoint:** `GET /api/market/stocks/{ticker}/filing-summaries` returns `{ ticker, filing_summaries, quality_assessment, summary_count, last_updated }`.

**Schema version:** `SchemaVersion` in `internal/common/version.go` (currently "6"). Bumped when model structs or computation logic changes invalidate cached derived data.

### Admin API

Admin endpoints (`internal/server/handlers_admin.go`) are protected by `requireAdmin()` which checks `X-Vire-User-ID` header and verifies the user has `role = "admin"` in the InternalStore.

| Endpoint | Method | Handler | Description |
|----------|--------|---------|-------------|
| `/api/admin/jobs` | GET | `handleAdminJobs` | List jobs with optional `?ticker=`, `?status=pending`, `?limit=` filters |
| `/api/admin/jobs/queue` | GET | `handleAdminJobQueue` | List pending jobs ordered by priority with count |
| `/api/admin/jobs/enqueue` | POST | `handleAdminJobEnqueue` | Manually enqueue a job (`{job_type, ticker, priority}`) |
| `/api/admin/jobs/{id}/priority` | PUT | `handleAdminJobPriority` | Set priority (`{priority: 10}` or `{priority: "top"}` for push-to-top) |
| `/api/admin/jobs/{id}/cancel` | POST | `handleAdminJobCancel` | Cancel a pending/running job |
| `/api/admin/stock-index` | GET | `handleAdminStockIndex` | List all stock index entries with count |
| `/api/admin/stock-index` | POST | `handleAdminStockIndex` | Add/upsert a stock index entry (`{ticker, code, exchange, name}`) |
| `/api/admin/ws/jobs` | GET | `handleAdminJobsWS` | WebSocket upgrade for real-time job events |

Route dispatch: `/api/admin/jobs/{id}/*` paths are handled by `routeAdminJobs` in `routes.go`, which extracts the job ID and dispatches to priority or cancel handlers.

### Stock Index

The stock index (`stock_index` table in SurrealDB) is a shared, user-agnostic registry of all tracked stocks. It is populated automatically when:
- A portfolio is synced (`portfolio/service.go` — upserts all portfolio tickers with source "navexa")
- An admin manually adds entries via `POST /api/admin/stock-index` (source "manual")

The job manager's watcher scans the stock index periodically and enqueues jobs for any ticker whose data components are stale. This decouples data collection from individual user requests.

### Documentation to Update

When the feature affects user-facing behaviour, update:
- `README.md` — if new tools, changed tool behaviour, or new capabilities
- `.claude/skills/vire-*/SKILL.md` — affected skill files
