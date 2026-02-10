package market

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/bobmccarthy/vire/internal/common"
	"github.com/bobmccarthy/vire/internal/interfaces"
	"github.com/bobmccarthy/vire/internal/models"
)

func approxEqual(a, b, epsilon float64) bool {
	return math.Abs(a-b) < epsilon
}

// --- mock EODHD client ---

type mockEODHDClient struct {
	realTimeQuoteFn func(ctx context.Context, ticker string) (*models.RealTimeQuote, error)
}

func (m *mockEODHDClient) GetRealTimeQuote(ctx context.Context, ticker string) (*models.RealTimeQuote, error) {
	if m.realTimeQuoteFn != nil {
		return m.realTimeQuoteFn(ctx, ticker)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *mockEODHDClient) GetEOD(ctx context.Context, ticker string, opts ...interfaces.EODOption) (*models.EODResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *mockEODHDClient) GetFundamentals(ctx context.Context, ticker string) (*models.Fundamentals, error) {
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
	data map[string]*models.MarketData
}

func (m *mockMarketDataStorage) GetMarketData(_ context.Context, ticker string) (*models.MarketData, error) {
	md, ok := m.data[ticker]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return md, nil
}
func (m *mockMarketDataStorage) SaveMarketData(_ context.Context, data *models.MarketData) error {
	m.data[data.Ticker] = data
	return nil
}
func (m *mockMarketDataStorage) GetMarketDataBatch(_ context.Context, tickers []string) ([]*models.MarketData, error) {
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

func (m *mockStorageManager) MarketDataStorage() interfaces.MarketDataStorage       { return m.market }
func (m *mockStorageManager) SignalStorage() interfaces.SignalStorage               { return m.signals }
func (m *mockStorageManager) PortfolioStorage() interfaces.PortfolioStorage         { return nil }
func (m *mockStorageManager) KeyValueStorage() interfaces.KeyValueStorage           { return nil }
func (m *mockStorageManager) ReportStorage() interfaces.ReportStorage               { return nil }
func (m *mockStorageManager) StrategyStorage() interfaces.StrategyStorage           { return nil }
func (m *mockStorageManager) PlanStorage() interfaces.PlanStorage                   { return nil }
func (m *mockStorageManager) SearchHistoryStorage() interfaces.SearchHistoryStorage { return nil }
func (m *mockStorageManager) WatchlistStorage() interfaces.WatchlistStorage         { return nil }
func (m *mockStorageManager) DataPath() string                                      { return "" }
func (m *mockStorageManager) WriteRaw(subdir, key string, data []byte) error        { return nil }
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
