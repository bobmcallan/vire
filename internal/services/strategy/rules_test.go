package strategy

import (
	"testing"

	"github.com/bobmcallan/vire/internal/models"
)

func TestEvaluateCondition_NumericComparisons(t *testing.T) {
	ctx := RuleContext{
		Signals: &models.TickerSignals{
			Technical: models.TechnicalSignals{RSI: 75.0, VolumeRatio: 2.5},
		},
		Holding: &models.Holding{PortfolioWeightPct: 12.0, NetReturnPct: -5.0},
	}

	tests := []struct {
		name     string
		cond     models.RuleCondition
		expected bool
	}{
		{"RSI > 70", models.RuleCondition{Field: "signals.rsi", Operator: models.RuleOpGT, Value: 70.0}, true},
		{"RSI > 80", models.RuleCondition{Field: "signals.rsi", Operator: models.RuleOpGT, Value: 80.0}, false},
		{"RSI >= 75", models.RuleCondition{Field: "signals.rsi", Operator: models.RuleOpGTE, Value: 75.0}, true},
		{"RSI < 80", models.RuleCondition{Field: "signals.rsi", Operator: models.RuleOpLT, Value: 80.0}, true},
		{"RSI < 70", models.RuleCondition{Field: "signals.rsi", Operator: models.RuleOpLT, Value: 70.0}, false},
		{"RSI <= 75", models.RuleCondition{Field: "signals.rsi", Operator: models.RuleOpLTE, Value: 75.0}, true},
		{"RSI == 75", models.RuleCondition{Field: "signals.rsi", Operator: models.RuleOpEQ, Value: 75.0}, true},
		{"RSI != 75", models.RuleCondition{Field: "signals.rsi", Operator: models.RuleOpNE, Value: 75.0}, false},
		{"Weight > 10", models.RuleCondition{Field: "holding.weight", Operator: models.RuleOpGT, Value: 10.0}, true},
		{"Gain < 0", models.RuleCondition{Field: "holding.gain_loss_pct", Operator: models.RuleOpLT, Value: 0.0}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := EvaluateCondition(tt.cond, ctx)
			if got != tt.expected {
				t.Errorf("EvaluateCondition(%s) = %v, want %v", tt.name, got, tt.expected)
			}
		})
	}
}

func TestEvaluateCondition_StringEquality(t *testing.T) {
	ctx := RuleContext{
		Signals: &models.TickerSignals{
			Trend: models.TrendBearish,
		},
		Fundamentals: &models.Fundamentals{
			Sector:   "Technology",
			Industry: "Software",
		},
	}

	tests := []struct {
		name     string
		cond     models.RuleCondition
		expected bool
	}{
		{"trend == bearish", models.RuleCondition{Field: "signals.trend", Operator: models.RuleOpEQ, Value: "bearish"}, true},
		{"trend == bullish", models.RuleCondition{Field: "signals.trend", Operator: models.RuleOpEQ, Value: "bullish"}, false},
		{"trend != bullish", models.RuleCondition{Field: "signals.trend", Operator: models.RuleOpNE, Value: "bullish"}, true},
		{"sector == Technology", models.RuleCondition{Field: "fundamentals.sector", Operator: models.RuleOpEQ, Value: "Technology"}, true},
		{"sector case insensitive", models.RuleCondition{Field: "fundamentals.sector", Operator: models.RuleOpEQ, Value: "technology"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := EvaluateCondition(tt.cond, ctx)
			if got != tt.expected {
				t.Errorf("EvaluateCondition(%s) = %v, want %v", tt.name, got, tt.expected)
			}
		})
	}
}

func TestEvaluateCondition_InNotIn(t *testing.T) {
	ctx := RuleContext{
		Fundamentals: &models.Fundamentals{Sector: "Technology"},
	}

	tests := []struct {
		name     string
		cond     models.RuleCondition
		expected bool
	}{
		{
			"sector in allowed",
			models.RuleCondition{Field: "fundamentals.sector", Operator: models.RuleOpIn, Value: []string{"Technology", "Healthcare"}},
			true,
		},
		{
			"sector not in allowed",
			models.RuleCondition{Field: "fundamentals.sector", Operator: models.RuleOpIn, Value: []string{"Financials", "Energy"}},
			false,
		},
		{
			"sector not_in excluded",
			models.RuleCondition{Field: "fundamentals.sector", Operator: models.RuleOpNotIn, Value: []string{"Gambling", "Tobacco"}},
			true,
		},
		{
			"sector not_in (is excluded)",
			models.RuleCondition{Field: "fundamentals.sector", Operator: models.RuleOpNotIn, Value: []string{"Technology", "Tobacco"}},
			false,
		},
		{
			"in with []interface{} (JSON-style)",
			models.RuleCondition{Field: "fundamentals.sector", Operator: models.RuleOpIn, Value: []interface{}{"Technology", "Healthcare"}},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := EvaluateCondition(tt.cond, ctx)
			if got != tt.expected {
				t.Errorf("EvaluateCondition(%s) = %v, want %v", tt.name, got, tt.expected)
			}
		})
	}
}

func TestEvaluateCondition_BooleanFields(t *testing.T) {
	ctx := RuleContext{
		Signals: &models.TickerSignals{
			Technical: models.TechnicalSignals{NearSupport: true, NearResistance: false},
		},
	}

	tests := []struct {
		name     string
		cond     models.RuleCondition
		expected bool
	}{
		{"near_support == true", models.RuleCondition{Field: "signals.near_support", Operator: models.RuleOpEQ, Value: true}, true},
		{"near_support == false", models.RuleCondition{Field: "signals.near_support", Operator: models.RuleOpEQ, Value: false}, false},
		{"near_resistance == false", models.RuleCondition{Field: "signals.near_resistance", Operator: models.RuleOpEQ, Value: false}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := EvaluateCondition(tt.cond, ctx)
			if got != tt.expected {
				t.Errorf("EvaluateCondition(%s) = %v, want %v", tt.name, got, tt.expected)
			}
		})
	}
}

func TestEvaluateCondition_NilContext(t *testing.T) {
	// Nil signals/fundamentals/holding should not match
	ctx := RuleContext{}

	tests := []struct {
		name string
		cond models.RuleCondition
	}{
		{"nil signals", models.RuleCondition{Field: "signals.rsi", Operator: models.RuleOpGT, Value: 70.0}},
		{"nil fundamentals", models.RuleCondition{Field: "fundamentals.pe", Operator: models.RuleOpLT, Value: 20.0}},
		{"nil holding", models.RuleCondition{Field: "holding.weight", Operator: models.RuleOpGT, Value: 10.0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := EvaluateCondition(tt.cond, ctx)
			if got {
				t.Errorf("EvaluateCondition with nil context should not match: %s", tt.name)
			}
		})
	}
}

func TestEvaluateRules_PriorityOrdering(t *testing.T) {
	rules := []models.Rule{
		{Name: "low-priority", Conditions: []models.RuleCondition{{Field: "signals.rsi", Operator: models.RuleOpGT, Value: 70.0}}, Action: models.RuleActionWatch, Priority: 1, Enabled: true},
		{Name: "high-priority", Conditions: []models.RuleCondition{{Field: "signals.rsi", Operator: models.RuleOpGT, Value: 70.0}}, Action: models.RuleActionSell, Priority: 5, Enabled: true},
		{Name: "medium-priority", Conditions: []models.RuleCondition{{Field: "signals.rsi", Operator: models.RuleOpGT, Value: 70.0}}, Action: models.RuleActionHold, Priority: 3, Enabled: true},
	}

	ctx := RuleContext{
		Signals: &models.TickerSignals{
			Technical: models.TechnicalSignals{RSI: 75.0},
		},
	}

	results := EvaluateRules(rules, ctx)
	if len(results) != 3 {
		t.Fatalf("Expected 3 results, got %d", len(results))
	}
	if results[0].Rule.Priority != 5 {
		t.Errorf("First result should be priority 5, got %d", results[0].Rule.Priority)
	}
	if results[1].Rule.Priority != 3 {
		t.Errorf("Second result should be priority 3, got %d", results[1].Rule.Priority)
	}
	if results[2].Rule.Priority != 1 {
		t.Errorf("Third result should be priority 1, got %d", results[2].Rule.Priority)
	}
}

func TestEvaluateRules_DisabledSkipped(t *testing.T) {
	rules := []models.Rule{
		{Name: "enabled", Conditions: []models.RuleCondition{{Field: "signals.rsi", Operator: models.RuleOpGT, Value: 70.0}}, Action: models.RuleActionSell, Priority: 1, Enabled: true},
		{Name: "disabled", Conditions: []models.RuleCondition{{Field: "signals.rsi", Operator: models.RuleOpGT, Value: 70.0}}, Action: models.RuleActionSell, Priority: 2, Enabled: false},
	}

	ctx := RuleContext{
		Signals: &models.TickerSignals{Technical: models.TechnicalSignals{RSI: 75.0}},
	}

	results := EvaluateRules(rules, ctx)
	if len(results) != 1 {
		t.Fatalf("Expected 1 result (disabled skipped), got %d", len(results))
	}
	if results[0].Rule.Name != "enabled" {
		t.Errorf("Expected 'enabled' rule, got '%s'", results[0].Rule.Name)
	}
}

func TestEvaluateRules_ANDConditions(t *testing.T) {
	rules := []models.Rule{
		{
			Name: "sell-overweight-overbought",
			Conditions: []models.RuleCondition{
				{Field: "signals.rsi", Operator: models.RuleOpGT, Value: 70.0},
				{Field: "holding.weight", Operator: models.RuleOpGT, Value: 10.0},
			},
			Action:   models.RuleActionSell,
			Priority: 1,
			Enabled:  true,
		},
	}

	// Both conditions met
	ctx := RuleContext{
		Signals: &models.TickerSignals{Technical: models.TechnicalSignals{RSI: 75.0}},
		Holding: &models.Holding{PortfolioWeightPct: 15.0},
	}
	results := EvaluateRules(rules, ctx)
	if len(results) != 1 {
		t.Fatalf("Both conditions met: expected 1 result, got %d", len(results))
	}

	// Only one condition met
	ctx.Holding = &models.Holding{PortfolioWeightPct: 5.0}
	results = EvaluateRules(rules, ctx)
	if len(results) != 0 {
		t.Fatalf("Only one condition met: expected 0 results, got %d", len(results))
	}
}

func TestInterpolateReason(t *testing.T) {
	ctx := RuleContext{
		Signals: &models.TickerSignals{
			Technical: models.TechnicalSignals{RSI: 78.5},
		},
		Holding: &models.Holding{PortfolioWeightPct: 12.3},
	}

	tests := []struct {
		template string
		want     string
	}{
		{"RSI at {signals.rsi} exceeds threshold", "RSI at 78.5 exceeds threshold"},
		{"Weight {holding.weight}% is over limit", "Weight 12.3% is over limit"},
		{"No placeholders here", "No placeholders here"},
		{"{unknown.field} value", "N/A value"},
	}

	for _, tt := range tests {
		t.Run(tt.template, func(t *testing.T) {
			got := interpolateReason(tt.template, ctx)
			if got != tt.want {
				t.Errorf("interpolateReason(%q) = %q, want %q", tt.template, got, tt.want)
			}
		})
	}
}

func TestResolveField_AllPaths(t *testing.T) {
	ctx := RuleContext{
		Signals: &models.TickerSignals{
			Technical: models.TechnicalSignals{
				RSI: 65, VolumeRatio: 1.5, MACD: 0.05, MACDHistogram: 0.02, ATRPct: 3.5,
				NearSupport: true, NearResistance: false,
			},
			Price:  models.PriceSignals{DistanceToSMA20: 2.5, DistanceToSMA50: 5.0, DistanceToSMA200: 15.0},
			PBAS:   models.PBASSignal{Score: 0.7, Interpretation: "underpriced"},
			VLI:    models.VLISignal{Score: 0.6, Interpretation: "accumulating"},
			Regime: models.RegimeSignal{Current: models.RegimeAccumulation},
			Trend:  models.TrendBullish,
		},
		Fundamentals: &models.Fundamentals{
			PE: 15, PB: 2.0, EPS: 1.5, DividendYield: 0.04, Beta: 1.2,
			MarketCap: 5e9, Sector: "Technology", Industry: "Software",
		},
		Holding: &models.Holding{
			PortfolioWeightPct: 8.5, NetReturnPct: 25.0, AnnualizedTotalReturnPct: 30.0,
			AnnualizedCapitalReturnPct: 20.0, TimeWeightedReturnPct: 28.0,
			Units: 500, MarketValue: 50000,
		},
	}

	fields := []string{
		"signals.rsi", "signals.volume_ratio", "signals.macd", "signals.macd_histogram",
		"signals.atr_pct", "signals.near_support", "signals.near_resistance",
		"signals.price.distance_to_sma20", "signals.price.distance_to_sma50", "signals.price.distance_to_sma200",
		"signals.pbas.score", "signals.pbas.interpretation",
		"signals.vli.score", "signals.vli.interpretation",
		"signals.regime.current", "signals.trend",
		"fundamentals.pe", "fundamentals.pb", "fundamentals.eps",
		"fundamentals.dividend_yield", "fundamentals.beta", "fundamentals.market_cap",
		"fundamentals.sector", "fundamentals.industry",
		"holding.weight", "holding.net_return_pct", "holding.net_return_pct_irr",
		"holding.capital_gain_pct", "holding.net_return_pct_twrr",
		"holding.units", "holding.market_value",
	}

	for _, field := range fields {
		t.Run(field, func(t *testing.T) {
			val, ok := resolveField(field, ctx)
			if !ok {
				t.Errorf("resolveField(%q) returned ok=false", field)
			}
			if val == nil {
				t.Errorf("resolveField(%q) returned nil", field)
			}
		})
	}
}

func TestEvaluateRules_ReasonInterpolation(t *testing.T) {
	rules := []models.Rule{
		{
			Name:       "overbought-sell",
			Conditions: []models.RuleCondition{{Field: "signals.rsi", Operator: models.RuleOpGT, Value: 65.0}},
			Action:     models.RuleActionSell,
			Reason:     "RSI overbought at {signals.rsi} (>{signals.rsi} threshold from rule)",
			Priority:   1,
			Enabled:    true,
		},
	}

	ctx := RuleContext{
		Signals: &models.TickerSignals{Technical: models.TechnicalSignals{RSI: 72.0}},
	}

	results := EvaluateRules(rules, ctx)
	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}
	if results[0].Reason != "RSI overbought at 72 (>72 threshold from rule)" {
		t.Errorf("Unexpected reason: %s", results[0].Reason)
	}
}

func TestResolveHoldingField_TWRRAndAliases(t *testing.T) {
	// After refactor: holding fields should support _twrr, _pa, and _irr aliases
	h := &models.Holding{
		NetReturnPct:            25.0,
		AnnualizedTotalReturnPct: 30.0,
		TimeWeightedReturnPct:    28.0,
	}

	tests := []struct {
		field    string
		expected float64
	}{
		// New primary names
		{"net_return_pct", 25.0},
		{"net_return_pct_irr", 30.0},
		{"capital_gain_pct", 20.0},
		{"net_return_pct_twrr", 28.0},
		// Old aliases (backward compat) — gain_loss_pct maps to NetReturnPct
		{"gain_loss_pct", 25.0},
		{"gain_loss_pct_pa", 25.0},
		{"gain_loss_pct_irr", 25.0},
		// total_return_pct aliases map to NetReturnPctIRR
		{"total_return_pct", 30.0},
		{"total_return_pct_pa", 30.0},
		{"total_return_pct_irr", 30.0},
		{"total_return_pct_twrr", 28.0},
		{"capital_gain_pct_pa", 20.0},
		{"capital_gain_pct_irr", 20.0},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			val, ok := resolveHoldingField(tt.field, h)
			if !ok {
				t.Errorf("resolveHoldingField(%q) returned ok=false", tt.field)
				return
			}
			if v, ok := val.(float64); !ok || v != tt.expected {
				t.Errorf("resolveHoldingField(%q) = %v, want %v", tt.field, val, tt.expected)
			}
		})
	}
}

func TestResolveField_TWRRAliasFullPath(t *testing.T) {
	// Test the full "holding.net_return_pct_twrr" path through resolveField
	ctx := RuleContext{
		Holding: &models.Holding{
			TimeWeightedReturnPct: 35.0,
		},
	}

	val, ok := resolveField("holding.net_return_pct_twrr", ctx)
	if !ok {
		t.Fatal("resolveField(holding.net_return_pct_twrr) returned ok=false")
	}
	if v, ok := val.(float64); !ok || v != 35.0 {
		t.Errorf("resolveField(holding.net_return_pct_twrr) = %v, want 35.0", val)
	}
}

func TestEvaluateCondition_TWRRField(t *testing.T) {
	// Test that TWRR field works in rule conditions
	ctx := RuleContext{
		Holding: &models.Holding{
			TimeWeightedReturnPct: 15.0,
		},
	}

	cond := models.RuleCondition{
		Field:    "holding.total_return_pct_twrr",
		Operator: models.RuleOpGT,
		Value:    10.0,
	}

	got, _ := EvaluateCondition(cond, ctx)
	if !got {
		t.Errorf("EvaluateCondition with TWRR > 10 should be true when TWRR=15")
	}
}

func TestEvaluateCondition_NestedSignalFields(t *testing.T) {
	ctx := RuleContext{
		Signals: &models.TickerSignals{
			PBAS:   models.PBASSignal{Score: 0.8, Interpretation: "underpriced"},
			VLI:    models.VLISignal{Score: 0.6, Interpretation: "accumulating"},
			Regime: models.RegimeSignal{Current: models.RegimeTrendUp},
		},
	}

	tests := []struct {
		name     string
		cond     models.RuleCondition
		expected bool
	}{
		{"pbas score > 0.5", models.RuleCondition{Field: "signals.pbas.score", Operator: models.RuleOpGT, Value: 0.5}, true},
		{"pbas interp == underpriced", models.RuleCondition{Field: "signals.pbas.interpretation", Operator: models.RuleOpEQ, Value: "underpriced"}, true},
		{"vli score > 0.5", models.RuleCondition{Field: "signals.vli.score", Operator: models.RuleOpGT, Value: 0.5}, true},
		{"vli interp == accumulating", models.RuleCondition{Field: "signals.vli.interpretation", Operator: models.RuleOpEQ, Value: "accumulating"}, true},
		{"regime == trend_up", models.RuleCondition{Field: "signals.regime.current", Operator: models.RuleOpEQ, Value: "trend_up"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := EvaluateCondition(tt.cond, ctx)
			if got != tt.expected {
				t.Errorf("EvaluateCondition(%s) = %v, want %v", tt.name, got, tt.expected)
			}
		})
	}
}
