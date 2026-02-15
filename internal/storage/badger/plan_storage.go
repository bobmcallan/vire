package badger

import (
	"context"
	"fmt"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/timshannon/badgerhold/v4"
)

type planStorage struct {
	store  *Store
	logger *common.Logger
}

// NewPlanStorage creates a new PlanStorage backed by BadgerHold.
func NewPlanStorage(store *Store, logger *common.Logger) *planStorage {
	return &planStorage{store: store, logger: logger}
}

func (s *planStorage) GetPlan(_ context.Context, portfolioName string) (*models.PortfolioPlan, error) {
	var plan models.PortfolioPlan
	err := s.store.db.Get(portfolioName, &plan)
	if err != nil {
		if err == badgerhold.ErrNotFound {
			return nil, fmt.Errorf("plan for '%s' not found", portfolioName)
		}
		return nil, fmt.Errorf("failed to get plan for '%s': %w", portfolioName, err)
	}
	return &plan, nil
}

func (s *planStorage) SavePlan(_ context.Context, plan *models.PortfolioPlan) error {
	// Read existing to preserve CreatedAt and increment Version
	var existing models.PortfolioPlan
	err := s.store.db.Get(plan.PortfolioName, &existing)
	if err == nil {
		plan.CreatedAt = existing.CreatedAt
		plan.Version = existing.Version + 1
	} else {
		plan.Version = 1
		if plan.CreatedAt.IsZero() {
			plan.CreatedAt = time.Now()
		}
	}

	plan.UpdatedAt = time.Now()

	if err := s.store.db.Upsert(plan.PortfolioName, plan); err != nil {
		return fmt.Errorf("failed to save plan: %w", err)
	}
	s.logger.Debug().Str("portfolio", plan.PortfolioName).Int("version", plan.Version).Msg("Plan saved")
	return nil
}

func (s *planStorage) DeletePlan(_ context.Context, portfolioName string) error {
	err := s.store.db.Delete(portfolioName, models.PortfolioPlan{})
	if err != nil && err != badgerhold.ErrNotFound {
		return fmt.Errorf("failed to delete plan for '%s': %w", portfolioName, err)
	}
	s.logger.Debug().Str("portfolio", portfolioName).Msg("Plan deleted")
	return nil
}

func (s *planStorage) ListPlans(_ context.Context) ([]string, error) {
	var plans []models.PortfolioPlan
	if err := s.store.db.Find(&plans, nil); err != nil {
		return nil, fmt.Errorf("failed to list plans: %w", err)
	}
	names := make([]string, len(plans))
	for i, p := range plans {
		names[i] = p.PortfolioName
	}
	return names, nil
}
