# Summary: Fix report pipeline collecting filings synchronously

**Date:** 2026-02-21
**Status:** completed

## Investigation

Container logs (68c1fd935...) confirmed `GenerateReport` (portfolio summary) was working correctly — it uses `CollectCoreMarketData` (EOD + fundamentals only). The filings visible in logs were from the background job manager, triggered by the watcher scanning newly-upserted stock index entries ~1 minute after `SyncPortfolio`.

However, `GenerateTickerReport` (single-ticker refresh) was calling `CollectMarketData` (the full path with filings, news, AI summaries) instead of `CollectCoreMarketData`.

## What Changed

| File | Change |
|------|--------|
| `internal/services/report/service.go:117` | Changed `CollectMarketData` to `CollectCoreMarketData` in `GenerateTickerReport` |
| `docs/portfolio-data-flow.md` | Updated to reflect `GenerateTickerReport` now uses core path |
| `.claude/skills/develop/SKILL.md` | Updated Report Generation Pipeline section |

## Tests
- All existing tests pass (`go test ./internal/services/report/...` — ok)
- `go vet ./...` clean
- Binary compiles successfully

## Documentation Updated
- `docs/portfolio-data-flow.md` — corrected `GenerateTickerReport` description
- `.claude/skills/develop/SKILL.md` — updated Report Generation Pipeline section

## Devils-Advocate Findings
- No edge cases found — the fix is a single method call change with matching signatures
- Report formatters do not depend on freshly-collected filing data (they use whatever is cached)
- Signal detection (`DetectSignals`) remains correct as a separate call

## Notes
- Local deployment validation skipped — SurrealDB not available locally (pre-existing infrastructure constraint)
- The background job manager filing collection is working as designed — it runs independently from the report pipeline
