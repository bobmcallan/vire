package market

import (
	"math"
	"time"

	"github.com/bobmcallan/vire/internal/models"
)

// computeQualityAssessment calculates a quality rating from fundamentals data.
func computeQualityAssessment(fund *models.Fundamentals) *models.QualityAssessment {
	if fund == nil {
		return nil
	}

	qa := &models.QualityAssessment{
		AssessedAt: time.Now(),
	}

	// Score each metric
	qa.ROE = scoreROE(fund.ReturnOnEquityTTM)
	qa.GrossMargin = scoreGrossMargin(fund.GrossProfitTTM, fund.RevenueTTM)
	qa.FCFConversion = scoreFCFConversion(fund) // estimated from EBITDA/Revenue
	qa.NetDebtToEBITDA = scoreNetDebtToEBITDA(fund)
	qa.EarningsStability = scoreEarningsStability(fund.HistoricalFinancials)
	qa.RevenueGrowth = scoreRevenueGrowth(fund.RevGrowthYOY)
	qa.MarginTrend = scoreMarginTrend(fund.HistoricalFinancials)

	// Detect red flags and strengths
	qa.RedFlags = detectRedFlags(fund, qa)
	qa.Strengths = detectStrengths(fund, qa)

	// Compute overall score (weighted average of component scores)
	totalWeight := 0.0
	weightedScore := 0.0
	metrics := []struct {
		metric models.QualityMetric
		weight float64
	}{
		{qa.ROE, 20},
		{qa.GrossMargin, 15},
		{qa.FCFConversion, 10},
		{qa.NetDebtToEBITDA, 10},
		{qa.EarningsStability, 15},
		{qa.RevenueGrowth, 20},
		{qa.MarginTrend, 10},
	}
	for _, m := range metrics {
		weightedScore += float64(m.metric.Score) * m.weight
		totalWeight += m.weight
	}

	if totalWeight > 0 {
		qa.OverallScore = int(math.Round(weightedScore / totalWeight))
	}

	// Clamp to 0-100
	if qa.OverallScore < 0 {
		qa.OverallScore = 0
	}
	if qa.OverallScore > 100 {
		qa.OverallScore = 100
	}

	qa.OverallRating = qualityRating(qa.OverallScore)

	return qa
}

// qualityRating maps a 0-100 score to a rating label.
func qualityRating(score int) string {
	switch {
	case score >= 80:
		return "High Quality"
	case score >= 60:
		return "Quality"
	case score >= 40:
		return "Average"
	case score >= 20:
		return "Below Average"
	default:
		return "Speculative"
	}
}

func scoreROE(roe float64) models.QualityMetric {
	m := models.QualityMetric{
		Value:     roe,
		Benchmark: "> 15%",
	}
	switch {
	case roe >= 20:
		m.Rating = "excellent"
		m.Score = 100
	case roe >= 15:
		m.Rating = "good"
		m.Score = 75
	case roe >= 10:
		m.Rating = "average"
		m.Score = 50
	case roe >= 0:
		m.Rating = "poor"
		m.Score = 25
	default:
		m.Rating = "poor"
		m.Score = 0
	}
	return m
}

func scoreGrossMargin(grossProfit, revenue float64) models.QualityMetric {
	var margin float64
	if revenue > 0 {
		margin = (grossProfit / revenue) * 100
	}

	m := models.QualityMetric{
		Value:     math.Round(margin*100) / 100,
		Benchmark: "> 40%",
	}
	switch {
	case margin >= 50:
		m.Rating = "excellent"
		m.Score = 100
	case margin >= 40:
		m.Rating = "good"
		m.Score = 75
	case margin >= 30:
		m.Rating = "average"
		m.Score = 50
	case margin >= 15:
		m.Rating = "poor"
		m.Score = 25
	default:
		m.Rating = "poor"
		m.Score = 0
	}
	return m
}

func scoreFCFConversion(fund *models.Fundamentals) models.QualityMetric {
	// Estimate FCF conversion as EBITDA/Revenue (simplified proxy)
	var ratio float64
	if fund.RevenueTTM > 0 && fund.EBITDA != 0 {
		ratio = (fund.EBITDA / fund.RevenueTTM) * 100
	}

	m := models.QualityMetric{
		Value:     math.Round(ratio*100) / 100,
		Benchmark: "> 15%",
	}
	switch {
	case ratio >= 25:
		m.Rating = "excellent"
		m.Score = 100
	case ratio >= 15:
		m.Rating = "good"
		m.Score = 75
	case ratio >= 8:
		m.Rating = "average"
		m.Score = 50
	case ratio >= 0:
		m.Rating = "poor"
		m.Score = 25
	default:
		m.Rating = "poor"
		m.Score = 0
	}
	return m
}

func scoreNetDebtToEBITDA(fund *models.Fundamentals) models.QualityMetric {
	// Without explicit debt data, use a simplified score
	// If EBITDA is negative, that's a red flag
	m := models.QualityMetric{
		Benchmark: "< 2x",
	}

	if fund.EBITDA <= 0 {
		m.Value = 0
		m.Rating = "poor"
		m.Score = 0
		return m
	}

	// Without explicit debt, give a middle score
	m.Value = 0
	m.Rating = "average"
	m.Score = 50
	return m
}

func scoreEarningsStability(historicals []models.HistoricalPeriod) models.QualityMetric {
	m := models.QualityMetric{
		Benchmark: "Low EPS variance",
	}

	if len(historicals) < 2 {
		m.Rating = "average"
		m.Score = 50
		return m
	}

	// Check for consistent positive earnings
	positiveCount := 0
	for _, h := range historicals {
		if h.NetIncome > 0 {
			positiveCount++
		}
	}

	positiveRatio := float64(positiveCount) / float64(len(historicals))
	m.Value = math.Round(positiveRatio*100) / 100

	switch {
	case positiveRatio >= 1.0:
		m.Rating = "excellent"
		m.Score = 100
	case positiveRatio >= 0.75:
		m.Rating = "good"
		m.Score = 75
	case positiveRatio >= 0.5:
		m.Rating = "average"
		m.Score = 50
	default:
		m.Rating = "poor"
		m.Score = 25
	}
	return m
}

func scoreRevenueGrowth(growthYOY float64) models.QualityMetric {
	m := models.QualityMetric{
		Value:     growthYOY,
		Benchmark: "> 10%",
	}
	switch {
	case growthYOY >= 20:
		m.Rating = "excellent"
		m.Score = 100
	case growthYOY >= 10:
		m.Rating = "good"
		m.Score = 75
	case growthYOY >= 0:
		m.Rating = "average"
		m.Score = 50
	case growthYOY >= -10:
		m.Rating = "poor"
		m.Score = 25
	default:
		m.Rating = "poor"
		m.Score = 0
	}
	return m
}

func scoreMarginTrend(historicals []models.HistoricalPeriod) models.QualityMetric {
	m := models.QualityMetric{
		Benchmark: "Improving",
	}

	if len(historicals) < 2 {
		m.Rating = "average"
		m.Score = 50
		return m
	}

	// Compare most recent vs oldest margin
	newest := historicals[0]
	oldest := historicals[len(historicals)-1]

	var newestMargin, oldestMargin float64
	if newest.Revenue > 0 {
		newestMargin = newest.GrossProfit / newest.Revenue
	}
	if oldest.Revenue > 0 {
		oldestMargin = oldest.GrossProfit / oldest.Revenue
	}

	marginChange := (newestMargin - oldestMargin) * 100
	m.Value = math.Round(marginChange*100) / 100

	switch {
	case marginChange >= 5:
		m.Rating = "excellent"
		m.Score = 100
	case marginChange >= 0:
		m.Rating = "good"
		m.Score = 75
	case marginChange >= -5:
		m.Rating = "average"
		m.Score = 50
	default:
		m.Rating = "poor"
		m.Score = 25
	}
	return m
}

func detectRedFlags(fund *models.Fundamentals, qa *models.QualityAssessment) []string {
	var flags []string

	if fund.ReturnOnEquityTTM < 0 {
		flags = append(flags, "Negative return on equity")
	}
	if fund.EBITDA < 0 {
		flags = append(flags, "Negative EBITDA")
	}
	if fund.RevGrowthYOY < -10 {
		flags = append(flags, "Revenue declining over 10%")
	}

	// Flat revenue check from historicals
	if len(fund.HistoricalFinancials) >= 3 {
		newest := fund.HistoricalFinancials[0].Revenue
		oldest := fund.HistoricalFinancials[len(fund.HistoricalFinancials)-1].Revenue
		if oldest > 0 && newest > 0 {
			growth := ((newest - oldest) / oldest) * 100
			if math.Abs(growth) < 5 {
				flags = append(flags, "Flat revenue over reporting history")
			}
		}
	}

	return flags
}

func detectStrengths(fund *models.Fundamentals, qa *models.QualityAssessment) []string {
	var strengths []string

	if qa.ROE.Rating == "excellent" {
		strengths = append(strengths, "Strong return on equity")
	}
	if qa.GrossMargin.Rating == "excellent" {
		strengths = append(strengths, "High gross margin")
	}
	if qa.RevenueGrowth.Rating == "excellent" {
		strengths = append(strengths, "Strong revenue growth")
	}
	if qa.EarningsStability.Rating == "excellent" {
		strengths = append(strengths, "Consistent positive earnings")
	}
	if qa.MarginTrend.Rating == "excellent" {
		strengths = append(strengths, "Improving margins")
	}

	return strengths
}
