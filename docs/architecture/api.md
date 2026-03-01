# API Surface

## User & Auth Endpoints

| Endpoint | Method | Handler |
|----------|--------|---------|
| `/api/users` | POST | `handlers_user.go` — create user |
| `/api/users/upsert` | POST | `handlers_user.go` — create or update |
| `/api/users/check/{username}` | GET | `handlers_user.go` — username availability |
| `/api/users/{id}` | GET/PUT/DELETE | `handlers_user.go` — CRUD |
| `/api/auth/login` | POST | `handlers_user.go` — credential verification (JWT) |
| `/api/auth/password-reset` | POST | `handlers_user.go` — reset password |
| `/api/auth/oauth` | POST | `handlers_auth.go` — exchange OAuth code for JWT |
| `/api/auth/validate` | POST | `handlers_auth.go` — validate JWT |
| `/api/auth/login/google` | GET | `handlers_auth.go` — Google OAuth redirect |
| `/api/auth/login/github` | GET | `handlers_auth.go` — GitHub OAuth redirect |
| `/api/auth/callback/google` | GET | `handlers_auth.go` — Google callback |
| `/api/auth/callback/github` | GET | `handlers_auth.go` — GitHub callback |

## OAuth 2.1 Endpoints

| Endpoint | Method | Handler |
|----------|--------|---------|
| `/.well-known/oauth-protected-resource` | GET | `handlers_oauth.go` — RFC 9728 |
| `/.well-known/oauth-authorization-server` | GET | `handlers_oauth.go` — RFC 8414 |
| `/oauth/register` | POST | `handlers_oauth.go` — RFC 7591 DCR |
| `/oauth/authorize` | GET/POST | `handlers_oauth.go` — authorization |
| `/oauth/token` | POST | `handlers_oauth.go` — token exchange |

## Portfolio Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/portfolios/{name}/external-balances` | GET/PUT/POST | External balances CRUD |
| `/api/portfolios/{name}/external-balances/{id}` | DELETE | Remove balance |
| `/api/portfolios/{name}/indicators` | GET | Portfolio-level indicators |
| `/api/portfolios/{name}/cash-transactions` | GET/POST/PUT | Cash transactions (PUT = bulk replace via `set_cash_transactions`) |
| `/api/portfolios/{name}/cash-transactions/transfer` | POST | Paired transfer between accounts |
| `/api/portfolios/{name}/cash-transactions/{id}` | PUT/DELETE | Transaction CRUD |
| `/api/portfolios/{name}/cash-transactions/performance` | GET | Capital performance (XIRR) |
| `/api/portfolios/{name}/review` | POST | Portfolio review (slim response) |
| `/api/portfolios/{name}/watchlist/review` | POST | Watchlist review |

## Internal OAuth Persistence Endpoints

Used by vire-portal to persist OAuth state in SurrealDB (survives Fly.io restarts). Handler: `oauth_internal.go`. No auth — internal network only (Docker/Fly private).

**Sessions** (pending auth sessions, 10-minute TTL enforced at application level):

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/internal/oauth/sessions` | POST | Create session |
| `/api/internal/oauth/sessions` | GET | Get latest session by `?client_id=X` |
| `/api/internal/oauth/sessions/{id}` | GET | Get session by ID |
| `/api/internal/oauth/sessions/{id}` | PATCH | Set user_id after login |
| `/api/internal/oauth/sessions/{id}` | DELETE | Delete session |

**Clients:**

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/internal/oauth/clients` | POST | Save/upsert client |
| `/api/internal/oauth/clients/{id}` | GET | Get client |
| `/api/internal/oauth/clients/{id}` | DELETE | Delete client |

**Codes:**

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/internal/oauth/codes` | POST | Save auth code |
| `/api/internal/oauth/codes/{code}` | GET | Get auth code |
| `/api/internal/oauth/codes/{code}/used` | PATCH | Mark code used |

**Tokens** (portal sends plaintext; server hashes with SHA-256 before storage):

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/internal/oauth/tokens` | POST | Save refresh token |
| `/api/internal/oauth/tokens/lookup` | POST | Lookup by plaintext token |
| `/api/internal/oauth/tokens/revoke` | POST | Revoke by plaintext token |
| `/api/internal/oauth/tokens/purge` | POST | Purge expired tokens |

## Service Registration Endpoints

Portal instances register as service users. Handler: `handlers_service.go`. Auth: shared key (`VIRE_SERVICE_KEY`).

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/services/register` | POST | Register service user (shared key auth) |
| `/api/admin/services/tidy` | POST | Purge stale service users (admin only) |

## Feedback Endpoints

| Endpoint | Method | Handler |
|----------|--------|---------|
| `/api/feedback` | GET | `handlers_feedback.go` — list feedback (paginated, filterable) |
| `/api/feedback` | POST | `handlers_feedback.go` — submit feedback (auto-captures user context) |
| `/api/feedback/summary` | GET | `handlers_feedback.go` — aggregate counts by status/severity/category |
| `/api/feedback/bulk` | PATCH | `handlers_feedback.go` — bulk status update (admin only) |
| `/api/feedback/{id}` | GET | `handlers_feedback.go` — get single feedback item |
| `/api/feedback/{id}` | PATCH | `handlers_feedback.go` — update status/resolution (captures updater identity) |
| `/api/feedback/{id}` | DELETE | `handlers_feedback.go` — delete feedback (admin only) |

User identity (`user_id`, `user_name`, `user_email`) is automatically extracted from the authenticated UserContext on create and update — not passed as request parameters.

## Middleware Stack

Execution order via `applyMiddleware`: recovery → CORS → bearer token → X-Vire-* headers → correlation ID → logging.

**Bearer token middleware** (`bearerTokenMiddleware`): Validates JWT from `Authorization: Bearer` header, loads user, populates UserContext. Invalid tokens return 401 with `WWW-Authenticate: Bearer`.

**X-Vire-* header middleware** (`userContextMiddleware`): Extracts `X-Vire-*` headers into UserContext. Loads user via `GetUser()`, resolves preferences from `ListUserKV`. Individual headers override profile values. Also resolves `X-Vire-Service-ID` for service identity (lowest priority, only applies if user has role `"service"`).

Bearer runs before X-Vire-* so OAuth-authenticated requests are resolved first. Priority: Bearer token > X-Vire-User-ID > X-Vire-Service-ID.

**Unauthenticated handling** (`requireNavexaContext`): When OAuth2 configured, unauthenticated requests return 401 with WWW-Authenticate resource metadata. Missing Navexa key returns 400 with `navexa_key_required`.

## User Model

`InternalUser`: user_id, email, name, password_hash, provider, role, created_at, modified_at. Provider tracks auth source: "email", "google", "github", "dev". Passwords bcrypt-hashed (cost 10). GET responses mask password_hash, return navexa_key_set (bool) + navexa_key_preview.

**Roles:** `RoleAdmin`, `RoleUser`, `RoleService` in `internal/models/storage.go`. Role field ignored on user endpoints — always "user". Changes only via `PATCH /api/admin/users/{id}/role` (which rejects `"service"` as a target role). Service users are created exclusively via `/api/services/register`.

## Portfolio Review Response

`POST /api/portfolios/{name}/review` returns slim response via `toSlimReview()`. Kept per holding: holding, overnight_move/pct, news_impact, action_required/reason, compliance. Stripped: signals, fundamentals, news_intelligence, filings_intelligence, filing_summaries, timeline.
