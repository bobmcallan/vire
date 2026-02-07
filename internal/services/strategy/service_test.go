package strategy

import (
	"context"
	"testing"

	"github.com/bobmccarthy/vire/internal/models"
)

func hasWarning(warnings []models.StrategyWarning, field, severity string) bool {
	for _, w := range warnings {
		if w.Field == field && w.Severity == severity {
			return true
		}
	}
	return false
}

func hasWarningContaining(warnings []models.StrategyWarning, severity, substring string) bool {
	for _, w := range warnings {
		if w.Severity == severity && contains(w.Field, substring) {
			return true
		}
	}
	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && searchString(s, substr)))
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestValidateStrategy_HighReturnTarget(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()

	strategy := &models.PortfolioStrategy{
		TargetReturns: models.TargetReturns{AnnualPct: 25},
	}

	warnings := svc.ValidateStrategy(ctx, strategy)
	if !hasWarning(warnings, "target_returns.annual_pct", "high") {
		t.Error("expected high severity warning for 25% return target")
	}
}

func TestValidateStrategy_ModerateReturnTarget(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()

	strategy := &models.PortfolioStrategy{
		TargetReturns: models.TargetReturns{AnnualPct: 18},
	}

	warnings := svc.ValidateStrategy(ctx, strategy)
	if !hasWarning(warnings, "target_returns.annual_pct", "medium") {
		t.Error("expected medium severity warning for 18% return target")
	}
}

func TestValidateStrategy_ReasonableReturnTarget(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()

	strategy := &models.PortfolioStrategy{
		TargetReturns: models.TargetReturns{AnnualPct: 8},
	}

	warnings := svc.ValidateStrategy(ctx, strategy)
	if hasWarning(warnings, "target_returns.annual_pct", "high") || hasWarning(warnings, "target_returns.annual_pct", "medium") {
		t.Error("should not warn for reasonable 8% return target")
	}
}

func TestValidateStrategy_ConservativeHighReturn(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()

	strategy := &models.PortfolioStrategy{
		RiskAppetite:  models.RiskAppetite{Level: "conservative"},
		TargetReturns: models.TargetReturns{AnnualPct: 15},
	}

	warnings := svc.ValidateStrategy(ctx, strategy)
	if !hasWarningContaining(warnings, "high", "risk_appetite") {
		t.Error("expected high warning for conservative + 15% return")
	}
}

func TestValidateStrategy_ConservativeHighPosition(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()

	strategy := &models.PortfolioStrategy{
		RiskAppetite:   models.RiskAppetite{Level: "conservative"},
		PositionSizing: models.PositionSizing{MaxPositionPct: 20},
	}

	warnings := svc.ValidateStrategy(ctx, strategy)
	if !hasWarningContaining(warnings, "medium", "position_sizing") {
		t.Error("expected medium warning for conservative + 20% max position")
	}
}

func TestValidateStrategy_ModerateHighReturn(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()

	strategy := &models.PortfolioStrategy{
		RiskAppetite:  models.RiskAppetite{Level: "moderate"},
		TargetReturns: models.TargetReturns{AnnualPct: 18},
	}

	warnings := svc.ValidateStrategy(ctx, strategy)
	if !hasWarningContaining(warnings, "medium", "risk_appetite") {
		t.Error("expected medium warning for moderate + 18% return")
	}
}

func TestValidateStrategy_HighConcentration(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()

	strategy := &models.PortfolioStrategy{
		PositionSizing: models.PositionSizing{MaxPositionPct: 40},
	}

	warnings := svc.ValidateStrategy(ctx, strategy)
	if !hasWarning(warnings, "position_sizing.max_position_pct", "high") {
		t.Error("expected high warning for 40% max position")
	}
}

func TestValidateStrategy_HighSectorConcentration(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()

	strategy := &models.PortfolioStrategy{
		PositionSizing: models.PositionSizing{MaxSectorPct: 60},
	}

	warnings := svc.ValidateStrategy(ctx, strategy)
	if !hasWarning(warnings, "position_sizing.max_sector_pct", "medium") {
		t.Error("expected medium warning for 60% max sector")
	}
}

func TestValidateStrategy_SMSFAggressive(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()

	strategy := &models.PortfolioStrategy{
		AccountType:  models.AccountTypeSMSF,
		RiskAppetite: models.RiskAppetite{Level: "aggressive"},
	}

	warnings := svc.ValidateStrategy(ctx, strategy)
	if !hasWarningContaining(warnings, "high", "account_type") {
		t.Error("expected high warning for SMSF + aggressive risk")
	}
}

func TestValidateStrategy_SMSFHighPosition(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()

	strategy := &models.PortfolioStrategy{
		AccountType:    models.AccountTypeSMSF,
		PositionSizing: models.PositionSizing{MaxPositionPct: 30},
	}

	warnings := svc.ValidateStrategy(ctx, strategy)
	if !hasWarningContaining(warnings, "medium", "account_type") {
		t.Error("expected medium warning for SMSF + 30% max position")
	}
}

func TestValidateStrategy_SMSFNoIncome(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()

	strategy := &models.PortfolioStrategy{
		AccountType: models.AccountTypeSMSF,
	}

	warnings := svc.ValidateStrategy(ctx, strategy)
	if !hasWarningContaining(warnings, "medium", "income_requirements") {
		t.Error("expected medium warning for SMSF with no income target")
	}
}

func TestValidateStrategy_HighDividendYield(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()

	strategy := &models.PortfolioStrategy{
		IncomeRequirements: models.IncomeRequirements{DividendYieldPct: 9.0},
	}

	warnings := svc.ValidateStrategy(ctx, strategy)
	if !hasWarning(warnings, "income_requirements.dividend_yield_pct", "medium") {
		t.Error("expected medium warning for 9% dividend yield target")
	}
}

func TestValidateStrategy_ConservativeHighDrawdown(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()

	strategy := &models.PortfolioStrategy{
		RiskAppetite: models.RiskAppetite{
			Level:          "conservative",
			MaxDrawdownPct: 25,
		},
	}

	warnings := svc.ValidateStrategy(ctx, strategy)
	if !hasWarningContaining(warnings, "medium", "max_drawdown_pct") {
		t.Error("expected medium warning for conservative + 25% max drawdown")
	}
}

func TestValidateStrategy_AggressiveLowDrawdown(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()

	strategy := &models.PortfolioStrategy{
		RiskAppetite: models.RiskAppetite{
			Level:          "aggressive",
			MaxDrawdownPct: 10,
		},
	}

	warnings := svc.ValidateStrategy(ctx, strategy)
	if !hasWarningContaining(warnings, "medium", "max_drawdown_pct") {
		t.Error("expected medium warning for aggressive + 10% max drawdown")
	}
}

func TestValidateStrategy_NoWarnings(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()

	// Well-balanced strategy should produce no warnings
	strategy := &models.PortfolioStrategy{
		AccountType:        models.AccountTypeTrading,
		RiskAppetite:       models.RiskAppetite{Level: "moderate", MaxDrawdownPct: 20},
		TargetReturns:      models.TargetReturns{AnnualPct: 10},
		PositionSizing:     models.PositionSizing{MaxPositionPct: 10, MaxSectorPct: 30},
		InvestmentUniverse: []string{"AU"},
	}

	warnings := svc.ValidateStrategy(ctx, strategy)
	if len(warnings) != 0 {
		t.Errorf("expected no warnings for balanced strategy, got %d: %v", len(warnings), warnings)
	}
}

func TestValidateStrategy_TradingNotSMSF(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()

	// Trading account with aggressive settings should NOT trigger SMSF warnings
	strategy := &models.PortfolioStrategy{
		AccountType:  models.AccountTypeTrading,
		RiskAppetite: models.RiskAppetite{Level: "aggressive"},
	}

	warnings := svc.ValidateStrategy(ctx, strategy)
	for _, w := range warnings {
		if searchString(w.Message, "SMSF") || searchString(w.Message, "super fund") {
			t.Errorf("trading account should not get SMSF warnings, got: %s", w.Message)
		}
	}
}

// Table-driven tests for devil's advocate edge cases
func TestValidateStrategy_EdgeCases(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()

	tests := []struct {
		name     string
		strategy *models.PortfolioStrategy
		// wantField and wantSeverity: if empty, expects NO warnings
		wantField    string
		wantSeverity string
		wantCount    int // -1 = don't check count, just check for presence
	}{
		{
			name: "aggressive_low_return",
			strategy: &models.PortfolioStrategy{
				RiskAppetite:  models.RiskAppetite{Level: "aggressive"},
				TargetReturns: models.TargetReturns{AnnualPct: 3},
			},
			wantField:    "risk_appetite, target_returns",
			wantSeverity: "medium",
			wantCount:    -1,
		},
		{
			name: "sector_in_both_preferred_and_excluded",
			strategy: &models.PortfolioStrategy{
				SectorPreferences: models.SectorPreferences{
					Preferred: []string{"Technology", "Healthcare"},
					Excluded:  []string{"Technology", "Mining"},
				},
			},
			wantField:    "sector_preferences",
			wantSeverity: "high",
			wantCount:    -1,
		},
		{
			name: "max_position_exceeds_max_sector",
			strategy: &models.PortfolioStrategy{
				PositionSizing: models.PositionSizing{MaxPositionPct: 40, MaxSectorPct: 25},
			},
			wantField:    "position_sizing",
			wantSeverity: "high",
			wantCount:    -1,
		},
		{
			name:     "empty_strategy_no_crashes",
			strategy: &models.PortfolioStrategy{},
			// empty strategy produces only a low "no investment universe" advisory
			wantField:    "investment_universe",
			wantSeverity: "low",
			wantCount:    1,
		},
		{
			name: "negative_return_target",
			strategy: &models.PortfolioStrategy{
				TargetReturns: models.TargetReturns{AnnualPct: -5},
			},
			wantField:    "target_returns.annual_pct",
			wantSeverity: "high",
			wantCount:    -1,
		},
		{
			name: "negative_position_sizing",
			strategy: &models.PortfolioStrategy{
				PositionSizing: models.PositionSizing{MaxPositionPct: -10},
			},
			wantField:    "position_sizing.max_position_pct",
			wantSeverity: "high",
			wantCount:    -1,
		},
		{
			name: "negative_drawdown",
			strategy: &models.PortfolioStrategy{
				RiskAppetite: models.RiskAppetite{Level: "moderate", MaxDrawdownPct: -5},
			},
			wantField:    "risk_appetite.max_drawdown_pct",
			wantSeverity: "high",
			wantCount:    -1,
		},
		{
			name: "position_over_100_pct",
			strategy: &models.PortfolioStrategy{
				PositionSizing: models.PositionSizing{MaxPositionPct: 150},
			},
			wantField:    "position_sizing.max_position_pct",
			wantSeverity: "high",
			wantCount:    -1,
		},
		{
			name: "sector_over_100_pct",
			strategy: &models.PortfolioStrategy{
				PositionSizing: models.PositionSizing{MaxSectorPct: 120},
			},
			wantField:    "position_sizing.max_sector_pct",
			wantSeverity: "high",
			wantCount:    -1,
		},
		{
			name: "case_insensitive_risk_level",
			strategy: &models.PortfolioStrategy{
				RiskAppetite:  models.RiskAppetite{Level: "Conservative"},
				TargetReturns: models.TargetReturns{AnnualPct: 15},
			},
			wantField:    "risk_appetite, target_returns",
			wantSeverity: "high",
			wantCount:    -1,
		},
		{
			name: "smsf_triple_threat",
			strategy: &models.PortfolioStrategy{
				AccountType:    models.AccountTypeSMSF,
				RiskAppetite:   models.RiskAppetite{Level: "aggressive"},
				PositionSizing: models.PositionSizing{MaxPositionPct: 35},
				// no income requirements
			},
			wantField:    "",
			wantSeverity: "",
			wantCount:    -1, // just verify multiple warnings produced
		},
		{
			name: "boundary_return_20pct",
			strategy: &models.PortfolioStrategy{
				TargetReturns: models.TargetReturns{AnnualPct: 20},
			},
			// exactly 20% should trigger medium warning (>15), not high (>20)
			wantField:    "target_returns.annual_pct",
			wantSeverity: "medium",
			wantCount:    -1,
		},
		{
			name: "boundary_return_15pct",
			strategy: &models.PortfolioStrategy{
				TargetReturns: models.TargetReturns{AnnualPct: 15},
			},
			// exactly 15% should NOT trigger a return warning (rule is >15)
			// only produces the low "no investment universe" advisory
			wantField:    "investment_universe",
			wantSeverity: "low",
			wantCount:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warnings := svc.ValidateStrategy(ctx, tt.strategy)

			if tt.wantCount == 0 {
				if len(warnings) != 0 {
					t.Errorf("expected no warnings, got %d: %+v", len(warnings), warnings)
				}
				return
			}

			// Verify exact count when specified (positive)
			if tt.wantCount > 0 && len(warnings) != tt.wantCount {
				t.Errorf("expected %d warnings, got %d: %+v", tt.wantCount, len(warnings), warnings)
			}

			if tt.wantField != "" && tt.wantSeverity != "" {
				found := false
				for _, w := range warnings {
					if w.Field == tt.wantField && w.Severity == tt.wantSeverity {
						found = true
						break
					}
				}
				if !found {
					// Try substring match for fields that may have additional context
					foundSubstring := false
					for _, w := range warnings {
						if w.Severity == tt.wantSeverity && searchString(w.Field, tt.wantField) {
							foundSubstring = true
							break
						}
					}
					if !foundSubstring {
						t.Errorf("expected warning with field=%q severity=%q, got: %+v",
							tt.wantField, tt.wantSeverity, warnings)
					}
				}
			}

			// SMSF triple threat: verify at least 3 warnings
			if tt.name == "smsf_triple_threat" && len(warnings) < 3 {
				t.Errorf("SMSF triple threat should produce at least 3 warnings, got %d: %+v",
					len(warnings), warnings)
			}
		})
	}
}
