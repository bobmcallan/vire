package signals

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bobmcallan/vire/internal/models"
)

// =============================================================================
// computeTrendMomentum — unit tests
// =============================================================================

func TestComputeTrendMomentum_InsufficientData_Zero(t *testing.T) {
	// With 0 EOD bars, Compute() returns early before computeTrendMomentum is called.
	// TrendMomentum will be the zero value (empty level, empty description).
	computer := NewComputer()
	md := &models.MarketData{Ticker: "TEST.AU", EOD: []models.EODBar{}}
	signals := computer.Compute(md)
	// No panic is the key assertion; level is empty (zero value)
	_ = signals.TrendMomentum.Level
	_ = signals.TrendMomentum.Description
}

func TestComputeTrendMomentum_InsufficientData_ThreeBars(t *testing.T) {
	computer := NewComputer()
	bars := generateBars([]float64{100, 90, 80})
	md := &models.MarketData{Ticker: "TEST.AU", EOD: bars}
	signals := computer.Compute(md)
	assert.Equal(t, models.TrendMomentumFlat, signals.TrendMomentum.Level)
	assert.Equal(t, "Insufficient data for trend momentum", signals.TrendMomentum.Description)
}

func TestComputeTrendMomentum_FourBarsMinimum(t *testing.T) {
	// 4 bars is the minimum to compute the 3-day change
	computer := NewComputer()
	bars := generateBars([]float64{110, 105, 100, 95})
	md := &models.MarketData{Ticker: "TEST.AU", EOD: bars}
	signals := computer.Compute(md)

	assert.NotEqual(t, models.TrendMomentumFlat, signals.TrendMomentum.Level, "4 bars should produce a non-flat signal with rising prices")
	// PriceChange3D = (110 - 95) / 95 * 100 = 15.78%
	assert.InDelta(t, 15.78, signals.TrendMomentum.PriceChange3D, 0.5)
	// No acceleration (need 11 bars)
	assert.Equal(t, 0.0, signals.TrendMomentum.Acceleration)
	// No volume confirm (need 20 bars)
	assert.False(t, signals.TrendMomentum.VolumeConfirm)
}

func TestComputeTrendMomentum_StrongUp_ScoreRange(t *testing.T) {
	computer := NewComputer()
	// Strong rising prices
	bars := generateBars([]float64{150, 140, 130, 120, 110, 100, 90, 80, 70, 60, 50, 45})
	md := &models.MarketData{Ticker: "TEST.AU", EOD: bars}
	signals := computer.Compute(md)

	assert.Equal(t, models.TrendMomentumStrongUp, signals.TrendMomentum.Level)
	assert.GreaterOrEqual(t, signals.TrendMomentum.Score, 0.5)
	assert.LessOrEqual(t, signals.TrendMomentum.Score, 1.0)
	assert.Greater(t, signals.TrendMomentum.PriceChange3D, 0.0)
	assert.Greater(t, signals.TrendMomentum.PriceChange10D, 0.0)
}

func TestComputeTrendMomentum_StrongDown_ScoreRange(t *testing.T) {
	computer := NewComputer()
	// Strongly falling prices
	bars := generateBars([]float64{50, 60, 70, 80, 90, 100, 110, 120, 130, 140, 150, 155})
	md := &models.MarketData{Ticker: "TEST.AU", EOD: bars}
	signals := computer.Compute(md)

	assert.Equal(t, models.TrendMomentumStrongDown, signals.TrendMomentum.Level)
	assert.LessOrEqual(t, signals.TrendMomentum.Score, -0.5)
	assert.GreaterOrEqual(t, signals.TrendMomentum.Score, -1.0)
	assert.Less(t, signals.TrendMomentum.PriceChange3D, 0.0)
}

func TestComputeTrendMomentum_Flat_IdenticalPrices(t *testing.T) {
	computer := NewComputer()
	bars := generateBars([]float64{100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100})
	md := &models.MarketData{Ticker: "TEST.AU", EOD: bars}
	signals := computer.Compute(md)

	assert.Equal(t, models.TrendMomentumFlat, signals.TrendMomentum.Level)
	assert.InDelta(t, 0.0, signals.TrendMomentum.Score, 0.001)
	assert.InDelta(t, 0.0, signals.TrendMomentum.PriceChange3D, 0.001)
	assert.InDelta(t, 0.0, signals.TrendMomentum.PriceChange5D, 0.001)
	assert.InDelta(t, 0.0, signals.TrendMomentum.PriceChange10D, 0.001)
}

func TestComputeTrendMomentum_PriceChangesComputed(t *testing.T) {
	computer := NewComputer()
	// 12 bars: prices at 100, 97, 95, 93, 91, 89, 87, 85, 83, 81, 79 …
	// bars[0]=100 (newest), bars[3]=93, bars[5]=89, bars[10]=79
	closes := []float64{100, 97, 95, 93, 91, 89, 87, 85, 83, 81, 79, 77}
	bars := generateBars(closes)
	md := &models.MarketData{Ticker: "TEST.AU", EOD: bars}
	signals := computer.Compute(md)

	// 3d: (100 - closes[3]) / closes[3] * 100 = (100-93)/93*100 = 7.53%
	assert.InDelta(t, 7.53, signals.TrendMomentum.PriceChange3D, 0.5)
	// 5d: (100 - closes[5]) / closes[5] * 100 = (100-89)/89*100 = 12.36%
	assert.InDelta(t, 12.36, signals.TrendMomentum.PriceChange5D, 0.5)
	// 10d: (100 - closes[10]) / closes[10] * 100 = (100-79)/79*100 = 26.58%
	assert.InDelta(t, 26.58, signals.TrendMomentum.PriceChange10D, 0.5)
}

func TestComputeTrendMomentum_ScoreInBounds(t *testing.T) {
	computer := NewComputer()
	// Various price patterns
	testCases := [][]float64{
		{120, 110, 100, 90, 80, 70, 60, 50, 40, 30, 20, 10},          // big rise
		{10, 20, 30, 40, 50, 60, 70, 80, 90, 100, 110, 120},          // big fall
		{100, 101, 100, 101, 100, 101, 100, 101, 100, 101, 100, 101}, // oscillating
	}
	for _, closes := range testCases {
		bars := generateBars(closes)
		md := &models.MarketData{Ticker: "TEST.AU", EOD: bars}
		signals := computer.Compute(md)
		assert.GreaterOrEqual(t, signals.TrendMomentum.Score, -1.0, "score must be >= -1.0")
		assert.LessOrEqual(t, signals.TrendMomentum.Score, 1.0, "score must be <= 1.0")
	}
}

func TestComputeTrendMomentum_AccelerationZeroUnder11Bars(t *testing.T) {
	computer := NewComputer()
	// Only 10 bars — no acceleration
	closes := make([]float64, 10)
	for i := range closes {
		closes[i] = 100 - float64(i)*2
	}
	bars := generateBars(closes)
	md := &models.MarketData{Ticker: "TEST.AU", EOD: bars}
	signals := computer.Compute(md)

	assert.Equal(t, 0.0, signals.TrendMomentum.Acceleration, "10 bars: no acceleration")
}

func TestComputeTrendMomentum_AccelerationComputedAt11Bars(t *testing.T) {
	computer := NewComputer()
	// 11 bars = minimum for acceleration
	closes := make([]float64, 11)
	for i := range closes {
		closes[i] = 100 - float64(i)*3
	}
	bars := generateBars(closes)
	md := &models.MarketData{Ticker: "TEST.AU", EOD: bars}
	signals := computer.Compute(md)

	assert.NotEqual(t, 0.0, signals.TrendMomentum.Acceleration, "11 bars: acceleration should be computed")
}

func TestComputeTrendMomentum_VolumeConfirmFalseUnder20Bars(t *testing.T) {
	computer := NewComputer()
	// 19 bars with high recent volume — no confirm
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

	assert.False(t, signals.TrendMomentum.VolumeConfirm, "under 20 bars: no volume confirm")
}

func TestComputeTrendMomentum_VolumeConfirmTrueAt20Bars(t *testing.T) {
	computer := NewComputer()
	closes := make([]float64, 20)
	volumes := make([]int64, 20)
	for i := range closes {
		closes[i] = 100 + float64(20-i) // rising
		volumes[i] = 1000000
	}
	// Boost recent 3-day volume to 3x
	volumes[0] = 3000000
	volumes[1] = 3000000
	volumes[2] = 3000000
	bars := make([]models.EODBar, 20)
	for i := range bars {
		bars[i] = models.EODBar{Close: closes[i], Volume: volumes[i]}
	}
	md := &models.MarketData{Ticker: "TEST.AU", EOD: bars}
	signals := computer.Compute(md)

	assert.True(t, signals.TrendMomentum.VolumeConfirm, "20 bars with 3x recent volume on rising prices should confirm")
}

func TestComputeTrendMomentum_DescriptionNonEmpty(t *testing.T) {
	// Every valid outcome should have a non-empty description
	computer := NewComputer()
	testCases := map[string][]float64{
		"strong_up":   {150, 140, 130, 120, 110, 100, 90, 80, 70, 60, 50, 40},
		"flat":        {100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100},
		"strong_down": {50, 60, 70, 80, 90, 100, 110, 120, 130, 140, 150, 155},
	}
	for name, closes := range testCases {
		bars := generateBars(closes)
		md := &models.MarketData{Ticker: name, EOD: bars}
		signals := computer.Compute(md)
		assert.NotEmpty(t, signals.TrendMomentum.Description, "%s: description must be non-empty", name)
	}
}

func TestComputeTrendMomentum_LevelEnum_ValidValues(t *testing.T) {
	validLevels := map[models.TrendMomentumLevel]bool{
		models.TrendMomentumStrongUp:   true,
		models.TrendMomentumUp:         true,
		models.TrendMomentumFlat:       true,
		models.TrendMomentumDown:       true,
		models.TrendMomentumStrongDown: true,
	}
	computer := NewComputer()
	testCases := [][]float64{
		{150, 140, 130, 120, 110, 100, 90, 80, 70, 60, 50, 40},
		{120, 118, 116, 114, 112, 110, 108, 106, 104, 102, 100, 98},
		{100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100},
		{88, 90, 92, 94, 96, 98, 100, 102, 104, 106, 108, 110},
		{50, 60, 70, 80, 90, 100, 110, 120, 130, 140, 150, 155},
	}
	for _, closes := range testCases {
		bars := generateBars(closes)
		md := &models.MarketData{Ticker: "TEST.AU", EOD: bars}
		signals := computer.Compute(md)
		assert.True(t, validLevels[signals.TrendMomentum.Level],
			"level %q is not a valid TrendMomentumLevel", signals.TrendMomentum.Level)
	}
}

// =============================================================================
// Compute — integration: TrendMomentum is set on TickerSignals
// =============================================================================

func TestCompute_TrendMomentum_FieldPopulated(t *testing.T) {
	computer := NewComputer()
	bars := generateBars([]float64{110, 108, 106, 104, 102, 100, 98, 96, 94, 92, 90, 88})
	md := &models.MarketData{Ticker: "BHP.AU", EOD: bars}
	signals := computer.Compute(md)

	assert.NotEmpty(t, signals.TrendMomentum.Level, "TrendMomentum.Level must be populated by Compute")
	assert.NotEmpty(t, signals.TrendMomentum.Description, "TrendMomentum.Description must be populated by Compute")
}

func TestCompute_TrendMomentum_ComputedAfterSupport(t *testing.T) {
	// TrendMomentum uses signals.Technical.SupportLevel which is computed before it.
	// With enough bars, nearSupport is meaningful.
	computer := NewComputer()
	// 65 bars oscillating around 100 — enough for support detection
	closes := make([]float64, 65)
	for i := range closes {
		closes[i] = 100 + float64(i%5)*1.0
	}
	// Current price near 100 (likely the support level)
	closes[0] = 100.5
	bars := generateBars(closes)
	md := &models.MarketData{Ticker: "TEST.AU", EOD: bars}
	signals := computer.Compute(md)

	// At minimum, SupportLevel should be non-zero with 65 bars
	assert.Greater(t, signals.Technical.SupportLevel, 0.0, "support level should be computed before trend momentum")
	// TrendMomentum.NearSupport is a bool — just verify it doesn't panic
	_ = signals.TrendMomentum.NearSupport
}
