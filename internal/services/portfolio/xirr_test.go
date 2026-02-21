package portfolio

import (
	"math"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
)

func TestXIRR_SimpleBuyAndHold(t *testing.T) {
	// Buy $10,000 worth, now worth $11,000 after exactly 1 year
	// Expected XIRR: ~10%
	buyDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	trades := []*models.NavexaTrade{
		{Type: "buy", Date: "2024-01-01", Units: 100, Price: 100.00, Fees: 0},
	}
	xirr := CalculateXIRR(trades, 11000, 0, false, now)

	if !approxEqual(xirr, 10.0, 0.5) {
		t.Errorf("XIRR = %.2f%%, want ~10%% for simple buy-and-hold", xirr)
	}
	_ = buyDate // suppress unused
}

func TestXIRR_SimpleBuyAndHold_ShortPeriod(t *testing.T) {
	// Buy $10,000, worth $10,500 after 6 months
	// Simple return = 5%, annualised XIRR should be ~10.25%
	now := time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)

	trades := []*models.NavexaTrade{
		{Type: "buy", Date: "2024-01-01", Units: 100, Price: 100.00, Fees: 0},
	}
	xirr := CalculateXIRR(trades, 10500, 0, false, now)

	// 6-month 5% gain annualises to ~10.25%
	if xirr < 9 || xirr > 12 {
		t.Errorf("XIRR = %.2f%%, want ~10.25%% for 6-month 5%% gain", xirr)
	}
}

func TestXIRR_BuyAndSellAtProfit(t *testing.T) {
	// Buy $10,000, sell for $12,000 after 1 year
	// Expected XIRR: ~20%
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	trades := []*models.NavexaTrade{
		{Type: "buy", Date: "2024-01-01", Units: 100, Price: 100.00, Fees: 0},
		{Type: "sell", Date: "2025-01-01", Units: 100, Price: 120.00, Fees: 0},
	}
	// Closed position — currentMarketValue = 0
	xirr := CalculateXIRR(trades, 0, 0, false, now)

	if !approxEqual(xirr, 20.0, 0.5) {
		t.Errorf("XIRR = %.2f%%, want ~20%% for buy $100 sell $120", xirr)
	}
}

func TestXIRR_BuyAndSellAtLoss(t *testing.T) {
	// Buy $10,000, sell for $8,000 after 1 year
	// Expected XIRR: ~-20%
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	trades := []*models.NavexaTrade{
		{Type: "buy", Date: "2024-01-01", Units: 100, Price: 100.00, Fees: 0},
		{Type: "sell", Date: "2025-01-01", Units: 100, Price: 80.00, Fees: 0},
	}
	xirr := CalculateXIRR(trades, 0, 0, false, now)

	if !approxEqual(xirr, -20.0, 0.5) {
		t.Errorf("XIRR = %.2f%%, want ~-20%% for 20%% loss", xirr)
	}
}

func TestXIRR_MultipleBuys(t *testing.T) {
	// Buy 100 @ $100 on Jan 1, Buy 100 @ $110 on Jul 1, worth $220/unit after 1 year
	// XIRR accounts for timing of cash flows
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	trades := []*models.NavexaTrade{
		{Type: "buy", Date: "2024-01-01", Units: 100, Price: 100.00, Fees: 0},
		{Type: "buy", Date: "2024-07-01", Units: 100, Price: 110.00, Fees: 0},
	}
	currentValue := 200 * 120.0 // 200 units @ $120
	xirr := CalculateXIRR(trades, currentValue, 0, false, now)

	// First investment had ~20% gain in 1 year, second had ~9.1% gain in 6 months
	// XIRR should be between 15-25%
	if xirr < 10 || xirr > 30 {
		t.Errorf("XIRR = %.2f%%, want 15-25%% range for multiple buys", xirr)
	}
}

func TestXIRR_WithDividends(t *testing.T) {
	// Buy $10,000, still worth $10,000 after 1 year, but received $500 dividends
	// Without dividends: XIRR = 0%
	// With dividends: XIRR = ~5%
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	trades := []*models.NavexaTrade{
		{Type: "buy", Date: "2024-01-01", Units: 100, Price: 100.00, Fees: 0},
	}

	xirrNoDivs := CalculateXIRR(trades, 10000, 500, false, now)
	xirrWithDivs := CalculateXIRR(trades, 10000, 500, true, now)

	if !approxEqual(xirrNoDivs, 0.0, 0.5) {
		t.Errorf("XIRR without dividends = %.2f%%, want ~0%%", xirrNoDivs)
	}
	if !approxEqual(xirrWithDivs, 5.0, 0.5) {
		t.Errorf("XIRR with dividends = %.2f%%, want ~5%%", xirrWithDivs)
	}
}

func TestXIRR_SKS_Scenario(t *testing.T) {
	// SKS: buy, partial sells, re-entry buys — same as other tests
	now := time.Date(2025, 2, 22, 0, 0, 0, 0, time.UTC)

	trades := []*models.NavexaTrade{
		{Type: "buy", Date: "2024-01-15", Units: 4925, Price: 4.0248, Fees: 0},
		{Type: "sell", Date: "2024-04-10", Units: 1333, Price: 3.7627, Fees: 0},
		{Type: "sell", Date: "2024-05-20", Units: 819, Price: 3.680, Fees: 0},
		{Type: "sell", Date: "2024-07-15", Units: 2773, Price: 3.4508, Fees: 0},
		{Type: "buy", Date: "2024-09-10", Units: 2511, Price: 3.980, Fees: 0},
		{Type: "buy", Date: "2024-10-05", Units: 2456, Price: 4.070, Fees: 0},
	}

	remainingUnits := 4925.0 - 1333 - 819 - 2773 + 2511 + 2456 // = 4967
	currentPrice := 4.71
	marketValue := remainingUnits * currentPrice

	xirr := CalculateXIRR(trades, marketValue, 0, false, now)

	// XIRR should be a reasonable annualised rate, different from simple 5.82%
	// since XIRR accounts for cash flow timing
	if math.IsNaN(xirr) || math.IsInf(xirr, 0) {
		t.Errorf("XIRR = %v, expected a finite value", xirr)
	}
	// Just verify it's in a reasonable range and converged
	if xirr < -50 || xirr > 100 {
		t.Errorf("XIRR = %.2f%%, outside reasonable range [-50%%, 100%%]", xirr)
	}
}

func TestXIRR_NoTrades(t *testing.T) {
	xirr := CalculateXIRR(nil, 10000, 0, false, time.Now())
	if xirr != 0 {
		t.Errorf("XIRR with no trades = %.2f, want 0", xirr)
	}
}

func TestXIRR_OnlyBuysNoValue(t *testing.T) {
	// All money invested, current value = 0 (total loss)
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	trades := []*models.NavexaTrade{
		{Type: "buy", Date: "2024-01-01", Units: 100, Price: 100.00, Fees: 0},
	}
	xirr := CalculateXIRR(trades, 0, 0, false, now)
	// No positive cash flows => can't compute
	if xirr != 0 {
		t.Errorf("XIRR with total loss and no proceeds = %.2f, want 0", xirr)
	}
}

func TestXIRR_WithFees(t *testing.T) {
	// Buy 100 @ $100 with $50 fees, sell 100 @ $120 with $50 fees after 1 year
	// Net invested: 10050, net received: 11950
	// XIRR should be slightly less than 20%
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	trades := []*models.NavexaTrade{
		{Type: "buy", Date: "2024-01-01", Units: 100, Price: 100.00, Fees: 50},
		{Type: "sell", Date: "2025-01-01", Units: 100, Price: 120.00, Fees: 50},
	}
	xirr := CalculateXIRR(trades, 0, 0, false, now)

	// (11950 - 10050) / 10050 = 18.9%
	if !approxEqual(xirr, 18.9, 1.0) {
		t.Errorf("XIRR with fees = %.2f%%, want ~18.9%%", xirr)
	}
}

func TestXIRR_CostBaseAdjustment(t *testing.T) {
	// Buy, then cost base increase, then sell
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	trades := []*models.NavexaTrade{
		{Type: "buy", Date: "2024-01-01", Units: 100, Price: 100.00, Fees: 0},
		{Type: "cost base increase", Date: "2024-06-01", Value: 500},
	}
	// Current value: 11000, cost base increase adds 500 to invested
	xirr := CalculateXIRR(trades, 11000, 0, false, now)

	// Net invested: 10000 + 500 = 10500, received: 11000
	// Modest positive XIRR
	if xirr < 0 || xirr > 15 {
		t.Errorf("XIRR with cost base adjustment = %.2f%%, want ~5%% range", xirr)
	}
}

func TestXIRR_OpeningBalance(t *testing.T) {
	// Opening balance treated same as buy
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	trades := []*models.NavexaTrade{
		{Type: "opening balance", Date: "2024-01-01", Units: 100, Price: 100.00, Fees: 0},
	}
	xirr := CalculateXIRR(trades, 11000, 0, false, now)

	if !approxEqual(xirr, 10.0, 0.5) {
		t.Errorf("XIRR with opening balance = %.2f%%, want ~10%%", xirr)
	}
}
