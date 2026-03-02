# Requirements: Timeline Corruption Fix (fb_b4387bb2, fb_5b41213e)

## Feedback Items

| ID | Summary | Severity |
|----|---------|----------|
| fb_b4387bb2 | `get_stock_data force_refresh` truncates EOD history | HIGH |
| fb_5b41213e | Portfolio timeline becomes corrupted after force refresh | HIGH |

## Root Cause

When `force=true`, both `collectCoreTicker()` and `CollectMarketData()` do a full 3-year
fetch and REPLACE all existing EOD bars:

```go
// collectCoreTicker() line 356-364
eodResp, err := s.eodhd.GetEOD(ctx, ticker, interfaces.WithDateRange(now.AddDate(-3, 0, 0), now))
marketData.EOD = eodResp.Data  // REPLACES entirely — existing history lost
```

**Two failure modes:**
1. If existing history goes back more than 3 years, older bars are lost
2. If EODHD returns partial/truncated data (API error, rate limit, empty response),
   good data is overwritten with bad data

**Downstream impact:** `GetDailyGrowth()` in `growth.go:206` filters holdings without
EOD data. If a holding's EOD is truncated, the timeline shrinks to only cover dates
where that holding has bars. The portfolio timeline chart becomes corrupted.

## What's IN Scope

1. **Fix `collectCoreTicker()`** — merge instead of replace when force=true and existing data exists
2. **Fix `CollectMarketData()`** — same merge-instead-of-replace fix
3. **Add sanity check** — reject EODHD responses that return empty or near-empty data
   when we know the ticker has existing history

## What's OUT of Scope

- Timeline handler changes (no `force_refresh` parameter needed — fix is at data layer)
- Rebuilding already-corrupted data (existing data is only 3 years max anyway)
- Changes to `GetDailyGrowth()` logic

---

## Fix 1: `collectCoreTicker()` — merge on force refresh

**File:** `internal/services/market/service.go`
**Location:** Lines 356-365 (the `else` block in the EOD section)

### Current code (lines 341-365):
```go
} else if s.eodhd != nil {
    // Full or incremental fetch
    if !force && existing != nil && len(existing.EOD) > 0 {
        // ... incremental fetch + mergeEODBars ...
    } else {
        eodResp, err := s.eodhd.GetEOD(ctx, ticker, interfaces.WithDateRange(now.AddDate(-3, 0, 0), now))
        if err != nil {
            s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to fetch EOD data (core)")
            return err
        }
        marketData.EOD = eodResp.Data     // <-- BUG: blind replacement
        marketData.EODUpdatedAt = now
        eodChanged = true
    }
}
```

### Fixed code — split into three paths:

Replace lines 341-366 with:
```go
} else if s.eodhd != nil {
    if !force && existing != nil && len(existing.EOD) > 0 {
        // Incremental fetch: only bars after the latest stored date
        latestDate := existing.EOD[0].Date
        fromDate := latestDate.AddDate(0, 0, 1)
        if fromDate.Before(now) {
            eodResp, err := s.eodhd.GetEOD(ctx, ticker, interfaces.WithDateRange(fromDate, now))
            if err != nil {
                s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to fetch incremental EOD data (core)")
            } else if len(eodResp.Data) > 0 {
                marketData.EOD = mergeEODBars(eodResp.Data, existing.EOD)
                eodChanged = true
            }
        }
        marketData.EODUpdatedAt = now
    } else if force && existing != nil && len(existing.EOD) > 0 {
        // Force refresh with existing data: full fetch + merge to preserve history
        eodResp, err := s.eodhd.GetEOD(ctx, ticker, interfaces.WithDateRange(now.AddDate(-3, 0, 0), now))
        if err != nil {
            s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to fetch EOD data (core, force)")
        } else if len(eodResp.Data) > 0 {
            marketData.EOD = mergeEODBars(eodResp.Data, existing.EOD)
            eodChanged = true
        }
        marketData.EODUpdatedAt = now
    } else {
        // No existing data: full fetch (new ticker or first collection)
        eodResp, err := s.eodhd.GetEOD(ctx, ticker, interfaces.WithDateRange(now.AddDate(-3, 0, 0), now))
        if err != nil {
            s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to fetch EOD data (core)")
            return err
        }
        marketData.EOD = eodResp.Data
        marketData.EODUpdatedAt = now
        eodChanged = true
    }
}
```

**Key change:** When `force=true && existing != nil && len(existing.EOD) > 0`:
- Still does a full 3-year fetch (force means re-fetch, not incremental)
- Uses `mergeEODBars()` instead of blind replacement — preserves existing history
- If EODHD returns empty/error, existing data is preserved (the `else if len(eodResp.Data) > 0` guard)

---

## Fix 2: `CollectMarketData()` — same merge fix

**File:** `internal/services/market/service.go`
**Location:** Lines 96-124 (the EOD section in the non-core path)

### Current code (lines 96-124):
```go
if !force && existing != nil && len(existing.EOD) > 0 {
    // ... incremental fetch + mergeEODBars ...
} else {
    // Full fetch
    eodResp, err = s.eodhd.GetEOD(ctx, ticker, interfaces.WithDateRange(now.AddDate(-3, 0, 0), now))
    if err != nil {
        s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to fetch EOD data")
        continue
    }
    marketData.EOD = eodResp.Data    // <-- BUG: blind replacement
    marketData.EODUpdatedAt = now
    eodChanged = true
}
```

### Fixed code — same three-path pattern:

Replace lines 96-124 with:
```go
if !force && existing != nil && len(existing.EOD) > 0 {
    // Incremental fetch: only bars after the latest stored date
    latestDate := existing.EOD[0].Date
    fromDate := latestDate.AddDate(0, 0, 1) // day after last bar
    if fromDate.Before(now) {
        s.logger.Debug().Str("ticker", ticker).Str("from", fromDate.Format(time.RFC3339)).Msg("Incremental EOD fetch")
        eodResp, err = s.eodhd.GetEOD(ctx, ticker, interfaces.WithDateRange(fromDate, now))
        if err != nil {
            s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to fetch incremental EOD data")
        } else if len(eodResp.Data) > 0 {
            marketData.EOD = mergeEODBars(eodResp.Data, existing.EOD)
            eodChanged = true
        }
    }
    marketData.EODUpdatedAt = now
} else if force && existing != nil && len(existing.EOD) > 0 {
    // Force refresh with existing data: full fetch + merge to preserve history
    eodResp, err = s.eodhd.GetEOD(ctx, ticker, interfaces.WithDateRange(now.AddDate(-3, 0, 0), now))
    if err != nil {
        s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to fetch EOD data (force)")
    } else if len(eodResp.Data) > 0 {
        marketData.EOD = mergeEODBars(eodResp.Data, existing.EOD)
        eodChanged = true
    }
    marketData.EODUpdatedAt = now
} else {
    // No existing data: full fetch (new ticker or first collection)
    eodResp, err = s.eodhd.GetEOD(ctx, ticker, interfaces.WithDateRange(now.AddDate(-3, 0, 0), now))
    if err != nil {
        s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to fetch EOD data")
        continue
    }
    marketData.EOD = eodResp.Data
    marketData.EODUpdatedAt = now
    eodChanged = true
}
```

---

## Tests

### Unit tests in `internal/services/market/service_test.go`:

**Test 1: `TestCollectCoreTicker_ForceRefreshMergesExistingEOD`**
- Setup: existing MarketData with 500 EOD bars (simulating >3 years of data)
- Mock EODHD returns 200 bars (3 years)
- Call `collectCoreTicker()` with `force=true`
- Assert: result EOD has merged bars (more than 200, preserving older history)
- Assert: `mergeEODBars` was used (new bars replace overlapping dates, old bars preserved)

**Test 2: `TestCollectCoreTicker_ForceRefreshEmptyResponsePreservesExisting`**
- Setup: existing MarketData with 100 EOD bars
- Mock EODHD returns empty response (0 bars)
- Call `collectCoreTicker()` with `force=true`
- Assert: existing 100 bars are preserved (not overwritten with empty)
- Assert: `eodChanged` is false (no signal recomputation triggered)

**Test 3: `TestCollectMarketData_ForceRefreshMergesExistingEOD`**
- Same as Test 1 but via `CollectMarketData()` instead of `collectCoreTicker()`
- Verifies the non-core path has the same merge behavior

**Test 4: `TestCollectMarketData_ForceRefreshEmptyResponsePreservesExisting`**
- Same as Test 2 but via `CollectMarketData()` path

### Implementation detail for tests:

The `collectCoreTicker` method is private, so tests must go through `CollectCoreMarketData()`.
Follow the existing test pattern in `core_stress_test.go:343` (`TestStress_CollectCoreMarketData_Force_BypassesFreshness`):
- Use `mockStorageManager` with pre-populated `mockMarketDataStorage`
- Use `mockEODHDClient` with custom `getEODFn`
- After the call, inspect `storage.market.data["TICKER"]` to verify merged bars

```go
func TestCollectCoreMarketData_ForceRefreshMergesExistingEOD(t *testing.T) {
    now := time.Now().Truncate(24 * time.Hour)

    // Create existing data: 500 bars going back ~2 years
    existingBars := make([]models.EODBar, 500)
    for i := range existingBars {
        existingBars[i] = models.EODBar{
            Date:  now.AddDate(0, 0, -i),
            Close: float64(100 + i),
        }
    }

    storage := &mockStorageManager{
        market: &mockMarketDataStorage{data: map[string]*models.MarketData{
            "BHP.AU": {
                Ticker:       "BHP.AU",
                Exchange:     "AU",
                DataVersion:  common.SchemaVersion,
                EODUpdatedAt: now.AddDate(0, 0, -1), // stale
                EOD:          existingBars,
            },
        }},
        signals: &mockSignalStorage{},
    }

    // EODHD returns 200 fresh bars (3-year window, but only 200 bars)
    freshBars := make([]models.EODBar, 200)
    for i := range freshBars {
        freshBars[i] = models.EODBar{
            Date:  now.AddDate(0, 0, -i),
            Close: float64(200 + i), // different prices to distinguish
        }
    }

    eodhd := &mockEODHDClient{
        getBulkEODFn: func(_ context.Context, _ string, _ []string) (map[string]models.EODBar, error) {
            return nil, fmt.Errorf("no bulk")
        },
        getEODFn: func(_ context.Context, _ string, _ ...interfaces.EODOption) (*models.EODResponse, error) {
            return &models.EODResponse{Data: freshBars}, nil
        },
        getFundFn: func(_ context.Context, _ string) (*models.Fundamentals, error) {
            return &models.Fundamentals{ISIN: "AU000000BHP4"}, nil
        },
    }

    logger := common.NewLogger("error")
    svc := NewService(storage, eodhd, nil, logger)

    err := svc.CollectCoreMarketData(context.Background(), []string{"BHP.AU"}, true)
    require.NoError(t, err)

    result := storage.market.data["BHP.AU"]
    // Merged: fresh bars (200) + unique old bars beyond fresh range (~300)
    // Total should be > 200 (preserving older history)
    assert.Greater(t, len(result.EOD), 200,
        "force refresh should merge with existing bars, not replace (got %d)", len(result.EOD))
}
```

```go
func TestCollectCoreMarketData_ForceRefreshEmptyResponsePreservesExisting(t *testing.T) {
    now := time.Now().Truncate(24 * time.Hour)

    existingBars := make([]models.EODBar, 100)
    for i := range existingBars {
        existingBars[i] = models.EODBar{
            Date:  now.AddDate(0, 0, -i),
            Close: float64(100 + i),
        }
    }

    storage := &mockStorageManager{
        market: &mockMarketDataStorage{data: map[string]*models.MarketData{
            "BHP.AU": {
                Ticker:       "BHP.AU",
                Exchange:     "AU",
                DataVersion:  common.SchemaVersion,
                EODUpdatedAt: now.AddDate(0, 0, -1),
                EOD:          existingBars,
            },
        }},
        signals: &mockSignalStorage{},
    }

    eodhd := &mockEODHDClient{
        getBulkEODFn: func(_ context.Context, _ string, _ []string) (map[string]models.EODBar, error) {
            return nil, fmt.Errorf("no bulk")
        },
        getEODFn: func(_ context.Context, _ string, _ ...interfaces.EODOption) (*models.EODResponse, error) {
            return &models.EODResponse{Data: []models.EODBar{}}, nil // empty response
        },
        getFundFn: func(_ context.Context, _ string) (*models.Fundamentals, error) {
            return &models.Fundamentals{ISIN: "AU000000BHP4"}, nil
        },
    }

    logger := common.NewLogger("error")
    svc := NewService(storage, eodhd, nil, logger)

    err := svc.CollectCoreMarketData(context.Background(), []string{"BHP.AU"}, true)
    require.NoError(t, err)

    result := storage.market.data["BHP.AU"]
    assert.Equal(t, 100, len(result.EOD),
        "empty EODHD response should preserve existing bars (got %d)", len(result.EOD))
}
```

---

## Integration Tests

### `tests/api/timeline_corruption_test.go`:

**Test 1: `TestForceRefresh_PreservesTimelineIntegrity`**
- Get portfolio timeline data points (count)
- Force refresh stock data for a holding
- Get portfolio timeline data points again
- Assert: timeline point count has not decreased

**Test 2: `TestForceRefresh_EODBarCount`**
- Get stock data with price (note candle count)
- Force refresh same stock
- Get stock data again
- Assert: candle count has not decreased significantly

---

## Verification Checklist

- [ ] `go build ./cmd/vire-server/`
- [ ] `go test ./internal/services/market/... -timeout 120s`
- [ ] `go vet ./...`
- [ ] `golangci-lint run`
- [ ] Integration tests pass against live server
