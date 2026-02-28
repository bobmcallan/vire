# Break-Glass Admin User

## Summary

Add a break-glass admin user that is created automatically at service startup. The admin credentials are logged to the service log for emergency access.

**Requirements 1 & 2 (list users, role enforcement) are already implemented:**
- `GET /api/admin/users` + `list_users` MCP tool — lists all users (admin only)
- `PATCH /api/admin/users/{id}/role` + `update_user_role` MCP tool — change roles (admin only)
- `handleUserCreate` forces `role = "user"` for all new users
- No changes needed for these.

## Scope: Break-Glass Admin Bootstrap

### Config
Add `breakglass` bool field to `AuthConfig` in `internal/common/config.go`:
```toml
[auth]
breakglass = true
```
Env override: `VIRE_AUTH_BREAKGLASS=true`

### Bootstrap Logic
New file: `internal/app/breakglass.go`

Function `ensureBreakglassAdmin(ctx, internalStore, logger)`:
1. `GetUser(ctx, "breakglass-admin")` — if found, log info and return (idempotent)
2. Generate 24-char cryptographically random password (crypto/rand, base64)
3. bcrypt hash it (cost 10, truncate to 72 bytes like existing code)
4. `SaveUser` with:
   - UserID: `"breakglass-admin"`
   - Email: `"admin@vire.local"`
   - Name: `"Break-Glass Admin"`
   - Provider: `"system"`
   - Role: `models.RoleAdmin`
5. Log at WARN level: `"Break-glass admin created"` with fields `email=admin@vire.local` and `password=<cleartext>` — WARN ensures visibility regardless of log level config

### App Startup Integration
In `internal/app/app.go` `NewApp()`, after storage init (line ~132) and before service init:
```go
if config.Auth.Breakglass {
    ensureBreakglassAdmin(ctx, internalStore, logger)
}
```

### Multi-Instance Safety
- **Check-before-create**: `GetUser` check is a read-only DB query. If admin exists (created by another instance), skip silently.
- **Primary designation**: Only set `VIRE_AUTH_BREAKGLASS=true` on the primary instance.
  - **Fly.io**: Set in `[env]` section of fly.toml, or as a secret on the primary machine only
  - **Local/dev**: Default to `true` in dev config
  - **Docker**: Set via `VIRE_AUTH_BREAKGLASS=true` env var on the primary container
- No distributed locking needed — the config flag controls which instance bootstraps.

### Test
New file: `internal/app/breakglass_test.go`
- Test: creates admin when not exists
- Test: skips when admin already exists (idempotent)
- Test: generated password is valid (bcrypt compare works)
- Test: user has correct fields (role=admin, provider=system, email=admin@vire.local)

## Files Expected to Change
| File | Change |
|------|--------|
| `internal/common/config.go` | Add `Breakglass bool` to `AuthConfig` |
| `internal/app/breakglass.go` | New file — bootstrap logic |
| `internal/app/breakglass_test.go` | New file — unit tests |
| `internal/app/app.go` | Call `ensureBreakglassAdmin` in `NewApp()` |

## Not in Scope
- No new API endpoints (list_users and update_user_role already exist)
- No new MCP tools (already exist)
- No changes to existing role enforcement
- No portal changes
