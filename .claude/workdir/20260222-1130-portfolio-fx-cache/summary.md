# Summary: Fix portfolio FX conversion and stale cache zeros

**Date:** 2026-02-22
**Status:** completed

## What Changed

| File | Change |
|------|--------|
| `internal/models/portfolio.go` | Added `OriginalCurrency` field to Holding, `DataVersion` field to Portfolio |
| `internal/services/portfolio/service.go` | Per-holding USD→AUD FX conversion, DataVersion save/check for cache invalidation |
| `internal/common/version.go` | Bumped SchemaVersion from "6" to "7" |
| `internal/services/portfolio/fx_test.go` | Unit tests for FX conversion logic |
| `internal/services/portfolio/fx_stress_test.go` | 18 stress tests for edge cases |
| `tests/api/portfolio_fx_test.go` | API integration tests for FX conversion and cache invalidation |
| `tests/data/portfolio_data_test.go` | Data layer integration tests for DataVersion round-trip |
| `.claude/skills/develop/SKILL.md` | Updated Storage Architecture with new fields |

## Tests
- 117 portfolio unit tests passing (including new FX and cache tests)
- 18 stress tests passing (edge cases, security, data consistency)
- 5 API integration tests passing (currency conversion, net_return non-zero, force sync, JSON field names)
- 26 data integration tests passing
- 0 test feedback rounds required (all passed first run)

## Documentation Updated
- `.claude/skills/develop/SKILL.md` — updated Holding and Portfolio model docs

## Devils-Advocate Findings
- All 18 stress tests passed, no fixes required
- FX rate=0 correctly leaves holdings in native currency
- Negative FX rates blocked by `quote.Close > 0` guard
- NZD not being converted is known scope limitation (only USD converted)
- Cache invalidation handles empty DataVersion, future versions, corrupt JSON
- No overflow or division-by-zero risks found

## Notes
- Bug 1 root cause: Per-holding values stayed in native currency (USD for CBOE). Fix: convert all USD holding monetary fields to AUD using AUDUSD FX rate after calculation.
- Bug 2 root cause: JSON field rename in commit 8054b79 (`gain_loss` → `net_return`) caused cached portfolio data to deserialize with zero values. Fix: DataVersion field on Portfolio triggers re-sync when schema changes.
- Pre-existing test failures in internal/app, internal/server, internal/storage/surrealdb are due to SurrealDB not being port-mapped to host (runs in Docker only). Not related to this change.
- Verified in production: CBOE now shows AUD values (current_price=406.70, market_value=50,430.34 at AUDUSD=0.7086), net_return fields populated (CBOE=2,827.50, ACDC=1,720.46, etc.)
