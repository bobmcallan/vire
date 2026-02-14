# Requirements: Fallback price source via ASX Markit Digital API

**Date:** 2026-02-13
**Requested:** EODHD does not provide timely real-time data for certain ASX ETFs/ETPs (confirmed: ETPMAG, likely PMGOLD). Need a fallback source so quotes are never stale during market hours.

## Problem

- EODHD `/real-time/ETPMAG.AU` returns yesterday's close during live trading
- BHP.AU, CBA.AU, SXE.AU all return correct 20-min delayed data from EODHD
- The issue is EODHD's coverage gap for certain ETPs, not a code bug
- The ASX Markit Digital API (`asx.api.markitdigital.com`) returns live data for all ASX instruments, confirmed via curl

## Scope

### In scope
- New ASX client (`internal/clients/asx/`) to call Markit Digital `/companies/{ticker}/header` endpoint
- New quote service (`internal/services/quote/`) that orchestrates fallback: EODHD → ASX Markit
- Staleness detection: if EODHD quote timestamp is >2 hours old during AEST market hours, try fallback
- Add `Source` field to `RealTimeQuote` model to indicate data provenance ("eodhd" or "asx")
- Wire quote service into App and update handler to use it
- Add ASX client interface to `internal/interfaces/clients.go`
- Tests for fallback logic and ASX response mapping

### Out of scope
- Changing EODHD subscription or contacting EODHD support
- Google Finance or other fallback sources (ASX Markit confirmed sufficient)
- Caching layer (existing FreshnessRealTimeQuote TTL covers this at handler level)

## Approach

1. Create `internal/clients/asx/client.go` — HTTP client calling `https://asx.api.markitdigital.com/asx-research/1.0/companies/{TICKER}/header`
2. Create `internal/services/quote/service.go` — tries EODHD first, if stale during market hours, falls back to ASX
3. Map Markit response fields (`priceLast`, `priceChange`, `priceChangePercent`, `volume`, etc.) to `RealTimeQuote`
4. Add `Source string` field to `RealTimeQuote`
5. Update `handleMarketQuote` to use the quote service instead of directly calling EODHD client
6. Update `portfolio/service.go` review flow to use quote service for live prices (if applicable)

## ASX Markit Digital API

Confirmed working endpoint:
```
GET https://asx.api.markitdigital.com/asx-research/1.0/companies/ETPMAG/header
```

Response fields:
- `priceLast`: current/last price (99.63)
- `priceChange`: absolute change (-8.93)
- `priceChangePercent`: percentage change (-8.22)
- `volume`: trading volume (397960)
- `priceAsk`, `priceBid`: spread
- `priceOpen`, `priceHigh`, `priceLow`: intraday OHLC

## Files Expected to Change

- `internal/models/market.go` — add `Source` to `RealTimeQuote`
- `internal/clients/asx/client.go` — new ASX Markit Digital client
- `internal/interfaces/clients.go` — add `ASXClient` interface
- `internal/services/quote/service.go` — new fallback quote service
- `internal/app/app.go` — wire ASX client and quote service
- `internal/server/handlers.go` — use quote service in `handleMarketQuote`
- `internal/services/portfolio/service.go` — use quote service for live prices if currently using EODHD directly
