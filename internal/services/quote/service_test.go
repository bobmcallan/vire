package quote

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

// --- Mocks ---

type mockEODHDClient struct {
	quote *models.RealTimeQuote
	err   error
}

func (m *mockEODHDClient) GetRealTimeQuote(_ context.Context, _ string) (*models.RealTimeQuote, error) {
	return m.quote, m.err
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

type mockASXClient struct {
	quote  *models.RealTimeQuote
	err    error
	called bool
}

func (m *mockASXClient) GetRealTimeQuote(_ context.Context, _ string) (*models.RealTimeQuote, error) {
	m.called = true
	return m.quote, m.err
}

func newTestService(eodhd *mockEODHDClient, asx *mockASXClient, now func() time.Time) *Service {
	var asxClient interfaces.ASXClient
	if asx != nil {
		asxClient = asx
	}
	svc := NewService(eodhd, asxClient, nil, common.NewSilentLogger())
	svc.now = now
	return svc
}

// duringMarketHours returns a time that is within ASX market hours (Wed 12:00 Sydney time).
// February is AEDT (UTC+11), so 12:00 AEDT = 01:00 UTC.
func duringMarketHours() time.Time {
	return time.Date(2026, 2, 11, 12, 0, 0, 0, sydneyLocation) // Wed 12:00 AEDT
}

// outsideMarketHours returns a time that is outside ASX market hours (Wed 20:00 Sydney time).
func outsideMarketHours() time.Time {
	return time.Date(2026, 2, 11, 20, 0, 0, 0, sydneyLocation) // Wed 20:00 AEDT
}

// weekend returns a Saturday midday in Sydney time.
func weekend() time.Time {
	return time.Date(2026, 2, 14, 12, 0, 0, 0, sydneyLocation) // Sat 12:00 AEDT
}

// --- Tests ---

func TestFreshEODHD_NoFallback(t *testing.T) {
	now := duringMarketHours()
	eodhd := &mockEODHDClient{
		quote: &models.RealTimeQuote{
			Code:      "BHP.AU",
			Close:     45.00,
			Timestamp: now.Add(-30 * time.Minute), // 30 min old = fresh (under 2-hour threshold)
		},
	}
	asx := &mockASXClient{}

	svc := newTestService(eodhd, asx, func() time.Time { return now })
	quote, err := svc.GetRealTimeQuote(context.Background(), "BHP.AU")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if quote.Source != "eodhd" {
		t.Errorf("expected source eodhd, got %s", quote.Source)
	}
	if quote.Close != 45.00 {
		t.Errorf("expected close 45.00, got %.2f", quote.Close)
	}
	if asx.called {
		t.Error("ASX client should not have been called for fresh EODHD data")
	}
}

func TestStaleEODHD_ASXFallbackSuccess(t *testing.T) {
	now := duringMarketHours()
	eodhd := &mockEODHDClient{
		quote: &models.RealTimeQuote{
			Code:      "ETPMAG.AU",
			Close:     100.00,
			Timestamp: now.Add(-3 * time.Hour), // 3 hours old = stale (over 2-hour threshold)
		},
	}
	asx := &mockASXClient{
		quote: &models.RealTimeQuote{
			Code:      "ETPMAG.AU",
			Close:     102.50,
			Timestamp: now,
			Source:    "asx",
		},
	}

	svc := newTestService(eodhd, asx, func() time.Time { return now })
	quote, err := svc.GetRealTimeQuote(context.Background(), "ETPMAG.AU")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !asx.called {
		t.Fatal("ASX client should have been called for stale EODHD data")
	}
	if quote.Source != "asx" {
		t.Errorf("expected source asx, got %s", quote.Source)
	}
	if quote.Close != 102.50 {
		t.Errorf("expected close 102.50 from ASX, got %.2f", quote.Close)
	}
}

func TestStaleEODHD_ASXFallbackFails_ReturnStale(t *testing.T) {
	now := duringMarketHours()
	eodhd := &mockEODHDClient{
		quote: &models.RealTimeQuote{
			Code:      "PMGOLD.AU",
			Close:     25.00,
			Timestamp: now.Add(-3 * time.Hour), // stale (over 2-hour threshold)
		},
	}
	asx := &mockASXClient{
		err: errors.New("ASX API unavailable"),
	}

	svc := newTestService(eodhd, asx, func() time.Time { return now })
	quote, err := svc.GetRealTimeQuote(context.Background(), "PMGOLD.AU")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !asx.called {
		t.Fatal("ASX client should have been called")
	}
	if quote.Source != "eodhd" {
		t.Errorf("expected source eodhd (stale fallback), got %s", quote.Source)
	}
	if quote.Close != 25.00 {
		t.Errorf("expected stale close 25.00, got %.2f", quote.Close)
	}
}

func TestEODHDFails_ASXFallbackSuccess(t *testing.T) {
	now := duringMarketHours()
	eodhd := &mockEODHDClient{
		err: errors.New("EODHD API error"),
	}
	asx := &mockASXClient{
		quote: &models.RealTimeQuote{
			Code:      "BHP.AU",
			Close:     46.00,
			Timestamp: now,
			Source:    "asx",
		},
	}

	svc := newTestService(eodhd, asx, func() time.Time { return now })
	quote, err := svc.GetRealTimeQuote(context.Background(), "BHP.AU")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !asx.called {
		t.Fatal("ASX client should have been called when EODHD fails")
	}
	if quote.Source != "asx" {
		t.Errorf("expected source asx, got %s", quote.Source)
	}
}

func TestEODHDFails_ASXFails_ReturnsError(t *testing.T) {
	now := duringMarketHours()
	eodhd := &mockEODHDClient{
		err: errors.New("EODHD API error"),
	}
	asx := &mockASXClient{
		err: errors.New("ASX API error"),
	}

	svc := newTestService(eodhd, asx, func() time.Time { return now })
	_, err := svc.GetRealTimeQuote(context.Background(), "BHP.AU")
	if err == nil {
		t.Fatal("expected error when both sources fail")
	}
}

func TestOutsideMarketHours_NoFallback(t *testing.T) {
	now := outsideMarketHours()
	eodhd := &mockEODHDClient{
		quote: &models.RealTimeQuote{
			Code:      "ETPMAG.AU",
			Close:     100.00,
			Timestamp: now.Add(-3 * time.Hour), // stale (over 2-hour threshold)
		},
	}
	asx := &mockASXClient{}

	svc := newTestService(eodhd, asx, func() time.Time { return now })
	quote, err := svc.GetRealTimeQuote(context.Background(), "ETPMAG.AU")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if asx.called {
		t.Error("ASX client should not be called outside market hours")
	}
	if quote.Source != "eodhd" {
		t.Errorf("expected source eodhd, got %s", quote.Source)
	}
}

func TestWeekend_NoFallback(t *testing.T) {
	now := weekend()
	eodhd := &mockEODHDClient{
		quote: &models.RealTimeQuote{
			Code:      "BHP.AU",
			Close:     44.00,
			Timestamp: now.Add(-24 * time.Hour),
		},
	}
	asx := &mockASXClient{}

	svc := newTestService(eodhd, asx, func() time.Time { return now })
	quote, err := svc.GetRealTimeQuote(context.Background(), "BHP.AU")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if asx.called {
		t.Error("ASX client should not be called on weekends")
	}
	if quote.Close != 44.00 {
		t.Errorf("expected close 44.00, got %.2f", quote.Close)
	}
}

func TestNonASXTicker_NoFallback(t *testing.T) {
	now := duringMarketHours()
	eodhd := &mockEODHDClient{
		quote: &models.RealTimeQuote{
			Code:      "AAPL.US",
			Close:     175.00,
			Timestamp: now.Add(-3 * time.Hour), // stale, but not .AU
		},
	}
	asx := &mockASXClient{}

	svc := newTestService(eodhd, asx, func() time.Time { return now })
	quote, err := svc.GetRealTimeQuote(context.Background(), "AAPL.US")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if asx.called {
		t.Error("ASX client should not be called for non-AU tickers")
	}
	if quote.Source != "eodhd" {
		t.Errorf("expected source eodhd, got %s", quote.Source)
	}
}

func TestForexTicker_NoFallback(t *testing.T) {
	now := duringMarketHours()
	eodhd := &mockEODHDClient{
		quote: &models.RealTimeQuote{
			Code:      "AUDUSD.FOREX",
			Close:     0.65,
			Timestamp: now.Add(-3 * time.Hour), // stale, but FOREX
		},
	}
	asx := &mockASXClient{}

	svc := newTestService(eodhd, asx, func() time.Time { return now })
	quote, err := svc.GetRealTimeQuote(context.Background(), "AUDUSD.FOREX")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if asx.called {
		t.Error("ASX client should not be called for FOREX tickers")
	}
	if quote.Close != 0.65 {
		t.Errorf("expected close 0.65, got %.2f", quote.Close)
	}
}

func TestNilASXClient_NoFallback(t *testing.T) {
	now := duringMarketHours()
	eodhd := &mockEODHDClient{
		quote: &models.RealTimeQuote{
			Code:      "BHP.AU",
			Close:     45.00,
			Timestamp: now.Add(-3 * time.Hour), // stale (over 2-hour threshold)
		},
	}

	svc := newTestService(eodhd, nil, func() time.Time { return now })
	quote, err := svc.GetRealTimeQuote(context.Background(), "BHP.AU")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if quote.Source != "eodhd" {
		t.Errorf("expected source eodhd with nil ASX client, got %s", quote.Source)
	}
}

func TestZeroTimestamp_TreatedAsStale(t *testing.T) {
	now := duringMarketHours()
	eodhd := &mockEODHDClient{
		quote: &models.RealTimeQuote{
			Code:      "BHP.AU",
			Close:     45.00,
			Timestamp: time.Time{}, // zero = stale
		},
	}
	asx := &mockASXClient{
		quote: &models.RealTimeQuote{
			Code:   "BHP.AU",
			Close:  46.00,
			Source: "asx",
		},
	}

	svc := newTestService(eodhd, asx, func() time.Time { return now })
	quote, err := svc.GetRealTimeQuote(context.Background(), "BHP.AU")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !asx.called {
		t.Fatal("ASX client should have been called for zero timestamp")
	}
	if quote.Source != "asx" {
		t.Errorf("expected source asx, got %s", quote.Source)
	}
}

// --- isASXMarketHours unit tests ---

func TestIsASXMarketHours(t *testing.T) {
	tests := []struct {
		name     string
		time     time.Time
		expected bool
	}{
		{
			"before open",
			time.Date(2026, 2, 11, 9, 59, 0, 0, sydneyLocation), // Wed 09:59 Sydney
			false,
		},
		{
			"at open",
			time.Date(2026, 2, 11, 10, 0, 0, 0, sydneyLocation), // Wed 10:00 Sydney
			true,
		},
		{
			"midday",
			time.Date(2026, 2, 11, 12, 0, 0, 0, sydneyLocation), // Wed 12:00 Sydney
			true,
		},
		{
			"at close",
			time.Date(2026, 2, 11, 16, 30, 0, 0, sydneyLocation), // Wed 16:30 Sydney
			true,
		},
		{
			"after close",
			time.Date(2026, 2, 11, 16, 31, 0, 0, sydneyLocation), // Wed 16:31 Sydney
			false,
		},
		{
			"saturday midday",
			time.Date(2026, 2, 14, 12, 0, 0, 0, sydneyLocation), // Sat 12:00 Sydney
			false,
		},
		{
			"sunday midday",
			time.Date(2026, 2, 15, 12, 0, 0, 0, sydneyLocation), // Sun 12:00 Sydney
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isASXMarketHours(tt.time)
			if result != tt.expected {
				t.Errorf("isASXMarketHours(%v) = %v, want %v", tt.time, result, tt.expected)
			}
		})
	}
}

func TestIsASXTicker(t *testing.T) {
	tests := []struct {
		ticker   string
		expected bool
	}{
		{"BHP.AU", true},
		{"ETPMAG.AU", true},
		{"etpmag.au", true},
		{"AAPL.US", false},
		{"AUDUSD.FOREX", false},
		{"BHP", false},
	}

	for _, tt := range tests {
		t.Run(tt.ticker, func(t *testing.T) {
			if got := isASXTicker(tt.ticker); got != tt.expected {
				t.Errorf("isASXTicker(%s) = %v, want %v", tt.ticker, got, tt.expected)
			}
		})
	}
}

// --- Historical Fields Tests ---

type mockMarketDataStorage struct {
	data map[string]*models.MarketData
}

func (m *mockMarketDataStorage) GetMarketData(_ context.Context, ticker string) (*models.MarketData, error) {
	if md, ok := m.data[ticker]; ok {
		return md, nil
	}
	return nil, errors.New("not found")
}
func (m *mockMarketDataStorage) SaveMarketData(_ context.Context, _ *models.MarketData) error {
	return nil
}
func (m *mockMarketDataStorage) GetMarketDataBatch(_ context.Context, tickers []string) ([]*models.MarketData, error) {
	return nil, nil
}
func (m *mockMarketDataStorage) GetStaleTickers(_ context.Context, _ string, _ int64) ([]string, error) {
	return nil, nil
}

type mockStorageManager struct {
	market *mockMarketDataStorage
}

func (m *mockStorageManager) MarketDataStorage() interfaces.MarketDataStorage { return m.market }
func (m *mockStorageManager) SignalStorage() interfaces.SignalStorage         { return nil }
func (m *mockStorageManager) InternalStore() interfaces.InternalStore         { return nil }
func (m *mockStorageManager) UserDataStore() interfaces.UserDataStore         { return nil }
func (m *mockStorageManager) StockIndexStore() interfaces.StockIndexStore     { return nil }
func (m *mockStorageManager) JobQueueStore() interfaces.JobQueueStore         { return nil }
func (m *mockStorageManager) FeedbackStore() interfaces.FeedbackStore         { return nil }
func (m *mockStorageManager) OAuthStore() interfaces.OAuthStore               { return nil }
func (m *mockStorageManager) FileStore() interfaces.FileStore                 { return nil }
func (m *mockStorageManager) Close() error                                    { return nil }
func (m *mockStorageManager) DataPath() string                                { return "" }
func (m *mockStorageManager) WriteRaw(_, _ string, _ []byte) error            { return nil }
func (m *mockStorageManager) PurgeDerivedData(_ context.Context) (map[string]int, error) {
	return nil, nil
}
func (m *mockStorageManager) PurgeReports(_ context.Context) (int, error) { return 0, nil }

func TestPopulateHistoricalFields(t *testing.T) {
	now := time.Now()

	// Create EOD data with 7 bars
	eod := []models.EODBar{
		{Date: now, Close: 50.00},                   // today
		{Date: now.AddDate(0, 0, -1), Close: 48.00}, // yesterday (EOD[1])
		{Date: now.AddDate(0, 0, -2), Close: 47.00}, // 2 days ago
		{Date: now.AddDate(0, 0, -3), Close: 46.00}, // 3 days ago
		{Date: now.AddDate(0, 0, -4), Close: 45.00}, // 4 days ago
		{Date: now.AddDate(0, 0, -5), Close: 44.00}, // 5 days ago (last week = EOD[5])
		{Date: now.AddDate(0, 0, -6), Close: 43.00}, // 6 days ago
	}

	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {
					Ticker: "BHP.AU",
					EOD:    eod,
				},
			},
		},
	}

	eodhd := &mockEODHDClient{
		quote: &models.RealTimeQuote{
			Code:      "BHP.AU",
			Close:     50.00,
			Timestamp: now,
		},
	}

	svc := NewService(eodhd, nil, storage, common.NewSilentLogger())
	svc.now = func() time.Time { return now }

	quote, err := svc.GetRealTimeQuote(context.Background(), "BHP.AU")
	if err != nil {
		t.Fatalf("GetRealTimeQuote failed: %v", err)
	}

	// Yesterday close should be EOD[1] = 48.00
	if quote.PreviousClose != 48.00 {
		t.Errorf("PreviousClose = %.2f, want 48.00", quote.PreviousClose)
	}

	// Yesterday % = (50 - 48) / 48 * 100 = 4.166...
	expectedYesterdayPct := (50.00 - 48.00) / 48.00 * 100
	if quote.YesterdayPct < expectedYesterdayPct-0.01 || quote.YesterdayPct > expectedYesterdayPct+0.01 {
		t.Errorf("YesterdayPct = %.2f, want %.2f", quote.YesterdayPct, expectedYesterdayPct)
	}

	// Last week close should be EOD[5] = 44.00
	if quote.LastWeekClose != 44.00 {
		t.Errorf("LastWeekClose = %.2f, want 44.00", quote.LastWeekClose)
	}

	// Last week % = (50 - 44) / 44 * 100 = 13.636...
	expectedLastWeekPct := (50.00 - 44.00) / 44.00 * 100
	if quote.LastWeekPct < expectedLastWeekPct-0.01 || quote.LastWeekPct > expectedLastWeekPct+0.01 {
		t.Errorf("LastWeekPct = %.2f, want %.2f", quote.LastWeekPct, expectedLastWeekPct)
	}
}

func TestPopulateHistoricalFields_NoStorage(t *testing.T) {
	now := time.Now()

	eodhd := &mockEODHDClient{
		quote: &models.RealTimeQuote{
			Code:      "BHP.AU",
			Close:     50.00,
			Timestamp: now,
		},
	}

	// Pass nil storage - historical fields should be omitted
	svc := NewService(eodhd, nil, nil, common.NewSilentLogger())
	svc.now = func() time.Time { return now }

	quote, err := svc.GetRealTimeQuote(context.Background(), "BHP.AU")
	if err != nil {
		t.Fatalf("GetRealTimeQuote failed: %v", err)
	}

	// Historical fields should be zero when no storage
	if quote.YesterdayClose != 0 {
		t.Errorf("YesterdayClose = %.2f, want 0 (no storage)", quote.YesterdayClose)
	}
	if quote.LastWeekClose != 0 {
		t.Errorf("LastWeekClose = %.2f, want 0 (no storage)", quote.LastWeekClose)
	}
}

func TestPopulateHistoricalFields_NoEODData(t *testing.T) {
	now := time.Now()

	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {
					Ticker: "BHP.AU",
					EOD:    []models.EODBar{}, // empty EOD
				},
			},
		},
	}

	eodhd := &mockEODHDClient{
		quote: &models.RealTimeQuote{
			Code:      "BHP.AU",
			Close:     50.00,
			Timestamp: now,
		},
	}

	svc := NewService(eodhd, nil, storage, common.NewSilentLogger())
	svc.now = func() time.Time { return now }

	quote, err := svc.GetRealTimeQuote(context.Background(), "BHP.AU")
	if err != nil {
		t.Fatalf("GetRealTimeQuote failed: %v", err)
	}

	// Historical fields should be zero when no EOD data
	if quote.YesterdayClose != 0 {
		t.Errorf("YesterdayClose = %.2f, want 0 (no EOD)", quote.YesterdayClose)
	}
	if quote.LastWeekClose != 0 {
		t.Errorf("LastWeekClose = %.2f, want 0 (no EOD)", quote.LastWeekClose)
	}
}
