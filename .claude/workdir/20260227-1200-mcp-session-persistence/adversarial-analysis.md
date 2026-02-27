# Adversarial Analysis: MCP Session Persistence

**Reviewer**: devils-advocate
**Date**: 2026-02-27
**Status**: COMPLETE

## Summary

Reviewed all implementation files for security, edge cases, race conditions, and failure modes.
Overall: **GOOD implementation, 2 bugs found (1 medium, 1 low), 0 critical security issues**.

---

## FINDINGS

### BUG-1 (MEDIUM): Session purge endpoint not routed

**File**: `internal/server/oauth_internal.go:21-23`

The integration test `TestInternalOAuth_SessionPurge` (line 679) calls `POST /api/internal/oauth/sessions/purge`. However, the router at line 21-23:

```go
case strings.HasPrefix(path, "sessions/"):
    id := strings.TrimPrefix(path, "sessions/")
    s.handleInternalOAuthSessionByID(w, r, id)
```

This sends `purge` as the `id` parameter to `handleInternalOAuthSessionByID`, which dispatches by method. A POST to `sessions/purge` falls through to `default` (Method Not Allowed) since the handler only accepts GET/PATCH/DELETE. There is NO route for session purge.

**Fix needed**: Add a case for `sessions/purge` BEFORE the generic `sessions/` prefix match, similar to how `codes/{code}/used` is handled before `codes/{code}`.

```go
case path == "sessions/purge":
    s.handleInternalOAuthSessionPurge(w, r)
case strings.HasPrefix(path, "sessions/"):
    // existing handler
```

And add the purge handler:
```go
func (s *Server) handleInternalOAuthSessionPurge(w http.ResponseWriter, r *http.Request) {
    if !RequireMethod(w, r, http.MethodPost) { return }
    count, err := s.app.Storage.OAuthStore().PurgeExpiredSessions(r.Context())
    if err != nil { WriteError(w, http.StatusInternalServerError, "..."); return }
    WriteJSON(w, http.StatusOK, map[string]int{"purged": count})
}
```

**Impact**: Session purge is dead code. Expired sessions accumulate in the DB until manual cleanup.

---

### BUG-2 (LOW): Client save returns 200 instead of 201

**File**: `internal/server/oauth_internal.go:169`

`handleInternalOAuthClients` returns `201 Created` per line 169, but the test `TestInternalOAuth_ClientLifecycle` line 444 asserts `200`. One of them is wrong. Per the requirements, POST save/upsert is an upsert, so:
- First call: 201 Created (new)
- Subsequent calls: 200 OK (updated)

Since the implementation can't distinguish (UPSERT is atomic), 201 is reasonable for consistency with sessions and codes. The test should expect 201, not 200.

**Impact**: Minor test expectation mismatch. Test will fail.

---

### SECURITY-1 (INFO - ACCEPTABLE): No authentication on internal API

**Observation**: The `/api/internal/oauth/` endpoints have no authentication middleware. This is explicitly called out in the requirements: "No auth on internal API: Endpoints are on internal network only."

**Risk**: If the internal API port is ever exposed externally (misconfigured Docker network, Caddy proxy leak), an attacker can:
- Create/delete OAuth sessions
- Read auth codes and client secrets
- Revoke all tokens (denial of service)

**Mitigation already planned**: Requirements say "Portal-side can add shared key later." This is acceptable for the current scope.

---

### SECURITY-2 (INFO - ACCEPTABLE): Error messages include store errors

**File**: `internal/server/oauth_internal.go` lines 79, 127, 134, 166, 187, 219, 249, 289, 341, 356

Error messages on `500 Internal Server Error` include `err.Error()` from the store layer, e.g.:
```go
WriteError(w, http.StatusInternalServerError, "Failed to save session: "+err.Error())
```

This could leak SurrealDB connection details or table names. However, since these are internal-only endpoints consumed by the portal (not end users), this is acceptable and useful for debugging.

---

### SECURITY-3 (PASS): Token hashing is correct

SHA-256 for refresh tokens (high-entropy, random) is appropriate. Tokens are not passwords - they are 32+ byte random values where brute force is infeasible regardless of hash function. The `hashToken` function at line 363 correctly uses `sha256.Sum256` + `hex.EncodeToString`.

---

### SECURITY-4 (PASS): No injection risk in SurrealDB queries

All queries use parameterized variables (`$rid`, `$client_id`, etc.) via `surrealmodels.NewRecordID`. No string concatenation in SQL. Session IDs, client IDs, and codes from URL paths are passed as parameters, not interpolated.

---

### SECURITY-5 (PASS): Plaintext token not exposed in responses

The `OAuthRefreshToken` struct has `TokenHash string json:"-"` (line 46 of models/oauth.go), which means the hash is never serialized in JSON responses. The `handleInternalOAuthTokenLookup` returns the token struct which correctly omits the hash. Test at line 567 of `oauth_internal_test.go` verifies this.

---

### SECURITY-6 (PASS): Request body size limit

`DecodeJSON` in `helpers.go` line 52 applies `http.MaxBytesReader(w, r.Body, 1<<20)` (1MB limit). All POST/PATCH handlers use `DecodeJSON`, preventing oversized payload attacks.

---

### RACE-1 (PASS): Session TTL enforcement via query-time filter

Session TTL is enforced at query time (`WHERE created_at > $cutoff`), not via a background purge. This means:
- `GetSession`: Returns nil for sessions >10 min old (line 341-344)
- `GetSessionByClientID`: Same filter (line 362)
- Even if purge hasn't run, expired sessions are invisible

Clock skew between portal and vire-server could cause edge cases (session appears expired on server but valid on portal), but the 10-minute window is generous enough that sub-second clock differences are irrelevant.

---

### RACE-2 (LOW RISK): Concurrent session creation for same client

If two authorize flows start simultaneously for the same client_id, `GetSessionByClientID` returns the latest by `ORDER BY created_at DESC LIMIT 1`. Both sessions are stored independently (different session_ids). The portal would get whichever is newest, and the other session simply expires after 10 minutes. No data corruption risk. This is the correct behavior.

---

### RACE-3 (PASS): Store mutex in unit test mock

The in-memory mock `memOAuthStore` (handlers_oauth_test.go) correctly uses `sync.Mutex` for all operations. The production SurrealDB store delegates concurrency to the database. No race conditions detected.

---

### EDGE-1 (PASS): Empty/missing field validation

Handler validates:
- Session: `session_id` and `client_id` required (line 70-72)
- Client: `client_id` required (line 158-160)
- Code: `code` and `client_id` required (line 211-213)
- Token save: `token` and `client_id` required (line 273-275)
- Token lookup/revoke: `token` required (lines 307-309, 334-336)
- Session patch: `user_id` required (line 122-124)

Empty string `""` passes validation but is semantically meaningless. However, this is an internal API where the portal controls inputs - over-validating would add complexity without value.

---

### EDGE-2 (PASS): GET sessions without client_id

Line 84-87: `GET /api/internal/oauth/sessions` requires `?client_id=X`. If missing, returns 400. This prevents accidentally fetching all sessions (which the endpoint doesn't support anyway).

---

### EDGE-3 (PASS): Delete idempotency

Both `DeleteSession` and `DeleteClient` in the store use SurrealDB `Delete` which tolerates non-existent records (`!isNotFoundError(err)`). Deleting an already-deleted session returns 200, not 404. This is correct REST semantics for idempotent DELETE.

---

### FAILURE-1 (PASS): SurrealDB down during API call

If SurrealDB is unreachable, store methods return errors. Handlers return 500 with error message. No panics, no hanging connections (context propagation ensures timeout). The `DecodeJSON` -> store call -> error handling chain is consistent across all handlers.

---

### TEST GAPS

1. **Session purge test will fail** - Routes to wrong handler (BUG-1)
2. **Client save test expects wrong status** - Expects 200, handler returns 201 (BUG-2)
3. **No test for DELETE on non-existent session/client** - Would verify idempotent DELETE behavior (nice-to-have)
4. **No test for PATCH on non-existent session** - `UpdateSessionUserID` would succeed silently on SurrealDB (UPDATE on non-existent record is a no-op). Consider returning 404 if the session doesn't exist.

---

## VERDICT

**APPROVED with 2 bugs to fix**

| ID | Severity | Fix Required? | Description |
|----|----------|---------------|-------------|
| BUG-1 | MEDIUM | YES | Session purge endpoint not routed |
| BUG-2 | LOW | YES | Client save returns 201, test expects 200 |
| SECURITY-1 | INFO | No (planned for later) | No auth on internal API |
| SECURITY-2 | INFO | No (internal API) | Error messages include store details |

The implementation is clean, follows project patterns, and correctly handles the security-sensitive aspects (token hashing, parameterized queries, body size limits, TTL enforcement). The two bugs are straightforward to fix.
