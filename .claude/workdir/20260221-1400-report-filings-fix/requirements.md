# Requirements: Fix report pipeline collecting filings synchronously

**Date:** 2026-02-21
**Requested:** Portfolio summary appears to collect filings as part of the report generation

## Investigation Findings

Container logs (68c1fd935...) confirm:
- `GenerateReport` itself is **correct** — calls `CollectCoreMarketData` (EOD + fundamentals only)
- The filings visible in logs during report generation are from the **job manager background workers**, triggered by the watcher scanning newly-upserted stock index entries
- Timeline: report starts 02:56:43, watcher enqueues 320 jobs at 02:57:42, report completes at 02:59:14

## Actual Bug

`GenerateTickerReport` (report/service.go:117) calls `CollectMarketData` instead of `CollectCoreMarketData`. This means single-ticker report refreshes DO collect filings, news, filing summaries, and timelines synchronously — violating the design.

## Scope
- **In scope:** Fix `GenerateTickerReport` to use `CollectCoreMarketData`
- **In scope:** Update tests to verify the correct method is called
- **Out of scope:** Watcher timing/contention (working as designed — filings are background only)

## Approach

1. Change `report/service.go:117` from `s.market.CollectMarketData(...)` to `s.market.CollectCoreMarketData(...)`
2. Update existing tests to verify the fast path is used
3. Add a test confirming `GenerateTickerReport` uses the core path

## Files Expected to Change
- `internal/services/report/service.go` — fix `GenerateTickerReport`
- `internal/services/report/service_test.go` (if exists) or new test file
