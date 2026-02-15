# Requirements: User profile preferences + dev mode auto-import

**Date:** 2026-02-15
**Requested:** Two changes:
1. Portal will only send X-Vire-User-ID — all user preferences (navexa_key, display_currency, default_portfolio, portfolios) stored in user profile on vire-server. Middleware resolves everything from user storage.
2. In dev mode (environment=development or ENVIRONMENT=DEV), auto-import users from `import/users.json` on startup.

## Scope

### In scope
- Expand User model with DisplayCurrency, DefaultPortfolio, Portfolios fields
- Update middleware to resolve all user context fields from user storage (not just navexa_key)
- Update user CRUD handlers/responses to include new fields
- Dev mode auto-import from `import/users.json` on app startup
- Update README architecture diagram and descriptions
- Update CORS headers (simplify, portal only sends X-Vire-User-ID)

### Out of scope
- Portal code changes
- JWT/auth changes
- Removing backward-compat support for individual X-Vire-* headers (keep for direct API use)

## Approach

### 1. Expand User model — `internal/models/user.go`
Add three new fields:
```go
type User struct {
    Username         string   `json:"username"`
    Email            string   `json:"email"`
    PasswordHash     string   `json:"password_hash"`
    Role             string   `json:"role"`
    NavexaKey        string   `json:"navexa_key,omitempty"`
    DisplayCurrency  string   `json:"display_currency,omitempty"`
    DefaultPortfolio string   `json:"default_portfolio,omitempty"`
    Portfolios       []string `json:"portfolios,omitempty"`
}
```

### 2. Update middleware — `internal/server/middleware.go`
When X-Vire-User-ID is present, look up user and resolve ALL context fields from the profile. Individual headers still take precedence as overrides. Change resolution order to:
1. If X-Vire-User-ID present → load user from storage
2. Set uc fields from user profile (navexa_key, display_currency, portfolios)
3. Override with any explicit X-Vire-* headers (backward compat)
This means the portal only needs to send X-Vire-User-ID and the server resolves everything.

### 3. Update handlers — `internal/server/handlers_user.go`
- handleUserUpdate: accept `display_currency`, `default_portfolio`, `portfolios` in PUT body
- userResponse: include these fields in GET response
- handleUserImport: accept these fields in import payload
- handleAuthLogin: include these in login response

### 4. Dev mode auto-import — `internal/app/app.go`
After storage is initialized and before services, check if environment is dev mode. If so, read `import/users.json` (relative to binary dir), parse it, and import users using same logic as the handler (bcrypt hash, skip existing). This is a standalone function — NOT coupled to HTTP handlers.

Extract the import logic from handleUserImport into a shared function in a new file `internal/app/import.go` so both the HTTP handler and the startup code can use it.

### 5. Update README — architecture diagram
- Portal only sends X-Vire-User-ID
- Server resolves user preferences from stored profile
- Remove X-Vire-Portfolios, X-Vire-Display-Currency, X-Vire-Navexa-Key from portal diagram
- Note that headers are still supported for direct API access

### 6. CORS simplification
Keep all X-Vire-* headers in CORS for backward compat. No functional change needed — just the documentation.

## Files Expected to Change
- `internal/models/user.go` — add preference fields
- `internal/server/middleware.go` — resolve all fields from user storage
- `internal/server/handlers_user.go` — support new fields in CRUD
- `internal/app/import.go` — NEW: shared import logic
- `internal/app/app.go` — call dev mode import on startup
- `internal/server/handlers_user_test.go` — update tests for new fields
- `internal/server/middleware_test.go` — update tests for full resolution
- `README.md` — architecture diagram and descriptions
