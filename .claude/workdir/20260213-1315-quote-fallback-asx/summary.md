# Summary: Fallback price source via ASX Markit Digital API

**Date:** 2026-02-13
**Status:** completed

## What Changed

| File | Change |
|------|--------|
| `internal/models/market.go` | Added `Source string` field to `RealTimeQuote` |
| `internal/clients/asx/client.go` | New ASX Markit Digital API client — calls `/companies/{TICKER}/header`, maps priceLast/priceChange/volume to RealTimeQuote |
| `internal/clients/asx/client_test.go` | 7 tests: response parsing, suffix stripping, API errors, timeout, invalid JSON, zero price |
| `internal/clients/eodhd/client.go` | Added `Source: "eodhd"` to `GetRealTimeQuote` return |
| `internal/interfaces/clients.go` | Added `ASXClient` interface |
| `internal/interfaces/services.go` | Added `QuoteService` interface |
| `internal/services/quote/service.go` | New quote service: EODHD-primary, ASX-fallback chain with 2-hour staleness threshold, Sydney timezone-aware market hours |
| `internal/services/quote/service_test.go` | 13 tests: fresh/stale EODHD, ASX fallback success/failure, market hours, weekends, non-AU tickers, FOREX, nil client, zero timestamp |
| `internal/app/app.go` | Added `ASXClient` and `QuoteService` fields, wired in `NewApp` |
| `internal/server/handlers.go` | `handleMarketQuote` uses QuoteService instead of EODHDClient directly; added `source` to response |

## Tests
- 20 new tests (7 ASX client + 13 quote service)
- All existing tests pass unchanged
- Race detector clean
- Full suite: all 22 packages pass

## Review Findings
- **DST bug caught**: `time.FixedZone("AEST", 10*60*60)` replaced with `time.LoadLocation("Australia/Sydney")` — Australia is in AEDT (UTC+11) during February
- **Phantom API fields caught**: ASX Markit `/header` endpoint doesn't return open/high/low/prevClose — removed from response struct
- **EODHD Source field**: Added `Source: "eodhd"` at client level for provenance on all callers
- **Staleness threshold debate**: Reviewer recommended 15 min, lead initially agreed, then reversed to 2 hours after deploy testing showed 15 min would trigger unnecessary fallbacks for every normal ASX quote (EODHD has ~20 min delay)

## Deploy Validation
- ETPMAG.AU: source "asx", close $99.45 — fallback working
- BHP.AU: source "eodhd", close $51.35 — no unnecessary fallback (20-min delay under 2h threshold)
- AUDUSD.FOREX: source "eodhd", close $0.7092 — FOREX correctly excluded

## Notes
- ASX Markit Digital API is unauthenticated (public endpoint), no API key needed
- Fallback only applies to `.AU` tickers during ASX market hours (10:00-16:30 Sydney time, Mon-Fri)
- Portfolio service and market service continue to use EODHD directly — can route through QuoteService in a follow-up if needed
- MCP tool formatters don't show Source field yet — follow-up item
