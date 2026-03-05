package portfolio

import (
	"testing"

	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestComputeBreadth_AllRising(t *testing.T) {
	svc := &Service{}
	p := &models.Portfolio{
		Holdings: []models.Holding{
			{Status: "open", MarketValue: 1000, TrendLabel: "Strong Uptrend", TrendScore: 0.8, CurrentPrice: 10, YesterdayClosePrice: 9.5, Units: 100},
			{Status: "open", MarketValue: 2000, TrendLabel: "Uptrend", TrendScore: 0.3, CurrentPrice: 20, YesterdayClosePrice: 19, Units: 100},
		},
	}

	svc.computeBreadth(p)

	assert.NotNil(t, p.Breadth)
	assert.Equal(t, 2, p.Breadth.RisingCount)
	assert.Equal(t, 0, p.Breadth.FlatCount)
	assert.Equal(t, 0, p.Breadth.FallingCount)
	assert.InDelta(t, 1.0, p.Breadth.RisingWeight, 0.001)
	assert.InDelta(t, 0.0, p.Breadth.FlatWeight, 0.001)
	assert.InDelta(t, 0.0, p.Breadth.FallingWeight, 0.001)
}

func TestComputeBreadth_MixedTrends(t *testing.T) {
	svc := &Service{}
	p := &models.Portfolio{
		Holdings: []models.Holding{
			{Status: "open", MarketValue: 1000, TrendLabel: "Strong Uptrend", TrendScore: 0.8, CurrentPrice: 10, YesterdayClosePrice: 9, Units: 100},
			{Status: "open", MarketValue: 1000, TrendLabel: "Consolidating", TrendScore: 0.0, CurrentPrice: 20, YesterdayClosePrice: 20, Units: 50},
			{Status: "open", MarketValue: 1000, TrendLabel: "Downtrend", TrendScore: -0.3, CurrentPrice: 5, YesterdayClosePrice: 5.5, Units: 200},
		},
	}

	svc.computeBreadth(p)

	assert.NotNil(t, p.Breadth)
	assert.Equal(t, 1, p.Breadth.RisingCount)
	assert.Equal(t, 1, p.Breadth.FlatCount)
	assert.Equal(t, 1, p.Breadth.FallingCount)
	assert.InDelta(t, 1.0/3, p.Breadth.RisingWeight, 0.001)
	assert.InDelta(t, 1.0/3, p.Breadth.FlatWeight, 0.001)
	assert.InDelta(t, 1.0/3, p.Breadth.FallingWeight, 0.001)
	// Weighted score: (0.8*1000 + (-0.3)*1000) / (1000+1000) = 0.5/2000 = 0.25
	// Note: flat holdings with TrendScore 0 are excluded from weighted score
	assert.InDelta(t, 0.25, p.Breadth.TrendScore, 0.001)
	assert.Equal(t, "Uptrend", p.Breadth.TrendLabel)
}

func TestComputeBreadth_NoOpenHoldings(t *testing.T) {
	svc := &Service{}
	p := &models.Portfolio{
		Holdings: []models.Holding{
			{Status: "closed", MarketValue: 0, TrendLabel: "Uptrend"},
			{Status: "closed", MarketValue: 0, TrendLabel: "Downtrend"},
		},
	}

	svc.computeBreadth(p)

	assert.Nil(t, p.Breadth)
}

func TestComputeBreadth_NoTrendData(t *testing.T) {
	svc := &Service{}
	p := &models.Portfolio{
		Holdings: []models.Holding{
			{Status: "open", MarketValue: 1000, TrendLabel: "", CurrentPrice: 10, Units: 100},
			{Status: "open", MarketValue: 2000, TrendLabel: "", CurrentPrice: 20, Units: 100},
		},
	}

	svc.computeBreadth(p)

	assert.NotNil(t, p.Breadth)
	assert.Equal(t, 0, p.Breadth.RisingCount)
	assert.Equal(t, 2, p.Breadth.FlatCount)
	assert.Equal(t, 0, p.Breadth.FallingCount)
	assert.InDelta(t, 1.0, p.Breadth.FlatWeight, 0.001)
}

func TestComputeBreadth_DollarWeighting(t *testing.T) {
	svc := &Service{}
	// Large position is falling, small position is rising
	p := &models.Portfolio{
		Holdings: []models.Holding{
			{Status: "open", MarketValue: 9000, TrendLabel: "Downtrend", TrendScore: -0.3, CurrentPrice: 90, YesterdayClosePrice: 92, Units: 100},
			{Status: "open", MarketValue: 1000, TrendLabel: "Strong Uptrend", TrendScore: 0.8, CurrentPrice: 10, YesterdayClosePrice: 9, Units: 100},
		},
	}

	svc.computeBreadth(p)

	assert.NotNil(t, p.Breadth)
	assert.InDelta(t, 0.9, p.Breadth.FallingWeight, 0.001)
	assert.InDelta(t, 0.1, p.Breadth.RisingWeight, 0.001)
	// Weighted score: (-0.3*9000 + 0.8*1000) / (9000+1000) = -1900/10000 = -0.19
	assert.InDelta(t, -0.19, p.Breadth.TrendScore, 0.001)
	assert.Equal(t, "Downtrend", p.Breadth.TrendLabel)
}

func TestComputeBreadth_TodayChange(t *testing.T) {
	svc := &Service{}
	p := &models.Portfolio{
		Holdings: []models.Holding{
			{Status: "open", MarketValue: 1000, TrendLabel: "Uptrend", CurrentPrice: 10, YesterdayClosePrice: 9, Units: 100},    // +$100
			{Status: "open", MarketValue: 2000, TrendLabel: "Downtrend", CurrentPrice: 20, YesterdayClosePrice: 21, Units: 100}, // -$100
		},
	}

	svc.computeBreadth(p)

	assert.NotNil(t, p.Breadth)
	// (10-9)*100 + (20-21)*100 = 100 - 100 = 0
	assert.InDelta(t, 0, p.Breadth.TodayChange, 0.001)
	assert.InDelta(t, 0, p.Breadth.TodayChangePct, 0.001)
}

func TestBreadthTrendLabel_Boundaries(t *testing.T) {
	tests := []struct {
		name  string
		score float64
		want  string
	}{
		{"strong_uptrend", 0.5, "Strong Uptrend"},
		{"strong_uptrend_boundary", 0.4, "Strong Uptrend"},
		{"uptrend", 0.3, "Uptrend"},
		{"uptrend_boundary", 0.15, "Uptrend"},
		{"mixed_positive", 0.14, "Mixed"},
		{"mixed_zero", 0.0, "Mixed"},
		{"mixed_negative", -0.14, "Mixed"},
		{"downtrend_boundary", -0.15, "Downtrend"},              // exactly -0.15 is not > -0.15, so Downtrend
		{"downtrend", -0.16, "Downtrend"},                       // < -0.15 is Downtrend
		{"downtrend_near_strong", -0.39, "Downtrend"},           // > -0.4 is Downtrend
		{"strong_downtrend_boundary", -0.4, "Strong Downtrend"}, // exactly -0.4 is not > -0.4, so Strong Downtrend
		{"strong_downtrend", -0.5, "Strong Downtrend"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, breadthTrendLabel(tt.score))
		})
	}
}
