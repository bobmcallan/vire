package data

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/bobmcallan/vire/internal/services/cashflow"
	"github.com/bobmcallan/vire/internal/services/portfolio"
)

// TestTimelineDataIncludesCash verifies that GetDailyGrowth automatically loads
// cash transactions and produces correct portfolio_value and net_cash_balance
// in all historical data points (end-to-end integration test).
func TestTimelineDataIncludesCash(t *testing.T) {
	mgr := testManager(t)
	ctx := testContext()
	ctx = common.WithUserContext(ctx, &common.UserContext{UserID: "timeline_cash_user"})
	store := mgr.UserDataStore()

	// Create portfolio service
	portfolioSvc := portfolio.NewService(
		mgr,
		nil, // no market service needed for this test
		nil, // no signal computer needed
		nil, // no indicator computer needed
		common.NewSilentLogger(),
	)

	// Create portfolio with trades directly
	// Trade: buy 100 units at $100 on Jan 15, 2025 = $10,000
	trades := []*models.NavexaTrade{
		{
			ID:       "trade_001",
			Symbol:   "TEST",
			Type:     "buy",
			Date:     "2025-01-15",
			Units:    100,
			Price:    100,
			Fees:     0,
			Value:    10000,
			Currency: "AUD",
		},
	}

	holding := models.Holding{
		Ticker:           "TEST",
		Units:            100,
		AvgCost:          100,
		Trades:           trades,
		Currency:         "AUD",
		OriginalCurrency: "AUD",
		Status:           "open",
		CostBasis:        10000,
		GrossInvested:    10000,
	}

	// Create portfolio with holding
	portfolio := models.Portfolio{
		Name:             "TimelineCashTest",
		SourceType:       models.SourceManual,
		Currency:         "AUD",
		Holdings:         []models.Holding{holding},
		EquityValue:      10000,
		PortfolioValue:   10000,
		NetEquityCost:    10000,
		GrossCashBalance: 0,
		DataVersion:      common.SchemaVersion,
	}

	// Store portfolio
	portfolioData, err := json.Marshal(portfolio)
	require.NoError(t, err, "marshal portfolio")

	portfolioRecord := &models.UserRecord{
		UserID:   "timeline_cash_user",
		Subject:  "portfolio",
		Key:      "TimelineCashTest",
		Value:    string(portfolioData),
		Version:  1,
		DateTime: time.Now().Truncate(time.Second),
	}
	require.NoError(t, store.Put(ctx, portfolioRecord), "store portfolio with holding")

	// Create cash transactions: two deposits
	// Deposit 1: Jan 5, 2025 - $20,000
	// Deposit 2: Jan 20, 2025 - $5,000
	ledger := models.CashFlowLedger{
		PortfolioName: "TimelineCashTest",
		Version:       1,
		Accounts: []models.CashAccount{
			{
				Name:            "Trading",
				Type:            "trading",
				IsTransactional: true,
				Currency:        "AUD",
			},
		},
		Transactions: []models.CashTransaction{
			{
				ID:          "ct_deposit1",
				Account:     "Trading",
				Category:    models.CashCatContribution,
				Date:        time.Date(2025, 1, 5, 0, 0, 0, 0, time.UTC),
				Amount:      20000,
				Description: "Initial deposit",
				CreatedAt:   time.Now().Truncate(time.Second),
				UpdatedAt:   time.Now().Truncate(time.Second),
			},
			{
				ID:          "ct_deposit2",
				Account:     "Trading",
				Category:    models.CashCatContribution,
				Date:        time.Date(2025, 1, 20, 0, 0, 0, 0, time.UTC),
				Amount:      5000,
				Description: "Second deposit",
				CreatedAt:   time.Now().Truncate(time.Second),
				UpdatedAt:   time.Now().Truncate(time.Second),
			},
		},
		CreatedAt: time.Now().Truncate(time.Second),
		UpdatedAt: time.Now().Truncate(time.Second),
	}

	// Store cash ledger
	ledgerData, err := json.Marshal(ledger)
	require.NoError(t, err, "marshal ledger")

	ledgerRecord := &models.UserRecord{
		UserID:   "timeline_cash_user",
		Subject:  "cashflow",
		Key:      "TimelineCashTest",
		Value:    string(ledgerData),
		Version:  1,
		DateTime: time.Now().Truncate(time.Second),
	}
	require.NoError(t, store.Put(ctx, ledgerRecord), "store cash ledger")

	// Create CashFlowService
	cashflowSvc := cashflow.NewService(mgr, portfolioSvc, common.NewSilentLogger())
	portfolioSvc.SetCashFlowService(cashflowSvc)

	// Call GetDailyGrowth with EMPTY opts (opts.Transactions is nil)
	// This should trigger auto-loading of cash
	dailyPoints, err := portfolioSvc.GetDailyGrowth(ctx, "TimelineCashTest", interfaces.GrowthOptions{})
	require.NoError(t, err, "GetDailyGrowth with auto-load")
	require.NotEmpty(t, dailyPoints, "should return daily data points")

	// Verify that we have data points from Jan 5 (first cash) onwards
	require.Greater(t, len(dailyPoints), 0, "should have daily data points")

	// Check data point on Jan 5 (first cash transaction date)
	jan5Point := dailyPoints[0]
	assert.Equal(t, time.Date(2025, 1, 5, 0, 0, 0, 0, time.UTC), jan5Point.Date)
	assert.InDelta(t, 20000, jan5Point.NetCashBalance, 0.01,
		"Jan 5: net_cash_balance should be 20000 after first deposit")
	assert.InDelta(t, 0, jan5Point.EquityValue, 0.01,
		"Jan 5: equity_value should be 0 (no trades yet)")
	assert.InDelta(t, 20000, jan5Point.PortfolioValue, 0.01,
		"Jan 5: portfolio_value should equal net_cash_balance")

	// Verify relationship: portfolio_value = equity_value + net_cash_balance for all points
	for i, dp := range dailyPoints {
		expectedPortfolioValue := dp.EquityValue + dp.NetCashBalance
		assert.InDelta(t, expectedPortfolioValue, dp.PortfolioValue, 0.01,
			fmt.Sprintf("Point %d (%s): portfolio_value should equal equity + cash", i, dp.Date.Format("2006-01-02")))
	}
}

// TestGetDailyGrowth_AutoLoadsCash verifies that GetDailyGrowth auto-loads
// cash transactions when opts.Transactions is nil (not explicitly provided).
func TestGetDailyGrowth_AutoLoadsCash(t *testing.T) {
	mgr := testManager(t)
	ctx := testContext()
	ctx = common.WithUserContext(ctx, &common.UserContext{UserID: "auto_load_user"})
	store := mgr.UserDataStore()

	// Create portfolio service
	portfolioSvc := portfolio.NewService(
		mgr,
		nil,
		nil,
		nil,
		common.NewSilentLogger(),
	)

	// Create portfolio with trade
	trades := []*models.NavexaTrade{
		{
			ID:       "trade_002",
			Symbol:   "TEST",
			Type:     "buy",
			Date:     "2025-02-01",
			Units:    50,
			Price:    200,
			Fees:     0,
			Value:    10000,
			Currency: "AUD",
		},
	}

	holding := models.Holding{
		Ticker:           "TEST",
		Units:            50,
		AvgCost:          200,
		Trades:           trades,
		Currency:         "AUD",
		OriginalCurrency: "AUD",
		Status:           "open",
		CostBasis:        10000,
		GrossInvested:    10000,
	}

	portfolio := models.Portfolio{
		Name:             "AutoLoadTest",
		SourceType:       models.SourceManual,
		Currency:         "AUD",
		Holdings:         []models.Holding{holding},
		EquityValue:      10000,
		PortfolioValue:   10000,
		NetEquityCost:    10000,
		GrossCashBalance: 0,
		DataVersion:      common.SchemaVersion,
	}

	portfolioData, _ := json.Marshal(portfolio)
	_ = store.Put(ctx, &models.UserRecord{
		UserID:   "auto_load_user",
		Subject:  "portfolio",
		Key:      "AutoLoadTest",
		Value:    string(portfolioData),
		Version:  1,
		DateTime: time.Now().Truncate(time.Second),
	})

	// Create and store cash ledger
	ledger := models.CashFlowLedger{
		PortfolioName: "AutoLoadTest",
		Version:       1,
		Accounts: []models.CashAccount{
			{Name: "Trading", Type: "trading", IsTransactional: true, Currency: "AUD"},
		},
		Transactions: []models.CashTransaction{
			{
				ID:          "ct_auto_1",
				Account:     "Trading",
				Category:    models.CashCatContribution,
				Date:        time.Date(2025, 1, 27, 0, 0, 0, 0, time.UTC),
				Amount:      15000,
				Description: "Cash for trade",
				CreatedAt:   time.Now().Truncate(time.Second),
				UpdatedAt:   time.Now().Truncate(time.Second),
			},
		},
		CreatedAt: time.Now().Truncate(time.Second),
		UpdatedAt: time.Now().Truncate(time.Second),
	}

	ledgerData, _ := json.Marshal(ledger)
	_ = store.Put(ctx, &models.UserRecord{
		UserID:   "auto_load_user",
		Subject:  "cashflow",
		Key:      "AutoLoadTest",
		Value:    string(ledgerData),
		Version:  1,
		DateTime: time.Now().Truncate(time.Second),
	})

	// Wire cashflow service
	cashflowSvc := cashflow.NewService(mgr, portfolioSvc, common.NewSilentLogger())
	portfolioSvc.SetCashFlowService(cashflowSvc)

	// Call with empty options (auto-load should trigger)
	points, err := portfolioSvc.GetDailyGrowth(ctx, "AutoLoadTest", interfaces.GrowthOptions{})
	require.NoError(t, err)

	// Verify cash is included: net_cash_balance should be > 0 at start
	if len(points) > 0 {
		firstPoint := points[0]
		assert.Greater(t, firstPoint.NetCashBalance, 0.0,
			"First point should include cash from ledger (auto-loaded)")
		assert.InDelta(t, 15000, firstPoint.NetCashBalance, 0.01,
			"Cash balance on first point should match deposited amount")
	}
}

// TestGetDailyGrowth_ExplicitTransactionsOverride verifies that when
// opts.Transactions is explicitly provided (even if empty), auto-load is skipped.
func TestGetDailyGrowth_ExplicitTransactionsOverride(t *testing.T) {
	mgr := testManager(t)
	ctx := testContext()
	ctx = common.WithUserContext(ctx, &common.UserContext{UserID: "override_user"})
	store := mgr.UserDataStore()

	// Create portfolio service
	portfolioSvc := portfolio.NewService(
		mgr,
		nil,
		nil,
		nil,
		common.NewSilentLogger(),
	)

	// Create portfolio with trade
	trades := []*models.NavexaTrade{
		{
			ID:       "trade_003",
			Symbol:   "TEST",
			Type:     "buy",
			Date:     "2025-03-01",
			Units:    10,
			Price:    150,
			Fees:     0,
			Value:    1500,
			Currency: "AUD",
		},
	}

	holding := models.Holding{
		Ticker:           "TEST",
		Units:            10,
		AvgCost:          150,
		Trades:           trades,
		Currency:         "AUD",
		OriginalCurrency: "AUD",
		Status:           "open",
		CostBasis:        1500,
		GrossInvested:    1500,
	}

	portfolio := models.Portfolio{
		Name:             "OverrideTest",
		SourceType:       models.SourceManual,
		Currency:         "AUD",
		Holdings:         []models.Holding{holding},
		EquityValue:      1500,
		PortfolioValue:   1500,
		NetEquityCost:    1500,
		GrossCashBalance: 0,
		DataVersion:      common.SchemaVersion,
	}

	portfolioData, _ := json.Marshal(portfolio)
	_ = store.Put(ctx, &models.UserRecord{
		UserID:   "override_user",
		Subject:  "portfolio",
		Key:      "OverrideTest",
		Value:    string(portfolioData),
		Version:  1,
		DateTime: time.Now().Truncate(time.Second),
	})

	// Create cash ledger
	ledger := models.CashFlowLedger{
		PortfolioName: "OverrideTest",
		Version:       1,
		Accounts: []models.CashAccount{
			{Name: "Trading", Type: "trading", IsTransactional: true, Currency: "AUD"},
		},
		Transactions: []models.CashTransaction{
			{
				ID:          "ct_override_1",
				Account:     "Trading",
				Category:    models.CashCatContribution,
				Date:        time.Date(2025, 2, 24, 0, 0, 0, 0, time.UTC),
				Amount:      5000,
				Description: "Cash deposit",
				CreatedAt:   time.Now().Truncate(time.Second),
				UpdatedAt:   time.Now().Truncate(time.Second),
			},
		},
		CreatedAt: time.Now().Truncate(time.Second),
		UpdatedAt: time.Now().Truncate(time.Second),
	}

	ledgerData, _ := json.Marshal(ledger)
	_ = store.Put(ctx, &models.UserRecord{
		UserID:   "override_user",
		Subject:  "cashflow",
		Key:      "OverrideTest",
		Value:    string(ledgerData),
		Version:  1,
		DateTime: time.Now().Truncate(time.Second),
	})

	// Wire cashflow service
	cashflowSvc := cashflow.NewService(mgr, portfolioSvc, common.NewSilentLogger())
	portfolioSvc.SetCashFlowService(cashflowSvc)

	// Call with EXPLICIT empty transaction slice (not nil)
	// This should SKIP auto-load and use the provided empty slice
	opts := interfaces.GrowthOptions{
		Transactions: []models.CashTransaction{},
	}
	points, err := portfolioSvc.GetDailyGrowth(ctx, "OverrideTest", opts)
	require.NoError(t, err)

	// Verify cash is NOT included: first point should have zero net_cash_balance
	if len(points) > 0 {
		firstPoint := points[0]
		assert.InDelta(t, 0, firstPoint.NetCashBalance, 0.01,
			"Explicit empty transactions should override auto-load")
	}
}

// TestGetDailyGrowth_NoCashflowService verifies graceful degradation
// when cashflowSvc is nil (not available).
func TestGetDailyGrowth_NoCashflowService(t *testing.T) {
	mgr := testManager(t)
	ctx := testContext()
	ctx = common.WithUserContext(ctx, &common.UserContext{UserID: "no_cashflow_user"})
	store := mgr.UserDataStore()

	// Create portfolio service WITHOUT cashflow service
	portfolioSvc := portfolio.NewService(
		mgr,
		nil,
		nil,
		nil,
		common.NewSilentLogger(),
	)

	// Create portfolio with trade
	trades := []*models.NavexaTrade{
		{
			ID:       "trade_004",
			Symbol:   "TEST",
			Type:     "buy",
			Date:     "2025-04-01",
			Units:    5,
			Price:    100,
			Fees:     0,
			Value:    500,
			Currency: "AUD",
		},
	}

	holding := models.Holding{
		Ticker:           "TEST",
		Units:            5,
		AvgCost:          100,
		Trades:           trades,
		Currency:         "AUD",
		OriginalCurrency: "AUD",
		Status:           "open",
		CostBasis:        500,
		GrossInvested:    500,
	}

	portfolio := models.Portfolio{
		Name:             "NoCashflowTest",
		SourceType:       models.SourceManual,
		Currency:         "AUD",
		Holdings:         []models.Holding{holding},
		EquityValue:      500,
		PortfolioValue:   500,
		NetEquityCost:    500,
		GrossCashBalance: 0,
		DataVersion:      common.SchemaVersion,
	}

	portfolioData, _ := json.Marshal(portfolio)
	_ = store.Put(ctx, &models.UserRecord{
		UserID:   "no_cashflow_user",
		Subject:  "portfolio",
		Key:      "NoCashflowTest",
		Value:    string(portfolioData),
		Version:  1,
		DateTime: time.Now().Truncate(time.Second),
	})

	// Note: NOT calling portfolioSvc.SetCashFlowService()

	// Call GetDailyGrowth with empty options
	// Should gracefully degrade (no cash loaded)
	points, err := portfolioSvc.GetDailyGrowth(ctx, "NoCashflowTest", interfaces.GrowthOptions{})
	require.NoError(t, err, "should not fail even without cashflow service")

	// Verify graceful degradation: portfolio_value = equity_value (no cash)
	if len(points) > 0 {
		for _, dp := range points {
			assert.InDelta(t, 0, dp.NetCashBalance, 0.01,
				"Without cashflow service, net_cash_balance should be 0")
			assert.InDelta(t, dp.EquityValue, dp.PortfolioValue, 0.01,
				"Without cash, portfolio_value should equal equity_value")
		}
	}
}
