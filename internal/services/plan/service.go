// Package plan provides portfolio plan management services
package plan

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

// Compile-time interface check
var _ interfaces.PlanService = (*Service)(nil)

// Service implements PlanService
type Service struct {
	storage  interfaces.StorageManager
	strategy interfaces.StrategyService
	logger   *common.Logger
}

// NewService creates a new plan service
func NewService(storage interfaces.StorageManager, strategy interfaces.StrategyService, logger *common.Logger) *Service {
	return &Service{
		storage:  storage,
		strategy: strategy,
		logger:   logger,
	}
}

// GetPlan retrieves the plan for a portfolio
func (s *Service) GetPlan(ctx context.Context, portfolioName string) (*models.PortfolioPlan, error) {
	plan, err := s.storage.PlanStorage().GetPlan(ctx, portfolioName)
	if err != nil {
		return nil, fmt.Errorf("failed to get plan: %w", err)
	}
	return plan, nil
}

// SavePlan saves a plan with version increment
func (s *Service) SavePlan(ctx context.Context, plan *models.PortfolioPlan) error {
	err := s.storage.PlanStorage().SavePlan(ctx, plan)
	if err != nil {
		return fmt.Errorf("failed to save plan: %w", err)
	}
	s.logger.Info().Str("portfolio", plan.PortfolioName).Msg("Plan saved")
	return nil
}

// DeletePlan removes a plan
func (s *Service) DeletePlan(ctx context.Context, portfolioName string) error {
	err := s.storage.PlanStorage().DeletePlan(ctx, portfolioName)
	if err != nil {
		return fmt.Errorf("failed to delete plan: %w", err)
	}
	s.logger.Info().Str("portfolio", portfolioName).Msg("Plan deleted")
	return nil
}

// AddPlanItem adds a single item to a portfolio plan
func (s *Service) AddPlanItem(ctx context.Context, portfolioName string, item *models.PlanItem) (*models.PortfolioPlan, error) {
	plan, err := s.storage.PlanStorage().GetPlan(ctx, portfolioName)
	if err != nil {
		// No existing plan — create one
		plan = &models.PortfolioPlan{
			PortfolioName: portfolioName,
			Items:         []models.PlanItem{},
		}
	}

	// Auto-generate ID if not provided
	if item.ID == "" {
		item.ID = fmt.Sprintf("plan-%d", len(plan.Items)+1)
	}

	// Ensure no duplicate IDs
	for _, existing := range plan.Items {
		if existing.ID == item.ID {
			return nil, fmt.Errorf("plan item with ID '%s' already exists", item.ID)
		}
	}

	now := time.Now()
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	item.UpdatedAt = now
	if item.Status == "" {
		item.Status = models.PlanItemStatusPending
	}

	plan.Items = append(plan.Items, *item)

	if err := s.storage.PlanStorage().SavePlan(ctx, plan); err != nil {
		return nil, fmt.Errorf("failed to save plan: %w", err)
	}

	s.logger.Info().Str("portfolio", portfolioName).Str("item_id", item.ID).Msg("Plan item added")
	return plan, nil
}

// UpdatePlanItem updates an existing plan item by ID
func (s *Service) UpdatePlanItem(ctx context.Context, portfolioName, itemID string, update *models.PlanItem) (*models.PortfolioPlan, error) {
	plan, err := s.storage.PlanStorage().GetPlan(ctx, portfolioName)
	if err != nil {
		return nil, fmt.Errorf("failed to get plan: %w", err)
	}

	found := false
	for i, item := range plan.Items {
		if item.ID == itemID {
			// Preserve immutable fields
			update.ID = item.ID
			update.CreatedAt = item.CreatedAt
			update.UpdatedAt = time.Now()

			// Merge: only overwrite non-zero fields from update
			merged := item
			if update.Type != "" {
				merged.Type = update.Type
			}
			if update.Description != "" {
				merged.Description = update.Description
			}
			if update.Status != "" {
				merged.Status = update.Status
				if update.Status == models.PlanItemStatusCompleted {
					now := time.Now()
					merged.CompletedAt = &now
				}
			}
			if update.Deadline != nil {
				merged.Deadline = update.Deadline
			}
			if len(update.Conditions) > 0 {
				merged.Conditions = update.Conditions
			}
			if update.Ticker != "" {
				merged.Ticker = update.Ticker
			}
			if update.Action != "" {
				merged.Action = update.Action
			}
			if update.TargetValue != 0 {
				merged.TargetValue = update.TargetValue
			}
			if update.Notes != "" {
				merged.Notes = update.Notes
			}
			merged.UpdatedAt = time.Now()

			plan.Items[i] = merged
			found = true
			break
		}
	}

	if !found {
		return nil, fmt.Errorf("plan item '%s' not found", itemID)
	}

	if err := s.storage.PlanStorage().SavePlan(ctx, plan); err != nil {
		return nil, fmt.Errorf("failed to save plan: %w", err)
	}

	s.logger.Info().Str("portfolio", portfolioName).Str("item_id", itemID).Msg("Plan item updated")
	return plan, nil
}

// RemovePlanItem removes an item from a plan by ID
func (s *Service) RemovePlanItem(ctx context.Context, portfolioName, itemID string) (*models.PortfolioPlan, error) {
	plan, err := s.storage.PlanStorage().GetPlan(ctx, portfolioName)
	if err != nil {
		return nil, fmt.Errorf("failed to get plan: %w", err)
	}

	found := false
	items := make([]models.PlanItem, 0, len(plan.Items))
	for _, item := range plan.Items {
		if item.ID == itemID {
			found = true
			continue
		}
		items = append(items, item)
	}

	if !found {
		return nil, fmt.Errorf("plan item '%s' not found", itemID)
	}

	plan.Items = items

	if err := s.storage.PlanStorage().SavePlan(ctx, plan); err != nil {
		return nil, fmt.Errorf("failed to save plan: %w", err)
	}

	s.logger.Info().Str("portfolio", portfolioName).Str("item_id", itemID).Msg("Plan item removed")
	return plan, nil
}

// CheckPlanEvents evaluates event-based pending items against current market data.
// Returns items that have been triggered (status changed to "triggered").
func (s *Service) CheckPlanEvents(ctx context.Context, portfolioName string) ([]models.PlanItem, error) {
	plan, err := s.storage.PlanStorage().GetPlan(ctx, portfolioName)
	if err != nil {
		return nil, fmt.Errorf("failed to get plan: %w", err)
	}

	var triggered []models.PlanItem
	changed := false

	for i, item := range plan.Items {
		if item.Status != models.PlanItemStatusPending || item.Type != models.PlanItemTypeEvent {
			continue
		}
		if len(item.Conditions) == 0 {
			continue
		}

		// Resolve signals for the item's ticker
		var signals *models.TickerSignals
		var fundamentals *models.Fundamentals
		if item.Ticker != "" {
			signals, _ = s.storage.SignalStorage().GetSignals(ctx, item.Ticker)
			md, _ := s.storage.MarketDataStorage().GetMarketData(ctx, item.Ticker)
			if md != nil {
				fundamentals = md.Fundamentals
			}
		}

		// Evaluate all conditions (AND logic)
		allMatch := true
		for _, cond := range item.Conditions {
			ok := evaluateConditionSimple(cond, signals, fundamentals)
			if !ok {
				allMatch = false
				break
			}
		}

		if allMatch {
			plan.Items[i].Status = models.PlanItemStatusTriggered
			plan.Items[i].UpdatedAt = time.Now()
			triggered = append(triggered, plan.Items[i])
			changed = true
		}
	}

	if changed {
		if err := s.storage.PlanStorage().SavePlan(ctx, plan); err != nil {
			return nil, fmt.Errorf("failed to save plan: %w", err)
		}
	}

	return triggered, nil
}

// CheckPlanDeadlines marks overdue time-based items as expired.
// Returns items that were newly expired.
func (s *Service) CheckPlanDeadlines(ctx context.Context, portfolioName string) ([]models.PlanItem, error) {
	plan, err := s.storage.PlanStorage().GetPlan(ctx, portfolioName)
	if err != nil {
		return nil, fmt.Errorf("failed to get plan: %w", err)
	}

	now := time.Now()
	var expired []models.PlanItem
	changed := false

	for i, item := range plan.Items {
		if item.Status != models.PlanItemStatusPending || item.Type != models.PlanItemTypeTime {
			continue
		}
		if item.Deadline == nil {
			continue
		}
		if now.After(*item.Deadline) {
			plan.Items[i].Status = models.PlanItemStatusExpired
			plan.Items[i].UpdatedAt = now
			expired = append(expired, plan.Items[i])
			changed = true
		}
	}

	if changed {
		if err := s.storage.PlanStorage().SavePlan(ctx, plan); err != nil {
			return nil, fmt.Errorf("failed to save plan: %w", err)
		}
	}

	return expired, nil
}

// ValidatePlanAgainstStrategy checks plan items against portfolio strategy
func (s *Service) ValidatePlanAgainstStrategy(_ context.Context, plan *models.PortfolioPlan, strategy *models.PortfolioStrategy) []models.StrategyWarning {
	if plan == nil || strategy == nil {
		return nil
	}

	var warnings []models.StrategyWarning

	for i, item := range plan.Items {
		if item.Status == models.PlanItemStatusCompleted || item.Status == models.PlanItemStatusCancelled {
			continue
		}

		// Check BUY actions against CompanyFilter
		if item.Action == models.RuleActionBuy && item.Ticker != "" {
			// Check excluded sectors (we can't check sector without market data,
			// but we can validate against investment universe)
			ticker := item.Ticker
			if len(strategy.InvestmentUniverse) > 0 {
				exchange := extractExchange(ticker)
				if exchange != "" {
					found := false
					for _, u := range strategy.InvestmentUniverse {
						if strings.EqualFold(u, exchange) {
							found = true
							break
						}
					}
					if !found {
						warnings = append(warnings, models.StrategyWarning{
							Severity: "medium",
							Field:    fmt.Sprintf("items[%d]", i),
							Message: fmt.Sprintf("Plan item '%s' targets %s which is outside the investment universe (%s).",
								item.ID, ticker, strings.Join(strategy.InvestmentUniverse, ", ")),
						})
					}
				}
			}
		}

		// Check that target values are reasonable for position sizing
		if item.TargetValue > 0 && strategy.PositionSizing.MaxPositionPct > 0 {
			// We can't check without portfolio value, but we can flag very large target values
			if item.TargetValue > 100000 {
				warnings = append(warnings, models.StrategyWarning{
					Severity: "low",
					Field:    fmt.Sprintf("items[%d]", i),
					Message: fmt.Sprintf("Plan item '%s' has a target value of $%.0f. Ensure this aligns with position sizing limits (max %.1f%% per position).",
						item.ID, item.TargetValue, strategy.PositionSizing.MaxPositionPct),
				})
			}
		}
	}

	return warnings
}

// extractExchange extracts the exchange suffix from a ticker (e.g., "BHP.AU" -> "AU")
func extractExchange(ticker string) string {
	parts := strings.Split(ticker, ".")
	if len(parts) >= 2 {
		return parts[len(parts)-1]
	}
	return ""
}

// evaluateConditionSimple evaluates a single rule condition against available data.
// Simplified version that works with signal/fundamental data directly.
func evaluateConditionSimple(cond models.RuleCondition, signals *models.TickerSignals, fundamentals *models.Fundamentals) bool {
	actual, ok := resolveFieldSimple(cond.Field, signals, fundamentals)
	if !ok {
		return false
	}
	return compareSimple(actual, cond.Operator, cond.Value)
}

// resolveFieldSimple resolves a dot-path field from signals/fundamentals data.
func resolveFieldSimple(field string, signals *models.TickerSignals, fundamentals *models.Fundamentals) (interface{}, bool) {
	parts := strings.SplitN(field, ".", 2)
	if len(parts) < 2 {
		return nil, false
	}

	switch parts[0] {
	case "signals":
		if signals == nil {
			return nil, false
		}
		return resolveSignalFieldSimple(parts[1], signals)
	case "fundamentals":
		if fundamentals == nil {
			return nil, false
		}
		return resolveFundamentalsFieldSimple(parts[1], fundamentals)
	}
	return nil, false
}

func resolveSignalFieldSimple(field string, sig *models.TickerSignals) (interface{}, bool) {
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
	case "trend":
		return string(sig.Trend), true
	case "price":
		if len(parts) < 2 {
			return nil, false
		}
		switch parts[1] {
		case "distance_to_sma20":
			return sig.Price.DistanceToSMA20, true
		case "distance_to_sma50":
			return sig.Price.DistanceToSMA50, true
		case "distance_to_sma200":
			return sig.Price.DistanceToSMA200, true
		}
	case "regime":
		if len(parts) < 2 {
			return nil, false
		}
		if parts[1] == "current" {
			return string(sig.Regime.Current), true
		}
	}
	return nil, false
}

func resolveFundamentalsFieldSimple(field string, f *models.Fundamentals) (interface{}, bool) {
	switch field {
	case "pe":
		return f.PE, true
	case "pb":
		return f.PB, true
	case "eps":
		return f.EPS, true
	case "dividend_yield":
		return f.DividendYield, true
	case "market_cap":
		return f.MarketCap, true
	case "sector":
		return f.Sector, true
	}
	return nil, false
}

// compareSimple compares actual vs expected using the given operator.
func compareSimple(actual interface{}, op models.RuleOperator, expected interface{}) bool {
	// String comparisons
	if as, ok := actual.(string); ok {
		es := fmt.Sprintf("%v", expected)
		switch op {
		case models.RuleOpEQ:
			return strings.EqualFold(as, es)
		case models.RuleOpNE:
			return !strings.EqualFold(as, es)
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
	}
	return 0
}
