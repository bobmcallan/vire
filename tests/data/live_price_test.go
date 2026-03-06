package data

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
	tcommon "github.com/bobmcallan/vire/tests/common"
)

func TestCollectLivePrices_StoresLivePrice(t *testing.T) {
	guard := tcommon.NewTestOutputGuard(t)

	mock := &mockEODHD{
		getBulkRealTimeQuotesFn: func(_ context.Context, tickers []string) (map[string]*models.RealTimeQuote, error) {
			result := make(map[string]*models.RealTimeQuote)
			for _, tk := range tickers {
				result[tk] = &models.RealTimeQuote{
					Code:      tk,
					Open:      50.0,
					High:      52.0,
					Low:       49.0,
					Close:     51.5,
					Volume:    1200000,
					Timestamp: time.Now(),
					Source:    "eodhd",
				}
			}
			return result, nil
		},
	}

	svc, mgr := testMarketService(t, mock)
	ctx := testContext()

	// Seed stock index and market data
	indexStore := mgr.StockIndexStore()
	marketStore := mgr.MarketDataStorage()
	yesterday := time.Now().AddDate(0, 0, -1).Truncate(24 * time.Hour)

	for _, ticker := range []string{"LPT.AU", "LPX.AU"} {
		require.NoError(t, indexStore.Upsert(ctx, &models.StockIndexEntry{
			Ticker:   ticker,
			Code:     ticker[:3],
			Exchange: "AU",
			Source:   "test",
			AddedAt:  yesterday,
		}))
		require.NoError(t, marketStore.SaveMarketData(ctx, &models.MarketData{
			Ticker:       ticker,
			Exchange:     "AU",
			EODUpdatedAt: yesterday,
			EOD:          []models.EODBar{{Date: yesterday, Close: 48.0, Volume: 800000}},
		}))
	}

	err := svc.CollectLivePrices(ctx, "AU")
	require.NoError(t, err)

	// Verify LivePrice is set on both tickers
	for _, ticker := range []string{"LPT.AU", "LPX.AU"} {
		md, err := marketStore.GetMarketData(ctx, ticker)
		require.NoError(t, err)
		require.NotNil(t, md.LivePrice, "%s should have LivePrice set", ticker)
		assert.Equal(t, 51.5, md.LivePrice.Close, "%s live close price", ticker)
		assert.Equal(t, int64(1200000), md.LivePrice.Volume, "%s live volume", ticker)
		assert.False(t, md.LivePriceUpdatedAt.IsZero(), "%s LivePriceUpdatedAt should be set", ticker)
	}

	guard.SaveResult("01_stores_live_price", fmt.Sprintf(
		"LivePrice set on 2 tickers: Close=51.5, Volume=1200000, LivePriceUpdatedAt non-zero"))
}

func TestCollectLivePrices_DoesNotModifyEOD(t *testing.T) {
	guard := tcommon.NewTestOutputGuard(t)
	yesterday := time.Now().AddDate(0, 0, -1).Truncate(24 * time.Hour)

	originalEOD := []models.EODBar{
		{Date: yesterday, Open: 47.0, High: 49.0, Low: 46.0, Close: 48.0, Volume: 700000},
		{Date: yesterday.AddDate(0, 0, -1), Open: 46.0, High: 48.0, Low: 45.0, Close: 47.0, Volume: 600000},
	}

	mock := &mockEODHD{
		getBulkRealTimeQuotesFn: func(_ context.Context, tickers []string) (map[string]*models.RealTimeQuote, error) {
			result := make(map[string]*models.RealTimeQuote)
			for _, tk := range tickers {
				result[tk] = &models.RealTimeQuote{
					Code:  tk,
					Close: 99.99,
				}
			}
			return result, nil
		},
	}

	svc, mgr := testMarketService(t, mock)
	ctx := testContext()

	require.NoError(t, mgr.StockIndexStore().Upsert(ctx, &models.StockIndexEntry{
		Ticker: "EOD.AU", Code: "EOD", Exchange: "AU", Source: "test", AddedAt: yesterday,
	}))
	require.NoError(t, mgr.MarketDataStorage().SaveMarketData(ctx, &models.MarketData{
		Ticker:       "EOD.AU",
		Exchange:     "AU",
		EODUpdatedAt: yesterday,
		EOD:          originalEOD,
	}))

	err := svc.CollectLivePrices(ctx, "AU")
	require.NoError(t, err)

	md, err := mgr.MarketDataStorage().GetMarketData(ctx, "EOD.AU")
	require.NoError(t, err)

	// EOD bars must be unchanged
	require.Len(t, md.EOD, 2, "EOD bar count should be unchanged")
	assert.Equal(t, 48.0, md.EOD[0].Close, "first EOD bar close unchanged")
	assert.Equal(t, 47.0, md.EOD[1].Close, "second EOD bar close unchanged")
	assert.Equal(t, int64(700000), md.EOD[0].Volume, "first EOD bar volume unchanged")

	// LivePrice should be set separately
	require.NotNil(t, md.LivePrice)
	assert.Equal(t, 99.99, md.LivePrice.Close, "LivePrice close should be live value")

	guard.SaveResult("02_does_not_modify_eod", fmt.Sprintf(
		"EOD bars unchanged (2 bars, closes=[48.0, 47.0]), LivePrice.Close=99.99"))
}

func TestLivePriceFreshness(t *testing.T) {
	guard := tcommon.NewTestOutputGuard(t)

	tests := []struct {
		name      string
		updatedAt time.Time
		wantFresh bool
	}{
		{"fresh (1 minute ago)", time.Now().Add(-1 * time.Minute), true},
		{"fresh (14 minutes ago)", time.Now().Add(-14 * time.Minute), true},
		{"stale (16 minutes ago)", time.Now().Add(-16 * time.Minute), false},
		{"stale (1 hour ago)", time.Now().Add(-1 * time.Hour), false},
		{"stale (zero time)", time.Time{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := common.IsFresh(tt.updatedAt, common.FreshnessLivePrice)
			assert.Equal(t, tt.wantFresh, got)
		})
	}

	// Verify FreshnessLivePrice is 15 minutes
	assert.Equal(t, 15*time.Minute, common.FreshnessLivePrice,
		"FreshnessLivePrice should be 15 minutes")

	guard.SaveResult("03_freshness_ttl", "FreshnessLivePrice=15m, boundary tests pass (14m=fresh, 16m=stale)")
}
