# Requirements: Expose role management through MCP tools

**Date:** 2026-02-23
**Requested:** Feedback item fb_0a24196f — InternalUser has a role field (admin/user) but role-based access is only enforced on admin endpoints via requireAdmin(). Expose role management through MCP and harden role handling.

## Scope

**In scope:**
- New admin endpoints for listing users and updating user roles
- New MCP tools (`list_users`, `update_user_role`) in catalog.go
- Role constants and validation (reject arbitrary strings)
- Include role in JWT claims for portal awareness
- Add Role to UserContext to avoid redundant DB lookups in requireAdmin()
- Role validation on existing user create/update/upsert handlers

**Out of scope:**
- New role types beyond "admin" and "user" (keep existing two-role model)
- RBAC / permission matrix (premature — only two roles exist)
- Portal UI changes for role management
- Changes to OAuth providers or Navexa client

## Approach

### 1. Role constants and validation (`internal/models/storage.go`)
Add constants `RoleAdmin = "admin"`, `RoleUser = "user"` and a `ValidateRole(role string) error` function that rejects anything except these two values.

### 2. UserContext enhancement (`internal/common/userctx.go` + `internal/server/middleware.go`)
Add `Role string` field to `UserContext`. The middleware already loads the user via `GetUser()` at line ~140 — populate `uc.Role` from `user.Role`. This avoids a second DB lookup in `requireAdmin()`.

### 3. Optimise requireAdmin (`internal/server/handlers_admin.go`)
Check `UserContext.Role` from request context first. Only fall back to DB lookup if user context doesn't have a role (backward compat for requests without the middleware).

### 4. New admin endpoints (`internal/server/handlers_admin.go` + `routes.go`)
- `GET /api/admin/users` — List all users with id, email, name, provider, role, created_at. Password hashes excluded. Admin-only via requireAdmin().
- `PATCH /api/admin/users/{id}/role` — Update role for a user. Body: `{"role": "admin"|"user"}`. Validates role, prevents self-demotion (admin can't remove their own admin). Admin-only.

### 5. MCP tools (`internal/server/catalog.go`)
- `list_users` → GET /api/admin/users
- `update_user_role` → PATCH /api/admin/users/{id}/role with `id` (path) and `role` (body) params

### 6. Role validation on existing handlers (`internal/server/handlers_user.go`)
Apply `ValidateRole()` in `handleUserCreate`, `handleUserUpsert`, and `handleUserUpdate`. Return 400 on invalid role.

### 7. JWT claims (`internal/server/handlers_auth.go`)
Add `"role"` claim to JWT token generation. Appears in two places: `handleAuthOAuth` (line ~195) and `handleAuthLogin` (line ~470+). The role is available from the loaded user object.

## Files Expected to Change
- `internal/models/storage.go` — role constants, ValidateRole
- `internal/common/userctx.go` — Role field on UserContext
- `internal/server/middleware.go` — populate Role in user context
- `internal/server/handlers_admin.go` — requireAdmin optimisation + new handlers
- `internal/server/routes.go` — new routes
- `internal/server/catalog.go` — new MCP tools
- `internal/server/handlers_user.go` — role validation
- `internal/server/handlers_auth.go` — role in JWT claims
- `internal/models/storage_test.go` (new) — ValidateRole tests
- `internal/server/handlers_admin_test.go` (new or existing) — handler tests
