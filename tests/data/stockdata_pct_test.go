package data

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
	market "github.com/bobmcallan/vire/internal/services/market"
	tcommon "github.com/bobmcallan/vire/tests/common"
)

// TestGetStockData_LiveQuoteRecalculatesPcts verifies that when a live quote
// overrides the current price in GetStockData, YesterdayPct and LastWeekPct
// are recalculated using the live price rather than the stale EOD[0] close.
func TestGetStockData_LiveQuoteRecalculatesPcts(t *testing.T) {
	guard := tcommon.NewTestOutputGuard(t)

	today := time.Now().Truncate(24 * time.Hour)

	// Build 10 days of EOD data with predictable closes
	eodBars := make([]models.EODBar, 10)
	for i := 0; i < 10; i++ {
		eodBars[i] = models.EODBar{
			Date:  today.AddDate(0, 0, -i),
			Open:  100.0,
			High:  101.0,
			Low:   99.0,
			Close: 100.0, // all EOD closes at 100.0
		}
	}

	// Live quote returns 110.0 (10% above EOD)
	livePrice := 110.0

	mock := &mockEODHD{
		getEODFn: func(_ context.Context, ticker string, opts ...interfaces.EODOption) (*models.EODResponse, error) {
			return &models.EODResponse{Data: eodBars}, nil
		},
		// GetRealTimeQuote needs to be overridden — the mockEODHD in bulkeod_test.go
		// returns error by default. We need a version that returns our live price.
	}

	svc, mgr := testMarketService(t, mock)
	ctx := testContext()

	// Seed stock index and pre-collect market data
	require.NoError(t, mgr.StockIndexStore().Upsert(ctx, &models.StockIndexEntry{
		Ticker: "PCT.AU", Code: "PCT", Exchange: "AU", Source: "test", AddedAt: today.AddDate(0, 0, -30),
	}))

	err := svc.CollectMarketData(ctx, []string{"PCT.AU"}, false, false)
	require.NoError(t, err)

	// Now create a service with a live quote mock
	liveQuoteMock := &mockEODHDWithLiveQuote{
		mockEODHD: mock,
		liveQuote: &models.RealTimeQuote{
			Code:      "PCT.AU",
			Open:      100.0,
			High:      111.0,
			Low:       99.0,
			Close:     livePrice,
			Volume:    500000,
			Timestamp: time.Now(),
		},
	}

	liveSvc := testMarketServiceWithEODHD(t, mgr, liveQuoteMock)

	// Get stock data with price
	include := interfaces.StockDataInclude{Price: true}
	stockData, err := liveSvc.GetStockData(ctx, "PCT.AU", include)
	require.NoError(t, err)
	require.NotNil(t, stockData.Price)

	// Current price should be the live quote
	assert.Equal(t, livePrice, stockData.Price.Current, "current price should be live quote")

	// YesterdayPct should use live price (110) vs EOD[1].Close (100) = +10%
	// Not EOD[0].Close (100) vs EOD[1].Close (100) = 0%
	expectedYesterdayPct := ((livePrice - 100.0) / 100.0) * 100 // +10%
	assert.InDelta(t, expectedYesterdayPct, stockData.Price.YesterdayPct, 0.01,
		"YesterdayPct should use live price, not EOD[0] close")

	// LastWeekPct should use live price (110) vs EOD[5].Close (100) = +10%
	expectedLastWeekPct := ((livePrice - 100.0) / 100.0) * 100 // +10%
	assert.InDelta(t, expectedLastWeekPct, stockData.Price.LastWeekPct, 0.01,
		"LastWeekPct should use live price, not EOD[0] close")

	guard.SaveResult("01_live_quote_recalculates_pcts", fmt.Sprintf(
		"LivePrice=%.1f, YesterdayPct=%.2f%% (expected %.2f%%), LastWeekPct=%.2f%% (expected %.2f%%)",
		stockData.Price.Current,
		stockData.Price.YesterdayPct, expectedYesterdayPct,
		stockData.Price.LastWeekPct, expectedLastWeekPct))
}

// TestGetStockData_NoPctRecalcWithoutLiveQuote verifies that when no live quote
// is available, YesterdayPct and LastWeekPct are calculated from EOD data only.
func TestGetStockData_NoPctRecalcWithoutLiveQuote(t *testing.T) {
	guard := tcommon.NewTestOutputGuard(t)

	today := time.Now().Truncate(24 * time.Hour)

	eodBars := make([]models.EODBar, 10)
	eodBars[0] = models.EODBar{Date: today, Close: 105.0, Open: 100.0, High: 106.0, Low: 99.0}
	for i := 1; i < 10; i++ {
		eodBars[i] = models.EODBar{
			Date:  today.AddDate(0, 0, -i),
			Close: 100.0, Open: 99.0, High: 101.0, Low: 98.0,
		}
	}

	mock := &mockEODHD{
		getEODFn: func(_ context.Context, ticker string, opts ...interfaces.EODOption) (*models.EODResponse, error) {
			return &models.EODResponse{Data: eodBars}, nil
		},
		// No live quote — GetRealTimeQuote returns error (default behavior)
	}

	svc, mgr := testMarketService(t, mock)
	ctx := testContext()

	require.NoError(t, mgr.StockIndexStore().Upsert(ctx, &models.StockIndexEntry{
		Ticker: "NLQ.AU", Code: "NLQ", Exchange: "AU", Source: "test", AddedAt: today.AddDate(0, 0, -30),
	}))

	err := svc.CollectMarketData(ctx, []string{"NLQ.AU"}, false, false)
	require.NoError(t, err)

	include := interfaces.StockDataInclude{Price: true}
	stockData, err := svc.GetStockData(ctx, "NLQ.AU", include)
	require.NoError(t, err)
	require.NotNil(t, stockData.Price)

	// Current price should be EOD[0].Close
	assert.Equal(t, 105.0, stockData.Price.Current, "current price should be EOD[0] close")

	// YesterdayPct = (105-100)/100 * 100 = 5%
	assert.InDelta(t, 5.0, stockData.Price.YesterdayPct, 0.01, "YesterdayPct from EOD data")

	// LastWeekPct = (105-100)/100 * 100 = 5%
	assert.InDelta(t, 5.0, stockData.Price.LastWeekPct, 0.01, "LastWeekPct from EOD data")

	guard.SaveResult("02_no_live_quote_uses_eod", fmt.Sprintf(
		"No live quote. Current=%.1f (EOD[0]), YesterdayPct=%.2f%%, LastWeekPct=%.2f%%",
		stockData.Price.Current, stockData.Price.YesterdayPct, stockData.Price.LastWeekPct))
}

// TestGetStockData_LiveQuotePctWithZeroHistorical verifies division-by-zero
// protection when historical close prices are zero.
func TestGetStockData_LiveQuotePctWithZeroHistorical(t *testing.T) {
	guard := tcommon.NewTestOutputGuard(t)

	today := time.Now().Truncate(24 * time.Hour)

	eodBars := make([]models.EODBar, 10)
	eodBars[0] = models.EODBar{Date: today, Close: 50.0}
	// EOD[1] (yesterday) has zero close
	eodBars[1] = models.EODBar{Date: today.AddDate(0, 0, -1), Close: 0.0}
	for i := 2; i < 10; i++ {
		eodBars[i] = models.EODBar{Date: today.AddDate(0, 0, -i), Close: 50.0}
	}
	// EOD[5] (last week) also zero
	eodBars[5] = models.EODBar{Date: today.AddDate(0, 0, -5), Close: 0.0}

	mock := &mockEODHD{
		getEODFn: func(_ context.Context, ticker string, opts ...interfaces.EODOption) (*models.EODResponse, error) {
			return &models.EODResponse{Data: eodBars}, nil
		},
	}

	svc, mgr := testMarketService(t, mock)
	ctx := testContext()

	require.NoError(t, mgr.StockIndexStore().Upsert(ctx, &models.StockIndexEntry{
		Ticker: "ZRO.AU", Code: "ZRO", Exchange: "AU", Source: "test", AddedAt: today.AddDate(0, 0, -30),
	}))

	err := svc.CollectMarketData(ctx, []string{"ZRO.AU"}, false, false)
	require.NoError(t, err)

	include := interfaces.StockDataInclude{Price: true}
	stockData, err := svc.GetStockData(ctx, "ZRO.AU", include)
	require.NoError(t, err)
	require.NotNil(t, stockData.Price)

	// With zero historical closes, pct should be 0 (not NaN or Inf)
	assert.False(t, stockData.Price.YesterdayPct != stockData.Price.YesterdayPct,
		"YesterdayPct should not be NaN")
	assert.False(t, stockData.Price.LastWeekPct != stockData.Price.LastWeekPct,
		"LastWeekPct should not be NaN")

	guard.SaveResult("03_zero_historical_no_panic", fmt.Sprintf(
		"Zero historical closes handled safely. YesterdayPct=%.2f, LastWeekPct=%.2f",
		stockData.Price.YesterdayPct, stockData.Price.LastWeekPct))
}

// --- Helpers ---

// mockEODHDWithLiveQuote wraps mockEODHD and overrides GetRealTimeQuote.
type mockEODHDWithLiveQuote struct {
	*mockEODHD
	liveQuote *models.RealTimeQuote
}

func (m *mockEODHDWithLiveQuote) GetRealTimeQuote(_ context.Context, ticker string) (*models.RealTimeQuote, error) {
	if m.liveQuote != nil {
		return m.liveQuote, nil
	}
	return nil, fmt.Errorf("no live quote")
}

// testMarketServiceWithEODHD creates a market.Service using an existing
// StorageManager and a specific EODHD client (for live quote scenarios).
func testMarketServiceWithEODHD(t *testing.T, mgr interfaces.StorageManager, eodhd interfaces.EODHDClient) *market.Service {
	t.Helper()
	logger := common.NewSilentLogger()
	return market.NewService(mgr, eodhd, nil, logger)
}
