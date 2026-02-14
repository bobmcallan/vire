# Requirements: Fix Container Warnings

**Date:** 2026-02-12
**Requested:** Review container logs for vire-server, understand and fix warnings, redeploy and test.

## Warnings Found

All warnings relate to ticker `CBOE.AU`:

### Issue 1: EODHD API 404 for CBOE.AU
- EODHD returns 404 for `/eod/CBOE.AU` — ticker not found
- Cascades through: market data collection → signal detection → portfolio review
- Logs: "EODHD API non-OK response", "Failed to fetch EOD data", "Failed to get market data for signal detection", "No market data in batch"

### Issue 2: Real-time quote JSON decode error
- `json: cannot unmarshal string into Go struct field realTimeResponse.timestamp of type int64`
- EODHD API returns `timestamp` as a string, but `realTimeResponse.Timestamp` is typed `int64`
- The code already has `flexFloat64` for handling string/number flexibility in price fields but lacks equivalent for `int64`

## Scope
- **In scope:** Fix the `realTimeResponse.timestamp` unmarshal error; handle unknown/invalid tickers gracefully so warnings don't recur on every call
- **Out of scope:** Changing how tickers are sourced from Navexa; adding new EODHD features

## Approach
1. Add a `flexInt64` custom type (similar to existing `flexFloat64`) for the `timestamp` field
2. Add graceful handling for tickers that consistently 404 from EODHD — either skip them after first failure or log at debug level
3. Redeploy and verify warnings are resolved

## Files Expected to Change
- `internal/clients/eodhd/client.go` — add `flexInt64`, update `realTimeResponse`
- `internal/services/market/service.go` — potentially improve handling of unknown tickers
