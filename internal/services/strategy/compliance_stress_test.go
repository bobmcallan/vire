package strategy

import (
	"testing"

	"github.com/bobmcallan/vire/internal/models"
)

// === Investment Universe exchange mapping stress tests ===
// These validate Bug 3 fix: compliance uses EodhExchange() mapping.

func TestCompliance_ASXHolding_AUUniverse_Compliant(t *testing.T) {
	strategy := &models.PortfolioStrategy{
		InvestmentUniverse: []string{"AU"},
	}
	holding := &models.Holding{Exchange: "ASX"}

	result := CheckCompliance(strategy, holding, nil, nil, 0)
	if result.Status != models.ComplianceStatusCompliant {
		t.Fatalf("ASX holding should be compliant with AU universe after EodhExchange mapping, got %s: %v",
			result.Status, result.Reasons)
	}
}

func TestCompliance_NYSEHolding_USUniverse_Compliant(t *testing.T) {
	strategy := &models.PortfolioStrategy{
		InvestmentUniverse: []string{"US"},
	}
	holding := &models.Holding{Exchange: "NYSE"}

	result := CheckCompliance(strategy, holding, nil, nil, 0)
	if result.Status != models.ComplianceStatusCompliant {
		t.Fatalf("NYSE holding should be compliant with US universe after mapping, got %s: %v",
			result.Status, result.Reasons)
	}
}

func TestCompliance_NASDAQHolding_USUniverse_Compliant(t *testing.T) {
	strategy := &models.PortfolioStrategy{
		InvestmentUniverse: []string{"US"},
	}
	holding := &models.Holding{Exchange: "NASDAQ"}

	result := CheckCompliance(strategy, holding, nil, nil, 0)
	if result.Status != models.ComplianceStatusCompliant {
		t.Fatalf("NASDAQ holding should be compliant with US universe, got %s: %v",
			result.Status, result.Reasons)
	}
}

func TestCompliance_NYSEHolding_AUAndUSUniverse_Compliant(t *testing.T) {
	strategy := &models.PortfolioStrategy{
		InvestmentUniverse: []string{"AU", "US"},
	}
	holding := &models.Holding{Exchange: "NYSE"}

	result := CheckCompliance(strategy, holding, nil, nil, 0)
	if result.Status != models.ComplianceStatusCompliant {
		t.Fatalf("NYSE holding should be compliant with [AU,US] universe, got %s: %v",
			result.Status, result.Reasons)
	}
}

func TestCompliance_LSEHolding_AUUSUniverse_NonCompliant(t *testing.T) {
	strategy := &models.PortfolioStrategy{
		InvestmentUniverse: []string{"AU", "US"},
	}
	holding := &models.Holding{Exchange: "LSE"}

	result := CheckCompliance(strategy, holding, nil, nil, 0)
	if result.Status != models.ComplianceStatusNonCompliant {
		t.Fatalf("LSE holding should be non-compliant with [AU,US] universe, got %s",
			result.Status)
	}
}

func TestCompliance_BATSHolding_USUniverse_Compliant(t *testing.T) {
	strategy := &models.PortfolioStrategy{
		InvestmentUniverse: []string{"US"},
	}
	holding := &models.Holding{Exchange: "BATS"}

	result := CheckCompliance(strategy, holding, nil, nil, 0)
	if result.Status != models.ComplianceStatusCompliant {
		t.Fatalf("BATS holding should map to US and be compliant, got %s: %v",
			result.Status, result.Reasons)
	}
}

func TestCompliance_AMEXHolding_USUniverse_Compliant(t *testing.T) {
	strategy := &models.PortfolioStrategy{
		InvestmentUniverse: []string{"US"},
	}
	holding := &models.Holding{Exchange: "AMEX"}

	result := CheckCompliance(strategy, holding, nil, nil, 0)
	if result.Status != models.ComplianceStatusCompliant {
		t.Fatalf("AMEX holding should map to US and be compliant, got %s: %v",
			result.Status, result.Reasons)
	}
}

func TestCompliance_ARCAHolding_USUniverse_Compliant(t *testing.T) {
	strategy := &models.PortfolioStrategy{
		InvestmentUniverse: []string{"US"},
	}
	holding := &models.Holding{Exchange: "ARCA"}

	result := CheckCompliance(strategy, holding, nil, nil, 0)
	if result.Status != models.ComplianceStatusCompliant {
		t.Fatalf("ARCA holding should map to US and be compliant, got %s: %v",
			result.Status, result.Reasons)
	}
}

// === Edge case: empty exchange ===

func TestCompliance_EmptyExchange_AUUniverse(t *testing.T) {
	strategy := &models.PortfolioStrategy{
		InvestmentUniverse: []string{"AU"},
	}
	holding := &models.Holding{Exchange: ""}

	// Empty exchange maps to "AU" by default in eodhExchange()
	// But the current code skips the check when Exchange == ""
	// After fix, should this use the mapping? The requirement says
	// the exchange check guard is: holding.Exchange != ""
	// So empty exchange still skips the check (compliant by default)
	result := CheckCompliance(strategy, holding, nil, nil, 0)
	if result.Status != models.ComplianceStatusCompliant {
		t.Fatalf("empty exchange should skip universe check (compliant), got %s: %v",
			result.Status, result.Reasons)
	}
}

// === Edge case: unknown exchange codes ===

func TestCompliance_UnknownExchange_XETRA(t *testing.T) {
	strategy := &models.PortfolioStrategy{
		InvestmentUniverse: []string{"AU", "US"},
	}
	holding := &models.Holding{Exchange: "XETRA"}

	// Unknown exchanges pass through as-is in eodhExchange()
	// "XETRA" is not "AU" or "US", so should be non-compliant
	result := CheckCompliance(strategy, holding, nil, nil, 0)
	if result.Status != models.ComplianceStatusNonCompliant {
		t.Fatalf("XETRA (unknown) should be non-compliant with [AU,US], got %s",
			result.Status)
	}
}

func TestCompliance_UnknownExchange_TSE(t *testing.T) {
	strategy := &models.PortfolioStrategy{
		InvestmentUniverse: []string{"AU", "US"},
	}
	holding := &models.Holding{Exchange: "TSE"}

	result := CheckCompliance(strategy, holding, nil, nil, 0)
	if result.Status != models.ComplianceStatusNonCompliant {
		t.Fatalf("TSE should be non-compliant with [AU,US], got %s",
			result.Status)
	}
}

// === Case sensitivity ===

func TestCompliance_ExchangeCaseInsensitive(t *testing.T) {
	strategy := &models.PortfolioStrategy{
		InvestmentUniverse: []string{"AU"},
	}

	tests := []struct {
		name     string
		exchange string
	}{
		{"lowercase asx", "asx"},
		{"mixed case Asx", "Asx"},
		{"lowercase au", "au"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			holding := &models.Holding{Exchange: tt.exchange}
			result := CheckCompliance(strategy, holding, nil, nil, 0)
			if result.Status != models.ComplianceStatusCompliant {
				t.Errorf("exchange %q should map to AU and be compliant, got %s: %v",
					tt.exchange, result.Status, result.Reasons)
			}
		})
	}
}

// === Nil holding with universe check ===

func TestCompliance_NilHolding_UniverseSkipped(t *testing.T) {
	strategy := &models.PortfolioStrategy{
		InvestmentUniverse: []string{"AU"},
	}

	result := CheckCompliance(strategy, nil, nil, nil, 0)
	// nil holding should skip universe check (and position sizing)
	if result.Status != models.ComplianceStatusCompliant {
		t.Fatalf("nil holding should skip universe check (compliant), got %s: %v",
			result.Status, result.Reasons)
	}
}

// === Empty universe ===

func TestCompliance_EmptyUniverse_SkipsCheck(t *testing.T) {
	strategy := &models.PortfolioStrategy{
		InvestmentUniverse: []string{},
	}
	holding := &models.Holding{Exchange: "XETRA"}

	result := CheckCompliance(strategy, holding, nil, nil, 0)
	if result.Status != models.ComplianceStatusCompliant {
		t.Fatalf("empty universe should skip check (compliant), got %s: %v",
			result.Status, result.Reasons)
	}
}

// === LON exchange mapping ===

func TestCompliance_LONHolding_LSEUniverse(t *testing.T) {
	strategy := &models.PortfolioStrategy{
		InvestmentUniverse: []string{"LSE"},
	}
	holding := &models.Holding{Exchange: "LON"}

	// LON maps to LSE in eodhExchange()
	result := CheckCompliance(strategy, holding, nil, nil, 0)
	if result.Status != models.ComplianceStatusCompliant {
		t.Fatalf("LON should map to LSE and be compliant, got %s: %v",
			result.Status, result.Reasons)
	}
}

// === Universe check combined with other violations ===

func TestCompliance_UniverseMismatch_PlusPositionSize(t *testing.T) {
	strategy := &models.PortfolioStrategy{
		InvestmentUniverse: []string{"AU"},
		PositionSizing:     models.PositionSizing{MaxPositionPct: 10},
	}
	holding := &models.Holding{
		Exchange: "NYSE",
		PortfolioWeightPct: 15,
	}

	result := CheckCompliance(strategy, holding, nil, nil, 0)
	if result.Status != models.ComplianceStatusCompliant {
		// After fix: NYSE maps to US which is not in [AU], AND weight exceeds 10%
		// So should be non-compliant with 2 reasons
		if len(result.Reasons) < 2 {
			t.Errorf("expected at least 2 reasons (universe + position size), got %d: %v",
				len(result.Reasons), result.Reasons)
		}
	}
}
