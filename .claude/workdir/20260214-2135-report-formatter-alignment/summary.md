# Summary: Report Formatter Alignment with 3-Layer Architecture

**Date:** 2026-02-14
**Status:** completed

## What Changed

| File | Change |
|------|--------|
| `internal/services/report/formatter.go` | Updated `formatStockReport()` fundamentals from 6-field single table to 4 subsections (Valuation, Profitability, Growth & Scale, Estimates, Analyst Consensus). Added Company Releases and Company Timeline rendering. Added `formatCompanyReleases()` and `formatCompanyTimeline()` functions. Also updated `formatETFReport()` with Company Releases and Company Timeline. |
| `internal/clients/eodhd/client.go` | Fixed `incomeStatementEntry` — changed `float64` fields to `flexFloat64` to handle EODHD string-encoded values. Updated historical financials extraction to cast `flexFloat64` to `float64`. |
| `internal/clients/eodhd/earnings_test.go` | Added `TestGetFundamentals_ParsesHistoricalFinancials` test with string-format income statement values. |
| `internal/common/version.go` | SchemaVersion 4 → 5 (purge stale data from broken deploy with float64 types). |

## Root Cause

Two separate formatters existed:
1. `cmd/vire-mcp/formatters.go` → `formatStockData()` — used by `get_stock_data` MCP tool. Updated in Sprint 4.
2. `internal/services/report/formatter.go` → `formatStockReport()` — used by `get_ticker_report` MCP tool. **NOT updated.**

Claude Desktop's stock report skill uses `get_ticker_report` for in-portfolio tickers, so SKS.AU used the old formatter — showing only 6 basic fundamental fields, no Company Releases, no Company Timeline.

## Tests
- `TestGetFundamentals_ParsesHistoricalFinancials` — verifies string-format income statement parsing
- Full suite: all pass

## Documentation Updated
- None required (internal formatter change, no user-facing API changes)

## Review Findings
- Reviewer confirmed HoldingReview model already had FilingSummaries and Timeline fields
- Report service already populated these fields from market data
- Only the formatter needed updating — no data flow changes required
- Reviewer verified nil-safety for all new rendering paths

## Notes
- The old `FilingsIntelligence` rendering is kept for backward compatibility with older cached reports
- Both formatters (MCP and report) now produce consistent output for fundamentals and filings
- `formatCompanyReleases()` and `formatCompanyTimeline()` are duplicated between the two formatter files — a shared package could be extracted later
