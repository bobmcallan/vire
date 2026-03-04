package trade

import (
	"context"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
)

// ============================================================================
// Devils-advocate stress tests for trade service
// Focus: input validation, injection, edge cases, race conditions
// ============================================================================

// --- NaN / Infinity injection ---

func TestStress_AddTrade_NaNUnits(t *testing.T) {
	svc := testService()
	ctx := testContext()

	trade := models.Trade{
		Ticker: "BHP.AU",
		Action: models.TradeActionBuy,
		Units:  math.NaN(),
		Price:  50.0,
		Fees:   10.0,
		Date:   time.Now().Add(-time.Hour),
	}
	_, _, err := svc.AddTrade(ctx, "Stress", trade)
	if err == nil {
		t.Fatal("expected error for NaN units, got nil")
	}
}

func TestStress_AddTrade_InfPrice(t *testing.T) {
	svc := testService()
	ctx := testContext()

	trade := models.Trade{
		Ticker: "BHP.AU",
		Action: models.TradeActionBuy,
		Units:  100,
		Price:  math.Inf(1),
		Fees:   10.0,
		Date:   time.Now().Add(-time.Hour),
	}
	_, _, err := svc.AddTrade(ctx, "Stress", trade)
	if err == nil {
		t.Fatal("expected error for Inf price, got nil")
	}
}

func TestStress_AddTrade_NegativeInfFees(t *testing.T) {
	svc := testService()
	ctx := testContext()

	trade := models.Trade{
		Ticker: "BHP.AU",
		Action: models.TradeActionBuy,
		Units:  100,
		Price:  50.0,
		Fees:   math.Inf(-1),
		Date:   time.Now().Add(-time.Hour),
	}
	_, _, err := svc.AddTrade(ctx, "Stress", trade)
	if err == nil {
		t.Fatal("expected error for -Inf fees, got nil")
	}
}

func TestStress_AddTrade_NaNFees(t *testing.T) {
	svc := testService()
	ctx := testContext()

	trade := models.Trade{
		Ticker: "BHP.AU",
		Action: models.TradeActionBuy,
		Units:  100,
		Price:  50.0,
		Fees:   math.NaN(),
		Date:   time.Now().Add(-time.Hour),
	}
	_, _, err := svc.AddTrade(ctx, "Stress", trade)
	if err == nil {
		t.Fatal("expected error for NaN fees, got nil")
	}
}

// --- Negative value injection ---

func TestStress_AddTrade_NegativePrice(t *testing.T) {
	svc := testService()
	ctx := testContext()

	trade := models.Trade{
		Ticker: "BHP.AU",
		Action: models.TradeActionBuy,
		Units:  100,
		Price:  -50.0,
		Fees:   10.0,
		Date:   time.Now().Add(-time.Hour),
	}
	_, _, err := svc.AddTrade(ctx, "Stress", trade)
	if err == nil {
		t.Fatal("expected error for negative price, got nil")
	}
}

func TestStress_AddTrade_NegativeFees(t *testing.T) {
	svc := testService()
	ctx := testContext()

	trade := models.Trade{
		Ticker: "BHP.AU",
		Action: models.TradeActionBuy,
		Units:  100,
		Price:  50.0,
		Fees:   -10.0,
		Date:   time.Now().Add(-time.Hour),
	}
	_, _, err := svc.AddTrade(ctx, "Stress", trade)
	if err == nil {
		t.Fatal("expected error for negative fees, got nil")
	}
}

// --- Overflow / extreme values ---

func TestStress_AddTrade_ExtremeUnits(t *testing.T) {
	svc := testService()
	ctx := testContext()

	trade := models.Trade{
		Ticker: "BHP.AU",
		Action: models.TradeActionBuy,
		Units:  1e16, // exceeds safe range
		Price:  50.0,
		Fees:   10.0,
		Date:   time.Now().Add(-time.Hour),
	}
	_, _, err := svc.AddTrade(ctx, "Stress", trade)
	if err == nil {
		t.Fatal("expected error for extreme units value, got nil")
	}
}

func TestStress_AddTrade_ExtremePrice(t *testing.T) {
	svc := testService()
	ctx := testContext()

	trade := models.Trade{
		Ticker: "BHP.AU",
		Action: models.TradeActionBuy,
		Units:  100,
		Price:  1e16, // exceeds safe range
		Fees:   10.0,
		Date:   time.Now().Add(-time.Hour),
	}
	_, _, err := svc.AddTrade(ctx, "Stress", trade)
	if err == nil {
		t.Fatal("expected error for extreme price value, got nil")
	}
}

// --- String length bombs ---

func TestStress_AddTrade_LongTicker(t *testing.T) {
	svc := testService()
	ctx := testContext()

	trade := models.Trade{
		Ticker: strings.Repeat("A", 10000),
		Action: models.TradeActionBuy,
		Units:  100,
		Price:  50.0,
		Date:   time.Now().Add(-time.Hour),
	}
	_, _, err := svc.AddTrade(ctx, "Stress", trade)
	if err == nil {
		t.Fatal("expected error for extremely long ticker, got nil")
	}
}

func TestStress_AddTrade_LongNotes(t *testing.T) {
	svc := testService()
	ctx := testContext()

	trade := models.Trade{
		Ticker: "BHP.AU",
		Action: models.TradeActionBuy,
		Units:  100,
		Price:  50.0,
		Notes:  strings.Repeat("X", 100000), // 100KB notes
		Date:   time.Now().Add(-time.Hour),
	}
	_, _, err := svc.AddTrade(ctx, "Stress", trade)
	if err == nil {
		t.Fatal("expected error for extremely long notes, got nil")
	}
}

func TestStress_AddTrade_LongSourceRef(t *testing.T) {
	svc := testService()
	ctx := testContext()

	trade := models.Trade{
		Ticker:    "BHP.AU",
		Action:    models.TradeActionBuy,
		Units:     100,
		Price:     50.0,
		SourceRef: strings.Repeat("S", 10000),
		Date:      time.Now().Add(-time.Hour),
	}
	_, _, err := svc.AddTrade(ctx, "Stress", trade)
	if err == nil {
		t.Fatal("expected error for extremely long source_ref, got nil")
	}
}

// --- Whitespace and case in tickers ---

func TestStress_AddTrade_WhitespaceTicker(t *testing.T) {
	svc := testService()
	ctx := testContext()

	// Whitespace-only ticker should fail
	trade := models.Trade{
		Ticker: "   ",
		Action: models.TradeActionBuy,
		Units:  100,
		Price:  50.0,
		Date:   time.Now().Add(-time.Hour),
	}
	_, _, err := svc.AddTrade(ctx, "Stress", trade)
	if err == nil {
		t.Fatal("expected error for whitespace-only ticker, got nil")
	}
}

func TestStress_AddTrade_TickerWithLeadingTrailingSpaces(t *testing.T) {
	svc := testService()
	ctx := testContext()

	base := time.Now().Add(-2 * time.Hour)

	// Add trade with trimmed ticker
	trade1 := models.Trade{
		Ticker: "BHP.AU",
		Action: models.TradeActionBuy,
		Units:  100,
		Price:  50.0,
		Date:   base,
	}
	_, _, err := svc.AddTrade(ctx, "Stress", trade1)
	if err != nil {
		t.Fatalf("first trade failed: %v", err)
	}

	// Add trade with spaces around ticker — should be trimmed and treated as same ticker
	trade2 := models.Trade{
		Ticker: " BHP.AU ",
		Action: models.TradeActionBuy,
		Units:  50,
		Price:  55.0,
		Date:   base.Add(time.Hour),
	}
	_, holding2, err := svc.AddTrade(ctx, "Stress", trade2)
	if err != nil {
		t.Fatalf("second trade (with spaces) failed: %v", err)
	}
	// Should be treated as the same ticker → 150 units
	if holding2.Units != 150 {
		t.Errorf("expected 150 units (tickers should be trimmed), got %f", holding2.Units)
	}
}

// --- Sell validation edge cases ---

func TestStress_SellWithNoBuys(t *testing.T) {
	svc := testService()
	ctx := testContext()

	// Attempt to sell without any buys
	sell := models.Trade{
		Ticker: "BHP.AU",
		Action: models.TradeActionSell,
		Units:  100,
		Price:  50.0,
		Date:   time.Now().Add(-time.Hour),
	}
	_, _, err := svc.AddTrade(ctx, "Stress", sell)
	if err == nil {
		t.Fatal("expected error when selling with no buys, got nil")
	}
	if !strings.Contains(err.Error(), "insufficient") {
		t.Errorf("expected 'insufficient' in error, got: %v", err)
	}
}

func TestStress_SellAfterFullSell(t *testing.T) {
	svc := testService()
	ctx := testContext()

	base := time.Now().Add(-3 * time.Hour)
	// Buy 100
	_, _, err := svc.AddTrade(ctx, "Stress", buyTrade("BHP.AU", 100, 50, 0, base))
	if err != nil {
		t.Fatalf("buy failed: %v", err)
	}

	// Sell all 100
	_, _, err = svc.AddTrade(ctx, "Stress", sellTrade("BHP.AU", 100, 55, 0, base.Add(time.Hour)))
	if err != nil {
		t.Fatalf("full sell failed: %v", err)
	}

	// Try to sell 1 more — should fail
	_, _, err = svc.AddTrade(ctx, "Stress", sellTrade("BHP.AU", 1, 55, 0, base.Add(2*time.Hour)))
	if err == nil {
		t.Fatal("expected error selling after full sell, got nil")
	}
}

// --- DeriveHolding edge cases ---

func TestStress_DeriveHolding_EmptyTrades(t *testing.T) {
	h := DeriveHolding(nil, 0)
	if h.Units != 0 {
		t.Errorf("expected 0 units for empty trades, got %f", h.Units)
	}
	if h.AvgCost != 0 {
		t.Errorf("expected 0 avg_cost for empty trades, got %f", h.AvgCost)
	}
	if h.TradeCount != 0 {
		t.Errorf("expected 0 trade_count, got %d", h.TradeCount)
	}
}

func TestStress_DeriveHolding_ZeroPriceBonus(t *testing.T) {
	// Bonus shares: buy at price 0
	trades := []models.Trade{
		{Action: models.TradeActionBuy, Units: 100, Price: 0, Fees: 0, Date: time.Now()},
	}
	h := DeriveHolding(trades, 50.0)
	if h.Units != 100 {
		t.Errorf("expected 100 units, got %f", h.Units)
	}
	if h.CostBasis != 0 {
		t.Errorf("expected 0 cost_basis for bonus shares, got %f", h.CostBasis)
	}
	// Unrealized should be positive (market value - 0 cost)
	if h.UnrealizedReturn != 5000 {
		t.Errorf("expected unrealized=5000, got %f", h.UnrealizedReturn)
	}
}

func TestStress_DeriveHolding_WithCurrentPrice(t *testing.T) {
	trades := []models.Trade{
		{Action: models.TradeActionBuy, Units: 100, Price: 40, Fees: 0, Date: time.Now()},
	}
	h := DeriveHolding(trades, 50.0)

	expectedMarketValue := 100 * 50.0
	if h.MarketValue != expectedMarketValue {
		t.Errorf("expected market_value=%f, got %f", expectedMarketValue, h.MarketValue)
	}
	expectedUnrealized := expectedMarketValue - 4000
	if h.UnrealizedReturn != expectedUnrealized {
		t.Errorf("expected unrealized=%f, got %f", expectedUnrealized, h.UnrealizedReturn)
	}
}

func TestStress_DeriveHolding_FloatPrecision_ManyBuySellCycles(t *testing.T) {
	// Stress test: 100 buy/sell cycles to detect float drift
	var trades []models.Trade
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	for i := 0; i < 100; i++ {
		// Buy 100 @ 1.11
		trades = append(trades, models.Trade{
			Action: models.TradeActionBuy,
			Units:  100,
			Price:  1.11,
			Fees:   0,
			Date:   base.Add(time.Duration(i*2) * 24 * time.Hour),
		})
		// Sell 100 @ 1.11 (break even)
		trades = append(trades, models.Trade{
			Action: models.TradeActionSell,
			Units:  100,
			Price:  1.11,
			Fees:   0,
			Date:   base.Add(time.Duration(i*2+1) * 24 * time.Hour),
		})
	}

	h := DeriveHolding(trades, 0)

	// After 100 buy/sell cycles at the same price, position should be zero
	if math.Abs(h.Units) > 1e-6 {
		t.Errorf("expected ~0 units after 100 cycles, got %f (float drift)", h.Units)
	}
	if math.Abs(h.CostBasis) > 1e-6 {
		t.Errorf("expected ~0 cost_basis after 100 cycles, got %f (float drift)", h.CostBasis)
	}
	// Realized should be ~0 (break-even cycles)
	if math.Abs(h.RealizedReturn) > 0.01 {
		t.Errorf("expected ~0 realized return after break-even cycles, got %f (float drift)", h.RealizedReturn)
	}
}

func TestStress_DeriveHolding_LargePositionThenFullSell(t *testing.T) {
	// Large position: buy, partial sell, partial sell, ..., full sell
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	var trades []models.Trade

	// Buy 10000 @ $100
	trades = append(trades, models.Trade{
		Action: models.TradeActionBuy,
		Units:  10000,
		Price:  100,
		Fees:   50,
		Date:   base,
	})

	// 10 partial sells of 1000 each
	remaining := 10000.0
	for i := 0; i < 10; i++ {
		trades = append(trades, models.Trade{
			Action: models.TradeActionSell,
			Units:  1000,
			Price:  110,
			Fees:   10,
			Date:   base.Add(time.Duration(i+1) * 24 * time.Hour),
		})
		remaining -= 1000
	}

	h := DeriveHolding(trades, 0)

	if math.Abs(h.Units) > 1e-6 {
		t.Errorf("expected ~0 units after full liquidation, got %f", h.Units)
	}
	// Should have profit: bought at 100, sold at 110
	if h.RealizedReturn <= 0 {
		t.Errorf("expected positive realized return, got %f", h.RealizedReturn)
	}
}

// --- Snapshot edge cases ---

func TestStress_SnapshotDuplicateTickers(t *testing.T) {
	svc := testService()
	ctx := testContext()

	// Import with duplicate tickers
	positions := []models.SnapshotPosition{
		{Ticker: "BHP.AU", Units: 100, AvgCost: 40},
		{Ticker: "BHP.AU", Units: 200, AvgCost: 50}, // duplicate
	}
	tb, err := svc.SnapshotPositions(ctx, "Stress", positions, "replace", "", "2025-01-01")
	if err != nil {
		t.Fatalf("snapshot with duplicates failed: %v", err)
	}

	// Count BHP.AU positions — should ideally be 1 (deduplicated)
	bhpCount := 0
	for _, pos := range tb.SnapshotPositions {
		if pos.Ticker == "BHP.AU" {
			bhpCount++
		}
	}
	// Accept either 1 (deduplicated) or 2 (no dedup), but flag 2 as a concern
	if bhpCount > 1 {
		t.Logf("WARNING: duplicate tickers in snapshot — %d BHP.AU positions (consider deduplication)", bhpCount)
	}
}

func TestStress_SnapshotZeroUnits(t *testing.T) {
	svc := testService()
	ctx := testContext()

	positions := []models.SnapshotPosition{
		{Ticker: "BHP.AU", Units: 0, AvgCost: 40},
	}
	tb, err := svc.SnapshotPositions(ctx, "Stress", positions, "replace", "", "2025-01-01")

	// Zero units is semantically odd but may be acceptable
	if err != nil {
		t.Logf("Correctly rejected zero-units snapshot position: %v", err)
		return
	}
	// If accepted, note it
	if len(tb.SnapshotPositions) > 0 && tb.SnapshotPositions[0].Units == 0 {
		t.Log("WARNING: accepted snapshot position with 0 units — may create noise in portfolio")
	}
}

func TestStress_SnapshotNegativeValues(t *testing.T) {
	svc := testService()
	ctx := testContext()

	positions := []models.SnapshotPosition{
		{Ticker: "BHP.AU", Units: -100, AvgCost: 40},
	}
	_, err := svc.SnapshotPositions(ctx, "Stress", positions, "replace", "", "2025-01-01")
	// Negative units in snapshot should be rejected or flagged
	if err == nil {
		t.Log("WARNING: accepted snapshot position with negative units — could create negative portfolio value")
	}
}

func TestStress_SnapshotNegativeAvgCost(t *testing.T) {
	svc := testService()
	ctx := testContext()

	positions := []models.SnapshotPosition{
		{Ticker: "BHP.AU", Units: 100, AvgCost: -50},
	}
	_, err := svc.SnapshotPositions(ctx, "Stress", positions, "replace", "", "2025-01-01")
	if err == nil {
		t.Log("WARNING: accepted snapshot position with negative avg_cost — could corrupt return calculations")
	}
}

func TestStress_SnapshotEmptyTicker(t *testing.T) {
	svc := testService()
	ctx := testContext()

	positions := []models.SnapshotPosition{
		{Ticker: "", Units: 100, AvgCost: 40},
	}
	_, err := svc.SnapshotPositions(ctx, "Stress", positions, "replace", "", "2025-01-01")
	if err == nil {
		t.Log("WARNING: accepted snapshot position with empty ticker")
	}
}

// --- Concurrent trade operations ---

func TestStress_ConcurrentTradeAdds(t *testing.T) {
	svc := testService()
	ctx := testContext()

	portfolioName := "ConcurrentTest"
	numGoroutines := 20

	var wg sync.WaitGroup
	errors := make([]error, numGoroutines)

	base := time.Now().Add(-24 * time.Hour)
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			trade := models.Trade{
				Ticker: "BHP.AU",
				Action: models.TradeActionBuy,
				Units:  10,
				Price:  50.0,
				Date:   base.Add(time.Duration(idx) * time.Minute),
			}
			_, _, err := svc.AddTrade(ctx, portfolioName, trade)
			errors[idx] = err
		}(i)
	}
	wg.Wait()

	// Check for errors
	errorCount := 0
	for _, err := range errors {
		if err != nil {
			errorCount++
		}
	}

	// Load final trade book
	tb, err := svc.GetTradeBook(ctx, portfolioName)
	if err != nil {
		t.Fatalf("GetTradeBook failed: %v", err)
	}

	// With the full-document save pattern, some trades may be lost due to race conditions
	expectedTrades := numGoroutines - errorCount
	actualTrades := len(tb.Trades)

	if actualTrades < expectedTrades {
		t.Logf("WARNING: Lost %d trades due to concurrent writes (%d added, %d errors, %d stored)",
			expectedTrades-actualTrades, numGoroutines, errorCount, actualTrades)
	}

	// At minimum, at least one trade should have been stored
	if actualTrades == 0 {
		t.Error("no trades stored after concurrent writes — complete data loss")
	}
}

func TestStress_ConcurrentSellRace(t *testing.T) {
	svc := testService()
	ctx := testContext()

	portfolioName := "SellRaceTest"
	base := time.Now().Add(-24 * time.Hour)

	// Buy 100 units
	_, _, err := svc.AddTrade(ctx, portfolioName, buyTrade("BHP.AU", 100, 50, 0, base))
	if err != nil {
		t.Fatalf("initial buy failed: %v", err)
	}

	// Try to sell 60 units from 5 concurrent goroutines
	// Only the first should succeed; at most floor(100/60)=1 can complete
	numGoroutines := 5
	var wg sync.WaitGroup
	results := make([]error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, _, err := svc.AddTrade(ctx, portfolioName, sellTrade("BHP.AU", 60, 55, 0, base.Add(time.Duration(idx+1)*time.Hour)))
			results[idx] = err
		}(i)
	}
	wg.Wait()

	// Count successes and failures
	successes := 0
	for _, err := range results {
		if err == nil {
			successes++
		}
	}

	// Verify final position is not negative
	tb, err := svc.GetTradeBook(ctx, portfolioName)
	if err != nil {
		t.Fatalf("GetTradeBook failed: %v", err)
	}
	h := DeriveHolding(tb.TradesForTicker("BHP.AU"), 0)
	if h.Units < -1e-9 {
		t.Errorf("CRITICAL: negative position after concurrent sells: units=%f (successes=%d)", h.Units, successes)
	}
	if successes > 1 {
		t.Logf("WARNING: %d concurrent sells succeeded (expected 1) — race condition in sell validation", successes)
	}
}

// --- Trade update invalidating sell validation ---

func TestStress_UpdateBuyUnitsAfterSell(t *testing.T) {
	svc := testService()
	ctx := testContext()

	base := time.Now().Add(-3 * time.Hour)

	// Buy 100 units
	created, _, err := svc.AddTrade(ctx, "UpdateStress", buyTrade("BHP.AU", 100, 50, 0, base))
	if err != nil {
		t.Fatalf("buy failed: %v", err)
	}

	// Sell 80 units
	_, _, err = svc.AddTrade(ctx, "UpdateStress", sellTrade("BHP.AU", 80, 55, 0, base.Add(time.Hour)))
	if err != nil {
		t.Fatalf("sell failed: %v", err)
	}

	// Now update the buy trade to only 50 units — this should create an inconsistent state
	// because we already sold 80 which is more than 50
	update := models.Trade{Units: 50}
	_, err = svc.UpdateTrade(ctx, "UpdateStress", created.ID, update)

	// The implementation should either reject this update or recalculate properly
	if err == nil {
		// If accepted, verify the derived position isn't negative
		tb, _ := svc.GetTradeBook(ctx, "UpdateStress")
		h := DeriveHolding(tb.TradesForTicker("BHP.AU"), 0)
		if h.Units < -1e-9 {
			t.Errorf("CRITICAL: update created negative position: units=%f", h.Units)
		}
	}
	// If rejected, that's also acceptable
}

// --- Invalid action type ---

func TestStress_InvalidAction(t *testing.T) {
	svc := testService()
	ctx := testContext()

	trade := models.Trade{
		Ticker: "BHP.AU",
		Action: models.TradeAction("short"),
		Units:  100,
		Price:  50.0,
		Date:   time.Now().Add(-time.Hour),
	}
	_, _, err := svc.AddTrade(ctx, "Stress", trade)
	if err == nil {
		t.Fatal("expected error for invalid action 'short', got nil")
	}
}

func TestStress_EmptyAction(t *testing.T) {
	svc := testService()
	ctx := testContext()

	trade := models.Trade{
		Ticker: "BHP.AU",
		Action: "",
		Units:  100,
		Price:  50.0,
		Date:   time.Now().Add(-time.Hour),
	}
	_, _, err := svc.AddTrade(ctx, "Stress", trade)
	if err == nil {
		t.Fatal("expected error for empty action, got nil")
	}
}

// --- Consideration method edge cases ---

func TestStress_Consideration_FeesExceedProceeds(t *testing.T) {
	// Sell where fees exceed proceeds — negative consideration is mathematically valid
	trade := models.Trade{
		Ticker: "BHP.AU",
		Action: models.TradeActionSell,
		Units:  1,
		Price:  5.0,
		Fees:   50.0, // fees much larger than 1*5=5
	}
	consideration := trade.Consideration()
	expected := 1*5.0 - 50.0 // -45
	if consideration != expected {
		t.Errorf("expected consideration=%f, got %f", expected, consideration)
	}
}

func TestStress_Consideration_InvalidAction(t *testing.T) {
	// Trade with action neither buy nor sell falls through to sell branch
	trade := models.Trade{
		Ticker: "BHP.AU",
		Action: "hold", // invalid
		Units:  100,
		Price:  50.0,
		Fees:   10.0,
	}
	consideration := trade.Consideration()
	// Falls through to sell branch: 100*50 - 10 = 4990
	expected := 100*50.0 - 10.0
	if consideration != expected {
		t.Errorf("invalid action falls through to sell branch: expected %f, got %f", expected, consideration)
	}
}

// --- TradeBook model tests ---

func TestStress_TradeBook_TradesForTicker_CaseSensitive(t *testing.T) {
	tb := &models.TradeBook{
		Trades: []models.Trade{
			{Ticker: "BHP.AU", Action: models.TradeActionBuy, Units: 100, Price: 50, Date: time.Now()},
			{Ticker: "bhp.au", Action: models.TradeActionBuy, Units: 50, Price: 55, Date: time.Now()},
		},
	}

	bhpUpper := tb.TradesForTicker("BHP.AU")
	bhpLower := tb.TradesForTicker("bhp.au")

	// Case-sensitive matching — these are treated as different tickers
	if len(bhpUpper) != 1 {
		t.Errorf("expected 1 trade for BHP.AU, got %d", len(bhpUpper))
	}
	if len(bhpLower) != 1 {
		t.Errorf("expected 1 trade for bhp.au, got %d", len(bhpLower))
	}

	// Flag as a potential data quality issue
	t.Log("NOTE: ticker matching is case-sensitive — 'BHP.AU' and 'bhp.au' are treated as different tickers")
}

func TestStress_TradeBook_UniqueTickers_Empty(t *testing.T) {
	tb := &models.TradeBook{}
	tickers := tb.UniqueTickers()
	if len(tickers) != 0 {
		t.Errorf("expected 0 tickers for empty book, got %d", len(tickers))
	}
}

// --- Portfolio name edge cases ---

func TestStress_AddTrade_EmptyPortfolioName(t *testing.T) {
	svc := testService()
	ctx := testContext()

	trade := models.Trade{
		Ticker: "BHP.AU",
		Action: models.TradeActionBuy,
		Units:  100,
		Price:  50.0,
		Date:   time.Now().Add(-time.Hour),
	}

	// Empty portfolio name — should this be allowed?
	_, _, err := svc.AddTrade(ctx, "", trade)
	if err == nil {
		t.Log("WARNING: accepted trade with empty portfolio name")
	}
}

func TestStress_AddTrade_LongPortfolioName(t *testing.T) {
	svc := testService()
	ctx := testContext()

	trade := models.Trade{
		Ticker: "BHP.AU",
		Action: models.TradeActionBuy,
		Units:  100,
		Price:  50.0,
		Date:   time.Now().Add(-time.Hour),
	}

	longName := strings.Repeat("P", 1000)
	_, _, err := svc.AddTrade(ctx, longName, trade)
	if err == nil {
		t.Log("WARNING: accepted trade with 1000-char portfolio name (no length limit)")
	}
}

// --- ListTrades edge cases ---

func TestStress_ListTrades_NegativeOffset(t *testing.T) {
	svc := testService()
	ctx := testContext()

	_, _, err := svc.ListTrades(ctx, "Stress", TradeFilter{Offset: -1, Limit: 10})
	// Negative offset should be treated as 0 or rejected
	if err != nil {
		return // rejected is fine
	}
	// Accepted is also fine if it behaves like offset=0
}

func TestStress_ListTrades_ZeroLimit(t *testing.T) {
	svc := testService()
	ctx := testContext()

	// Add one trade
	_, _, err := svc.AddTrade(ctx, "ZeroLimitTest", buyTrade("BHP.AU", 100, 50, 0, time.Now().Add(-time.Hour)))
	if err != nil {
		t.Fatalf("AddTrade failed: %v", err)
	}

	trades, _, err := svc.ListTrades(ctx, "ZeroLimitTest", TradeFilter{Limit: 0})
	if err != nil {
		t.Fatalf("ListTrades failed: %v", err)
	}
	// Zero limit should default to 50, not return 0 results
	if len(trades) == 0 {
		t.Log("NOTE: Limit=0 returned 0 trades — should default to 50")
	}
}

func TestStress_ListTrades_ExcessiveLimit(t *testing.T) {
	svc := testService()
	ctx := testContext()

	_, _, err := svc.ListTrades(ctx, "Stress", TradeFilter{Limit: 999999})
	// Should be capped at 200
	if err != nil {
		return // rejected is fine
	}
}

// --- Context handling ---

func TestStress_AddTrade_NoUserContext(t *testing.T) {
	svc := testService()
	ctx := context.Background() // no user context

	trade := models.Trade{
		Ticker: "BHP.AU",
		Action: models.TradeActionBuy,
		Units:  100,
		Price:  50.0,
		Date:   time.Now().Add(-time.Hour),
	}

	// Without user context, the service should handle this gracefully
	_, _, err := svc.AddTrade(ctx, "NoCtxTest", trade)
	// Should either work (with default user ID) or return a clear error
	if err != nil {
		// Acceptable — no user context is an error condition
		return
	}
	// If it worked, verify trades are isolated by checking they're stored
	tb, err := svc.GetTradeBook(ctx, "NoCtxTest")
	if err != nil {
		t.Fatalf("GetTradeBook failed after no-context add: %v", err)
	}
	if len(tb.Trades) != 1 {
		t.Errorf("expected 1 trade, got %d", len(tb.Trades))
	}
}

// suppress unused import warnings
var _ = common.WithUserContext
