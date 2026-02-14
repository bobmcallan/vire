# Summary: Fix Container Warnings

**Date:** 2026-02-12
**Status:** Completed

## What Changed

| File | Change |
|------|--------|
| `internal/clients/eodhd/client.go` | Added `flexInt64` type (mirrors existing `flexFloat64`) for handling JSON values that may be number or string. Updated `realTimeResponse` struct to use `flexInt64` for `Timestamp` and `Volume` fields. |
| `internal/clients/eodhd/realtime_test.go` | Added `TestGetRealTimeQuote_StringTimestamp` and `TestFlexInt64_UnmarshalJSON` tests. |
| `internal/models/portfolio.go` | Added `Holding.EODHDTicker()` method — constructs EODHD-format ticker using the holding's exchange (e.g., "BHP.AU", "CBOE.US"). |
| `internal/models/navexa.go` | Added `NavexaHolding.EODHDTicker()` method — same pattern for Navexa holdings. |
| `internal/services/portfolio/service.go` | Replaced 4 hardcoded `+ ".AU"` with `EODHDTicker()` calls. |
| `internal/services/portfolio/growth.go` | Replaced 2 hardcoded `+ ".AU"` with `EODHDTicker()` calls. |
| `internal/services/portfolio/snapshot.go` | Replaced 1 hardcoded `+ ".AU"` with `EODHDTicker()` call. |
| `internal/services/report/service.go` | Replaced 1 hardcoded `+ ".AU"` with `EODHDTicker()` call. Added portfolio lookup for ticker report exchange resolution. |
| `internal/app/warmcache.go` | Replaced hardcoded `+ ".AU"` with `EODHDTicker()` call. |
| `internal/app/scheduler.go` | Replaced hardcoded `+ ".AU"` with `EODHDTicker()` call. |
| `internal/server/handlers.go` | Replaced hardcoded `+ ".AU"` in `extractTickers()` with `EODHDTicker()` call. |

## Root Causes

### Issue 1: JSON timestamp unmarshal error
EODHD's real-time API returns `timestamp` as a JSON string for certain tickers. The `realTimeResponse` struct used bare `int64`, causing `json.Unmarshal` to fail.

### Issue 2: Hardcoded `.AU` exchange suffix
All EODHD ticker construction hardcoded `.AU` (e.g., `h.Ticker + ".AU"`), ignoring the actual exchange from Navexa. CBOE is a US stock (NYSE) but was being queried as `CBOE.AU`, resulting in EODHD 404s. The Navexa API already provides the correct exchange via `displayExchange` field, and the holding model already stored it — it was just never used.

## All Warnings Resolved

| Warning | Root Cause | Fix |
|---------|-----------|-----|
| `Real-time quote unavailable for holding` | timestamp unmarshal error | `flexInt64` type |
| `No market data in batch` (CBOE.AU) | Wrong exchange suffix | `EODHDTicker()` uses Navexa exchange |
| `Failed to get market data for signal detection` | No EOD data due to wrong ticker | `EODHDTicker()` |
| `EODHD API non-OK response` (404 /eod/CBOE.AU) | CBOE.AU doesn't exist, CBOE.US does | `EODHDTicker()` |
| `Failed to fetch EOD data` (CBOE.AU) | Cascading from 404 | `EODHDTicker()` |

## Tests
- `TestGetRealTimeQuote_StringTimestamp` — PASS
- `TestFlexInt64_UnmarshalJSON` (8 sub-tests) — PASS
- All existing tests — PASS (`go test ./internal/...`)
- Docker rebuild and deploy — successful
- Post-deploy log verification — **zero warnings**
- Manual verification: `CBOE.US` returns HTTP 200 from EODHD for both EOD and fundamentals

## Notes
- `EODHDTicker()` falls back to `.AU` if the exchange field is empty, for backward compatibility with any holdings that might have been stored before exchange data was populated.
- The `flexInt64` pattern matches the existing `flexFloat64` pattern already used for OHLC price fields.
