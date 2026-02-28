# Service Registration: Implementation Requirements

**Feature:** Portal instances register as service users with vire-server using a shared key
**Spec:** `docs/features/20260228-service-registration.md`

## Scope

Service users can list users and update roles (for admin sync at startup) but cannot login via password or OAuth. A new `"service"` role, registration endpoint, middleware auth header, and tidy endpoint.

## Approach

Follow existing patterns exactly — role constants in models, config in common, handlers in server, middleware in server, routes in server.

## Files to Change

### 1. `internal/models/storage.go` (Lines 8-22)
- Add `RoleService = "service"` constant at line 11
- Update `ValidateRole()` switch to include `RoleService`

### 2. `internal/common/config.go`
- Add `ServiceKey string` field to `AuthConfig` struct (line ~219, after `Breakglass bool`)
  - TOML tag: `toml:"service_key"`
- Add env override at line ~487 (after VIRE_AUTH_BREAKGLASS block):
  ```go
  if v := os.Getenv("VIRE_SERVICE_KEY"); v != "" {
      config.Auth.ServiceKey = v
  }
  ```

### 3. `internal/server/handlers_service.go` (NEW FILE)
Two handlers:

**handleServiceRegister** (`POST /api/services/register`):
- Decode JSON body: `service_id`, `service_key`, `service_type`
- Validate: server key configured (501), key match (403), key length >=32 (400), service_id non-empty (400)
- Create/update service user via `s.app.Storage.InternalStore().SaveUser()`:
  - UserID: `service:<service_id>`
  - Email: `<service_id>@service.vire.local`
  - Name: `Service: <service_id>`
  - PasswordHash: `""` (empty)
  - Provider: `"service"`
  - Role: `models.RoleService`
  - CreatedAt: now (only on create)
  - ModifiedAt: now (always)
- Idempotent: check GetUser first, if exists only update ModifiedAt
- Return 200 with `{"status":"ok","service_user_id":"service:...","registered_at":"..."}`

**handleServiceTidy** (`POST /api/admin/services/tidy`):
- Require admin (use `requireAdmin()`, NOT requireAdminOrService)
- List all users, filter by `provider == "service"`
- Delete users where `ModifiedAt` older than 7 days
- Return `{"purged": N, "remaining": M}`

### 4. `internal/server/handlers_admin.go`
- Add `requireAdminOrService()` method (parallel to `requireAdmin()`, lines 13-44)
  - Accept both `models.RoleAdmin` and `models.RoleService` roles
  - Same two-path structure: UserContext fast path + header fallback
  - For the header fallback path, check BOTH `X-Vire-User-ID` AND `X-Vire-Service-ID` headers
- Update `handleAdminListUsers()` to use `requireAdminOrService()` instead of `requireAdmin()`
- Update `handleAdminUpdateUserRole()` to use `requireAdminOrService()` instead of `requireAdmin()`
- Add guard in `handleAdminUpdateUserRole()`: reject `"service"` as target role with 400

### 5. `internal/server/middleware.go`
- In `userContextMiddleware` (line ~265), add `X-Vire-Service-ID` resolution:
  1. Read `X-Vire-Service-ID` header
  2. Look up user in store
  3. Verify `role == "service"`
  4. Set on UserContext (add ServiceID field or use existing Role field)
  - Priority: Bearer token > X-Vire-User-ID > X-Vire-Service-ID
- In `corsMiddleware` (line ~57), add `X-Vire-Service-ID` to allowed headers

### 6. `internal/server/routes.go`
- Add routes (after user routes, before OAuth):
  ```go
  mux.HandleFunc("/api/services/register", s.handleServiceRegister)
  ```
- Add admin route (in admin block):
  ```go
  mux.HandleFunc("/api/admin/services/tidy", s.handleServiceTidy)
  ```

### 7. `internal/server/handlers_user.go`
- In `handleAuthLogin()` (line ~445), after user lookup but before bcrypt:
  ```go
  if user.Provider == "service" {
      WriteError(w, http.StatusForbidden, "service accounts cannot login")
      return
  }
  ```

### 8. Documentation
- `docs/architecture/auth.md`: Add Service Registration section after Break-Glass
- `docs/architecture/api.md`: Add service endpoints

## Test Cases

### Unit Tests (in handlers_service_test.go)
- Valid registration creates service user
- Re-registration updates modified_at (idempotent)
- Wrong key returns 403
- Missing server key returns 501
- Empty service_id returns 400
- Short key (<32 chars) returns 400
- Tidy purges stale, preserves recent
- requireAdminOrService accepts admin role
- requireAdminOrService accepts service role
- requireAdminOrService rejects user role
- Login rejects service provider

### Integration Tests (in tests/api/)
- Full registration flow with admin access
- Service user PATCH role endpoint
- Service user cannot access job endpoints
- Tidy endpoint cleanup
