# Requirements: Add get_portfolio_stock, Remove get_ticker_report

**Date:** 2026-02-14
**Requested:** Replace `get_ticker_report` with a focused `get_portfolio_stock` tool. The LLM combines portfolio position data with `get_stock_data` market data.

## Rationale

`get_ticker_report` duplicated market data formatting already in `get_stock_data`. The LLM can combine portfolio position + market data itself. A focused `get_portfolio_stock` tool provides just the portfolio-specific data.

## Scope

### Add: `get_portfolio_stock` MCP tool
- Parameters: `portfolio_name` (optional, defaults), `ticker` (required, e.g., "SKS" or "SKS.AU")
- Returns: single holding position data formatted as markdown:
  - Ticker, Name, Country, Currency
  - Units, Avg Cost, Current Price, Market Value, Weight
  - Cost Basis, Capital Gain ($ and %)
  - Dividend/Income Return
  - Total Return (value, %, TWRR)
  - Trade History (date, type, units, price, fees, value)
- Uses existing portfolio data (from `get_portfolio` / portfolio service)
- Fast — reads from cached portfolio, no market data collection

### Remove: dead report tools
- `get_ticker_report` — tool definition, handler, commented-out registration
- `generate_ticker_report` — tool definition, handler, commented-out registration
- `list_tickers` — tool definition, handler, commented-out registration
- Keep `generate_report`, `list_reports`, `get_summary` (used by portfolio review flow)

### Update: stock report skill
- `.claude/skills/vire-stock-report/SKILL.md` — change workflow to:
  1. Call `get_stock_data` (market data)
  2. If ticker is in portfolio, also call `get_portfolio_stock` (position data)
  3. Save combined output to file

### Update: README.md
- Remove references to removed tools
- Add `get_portfolio_stock` to tool list

## Files Expected to Change
- `cmd/vire-mcp/tools.go` — add `get_portfolio_stock` tool definition, remove dead tools
- `cmd/vire-mcp/handlers.go` — add handler, remove dead handlers
- `cmd/vire-mcp/formatters.go` — add `formatPortfolioStock()` formatter
- `.claude/skills/vire-stock-report/SKILL.md` — update workflow
- `README.md` — update tool list

## Out of Scope
- Removing `generate_report` / `get_summary` / `list_reports` (still used by portfolio review)
- Removing report service or report formatter (still used by generate_report)
- Changes to `get_stock_data`
