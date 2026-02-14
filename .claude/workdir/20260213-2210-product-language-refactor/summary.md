# Summary: Portfolio Compliance Engine Language Refactor

**Date:** 2026-02-13
**Status:** completed

## What Changed

| File | Change |
|------|--------|
| `cmd/vire-mcp/tools.go` | Renamed MCP tools: `portfolio_review` → `portfolio_compliance`, `market_snipe` → `strategy_scanner`, `detect_signals` → `compute_indicators`. Rewrote all tool descriptions to remove advisory language. |
| `cmd/vire-mcp/handlers.go` | Renamed handler functions to match new tool names. Updated error messages and AI prompt language. |
| `cmd/vire-mcp/formatters.go` | Added `formatTrend()` and `formatTrendDescription()` display-layer helpers: `bullish` → `upward trend`, `bearish` → `downward trend`, MACD `bullish`/`bearish` → `positive`/`negative`. Added `formatAction()` to translate BUY/SELL/HOLD → compliance labels. Changed "Alerts & Recommendations" → "Alerts & Observations". Applied translation to stock data, signal detection, screen, and funnel formatters. |
| `cmd/vire-mcp/handlers_test.go` | Updated test references to renamed handlers and tools. |
| `internal/services/portfolio/service.go` | Changed `determineAction()` return values: BUY → ENTRY CRITERIA MET, SELL → EXIT TRIGGER, HOLD → COMPLIANT, STRONG BUY → MULTIPLE ENTRY CRITERIA MET, STRONG SELL → MULTIPLE EXIT TRIGGERS ACTIVE. Updated comments to use "observations" instead of "recommendations". |
| `internal/services/portfolio/service_test.go` | Updated test assertions to use new compliance classification labels. |
| `internal/services/market/screen.go` | Changed "Attractive P/E" → "Low P/E". Changed AI prompt from "turnaround play" to "Focus on factual data points". |
| `internal/services/report/formatter.go` | Changed "Alerts & Recommendations" → "Alerts & Observations". |
| `internal/common/logging_test.go` | Updated test fixtures from `portfolio_review` → `portfolio_compliance`. |
| `internal/common/logging_stress_test.go` | Updated test fixture from `portfolio_review` → `portfolio_compliance`. |
| `README.md` | Updated tool tables with new names/descriptions, compliance language throughout. |

## Design Decision: Two-Layer Translation

Internal data values remain **unchanged**:
- `TrendBullish = "bullish"`, `TrendBearish = "bearish"` (in `models/signals.go`)
- MACD crossover `"bullish"`, `"bearish"` (in `signals/computer.go`)
- All regime descriptions, scoring strings, and news sentiment labels in services

User strategy rules depend on these string values for matching (e.g. `signals.trend == "bearish"`). Changing them would silently break existing user-authored strategy JSON.

Advisory language is translated **only at the MCP formatting boundary** in `cmd/vire-mcp/formatters.go` using:
- `formatTrend()` — trend label translation
- `formatTrendDescription()` — `strings.NewReplacer` for descriptions (bullish→upward, bearish→downward, etc.)
- `formatAction()` — classification label translation (BUY→ENTRY CRITERIA MET, etc.)
- Inline MACD crossover translation in stock data formatter

## Tests
- Build: `go build ./...` — clean
- Vet: `go vet ./...` — clean
- Tests: `go test -count=1 ./...` — all 17 packages pass, 0 failures
- Tests updated to reflect new classification labels and tool names

## Documentation Updated
- `README.md` — tool table, descriptions, compliance language

## Review Findings
- Reviewer caught implementer changing TrendType constants (violating two-layer approach). Required two correction cycles before proper revert.
- Reviewer identified partial revert leaving stale `TrendDownward`/`TrendUpward` references causing compile errors. Fixed by implementer.
- Reviewer found ~10 missed advisory language items (screen.go, report/formatter.go, logging tests). All fixed.
- Internal function name `generateRecommendations` and struct field `Recommendations` left unchanged — renaming internal Go identifiers is out of scope per requirements.

## Deployed
- Version: 0.3.13
- Both `vire-server` and `vire-mcp` containers healthy
