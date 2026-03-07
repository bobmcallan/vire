package data

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
	tcommon "github.com/bobmcallan/vire/tests/common"
)

// TestFilterBadEODBars_StorageRoundtrip verifies that divergent EOD bars are
// filtered out when market data is collected and persisted via the market service.
// This tests Fix A end-to-end through storage.
func TestFilterBadEODBars_StorageRoundtrip(t *testing.T) {
	guard := tcommon.NewTestOutputGuard(t)

	today := time.Now().Truncate(24 * time.Hour)

	// Build 10 days of EOD bars with a spike on day 3 (40%+ divergence)
	normalClose := 100.0
	eodBars := make([]models.EODBar, 10)
	for i := 0; i < 10; i++ {
		close := normalClose - float64(i)*0.5 // gentle downtrend: 100, 99.5, 99, ...
		eodBars[i] = models.EODBar{
			Date:  today.AddDate(0, 0, -i),
			Open:  close - 1,
			High:  close + 1,
			Low:   close - 2,
			Close: close,
		}
	}
	// Inject a bad bar at position 3 with >40% divergence from neighbors
	eodBars[3].Close = 50.0 // ~49% drop from ~98.5, clearly divergent

	mock := &mockEODHD{
		getEODFn: func(_ context.Context, ticker string, opts ...interfaces.EODOption) (*models.EODResponse, error) {
			return &models.EODResponse{Data: eodBars}, nil
		},
	}

	svc, mgr := testMarketService(t, mock)
	ctx := testContext()

	// Seed stock index entry so CollectMarketData processes this ticker
	require.NoError(t, mgr.StockIndexStore().Upsert(ctx, &models.StockIndexEntry{
		Ticker:   "DIV.AU",
		Code:     "DIV",
		Exchange: "AU",
		Source:   "test",
		AddedAt:  today.AddDate(0, 0, -30),
	}))

	// Collect market data (this triggers mergeEODBars + filterBadEODBars)
	err := svc.CollectMarketData(ctx, []string{"DIV.AU"}, false, false)
	require.NoError(t, err)

	// Retrieve stored market data
	md, err := mgr.MarketDataStorage().GetMarketData(ctx, "DIV.AU")
	require.NoError(t, err)
	require.NotNil(t, md)

	// The bad bar (close=50.0) should have been filtered out
	for _, bar := range md.EOD {
		assert.NotEqual(t, 50.0, bar.Close,
			"bar with divergent close=50.0 on %s should be filtered",
			bar.Date.Format("2006-01-02"))
	}

	// Should have 9 bars (10 minus the 1 divergent)
	assert.Equal(t, 9, len(md.EOD), "expected 9 bars after filtering 1 divergent bar")

	// Bars should still be sorted descending
	for i := 1; i < len(md.EOD); i++ {
		assert.True(t, md.EOD[i-1].Date.After(md.EOD[i].Date) || md.EOD[i-1].Date.Equal(md.EOD[i].Date),
			"bars should be sorted descending: bar[%d]=%s should be >= bar[%d]=%s",
			i-1, md.EOD[i-1].Date.Format("2006-01-02"),
			i, md.EOD[i].Date.Format("2006-01-02"))
	}

	guard.SaveResult("01_divergent_bar_filtered", fmt.Sprintf(
		"Divergent bar (close=50.0) filtered. Remaining bars: %d. All closes near 100.", len(md.EOD)))
}

// TestFilterBadEODBars_DivergentFirstBar tests the case where the most recent
// bar (EOD[0]) is the divergent one. This is the most critical case because
// EOD[0] is used for current price, yesterday_close_price, etc.
func TestFilterBadEODBars_DivergentFirstBar(t *testing.T) {
	guard := tcommon.NewTestOutputGuard(t)

	today := time.Now().Truncate(24 * time.Hour)

	eodBars := []models.EODBar{
		{Date: today, Close: 50.0, Open: 49.0, High: 51.0, Low: 48.0}, // BAD: ~49% below normal
		{Date: today.AddDate(0, 0, -1), Close: 98.0, Open: 97.0, High: 99.0, Low: 96.0},
		{Date: today.AddDate(0, 0, -2), Close: 97.5, Open: 96.5, High: 98.5, Low: 95.5},
		{Date: today.AddDate(0, 0, -3), Close: 97.0, Open: 96.0, High: 98.0, Low: 95.0},
	}

	mock := &mockEODHD{
		getEODFn: func(_ context.Context, ticker string, opts ...interfaces.EODOption) (*models.EODResponse, error) {
			return &models.EODResponse{Data: eodBars}, nil
		},
	}

	svc, mgr := testMarketService(t, mock)
	ctx := testContext()

	require.NoError(t, mgr.StockIndexStore().Upsert(ctx, &models.StockIndexEntry{
		Ticker: "FB.AU", Code: "FB", Exchange: "AU", Source: "test", AddedAt: today.AddDate(0, 0, -30),
	}))

	err := svc.CollectMarketData(ctx, []string{"FB.AU"}, false, false)
	require.NoError(t, err)

	md, err := mgr.MarketDataStorage().GetMarketData(ctx, "FB.AU")
	require.NoError(t, err)
	require.NotNil(t, md)

	// The first bar (close=50.0) should be filtered
	if len(md.EOD) > 0 {
		assert.NotEqual(t, 50.0, md.EOD[0].Close,
			"most recent bar should not have divergent close=50.0")
		// The new EOD[0] should be the next legitimate bar
		assert.InDelta(t, 98.0, md.EOD[0].Close, 1.0,
			"most recent bar should be ~98.0 after filtering")
	}

	assert.Equal(t, 3, len(md.EOD), "expected 3 bars after filtering divergent first bar")

	guard.SaveResult("02_divergent_first_bar", fmt.Sprintf(
		"Divergent first bar filtered. New EOD[0] close: %.1f", md.EOD[0].Close))
}

// TestFilterBadEODBars_LegitimateMove verifies that a <=40% move is NOT filtered.
func TestFilterBadEODBars_LegitimateMove(t *testing.T) {
	guard := tcommon.NewTestOutputGuard(t)

	today := time.Now().Truncate(24 * time.Hour)

	// 20% drop is legitimate (e.g., earnings miss)
	eodBars := []models.EODBar{
		{Date: today, Close: 80.0, Open: 85.0, High: 86.0, Low: 79.0},
		{Date: today.AddDate(0, 0, -1), Close: 100.0, Open: 99.0, High: 101.0, Low: 98.0},
		{Date: today.AddDate(0, 0, -2), Close: 99.5, Open: 98.5, High: 100.5, Low: 97.5},
		{Date: today.AddDate(0, 0, -3), Close: 99.0, Open: 98.0, High: 100.0, Low: 97.0},
	}

	mock := &mockEODHD{
		getEODFn: func(_ context.Context, ticker string, opts ...interfaces.EODOption) (*models.EODResponse, error) {
			return &models.EODResponse{Data: eodBars}, nil
		},
	}

	svc, mgr := testMarketService(t, mock)
	ctx := testContext()

	require.NoError(t, mgr.StockIndexStore().Upsert(ctx, &models.StockIndexEntry{
		Ticker: "LEG.AU", Code: "LEG", Exchange: "AU", Source: "test", AddedAt: today.AddDate(0, 0, -30),
	}))

	err := svc.CollectMarketData(ctx, []string{"LEG.AU"}, false, false)
	require.NoError(t, err)

	md, err := mgr.MarketDataStorage().GetMarketData(ctx, "LEG.AU")
	require.NoError(t, err)
	require.NotNil(t, md)

	// All 4 bars should be kept (20% move is within threshold)
	assert.Equal(t, 4, len(md.EOD), "all bars should be kept for 20%% move")
	assert.Equal(t, 80.0, md.EOD[0].Close, "first bar close should be 80.0")

	guard.SaveResult("03_legitimate_move_kept", fmt.Sprintf(
		"All %d bars kept for legitimate 20%% move. EOD[0].Close=%.1f", len(md.EOD), md.EOD[0].Close))
}

// TestFilterBadEODBars_TooFewBars verifies that bars are returned as-is
// when there are fewer than 3 bars (not enough context to judge divergence).
func TestFilterBadEODBars_TooFewBars(t *testing.T) {
	guard := tcommon.NewTestOutputGuard(t)

	today := time.Now().Truncate(24 * time.Hour)

	tests := []struct {
		name string
		bars []models.EODBar
	}{
		{
			name: "two bars with huge divergence",
			bars: []models.EODBar{
				{Date: today, Close: 50.0},
				{Date: today.AddDate(0, 0, -1), Close: 100.0},
			},
		},
		{
			name: "single bar",
			bars: []models.EODBar{
				{Date: today, Close: 50.0},
			},
		},
		{
			name: "empty bars",
			bars: []models.EODBar{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockEODHD{
				getEODFn: func(_ context.Context, ticker string, opts ...interfaces.EODOption) (*models.EODResponse, error) {
					return &models.EODResponse{Data: tt.bars}, nil
				},
			}

			svc, mgr := testMarketService(t, mock)
			ctx := testContext()

			ticker := fmt.Sprintf("FEW_%s.AU", tt.name[:3])
			require.NoError(t, mgr.StockIndexStore().Upsert(ctx, &models.StockIndexEntry{
				Ticker: ticker, Code: ticker[:3], Exchange: "AU", Source: "test", AddedAt: today.AddDate(0, 0, -30),
			}))

			err := svc.CollectMarketData(ctx, []string{ticker}, false, false)
			require.NoError(t, err)

			md, err := mgr.MarketDataStorage().GetMarketData(ctx, ticker)
			if len(tt.bars) == 0 {
				// No bars means no market data saved (or empty EOD)
				if err == nil && md != nil {
					assert.Empty(t, md.EOD, "empty bars should remain empty")
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, md)
				assert.Equal(t, len(tt.bars), len(md.EOD),
					"bars should be returned as-is with fewer than 3 bars")
			}
		})
	}

	guard.SaveResult("04_too_few_bars", "Bars with <3 entries returned as-is (no filtering)")
}
