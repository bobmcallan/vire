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
| `/api/portfolios/{name}/cashflows` | GET/POST | Cash transactions |
| `/api/portfolios/{name}/cashflows/{id}` | PUT/DELETE | Transaction CRUD |
| `/api/portfolios/{name}/cashflows/performance` | GET | Capital performance (XIRR) |
| `/api/portfolios/{name}/review` | POST | Portfolio review (slim response) |
| `/api/portfolios/{name}/watchlist/review` | POST | Watchlist review |

## Middleware Stack

Execution order via `applyMiddleware`: recovery → CORS → bearer token → X-Vire-* headers → correlation ID → logging.

**Bearer token middleware** (`bearerTokenMiddleware`): Validates JWT from `Authorization: Bearer` header, loads user, populates UserContext. Invalid tokens return 401 with `WWW-Authenticate: Bearer`.

**X-Vire-* header middleware** (`userContextMiddleware`): Extracts `X-Vire-*` headers into UserContext. Loads user via `GetUser()`, resolves preferences from `ListUserKV`. Individual headers override profile values.

Bearer runs before X-Vire-* so OAuth-authenticated requests are resolved first.

**Unauthenticated handling** (`requireNavexaContext`): When OAuth2 configured, unauthenticated requests return 401 with WWW-Authenticate resource metadata. Missing Navexa key returns 400 with `navexa_key_required`.

## User Model

`InternalUser`: user_id, email, name, password_hash, provider, role, created_at, modified_at. Provider tracks auth source: "email", "google", "github", "dev". Passwords bcrypt-hashed (cost 10). GET responses mask password_hash, return navexa_key_set (bool) + navexa_key_preview.

**Roles:** `RoleAdmin`, `RoleUser` in `internal/models/storage.go`. Role field ignored on user endpoints — always "user". Changes only via `PATCH /api/admin/users/{id}/role`.

## Portfolio Review Response

`POST /api/portfolios/{name}/review` returns slim response via `toSlimReview()`. Kept per holding: holding, overnight_move/pct, news_impact, action_required/reason, compliance. Stripped: signals, fundamentals, news_intelligence, filings_intelligence, filing_summaries, timeline.
