# Requirements: Add daily % gain/loss to portfolio history

**Date:** 2026-02-13
**Requested:** The daily portfolio performance view exists (`get_portfolio_history` MCP tool) but only shows dollar day-change, not percentage. Add day-over-day % gain/loss for each day.

## Problem

- `formatPortfolioHistory` (formatters.go:463-476) calculates day-over-day dollar change: `dc := p.TotalValue - points[i-1].TotalValue`
- But does NOT calculate the percentage: `dcPct := (dc / points[i-1].TotalValue) * 100`
- The `GainLossPct` field in `GrowthDataPoint` is total gain from cost basis, not daily return
- The JSON export (`formatHistoryJSON`) also lacks day-over-day change fields
- Claude Desktop using `get_portfolio_history` or `portfolio_review` cannot see daily % performance

## Scope

### In scope
- Add "Day %" column to daily/weekly/monthly formatters in `formatPortfolioHistory`
- Add `day_change` and `day_change_pct` to `formatHistoryJSON` JSON export
- Ensure the MCP `get_portfolio_history` tool description mentions daily % gain/loss

### Out of scope
- Changing the `GrowthDataPoint` model (keep computation in formatters)
- Modifying `portfolio_review` tool (already has DayChangePct for today)
- Adding new MCP tools

## Approach

Compute day-over-day % change inline in the formatters (same as dollar change is computed now). Add the percentage column to the markdown tables and the JSON export.

## Files Expected to Change
- `cmd/vire-mcp/formatters.go` — add % column to daily/weekly/monthly tables, add fields to JSON
- `cmd/vire-mcp/tools.go` — update tool description to mention daily % (if needed)
