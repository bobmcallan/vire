package market

import (
	"testing"

	"github.com/bobmcallan/vire/internal/models"
)

func TestComputeQualityAssessment_NilFundamentals(t *testing.T) {
	result := computeQualityAssessment(nil)
	if result != nil {
		t.Error("expected nil for nil fundamentals")
	}
}

func TestComputeQualityAssessment_HighQuality(t *testing.T) {
	fund := &models.Fundamentals{
		ReturnOnEquityTTM: 25.0,
		GrossProfitTTM:    500_000_000,
		RevenueTTM:        1_000_000_000,
		RevGrowthYOY:      15.0,
		EBITDA:            200_000_000,
		HistoricalFinancials: []models.HistoricalPeriod{
			{Date: "2025-06-30", Revenue: 1_000_000_000, NetIncome: 100_000_000, GrossProfit: 500_000_000},
			{Date: "2024-06-30", Revenue: 870_000_000, NetIncome: 90_000_000, GrossProfit: 435_000_000},
			{Date: "2023-06-30", Revenue: 780_000_000, NetIncome: 80_000_000, GrossProfit: 390_000_000},
		},
	}

	qa := computeQualityAssessment(fund)
	if qa == nil {
		t.Fatal("expected non-nil quality assessment")
	}

	// ROE of 25% should score well
	if qa.ROE.Rating != "excellent" {
		t.Errorf("ROE rating = %s, want excellent for 25%% ROE", qa.ROE.Rating)
	}

	// 50% gross margin should score well
	if qa.GrossMargin.Rating != "excellent" {
		t.Errorf("GrossMargin rating = %s, want excellent for 50%% margin", qa.GrossMargin.Rating)
	}

	// Revenue growth of 15% should be good or excellent
	if qa.RevenueGrowth.Rating == "poor" {
		t.Errorf("RevenueGrowth rating = %s, unexpected poor for 15%% growth", qa.RevenueGrowth.Rating)
	}

	// Overall should be high quality or quality
	if qa.OverallScore < 60 {
		t.Errorf("OverallScore = %d, expected >= 60 for high-quality fundamentals", qa.OverallScore)
	}

	if qa.OverallRating != "High Quality" && qa.OverallRating != "Quality" {
		t.Errorf("OverallRating = %s, expected High Quality or Quality", qa.OverallRating)
	}

	if qa.AssessedAt.IsZero() {
		t.Error("AssessedAt should be set")
	}
}

func TestComputeQualityAssessment_PoorQuality(t *testing.T) {
	fund := &models.Fundamentals{
		ReturnOnEquityTTM: -5.0,
		GrossProfitTTM:    100_000_000,
		RevenueTTM:        1_000_000_000, // 10% gross margin
		RevGrowthYOY:      -10.0,
		EBITDA:            -50_000_000,
	}

	qa := computeQualityAssessment(fund)
	if qa == nil {
		t.Fatal("expected non-nil quality assessment")
	}

	// Negative ROE
	if qa.ROE.Rating != "poor" {
		t.Errorf("ROE rating = %s, want poor for negative ROE", qa.ROE.Rating)
	}

	// Overall should be low
	if qa.OverallScore > 40 {
		t.Errorf("OverallScore = %d, expected <= 40 for poor fundamentals", qa.OverallScore)
	}

	// Should have red flags
	if len(qa.RedFlags) == 0 {
		t.Error("expected red flags for negative fundamentals")
	}
}

func TestComputeQualityAssessment_AverageCompany(t *testing.T) {
	fund := &models.Fundamentals{
		ReturnOnEquityTTM: 12.0,
		GrossProfitTTM:    300_000_000,
		RevenueTTM:        1_000_000_000, // 30% gross margin
		RevGrowthYOY:      5.0,
		EBITDA:            150_000_000,
	}

	qa := computeQualityAssessment(fund)
	if qa == nil {
		t.Fatal("expected non-nil quality assessment")
	}

	// Should fall in the average range
	if qa.OverallScore < 20 || qa.OverallScore > 80 {
		t.Errorf("OverallScore = %d, expected 20-80 for average fundamentals", qa.OverallScore)
	}
}

func TestComputeQualityAssessment_ZeroRevenue(t *testing.T) {
	fund := &models.Fundamentals{
		ReturnOnEquityTTM: 10.0,
		RevenueTTM:        0, // pre-revenue company
		EBITDA:            -10_000_000,
	}

	qa := computeQualityAssessment(fund)
	if qa == nil {
		t.Fatal("expected non-nil quality assessment")
	}

	// Gross margin should be zero with zero revenue
	if qa.GrossMargin.Value != 0 {
		t.Errorf("GrossMargin value = %.2f, want 0 for zero revenue", qa.GrossMargin.Value)
	}
}

func TestComputeQualityAssessment_RedFlags(t *testing.T) {
	fund := &models.Fundamentals{
		ReturnOnEquityTTM: -15.0,
		RevGrowthYOY:      -5.0,
		EBITDA:            -100_000_000,
		HistoricalFinancials: []models.HistoricalPeriod{
			{Date: "2025-06-30", Revenue: 100_000_000, NetIncome: -10_000_000},
			{Date: "2024-06-30", Revenue: 100_000_000, NetIncome: -8_000_000},
			{Date: "2023-06-30", Revenue: 100_000_000, NetIncome: -5_000_000},
		},
	}

	qa := computeQualityAssessment(fund)
	if qa == nil {
		t.Fatal("expected non-nil quality assessment")
	}

	// Should detect negative ROE, negative EBITDA, and flat revenue
	hasNegativeROE := false
	hasNegativeEBITDA := false
	for _, rf := range qa.RedFlags {
		if rf == "Negative return on equity" {
			hasNegativeROE = true
		}
		if rf == "Negative EBITDA" {
			hasNegativeEBITDA = true
		}
	}
	if !hasNegativeROE {
		t.Error("expected 'Negative return on equity' red flag")
	}
	if !hasNegativeEBITDA {
		t.Error("expected 'Negative EBITDA' red flag")
	}
}

func TestComputeQualityAssessment_Strengths(t *testing.T) {
	fund := &models.Fundamentals{
		ReturnOnEquityTTM: 25.0,
		GrossProfitTTM:    600_000_000,
		RevenueTTM:        1_000_000_000, // 60% gross margin
		RevGrowthYOY:      20.0,
		EBITDA:            300_000_000,
	}

	qa := computeQualityAssessment(fund)
	if qa == nil {
		t.Fatal("expected non-nil quality assessment")
	}

	if len(qa.Strengths) == 0 {
		t.Error("expected strengths for strong fundamentals")
	}
}

func TestComputeQualityAssessment_RatingBands(t *testing.T) {
	tests := []struct {
		name     string
		score    int
		expected string
	}{
		{"high quality", 85, "High Quality"},
		{"quality", 70, "Quality"},
		{"average", 50, "Average"},
		{"below average", 30, "Below Average"},
		{"speculative", 10, "Speculative"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := qualityRating(tt.score)
			if got != tt.expected {
				t.Errorf("qualityRating(%d) = %s, want %s", tt.score, got, tt.expected)
			}
		})
	}
}

func TestComputeQualityAssessment_EarningsStability(t *testing.T) {
	// Company with consistent positive EPS
	fund := &models.Fundamentals{
		ReturnOnEquityTTM: 15.0,
		RevenueTTM:        1_000_000_000,
		GrossProfitTTM:    400_000_000,
		EPS:               1.50,
		HistoricalFinancials: []models.HistoricalPeriod{
			{Date: "2025-06-30", Revenue: 1_000_000_000, NetIncome: 100_000_000},
			{Date: "2024-06-30", Revenue: 950_000_000, NetIncome: 95_000_000},
			{Date: "2023-06-30", Revenue: 900_000_000, NetIncome: 90_000_000},
		},
	}

	qa := computeQualityAssessment(fund)
	if qa == nil {
		t.Fatal("expected non-nil quality assessment")
	}

	// Consistent earnings should score well on stability
	if qa.EarningsStability.Rating == "poor" {
		t.Errorf("EarningsStability rating = %s, expected better for consistent positive earnings", qa.EarningsStability.Rating)
	}
}
