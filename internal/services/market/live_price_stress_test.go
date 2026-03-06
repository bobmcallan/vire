package market

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
)

// ============================================================================
// 1. CollectLivePrices -- nil EODHD client returns error
// ============================================================================

func TestStress_CollectLivePrices_NilEODHD(t *testing.T) {
	index := newBulkTestStockIndex(
		&models.StockIndexEntry{Ticker: "BHP.AU", Code: "BHP", Exchange: "AU"},
	)
	storage := &bulkTestStorage{
		market:  &mockMarketDataStorage{data: map[string]*models.MarketData{}},
		signals: &mockSignalStorage{},
		index:   index,
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, logger)

	err := svc.CollectLivePrices(context.Background(), "AU")
	if err == nil {
		t.Fatal("expected error when EODHD client is nil")
	}
}

// ============================================================================
// 2. CollectLivePrices -- empty stock index returns nil (no error)
// ============================================================================

func TestStress_CollectLivePrices_EmptyStockIndex(t *testing.T) {
	index := newBulkTestStockIndex()
	storage := &bulkTestStorage{
		market:  &mockMarketDataStorage{data: map[string]*models.MarketData{}},
		signals: &mockSignalStorage{},
		index:   index,
	}

	apiCalled := false
	eodhd := &mockEODHDClient{
		getBulkRealTimeQuotesFn: func(_ context.Context, _ []string) (map[string]*models.RealTimeQuote, error) {
			apiCalled = true
			return nil, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)

	err := svc.CollectLivePrices(context.Background(), "AU")
	if err != nil {
		t.Fatalf("empty stock index should not error: %v", err)
	}
	if apiCalled {
		t.Error("API should not be called when stock index is empty")
	}
}

// ============================================================================
// 3. CollectLivePrices -- exchange filter: only tickers for requested exchange
// ============================================================================

func TestStress_CollectLivePrices_ExchangeFilter(t *testing.T) {
	index := newBulkTestStockIndex(
		&models.StockIndexEntry{Ticker: "BHP.AU", Code: "BHP", Exchange: "AU"},
		&models.StockIndexEntry{Ticker: "AAPL.US", Code: "AAPL", Exchange: "US"},
		&models.StockIndexEntry{Ticker: "RIO.AU", Code: "RIO", Exchange: "AU"},
	)

	storage := &bulkTestStorage{
		market: &mockMarketDataStorage{data: map[string]*models.MarketData{
			"BHP.AU":  {Ticker: "BHP.AU", Exchange: "AU"},
			"RIO.AU":  {Ticker: "RIO.AU", Exchange: "AU"},
			"AAPL.US": {Ticker: "AAPL.US", Exchange: "US"},
		}},
		signals: &mockSignalStorage{},
		index:   index,
	}

	var tickersSent []string
	eodhd := &mockEODHDClient{
		getBulkRealTimeQuotesFn: func(_ context.Context, tickers []string) (map[string]*models.RealTimeQuote, error) {
			tickersSent = append(tickersSent, tickers...)
			return map[string]*models.RealTimeQuote{}, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)
	svc.CollectLivePrices(context.Background(), "AU")

	for _, ticker := range tickersSent {
		if ticker == "AAPL.US" {
			t.Error("AAPL.US (US exchange) should not be included in AU live price request")
		}
	}
	if len(tickersSent) != 2 {
		t.Errorf("expected 2 AU tickers, got %d: %v", len(tickersSent), tickersSent)
	}
}

// ============================================================================
// 4. CollectLivePrices -- skips tickers without existing MarketData
// ============================================================================

func TestStress_CollectLivePrices_SkipsNewTickers(t *testing.T) {
	index := newBulkTestStockIndex(
		&models.StockIndexEntry{Ticker: "BHP.AU", Code: "BHP", Exchange: "AU"},
		&models.StockIndexEntry{Ticker: "NEW.AU", Code: "NEW", Exchange: "AU"},
	)

	storage := &bulkTestStorage{
		market: &mockMarketDataStorage{data: map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", Exchange: "AU", EOD: []models.EODBar{{Close: 42}}},
			// NEW.AU has no MarketData — should be filtered out
		}},
		signals: &mockSignalStorage{},
		index:   index,
	}

	var tickersSent []string
	eodhd := &mockEODHDClient{
		getBulkRealTimeQuotesFn: func(_ context.Context, tickers []string) (map[string]*models.RealTimeQuote, error) {
			tickersSent = tickers
			return map[string]*models.RealTimeQuote{
				"BHP.AU": {Code: "BHP.AU", Close: 43.5, Source: "eodhd"},
			}, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)
	svc.CollectLivePrices(context.Background(), "AU")

	for _, t2 := range tickersSent {
		if t2 == "NEW.AU" {
			t.Error("NEW.AU should not be sent to API (no existing MarketData)")
		}
	}
}

// ============================================================================
// 5. CollectLivePrices -- API error on one batch does not abort remaining
// ============================================================================

func TestStress_CollectLivePrices_PartialBatchFailure(t *testing.T) {
	// Create 25 tickers to force 2 batches (20 + 5)
	var entries []*models.StockIndexEntry
	data := make(map[string]*models.MarketData)
	for i := 0; i < 25; i++ {
		ticker := fmt.Sprintf("T%02d.AU", i)
		entries = append(entries, &models.StockIndexEntry{Ticker: ticker, Code: fmt.Sprintf("T%02d", i), Exchange: "AU"})
		data[ticker] = &models.MarketData{Ticker: ticker, Exchange: "AU"}
	}

	index := newBulkTestStockIndex(entries...)
	storage := &bulkTestStorage{
		market:  &mockMarketDataStorage{data: data},
		signals: &mockSignalStorage{},
		index:   index,
	}

	var callCount atomic.Int64
	eodhd := &mockEODHDClient{
		getBulkRealTimeQuotesFn: func(_ context.Context, tickers []string) (map[string]*models.RealTimeQuote, error) {
			n := callCount.Add(1)
			if n == 1 {
				// First batch of 20 fails
				return nil, fmt.Errorf("EODHD API down")
			}
			// Second batch of 5 succeeds
			result := make(map[string]*models.RealTimeQuote)
			for _, t := range tickers {
				result[t] = &models.RealTimeQuote{Code: t, Close: 10.0, Source: "eodhd"}
			}
			return result, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)
	err := svc.CollectLivePrices(context.Background(), "AU")
	if err != nil {
		t.Fatalf("partial batch failure should not return error: %v", err)
	}

	if callCount.Load() != 2 {
		t.Errorf("expected 2 API calls (2 batches), got %d", callCount.Load())
	}

	// Verify second batch tickers got updated
	for i := 20; i < 25; i++ {
		ticker := fmt.Sprintf("T%02d.AU", i)
		md, _ := storage.market.GetMarketData(context.Background(), ticker)
		if md == nil || md.LivePrice == nil {
			t.Errorf("ticker %s should have LivePrice set from second batch", ticker)
		}
	}
}

// ============================================================================
// 6. CollectLivePrices -- does NOT modify EOD bars (data integrity)
// ============================================================================

func TestStress_CollectLivePrices_DoesNotModifyEOD(t *testing.T) {
	yesterday := time.Now().AddDate(0, 0, -1)
	originalBars := []models.EODBar{
		{Date: yesterday, Open: 40, High: 43, Low: 39, Close: 42, Volume: 1000000},
	}

	index := newBulkTestStockIndex(
		&models.StockIndexEntry{Ticker: "BHP.AU", Code: "BHP", Exchange: "AU"},
	)
	storage := &bulkTestStorage{
		market: &mockMarketDataStorage{data: map[string]*models.MarketData{
			"BHP.AU": {
				Ticker:   "BHP.AU",
				Exchange: "AU",
				EOD:      originalBars,
			},
		}},
		signals: &mockSignalStorage{},
		index:   index,
	}

	eodhd := &mockEODHDClient{
		getBulkRealTimeQuotesFn: func(_ context.Context, _ []string) (map[string]*models.RealTimeQuote, error) {
			return map[string]*models.RealTimeQuote{
				"BHP.AU": {Code: "BHP.AU", Open: 42.5, High: 44, Low: 42, Close: 43.8, Volume: 2000000, Source: "eodhd"},
			}, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)
	svc.CollectLivePrices(context.Background(), "AU")

	md, _ := storage.market.GetMarketData(context.Background(), "BHP.AU")
	if md == nil {
		t.Fatal("market data should exist")
	}

	// LivePrice should be set
	if md.LivePrice == nil {
		t.Fatal("LivePrice should be set")
	}
	if md.LivePrice.Close != 43.8 {
		t.Errorf("LivePrice.Close = %.2f, want 43.8", md.LivePrice.Close)
	}

	// EOD bars must be unchanged
	if len(md.EOD) != 1 {
		t.Fatalf("expected 1 EOD bar, got %d", len(md.EOD))
	}
	if md.EOD[0].Close != 42 {
		t.Errorf("EOD bar close was modified: got %.2f, want 42", md.EOD[0].Close)
	}
	if md.EOD[0].Volume != 1000000 {
		t.Errorf("EOD bar volume was modified: got %d, want 1000000", md.EOD[0].Volume)
	}
}

// ============================================================================
// 7. CollectLivePrices -- LivePriceUpdatedAt is always set
// ============================================================================

func TestStress_CollectLivePrices_SetsTimestamp(t *testing.T) {
	before := time.Now()

	index := newBulkTestStockIndex(
		&models.StockIndexEntry{Ticker: "BHP.AU", Code: "BHP", Exchange: "AU"},
	)
	storage := &bulkTestStorage{
		market: &mockMarketDataStorage{data: map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", Exchange: "AU"},
		}},
		signals: &mockSignalStorage{},
		index:   index,
	}

	eodhd := &mockEODHDClient{
		getBulkRealTimeQuotesFn: func(_ context.Context, _ []string) (map[string]*models.RealTimeQuote, error) {
			return map[string]*models.RealTimeQuote{
				"BHP.AU": {Code: "BHP.AU", Close: 43.0, Source: "eodhd"},
			}, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)
	svc.CollectLivePrices(context.Background(), "AU")

	md, _ := storage.market.GetMarketData(context.Background(), "BHP.AU")
	if md.LivePriceUpdatedAt.Before(before) {
		t.Error("LivePriceUpdatedAt should be set to a recent time")
	}

	// Stock index timestamp should also be updated
	key := "BHP.AU:live_price_collected_at"
	storage.index.mu.RLock()
	ts, ok := storage.index.updates[key]
	storage.index.mu.RUnlock()
	if !ok {
		t.Error("stock index live_price_collected_at should be updated")
	}
	if ts.Before(before) {
		t.Error("stock index timestamp should be recent")
	}
}

// ============================================================================
// 8. CollectLivePrices -- ticker code mismatch (EODHD returns code without
//    exchange suffix). The implementation should fall back to extractCode.
// ============================================================================

func TestStress_CollectLivePrices_TickerCodeFallback(t *testing.T) {
	index := newBulkTestStockIndex(
		&models.StockIndexEntry{Ticker: "BHP.AU", Code: "BHP", Exchange: "AU"},
		&models.StockIndexEntry{Ticker: "RIO.AU", Code: "RIO", Exchange: "AU"},
	)
	storage := &bulkTestStorage{
		market: &mockMarketDataStorage{data: map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", Exchange: "AU"},
			"RIO.AU": {Ticker: "RIO.AU", Exchange: "AU"},
		}},
		signals: &mockSignalStorage{},
		index:   index,
	}

	eodhd := &mockEODHDClient{
		getBulkRealTimeQuotesFn: func(_ context.Context, _ []string) (map[string]*models.RealTimeQuote, error) {
			// EODHD returns codes WITHOUT exchange suffix
			return map[string]*models.RealTimeQuote{
				"BHP": {Code: "BHP", Close: 43.5, Source: "eodhd"},
				"RIO": {Code: "RIO", Close: 80.2, Source: "eodhd"},
			}, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)
	svc.CollectLivePrices(context.Background(), "AU")

	// Both should be updated via the code fallback
	md1, _ := storage.market.GetMarketData(context.Background(), "BHP.AU")
	if md1 == nil || md1.LivePrice == nil {
		t.Error("BHP.AU should have LivePrice set via code fallback")
	}
	md2, _ := storage.market.GetMarketData(context.Background(), "RIO.AU")
	if md2 == nil || md2.LivePrice == nil {
		t.Error("RIO.AU should have LivePrice set via code fallback")
	}
}

// ============================================================================
// 9. CollectLivePrices -- API returns empty map (market closed / no data)
// ============================================================================

func TestStress_CollectLivePrices_EmptyAPIResponse(t *testing.T) {
	index := newBulkTestStockIndex(
		&models.StockIndexEntry{Ticker: "BHP.AU", Code: "BHP", Exchange: "AU"},
	)
	storage := &bulkTestStorage{
		market: &mockMarketDataStorage{data: map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", Exchange: "AU"},
		}},
		signals: &mockSignalStorage{},
		index:   index,
	}

	eodhd := &mockEODHDClient{
		getBulkRealTimeQuotesFn: func(_ context.Context, _ []string) (map[string]*models.RealTimeQuote, error) {
			return map[string]*models.RealTimeQuote{}, nil // empty
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)
	err := svc.CollectLivePrices(context.Background(), "AU")
	if err != nil {
		t.Fatalf("empty API response should not error: %v", err)
	}

	md, _ := storage.market.GetMarketData(context.Background(), "BHP.AU")
	if md.LivePrice != nil {
		t.Error("LivePrice should remain nil when API returns no data")
	}
}

// ============================================================================
// 10. CollectLivePrices -- zero-price quote (market closed / pre-market)
// ============================================================================

func TestStress_CollectLivePrices_ZeroPriceQuote(t *testing.T) {
	index := newBulkTestStockIndex(
		&models.StockIndexEntry{Ticker: "BHP.AU", Code: "BHP", Exchange: "AU"},
	)
	storage := &bulkTestStorage{
		market: &mockMarketDataStorage{data: map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", Exchange: "AU"},
		}},
		signals: &mockSignalStorage{},
		index:   index,
	}

	eodhd := &mockEODHDClient{
		getBulkRealTimeQuotesFn: func(_ context.Context, _ []string) (map[string]*models.RealTimeQuote, error) {
			return map[string]*models.RealTimeQuote{
				"BHP.AU": {Code: "BHP.AU", Close: 0, Open: 0, High: 0, Low: 0, Volume: 0, Source: "eodhd"},
			}, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)
	svc.CollectLivePrices(context.Background(), "AU")

	// Zero price is stored as-is -- consumers must check Close > 0 before using
	md, _ := storage.market.GetMarketData(context.Background(), "BHP.AU")
	if md.LivePrice == nil {
		t.Fatal("LivePrice should be set even with zero close")
	}
	if md.LivePrice.Close != 0 {
		t.Errorf("expected Close 0, got %.2f", md.LivePrice.Close)
	}
}

// ============================================================================
// 11. CollectLivePrices -- batch boundary: exactly 20 tickers = 1 batch
// ============================================================================

func TestStress_CollectLivePrices_BatchBoundary(t *testing.T) {
	var entries []*models.StockIndexEntry
	data := make(map[string]*models.MarketData)
	for i := 0; i < 20; i++ {
		ticker := fmt.Sprintf("T%02d.AU", i)
		entries = append(entries, &models.StockIndexEntry{Ticker: ticker, Code: fmt.Sprintf("T%02d", i), Exchange: "AU"})
		data[ticker] = &models.MarketData{Ticker: ticker, Exchange: "AU"}
	}

	index := newBulkTestStockIndex(entries...)
	storage := &bulkTestStorage{
		market:  &mockMarketDataStorage{data: data},
		signals: &mockSignalStorage{},
		index:   index,
	}

	var callCount atomic.Int64
	eodhd := &mockEODHDClient{
		getBulkRealTimeQuotesFn: func(_ context.Context, tickers []string) (map[string]*models.RealTimeQuote, error) {
			callCount.Add(1)
			if len(tickers) > 20 {
				t.Errorf("batch size %d exceeds max 20", len(tickers))
			}
			result := make(map[string]*models.RealTimeQuote)
			for _, tk := range tickers {
				result[tk] = &models.RealTimeQuote{Code: tk, Close: 10.0, Source: "eodhd"}
			}
			return result, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)
	svc.CollectLivePrices(context.Background(), "AU")

	if callCount.Load() != 1 {
		t.Errorf("expected exactly 1 batch for 20 tickers, got %d", callCount.Load())
	}
}

// ============================================================================
// 12. CollectLivePrices -- batch boundary: 21 tickers = 2 batches (20 + 1)
// ============================================================================

func TestStress_CollectLivePrices_BatchBoundary21(t *testing.T) {
	var entries []*models.StockIndexEntry
	data := make(map[string]*models.MarketData)
	for i := 0; i < 21; i++ {
		ticker := fmt.Sprintf("T%02d.AU", i)
		entries = append(entries, &models.StockIndexEntry{Ticker: ticker, Code: fmt.Sprintf("T%02d", i), Exchange: "AU"})
		data[ticker] = &models.MarketData{Ticker: ticker, Exchange: "AU"}
	}

	index := newBulkTestStockIndex(entries...)
	storage := &bulkTestStorage{
		market:  &mockMarketDataStorage{data: data},
		signals: &mockSignalStorage{},
		index:   index,
	}

	var batchSizes []int
	var mu sync.Mutex
	eodhd := &mockEODHDClient{
		getBulkRealTimeQuotesFn: func(_ context.Context, tickers []string) (map[string]*models.RealTimeQuote, error) {
			mu.Lock()
			batchSizes = append(batchSizes, len(tickers))
			mu.Unlock()
			result := make(map[string]*models.RealTimeQuote)
			for _, tk := range tickers {
				result[tk] = &models.RealTimeQuote{Code: tk, Close: 10.0, Source: "eodhd"}
			}
			return result, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)
	svc.CollectLivePrices(context.Background(), "AU")

	if len(batchSizes) != 2 {
		t.Fatalf("expected 2 batches for 21 tickers, got %d", len(batchSizes))
	}
	if batchSizes[0] != 20 {
		t.Errorf("first batch should be 20, got %d", batchSizes[0])
	}
	if batchSizes[1] != 1 {
		t.Errorf("second batch should be 1, got %d", batchSizes[1])
	}
}

// ============================================================================
// 13. CollectLivePrices -- context cancellation mid-batch
// ============================================================================

func TestStress_CollectLivePrices_ContextCancellation(t *testing.T) {
	var entries []*models.StockIndexEntry
	data := make(map[string]*models.MarketData)
	for i := 0; i < 40; i++ {
		ticker := fmt.Sprintf("T%02d.AU", i)
		entries = append(entries, &models.StockIndexEntry{Ticker: ticker, Code: fmt.Sprintf("T%02d", i), Exchange: "AU"})
		data[ticker] = &models.MarketData{Ticker: ticker, Exchange: "AU"}
	}

	index := newBulkTestStockIndex(entries...)
	storage := &bulkTestStorage{
		market:  &mockMarketDataStorage{data: data},
		signals: &mockSignalStorage{},
		index:   index,
	}

	ctx, cancel := context.WithCancel(context.Background())
	var callCount atomic.Int64
	eodhd := &mockEODHDClient{
		getBulkRealTimeQuotesFn: func(innerCtx context.Context, tickers []string) (map[string]*models.RealTimeQuote, error) {
			n := callCount.Add(1)
			if n == 1 {
				cancel() // Cancel after first batch
			}
			// Check if context is already cancelled
			if innerCtx.Err() != nil {
				return nil, innerCtx.Err()
			}
			result := make(map[string]*models.RealTimeQuote)
			for _, tk := range tickers {
				result[tk] = &models.RealTimeQuote{Code: tk, Close: 10.0, Source: "eodhd"}
			}
			return result, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)
	// Should not panic or deadlock
	svc.CollectLivePrices(ctx, "AU")

	// At least one batch should have been attempted
	if callCount.Load() == 0 {
		t.Error("expected at least 1 API call")
	}
}

// ============================================================================
// 14. CollectLivePrices -- concurrent calls for same exchange (scheduler overlap)
// ============================================================================

func TestStress_CollectLivePrices_ConcurrentCalls(t *testing.T) {
	index := newBulkTestStockIndex(
		&models.StockIndexEntry{Ticker: "BHP.AU", Code: "BHP", Exchange: "AU"},
		&models.StockIndexEntry{Ticker: "RIO.AU", Code: "RIO", Exchange: "AU"},
	)
	storage := &bulkTestStorage{
		market: &mockMarketDataStorage{data: map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", Exchange: "AU"},
			"RIO.AU": {Ticker: "RIO.AU", Exchange: "AU"},
		}},
		signals: &mockSignalStorage{},
		index:   index,
	}

	var apiCalls atomic.Int64
	eodhd := &mockEODHDClient{
		getBulkRealTimeQuotesFn: func(_ context.Context, tickers []string) (map[string]*models.RealTimeQuote, error) {
			apiCalls.Add(1)
			time.Sleep(10 * time.Millisecond) // Simulate API latency
			result := make(map[string]*models.RealTimeQuote)
			for _, tk := range tickers {
				result[tk] = &models.RealTimeQuote{Code: tk, Close: 10.0, Source: "eodhd"}
			}
			return result, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)

	// Run 5 concurrent calls -- should not panic, deadlock, or corrupt data
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc.CollectLivePrices(context.Background(), "AU")
		}()
	}
	wg.Wait()

	// All calls should complete. Each call makes 1 API call (2 tickers < 20 batch size)
	if apiCalls.Load() != 5 {
		t.Errorf("expected 5 API calls (1 per concurrent call), got %d", apiCalls.Load())
	}

	// Data should be consistent (last writer wins, but no corruption)
	md, _ := storage.market.GetMarketData(context.Background(), "BHP.AU")
	if md == nil || md.LivePrice == nil {
		t.Error("BHP.AU should have LivePrice after concurrent updates")
	}
}

// ============================================================================
// 15. CollectLivePrices -- large ticker count (100 tickers, 5 batches)
// ============================================================================

func TestStress_CollectLivePrices_LargeTickerCount(t *testing.T) {
	var entries []*models.StockIndexEntry
	data := make(map[string]*models.MarketData)
	for i := 0; i < 100; i++ {
		ticker := fmt.Sprintf("T%03d.AU", i)
		entries = append(entries, &models.StockIndexEntry{Ticker: ticker, Code: fmt.Sprintf("T%03d", i), Exchange: "AU"})
		data[ticker] = &models.MarketData{Ticker: ticker, Exchange: "AU"}
	}

	index := newBulkTestStockIndex(entries...)
	storage := &bulkTestStorage{
		market:  &mockMarketDataStorage{data: data},
		signals: &mockSignalStorage{},
		index:   index,
	}

	var apiCalls atomic.Int64
	eodhd := &mockEODHDClient{
		getBulkRealTimeQuotesFn: func(_ context.Context, tickers []string) (map[string]*models.RealTimeQuote, error) {
			apiCalls.Add(1)
			result := make(map[string]*models.RealTimeQuote)
			for _, tk := range tickers {
				result[tk] = &models.RealTimeQuote{Code: tk, Close: 10.0, Source: "eodhd"}
			}
			return result, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)
	err := svc.CollectLivePrices(context.Background(), "AU")
	if err != nil {
		t.Fatalf("100 tickers should work: %v", err)
	}

	if apiCalls.Load() != 5 {
		t.Errorf("expected 5 batches (100/20), got %d", apiCalls.Load())
	}

	// Verify all tickers updated
	updated := 0
	for i := 0; i < 100; i++ {
		ticker := fmt.Sprintf("T%03d.AU", i)
		md, _ := storage.market.GetMarketData(context.Background(), ticker)
		if md != nil && md.LivePrice != nil {
			updated++
		}
	}
	if updated != 100 {
		t.Errorf("expected 100 tickers updated, got %d", updated)
	}
}

// ============================================================================
// 16. CollectLivePrices -- LivePrice is overwritten on each refresh (ephemeral)
// ============================================================================

func TestStress_CollectLivePrices_OverwritesPreviousLivePrice(t *testing.T) {
	index := newBulkTestStockIndex(
		&models.StockIndexEntry{Ticker: "BHP.AU", Code: "BHP", Exchange: "AU"},
	)
	storage := &bulkTestStorage{
		market: &mockMarketDataStorage{data: map[string]*models.MarketData{
			"BHP.AU": {
				Ticker:   "BHP.AU",
				Exchange: "AU",
				LivePrice: &models.RealTimeQuote{
					Code: "BHP.AU", Close: 40.0, Source: "eodhd",
				},
				LivePriceUpdatedAt: time.Now().Add(-30 * time.Minute),
			},
		}},
		signals: &mockSignalStorage{},
		index:   index,
	}

	eodhd := &mockEODHDClient{
		getBulkRealTimeQuotesFn: func(_ context.Context, _ []string) (map[string]*models.RealTimeQuote, error) {
			return map[string]*models.RealTimeQuote{
				"BHP.AU": {Code: "BHP.AU", Close: 43.8, Source: "eodhd"},
			}, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)
	svc.CollectLivePrices(context.Background(), "AU")

	md, _ := storage.market.GetMarketData(context.Background(), "BHP.AU")
	if md.LivePrice.Close != 43.8 {
		t.Errorf("LivePrice should be overwritten to 43.8, got %.2f", md.LivePrice.Close)
	}
}

// ============================================================================
// 17. TimestampFieldForJobType -- collect_live_prices returns "" (per-ticker)
// ============================================================================

func TestStress_TimestampFieldForJobType_LivePrices(t *testing.T) {
	field := models.TimestampFieldForJobType(models.JobTypeCollectLivePrices)
	if field != "" {
		t.Errorf("collect_live_prices should return empty string (handled per-ticker), got %q", field)
	}
}

// ============================================================================
// 18. DefaultPriority -- collect_live_prices has priority 11 (higher than EOD)
// ============================================================================

func TestStress_DefaultPriority_LivePrices(t *testing.T) {
	p := models.DefaultPriority(models.JobTypeCollectLivePrices)
	if p != models.PriorityCollectLivePrices {
		t.Errorf("expected priority %d, got %d", models.PriorityCollectLivePrices, p)
	}
	if p <= models.PriorityCollectEOD {
		t.Errorf("live price priority (%d) should be higher than EOD priority (%d)", p, models.PriorityCollectEOD)
	}
}
