package strategy

import (
	"testing"

	"github.com/bobmccarthy/vire/internal/models"
)

func TestCheckCompliance_NilStrategy(t *testing.T) {
	result := CheckCompliance(nil, &models.Holding{}, nil, nil, 0)
	if result.Status != models.ComplianceStatusUnknown {
		t.Errorf("Expected unknown status with nil strategy, got %s", result.Status)
	}
}

func TestCheckCompliance_FullyCompliant(t *testing.T) {
	strategy := &models.PortfolioStrategy{
		PositionSizing: models.PositionSizing{MaxPositionPct: 15, MaxSectorPct: 40},
	}
	holding := &models.Holding{Weight: 8.0}
	result := CheckCompliance(strategy, holding, nil, nil, 25.0)
	if result.Status != models.ComplianceStatusCompliant {
		t.Errorf("Expected compliant, got %s with reasons: %v", result.Status, result.Reasons)
	}
}

func TestCheckCompliance_PositionSizeExceeded(t *testing.T) {
	strategy := &models.PortfolioStrategy{
		PositionSizing: models.PositionSizing{MaxPositionPct: 10},
	}
	holding := &models.Holding{Weight: 12.3}

	result := CheckCompliance(strategy, holding, nil, nil, 0)
	if result.Status != models.ComplianceStatusNonCompliant {
		t.Fatalf("Expected non_compliant, got %s", result.Status)
	}
	if len(result.Reasons) == 0 {
		t.Fatal("Expected at least one reason")
	}
}

func TestCheckCompliance_SectorWeightExceeded(t *testing.T) {
	strategy := &models.PortfolioStrategy{
		PositionSizing: models.PositionSizing{MaxSectorPct: 30},
	}
	holding := &models.Holding{Weight: 5}

	result := CheckCompliance(strategy, holding, nil, nil, 35.0)
	if result.Status != models.ComplianceStatusNonCompliant {
		t.Fatalf("Expected non_compliant, got %s", result.Status)
	}
}

func TestCheckCompliance_CompanyFilter_ExcludedSector(t *testing.T) {
	strategy := &models.PortfolioStrategy{
		CompanyFilter: models.CompanyFilter{
			ExcludedSectors: []string{"Gambling", "Tobacco"},
		},
	}
	fundamentals := &models.Fundamentals{Sector: "Gambling"}

	result := CheckCompliance(strategy, &models.Holding{}, nil, fundamentals, 0)
	if result.Status != models.ComplianceStatusNonCompliant {
		t.Fatalf("Expected non_compliant for excluded sector, got %s", result.Status)
	}
}

func TestCheckCompliance_CompanyFilter_NotInAllowed(t *testing.T) {
	strategy := &models.PortfolioStrategy{
		CompanyFilter: models.CompanyFilter{
			AllowedSectors: []string{"Technology", "Healthcare"},
		},
	}
	fundamentals := &models.Fundamentals{Sector: "Energy"}

	result := CheckCompliance(strategy, &models.Holding{}, nil, fundamentals, 0)
	if result.Status != models.ComplianceStatusNonCompliant {
		t.Fatalf("Expected non_compliant for non-allowed sector, got %s", result.Status)
	}
}

func TestCheckCompliance_CompanyFilter_MarketCapBelowMin(t *testing.T) {
	strategy := &models.PortfolioStrategy{
		CompanyFilter: models.CompanyFilter{MinMarketCap: 1e9},
	}
	fundamentals := &models.Fundamentals{MarketCap: 500e6}

	result := CheckCompliance(strategy, &models.Holding{}, nil, fundamentals, 0)
	if result.Status != models.ComplianceStatusNonCompliant {
		t.Fatalf("Expected non_compliant for low market cap, got %s", result.Status)
	}
}

func TestCheckCompliance_CompanyFilter_PEAboveMax(t *testing.T) {
	strategy := &models.PortfolioStrategy{
		CompanyFilter: models.CompanyFilter{MaxPE: 20},
	}
	fundamentals := &models.Fundamentals{PE: 25}

	result := CheckCompliance(strategy, &models.Holding{}, nil, fundamentals, 0)
	if result.Status != models.ComplianceStatusNonCompliant {
		t.Fatalf("Expected non_compliant for high P/E, got %s", result.Status)
	}
}

func TestCheckCompliance_CompanyFilter_DividendYieldBelowMin(t *testing.T) {
	strategy := &models.PortfolioStrategy{
		CompanyFilter: models.CompanyFilter{MinDividendYield: 0.03},
	}
	fundamentals := &models.Fundamentals{DividendYield: 0.01}

	result := CheckCompliance(strategy, &models.Holding{}, nil, fundamentals, 0)
	if result.Status != models.ComplianceStatusNonCompliant {
		t.Fatalf("Expected non_compliant for low dividend yield, got %s", result.Status)
	}
}

func TestCheckCompliance_SectorPreferencesExcluded(t *testing.T) {
	strategy := &models.PortfolioStrategy{
		SectorPreferences: models.SectorPreferences{
			Excluded: []string{"Telecommunications"},
		},
	}
	fundamentals := &models.Fundamentals{Sector: "Telecommunications"}

	result := CheckCompliance(strategy, &models.Holding{}, nil, fundamentals, 0)
	if result.Status != models.ComplianceStatusNonCompliant {
		t.Fatalf("Expected non_compliant for sector preference exclusion, got %s", result.Status)
	}
}

func TestCheckCompliance_InvestmentUniverseMismatch(t *testing.T) {
	strategy := &models.PortfolioStrategy{
		InvestmentUniverse: []string{"AU"},
	}
	holding := &models.Holding{Exchange: "US"}

	result := CheckCompliance(strategy, holding, nil, nil, 0)
	if result.Status != models.ComplianceStatusNonCompliant {
		t.Fatalf("Expected non_compliant for exchange mismatch, got %s", result.Status)
	}
}

func TestCheckCompliance_RuleSellTrigger(t *testing.T) {
	strategy := &models.PortfolioStrategy{
		Rules: []models.Rule{
			{
				Name:       "overbought-sell",
				Conditions: []models.RuleCondition{{Field: "signals.rsi", Operator: models.RuleOpGT, Value: 65.0}},
				Action:     models.RuleActionSell,
				Reason:     "RSI overbought",
				Priority:   1,
				Enabled:    true,
			},
		},
	}
	signals := &models.TickerSignals{Technical: models.TechnicalSignals{RSI: 72.0}}

	result := CheckCompliance(strategy, &models.Holding{}, signals, nil, 0)
	if result.Status != models.ComplianceStatusNonCompliant {
		t.Fatalf("Expected non_compliant for SELL rule trigger, got %s", result.Status)
	}
	if len(result.RuleHits) != 1 || result.RuleHits[0] != "overbought-sell" {
		t.Errorf("Expected rule hit 'overbought-sell', got %v", result.RuleHits)
	}
}

func TestCheckCompliance_RuleBuyDoesNotFlagNonCompliant(t *testing.T) {
	strategy := &models.PortfolioStrategy{
		Rules: []models.Rule{
			{
				Name:       "oversold-buy",
				Conditions: []models.RuleCondition{{Field: "signals.rsi", Operator: models.RuleOpLT, Value: 30.0}},
				Action:     models.RuleActionBuy,
				Reason:     "RSI oversold",
				Priority:   1,
				Enabled:    true,
			},
		},
	}
	signals := &models.TickerSignals{Technical: models.TechnicalSignals{RSI: 25.0}}

	result := CheckCompliance(strategy, &models.Holding{}, signals, nil, 0)
	if result.Status != models.ComplianceStatusCompliant {
		t.Fatalf("BUY rule should not trigger non-compliance, got %s with reasons: %v", result.Status, result.Reasons)
	}
}

func TestCheckCompliance_MultipleViolations(t *testing.T) {
	strategy := &models.PortfolioStrategy{
		PositionSizing: models.PositionSizing{MaxPositionPct: 10},
		SectorPreferences: models.SectorPreferences{
			Excluded: []string{"Gambling"},
		},
	}
	holding := &models.Holding{Weight: 15}
	fundamentals := &models.Fundamentals{Sector: "Gambling"}

	result := CheckCompliance(strategy, holding, nil, fundamentals, 0)
	if result.Status != models.ComplianceStatusNonCompliant {
		t.Fatalf("Expected non_compliant for multiple violations, got %s", result.Status)
	}
	if len(result.Reasons) < 2 {
		t.Errorf("Expected at least 2 reasons, got %d: %v", len(result.Reasons), result.Reasons)
	}
}

func TestCheckCompliance_NilFundamentals(t *testing.T) {
	strategy := &models.PortfolioStrategy{
		CompanyFilter: models.CompanyFilter{
			MinMarketCap:    1e9,
			AllowedSectors:  []string{"Technology"},
			ExcludedSectors: []string{"Gambling"},
		},
	}

	// With nil fundamentals, company filter checks are skipped
	result := CheckCompliance(strategy, &models.Holding{}, nil, nil, 0)
	if result.Status != models.ComplianceStatusCompliant {
		t.Errorf("Nil fundamentals should not trigger company filter violations, got %s: %v", result.Status, result.Reasons)
	}
}
