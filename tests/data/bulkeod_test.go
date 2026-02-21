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
)

// --- mock EODHD client for data integration tests ---

type mockEODHD struct {
	getBulkEODFn func(ctx context.Context, exchange string, tickers []string) (map[string]models.EODBar, error)
	getEODFn     func(ctx context.Context, ticker string, opts ...interfaces.EODOption) (*models.EODResponse, error)
}

func (m *mockEODHD) GetRealTimeQuote(ctx context.Context, ticker string) (*models.RealTimeQuote, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *mockEODHD) GetEOD(ctx context.Context, ticker string, opts ...interfaces.EODOption) (*models.EODResponse, error) {
	if m.getEODFn != nil {
		return m.getEODFn(ctx, ticker, opts...)
	}
	return nil, fmt.Errorf("not implemented")
}
func (m *mockEODHD) GetBulkEOD(ctx context.Context, exchange string, tickers []string) (map[string]models.EODBar, error) {
	if m.getBulkEODFn != nil {
		return m.getBulkEODFn(ctx, exchange, tickers)
	}
	return nil, fmt.Errorf("not implemented")
}
func (m *mockEODHD) GetFundamentals(ctx context.Context, ticker string) (*models.Fundamentals, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *mockEODHD) GetTechnicals(ctx context.Context, ticker string, function string) (*models.TechnicalResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *mockEODHD) GetNews(ctx context.Context, ticker string, limit int) ([]*models.NewsItem, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *mockEODHD) GetExchangeSymbols(ctx context.Context, exchange string) ([]*models.Symbol, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *mockEODHD) ScreenStocks(ctx context.Context, options models.ScreenerOptions) ([]*models.ScreenerResult, error) {
	return nil, fmt.Errorf("not implemented")
}

// testMarketService creates a market.Service with real storage and a mock EODHD client.
func testMarketService(t *testing.T, eodhd interfaces.EODHDClient) (*market.Service, interfaces.StorageManager) {
	t.Helper()
	mgr := testManager(t)
	logger := common.NewSilentLogger()
	svc := market.NewService(mgr, eodhd, nil, logger)
	return svc, mgr
}

func TestCollectBulkEOD_MergeExistingData(t *testing.T) {
	today := time.Now().Truncate(24 * time.Hour)
	yesterday := today.AddDate(0, 0, -1)
	twoDaysAgo := today.AddDate(0, 0, -2)

	mock := &mockEODHD{
		getBulkEODFn: func(_ context.Context, exchange string, tickers []string) (map[string]models.EODBar, error) {
			assert.Equal(t, "AU", exchange)
			result := make(map[string]models.EODBar)
			for _, ticker := range tickers {
				result[ticker] = models.EODBar{
					Date:   today,
					Open:   50.0,
					High:   52.0,
					Low:    49.0,
					Close:  51.5,
					Volume: 1000000,
				}
			}
			return result, nil
		},
	}

	svc, mgr := testMarketService(t, mock)
	ctx := testContext()

	// Seed stock index with tickers
	indexStore := mgr.StockIndexStore()
	for _, ticker := range []string{"AAA.AU", "BBB.AU"} {
		require.NoError(t, indexStore.Upsert(ctx, &models.StockIndexEntry{
			Ticker:   ticker,
			Code:     ticker[:3],
			Exchange: "AU",
			Source:   "test",
			AddedAt:  twoDaysAgo,
		}))
	}

	// Seed existing market data with EOD history for AAA.AU
	marketStore := mgr.MarketDataStorage()
	require.NoError(t, marketStore.SaveMarketData(ctx, &models.MarketData{
		Ticker:       "AAA.AU",
		Exchange:     "AU",
		EODUpdatedAt: twoDaysAgo, // stale
		EOD: []models.EODBar{
			{Date: yesterday, Open: 48.0, High: 50.0, Low: 47.0, Close: 49.0, Volume: 800000},
			{Date: twoDaysAgo, Open: 47.0, High: 49.0, Low: 46.0, Close: 48.0, Volume: 700000},
		},
	}))

	// Seed existing market data with EOD history for BBB.AU
	require.NoError(t, marketStore.SaveMarketData(ctx, &models.MarketData{
		Ticker:       "BBB.AU",
		Exchange:     "AU",
		EODUpdatedAt: twoDaysAgo, // stale
		EOD: []models.EODBar{
			{Date: yesterday, Open: 30.0, High: 31.0, Low: 29.0, Close: 30.5, Volume: 500000},
		},
	}))

	// Run CollectBulkEOD
	err := svc.CollectBulkEOD(ctx, "AU", false)
	require.NoError(t, err)

	// Verify AAA.AU has merged data (today + yesterday + twoDaysAgo)
	aaaMD, err := marketStore.GetMarketData(ctx, "AAA.AU")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(aaaMD.EOD), 3, "AAA.AU should have at least 3 EOD bars after merge")
	assert.Equal(t, 51.5, aaaMD.EOD[0].Close, "most recent bar should be today's bulk bar")
	assert.False(t, aaaMD.EODUpdatedAt.IsZero(), "EODUpdatedAt should be set")

	// Verify BBB.AU has merged data
	bbbMD, err := marketStore.GetMarketData(ctx, "BBB.AU")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(bbbMD.EOD), 2, "BBB.AU should have at least 2 EOD bars after merge")
}

func TestCollectBulkEOD_FallbackForNewTickers(t *testing.T) {
	today := time.Now().Truncate(24 * time.Hour)
	twoDaysAgo := today.AddDate(0, 0, -2)
	individualCalled := false

	mock := &mockEODHD{
		getBulkEODFn: func(_ context.Context, exchange string, tickers []string) (map[string]models.EODBar, error) {
			result := make(map[string]models.EODBar)
			for _, ticker := range tickers {
				result[ticker] = models.EODBar{
					Date:  today,
					Close: 20.0,
				}
			}
			return result, nil
		},
		getEODFn: func(_ context.Context, ticker string, opts ...interfaces.EODOption) (*models.EODResponse, error) {
			individualCalled = true
			// Return 3 years of data (simulating full history fetch)
			return &models.EODResponse{
				Data: []models.EODBar{
					{Date: today, Close: 20.0, Volume: 100000},
					{Date: today.AddDate(0, 0, -1), Close: 19.5, Volume: 90000},
					{Date: today.AddDate(0, 0, -2), Close: 19.0, Volume: 85000},
				},
			}, nil
		},
	}

	svc, mgr := testMarketService(t, mock)
	ctx := testContext()

	// Seed stock index with a new ticker (no existing market data)
	indexStore := mgr.StockIndexStore()
	require.NoError(t, indexStore.Upsert(ctx, &models.StockIndexEntry{
		Ticker:   "NEW.AU",
		Code:     "NEW",
		Exchange: "AU",
		Source:   "test",
		AddedAt:  twoDaysAgo,
	}))

	// Do NOT seed market data — NEW.AU is a new ticker with no history

	err := svc.CollectBulkEOD(ctx, "AU", false)
	require.NoError(t, err)

	assert.True(t, individualCalled, "should fall back to individual CollectEOD for new ticker")

	// Verify market data was saved via the individual fallback
	newMD, err := mgr.MarketDataStorage().GetMarketData(ctx, "NEW.AU")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(newMD.EOD), 3, "new ticker should have full history from individual fetch")
}

func TestCollectBulkEOD_StockIndexTimestampUpdated(t *testing.T) {
	today := time.Now().Truncate(24 * time.Hour)
	yesterday := today.AddDate(0, 0, -1)
	twoDaysAgo := today.AddDate(0, 0, -2)

	mock := &mockEODHD{
		getBulkEODFn: func(_ context.Context, _ string, tickers []string) (map[string]models.EODBar, error) {
			return map[string]models.EODBar{
				"TST.AU": {Date: today, Close: 10.0, Volume: 50000},
			}, nil
		},
	}

	svc, mgr := testMarketService(t, mock)
	ctx := testContext()

	// Seed stock index
	indexStore := mgr.StockIndexStore()
	require.NoError(t, indexStore.Upsert(ctx, &models.StockIndexEntry{
		Ticker:   "TST.AU",
		Code:     "TST",
		Exchange: "AU",
		Source:   "test",
		AddedAt:  twoDaysAgo,
	}))

	// Seed existing market data
	require.NoError(t, mgr.MarketDataStorage().SaveMarketData(ctx, &models.MarketData{
		Ticker:       "TST.AU",
		Exchange:     "AU",
		EODUpdatedAt: twoDaysAgo,
		EOD:          []models.EODBar{{Date: yesterday, Close: 9.5}},
	}))

	err := svc.CollectBulkEOD(ctx, "AU", false)
	require.NoError(t, err)

	// Verify stock index timestamp was updated
	entry, err := indexStore.Get(ctx, "TST.AU")
	require.NoError(t, err)
	assert.False(t, entry.EODCollectedAt.IsZero(), "eod_collected_at should be updated")
	assert.True(t, entry.EODCollectedAt.After(twoDaysAgo), "eod_collected_at should be recent")
}

func TestCollectBulkEOD_EmptyExchange(t *testing.T) {
	mock := &mockEODHD{
		getBulkEODFn: func(_ context.Context, _ string, _ []string) (map[string]models.EODBar, error) {
			t.Fatal("should not call bulk API when no tickers for exchange")
			return nil, nil
		},
	}

	svc, mgr := testMarketService(t, mock)
	ctx := testContext()

	// Seed stock index with a US ticker only
	require.NoError(t, mgr.StockIndexStore().Upsert(ctx, &models.StockIndexEntry{
		Ticker:   "AAPL.US",
		Code:     "AAPL",
		Exchange: "US",
		Source:   "test",
		AddedAt:  time.Now(),
	}))

	// Request AU exchange — should find no tickers and return early
	err := svc.CollectBulkEOD(ctx, "AU", false)
	require.NoError(t, err)
}

func TestCollectBulkEOD_SignalsComputed(t *testing.T) {
	today := time.Now().Truncate(24 * time.Hour)
	twoDaysAgo := today.AddDate(0, 0, -2)

	// Build enough bars for signal computation
	existingBars := make([]models.EODBar, 50)
	for i := 0; i < 50; i++ {
		existingBars[i] = models.EODBar{
			Date:   today.AddDate(0, 0, -(i + 1)),
			Close:  40.0 + float64(i%5),
			High:   42.0 + float64(i%5),
			Low:    38.0 + float64(i%5),
			Volume: int64(500000 + i*10000),
		}
	}

	mock := &mockEODHD{
		getBulkEODFn: func(_ context.Context, _ string, _ []string) (map[string]models.EODBar, error) {
			return map[string]models.EODBar{
				"SIG.AU": {Date: today, Open: 42.0, High: 44.0, Low: 41.0, Close: 43.5, Volume: 600000},
			}, nil
		},
	}

	svc, mgr := testMarketService(t, mock)
	ctx := testContext()

	// Seed stock index
	require.NoError(t, mgr.StockIndexStore().Upsert(ctx, &models.StockIndexEntry{
		Ticker:   "SIG.AU",
		Code:     "SIG",
		Exchange: "AU",
		Source:   "test",
		AddedAt:  twoDaysAgo,
	}))

	// Seed market data with existing bars
	require.NoError(t, mgr.MarketDataStorage().SaveMarketData(ctx, &models.MarketData{
		Ticker:       "SIG.AU",
		Exchange:     "AU",
		EODUpdatedAt: twoDaysAgo,
		EOD:          existingBars,
	}))

	err := svc.CollectBulkEOD(ctx, "AU", false)
	require.NoError(t, err)

	// Verify signals were computed
	signals, err := mgr.SignalStorage().GetSignals(ctx, "SIG.AU")
	require.NoError(t, err)
	assert.Equal(t, "SIG.AU", signals.Ticker)
	assert.NotZero(t, signals.Price.Current, "signals should have current price")
}

func TestCollectBulkEOD_PartialBulkResponse(t *testing.T) {
	today := time.Now().Truncate(24 * time.Hour)
	yesterday := today.AddDate(0, 0, -1)
	twoDaysAgo := today.AddDate(0, 0, -2)

	mock := &mockEODHD{
		getBulkEODFn: func(_ context.Context, _ string, tickers []string) (map[string]models.EODBar, error) {
			// Only return data for the first ticker (simulating partial response)
			return map[string]models.EODBar{
				"FOUND.AU": {Date: today, Close: 15.0, Volume: 200000},
			}, nil
		},
	}

	svc, mgr := testMarketService(t, mock)
	ctx := testContext()

	indexStore := mgr.StockIndexStore()
	marketStore := mgr.MarketDataStorage()

	// Seed two tickers in the index
	for _, ticker := range []string{"FOUND.AU", "MISSING.AU"} {
		require.NoError(t, indexStore.Upsert(ctx, &models.StockIndexEntry{
			Ticker:   ticker,
			Code:     ticker[:len(ticker)-3],
			Exchange: "AU",
			Source:   "test",
			AddedAt:  twoDaysAgo,
		}))
		// Both have existing market data
		require.NoError(t, marketStore.SaveMarketData(ctx, &models.MarketData{
			Ticker:       ticker,
			Exchange:     "AU",
			EODUpdatedAt: twoDaysAgo,
			EOD:          []models.EODBar{{Date: yesterday, Close: 14.0}},
		}))
	}

	err := svc.CollectBulkEOD(ctx, "AU", false)
	require.NoError(t, err)

	// FOUND.AU should have new bar merged
	found, err := marketStore.GetMarketData(ctx, "FOUND.AU")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(found.EOD), 2, "FOUND.AU should have merged bar")

	// MISSING.AU should still have only its original bar (not in bulk response)
	missing, err := marketStore.GetMarketData(ctx, "MISSING.AU")
	require.NoError(t, err)
	assert.Len(t, missing.EOD, 1, "MISSING.AU should not be modified when not in bulk response")
}

func TestCollectBulkEOD_MixedExchangeTickers(t *testing.T) {
	today := time.Now().Truncate(24 * time.Hour)
	yesterday := today.AddDate(0, 0, -1)
	twoDaysAgo := today.AddDate(0, 0, -2)

	auBulkCalled := false
	mock := &mockEODHD{
		getBulkEODFn: func(_ context.Context, exchange string, tickers []string) (map[string]models.EODBar, error) {
			if exchange == "AU" {
				auBulkCalled = true
				// Should only receive AU tickers
				for _, tk := range tickers {
					assert.Contains(t, tk, ".AU", "AU bulk should only receive AU tickers")
				}
				result := make(map[string]models.EODBar)
				for _, ticker := range tickers {
					result[ticker] = models.EODBar{Date: today, Close: 25.0}
				}
				return result, nil
			}
			return nil, fmt.Errorf("unexpected exchange: %s", exchange)
		},
	}

	svc, mgr := testMarketService(t, mock)
	ctx := testContext()

	indexStore := mgr.StockIndexStore()
	marketStore := mgr.MarketDataStorage()

	// Seed AU ticker
	require.NoError(t, indexStore.Upsert(ctx, &models.StockIndexEntry{
		Ticker: "XAU.AU", Code: "XAU", Exchange: "AU", Source: "test", AddedAt: twoDaysAgo,
	}))
	require.NoError(t, marketStore.SaveMarketData(ctx, &models.MarketData{
		Ticker: "XAU.AU", Exchange: "AU", EODUpdatedAt: twoDaysAgo,
		EOD: []models.EODBar{{Date: yesterday, Close: 24.0}},
	}))

	// Seed US ticker (should not be included when collecting AU)
	require.NoError(t, indexStore.Upsert(ctx, &models.StockIndexEntry{
		Ticker: "AAPL.US", Code: "AAPL", Exchange: "US", Source: "test", AddedAt: twoDaysAgo,
	}))
	require.NoError(t, marketStore.SaveMarketData(ctx, &models.MarketData{
		Ticker: "AAPL.US", Exchange: "US", EODUpdatedAt: twoDaysAgo,
		EOD: []models.EODBar{{Date: yesterday, Close: 175.0}},
	}))

	// Collect only AU exchange
	err := svc.CollectBulkEOD(ctx, "AU", false)
	require.NoError(t, err)

	assert.True(t, auBulkCalled, "should call bulk API for AU exchange")

	// AU ticker should be updated
	xau, err := marketStore.GetMarketData(ctx, "XAU.AU")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(xau.EOD), 2)

	// US ticker should be untouched
	aapl, err := marketStore.GetMarketData(ctx, "AAPL.US")
	require.NoError(t, err)
	assert.Len(t, aapl.EOD, 1, "US ticker should not be modified by AU bulk collection")
}
