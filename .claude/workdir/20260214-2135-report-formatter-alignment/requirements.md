# Requirements: Report Formatter Alignment with 3-Layer Architecture

**Date:** 2026-02-14
**Requested:** Claude Desktop stock reports are missing extended fundamentals, Company Releases, and Company Timeline sections that `get_stock_data` shows correctly.

## Problem

Two formatters exist:
1. `cmd/vire-mcp/formatters.go` → `formatStockData()` — used by `get_stock_data`. Updated with 3-layer architecture.
2. `internal/services/report/formatter.go` → `formatStockReport()` — used by `get_ticker_report`. **NOT updated.**

The stock report skill (`get_ticker_report`) is preferred when a ticker is in a portfolio. Claude Desktop uses this path for SKS.AU, so it gets the old format.

### What's missing in `formatStockReport`:
- Extended fundamentals (Profitability, Growth & Scale, Estimates, Analyst Consensus)
- Company Releases (per-filing extraction from `hr.FilingSummaries`)
- Company Timeline (from `hr.Timeline`)
- The old `FilingsIntelligence` is nil because it's been replaced by the above

## Scope
- Update `formatStockReport()` in report formatter to render extended fundamentals, Company Releases, Company Timeline
- Verify `HoldingReview` is populated with `FilingSummaries` and `Timeline` data in the report service
- Deploy and validate with Claude Desktop (test via `get_ticker_report`)

## Out of Scope
- Changes to the MCP `formatStockData()` (already correct)
- Adding new data sources or Gemini prompts

## Approach
1. Check report service populates HoldingReview.FilingSummaries and Timeline
2. Update `formatStockReport()` fundamentals section to match `formatStockData()` (4 subsections)
3. Add Company Releases and Company Timeline rendering (reuse functions from MCP formatter or duplicate)
4. Deploy, validate via `get_ticker_report` MCP tool

## Files Expected to Change
- `internal/services/report/formatter.go` — fundamentals update, Company Releases + Timeline rendering
- Possibly `internal/services/report/service.go` — if HoldingReview population needs fixing
