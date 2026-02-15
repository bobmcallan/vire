# Summary: Resolve startup and request log warnings

**Date:** 2026-02-15
**Status:** Completed

## What Changed

| File | Change |
|------|--------|
| `internal/app/warmcache.go` | Fall back to cached portfolio data when Navexa sync unavailable; downgraded log from WRN to INFO |
| `internal/server/middleware.go` | Changed 4xx HTTP responses from WRN to INFO log level |

## Tests
- All existing tests pass (`go test ./internal/...`, `go vet ./...`)
- Server restarted — startup log shows zero WRN lines
- 404 response confirmed logging at INFO level

## Devils-Advocate Findings
- No issues found — fallback matches existing scheduler.go pattern

## Notes
- Warm cache now gracefully degrades: sync fails → try cached data → skip if none
- 4xx responses are expected client errors, not server warnings
