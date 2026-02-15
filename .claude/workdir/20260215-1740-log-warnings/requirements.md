# Requirements: Resolve startup and request log warnings

**Date:** 2026-02-15
**Requested:** Review and fix warnings in bin/logs/vire.2026-02-15T17-24-40.log

## Scope

### In scope
- Fix warm cache returning early when Navexa sync fails (should fall back to cached portfolio data)
- Fix 4xx HTTP responses logged as WRN (should be INFO — client errors are expected)

### Out of scope
- Navexa client architecture changes
- Log aggregation or monitoring

## Approach

### 1. Warm cache fallback (`internal/app/warmcache.go`)
When `SyncPortfolio` fails (no Navexa client in headless/standalone mode), fall back to reading existing portfolio data from storage — same pattern the price scheduler already uses in `scheduler.go:44`. Log the sync failure as INFO (expected when running without portal), not WRN.

### 2. HTTP logging levels (`internal/server/middleware.go`)
Change 4xx responses from WRN to INFO. 4xx are client-side errors (bad input, not found, conflict, auth failure) — expected in normal operation. Only 5xx should be elevated.

## Files Expected to Change
- `internal/app/warmcache.go`
- `internal/server/middleware.go`
