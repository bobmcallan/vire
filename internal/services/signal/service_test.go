package signal

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

// --- Mock EODHD Client ---

type mockEODHDClient struct {
	quote    *models.RealTimeQuote
	quoteErr error
}

func (m *mockEODHDClient) GetRealTimeQuote(_ context.Context, _ string) (*models.RealTimeQuote, error) {
	return m.quote, m.quoteErr
}

func (m *mockEODHDClient) GetEOD(_ context.Context, _ string, _ ...interfaces.EODOption) (*models.EODResponse, error) {
	return nil, nil
}

func (m *mockEODHDClient) GetBulkEOD(_ context.Context, _ string, _ []string) (map[string]models.EODBar, error) {
	return nil, nil
}

func (m *mockEODHDClient) GetFundamentals(_ context.Context, _ string) (*models.Fundamentals, error) {
	return nil, nil
}

func (m *mockEODHDClient) GetTechnicals(_ context.Context, _ string, _ string) (*models.TechnicalResponse, error) {
	return nil, nil
}

func (m *mockEODHDClient) GetNews(_ context.Context, _ string, _ int) ([]*models.NewsItem, error) {
	return nil, nil
}

func (m *mockEODHDClient) GetExchangeSymbols(_ context.Context, _ string) ([]*models.Symbol, error) {
	return nil, nil
}

func (m *mockEODHDClient) ScreenStocks(_ context.Context, _ models.ScreenerOptions) ([]*models.ScreenerResult, error) {
	return nil, nil
}

// --- Mock Storage ---

type mockStorageManager struct {
	marketStorage *mockMarketDataStorage
	signalStorage *mockSignalStorage
}

func (m *mockStorageManager) InternalStore() interfaces.InternalStore         { return nil }
func (m *mockStorageManager) UserDataStore() interfaces.UserDataStore         { return nil }
func (m *mockStorageManager) MarketDataStorage() interfaces.MarketDataStorage { return m.marketStorage }
func (m *mockStorageManager) SignalStorage() interfaces.SignalStorage         { return m.signalStorage }
func (m *mockStorageManager) StockIndexStore() interfaces.StockIndexStore     { return nil }
func (m *mockStorageManager) JobQueueStore() interfaces.JobQueueStore         { return nil }
func (m *mockStorageManager) FileStore() interfaces.FileStore                 { return nil }
func (m *mockStorageManager) FeedbackStore() interfaces.FeedbackStore         { return nil }
func (m *mockStorageManager) DataPath() string                                { return "" }
func (m *mockStorageManager) WriteRaw(_, _ string, _ []byte) error            { return nil }
func (m *mockStorageManager) PurgeDerivedData(_ context.Context) (map[string]int, error) {
	return nil, nil
}
func (m *mockStorageManager) PurgeReports(_ context.Context) (int, error) { return 0, nil }
func (m *mockStorageManager) Close() error                                { return nil }

type mockMarketDataStorage struct {
	data map[string]*models.MarketData
}

func (m *mockMarketDataStorage) GetMarketData(_ context.Context, ticker string) (*models.MarketData, error) {
	d, ok := m.data[ticker]
	if !ok {
		return nil, errors.New("not found")
	}
	return d, nil
}

func (m *mockMarketDataStorage) SaveMarketData(_ context.Context, _ *models.MarketData) error {
	return nil
}

func (m *mockMarketDataStorage) GetMarketDataBatch(_ context.Context, _ []string) ([]*models.MarketData, error) {
	return nil, nil
}

func (m *mockMarketDataStorage) GetStaleTickers(_ context.Context, _ string, _ int64) ([]string, error) {
	return nil, nil
}

type mockSignalStorage struct {
	saved []*models.TickerSignals
}

func (m *mockSignalStorage) GetSignals(_ context.Context, _ string) (*models.TickerSignals, error) {
	return nil, errors.New("not found")
}

func (m *mockSignalStorage) SaveSignals(_ context.Context, signals *models.TickerSignals) error {
	m.saved = append(m.saved, signals)
	return nil
}

func (m *mockSignalStorage) GetSignalsBatch(_ context.Context, _ []string) ([]*models.TickerSignals, error) {
	return nil, nil
}

// --- Tests ---

func TestOverlayLiveQuote_SameDay(t *testing.T) {
	today := time.Now().Truncate(24 * time.Hour)

	md := &models.MarketData{
		Ticker: "BHP.AU",
		EOD: []models.EODBar{
			{Date: today, Open: 40.0, High: 42.0, Low: 39.0, Close: 41.0, Volume: 1000},
			{Date: today.AddDate(0, 0, -1), Open: 39.0, High: 41.0, Low: 38.0, Close: 40.0, Volume: 900},
		},
	}

	eodhd := &mockEODHDClient{
		quote: &models.RealTimeQuote{
			Close: 43.5,
			High:  44.0,
			Low:   39.5,
		},
	}

	logger := common.NewLogger("debug")
	svc := &Service{eodhd: eodhd, logger: logger}
	svc.overlayLiveQuote(context.Background(), "BHP.AU", md)

	// Close should be updated
	if md.EOD[0].Close != 43.5 {
		t.Errorf("Close = %v, want 43.5", md.EOD[0].Close)
	}
	// High should be updated (44.0 > 42.0)
	if md.EOD[0].High != 44.0 {
		t.Errorf("High = %v, want 44.0", md.EOD[0].High)
	}
	// Low should not change (39.5 > 39.0)
	if md.EOD[0].Low != 39.0 {
		t.Errorf("Low = %v, want 39.0 (unchanged)", md.EOD[0].Low)
	}
	// Bar count should be unchanged
	if len(md.EOD) != 2 {
		t.Errorf("bar count = %d, want 2", len(md.EOD))
	}
}

func TestOverlayLiveQuote_PreviousDay(t *testing.T) {
	yesterday := time.Now().Truncate(24*time.Hour).AddDate(0, 0, -1)

	md := &models.MarketData{
		Ticker: "BHP.AU",
		EOD: []models.EODBar{
			{Date: yesterday, Open: 40.0, High: 42.0, Low: 39.0, Close: 41.0, Volume: 1000},
		},
	}

	eodhd := &mockEODHDClient{
		quote: &models.RealTimeQuote{
			Open:          41.0,
			High:          43.0,
			Low:           40.5,
			Close:         42.5,
			PreviousClose: 41.0,
			Volume:        500,
		},
	}

	logger := common.NewLogger("debug")
	svc := &Service{eodhd: eodhd, logger: logger}
	svc.overlayLiveQuote(context.Background(), "BHP.AU", md)

	// Should have prepended a synthetic bar
	if len(md.EOD) != 2 {
		t.Fatalf("bar count = %d, want 2", len(md.EOD))
	}

	synthetic := md.EOD[0]
	if synthetic.Close != 42.5 {
		t.Errorf("synthetic Close = %v, want 42.5", synthetic.Close)
	}
	if synthetic.High != 43.0 {
		t.Errorf("synthetic High = %v, want 43.0", synthetic.High)
	}
	if synthetic.Low != 40.5 {
		t.Errorf("synthetic Low = %v, want 40.5", synthetic.Low)
	}
	if synthetic.Open != 41.0 {
		t.Errorf("synthetic Open = %v, want 41.0", synthetic.Open)
	}
	if synthetic.Volume != 500 {
		t.Errorf("synthetic Volume = %v, want 500", synthetic.Volume)
	}

	// Original bar should be untouched
	if md.EOD[1].Close != 41.0 {
		t.Errorf("original Close = %v, want 41.0", md.EOD[1].Close)
	}
}

func TestOverlayLiveQuote_NilEODHDClient(t *testing.T) {
	today := time.Now().Truncate(24 * time.Hour)

	md := &models.MarketData{
		Ticker: "BHP.AU",
		EOD: []models.EODBar{
			{Date: today, Close: 41.0},
		},
	}

	logger := common.NewLogger("debug")
	svc := &Service{eodhd: nil, logger: logger}
	svc.overlayLiveQuote(context.Background(), "BHP.AU", md)

	// Should be unchanged
	if md.EOD[0].Close != 41.0 {
		t.Errorf("Close = %v, want 41.0 (unchanged)", md.EOD[0].Close)
	}
}

func TestOverlayLiveQuote_QuoteFetchError(t *testing.T) {
	today := time.Now().Truncate(24 * time.Hour)

	md := &models.MarketData{
		Ticker: "BHP.AU",
		EOD: []models.EODBar{
			{Date: today, Close: 41.0},
		},
	}

	eodhd := &mockEODHDClient{
		quoteErr: errors.New("network error"),
	}

	logger := common.NewLogger("debug")
	svc := &Service{eodhd: eodhd, logger: logger}
	svc.overlayLiveQuote(context.Background(), "BHP.AU", md)

	// Should be unchanged
	if md.EOD[0].Close != 41.0 {
		t.Errorf("Close = %v, want 41.0 (unchanged)", md.EOD[0].Close)
	}
}

func TestOverlayLiveQuote_ZeroCloseQuote(t *testing.T) {
	today := time.Now().Truncate(24 * time.Hour)

	md := &models.MarketData{
		Ticker: "BHP.AU",
		EOD: []models.EODBar{
			{Date: today, Close: 41.0},
		},
	}

	eodhd := &mockEODHDClient{
		quote: &models.RealTimeQuote{Close: 0},
	}

	logger := common.NewLogger("debug")
	svc := &Service{eodhd: eodhd, logger: logger}
	svc.overlayLiveQuote(context.Background(), "BHP.AU", md)

	// Should be unchanged when quote Close is zero
	if md.EOD[0].Close != 41.0 {
		t.Errorf("Close = %v, want 41.0 (unchanged)", md.EOD[0].Close)
	}
}

func TestOverlayLiveQuote_EmptyBars(t *testing.T) {
	md := &models.MarketData{
		Ticker: "BHP.AU",
		EOD:    []models.EODBar{},
	}

	eodhd := &mockEODHDClient{
		quote: &models.RealTimeQuote{Close: 42.5},
	}

	logger := common.NewLogger("debug")
	svc := &Service{eodhd: eodhd, logger: logger}
	svc.overlayLiveQuote(context.Background(), "BHP.AU", md)

	// Should not panic or add bars
	if len(md.EOD) != 0 {
		t.Errorf("bar count = %d, want 0", len(md.EOD))
	}
}

func TestOverlayLiveQuote_PreviousDay_ZeroOpen(t *testing.T) {
	yesterday := time.Now().Truncate(24*time.Hour).AddDate(0, 0, -1)

	md := &models.MarketData{
		Ticker: "BHP.AU",
		EOD: []models.EODBar{
			{Date: yesterday, Close: 41.0},
		},
	}

	eodhd := &mockEODHDClient{
		quote: &models.RealTimeQuote{
			Open:          0, // zero open
			Close:         42.5,
			High:          43.0,
			Low:           40.5,
			PreviousClose: 41.0,
		},
	}

	logger := common.NewLogger("debug")
	svc := &Service{eodhd: eodhd, logger: logger}
	svc.overlayLiveQuote(context.Background(), "BHP.AU", md)

	if len(md.EOD) != 2 {
		t.Fatalf("bar count = %d, want 2", len(md.EOD))
	}

	// When Open is zero, should use PreviousClose
	if md.EOD[0].Open != 41.0 {
		t.Errorf("synthetic Open = %v, want 41.0 (from PreviousClose)", md.EOD[0].Open)
	}
}

func TestDetectSignals_WithLiveOverlay(t *testing.T) {
	today := time.Now().Truncate(24 * time.Hour)

	// Create market data with enough bars for signal computation
	bars := make([]models.EODBar, 50)
	for i := range bars {
		bars[i] = models.EODBar{
			Date:  today.AddDate(0, 0, -i),
			Open:  100.0 + float64(i),
			High:  105.0 + float64(i),
			Low:   95.0 + float64(i),
			Close: 100.0 + float64(i)*0.5,
		}
	}

	marketStorage := &mockMarketDataStorage{
		data: map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", EOD: bars},
		},
	}
	signalStorage := &mockSignalStorage{}
	storage := &mockStorageManager{
		marketStorage: marketStorage,
		signalStorage: signalStorage,
	}
	eodhd := &mockEODHDClient{
		quote: &models.RealTimeQuote{
			Close: 120.0,
			High:  125.0,
			Low:   99.0,
		},
	}

	logger := common.NewLogger("debug")
	svc := NewService(storage, eodhd, logger)

	results, err := svc.DetectSignals(context.Background(), []string{"BHP.AU"}, nil, true)
	if err != nil {
		t.Fatalf("DetectSignals error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// Verify the live price was used (Close should reflect 120.0, not original 100.0)
	if results[0].Price.Current != 120.0 {
		t.Errorf("signal Price.Close = %v, want 120.0 (from live quote)", results[0].Price.Current)
	}
}

func TestDetectSignals_NilEODHDClient_StillWorks(t *testing.T) {
	today := time.Now().Truncate(24 * time.Hour)

	bars := make([]models.EODBar, 50)
	for i := range bars {
		bars[i] = models.EODBar{
			Date:  today.AddDate(0, 0, -i),
			Open:  100.0 + float64(i),
			High:  105.0 + float64(i),
			Low:   95.0 + float64(i),
			Close: 100.0,
		}
	}

	marketStorage := &mockMarketDataStorage{
		data: map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", EOD: bars},
		},
	}
	signalStorage := &mockSignalStorage{}
	storage := &mockStorageManager{
		marketStorage: marketStorage,
		signalStorage: signalStorage,
	}

	logger := common.NewLogger("debug")
	svc := NewService(storage, nil, logger) // nil EODHD client

	results, err := svc.DetectSignals(context.Background(), []string{"BHP.AU"}, nil, true)
	if err != nil {
		t.Fatalf("DetectSignals error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// Should use cached price
	if results[0].Price.Current != 100.0 {
		t.Errorf("signal Price.Close = %v, want 100.0 (cached)", results[0].Price.Current)
	}
}
