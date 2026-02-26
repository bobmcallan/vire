# Requirements: OAuth Token Expiry Fix

**Date:** 2026-02-26
**Requested:** Fix OAuth tokens expiring mid-session (fb_72a4dd2b)

## Problem

OAuth tokens expire mid-session, causing "Authentication timed out" errors in Claude.ai and ChatGPT. Neither MCP client attempts to refresh tokens when they expire.

**Root causes:**
1. Access token TTL is too short (1 hour default)
2. MCP clients don't use refresh tokens proactively
3. No sliding expiry to keep sessions alive during active use

## Scope

**In scope:**
- Extend default access token TTL to 7 days (configurable)
- Implement sliding expiry: reset token expiry on each authenticated request
- Enhance WWW-Authenticate header with resource metadata URL on 401 responses

**Out of scope:**
- Changes to MCP client behavior (outside our control)
- Token introspection endpoints
- Blacklisting/revocation mechanisms

## Approach

### 1. Extended Access Token TTL

Change default `access_token_expiry` from "1h" to "168h" (7 days). This is a pragmatic workaround giving MCP clients a week-long session window.

**Files:**
- `internal/common/config.go` — update `GetAccessTokenExpiry()` default

### 2. Sliding Expiry

When sliding expiry is enabled, each authenticated request extends the token's validity. Implementation:

- Add `sliding_expiry` config option (default: true when OAuth2 enabled)
- Add `last_used_at` field to `OAuthRefreshToken` model to track activity
- In `bearerTokenMiddleware`, when validating a valid JWT:
  - Check if sliding expiry is enabled
  - If token is >50% through its lifetime, issue a new access token in response header
  - Update refresh token's `last_used_at` timestamp

This approach avoids modifying JWT claims (which would invalidate the signature) and instead issues fresh tokens proactively.

**Files:**
- `internal/common/config.go` — add `SlidingExpiry bool` to `OAuth2Config`
- `internal/models/oauth.go` — add `LastUsedAt` to `OAuthRefreshToken`
- `internal/storage/surrealdb/oauthstore.go` — add `UpdateLastUsed(tokenHash)` method
- `internal/server/middleware.go` — implement sliding expiry logic
- `internal/server/handlers_oauth.go` — add helper for signing access tokens from middleware

### 3. Enhanced WWW-Authenticate Header

The current 401 response only returns `WWW-Authenticate: Bearer`. Per RFC 9728, include the resource metadata URL so clients can re-initiate OAuth.

**Current:**
```
WWW-Authenticate: Bearer
```

**New:**
```
WWW-Authenticate: Bearer resource_metadata="<issuer>/.well-known/oauth-protected-resource"
```

**Files:**
- `internal/server/middleware.go` — enhance `bearerTokenMiddleware` 401 response
- `internal/server/handlers.go` — enhance `requireNavexaContext` 401 response (already done, verify)

## Files Expected to Change

| File | Change |
|------|--------|
| `internal/common/config.go` | Add `SlidingExpiry`, update default TTL to 7 days |
| `internal/models/oauth.go` | Add `LastUsedAt` to `OAuthRefreshToken` |
| `internal/interfaces/storage.go` | Add `UpdateRefreshTokenLastUsed` to `OAuthStore` interface |
| `internal/storage/surrealdb/oauthstore.go` | Implement `UpdateRefreshTokenLastUsed` |
| `internal/server/middleware.go` | Sliding expiry logic, enhanced WWW-Authenticate |
| `internal/server/handlers_oauth.go` | Export `signOAuthAccessToken` for middleware use |

## Acceptance Criteria

1. Default access token TTL is 7 days (168h)
2. Sliding expiry can be disabled via config (`sliding_expiry = false`)
3. Active sessions (requests within token lifetime) receive fresh tokens automatically
4. 401 responses include full `WWW-Authenticate` header with resource metadata URL
5. Existing tests pass
6. New unit tests for sliding expiry logic
