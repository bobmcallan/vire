# Requirements: Fix portfolio auto-refresh to cover all callers

**Date:** 2026-02-13
**Requested:** Portfolio data still stale when using Claude Desktop. The earlier auto-refresh feature (commit ae22762) only worked for `get_portfolio` but not for `portfolio_review`, which is the tool Claude Desktop typically uses.

## Problem

- Auto-refresh was added in `handlePortfolioGet` (REST handler level)
- `ReviewPortfolio` service method calls `GetPortfolio` directly on the service, bypassing the handler
- When Claude asks for "portfolio numbers", it uses `portfolio_review` → `handlePortfolioReview` → `ReviewPortfolio` → `GetPortfolio` (no freshness check)
- Result: stale holdings from last night's sync, with yesterday's prices

## Scope

### In scope
- Move auto-refresh logic from handler to service-level `GetPortfolio`
- Fix existing tests that relied on handler-level auto-refresh
- Fix review tests that had zero `LastSynced` (now triggers auto-refresh)
- Add service-level tests for auto-refresh behavior

### Out of scope
- Changing freshness TTL (already correct at 30 minutes)

## Approach

Move the freshness check from `handlePortfolioGet` handler into `Service.GetPortfolio`. This way every caller — handlers, `ReviewPortfolio`, and any future code — gets fresh data automatically.
