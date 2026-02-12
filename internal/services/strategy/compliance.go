// Package strategy provides portfolio strategy management services
package strategy

import (
	"fmt"
	"strings"

	"github.com/bobmcallan/vire/internal/models"
)

// CheckCompliance evaluates a holding against the portfolio strategy.
// Returns nil if no strategy is provided.
func CheckCompliance(
	strategy *models.PortfolioStrategy,
	holding *models.Holding,
	signals *models.TickerSignals,
	fundamentals *models.Fundamentals,
	sectorWeight float64,
) *models.ComplianceResult {
	if strategy == nil {
		return &models.ComplianceResult{Status: models.ComplianceStatusUnknown}
	}

	var reasons []string
	var ruleHits []string

	// 1. CompanyFilter checks
	if fundamentals != nil {
		cf := strategy.CompanyFilter

		// Sector in excluded list
		if len(cf.ExcludedSectors) > 0 && fundamentals.Sector != "" {
			for _, ex := range cf.ExcludedSectors {
				if strings.EqualFold(fundamentals.Sector, ex) {
					reasons = append(reasons, fmt.Sprintf("Sector %q is excluded by company filter", fundamentals.Sector))
					break
				}
			}
		}

		// Sector not in allowed list (if allowed list is specified)
		if len(cf.AllowedSectors) > 0 && fundamentals.Sector != "" {
			found := false
			for _, a := range cf.AllowedSectors {
				if strings.EqualFold(fundamentals.Sector, a) {
					found = true
					break
				}
			}
			if !found {
				reasons = append(reasons, fmt.Sprintf("Sector %q not in allowed sectors", fundamentals.Sector))
			}
		}

		// Market cap below minimum
		if cf.MinMarketCap > 0 && fundamentals.MarketCap > 0 && fundamentals.MarketCap < cf.MinMarketCap {
			reasons = append(reasons, fmt.Sprintf("Market cap $%.0fM below minimum $%.0fM",
				fundamentals.MarketCap/1e6, cf.MinMarketCap/1e6))
		}

		// P/E above maximum
		if cf.MaxPE > 0 && fundamentals.PE > 0 && fundamentals.PE > cf.MaxPE {
			reasons = append(reasons, fmt.Sprintf("P/E %.1f exceeds maximum %.1f", fundamentals.PE, cf.MaxPE))
		}

		// Dividend yield below minimum
		if cf.MinDividendYield > 0 && fundamentals.DividendYield < cf.MinDividendYield {
			reasons = append(reasons, fmt.Sprintf("Dividend yield %.2f%% below minimum %.2f%%",
				fundamentals.DividendYield*100, cf.MinDividendYield*100))
		}
	}

	// 2. PositionSizing checks
	if holding != nil {
		if strategy.PositionSizing.MaxPositionPct > 0 && holding.Weight > strategy.PositionSizing.MaxPositionPct {
			reasons = append(reasons, fmt.Sprintf("Position weight %.1f%% exceeds max %.1f%%",
				holding.Weight, strategy.PositionSizing.MaxPositionPct))
		}

		if strategy.PositionSizing.MaxSectorPct > 0 && sectorWeight > strategy.PositionSizing.MaxSectorPct {
			reasons = append(reasons, fmt.Sprintf("Sector weight %.1f%% exceeds max %.1f%%",
				sectorWeight, strategy.PositionSizing.MaxSectorPct))
		}
	}

	// 3. SectorPreferences checks
	if fundamentals != nil && fundamentals.Sector != "" {
		if len(strategy.SectorPreferences.Excluded) > 0 {
			for _, ex := range strategy.SectorPreferences.Excluded {
				if strings.EqualFold(fundamentals.Sector, ex) {
					reasons = append(reasons, fmt.Sprintf("Sector %q is in strategy excluded sectors", fundamentals.Sector))
					break
				}
			}
		}
	}

	// 4. InvestmentUniverse checks
	if holding != nil && len(strategy.InvestmentUniverse) > 0 && holding.Exchange != "" {
		found := false
		for _, u := range strategy.InvestmentUniverse {
			if strings.EqualFold(holding.Exchange, u) {
				found = true
				break
			}
		}
		if !found {
			reasons = append(reasons, fmt.Sprintf("Exchange %q not in investment universe %v",
				holding.Exchange, strategy.InvestmentUniverse))
		}
	}

	// 5. Rules: any SELL-action rule that matches = non-compliant
	if len(strategy.Rules) > 0 {
		ctx := RuleContext{
			Holding:      holding,
			Signals:      signals,
			Fundamentals: fundamentals,
		}
		results := EvaluateRules(strategy.Rules, ctx)
		for _, r := range results {
			if r.Rule.Action == models.RuleActionSell {
				reasons = append(reasons, fmt.Sprintf("Rule %q triggered: %s", r.Rule.Name, r.Reason))
				ruleHits = append(ruleHits, r.Rule.Name)
			}
		}
	}

	// Deduplicate reasons (e.g. sector excluded both by CompanyFilter and SectorPreferences)
	reasons = deduplicateStrings(reasons)

	if len(reasons) > 0 {
		return &models.ComplianceResult{
			Status:   models.ComplianceStatusNonCompliant,
			Reasons:  reasons,
			RuleHits: ruleHits,
		}
	}

	return &models.ComplianceResult{Status: models.ComplianceStatusCompliant}
}

// ComputeSectorWeight calculates the total portfolio weight for the sector of the given holding
func ComputeSectorWeight(holding *models.Holding, allHoldings []models.Holding, getFundamentals func(ticker string) *models.Fundamentals) float64 {
	if holding == nil {
		return 0
	}
	holdingFundamentals := getFundamentals(holding.Ticker)
	if holdingFundamentals == nil || holdingFundamentals.Sector == "" {
		return 0
	}

	totalWeight := 0.0
	for _, h := range allHoldings {
		if h.Units <= 0 {
			continue
		}
		f := getFundamentals(h.Ticker)
		if f != nil && strings.EqualFold(f.Sector, holdingFundamentals.Sector) {
			totalWeight += h.Weight
		}
	}
	return totalWeight
}

func deduplicateStrings(s []string) []string {
	seen := make(map[string]bool, len(s))
	result := make([]string, 0, len(s))
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	return result
}
