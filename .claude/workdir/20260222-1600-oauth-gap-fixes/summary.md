# Summary: Fix OAuth Provider Implementation Gaps

**Date:** 2026-02-22
**Status:** Completed

## What Changed

| File | Change |
|------|--------|
| `internal/models/storage.go` | Added `Name` field to `InternalUser` struct |
| `internal/interfaces/storage.go` | Added `GetUserByEmail` to `InternalStore` interface |
| `internal/storage/surrealdb/internalstore.go` | Implemented `GetUserByEmail` with case-insensitive SurrealDB query |
| `internal/server/handlers_auth.go` | All 3 gaps fixed: email-based account linking, error redirects in callbacks, JWT name claim |
| `internal/server/handlers_auth_test.go` | Added tests for email linking, name updates, error redirects, name in JWT |
| `tests/api/auth_test.go` | Integration tests for OAuth callback error redirects and email-based linking |
| `tests/data/internal_store_test.go` | Data-layer tests for `GetUserByEmail` (case-insensitive, empty email guard) |
| `docs/authentication/vire-server-oauth-requirements.md` | Updated status tables to reflect implementation |

## Gap Fixes

### Gap 1: Email-Based Account Linking
- Added `GetUserByEmail(ctx, email)` to `InternalStore` interface and SurrealDB implementation
- Uses `string::lowercase()` for case-insensitive matching with empty email guard
- `findOrCreateOAuthUser` now: (1) checks by provider ID, (2) checks by email for linking, (3) creates new user
- Same email via Google and GitHub returns the same account

### Gap 2: Error Redirects in Callbacks
- Added `redirectWithError(w, r, callback, errorCode)` helper
- Both callback handlers now check `error` query param from provider (e.g., `access_denied`)
- All error cases after state validation use `redirectWithError` instead of `WriteError`
- Error codes: `provider_not_configured`, `exchange_failed`, `profile_failed`, `user_creation_failed`, `token_failed`
- Invalid state (no callback URL): keeps `WriteError` (correct — can't redirect)

### Gap 3: JWT Name Claim
- Added `Name` field to `InternalUser` model (backward compatible — defaults to empty)
- `signJWT` uses `user.Name` instead of hardcoded `""`
- Google handlers parse `name` from userinfo response
- GitHub handlers parse `name` from user response, fall back to `login`
- `oauthUserResponse` includes `name` in response
- `findOrCreateOAuthUser` accepts and stores name, updates on re-login

## Tests
- Unit tests: 275 pass in `internal/server/` (11 new OAuth tests)
- Data layer: 21 pass (3 new `GetUserByEmail` tests)
- API integration: 32 pass for OAuth (10 new tests)
- 1 feedback loop round (empty email guard fix)
- `go vet ./...` clean

## Documentation Updated
- `docs/authentication/vire-server-oauth-requirements.md` — status tables updated

## Devils-Advocate Findings
- Compile error in initial implementation — fixed
- Verified Gap 2 was fully implemented after initial concern
- SurrealDB parameterized queries prevent injection in `GetUserByEmail`
- HMAC-signed state prevents callback URL manipulation

## Notes
- `InternalUser.Name` is a backward-compatible addition — existing records without it default to empty string
- Account linking preserves the first user record found by email (provider-specific ID unchanged)
- Two pre-existing test failures noted (not OAuth-related): `TestStress_WriteRaw_AtomicWrite` and `TestPortfolioStock_GainPercentage/capital_gain_pct`
