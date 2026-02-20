# Summary: API Integration Tests for User & Portfolio Workflow

**Date:** 2026-02-20
**Status:** Complete

## What Changed

| File | Change |
|------|--------|
| `tests/docker/.env.example` | New file — placeholder `.env` with comments explaining each variable |
| `tests/common/containers.go` | Added `HTTPRequest` method for custom headers; exported `FindProjectRoot()` |
| `tests/api/portfolio_workflow_test.go` | New integration test: user import, navexa key, portfolio list/default/get |

## Tests
- `TestPortfolioWorkflow` — 5 sequential subtests:
  1. `import_users` — imports users from `tests/import/users.json` via `POST /api/users/upsert`
  2. `set_navexa_key` — sets Navexa key on dev_user via `PUT /api/users/dev_user`
  3. `get_portfolios` — lists portfolios via `GET /api/portfolios`, asserts > 1 returned
  4. `set_default_portfolio` — sets default via `PUT /api/portfolios/default`
  5. `get_portfolio` — fetches single portfolio, asserts response < 120 seconds
- Gated on `VIRE_TEST_DOCKER=true`, `NAVEXA_API_KEY`, `DEFAULT_PORTFOLIO`
- `go build ./...` — clean
- `go vet ./...` — clean

## Documentation Updated
- `tests/docker/.env.example` — new file with descriptive comments

## Devils-Advocate Findings
- Loop `defer resp.Body.Close()` in import_users fixed to immediate close
- No secrets exposure in test code (env vars only, skip if not set)
- Test is idempotent (upsert handles re-runs)
- All response bodies properly closed

## Notes
- The `HTTPRequest` method on `Env` enables any test to pass custom headers (X-Vire-User-ID, etc.)
- `FindProjectRoot()` exported so test files can locate fixtures via `tests/import/users.json`
- Integration test requires Docker + real Navexa API access; skips gracefully when env vars missing
