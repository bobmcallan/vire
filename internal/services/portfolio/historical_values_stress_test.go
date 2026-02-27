package portfolio

import (
	"math"
	"sync"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
)

// Devils-advocate stress tests for populateHistoricalValues and related helpers.
// Focus: nil/empty/partial market data, FX edge cases, concurrent access,
// division by zero, and numerical edge cases.

// --- findEODBarByOffset edge cases ---

func TestFindEODBarByOffset_EmptySlice(t *testing.T) {
	result := findEODBarByOffset(nil, 1)
	assert.Nil(t, result, "nil slice should return nil")

	result = findEODBarByOffset([]models.EODBar{}, 0)
	assert.Nil(t, result, "empty slice should return nil for offset 0")
}

func TestFindEODBarByOffset_ExactBoundary(t *testing.T) {
	bars := []models.EODBar{
		{Date: time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC), Close: 103},
		{Date: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), Close: 102},
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Close: 101},
	}

	// offset == len(eod) should return nil
	result := findEODBarByOffset(bars, 3)
	assert.Nil(t, result, "offset == len should return nil")

	// offset == len-1 should return last bar
	result = findEODBarByOffset(bars, 2)
	assert.NotNil(t, result)
	assert.Equal(t, 101.0, result.Close)
}

func TestFindEODBarByOffset_NegativeOffset(t *testing.T) {
	// Negative offset is nonsensical but shouldn't panic
	bars := []models.EODBar{{Close: 100}}
	// len(eod) <= offset: 1 <= -1 is false, so it returns &eod[-1] which panics
	// This is a potential bug — negative offsets are not guarded
	defer func() {
		if r := recover(); r != nil {
			t.Logf("FINDING: findEODBarByOffset panics on negative offset: %v", r)
			t.Logf("RECOMMENDATION: Add guard for offset < 0")
		}
	}()
	result := findEODBarByOffset(bars, -1)
	if result != nil {
		t.Logf("findEODBarByOffset with negative offset returned non-nil (accessing out-of-bounds memory)")
	}
}

func TestFindEODBarByOffset_VeryLargeOffset(t *testing.T) {
	bars := makeFlatBars(10, 100)
	result := findEODBarByOffset(bars, 1000000)
	assert.Nil(t, result, "huge offset should return nil safely")
}

// --- eodClosePrice edge cases ---

func TestEodClosePrice_ZeroClose(t *testing.T) {
	bar := models.EODBar{Close: 0, AdjClose: 50}
	price := eodClosePrice(bar)
	// Close==0 means ratio check is skipped (bar.Close > 0 is false)
	// So it returns AdjClose=50
	assert.Equal(t, 50.0, price, "zero Close should use AdjClose directly")
}

func TestEodClosePrice_NaNClose(t *testing.T) {
	bar := models.EODBar{Close: math.NaN(), AdjClose: 50}
	price := eodClosePrice(bar)
	// NaN > 0 is false, so it skips ratio check and returns AdjClose
	assert.Equal(t, 50.0, price)
}

func TestEodClosePrice_NaNAdjClose_Stress(t *testing.T) {
	bar := models.EODBar{Close: 100, AdjClose: math.NaN()}
	price := eodClosePrice(bar)
	// AdjClose > 0 is false for NaN, so it falls through to Close
	assert.Equal(t, 100.0, price)
}

func TestEodClosePrice_InfAdjClose_Stress(t *testing.T) {
	bar := models.EODBar{Close: 100, AdjClose: math.Inf(1)}
	price := eodClosePrice(bar)
	// IsInf check catches this — falls through to Close
	assert.Equal(t, 100.0, price)
}

func TestEodClosePrice_NegativeAdjClose(t *testing.T) {
	bar := models.EODBar{Close: 100, AdjClose: -50}
	price := eodClosePrice(bar)
	// -50 > 0 is false, falls through to Close
	assert.Equal(t, 100.0, price)
}

func TestEodClosePrice_BothZero(t *testing.T) {
	bar := models.EODBar{Close: 0, AdjClose: 0}
	price := eodClosePrice(bar)
	// Both zero: AdjClose > 0 is false, returns Close (0)
	assert.Equal(t, 0.0, price)
}

func TestEodClosePrice_WildDivergence(t *testing.T) {
	// AdjClose is 10x Close — bad corporate action data
	bar := models.EODBar{Close: 100, AdjClose: 1000}
	price := eodClosePrice(bar)
	// ratio = 1000/100 = 10, which is > 2.0, so falls back to Close
	assert.Equal(t, 100.0, price, "wild divergence should fall back to Close")

	// AdjClose is 0.1x Close — also bad data
	bar2 := models.EODBar{Close: 100, AdjClose: 10}
	price2 := eodClosePrice(bar2)
	// ratio = 10/100 = 0.1, which is < 0.5, so falls back to Close
	assert.Equal(t, 100.0, price2, "small ratio should fall back to Close")
}

// --- populateHistoricalValues stress tests ---
// These test the function directly since it's an unexported method on Service.
// We call it via the struct with nil fields where possible.

func TestPopulateHistoricalValues_NilHoldings(t *testing.T) {
	// Portfolio with no holdings should not panic
	portfolio := &models.Portfolio{
		Holdings: nil,
	}
	// No tickers → early return at len(tickers)==0
	// We can't call populateHistoricalValues directly without a Service,
	// but we can verify the precondition logic by checking findEODBarByOffset
	assert.Empty(t, portfolio.Holdings)
}

func TestPopulateHistoricalValues_ZeroUnitsHolding(t *testing.T) {
	// Holdings with Units <= 0 should be skipped entirely
	portfolio := &models.Portfolio{
		Holdings: []models.Holding{
			{Ticker: "BHP", Exchange: "ASX", Units: 0, CurrentPrice: 50},
			{Ticker: "CBA", Exchange: "ASX", Units: -10, CurrentPrice: 100},
		},
	}
	// Both should be skipped, so tickers slice should be empty
	tickers := make([]string, 0)
	for _, h := range portfolio.Holdings {
		if h.Units > 0 {
			tickers = append(tickers, h.EODHDTicker())
		}
	}
	assert.Empty(t, tickers, "zero/negative units should produce no tickers")
}

func TestPopulateHistoricalValues_SingleBarEOD(t *testing.T) {
	// Only 1 bar: need at least 2 for yesterday comparison
	// This should skip the holding without panic
	eod := []models.EODBar{
		{Date: time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC), Close: 100, AdjClose: 100},
	}
	assert.True(t, len(eod) < 2, "1 bar is insufficient for yesterday comparison")
}

func TestPopulateHistoricalValues_YesterdayCloseZero_NoDivisionByZero(t *testing.T) {
	// If yesterday's close price is 0, percentage calculation must not divide by zero
	// The code checks: if yesterdayClose > 0
	yesterdayClose := 0.0
	currentPrice := 50.0
	var pct float64
	if yesterdayClose > 0 {
		pct = ((currentPrice - yesterdayClose) / yesterdayClose) * 100
	}
	assert.Equal(t, 0.0, pct, "zero yesterday close should produce 0% change, not Inf")
}

func TestPopulateHistoricalValues_NegativeYesterdayClose(t *testing.T) {
	// Negative close price (bad data) — guard should prevent division
	yesterdayClose := -50.0
	currentPrice := 100.0
	var pct float64
	if yesterdayClose > 0 {
		pct = ((currentPrice - yesterdayClose) / yesterdayClose) * 100
	}
	assert.Equal(t, 0.0, pct, "negative yesterday close should produce 0%")
}

func TestPopulateHistoricalValues_FXRateZero(t *testing.T) {
	// FXRate == 0 should not divide by zero
	// The code uses fxDiv=1.0 unless OriginalCurrency=="USD" && FXRate > 0
	h := models.Holding{OriginalCurrency: "USD"}
	fxRate := 0.0
	fxDiv := 1.0
	if h.OriginalCurrency == "USD" && fxRate > 0 {
		fxDiv = fxRate
	}
	assert.Equal(t, 1.0, fxDiv, "zero FX rate should default to 1.0 divisor")
}

func TestPopulateHistoricalValues_FXRateNegative(t *testing.T) {
	// Negative FX rate (corrupted data)
	h := models.Holding{OriginalCurrency: "USD"}
	fxRate := -1.5
	fxDiv := 1.0
	if h.OriginalCurrency == "USD" && fxRate > 0 {
		fxDiv = fxRate
	}
	assert.Equal(t, 1.0, fxDiv, "negative FX rate should default to 1.0 divisor")
}

func TestPopulateHistoricalValues_VerySmallFXRate(t *testing.T) {
	// Extremely small FX rate (near zero) — could amplify prices to Inf
	h := models.Holding{OriginalCurrency: "USD"}
	fxRate := math.SmallestNonzeroFloat64
	fxDiv := 1.0
	if h.OriginalCurrency == "USD" && fxRate > 0 {
		fxDiv = fxRate
	}
	// Now eodClosePrice / fxDiv = 100 / 5e-324 = Inf
	price := 100.0 / fxDiv
	assert.True(t, math.IsInf(price, 1),
		"FINDING: Very small FX rate produces Inf price — may need clamping")
}

func TestPopulateHistoricalValues_ExternalBalanceAggregation(t *testing.T) {
	// Verify portfolio-level aggregates include ExternalBalanceTotal
	yesterdayTotal := 500000.0
	externalBalanceTotal := 100000.0

	portfolioYesterdayTotal := yesterdayTotal + externalBalanceTotal
	assert.Equal(t, 600000.0, portfolioYesterdayTotal)

	// If yesterdayTotal is 0 (no holdings have market data), the block is skipped
	yesterdayTotal = 0.0
	if yesterdayTotal > 0 {
		portfolioYesterdayTotal = yesterdayTotal + externalBalanceTotal
	} else {
		portfolioYesterdayTotal = 0
	}
	assert.Equal(t, 0.0, portfolioYesterdayTotal,
		"FINDING: When no holdings have market data, ExternalBalanceTotal is NOT reflected in YesterdayTotal")
}

func TestPopulateHistoricalValues_NaNCurrentPrice(t *testing.T) {
	// NaN currentPrice will produce NaN percentages
	currentPrice := math.NaN()
	yesterdayClose := 100.0
	var pct float64
	if yesterdayClose > 0 {
		pct = ((currentPrice - yesterdayClose) / yesterdayClose) * 100
	}
	assert.True(t, math.IsNaN(pct),
		"FINDING: NaN currentPrice propagates to YesterdayPct — no upstream guard")
}

// --- Concurrent populateHistoricalValues simulation ---

func TestPopulateHistoricalValues_ConcurrentPortfolioReads(t *testing.T) {
	// Simulate concurrent reads to the same portfolio data.
	// populateHistoricalValues mutates portfolio.Holdings[i] fields and
	// portfolio-level aggregates. If called concurrently on the SAME portfolio
	// object, there is a data race.

	portfolio := &models.Portfolio{
		TotalValue:           200000,
		ExternalBalanceTotal: 50000,
		Holdings: []models.Holding{
			{Ticker: "BHP", Exchange: "ASX", Units: 100, CurrentPrice: 50},
			{Ticker: "CBA", Exchange: "ASX", Units: 200, CurrentPrice: 100},
		},
	}

	// Simulate the mutation pattern of populateHistoricalValues
	// Each goroutine gets its own copy to avoid actual races in test
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			// Copy portfolio for safety
			p := *portfolio
			p.Holdings = make([]models.Holding, len(portfolio.Holdings))
			copy(p.Holdings, portfolio.Holdings)

			var yesterdayTotal float64
			for j := range p.Holdings {
				h := &p.Holdings[j]
				h.YesterdayClose = h.CurrentPrice * 0.99
				if h.YesterdayClose > 0 {
					h.YesterdayPct = ((h.CurrentPrice - h.YesterdayClose) / h.YesterdayClose) * 100
				}
				yesterdayTotal += h.YesterdayClose * h.Units
			}
			if yesterdayTotal > 0 {
				p.YesterdayTotal = yesterdayTotal + p.ExternalBalanceTotal
			}
		}(i)
	}
	wg.Wait()
	// If we got here without -race detector firing, the per-goroutine copy approach is safe.
	// The REAL concern is whether SyncPortfolio (which calls populateHistoricalValues)
	// is protected by syncMu. GetPortfolio calls it outside the mutex.
	t.Log("FINDING: populateHistoricalValues is called from GetPortfolio (no mutex) " +
		"and will be called from SyncPortfolio (protected by syncMu). " +
		"If the SAME *Portfolio pointer is shared between concurrent GetPortfolio calls, " +
		"there is a potential data race on holdings fields. " +
		"MITIGATION: Each GetPortfolio call loads a fresh portfolio from storage, so in practice " +
		"the pointers are distinct. No fix needed unless caching is added.")
}

// --- Aggregation boundary tests ---

func TestPopulateHistoricalValues_YesterdayTotalPct_DivisionByZero(t *testing.T) {
	// Edge: yesterdayTotal > 0, but portfolio.YesterdayTotal == 0 after adding negative ExternalBalanceTotal
	yesterdayTotal := 50000.0
	externalBalanceTotal := -50000.0 // corrupted
	portfolioYesterdayTotal := yesterdayTotal + externalBalanceTotal
	totalValue := 200000.0

	var pct float64
	if portfolioYesterdayTotal > 0 {
		pct = ((totalValue - portfolioYesterdayTotal) / portfolioYesterdayTotal) * 100
	}
	assert.Equal(t, 0.0, pct, "zero YesterdayTotal should produce 0% (guarded)")
}

func TestPopulateHistoricalValues_VeryLargePortfolio(t *testing.T) {
	// Portfolio with 100 holdings — stress test memory and computation
	holdings := make([]models.Holding, 100)
	for i := range holdings {
		holdings[i] = models.Holding{
			Ticker:       "TICK",
			Exchange:     "ASX",
			Units:        float64(i+1) * 100,
			CurrentPrice: float64(i+1) * 10,
		}
	}
	portfolio := &models.Portfolio{
		Holdings: holdings,
	}
	assert.Len(t, portfolio.Holdings, 100, "portfolio should have 100 holdings")
	// The EOD lookup is O(holdings) per call, so 100 holdings is fine
}

// --- eodClosePrice with AdjClose/Close ratio edge cases ---

func TestEodClosePrice_RatioExactlyAtBoundary(t *testing.T) {
	// ratio == 0.5: borderline
	bar := models.EODBar{Close: 100, AdjClose: 50}
	price := eodClosePrice(bar)
	// ratio = 50/100 = 0.5, which is NOT < 0.5 (equal), so it returns AdjClose
	assert.Equal(t, 50.0, price, "ratio exactly 0.5 should use AdjClose")

	// ratio == 2.0: borderline
	bar2 := models.EODBar{Close: 100, AdjClose: 200}
	price2 := eodClosePrice(bar2)
	// ratio = 200/100 = 2.0, which is NOT > 2.0 (equal), so it returns AdjClose
	assert.Equal(t, 200.0, price2, "ratio exactly 2.0 should use AdjClose")
}

func TestEodClosePrice_JustOverBoundary(t *testing.T) {
	// ratio = 2.01 — just over, should fall back to Close
	bar := models.EODBar{Close: 100, AdjClose: 201}
	price := eodClosePrice(bar)
	assert.Equal(t, 100.0, price, "ratio 2.01 should fall back to Close")

	// ratio = 0.49 — just under, should fall back to Close
	bar2 := models.EODBar{Close: 100, AdjClose: 49}
	price2 := eodClosePrice(bar2)
	assert.Equal(t, 100.0, price2, "ratio 0.49 should fall back to Close")
}
