package surrealdb

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestMarketData(ticker, exchange string) *models.MarketData {
	return &models.MarketData{
		Ticker:      ticker,
		Exchange:    exchange,
		Name:        ticker + " Corp",
		LastUpdated: time.Now().Truncate(time.Second),
		EOD: []models.EODBar{
			{
				Date:     time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				Open:     100.0,
				High:     105.0,
				Low:      99.0,
				Close:    103.0,
				AdjClose: 103.0,
				Volume:   1000000,
			},
		},
	}
}

func newTestSignals(ticker string) *models.TickerSignals {
	return &models.TickerSignals{
		Ticker:           ticker,
		ComputeTimestamp: time.Now().Truncate(time.Second),
		Trend:            models.TrendBullish,
		TrendDescription: "Bullish trend",
		Price: models.PriceSignals{
			Current:   50.0,
			Change:    1.5,
			ChangePct: 3.1,
			SMA20:     48.0,
			SMA50:     46.0,
			SMA200:    42.0,
		},
		Technical: models.TechnicalSignals{
			RSI:       55.0,
			RSISignal: "neutral",
		},
	}
}

func TestGetMarketData(t *testing.T) {
	db := testDB(t)
	dataPath := t.TempDir()
	store := NewMarketStore(db, testLogger(), dataPath)
	ctx := context.Background()

	data := newTestMarketData("BHP", "AU")
	require.NoError(t, store.SaveMarketData(ctx, data))

	got, err := store.GetMarketData(ctx, "BHP")
	require.NoError(t, err)
	assert.Equal(t, "BHP", got.Ticker)
	assert.Equal(t, "AU", got.Exchange)
	assert.Equal(t, "BHP Corp", got.Name)
	assert.Len(t, got.EOD, 1)
}

func TestGetMarketDataNotFound(t *testing.T) {
	db := testDB(t)
	dataPath := t.TempDir()
	store := NewMarketStore(db, testLogger(), dataPath)
	ctx := context.Background()

	_, err := store.GetMarketData(ctx, "NONEXIST")
	assert.Error(t, err)
}

func TestSaveMarketData(t *testing.T) {
	db := testDB(t)
	dataPath := t.TempDir()
	store := NewMarketStore(db, testLogger(), dataPath)
	ctx := context.Background()

	tests := []struct {
		ticker   string
		exchange string
	}{
		{"CBA", "AU"},
		{"AAPL", "US"},
		{"MSFT", "US"},
	}

	for _, tt := range tests {
		t.Run(tt.ticker, func(t *testing.T) {
			data := newTestMarketData(tt.ticker, tt.exchange)
			require.NoError(t, store.SaveMarketData(ctx, data))

			got, err := store.GetMarketData(ctx, tt.ticker)
			require.NoError(t, err)
			assert.Equal(t, tt.ticker, got.Ticker)
			assert.Equal(t, tt.exchange, got.Exchange)
		})
	}
}

func TestSaveMarketDataOverwrite(t *testing.T) {
	db := testDB(t)
	dataPath := t.TempDir()
	store := NewMarketStore(db, testLogger(), dataPath)
	ctx := context.Background()

	data := newTestMarketData("OVR", "AU")
	require.NoError(t, store.SaveMarketData(ctx, data))

	data.Name = "Updated Name"
	data.EOD = append(data.EOD, models.EODBar{
		Date:  time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
		Close: 105.0,
	})
	require.NoError(t, store.SaveMarketData(ctx, data))

	got, err := store.GetMarketData(ctx, "OVR")
	require.NoError(t, err)
	assert.Equal(t, "Updated Name", got.Name)
	assert.Len(t, got.EOD, 2)
}

func TestGetMarketDataBatch(t *testing.T) {
	db := testDB(t)
	dataPath := t.TempDir()
	store := NewMarketStore(db, testLogger(), dataPath)
	ctx := context.Background()

	for _, ticker := range []string{"BATCH1", "BATCH2", "BATCH3"} {
		require.NoError(t, store.SaveMarketData(ctx, newTestMarketData(ticker, "AU")))
	}

	results, err := store.GetMarketDataBatch(ctx, []string{"BATCH1", "BATCH2", "BATCH3"})
	require.NoError(t, err)
	assert.Len(t, results, 3)

	tickers := make(map[string]bool)
	for _, r := range results {
		tickers[r.Ticker] = true
	}
	assert.True(t, tickers["BATCH1"])
	assert.True(t, tickers["BATCH2"])
	assert.True(t, tickers["BATCH3"])
}

func TestGetMarketDataBatchEmpty(t *testing.T) {
	db := testDB(t)
	dataPath := t.TempDir()
	store := NewMarketStore(db, testLogger(), dataPath)
	ctx := context.Background()

	results, err := store.GetMarketDataBatch(ctx, []string{})
	require.NoError(t, err)
	assert.Nil(t, results)
}

func TestGetStaleTickers(t *testing.T) {
	db := testDB(t)
	dataPath := t.TempDir()
	store := NewMarketStore(db, testLogger(), dataPath)
	ctx := context.Background()

	// Save a "stale" ticker (updated long ago)
	staleData := newTestMarketData("STALE1", "AU")
	staleData.LastUpdated = time.Now().Add(-48 * time.Hour)
	require.NoError(t, store.SaveMarketData(ctx, staleData))

	// Save a "fresh" ticker (just updated)
	freshData := newTestMarketData("FRESH1", "AU")
	freshData.LastUpdated = time.Now()
	require.NoError(t, store.SaveMarketData(ctx, freshData))

	// Get tickers stale by more than 24 hours
	stale, err := store.GetStaleTickers(ctx, "AU", 86400) // 24h in seconds
	require.NoError(t, err)
	assert.Contains(t, stale, "STALE1")
	assert.NotContains(t, stale, "FRESH1")
}

func TestGetSignals(t *testing.T) {
	db := testDB(t)
	dataPath := t.TempDir()
	store := NewMarketStore(db, testLogger(), dataPath)
	ctx := context.Background()

	signals := newTestSignals("SIG1")
	require.NoError(t, store.SaveSignals(ctx, signals))

	got, err := store.GetSignals(ctx, "SIG1")
	require.NoError(t, err)
	assert.Equal(t, "SIG1", got.Ticker)
	assert.Equal(t, models.TrendBullish, got.Trend)
	assert.Equal(t, 50.0, got.Price.Current)
	assert.Equal(t, 55.0, got.Technical.RSI)
}

func TestGetSignalsNotFound(t *testing.T) {
	db := testDB(t)
	dataPath := t.TempDir()
	store := NewMarketStore(db, testLogger(), dataPath)
	ctx := context.Background()

	_, err := store.GetSignals(ctx, "NOSIG")
	assert.Error(t, err)
}

func TestSaveSignals(t *testing.T) {
	db := testDB(t)
	dataPath := t.TempDir()
	store := NewMarketStore(db, testLogger(), dataPath)
	ctx := context.Background()

	signals := newTestSignals("SAVESIG")
	err := store.SaveSignals(ctx, signals)
	require.NoError(t, err)

	got, err := store.GetSignals(ctx, "SAVESIG")
	require.NoError(t, err)
	assert.Equal(t, "SAVESIG", got.Ticker)
}

func TestGetSignalsBatch(t *testing.T) {
	db := testDB(t)
	dataPath := t.TempDir()
	store := NewMarketStore(db, testLogger(), dataPath)
	ctx := context.Background()

	for _, ticker := range []string{"SIGB1", "SIGB2", "SIGB3"} {
		require.NoError(t, store.SaveSignals(ctx, newTestSignals(ticker)))
	}

	results, err := store.GetSignalsBatch(ctx, []string{"SIGB1", "SIGB2", "SIGB3"})
	require.NoError(t, err)
	assert.Len(t, results, 3)
}

func TestGetSignalsBatchEmpty(t *testing.T) {
	db := testDB(t)
	dataPath := t.TempDir()
	store := NewMarketStore(db, testLogger(), dataPath)
	ctx := context.Background()

	results, err := store.GetSignalsBatch(ctx, []string{})
	require.NoError(t, err)
	assert.Nil(t, results)
}

func TestPurgeMarketData(t *testing.T) {
	db := testDB(t)
	dataPath := t.TempDir()
	store := NewMarketStore(db, testLogger(), dataPath)
	ctx := context.Background()

	for _, ticker := range []string{"P1", "P2"} {
		require.NoError(t, store.SaveMarketData(ctx, newTestMarketData(ticker, "AU")))
	}

	count, err := store.PurgeMarketData(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	_, err = store.GetMarketData(ctx, "P1")
	assert.Error(t, err)
}

func TestPurgeSignalsData(t *testing.T) {
	db := testDB(t)
	dataPath := t.TempDir()
	store := NewMarketStore(db, testLogger(), dataPath)
	ctx := context.Background()

	for _, ticker := range []string{"PS1", "PS2", "PS3"} {
		require.NoError(t, store.SaveSignals(ctx, newTestSignals(ticker)))
	}

	count, err := store.PurgeSignalsData(ctx)
	require.NoError(t, err)
	assert.Equal(t, 3, count)

	_, err = store.GetSignals(ctx, "PS1")
	assert.Error(t, err)
}

func TestPurgeCharts(t *testing.T) {
	dataPath := t.TempDir()
	db := testDB(t)
	store := NewMarketStore(db, testLogger(), dataPath)

	// Create charts directory with some files
	chartsDir := filepath.Join(dataPath, "charts")
	require.NoError(t, os.MkdirAll(chartsDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(chartsDir, "chart1.png"), []byte("png data"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(chartsDir, "chart2.png"), []byte("png data"), 0644))

	count, err := store.PurgeCharts()
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	entries, err := os.ReadDir(chartsDir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestPurgeChartsNonExistentDir(t *testing.T) {
	dataPath := t.TempDir()
	db := testDB(t)
	store := NewMarketStore(db, testLogger(), dataPath)

	count, err := store.PurgeCharts()
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}
