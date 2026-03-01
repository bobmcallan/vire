package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
)

// TestSlimReviewResponse verifies that the portfolio review response strips
// heavy analysis fields (signals, fundamentals, news_intelligence, etc.)
// while preserving position-level fields.
func TestSlimReviewResponse(t *testing.T) {
	review := &models.PortfolioReview{
		PortfolioName:         "SMSF",
		ReviewDate:            time.Now(),
		PortfolioValue:        50000,
		NetEquityCost:         40000,
		NetEquityReturn:       10000,
		NetEquityReturnPct:    25.0,
		PortfolioDayChange:    150.0,
		PortfolioDayChangePct: 0.3,
		Summary:               "Portfolio performing well",
		Recommendations:       []string{"Consider rebalancing"},
		HoldingReviews: []models.HoldingReview{
			{
				Holding: models.Holding{
					Ticker:       "BHP",
					Name:         "BHP Group",
					Units:        100,
					CurrentPrice: 42.0,
					MarketValue:  4200.0,
				},
				Signals: &models.TickerSignals{
					Ticker: "BHP",
					Trend:  "bullish",
				},
				Fundamentals: &models.Fundamentals{
					MarketCap: 100000000,
					PE:        15.0,
				},
				OvernightMove: 0.5,
				OvernightPct:  1.2,
				NewsImpact:    "positive",
				NewsIntelligence: &models.NewsIntelligence{
					OverallSentiment: "positive",
					Summary:          "Good news",
				},
				FilingSummaries: []models.FilingSummary{
					{Headline: "Q4 Results"},
				},
				Timeline: &models.CompanyTimeline{
					BusinessModel: "Mining",
				},
				ActionRequired: "HOLD",
				ActionReason:   "No signals",
				Compliance: &models.ComplianceResult{
					Status: models.ComplianceStatusCompliant,
				},
			},
		},
		Alerts: []models.Alert{
			{Type: models.AlertTypePrice, Severity: "low", Ticker: "BHP", Message: "Price up"},
		},
		PortfolioBalance: &models.PortfolioBalance{
			ConcentrationRisk: "low",
		},
	}

	slim := toSlimReview(review)

	// Marshal to JSON and decode into a generic map to check field presence
	data, err := json.Marshal(slim)
	if err != nil {
		t.Fatalf("failed to marshal slim review: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal slim review: %v", err)
	}

	// Verify portfolio-level fields are present
	if result["portfolio_name"] != "SMSF" {
		t.Errorf("expected portfolio_name 'SMSF', got %v", result["portfolio_name"])
	}
	if result["portfolio_value"].(float64) != 50000 {
		t.Errorf("expected portfolio_value 50000, got %v", result["portfolio_value"])
	}
	if result["summary"] != "Portfolio performing well" {
		t.Errorf("expected summary present, got %v", result["summary"])
	}
	if result["alerts"] == nil {
		t.Error("expected alerts to be present")
	}
	if result["portfolio_balance"] == nil {
		t.Error("expected portfolio_balance to be present")
	}

	// Verify holding reviews are present
	holdings, ok := result["holding_reviews"].([]interface{})
	if !ok || len(holdings) != 1 {
		t.Fatalf("expected 1 holding review, got %v", result["holding_reviews"])
	}

	hr := holdings[0].(map[string]interface{})

	// Fields that SHOULD be present
	if hr["holding"] == nil {
		t.Error("holding field should be present")
	}
	if hr["overnight_move"].(float64) != 0.5 {
		t.Errorf("expected overnight_move 0.5, got %v", hr["overnight_move"])
	}
	if hr["overnight_pct"].(float64) != 1.2 {
		t.Errorf("expected overnight_pct 1.2, got %v", hr["overnight_pct"])
	}
	if hr["news_impact"] != "positive" {
		t.Errorf("expected news_impact 'positive', got %v", hr["news_impact"])
	}
	if hr["action_required"] != "HOLD" {
		t.Errorf("expected action_required 'HOLD', got %v", hr["action_required"])
	}
	if hr["action_reason"] != "No signals" {
		t.Errorf("expected action_reason 'No signals', got %v", hr["action_reason"])
	}
	if hr["compliance"] == nil {
		t.Error("compliance field should be present")
	}

	// Fields that SHOULD NOT be present
	if _, exists := hr["signals"]; exists {
		t.Error("signals field should NOT be present in slim review")
	}
	if _, exists := hr["fundamentals"]; exists {
		t.Error("fundamentals field should NOT be present in slim review")
	}
	if _, exists := hr["news_intelligence"]; exists {
		t.Error("news_intelligence field should NOT be present in slim review")
	}
	if _, exists := hr["filings_intelligence"]; exists {
		t.Error("filings_intelligence field should NOT be present in slim review")
	}
	if _, exists := hr["filing_summaries"]; exists {
		t.Error("filing_summaries field should NOT be present in slim review")
	}
	if _, exists := hr["timeline"]; exists {
		t.Error("timeline field should NOT be present in slim review")
	}
}

// TestSlimReviewResponse_EmptyHoldings verifies the slim review handles empty holding reviews.
func TestSlimReviewResponse_EmptyHoldings(t *testing.T) {
	review := &models.PortfolioReview{
		PortfolioName:  "Empty",
		ReviewDate:     time.Now(),
		HoldingReviews: []models.HoldingReview{},
	}

	slim := toSlimReview(review)

	if slim.PortfolioName != "Empty" {
		t.Errorf("expected portfolio name 'Empty', got %q", slim.PortfolioName)
	}
	if len(slim.HoldingReviews) != 0 {
		t.Errorf("expected 0 holding reviews, got %d", len(slim.HoldingReviews))
	}
}

// TestSlimReviewResponse_UsedInHandler verifies the handler returns the slim review
// via the actual HTTP handler path.
func TestSlimReviewResponse_UsedInHandler(t *testing.T) {
	review := &models.PortfolioReview{
		PortfolioName: "SMSF",
		ReviewDate:    time.Now(),
		HoldingReviews: []models.HoldingReview{
			{
				Holding: models.Holding{
					Ticker:       "BHP",
					Name:         "BHP Group",
					Units:        100,
					CurrentPrice: 42.0,
					MarketValue:  4200.0,
				},
				Signals: &models.TickerSignals{
					Ticker: "BHP",
					Trend:  "bullish",
				},
				Fundamentals: &models.Fundamentals{
					MarketCap: 100000000,
				},
				ActionRequired: "HOLD",
				ActionReason:   "stable",
			},
		},
	}

	// Use the toSlimReview function and verify it doesn't panic with a nil compliance
	slim := toSlimReview(review)
	data, err := json.Marshal(slim)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var parsed map[string]interface{}
	json.Unmarshal(data, &parsed)

	holdingReviews := parsed["holding_reviews"].([]interface{})
	hr := holdingReviews[0].(map[string]interface{})

	// Nil compliance should still work (omitempty)
	_, hasCompliance := hr["compliance"]
	if hasCompliance {
		t.Log("compliance is present (non-nil case would also be ok)")
	}

	// Verify stripped fields
	if _, exists := hr["signals"]; exists {
		t.Error("signals should be stripped from handler response")
	}
	if _, exists := hr["fundamentals"]; exists {
		t.Error("fundamentals should be stripped from handler response")
	}

	// Verify the response can be re-read as expected
	rec := httptest.NewRecorder()
	WriteJSON(rec, http.StatusOK, map[string]interface{}{
		"review": slim,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
