# Summary: Add get_portfolio_stock, Remove Dead Report Tools

**Date:** 2026-02-14
**Status:** completed

## What Changed

| File | Change |
|------|--------|
| `cmd/vire-mcp/tools.go` | Added `createGetPortfolioStockTool()` definition and registration. Removed dead tool definitions: `createGetTickerReportTool()`, `createGenerateTickerReportTool()`, `createListTickersTool()`. |
| `cmd/vire-mcp/handlers.go` | Added `handleGetPortfolioStock()` handler. Removed dead handlers: `handleGetTickerReport()`, `handleGenerateTickerReport()`, `handleListTickers()`. |
| `cmd/vire-mcp/formatters.go` | Added `formatPortfolioStock()` formatter — renders single holding position data (units, cost, value, weight, gains, dividends, returns, trade history) as markdown. |
| `.claude/skills/vire-stock-report/SKILL.md` | Updated workflow: call `get_stock_data` for market data, optionally call `get_portfolio_stock` for position data. Removed `get_ticker_report` references. |
| `README.md` | Updated tool table: removed dead tools, added `get_portfolio_stock`. |

## Tests
- All existing tests pass (`go test ./...`)
- `go vet ./...` clean
- Docker container builds and deploys successfully
- Manual validation: `get_portfolio_stock` returns correct position data for SKS.AU

## Documentation Updated
- `.claude/skills/vire-stock-report/SKILL.md` — new workflow using `get_stock_data` + `get_portfolio_stock`
- `README.md` — updated tool list

## Review Findings
- Reviewer confirmed dead tools were already commented out in registration but definitions and handlers still existed — clean removal was appropriate
- Reviewer verified `formatPortfolioStock()` handles nil/zero values correctly for all fields
- Reviewer confirmed trade history formatting matches existing patterns

## Notes
- New tool architecture: `get_stock_data` (market intelligence) + `get_portfolio_stock` (portfolio position) — LLM combines them
- `generate_report`, `list_reports`, `get_summary` kept (used by portfolio review flow)
- Report service and report formatter kept (used by `generate_report`)
