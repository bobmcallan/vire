package portfolio

import (
	"math"
	"testing"
	"time"

	"github.com/bobmccarthy/vire/internal/models"
)

// twrrApproxEqual checks float equality within epsilon
func twrrApproxEqual(a, b, epsilon float64) bool {
	return math.Abs(a-b) < epsilon
}

func TestCalculateTWRR_SingleBuyHold(t *testing.T) {
	// Buy 100 units at $10 on 2024-01-01, price is now $12 on 2025-01-01
	// Cumulative TWRR = (12/10 - 1) = 20%
	// Held 366 days (2024 is leap year), >= 365 so annualised:
	// Annualised = (1.20)^(365/366) - 1 = ~19.95%
	trades := []*models.NavexaTrade{
		{Type: "buy", Date: "2024-01-01", Units: 100, Price: 10.00, Fees: 0},
	}

	eodBars := []models.EODBar{
		{Date: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Close: 12.00},
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Close: 10.00},
	}

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	result := CalculateTWRR(trades, eodBars, 12.00, now)

	// Annualised: (1.20)^(365/366) - 1 = 19.95%
	if !twrrApproxEqual(result, 19.95, 1.0) {
		t.Errorf("CalculateTWRR single buy+hold = %.2f%%, want ~19.95%%", result)
	}
}

func TestCalculateTWRR_BuySellRebuy(t *testing.T) {
	// Buy 100 @ $10 on 2024-01-01
	// Sell 100 @ $15 on 2024-07-01 (close on that date: $15)
	// Buy 100 @ $14 on 2024-07-01 (same day rebuy)
	// Current price $16 on 2025-01-01
	//
	// Sub-period 1: buy date 2024-01-01, close=$10 -> sell date 2024-07-01, close=$15
	//   return = 15/10 = 1.50
	// Sub-period 2: rebuy date 2024-07-01, close=$15 -> now 2025-01-01, price=$16
	//   return = 16/15 = 1.0667
	// (The sell+rebuy on the same date creates a zero-length sub-period with 0 units — skipped)
	//
	// Cumulative TWRR = 1.50 * 1.0667 - 1 = 0.60 = 60%
	// Held 366 days, annualised: (1.60)^(365/366) - 1 = ~59.84%
	trades := []*models.NavexaTrade{
		{Type: "buy", Date: "2024-01-01", Units: 100, Price: 10.00, Fees: 0},
		{Type: "sell", Date: "2024-07-01", Units: 100, Price: 15.00, Fees: 0},
		{Type: "buy", Date: "2024-07-01", Units: 100, Price: 14.00, Fees: 0},
	}

	eodBars := []models.EODBar{
		{Date: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Close: 16.00},
		{Date: time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC), Close: 15.00},
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Close: 10.00},
	}

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	result := CalculateTWRR(trades, eodBars, 16.00, now)

	// Annualised: (1.60)^(365/366) - 1 = ~59.84%
	if !twrrApproxEqual(result, 59.84, 2.0) {
		t.Errorf("CalculateTWRR buy+sell+rebuy = %.2f%%, want ~59.84%%", result)
	}
}

func TestCalculateTWRR_MultipleBuysDifferentPrices(t *testing.T) {
	// Buy 100 @ $10 on 2024-01-01
	// Buy 100 @ $15 on 2024-07-01 (cost averaging)
	// Current price $18 on 2025-01-01
	//
	// Sub-period 1: close $10 -> close $15 = 15/10 = 1.50
	// Sub-period 2: close $15 -> price $18 = 18/15 = 1.20
	// Cumulative TWRR = 1.50 * 1.20 - 1 = 0.80 = 80%
	// Held 366 days, annualised: (1.80)^(365/366) - 1 = ~79.85%
	//
	// Note: Simple return = (200*18 - (100*10 + 100*15)) / (100*10 + 100*15) = (3600-2500)/2500 = 44%
	// TWRR (80%) differs significantly from simple return (44%) because TWRR removes
	// the effect of the cash flow timing (the second buy at a higher price).
	trades := []*models.NavexaTrade{
		{Type: "buy", Date: "2024-01-01", Units: 100, Price: 10.00, Fees: 0},
		{Type: "buy", Date: "2024-07-01", Units: 100, Price: 15.00, Fees: 0},
	}

	eodBars := []models.EODBar{
		{Date: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Close: 18.00},
		{Date: time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC), Close: 15.00},
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Close: 10.00},
	}

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	result := CalculateTWRR(trades, eodBars, 18.00, now)

	// Annualised: (1.80)^(365/366) - 1 = ~79.85%
	if !twrrApproxEqual(result, 79.85, 2.0) {
		t.Errorf("CalculateTWRR multiple buys = %.2f%%, want ~79.85%%", result)
	}
	// Verify it's clearly not the simple return (44%)
	if twrrApproxEqual(result, 44.0, 5.0) {
		t.Errorf("CalculateTWRR returned %.2f%% which looks like simple return (44%%), not TWRR", result)
	}
}

func TestCalculateTWRR_ClosedPosition(t *testing.T) {
	// Buy 100 @ $10 on 2024-01-01
	// Sell 100 @ $15 on 2024-07-01
	// Position is closed. Cumulative TWRR = (15/10 - 1) = 50%
	// Held 182 days (2024-01-01 to 2024-07-01). < 365 days, so NOT annualised.
	// Return cumulative 50%.
	trades := []*models.NavexaTrade{
		{Type: "buy", Date: "2024-01-01", Units: 100, Price: 10.00, Fees: 0},
		{Type: "sell", Date: "2024-07-01", Units: 100, Price: 15.00, Fees: 0},
	}

	eodBars := []models.EODBar{
		{Date: time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC), Close: 15.00},
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Close: 10.00},
	}

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	result := CalculateTWRR(trades, eodBars, 0, now)

	// Held < 365 days, return cumulative TWRR = 50%
	// If annualised, would be (1.50)^(365/182) - 1 = ~125.6% -- distinctly different
	if !twrrApproxEqual(result, 50.0, 2.0) {
		t.Errorf("CalculateTWRR closed position = %.2f%%, want ~50.00%% (cumulative, not annualised)", result)
	}
}

func TestCalculateTWRR_TwoYearHold(t *testing.T) {
	// Buy 100 @ $10 on 2023-01-01, price is now $14.40 on 2025-01-01
	// Cumulative TWRR = (14.40/10 - 1) = 44%
	// Held 731 days (2 years), >= 365 so annualised:
	// Annualised = (1.44)^(365/731) - 1 = ~20.0%
	// This test verifies annualization produces a clearly different result from cumulative.
	trades := []*models.NavexaTrade{
		{Type: "buy", Date: "2023-01-01", Units: 100, Price: 10.00, Fees: 0},
	}

	eodBars := []models.EODBar{
		{Date: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Close: 14.40},
		{Date: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), Close: 10.00},
	}

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	result := CalculateTWRR(trades, eodBars, 14.40, now)

	// Annualised: (1.44)^(365/731) - 1 = ~20.0%
	// Cumulative would be 44% -- verify we get the annualised value, not cumulative
	if !twrrApproxEqual(result, 20.0, 1.0) {
		t.Errorf("CalculateTWRR 2-year hold = %.2f%%, want ~20.0%% (annualised)", result)
	}
	if twrrApproxEqual(result, 44.0, 3.0) {
		t.Errorf("CalculateTWRR returned %.2f%% which looks like cumulative (44%%), should be annualised", result)
	}
}

func TestCalculateTWRR_ShortHoldingPeriod(t *testing.T) {
	// Buy 100 @ $10 on 2025-01-01
	// Current price $10.20 on 2025-01-10 (9 days)
	// Cumulative TWRR = (10.20/10 - 1) = 2%
	// Held < 365 days, so NOT annualised (annualised would be ~131%)
	trades := []*models.NavexaTrade{
		{Type: "buy", Date: "2025-01-01", Units: 100, Price: 10.00, Fees: 0},
	}

	eodBars := []models.EODBar{
		{Date: time.Date(2025, 1, 10, 0, 0, 0, 0, time.UTC), Close: 10.20},
		{Date: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Close: 10.00},
	}

	now := time.Date(2025, 1, 10, 0, 0, 0, 0, time.UTC)
	result := CalculateTWRR(trades, eodBars, 10.20, now)

	// Should return raw cumulative ~2%, NOT annualised (which would be ~131%)
	if !twrrApproxEqual(result, 2.0, 0.5) {
		t.Errorf("CalculateTWRR short period = %.2f%%, want ~2.00%% (not annualised)", result)
	}
}

func TestCalculateTWRR_NoTrades(t *testing.T) {
	trades := []*models.NavexaTrade{}

	eodBars := []models.EODBar{
		{Date: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Close: 10.00},
	}

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	result := CalculateTWRR(trades, eodBars, 10.00, now)

	if result != 0 {
		t.Errorf("CalculateTWRR no trades = %.2f%%, want 0", result)
	}
}

func TestCalculateTWRR_NoEODData(t *testing.T) {
	trades := []*models.NavexaTrade{
		{Type: "buy", Date: "2024-01-01", Units: 100, Price: 10.00, Fees: 0},
	}

	eodBars := []models.EODBar{} // no bars

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	result := CalculateTWRR(trades, eodBars, 12.00, now)

	// With no EOD data, fallback to simple return from trade price to current price
	// (12 - 10) / 10 = 20%, held 366 days, annualised ~19.95%
	if !twrrApproxEqual(result, 19.95, 1.0) {
		t.Errorf("CalculateTWRR no EOD = %.2f%%, want ~19.95%%", result)
	}
}

func TestCalculateTWRR_OpeningBalance(t *testing.T) {
	// Opening balance treated same as buy
	trades := []*models.NavexaTrade{
		{Type: "opening balance", Date: "2024-01-01", Units: 200, Price: 5.00, Fees: 0},
	}

	eodBars := []models.EODBar{
		{Date: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Close: 8.00},
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Close: 5.00},
	}

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	result := CalculateTWRR(trades, eodBars, 8.00, now)

	// TWRR = (8/5 - 1) = 60% cumulative, held 366 days
	// Annualised: (1.60)^(365/366) - 1 = ~59.84%
	if !twrrApproxEqual(result, 59.84, 2.0) {
		t.Errorf("CalculateTWRR opening balance = %.2f%%, want ~59.84%%", result)
	}
}

func TestCalculateTWRR_PartialSell(t *testing.T) {
	// Buy 200 @ $10 on 2024-01-01
	// Sell 100 @ $15 on 2024-07-01
	// Still holding 100 units, current price $18 on 2025-01-01
	// Sub-period 1: close 10 -> 15 = 1.50
	// Sub-period 2: close 15 -> 18 = 1.20
	// Cumulative TWRR = 1.50 * 1.20 - 1 = 0.80 = 80%
	// Held 366 days, annualised: (1.80)^(365/366) - 1 = ~79.85%
	trades := []*models.NavexaTrade{
		{Type: "buy", Date: "2024-01-01", Units: 200, Price: 10.00, Fees: 0},
		{Type: "sell", Date: "2024-07-01", Units: 100, Price: 15.00, Fees: 0},
	}

	eodBars := []models.EODBar{
		{Date: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Close: 18.00},
		{Date: time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC), Close: 15.00},
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Close: 10.00},
	}

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	result := CalculateTWRR(trades, eodBars, 18.00, now)

	// Annualised: (1.80)^(365/366) - 1 = ~79.85%
	if !twrrApproxEqual(result, 79.85, 2.0) {
		t.Errorf("CalculateTWRR partial sell = %.2f%%, want ~79.85%%", result)
	}
}

func TestCalculateTWRR_PriceDrop(t *testing.T) {
	// Buy 100 @ $20 on 2024-01-01
	// Price drops to $15 (25% loss)
	// Cumulative TWRR = (15/20 - 1) = -25%
	// Held 366 days, annualised: (0.75)^(365/366) - 1 = ~-25.02%
	trades := []*models.NavexaTrade{
		{Type: "buy", Date: "2024-01-01", Units: 100, Price: 20.00, Fees: 0},
	}

	eodBars := []models.EODBar{
		{Date: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Close: 15.00},
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Close: 20.00},
	}

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	result := CalculateTWRR(trades, eodBars, 15.00, now)

	// Annualised: (0.75)^(365/366) - 1 = ~-25.02%
	if !twrrApproxEqual(result, -25.0, 2.0) {
		t.Errorf("CalculateTWRR price drop = %.2f%%, want ~-25%%", result)
	}
}

func TestCalculateTWRR_ZeroClosePrice(t *testing.T) {
	// Edge case: if a close price is 0 (delisted/bad data), the sub-period
	// denominator would be 0 causing division by zero.
	// The implementation should handle this gracefully (skip the sub-period or return 0).
	trades := []*models.NavexaTrade{
		{Type: "buy", Date: "2024-01-01", Units: 100, Price: 10.00, Fees: 0},
		{Type: "buy", Date: "2024-07-01", Units: 100, Price: 5.00, Fees: 0},
	}

	eodBars := []models.EODBar{
		{Date: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Close: 12.00},
		{Date: time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC), Close: 0.00}, // bad data
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Close: 10.00},
	}

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	result := CalculateTWRR(trades, eodBars, 12.00, now)

	// Should not panic or return NaN/Inf. Any finite result is acceptable.
	if math.IsNaN(result) || math.IsInf(result, 0) {
		t.Errorf("CalculateTWRR with zero close = %v, want finite value (not NaN/Inf)", result)
	}
}

func TestCalculateTWRR_FeesDoNotAffectReturn(t *testing.T) {
	// TWRR is purely price-based return. Fees affect cost basis but should NOT
	// change the TWRR calculation (which is close-to-close price ratio).
	trades := []*models.NavexaTrade{
		{Type: "buy", Date: "2024-01-01", Units: 100, Price: 10.00, Fees: 50.00},
	}

	tradesNoFees := []*models.NavexaTrade{
		{Type: "buy", Date: "2024-01-01", Units: 100, Price: 10.00, Fees: 0},
	}

	eodBars := []models.EODBar{
		{Date: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Close: 12.00},
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Close: 10.00},
	}

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	resultWithFees := CalculateTWRR(trades, eodBars, 12.00, now)
	resultNoFees := CalculateTWRR(tradesNoFees, eodBars, 12.00, now)

	if !twrrApproxEqual(resultWithFees, resultNoFees, 0.01) {
		t.Errorf("TWRR with fees (%.2f%%) != TWRR without fees (%.2f%%); fees should not affect TWRR",
			resultWithFees, resultNoFees)
	}
}

func TestCalculateTWRR_CostBaseAdjustmentIgnored(t *testing.T) {
	// Cost base increase/decrease are accounting adjustments, not cash flows.
	// They should NOT create sub-period boundaries in TWRR calculation.
	// Result should be the same as a simple buy+hold.
	trades := []*models.NavexaTrade{
		{Type: "buy", Date: "2024-01-01", Units: 100, Price: 10.00, Fees: 0},
		{Type: "cost base increase", Date: "2024-06-01", Units: 0, Price: 0, Fees: 0, Value: 200},
		{Type: "cost base decrease", Date: "2024-09-01", Units: 0, Price: 0, Fees: 0, Value: 50},
	}

	tradesSimple := []*models.NavexaTrade{
		{Type: "buy", Date: "2024-01-01", Units: 100, Price: 10.00, Fees: 0},
	}

	eodBars := []models.EODBar{
		{Date: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Close: 12.00},
		{Date: time.Date(2024, 9, 1, 0, 0, 0, 0, time.UTC), Close: 11.50},
		{Date: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), Close: 11.00},
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Close: 10.00},
	}

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	resultWithAdj := CalculateTWRR(trades, eodBars, 12.00, now)
	resultSimple := CalculateTWRR(tradesSimple, eodBars, 12.00, now)

	if !twrrApproxEqual(resultWithAdj, resultSimple, 0.01) {
		t.Errorf("TWRR with cost base adjustments (%.2f%%) != simple buy+hold (%.2f%%); adjustments should be ignored",
			resultWithAdj, resultSimple)
	}
}
