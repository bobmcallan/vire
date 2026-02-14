# Summary: Freshness Architecture & Cache Invalidation

**Date:** 2026-02-14
**Status:** completed

## What Changed

| File | Change |
|------|--------|
| `internal/common/version.go` | SchemaVersion "1" → "2" — triggers PurgeDerivedData on next startup |
| `internal/common/freshness.go` | Added three-tier freshness documentation (fast/source/derived) |
| `internal/models/market.go` | Added DataVersion field to MarketData for per-ticker schema tracking |
| `internal/services/market/service.go` | Schema-aware invalidation: clears stale FilingSummaries/CompanyTimeline when DataVersion mismatches. Force flag zeros out summaries before re-analysis. DataVersion stamped on save. |
| `internal/services/market/service_test.go` | 5 new tests: schema mismatch triggers re-extraction, matching schema preserves summaries, empty DataVersion treated as mismatch, DataVersion stamped on save, force clears summaries |

## Tests
- 5 new tests added in `service_test.go`
- All 20 market service tests pass
- `go test ./...` — PASS
- `go vet ./...` — PASS

## Documentation Updated
- `internal/common/freshness.go` — Three-tier freshness model documented in comments
- No README/skill changes needed (internal mechanism, no user-facing behavior change)

## Review Findings
- **Force flag bug** (reviewer, task #2): passing empty existing to `summarizeNewFilings` was correct, but `len(newSummaries) > len(marketData.FilingSummaries)` compared against original value. Fix: zero out `marketData.FilingSummaries` before the call when force=true, and add `force ||` to the condition. Applied by team lead.
- **SchemaVersion aggressiveness** (reviewer, task #2): confirmed acceptable — warmCache self-limits after purge, Gemini calls spread across user requests not concentrated at startup
- **DataVersion granularity** (reviewer, task #2): single field at MarketData level is correct — per-component versioning would be over-engineering

## Deployment Validation
- Schema migration executed: purged 680 stale entries
- SKS.AU: 49 filings re-analyzed under schema v2
- FY2025 results and Feb 5 guidance upgrade now have structured financial extraction
- Container health: OK
- Version: confirmed SchemaVersion "2"

## Notes
- The SchemaVersion bump is a one-time migration cost. Future schema changes only need to bump the version constant.
- DataVersion on MarketData enables incremental per-ticker migration: if only extraction logic changes, stale summaries are rebuilt on next access (no global purge needed).
- The three-tier freshness model (fast/source/derived) is now documented but not enforced in code — current IsFresh() pattern with fixed TTLs is sufficient.
