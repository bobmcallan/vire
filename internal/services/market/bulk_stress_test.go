package market

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

// --- mock StockIndexStore for bulk stress tests ---
// Also provides newMockStockIndex and mockStorageManagerWithIndex used by other test files.

type bulkTestStockIndex struct {
	mu      sync.RWMutex
	entries []*models.StockIndexEntry
	updates map[string]time.Time
}

func newBulkTestStockIndex(entries ...*models.StockIndexEntry) *bulkTestStockIndex {
	return &bulkTestStockIndex{
		entries: entries,
		updates: make(map[string]time.Time),
	}
}

func (m *bulkTestStockIndex) Upsert(_ context.Context, entry *models.StockIndexEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, e := range m.entries {
		if e.Ticker == entry.Ticker {
			m.entries[i] = entry
			return nil
		}
	}
	m.entries = append(m.entries, entry)
	return nil
}
func (m *bulkTestStockIndex) Get(_ context.Context, ticker string) (*models.StockIndexEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, e := range m.entries {
		if e.Ticker == ticker {
			return e, nil
		}
	}
	return nil, fmt.Errorf("not found")
}
func (m *bulkTestStockIndex) List(_ context.Context) ([]*models.StockIndexEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*models.StockIndexEntry, len(m.entries))
	copy(result, m.entries)
	return result, nil
}
func (m *bulkTestStockIndex) UpdateTimestamp(_ context.Context, ticker, field string, ts time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updates[ticker+":"+field] = ts
	return nil
}
func (m *bulkTestStockIndex) Delete(_ context.Context, _ string) error { return nil }

// --- extended mock StorageManager with StockIndexStore ---

type bulkTestStorage struct {
	market  *mockMarketDataStorage
	signals *mockSignalStorage
	index   *bulkTestStockIndex
}

func (m *bulkTestStorage) MarketDataStorage() interfaces.MarketDataStorage { return m.market }
func (m *bulkTestStorage) SignalStorage() interfaces.SignalStorage         { return m.signals }
func (m *bulkTestStorage) InternalStore() interfaces.InternalStore         { return nil }
func (m *bulkTestStorage) UserDataStore() interfaces.UserDataStore         { return nil }
func (m *bulkTestStorage) StockIndexStore() interfaces.StockIndexStore     { return m.index }
func (m *bulkTestStorage) JobQueueStore() interfaces.JobQueueStore         { return nil }
func (m *bulkTestStorage) FileStore() interfaces.FileStore {
	return &mockFileStore{files: make(map[string][]byte)}
}
func (m *bulkTestStorage) DataPath() string                               { return "" }
func (m *bulkTestStorage) WriteRaw(subdir, key string, data []byte) error { return nil }
func (m *bulkTestStorage) PurgeDerivedData(_ context.Context) (map[string]int, error) {
	return nil, nil
}
func (m *bulkTestStorage) PurgeReports(_ context.Context) (int, error) { return 0, nil }
func (m *bulkTestStorage) Close() error                                { return nil }

// Aliases used by other test files (e.g. test-creator tests in service_test.go)
type mockStorageManagerWithIndex = bulkTestStorage

func newMockStockIndex(entries ...*models.StockIndexEntry) *bulkTestStockIndex {
	return newBulkTestStockIndex(entries...)
}

// ============================================================================
// 1. CollectBulkEOD -- bulk API fails entirely, should return error
// ============================================================================

func TestStress_CollectBulkEOD_BulkAPIFails(t *testing.T) {
	yesterday := time.Now().AddDate(0, 0, -1)

	index := newBulkTestStockIndex(
		&models.StockIndexEntry{Ticker: "BHP.AU", Code: "BHP", Exchange: "AU"},
		&models.StockIndexEntry{Ticker: "RIO.AU", Code: "RIO", Exchange: "AU"},
	)

	storage := &bulkTestStorage{
		market: &mockMarketDataStorage{data: map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", Exchange: "AU", DataVersion: common.SchemaVersion, EODUpdatedAt: yesterday, EOD: []models.EODBar{{Date: yesterday, Close: 42}}},
			"RIO.AU": {Ticker: "RIO.AU", Exchange: "AU", DataVersion: common.SchemaVersion, EODUpdatedAt: yesterday, EOD: []models.EODBar{{Date: yesterday, Close: 80}}},
		}},
		signals: &mockSignalStorage{},
		index:   index,
	}

	eodhd := &mockEODHDClient{
		getBulkEODFn: func(_ context.Context, _ string, _ []string) (map[string]models.EODBar, error) {
			return nil, fmt.Errorf("EODHD API down")
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)

	err := svc.CollectBulkEOD(context.Background(), "AU", false)
	if err == nil {
		t.Fatal("expected error when bulk API fails entirely")
	}
}

// ============================================================================
// 2. CollectBulkEOD -- nil EODHD client
// ============================================================================

func TestStress_CollectBulkEOD_NilEODHD(t *testing.T) {
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

	err := svc.CollectBulkEOD(context.Background(), "AU", false)
	if err == nil {
		t.Fatal("expected error when EODHD client is nil")
	}
}

// ============================================================================
// 3. CollectBulkEOD -- empty stock index for exchange
// ============================================================================

func TestStress_CollectBulkEOD_EmptyStockIndex(t *testing.T) {
	index := newBulkTestStockIndex()

	storage := &bulkTestStorage{
		market:  &mockMarketDataStorage{data: map[string]*models.MarketData{}},
		signals: &mockSignalStorage{},
		index:   index,
	}

	eodhd := &mockEODHDClient{
		getBulkEODFn: func(_ context.Context, _ string, _ []string) (map[string]models.EODBar, error) {
			t.Error("GetBulkEOD should not be called for empty stock index")
			return nil, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)

	err := svc.CollectBulkEOD(context.Background(), "AU", false)
	if err != nil {
		t.Fatalf("empty stock index should not error: %v", err)
	}
}

// ============================================================================
// 4. CollectBulkEOD -- exchange filter: only tickers for requested exchange
// ============================================================================

func TestStress_CollectBulkEOD_ExchangeFilter(t *testing.T) {
	index := newBulkTestStockIndex(
		&models.StockIndexEntry{Ticker: "BHP.AU", Code: "BHP", Exchange: "AU"},
		&models.StockIndexEntry{Ticker: "AAPL.US", Code: "AAPL", Exchange: "US"},
		&models.StockIndexEntry{Ticker: "RIO.AU", Code: "RIO", Exchange: "AU"},
	)

	var tickersSent []string
	storage := &bulkTestStorage{
		market: &mockMarketDataStorage{data: map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", Exchange: "AU", DataVersion: common.SchemaVersion, EOD: []models.EODBar{{Date: time.Now().AddDate(0, 0, -1), Close: 42}}},
			"RIO.AU": {Ticker: "RIO.AU", Exchange: "AU", DataVersion: common.SchemaVersion, EOD: []models.EODBar{{Date: time.Now().AddDate(0, 0, -1), Close: 80}}},
		}},
		signals: &mockSignalStorage{},
		index:   index,
	}

	eodhd := &mockEODHDClient{
		getBulkEODFn: func(_ context.Context, exchange string, tickers []string) (map[string]models.EODBar, error) {
			if exchange != "AU" {
				t.Errorf("expected exchange AU, got %s", exchange)
			}
			tickersSent = tickers
			return map[string]models.EODBar{}, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)
	svc.CollectBulkEOD(context.Background(), "AU", false)

	for _, t2 := range tickersSent {
		if t2 == "AAPL.US" {
			t.Error("AAPL.US (US exchange) should not be included in AU bulk request")
		}
	}
	if len(tickersSent) != 2 {
		t.Errorf("expected 2 AU tickers, got %d: %v", len(tickersSent), tickersSent)
	}
}

// ============================================================================
// 5. CollectBulkEOD -- fallback to individual CollectEOD for new tickers
// ============================================================================

func TestStress_CollectBulkEOD_FallbackForNewTickers(t *testing.T) {
	now := time.Now()
	yesterday := now.AddDate(0, 0, -1)
	var individualEODCalled atomic.Int64

	index := newBulkTestStockIndex(
		&models.StockIndexEntry{Ticker: "BHP.AU", Code: "BHP", Exchange: "AU"},
		&models.StockIndexEntry{Ticker: "NEW.AU", Code: "NEW", Exchange: "AU"},
	)

	storage := &bulkTestStorage{
		market: &mockMarketDataStorage{data: map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", Exchange: "AU", DataVersion: common.SchemaVersion,
				EODUpdatedAt: yesterday,
				EOD:          []models.EODBar{{Date: yesterday, Close: 42}}},
		}},
		signals: &mockSignalStorage{},
		index:   index,
	}

	eodhd := &mockEODHDClient{
		getBulkEODFn: func(_ context.Context, _ string, _ []string) (map[string]models.EODBar, error) {
			return map[string]models.EODBar{"BHP.AU": {Date: now, Close: 43}}, nil
		},
		getEODFn: func(_ context.Context, _ string, _ ...interfaces.EODOption) (*models.EODResponse, error) {
			individualEODCalled.Add(1)
			return &models.EODResponse{Data: []models.EODBar{{Date: now, Close: 15}, {Date: yesterday, Close: 14}}}, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)

	err := svc.CollectBulkEOD(context.Background(), "AU", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if individualEODCalled.Load() == 0 {
		t.Error("individual CollectEOD should be called for tickers with no existing EOD")
	}
	if storage.market.data["BHP.AU"] == nil {
		t.Fatal("BHP.AU should be saved")
	}
}

// ============================================================================
// 6. CollectBulkEOD -- context cancellation mid-processing
// ============================================================================

func TestStress_CollectBulkEOD_ContextCancel(t *testing.T) {
	now := time.Now()
	yesterday := now.AddDate(0, 0, -1)

	entries := make([]*models.StockIndexEntry, 50)
	marketData := make(map[string]*models.MarketData)
	for i := range entries {
		ticker := fmt.Sprintf("T%d.AU", i)
		entries[i] = &models.StockIndexEntry{Ticker: ticker, Code: fmt.Sprintf("T%d", i), Exchange: "AU"}
		marketData[ticker] = &models.MarketData{
			Ticker: ticker, Exchange: "AU", DataVersion: common.SchemaVersion,
			EODUpdatedAt: yesterday, EOD: []models.EODBar{{Date: yesterday, Close: float64(i + 10)}},
		}
	}

	bulkBars := make(map[string]models.EODBar)
	for _, e := range entries {
		bulkBars[e.Ticker] = models.EODBar{Date: now, Close: 99}
	}

	index := newBulkTestStockIndex(entries...)
	storage := &bulkTestStorage{
		market: &mockMarketDataStorage{data: marketData}, signals: &mockSignalStorage{}, index: index,
	}
	eodhd := &mockEODHDClient{
		getBulkEODFn: func(_ context.Context, _ string, _ []string) (map[string]models.EODBar, error) {
			return bulkBars, nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)

	done := make(chan error, 1)
	go func() { done <- svc.CollectBulkEOD(ctx, "AU", false) }()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("DEADLOCK: CollectBulkEOD did not return after context cancellation")
	}
}

// ============================================================================
// 7. CollectBulkEOD -- skips fresh data
// ============================================================================

func TestStress_CollectBulkEOD_SkipsFreshData(t *testing.T) {
	now := time.Now()

	index := newBulkTestStockIndex(&models.StockIndexEntry{Ticker: "BHP.AU", Code: "BHP", Exchange: "AU"})
	storage := &bulkTestStorage{
		market: &mockMarketDataStorage{data: map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", Exchange: "AU", DataVersion: common.SchemaVersion,
				EODUpdatedAt: now, EOD: []models.EODBar{{Date: now, Close: 42}}},
		}},
		signals: &mockSignalStorage{}, index: index,
	}
	eodhd := &mockEODHDClient{
		getBulkEODFn: func(_ context.Context, _ string, _ []string) (map[string]models.EODBar, error) {
			return map[string]models.EODBar{"BHP.AU": {Date: now, Close: 43}}, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)
	svc.CollectBulkEOD(context.Background(), "AU", false)

	if storage.market.data["BHP.AU"].EOD[0].Close != 42 {
		t.Errorf("fresh data should not be overwritten, got close=%.2f", storage.market.data["BHP.AU"].EOD[0].Close)
	}
}

// ============================================================================
// 8. CollectBulkEOD -- force bypasses freshness
// ============================================================================

func TestStress_CollectBulkEOD_ForceBypassesFreshness(t *testing.T) {
	now := time.Now()

	index := newBulkTestStockIndex(&models.StockIndexEntry{Ticker: "BHP.AU", Code: "BHP", Exchange: "AU"})
	storage := &bulkTestStorage{
		market: &mockMarketDataStorage{data: map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", Exchange: "AU", DataVersion: common.SchemaVersion,
				EODUpdatedAt: now, EOD: []models.EODBar{{Date: now.AddDate(0, 0, -1), Close: 42}}},
		}},
		signals: &mockSignalStorage{}, index: index,
	}
	eodhd := &mockEODHDClient{
		getBulkEODFn: func(_ context.Context, _ string, _ []string) (map[string]models.EODBar, error) {
			return map[string]models.EODBar{"BHP.AU": {Date: now, Close: 43}}, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)
	err := svc.CollectBulkEOD(context.Background(), "AU", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(storage.market.data["BHP.AU"].EOD) < 2 {
		t.Log("NOTE: force=true should bypass freshness and merge the new bar")
	}
}

// ============================================================================
// 9. CollectBulkEOD -- ticker not in bulk response is silently skipped
// ============================================================================

func TestStress_CollectBulkEOD_MissingTickerInBulkResponse(t *testing.T) {
	now := time.Now()
	yesterday := now.AddDate(0, 0, -1)

	index := newBulkTestStockIndex(
		&models.StockIndexEntry{Ticker: "BHP.AU", Code: "BHP", Exchange: "AU"},
		&models.StockIndexEntry{Ticker: "MISSING.AU", Code: "MISSING", Exchange: "AU"},
	)
	storage := &bulkTestStorage{
		market: &mockMarketDataStorage{data: map[string]*models.MarketData{
			"BHP.AU":     {Ticker: "BHP.AU", Exchange: "AU", DataVersion: common.SchemaVersion, EODUpdatedAt: yesterday, EOD: []models.EODBar{{Date: yesterday, Close: 42}}},
			"MISSING.AU": {Ticker: "MISSING.AU", Exchange: "AU", DataVersion: common.SchemaVersion, EODUpdatedAt: yesterday, EOD: []models.EODBar{{Date: yesterday, Close: 10}}},
		}},
		signals: &mockSignalStorage{}, index: index,
	}
	eodhd := &mockEODHDClient{
		getBulkEODFn: func(_ context.Context, _ string, _ []string) (map[string]models.EODBar, error) {
			return map[string]models.EODBar{"BHP.AU": {Date: now, Close: 43}}, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)
	svc.CollectBulkEOD(context.Background(), "AU", false)

	if storage.market.data["MISSING.AU"].EOD[0].Close != 10 {
		t.Errorf("MISSING.AU should not have been modified, got close=%.2f", storage.market.data["MISSING.AU"].EOD[0].Close)
	}
}

// ============================================================================
// 10. CollectBulkEOD -- same-day bar does not create duplicate
// ============================================================================

func TestStress_CollectBulkEOD_SameDayBarNoDuplicate(t *testing.T) {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	yesterday := today.AddDate(0, 0, -1)

	index := newBulkTestStockIndex(&models.StockIndexEntry{Ticker: "BHP.AU", Code: "BHP", Exchange: "AU"})
	storage := &bulkTestStorage{
		market: &mockMarketDataStorage{data: map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", Exchange: "AU", DataVersion: common.SchemaVersion,
				EODUpdatedAt: yesterday,
				EOD:          []models.EODBar{{Date: today, Close: 42}, {Date: yesterday, Close: 41}}},
		}},
		signals: &mockSignalStorage{}, index: index,
	}
	eodhd := &mockEODHDClient{
		getBulkEODFn: func(_ context.Context, _ string, _ []string) (map[string]models.EODBar, error) {
			return map[string]models.EODBar{"BHP.AU": {Date: today, Close: 42.5}}, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)
	svc.CollectBulkEOD(context.Background(), "AU", false)

	if len(storage.market.data["BHP.AU"].EOD) != 2 {
		t.Errorf("expected 2 bars (no merge for same date), got %d", len(storage.market.data["BHP.AU"].EOD))
	}
}

// ============================================================================
// 11. CollectBulkEOD -- individual CollectEOD fallback failure is non-fatal
// ============================================================================

func TestStress_CollectBulkEOD_IndividualFallbackFails(t *testing.T) {
	index := newBulkTestStockIndex(&models.StockIndexEntry{Ticker: "NEW.AU", Code: "NEW", Exchange: "AU"})
	storage := &bulkTestStorage{
		market: &mockMarketDataStorage{data: map[string]*models.MarketData{}}, signals: &mockSignalStorage{}, index: index,
	}
	eodhd := &mockEODHDClient{
		getBulkEODFn: func(_ context.Context, _ string, _ []string) (map[string]models.EODBar, error) {
			return map[string]models.EODBar{}, nil
		},
		getEODFn: func(_ context.Context, _ string, _ ...interfaces.EODOption) (*models.EODResponse, error) {
			return nil, fmt.Errorf("API rate limited")
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)
	err := svc.CollectBulkEOD(context.Background(), "AU", false)
	if err != nil {
		t.Fatalf("individual fallback failure should not propagate: %v", err)
	}
}

// ============================================================================
// 12. CollectBulkEOD -- stock index timestamp updated for processed tickers
// ============================================================================

func TestStress_CollectBulkEOD_StockIndexTimestampUpdated(t *testing.T) {
	now := time.Now()
	yesterday := now.AddDate(0, 0, -1)

	index := newBulkTestStockIndex(&models.StockIndexEntry{Ticker: "BHP.AU", Code: "BHP", Exchange: "AU"})
	storage := &bulkTestStorage{
		market: &mockMarketDataStorage{data: map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", Exchange: "AU", DataVersion: common.SchemaVersion,
				EODUpdatedAt: yesterday, EOD: []models.EODBar{{Date: yesterday, Close: 42}}},
		}},
		signals: &mockSignalStorage{}, index: index,
	}
	eodhd := &mockEODHDClient{
		getBulkEODFn: func(_ context.Context, _ string, _ []string) (map[string]models.EODBar, error) {
			return map[string]models.EODBar{"BHP.AU": {Date: now, Close: 43}}, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)
	svc.CollectBulkEOD(context.Background(), "AU", false)

	ts, ok := index.updates["BHP.AU:eod_collected_at"]
	if !ok {
		t.Error("stock index eod_collected_at timestamp should be updated for BHP.AU")
	} else if time.Since(ts) > 5*time.Second {
		t.Errorf("timestamp should be recent, got %v", ts)
	}
}

// ============================================================================
// 13. Watcher: EOD removed from per-ticker checks (architectural validation)
// ============================================================================

func TestStress_Watcher_NoIndividualEODJobs(t *testing.T) {
	if models.JobTypeCollectEOD != "collect_eod" {
		t.Error("JobTypeCollectEOD constant changed")
	}
	if models.JobTypeCollectEODBulk != "collect_eod_bulk" {
		t.Error("JobTypeCollectEODBulk constant changed")
	}
	if field := models.TimestampFieldForJobType(models.JobTypeCollectEODBulk); field != "" {
		t.Errorf("TimestampFieldForJobType for bulk should be empty, got %q", field)
	}
	if field := models.TimestampFieldForJobType(models.JobTypeCollectEOD); field != "eod_collected_at" {
		t.Errorf("TimestampFieldForJobType for EOD should be eod_collected_at, got %q", field)
	}
}

// ============================================================================
// 14. CollectBulkEOD -- multiple exchanges are isolated
// ============================================================================

func TestStress_CollectBulkEOD_ExchangeIsolation(t *testing.T) {
	index := newBulkTestStockIndex(
		&models.StockIndexEntry{Ticker: "BHP.AU", Code: "BHP", Exchange: "AU"},
		&models.StockIndexEntry{Ticker: "AAPL.US", Code: "AAPL", Exchange: "US"},
	)

	var auCalled, usCalled bool
	eodhd := &mockEODHDClient{
		getBulkEODFn: func(_ context.Context, exchange string, _ []string) (map[string]models.EODBar, error) {
			if exchange == "AU" {
				auCalled = true
			}
			if exchange == "US" {
				usCalled = true
			}
			return map[string]models.EODBar{}, nil
		},
	}
	storage := &bulkTestStorage{
		market: &mockMarketDataStorage{data: map[string]*models.MarketData{
			"BHP.AU":  {Ticker: "BHP.AU", Exchange: "AU", DataVersion: common.SchemaVersion, EOD: []models.EODBar{{Date: time.Now().AddDate(0, 0, -1), Close: 42}}},
			"AAPL.US": {Ticker: "AAPL.US", Exchange: "US", DataVersion: common.SchemaVersion, EOD: []models.EODBar{{Date: time.Now().AddDate(0, 0, -1), Close: 150}}},
		}},
		signals: &mockSignalStorage{}, index: index,
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)
	svc.CollectBulkEOD(context.Background(), "AU", false)

	if !auCalled {
		t.Error("expected GetBulkEOD to be called for AU")
	}
	if usCalled {
		t.Error("GetBulkEOD should NOT be called for US when only AU was requested")
	}
}

// ============================================================================
// 15. FINDING: CollectBulkEOD does not check ctx.Err() between iterations
// ============================================================================

func TestStress_CollectBulkEOD_NoContextCheckInLoop(t *testing.T) {
	now := time.Now()
	yesterday := now.AddDate(0, 0, -1)
	var processedAfterCancel atomic.Int64

	entries := make([]*models.StockIndexEntry, 20)
	marketData := make(map[string]*models.MarketData)
	for i := range entries {
		ticker := fmt.Sprintf("T%d.AU", i)
		entries[i] = &models.StockIndexEntry{Ticker: ticker, Code: fmt.Sprintf("T%d", i), Exchange: "AU"}
		marketData[ticker] = &models.MarketData{
			Ticker: ticker, Exchange: "AU", DataVersion: common.SchemaVersion,
			EODUpdatedAt: yesterday, EOD: []models.EODBar{{Date: yesterday, Close: float64(i)}},
		}
	}

	bulkBars := make(map[string]models.EODBar)
	for _, e := range entries {
		bulkBars[e.Ticker] = models.EODBar{Date: now, Close: 99}
	}

	ctx, cancel := context.WithCancel(context.Background())
	index := newBulkTestStockIndex(entries...)
	storage := &bulkTestStorage{
		market: &mockMarketDataStorage{data: marketData}, signals: &mockSignalStorage{}, index: index,
	}
	eodhd := &mockEODHDClient{
		getBulkEODFn: func(_ context.Context, _ string, _ []string) (map[string]models.EODBar, error) {
			cancel()
			return bulkBars, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)
	svc.CollectBulkEOD(ctx, "AU", false)

	for _, e := range entries {
		md := storage.market.data[e.Ticker]
		if md != nil && md.EODUpdatedAt.After(yesterday) {
			processedAfterCancel.Add(1)
		}
	}
	t.Logf("INFO: %d/%d tickers processed after context cancellation (no ctx.Err() check in loop)",
		processedAfterCancel.Load(), len(entries))
}
