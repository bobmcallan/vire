package models

import (
	"strings"
	"testing"
	"time"
)

func TestDefaultDisclaimer(t *testing.T) {
	if DefaultDisclaimer == "" {
		t.Error("DefaultDisclaimer should not be empty")
	}
}

func TestToMarkdown(t *testing.T) {
	now := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	strategy := &PortfolioStrategy{
		PortfolioName:      "SMSF",
		Version:            3,
		AccountType:        AccountTypeSMSF,
		InvestmentUniverse: []string{"AU", "US"},
		RiskAppetite: RiskAppetite{
			Level:          "moderate",
			MaxDrawdownPct: 15.0,
			Description:    "Balanced approach with some growth exposure",
		},
		TargetReturns: TargetReturns{
			AnnualPct: 8.5,
			Timeframe: "3-5 years",
		},
		IncomeRequirements: IncomeRequirements{
			DividendYieldPct: 4.0,
			Description:      "Focus on franked dividends for tax efficiency",
		},
		SectorPreferences: SectorPreferences{
			Preferred: []string{"Financials", "Healthcare"},
			Excluded:  []string{"Gambling", "Tobacco"},
		},
		PositionSizing: PositionSizing{
			MaxPositionPct: 10.0,
			MaxSectorPct:   30.0,
		},
		ReferenceStrategies: []ReferenceStrategy{
			{Name: "Dividend Growth", Description: "Focus on companies with growing dividends"},
			{Name: "Value Investing", Description: "Buy undervalued companies with strong fundamentals"},
		},
		RebalanceFrequency: "quarterly",
		Notes:              "Review after tax year ends in July.",
		Disclaimer:         DefaultDisclaimer,
		CreatedAt:          now,
		UpdatedAt:          now.Add(24 * time.Hour),
		LastReviewedAt:     now.Add(48 * time.Hour),
	}

	md := strategy.ToMarkdown()

	// Verify all sections are present
	checks := []string{
		"# Portfolio Strategy: SMSF",
		"**Account Type:** smsf",
		"**Investment Universe:** AU, US",
		"## Risk Appetite",
		"**Level:** moderate",
		"**Max Drawdown:** 15.0%",
		"Balanced approach with some growth exposure",
		"## Target Returns",
		"**Annual Target:** 8.5%",
		"**Timeframe:** 3-5 years",
		"## Income Requirements",
		"**Dividend Yield Target:** 4.0%",
		"Focus on franked dividends",
		"## Sector Preferences",
		"**Preferred:** Financials, Healthcare",
		"**Excluded:** Gambling, Tobacco",
		"## Position Sizing",
		"**Max Single Position:** 10.0%",
		"**Max Sector Allocation:** 30.0%",
		"## Reference Strategies",
		"**Dividend Growth:** Focus on companies with growing dividends",
		"**Value Investing:** Buy undervalued companies",
		"**Rebalancing:** quarterly",
		"## Notes",
		"Review after tax year ends in July.",
		"does not constitute financial advice",
		"Version 3",
		"Created 2026-01-15",
		"Updated 2026-01-16",
		"Last Reviewed 2026-01-17",
	}

	for _, check := range checks {
		if !strings.Contains(md, check) {
			t.Errorf("ToMarkdown() missing expected content: %q", check)
		}
	}
}

func TestToMarkdown_EmptyStrategy(t *testing.T) {
	strategy := &PortfolioStrategy{}

	// Should not panic on zero-value strategy
	md := strategy.ToMarkdown()

	if md == "" {
		t.Error("ToMarkdown() should produce output even for empty strategy")
	}

	// Should still have the header and structural sections
	if !strings.Contains(md, "# Portfolio Strategy:") {
		t.Error("ToMarkdown() missing header for empty strategy")
	}
	if !strings.Contains(md, "## Risk Appetite") {
		t.Error("ToMarkdown() missing Risk Appetite section for empty strategy")
	}
	if !strings.Contains(md, "## Target Returns") {
		t.Error("ToMarkdown() missing Target Returns section for empty strategy")
	}
	if !strings.Contains(md, "## Position Sizing") {
		t.Error("ToMarkdown() missing Position Sizing section for empty strategy")
	}
	if !strings.Contains(md, "Version 0") {
		t.Error("ToMarkdown() should show Version 0 for empty strategy")
	}

	// Optional sections should NOT appear when empty
	if strings.Contains(md, "## Income Requirements") {
		t.Error("ToMarkdown() should omit Income Requirements when empty")
	}
	if strings.Contains(md, "## Sector Preferences") {
		t.Error("ToMarkdown() should omit Sector Preferences when empty")
	}
	if strings.Contains(md, "## Reference Strategies") {
		t.Error("ToMarkdown() should omit Reference Strategies when empty")
	}
	if strings.Contains(md, "## Notes") {
		t.Error("ToMarkdown() should omit Notes when empty")
	}
}
