# /vire-develop - Vire Development Workflow (Kilocode)

Develop and test Vire server features using Kilo's sub-agent team and Todo list.

## Usage
```
/vire-develop <feature-description>
```

## Outputs

Every invocation produces documentation in `.kilocode/workdir/<datetime>-<taskname>/`:
- `requirements.md` — what was requested, scope, approach chosen
- `summary.md` — what was built, files changed, tests added, outcome

## Procedure

### Step 1: Create Work Directory

Generate the work directory path using the current datetime and a short task slug:
```
.kilocode/workdir/YYYYMMDD-HHMM-<task-slug>/
```

Example: `.kilocode/workdir/20260210-1430-arbor-logging/`

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

Before creating tasks, investigate the codebase:

1. Use Kilo's `explore` sub-agent via the `Task` tool (or native `glob`/`grep`/`read` tools) to understand relevant files, patterns, and existing implementations.
2. Determine the approach, files to change, and any risks.
3. Write this into `requirements.md` (created in Step 1) under the Approach section.
4. Use this knowledge to populate a detailed plan using the `TodoWrite` tool.

### Step 3: Create Tasks (TodoWrite)

Create a structured Todo list tracking the development phases:

1. "Implement Feature (write tests & code)"
2. "Review code quality, pattern consistency, test coverage"
3. "Stress-test implementation (security, edge cases)"
4. "Verify (build, test, run locally)"
5. "Update documentation (README, skills)"

### Step 4: Execute & Coordinate

As Kilo, coordinate the workflow:

1. **Implement:** Write code and tests directly using the `write`/`edit` tools. Follow the established testing and storage conventions.
2. **Review:** Launch a `general` sub-agent using the `Task` tool, asking it to review the newly implemented files for bugs, pattern consistency, and test coverage.
3. **Stress-test (Devils-Advocate):** Launch another `general` sub-agent using the `Task` tool to stress-test the implementation for input validation, edge cases, and hostile inputs.
4. **Fix Issues:** Address the findings from your sub-agents directly. Update the code to fix any problems discovered.

### Step 5: Completion

When all tasks are complete, verify the code quality checklist:
- All new code has tests
- All tests pass (`go test ./...`, `VIRE_TEST_DOCKER=true go test ./...`)
- No new linter warnings (`golangci-lint run`)
- Go vet is clean (`go vet ./...`)
- Server builds and runs (`./scripts/run.sh restart`)
- Health endpoint responds (`curl -s http://localhost:8501/api/health`)
- README.md updated if user-facing behaviour changed
- Affected skill files updated

Write `summary.md` in the work directory:

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
- <key issues raised by sub-agents and how they were resolved>

## Notes
- <anything notable: trade-offs, follow-up work, risks>
```

Mark tasks complete in the Todo list as you finish them. Provide the user with a final summary.

## Reference

### Key Directories

| Component | Location |
|-----------|----------|
| Entry Point | `cmd/vire-server/` |
| Application | `internal/app/` |
| Services | `internal/services/` |
| Clients | `internal/clients/` |
| Models | `internal/models/` (includes `storage.go` for InternalUser, UserKeyValue, UserRecord) |
| Config (code) | `internal/common/config.go` |
| Config (files) | `config/` |
| Signals | `internal/signals/` |
| HTTP Server | `internal/server/` (includes `handlers_user.go` for user/auth, `handlers_auth.go` for OAuth/JWT) |
| Storage | `internal/storage/` (manager, migration) |
| Storage (internal) | `internal/storage/internaldb/` (BadgerHold: user accounts, config KV, system KV) |
| Storage (user data) | `internal/storage/userdb/` (BadgerHold: generic UserRecord for all domain data) |
| Storage (market) | `internal/storage/marketfs/` (file-based: market data, signals, charts) |
| Interfaces | `internal/interfaces/` |
| User Context | `internal/common/userctx.go` (X-Vire-* header resolution) |
| Tests | `tests/` |
| Docker | `docker/` |
| Scripts | `scripts/` |
| Skills | `.claude/skills/vire-*/` or `.kilocode/workflows/` |

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

### Documentation to Update

When the feature affects user-facing behaviour, update:
- `README.md` — if new tools, changed tool behaviour, or new capabilities
- `.claude/skills/vire-*/SKILL.md` or `.kilocode/workflows/*.md` — affected skill files