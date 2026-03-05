package portfolio

import (
	"math"
	"testing"

	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- NaN / Inf propagation ---

func TestComputeBreadth_NaNTrendScore(t *testing.T) {
	svc := &Service{}
	p := &models.Portfolio{
		Holdings: []models.Holding{
			{Status: "open", MarketValue: 1000, TrendLabel: "Uptrend", TrendScore: math.NaN(), CurrentPrice: 10, Units: 100},
			{Status: "open", MarketValue: 1000, TrendLabel: "Downtrend", TrendScore: -0.3, CurrentPrice: 5, Units: 200},
		},
	}

	svc.computeBreadth(p)

	require.NotNil(t, p.Breadth)
	// NaN propagates through weightedScore — final TrendScore becomes NaN
	assert.True(t, math.IsNaN(p.Breadth.TrendScore), "NaN TrendScore on a holding propagates to portfolio TrendScore — consider guarding against NaN inputs")
}

func TestComputeBreadth_InfTrendScore(t *testing.T) {
	svc := &Service{}
	p := &models.Portfolio{
		Holdings: []models.Holding{
			{Status: "open", MarketValue: 1000, TrendLabel: "Uptrend", TrendScore: math.Inf(1), CurrentPrice: 10, Units: 100},
			{Status: "open", MarketValue: 1000, TrendLabel: "Downtrend", TrendScore: -0.3, CurrentPrice: 5, Units: 200},
		},
	}

	svc.computeBreadth(p)

	require.NotNil(t, p.Breadth)
	// Inf propagates through weightedScore — final TrendScore becomes Inf
	assert.True(t, math.IsInf(p.Breadth.TrendScore, 1), "Inf TrendScore on a holding propagates to portfolio TrendScore — consider guarding against Inf inputs")
}

// --- Single holding ---

func TestComputeBreadth_SingleHolding(t *testing.T) {
	svc := &Service{}
	p := &models.Portfolio{
		Holdings: []models.Holding{
			{Status: "open", MarketValue: 5000, TrendLabel: "Uptrend", TrendScore: 0.5, CurrentPrice: 50, YesterdayClosePrice: 48, Units: 100},
		},
	}

	svc.computeBreadth(p)

	require.NotNil(t, p.Breadth)
	assert.Equal(t, 1, p.Breadth.RisingCount)
	assert.Equal(t, 0, p.Breadth.FlatCount)
	assert.Equal(t, 0, p.Breadth.FallingCount)
	assert.InDelta(t, 1.0, p.Breadth.RisingWeight, 0.001)
	assert.InDelta(t, 0.0, p.Breadth.FlatWeight, 0.001)
	assert.InDelta(t, 0.0, p.Breadth.FallingWeight, 0.001)
	assert.InDelta(t, 0.5, p.Breadth.TrendScore, 0.001)
	assert.Equal(t, "Strong Uptrend", p.Breadth.TrendLabel)
	// TodayChange: (50-48)*100 = 200
	assert.InDelta(t, 200.0, p.Breadth.TodayChange, 0.001)
	// TodayChangePct: 200/5000*100 = 4.0
	assert.InDelta(t, 4.0, p.Breadth.TodayChangePct, 0.001)
}

// --- All same direction (degenerate weights) ---

func TestComputeBreadth_AllFalling(t *testing.T) {
	svc := &Service{}
	p := &models.Portfolio{
		Holdings: []models.Holding{
			{Status: "open", MarketValue: 1000, TrendLabel: "Downtrend", TrendScore: -0.3},
			{Status: "open", MarketValue: 2000, TrendLabel: "Strong Downtrend", TrendScore: -0.8},
			{Status: "open", MarketValue: 500, TrendLabel: "Downtrend", TrendScore: -0.2},
		},
	}

	svc.computeBreadth(p)

	require.NotNil(t, p.Breadth)
	assert.Equal(t, 0, p.Breadth.RisingCount)
	assert.Equal(t, 0, p.Breadth.FlatCount)
	assert.Equal(t, 3, p.Breadth.FallingCount)
	assert.InDelta(t, 0.0, p.Breadth.RisingWeight, 0.001)
	assert.InDelta(t, 0.0, p.Breadth.FlatWeight, 0.001)
	assert.InDelta(t, 1.0, p.Breadth.FallingWeight, 0.001)
	// Weighted score: (-0.3*1000 + -0.8*2000 + -0.2*500) / (1000+2000+500) = -2000/3500 = -0.5714
	assert.InDelta(t, -0.5714, p.Breadth.TrendScore, 0.001)
	assert.Equal(t, "Strong Downtrend", p.Breadth.TrendLabel)
}

// --- Large portfolio (100 holdings) — float precision ---

func TestComputeBreadth_LargePortfolio(t *testing.T) {
	svc := &Service{}
	holdings := make([]models.Holding, 100)
	for i := 0; i < 100; i++ {
		label := "Uptrend"
		score := 0.3
		if i%3 == 0 {
			label = "Downtrend"
			score = -0.3
		} else if i%3 == 1 {
			label = "Consolidating"
			score = 0.0 // will be excluded from weighted average
		}
		holdings[i] = models.Holding{
			Status:              "open",
			MarketValue:         1000.0,
			TrendLabel:          label,
			TrendScore:          score,
			CurrentPrice:        10.0,
			YesterdayClosePrice: 9.9,
			Units:               100,
		}
	}
	p := &models.Portfolio{Holdings: holdings}

	svc.computeBreadth(p)

	require.NotNil(t, p.Breadth)
	// 100 holdings: i%3==0 => 34 falling, i%3==1 => 33 flat, i%3==2 => 33 rising
	assert.Equal(t, 33, p.Breadth.RisingCount)
	assert.Equal(t, 33, p.Breadth.FlatCount)
	assert.Equal(t, 34, p.Breadth.FallingCount)

	// Weights should sum to 1.0
	weightSum := p.Breadth.RisingWeight + p.Breadth.FlatWeight + p.Breadth.FallingWeight
	assert.InDelta(t, 1.0, weightSum, 0.0001, "weights must sum to 1.0")

	// Values should sum to total market value
	valSum := p.Breadth.RisingValue + p.Breadth.FlatValue + p.Breadth.FallingValue
	assert.InDelta(t, 100000.0, valSum, 0.01)
}

// --- Zero MarketValue holdings are skipped ---

func TestComputeBreadth_ZeroMarketValueSkipped(t *testing.T) {
	svc := &Service{}
	p := &models.Portfolio{
		Holdings: []models.Holding{
			{Status: "open", MarketValue: 0, TrendLabel: "Uptrend", TrendScore: 0.5},
			{Status: "open", MarketValue: 0, TrendLabel: "Downtrend", TrendScore: -0.5},
			{Status: "open", MarketValue: 1000, TrendLabel: "Uptrend", TrendScore: 0.3, CurrentPrice: 10, YesterdayClosePrice: 9.5, Units: 100},
		},
	}

	svc.computeBreadth(p)

	require.NotNil(t, p.Breadth)
	// Only the third holding should be counted
	assert.Equal(t, 1, p.Breadth.RisingCount)
	assert.Equal(t, 0, p.Breadth.FlatCount)
	assert.Equal(t, 0, p.Breadth.FallingCount)
}

// --- All holdings have MarketValue == 0 → breadth is nil ---

func TestComputeBreadth_AllZeroMarketValue(t *testing.T) {
	svc := &Service{}
	p := &models.Portfolio{
		Holdings: []models.Holding{
			{Status: "open", MarketValue: 0, TrendLabel: "Uptrend"},
			{Status: "open", MarketValue: 0, TrendLabel: "Downtrend"},
		},
	}

	svc.computeBreadth(p)

	assert.Nil(t, p.Breadth, "all zero market value holdings should produce nil breadth")
}

// --- Negative MarketValue (data anomaly) ---

func TestComputeBreadth_NegativeMarketValue(t *testing.T) {
	svc := &Service{}
	p := &models.Portfolio{
		Holdings: []models.Holding{
			{Status: "open", MarketValue: -500, TrendLabel: "Uptrend", TrendScore: 0.5},
			{Status: "open", MarketValue: 2000, TrendLabel: "Downtrend", TrendScore: -0.3},
		},
	}

	svc.computeBreadth(p)

	require.NotNil(t, p.Breadth)
	total := -500.0 + 2000.0 // 1500
	// Negative MV produces a negative weight — potentially surprising
	assert.InDelta(t, -500.0/total, p.Breadth.RisingWeight, 0.001, "negative market value produces negative weight — consider abs() or skipping")
	assert.InDelta(t, 2000.0/total, p.Breadth.FallingWeight, 0.001)
	// Weights still sum to 1.0 mathematically
	weightSum := p.Breadth.RisingWeight + p.Breadth.FlatWeight + p.Breadth.FallingWeight
	assert.InDelta(t, 1.0, weightSum, 0.001)
}

// --- TodayChange edge cases ---

func TestComputeBreadth_TodayChange_NoYesterdayPrice(t *testing.T) {
	svc := &Service{}
	p := &models.Portfolio{
		Holdings: []models.Holding{
			{Status: "open", MarketValue: 1000, TrendLabel: "Uptrend", CurrentPrice: 10, YesterdayClosePrice: 0, Units: 100},
			{Status: "open", MarketValue: 2000, TrendLabel: "Uptrend", CurrentPrice: 20, Units: 100}, // YesterdayClosePrice defaults to 0
		},
	}

	svc.computeBreadth(p)

	require.NotNil(t, p.Breadth)
	// No yesterday prices means no today change computed
	assert.InDelta(t, 0.0, p.Breadth.TodayChange, 0.001)
	assert.InDelta(t, 0.0, p.Breadth.TodayChangePct, 0.001)
}

func TestComputeBreadth_TodayChangePct_SmallTotal(t *testing.T) {
	svc := &Service{}
	// Very small market value but large price swing — TodayChangePct can be huge
	p := &models.Portfolio{
		Holdings: []models.Holding{
			{Status: "open", MarketValue: 0.01, TrendLabel: "Uptrend", CurrentPrice: 100, YesterdayClosePrice: 1, Units: 1000},
		},
	}

	svc.computeBreadth(p)

	require.NotNil(t, p.Breadth)
	// TodayChange = (100-1)*1000 = 99000
	assert.InDelta(t, 99000.0, p.Breadth.TodayChange, 0.01)
	// TodayChangePct = 99000/0.01*100 = 990,000,000 — astronomically large
	// This is "correct" but could surprise consumers expecting bounded percentages
	assert.True(t, p.Breadth.TodayChangePct > 1000000, "tiny market value with large price change produces extreme TodayChangePct — consider clamping")
}

// --- TrendScore == 0 exclusion verification ---

func TestComputeBreadth_TrendScoreZeroExcluded(t *testing.T) {
	svc := &Service{}
	// Two holdings: one with score 0.5, one with score 0.0 (excluded from weighted avg)
	p := &models.Portfolio{
		Holdings: []models.Holding{
			{Status: "open", MarketValue: 1000, TrendLabel: "Uptrend", TrendScore: 0.5},
			{Status: "open", MarketValue: 9000, TrendLabel: "Consolidating", TrendScore: 0.0},
		},
	}

	svc.computeBreadth(p)

	require.NotNil(t, p.Breadth)
	// Because TrendScore==0 is excluded, only the first holding contributes
	// Weighted score = (0.5*1000) / 1000 = 0.5, NOT (0.5*1000+0*9000)/10000 = 0.05
	assert.InDelta(t, 0.5, p.Breadth.TrendScore, 0.001, "holdings with TrendScore==0 are excluded from weighted average — score is biased toward non-zero holdings")
	assert.Equal(t, "Strong Uptrend", p.Breadth.TrendLabel)
}

// --- Empty holdings slice ---

func TestComputeBreadth_EmptyHoldings(t *testing.T) {
	svc := &Service{}
	p := &models.Portfolio{Holdings: []models.Holding{}}

	svc.computeBreadth(p)

	assert.Nil(t, p.Breadth, "empty holdings should produce nil breadth")
}

// --- Mixed open/closed holdings ---

func TestComputeBreadth_ClosedHoldingsIgnored(t *testing.T) {
	svc := &Service{}
	p := &models.Portfolio{
		Holdings: []models.Holding{
			{Status: "closed", MarketValue: 5000, TrendLabel: "Strong Uptrend", TrendScore: 0.9},
			{Status: "open", MarketValue: 1000, TrendLabel: "Downtrend", TrendScore: -0.3},
		},
	}

	svc.computeBreadth(p)

	require.NotNil(t, p.Breadth)
	assert.Equal(t, 0, p.Breadth.RisingCount, "closed holdings should not count as rising")
	assert.Equal(t, 1, p.Breadth.FallingCount)
	assert.InDelta(t, 1.0, p.Breadth.FallingWeight, 0.001)
}

// --- breadthTrendLabel edge: exact boundaries ---

func TestBreadthTrendLabel_ExactBoundaries(t *testing.T) {
	tests := []struct {
		name  string
		score float64
		want  string
	}{
		{"exactly_0.4", 0.4, "Strong Uptrend"},
		{"just_below_0.4", 0.3999999, "Uptrend"},
		{"exactly_0.15", 0.15, "Uptrend"},
		{"just_below_0.15", 0.1499999, "Mixed"},
		{"exactly_-0.15", -0.15, "Downtrend"}, // boundary: -0.15 > -0.15 is false, so NOT Mixed — falls to Downtrend (asymmetric with positive boundary)
		{"just_above_-0.15", -0.1499999, "Mixed"},
		{"just_below_-0.15", -0.1500001, "Downtrend"},
		{"exactly_-0.4", -0.4, "Strong Downtrend"}, // boundary: -0.4 > -0.4 is false, so NOT Downtrend — falls to Strong Downtrend
		{"just_above_-0.4", -0.3999999, "Downtrend"},
		{"just_below_-0.4", -0.4000001, "Strong Downtrend"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := breadthTrendLabel(tt.score)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- breadthTrendLabel with NaN/Inf ---

func TestBreadthTrendLabel_NaN(t *testing.T) {
	// NaN comparisons are all false, so all switch cases fail, hitting default
	got := breadthTrendLabel(math.NaN())
	assert.Equal(t, "Strong Downtrend", got, "NaN falls through to default — may want explicit handling")
}

func TestBreadthTrendLabel_PosInf(t *testing.T) {
	got := breadthTrendLabel(math.Inf(1))
	assert.Equal(t, "Strong Uptrend", got, "+Inf >= 0.4 is true")
}

func TestBreadthTrendLabel_NegInf(t *testing.T) {
	got := breadthTrendLabel(math.Inf(-1))
	assert.Equal(t, "Strong Downtrend", got, "-Inf falls to default")
}
