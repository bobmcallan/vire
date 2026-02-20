# Requirements: API Integration Tests for User & Portfolio Workflow

**Date:** 2026-02-20
**Requested:** Create/update API integration tests covering user import, Navexa key setup, portfolio listing, default portfolio, and single portfolio retrieval.

## Scope

### In Scope
1. Create `tests/docker/.env.example` from existing `.env` (with placeholder values)
2. New test file `tests/api/portfolio_workflow_test.go` that:
   - Imports users from `tests/import/users.json` via `POST /api/users/upsert`
   - Sets Navexa key on dev_user via `PUT /api/users/dev_user` (key from `NAVEXA_API_KEY` env var)
   - Lists portfolios via `GET /api/portfolios` with `X-Vire-User-ID` header — asserts > 1 returned
   - Sets default portfolio via `PUT /api/portfolios/default` (name from `DEFAULT_PORTFOLIO` env var)
   - Gets single portfolio via `GET /api/portfolios/{name}` — asserts response < 120 seconds
3. Add `HTTPRequest` helper to `tests/common/containers.go` for custom header support

### Out of Scope
- Modifying existing tests
- Auth/JWT token flow (tests use X-Vire-* headers directly)
- MCP endpoint tests

## Approach

### Test Flow
The test is a sequential integration test using the Docker test environment (`common.NewEnv(t)`):

1. **Setup:** Start container, load `.env` values (`NAVEXA_API_KEY`, `DEFAULT_PORTFOLIO`)
2. **Import users:** Read `tests/import/users.json`, upsert each user via `POST /api/users/upsert`
3. **Set Navexa key:** `PUT /api/users/dev_user` with `{"navexa_key": "<NAVEXA_API_KEY>"}`
4. **List portfolios:** `GET /api/portfolios` with header `X-Vire-User-ID: dev_user` — middleware resolves navexa_key from storage. Assert HTTP 200, parse response, assert `len(portfolios) > 1`
5. **Set default portfolio:** `PUT /api/portfolios/default` with `{"name": "<DEFAULT_PORTFOLIO>"}` and `X-Vire-User-ID: dev_user` header
6. **Get portfolio:** `GET /api/portfolios/<DEFAULT_PORTFOLIO>` with `X-Vire-User-ID: dev_user` header, measure response time, assert < 120s

### Key Details
- The `Env` helpers (HTTPGet/Post/Put) don't support custom headers. Add an `HTTPRequest(method, path, body, headers)` method to `containers.go`.
- `.env` values are read via `os.Getenv()` in the test — the CI/CD or developer sets these before running.
- The test uses `X-Vire-User-ID: dev_user` header which triggers middleware to resolve the navexa_key from storage (set in step 3).
- Portfolio endpoint requires both UserID and NavexaAPIKey in context (`requireNavexaContext()`).
- Test timeout should be generous (180s) since portfolio sync hits external Navexa API.

## Files Expected to Change

| File | Change |
|------|--------|
| `tests/docker/.env.example` | New file — placeholder version of `.env` |
| `tests/common/containers.go` | Add `HTTPRequest` method for custom headers |
| `tests/api/portfolio_workflow_test.go` | New file — integration test |
