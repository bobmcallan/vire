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
	svc := NewService(eodhd, asxClient, common.NewSilentLogger())
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
