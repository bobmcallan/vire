package badger

import (
	"context"
	"fmt"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/timshannon/badgerhold/v4"
)

type strategyStorage struct {
	store  *Store
	logger *common.Logger
}

// NewStrategyStorage creates a new StrategyStorage backed by BadgerHold.
func NewStrategyStorage(store *Store, logger *common.Logger) *strategyStorage {
	return &strategyStorage{store: store, logger: logger}
}

func (s *strategyStorage) GetStrategy(_ context.Context, portfolioName string) (*models.PortfolioStrategy, error) {
	var strategy models.PortfolioStrategy
	err := s.store.db.Get(portfolioName, &strategy)
	if err != nil {
		if err == badgerhold.ErrNotFound {
			return nil, fmt.Errorf("strategy for '%s' not found", portfolioName)
		}
		return nil, fmt.Errorf("failed to get strategy for '%s': %w", portfolioName, err)
	}
	return &strategy, nil
}

func (s *strategyStorage) SaveStrategy(_ context.Context, strategy *models.PortfolioStrategy) error {
	// Read existing to preserve CreatedAt and increment Version
	var existing models.PortfolioStrategy
	err := s.store.db.Get(strategy.PortfolioName, &existing)
	if err == nil {
		// Existing strategy: preserve CreatedAt, increment Version
		strategy.CreatedAt = existing.CreatedAt
		strategy.Version = existing.Version + 1
	} else {
		// New strategy
		strategy.Version = 1
		if strategy.CreatedAt.IsZero() {
			strategy.CreatedAt = time.Now()
		}
		if strategy.Disclaimer == "" {
			strategy.Disclaimer = models.DefaultDisclaimer
		}
	}

	strategy.UpdatedAt = time.Now()

	if err := s.store.db.Upsert(strategy.PortfolioName, strategy); err != nil {
		return fmt.Errorf("failed to save strategy: %w", err)
	}
	s.logger.Debug().Str("portfolio", strategy.PortfolioName).Int("version", strategy.Version).Msg("Strategy saved")
	return nil
}

func (s *strategyStorage) DeleteStrategy(_ context.Context, portfolioName string) error {
	err := s.store.db.Delete(portfolioName, models.PortfolioStrategy{})
	if err != nil && err != badgerhold.ErrNotFound {
		return fmt.Errorf("failed to delete strategy for '%s': %w", portfolioName, err)
	}
	s.logger.Debug().Str("portfolio", portfolioName).Msg("Strategy deleted")
	return nil
}

func (s *strategyStorage) ListStrategies(_ context.Context) ([]string, error) {
	var strategies []models.PortfolioStrategy
	if err := s.store.db.Find(&strategies, nil); err != nil {
		return nil, fmt.Errorf("failed to list strategies: %w", err)
	}
	names := make([]string, len(strategies))
	for i, s := range strategies {
		names[i] = s.PortfolioName
	}
	return names, nil
}
