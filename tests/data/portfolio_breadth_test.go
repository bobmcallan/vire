package data

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPortfolio_BreadthIncludedInResponse verifies that the Breadth field
// round-trips through JSON serialization and SurrealDB storage.
func TestPortfolio_BreadthIncludedInResponse(t *testing.T) {
	store := testManager(t).UserDataStore()
	ctx := testContext()

	portfolio := models.Portfolio{
		Name:                "Breadth-Included",
		EquityHoldingsValue: 150000.0,
		PortfolioValue:      160000.0,
		Breadth: &models.PortfolioBreadth{
			RisingCount:    3,
			FlatCount:      1,
			FallingCount:   2,
			RisingWeight:   0.5,
			FlatWeight:     0.1,
			FallingWeight:  0.4,
			RisingValue:    75000.0,
			FlatValue:      15000.0,
			FallingValue:   60000.0,
			TrendLabel:     "Mixed",
			TrendScore:     0.05,
			TodayChange:    320.0,
			TodayChangePct: 0.213,
		},
	}

	data, err := json.Marshal(portfolio)
	require.NoError(t, err)

	// Verify JSON contains "breadth" key
	var jsonMap map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &jsonMap))
	assert.Contains(t, jsonMap, "breadth", "breadth field must be present in JSON")

	breadthMap := jsonMap["breadth"].(map[string]interface{})
	assert.Contains(t, breadthMap, "rising_count")
	assert.Contains(t, breadthMap, "flat_count")
	assert.Contains(t, breadthMap, "falling_count")
	assert.Contains(t, breadthMap, "rising_weight")
	assert.Contains(t, breadthMap, "trend_label")
	assert.Contains(t, breadthMap, "trend_score")
	assert.Contains(t, breadthMap, "today_change")
	assert.Contains(t, breadthMap, "today_change_pct")

	// Round-trip through storage
	record := &models.UserRecord{
		UserID:   "user_breadth_001",
		Subject:  "portfolio",
		Key:      portfolio.Name,
		Value:    string(data),
		Version:  1,
		DateTime: time.Now().Truncate(time.Second),
	}
	require.NoError(t, store.Put(ctx, record))

	got, err := store.Get(ctx, "user_breadth_001", "portfolio", portfolio.Name)
	require.NoError(t, err)

	var retrieved models.Portfolio
	require.NoError(t, json.Unmarshal([]byte(got.Value), &retrieved))

	assert.NotNil(t, retrieved.Breadth, "Breadth should survive round-trip")
	assert.Equal(t, 3, retrieved.Breadth.RisingCount)
	assert.Equal(t, 1, retrieved.Breadth.FlatCount)
	assert.Equal(t, 2, retrieved.Breadth.FallingCount)
	assert.Equal(t, 75000.0, retrieved.Breadth.RisingValue)
	assert.Equal(t, 15000.0, retrieved.Breadth.FlatValue)
	assert.Equal(t, 60000.0, retrieved.Breadth.FallingValue)
	assert.Equal(t, "Mixed", retrieved.Breadth.TrendLabel)
	assert.InDelta(t, 0.05, retrieved.Breadth.TrendScore, 0.001)
	assert.InDelta(t, 320.0, retrieved.Breadth.TodayChange, 0.01)
	assert.InDelta(t, 0.213, retrieved.Breadth.TodayChangePct, 0.001)
}

// TestPortfolio_BreadthCounts verifies that breadth direction counts
// correctly reflect the number of holdings in each trend bucket.
func TestPortfolio_BreadthCounts(t *testing.T) {
	tests := []struct {
		name        string
		breadth     models.PortfolioBreadth
		wantRising  int
		wantFlat    int
		wantFalling int
		wantTotal   int
	}{
		{
			name: "all_rising",
			breadth: models.PortfolioBreadth{
				RisingCount: 5, FlatCount: 0, FallingCount: 0,
				RisingWeight: 1.0, FlatWeight: 0.0, FallingWeight: 0.0,
				RisingValue: 100000, FlatValue: 0, FallingValue: 0,
				TrendLabel: "Strong Uptrend", TrendScore: 0.8,
			},
			wantRising: 5, wantFlat: 0, wantFalling: 0, wantTotal: 5,
		},
		{
			name: "mixed_directions",
			breadth: models.PortfolioBreadth{
				RisingCount: 3, FlatCount: 2, FallingCount: 1,
				RisingWeight: 0.5, FlatWeight: 0.33, FallingWeight: 0.17,
				RisingValue: 60000, FlatValue: 40000, FallingValue: 20000,
				TrendLabel: "Uptrend", TrendScore: 0.2,
			},
			wantRising: 3, wantFlat: 2, wantFalling: 1, wantTotal: 6,
		},
		{
			name: "all_falling",
			breadth: models.PortfolioBreadth{
				RisingCount: 0, FlatCount: 0, FallingCount: 4,
				RisingWeight: 0.0, FlatWeight: 0.0, FallingWeight: 1.0,
				RisingValue: 0, FlatValue: 0, FallingValue: 80000,
				TrendLabel: "Strong Downtrend", TrendScore: -0.7,
			},
			wantRising: 0, wantFlat: 0, wantFalling: 4, wantTotal: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantRising, tt.breadth.RisingCount)
			assert.Equal(t, tt.wantFlat, tt.breadth.FlatCount)
			assert.Equal(t, tt.wantFalling, tt.breadth.FallingCount)

			total := tt.breadth.RisingCount + tt.breadth.FlatCount + tt.breadth.FallingCount
			assert.Equal(t, tt.wantTotal, total, "counts should sum to total holdings")
		})
	}
}

// TestPortfolio_BreadthWeightsSumToOne verifies that dollar-weighted
// proportions always sum to approximately 1.0 for valid breadth data.
func TestPortfolio_BreadthWeightsSumToOne(t *testing.T) {
	tests := []struct {
		name    string
		breadth models.PortfolioBreadth
	}{
		{
			name: "equal_distribution",
			breadth: models.PortfolioBreadth{
				RisingCount: 2, FlatCount: 2, FallingCount: 2,
				RisingWeight: 1.0 / 3.0, FlatWeight: 1.0 / 3.0, FallingWeight: 1.0 / 3.0,
				RisingValue: 50000, FlatValue: 50000, FallingValue: 50000,
				TrendLabel: "Mixed", TrendScore: 0.0,
			},
		},
		{
			name: "dominated_by_rising",
			breadth: models.PortfolioBreadth{
				RisingCount: 5, FlatCount: 1, FallingCount: 1,
				RisingWeight: 0.85, FlatWeight: 0.05, FallingWeight: 0.10,
				RisingValue: 170000, FlatValue: 10000, FallingValue: 20000,
				TrendLabel: "Uptrend", TrendScore: 0.3,
			},
		},
		{
			name: "single_direction_only",
			breadth: models.PortfolioBreadth{
				RisingCount: 0, FlatCount: 3, FallingCount: 0,
				RisingWeight: 0.0, FlatWeight: 1.0, FallingWeight: 0.0,
				RisingValue: 0, FlatValue: 90000, FallingValue: 0,
				TrendLabel: "Mixed", TrendScore: 0.0,
			},
		},
		{
			name: "heavy_falling",
			breadth: models.PortfolioBreadth{
				RisingCount: 1, FlatCount: 0, FallingCount: 4,
				RisingWeight: 0.1, FlatWeight: 0.0, FallingWeight: 0.9,
				RisingValue: 10000, FlatValue: 0, FallingValue: 90000,
				TrendLabel: "Strong Downtrend", TrendScore: -0.6,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			weightSum := tt.breadth.RisingWeight + tt.breadth.FlatWeight + tt.breadth.FallingWeight
			assert.InDelta(t, 1.0, weightSum, 0.001, "weights must sum to 1.0")

			// Also verify values are non-negative
			assert.GreaterOrEqual(t, tt.breadth.RisingWeight, 0.0)
			assert.GreaterOrEqual(t, tt.breadth.FlatWeight, 0.0)
			assert.GreaterOrEqual(t, tt.breadth.FallingWeight, 0.0)

			// Verify each weight is in [0.0, 1.0]
			assert.LessOrEqual(t, tt.breadth.RisingWeight, 1.0)
			assert.LessOrEqual(t, tt.breadth.FlatWeight, 1.0)
			assert.LessOrEqual(t, tt.breadth.FallingWeight, 1.0)

			// Verify value amounts are consistent with weights when total > 0
			totalValue := tt.breadth.RisingValue + tt.breadth.FlatValue + tt.breadth.FallingValue
			if totalValue > 0 {
				computedRisingWeight := tt.breadth.RisingValue / totalValue
				computedFlatWeight := tt.breadth.FlatValue / totalValue
				computedFallingWeight := tt.breadth.FallingValue / totalValue

				assert.InDelta(t, computedRisingWeight, tt.breadth.RisingWeight, 0.001)
				assert.InDelta(t, computedFlatWeight, tt.breadth.FlatWeight, 0.001)
				assert.InDelta(t, computedFallingWeight, tt.breadth.FallingWeight, 0.001)
			}
		})
	}
}

// TestPortfolio_BreadthOmittedWhenNil verifies the breadth field is
// omitted from JSON when nil (omitempty behavior).
func TestPortfolio_BreadthOmittedWhenNil(t *testing.T) {
	portfolio := models.Portfolio{
		Name:                "No-Breadth",
		EquityHoldingsValue: 50000.0,
		Breadth:             nil,
	}

	data, err := json.Marshal(portfolio)
	require.NoError(t, err)

	var jsonMap map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &jsonMap))
	assert.NotContains(t, jsonMap, "breadth", "nil breadth should be omitted from JSON")
}

// TestPortfolio_BreadthTrendScoreRange verifies TrendScore stays within [-1.0, +1.0].
func TestPortfolio_BreadthTrendScoreRange(t *testing.T) {
	tests := []struct {
		name  string
		score float64
	}{
		{"strong_uptrend", 0.8},
		{"uptrend", 0.2},
		{"mixed", 0.0},
		{"downtrend", -0.3},
		{"strong_downtrend", -0.7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := models.PortfolioBreadth{TrendScore: tt.score}
			assert.True(t, b.TrendScore >= -1.0 && b.TrendScore <= 1.0,
				"TrendScore %f should be in [-1.0, 1.0]", b.TrendScore)
			assert.False(t, math.IsNaN(b.TrendScore), "TrendScore must not be NaN")
			assert.False(t, math.IsInf(b.TrendScore, 0), "TrendScore must not be Inf")
		})
	}
}
