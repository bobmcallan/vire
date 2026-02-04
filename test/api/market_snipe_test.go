package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmccarthy/vire/internal/interfaces"
	"github.com/bobmccarthy/vire/internal/models"
	"github.com/bobmccarthy/vire/internal/services/market"
	"github.com/bobmccarthy/vire/test/common"
)

func TestMarketSnipe(t *testing.T) {
	env := common.SetupDockerTestEnvironment(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	ctx := env.Context()

	// Setup mock with exchange symbols
	mockEODHD := common.NewMockEODHDClient()
	mockEODHD.Symbols["AU"] = []*models.Symbol{
		{Code: "BHP", Name: "BHP Group", Exchange: "AU", Type: "Common Stock"},
		{Code: "RIO", Name: "Rio Tinto", Exchange: "AU", Type: "Common Stock"},
		{Code: "FMG", Name: "Fortescue Metals", Exchange: "AU", Type: "Common Stock"},
	}

	// Pre-populate market data with oversold conditions
	for _, sym := range mockEODHD.Symbols["AU"] {
		ticker := sym.Code + ".AU"
		bars := generateOversoldBars()
		err := env.StorageManager.MarketDataStorage().SaveMarketData(ctx, &models.MarketData{
			Ticker:   ticker,
			Exchange: "AU",
			Name:     sym.Name,
			EOD:      bars,
			Fundamentals: &models.Fundamentals{
				Ticker:    ticker,
				PE:        12.0,
				MarketCap: 50000000000,
				Sector:    "Materials",
			},
		})
		require.NoError(t, err)
	}

	// Create market service
	svc := market.NewService(
		env.StorageManager,
		mockEODHD,
		common.NewMockGeminiClient(),
		env.Logger,
	)

	// Find snipe buys
	snipeBuys, err := svc.FindSnipeBuys(ctx, interfaces.SnipeOptions{
		Exchange: "AU",
		Limit:    3,
	})
	require.NoError(t, err)

	// Validate results
	assert.LessOrEqual(t, len(snipeBuys), 3)

	// Check snipe buy structure
	for _, buy := range snipeBuys {
		assert.NotEmpty(t, buy.Ticker)
		assert.NotEmpty(t, buy.Name)
		assert.Greater(t, buy.Score, 0.0)
		assert.Greater(t, buy.UpsidePct, 0.0)
	}
}

func TestMarketSnipeWithSectorFilter(t *testing.T) {
	env := common.SetupDockerTestEnvironment(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	ctx := env.Context()

	mockEODHD := common.NewMockEODHDClient()
	mockEODHD.Symbols["AU"] = []*models.Symbol{
		{Code: "BHP", Name: "BHP Group", Exchange: "AU", Type: "Common Stock"},
	}

	svc := market.NewService(
		env.StorageManager,
		mockEODHD,
		nil, // No Gemini for this test
		env.Logger,
	)

	// Find snipe buys with sector filter
	snipeBuys, err := svc.FindSnipeBuys(ctx, interfaces.SnipeOptions{
		Exchange: "AU",
		Limit:    5,
		Sector:   "Technology",
	})
	require.NoError(t, err)

	// With sector filter that doesn't match, should get empty results
	assert.Empty(t, snipeBuys)
}

func TestCollectMarketData(t *testing.T) {
	env := common.SetupDockerTestEnvironment(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	ctx := env.Context()

	mockEODHD := common.NewMockEODHDClient()
	svc := market.NewService(
		env.StorageManager,
		mockEODHD,
		nil,
		env.Logger,
	)

	// Collect data for tickers
	tickers := []string{"BHP.AU", "CBA.AU"}
	err := svc.CollectMarketData(ctx, tickers, false)
	require.NoError(t, err)

	// Verify data was collected
	assert.Equal(t, 2, mockEODHD.GetEODCalls)
	assert.Equal(t, 2, mockEODHD.GetFundCalls)

	// Verify data is in storage
	for _, ticker := range tickers {
		data, err := env.StorageManager.MarketDataStorage().GetMarketData(ctx, ticker)
		require.NoError(t, err)
		assert.NotNil(t, data)
		assert.Equal(t, ticker, data.Ticker)
	}
}

func TestGetStockData(t *testing.T) {
	env := common.SetupDockerTestEnvironment(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	ctx := env.Context()

	mockEODHD := common.NewMockEODHDClient()
	svc := market.NewService(
		env.StorageManager,
		mockEODHD,
		nil,
		env.Logger,
	)

	// Get stock data (will collect if not present)
	stockData, err := svc.GetStockData(ctx, "BHP.AU", interfaces.StockDataInclude{
		Price:        true,
		Fundamentals: true,
		Signals:      true,
		News:         false,
	})
	require.NoError(t, err)

	assert.Equal(t, "BHP.AU", stockData.Ticker)
	assert.NotNil(t, stockData.Price)
	assert.NotNil(t, stockData.Fundamentals)
	assert.NotNil(t, stockData.Signals)
}

// Helper to generate oversold price bars
func generateOversoldBars() []models.EODBar {
	bars := make([]models.EODBar, 365)
	price := 50.0

	for i := 364; i >= 0; i-- {
		// Simulate downtrend for oversold RSI
		if i > 14 {
			price -= 0.1
		}
		bars[364-i] = models.EODBar{
			Open:     price + 0.5,
			High:     price + 1.0,
			Low:      price - 0.5,
			Close:    price,
			AdjClose: price,
			Volume:   500000,
		}
	}
	return bars
}
