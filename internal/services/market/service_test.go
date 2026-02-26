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
	files   *mockFileStore
}

func (m *mockStorageManager) MarketDataStorage() interfaces.MarketDataStorage { return m.market }
func (m *mockStorageManager) SignalStorage() interfaces.SignalStorage         { return m.signals }
func (m *mockStorageManager) InternalStore() interfaces.InternalStore         { return nil }
func (m *mockStorageManager) UserDataStore() interfaces.UserDataStore         { return nil }
func (m *mockStorageManager) StockIndexStore() interfaces.StockIndexStore     { return nil }
func (m *mockStorageManager) JobQueueStore() interfaces.JobQueueStore         { return nil }
func (m *mockStorageManager) FileStore() interfaces.FileStore {
	if m.files != nil {
		return m.files
	}
	return &mockFileStore{files: make(map[string][]byte)}
}
func (m *mockStorageManager) FeedbackStore() interfaces.FeedbackStore        { return nil }
func (m *mockStorageManager) OAuthStore() interfaces.OAuthStore              { return nil }
func (m *mockStorageManager) DataPath() string                               { return "" }
func (m *mockStorageManager) WriteRaw(subdir, key string, data []byte) error { return nil }
func (m *mockStorageManager) PurgeDerivedData(_ context.Context) (map[string]int, error) {
	return nil, nil
}
func (m *mockStorageManager) PurgeReports(_ context.Context) (int, error) { return 0, nil }
func (m *mockStorageManager) Close() error                                { return nil }

type mockFileStore struct {
	files map[string][]byte
}

func (m *mockFileStore) SaveFile(_ context.Context, category, key string, data []byte, _ string) error {
	m.files[category+"/"+key] = data
	return nil
}
func (m *mockFileStore) GetFile(_ context.Context, category, key string) ([]byte, string, error) {
	if d, ok := m.files[category+"/"+key]; ok {
		return d, "application/octet-stream", nil
	}
	return nil, "", fmt.Errorf("not found")
}
func (m *mockFileStore) DeleteFile(_ context.Context, category, key string) error {
	delete(m.files, category+"/"+key)
	return nil
}
func (m *mockFileStore) HasFile(_ context.Context, category, key string) (bool, error) {
	_, ok := m.files[category+"/"+key]
	return ok, nil
}

// --- tests ---

func TestGetStockData_UsesRealTimePrice(t *testing.T) {
	today := time.Now()
	eodClose := 42.50
	livePrice := 43.25

	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {
					Ticker:                "BHP.AU",
					Exchange:              "AU",
					LastUpdated:           today,
					FilingsIndexUpdatedAt: today,
					Filings:               []models.CompanyFiling{{Date: today, Headline: "Test"}},
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
					Ticker:                "BHP.AU",
					Exchange:              "AU",
					LastUpdated:           today,
					FilingsIndexUpdatedAt: today,
					Filings:               []models.CompanyFiling{{Date: today, Headline: "Test"}},
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
					Ticker:                "BHP.AU",
					Exchange:              "AU",
					LastUpdated:           today,
					FilingsIndexUpdatedAt: today,
					Filings:               []models.CompanyFiling{{Date: today, Headline: "Test"}},
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
					Ticker:                "BHP.AU",
					LastUpdated:           today,
					FilingsIndexUpdatedAt: today,
					Filings:               []models.CompanyFiling{{Date: today, Headline: "Test"}},
					EOD:                   []models.EODBar{{Date: today, Close: 42.50}},
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
					Ticker:                "BHP.AU",
					Exchange:              "AU",
					LastUpdated:           today,
					FilingsIndexUpdatedAt: today,
					Filings:               []models.CompanyFiling{{Date: today, Headline: "Test"}},
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
					FilingsIndexUpdatedAt:    now,
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
					FilingsIndexUpdatedAt:    now,
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
					FilingsIndexUpdatedAt:    now,
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
					FilingsIndexUpdatedAt: now,
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
					Ticker:                "SKS.AU",
					Exchange:              "AU",
					LastUpdated:           today,
					FilingsIndexUpdatedAt: today,
					Filings:               []models.CompanyFiling{{Date: today, Headline: "Test"}},
					EOD:                   []models.EODBar{{Date: today, Close: 2.50}},
					FilingSummaries:       summaries,
					CompanyTimeline:       timeline,
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

func TestCollectCoreMarketData_CollectsFilingsIndex(t *testing.T) {
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

	// Core path SHOULD have fetched filing index (fast path)
	if saved.FilingsIndexUpdatedAt == (time.Time{}) {
		t.Error("FilingsIndexUpdatedAt should be set — core path collects filing index")
	}
	// Core path should NOT have fetched news (still slow)
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

// --- CollectBulkEOD tests ---

func TestCollectBulkEOD_MergesBulkBar(t *testing.T) {
	now := time.Now()
	yesterday := now.AddDate(0, 0, -1)
	bulkCalled := false

	index := newBulkTestStockIndex(
		&models.StockIndexEntry{Ticker: "BHP.AU", Code: "BHP", Exchange: "AU"},
		&models.StockIndexEntry{Ticker: "RIO.AU", Code: "RIO", Exchange: "AU"},
	)

	storage := &bulkTestStorage{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {
					Ticker:       "BHP.AU",
					Exchange:     "AU",
					DataVersion:  common.SchemaVersion,
					EODUpdatedAt: yesterday, // stale
					EOD:          []models.EODBar{{Date: yesterday, Close: 42.50}},
				},
				"RIO.AU": {
					Ticker:       "RIO.AU",
					Exchange:     "AU",
					DataVersion:  common.SchemaVersion,
					EODUpdatedAt: yesterday,
					EOD:          []models.EODBar{{Date: yesterday, Close: 110.00}},
				},
			},
		},
		signals: &mockSignalStorage{},
		index:   index,
	}

	eodhd := &mockEODHDClient{
		getBulkEODFn: func(_ context.Context, exchange string, tickers []string) (map[string]models.EODBar, error) {
			bulkCalled = true
			if exchange != "AU" {
				t.Errorf("expected exchange AU, got %s", exchange)
			}
			return map[string]models.EODBar{
				"BHP.AU": {Date: now, Close: 43.00, Volume: 5000000},
				"RIO.AU": {Date: now, Close: 111.50, Volume: 3000000},
			}, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)

	err := svc.CollectBulkEOD(context.Background(), "AU", false)
	if err != nil {
		t.Fatalf("CollectBulkEOD failed: %v", err)
	}

	if !bulkCalled {
		t.Error("expected GetBulkEOD to be called")
	}

	// Both tickers should have merged bars
	for _, ticker := range []string{"BHP.AU", "RIO.AU"} {
		saved := storage.market.data[ticker]
		if saved == nil {
			t.Fatalf("expected %s to be saved", ticker)
		}
		if len(saved.EOD) < 2 {
			t.Errorf("%s: expected at least 2 EOD bars after merge, got %d", ticker, len(saved.EOD))
		}
	}

	// Stock index timestamps should be updated
	if _, ok := index.updates["BHP.AU:eod_collected_at"]; !ok {
		t.Error("expected stock index timestamp update for BHP.AU")
	}
	if _, ok := index.updates["RIO.AU:eod_collected_at"]; !ok {
		t.Error("expected stock index timestamp update for RIO.AU")
	}
}

func TestCollectBulkEOD_FallsBackForNewTickers(t *testing.T) {
	now := time.Now()
	eodCalled := false

	index := newBulkTestStockIndex(
		&models.StockIndexEntry{Ticker: "NEW.AU", Code: "NEW", Exchange: "AU"},
	)

	storage := &bulkTestStorage{
		market:  &mockMarketDataStorage{data: map[string]*models.MarketData{}}, // no existing data
		signals: &mockSignalStorage{},
		index:   index,
	}

	eodhd := &mockEODHDClient{
		getBulkEODFn: func(_ context.Context, _ string, _ []string) (map[string]models.EODBar, error) {
			return map[string]models.EODBar{
				"NEW.AU": {Date: now, Close: 10.00},
			}, nil
		},
		getEODFn: func(_ context.Context, ticker string, _ ...interfaces.EODOption) (*models.EODResponse, error) {
			eodCalled = true
			return &models.EODResponse{
				Data: []models.EODBar{
					{Date: now, Close: 10.00},
					{Date: now.AddDate(0, 0, -1), Close: 9.50},
				},
			}, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)

	err := svc.CollectBulkEOD(context.Background(), "AU", false)
	if err != nil {
		t.Fatalf("CollectBulkEOD failed: %v", err)
	}

	if !eodCalled {
		t.Error("expected individual GetEOD fallback for ticker with no existing data")
	}

	saved := storage.market.data["NEW.AU"]
	if saved == nil {
		t.Fatal("expected NEW.AU to be saved")
	}
	if len(saved.EOD) < 2 {
		t.Errorf("expected at least 2 EOD bars from individual fetch, got %d", len(saved.EOD))
	}
}

func TestCollectBulkEOD_SkipsFreshData(t *testing.T) {
	now := time.Now()

	index := newBulkTestStockIndex(
		&models.StockIndexEntry{Ticker: "BHP.AU", Code: "BHP", Exchange: "AU"},
	)

	storage := &bulkTestStorage{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {
					Ticker:       "BHP.AU",
					Exchange:     "AU",
					DataVersion:  common.SchemaVersion,
					EODUpdatedAt: now, // fresh
					EOD:          []models.EODBar{{Date: now, Close: 42.50}},
				},
			},
		},
		signals: &mockSignalStorage{},
		index:   index,
	}

	bulkCalled := false
	eodhd := &mockEODHDClient{
		getBulkEODFn: func(_ context.Context, _ string, _ []string) (map[string]models.EODBar, error) {
			bulkCalled = true
			return map[string]models.EODBar{
				"BHP.AU": {Date: now, Close: 43.00},
			}, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)

	err := svc.CollectBulkEOD(context.Background(), "AU", false)
	if err != nil {
		t.Fatalf("CollectBulkEOD failed: %v", err)
	}

	if !bulkCalled {
		t.Error("expected GetBulkEOD to still be called (fetches for all tickers)")
	}

	// Data should not have been updated since it was fresh
	saved := storage.market.data["BHP.AU"]
	if len(saved.EOD) != 1 {
		t.Errorf("expected 1 EOD bar (unchanged), got %d", len(saved.EOD))
	}
}

func TestCollectBulkEOD_FiltersExchange(t *testing.T) {
	index := newBulkTestStockIndex(
		&models.StockIndexEntry{Ticker: "BHP.AU", Code: "BHP", Exchange: "AU"},
		&models.StockIndexEntry{Ticker: "AAPL.US", Code: "AAPL", Exchange: "US"},
	)

	storage := &bulkTestStorage{
		market:  &mockMarketDataStorage{data: map[string]*models.MarketData{}},
		signals: &mockSignalStorage{},
		index:   index,
	}

	var requestedTickers []string
	eodhd := &mockEODHDClient{
		getBulkEODFn: func(_ context.Context, exchange string, tickers []string) (map[string]models.EODBar, error) {
			if exchange != "US" {
				t.Errorf("expected exchange US, got %s", exchange)
			}
			requestedTickers = tickers
			return nil, nil
		},
		getEODFn: func(_ context.Context, ticker string, _ ...interfaces.EODOption) (*models.EODResponse, error) {
			return &models.EODResponse{Data: []models.EODBar{{Date: time.Now(), Close: 100.00}}}, nil
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, eodhd, nil, logger)

	err := svc.CollectBulkEOD(context.Background(), "US", false)
	if err != nil {
		t.Fatalf("CollectBulkEOD failed: %v", err)
	}

	// Should only request US tickers
	if len(requestedTickers) != 1 || requestedTickers[0] != "AAPL.US" {
		t.Errorf("expected [AAPL.US], got %v", requestedTickers)
	}
}

func TestCollectBulkEOD_NoEODHDClient(t *testing.T) {
	logger := common.NewLogger("error")
	index := newBulkTestStockIndex()
	storage := &bulkTestStorage{
		market:  &mockMarketDataStorage{data: map[string]*models.MarketData{}},
		signals: &mockSignalStorage{},
		index:   index,
	}
	svc := NewService(storage, nil, nil, logger)

	err := svc.CollectBulkEOD(context.Background(), "AU", false)
	if err == nil {
		t.Fatal("expected error when EODHD client is nil")
	}
}

// --- Historical Fields Tests ---

func TestGetStockData_HistoricalFields(t *testing.T) {
	now := time.Now()

	// Create EOD data with 7 bars for testing historical fields
	eod := []models.EODBar{
		{Date: now, Close: 50.00},                   // today (EOD[0])
		{Date: now.AddDate(0, 0, -1), Close: 48.00}, // yesterday (EOD[1])
		{Date: now.AddDate(0, 0, -2), Close: 47.00}, // 2 days ago (EOD[2])
		{Date: now.AddDate(0, 0, -3), Close: 46.00}, // 3 days ago
		{Date: now.AddDate(0, 0, -4), Close: 45.00}, // 4 days ago
		{Date: now.AddDate(0, 0, -5), Close: 44.00}, // 5 days ago (last week = EOD[5])
		{Date: now.AddDate(0, 0, -6), Close: 43.00}, // 6 days ago
	}

	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {
					Ticker:                "BHP.AU",
					Exchange:              "AU",
					EOD:                   eod,
					LastUpdated:           now,
					Filings:               []models.CompanyFiling{{Date: now, Headline: "Test"}},
					FilingsIndexUpdatedAt: now,
					FundamentalsUpdatedAt: now,
				},
			},
		},
		signals: &mockSignalStorage{},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, logger)

	data, err := svc.GetStockData(context.Background(), "BHP.AU", interfaces.StockDataInclude{Price: true})
	if err != nil {
		t.Fatalf("GetStockData failed: %v", err)
	}

	if data.Price == nil {
		t.Fatal("Price data is nil")
	}

	// Yesterday close should be EOD[1] = 48.00
	if !approxEqual(data.Price.YesterdayClose, 48.00, 0.01) {
		t.Errorf("YesterdayClose = %.2f, want 48.00", data.Price.YesterdayClose)
	}

	// Yesterday % = (50 - 48) / 48 * 100 = 4.166...
	expectedYesterdayPct := (50.00 - 48.00) / 48.00 * 100
	if !approxEqual(data.Price.YesterdayPct, expectedYesterdayPct, 0.01) {
		t.Errorf("YesterdayPct = %.2f, want %.2f", data.Price.YesterdayPct, expectedYesterdayPct)
	}

	// Last week close should be EOD[5] = 44.00
	if !approxEqual(data.Price.LastWeekClose, 44.00, 0.01) {
		t.Errorf("LastWeekClose = %.2f, want 44.00", data.Price.LastWeekClose)
	}

	// Last week % = (50 - 44) / 44 * 100 = 13.636...
	expectedLastWeekPct := (50.00 - 44.00) / 44.00 * 100
	if !approxEqual(data.Price.LastWeekPct, expectedLastWeekPct, 0.01) {
		t.Errorf("LastWeekPct = %.2f, want %.2f", data.Price.LastWeekPct, expectedLastWeekPct)
	}
}

func TestGetStockData_HistoricalFields_InsufficientEOD(t *testing.T) {
	now := time.Now()

	// Only 2 EOD bars - yesterday should work, last week should not
	eod := []models.EODBar{
		{Date: now, Close: 50.00},
		{Date: now.AddDate(0, 0, -1), Close: 48.00},
	}

	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {
					Ticker:                "BHP.AU",
					Exchange:              "AU",
					EOD:                   eod,
					LastUpdated:           now,
					Filings:               []models.CompanyFiling{{Date: now, Headline: "Test"}},
					FilingsIndexUpdatedAt: now,
					FundamentalsUpdatedAt: now,
				},
			},
		},
		signals: &mockSignalStorage{},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, logger)

	data, err := svc.GetStockData(context.Background(), "BHP.AU", interfaces.StockDataInclude{Price: true})
	if err != nil {
		t.Fatalf("GetStockData failed: %v", err)
	}

	// Yesterday should be populated
	if !approxEqual(data.Price.YesterdayClose, 48.00, 0.01) {
		t.Errorf("YesterdayClose = %.2f, want 48.00", data.Price.YesterdayClose)
	}

	// Last week should NOT be populated (need >5 bars)
	if data.Price.LastWeekClose != 0 {
		t.Errorf("LastWeekClose = %.2f, want 0 (insufficient data)", data.Price.LastWeekClose)
	}
	if data.Price.LastWeekPct != 0 {
		t.Errorf("LastWeekPct = %.2f, want 0 (insufficient data)", data.Price.LastWeekPct)
	}
}

func TestGetStockData_HistoricalFields_ZeroClose(t *testing.T) {
	now := time.Now()

	// EOD with zero close price
	eod := []models.EODBar{
		{Date: now, Close: 50.00},
		{Date: now.AddDate(0, 0, -1), Close: 0}, // zero close
		{Date: now.AddDate(0, 0, -2), Close: 47.00},
		{Date: now.AddDate(0, 0, -3), Close: 46.00},
		{Date: now.AddDate(0, 0, -4), Close: 45.00},
		{Date: now.AddDate(0, 0, -5), Close: 0}, // zero close
	}

	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {
					Ticker:                "BHP.AU",
					Exchange:              "AU",
					EOD:                   eod,
					LastUpdated:           now,
					Filings:               []models.CompanyFiling{{Date: now, Headline: "Test"}},
					FilingsIndexUpdatedAt: now,
					FundamentalsUpdatedAt: now,
				},
			},
		},
		signals: &mockSignalStorage{},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, logger)

	data, err := svc.GetStockData(context.Background(), "BHP.AU", interfaces.StockDataInclude{Price: true})
	if err != nil {
		t.Fatalf("GetStockData failed: %v", err)
	}

	// YesterdayClose should be 0 (from EOD[1])
	if data.Price.YesterdayClose != 0 {
		t.Errorf("YesterdayClose = %.2f, want 0", data.Price.YesterdayClose)
	}

	// YesterdayPct should NOT be calculated when close is 0
	if data.Price.YesterdayPct != 0 {
		t.Errorf("YesterdayPct = %.2f, want 0 (division by zero guard)", data.Price.YesterdayPct)
	}

	// Last week close should be 0 (from EOD[5])
	if data.Price.LastWeekClose != 0 {
		t.Errorf("LastWeekClose = %.2f, want 0", data.Price.LastWeekClose)
	}

	// Last week % should NOT be calculated when close is 0
	if data.Price.LastWeekPct != 0 {
		t.Errorf("LastWeekPct = %.2f, want 0 (division by zero guard)", data.Price.LastWeekPct)
	}
}
