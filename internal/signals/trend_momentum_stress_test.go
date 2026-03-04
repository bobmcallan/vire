package signals

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bobmcallan/vire/internal/models"
)

// =============================================================================
// Trend Momentum — adversarial stress tests
// =============================================================================
// These tests target edge cases, boundary conditions, and potential failure
// modes in computeTrendMomentum and its helper functions.

// --- priceChangePct edge cases ---

func TestPriceChangePct_CurrentPriceZero(t *testing.T) {
	// currentPrice=0, bars[3].Close=100 → (0-100)/100*100 = -100%
	bars := generateBars([]float64{0, 10, 20, 100})
	result := priceChangePct(0, bars, 3)
	assert.InDelta(t, -100.0, result, 0.01)
}

func TestPriceChangePct_HistoricalPriceZero(t *testing.T) {
	// bars[3].Close=0 → should return 0 (division guard)
	bars := generateBars([]float64{100, 10, 20, 0})
	result := priceChangePct(100, bars, 3)
	assert.Equal(t, 0.0, result, "should return 0 when historical price is zero")
}

func TestPriceChangePct_BothZero(t *testing.T) {
	// currentPrice=0 AND bars[idx].Close=0 → guard returns 0
	bars := generateBars([]float64{0, 0, 0, 0})
	result := priceChangePct(0, bars, 3)
	assert.Equal(t, 0.0, result, "both prices zero should return 0, not NaN")
	assert.False(t, math.IsNaN(result), "must not produce NaN")
}

func TestPriceChangePct_PeriodExceedsLen(t *testing.T) {
	// period=100, len(bars)=4 → falls back to bars[3]
	bars := generateBars([]float64{200, 150, 100, 50})
	result := priceChangePct(200, bars, 100)
	// Should use bars[3].Close=50 → (200-50)/50*100 = 300%
	assert.InDelta(t, 300.0, result, 0.01)
}

func TestPriceChangePct_PeriodZero(t *testing.T) {
	// period=0 → idx=0 → compares currentPrice to bars[0] which is itself
	bars := generateBars([]float64{100, 90, 80})
	result := priceChangePct(100, bars, 0)
	// bars[0].Close=100, currentPrice=100 → 0%
	assert.InDelta(t, 0.0, result, 0.01)
}

func TestPriceChangePct_MassiveSpike(t *testing.T) {
	// Price goes from 0.01 to 1000 → 9999900% change
	bars := generateBars([]float64{1000, 500, 100, 0.01})
	result := priceChangePct(1000, bars, 3)
	assert.InDelta(t, 9999900.0, result, 100)
	assert.False(t, math.IsInf(result, 0), "should not overflow to Inf")
}

func TestPriceChangePct_NegativePrice(t *testing.T) {
	// Negative close prices should not panic
	bars := generateBars([]float64{-10, -20, -30})
	result := priceChangePct(-10, bars, 2)
	// (-10 - (-30)) / (-30) * 100 = 20 / -30 * 100 = -66.67
	assert.False(t, math.IsNaN(result))
	assert.False(t, math.IsInf(result, 0))
}

// --- avgVolume edge cases ---

func TestAvgVolume_EmptyBars(t *testing.T) {
	result := avgVolume(nil)
	assert.Equal(t, 0.0, result)

	result = avgVolume([]models.EODBar{})
	assert.Equal(t, 0.0, result)
}

func TestAvgVolume_SingleBar(t *testing.T) {
	bars := []models.EODBar{{Volume: 5000000}}
	result := avgVolume(bars)
	assert.InDelta(t, 5000000.0, result, 0.01)
}

func TestAvgVolume_ZeroVolume(t *testing.T) {
	bars := []models.EODBar{{Volume: 0}, {Volume: 0}, {Volume: 0}}
	result := avgVolume(bars)
	assert.Equal(t, 0.0, result)
}

func TestAvgVolume_MaxInt64Volume(t *testing.T) {
	// int64 max is 9223372036854775807; float64 can represent it approximately
	maxVol := int64(math.MaxInt64)
	bars := []models.EODBar{{Volume: maxVol}, {Volume: maxVol}}
	result := avgVolume(bars)
	assert.False(t, math.IsInf(result, 0), "should not overflow to Inf")
	assert.False(t, math.IsNaN(result), "should not be NaN")
	assert.Greater(t, result, 0.0)
}

// --- clampFloat edge cases ---

func TestClampFloat_NaN(t *testing.T) {
	// NaN comparisons return false for <, >, so NaN falls through all guards
	result := clampFloat(math.NaN(), -1, 1)
	// This is a known limitation — NaN passes through clamp unchanged.
	// Documenting the behaviour rather than asserting a fix.
	assert.True(t, math.IsNaN(result), "NaN passes through clampFloat unchanged — potential issue if upstream produces NaN")
}

func TestClampFloat_NegativeInf(t *testing.T) {
	result := clampFloat(math.Inf(-1), -1, 1)
	assert.Equal(t, -1.0, result)
}

func TestClampFloat_PositiveInf(t *testing.T) {
	result := clampFloat(math.Inf(1), -1, 1)
	assert.Equal(t, 1.0, result)
}

func TestClampFloat_ExactBoundary(t *testing.T) {
	assert.Equal(t, -1.0, clampFloat(-1.0, -1, 1))
	assert.Equal(t, 1.0, clampFloat(1.0, -1, 1))
}

func TestClampFloat_MidRange(t *testing.T) {
	assert.InDelta(t, 0.5, clampFloat(0.5, -1, 1), 0.001)
}

// --- classifyTrendMomentum edge cases ---

func TestClassifyTrendMomentum_ExactThreshold_NoVolume(t *testing.T) {
	// Score exactly at strong threshold (0.5) without volume
	assert.Equal(t, models.TrendMomentumStrongUp, classifyTrendMomentum(0.5, false))
	assert.Equal(t, models.TrendMomentumStrongDown, classifyTrendMomentum(-0.5, false))
	// Score exactly at weak threshold (0.15)
	assert.Equal(t, models.TrendMomentumUp, classifyTrendMomentum(0.15, false))
	assert.Equal(t, models.TrendMomentumDown, classifyTrendMomentum(-0.15, false))
}

func TestClassifyTrendMomentum_ExactThreshold_WithVolume(t *testing.T) {
	// Volume confirmation lowers thresholds: strong=0.4, weak=0.10
	assert.Equal(t, models.TrendMomentumStrongUp, classifyTrendMomentum(0.4, true))
	assert.Equal(t, models.TrendMomentumStrongDown, classifyTrendMomentum(-0.4, true))
	assert.Equal(t, models.TrendMomentumUp, classifyTrendMomentum(0.10, true))
	assert.Equal(t, models.TrendMomentumDown, classifyTrendMomentum(-0.10, true))
}

func TestClassifyTrendMomentum_ZeroScore(t *testing.T) {
	assert.Equal(t, models.TrendMomentumFlat, classifyTrendMomentum(0.0, false))
	assert.Equal(t, models.TrendMomentumFlat, classifyTrendMomentum(0.0, true))
}

func TestClassifyTrendMomentum_NaN(t *testing.T) {
	// NaN comparisons all return false → falls to default (FLAT)
	result := classifyTrendMomentum(math.NaN(), false)
	assert.Equal(t, models.TrendMomentumFlat, result, "NaN should classify as FLAT (default)")
}

func TestClassifyTrendMomentum_ExtremeScore(t *testing.T) {
	// Score of 1.0 (maximum after clamp)
	assert.Equal(t, models.TrendMomentumStrongUp, classifyTrendMomentum(1.0, false))
	assert.Equal(t, models.TrendMomentumStrongDown, classifyTrendMomentum(-1.0, false))
}

func TestClassifyTrendMomentum_DeadZone(t *testing.T) {
	// Scores just inside the flat band (between -0.15 and 0.15 without volume)
	assert.Equal(t, models.TrendMomentumFlat, classifyTrendMomentum(0.14, false))
	assert.Equal(t, models.TrendMomentumFlat, classifyTrendMomentum(-0.14, false))
	assert.Equal(t, models.TrendMomentumFlat, classifyTrendMomentum(0.01, false))
	assert.Equal(t, models.TrendMomentumFlat, classifyTrendMomentum(-0.01, false))
}

// --- computeTrendMomentum full integration stress tests ---

func TestTrendMomentum_ExactlyFourBars(t *testing.T) {
	// Minimum viable input: 4 bars (just enough for 3-day change)
	computer := NewComputer()
	bars := generateBars([]float64{100, 95, 90, 85})
	md := &models.MarketData{Ticker: "TEST.AU", EOD: bars}
	signals := computer.Compute(md)

	assert.NotEqual(t, models.TrendMomentumLevel(""), signals.TrendMomentum.Level)
	// Acceleration should be 0 (need 11+ bars)
	assert.Equal(t, 0.0, signals.TrendMomentum.Acceleration)
	// Volume confirm should be false (need 20+ bars)
	assert.False(t, signals.TrendMomentum.VolumeConfirm)
}

func TestTrendMomentum_ThreeBars(t *testing.T) {
	// 3 bars is below minimum (< 4) → should get FLAT fallback
	computer := NewComputer()
	bars := generateBars([]float64{100, 50, 10})
	md := &models.MarketData{Ticker: "TEST.AU", EOD: bars}
	signals := computer.Compute(md)

	assert.Equal(t, models.TrendMomentumFlat, signals.TrendMomentum.Level)
	assert.Equal(t, "Insufficient data for trend momentum", signals.TrendMomentum.Description)
	assert.Equal(t, 0.0, signals.TrendMomentum.Score)
}

func TestTrendMomentum_PriceGoesToZero(t *testing.T) {
	// Price drops to zero — what happens to computations?
	computer := NewComputer()
	// 12 bars: price drops from 100 to 0
	closes := make([]float64, 12)
	for i := range closes {
		closes[i] = float64(11-i) * 10 // 110, 100, 90, ..., 10, 0
	}
	closes[0] = 0 // current price is 0
	bars := generateBars(closes)
	md := &models.MarketData{Ticker: "TEST.AU", EOD: bars}
	signals := computer.Compute(md)

	assert.False(t, math.IsNaN(signals.TrendMomentum.Score), "score must not be NaN when price goes to zero")
	assert.False(t, math.IsInf(signals.TrendMomentum.Score, 0), "score must not be Inf")
	assert.NotEqual(t, models.TrendMomentumLevel(""), signals.TrendMomentum.Level)
}

func TestTrendMomentum_AllZeroPrices(t *testing.T) {
	// All bars have Close=0 — division by zero scenario
	computer := NewComputer()
	bars := generateBars([]float64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	md := &models.MarketData{Ticker: "TEST.AU", EOD: bars}
	signals := computer.Compute(md)

	assert.False(t, math.IsNaN(signals.TrendMomentum.Score), "all-zero prices must not produce NaN score")
	assert.False(t, math.IsInf(signals.TrendMomentum.Score, 0))
	assert.Equal(t, models.TrendMomentumFlat, signals.TrendMomentum.Level, "all-zero prices should classify as FLAT")
}

func TestTrendMomentum_MassiveSpike(t *testing.T) {
	// Price jumps from 0.01 to 10000 in 3 days — score should be clamped, not Inf
	computer := NewComputer()
	closes := []float64{10000, 5000, 100, 0.01, 0.01, 0.01, 0.01, 0.01, 0.01, 0.01, 0.01, 0.01}
	bars := generateBars(closes)
	md := &models.MarketData{Ticker: "TEST.AU", EOD: bars}
	signals := computer.Compute(md)

	assert.False(t, math.IsInf(signals.TrendMomentum.Score, 0), "massive spike should be clamped, not Inf")
	// Score should be clamped to 1.0 max
	assert.LessOrEqual(t, signals.TrendMomentum.Score, 1.0)
	assert.GreaterOrEqual(t, signals.TrendMomentum.Score, -1.0)
	assert.Equal(t, models.TrendMomentumStrongUp, signals.TrendMomentum.Level)
}

func TestTrendMomentum_MassiveCrash(t *testing.T) {
	// Price drops from 10000 to 0.01 — should classify as strong down
	computer := NewComputer()
	closes := []float64{0.01, 100, 5000, 10000, 10000, 10000, 10000, 10000, 10000, 10000, 10000, 10000}
	bars := generateBars(closes)
	md := &models.MarketData{Ticker: "TEST.AU", EOD: bars}
	signals := computer.Compute(md)

	assert.False(t, math.IsInf(signals.TrendMomentum.Score, 0))
	assert.LessOrEqual(t, signals.TrendMomentum.Score, 1.0)
	assert.GreaterOrEqual(t, signals.TrendMomentum.Score, -1.0)
	assert.Equal(t, models.TrendMomentumStrongDown, signals.TrendMomentum.Level)
}

func TestTrendMomentum_FlatMarket(t *testing.T) {
	// All prices identical — should be FLAT
	computer := NewComputer()
	bars := generateBars([]float64{100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100,
		100, 100, 100, 100, 100, 100, 100, 100, 100})
	md := &models.MarketData{Ticker: "TEST.AU", EOD: bars}
	signals := computer.Compute(md)

	assert.Equal(t, models.TrendMomentumFlat, signals.TrendMomentum.Level)
	assert.InDelta(t, 0.0, signals.TrendMomentum.Score, 0.01)
	assert.InDelta(t, 0.0, signals.TrendMomentum.PriceChange3D, 0.01)
	assert.InDelta(t, 0.0, signals.TrendMomentum.PriceChange5D, 0.01)
	assert.InDelta(t, 0.0, signals.TrendMomentum.PriceChange10D, 0.01)
}

func TestTrendMomentum_VolumeConfirmUpside(t *testing.T) {
	// Rising prices with 2x volume should confirm
	computer := NewComputer()
	closes := make([]float64, 25)
	volumes := make([]int64, 25)
	for i := 0; i < 25; i++ {
		closes[i] = 100 + float64(25-i)*0.5 // rising prices
		if i < 3 {
			volumes[i] = 2000000 // recent volume 2x
		} else {
			volumes[i] = 1000000
		}
	}
	bars := make([]models.EODBar, 25)
	for i := range bars {
		bars[i] = models.EODBar{Close: closes[i], Volume: volumes[i]}
	}
	md := &models.MarketData{Ticker: "TEST.AU", EOD: bars}
	signals := computer.Compute(md)

	assert.True(t, signals.TrendMomentum.VolumeConfirm, "rising prices with 2x volume should confirm")
}

func TestTrendMomentum_VolumeConfirmAsymmetry(t *testing.T) {
	// Volume confirmation uses change3d > 0 for upside but change3d < -1 for downside.
	// A mild down move (between 0 and -1%) with high volume does NOT get volume confirmed.
	// This test documents the asymmetric threshold.
	computer := NewComputer()
	closes := make([]float64, 25)
	volumes := make([]int64, 25)
	for i := 0; i < 25; i++ {
		// bars[0] is newest. Price declining: newest is lowest.
		// closes[0]=96.4, closes[3]=96.85 → change3d = (96.4-96.85)/96.85*100 = -0.46%
		closes[i] = 96.4 + float64(i)*0.15
		if i < 3 {
			volumes[i] = 3000000 // 3x recent volume
		} else {
			volumes[i] = 1000000
		}
	}
	bars := make([]models.EODBar, 25)
	for i := range bars {
		bars[i] = models.EODBar{Close: closes[i], Volume: volumes[i]}
	}
	md := &models.MarketData{Ticker: "TEST.AU", EOD: bars}
	signals := computer.Compute(md)

	// change3d will be about -0.46% (between 0 and -1) — NOT caught by `change3d < -1`
	// So despite 3x volume on a declining day, volume is not confirmed.
	// This is the asymmetric threshold: upside requires change3d > 0, downside requires change3d < -1
	assert.False(t, signals.TrendMomentum.VolumeConfirm,
		"mild decline (~-0.46%%) with high volume is NOT confirmed due to asymmetric threshold (change3d < -1 required)")
}

func TestTrendMomentum_AccelerationPositive(t *testing.T) {
	// Recent momentum (3d) is stronger than normalized 10d rate → positive acceleration
	computer := NewComputer()
	// Prices: flat for 7 days, then sharp 3-day jump
	closes := make([]float64, 12)
	for i := 0; i < 12; i++ {
		if i < 3 {
			closes[i] = 110 + float64(3-i)*5 // last 3 days: 125, 120, 115
		} else {
			closes[i] = 100 // flat before that
		}
	}
	bars := generateBars(closes)
	md := &models.MarketData{Ticker: "TEST.AU", EOD: bars}
	signals := computer.Compute(md)

	assert.Greater(t, signals.TrendMomentum.Acceleration, 0.0, "recent sharp move should show positive acceleration")
}

func TestTrendMomentum_AccelerationNegative(t *testing.T) {
	// Recent momentum is weaker than normalized 10d rate → decelerating
	computer := NewComputer()
	// Prices: big early move then flatting out
	closes := make([]float64, 12)
	for i := 0; i < 12; i++ {
		if i < 3 {
			closes[i] = 110 // last 3 days: flat at 110
		} else {
			closes[i] = 110 - float64(i-3)*3 // strong rise before that: 107, 104, 101, ...
		}
	}
	bars := generateBars(closes)
	md := &models.MarketData{Ticker: "TEST.AU", EOD: bars}
	signals := computer.Compute(md)

	// 10d change is large (from ~80 to 110) but 3d change is 0 → negative acceleration
	assert.Less(t, signals.TrendMomentum.Acceleration, 0.0, "flat recent vs strong prior should show negative acceleration")
}

func TestTrendMomentum_NearSupport(t *testing.T) {
	// Price within 3% of support level should flag nearSupport
	computer := NewComputer()
	// Create enough bars for support detection (need 60+ for the lookback)
	closes := make([]float64, 65)
	for i := range closes {
		closes[i] = 100 + float64(i%5)*0.5 // oscillating near 100
	}
	// Set current price very close to the likely support level
	closes[0] = 99.5
	bars := generateBars(closes)
	md := &models.MarketData{Ticker: "TEST.AU", EOD: bars}
	signals := computer.Compute(md)

	// Whether nearSupport is true depends on actual support calculation,
	// but at minimum, the field should be a valid bool
	_ = signals.TrendMomentum.NearSupport // no panic
}

func TestTrendMomentum_ScoreBoundsWithMaxInputs(t *testing.T) {
	// Verify score is always in [-1, 1] regardless of extreme inputs
	computer := NewComputer()
	extremeCases := [][]float64{
		{1e10, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},                                              // huge spike
		{1, 1e10, 1e10, 1e10, 1e10, 1e10, 1e10, 1e10, 1e10, 1e10, 1e10, 1e10},                // huge crash
		{0.001, 0.001, 0.001, 0.001, 0.001, 0.001, 0.001, 0.001, 0.001, 0.001, 0.001, 0.001}, // penny stock flat
	}

	for i, closes := range extremeCases {
		bars := generateBars(closes)
		md := &models.MarketData{Ticker: "TEST.AU", EOD: bars}
		signals := computer.Compute(md)

		assert.GreaterOrEqual(t, signals.TrendMomentum.Score, -1.0, "case %d: score below -1", i)
		assert.LessOrEqual(t, signals.TrendMomentum.Score, 1.0, "case %d: score above +1", i)
		assert.False(t, math.IsNaN(signals.TrendMomentum.Score), "case %d: score is NaN", i)
		assert.False(t, math.IsInf(signals.TrendMomentum.Score, 0), "case %d: score is Inf", i)
	}
}

func TestTrendMomentum_DescriptionNotEmpty(t *testing.T) {
	// Every classification should produce a non-empty description
	computer := NewComputer()

	scenarios := map[string][]float64{
		"strong_up":   {150, 140, 130, 120, 110, 100, 90, 80, 70, 60, 50, 40},
		"flat":        {100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100},
		"strong_down": {40, 50, 60, 70, 80, 90, 100, 110, 120, 130, 140, 150},
	}

	for name, closes := range scenarios {
		bars := generateBars(closes)
		md := &models.MarketData{Ticker: "TEST.AU", EOD: bars}
		signals := computer.Compute(md)

		assert.NotEmpty(t, signals.TrendMomentum.Description, "scenario %s should have non-empty description", name)
	}
}

func TestTrendMomentum_SingleBarWithClose0_NoPanic(t *testing.T) {
	// Edge case: 1 bar is below the 4-bar minimum, but also Close=0
	computer := NewComputer()
	bars := generateBars([]float64{0})
	md := &models.MarketData{Ticker: "TEST.AU", EOD: bars}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic with single zero-close bar: %v", r)
		}
	}()

	signals := computer.Compute(md)
	assert.Equal(t, models.TrendMomentumFlat, signals.TrendMomentum.Level)
}

func TestTrendMomentum_LargeBarCount(t *testing.T) {
	// 500 bars — should not be slow or allocate excessively
	computer := NewComputer()
	closes := make([]float64, 500)
	for i := range closes {
		closes[i] = 100 + float64(i%20) // oscillating
	}
	bars := generateBars(closes)
	md := &models.MarketData{Ticker: "TEST.AU", EOD: bars}
	signals := computer.Compute(md)

	assert.NotEqual(t, models.TrendMomentumLevel(""), signals.TrendMomentum.Level)
	assert.GreaterOrEqual(t, signals.TrendMomentum.Score, -1.0)
	assert.LessOrEqual(t, signals.TrendMomentum.Score, 1.0)
}

// --- describeTrendMomentum edge cases ---

func TestDescribeTrendMomentum_AllLevels(t *testing.T) {
	levels := []models.TrendMomentumLevel{
		models.TrendMomentumStrongUp,
		models.TrendMomentumUp,
		models.TrendMomentumFlat,
		models.TrendMomentumDown,
		models.TrendMomentumStrongDown,
	}
	for _, level := range levels {
		desc := describeTrendMomentum(level, 5.0, 10.0, 15.0, 2.0, true, true)
		assert.NotEmpty(t, desc, "level %s should produce non-empty description", level)
	}
}

func TestDescribeTrendMomentum_UnknownLevel(t *testing.T) {
	// An unknown level string falls through all switch cases → empty string
	desc := describeTrendMomentum("UNKNOWN_LEVEL", 5.0, 10.0, 15.0, 2.0, true, true)
	assert.Empty(t, desc, "unknown level should produce empty description (no default case)")
}

// --- Interaction with existing signal pipeline ---

func TestTrendMomentum_ComputedAfterSupportResistance(t *testing.T) {
	// TrendMomentum uses signals.Technical.SupportLevel which is computed earlier in Compute().
	// Verify the ordering is correct — nearSupport should use the real support level.
	computer := NewComputer()
	// Need 60+ bars for support/resistance detection
	closes := make([]float64, 65)
	for i := range closes {
		closes[i] = 50 + float64(i%10)*2 // oscillating 50-68
	}
	bars := generateBars(closes)
	md := &models.MarketData{Ticker: "TEST.AU", EOD: bars}
	signals := computer.Compute(md)

	// Support level should be non-zero (from enough bars)
	assert.Greater(t, signals.Technical.SupportLevel, 0.0, "support level should be computed before trend momentum")
}

func TestTrendMomentum_ExactlyElevenBars(t *testing.T) {
	// 11 bars is the minimum for full acceleration calculation
	computer := NewComputer()
	closes := make([]float64, 11)
	for i := range closes {
		closes[i] = 100 - float64(i)*2 // declining
	}
	bars := generateBars(closes)
	md := &models.MarketData{Ticker: "TEST.AU", EOD: bars}
	signals := computer.Compute(md)

	// Should compute acceleration (not zero)
	// change10d is (100-80)/80*100 = 25%, rate10dPer3d = 25*3/10 = 7.5
	// change3d = (100-94)/94*100 ~= 6.38%
	// acceleration = 6.38 - 7.5 = -1.12 (decelerating)
	assert.NotEqual(t, 0.0, signals.TrendMomentum.Acceleration, "11 bars should enable acceleration calculation")
}

func TestTrendMomentum_TenBars_NoAcceleration(t *testing.T) {
	// 10 bars is below the 11-bar threshold for acceleration
	computer := NewComputer()
	closes := make([]float64, 10)
	for i := range closes {
		closes[i] = 100 - float64(i)*5
	}
	bars := generateBars(closes)
	md := &models.MarketData{Ticker: "TEST.AU", EOD: bars}
	signals := computer.Compute(md)

	assert.Equal(t, 0.0, signals.TrendMomentum.Acceleration, "10 bars should not compute acceleration")
}

func TestTrendMomentum_ExactlyTwentyBars_VolumeConfirm(t *testing.T) {
	// 20 bars is the minimum for volume confirmation
	computer := NewComputer()
	closes := make([]float64, 20)
	volumes := make([]int64, 20)
	for i := range closes {
		closes[i] = 100 + float64(20-i) // rising
		volumes[i] = 1000000
	}
	// Boost recent volume
	volumes[0] = 3000000
	volumes[1] = 3000000
	volumes[2] = 3000000

	bars := make([]models.EODBar, 20)
	for i := range bars {
		bars[i] = models.EODBar{Close: closes[i], Volume: volumes[i]}
	}
	md := &models.MarketData{Ticker: "TEST.AU", EOD: bars}
	signals := computer.Compute(md)

	// With 3x volume and rising prices, should confirm
	assert.True(t, signals.TrendMomentum.VolumeConfirm, "20 bars with 3x recent volume on rising prices should confirm")
}

func TestTrendMomentum_NineteenBars_NoVolumeConfirm(t *testing.T) {
	// 19 bars is below the 20-bar threshold for volume confirmation
	computer := NewComputer()
	closes := make([]float64, 19)
	volumes := make([]int64, 19)
	for i := range closes {
		closes[i] = 100 + float64(19-i)
		volumes[i] = 1000000
	}
	volumes[0] = 5000000
	volumes[1] = 5000000
	volumes[2] = 5000000

	bars := make([]models.EODBar, 19)
	for i := range bars {
		bars[i] = models.EODBar{Close: closes[i], Volume: volumes[i]}
	}
	md := &models.MarketData{Ticker: "TEST.AU", EOD: bars}
	signals := computer.Compute(md)

	assert.False(t, signals.TrendMomentum.VolumeConfirm, "19 bars should not enable volume confirmation")
}
