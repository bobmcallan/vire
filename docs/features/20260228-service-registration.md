# Service Registration: Server Requirements

**Date:** 2026-02-28
**Status:** Requirements

## Overview

Portal instances register as service users with vire-server using a shared key. Service users can list users and update roles (for admin sync at startup) but cannot login via password or OAuth.

This enables the portal to manage admin users at startup without requiring an existing admin user (chicken-and-egg problem) and without exposing unauthenticated internal endpoints.

## Motivation

- The portal needs to sync configured admin emails at startup via `PATCH /api/admin/users/{id}/role`
- Admin endpoints require admin or service role — no unauthenticated access
- Breakglass admin credentials are not available to the portal (logged to server container only)
- Multiple portal instances must each register independently with unique IDs
- Standard role-based approach avoids provider lock-in and public exposure risks

## New Role: "service"

Add `RoleService = "service"` alongside existing `RoleAdmin` and `RoleUser`.

### Permissions

| Action | Allowed |
|--------|---------|
| List users (`GET /api/admin/users`) | Yes |
| Update user roles (`PATCH /api/admin/users/{id}/role`) | Yes |
| Login via password (`POST /api/auth/login`) | No |
| Login via OAuth (`POST /api/auth/oauth`) | No |
| Access portfolio, market, scan, report endpoints | No |
| Access MCP protocol | No |
| Manage jobs | No |

### Restrictions

- The `PATCH /api/admin/users/{id}/role` endpoint must reject `"service"` as a target role — only the registration endpoint can create service users
- Login handlers must explicitly reject users with `provider: "service"`

## Registration Endpoint

### `POST /api/services/register`

**Request:**
```json
{
  "service_id": "portal-prod-1",
  "service_key": "the-shared-secret",
  "service_type": "portal"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `service_id` | string | Yes | Unique identifier for this portal instance |
| `service_key` | string | Yes | Shared secret matching server's `VIRE_SERVICE_KEY` |
| `service_type` | string | Yes | Fixed string `"portal"` (extensible for future service types) |

**Validation:**
1. If `VIRE_SERVICE_KEY` is not configured on the server, return `501 Not Implemented`
2. If `service_key` does not match, return `403 Forbidden`
3. If `service_key` is shorter than 32 characters (on both sides), reject with `400 Bad Request`
4. If `service_id` is empty, return `400 Bad Request`

**Success Response (`200 OK`):**
```json
{
  "status": "ok",
  "service_user_id": "service:portal-prod-1",
  "registered_at": "2026-02-28T10:00:00Z"
}
```

### Service User Record

Created in the `user` table with the standard `InternalUser` struct:

| Field | Value |
|-------|-------|
| `user_id` | `service:<service_id>` (e.g. `service:portal-prod-1`) |
| `email` | `<service_id>@service.vire.local` |
| `name` | `Service: <service_id>` |
| `password_hash` | `""` (empty — bcrypt comparison always fails) |
| `provider` | `"service"` |
| `role` | `"service"` |
| `created_at` | Creation timestamp |
| `modified_at` | Updated on every re-registration (heartbeat) |

### Idempotency

Re-registration is fully idempotent. The same portal instance can register on every restart. If the service user already exists, only `modified_at` is updated (serves as last-seen heartbeat).

## Authentication Header

### `X-Vire-Service-ID`

After registration, the portal sends `X-Vire-Service-ID: service:portal-prod-1` on admin API calls.

This is a **separate header** from `X-Vire-User-ID` (which identifies end users). Both can be present on the same request — `X-Vire-User-ID` carries the end user identity for the API operation, `X-Vire-Service-ID` authorizes the portal to make the call.

### Middleware Resolution

In `userContextMiddleware`, add `X-Vire-Service-ID` resolution:

1. Read `X-Vire-Service-ID` header
2. Look up user with that ID in the store
3. Verify `role == "service"`
4. Populate a service context on the request (separate from user context)

Priority: Bearer token > `X-Vire-User-ID` > `X-Vire-Service-ID`. Service identity is the lowest priority.

### CORS

Add `X-Vire-Service-ID` to `Access-Control-Allow-Headers`.

## Admin Endpoint Changes

### `requireAdminOrService()`

New method alongside existing `requireAdmin()`. Accepts both `admin` and `service` roles. Applied only to:

- `GET /api/admin/users`
- `PATCH /api/admin/users/{id}/role`

All other admin endpoints (jobs, stock-index, WebSocket) continue using `requireAdmin()` (admin-only).

## Tidy Job

### `POST /api/admin/services/tidy`

Admin-only endpoint to clean up stale service registrations.

**Behaviour:**
1. List all users with `provider = "service"`
2. Delete users where `modified_at` is older than 7 days (configurable)
3. Return `{"purged": 2, "remaining": 1}`

**Impact on running portals:** A deleted service user causes the portal's `X-Vire-Service-ID` to fail auth. The portal logs a warning and continues operating (admin sync is non-fatal). On next restart, the portal re-registers.

## Configuration

### New Config Option

**TOML** (`vire-service.toml`):
```toml
[auth]
service_key = ""
```

**Environment variable:**
```
VIRE_SERVICE_KEY=<32+ char random string>
```

If unset, service registration returns `501` and the portal falls back to operating without admin sync.

## Login Block

The `POST /api/auth/login` handler must reject login attempts for service users:

- If user's `provider == "service"`, return `403 Forbidden` with `{"error": "service accounts cannot login"}`
- This is a safety net — the empty password hash already prevents bcrypt match, but an explicit check is clearer

## Files to Change

| File | Change |
|------|--------|
| `internal/models/storage.go` | Add `RoleService` constant, update `ValidateRole` |
| `internal/common/config.go` | Add `ServiceKey` to `AuthConfig`, add `VIRE_SERVICE_KEY` env override |
| `internal/server/handlers_service.go` | **New**: `handleServiceRegister`, `handleServiceTidy` |
| `internal/server/handlers_admin.go` | Add `requireAdminOrService()`, use on user management endpoints |
| `internal/server/middleware.go` | Add `X-Vire-Service-ID` resolution, add to CORS headers |
| `internal/server/routes.go` | Add `/api/services/register` and `/api/admin/services/tidy` routes |
| `internal/server/handlers_user.go` | Block login for `provider == "service"` |
| `tests/api/service_register_test.go` | **New**: Integration tests |
| `tests/api/admin_users_test.go` | Add tests for service role accessing admin endpoints |

## Test Cases

### Registration

- Valid registration creates service user
- Re-registration updates `modified_at` (idempotent)
- Wrong key returns 403
- Missing key on server returns 501
- Empty `service_id` returns 400
- Short key (<32 chars) returns 400

### Admin Access

- Service user can `GET /api/admin/users`
- Service user can `PATCH /api/admin/users/{id}/role`
- Service user cannot access job endpoints (403)
- Service user cannot promote to "service" role (400)
- Service user self-demotion guard applies

### Login Block

- `POST /api/auth/login` rejects service user with 403
- `POST /api/auth/oauth` does not match service provider

### Tidy

- Stale service users are purged
- Recent service users are preserved
- Non-service users are not affected
