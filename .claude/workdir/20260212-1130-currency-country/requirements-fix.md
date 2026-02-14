# Requirements Fix: Convert per-holding display values to AUD

**Date:** 2026-02-13
**Requested:** All holding values in the portfolio table should display in AUD (display currency), not native currency. Add FX rate note.

## Problem
CBOE shows as $33,924 (USD) in the portfolio table. It should be converted to ~A$47,566 (at AUDUSD 0.7133). The total ($404,988) is also wrong because it mixed unconverted USD values.

## What needs to change
- Formatters (`formatPortfolioHoldings`, `formatPortfolioReview`, `formatSyncResult`) must convert USD values to AUD at display time using `Portfolio.FXRate` / `PortfolioReview.FXRate`
- All monetary values shown in AUD (A$ prefix)
- Add note: "FX Rate: AUDUSD 0.7133 — USD holdings converted to AUD"
- Remove `Ccy` column from tables (everything is AUD now)
- Keep `Country` column

## Scope
- Formatter-layer only — native values stay on the model
- Only `formatPortfolioHoldings`, `formatPortfolioReview`, `formatSyncResult` need changes
- Update existing formatter tests
