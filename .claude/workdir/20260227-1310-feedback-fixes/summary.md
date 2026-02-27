# Summary: Fix 4 Feedback Items

**Status:** completed

## Changes

| File | Change |
|------|--------|
| `internal/clients/eodhd/client.go` | Added `tickerMatches()` validation helper, response validation in `GetRealTimeQuote()` |
| `internal/clients/eodhd/client_test.go` | 11 unit tests for ticker matching |
| `internal/clients/eodhd/ticker_stress_test.go` | 16 stress tests for ticker edge cases |
| `internal/signals/indicators.go` | Fixed EMA loop direction, implemented Wilder's smoothing for RSI |
| `internal/signals/indicators_test.go` | Unit tests for corrected EMA/RSI |
| `internal/signals/computer.go` | Fixed nil pointer dereference when marketData is nil |
| `internal/services/signal/service.go` | Return error entries instead of silently skipping failed tickers |
| `internal/models/portfolio.go` | Exported `eodhExchange()` → `EodhExchange()` |
| `internal/models/signals.go` | Added `Error` field to `TickerSignals` model |
| `internal/services/strategy/compliance.go` | Use `EodhExchange()` for exchange→country mapping in universe check |
| `internal/services/strategy/compliance_test.go` | Fixed and added compliance tests (ASX→AU, NYSE→US, LSE rejection) |
| `internal/services/strategy/compliance_stress_test.go` | 16 stress tests for compliance edge cases |
| `internal/signals/indicators_stress_test.go` | 34 stress tests for EMA/RSI edge cases |
| `internal/services/portfolio/indicators_stress_test.go` | RSI tolerance fix for Wilder's smoothing |
| `internal/clients/navexa/client.go` | Updated call from `eodhExchange()` to `EodhExchange()` |

## Bugs Fixed

### Bug 1: EODHD Ticker Resolution (fb_9f137670 + fb_fb1b044f)
- Added `tickerMatches()` to validate EODHD API responses match requested tickers
- Prevents ACDC.AU from silently returning US-listed ACDC (~$5) data
- Catches exchange suffix mismatches

### Bug 2a: compute_indicators Silent Failure (fb_5581bfa1)
- `DetectSignals()` now returns error entries for failed tickers instead of silently skipping
- Added `Error` field to `TickerSignals` model with `omitempty`

### Bug 2b: EMA Backward Loop (fb_5581bfa1)
- Fixed loop to iterate from SMA seed window toward newest bar
- Previously iterated backwards causing exponential value explosion

### Bug 2c: RSI Without Wilder's Smoothing (fb_5581bfa1)
- Implemented Wilder's smoothing for proper gain/loss averaging
- Prevents RSI=100 for all growing portfolios

### Bonus: Nil Pointer in Computer.Compute()
- Added nil check for `marketData` before accessing fields
- Found by devils-advocate adversarial analysis

### Bug 3: Compliance Exchange Mapping (fb_c6fd4a3e)
- Exported `EodhExchange()` mapping function
- Compliance now maps ASX→AU, NYSE/NASDAQ→US before universe comparison
- Fixed existing test that used wrong exchange value

## Tests
- 40+ unit tests added/modified across all fixes
- 66 stress tests added (indicators, ticker, compliance)
- Integration tests created by test-creator
- All modified packages pass tests
- Pre-existing failures documented (feedback sorting, storage nil, config)

## Architecture
- No architecture changes — all fixes are implementation-level
- Docs validated as accurate by reviewer

## Devils-Advocate
- 66 stress tests covering edge cases, hostile inputs, nil handling
- Found critical nil dereference in computer.go — fixed by implementer
- No remaining security or stability concerns

## Notes
- RSI Wilder's smoothing produces ~46.87 for equal alternating gains/losses (not exactly 50) — this is mathematically correct due to recency weighting
- Feedback sorting failures (sort_created_at_asc/desc, before_filter) are pre-existing and unrelated to these changes
- Storage nil pointer in WriteRaw stress test is pre-existing (requires running SurrealDB)
