# Requirements: Portfolio Compliance Engine Language Refactor

**Date:** 2026-02-13
**Requested:** Reposition Vire from "Portfolio Reviewer" to "Portfolio Compliance Engine" by refactoring all advisory language to compliance-based language.

## Scope

**In scope:**
- Rename MCP tools: `portfolio_review` -> `portfolio_compliance`, `market_snipe` -> `strategy_scanner`, `detect_signals` -> `compute_indicators`
- Rewrite all MCP tool descriptions to remove advisory language
- Replace BUY/SELL/HOLD classification with ENTRY CRITERIA MET / EXIT TRIGGER ACTIVE / COMPLIANT
- Update signal output language (bullish -> upward trend, bearish -> downward trend, etc.)
- Update MCP formatter output to use compliance framing
- Update README.md with new description and disclaimer
- Update all tests to match renamed tools and new output language

**Out of scope:**
- No changes to signal computation logic (maths stays the same)
- No changes to strategy template or storage
- No new features or tools
- No changes to API endpoints or data structures beyond classification labels
- No changes to data retrieval tools (get_portfolio, get_summary, etc.)

## Approach
Language-only refactor across three layers:
1. MCP tool registration (names + descriptions) in `cmd/vire-mcp/tools.go`
2. Output formatting in `cmd/vire-mcp/formatters.go` and signal/review models
3. README and public-facing text

## Files Expected to Change
- `cmd/vire-mcp/tools.go` — tool registration, names, descriptions
- `cmd/vire-mcp/handlers.go` — handler function names following tool renames
- `cmd/vire-mcp/formatters.go` — output formatting language
- `cmd/vire-mcp/handlers_test.go` — test updates for renamed handlers
- `internal/models/signals.go` — signal classification labels
- `internal/models/portfolio.go` — review classification labels
- `internal/services/portfolio/service.go` — review output language
- `internal/services/market/service.go` — snipe/screen output language
- `README.md` — description, disclaimer, tool table updates
- `docker/README.md` — if affected
