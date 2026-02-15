package badger

import (
	"context"
	"fmt"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/timshannon/badgerhold/v4"
)

type portfolioStorage struct {
	store  *Store
	logger *common.Logger
}

// NewPortfolioStorage creates a new PortfolioStorage backed by BadgerHold.
func NewPortfolioStorage(store *Store, logger *common.Logger) *portfolioStorage {
	return &portfolioStorage{store: store, logger: logger}
}

func (s *portfolioStorage) GetPortfolio(_ context.Context, name string) (*models.Portfolio, error) {
	var portfolio models.Portfolio
	err := s.store.db.Get(name, &portfolio)
	if err != nil {
		if err == badgerhold.ErrNotFound {
			return nil, fmt.Errorf("portfolio '%s' not found", name)
		}
		return nil, fmt.Errorf("failed to get portfolio '%s': %w", name, err)
	}
	return &portfolio, nil
}

func (s *portfolioStorage) SavePortfolio(_ context.Context, portfolio *models.Portfolio) error {
	portfolio.UpdatedAt = time.Now()
	if portfolio.CreatedAt.IsZero() {
		portfolio.CreatedAt = time.Now()
	}
	if portfolio.ID == "" {
		portfolio.ID = portfolio.Name
	}

	if err := s.store.db.Upsert(portfolio.ID, portfolio); err != nil {
		return fmt.Errorf("failed to save portfolio: %w", err)
	}
	s.logger.Debug().Str("name", portfolio.Name).Msg("Portfolio saved")
	return nil
}

func (s *portfolioStorage) ListPortfolios(_ context.Context) ([]string, error) {
	var portfolios []models.Portfolio
	if err := s.store.db.Find(&portfolios, nil); err != nil {
		return nil, fmt.Errorf("failed to list portfolios: %w", err)
	}
	names := make([]string, len(portfolios))
	for i, p := range portfolios {
		names[i] = p.ID
	}
	return names, nil
}

func (s *portfolioStorage) DeletePortfolio(_ context.Context, name string) error {
	err := s.store.db.Delete(name, models.Portfolio{})
	if err != nil && err != badgerhold.ErrNotFound {
		return fmt.Errorf("failed to delete portfolio '%s': %w", name, err)
	}
	s.logger.Debug().Str("name", name).Msg("Portfolio deleted")
	return nil
}
