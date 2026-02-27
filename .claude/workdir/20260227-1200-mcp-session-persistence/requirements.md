# Requirements: MCP Session Persistence API

**Feedback**: fb_c4a661a8, fb_00e43378
**Date**: 2026-02-27
**Scope**: vire-server only (portal changes are a follow-up)

## Problem

vire-portal stores all OAuth state (sessions, auth codes, refresh tokens, client registrations) in-memory. Every Fly.io deployment or restart wipes this state, breaking all active MCP client sessions with 404 "Link not found". Deployments happen multiple times daily.

## Solution

Add internal REST API endpoints on vire-server that the portal can call to persist OAuth state in SurrealDB. This is the server-side half of a two-part effort:
1. **vire-server** (this PR): CRUD API for OAuth sessions, codes, tokens, clients
2. **vire-portal** (follow-up): Replace in-memory stores with HTTP client calls to these endpoints

## Scope

### New Model: `OAuthSession`
Add `models.OAuthSession` struct for pending auth sessions:
```go
type OAuthSession struct {
    SessionID     string    `json:"session_id"`
    ClientID      string    `json:"client_id"`
    RedirectURI   string    `json:"redirect_uri"`
    State         string    `json:"state"`
    CodeChallenge string    `json:"code_challenge"`
    CodeMethod    string    `json:"code_method"`
    Scope         string    `json:"scope"`
    UserID        string    `json:"user_id,omitempty"` // filled after login
    CreatedAt     time.Time `json:"created_at"`
}
```

### Extended Model: `OAuthClient`
Add portal-specific fields to existing OAuthClient:
```go
GrantTypes              []string `json:"grant_types,omitempty"`
ResponseTypes           []string `json:"response_types,omitempty"`
TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method,omitempty"`
```

### Extended Interface: `OAuthStore`
Add session methods:
```go
// Sessions (new)
SaveSession(ctx, *models.OAuthSession) error
GetSession(ctx, sessionID string) (*models.OAuthSession, error)
GetSessionByClientID(ctx, clientID string) (*models.OAuthSession, error)
UpdateSessionUserID(ctx, sessionID, userID string) error
DeleteSession(ctx, sessionID string) error
PurgeExpiredSessions(ctx context.Context) (int, error)
```

### New SurrealDB Table: `mcp_auth_session`
Add to manager.go table list. Fields match OAuthSession model.

### SurrealDB Store Implementation
Implement all new session methods in `oauthstore.go`.

### Internal REST API: `/api/internal/oauth/...`
New handler file: `internal/server/oauth_internal.go`

**Sessions:**
| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/internal/oauth/sessions` | Create session |
| GET | `/api/internal/oauth/sessions/{id}` | Get session by ID |
| GET | `/api/internal/oauth/sessions?client_id=X` | Get latest session by client ID |
| PATCH | `/api/internal/oauth/sessions/{id}` | Update session (set user_id after login) |
| DELETE | `/api/internal/oauth/sessions/{id}` | Delete session |

**Clients:**
| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/internal/oauth/clients` | Save/upsert client |
| GET | `/api/internal/oauth/clients/{id}` | Get client |
| DELETE | `/api/internal/oauth/clients/{id}` | Delete client |

**Codes:**
| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/internal/oauth/codes` | Save auth code |
| GET | `/api/internal/oauth/codes/{code}` | Get auth code |
| PATCH | `/api/internal/oauth/codes/{code}/used` | Mark code used |

**Tokens (plaintext in body, server hashes internally):**
| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/internal/oauth/tokens` | Save refresh token (hashes internally) |
| POST | `/api/internal/oauth/tokens/lookup` | Lookup token by plaintext (hashes to find) |
| POST | `/api/internal/oauth/tokens/revoke` | Revoke token by plaintext (hashes to find) |
| POST | `/api/internal/oauth/tokens/purge` | Purge expired tokens |

### Route Registration
Wire `/api/internal/oauth/` into `registerRoutes()` in routes.go.

## Files Expected to Change

| File | Change |
|------|--------|
| `internal/models/oauth.go` | Add OAuthSession struct, extend OAuthClient |
| `internal/interfaces/storage.go` | Add session methods to OAuthStore interface |
| `internal/storage/surrealdb/oauthstore.go` | Implement session methods |
| `internal/storage/surrealdb/manager.go` | Add mcp_auth_session table |
| `internal/server/oauth_internal.go` | **NEW** — internal OAuth REST API handler |
| `internal/server/routes.go` | Register internal OAuth routes |

## Design Decisions

1. **Token hashing**: Server hashes plaintext tokens using SHA-256 before storage. Portal sends plaintext in POST body (never in URL). This matches existing server pattern.
2. **No auth on internal API**: Endpoints are on internal network only (Docker/Fly private). Portal-side can add shared key later.
3. **Session TTL**: 10-minute TTL enforced at query time (matching portal's current TTL). PurgeExpiredSessions for cleanup.
4. **Backward compatible**: New fields on OAuthClient use `omitempty`. Existing code unaffected.

## Out of Scope
- vire-portal changes (separate follow-up)
- Auth gating on internal endpoints (portal follow-up)
- ACDC symbol resolution (already fixed in 364a5a2)
