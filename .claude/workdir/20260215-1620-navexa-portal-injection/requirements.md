# Requirements: Navexa API Key and User ID Portal Injection

**Date:** 2026-02-15
**Requested:** Remove Navexa API key from backend config. Both Navexa API key and user ID must be injected by the MCP portal via request headers. If either is missing, return "configuration not correct" error. User data should be stored by user ID. Update README.

## Scope

**In scope:**
- Remove `api_key` from `NavexaConfig` and all resolution logic (env vars, KV store, config file)
- Add `X-Vire-User-ID` header extraction to middleware and `UserContext`
- Add `UserID` field to `UserContext`
- Require both `user_id` and `navexa_key` for all Navexa-dependent endpoints — return "configuration not correct" if missing
- Remove default/fallback Navexa client from app initialisation
- Make `InjectNavexaClient` strictly require portal-injected key
- Remove `api_key` from `docker/vire.toml`
- Add `X-Vire-User-ID` to CORS allowed headers
- Update user context logging/tracing to include user ID
- Update README

**Out of scope:**
- Full multi-tenant storage refactoring (storage by user ID is noted as future work — current request is about injection and validation)
- Changes to vire-portal (separate repo)
- Changes to EODHD or Gemini client config

## Approach

The backend already has most of the plumbing:
- `userContextMiddleware` extracts `X-Vire-Navexa-Key` into `UserContext.NavexaAPIKey`
- `InjectNavexaClient` creates per-request clients from the header
- `resolveNavexaClient` in portfolio service checks context for overrides

**Changes needed:**

1. **`internal/common/config.go`** — Remove `APIKey` field from `NavexaConfig`. Remove navexa key from `ResolveAPIKey` call in app init. Keep `BaseURL`, `RateLimit`, `Timeout`.

2. **`internal/common/userctx.go`** — Add `UserID string` field to `UserContext`.

3. **`internal/server/middleware.go`** — Extract `X-Vire-User-ID` header in `userContextMiddleware`. Add `X-Vire-User-ID` to CORS allowed headers.

4. **`internal/app/app.go`** — Remove navexa API key resolution. Remove default `NavexaClient` creation. Update `InjectNavexaClient` to return error context if key is missing. Remove `NavexaClient` field from `App` struct.

5. **`internal/server/handlers.go`** — Add validation in Navexa-dependent handlers: if `UserContext` is nil, or `UserID` is empty, or `NavexaAPIKey` is empty, return 400 with `{"error": "configuration not correct"}`.

6. **`internal/services/portfolio/service.go`** — Remove fallback to `s.navexa` in `resolveNavexaClient`. If no context override exists, return error. Update `NewService` to not require a navexa client parameter (or make it optional/nil).

7. **`docker/vire.toml`** — Remove `api_key` from `[clients.navexa]`.

8. **`README.md`** — Document that Navexa API key and user ID are injected by the portal, not stored in backend config.

## Files Expected to Change
- `internal/common/config.go`
- `internal/common/userctx.go`
- `internal/server/middleware.go`
- `internal/server/handlers.go`
- `internal/app/app.go`
- `internal/services/portfolio/service.go`
- `docker/vire.toml`
- `README.md`
- Test files as needed
