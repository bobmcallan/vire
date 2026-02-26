# Summary: OAuth Token Expiry Fix

**Date:** 2026-02-26
**Status:** completed
**Feedback:** fb_72a4dd2b

## Problem

OAuth tokens were expiring mid-session, causing "Authentication timed out" errors in Claude.ai and ChatGPT. MCP clients don't proactively refresh tokens when they expire.

## Solution

Implemented three fixes as recommended in the feedback:

1. **Extended access token TTL**: Changed default from 1 hour to 7 days (168h)
2. **Sliding expiry**: Tokens >50% through their lifetime receive a fresh access token via response header
3. **Enhanced WWW-Authenticate header**: RFC 9728 compliant with resource_metadata URL

## What Changed

| File | Change |
|------|--------|
| `internal/common/config.go` | Added `SlidingExpiry *bool` to OAuth2Config, changed default access token TTL to 168h, added `GetSlidingExpiry()` method |
| `internal/models/oauth.go` | Added `LastUsedAt time.Time` to OAuthRefreshToken |
| `internal/interfaces/storage.go` | Added `UpdateRefreshTokenLastUsed(ctx, tokenHash, lastUsedAt)` to OAuthStore interface |
| `internal/storage/surrealdb/oauthstore.go` | Implemented `UpdateRefreshTokenLastUsed`, added `last_used_at` to save/get queries |
| `internal/server/middleware.go` | Added `writeBearerChallenge()` with RFC 9728 WWW-Authenticate header, added sliding expiry token refresh in `bearerTokenMiddleware`, added `shouldRefreshToken()` and `signAccessToken()` helpers |
| `internal/server/handlers_oauth_test.go` | Added `UpdateRefreshTokenLastUsed` to memOAuthStore mock |
| `internal/common/config_test.go` | Added 9 unit tests for OAuth2Config methods |

## Tests

- **Unit tests added:** 9 new tests in `internal/common/config_test.go`
  - TestOAuth2Config_GetAccessTokenExpiry_Default
  - TestOAuth2Config_GetAccessTokenExpiry_Configured
  - TestOAuth2Config_GetAccessTokenExpiry_InvalidFallsBack
  - TestOAuth2Config_GetSlidingExpiry_DefaultTrue
  - TestOAuth2Config_GetSlidingExpiry_ExplicitlyEnabled
  - TestOAuth2Config_GetSlidingExpiry_ExplicitlyDisabled
  - TestOAuth2Config_GetSlidingExpiry_NoIssuer
  - TestOAuth2Config_GetCodeExpiry_Default
  - TestOAuth2Config_GetRefreshTokenExpiry_Default

- **Test results:** All pass
- **Build:** Clean
- **Go vet:** Clean

## Configuration

New config option in `[auth.oauth2]`:

```toml
[auth.oauth2]
issuer = "https://api.vire.au"
access_token_expiry = "168h"  # Default: 7 days (was 1h)
sliding_expiry = true          # Default: true (optional, omit for default)
```

To disable sliding expiry (not recommended):
```toml
sliding_expiry = false
```

## API Changes

### New Response Headers

When sliding expiry is enabled and a token is >50% through its lifetime, the response includes:

- `X-New-Access-Token`: Fresh JWT access token
- `X-New-Token-Expires-In`: Expiry time in seconds

MCP clients can use these headers to update their stored token.

### Enhanced WWW-Authenticate Header

401 responses now include the RFC 9728 resource metadata URL:

```
WWW-Authenticate: Bearer error="invalid_token", error_description="...", resource_metadata="https://api.vire.au/.well-known/oauth-protected-resource"
```

This enables MCP clients to discover OAuth configuration when they eventually implement refresh token handling.

## Notes

- The 7-day default is a pragmatic workaround for MCP clients that don't implement token refresh
- Sliding expiry keeps active sessions alive indefinitely
- The `LastUsedAt` field on refresh tokens enables future analytics on session activity
- No breaking changes to existing tokens (old tokens continue to work)
