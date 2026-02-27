# Architecture Review: MCP Session Persistence API
**Reviewer**: architect
**Date**: 2026-02-27
**Task**: #2 — Review architecture alignment of internal OAuth API

## Overall Assessment: APPROVED with 2 issues to fix

The implementation aligns with the established architecture patterns. Two issues require fixes before this can be merged.

---

## ISSUE 1: Duplicate Hash Function (Must Fix)

**Severity**: High (silent correctness risk)
**File**: `internal/server/oauth_internal.go:363`

`oauth_internal.go` introduces a new function `hashToken()` that is byte-for-byte identical to `hashRefreshToken()` in `handlers_oauth.go`. Both compute SHA-256 hex:

```go
// oauth_internal.go (new - line 363)
func hashToken(plaintext string) string {
    h := sha256.Sum256([]byte(plaintext))
    return hex.EncodeToString(h[:])
}

// handlers_oauth.go (existing - line 692)
func hashRefreshToken(token string) string {
    h := sha256.Sum256([]byte(token))
    return hex.EncodeToString(h[:])
}
```

**Why it matters**: These two functions hash tokens stored in the same `oauth_refresh_token` table. If a portal token saved via the internal API (using `hashToken`) is later looked up by the existing token endpoint (using `hashRefreshToken`), they both produce the same hash today — but the duplication creates silent drift risk. If one is ever changed (e.g., to use a salt), the other won't be, breaking cross-path lookups silently.

**Fix**: Replace `hashToken` calls in `oauth_internal.go` with `hashRefreshToken`. Delete the `hashToken` function entirely.

```go
// oauth_internal.go: replace all occurrences of hashToken with hashRefreshToken
hash := hashRefreshToken(body.Token)  // was: hashToken(body.Token)
```

---

## ISSUE 2: `GetSession` WHERE Clause on RecordID (Must Fix)

**Severity**: High (potential security bypass)
**File**: `internal/storage/surrealdb/oauthstore.go:340`

The `GetSession` method uses `FROM $rid WHERE created_at > $cutoff`:

```go
sql := `SELECT session_id, client_id, ... FROM $rid WHERE created_at > $cutoff`
vars := map[string]any{
    "rid":    surrealmodels.NewRecordID("mcp_auth_session", sessionID),
    "cutoff": time.Now().Add(-10 * time.Minute),
}
```

In SurrealDB, `SELECT ... FROM {record_id} WHERE ...` may fetch the record by ID first then filter, OR it may work correctly — but this is NOT an established pattern in this codebase. Every other WHERE clause in `oauthstore.go` operates on table names (e.g., `FROM oauth_code WHERE expires_at < $now`), never on `$rid`. If the WHERE is silently ignored, expired sessions would be returned, defeating the TTL enforcement.

**Fix**: Filter expiry at the application level after fetching, consistent with existing patterns:

```go
func (s *OAuthStore) GetSession(ctx context.Context, sessionID string) (*models.OAuthSession, error) {
    sql := `SELECT session_id, client_id, redirect_uri, state, code_challenge, code_method, scope, user_id, created_at FROM $rid`
    vars := map[string]any{
        "rid": surrealmodels.NewRecordID("mcp_auth_session", sessionID),
    }
    results, err := surrealdb.Query[[]oauthSessionRow](ctx, s.db, sql, vars)
    // ... error handling ...
    row := (*results)[0].Result[0]
    // Enforce TTL at application level
    if time.Since(row.CreatedAt) > 10*time.Minute {
        return nil, fmt.Errorf("oauth session expired: %s", sessionID)
    }
    return sessionFromRow(row), nil
}
```

---

## PASSED: Pattern Alignment

### Interface Extension (`internal/interfaces/storage.go`)
✅ Session methods added to `OAuthStore` interface correctly
✅ Method signatures match requirements exactly
✅ Comment updated to reflect new capability

### Model Changes (`internal/models/oauth.go`)
✅ `OAuthSession` struct matches spec exactly (SessionID, ClientID, RedirectURI, State, CodeChallenge, CodeMethod, Scope, UserID, CreatedAt)
✅ `OAuthClient` extended with `omitempty` fields (backward compatible)
✅ No breaking changes to existing fields

### SurrealDB Manager (`internal/storage/surrealdb/manager.go`)
✅ `mcp_auth_session` table added to the table list correctly
✅ Follows existing pattern (single-line addition in the tables slice)

### Store Implementation (`internal/storage/surrealdb/oauthstore.go`)
✅ `oauthSessionRow` DB-level struct matches `OAuthSession` model
✅ `SaveSession` uses `UPSERT $rid SET` — correct UPSERT pattern matching `SaveClient`
✅ `DeleteSession` uses `surrealdb.Delete[oauthSessionRow]` — same as `DeleteClient`
✅ `PurgeExpiredSessions` uses `DELETE FROM mcp_auth_session WHERE` — same as purge methods
✅ `GetSessionByClientID` uses table-level SELECT with WHERE — correct pattern
✅ `sessionFromRow` helper function is well-structured
✅ Compile-time interface check at line 424 enforces interface compliance
❌ `GetSession` WHERE on RecordID — see ISSUE 2

### Handler (`internal/server/oauth_internal.go`)
✅ `routeInternalOAuth` follows the same dispatch pattern as `routePortfolios`, `routeAdminJobs`
✅ Uses `WriteJSON`, `WriteError`, `DecodeJSON`, `RequireMethod` from helpers.go
✅ Method validation is correct for each endpoint
✅ All 13 required endpoints are implemented and reachable
✅ Token hashing in the handler (not exposed to client) — correct
❌ `hashToken` duplicates `hashRefreshToken` — see ISSUE 1

### Route Registration (`internal/server/routes.go`)
✅ `mux.HandleFunc("/api/internal/oauth/", s.routeInternalOAuth)` correctly registered
✅ No conflicts with existing routes (no prefix overlap with `/oauth/`, `/api/auth/`, etc.)
✅ Placement at end of `registerRoutes()` with descriptive comment

### Security Design
✅ No auth on internal endpoints — matches design decision (internal network only)
✅ Token plaintext never stored — SHA-256 hashed before `SaveRefreshToken`
✅ Token plaintext not in URL — passed as JSON body in POST

### Backward Compatibility
✅ New OAuthClient fields use `omitempty` — existing `handleOAuthRegister` unaffected
✅ `SaveClient` now stores new fields but the `GetClient` → `handleOAuthRegister` path still works
✅ Compile-time check catches any interface drift immediately

---

## Action Required

Send to **implementer** via SendMessage:

**Fix 1**: In `oauth_internal.go`, rename all calls to `hashToken()` to `hashRefreshToken()`, and delete the `hashToken` function (it's a duplicate of `hashRefreshToken` in `handlers_oauth.go`).

**Fix 2**: In `oauthstore.go`, change `GetSession` to not use `WHERE` on a RecordID-scoped SELECT. Fetch the record without a WHERE clause, then check `time.Since(row.CreatedAt) > 10*time.Minute` at the application level and return an error if expired.
