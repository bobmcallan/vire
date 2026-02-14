# Requirements: Freshness Architecture & Cache Invalidation

**Date:** 2026-02-14
**Requested:** Fix data quality gaps exposed by SKS comparison assessment. The 3-layer architecture is structurally sound, but stale cached data from pre-refactor survives deployments and produces incomplete results. Users and LLMs cannot resolve this — it must be handled by the system.

**Context:**
- `docs/stock-assessment-comparison-v2.md` — assessment showing recent filings (Nov 2025 - Feb 2026) have no extracted financial detail because they were cached pre-refactor
- Previous sprint requirements (`.claude/workdir/20260214-1500-stock-assessment-refactor/requirements.md`) specified dynamic freshness but didn't implement schema-version-triggered invalidation for the new data fields

## Problem Statement

1. **Stale filing summaries**: Filings analyzed before the 3-layer refactor have no structured extraction (empty revenue/profit/guidance fields). The incremental model treats them as "already analyzed" because the date+headline key matches — so the new extraction code never runs on them.

2. **No schema-aware invalidation**: `SchemaVersion` is still `"1"` — it was never bumped for the 3-layer refactor. The existing `checkSchemaVersion` → `PurgeDerivedData` mechanism would solve this, but the version wasn't updated.

3. **Freshness categories not differentiated**: The user's original requirements specify three data tiers:
   - **Fast data (0 caching)**: real-time quotes, live prices — always fresh
   - **Reliable data (0 caching)**: data that must be accurate when shown — fundamentals, filings list
   - **Slow data (cached, import latest)**: derived intelligence — filing summaries, timelines, news intel

   Currently, all data uses the same IsFresh() pattern with fixed TTLs. There's no distinction between "source data that should be re-fetched when accessed" and "derived data that should be rebuilt when inputs change."

## Scope

### In Scope

1. **Bump SchemaVersion to "2"** — triggers `PurgeDerivedData()` on next startup, clearing all stale pre-refactor market data. This is the "blunt instrument" the user wants — clean and works.

2. **Add targeted field-level invalidation for filing summaries** — when `SchemaVersion` changes, clear `FilingSummaries` and `CompanyTimeline` specifically (not just whole-file purge). This means after schema bump, the next `CollectMarketData` call will re-analyze all filings through the new extraction code.

   Implementation: In `CollectMarketData`, check if existing `FilingSummaries` were produced under the current schema. If not, discard them and re-run extraction.

3. **Tag derived data with schema version** — add a `SchemaVersion` field to `MarketData` so the system can detect when cached data was produced under an older schema without requiring a full global purge every time.

4. **Ensure `force=true` clears filing summaries** — currently `force` re-fetches filings but doesn't clear existing summaries, so the incremental model skips re-analysis. When force is true, summaries should be rebuilt from scratch.

### Out of Scope
- Changing freshness TTL values (current values are reasonable)
- Adding new data sources
- Forward P/E or analyst estimates (separate feature)
- Real-time streaming / websocket architecture

## Approach

### Change 1: Bump SchemaVersion

**File:** `internal/common/version.go`
- Change `SchemaVersion = "1"` → `SchemaVersion = "2"`
- On next deploy/restart, `checkSchemaVersion()` triggers `PurgeDerivedData()` which clears all market data and signals
- All tickers will be re-collected from scratch on next access, using the new extraction code

### Change 2: Schema-tagged MarketData

**File:** `internal/models/market.go`
- Add `DataVersion string` field to MarketData

**File:** `internal/services/market/service.go`
- In `CollectMarketData`, after loading existing data, check `marketData.DataVersion != common.SchemaVersion`
- If mismatch: clear `FilingSummaries`, `CompanyTimeline`, and their timestamps — forces re-extraction
- Set `marketData.DataVersion = common.SchemaVersion` before save
- This provides incremental migration: if only filing extraction logic changes in future, we bump SchemaVersion and stale summaries are rebuilt per-ticker on next access (no need for global purge)

### Change 3: Force flag clears summaries

**File:** `internal/services/market/service.go`
- In the Filing Summaries block, when `force` is true, pass empty `[]FilingSummary{}` as `existingSummaries` to `summarizeNewFilings` instead of `marketData.FilingSummaries`
- This ensures `force=true` produces a complete re-analysis

### Change 4: Clarify freshness model in code comments

No functional change, but document the three tiers in `freshness.go`:
- Fast (real-time): quotes — short TTL, always re-fetch on access
- Source data: EOD, fundamentals, news, filings list — time-based TTL, re-fetch when stale
- Derived data: filing summaries, timelines, news intel, signals — rebuild when inputs change or schema version bumps

## Files Expected to Change

- `internal/common/version.go` — SchemaVersion "1" → "2"
- `internal/common/freshness.go` — Add tier documentation comments
- `internal/models/market.go` — Add DataVersion field to MarketData
- `internal/services/market/service.go` — Schema-aware invalidation in CollectMarketData, force flag clears summaries
- `internal/services/market/service_test.go` — Test schema mismatch triggers re-extraction, test force clears summaries

## Expected Outcome

After deploy with SchemaVersion "2":
1. All cached market data is purged on startup
2. First `get_stock_data SKS.AU` re-collects everything from scratch
3. All 49+ filings re-analyzed through new extraction code with PDF text
4. Recent filings (Feb 5 guidance upgrade, Delta Elcom acquisition) now get structured extraction
5. Future schema changes automatically invalidate stale derived data per-ticker
