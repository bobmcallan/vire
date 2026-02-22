# Summary: OAuth Provider Gap Analysis

**Date:** 2026-02-22
**Status:** Complete

## Overall Assessment

The OAuth implementation is **substantially complete** — all 4 endpoints exist, the OAuth flow works end-to-end, config/env support is correct, and there's solid test coverage for unit-testable components. Three gaps were identified, two of which affect correctness.

## Audit Results

### 1. Endpoints — PASS

All 4 OAuth endpoints implemented and registered:

| Endpoint | Handler | Route Registration |
|----------|---------|-------------------|
| `GET /api/auth/login/google` | `handleOAuthLoginGoogle` (line 417) | `routes.go:61` |
| `GET /api/auth/callback/google` | `handleOAuthCallbackGoogle` (line 493) | `routes.go:63` |
| `GET /api/auth/login/github` | `handleOAuthLoginGitHub` (line 454) | `routes.go:62` |
| `GET /api/auth/callback/github` | `handleOAuthCallbackGitHub` (line 583) | `routes.go:64` |

Additionally, `POST /api/auth/oauth` handles Google/GitHub code exchange via JSON (not in spec — bonus capability for non-browser clients).

### 2. State Parameter — PASS (Improvement)

| Requirement | Spec | Implementation | Match |
|-------------|------|---------------|-------|
| Randomness | Min 32 bytes | 16-byte nonce (total payload >> 32 bytes) | Adequate |
| Time-limited | 10-minute TTL | 10-minute TTL via timestamp check | Yes |
| Maps to callback | State → callback URL | Callback embedded in signed payload | Yes |
| Single-use | Delete after exchange | Not enforced (stateless) | Deviation |
| Storage | In-memory map + mutex | HMAC-signed stateless token | Improvement |

**Deviation: not single-use.** The stateless HMAC approach means a state token could theoretically be replayed within the 10-minute window. In practice this requires both the signed state AND a valid auth code from the provider (codes are single-use at the provider level), making replay infeasible. The stateless design eliminates server-side state management, race conditions, and memory cleanup — a net improvement.

### 3. User Mapping — GAP

| Requirement | Spec | Implementation | Match |
|-------------|------|---------------|-------|
| Match strategy | By email (case-insensitive) | By provider-specific ID (`google_<id>`, `github_<id>`) | **No** |
| Account linking | Same email = same account | Same email = separate accounts | **No** |
| Username | Email local part or GitHub login | Provider-prefixed ID | No |
| Update on re-login | Update name, picture | Update email only | Partial |

`findOrCreateOAuthUser` (line 377) looks up users by `userID` which is `google_<google-numeric-id>` or `github_<github-numeric-id>`. Two consequences:

1. **No account linking**: A user signing in via Google (alice@example.com → `google_12345`) and later via GitHub (same email → `github_67890`) gets two separate accounts.
2. **User IDs are provider-specific**: The `sub` JWT claim contains `google_12345` instead of a provider-agnostic identifier.

**Security note:** The current approach has a security advantage — it prevents account takeover if an attacker controls a different provider account with the same email. The spec's email-based approach trusts all providers equally, which may not be desirable. This is a product decision, not purely a technical gap.

### 4. JWT Claims — MINOR GAP

| Claim | Spec | Implementation | Match |
|-------|------|---------------|-------|
| `sub` | vire user ID | user.UserID | Yes |
| `email` | user email | user.Email | Yes |
| `name` | display name | Hardcoded `""` | **No** |
| `provider` | google/github/email | Correct value | Yes |
| `iss` | "vire-server" | "vire-server" | Yes |
| `iat` | unix seconds | now.Unix() | Yes |
| `exp` | now + 24h | now + config expiry | Yes |
| Signing | HMAC-SHA256 | HS256 | Yes |

`signJWT` (line 29) hardcodes `"name": ""`. Google's userinfo response includes `name` and GitHub's user response includes `name`, but neither value is stored on `InternalUser` or passed to `signJWT`. The portal may display an empty name for OAuth users.

### 5. Error Handling — GAP

| Requirement | Spec | Implementation | Match |
|-------------|------|---------------|-------|
| Callback errors | Redirect to portal with `?error=code` | Returns JSON via `WriteError` | **No** |
| Provider `error` param | Check before processing | Not checked | **No** |
| Error codes | 6 specific codes defined | None used | **No** |

The callback handlers (`handleOAuthCallbackGoogle`, `handleOAuthCallbackGitHub`) return JSON error responses for all failure cases. Since these are browser-facing endpoints in the OAuth redirect flow, the user sees raw JSON in their browser instead of being redirected to the portal's error page.

**Specific issues:**

1. **No `error` param check on callbacks** (lines 498-499, 588-589): If the user denies consent at Google/GitHub, the provider redirects back with `?error=access_denied`. The implementation doesn't check for this and proceeds to decode the state, which works but then tries to exchange an empty code.

2. **JSON errors instead of redirects**: All `WriteError` calls in the callback handlers should be `http.Redirect(w, r, callback+"?error=<code>", http.StatusFound)`. Exception: if the state is invalid/expired, there's no valid callback URL, so a JSON error or fallback is acceptable there.

### 6. Config & Environment Variables — PASS

| Config | Spec | Implementation | Match |
|--------|------|---------------|-------|
| `[auth]` section | jwt_secret, token_expiry | `AuthConfig` struct | Yes |
| `[auth.google]` | client_id, client_secret | `OAuthProvider` struct | Yes |
| `[auth.github]` | client_id, client_secret | `OAuthProvider` struct | Yes |
| VIRE_AUTH_JWT_SECRET | env override | `config.go:312` | Yes |
| VIRE_AUTH_TOKEN_EXPIRY | env override | `config.go:315` | Yes |
| VIRE_AUTH_GOOGLE_CLIENT_ID | env override | `config.go:318` | Yes |
| VIRE_AUTH_GOOGLE_CLIENT_SECRET | env override | `config.go:320` | Yes |
| VIRE_AUTH_GITHUB_CLIENT_ID | env override | `config.go:322` | Yes |
| VIRE_AUTH_GITHUB_CLIENT_SECRET | env override | `config.go:325` | Yes |

### 7. Test Coverage — PARTIAL

| Spec Test | Status | Existing Tests |
|-----------|--------|---------------|
| State generation/validation | ✅ | `TestStateEncodeDecode_RoundTrip`, `TestStateParameter_HMACValidation`, `TestStateParameter_Expiry` |
| Google token exchange (mocked) | ❌ | None — would need httptest server |
| GitHub token exchange (mocked) | ❌ | None |
| GitHub email fallback (mocked) | ❌ | None |
| User creation | ✅ | `TestHandleAuthOAuth_DevProvider` |
| User matching by email | ❌ | Not applicable (email matching not implemented) |
| JWT minting | ✅ | 3 JWT tests (roundtrip, expired, wrong secret) |
| Redirect URL construction | ✅ | `TestBuildCallbackRedirectURL_*` (2 tests) |
| Error redirects | ❌ | Not applicable (error redirects not implemented) |
| Full flow with mock provider | ❌ | None |

**Present but not in spec:** 22 additional tests covering dev provider, validation endpoint, login endpoint, callback URL validation, OAuth login redirects, and invalid callbacks.

## Improvements Over Spec

1. **Stateless HMAC state parameter**: Eliminates server-side state, cleanup goroutines, and mutex contention. More resilient to server restarts.
2. **Callback URL validation** (`validateCallbackURL`): Not in spec. Blocks protocol-relative URLs, javascript: scheme, enforces HTTPS in production.
3. **POST-based code exchange**: `POST /api/auth/oauth` accepts Google/GitHub provider codes for non-browser (API) clients.
4. **Dynamic redirect URI**: `oauthRedirectURI` uses `X-Forwarded-Proto` for reverse proxy awareness.

## Gap Summary

| # | Gap | Severity | Fix Effort |
|---|-----|----------|-----------|
| 1 | User mapping by provider ID instead of email — no account linking | Medium | Moderate — change `findOrCreateOAuthUser` to query by email first, fall back to create. Needs email index on InternalStore. |
| 2 | Callback errors return JSON instead of portal redirects | Medium | Low — replace `WriteError` with `http.Redirect` in callback handlers, add `error` param check at top of each callback |
| 3 | JWT `name` claim always empty | Low | Low — parse `name` from provider userinfo, pass to `signJWT` or store on InternalUser |
| 4 | Missing mock-based exchange tests | Low | Moderate — requires httptest server mocking Google/GitHub token and userinfo endpoints |

## Files Audited

| File | Lines | Findings |
|------|-------|----------|
| `internal/server/handlers_auth.go` | 802 | Gaps #1, #2, #3 |
| `internal/server/routes.go` | 391 | All routes registered correctly |
| `internal/common/config.go` | 395 | Config fully matches spec |
| `internal/server/handlers_auth_test.go` | 622 | Good coverage, missing mock exchange tests |
