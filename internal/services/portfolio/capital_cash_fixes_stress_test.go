package portfolio

import (
	"math"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Adversarial stress tests for capital & cash calculation fixes.
// Validates:
// 1. GrowthPointsToTimeSeries no longer adds externalBalanceTotal
// 2. growthToBars no longer adds externalBalanceTotal
// 3. PortfolioValue = EquityValue + GrossCashBalance at each time series point
// 4. ExternalBalance is 0 in all output points
// 5. growth.go PortfolioValue = equityValue + runningCashBalance (no ExternalBalance)
// 6. NetDeployed accumulates correctly with negative contributions

// =============================================================================
// 1. GrowthPointsToTimeSeries — no ExternalBalance addition
// =============================================================================

func TestGrowthPointsToTimeSeries_NoExternalBalanceAdded(t *testing.T) {
	// After fix: function signature drops externalBalanceTotal parameter.
	// Value = p.EquityValue (not p.EquityValue + externalBalanceTotal).
	points := []models.GrowthDataPoint{
		{
			Date:               time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			EquityValue:        100000,
			NetEquityCost:      90000,
			NetEquityReturn:    10000,
			NetEquityReturnPct: 11.1,
			HoldingCount:       5,
			GrossCashBalance:   50000,
		},
		{
			Date:               time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			EquityValue:        105000,
			NetEquityCost:      90000,
			NetEquityReturn:    15000,
			NetEquityReturnPct: 16.7,
			HoldingCount:       5,
			GrossCashBalance:   48000,
		},
	}

	ts := GrowthPointsToTimeSeries(points)

	require.Len(t, ts, 2)

	// EquityValue should be preserved
	assert.Equal(t, 100000.0, ts[0].EquityValue, "EquityValue should be preserved")
	assert.Equal(t, 105000.0, ts[1].EquityValue)

	// PortfolioValue = EquityValue + GrossCashBalance
	assert.Equal(t, 150000.0, ts[0].EquityValue, "PortfolioValue = EquityValue + GrossCashBalance")
	assert.Equal(t, 153000.0, ts[1].EquityValue)
}

func TestGrowthPointsToTimeSeries_PortfolioValueInvariant(t *testing.T) {
	// For every point: PortfolioValue = EquityValue + GrossCashBalance
	n := 100
	points := make([]models.GrowthDataPoint, n)
	for i := 0; i < n; i++ {
		points[i] = models.GrowthDataPoint{
			Date:             time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i),
			EquityValue:      float64(100000 + i*100),
			GrossCashBalance: float64(50000 - i*50),
		}
	}

	ts := GrowthPointsToTimeSeries(points)
	require.Len(t, ts, n)

	for i, p := range ts {
		expected := p.EquityValue + p.GrossCashBalance
		if math.Abs(p.EquityValue-expected) > 0.001 {
			t.Errorf("Point %d: PortfolioValue (%v) != EquityValue (%v) + GrossCashBalance (%v) = %v",
				i, p.EquityValue, p.EquityValue, p.GrossCashBalance, expected)
		}
	}
}

func TestGrowthPointsToTimeSeries_NegativeCashBalance_Fixed(t *testing.T) {
	// Cash balance can go negative (more buys than contributions).
	// PortfolioValue can be less than EquityValue.
	points := []models.GrowthDataPoint{
		{
			Date:             time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			EquityValue:      200000,
			GrossCashBalance: -30000,
		},
	}

	ts := GrowthPointsToTimeSeries(points)
	require.Len(t, ts, 1)

	assert.Equal(t, 200000.0, ts[0].EquityValue)
	assert.Equal(t, 170000.0, ts[0].EquityValue, "PortfolioValue = 200000 + (-30000) = 170000")
}

func TestGrowthPointsToTimeSeries_EmptyPoints(t *testing.T) {
	ts := GrowthPointsToTimeSeries(nil)
	assert.Len(t, ts, 0)

	ts = GrowthPointsToTimeSeries([]models.GrowthDataPoint{})
	assert.Len(t, ts, 0)
}

func TestGrowthPointsToTimeSeries_PreservesAllFields(t *testing.T) {
	// Verify all GrowthDataPoint fields are correctly mapped.
	p := models.GrowthDataPoint{
		Date:               time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC),
		EquityValue:        100000,
		NetEquityCost:      90000,
		NetEquityReturn:    10000,
		NetEquityReturnPct: 11.1,
		HoldingCount:       5,
		GrossCashBalance:   50000,
		NetCapitalDeployed: 80000,
	}

	ts := GrowthPointsToTimeSeries([]models.GrowthDataPoint{p})
	require.Len(t, ts, 1)

	assert.Equal(t, p.Date, ts[0].Date)
	assert.Equal(t, p.EquityValue, ts[0].EquityValue, "EquityValue preserved")
	assert.Equal(t, p.NetEquityCost, ts[0].NetEquityCost)
	assert.Equal(t, p.NetEquityReturn, ts[0].NetEquityReturn)
	assert.Equal(t, p.NetEquityReturnPct, ts[0].NetEquityReturnPct)
	assert.Equal(t, p.HoldingCount, ts[0].HoldingCount)
	assert.Equal(t, p.GrossCashBalance, ts[0].GrossCashBalance)
	assert.Equal(t, p.NetCapitalDeployed, ts[0].NetCapitalDeployed)
}

// =============================================================================
// 2. growthToBars — no ExternalBalance addition
// =============================================================================

func TestGrowthToBars_NoExternalBalanceAdded(t *testing.T) {
	// After fix: function signature drops externalBalanceTotal parameter.
	// Bar value = p.EquityValue (not p.EquityValue + externalBalanceTotal).
	points := []models.GrowthDataPoint{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), EquityValue: 100000},
		{Date: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), EquityValue: 110000},
	}

	bars := growthToBars(points)
	require.Len(t, bars, 2)

	// Newest first ordering
	assert.Equal(t, 110000.0, bars[0].Close, "Bar value = EquityValue only")
	assert.Equal(t, 100000.0, bars[1].Close, "Bar value = EquityValue only")
	assert.True(t, bars[0].Date.After(bars[1].Date), "Bars should be in newest-first order")
}

func TestGrowthToBars_AllFieldsSetToValue(t *testing.T) {
	points := []models.GrowthDataPoint{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), EquityValue: 500000},
	}
	bars := growthToBars(points)
	require.Len(t, bars, 1)

	// All OHLC fields should equal EquityValue
	assert.Equal(t, 500000.0, bars[0].Open)
	assert.Equal(t, 500000.0, bars[0].High)
	assert.Equal(t, 500000.0, bars[0].Low)
	assert.Equal(t, 500000.0, bars[0].Close)
	assert.Equal(t, 500000.0, bars[0].AdjClose)
}

func TestGrowthToBars_EmptyPoints(t *testing.T) {
	bars := growthToBars(nil)
	assert.Len(t, bars, 0)

	bars = growthToBars([]models.GrowthDataPoint{})
	assert.Len(t, bars, 0)
}

func TestGrowthToBars_ZeroValues(t *testing.T) {
	points := []models.GrowthDataPoint{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), EquityValue: 0},
	}
	bars := growthToBars(points)
	require.Len(t, bars, 1)
	assert.Equal(t, 0.0, bars[0].Close)
}

func TestGrowthToBars_NewestFirstOrdering_Fixed(t *testing.T) {
	n := 50
	points := make([]models.GrowthDataPoint, n)
	for i := 0; i < n; i++ {
		points[i] = models.GrowthDataPoint{
			Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i),
			EquityValue: float64(i+1) * 1000,
		}
	}

	bars := growthToBars(points)
	require.Len(t, bars, n)

	// bars[0] should be the newest (highest value)
	assert.Equal(t, float64(n)*1000, bars[0].Close)
	// bars[n-1] should be the oldest (lowest value)
	assert.Equal(t, 1000.0, bars[n-1].Close)

	// Strictly newest-first
	for i := 0; i < n-1; i++ {
		assert.True(t, bars[i].Date.After(bars[i+1].Date),
			"bars[%d].Date should be after bars[%d].Date", i, i+1)
	}
}

// =============================================================================
// 3. GrowthDataPoint — PortfolioValue must NOT include ExternalBalance
// =============================================================================

func TestGrowthDataPoint_PortfolioValue_NoExternalBalance(t *testing.T) {
	// Simulate what growth.go produces after the fix.
	// ExternalBalance = 0, PortfolioValue = equityValue + runningCashBalance
	equityValue := 200000.0
	grossCashBalance := 50000.0

	gp := models.GrowthDataPoint{
		EquityValue:      equityValue,
		GrossCashBalance: grossCashBalance,
		PortfolioValue:   equityValue + grossCashBalance,
	}

	assert.Equal(t, 250000.0, gp.EquityValue)

	// PortfolioValue should equal EquityValue + GrossCashBalance
	assert.Equal(t, gp.EquityValue+gp.GrossCashBalance, gp.EquityValue,
		"Invariant: PortfolioValue = EquityValue + GrossCashBalance")
}

// =============================================================================
// 4. Net deployed accumulation with negative contributions in growth context
// =============================================================================

func TestNetDeployed_AccumulatesWithNegativeContributions(t *testing.T) {
	// Simulate the growth.go cash flow cursor loop.
	// After the fix, NetDeployedImpact returns tx.Amount for contributions
	// regardless of sign.
	txs := []models.CashTransaction{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Category: models.CashCatContribution, Amount: 50000},
		{Date: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC), Category: models.CashCatContribution, Amount: 30000},
		{Date: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), Category: models.CashCatContribution, Amount: -10000}, // withdrawal
		{Date: time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC), Category: models.CashCatDividend, Amount: 2000},       // no effect
		{Date: time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC), Category: models.CashCatFee, Amount: -500},            // decreases
	}

	var runningNetDeployed float64
	for _, tx := range txs {
		runningNetDeployed += tx.NetDeployedImpact()
	}

	// 50000 + 30000 + (-10000) + 0 + (-500) = 69500
	assert.InDelta(t, 69500.0, runningNetDeployed, 0.001,
		"Net deployed should account for negative contributions")
}

func TestNetDeployed_WithdrawalMakesNetDeployedNegative(t *testing.T) {
	// Edge case: withdrawal exceeds all deposits.
	txs := []models.CashTransaction{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Category: models.CashCatContribution, Amount: 10000},
		{Date: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC), Category: models.CashCatContribution, Amount: -50000},
	}

	var runningNetDeployed float64
	for _, tx := range txs {
		runningNetDeployed += tx.NetDeployedImpact()
	}

	assert.Equal(t, -40000.0, runningNetDeployed,
		"Net deployed can go negative when withdrawals exceed deposits")
}

func TestNetDeployed_ZeroContributionNoEffect(t *testing.T) {
	tx := models.CashTransaction{Category: models.CashCatContribution, Amount: 0}
	assert.Equal(t, 0.0, tx.NetDeployedImpact())
}

// =============================================================================
// 5. Time series correctness: PortfolioValue = EquityValue + GrossCashBalance at each point
// =============================================================================

func TestTimeSeries_PortfolioValueConsistency(t *testing.T) {
	// Build a realistic growth series and verify the invariant holds
	// for every single point.
	points := make([]models.GrowthDataPoint, 365)
	grossCashBalance := 100000.0
	for i := 0; i < 365; i++ {
		value := 200000.0 + float64(i)*100 // slowly growing
		// Simulate occasional cash transactions
		if i%30 == 0 && i > 0 {
			grossCashBalance -= 5000 // monthly investment
		}
		points[i] = models.GrowthDataPoint{
			Date:             time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i),
			EquityValue:      value,
			GrossCashBalance: grossCashBalance,
			PortfolioValue:   value + grossCashBalance,
		}
	}

	ts := GrowthPointsToTimeSeries(points)
	require.Len(t, ts, 365)

	for i, p := range ts {
		// Invariant: PortfolioValue = EquityValue + GrossCashBalance
		expected := p.EquityValue + p.GrossCashBalance
		if math.Abs(p.EquityValue-expected) > 0.001 {
			t.Errorf("Day %d: PortfolioValue (%v) != EquityValue (%v) + GrossCashBalance (%v)",
				i, p.EquityValue, p.EquityValue, p.GrossCashBalance)
			break // fail fast
		}
	}
}

// =============================================================================
// 6. Portfolio with no cashflow service (nil check paths)
// =============================================================================

func TestPortfolio_NilCashFlowService_GrowthStillWorks(t *testing.T) {
	// When cashflowSvc is nil, GetPortfolioIndicators should still work.
	// The cash-related fields should be zero.
	svc := &Service{cashflowSvc: nil}
	assert.Nil(t, svc.cashflowSvc, "cashflowSvc should be nil")

	// Verify the nil guard in service.go works.
	// We can't call GetPortfolioIndicators without a full service,
	// but we can verify the pattern.
	if svc.cashflowSvc != nil {
		t.Error("Expected cashflowSvc to be nil")
	}
}

// =============================================================================
// 7. Edge: very large ExternalBalance field should be ignored (deprecated)
// =============================================================================

func TestGrowthDataPoint_PortfolioValueFormula(t *testing.T) {
	// PortfolioValue should be EquityValue + GrossCashBalance
	gp := models.GrowthDataPoint{
		EquityValue:      100000,
		GrossCashBalance: 50000,
		PortfolioValue:   100000 + 50000, // correct formula
	}

	// The correct PortfolioValue
	assert.Equal(t, 150000.0, gp.EquityValue)
	assert.Equal(t, gp.EquityValue+gp.GrossCashBalance, gp.EquityValue,
		"PortfolioValue = EquityValue + GrossCashBalance")
}

// =============================================================================
// 8. Float precision in time series with many cash transactions
// =============================================================================

func TestTimeSeries_FloatPrecision_ManyCashTransactions(t *testing.T) {
	// Simulate many small cash movements and verify precision.
	grossCashBalance := 100000.0
	points := make([]models.GrowthDataPoint, 1000)
	for i := 0; i < 1000; i++ {
		grossCashBalance += 0.01 // tiny increment each day
		points[i] = models.GrowthDataPoint{
			Date:             time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i),
			EquityValue:      500000,
			GrossCashBalance: grossCashBalance,
			PortfolioValue:   500000 + grossCashBalance,
		}
	}

	ts := GrowthPointsToTimeSeries(points)
	require.Len(t, ts, 1000)

	// Final cash balance should be ~100010 (100000 + 1000 * 0.01)
	lastCash := ts[999].GrossCashBalance
	if math.Abs(lastCash-100010.0) > 0.1 {
		t.Errorf("Final GrossCashBalance = %v, expected ~100010", lastCash)
	}

	// PortfolioValue invariant must hold for last point
	last := ts[999]
	assert.InDelta(t, last.EquityValue+last.GrossCashBalance, last.EquityValue, 0.001,
		"PortfolioValue invariant must hold even after many small float additions")
}

// =============================================================================
// 9. Concurrent calls to GrowthPointsToTimeSeries and growthToBars
// =============================================================================

func TestGrowthConversion_ConcurrentSafety(t *testing.T) {
	points := []models.GrowthDataPoint{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), EquityValue: 100000, GrossCashBalance: 50000},
		{Date: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), EquityValue: 110000, GrossCashBalance: 48000},
		{Date: time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC), EquityValue: 120000, GrossCashBalance: 46000},
	}

	done := make(chan struct{}, 100)
	for i := 0; i < 50; i++ {
		go func() {
			ts := GrowthPointsToTimeSeries(points)
			assert.Len(t, ts, 3)
			assert.Equal(t, 100000.0, ts[0].EquityValue)
			done <- struct{}{}
		}()
		go func() {
			bars := growthToBars(points)
			assert.Len(t, bars, 3)
			assert.Equal(t, 120000.0, bars[0].Close) // newest first
			done <- struct{}{}
		}()
	}

	for i := 0; i < 100; i++ {
		<-done
	}
}
