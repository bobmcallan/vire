package badger

import (
	"context"
	"fmt"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/timshannon/badgerhold/v4"
)

type watchlistStorage struct {
	store  *Store
	logger *common.Logger
}

// NewWatchlistStorage creates a new WatchlistStorage backed by BadgerHold.
func NewWatchlistStorage(store *Store, logger *common.Logger) *watchlistStorage {
	return &watchlistStorage{store: store, logger: logger}
}

func (s *watchlistStorage) GetWatchlist(_ context.Context, portfolioName string) (*models.PortfolioWatchlist, error) {
	var watchlist models.PortfolioWatchlist
	err := s.store.db.Get(portfolioName, &watchlist)
	if err != nil {
		if err == badgerhold.ErrNotFound {
			return nil, fmt.Errorf("watchlist for '%s' not found", portfolioName)
		}
		return nil, fmt.Errorf("failed to get watchlist for '%s': %w", portfolioName, err)
	}
	return &watchlist, nil
}

func (s *watchlistStorage) SaveWatchlist(_ context.Context, watchlist *models.PortfolioWatchlist) error {
	// Read existing to preserve CreatedAt and increment Version
	var existing models.PortfolioWatchlist
	err := s.store.db.Get(watchlist.PortfolioName, &existing)
	if err == nil {
		watchlist.CreatedAt = existing.CreatedAt
		watchlist.Version = existing.Version + 1
	} else {
		watchlist.Version = 1
		if watchlist.CreatedAt.IsZero() {
			watchlist.CreatedAt = time.Now()
		}
	}

	watchlist.UpdatedAt = time.Now()

	if err := s.store.db.Upsert(watchlist.PortfolioName, watchlist); err != nil {
		return fmt.Errorf("failed to save watchlist: %w", err)
	}
	s.logger.Debug().Str("portfolio", watchlist.PortfolioName).Int("version", watchlist.Version).Msg("Watchlist saved")
	return nil
}

func (s *watchlistStorage) DeleteWatchlist(_ context.Context, portfolioName string) error {
	err := s.store.db.Delete(portfolioName, models.PortfolioWatchlist{})
	if err != nil && err != badgerhold.ErrNotFound {
		return fmt.Errorf("failed to delete watchlist for '%s': %w", portfolioName, err)
	}
	s.logger.Debug().Str("portfolio", portfolioName).Msg("Watchlist deleted")
	return nil
}

func (s *watchlistStorage) ListWatchlists(_ context.Context) ([]string, error) {
	var watchlists []models.PortfolioWatchlist
	if err := s.store.db.Find(&watchlists, nil); err != nil {
		return nil, fmt.Errorf("failed to list watchlists: %w", err)
	}
	names := make([]string, len(watchlists))
	for i, w := range watchlists {
		names[i] = w.PortfolioName
	}
	return names, nil
}
