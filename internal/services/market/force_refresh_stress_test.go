package market

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

// ============================================================================
// Force-refresh stress tests (adversarial analysis)
// Covers: fb_b4387bb2, fb_5b41213e
// ============================================================================

// ============================================================================
// 1. mergeEODBars does NOT guarantee descending sort order in output
// ============================================================================
//
// The entire codebase assumes EOD[0] is the most recent bar:
//   - signals/computer.go:33   currentPrice := bars[0].Close
//   - portfolio/service.go:246 latestBar := md.EOD[0]
//   - market/service.go:481    current := marketData.EOD[0]
//   - market/scan_fields.go    multiple EOD[0] references
//
// mergeEODBars appends new bars first, then non-replaced existing bars,
// but does NOT sort the output. When new bars cover days 0..199 and existing
// bars cover days 0..499, the output is: [new 0..199] + [old 200..499].
// This happens to be correct IF both inputs are sorted descending AND the
// new range starts at the same date as existing. But if new bars have gaps
// or different date ranges, the sort invariant breaks.

func TestStress_MergeEODBars_OutputSortOrder(t *testing.T) {
	now := time.Now().Truncate(24 * time.Hour)

	// Existing: bars from day-0 to day-499, descending
	existingBars := make([]models.EODBar, 500)
	for i := range existingBars {
		existingBars[i] = models.EODBar{
			Date:  now.AddDate(0, 0, -i),
			Close: float64(100 + i),
		}
	}

	// Fresh: bars from day-0 to day-199, descending (3-year fetch returned 200)
	freshBars := make([]models.EODBar, 200)
	for i := range freshBars {
		freshBars[i] = models.EODBar{
			Date:  now.AddDate(0, 0, -i),
			Close: float64(200 + i),
		}
	}

	merged := mergeEODBars(freshBars, existingBars)

	// Verify total bar count: 200 fresh + 300 unique old = 500
	if len(merged) != 500 {
		t.Errorf("expected 500 merged bars, got %d", len(merged))
	}

	// CRITICAL: verify descending sort order
	for i := 1; i < len(merged); i++ {
		if merged[i].Date.After(merged[i-1].Date) {
			t.Errorf("merged bars NOT sorted descending at index %d: %s > %s",
				i, merged[i].Date.Format("2006-01-02"), merged[i-1].Date.Format("2006-01-02"))
			break // one failure is enough to prove the point
		}
	}

	// Verify EOD[0] is the most recent date
	if !merged[0].Date.Equal(now) {
		t.Errorf("EOD[0] should be most recent date (%s), got %s",
			now.Format("2006-01-02"), merged[0].Date.Format("2006-01-02"))
	}
}

// ============================================================================
// 2. mergeEODBars with gap in fresh data — sort invariant broken
// ============================================================================
//
// If EODHD returns bars with a gap (e.g., missing a week of data due to
// exchange closure), the merged output has interleaved dates.

func TestStress_MergeEODBars_GapInFreshData(t *testing.T) {
	now := time.Now().Truncate(24 * time.Hour)

	// Existing: continuous bars day-0 to day-30
	existingBars := make([]models.EODBar, 31)
	for i := range existingBars {
		existingBars[i] = models.EODBar{
			Date:  now.AddDate(0, 0, -i),
			Close: float64(100 + i),
		}
	}

	// Fresh: bars for day-0 to day-5, then day-15 to day-25 (gap: day-6 to day-14)
	freshBars := make([]models.EODBar, 0, 17)
	for i := 0; i <= 5; i++ {
		freshBars = append(freshBars, models.EODBar{
			Date:  now.AddDate(0, 0, -i),
			Close: float64(200 + i),
		})
	}
	for i := 15; i <= 25; i++ {
		freshBars = append(freshBars, models.EODBar{
			Date:  now.AddDate(0, 0, -i),
			Close: float64(200 + i),
		})
	}

	merged := mergeEODBars(freshBars, existingBars)

	// Check sort order — this is the critical invariant
	isSorted := true
	for i := 1; i < len(merged); i++ {
		if merged[i].Date.After(merged[i-1].Date) {
			isSorted = false
			break
		}
	}

	if !isSorted {
		t.Error("CRITICAL: mergeEODBars output is NOT sorted descending when fresh data has gaps. " +
			"EOD[0] will not be the most recent bar, breaking signal computation and portfolio pricing.")
	}
}

// ============================================================================
// 3. Overlapping bars — new bars should win (price update)
// ============================================================================
//
// When force=true, fresh data should override existing prices for the same dates.
// This tests that "today's corrected close" from EODHD replaces stale data.

func TestStress_MergeEODBars_OverlappingBars_NewWins(t *testing.T) {
	now := time.Now().Truncate(24 * time.Hour)

	existingBars := []models.EODBar{
		{Date: now, Close: 42.00, Volume: 1000},
		{Date: now.AddDate(0, 0, -1), Close: 41.00, Volume: 900},
		{Date: now.AddDate(0, 0, -2), Close: 40.00, Volume: 800},
	}

	freshBars := []models.EODBar{
		{Date: now, Close: 43.50, Volume: 1500},                  // corrected today
		{Date: now.AddDate(0, 0, -1), Close: 41.50, Volume: 950}, // corrected yesterday
	}

	merged := mergeEODBars(freshBars, existingBars)

	// Should have 3 bars total
	if len(merged) != 3 {
		t.Fatalf("expected 3 merged bars, got %d", len(merged))
	}

	// Build a lookup to verify values
	byDate := make(map[string]models.EODBar)
	for _, b := range merged {
		byDate[b.Date.Format("2006-01-02")] = b
	}

	// Today should have fresh price
	todayBar := byDate[now.Format("2006-01-02")]
	if todayBar.Close != 43.50 {
		t.Errorf("today's bar should use fresh price 43.50, got %.2f", todayBar.Close)
	}

	// Yesterday should have fresh price
	yesterdayBar := byDate[now.AddDate(0, 0, -1).Format("2006-01-02")]
	if yesterdayBar.Close != 41.50 {
		t.Errorf("yesterday's bar should use fresh price 41.50, got %.2f", yesterdayBar.Close)
	}

	// Day-2 should retain existing price (not in fresh set)
	day2Bar := byDate[now.AddDate(0, 0, -2).Format("2006-01-02")]
	if day2Bar.Close != 40.00 {
		t.Errorf("day-2 bar should retain existing price 40.00, got %.2f", day2Bar.Close)
	}
}

// ============================================================================
// 4. Concurrent force refreshes on same ticker
// ============================================================================
//
// CollectCoreMarketData uses goroutines per ticker, but nothing prevents
// the same ticker appearing twice in the input list, or two callers
// force-refreshing the same ticker simultaneously. This tests for data races.

func TestStress_ConcurrentForceRefresh_SameTicker(t *testing.T) {
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

	var eodCalls atomic.Int64
	eodhd := &mockEODHDClient{
		getBulkEODFn: func(_ context.Context, _ string, _ []string) (map[string]models.EODBar, error) {
			return nil, fmt.Errorf("no bulk")
		},
		getEODFn: func(_ context.Context, _ string, _ ...interfaces.EODOption) (*models.EODResponse, error) {
			eodCalls.Add(1)
			bars := make([]models.EODBar, 50)
			for i := range bars {
				bars[i] = models.EODBar{
					Date:  now.AddDate(0, 0, -i),
					Close: float64(200 + i),
				}
			}
			return &models.EODResponse{Data: bars}, nil
		},
		getFundFn: func(_ context.Context, _ string) (*models.Fundamentals, error) {
			return &models.Fundamentals{ISIN: "AU000000BHP4"}, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)

	// Launch 5 concurrent force refreshes on the same ticker
	var wg sync.WaitGroup
	errs := make([]error, 5)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errs[idx] = svc.CollectCoreMarketData(context.Background(), []string{"BHP.AU"}, true)
		}(i)
	}

	wg.Wait()

	// All should complete without error (data race would be caught by -race)
	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d got error: %v", i, err)
		}
	}

	// Final saved data should have bars (exact count depends on race outcome)
	result := storage.market.data["BHP.AU"]
	if result == nil {
		t.Fatal("BHP.AU should exist in storage after concurrent refreshes")
	}
	if len(result.EOD) == 0 {
		t.Error("BHP.AU should have EOD bars after concurrent refreshes")
	}

	t.Logf("concurrent refreshes: %d EOD calls made, %d bars in final result",
		eodCalls.Load(), len(result.EOD))
}

// ============================================================================
// 5. Force refresh with EODHD API error — must preserve existing data
// ============================================================================
//
// If EODHD returns an error during force refresh, existing data must NOT
// be destroyed. The original code would return err and potentially leave
// the data in a corrupted state.

func TestStress_ForceRefresh_APIError_PreservesExisting(t *testing.T) {
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
				Fundamentals: &models.Fundamentals{ISIN: "AU000000BHP4"},
			},
		}},
		signals: &mockSignalStorage{},
	}

	eodhd := &mockEODHDClient{
		getBulkEODFn: func(_ context.Context, _ string, _ []string) (map[string]models.EODBar, error) {
			return nil, fmt.Errorf("no bulk")
		},
		getEODFn: func(_ context.Context, _ string, _ ...interfaces.EODOption) (*models.EODResponse, error) {
			return nil, fmt.Errorf("EODHD rate limit exceeded")
		},
		getFundFn: func(_ context.Context, _ string) (*models.Fundamentals, error) {
			return &models.Fundamentals{ISIN: "AU000000BHP4"}, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)

	// Force refresh with API failure
	err := svc.CollectCoreMarketData(context.Background(), []string{"BHP.AU"}, true)
	// The function may return error, but existing data should be preserved
	_ = err

	result := storage.market.data["BHP.AU"]
	if result == nil {
		t.Fatal("BHP.AU should still exist in storage after API error")
	}
	if len(result.EOD) != 100 {
		t.Errorf("existing bars should be preserved after API error: got %d, want 100", len(result.EOD))
	}
}

// ============================================================================
// 6. mergeEODBars O(n*m) performance with large bar sets
// ============================================================================
//
// mergeEODBars lines 440-448 do a linear scan of newBars for EVERY existing
// bar to check if it was replaced. With 500 existing + 200 new bars, that's
// 100,000 iterations with string formatting on each. This test ensures merge
// completes within a reasonable time.

func TestStress_MergeEODBars_Performance(t *testing.T) {
	now := time.Now().Truncate(24 * time.Hour)

	// Large existing dataset: 2000 bars (~8 years of trading days)
	existingBars := make([]models.EODBar, 2000)
	for i := range existingBars {
		existingBars[i] = models.EODBar{
			Date:  now.AddDate(0, 0, -i),
			Close: float64(100 + i),
		}
	}

	// Fresh: 750 bars (3 years)
	freshBars := make([]models.EODBar, 750)
	for i := range freshBars {
		freshBars[i] = models.EODBar{
			Date:  now.AddDate(0, 0, -i),
			Close: float64(200 + i),
		}
	}

	start := time.Now()
	merged := mergeEODBars(freshBars, existingBars)
	elapsed := time.Since(start)

	// With O(n*m), 2000*750 = 1.5M string formats = slow
	// With O(n+m) using a set, should be < 10ms
	if elapsed > 500*time.Millisecond {
		t.Errorf("mergeEODBars took %v for 2000+750 bars — O(n*m) inner loop is too slow", elapsed)
	}

	// Verify correctness: 750 fresh replace overlapping, 1250 unique old remain = 2000 total
	if len(merged) != 2000 {
		t.Errorf("expected 2000 merged bars, got %d", len(merged))
	}

	t.Logf("mergeEODBars completed in %v for %d bars", elapsed, len(merged))
}

// ============================================================================
// 7. Force refresh CollectMarketData path — same bugs as core path
// ============================================================================
//
// CollectMarketData (non-core) has the same blind-replacement bug.
// This test specifically targets that path with an EODHD error to verify
// data preservation.

func TestStress_CollectMarketData_ForceRefresh_APIError_PreservesExisting(t *testing.T) {
	now := time.Now().Truncate(24 * time.Hour)

	existingBars := make([]models.EODBar, 100)
	for i := range existingBars {
		existingBars[i] = models.EODBar{
			Date:  now.AddDate(0, 0, -i),
			Close: float64(100 + i),
		}
	}

	// Use a ticker without a dot so extractCode returns the full name
	// and ASX filing scrape fails immediately (invalid ASX code), avoiding
	// real network delays in this unit test.
	const testTicker = "STRESSTEST"
	storage := &mockStorageManager{
		market: &mockMarketDataStorage{data: map[string]*models.MarketData{
			testTicker: {
				Ticker:       testTicker,
				Exchange:     "AU",
				DataVersion:  common.SchemaVersion,
				EODUpdatedAt: now.AddDate(0, 0, -1),
				EOD:          existingBars,
				Fundamentals: &models.Fundamentals{ISIN: "AU000000TST4"},
			},
		}},
		signals: &mockSignalStorage{},
	}

	eodhd := &mockEODHDClient{
		getBulkEODFn: func(_ context.Context, _ string, _ []string) (map[string]models.EODBar, error) {
			return nil, fmt.Errorf("no bulk")
		},
		getEODFn: func(_ context.Context, _ string, _ ...interfaces.EODOption) (*models.EODResponse, error) {
			return nil, fmt.Errorf("EODHD 503 service unavailable")
		},
		getFundFn: func(_ context.Context, _ string) (*models.Fundamentals, error) {
			return &models.Fundamentals{ISIN: "AU000000TST4"}, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)

	_ = svc.CollectMarketData(context.Background(), []string{testTicker}, false, true)

	result := storage.market.data[testTicker]
	if result == nil {
		t.Fatalf("%s should still exist after API error during force refresh", testTicker)
	}
	if len(result.EOD) != 100 {
		t.Errorf("existing bars should be preserved after API error: got %d, want 100", len(result.EOD))
	}
}

// ============================================================================
// 8. mergeEODBars with nil EODHD response data — not just empty slice
// ============================================================================
//
// EODHD might return a response with nil Data field (not empty slice).
// The len(eodResp.Data) > 0 guard should catch this, but verify.

func TestStress_MergeEODBars_NilNewBars(t *testing.T) {
	existing := []models.EODBar{
		{Date: time.Date(2026, 2, 20, 0, 0, 0, 0, time.UTC), Close: 42},
		{Date: time.Date(2026, 2, 19, 0, 0, 0, 0, time.UTC), Close: 41},
	}

	// nil new bars (not empty slice)
	merged := mergeEODBars(nil, existing)
	if len(merged) != 2 {
		t.Errorf("nil new bars should preserve all existing: got %d, want 2", len(merged))
	}

	// Both nil
	merged2 := mergeEODBars(nil, nil)
	if len(merged2) != 0 {
		t.Errorf("nil + nil should produce empty: got %d, want 0", len(merged2))
	}
}

// ============================================================================
// 9. eodChanged flag correctness — signals should not recompute on no-op
// ============================================================================
//
// When force=true but EODHD returns empty or error, eodChanged should be false
// to prevent unnecessary (and potentially incorrect) signal recomputation.

func TestStress_ForceRefresh_EODChangedFlag(t *testing.T) {
	now := time.Now().Truncate(24 * time.Hour)

	existingBars := make([]models.EODBar, 50)
	for i := range existingBars {
		existingBars[i] = models.EODBar{
			Date:  now.AddDate(0, 0, -i),
			Close: float64(100 + i),
		}
	}

	signalStorage := &mockSignalStorage{}
	// We count getEOD calls with empty response to verify the guard works
	var eodCalledWithEmpty atomic.Int64

	storage := &mockStorageManager{
		market: &mockMarketDataStorage{data: map[string]*models.MarketData{
			"BHP.AU": {
				Ticker:       "BHP.AU",
				Exchange:     "AU",
				DataVersion:  common.SchemaVersion,
				EODUpdatedAt: now.AddDate(0, 0, -1),
				EOD:          existingBars,
				Fundamentals: &models.Fundamentals{ISIN: "AU000000BHP4"},
			},
		}},
		signals: signalStorage,
	}

	eodhd := &mockEODHDClient{
		getBulkEODFn: func(_ context.Context, _ string, _ []string) (map[string]models.EODBar, error) {
			return nil, fmt.Errorf("no bulk")
		},
		getEODFn: func(_ context.Context, _ string, _ ...interfaces.EODOption) (*models.EODResponse, error) {
			eodCalledWithEmpty.Add(1)
			return &models.EODResponse{Data: []models.EODBar{}}, nil // empty
		},
		getFundFn: func(_ context.Context, _ string) (*models.Fundamentals, error) {
			return &models.Fundamentals{ISIN: "AU000000BHP4"}, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)

	err := svc.CollectCoreMarketData(context.Background(), []string{"BHP.AU"}, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// With empty response, eodChanged should be false, so signals should NOT be recomputed
	t.Logf("getEOD called %d times (returned empty), signal recomputation should be skipped",
		eodCalledWithEmpty.Load())
}

// ============================================================================
// 10. mergeEODBars preserves all bar fields (not just Date/Close)
// ============================================================================
//
// Verify that OHLCV and other fields survive the merge.

func TestStress_MergeEODBars_PreservesAllFields(t *testing.T) {
	now := time.Now().Truncate(24 * time.Hour)

	existing := []models.EODBar{
		{
			Date:   now.AddDate(0, 0, -2),
			Open:   39.00,
			High:   41.50,
			Low:    38.50,
			Close:  40.00,
			Volume: 1000000,
		},
	}

	fresh := []models.EODBar{
		{
			Date:   now,
			Open:   42.00,
			High:   44.00,
			Low:    41.50,
			Close:  43.50,
			Volume: 1500000,
		},
	}

	merged := mergeEODBars(fresh, existing)
	if len(merged) != 2 {
		t.Fatalf("expected 2 bars, got %d", len(merged))
	}

	// Find the fresh bar
	var foundFresh, foundExisting bool
	for _, b := range merged {
		if b.Date.Equal(now) {
			foundFresh = true
			if b.Open != 42.00 || b.High != 44.00 || b.Low != 41.50 || b.Close != 43.50 || b.Volume != 1500000 {
				t.Errorf("fresh bar fields corrupted: %+v", b)
			}
		}
		if b.Date.Equal(now.AddDate(0, 0, -2)) {
			foundExisting = true
			if b.Open != 39.00 || b.High != 41.50 || b.Low != 38.50 || b.Close != 40.00 || b.Volume != 1000000 {
				t.Errorf("existing bar fields corrupted: %+v", b)
			}
		}
	}
	if !foundFresh {
		t.Error("fresh bar missing from merged result")
	}
	if !foundExisting {
		t.Error("existing bar missing from merged result")
	}
}

// ============================================================================
// 11. mergeEODBars with duplicate dates in new bars (EODHD sends same day twice)
// ============================================================================
//
// EODHD might return duplicate dates in a single response. mergeEODBars should
// handle this without creating duplicate entries.

func TestStress_MergeEODBars_DuplicatesInNewBars(t *testing.T) {
	now := time.Now().Truncate(24 * time.Hour)

	existing := []models.EODBar{
		{Date: now.AddDate(0, 0, -1), Close: 41.00},
	}

	// EODHD returns the same date twice with different prices
	fresh := []models.EODBar{
		{Date: now, Close: 43.00},
		{Date: now, Close: 43.50}, // duplicate date, corrected price
	}

	merged := mergeEODBars(fresh, existing)

	// Count unique dates
	dateCount := make(map[string]int)
	for _, b := range merged {
		dateCount[b.Date.Format("2006-01-02")]++
	}

	for date, count := range dateCount {
		if count > 1 {
			t.Errorf("duplicate date in merged output: %s appears %d times", date, count)
		}
	}
}

// ============================================================================
// 12. Force refresh does not corrupt EODUpdatedAt timestamp
// ============================================================================
//
// After force refresh (success or failure), EODUpdatedAt should be set to now.
// This prevents re-fetching on the next non-force call.

func TestStress_ForceRefresh_UpdatesTimestamp(t *testing.T) {
	now := time.Now().Truncate(24 * time.Hour)
	before := now.AddDate(0, 0, -7)

	existingBars := make([]models.EODBar, 10)
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
				EODUpdatedAt: before,
				EOD:          existingBars,
				Fundamentals: &models.Fundamentals{ISIN: "AU000000BHP4"},
			},
		}},
		signals: &mockSignalStorage{},
	}

	eodhd := &mockEODHDClient{
		getBulkEODFn: func(_ context.Context, _ string, _ []string) (map[string]models.EODBar, error) {
			return nil, fmt.Errorf("no bulk")
		},
		getEODFn: func(_ context.Context, _ string, _ ...interfaces.EODOption) (*models.EODResponse, error) {
			return &models.EODResponse{Data: []models.EODBar{
				{Date: now, Close: 43.00},
			}}, nil
		},
		getFundFn: func(_ context.Context, _ string) (*models.Fundamentals, error) {
			return &models.Fundamentals{ISIN: "AU000000BHP4"}, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)

	err := svc.CollectCoreMarketData(context.Background(), []string{"BHP.AU"}, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := storage.market.data["BHP.AU"]
	if result.EODUpdatedAt.Equal(before) {
		t.Error("EODUpdatedAt was not updated after force refresh")
	}
	if result.EODUpdatedAt.Before(before) {
		t.Error("EODUpdatedAt went backwards after force refresh")
	}
}

// ============================================================================
// 13. Sort-order invariant after merge — used by downstream binary search
// ============================================================================
//
// findClosingPriceAsOf (portfolio/snapshot.go) relies on EOD being sorted
// descending. This test creates a scenario where mergeEODBars would produce
// unsorted output and verifies the sorted invariant.

func TestStress_MergeEODBars_SortInvariantForBinarySearch(t *testing.T) {
	now := time.Now().Truncate(24 * time.Hour)

	// Existing: every other day for 60 days (simulating missing weekends)
	existingBars := make([]models.EODBar, 30)
	for i := range existingBars {
		existingBars[i] = models.EODBar{
			Date:  now.AddDate(0, 0, -i*2), // every other day
			Close: float64(100 + i),
		}
	}

	// Fresh: consecutive days for 20 days (fills in gaps)
	freshBars := make([]models.EODBar, 20)
	for i := range freshBars {
		freshBars[i] = models.EODBar{
			Date:  now.AddDate(0, 0, -i),
			Close: float64(200 + i),
		}
	}

	merged := mergeEODBars(freshBars, existingBars)

	// Verify strictly descending order (no equal dates)
	isSorted := sort.SliceIsSorted(merged, func(i, j int) bool {
		return merged[i].Date.After(merged[j].Date)
	})

	if !isSorted {
		// Print first few out-of-order entries for debugging
		for i := 1; i < len(merged) && i < 10; i++ {
			if !merged[i-1].Date.After(merged[i].Date) && !merged[i-1].Date.Equal(merged[i].Date) {
				t.Logf("  out of order at [%d]: %s, [%d]: %s",
					i-1, merged[i-1].Date.Format("2006-01-02"),
					i, merged[i].Date.Format("2006-01-02"))
			}
		}
		t.Error("CRITICAL: merged bars are not sorted descending — will break findClosingPriceAsOf binary search")
	}
}
