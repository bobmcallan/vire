package server

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
)

// ============================================================================
// Devils-advocate stress tests for slim portfolio review response.
// Validates data leakage prevention, nil handling, and JSON serialization
// correctness of the toSlimReview conversion.
// ============================================================================

// ============================================================================
// 1. Data leakage — heavy fields must not appear in JSON output
// ============================================================================

func TestSlimReview_NoSignalsInJSON(t *testing.T) {
	review := &models.PortfolioReview{
		PortfolioName: "SMSF",
		ReviewDate:    time.Now(),
		HoldingReviews: []models.HoldingReview{
			{
				Holding: models.Holding{Ticker: "BHP", Exchange: "ASX"},
				Signals: &models.TickerSignals{
					Trend:            "bullish",
					TrendDescription: "Strong uptrend",
					Price: models.PriceSignals{
						Current: 42.0,
						SMA20:   40.0,
						SMA50:   38.0,
						SMA200:  35.0,
					},
				},
				ActionRequired: "HOLD",
			},
		},
	}

	slim := toSlimReview(review)
	data, err := json.Marshal(slim)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	jsonStr := string(data)
	forbiddenFields := []string{
		`"signals"`,
		`"trend"`,
		`"trend_description"`,
		`"sma20"`,
		`"sma50"`,
		`"sma200"`,
	}
	for _, field := range forbiddenFields {
		if contains(jsonStr, field) {
			t.Errorf("DATA LEAKAGE: slim review JSON contains %s — signals data leaked through", field)
		}
	}
}

func TestSlimReview_NoFundamentalsInJSON(t *testing.T) {
	review := &models.PortfolioReview{
		PortfolioName: "SMSF",
		ReviewDate:    time.Now(),
		HoldingReviews: []models.HoldingReview{
			{
				Holding: models.Holding{Ticker: "BHP", Exchange: "ASX"},
				Fundamentals: &models.Fundamentals{
					Sector:      "Materials",
					Industry:    "Mining",
					PE:          15.5,
					MarketCap:   180000000000,
					Description: "BHP Group is a mining company",
				},
				ActionRequired: "HOLD",
			},
		},
	}

	slim := toSlimReview(review)
	data, err := json.Marshal(slim)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	jsonStr := string(data)
	forbiddenFields := []string{
		`"fundamentals"`,
		`"sector"`,
		`"industry"`,
		`"market_cap"`,
		`"description"`,
		`"pe"`,
	}
	for _, field := range forbiddenFields {
		if contains(jsonStr, field) {
			t.Errorf("DATA LEAKAGE: slim review JSON contains %s — fundamentals data leaked through", field)
		}
	}
}

func TestSlimReview_NoNewsIntelligenceInJSON(t *testing.T) {
	review := &models.PortfolioReview{
		PortfolioName: "SMSF",
		ReviewDate:    time.Now(),
		HoldingReviews: []models.HoldingReview{
			{
				Holding: models.Holding{Ticker: "BHP", Exchange: "ASX"},
				NewsIntelligence: &models.NewsIntelligence{
					OverallSentiment: "positive",
					Summary:          "BHP has strong earnings",
					KeyThemes:        []string{"iron ore prices", "dividend"},
				},
				ActionRequired: "HOLD",
			},
		},
	}

	slim := toSlimReview(review)
	data, err := json.Marshal(slim)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	jsonStr := string(data)
	forbiddenFields := []string{
		`"news_intelligence"`,
		`"overall_sentiment"`,
		`"key_themes"`,
	}
	for _, field := range forbiddenFields {
		if contains(jsonStr, field) {
			t.Errorf("DATA LEAKAGE: slim review JSON contains %s — news intelligence data leaked through", field)
		}
	}
}

func TestSlimReview_NoFilingsIntelligenceInJSON(t *testing.T) {
	review := &models.PortfolioReview{
		PortfolioName: "SMSF",
		ReviewDate:    time.Now(),
		HoldingReviews: []models.HoldingReview{
			{
				Holding: models.Holding{Ticker: "BHP", Exchange: "ASX"},
				FilingsIntelligence: &models.FilingsIntelligence{
					FinancialHealth: "strong",
					GrowthOutlook:   "positive",
					Summary:         "Filings show strong performance",
				},
				ActionRequired: "HOLD",
			},
		},
	}

	slim := toSlimReview(review)
	data, err := json.Marshal(slim)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	jsonStr := string(data)
	forbiddenFields := []string{
		`"filings_intelligence"`,
		`"financial_health"`,
		`"growth_outlook"`,
	}
	for _, field := range forbiddenFields {
		if contains(jsonStr, field) {
			t.Errorf("DATA LEAKAGE: slim review JSON contains %s — filings intelligence data leaked through", field)
		}
	}
}

func TestSlimReview_NoFilingSummariesInJSON(t *testing.T) {
	review := &models.PortfolioReview{
		PortfolioName: "SMSF",
		ReviewDate:    time.Now(),
		HoldingReviews: []models.HoldingReview{
			{
				Holding: models.Holding{Ticker: "BHP", Exchange: "ASX"},
				FilingSummaries: []models.FilingSummary{
					{
						Headline: "BHP H1 Results",
						Type:     "financial_results",
						Revenue:  "$25B",
					},
				},
				ActionRequired: "HOLD",
			},
		},
	}

	slim := toSlimReview(review)
	data, err := json.Marshal(slim)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	jsonStr := string(data)
	forbiddenFields := []string{
		`"filing_summaries"`,
		`"headline"`,
		`"financial_results"`,
	}
	for _, field := range forbiddenFields {
		if contains(jsonStr, field) {
			t.Errorf("DATA LEAKAGE: slim review JSON contains %s — filing summaries data leaked through", field)
		}
	}
}

func TestSlimReview_NoTimelineInJSON(t *testing.T) {
	review := &models.PortfolioReview{
		PortfolioName: "SMSF",
		ReviewDate:    time.Now(),
		HoldingReviews: []models.HoldingReview{
			{
				Holding: models.Holding{Ticker: "BHP", Exchange: "ASX"},
				Timeline: &models.CompanyTimeline{
					BusinessModel: "Diversified mining",
					GeneratedAt:   time.Now(),
				},
				ActionRequired: "HOLD",
			},
		},
	}

	slim := toSlimReview(review)
	data, err := json.Marshal(slim)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	jsonStr := string(data)
	forbiddenFields := []string{
		`"timeline"`,
		`"business_model"`,
	}
	for _, field := range forbiddenFields {
		if contains(jsonStr, field) {
			t.Errorf("DATA LEAKAGE: slim review JSON contains %s — timeline data leaked through", field)
		}
	}
}

// ============================================================================
// 2. Kept fields — verify all required fields are present
// ============================================================================

func TestSlimReview_KeptFieldsPresent(t *testing.T) {
	review := &models.PortfolioReview{
		PortfolioName:   "SMSF",
		ReviewDate:      time.Now(),
		TotalValue:      100000,
		TotalCost:       80000,
		TotalGain:       20000,
		TotalGainPct:    25.0,
		DayChange:       500,
		DayChangePct:    0.5,
		FXRate:          0.65,
		Summary:         "Portfolio looks strong",
		Recommendations: []string{"Consider rebalancing"},
		Alerts: []models.Alert{
			{Type: models.AlertTypePrice, Severity: "high", Ticker: "BHP", Message: "Price spike"},
		},
		PortfolioBalance: &models.PortfolioBalance{
			ConcentrationRisk: "medium",
		},
		HoldingReviews: []models.HoldingReview{
			{
				Holding:        models.Holding{Ticker: "BHP", Exchange: "ASX", Units: 100, CurrentPrice: 42.0, MarketValue: 4200},
				OvernightMove:  1.5,
				OvernightPct:   3.7,
				NewsImpact:     "positive",
				ActionRequired: "HOLD",
				ActionReason:   "No action needed",
				Compliance: &models.ComplianceResult{
					Status:  models.ComplianceStatusCompliant,
					Reasons: []string{"within limits"},
				},
			},
		},
	}

	slim := toSlimReview(review)
	data, err := json.Marshal(slim)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	// Unmarshal into generic map to check field presence
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	requiredTopLevel := []string{
		"portfolio_name", "review_date", "total_value", "total_cost",
		"total_gain", "total_gain_pct", "day_change", "day_change_pct",
		"fx_rate", "holding_reviews", "alerts", "summary",
		"recommendations", "portfolio_balance",
	}
	for _, field := range requiredTopLevel {
		if _, ok := result[field]; !ok {
			t.Errorf("MISSING required field %q in slim review response", field)
		}
	}

	// Check holding reviews
	reviews, ok := result["holding_reviews"].([]interface{})
	if !ok || len(reviews) == 0 {
		t.Fatal("holding_reviews is missing or empty")
	}
	hr, ok := reviews[0].(map[string]interface{})
	if !ok {
		t.Fatal("first holding review is not an object")
	}

	requiredHolding := []string{
		"holding", "overnight_move", "overnight_pct", "news_impact",
		"action_required", "action_reason", "compliance",
	}
	for _, field := range requiredHolding {
		if _, ok := hr[field]; !ok {
			t.Errorf("MISSING required field %q in slim holding review", field)
		}
	}
}

// ============================================================================
// 3. Nil handling — nil signals, fundamentals, compliance, empty holdings
// ============================================================================

func TestSlimReview_NilSignals_NilFundamentals(t *testing.T) {
	review := &models.PortfolioReview{
		PortfolioName: "SMSF",
		ReviewDate:    time.Now(),
		HoldingReviews: []models.HoldingReview{
			{
				Holding:        models.Holding{Ticker: "BHP", Exchange: "ASX"},
				Signals:        nil,
				Fundamentals:   nil,
				ActionRequired: "HOLD",
			},
		},
	}

	slim := toSlimReview(review)
	data, err := json.Marshal(slim)
	if err != nil {
		t.Fatalf("marshal with nil signals/fundamentals should not error: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	reviews := result["holding_reviews"].([]interface{})
	hr := reviews[0].(map[string]interface{})

	if _, ok := hr["signals"]; ok {
		t.Error("signals should not appear in slim review (it's not a field)")
	}
	if _, ok := hr["fundamentals"]; ok {
		t.Error("fundamentals should not appear in slim review (it's not a field)")
	}
}

func TestSlimReview_NilCompliance(t *testing.T) {
	review := &models.PortfolioReview{
		PortfolioName: "SMSF",
		ReviewDate:    time.Now(),
		HoldingReviews: []models.HoldingReview{
			{
				Holding:        models.Holding{Ticker: "BHP", Exchange: "ASX"},
				Compliance:     nil,
				ActionRequired: "HOLD",
			},
		},
	}

	slim := toSlimReview(review)
	data, err := json.Marshal(slim)
	if err != nil {
		t.Fatalf("marshal with nil compliance should not error: %v", err)
	}

	var result map[string]interface{}
	json.Unmarshal(data, &result)
	reviews := result["holding_reviews"].([]interface{})
	hr := reviews[0].(map[string]interface{})

	// compliance has omitempty, so nil should be absent
	if _, ok := hr["compliance"]; ok {
		t.Error("nil compliance should be omitted from JSON (has omitempty tag)")
	}
}

func TestSlimReview_EmptyHoldings(t *testing.T) {
	review := &models.PortfolioReview{
		PortfolioName:  "SMSF",
		ReviewDate:     time.Now(),
		HoldingReviews: []models.HoldingReview{},
	}

	slim := toSlimReview(review)
	data, err := json.Marshal(slim)
	if err != nil {
		t.Fatalf("marshal with empty holdings should not error: %v", err)
	}

	var result map[string]interface{}
	json.Unmarshal(data, &result)
	reviews, ok := result["holding_reviews"].([]interface{})
	if !ok {
		t.Fatal("holding_reviews should be an array, not null")
	}
	if len(reviews) != 0 {
		t.Errorf("expected 0 holding reviews, got %d", len(reviews))
	}
}

func TestSlimReview_NilAlerts_NilRecommendations(t *testing.T) {
	review := &models.PortfolioReview{
		PortfolioName:   "SMSF",
		ReviewDate:      time.Now(),
		Alerts:          nil,
		Recommendations: nil,
		HoldingReviews:  []models.HoldingReview{},
	}

	slim := toSlimReview(review)
	data, err := json.Marshal(slim)
	if err != nil {
		t.Fatalf("marshal with nil alerts/recommendations should not error: %v", err)
	}

	// Should not panic during serialization
	if len(data) == 0 {
		t.Error("marshalled JSON should not be empty")
	}
}

func TestSlimReview_NilPortfolioBalance(t *testing.T) {
	review := &models.PortfolioReview{
		PortfolioName:    "SMSF",
		ReviewDate:       time.Now(),
		PortfolioBalance: nil,
		HoldingReviews:   []models.HoldingReview{},
	}

	slim := toSlimReview(review)
	data, err := json.Marshal(slim)
	if err != nil {
		t.Fatalf("marshal with nil balance should not error: %v", err)
	}

	var result map[string]interface{}
	json.Unmarshal(data, &result)

	// portfolio_balance has omitempty, so nil should be absent
	if _, ok := result["portfolio_balance"]; ok {
		t.Error("nil portfolio_balance should be omitted from JSON (has omitempty tag)")
	}
}

// ============================================================================
// 4. JSON serialization — omitempty correctness
// ============================================================================

func TestSlimReview_OmitEmpty_NewsImpact(t *testing.T) {
	review := &models.PortfolioReview{
		PortfolioName: "SMSF",
		ReviewDate:    time.Now(),
		HoldingReviews: []models.HoldingReview{
			{
				Holding:        models.Holding{Ticker: "BHP", Exchange: "ASX"},
				NewsImpact:     "", // empty — should be omitted
				ActionRequired: "HOLD",
			},
		},
	}

	slim := toSlimReview(review)
	data, _ := json.Marshal(slim)

	var result map[string]interface{}
	json.Unmarshal(data, &result)
	reviews := result["holding_reviews"].([]interface{})
	hr := reviews[0].(map[string]interface{})

	if _, ok := hr["news_impact"]; ok {
		t.Error("empty news_impact should be omitted (has omitempty tag)")
	}
}

func TestSlimReview_OmitEmpty_FXRate_Zero(t *testing.T) {
	review := &models.PortfolioReview{
		PortfolioName:  "SMSF",
		ReviewDate:     time.Now(),
		FXRate:         0.0, // zero — should be omitted
		HoldingReviews: []models.HoldingReview{},
	}

	slim := toSlimReview(review)
	data, _ := json.Marshal(slim)

	var result map[string]interface{}
	json.Unmarshal(data, &result)

	if _, ok := result["fx_rate"]; ok {
		t.Error("zero fx_rate should be omitted (has omitempty tag)")
	}
}

// ============================================================================
// 5. All-fields-populated test — the "fat" holding review
// ============================================================================

func TestSlimReview_AllFieldsPopulated_StillStripsHeavyData(t *testing.T) {
	review := &models.PortfolioReview{
		PortfolioName:   "SMSF",
		ReviewDate:      time.Now(),
		TotalValue:      500000,
		TotalCost:       400000,
		TotalGain:       100000,
		TotalGainPct:    25.0,
		DayChange:       1000,
		DayChangePct:    0.2,
		FXRate:          0.65,
		Summary:         "Strong performance",
		Recommendations: []string{"Rebalance", "Add income stocks"},
		Alerts: []models.Alert{
			{Type: models.AlertTypeSignal, Severity: "high", Ticker: "BHP", Message: "RSI oversold"},
		},
		PortfolioBalance: &models.PortfolioBalance{
			ConcentrationRisk: "low",
			DefensiveWeight:   40.0,
			GrowthWeight:      50.0,
			IncomeWeight:      10.0,
		},
		HoldingReviews: []models.HoldingReview{
			{
				Holding: models.Holding{
					Ticker:       "BHP",
					Exchange:     "ASX",
					Name:         "BHP Group",
					Units:        200,
					AvgCost:      35.0,
					CurrentPrice: 45.0,
					MarketValue:  9000,
				},
				Signals: &models.TickerSignals{
					Trend: "bullish",
					Price: models.PriceSignals{Current: 45.0, SMA20: 43.0},
				},
				Fundamentals: &models.Fundamentals{
					Sector:    "Materials",
					MarketCap: 180000000000,
					PE:        12.5,
				},
				OvernightMove: 2.3,
				OvernightPct:  5.1,
				NewsImpact:    "positive",
				NewsIntelligence: &models.NewsIntelligence{
					OverallSentiment: "bullish",
					Summary:          "Strong iron ore demand",
				},
				FilingsIntelligence: &models.FilingsIntelligence{
					FinancialHealth: "excellent",
				},
				FilingSummaries: []models.FilingSummary{
					{Headline: "H1 Results", Type: "financial_results"},
				},
				Timeline: &models.CompanyTimeline{
					BusinessModel: "Mining",
				},
				ActionRequired: "HOLD",
				ActionReason:   "All good",
				Compliance: &models.ComplianceResult{
					Status: models.ComplianceStatusCompliant,
				},
			},
		},
	}

	slim := toSlimReview(review)
	data, err := json.Marshal(slim)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	jsonStr := string(data)

	// Heavy fields must NOT be present
	heavyFields := []string{
		`"signals"`, `"fundamentals"`, `"news_intelligence"`,
		`"filings_intelligence"`, `"filing_summaries"`, `"timeline"`,
		`"trend"`, `"market_cap"`, `"overall_sentiment"`,
		`"financial_health"`, `"business_model"`,
	}
	for _, field := range heavyFields {
		if contains(jsonStr, field) {
			t.Errorf("DATA LEAKAGE: fully populated review still leaks %s", field)
		}
	}

	// Light fields MUST be present
	lightFields := []string{
		`"holding"`, `"overnight_move"`, `"overnight_pct"`,
		`"news_impact"`, `"action_required"`, `"action_reason"`,
		`"compliance"`, `"portfolio_name"`, `"total_value"`,
	}
	for _, field := range lightFields {
		if !contains(jsonStr, field) {
			t.Errorf("MISSING required field %s in slim review", field)
		}
	}
}

// ============================================================================
// 6. Multiple holdings — verify all are converted
// ============================================================================

func TestSlimReview_MultipleHoldings(t *testing.T) {
	review := &models.PortfolioReview{
		PortfolioName: "SMSF",
		ReviewDate:    time.Now(),
		HoldingReviews: []models.HoldingReview{
			{
				Holding:        models.Holding{Ticker: "BHP", Exchange: "ASX"},
				Signals:        &models.TickerSignals{Trend: "bullish"},
				ActionRequired: "HOLD",
			},
			{
				Holding:        models.Holding{Ticker: "CBA", Exchange: "ASX"},
				Fundamentals:   &models.Fundamentals{PE: 18.0},
				ActionRequired: "BUY",
			},
			{
				Holding:        models.Holding{Ticker: "VAS", Exchange: "ASX"},
				ActionRequired: "WATCH",
			},
		},
	}

	slim := toSlimReview(review)

	if len(slim.HoldingReviews) != 3 {
		t.Fatalf("expected 3 slim holding reviews, got %d", len(slim.HoldingReviews))
	}

	tickers := map[string]string{
		"BHP": "HOLD",
		"CBA": "BUY",
		"VAS": "WATCH",
	}

	for _, hr := range slim.HoldingReviews {
		expected, ok := tickers[hr.Holding.Ticker]
		if !ok {
			t.Errorf("unexpected ticker %s in slim reviews", hr.Holding.Ticker)
			continue
		}
		if hr.ActionRequired != expected {
			t.Errorf("ticker %s: expected action %s, got %s", hr.Holding.Ticker, expected, hr.ActionRequired)
		}
	}

	// Verify no heavy data leaked
	data, _ := json.Marshal(slim)
	jsonStr := string(data)
	if contains(jsonStr, `"signals"`) || contains(jsonStr, `"fundamentals"`) {
		t.Error("DATA LEAKAGE: heavy fields leaked in multi-holding response")
	}
}

// ============================================================================
// 7. Hostile input — very large holding reviews
// ============================================================================

func TestSlimReview_LargeHoldingList(t *testing.T) {
	holdings := make([]models.HoldingReview, 500)
	for i := range holdings {
		holdings[i] = models.HoldingReview{
			Holding: models.Holding{
				Ticker:   "T" + string(rune('A'+i%26)),
				Exchange: "ASX",
			},
			Signals:      &models.TickerSignals{Trend: "neutral"},
			Fundamentals: &models.Fundamentals{PE: float64(i)},
			NewsIntelligence: &models.NewsIntelligence{
				Summary: "Some news",
			},
			ActionRequired: "HOLD",
		}
	}

	review := &models.PortfolioReview{
		PortfolioName:  "HUGE",
		ReviewDate:     time.Now(),
		HoldingReviews: holdings,
	}

	slim := toSlimReview(review)

	if len(slim.HoldingReviews) != 500 {
		t.Fatalf("expected 500 slim holdings, got %d", len(slim.HoldingReviews))
	}

	// Marshal should succeed without OOM
	data, err := json.Marshal(slim)
	if err != nil {
		t.Fatalf("failed to marshal large slim review: %v", err)
	}

	// The slim JSON should be significantly smaller than the full review
	fullData, _ := json.Marshal(review)
	if len(data) >= len(fullData) {
		t.Errorf("slim review (%d bytes) should be smaller than full review (%d bytes)",
			len(data), len(fullData))
	}
}

// contains helper is defined in middleware_test.go (same package)
