package market

import (
	"context"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

func approxEqual(a, b, epsilon float64) bool {
	return math.Abs(a-b) < epsilon
}

// --- mock EODHD client ---

type mockEODHDClient struct {
	realTimeQuoteFn func(ctx context.Context, ticker string) (*models.RealTimeQuote, error)
	getEODFn        func(ctx context.Context, ticker string, opts ...interfaces.EODOption) (*models.EODResponse, error)
	getBulkEODFn    func(ctx context.Context, exchange string, tickers []string) (map[string]models.EODBar, error)
	getFundFn       func(ctx context.Context, ticker string) (*models.Fundamentals, error)
}

func (m *mockEODHDClient) GetRealTimeQuote(ctx context.Context, ticker string) (*models.RealTimeQuote, error) {
	if m.realTimeQuoteFn != nil {
		return m.realTimeQuoteFn(ctx, ticker)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *mockEODHDClient) GetEOD(ctx context.Context, ticker string, opts ...interfaces.EODOption) (*models.EODResponse, error) {
	if m.getEODFn != nil {
		return m.getEODFn(ctx, ticker, opts...)
	}
	return nil, fmt.Errorf("not implemented")
}
func (m *mockEODHDClient) GetBulkEOD(ctx context.Context, exchange string, tickers []string) (map[string]models.EODBar, error) {
	if m.getBulkEODFn != nil {
		return m.getBulkEODFn(ctx, exchange, tickers)
	}
	return nil, fmt.Errorf("not implemented")
}
func (m *mockEODHDClient) GetFundamentals(ctx context.Context, ticker string) (*models.Fundamentals, error) {
	if m.getFundFn != nil {
		return m.getFundFn(ctx, ticker)
	}
	return nil, fmt.Errorf("not implemented")
}
func (m *mockEODHDClient) GetTechnicals(ctx context.Context, ticker string, function string) (*models.TechnicalResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *mockEODHDClient) GetNews(ctx context.Context, ticker string, limit int) ([]*models.NewsItem, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *mockEODHDClient) GetExchangeSymbols(ctx context.Context, exchange string) ([]*models.Symbol, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *mockEODHDClient) ScreenStocks(ctx context.Context, options models.ScreenerOptions) ([]*models.ScreenerResult, error) {
	return nil, fmt.Errorf("not implemented")
}

// --- mock storage ---

type mockMarketDataStorage struct {
	mu   sync.RWMutex
	data map[string]*models.MarketData
}

func (m *mockMarketDataStorage) GetMarketData(_ context.Context, ticker string) (*models.MarketData, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	md, ok := m.data[ticker]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return md, nil
}
func (m *mockMarketDataStorage) SaveMarketData(_ context.Context, data *models.MarketData) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[data.Ticker] = data
	return nil
}
func (m *mockMarketDataStorage) GetMarketDataBatch(_ context.Context, tickers []string) ([]*models.MarketData, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*models.MarketData
	for _, t := range tickers {
		if md, ok := m.data[t]; ok {
			result = append(result, md)
		}
	}
	return result, nil
}
func (m *mockMarketDataStorage) GetStaleTickers(_ context.Context, _ string, _ int64) ([]string, error) {
	return nil, nil
}

type mockSignalStorage struct{}

func (m *mockSignalStorage) GetSignals(_ context.Context, _ string) (*models.TickerSignals, error) {
	return nil, fmt.Errorf("not found")
}
func (m *mockSignalStorage) SaveSignals(_ context.Context, _ *models.TickerSignals) error {
	return nil
}
func (m *mockSignalStorage) GetSignalsBatch(_ context.Context, _ []string) ([]*models.TickerSignals, error) {
	return nil, nil
}

type mockStorageManager struct {
	market  *mockMarketDataStorage
	signals *mockSignalStorage
}

func (m *mockStorageManager) MarketDataStorage() interfaces.MarketDataStorage { return m.market }
func (m *mockStorageManager) SignalStorage() interfaces.SignalStorage         { return m.signals }
func (m *mockStorageManager) InternalStore() interfaces.InternalStore         { return nil }
func (m *mockStorageManager) UserDataStore() interfaces.UserDataStore         { return nil }
func (m *mockStorageManager) StockIndexStore() interfaces.StockIndexStore     { return nil }
func (m *mockStorageManager) JobQueueStore() interfaces.JobQueueStore         { return nil }
func (m *mockStorageManager) DataPath() string                                { return "" }
func (m *mockStorageManager) WriteRaw(subdir, key string, data []byte) error  { return nil }
func (m *mockStorageManager) PurgeDerivedData(_ context.Context) (map[string]int, error) {
	return nil, nil
}
func (m *mockStorageManager) PurgeReports(_ context.Context) (int, error) { return 0, nil }
func (m *mockStorageManager) Close() error                                { return nil }

// --- tests ---

func TestGetStockData_UsesRealTimePrice(t *testing.T) {
	today := time.Now()
	eodClose := 42.50
	livePrice := 43.25

	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {
					Ticker:      "BHP.AU",
					Exchange:    "AU",
					LastUpdated: today,
					EOD: []models.EODBar{
						{Date: today, Open: 42.00, High: 43.00, Low: 41.50, Close: eodClose, Volume: 3000000},
						{Date: today.AddDate(0, 0, -1), Close: 41.80},
					},
				},
			},
		},
		signals: &mockSignalStorage{},
	}

	eodhd := &mockEODHDClient{
		realTimeQuoteFn: func(_ context.Context, ticker string) (*models.RealTimeQuote, error) {
			return &models.RealTimeQuote{
				Code:      ticker,
				Open:      42.20,
				High:      43.50,
				Low:       41.90,
				Close:     livePrice,
				Volume:    5000000,
				Timestamp: today,
			}, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)

	data, err := svc.GetStockData(context.Background(), "BHP.AU", interfaces.StockDataInclude{Price: true})
	if err != nil {
		t.Fatalf("GetStockData failed: %v", err)
	}

	if data.Price == nil {
		t.Fatal("expected price data")
	}

	// Should use live price, not EOD close
	if !approxEqual(data.Price.Current, livePrice, 0.01) {
		t.Errorf("Current = %.2f, want %.2f (live price)", data.Price.Current, livePrice)
	}
	if !approxEqual(data.Price.Open, 42.20, 0.01) {
		t.Errorf("Open = %.2f, want 42.20 (live open)", data.Price.Open)
	}
	if !approxEqual(data.Price.High, 43.50, 0.01) {
		t.Errorf("High = %.2f, want 43.50 (live high)", data.Price.High)
	}
	if data.Price.Volume != 5000000 {
		t.Errorf("Volume = %d, want 5000000 (live volume)", data.Price.Volume)
	}

	// Change should be calculated from live price vs previous EOD close
	expectedChange := livePrice - 41.80
	if !approxEqual(data.Price.Change, expectedChange, 0.01) {
		t.Errorf("Change = %.2f, want %.2f", data.Price.Change, expectedChange)
	}
}

func TestGetStockData_FallsBackToEODOnRealTimeError(t *testing.T) {
	today := time.Now()
	eodClose := 42.50

	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {
					Ticker:      "BHP.AU",
					Exchange:    "AU",
					LastUpdated: today,
					EOD: []models.EODBar{
						{Date: today, Open: 42.00, High: 43.00, Low: 41.50, Close: eodClose, Volume: 3000000},
						{Date: today.AddDate(0, 0, -1), Close: 41.80},
					},
				},
			},
		},
		signals: &mockSignalStorage{},
	}

	eodhd := &mockEODHDClient{
		realTimeQuoteFn: func(_ context.Context, ticker string) (*models.RealTimeQuote, error) {
			return nil, fmt.Errorf("API unavailable")
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)

	data, err := svc.GetStockData(context.Background(), "BHP.AU", interfaces.StockDataInclude{Price: true})
	if err != nil {
		t.Fatalf("GetStockData failed: %v", err)
	}

	if data.Price == nil {
		t.Fatal("expected price data")
	}

	// Should fall back to EOD close
	if !approxEqual(data.Price.Current, eodClose, 0.01) {
		t.Errorf("Current = %.2f, want %.2f (EOD fallback)", data.Price.Current, eodClose)
	}
}

func TestGetStockData_FallsBackOnZeroClosePrice(t *testing.T) {
	today := time.Now()
	eodClose := 42.50

	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {
					Ticker:      "BHP.AU",
					Exchange:    "AU",
					LastUpdated: today,
					EOD: []models.EODBar{
						{Date: today, Open: 42.00, High: 43.00, Low: 41.50, Close: eodClose, Volume: 3000000},
						{Date: today.AddDate(0, 0, -1), Close: 41.80},
					},
				},
			},
		},
		signals: &mockSignalStorage{},
	}

	eodhd := &mockEODHDClient{
		realTimeQuoteFn: func(_ context.Context, ticker string) (*models.RealTimeQuote, error) {
			return &models.RealTimeQuote{Close: 0}, nil // market closed, zero response
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)

	data, err := svc.GetStockData(context.Background(), "BHP.AU", interfaces.StockDataInclude{Price: true})
	if err != nil {
		t.Fatalf("GetStockData failed: %v", err)
	}

	// Zero close should NOT override EOD
	if !approxEqual(data.Price.Current, eodClose, 0.01) {
		t.Errorf("Current = %.2f, want %.2f (should not use zero live price)", data.Price.Current, eodClose)
	}
}

func TestGetStockData_PriceNotRequestedSkipsRealTime(t *testing.T) {
	today := time.Now()
	realTimeCalled := false

	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {
					Ticker:      "BHP.AU",
					LastUpdated: today,
					EOD:         []models.EODBar{{Date: today, Close: 42.50}},
				},
			},
		},
		signals: &mockSignalStorage{},
	}

	eodhd := &mockEODHDClient{
		realTimeQuoteFn: func(_ context.Context, ticker string) (*models.RealTimeQuote, error) {
			realTimeCalled = true
			return &models.RealTimeQuote{Close: 99.99}, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)

	_, err := svc.GetStockData(context.Background(), "BHP.AU", interfaces.StockDataInclude{Price: false})
	if err != nil {
		t.Fatalf("GetStockData failed: %v", err)
	}

	if realTimeCalled {
		t.Error("real-time should not be called when price is not requested")
	}
}

func TestGetStockData_HistoricalFieldsPreservedWithRealTime(t *testing.T) {
	today := time.Now()

	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {
					Ticker:      "BHP.AU",
					Exchange:    "AU",
					LastUpdated: today,
					EOD: func() []models.EODBar {
						// Generate enough bars for 52-week high/low and avg volume
						bars := make([]models.EODBar, 260)
						for i := 0; i < 260; i++ {
							bars[i] = models.EODBar{
								Date:   today.AddDate(0, 0, -i),
								Open:   40.0 + float64(i%10),
								High:   45.0 + float64(i%5),
								Low:    38.0 + float64(i%3),
								Close:  42.0 + float64(i%8),
								Volume: int64(1000000 + i*10000),
							}
						}
						return bars
					}(),
				},
			},
		},
		signals: &mockSignalStorage{},
	}

	eodhd := &mockEODHDClient{
		realTimeQuoteFn: func(_ context.Context, ticker string) (*models.RealTimeQuote, error) {
			return &models.RealTimeQuote{
				Code: ticker, Close: 99.99, Open: 98.0, High: 100.0, Low: 97.0,
				Volume: 9999999, Timestamp: today,
			}, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)

	data, err := svc.GetStockData(context.Background(), "BHP.AU", interfaces.StockDataInclude{Price: true})
	if err != nil {
		t.Fatalf("GetStockData failed: %v", err)
	}

	// 52-week high/low and avg volume should come from EOD bars, not real-time
	if data.Price.High52Week == 0 {
		t.Error("High52Week should be computed from EOD bars")
	}
	if data.Price.Low52Week == 0 {
		t.Error("Low52Week should be computed from EOD bars")
	}
	if data.Price.AvgVolume == 0 {
		t.Error("AvgVolume should be computed from EOD bars")
	}
	if data.Price.PreviousClose == 0 {
		t.Error("PreviousClose should come from EOD bars")
	}
}

func TestCollectMarketData_SchemaMismatchClearsSummaries(t *testing.T) {
	now := time.Now()

	staleSummaries := []models.FilingSummary{
		{Date: now, Headline: "Full Year Results", Type: "other", Period: "FY2025"},
	}
	staleTimeline := &models.CompanyTimeline{
		BusinessModel: "Old model",
		GeneratedAt:   now,
	}

	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"SKS.AU": {
					Ticker:                   "SKS.AU",
					Exchange:                 "AU",
					DataVersion:              "1", // old schema
					EODUpdatedAt:             now, // fresh — skip EOD fetch
					FundamentalsUpdatedAt:    now,
					FilingsUpdatedAt:         now,
					FilingSummariesUpdatedAt: now,
					CompanyTimelineUpdatedAt: now,
					EOD:                      []models.EODBar{{Date: now, Close: 2.50}},
					FilingSummaries:          staleSummaries,
					CompanyTimeline:          staleTimeline,
				},
			},
		},
		signals: &mockSignalStorage{},
	}

	eodhd := &mockEODHDClient{}
	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)

	err := svc.CollectMarketData(context.Background(), []string{"SKS.AU"}, false, false)
	if err != nil {
		t.Fatalf("CollectMarketData failed: %v", err)
	}

	saved := storage.market.data["SKS.AU"]

	// Schema mismatch should clear derived data
	if saved.FilingSummaries != nil {
		t.Errorf("FilingSummaries should be nil after schema mismatch, got %d items", len(saved.FilingSummaries))
	}
	if saved.CompanyTimeline != nil {
		t.Error("CompanyTimeline should be nil after schema mismatch")
	}
	if !saved.FilingSummariesUpdatedAt.IsZero() {
		t.Error("FilingSummariesUpdatedAt should be zero after schema mismatch")
	}
	if !saved.CompanyTimelineUpdatedAt.IsZero() {
		t.Error("CompanyTimelineUpdatedAt should be zero after schema mismatch")
	}

	// FundamentalsUpdatedAt should be zeroed to force re-fetch of new parsed fields
	if !saved.FundamentalsUpdatedAt.IsZero() {
		t.Error("FundamentalsUpdatedAt should be zero after schema mismatch to force re-fetch")
	}

	// DataVersion should be stamped with current schema
	if saved.DataVersion != common.SchemaVersion {
		t.Errorf("DataVersion = %q, want %q", saved.DataVersion, common.SchemaVersion)
	}
}

func TestCollectMarketData_MatchingSchemaPreservesSummaries(t *testing.T) {
	now := time.Now()

	summaries := []models.FilingSummary{
		{Date: now, Headline: "Full Year Results", Type: "financial_results", Revenue: "$261.7M", Period: "FY2025"},
	}
	timeline := &models.CompanyTimeline{
		BusinessModel: "Engineering services",
		GeneratedAt:   now,
	}

	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"SKS.AU": {
					Ticker:                   "SKS.AU",
					Exchange:                 "AU",
					DataVersion:              common.SchemaVersion, // matches current
					EODUpdatedAt:             now,
					FundamentalsUpdatedAt:    now,
					FilingsUpdatedAt:         now,
					FilingSummariesUpdatedAt: now,
					CompanyTimelineUpdatedAt: now,
					EOD:                      []models.EODBar{{Date: now, Close: 2.50}},
					FilingSummaries:          summaries,
					CompanyTimeline:          timeline,
				},
			},
		},
		signals: &mockSignalStorage{},
	}

	eodhd := &mockEODHDClient{}
	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)

	err := svc.CollectMarketData(context.Background(), []string{"SKS.AU"}, false, false)
	if err != nil {
		t.Fatalf("CollectMarketData failed: %v", err)
	}

	saved := storage.market.data["SKS.AU"]

	// Matching schema should preserve existing summaries
	if len(saved.FilingSummaries) != 1 {
		t.Errorf("FilingSummaries length = %d, want 1 (should be preserved)", len(saved.FilingSummaries))
	}
	if saved.CompanyTimeline == nil {
		t.Error("CompanyTimeline should be preserved when schema matches")
	}
	if saved.DataVersion != common.SchemaVersion {
		t.Errorf("DataVersion = %q, want %q", saved.DataVersion, common.SchemaVersion)
	}
}

func TestCollectMarketData_EmptyDataVersionTriggersMismatch(t *testing.T) {
	now := time.Now()

	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"SKS.AU": {
					Ticker:                   "SKS.AU",
					Exchange:                 "AU",
					DataVersion:              "", // pre-versioning data
					EODUpdatedAt:             now,
					FundamentalsUpdatedAt:    now,
					FilingsUpdatedAt:         now,
					FilingSummariesUpdatedAt: now,
					CompanyTimelineUpdatedAt: now,
					EOD:                      []models.EODBar{{Date: now, Close: 2.50}},
					FilingSummaries:          []models.FilingSummary{{Date: now, Headline: "Stale"}},
					CompanyTimeline:          &models.CompanyTimeline{BusinessModel: "Stale"},
				},
			},
		},
		signals: &mockSignalStorage{},
	}

	eodhd := &mockEODHDClient{}
	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)

	err := svc.CollectMarketData(context.Background(), []string{"SKS.AU"}, false, false)
	if err != nil {
		t.Fatalf("CollectMarketData failed: %v", err)
	}

	saved := storage.market.data["SKS.AU"]

	// Empty DataVersion should trigger mismatch (pre-versioning data)
	if saved.FilingSummaries != nil {
		t.Errorf("FilingSummaries should be nil for pre-versioning data, got %d items", len(saved.FilingSummaries))
	}
	if saved.CompanyTimeline != nil {
		t.Error("CompanyTimeline should be nil for pre-versioning data")
	}
	if saved.DataVersion != common.SchemaVersion {
		t.Errorf("DataVersion = %q, want %q", saved.DataVersion, common.SchemaVersion)
	}
}

func TestCollectMarketData_DataVersionStampedOnSave(t *testing.T) {
	now := time.Now()

	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {
					Ticker:                "BHP.AU",
					Exchange:              "AU",
					DataVersion:           common.SchemaVersion,
					EODUpdatedAt:          now,
					FundamentalsUpdatedAt: now,
					EOD:                   []models.EODBar{{Date: now, Close: 42.50}},
				},
			},
		},
		signals: &mockSignalStorage{},
	}

	eodhd := &mockEODHDClient{}
	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)

	err := svc.CollectMarketData(context.Background(), []string{"BHP.AU"}, false, false)
	if err != nil {
		t.Fatalf("CollectMarketData failed: %v", err)
	}

	saved := storage.market.data["BHP.AU"]
	if saved.DataVersion != common.SchemaVersion {
		t.Errorf("DataVersion = %q, want %q", saved.DataVersion, common.SchemaVersion)
	}
}

func TestGetStockData_SurfacesFilingSummaries(t *testing.T) {
	today := time.Now()

	summaries := []models.FilingSummary{
		{Date: today, Headline: "Full Year Results", Type: "financial_results", Revenue: "$261.7M", Period: "FY2025"},
	}
	timeline := &models.CompanyTimeline{
		BusinessModel: "Engineering services",
		Periods:       []models.PeriodSummary{{Period: "FY2025", Revenue: "$261.7M"}},
		GeneratedAt:   today,
	}

	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"SKS.AU": {
					Ticker:          "SKS.AU",
					Exchange:        "AU",
					LastUpdated:     today,
					EOD:             []models.EODBar{{Date: today, Close: 2.50}},
					FilingSummaries: summaries,
					CompanyTimeline: timeline,
				},
			},
		},
		signals: &mockSignalStorage{},
	}

	eodhd := &mockEODHDClient{}
	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)

	data, err := svc.GetStockData(context.Background(), "SKS.AU", interfaces.StockDataInclude{Price: false})
	if err != nil {
		t.Fatalf("GetStockData failed: %v", err)
	}

	// Filing summaries should be surfaced
	if len(data.FilingSummaries) != 1 {
		t.Fatalf("FilingSummaries length = %d, want 1", len(data.FilingSummaries))
	}
	if data.FilingSummaries[0].Revenue != "$261.7M" {
		t.Errorf("FilingSummaries[0].Revenue = %s, want $261.7M", data.FilingSummaries[0].Revenue)
	}

	// Timeline should be surfaced
	if data.Timeline == nil {
		t.Fatal("Timeline is nil")
	}
	if data.Timeline.BusinessModel != "Engineering services" {
		t.Errorf("Timeline.BusinessModel = %s, want Engineering services", data.Timeline.BusinessModel)
	}
}

// --- CollectCoreMarketData tests ---

func TestCollectCoreMarketData_UsesBulkEOD(t *testing.T) {
	now := time.Now()
	yesterday := now.AddDate(0, 0, -1)
	bulkCalled := false

	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {
					Ticker:       "BHP.AU",
					Exchange:     "AU",
					DataVersion:  common.SchemaVersion,
					EODUpdatedAt: yesterday, // stale
					EOD:          []models.EODBar{{Date: yesterday, Close: 42.50}},
				},
			},
		},
		signals: &mockSignalStorage{},
	}

	eodhd := &mockEODHDClient{
		getBulkEODFn: func(_ context.Context, exchange string, tickers []string) (map[string]models.EODBar, error) {
			bulkCalled = true
			if exchange != "AU" {
				t.Errorf("expected exchange AU, got %s", exchange)
			}
			return map[string]models.EODBar{
				"BHP.AU": {Date: now, Close: 43.00, Volume: 5000000},
			}, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)

	err := svc.CollectCoreMarketData(context.Background(), []string{"BHP.AU"}, false)
	if err != nil {
		t.Fatalf("CollectCoreMarketData failed: %v", err)
	}

	if !bulkCalled {
		t.Error("expected GetBulkEOD to be called")
	}

	saved := storage.market.data["BHP.AU"]
	if saved == nil {
		t.Fatal("expected market data to be saved")
	}
	// Should have merged bars
	if len(saved.EOD) < 2 {
		t.Errorf("expected at least 2 EOD bars after merge, got %d", len(saved.EOD))
	}
}

func TestCollectCoreMarketData_SkipsFilingsAndNews(t *testing.T) {
	now := time.Now()
	eodCalled := false
	fundCalled := false

	storage := &mockStorageManager{
		market:  &mockMarketDataStorage{data: map[string]*models.MarketData{}},
		signals: &mockSignalStorage{},
	}

	eodhd := &mockEODHDClient{
		getBulkEODFn: func(_ context.Context, _ string, _ []string) (map[string]models.EODBar, error) {
			return nil, fmt.Errorf("no bulk data")
		},
		getEODFn: func(_ context.Context, ticker string, _ ...interfaces.EODOption) (*models.EODResponse, error) {
			eodCalled = true
			return &models.EODResponse{
				Data: []models.EODBar{{Date: now, Close: 10.0}},
			}, nil
		},
		getFundFn: func(_ context.Context, ticker string) (*models.Fundamentals, error) {
			fundCalled = true
			return &models.Fundamentals{Sector: "Materials"}, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)

	err := svc.CollectCoreMarketData(context.Background(), []string{"BHP.AU"}, false)
	if err != nil {
		t.Fatalf("CollectCoreMarketData failed: %v", err)
	}

	if !eodCalled {
		t.Error("expected GetEOD to be called for new ticker")
	}
	if !fundCalled {
		t.Error("expected GetFundamentals to be called for new ticker")
	}

	saved := storage.market.data["BHP.AU"]
	if saved == nil {
		t.Fatal("expected market data to be saved")
	}

	// Core path should NOT have fetched filings or news
	if saved.FilingsUpdatedAt != (time.Time{}) {
		t.Error("FilingsUpdatedAt should be zero — core path skips filings")
	}
	if saved.NewsUpdatedAt != (time.Time{}) {
		t.Error("NewsUpdatedAt should be zero — core path skips news")
	}
}

func TestCollectCoreMarketData_SkipsFreshData(t *testing.T) {
	now := time.Now()
	eodCalled := false
	fundCalled := false

	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {
					Ticker:                "BHP.AU",
					Exchange:              "AU",
					DataVersion:           common.SchemaVersion,
					EODUpdatedAt:          now, // fresh
					FundamentalsUpdatedAt: now, // fresh
					EOD:                   []models.EODBar{{Date: now, Close: 42.50}},
					Fundamentals:          &models.Fundamentals{ISIN: "AU000000BHP4"},
				},
			},
		},
		signals: &mockSignalStorage{},
	}

	eodhd := &mockEODHDClient{
		getBulkEODFn: func(_ context.Context, _ string, _ []string) (map[string]models.EODBar, error) {
			return nil, fmt.Errorf("no bulk data")
		},
		getEODFn: func(_ context.Context, _ string, _ ...interfaces.EODOption) (*models.EODResponse, error) {
			eodCalled = true
			return nil, fmt.Errorf("should not be called")
		},
		getFundFn: func(_ context.Context, _ string) (*models.Fundamentals, error) {
			fundCalled = true
			return nil, fmt.Errorf("should not be called")
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)

	err := svc.CollectCoreMarketData(context.Background(), []string{"BHP.AU"}, false)
	if err != nil {
		t.Fatalf("CollectCoreMarketData failed: %v", err)
	}

	if eodCalled {
		t.Error("GetEOD should not be called when data is fresh")
	}
	if fundCalled {
		t.Error("GetFundamentals should not be called when data is fresh")
	}
}

func TestCollectCoreMarketData_EmptyTickers(t *testing.T) {
	logger := common.NewLogger("error")
	svc := NewService(nil, nil, nil, logger)

	err := svc.CollectCoreMarketData(context.Background(), []string{}, false)
	if err != nil {
		t.Fatalf("CollectCoreMarketData with empty tickers should not fail: %v", err)
	}
}

func TestCollectCoreMarketData_MultipleTickers(t *testing.T) {
	now := time.Now()

	storage := &mockStorageManager{
		market:  &mockMarketDataStorage{data: map[string]*models.MarketData{}},
		signals: &mockSignalStorage{},
	}

	eodhd := &mockEODHDClient{
		getBulkEODFn: func(_ context.Context, _ string, _ []string) (map[string]models.EODBar, error) {
			return nil, fmt.Errorf("no bulk")
		},
		getEODFn: func(_ context.Context, ticker string, _ ...interfaces.EODOption) (*models.EODResponse, error) {
			return &models.EODResponse{
				Data: []models.EODBar{{Date: now, Close: 10.0}},
			}, nil
		},
		getFundFn: func(_ context.Context, ticker string) (*models.Fundamentals, error) {
			return &models.Fundamentals{Sector: "Test"}, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)

	err := svc.CollectCoreMarketData(context.Background(), []string{"BHP.AU", "RIO.AU", "WOW.AU"}, false)
	if err != nil {
		t.Fatalf("CollectCoreMarketData failed: %v", err)
	}

	// All three should be saved
	for _, ticker := range []string{"BHP.AU", "RIO.AU", "WOW.AU"} {
		if _, ok := storage.market.data[ticker]; !ok {
			t.Errorf("expected %s to be saved", ticker)
		}
	}
}
