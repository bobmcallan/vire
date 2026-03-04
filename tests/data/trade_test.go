package data

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/bobmcallan/vire/internal/services/portfolio"
	"github.com/bobmcallan/vire/internal/services/trade"
)

// --- Portfolio Creation Tests ---

// TestCreateManualPortfolio verifies that a manual portfolio can be created
// and includes source_type in the response.
func TestCreateManualPortfolio(t *testing.T) {
	mgr := testManager(t)
	store := mgr.UserDataStore()
	ctx := testContext()

	portfolioSvc := portfolio.NewService(mgr, nil, nil, nil, common.NewLogger("error"))
	ctx = context.WithValue(ctx, "user_id", "create_manual_user")

	// Create manual portfolio
	created, err := portfolioSvc.CreatePortfolio(ctx, "ManualTest", models.SourceManual, "AUD")
	require.NoError(t, err, "create manual portfolio")
	assert.Equal(t, "ManualTest", created.Name)
	assert.Equal(t, models.SourceManual, created.SourceType)
	assert.Equal(t, "AUD", created.Currency)

	// Retrieve from store and verify persistence
	record, err := store.Get(ctx, "create_manual_user", "portfolio", "ManualTest")
	require.NoError(t, err, "retrieve portfolio from store")
	require.NotNil(t, record)

	var retrieved models.Portfolio
	require.NoError(t, json.Unmarshal([]byte(record.Value), &retrieved), "unmarshal portfolio")
	assert.Equal(t, models.SourceManual, retrieved.SourceType)
	assert.Equal(t, "ManualTest", retrieved.Name)
}

// TestCreateSnapshotPortfolio verifies that a snapshot portfolio can be created
// with correct source_type.
func TestCreateSnapshotPortfolio(t *testing.T) {
	mgr := testManager(t)
	ctx := testContext()

	portfolioSvc := portfolio.NewService(mgr, nil, nil, nil, common.NewLogger("error"))
	ctx = context.WithValue(ctx, "user_id", "create_snapshot_user")

	// Create snapshot portfolio
	created, err := portfolioSvc.CreatePortfolio(ctx, "SnapshotTest", models.SourceSnapshot, "USD")
	require.NoError(t, err, "create snapshot portfolio")
	assert.Equal(t, "SnapshotTest", created.Name)
	assert.Equal(t, models.SourceSnapshot, created.SourceType)
	assert.Equal(t, "USD", created.Currency)
}

// TestCreatePortfolioValidation verifies validation of portfolio creation parameters.
func TestCreatePortfolioValidation(t *testing.T) {
	mgr := testManager(t)
	ctx := testContext()

	portfolioSvc := portfolio.NewService(mgr, nil, nil, nil, common.NewLogger("error"))
	ctx = context.WithValue(ctx, "user_id", "validation_user")

	tests := []struct {
		name       string
		portfolio  string
		sourceType models.SourceType
		currency   string
		expectErr  bool
	}{
		{
			name:       "valid manual",
			portfolio:  "ValidManual",
			sourceType: models.SourceManual,
			currency:   "AUD",
			expectErr:  false,
		},
		{
			name:       "empty portfolio name",
			portfolio:  "",
			sourceType: models.SourceManual,
			currency:   "AUD",
			expectErr:  true,
		},
		{
			name:       "invalid source type",
			portfolio:  "InvalidSource",
			sourceType: models.SourceType("unknown"),
			currency:   "AUD",
			expectErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := portfolioSvc.CreatePortfolio(ctx, tt.portfolio, tt.sourceType, tt.currency)
			if tt.expectErr {
				assert.Error(t, err, "expected error for %s", tt.name)
			} else {
				assert.NoError(t, err, "expected success for %s", tt.name)
			}
		})
	}
}

// --- Trade Add Tests ---

// TestAddTradeBuy verifies that a buy trade is recorded and holding is derived correctly.
func TestAddTradeBuy(t *testing.T) {
	mgr := testManager(t)
	ctx := testContext()

	tradeSvc := trade.NewService(mgr, common.NewLogger("error"))
	portfolioName := "BuyTradeTest"

	// Add first buy trade
	buyTrade := models.Trade{
		Ticker: "BHP.AU",
		Action: models.TradeActionBuy,
		Date:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Units:  100,
		Price:  50.0,
		Fees:   50.0,
		Notes:  "Initial purchase",
	}

	added, holding, err := tradeSvc.AddTrade(ctx, portfolioName, buyTrade)
	require.NoError(t, err, "add buy trade")
	require.NotNil(t, added)
	require.NotNil(t, holding)

	// Verify trade ID was generated
	assert.NotEmpty(t, added.ID)
	assert.True(t, len(added.ID) > 0)

	// Verify holding derivation
	assert.Equal(t, "BHP.AU", holding.Ticker)
	assert.Equal(t, 100.0, holding.Units)
	expectedCostBasis := (100 * 50.0) + 50.0 // units * price + fees
	assert.InDelta(t, expectedCostBasis, holding.CostBasis, 0.01)
	assert.InDelta(t, 50.5, holding.AvgCost, 0.01) // (5050 / 100)
	assert.InDelta(t, 5050.0, holding.GrossInvested, 0.01)
}

// TestAddTradeSell verifies that a sell trade reduces position and computes realized P&L.
func TestAddTradeSell(t *testing.T) {
	mgr := testManager(t)
	ctx := testContext()

	tradeSvc := trade.NewService(mgr, common.NewLogger("error"))
	portfolioName := "SellTradeTest"

	// Add buy trade first
	buyTrade := models.Trade{
		Ticker: "CBA.AU",
		Action: models.TradeActionBuy,
		Date:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Units:  100,
		Price:  100.0,
		Fees:   100.0,
	}
	_, _, err := tradeSvc.AddTrade(ctx, portfolioName, buyTrade)
	require.NoError(t, err)

	// Add sell trade
	sellTrade := models.Trade{
		Ticker: "CBA.AU",
		Action: models.TradeActionSell,
		Date:   time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
		Units:  50,
		Price:  110.0,
		Fees:   50.0,
	}
	addedSell, holdingAfterSell, err := tradeSvc.AddTrade(ctx, portfolioName, sellTrade)
	require.NoError(t, err, "add sell trade")
	require.NotNil(t, addedSell)
	require.NotNil(t, holdingAfterSell)

	// Verify trade was added
	assert.NotEmpty(t, addedSell.ID)

	// Verify holding after sell
	assert.Equal(t, 50.0, holdingAfterSell.Units)
	// Remaining cost basis: (100 * 100 + 100) - (50 * 101.0) = 10100 - 5050 = 5050
	assert.InDelta(t, 5050.0, holdingAfterSell.CostBasis, 0.01)
	// Realized P&L: (50 * 110 - 50) - (50 * 101.0) = 5450 - 5050 = 400
	assert.InDelta(t, 400.0, holdingAfterSell.RealizedReturn, 0.01)
}

// TestAddTradeSellValidation verifies that selling more than held is rejected.
func TestAddTradeSellValidation(t *testing.T) {
	mgr := testManager(t)
	ctx := testContext()

	tradeSvc := trade.NewService(mgr, common.NewLogger("error"))
	portfolioName := "SellValidationTest"

	// Add buy trade
	buyTrade := models.Trade{
		Ticker: "NAB.AU",
		Action: models.TradeActionBuy,
		Date:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Units:  50,
		Price:  100.0,
		Fees:   50.0,
	}
	_, _, err := tradeSvc.AddTrade(ctx, portfolioName, buyTrade)
	require.NoError(t, err)

	// Try to sell more than held
	sellTrade := models.Trade{
		Ticker: "NAB.AU",
		Action: models.TradeActionSell,
		Date:   time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
		Units:  100, // more than 50 held
		Price:  110.0,
		Fees:   50.0,
	}
	_, _, err = tradeSvc.AddTrade(ctx, portfolioName, sellTrade)
	assert.Error(t, err, "expected error when selling more than held")
}

// TestAddTradeValidation verifies validation of trade parameters.
func TestAddTradeValidation(t *testing.T) {
	mgr := testManager(t)
	ctx := testContext()

	tradeSvc := trade.NewService(mgr, common.NewLogger("error"))
	portfolioName := "ValidationTest"

	tests := []struct {
		name      string
		trade     models.Trade
		expectErr bool
	}{
		{
			name: "valid buy",
			trade: models.Trade{
				Ticker: "BHP.AU",
				Action: models.TradeActionBuy,
				Date:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				Units:  100,
				Price:  50.0,
			},
			expectErr: false,
		},
		{
			name: "empty ticker",
			trade: models.Trade{
				Ticker: "",
				Action: models.TradeActionBuy,
				Date:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				Units:  100,
				Price:  50.0,
			},
			expectErr: true,
		},
		{
			name: "zero units",
			trade: models.Trade{
				Ticker: "BHP.AU",
				Action: models.TradeActionBuy,
				Date:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				Units:  0,
				Price:  50.0,
			},
			expectErr: true,
		},
		{
			name: "negative units",
			trade: models.Trade{
				Ticker: "BHP.AU",
				Action: models.TradeActionBuy,
				Date:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				Units:  -100,
				Price:  50.0,
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := tradeSvc.AddTrade(ctx, portfolioName, tt.trade)
			if tt.expectErr {
				assert.Error(t, err, "expected error for %s", tt.name)
			} else {
				assert.NoError(t, err, "expected success for %s", tt.name)
			}
		})
	}
}

// --- Multiple Trades Tests ---

// TestMultipleBuyTrades verifies weighted average cost calculation.
func TestMultipleBuyTrades(t *testing.T) {
	mgr := testManager(t)
	ctx := testContext()

	tradeSvc := trade.NewService(mgr, common.NewLogger("error"))
	portfolioName := "MultipleBuysTest"

	// Buy 1: 100 @ 50
	trade1 := models.Trade{
		Ticker: "ANZ.AU",
		Action: models.TradeActionBuy,
		Date:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Units:  100,
		Price:  50.0,
		Fees:   0,
	}
	_, holding1, err := tradeSvc.AddTrade(ctx, portfolioName, trade1)
	require.NoError(t, err)
	assert.Equal(t, 100.0, holding1.Units)
	assert.InDelta(t, 50.0, holding1.AvgCost, 0.01)

	// Buy 2: 50 @ 60
	trade2 := models.Trade{
		Ticker: "ANZ.AU",
		Action: models.TradeActionBuy,
		Date:   time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
		Units:  50,
		Price:  60.0,
		Fees:   0,
	}
	_, holding2, err := tradeSvc.AddTrade(ctx, portfolioName, trade2)
	require.NoError(t, err)
	assert.Equal(t, 150.0, holding2.Units)
	// Weighted average: (100*50 + 50*60) / 150 = 8000/150 = 53.33
	assert.InDelta(t, 53.33, holding2.AvgCost, 0.01)
	assert.InDelta(t, 8000.0, holding2.CostBasis, 0.01)
}

// TestBuyThenPartialSell verifies realized P&L on partial sell.
func TestBuyThenPartialSell(t *testing.T) {
	mgr := testManager(t)
	ctx := testContext()

	tradeSvc := trade.NewService(mgr, common.NewLogger("error"))
	portfolioName := "PartialSellTest"

	// Buy 100 @ 100
	buyTrade := models.Trade{
		Ticker: "WBC.AU",
		Action: models.TradeActionBuy,
		Date:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Units:  100,
		Price:  100.0,
		Fees:   0,
	}
	_, _, err := tradeSvc.AddTrade(ctx, portfolioName, buyTrade)
	require.NoError(t, err)

	// Sell 60 @ 120
	sellTrade := models.Trade{
		Ticker: "WBC.AU",
		Action: models.TradeActionSell,
		Date:   time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
		Units:  60,
		Price:  120.0,
		Fees:   0,
	}
	_, holding, err := tradeSvc.AddTrade(ctx, portfolioName, sellTrade)
	require.NoError(t, err)

	// After partial sell
	assert.Equal(t, 40.0, holding.Units) // 100 - 60
	// Cost basis of remaining: 100 * 100 - (60 * 100) = 4000
	assert.InDelta(t, 4000.0, holding.CostBasis, 0.01)
	// Realized: (60 * 120) - (60 * 100) = 1200
	assert.InDelta(t, 1200.0, holding.RealizedReturn, 0.01)
	// Gross proceeds: 60 * 120 = 7200
	assert.InDelta(t, 7200.0, holding.GrossProceeds, 0.01)
}

// TestFullSellClosed verifies that selling all units closes the position.
func TestFullSellClosed(t *testing.T) {
	mgr := testManager(t)
	ctx := testContext()

	tradeSvc := trade.NewService(mgr, common.NewLogger("error"))
	portfolioName := "FullSellTest"

	// Buy 100 @ 50
	buyTrade := models.Trade{
		Ticker: "FMG.AU",
		Action: models.TradeActionBuy,
		Date:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Units:  100,
		Price:  50.0,
		Fees:   0,
	}
	_, _, err := tradeSvc.AddTrade(ctx, portfolioName, buyTrade)
	require.NoError(t, err)

	// Sell all 100 @ 75
	sellTrade := models.Trade{
		Ticker: "FMG.AU",
		Action: models.TradeActionSell,
		Date:   time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
		Units:  100,
		Price:  75.0,
		Fees:   0,
	}
	_, holding, err := tradeSvc.AddTrade(ctx, portfolioName, sellTrade)
	require.NoError(t, err)

	// Position should be fully realized
	assert.Equal(t, 0.0, holding.Units)
	assert.Equal(t, 0.0, holding.CostBasis)
	assert.InDelta(t, 2500.0, holding.RealizedReturn, 0.01) // (100*75) - (100*50) = 2500
	assert.Equal(t, 0.0, holding.UnrealizedReturn)
}

// --- Trade CRUD Tests ---

// TestRemoveTrade verifies that a trade can be removed and position recalculated.
func TestRemoveTrade(t *testing.T) {
	mgr := testManager(t)
	ctx := testContext()

	tradeSvc := trade.NewService(mgr, common.NewLogger("error"))
	portfolioName := "RemoveTradeTest"

	// Add two buy trades
	trade1 := models.Trade{
		Ticker: "MQG.AU",
		Action: models.TradeActionBuy,
		Date:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Units:  100,
		Price:  50.0,
	}
	added1, _, err := tradeSvc.AddTrade(ctx, portfolioName, trade1)
	require.NoError(t, err)

	trade2 := models.Trade{
		Ticker: "MQG.AU",
		Action: models.TradeActionBuy,
		Date:   time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
		Units:  50,
		Price:  60.0,
	}
	added2, _, err := tradeSvc.AddTrade(ctx, portfolioName, trade2)
	require.NoError(t, err)

	// Remove first trade
	book, err := tradeSvc.RemoveTrade(ctx, portfolioName, added1.ID)
	require.NoError(t, err, "remove trade")
	require.NotNil(t, book)

	// Verify only second trade remains
	remaining := book.TradesForTicker("MQG.AU")
	assert.Len(t, remaining, 1)
	assert.Equal(t, added2.ID, remaining[0].ID)
}

// TestRemoveTradeNotFound verifies error when removing nonexistent trade.
func TestRemoveTradeNotFound(t *testing.T) {
	mgr := testManager(t)
	ctx := testContext()

	tradeSvc := trade.NewService(mgr, common.NewLogger("error"))
	portfolioName := "RemoveNotFoundTest"

	// Try to remove trade that doesn't exist
	_, err := tradeSvc.RemoveTrade(ctx, portfolioName, "tr_nonexistent")
	assert.Error(t, err, "expected error when removing nonexistent trade")
}

// TestUpdateTrade verifies merge semantics on trade updates.
func TestUpdateTrade(t *testing.T) {
	mgr := testManager(t)
	ctx := testContext()

	tradeSvc := trade.NewService(mgr, common.NewLogger("error"))
	portfolioName := "UpdateTradeTest"

	// Add trade
	trade := models.Trade{
		Ticker: "RIO.AU",
		Action: models.TradeActionBuy,
		Date:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Units:  100,
		Price:  50.0,
		Fees:   50.0,
		Notes:  "Original note",
	}
	added, _, err := tradeSvc.AddTrade(ctx, portfolioName, trade)
	require.NoError(t, err)

	// Update only price
	update := models.Trade{
		Price: 52.0,
	}
	updated, err := tradeSvc.UpdateTrade(ctx, portfolioName, added.ID, update)
	require.NoError(t, err, "update trade")
	require.NotNil(t, updated)

	// Verify only price changed (merge semantics)
	assert.InDelta(t, 52.0, updated.Price, 0.01)
	assert.Equal(t, 100.0, updated.Units)           // unchanged
	assert.Equal(t, "Original note", updated.Notes) // unchanged
}

// TestUpdateTradeNotFound verifies error when updating nonexistent trade.
func TestUpdateTradeNotFound(t *testing.T) {
	mgr := testManager(t)
	ctx := testContext()

	tradeSvc := trade.NewService(mgr, common.NewLogger("error"))
	portfolioName := "UpdateNotFoundTest"

	update := models.Trade{
		Price: 100.0,
	}
	_, err := tradeSvc.UpdateTrade(ctx, portfolioName, "tr_nonexistent", update)
	assert.Error(t, err, "expected error when updating nonexistent trade")
}

// --- Trade List Tests ---

// TestListTrades verifies filtering and pagination.
func TestListTrades(t *testing.T) {
	mgr := testManager(t)
	ctx := testContext()

	tradeSvc := trade.NewService(mgr, common.NewLogger("error"))
	portfolioName := "ListTradesTest"

	// Add trades for multiple tickers and actions
	trades := []models.Trade{
		{
			Ticker: "BHP.AU",
			Action: models.TradeActionBuy,
			Date:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Units:  100,
			Price:  50.0,
		},
		{
			Ticker: "CBA.AU",
			Action: models.TradeActionBuy,
			Date:   time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			Units:  50,
			Price:  100.0,
		},
		{
			Ticker: "BHP.AU",
			Action: models.TradeActionSell,
			Date:   time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
			Units:  50,
			Price:  60.0,
		},
	}

	for _, tr := range trades {
		_, _, err := tradeSvc.AddTrade(ctx, portfolioName, tr)
		require.NoError(t, err)
	}

	// List all trades
	allTrades, total, err := tradeSvc.ListTrades(ctx, portfolioName, interfaces.TradeFilter{})
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Len(t, allTrades, 3)

	// Filter by ticker
	bhpTrades, totalBhp, err := tradeSvc.ListTrades(ctx, portfolioName, interfaces.TradeFilter{
		Ticker: "BHP.AU",
	})
	require.NoError(t, err)
	assert.Equal(t, 2, totalBhp)
	assert.Len(t, bhpTrades, 2)

	// Filter by action
	buyTrades, totalBuy, err := tradeSvc.ListTrades(ctx, portfolioName, interfaces.TradeFilter{
		Action: models.TradeActionBuy,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, totalBuy)
	assert.Len(t, buyTrades, 2)
}

// TestListTradesPagination verifies offset and limit.
func TestListTradesPagination(t *testing.T) {
	mgr := testManager(t)
	ctx := testContext()

	tradeSvc := trade.NewService(mgr, common.NewLogger("error"))
	portfolioName := "PaginationTest"

	// Add 10 trades
	for i := 1; i <= 10; i++ {
		trade := models.Trade{
			Ticker: "BHP.AU",
			Action: models.TradeActionBuy,
			Date:   time.Date(2025, 1, i, 0, 0, 0, 0, time.UTC),
			Units:  100,
			Price:  50.0,
		}
		_, _, err := tradeSvc.AddTrade(ctx, portfolioName, trade)
		require.NoError(t, err)
	}

	// Page 1: offset 0, limit 5
	page1, total1, err := tradeSvc.ListTrades(ctx, portfolioName, interfaces.TradeFilter{
		Limit:  5,
		Offset: 0,
	})
	require.NoError(t, err)
	assert.Equal(t, 10, total1)
	assert.Len(t, page1, 5)

	// Page 2: offset 5, limit 5
	page2, total2, err := tradeSvc.ListTrades(ctx, portfolioName, interfaces.TradeFilter{
		Limit:  5,
		Offset: 5,
	})
	require.NoError(t, err)
	assert.Equal(t, 10, total2)
	assert.Len(t, page2, 5)

	// Verify different trades on each page
	assert.NotEqual(t, page1[0].ID, page2[0].ID)
}

// --- Snapshot Tests ---

// TestSnapshotPositionsReplace verifies replace mode clears and inserts.
func TestSnapshotPositionsReplace(t *testing.T) {
	mgr := testManager(t)
	ctx := testContext()

	tradeSvc := trade.NewService(mgr, common.NewLogger("error"))
	portfolioName := "SnapshotReplaceTest"

	// Add initial snapshot
	initialPositions := []models.SnapshotPosition{
		{
			Ticker:       "BHP.AU",
			Units:        100,
			AvgCost:      50.0,
			CurrentPrice: 55.0,
			SourceRef:    "initial",
		},
	}
	book, err := tradeSvc.SnapshotPositions(ctx, portfolioName, initialPositions, "replace", "initial", "2025-01-01")
	require.NoError(t, err)
	require.Len(t, book.SnapshotPositions, 1)
	assert.Equal(t, "BHP.AU", book.SnapshotPositions[0].Ticker)

	// Replace with new snapshot
	newPositions := []models.SnapshotPosition{
		{
			Ticker:       "CBA.AU",
			Units:        50,
			AvgCost:      100.0,
			CurrentPrice: 105.0,
			SourceRef:    "replaced",
		},
		{
			Ticker:       "NAB.AU",
			Units:        75,
			AvgCost:      30.0,
			CurrentPrice: 32.0,
			SourceRef:    "replaced",
		},
	}
	book, err = tradeSvc.SnapshotPositions(ctx, portfolioName, newPositions, "replace", "replaced", "2025-02-01")
	require.NoError(t, err)

	// Old position should be gone, new ones present
	assert.Len(t, book.SnapshotPositions, 2)
	tickers := make(map[string]bool)
	for _, pos := range book.SnapshotPositions {
		tickers[pos.Ticker] = true
	}
	assert.True(t, tickers["CBA.AU"])
	assert.True(t, tickers["NAB.AU"])
	assert.False(t, tickers["BHP.AU"])
}

// TestSnapshotPositionsMerge verifies merge mode updates matching, adds new.
func TestSnapshotPositionsMerge(t *testing.T) {
	mgr := testManager(t)
	ctx := testContext()

	tradeSvc := trade.NewService(mgr, common.NewLogger("error"))
	portfolioName := "SnapshotMergeTest"

	// Initial snapshot
	initialPositions := []models.SnapshotPosition{
		{
			Ticker:       "BHP.AU",
			Units:        100,
			AvgCost:      50.0,
			CurrentPrice: 55.0,
		},
		{
			Ticker:       "CBA.AU",
			Units:        50,
			AvgCost:      100.0,
			CurrentPrice: 105.0,
		},
	}
	book, err := tradeSvc.SnapshotPositions(ctx, portfolioName, initialPositions, "replace", "v1", "2025-01-01")
	require.NoError(t, err)
	assert.Len(t, book.SnapshotPositions, 2)

	// Merge: update BHP, add NAB, leave CBA unchanged
	mergePositions := []models.SnapshotPosition{
		{
			Ticker:       "BHP.AU",
			Units:        120, // updated
			AvgCost:      52.0,
			CurrentPrice: 56.0,
		},
		{
			Ticker:       "NAB.AU", // new
			Units:        75,
			AvgCost:      30.0,
			CurrentPrice: 32.0,
		},
	}
	book, err = tradeSvc.SnapshotPositions(ctx, portfolioName, mergePositions, "merge", "v2", "2025-02-01")
	require.NoError(t, err)

	// Should have 3 positions: BHP (updated), CBA (unchanged), NAB (new)
	assert.Len(t, book.SnapshotPositions, 3)

	// Verify BHP was updated
	for _, pos := range book.SnapshotPositions {
		if pos.Ticker == "BHP.AU" {
			assert.Equal(t, 120.0, pos.Units)
			assert.InDelta(t, 52.0, pos.AvgCost, 0.01)
			break
		}
	}
}

// --- Manual Portfolio Assembly Tests ---

// TestGetManualPortfolio verifies holdings derived from trades.
func TestGetManualPortfolio(t *testing.T) {
	mgr := testManager(t)
	ctx := testContext()

	tradeSvc := trade.NewService(mgr, common.NewLogger("error"))
	portfolioSvc := portfolio.NewService(mgr, nil, nil, nil, common.NewLogger("error"))
	portfolioSvc.SetTradeService(tradeSvc)
	ctx = context.WithValue(ctx, "user_id", "manual_portfolio_user")

	// Create manual portfolio
	portfolioName := "ManualPortfolioTest"
	created, err := portfolioSvc.CreatePortfolio(ctx, portfolioName, models.SourceManual, "AUD")
	require.NoError(t, err)
	assert.Equal(t, models.SourceManual, created.SourceType)

	// Add trades
	trades := []models.Trade{
		{
			Ticker: "BHP.AU",
			Action: models.TradeActionBuy,
			Date:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Units:  100,
			Price:  50.0,
		},
		{
			Ticker: "CBA.AU",
			Action: models.TradeActionBuy,
			Date:   time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			Units:  50,
			Price:  100.0,
		},
	}
	for _, tr := range trades {
		_, _, err := tradeSvc.AddTrade(ctx, portfolioName, tr)
		require.NoError(t, err)
	}

	// Get portfolio - should assemble from trades
	retrieved, err := portfolioSvc.GetPortfolio(ctx, portfolioName)
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	// Holdings should be derived from trades
	assert.Len(t, retrieved.Holdings, 2)
	assert.True(t, retrieved.EquityValue > 0)
	assert.True(t, retrieved.NetEquityCost > 0)
}

// TestGetSnapshotPortfolio verifies holdings from snapshot positions.
func TestGetSnapshotPortfolio(t *testing.T) {
	mgr := testManager(t)
	ctx := testContext()

	tradeSvc := trade.NewService(mgr, common.NewLogger("error"))
	portfolioSvc := portfolio.NewService(mgr, nil, nil, nil, common.NewLogger("error"))
	portfolioSvc.SetTradeService(tradeSvc)
	ctx = context.WithValue(ctx, "user_id", "snapshot_portfolio_user")

	// Create snapshot portfolio
	portfolioName := "SnapshotPortfolioTest"
	created, err := portfolioSvc.CreatePortfolio(ctx, portfolioName, models.SourceSnapshot, "AUD")
	require.NoError(t, err)
	assert.Equal(t, models.SourceSnapshot, created.SourceType)

	// Import snapshot positions
	positions := []models.SnapshotPosition{
		{
			Ticker:       "BHP.AU",
			Units:        100,
			AvgCost:      50.0,
			CurrentPrice: 55.0,
		},
		{
			Ticker:       "CBA.AU",
			Units:        50,
			AvgCost:      100.0,
			CurrentPrice: 105.0,
		},
	}
	_, err = tradeSvc.SnapshotPositions(ctx, portfolioName, positions, "replace", "snapshot", "2025-02-01")
	require.NoError(t, err)

	// Get portfolio - should assemble from snapshot
	retrieved, err := portfolioSvc.GetPortfolio(ctx, portfolioName)
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	// Holdings should be from snapshot
	assert.Len(t, retrieved.Holdings, 2)
	assert.True(t, retrieved.EquityValue > 0)
}
