# Summary: Expose role management through MCP tools

**Date:** 2026-02-23
**Status:** completed

## What Changed

| File | Change |
|------|--------|
| `internal/models/storage.go` | Added `RoleAdmin`, `RoleUser` constants and `ValidateRole()` function |
| `internal/common/userctx.go` | Added `Role` field to `UserContext` struct |
| `internal/server/middleware.go` | Populate `Role` in UserContext from loaded user |
| `internal/server/handlers_admin.go` | Optimised `requireAdmin()` to check context first; added `handleAdminListUsers` (GET /api/admin/users) and `handleAdminUpdateUserRole` (PATCH /api/admin/users/{id}/role) |
| `internal/server/routes.go` | Added routes for `/api/admin/users` and `/api/admin/users/{id}/role` |
| `internal/server/catalog.go` | Added `list_users` and `update_user_role` MCP tools |
| `internal/server/handlers_user.go` | Added role validation on create/update/upsert; role field ignored on non-admin endpoints (prevents escalation) |
| `internal/server/handlers_auth.go` | Added `role` claim to JWT tokens (both OAuth and password login) |
| `README.md` | Documented new MCP tools and admin endpoints |
| `.claude/skills/develop/SKILL.md` | Updated Admin API and User & Auth endpoint tables |

## Tests

### Unit tests
- `internal/models/storage_test.go` (new) — `TestValidateRole`: valid roles, invalid roles, edge cases
- `internal/server/handlers_admin_test.go` (new) — Tests for list users and update role endpoints
- `internal/server/handlers_admin_stress_test.go` (new) — Security stress tests: role escalation, hostile inputs
- `internal/server/handlers_user_test.go` (modified) — Updated for role validation behaviour
- `internal/server/catalog_test.go` (modified) — Updated tool count for new MCP tools

### Integration tests
- `tests/api/admin_users_test.go` (new) — API integration tests for admin user endpoints
- `tests/api/user_test.go` (modified) — Updated role expectations for existing tests

### Test results
- All unit tests pass (`go test ./internal/...`)
- All integration tests pass (`go test ./tests/api/...`)
- `go vet ./...` clean
- `golangci-lint run` clean
- 2 pre-existing test failures fixed (role field expectations in user_test.go)
- Test feedback loop: 1 round (test-executor → implementer → re-run, all passed)

## Documentation Updated
- `README.md` — Added list_users and update_user_role to MCP tools, documented admin user endpoints
- `.claude/skills/develop/SKILL.md` — Updated Admin API table and User & Auth Endpoints table

## Devils-Advocate Findings
- **Critical: Unauthenticated role escalation** — The existing POST /api/users, POST /api/users/upsert, and PUT /api/users/{id} endpoints accepted a `role` field in the request body, allowing any user to set their own role to "admin". Fixed by ignoring the `role` field on non-admin endpoints; only PATCH /api/admin/users/{id}/role (admin-only) can change roles.
- **Minor: String literals vs constants** — handlers_auth.go uses string literals "admin"/"user" instead of models.RoleAdmin/RoleUser in 3 places. Non-blocking (values are correct).

## Notes
- Feedback item: fb_0a24196f
- The `requireAdmin()` optimisation avoids a redundant DB lookup when UserContext already has the role from middleware
- Self-demotion prevention: admins cannot remove their own admin role via the update endpoint
- Full end-to-end admin endpoint testing limited by SurrealDB accessibility (only in Docker network)
