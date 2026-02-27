# Requirements: Active Glossary Endpoint

## Overview
New MCP tool `get_glossary` that returns portfolio terms, calculation definitions, and live examples using data from the selected portfolio. An "active glossary" — not static docs, but dynamically computed with real values.

## Scope

### New Endpoint
- **Tool**: `get_glossary`
- **Method**: GET
- **Path**: `/api/portfolios/{portfolio_name}/glossary`
- **Params**: `portfolio_name` (path, uses DefaultFrom: user_config.default_portfolio)

### Response Structure
```json
{
  "portfolio_name": "SMSF",
  "generated_at": "2026-02-27T10:45:00Z",
  "categories": [
    {
      "name": "Portfolio Valuation",
      "terms": [
        {
          "term": "total_value",
          "label": "Total Value",
          "definition": "Current market value of all equity holdings at today's prices.",
          "formula": "Σ(units × current_price) for each holding",
          "value": 426000.00,
          "example": "BHP: 100 × $45.50 = $4,550 + VAS: 200 × $92.30 = $18,460 + ... = $426,000"
        }
      ]
    }
  ]
}
```

### Categories and Terms

**Portfolio Valuation**:
- `total_value` — sum of holdings market value
- `total_cost` — sum of holdings cost basis
- `net_return` / `net_return_pct` — gain/loss on holdings
- `total_capital` — total_value + cash_balance + external_balance_total
- `external_balance_total` — sum of external balance accounts

**Holding Metrics**:
- `market_value` — units × current_price
- `avg_cost` — average purchase price per unit
- `weight` — holding value / portfolio total value × 100
- `net_return` — market_value - total_cost
- `net_return_pct` — (net_return / total_cost) × 100

**Capital Performance**:
- `total_deposited` — sum of all deposits + contributions
- `total_withdrawn` — net of transfer_outs minus transfer_ins (internal transfers netted)
- `net_capital_deployed` — total_deposited - total_withdrawn
- `simple_return_pct` — (current_value - net_capital) / net_capital × 100
- `annualized_return_pct` — XIRR time-weighted annualized return

**External Balance Performance**:
- `net_transferred` — total_out - total_in per category
- `gain_loss` — current_balance - net_transferred

**Technical Indicators**:
- `ema_20` / `ema_50` / `ema_200` — Exponential Moving Averages
- `rsi` — Relative Strength Index (0-100)
- `trend` — Portfolio trend direction (bullish/bearish/neutral)

**Growth Metrics**:
- `yesterday_change` / `last_week_change` — value changes over periods
- `cash_balance` — running cash from transactions
- `net_deployed` — deposits + contributions - withdrawals

## Files to Change

| File | Change |
|------|--------|
| `internal/models/glossary.go` | New file — GlossaryResponse, GlossaryCategory, GlossaryTerm structs |
| `internal/server/glossary.go` | New file — handleGlossary handler, buildGlossary logic |
| `internal/server/routes.go` | Add `case "glossary"` to routePortfolios switch |
| `internal/server/catalog.go` | Add `get_glossary` tool definition |
| `internal/server/catalog_test.go` | Update expected tool count |

## Architecture Notes
- No new service needed — the handler loads portfolio + capital performance + indicators directly
- Read-only endpoint, no state mutation
- Non-fatal enrichment: if capital performance or indicators fail, include what's available
- Build examples from top 3 holdings by weight for readability
- Glossary logic in its own file (`glossary.go`) to keep handlers.go clean
