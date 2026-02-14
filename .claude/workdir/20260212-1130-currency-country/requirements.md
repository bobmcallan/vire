# Requirements: Currency Support and Country Field

**Date:** 2026-02-12
**Requested:** Two features for multi-currency portfolio support and country metadata.

## Feature 1: Currency Support

CBOE ($33,396) is priced in USD. Currently all amounts are summed without conversion and displayed with a generic `$` symbol.

### Requirements
- Add `Currency` field to `Holding` and `NavexaHolding` models (from Navexa `currencyCode`)
- Add `display_currency` config to vire.toml (default: `"AUD"`)
- Only AUD and USD are valid currencies
- Use EODHD real-time forex endpoint (`AUDUSD.FOREX`) to get FX rate for conversion
- Convert foreign-currency holdings to display currency for:
  - Individual holding MarketValue, GainLoss, TotalCost, DividendReturn, TotalReturnValue
  - Portfolio totals (TotalValue, TotalCost, TotalGain)
- Show currency marker in formatted output (e.g., "A$" for AUD, "US$" for USD)
- Store original (native) currency amounts alongside converted amounts
- Navexa provides `currencyCode` on holdings but NOT FX gain/loss — we compute conversion ourselves

### Key files to change
- `internal/common/config.go` — add `DisplayCurrency` config field
- `docker/vire.toml` — add `display_currency = "AUD"`
- `internal/models/navexa.go` — add `Currency` field to `NavexaHolding`
- `internal/models/portfolio.go` — add `Currency` field to `Holding`
- `internal/clients/navexa/client.go` — pass through `currencyCode`
- `internal/clients/eodhd/client.go` — no change needed (real-time endpoint already supports forex tickers)
- `internal/services/portfolio/service.go` — fetch FX rate, convert amounts, fix totals
- `cmd/vire-mcp/formatters.go` — show currency markers
- `cmd/vire-mcp/format.go` — update `formatMoney` to accept currency

## Feature 2: Country Field in Portfolio

### Requirements
- Add `Country` field to `Holding` model
- Populate from EODHD fundamentals `CountryISO` (already extracted via `domicileFromISIN`)
- Output one list of holdings ordered by symbol (not grouped by country)
- Country info enables AI chats to provide country-aware analysis

### Key files to change
- `internal/models/portfolio.go` — add `Country` field to `Holding`
- `internal/services/portfolio/service.go` — populate country from stored fundamentals
- `cmd/vire-mcp/formatters.go` — include country in output

## Scope
- **In scope:** AUD/USD conversion, display currency config, country field from fundamentals
- **Out of scope:** Other currencies, historical FX rates, FX gain/loss tracking over time

## Approach
1. Add model fields (Currency, Country to Holding/NavexaHolding)
2. Plumb Navexa currencyCode through to holdings
3. Add config for display_currency
4. Fetch EODHD forex rate during portfolio sync/review
5. Convert amounts and fix totals
6. Update formatters for currency markers and country
