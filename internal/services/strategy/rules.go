// Package strategy provides portfolio strategy management services
package strategy

import (
	"fmt"
	"sort"
	"strings"

	"github.com/bobmcallan/vire/internal/models"
)

// RuleContext provides live data for rule evaluation
type RuleContext struct {
	Holding      *models.Holding
	Signals      *models.TickerSignals
	Fundamentals *models.Fundamentals
}

// RuleResult captures the outcome of evaluating a single rule
type RuleResult struct {
	Rule    *models.Rule
	Matched bool
	Reason  string // interpolated with actual values
}

// EvaluateRules evaluates all enabled rules against the context.
// Returns matched results sorted by priority descending (highest priority first).
func EvaluateRules(rules []models.Rule, ctx RuleContext) []RuleResult {
	var results []RuleResult

	for i := range rules {
		r := &rules[i]
		if !r.Enabled {
			continue
		}

		matched := true
		for _, cond := range r.Conditions {
			ok, _ := EvaluateCondition(cond, ctx)
			if !ok {
				matched = false
				break
			}
		}

		if matched {
			results = append(results, RuleResult{
				Rule:    r,
				Matched: true,
				Reason:  interpolateReason(r.Reason, ctx),
			})
		}
	}

	// Sort by priority descending
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].Rule.Priority > results[j].Rule.Priority
	})

	return results
}

// EvaluateCondition evaluates a single condition against the context.
// Returns (matched, actualValue).
func EvaluateCondition(cond models.RuleCondition, ctx RuleContext) (bool, interface{}) {
	actual, ok := resolveField(cond.Field, ctx)
	if !ok {
		return false, nil
	}
	return compareValues(actual, cond.Operator, cond.Value), actual
}

// resolveField looks up a dot-path field from the rule context.
// Uses explicit switches — no reflection.
func resolveField(field string, ctx RuleContext) (interface{}, bool) {
	parts := strings.SplitN(field, ".", 2)
	if len(parts) < 2 {
		return nil, false
	}

	prefix := parts[0]
	rest := parts[1]

	switch prefix {
	case "signals":
		return resolveSignalField(rest, ctx.Signals)
	case "fundamentals":
		return resolveFundamentalsField(rest, ctx.Fundamentals)
	case "holding":
		return resolveHoldingField(rest, ctx.Holding)
	}
	return nil, false
}

func resolveSignalField(field string, sig *models.TickerSignals) (interface{}, bool) {
	if sig == nil {
		return nil, false
	}

	// Handle nested prefixes
	parts := strings.SplitN(field, ".", 2)

	switch parts[0] {
	case "rsi":
		return sig.Technical.RSI, true
	case "volume_ratio":
		return sig.Technical.VolumeRatio, true
	case "macd":
		return sig.Technical.MACD, true
	case "macd_histogram":
		return sig.Technical.MACDHistogram, true
	case "atr_pct":
		return sig.Technical.ATRPct, true
	case "near_support":
		return sig.Technical.NearSupport, true
	case "near_resistance":
		return sig.Technical.NearResistance, true
	case "trend":
		return string(sig.Trend), true
	case "price":
		if len(parts) < 2 {
			return nil, false
		}
		return resolvePriceField(parts[1], &sig.Price)
	case "pbas":
		if len(parts) < 2 {
			return nil, false
		}
		return resolvePBASField(parts[1], &sig.PBAS)
	case "vli":
		if len(parts) < 2 {
			return nil, false
		}
		return resolveVLIField(parts[1], &sig.VLI)
	case "regime":
		if len(parts) < 2 {
			return nil, false
		}
		return resolveRegimeField(parts[1], &sig.Regime)
	}
	return nil, false
}

func resolvePriceField(field string, price *models.PriceSignals) (interface{}, bool) {
	switch field {
	case "distance_to_sma20":
		return price.DistanceToSMA20, true
	case "distance_to_sma50":
		return price.DistanceToSMA50, true
	case "distance_to_sma200":
		return price.DistanceToSMA200, true
	}
	return nil, false
}

func resolvePBASField(field string, pbas *models.PBASSignal) (interface{}, bool) {
	switch field {
	case "score":
		return pbas.Score, true
	case "interpretation":
		return pbas.Interpretation, true
	}
	return nil, false
}

func resolveVLIField(field string, vli *models.VLISignal) (interface{}, bool) {
	switch field {
	case "score":
		return vli.Score, true
	case "interpretation":
		return vli.Interpretation, true
	}
	return nil, false
}

func resolveRegimeField(field string, regime *models.RegimeSignal) (interface{}, bool) {
	switch field {
	case "current":
		return string(regime.Current), true
	}
	return nil, false
}

func resolveFundamentalsField(field string, f *models.Fundamentals) (interface{}, bool) {
	if f == nil {
		return nil, false
	}
	switch field {
	case "pe":
		return f.PE, true
	case "pb":
		return f.PB, true
	case "eps":
		return f.EPS, true
	case "dividend_yield":
		return f.DividendYield, true
	case "beta":
		return f.Beta, true
	case "market_cap":
		return f.MarketCap, true
	case "sector":
		return f.Sector, true
	case "industry":
		return f.Industry, true
	}
	return nil, false
}

// resolveHoldingField maps field names to Holding struct values.
// After the return metrics refactor, GainLossPct/CapitalGainPct/TotalReturnPct
// are IRR p.a. from Navexa (previously these were simple return %).
// The _pa and _irr suffixed aliases resolve to the same IRR values.
// TotalReturnPctTWRR is the locally-computed time-weighted return.
func resolveHoldingField(field string, h *models.Holding) (interface{}, bool) {
	if h == nil {
		return nil, false
	}
	switch field {
	case "weight", "portfolio_weight_pct":
		return h.PortfolioWeightPct, true
	case "net_return_pct", "gain_loss_pct", "gain_loss_pct_pa", "gain_loss_pct_irr":
		return h.NetReturnPct, true
	case "net_return_pct_irr", "total_return_pct", "total_return_pct_pa", "total_return_pct_irr", "annualized_total_return_pct":
		return h.AnnualizedTotalReturnPct, true
	case "capital_gain_pct", "capital_gain_pct_pa", "capital_gain_pct_irr", "annualized_capital_return_pct":
		return h.AnnualizedCapitalReturnPct, true
	case "net_return_pct_twrr", "total_return_pct_twrr", "time_weighted_return_pct":
		return h.TimeWeightedReturnPct, true
	case "units":
		return h.Units, true
	case "market_value":
		return h.MarketValue, true
	}
	return nil, false
}

// compareValues compares an actual value against an expected value using the given operator.
func compareValues(actual interface{}, op models.RuleOperator, expected interface{}) bool {
	// Handle boolean comparisons
	if ab, ok := actual.(bool); ok {
		eb := toBool(expected)
		switch op {
		case models.RuleOpEQ:
			return ab == eb
		case models.RuleOpNE:
			return ab != eb
		}
		return false
	}

	// Handle string comparisons
	if as, ok := actual.(string); ok {
		switch op {
		case models.RuleOpEQ:
			return strings.EqualFold(as, toString(expected))
		case models.RuleOpNE:
			return !strings.EqualFold(as, toString(expected))
		case models.RuleOpIn:
			return stringInSlice(as, toStringSlice(expected))
		case models.RuleOpNotIn:
			return !stringInSlice(as, toStringSlice(expected))
		}
		return false
	}

	// Numeric comparisons
	af := toFloat64(actual)
	ef := toFloat64(expected)

	switch op {
	case models.RuleOpGT:
		return af > ef
	case models.RuleOpGTE:
		return af >= ef
	case models.RuleOpLT:
		return af < ef
	case models.RuleOpLTE:
		return af <= ef
	case models.RuleOpEQ:
		return af == ef
	case models.RuleOpNE:
		return af != ef
	}
	return false
}

// interpolateReason replaces {field} placeholders with actual values from the context
func interpolateReason(template string, ctx RuleContext) string {
	if !strings.Contains(template, "{") {
		return template
	}

	result := template
	// Find all {field} patterns and replace with actual values
	for {
		start := strings.Index(result, "{")
		if start == -1 {
			break
		}
		end := strings.Index(result[start:], "}")
		if end == -1 {
			break
		}
		end += start

		field := result[start+1 : end]
		if val, ok := resolveField(field, ctx); ok {
			result = result[:start] + fmt.Sprintf("%v", val) + result[end+1:]
		} else {
			result = result[:start] + "N/A" + result[end+1:]
		}
	}
	return result
}

// Type conversion helpers

func toFloat64(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json_number:
		f, _ := n.Float64()
		return f
	}
	return 0
}

// json_number is an interface for json.Number
type json_number interface {
	Float64() (float64, error)
}

func toString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func toBool(v interface{}) bool {
	switch b := v.(type) {
	case bool:
		return b
	case string:
		return strings.EqualFold(b, "true")
	case float64:
		return b != 0
	}
	return false
}

func toStringSlice(v interface{}) []string {
	// Already a []string
	if ss, ok := v.([]string); ok {
		return ss
	}
	// []interface{} from JSON unmarshalling
	if arr, ok := v.([]interface{}); ok {
		result := make([]string, len(arr))
		for i, item := range arr {
			result[i] = toString(item)
		}
		return result
	}
	return nil
}

func stringInSlice(s string, slice []string) bool {
	for _, item := range slice {
		if strings.EqualFold(s, item) {
			return true
		}
	}
	return false
}
