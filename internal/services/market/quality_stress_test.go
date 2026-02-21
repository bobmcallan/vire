package market

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

// ============================================================================
// Quality Assessment — edge cases, hostile inputs, boundary conditions
// ============================================================================

// 1. Negative revenue — division creates negative margin, should not panic
func TestStressQuality_NegativeRevenue(t *testing.T) {
	fund := &models.Fundamentals{
		RevenueTTM:     -500_000_000, // negative revenue (restatement, accounting quirk)
		GrossProfitTTM: 100_000_000,
		EBITDA:         -50_000_000,
	}

	qa := computeQualityAssessment(fund)
	if qa == nil {
		t.Fatal("expected non-nil quality assessment even with negative revenue")
	}

	// Gross margin = 100M / -500M = -20% — should not crash, should score poorly
	if qa.GrossMargin.Rating != "poor" {
		t.Errorf("GrossMargin.Rating = %s, expected poor for negative revenue", qa.GrossMargin.Rating)
	}
	if qa.OverallScore < 0 || qa.OverallScore > 100 {
		t.Errorf("OverallScore %d out of 0-100 range", qa.OverallScore)
	}
}

// 2. NaN / Inf values in fundamentals — should not panic or produce NaN scores
func TestStressQuality_NaNAndInfValues(t *testing.T) {
	tests := []struct {
		name string
		fund *models.Fundamentals
	}{
		{
			"NaN ROE",
			&models.Fundamentals{ReturnOnEquityTTM: math.NaN()},
		},
		{
			"Inf revenue growth",
			&models.Fundamentals{RevGrowthYOY: math.Inf(1)},
		},
		{
			"NegInf EBITDA",
			&models.Fundamentals{EBITDA: math.Inf(-1)},
		},
		{
			"NaN gross profit with positive revenue",
			&models.Fundamentals{GrossProfitTTM: math.NaN(), RevenueTTM: 1_000_000},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qa := computeQualityAssessment(tt.fund)
			if qa == nil {
				t.Fatal("expected non-nil result")
			}

			// Overall score should be clamped to 0-100
			if qa.OverallScore < 0 || qa.OverallScore > 100 {
				t.Errorf("OverallScore %d out of bounds", qa.OverallScore)
			}

			// Score should not be NaN
			if math.IsNaN(float64(qa.OverallScore)) {
				t.Error("OverallScore is NaN")
			}
		})
	}
}

// 3. All-zero fundamentals — pre-revenue company with no data
func TestStressQuality_AllZeroFundamentals(t *testing.T) {
	fund := &models.Fundamentals{} // all fields zero

	qa := computeQualityAssessment(fund)
	if qa == nil {
		t.Fatal("expected non-nil for zero fundamentals")
	}

	// Should produce a valid score in bounds
	if qa.OverallScore < 0 || qa.OverallScore > 100 {
		t.Errorf("OverallScore %d out of bounds for zero fundamentals", qa.OverallScore)
	}

	// Rating should be valid
	validRatings := map[string]bool{
		"High Quality": true, "Quality": true, "Average": true,
		"Below Average": true, "Speculative": true,
	}
	if !validRatings[qa.OverallRating] {
		t.Errorf("OverallRating %q is not a valid rating", qa.OverallRating)
	}
}

// 4. Extremely large values — should not overflow
func TestStressQuality_ExtremeValues(t *testing.T) {
	fund := &models.Fundamentals{
		ReturnOnEquityTTM: 999_999.0,
		GrossProfitTTM:    math.MaxFloat64 / 2,
		RevenueTTM:        math.MaxFloat64 / 4,
		RevGrowthYOY:      99999.0,
		EBITDA:            math.MaxFloat64 / 2,
	}

	qa := computeQualityAssessment(fund)
	if qa == nil {
		t.Fatal("expected non-nil result for extreme values")
	}

	// Should clamp to 100, not overflow
	if qa.OverallScore > 100 {
		t.Errorf("OverallScore %d exceeds 100 for extreme values", qa.OverallScore)
	}
}

// 5. Single historical period — margin trend and earnings stability with min data
func TestStressQuality_SingleHistoricalPeriod(t *testing.T) {
	fund := &models.Fundamentals{
		ReturnOnEquityTTM: 10,
		HistoricalFinancials: []models.HistoricalPeriod{
			{Date: "2025-06-30", Revenue: 100_000_000, NetIncome: 10_000_000, GrossProfit: 40_000_000},
		},
	}

	qa := computeQualityAssessment(fund)
	if qa == nil {
		t.Fatal("expected non-nil result with single historical period")
	}

	// With only 1 period, margin trend and earnings stability should default to average
	if qa.MarginTrend.Rating != "average" {
		t.Errorf("MarginTrend.Rating = %s, expected average for single period", qa.MarginTrend.Rating)
	}
	if qa.EarningsStability.Rating != "average" {
		t.Errorf("EarningsStability.Rating = %s, expected average for single period", qa.EarningsStability.Rating)
	}
}

// 6. Historical periods with zero revenue — margin computation division by zero
func TestStressQuality_ZeroRevenueHistoricals(t *testing.T) {
	fund := &models.Fundamentals{
		ReturnOnEquityTTM: 5,
		HistoricalFinancials: []models.HistoricalPeriod{
			{Date: "2025-06-30", Revenue: 0, NetIncome: -5_000_000, GrossProfit: 0},
			{Date: "2024-06-30", Revenue: 0, NetIncome: -3_000_000, GrossProfit: 0},
			{Date: "2023-06-30", Revenue: 0, NetIncome: -2_000_000, GrossProfit: 0},
		},
	}

	qa := computeQualityAssessment(fund)
	if qa == nil {
		t.Fatal("expected non-nil result")
	}

	// MarginTrend should handle zero revenue without panic or NaN
	if math.IsNaN(qa.MarginTrend.Value) {
		t.Error("MarginTrend.Value is NaN — division by zero in margin computation")
	}
	if math.IsInf(qa.MarginTrend.Value, 0) {
		t.Error("MarginTrend.Value is Inf — division by zero in margin computation")
	}
}

// 7. detectRedFlags — flat revenue boundary (exactly 5%)
func TestStressQuality_FlatRevenueBoundary(t *testing.T) {
	// Growth of exactly 4.99% should be flagged as "flat"
	fund := &models.Fundamentals{
		HistoricalFinancials: []models.HistoricalPeriod{
			{Date: "2025-06-30", Revenue: 104_900_000},
			{Date: "2024-06-30", Revenue: 102_000_000},
			{Date: "2023-06-30", Revenue: 100_000_000},
		},
	}

	qa := computeQualityAssessment(fund)
	if qa == nil {
		t.Fatal("expected non-nil result")
	}

	hasFlatFlag := false
	for _, rf := range qa.RedFlags {
		if rf == "Flat revenue over reporting history" {
			hasFlatFlag = true
		}
	}
	if !hasFlatFlag {
		t.Error("expected 'Flat revenue' red flag for < 5% growth over 3 years")
	}
}

// 8. Quality rating boundary values
func TestStressQuality_RatingBoundaries(t *testing.T) {
	tests := []struct {
		score    int
		expected string
	}{
		{0, "Speculative"},
		{19, "Speculative"},
		{20, "Below Average"},
		{39, "Below Average"},
		{40, "Average"},
		{59, "Average"},
		{60, "Quality"},
		{79, "Quality"},
		{80, "High Quality"},
		{100, "High Quality"},
	}

	for _, tt := range tests {
		got := qualityRating(tt.score)
		if got != tt.expected {
			t.Errorf("qualityRating(%d) = %q, want %q", tt.score, got, tt.expected)
		}
	}
}

// 9. Negative score boundary — OverallScore should be clamped to 0
func TestStressQuality_NegativeScoreClamped(t *testing.T) {
	// Extremely negative fundamentals
	fund := &models.Fundamentals{
		ReturnOnEquityTTM: -999.0,
		RevGrowthYOY:      -999.0,
		EBITDA:            -999_000_000,
	}

	qa := computeQualityAssessment(fund)
	if qa == nil {
		t.Fatal("expected non-nil result")
	}

	if qa.OverallScore < 0 {
		t.Errorf("OverallScore %d should be clamped to >= 0", qa.OverallScore)
	}
}

// 10. GetStockData computes quality assessment on demand
func TestStressQuality_GetStockData_ComputesOnDemand(t *testing.T) {
	today := time.Now()

	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {
					Ticker:      "BHP.AU",
					Exchange:    "AU",
					LastUpdated: today,
					EOD:         []models.EODBar{{Date: today, Close: 42.50}},
					Fundamentals: &models.Fundamentals{
						ReturnOnEquityTTM: 20.0,
						GrossProfitTTM:    500_000_000,
						RevenueTTM:        1_000_000_000,
						EBITDA:            200_000_000,
					},
					QualityAssessment: nil, // not computed yet
					// Provide filings + fresh timestamp to prevent GetStockData from
					// triggering collectFilings -> downloadFilingPDFs -> real HTTP calls
					Filings:          []models.CompanyFiling{{Date: today, Headline: "Test"}},
					FilingsUpdatedAt: today,
				},
			},
		},
		signals: &mockSignalStorage{},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, logger)

	data, err := svc.GetStockData(context.Background(), "BHP.AU", interfaces.StockDataInclude{Price: false})
	if err != nil {
		t.Fatalf("GetStockData failed: %v", err)
	}

	// Should compute quality assessment on demand
	if data.QualityAssessment == nil {
		t.Fatal("expected QualityAssessment to be computed on demand")
	}

	if data.QualityAssessment.OverallScore < 60 {
		t.Errorf("OverallScore %d unexpectedly low for good fundamentals", data.QualityAssessment.OverallScore)
	}

	// Should also persist to storage
	saved := storage.market.data["BHP.AU"]
	if saved.QualityAssessment == nil {
		t.Error("QualityAssessment should be persisted to storage")
	}
}

// 11. GetStockData does NOT recompute when assessment already exists
func TestStressQuality_GetStockData_SkipsExisting(t *testing.T) {
	today := time.Now()
	existingAssessment := &models.QualityAssessment{
		OverallScore:  42,
		OverallRating: "Average",
		AssessedAt:    today.Add(-1 * time.Hour),
	}

	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {
					Ticker:            "BHP.AU",
					Exchange:          "AU",
					LastUpdated:       today,
					EOD:               []models.EODBar{{Date: today, Close: 42.50}},
					Fundamentals:      &models.Fundamentals{ReturnOnEquityTTM: 25.0},
					QualityAssessment: existingAssessment,
					Filings:           []models.CompanyFiling{{Date: today, Headline: "Test"}},
					FilingsUpdatedAt:  today, // fresh — skip filing collection
				},
			},
		},
		signals: &mockSignalStorage{},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, logger)

	data, err := svc.GetStockData(context.Background(), "BHP.AU", interfaces.StockDataInclude{Price: false})
	if err != nil {
		t.Fatalf("GetStockData failed: %v", err)
	}

	// Should use existing, not recompute
	if data.QualityAssessment.OverallScore != 42 {
		t.Errorf("OverallScore = %d, expected 42 (existing assessment)", data.QualityAssessment.OverallScore)
	}
}

// 12. GetStockData with nil fundamentals — should not compute quality
func TestStressQuality_GetStockData_NilFundamentals(t *testing.T) {
	today := time.Now()

	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"NEW.AU": {
					Ticker:           "NEW.AU",
					Exchange:         "AU",
					LastUpdated:      today,
					EOD:              []models.EODBar{{Date: today, Close: 1.00}},
					Fundamentals:     nil, // no fundamentals
					Filings:          []models.CompanyFiling{{Date: today, Headline: "IPO"}},
					FilingsUpdatedAt: today, // fresh — skip filing collection
				},
			},
		},
		signals: &mockSignalStorage{},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, logger)

	data, err := svc.GetStockData(context.Background(), "NEW.AU", interfaces.StockDataInclude{Price: false})
	if err != nil {
		t.Fatalf("GetStockData failed: %v", err)
	}

	// Should NOT compute when no fundamentals
	if data.QualityAssessment != nil {
		t.Error("QualityAssessment should be nil when fundamentals are nil")
	}
}

// 13. Weighted scoring — verify weights sum correctly
func TestStressQuality_WeightedScoring(t *testing.T) {
	// All metrics at 100 should produce 100 overall
	fund := &models.Fundamentals{
		ReturnOnEquityTTM: 30.0,    // excellent: 100
		GrossProfitTTM:    600_000, // 60% margin: excellent: 100
		RevenueTTM:        1_000_000,
		RevGrowthYOY:      25.0, // excellent: 100
		EBITDA:            300_000,
		HistoricalFinancials: []models.HistoricalPeriod{
			{Date: "2025-06-30", Revenue: 1000, NetIncome: 200, GrossProfit: 600},
			{Date: "2024-06-30", Revenue: 800, NetIncome: 150, GrossProfit: 400},
		},
	}

	qa := computeQualityAssessment(fund)
	if qa == nil {
		t.Fatal("expected non-nil")
	}

	// With mostly excellent scores, overall should be high
	if qa.OverallScore < 70 {
		t.Errorf("OverallScore = %d, expected >= 70 for all-excellent metrics", qa.OverallScore)
	}
}

// 14. ETF fundamentals — should still work (IsETF flag doesn't change scoring)
func TestStressQuality_ETFFundamentals(t *testing.T) {
	fund := &models.Fundamentals{
		IsETF:          true,
		ExpenseRatio:   0.05,
		GrossProfitTTM: 0, // ETFs don't have gross profit
		RevenueTTM:     0,
		EBITDA:         0,
	}

	qa := computeQualityAssessment(fund)
	if qa == nil {
		t.Fatal("expected non-nil for ETF")
	}

	// Should produce a valid (likely low) score
	if qa.OverallScore < 0 || qa.OverallScore > 100 {
		t.Errorf("OverallScore %d out of bounds for ETF", qa.OverallScore)
	}
}
