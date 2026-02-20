package data

import (
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMarketData(ticker, exchange string) *models.MarketData {
	return &models.MarketData{
		Ticker:      ticker,
		Exchange:    exchange,
		Name:        ticker + " Ltd",
		LastUpdated: time.Now().Truncate(time.Second),
		EOD: []models.EODBar{
			{
				Date:     time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
				Open:     50.0,
				High:     52.0,
				Low:      49.0,
				Close:    51.0,
				AdjClose: 51.0,
				Volume:   500000,
			},
		},
	}
}

func newSignals(ticker string) *models.TickerSignals {
	return &models.TickerSignals{
		Ticker:           ticker,
		ComputeTimestamp: time.Now().Truncate(time.Second),
		Trend:            models.TrendBullish,
		TrendDescription: "Strong uptrend",
		Price: models.PriceSignals{
			Current: 51.0,
			SMA20:   49.5,
			SMA50:   47.0,
			SMA200:  43.0,
		},
	}
}

func TestMarketDataLifecycle(t *testing.T) {
	mgr := testManager(t)
	store := mgr.MarketDataStorage()
	ctx := testContext()

	data := newMarketData("BHP", "AU")

	// Save
	require.NoError(t, store.SaveMarketData(ctx, data))

	// Get
	got, err := store.GetMarketData(ctx, "BHP")
	require.NoError(t, err)
	assert.Equal(t, "BHP", got.Ticker)
	assert.Equal(t, "AU", got.Exchange)
	assert.Equal(t, "BHP Ltd", got.Name)
	assert.Len(t, got.EOD, 1)
	assert.Equal(t, 51.0, got.EOD[0].Close)

	// Update
	data.Name = "BHP Group"
	data.EOD = append(data.EOD, models.EODBar{
		Date:  time.Date(2025, 6, 2, 0, 0, 0, 0, time.UTC),
		Close: 52.5,
	})
	require.NoError(t, store.SaveMarketData(ctx, data))

	updated, err := store.GetMarketData(ctx, "BHP")
	require.NoError(t, err)
	assert.Equal(t, "BHP Group", updated.Name)
	assert.Len(t, updated.EOD, 2)

	// Not found
	_, err = store.GetMarketData(ctx, "NOEXIST")
	assert.Error(t, err)
}

func TestSignalLifecycle(t *testing.T) {
	mgr := testManager(t)
	store := mgr.SignalStorage()
	ctx := testContext()

	signals := newSignals("CBA")

	// Save
	require.NoError(t, store.SaveSignals(ctx, signals))

	// Get
	got, err := store.GetSignals(ctx, "CBA")
	require.NoError(t, err)
	assert.Equal(t, "CBA", got.Ticker)
	assert.Equal(t, models.TrendBullish, got.Trend)
	assert.Equal(t, 51.0, got.Price.Current)

	// Update
	signals.Trend = models.TrendBearish
	signals.TrendDescription = "Trend reversed"
	require.NoError(t, store.SaveSignals(ctx, signals))

	updated, err := store.GetSignals(ctx, "CBA")
	require.NoError(t, err)
	assert.Equal(t, models.TrendBearish, updated.Trend)

	// Not found
	_, err = store.GetSignals(ctx, "NOEXIST")
	assert.Error(t, err)
}

func TestBatchOperations(t *testing.T) {
	mgr := testManager(t)
	marketStore := mgr.MarketDataStorage()
	signalStore := mgr.SignalStorage()
	ctx := testContext()

	tickers := []string{"BATCH_A", "BATCH_B", "BATCH_C"}
	for _, ticker := range tickers {
		require.NoError(t, marketStore.SaveMarketData(ctx, newMarketData(ticker, "AU")))
		require.NoError(t, signalStore.SaveSignals(ctx, newSignals(ticker)))
	}

	t.Run("market data batch", func(t *testing.T) {
		results, err := marketStore.GetMarketDataBatch(ctx, tickers)
		require.NoError(t, err)
		assert.Len(t, results, 3)
	})

	t.Run("signals batch", func(t *testing.T) {
		results, err := signalStore.GetSignalsBatch(ctx, tickers)
		require.NoError(t, err)
		assert.Len(t, results, 3)
	})

	t.Run("partial batch", func(t *testing.T) {
		results, err := marketStore.GetMarketDataBatch(ctx, []string{"BATCH_A", "NONEXIST"})
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, "BATCH_A", results[0].Ticker)
	})

	t.Run("empty batch", func(t *testing.T) {
		results, err := marketStore.GetMarketDataBatch(ctx, []string{})
		require.NoError(t, err)
		assert.Nil(t, results)
	})
}

func TestStaleTickers(t *testing.T) {
	mgr := testManager(t)
	store := mgr.MarketDataStorage()
	ctx := testContext()

	// Stale ticker
	stale := newMarketData("STALE", "AU")
	stale.LastUpdated = time.Now().Add(-72 * time.Hour)
	require.NoError(t, store.SaveMarketData(ctx, stale))

	// Fresh ticker
	fresh := newMarketData("FRESH", "AU")
	fresh.LastUpdated = time.Now()
	require.NoError(t, store.SaveMarketData(ctx, fresh))

	// Different exchange
	other := newMarketData("OTHER", "US")
	other.LastUpdated = time.Now().Add(-72 * time.Hour)
	require.NoError(t, store.SaveMarketData(ctx, other))

	staleTickers, err := store.GetStaleTickers(ctx, "AU", 86400)
	require.NoError(t, err)
	assert.Contains(t, staleTickers, "STALE")
	assert.NotContains(t, staleTickers, "FRESH")
	assert.NotContains(t, staleTickers, "OTHER") // different exchange
}
