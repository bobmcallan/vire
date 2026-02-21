# Requirements: Portfolio Response Cleanup & Report Sectioning

**Date:** 2026-02-21
**Requested:** For get portfolio, don't return technical analysis, just the position calculations. If a review or compliance report is requested, ensure that the technical analysis/markers from EODHD are sectioned.

## Scope

### In Scope
1. **Portfolio Review API Response** (`POST /api/portfolios/{name}/review`): Strip `Signals`, `Fundamentals`, `NewsIntelligence`, `FilingsIntelligence`, `FilingSummaries`, `Timeline` from each `HoldingReview` in the JSON response. Return only position calculations (`Holding`), action (`ActionRequired`, `ActionReason`), compliance (`Compliance`), and overnight movement (`OvernightMove`, `OvernightPct`).
2. **Report Formatter** (`formatStockReport`, `formatETFReport`): Group EODHD-sourced data (Fundamentals + Technical Signals) under a parent `## EODHD Market Analysis` heading so the technical analysis markers are clearly sectioned and separated from position data.

### Out of Scope
- No changes to `GET /api/portfolios/{name}` (already returns only position data)
- No changes to internal `ReviewPortfolio` logic (signals still computed internally for action determination)
- No changes to models or interfaces

## Approach

### Change 1: Slim Review Response
In `handlePortfolioReview` (`internal/server/handlers.go`), transform the `PortfolioReview` before returning it. Create a slim response struct that includes only position fields and excludes the full signals/fundamentals/intelligence data from each `HoldingReview`. The internal `ReviewPortfolio` method continues to compute everything (needed for action determination and report generation), but the API response is trimmed.

### Change 2: EODHD Section in Reports
In `formatStockReport` and `formatETFReport` (`internal/services/report/formatter.go`), wrap the Fundamentals and Technical Signals sections under a parent `## EODHD Market Analysis` heading. Sub-sections become `###` level. This makes it clear which data originates from EODHD.

## Files Expected to Change
- `internal/server/handlers.go` — slim review response
- `internal/services/report/formatter.go` — EODHD sectioning in reports
- `internal/server/handlers_test.go` or `tests/api/` — test updates if applicable
