package data

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/bobmcallan/vire/internal/services/portfolio"
	"github.com/bobmcallan/vire/internal/services/trade"
)

// TestManualPortfolio_IncludesClosedPositions verifies that assembleManualPortfolio
// includes fully sold positions in the holdings array with status="closed".
func TestManualPortfolio_IncludesClosedPositions(t *testing.T) {
	mgr := testManager(t)
	ctx := testContext()

	tradeSvc := trade.NewService(mgr, common.NewLogger("error"))
	portfolioSvc := portfolio.NewService(mgr, nil, nil, nil, common.NewLogger("error"))
	portfolioSvc.SetTradeService(tradeSvc)
	ctx = common.WithUserContext(ctx, &common.UserContext{UserID: "closed_positions_user"})

	portfolioName := "ClosedPositionsTest"
	_, err := portfolioSvc.CreatePortfolio(ctx, portfolioName, models.SourceManual, "AUD")
	require.NoError(t, err, "create manual portfolio")

	// Add BUY trade for BHP (will be closed)
	_, _, err = tradeSvc.AddTrade(ctx, portfolioName, models.Trade{
		Ticker: "BHP.AU",
		Action: models.TradeActionBuy,
		Date:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Units:  100,
		Price:  50.0,
	})
	require.NoError(t, err, "add buy trade for BHP")

	// Add BUY trade for CBA (will remain open)
	_, _, err = tradeSvc.AddTrade(ctx, portfolioName, models.Trade{
		Ticker: "CBA.AU",
		Action: models.TradeActionBuy,
		Date:   time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
		Units:  50,
		Price:  100.0,
	})
	require.NoError(t, err, "add buy trade for CBA")

	// SELL all BHP units to close the position
	_, _, err = tradeSvc.AddTrade(ctx, portfolioName, models.Trade{
		Ticker: "BHP.AU",
		Action: models.TradeActionSell,
		Date:   time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
		Units:  100,
		Price:  60.0,
	})
	require.NoError(t, err, "add sell trade to close BHP")

	// Get portfolio — should include both open and closed positions
	p, err := portfolioSvc.GetPortfolio(ctx, portfolioName)
	require.NoError(t, err, "get portfolio")
	require.NotNil(t, p)

	// Should have 2 holdings: BHP (closed) and CBA (open)
	require.Len(t, p.Holdings, 2, "expected both open and closed holdings from service layer")

	holdingsByTicker := make(map[string]models.Holding)
	for _, h := range p.Holdings {
		holdingsByTicker[h.Ticker] = h
	}

	// Verify BHP is closed
	bhp, ok := holdingsByTicker["BHP.AU"]
	require.True(t, ok, "BHP holding should be present")
	assert.Equal(t, "closed", bhp.Status, "BHP should have status=closed")
	assert.Equal(t, 0.0, bhp.Units, "closed position should have 0 units")

	// Verify CBA is open
	cba, ok := holdingsByTicker["CBA.AU"]
	require.True(t, ok, "CBA holding should be present")
	assert.Equal(t, "open", cba.Status, "CBA should have status=open")
	assert.Equal(t, 50.0, cba.Units, "CBA should still have 50 units")

	// Closed positions should NOT affect portfolio aggregates
	// Only CBA (50 * 100 = 5000) should count toward equity value
	assert.InDelta(t, 5000.0, p.EquityHoldingsValue, 0.01, "equity value should only include open positions")
	assert.InDelta(t, 5000.0, p.EquityHoldingsCost, 0.01, "equity cost should only include open positions")
}

// TestManualPortfolio_ClosedPositionHasRealizedReturn verifies that closed positions
// have correct RealizedReturn and ReturnNet values reflecting the final P&L.
func TestManualPortfolio_ClosedPositionHasRealizedReturn(t *testing.T) {
	mgr := testManager(t)
	ctx := testContext()

	tradeSvc := trade.NewService(mgr, common.NewLogger("error"))
	portfolioSvc := portfolio.NewService(mgr, nil, nil, nil, common.NewLogger("error"))
	portfolioSvc.SetTradeService(tradeSvc)
	ctx = common.WithUserContext(ctx, &common.UserContext{UserID: "closed_return_user"})

	portfolioName := "ClosedReturnTest"
	_, err := portfolioSvc.CreatePortfolio(ctx, portfolioName, models.SourceManual, "AUD")
	require.NoError(t, err, "create portfolio")

	// Buy 100 @ 50 = cost basis 5000
	_, _, err = tradeSvc.AddTrade(ctx, portfolioName, models.Trade{
		Ticker: "FMG.AU",
		Action: models.TradeActionBuy,
		Date:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Units:  100,
		Price:  50.0,
	})
	require.NoError(t, err, "add buy trade")

	// Sell all 100 @ 75 = proceeds 7500, realized return = 7500 - 5000 = 2500
	_, _, err = tradeSvc.AddTrade(ctx, portfolioName, models.Trade{
		Ticker: "FMG.AU",
		Action: models.TradeActionSell,
		Date:   time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
		Units:  100,
		Price:  75.0,
	})
	require.NoError(t, err, "add sell trade to close position")

	// Get portfolio and find the closed holding
	p, err := portfolioSvc.GetPortfolio(ctx, portfolioName)
	require.NoError(t, err, "get portfolio")
	require.NotNil(t, p)

	var fmg *models.Holding
	for i := range p.Holdings {
		if p.Holdings[i].Ticker == "FMG.AU" {
			fmg = &p.Holdings[i]
			break
		}
	}
	require.NotNil(t, fmg, "FMG holding should be present")

	assert.Equal(t, "closed", fmg.Status, "position should be closed")
	assert.Equal(t, 0.0, fmg.Units, "closed position should have 0 units")
	assert.InDelta(t, 2500.0, fmg.RealizedReturn, 0.01, "realized return: (100*75) - (100*50) = 2500")
	assert.InDelta(t, 2500.0, fmg.ReturnNet, 0.01, "ReturnNet for closed position equals RealizedReturn")
	assert.InDelta(t, 5000.0, fmg.GrossInvested, 0.01, "GrossInvested: 100*50 = 5000")
	assert.InDelta(t, 7500.0, fmg.GrossProceeds, 0.01, "GrossProceeds: 100*75 = 7500")

	// ReturnNetPct = (2500 / 5000) * 100 = 50%
	assert.InDelta(t, 50.0, fmg.ReturnNetPct, 0.01, "return percentage should be 50%%")

	// No open positions — aggregates should be zero
	assert.InDelta(t, 0.0, p.EquityHoldingsValue, 0.01, "no open positions, equity value should be 0")
	assert.InDelta(t, 0.0, p.EquityHoldingsCost, 0.01, "no open positions, equity cost should be 0")
}
