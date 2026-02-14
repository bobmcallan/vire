# Summary: Add daily % gain/loss to portfolio history

**Date:** 2026-02-13
**Status:** completed

## What Changed

| File | Change |
|------|--------|
| `cmd/vire-mcp/formatters.go` | Added "Day %", "Week %", "Month %" columns to markdown tables; added `period_change` and `period_change_pct` fields to JSON export |
| `cmd/vire-mcp/formatters_test.go` | 7 new tests covering daily/weekly/monthly %, first-row empty, zero-prev-value, JSON fields, JSON div-by-zero |

## Tests
- 7 new tests added
- All 48 formatter tests pass
- Full suite passes
- No regressions

## Review Findings
- Reviewer caught missing JSON export fields in first implementation pass — fixed
- Division-by-zero guard verified for `prev == 0` edge case
- JSON field naming: `period_change` / `period_change_pct` (granularity-agnostic) rather than `day_change`

## Notes
- No model changes — computation stays in MCP formatters
- The REST API returns raw `GrowthDataPoint` data; the MCP formatter adds period-over-period fields
- Example daily output: `| 2026-02-11 | $404,513 | +$10,488 | +2.66% | 13 | +$34,455 | +9.31% |`
- Claude Desktop sees this via the `get_portfolio_history` MCP tool
