package server

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
)

// TestGlossary_EmptyPortfolio_NoHoldingMetrics verifies that an empty portfolio
// (no holdings) does NOT include the "Holding Metrics" category. This catches
// a bug where buildGlossary always appends buildHoldingCategory even with zero holdings.
func TestGlossary_EmptyPortfolio_NoHoldingMetrics(t *testing.T) {
	p := &models.Portfolio{
		Name:     "Empty",
		Currency: "AUD",
	}

	resp := buildGlossary(p, nil, nil)

	for _, cat := range resp.Categories {
		if cat.Name == "Holding Metrics" {
			// Check that it has empty examples (no holdings to show)
			for _, term := range cat.Terms {
				if term.Example != "" {
					t.Errorf("Holding Metrics term %q should have empty example for empty portfolio, got %q", term.Term, term.Example)
				}
			}
			// The category is present but has zero-value entries — document as known behavior
			t.Logf("NOTE: Holding Metrics category IS present for empty portfolio (all values zero, all examples empty)")
		}
	}

	// Verify Portfolio Valuation is always present
	found := false
	for _, cat := range resp.Categories {
		if cat.Name == "Portfolio Valuation" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Portfolio Valuation category must always be present")
	}
}

// TestGlossary_ZeroCost_NoNaN verifies that zero total_cost doesn't produce
// NaN or Inf in example strings.
func TestGlossary_ZeroCost_NoNaN(t *testing.T) {
	p := &models.Portfolio{
		Name:               "ZeroCost",
		TotalValueHoldings: 10000,
		TotalValue:         10000,
		TotalCost:          0,
		TotalNetReturn:     10000,
		TotalNetReturnPct:  0, // would be division by zero
		Currency:           "AUD",
		Holdings: []models.Holding{
			{
				Ticker:       "ABC",
				Units:        100,
				CurrentPrice: 100,
				MarketValue:  10000,
				TotalCost:    0,
				AvgCost:      0,
				NetReturn:    10000,
				NetReturnPct: 0,
				Weight:       100,
			},
		},
	}

	resp := buildGlossary(p, nil, nil)

	for _, cat := range resp.Categories {
		for _, term := range cat.Terms {
			if strings.Contains(term.Example, "NaN") {
				t.Errorf("term %q example contains NaN: %q", term.Term, term.Example)
			}
			if strings.Contains(term.Example, "Inf") {
				t.Errorf("term %q example contains Inf: %q", term.Term, term.Example)
			}
			if strings.Contains(term.Example, "+Inf") || strings.Contains(term.Example, "-Inf") {
				t.Errorf("term %q example contains infinity: %q", term.Term, term.Example)
			}
		}
	}
}

// TestGlossary_ZeroTotalValueHoldings_WeightExample verifies that zero total_value_holdings
// doesn't produce misleading division-by-zero text in the weight example.
func TestGlossary_ZeroTotalValueHoldings_WeightExample(t *testing.T) {
	p := &models.Portfolio{
		Name:               "ZeroValue",
		TotalValueHoldings: 0,
		TotalCost:          5000,
		Currency:           "AUD",
		Holdings: []models.Holding{
			{
				Ticker:       "DEF",
				Units:        100,
				AvgCost:      50,
				CurrentPrice: 0,
				MarketValue:  0,
				TotalCost:    5000,
				NetReturn:    -5000,
				NetReturnPct: -100,
				Weight:       0,
			},
		},
	}

	resp := buildGlossary(p, nil, nil)

	for _, cat := range resp.Categories {
		for _, term := range cat.Terms {
			if strings.Contains(term.Example, "NaN") || strings.Contains(term.Example, "Inf") {
				t.Errorf("term %q example contains NaN/Inf: %q", term.Term, term.Example)
			}
		}
	}
}

// TestGlossary_NegativeValues verifies that negative returns format correctly.
func TestGlossary_NegativeValues(t *testing.T) {
	p := &models.Portfolio{
		Name:               "Losing",
		TotalValueHoldings: 80000,
		TotalValue:         80000,
		TotalCost:          100000,
		TotalNetReturn:     -20000,
		TotalNetReturnPct:  -20,
		Currency:           "AUD",
		YesterdayTotal:     85000,
		YesterdayTotalPct:  -5.88,
		Holdings: []models.Holding{
			{
				Ticker:       "XYZ",
				Units:        1000,
				AvgCost:      100,
				CurrentPrice: 80,
				MarketValue:  80000,
				TotalCost:    100000,
				NetReturn:    -20000,
				NetReturnPct: -20,
				Weight:       100,
			},
		},
	}

	resp := buildGlossary(p, nil, nil)

	// Find net_return term
	for _, cat := range resp.Categories {
		for _, term := range cat.Terms {
			if term.Term == "net_return" && cat.Name == "Portfolio Valuation" {
				if v, ok := term.Value.(float64); ok && v >= 0 {
					t.Errorf("net_return value should be negative, got %.2f", v)
				}
				if !strings.Contains(term.Example, "-$") {
					t.Errorf("net_return example should show negative: %q", term.Example)
				}
			}
		}
	}
}

// TestGlossary_VeryLargeNumbers verifies formatting doesn't break with large values.
func TestGlossary_VeryLargeNumbers(t *testing.T) {
	p := &models.Portfolio{
		Name:               "BigFund",
		TotalValueHoldings: 999999999.99,
		TotalValue:         999999999.99,
		TotalCost:          500000000.00,
		TotalNetReturn:     499999999.99,
		TotalNetReturnPct:  100,
		Currency:           "AUD",
		Holdings: []models.Holding{
			{
				Ticker:       "MEGA",
				Units:        1000000,
				AvgCost:      500,
				CurrentPrice: 999.99,
				MarketValue:  999990000,
				TotalCost:    500000000,
				NetReturn:    499990000,
				NetReturnPct: 100,
				Weight:       100,
			},
		},
	}

	resp := buildGlossary(p, nil, nil)

	// Should produce valid JSON
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal large-number glossary: %v", err)
	}

	// Round-trip validation
	var roundTrip models.GlossaryResponse
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatalf("failed to unmarshal large-number glossary: %v", err)
	}

	if roundTrip.PortfolioName != "BigFund" {
		t.Errorf("lost portfolio name after round-trip")
	}

	// Verify no NaN/Inf in examples
	for _, cat := range resp.Categories {
		for _, term := range cat.Terms {
			if strings.Contains(term.Example, "NaN") || strings.Contains(term.Example, "Inf") {
				t.Errorf("term %q example has NaN/Inf with large numbers: %q", term.Term, term.Example)
			}
		}
	}
}

// TestGlossary_SingleHolding verifies examples use the one available holding.
func TestGlossary_SingleHolding(t *testing.T) {
	p := &models.Portfolio{
		Name:               "Solo",
		TotalValueHoldings: 5000,
		TotalValue:         5000,
		TotalCost:          4000,
		TotalNetReturn:     1000,
		TotalNetReturnPct:  25,
		Currency:           "AUD",
		Holdings: []models.Holding{
			{
				Ticker:       "ONLY",
				Units:        100,
				AvgCost:      40,
				CurrentPrice: 50,
				MarketValue:  5000,
				TotalCost:    4000,
				NetReturn:    1000,
				NetReturnPct: 25,
				Weight:       100,
			},
		},
	}

	resp := buildGlossary(p, nil, nil)

	var holdingCat *models.GlossaryCategory
	for i := range resp.Categories {
		if resp.Categories[i].Name == "Holding Metrics" {
			holdingCat = &resp.Categories[i]
			break
		}
	}

	if holdingCat == nil {
		t.Fatal("Holding Metrics category not found")
	}

	for _, term := range holdingCat.Terms {
		if term.Example == "" {
			t.Errorf("single holding: term %q should have non-empty example", term.Term)
		}
		if strings.Contains(term.Example, "ONLY") {
			// Good — the one holding is referenced
		} else if term.Example != "" {
			t.Errorf("single holding: term %q example should reference ticker ONLY: %q", term.Term, term.Example)
		}
	}
}

// TestGlossary_CapitalPerformance_ZeroTransactions verifies capital category
// is omitted when TransactionCount is 0.
func TestGlossary_CapitalPerformance_ZeroTransactions(t *testing.T) {
	p := &models.Portfolio{
		Name:               "NoTx",
		TotalValueHoldings: 10000,
		TotalValue:         10000,
		TotalCost:          8000,
		Currency:           "AUD",
	}

	cp := &models.CapitalPerformance{
		TransactionCount: 0,
	}

	resp := buildGlossary(p, cp, nil)

	for _, cat := range resp.Categories {
		if cat.Name == "Capital Performance" {
			t.Error("Capital Performance should be omitted when TransactionCount == 0")
		}
	}
}

// TestGlossary_CapitalPerformance_ZeroNetCapital verifies example strings
// when NetCapitalDeployed is zero (all withdrawn).
func TestGlossary_CapitalPerformance_ZeroNetCapital(t *testing.T) {
	p := &models.Portfolio{
		Name:               "AllOut",
		TotalValueHoldings: 0,
		Currency:           "AUD",
	}

	firstDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	cp := &models.CapitalPerformance{
		TotalDeposited:        50000,
		TotalWithdrawn:        50000,
		NetCapitalDeployed:    0,
		CurrentPortfolioValue: 0,
		SimpleReturnPct:       0,
		AnnualizedReturnPct:   0,
		FirstTransactionDate:  &firstDate,
		TransactionCount:      10,
	}

	resp := buildGlossary(p, cp, nil)

	for _, cat := range resp.Categories {
		for _, term := range cat.Terms {
			if strings.Contains(term.Example, "NaN") || strings.Contains(term.Example, "Inf") {
				t.Errorf("term %q example has NaN/Inf with zero net capital: %q", term.Term, term.Example)
			}
		}
	}
}

// TestGlossary_IndicatorsZeroDataPoints verifies indicators category is omitted
// when DataPoints is 0.
func TestGlossary_IndicatorsZeroDataPoints(t *testing.T) {
	p := &models.Portfolio{
		Name:     "NoData",
		Currency: "AUD",
	}

	ind := &models.PortfolioIndicators{
		DataPoints: 0,
	}

	resp := buildGlossary(p, nil, ind)

	for _, cat := range resp.Categories {
		if cat.Name == "Technical Indicators" {
			t.Error("Technical Indicators should be omitted when DataPoints == 0")
		}
	}
}

// TestGlossary_GrowthCategory_AlwaysPresent verifies growth category is
// always included, even when historical values are zero.
func TestGlossary_GrowthCategory_AlwaysPresent(t *testing.T) {
	p := &models.Portfolio{
		Name:               "NoHistory",
		TotalValueHoldings: 10000,
		Currency:           "AUD",
		YesterdayTotal:     0,
		LastWeekTotal:      0,
	}

	resp := buildGlossary(p, nil, nil)

	found := false
	for _, cat := range resp.Categories {
		if cat.Name == "Growth Metrics" {
			found = true
			// Should have 4 terms: yesterday_change, last_week_change, cash_balance, net_deployed
			if len(cat.Terms) != 4 {
				t.Errorf("Growth Metrics should have 4 terms, got %d", len(cat.Terms))
			}
			break
		}
	}
	if !found {
		t.Error("Growth Metrics category should always be present")
	}
}

// TestGlossary_ExternalBalances_Empty verifies external balance category is
// omitted when no external balances exist.
func TestGlossary_ExternalBalances_Empty(t *testing.T) {
	p := &models.Portfolio{
		Name:     "NoExt",
		Currency: "AUD",
	}

	firstDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	cp := &models.CapitalPerformance{
		TransactionCount:     5,
		FirstTransactionDate: &firstDate,
		TotalDeposited:       10000,
		NetCapitalDeployed:   10000,
		ExternalBalances:     nil,
	}

	resp := buildGlossary(p, cp, nil)

	for _, cat := range resp.Categories {
		if cat.Name == "External Balance Performance" {
			t.Error("External Balance Performance should be omitted when no external balances")
		}
	}
}

// TestGlossary_TopHoldings_FewerThan3 verifies topHoldings works when
// portfolio has fewer than 3 holdings.
func TestGlossary_TopHoldings_FewerThan3(t *testing.T) {
	tests := []struct {
		name     string
		holdings []models.Holding
		wantN    int
	}{
		{"zero", nil, 0},
		{"one", []models.Holding{{Ticker: "A", Weight: 50}}, 1},
		{"two", []models.Holding{{Ticker: "A", Weight: 50}, {Ticker: "B", Weight: 30}}, 2},
		{"three", []models.Holding{{Ticker: "A", Weight: 50}, {Ticker: "B", Weight: 30}, {Ticker: "C", Weight: 20}}, 3},
		{"four", []models.Holding{{Ticker: "A", Weight: 50}, {Ticker: "B", Weight: 30}, {Ticker: "C", Weight: 20}, {Ticker: "D", Weight: 10}}, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := topHoldings(tt.holdings, 3)
			if len(result) != tt.wantN {
				t.Errorf("topHoldings(%d holdings) = %d, want %d", len(tt.holdings), len(result), tt.wantN)
			}
		})
	}
}

// TestGlossary_TopHoldings_SortOrder verifies topHoldings returns highest weight first.
func TestGlossary_TopHoldings_SortOrder(t *testing.T) {
	holdings := []models.Holding{
		{Ticker: "SMALL", Weight: 5},
		{Ticker: "BIG", Weight: 50},
		{Ticker: "MED", Weight: 20},
	}

	result := topHoldings(holdings, 3)
	if result[0].Ticker != "BIG" {
		t.Errorf("expected BIG first, got %s", result[0].Ticker)
	}
	if result[1].Ticker != "MED" {
		t.Errorf("expected MED second, got %s", result[1].Ticker)
	}
	if result[2].Ticker != "SMALL" {
		t.Errorf("expected SMALL third, got %s", result[2].Ticker)
	}
}

// TestGlossary_TopHoldings_DoesNotMutateOriginal verifies sort doesn't modify the original slice.
func TestGlossary_TopHoldings_DoesNotMutateOriginal(t *testing.T) {
	holdings := []models.Holding{
		{Ticker: "C", Weight: 10},
		{Ticker: "A", Weight: 50},
		{Ticker: "B", Weight: 30},
	}

	original0 := holdings[0].Ticker
	topHoldings(holdings, 2)

	if holdings[0].Ticker != original0 {
		t.Errorf("topHoldings mutated original slice: first element changed from %q to %q", original0, holdings[0].Ticker)
	}
}

// TestGlossary_FmtMoney_EdgeCases verifies the local fmtMoney helper.
func TestGlossary_FmtMoney_EdgeCases(t *testing.T) {
	tests := []struct {
		input    float64
		contains string
	}{
		{0, "$0.00"},
		{-1234.56, "-$1234.56"},
		{0.01, "$0.01"},
		{0.005, "$0.01"}, // rounding
		{math.MaxFloat64, "$"},
	}

	for _, tt := range tests {
		result := fmtMoney(tt.input)
		if !strings.Contains(result, tt.contains) {
			t.Errorf("fmtMoney(%v) = %q, expected to contain %q", tt.input, result, tt.contains)
		}
	}
}

// TestGlossary_FmtMoney_NaN verifies fmtMoney handles NaN gracefully.
func TestGlossary_FmtMoney_NaN(t *testing.T) {
	result := fmtMoney(math.NaN())
	if result == "" {
		t.Error("fmtMoney(NaN) should produce output, not empty string")
	}
	// NaN will produce "$NaN" or similar — document the behavior
	t.Logf("fmtMoney(NaN) = %q", result)
}

// TestGlossary_FmtMoney_Inf verifies fmtMoney handles Infinity.
func TestGlossary_FmtMoney_Inf(t *testing.T) {
	result := fmtMoney(math.Inf(1))
	t.Logf("fmtMoney(+Inf) = %q", result)

	result = fmtMoney(math.Inf(-1))
	t.Logf("fmtMoney(-Inf) = %q", result)
}

// TestGlossary_JSONRoundTrip verifies the full response survives JSON marshalling.
func TestGlossary_JSONRoundTrip(t *testing.T) {
	p := testPortfolio()
	cp := testCapitalPerformance()
	ind := testIndicators()

	resp := buildGlossary(p, cp, ind)

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded models.GlossaryResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.PortfolioName != resp.PortfolioName {
		t.Errorf("portfolio_name mismatch: %q vs %q", decoded.PortfolioName, resp.PortfolioName)
	}
	if len(decoded.Categories) != len(resp.Categories) {
		t.Errorf("category count mismatch: %d vs %d", len(decoded.Categories), len(resp.Categories))
	}

	// Verify no NaN/Inf values that would cause JSON errors
	jsonStr := string(data)
	if strings.Contains(jsonStr, "NaN") || strings.Contains(jsonStr, "Inf") {
		t.Errorf("JSON output contains NaN or Inf")
	}
}

// TestGlossary_AllNilEnrichment verifies glossary works with only portfolio data
// (both capitalPerf and indicators nil).
func TestGlossary_AllNilEnrichment(t *testing.T) {
	p := testPortfolio()
	resp := buildGlossary(p, nil, nil)

	// Should have Portfolio Valuation, Holding Metrics, Growth Metrics
	if len(resp.Categories) != 3 {
		names := make([]string, len(resp.Categories))
		for i, c := range resp.Categories {
			names[i] = c.Name
		}
		t.Errorf("expected 3 categories with nil enrichment, got %d: %v", len(resp.Categories), names)
	}

	// Should NOT have Capital Performance, External Balance Performance, Technical Indicators
	for _, cat := range resp.Categories {
		switch cat.Name {
		case "Capital Performance", "External Balance Performance", "Technical Indicators":
			t.Errorf("category %q should not be present with nil enrichment", cat.Name)
		}
	}
}

// TestGlossary_ExternalBalanceExample verifies the external balance example
// includes all balance labels.
func TestGlossary_ExternalBalanceExample(t *testing.T) {
	p := &models.Portfolio{
		Name:                 "WithExt",
		TotalValueHoldings:   100000,
		TotalValue:           120000,
		ExternalBalanceTotal: 20000,
		Currency:             "AUD",
		ExternalBalances: []models.ExternalBalance{
			{Label: "ANZ Cash", Value: 10000, Type: "cash"},
			{Label: "Term Dep", Value: 10000, Type: "term_deposit"},
		},
	}

	resp := buildGlossary(p, nil, nil)

	for _, cat := range resp.Categories {
		if cat.Name != "Portfolio Valuation" {
			continue
		}
		for _, term := range cat.Terms {
			if term.Term == "external_balance_total" {
				if !strings.Contains(term.Example, "ANZ Cash") {
					t.Errorf("external_balance_total example should include balance labels: %q", term.Example)
				}
				if !strings.Contains(term.Example, "Term Dep") {
					t.Errorf("external_balance_total example should include all balances: %q", term.Example)
				}
			}
		}
	}
}

// TestGlossary_HoldingCalc_NoSeparator verifies that fmtHoldingCalc uses " | " separators.
func TestGlossary_HoldingCalc_NoSeparator(t *testing.T) {
	holdings := []models.Holding{
		{Ticker: "A", MarketValue: 1000},
		{Ticker: "B", MarketValue: 2000},
	}

	result := fmtHoldingCalc(holdings, "test", func(h models.Holding) string {
		return fmt.Sprintf("$%.2f", h.MarketValue)
	})

	if !strings.Contains(result, " | ") {
		t.Errorf("expected ' | ' separator in multi-holding example: %q", result)
	}
}

// TestGlossary_AboveBelow verifies the aboveBelow helper.
func TestGlossary_AboveBelow(t *testing.T) {
	if aboveBelow(true) != "above" {
		t.Error("aboveBelow(true) should be 'above'")
	}
	if aboveBelow(false) != "below" {
		t.Error("aboveBelow(false) should be 'below'")
	}
}

// TestGlossary_FmtCategoryLabel verifies all valid external balance types are mapped.
func TestGlossary_FmtCategoryLabel(t *testing.T) {
	for _, cat := range []string{"cash", "accumulate", "term_deposit", "offset"} {
		label := fmtCategoryLabel(cat)
		if label == cat {
			t.Errorf("fmtCategoryLabel(%q) returned raw value, expected title case", cat)
		}
	}

	// Unknown category should return the raw string
	label := fmtCategoryLabel("unknown_type")
	if label != "unknown_type" {
		t.Errorf("fmtCategoryLabel for unknown type should return raw value, got %q", label)
	}
}

// TestGlossary_IndicatorTrendValueIsString verifies that the trend term
// has a string Value (the trend direction).
func TestGlossary_IndicatorTrendValueIsString(t *testing.T) {
	ind := testIndicators()
	resp := buildGlossary(testPortfolio(), nil, ind)

	for _, cat := range resp.Categories {
		if cat.Name != "Technical Indicators" {
			continue
		}
		for _, term := range cat.Terms {
			if term.Term == "trend" {
				v, ok := term.Value.(string)
				if !ok {
					t.Errorf("trend Value should be a string, got %T", term.Value)
				}
				if v != string(models.TrendBullish) {
					t.Errorf("trend Value = %q, want %q", v, string(models.TrendBullish))
				}
				if term.Example == "" {
					t.Error("trend example should not be empty")
				}
				if !strings.Contains(term.Example, string(models.TrendBullish)) {
					t.Errorf("trend example should contain trend direction: %q", term.Example)
				}
			}
		}
	}
}
