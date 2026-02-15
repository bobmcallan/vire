# Summary: User profile preferences + dev mode auto-import

**Date:** 2026-02-15
**Status:** Completed

## What Changed

| File | Change |
|------|--------|
| `internal/models/user.go` | Added `DisplayCurrency`, `DefaultPortfolio`, `Portfolios` fields to User struct |
| `internal/server/middleware.go` | Middleware now resolves ALL user context fields from stored profile (base layer), with X-Vire-* headers as overrides |
| `internal/server/handlers_user.go` | User CRUD and auth responses include new preference fields; update handler accepts them |
| `internal/app/import.go` | New shared `ImportUsersFromFile` function for both startup and handler reuse |
| `internal/app/app.go` | Dev mode auto-import from `import/users.json` on startup |
| `internal/interfaces/storage.go` | UserStorage interface updated (no signature change, preferences stored in User model) |
| `internal/storage/file.go` | User storage persists new preference fields (already handled by JSON serialization) |
| `internal/storage/manager.go` | No functional change (UserStorage accessor already wired) |
| `internal/server/routes.go` | No change to routes (endpoints already registered) |
| `internal/server/server.go` | Middleware constructor updated to pass UserStorage |
| `README.md` | Architecture diagram updated: portal sends only X-Vire-User-ID, server resolves preferences |
| `.claude/skills/develop/SKILL.md` | Added Dev Mode Auto-Import section, updated middleware and user model documentation |
| `scripts/run.sh` | Copies `import/` directory to binary dir for dev mode auto-import |

## Tests
- `internal/server/middleware_test.go` — 239+ lines added: tests for full profile resolution, header override precedence, missing user fallback, partial header overrides
- `internal/server/handlers_user_test.go` — existing tests updated for new preference fields in responses
- `internal/app/import_test.go` — tests for ImportUsersFromFile (valid file, missing file, idempotent skip)
- `internal/storage/file_test.go` — user storage tests updated for preference field round-trip
- All tests pass: `go test ./internal/...` (0 failures)
- `go vet ./...` clean
- Server builds, runs, health endpoint responds `{"status":"ok"}`

## Documentation Updated
- `README.md` — architecture diagram simplified (portal sends X-Vire-User-ID only), noted server resolves preferences
- `.claude/skills/develop/SKILL.md` — added Dev Mode Auto-Import section, updated User model and Middleware documentation

## Devils-Advocate Findings
- Stress tests on preference fields (empty strings, very long values, special characters) — handled correctly by existing validation
- Import file edge cases (missing file, malformed JSON, duplicate users) — ImportUsersFromFile handles all gracefully
- No new security issues found beyond those already addressed in the user storage implementation

## Notes
- Backward compatibility preserved: individual X-Vire-* headers still work as overrides for direct API access
- Profile is the base layer, headers override — this means portal only needs X-Vire-User-ID
- `import/users.json` is only loaded in non-production mode (environment != "production"/"prod")
- The shared `ImportUsersFromFile` function can be reused by the HTTP import handler if desired
