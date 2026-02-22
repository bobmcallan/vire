# Requirements: Fix OAuth Provider Implementation Gaps

**Date:** 2026-02-22
**Requested:** Fix 3 gaps identified in the OAuth gap analysis (`.claude/workdir/20260222-1520-oauth-gap-analysis/summary.md`)

## Scope

### In Scope
- **Gap 1:** Email-based user matching with account linking in `findOrCreateOAuthUser`
- **Gap 2:** Error redirects in OAuth callback handlers (replace JSON errors with portal redirects)
- **Gap 3:** Populate JWT `name` claim from provider userinfo

### Out of Scope
- Changing state parameter design (already assessed as improvement over spec)
- Portal-side changes
- Mock-based exchange tests (separate effort)

## Approach

### Gap 1: Email-based Account Linking

Add `GetUserByEmail(ctx, email)` to `InternalStore` interface and SurrealDB implementation. Update `findOrCreateOAuthUser` to try email match before creating a new user.

**Files:**
- `internal/interfaces/storage.go` — add `GetUserByEmail` to `InternalStore` interface
- `internal/storage/surrealdb/internalstore.go` — implement `GetUserByEmail` using SurrealDB query: `SELECT * FROM user WHERE string::lowercase(email) = string::lowercase($email) LIMIT 1`
- `internal/server/handlers_auth.go` — update `findOrCreateOAuthUser` lookup order:
  1. `GetUser(ctx, userID)` — existing provider-specific match (e.g., `google_12345`)
  2. `GetUserByEmail(ctx, email)` — account linking via email
  3. If email match found, return that user (update email/name if changed)
  4. If neither found, create new user with provider-specific ID

This preserves backward compatibility — existing users with provider-specific IDs keep working. Account linking activates when a user logs in via a second provider with the same email.

### Gap 2: Error Redirects in Callbacks

Replace `WriteError` with `http.Redirect` in callback handlers when a valid callback URL is available. Add `error` query param check at top of each callback to handle provider-side denials (e.g., `access_denied`).

**Files:**
- `internal/server/handlers_auth.go`:
  - Add helper `redirectWithError(w, r, callback, errorCode string)` — builds `callback?error=code` and 302 redirects
  - In `handleOAuthCallbackGoogle` and `handleOAuthCallbackGitHub`:
    - Check `r.URL.Query().Get("error")` before state validation — if present, decode state to get callback, redirect with error
    - After state decoded (callback available): replace all `WriteError` calls with `redirectWithError`
    - If state is invalid/expired (no callback URL): keep `WriteError` (can't redirect)
  - Error codes: `access_denied`, `exchange_failed`, `profile_failed`, `user_creation_failed`, `provider_not_configured`

### Gap 3: JWT Name Claim

Add `Name` field to `InternalUser` model, populate from provider userinfo, use in `signJWT`.

**Files:**
- `internal/models/storage.go` — add `Name string` field to `InternalUser`
- `internal/server/handlers_auth.go`:
  - Update `signJWT` to use `user.Name` instead of hardcoded `""`
  - Update `findOrCreateOAuthUser` signature to accept `name` parameter, store on user
  - Update Google handlers to parse `Name` from userinfo response and pass to `findOrCreateOAuthUser`
  - Update GitHub handlers to parse `Name` from user response and pass to `findOrCreateOAuthUser`
  - Update `oauthUserResponse` to include `name`

## Files Expected to Change

| File | Change |
|------|--------|
| `internal/models/storage.go` | Add `Name` field to `InternalUser` |
| `internal/interfaces/storage.go` | Add `GetUserByEmail` to `InternalStore` interface |
| `internal/storage/surrealdb/internalstore.go` | Implement `GetUserByEmail` |
| `internal/server/handlers_auth.go` | All 3 gaps: error redirects, email matching, name claim |
| `internal/server/handlers_auth_test.go` | Tests for new behavior |
