# Summary: Service Registration

**Status:** completed

## Changes

| File | Change |
|------|--------|
| `internal/models/storage.go` | Added `RoleService = "service"` constant, updated `ValidateRole()` |
| `internal/common/config.go` | Added `ServiceKey` to `AuthConfig`, `VIRE_SERVICE_KEY` env override |
| `internal/server/handlers_service.go` | **New**: `handleServiceRegister`, `handleServiceTidy` |
| `internal/server/handlers_service_test.go` | **New**: 24 unit tests |
| `internal/server/handlers_service_stress_test.go` | **New**: 55+ stress tests (devils-advocate) |
| `internal/server/handlers_admin.go` | Added `requireAdminOrService()`, service role guard on PATCH, updated list/update handlers |
| `internal/server/handlers_user.go` | Login block for `provider == "service"` |
| `internal/server/middleware.go` | `X-Vire-Service-ID` resolution, CORS header update |
| `internal/server/routes.go` | Added `/api/services/register` and `/api/admin/services/tidy` routes |
| `tests/api/service_register_test.go` | **New**: 15 integration tests |
| `docs/architecture/auth.md` | Service Registration section + config example |
| `docs/architecture/api.md` | Service registration endpoints + role documentation |

## Tests

- Unit tests: 24 tests in `handlers_service_test.go` — all PASS
- Stress tests: 55+ tests in `handlers_service_stress_test.go` — all PASS
- Integration tests: 15 tests in `tests/api/service_register_test.go` — created (need Docker to run)
- `go build ./cmd/vire-server/` — PASS
- `go vet ./...` — PASS
- Pre-existing failures: `TestStress_WriteRaw_AtomicWrite` (surrealdb), `TestFeedbackStress_List_HostileQueryParams` (feedback) — not related

## Security Fix

- Changed key comparison from `!=` to `crypto/subtle.ConstantTimeCompare()` to prevent timing attacks (team-lead fix)

## Architecture

- Docs updated by architect and implementer: `docs/architecture/auth.md`, `docs/architecture/api.md`
- Pattern-consistent with existing `requireAdmin()`, breakglass, middleware, and route patterns

## Devils-Advocate

- 55+ stress tests covering: injection in service_id, timing attacks, empty/huge payloads, concurrent registration, key boundary conditions, middleware edge cases
- Key finding: timing attack on key comparison — fixed with constant-time compare

## Notes

- Middleware design: when both `X-Vire-User-ID` and `X-Vire-Service-ID` are present, service role overrides user role. This is intentional for portal use case (portal proxies user requests with service auth).
- Feature is backward-compatible: no changes to existing users, endpoints, or behavior
- Service key must be ≥32 characters on both client and server side
