package api

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmccarthy/vire/internal/interfaces"
	"github.com/bobmccarthy/vire/internal/models"
	"github.com/bobmccarthy/vire/internal/services/portfolio"
	"github.com/bobmccarthy/vire/test/common"
)

func TestPortfolioReview(t *testing.T) {
	env := common.SetupDockerTestEnvironment(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	ctx := env.Context()

	// Setup mock clients
	mockNavexa := common.NewMockNavexaClient()
	mockNavexa.Portfolios = []models.NavexaPortfolio{
		{ID: "test-1", Name: "TestPortfolio", Currency: "AUD", TotalValue: 100000},
	}
	mockNavexa.Holdings["test-1"] = []*models.NavexaHolding{
		{Ticker: "BHP", Exchange: "AU", Name: "BHP Group", Units: 100, CurrentPrice: 45.0, MarketValue: 4500},
		{Ticker: "CBA", Exchange: "AU", Name: "Commonwealth Bank", Units: 50, CurrentPrice: 100.0, MarketValue: 5000},
	}

	mockEODHD := common.NewMockEODHDClient()
	mockGemini := common.NewMockGeminiClient()

	// Create portfolio service
	svc := portfolio.NewService(
		env.StorageManager,
		mockNavexa,
		mockEODHD,
		mockGemini,
		env.Logger,
	)

	// Sync portfolio first
	syncedPortfolio, err := svc.SyncPortfolio(ctx, "TestPortfolio", true)
	require.NoError(t, err)
	assert.Equal(t, "TestPortfolio", syncedPortfolio.Name)
	assert.Len(t, syncedPortfolio.Holdings, 2)

	// Setup market data for holdings
	for _, holding := range syncedPortfolio.Holdings {
		ticker := holding.Ticker + "." + holding.Exchange
		err := env.StorageManager.MarketDataStorage().SaveMarketData(ctx, &models.MarketData{
			Ticker:      ticker,
			Exchange:    holding.Exchange,
			Name:        holding.Name,
			EOD:         common.NewMockEODHDClient().EODData[ticker].Data,
			LastUpdated: time.Now(),
		})
		// Ignore errors for missing mock data - will use defaults
		_ = err
	}

	// Review portfolio
	review, err := svc.ReviewPortfolio(ctx, "TestPortfolio", interfaces.ReviewOptions{
		IncludeNews: false,
	})
	require.NoError(t, err)

	// Validate review
	assert.Equal(t, "TestPortfolio", review.PortfolioName)
	assert.NotZero(t, review.ReviewDate)

	// Save results
	guard := common.NewTestOutputGuard(t)
	guard.AssertNotContains(review.Summary, "error")
}

func TestPortfolioReviewWithSignalFocus(t *testing.T) {
	env := common.SetupDockerTestEnvironment(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	ctx := env.Context()

	// Setup minimal test data
	mockNavexa := common.NewMockNavexaClient()
	mockNavexa.Portfolios = []models.NavexaPortfolio{
		{ID: "test-1", Name: "SMSF", Currency: "AUD", TotalValue: 50000},
	}
	mockNavexa.Holdings["test-1"] = []*models.NavexaHolding{
		{Ticker: "CSL", Exchange: "AU", Name: "CSL Limited", Units: 20, CurrentPrice: 280.0, MarketValue: 5600},
	}

	svc := portfolio.NewService(
		env.StorageManager,
		mockNavexa,
		common.NewMockEODHDClient(),
		common.NewMockGeminiClient(),
		env.Logger,
	)

	// Sync and review with signal focus
	_, err := svc.SyncPortfolio(ctx, "SMSF", true)
	require.NoError(t, err)

	review, err := svc.ReviewPortfolio(ctx, "SMSF", interfaces.ReviewOptions{
		FocusSignals: []string{"rsi", "sma"},
		IncludeNews:  false,
	})
	require.NoError(t, err)
	assert.NotNil(t, review)
}

func TestListPortfolios(t *testing.T) {
	env := common.SetupDockerTestEnvironment(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	ctx := env.Context()

	// Save some portfolios directly
	err := env.StorageManager.PortfolioStorage().SavePortfolio(ctx, &models.Portfolio{
		Name: "Portfolio1",
	})
	require.NoError(t, err)

	err = env.StorageManager.PortfolioStorage().SavePortfolio(ctx, &models.Portfolio{
		Name: "Portfolio2",
	})
	require.NoError(t, err)

	// List portfolios
	svc := portfolio.NewService(
		env.StorageManager,
		common.NewMockNavexaClient(),
		common.NewMockEODHDClient(),
		common.NewMockGeminiClient(),
		env.Logger,
	)

	portfolios, err := svc.ListPortfolios(ctx)
	require.NoError(t, err)
	assert.Len(t, portfolios, 2)
}
