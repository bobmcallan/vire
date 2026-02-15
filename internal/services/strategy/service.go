// Package strategy provides portfolio strategy management services
package strategy

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

// Compile-time interface check
var _ interfaces.StrategyService = (*Service)(nil)

// Service implements StrategyService
type Service struct {
	storage interfaces.StorageManager
	logger  *common.Logger
}

// NewService creates a new strategy service
func NewService(storage interfaces.StorageManager, logger *common.Logger) *Service {
	return &Service{
		storage: storage,
		logger:  logger,
	}
}

// GetStrategy retrieves the strategy for a portfolio
func (s *Service) GetStrategy(ctx context.Context, portfolioName string) (*models.PortfolioStrategy, error) {
	userID := common.ResolveUserID(ctx)
	rec, err := s.storage.UserDataStore().Get(ctx, userID, "strategy", portfolioName)
	if err != nil {
		return nil, fmt.Errorf("failed to get strategy: %w", err)
	}
	var strategy models.PortfolioStrategy
	if err := json.Unmarshal([]byte(rec.Value), &strategy); err != nil {
		return nil, fmt.Errorf("failed to unmarshal strategy: %w", err)
	}
	return &strategy, nil
}

// SaveStrategy saves a strategy and returns devil's advocate warnings
func (s *Service) SaveStrategy(ctx context.Context, strategy *models.PortfolioStrategy) ([]models.StrategyWarning, error) {
	warnings := s.ValidateStrategy(ctx, strategy)

	userID := common.ResolveUserID(ctx)
	data, err := json.Marshal(strategy)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal strategy: %w", err)
	}
	if err := s.storage.UserDataStore().Put(ctx, &models.UserRecord{
		UserID:  userID,
		Subject: "strategy",
		Key:     strategy.PortfolioName,
		Value:   string(data),
	}); err != nil {
		return nil, fmt.Errorf("failed to save strategy: %w", err)
	}

	s.logger.Info().
		Str("portfolio", strategy.PortfolioName).
		Int("warnings", len(warnings)).
		Msg("Strategy saved")

	return warnings, nil
}

// DeleteStrategy removes a strategy
func (s *Service) DeleteStrategy(ctx context.Context, portfolioName string) error {
	userID := common.ResolveUserID(ctx)
	if err := s.storage.UserDataStore().Delete(ctx, userID, "strategy", portfolioName); err != nil {
		return fmt.Errorf("failed to delete strategy: %w", err)
	}
	s.logger.Info().Str("portfolio", portfolioName).Msg("Strategy deleted")
	return nil
}

// ValidateStrategy checks for unrealistic goals and internal contradictions.
// Returns a list of warnings that should be presented to the user as devil's advocate challenges.
func (s *Service) ValidateStrategy(_ context.Context, strategy *models.PortfolioStrategy) []models.StrategyWarning {
	var warnings []models.StrategyWarning

	warnings = append(warnings, validateReturns(strategy)...)
	warnings = append(warnings, validateRiskConsistency(strategy)...)
	warnings = append(warnings, validatePositionSizing(strategy)...)
	warnings = append(warnings, validateSMSF(strategy)...)
	warnings = append(warnings, validateIncome(strategy)...)
	warnings = append(warnings, validateDrawdown(strategy)...)
	warnings = append(warnings, validateSectorConsistency(strategy)...)
	warnings = append(warnings, validateSanity(strategy)...)
	warnings = append(warnings, validateRules(strategy)...)

	return warnings
}

// validateReturns checks if target returns are realistic
func validateReturns(s *models.PortfolioStrategy) []models.StrategyWarning {
	var warnings []models.StrategyWarning

	if s.TargetReturns.AnnualPct > 20 {
		warnings = append(warnings, models.StrategyWarning{
			Severity: "high",
			Field:    "target_returns.annual_pct",
			Message: fmt.Sprintf("Target annual return of %.1f%% significantly exceeds long-term equity market averages "+
				"(ASX200 ~10%%, S&P500 ~10-12%%). Returns above 20%% are rarely sustained without taking substantial risk.",
				s.TargetReturns.AnnualPct),
		})
	} else if s.TargetReturns.AnnualPct > 15 {
		warnings = append(warnings, models.StrategyWarning{
			Severity: "medium",
			Field:    "target_returns.annual_pct",
			Message: fmt.Sprintf("Target annual return of %.1f%% is above long-term equity averages. "+
				"This typically requires concentrated positions or growth/momentum strategies with higher volatility.",
				s.TargetReturns.AnnualPct),
		})
	}

	return warnings
}

// validateRiskConsistency checks for contradictions between risk appetite and other settings
func validateRiskConsistency(s *models.PortfolioStrategy) []models.StrategyWarning {
	var warnings []models.StrategyWarning
	level := strings.ToLower(s.RiskAppetite.Level)

	// Conservative risk + aggressive return targets
	if level == "conservative" && s.TargetReturns.AnnualPct > 10 {
		warnings = append(warnings, models.StrategyWarning{
			Severity: "high",
			Field:    "risk_appetite, target_returns",
			Message: fmt.Sprintf("Conservative risk appetite conflicts with %.1f%% annual return target. "+
				"Conservative portfolios typically return 5-8%% annually. Consider lowering your return target or "+
				"accepting a moderate risk appetite.",
				s.TargetReturns.AnnualPct),
		})
	}

	// Conservative risk + high max position
	if level == "conservative" && s.PositionSizing.MaxPositionPct > 15 {
		warnings = append(warnings, models.StrategyWarning{
			Severity: "medium",
			Field:    "risk_appetite, position_sizing.max_position_pct",
			Message: fmt.Sprintf("Conservative risk appetite with %.1f%% maximum single position is inconsistent. "+
				"Conservative portfolios typically limit individual positions to 5-10%%.",
				s.PositionSizing.MaxPositionPct),
		})
	}

	// Aggressive risk + low return target (why take the risk?)
	if level == "aggressive" && s.TargetReturns.AnnualPct > 0 && s.TargetReturns.AnnualPct < 5 {
		warnings = append(warnings, models.StrategyWarning{
			Severity: "medium",
			Field:    "risk_appetite, target_returns",
			Message: fmt.Sprintf("Aggressive risk appetite with only %.1f%% annual return target is unusual. "+
				"If you only need low single-digit returns, a conservative or moderate approach achieves this with less risk.",
				s.TargetReturns.AnnualPct),
		})
	}

	// Moderate risk + very high return targets
	if level == "moderate" && s.TargetReturns.AnnualPct > 15 {
		warnings = append(warnings, models.StrategyWarning{
			Severity: "medium",
			Field:    "risk_appetite, target_returns",
			Message: fmt.Sprintf("Moderate risk appetite with %.1f%% annual return target may be difficult to achieve. "+
				"Moderate-risk portfolios typically target 8-12%% annually.",
				s.TargetReturns.AnnualPct),
		})
	}

	return warnings
}

// validatePositionSizing checks position sizing rules for sanity
func validatePositionSizing(s *models.PortfolioStrategy) []models.StrategyWarning {
	var warnings []models.StrategyWarning

	if s.PositionSizing.MaxPositionPct > 30 {
		warnings = append(warnings, models.StrategyWarning{
			Severity: "high",
			Field:    "position_sizing.max_position_pct",
			Message: fmt.Sprintf("Maximum single position of %.1f%% creates significant concentration risk. "+
				"A single stock declining 30%% would impact the portfolio by %.1f%%.",
				s.PositionSizing.MaxPositionPct,
				s.PositionSizing.MaxPositionPct*0.3),
		})
	}

	if s.PositionSizing.MaxSectorPct > 50 {
		warnings = append(warnings, models.StrategyWarning{
			Severity: "medium",
			Field:    "position_sizing.max_sector_pct",
			Message: fmt.Sprintf("Maximum sector allocation of %.1f%% exposes the portfolio to sector-specific downturns. "+
				"Consider capping sector exposure at 30-40%% for diversification.",
				s.PositionSizing.MaxSectorPct),
		})
	}

	return warnings
}

// validateSMSF checks SMSF-specific regulatory and practical concerns
func validateSMSF(s *models.PortfolioStrategy) []models.StrategyWarning {
	var warnings []models.StrategyWarning

	if s.AccountType != models.AccountTypeSMSF {
		return warnings
	}

	level := strings.ToLower(s.RiskAppetite.Level)

	if level == "aggressive" {
		warnings = append(warnings, models.StrategyWarning{
			Severity: "high",
			Field:    "account_type, risk_appetite",
			Message:  "Aggressive risk appetite in an SMSF raises regulatory concerns. SMSF trustees have a legal duty to consider the fund's investment strategy in relation to risk, diversification, liquidity, and the ability to pay member benefits. An aggressive approach may not meet the 'prudent person' test.",
		})
	}

	if s.PositionSizing.MaxPositionPct > 25 {
		warnings = append(warnings, models.StrategyWarning{
			Severity: "medium",
			Field:    "account_type, position_sizing.max_position_pct",
			Message: fmt.Sprintf("SMSF with %.1f%% maximum single position may face liquidity concerns. "+
				"SMSFs must maintain adequate liquidity to pay member benefits and expenses.",
				s.PositionSizing.MaxPositionPct),
		})
	}

	return warnings
}

// validateIncome checks income requirements for consistency
func validateIncome(s *models.PortfolioStrategy) []models.StrategyWarning {
	var warnings []models.StrategyWarning

	// SMSF without income component
	if s.AccountType == models.AccountTypeSMSF && s.IncomeRequirements.DividendYieldPct == 0 {
		warnings = append(warnings, models.StrategyWarning{
			Severity: "medium",
			Field:    "account_type, income_requirements",
			Message:  "SMSF strategy has no dividend yield target. Super funds in pension phase must make minimum annual pension payments. Consider including an income component to meet payment obligations.",
		})
	}

	// Very high dividend yield targets
	if s.IncomeRequirements.DividendYieldPct > 7 {
		warnings = append(warnings, models.StrategyWarning{
			Severity: "medium",
			Field:    "income_requirements.dividend_yield_pct",
			Message: fmt.Sprintf("Dividend yield target of %.1f%% is significantly above market averages "+
				"(ASX200 yield ~4%%, S&P500 yield ~1.5%%). Chasing high yields can indicate value traps or unsustainable payouts.",
				s.IncomeRequirements.DividendYieldPct),
		})
	}

	return warnings
}

// validateDrawdown checks drawdown tolerance for consistency with risk level
func validateDrawdown(s *models.PortfolioStrategy) []models.StrategyWarning {
	var warnings []models.StrategyWarning
	level := strings.ToLower(s.RiskAppetite.Level)

	if s.RiskAppetite.MaxDrawdownPct == 0 {
		return warnings
	}

	if level == "conservative" && s.RiskAppetite.MaxDrawdownPct > 15 {
		warnings = append(warnings, models.StrategyWarning{
			Severity: "medium",
			Field:    "risk_appetite.level, risk_appetite.max_drawdown_pct",
			Message: fmt.Sprintf("Conservative risk level with %.1f%% maximum drawdown tolerance is contradictory. "+
				"Conservative investors typically accept 5-10%% maximum drawdowns.",
				s.RiskAppetite.MaxDrawdownPct),
		})
	}

	if level == "aggressive" && s.RiskAppetite.MaxDrawdownPct < 15 {
		warnings = append(warnings, models.StrategyWarning{
			Severity: "medium",
			Field:    "risk_appetite.level, risk_appetite.max_drawdown_pct",
			Message: fmt.Sprintf("Aggressive risk level with only %.1f%% maximum drawdown tolerance may be unrealistic. "+
				"Aggressive equity portfolios can experience 20-40%% drawdowns during corrections.",
				s.RiskAppetite.MaxDrawdownPct),
		})
	}

	return warnings
}

// validateSectorConsistency checks sector preferences for contradictions
func validateSectorConsistency(s *models.PortfolioStrategy) []models.StrategyWarning {
	var warnings []models.StrategyWarning

	// Check for overlap between preferred and excluded sectors
	if len(s.SectorPreferences.Preferred) > 0 && len(s.SectorPreferences.Excluded) > 0 {
		for _, pref := range s.SectorPreferences.Preferred {
			for _, excl := range s.SectorPreferences.Excluded {
				if strings.EqualFold(pref, excl) {
					warnings = append(warnings, models.StrategyWarning{
						Severity: "high",
						Field:    "sector_preferences",
						Message: fmt.Sprintf("Sector '%s' appears in both preferred and excluded lists. "+
							"Remove it from one list to resolve the contradiction.", pref),
					})
				}
			}
		}
	}

	// Conservative + growth/momentum reference strategies
	level := strings.ToLower(s.RiskAppetite.Level)
	if level == "conservative" {
		for _, ref := range s.ReferenceStrategies {
			nameLower := strings.ToLower(ref.Name)
			if strings.Contains(nameLower, "momentum") || strings.Contains(nameLower, "growth") {
				warnings = append(warnings, models.StrategyWarning{
					Severity: "medium",
					Field:    "risk_appetite, reference_strategies",
					Message: fmt.Sprintf("Conservative risk appetite with '%s' reference strategy is contradictory. "+
						"Momentum and growth strategies typically involve higher volatility and drawdowns.", ref.Name),
				})
			}
		}
	}

	return warnings
}

// validateSanity checks for logically impossible or invalid values
func validateSanity(s *models.PortfolioStrategy) []models.StrategyWarning {
	var warnings []models.StrategyWarning

	// MaxPosition > MaxSector is a logical impossibility
	if s.PositionSizing.MaxPositionPct > 0 && s.PositionSizing.MaxSectorPct > 0 &&
		s.PositionSizing.MaxPositionPct > s.PositionSizing.MaxSectorPct {
		warnings = append(warnings, models.StrategyWarning{
			Severity: "high",
			Field:    "position_sizing",
			Message: fmt.Sprintf("Maximum position (%.1f%%) exceeds maximum sector allocation (%.1f%%). "+
				"A single position cannot be larger than its sector limit.",
				s.PositionSizing.MaxPositionPct, s.PositionSizing.MaxSectorPct),
		})
	}

	// Position sizing over 100%
	if s.PositionSizing.MaxPositionPct > 100 {
		warnings = append(warnings, models.StrategyWarning{
			Severity: "high",
			Field:    "position_sizing.max_position_pct",
			Message:  fmt.Sprintf("Maximum position of %.1f%% exceeds 100%%. This is not possible without leverage.", s.PositionSizing.MaxPositionPct),
		})
	}
	if s.PositionSizing.MaxSectorPct > 100 {
		warnings = append(warnings, models.StrategyWarning{
			Severity: "high",
			Field:    "position_sizing.max_sector_pct",
			Message:  fmt.Sprintf("Maximum sector allocation of %.1f%% exceeds 100%%. This is not possible without leverage.", s.PositionSizing.MaxSectorPct),
		})
	}

	// Negative values
	if s.TargetReturns.AnnualPct < 0 {
		warnings = append(warnings, models.StrategyWarning{
			Severity: "high",
			Field:    "target_returns.annual_pct",
			Message:  "Negative annual return target does not make sense. Set to 0 if you have no return target.",
		})
	}
	if s.PositionSizing.MaxPositionPct < 0 {
		warnings = append(warnings, models.StrategyWarning{
			Severity: "high",
			Field:    "position_sizing.max_position_pct",
			Message:  "Negative position sizing is invalid.",
		})
	}
	if s.RiskAppetite.MaxDrawdownPct < 0 {
		warnings = append(warnings, models.StrategyWarning{
			Severity: "high",
			Field:    "risk_appetite.max_drawdown_pct",
			Message:  "Negative maximum drawdown is invalid.",
		})
	}

	// High income target + aggressive risk
	level := strings.ToLower(s.RiskAppetite.Level)
	if level == "aggressive" && s.IncomeRequirements.DividendYieldPct > 4 {
		warnings = append(warnings, models.StrategyWarning{
			Severity: "medium",
			Field:    "risk_appetite, income_requirements",
			Message: fmt.Sprintf("Aggressive risk appetite with %.1f%% dividend yield target is unusual. "+
				"Aggressive strategies typically prioritise capital growth over income.", s.IncomeRequirements.DividendYieldPct),
		})
	}

	// No investment universe specified
	if len(s.InvestmentUniverse) == 0 {
		warnings = append(warnings, models.StrategyWarning{
			Severity: "low",
			Field:    "investment_universe",
			Message:  "No investment universe specified. Consider adding exchanges (e.g., 'AU', 'US') to focus screening and analysis.",
		})
	}

	return warnings
}

// validRuleFields lists all known field paths for rule conditions
var validRuleFields = map[string]bool{
	"signals.rsi": true, "signals.volume_ratio": true, "signals.macd": true,
	"signals.macd_histogram": true, "signals.atr_pct": true,
	"signals.near_support": true, "signals.near_resistance": true,
	"signals.price.distance_to_sma20": true, "signals.price.distance_to_sma50": true,
	"signals.price.distance_to_sma200": true,
	"signals.pbas.score":               true, "signals.pbas.interpretation": true,
	"signals.vli.score": true, "signals.vli.interpretation": true,
	"signals.regime.current": true, "signals.trend": true,
	"fundamentals.pe": true, "fundamentals.pb": true, "fundamentals.eps": true,
	"fundamentals.dividend_yield": true, "fundamentals.beta": true,
	"fundamentals.market_cap": true, "fundamentals.sector": true,
	"fundamentals.industry": true,
	"holding.weight":        true, "holding.gain_loss_pct": true, "holding.total_return_pct": true,
	"holding.capital_gain_pct": true, "holding.units": true, "holding.market_value": true,
}

// validateRules checks rules for structural issues
func validateRules(s *models.PortfolioStrategy) []models.StrategyWarning {
	var warnings []models.StrategyWarning

	for i, rule := range s.Rules {
		// Warn if rule has no conditions (always triggers)
		if len(rule.Conditions) == 0 {
			warnings = append(warnings, models.StrategyWarning{
				Severity: "medium",
				Field:    fmt.Sprintf("rules[%d]", i),
				Message:  fmt.Sprintf("Rule '%s' has no conditions and will always trigger.", rule.Name),
			})
		}

		// Warn if conditions reference unknown fields
		for _, cond := range rule.Conditions {
			if !validRuleFields[cond.Field] {
				warnings = append(warnings, models.StrategyWarning{
					Severity: "medium",
					Field:    fmt.Sprintf("rules[%d].conditions", i),
					Message:  fmt.Sprintf("Rule '%s' references unknown field '%s'.", rule.Name, cond.Field),
				})
			}
		}
	}

	// Check for contradictory rules (same priority, overlapping conditions, different actions)
	for i := 0; i < len(s.Rules); i++ {
		for j := i + 1; j < len(s.Rules); j++ {
			ri, rj := s.Rules[i], s.Rules[j]
			if !ri.Enabled || !rj.Enabled {
				continue
			}
			if ri.Priority == rj.Priority && ri.Action != rj.Action && conditionsOverlap(ri.Conditions, rj.Conditions) {
				warnings = append(warnings, models.StrategyWarning{
					Severity: "high",
					Field:    fmt.Sprintf("rules[%d], rules[%d]", i, j),
					Message: fmt.Sprintf("Rules '%s' and '%s' have the same priority (%d) with overlapping conditions but different actions (%s vs %s).",
						ri.Name, rj.Name, ri.Priority, ri.Action, rj.Action),
				})
			}
		}
	}

	return warnings
}

// conditionsOverlap returns true if both rules have identical condition fields
func conditionsOverlap(a, b []models.RuleCondition) bool {
	if len(a) != len(b) || len(a) == 0 {
		return false
	}
	fieldsA := make(map[string]bool, len(a))
	for _, c := range a {
		fieldsA[c.Field] = true
	}
	for _, c := range b {
		if !fieldsA[c.Field] {
			return false
		}
	}
	return true
}
