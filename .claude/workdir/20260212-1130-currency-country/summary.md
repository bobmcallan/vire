# Summary: Currency Support (AUD/USD) and Country Field

**Date:** 2026-02-12/13
**Status:** Completed

## What Changed

| File | Change |
|------|--------|
| `internal/models/portfolio.go` | Added `Currency`, `Country` to Holding; `FXRate` to Portfolio; `FXRate` to PortfolioReview; `eodhExchange()` mapping function |
| `internal/models/navexa.go` | Added `Currency` to NavexaHolding; updated `EODHDTicker()` to use `eodhExchange()` |
| `internal/common/config.go` | Added `DisplayCurrency` field (default "AUD"), env override `VIRE_DISPLAY_CURRENCY`, validation (AUD/USD only) |
| `internal/common/format.go` | Added `FormatMoneyWithCurrency` (A$/US$), `FormatSignedMoneyWithCurrency`, `currencySymbol` helper |
| `internal/clients/navexa/client.go` | Mapped `h.CurrencyCode` to `NavexaHolding.Currency` |
| `internal/services/portfolio/service.go` | Currency pass-through (default AUD), country lookup from stored fundamentals, FX rate fetch via EODHD `AUDUSD.FOREX`, FX-converted portfolio totals, FX-converted review liveTotal |
| `cmd/vire-mcp/formatters.go` | Added Ccy/Country columns to holdings tables, currency-aware money formatting, FX rate in sync output, FX-converted subtotals in review |
| `docker/vire.toml` | Added `display_currency = "AUD"` |

## Bugs Fixed During Implementation

1. **Exchange mapping regression** — `EODHDTicker()` was using raw Navexa exchange names ("ASX", "NYSE") instead of EODHD codes ("AU", "US"). Added `eodhExchange()` mapping function.
2. **Review liveTotal missing FX conversion** — `ReviewPortfolio` recomputed `TotalValue` from live holdings without applying FX conversion for USD. Fixed by using `portfolio.FXRate`.
3. **Review formatter subtotals mixing currencies** — Stocks/ETFs subtotals summed `MarketValue` across AUD/USD without conversion. Fixed by applying `fxMul()` in formatter.

## Tests

- `internal/common/config_test.go` — 4 tests (default, TOML, env override, validation)
- `internal/common/format_test.go` — 5 tests (existing, AUD, USD, unknown, signed)
- `internal/services/portfolio/currency_test.go` — 10 tests (model fields, currency mapping, defaults, country lookup, FX conversion, graceful fallback)
- `cmd/vire-mcp/formatters_test.go` — 4 tests (Ccy/Country columns, sort order, review Ccy, FX rate display)
- All existing tests pass unchanged (no regressions)

## Live Verification

- CBOE correctly identified as USD (from Navexa `currencyCode`)
- FX rate 0.7133 fetched from EODHD `AUDUSD.FOREX` endpoint
- CBOE weight calculated correctly: US$33,396 / 0.7133 = A$46,823 → 11.2% of A$417,884
- Graceful fallback: when EODHD unreachable, sync continues with FX rate 0 (USD values added as-is)
- Country field populated from stored fundamentals (`CountryISO` from ISIN prefix)
- Container logs: zero warnings, zero errors

## Devils-Advocate Findings

- 9 approach challenges (all addressed)
- 10 test coverage gaps identified (3 critical, addressed)
- 3 formatter bugs found during stress-test (all fixed)
- Key insight: Navexa's `showLocalCurrency=false` means values are in native currency, confirming FX conversion is needed

## Notes

- Country field requires fundamentals to be fetched first (via review or collect). Empty until then — this is by design.
- Docker DNS can be unreliable (WSL2), causing transient EODHD failures. The code handles this gracefully.
- Only AUD and USD are supported. Other currencies would need additional exchange code mappings and FX pairs.
