package market

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

// ============================================================================
// 1. CollectCoreMarketData swallows errors — returns nil always
// ============================================================================
//
// CollectCoreMarketData (service.go:280) returns nil even when errors occurred
// in individual ticker processing. The caller has no way to know if collection
// partially failed. Only a log warning is emitted.
//
// FINDING: The function should return an error (or the errors slice) when
// partial failures occur, not silently return nil.

func TestStress_CollectCoreMarketData_SwallowsErrors(t *testing.T) {
	storage := &mockStorageManager{
		market:  &mockMarketDataStorage{data: map[string]*models.MarketData{}},
		signals: &mockSignalStorage{},
	}

	eodhd := &mockEODHDClient{
		getBulkEODFn: func(_ context.Context, _ string, _ []string) (map[string]models.EODBar, error) {
			return nil, fmt.Errorf("exchange down")
		},
		getEODFn: func(_ context.Context, _ string, _ ...interfaces.EODOption) (*models.EODResponse, error) {
			return nil, fmt.Errorf("API timeout")
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)

	// All tickers fail, but error is swallowed
	err := svc.CollectCoreMarketData(context.Background(), []string{"FAIL1.AU", "FAIL2.AU"}, false)
	if err != nil {
		t.Logf("Good: error was returned: %v", err)
	} else {
		t.Log("NOTE: CollectCoreMarketData returned nil despite all tickers failing. " +
			"Caller has no way to detect partial failure.")
	}
}

// ============================================================================
// 2. Context cancellation during concurrent ticker processing
// ============================================================================
//
// CollectCoreMarketData spawns goroutines per ticker. If context is cancelled,
// the semaphore acquire (sem <- struct{}{}) blocks without checking ctx.Done().
// This can cause goroutine hangs when shutting down.

func TestStress_CollectCoreMarketData_ContextCancel(t *testing.T) {
	storage := &mockStorageManager{
		market:  &mockMarketDataStorage{data: map[string]*models.MarketData{}},
		signals: &mockSignalStorage{},
	}

	var eodCalls atomic.Int64
	eodhd := &mockEODHDClient{
		getBulkEODFn: func(_ context.Context, _ string, _ []string) (map[string]models.EODBar, error) {
			return nil, fmt.Errorf("no bulk")
		},
		getEODFn: func(ctx context.Context, _ string, _ ...interfaces.EODOption) (*models.EODResponse, error) {
			eodCalls.Add(1)
			// Simulate slow API
			select {
			case <-time.After(5 * time.Second):
				return &models.EODResponse{Data: []models.EODBar{{Date: time.Now(), Close: 10}}}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
		getFundFn: func(_ context.Context, _ string) (*models.Fundamentals, error) {
			return &models.Fundamentals{}, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)

	// Create many tickers to overwhelm the semaphore (maxConcurrent=5 hardcoded)
	tickers := make([]string, 20)
	for i := range tickers {
		tickers[i] = fmt.Sprintf("T%d.AU", i)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		svc.CollectCoreMarketData(ctx, tickers, false)
		close(done)
	}()

	select {
	case <-done:
		// Good — returned after context cancel
	case <-time.After(3 * time.Second):
		t.Error("DEADLOCK: CollectCoreMarketData did not return after context cancellation — " +
			"semaphore acquire blocks without select on ctx.Done()")
	}
}

// ============================================================================
// 3. Ticker with no exchange suffix defaults to AU
// ============================================================================

func TestStress_CollectCoreMarketData_TickerWithoutExchange(t *testing.T) {
	var exchangeSeen string
	storage := &mockStorageManager{
		market:  &mockMarketDataStorage{data: map[string]*models.MarketData{}},
		signals: &mockSignalStorage{},
	}

	eodhd := &mockEODHDClient{
		getBulkEODFn: func(_ context.Context, exchange string, _ []string) (map[string]models.EODBar, error) {
			exchangeSeen = exchange
			return nil, fmt.Errorf("test")
		},
		getEODFn: func(_ context.Context, _ string, _ ...interfaces.EODOption) (*models.EODResponse, error) {
			return &models.EODResponse{Data: []models.EODBar{{Date: time.Now(), Close: 10}}}, nil
		},
		getFundFn: func(_ context.Context, _ string) (*models.Fundamentals, error) {
			return &models.Fundamentals{}, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)

	// Ticker without exchange suffix: "NOTICKER" (no dot)
	svc.CollectCoreMarketData(context.Background(), []string{"NOTICKER"}, false)

	if exchangeSeen != "AU" {
		t.Errorf("expected default exchange AU for bare ticker, got %q", exchangeSeen)
	}
}

// ============================================================================
// 4. Multi-exchange grouping
// ============================================================================

func TestStress_CollectCoreMarketData_MultiExchangeGrouping(t *testing.T) {
	var exchangesCalled []string
	storage := &mockStorageManager{
		market:  &mockMarketDataStorage{data: map[string]*models.MarketData{}},
		signals: &mockSignalStorage{},
	}

	eodhd := &mockEODHDClient{
		getBulkEODFn: func(_ context.Context, exchange string, _ []string) (map[string]models.EODBar, error) {
			exchangesCalled = append(exchangesCalled, exchange)
			return nil, nil
		},
		getEODFn: func(_ context.Context, _ string, _ ...interfaces.EODOption) (*models.EODResponse, error) {
			return &models.EODResponse{Data: []models.EODBar{{Date: time.Now(), Close: 10}}}, nil
		},
		getFundFn: func(_ context.Context, _ string) (*models.Fundamentals, error) {
			return &models.Fundamentals{}, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)

	tickers := []string{"BHP.AU", "RIO.AU", "AAPL.US", "MSFT.US", "SAP.XETRA"}
	svc.CollectCoreMarketData(context.Background(), tickers, false)

	// Should call GetBulkEOD for each unique exchange
	exchangeSet := make(map[string]bool)
	for _, e := range exchangesCalled {
		exchangeSet[e] = true
	}

	if !exchangeSet["AU"] {
		t.Error("expected GetBulkEOD call for AU exchange")
	}
	if !exchangeSet["US"] {
		t.Error("expected GetBulkEOD call for US exchange")
	}
	if !exchangeSet["XETRA"] {
		t.Error("expected GetBulkEOD call for XETRA exchange")
	}
}

// ============================================================================
// 5. BulkEOD returns fewer tickers than requested — missing tickers fallback
// ============================================================================

func TestStress_CollectCoreMarketData_BulkEOD_MissingTickers(t *testing.T) {
	now := time.Now()
	yesterday := now.AddDate(0, 0, -1)
	var eodFallbackCalled atomic.Int64

	storage := &mockStorageManager{
		market: &mockMarketDataStorage{data: map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", Exchange: "AU", DataVersion: common.SchemaVersion, EODUpdatedAt: yesterday, EOD: []models.EODBar{{Date: yesterday, Close: 42}}},
			"RIO.AU": {Ticker: "RIO.AU", Exchange: "AU", DataVersion: common.SchemaVersion, EODUpdatedAt: yesterday, EOD: []models.EODBar{{Date: yesterday, Close: 80}}},
		}},
		signals: &mockSignalStorage{},
	}

	eodhd := &mockEODHDClient{
		getBulkEODFn: func(_ context.Context, _ string, _ []string) (map[string]models.EODBar, error) {
			// Only return BHP, missing RIO
			return map[string]models.EODBar{
				"BHP.AU": {Date: now, Close: 43},
			}, nil
		},
		getEODFn: func(_ context.Context, ticker string, _ ...interfaces.EODOption) (*models.EODResponse, error) {
			eodFallbackCalled.Add(1)
			return &models.EODResponse{
				Data: []models.EODBar{{Date: now, Close: 81}},
			}, nil
		},
		getFundFn: func(_ context.Context, _ string) (*models.Fundamentals, error) {
			return nil, fmt.Errorf("not needed")
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)

	err := svc.CollectCoreMarketData(context.Background(), []string{"BHP.AU", "RIO.AU"}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// RIO should fall back to individual GetEOD since it wasn't in bulk response
	if eodFallbackCalled.Load() == 0 {
		t.Log("NOTE: Missing ticker from BulkEOD was not fetched via individual GetEOD fallback")
	}

	// Both should have data saved
	savedBHP := storage.market.data["BHP.AU"]
	if savedBHP == nil {
		t.Error("BHP.AU should be saved")
	}
	savedRIO := storage.market.data["RIO.AU"]
	if savedRIO == nil {
		t.Error("RIO.AU should be saved")
	}
}

// ============================================================================
// 6. Nil EODHD client — should not panic
// ============================================================================

func TestStress_CollectCoreMarketData_NilEODHDClient(t *testing.T) {
	storage := &mockStorageManager{
		market:  &mockMarketDataStorage{data: map[string]*models.MarketData{}},
		signals: &mockSignalStorage{},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, logger)

	// Should not panic
	err := svc.CollectCoreMarketData(context.Background(), []string{"BHP.AU"}, false)
	if err != nil {
		t.Logf("Got error (expected): %v", err)
	}
}

// ============================================================================
// 7. mergeEODBars edge cases
// ============================================================================

func TestStress_MergeEODBars_EmptyNew(t *testing.T) {
	existing := []models.EODBar{
		{Date: time.Date(2026, 2, 20, 0, 0, 0, 0, time.UTC), Close: 42},
		{Date: time.Date(2026, 2, 19, 0, 0, 0, 0, time.UTC), Close: 41},
	}

	merged := mergeEODBars(nil, existing)
	if len(merged) != 2 {
		t.Errorf("expected 2 bars, got %d", len(merged))
	}
}

func TestStress_MergeEODBars_EmptyExisting(t *testing.T) {
	newBars := []models.EODBar{
		{Date: time.Date(2026, 2, 20, 0, 0, 0, 0, time.UTC), Close: 42},
	}

	merged := mergeEODBars(newBars, nil)
	if len(merged) != 1 {
		t.Errorf("expected 1 bar, got %d", len(merged))
	}
}

func TestStress_MergeEODBars_DuplicateDate_NewOverrides(t *testing.T) {
	existing := []models.EODBar{
		{Date: time.Date(2026, 2, 20, 0, 0, 0, 0, time.UTC), Close: 42, Volume: 1000},
	}
	newBars := []models.EODBar{
		{Date: time.Date(2026, 2, 20, 0, 0, 0, 0, time.UTC), Close: 43, Volume: 2000},
	}

	merged := mergeEODBars(newBars, existing)

	// Should have exactly 1 bar (dedup by date)
	if len(merged) != 1 {
		t.Errorf("expected 1 bar after dedup, got %d", len(merged))
	}

	// New bar should win
	if merged[0].Close != 43 {
		t.Errorf("expected new bar Close=43, got %.2f", merged[0].Close)
	}
}

// ============================================================================
// 8. ExtractPDFTextFromBytes panic recovery
// ============================================================================
//
// The panic recovery in ExtractPDFTextFromBytes was the primary production crash fix.
// We verify that invalid PDF data is handled and returns an error
// instead of crashing the process.

func TestStress_ExtractPDFTextFromBytes_InvalidData(t *testing.T) {
	_, err := ExtractPDFTextFromBytes([]byte("not a valid PDF"))
	if err == nil {
		t.Error("expected error for invalid PDF data")
	}
}

// ============================================================================
// 9. Force flag bypasses freshness on CollectCoreMarketData
// ============================================================================

func TestStress_CollectCoreMarketData_Force_BypassesFreshness(t *testing.T) {
	now := time.Now()
	var eodCalled, fundCalled atomic.Int64

	storage := &mockStorageManager{
		market: &mockMarketDataStorage{data: map[string]*models.MarketData{
			"BHP.AU": {
				Ticker:                "BHP.AU",
				Exchange:              "AU",
				DataVersion:           common.SchemaVersion,
				EODUpdatedAt:          now, // fresh
				FundamentalsUpdatedAt: now, // fresh
				EOD:                   []models.EODBar{{Date: now, Close: 42}},
				Fundamentals:          &models.Fundamentals{ISIN: "AU000000BHP4"},
			},
		}},
		signals: &mockSignalStorage{},
	}

	eodhd := &mockEODHDClient{
		getBulkEODFn: func(_ context.Context, _ string, _ []string) (map[string]models.EODBar, error) {
			return nil, fmt.Errorf("no bulk")
		},
		getEODFn: func(_ context.Context, _ string, _ ...interfaces.EODOption) (*models.EODResponse, error) {
			eodCalled.Add(1)
			return &models.EODResponse{Data: []models.EODBar{{Date: now, Close: 43}}}, nil
		},
		getFundFn: func(_ context.Context, _ string) (*models.Fundamentals, error) {
			fundCalled.Add(1)
			return &models.Fundamentals{ISIN: "AU000000BHP4"}, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)

	// force=true should bypass freshness
	err := svc.CollectCoreMarketData(context.Background(), []string{"BHP.AU"}, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if eodCalled.Load() == 0 {
		t.Error("GetEOD should be called when force=true, even if data is fresh")
	}
	if fundCalled.Load() == 0 {
		t.Error("GetFundamentals should be called when force=true, even if data is fresh")
	}
}
