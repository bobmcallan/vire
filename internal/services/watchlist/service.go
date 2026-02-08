// Package watchlist provides portfolio watchlist management services
package watchlist

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bobmccarthy/vire/internal/common"
	"github.com/bobmccarthy/vire/internal/interfaces"
	"github.com/bobmccarthy/vire/internal/models"
)

// Compile-time interface check
var _ interfaces.WatchlistService = (*Service)(nil)

// Service implements WatchlistService
type Service struct {
	storage interfaces.StorageManager
	logger  *common.Logger
}

// NewService creates a new watchlist service
func NewService(storage interfaces.StorageManager, logger *common.Logger) *Service {
	return &Service{
		storage: storage,
		logger:  logger,
	}
}

// GetWatchlist retrieves the watchlist for a portfolio
func (s *Service) GetWatchlist(ctx context.Context, portfolioName string) (*models.PortfolioWatchlist, error) {
	wl, err := s.storage.WatchlistStorage().GetWatchlist(ctx, portfolioName)
	if err != nil {
		return nil, fmt.Errorf("failed to get watchlist: %w", err)
	}
	return wl, nil
}

// SaveWatchlist saves a watchlist with version increment
func (s *Service) SaveWatchlist(ctx context.Context, watchlist *models.PortfolioWatchlist) error {
	err := s.storage.WatchlistStorage().SaveWatchlist(ctx, watchlist)
	if err != nil {
		return fmt.Errorf("failed to save watchlist: %w", err)
	}
	s.logger.Info().Str("portfolio", watchlist.PortfolioName).Msg("Watchlist saved")
	return nil
}

// DeleteWatchlist removes a watchlist
func (s *Service) DeleteWatchlist(ctx context.Context, portfolioName string) error {
	err := s.storage.WatchlistStorage().DeleteWatchlist(ctx, portfolioName)
	if err != nil {
		return fmt.Errorf("failed to delete watchlist: %w", err)
	}
	s.logger.Info().Str("portfolio", portfolioName).Msg("Watchlist deleted")
	return nil
}

// AddOrUpdateItem adds a new item or updates an existing one (upsert keyed on ticker)
func (s *Service) AddOrUpdateItem(ctx context.Context, portfolioName string, item *models.WatchlistItem) (*models.PortfolioWatchlist, error) {
	wl, err := s.storage.WatchlistStorage().GetWatchlist(ctx, portfolioName)
	if err != nil {
		// No existing watchlist — create one
		wl = &models.PortfolioWatchlist{
			PortfolioName: portfolioName,
			Items:         []models.WatchlistItem{},
		}
	}

	now := time.Now()

	existing, idx := wl.FindByTicker(item.Ticker)
	if idx >= 0 {
		// Update existing: preserve CreatedAt, update ReviewedAt only if verdict changed
		item.CreatedAt = existing.CreatedAt
		if item.Verdict != existing.Verdict {
			item.ReviewedAt = now
		} else if item.ReviewedAt.IsZero() {
			item.ReviewedAt = existing.ReviewedAt
		}
		item.UpdatedAt = now
		wl.Items[idx] = *item
	} else {
		// New item
		if item.CreatedAt.IsZero() {
			item.CreatedAt = now
		}
		if item.ReviewedAt.IsZero() {
			item.ReviewedAt = now
		}
		item.UpdatedAt = now
		wl.Items = append(wl.Items, *item)
	}

	if err := s.storage.WatchlistStorage().SaveWatchlist(ctx, wl); err != nil {
		return nil, fmt.Errorf("failed to save watchlist: %w", err)
	}

	s.logger.Info().Str("portfolio", portfolioName).Str("ticker", item.Ticker).Msg("Watchlist item upserted")
	return wl, nil
}

// UpdateItem updates an existing item by ticker (merge semantics — only overwrite non-zero fields)
func (s *Service) UpdateItem(ctx context.Context, portfolioName, ticker string, update *models.WatchlistItem) (*models.PortfolioWatchlist, error) {
	wl, err := s.storage.WatchlistStorage().GetWatchlist(ctx, portfolioName)
	if err != nil {
		return nil, fmt.Errorf("failed to get watchlist: %w", err)
	}

	ticker = strings.ToUpper(strings.TrimSpace(ticker))
	_, idx := wl.FindByTicker(ticker)
	if idx < 0 {
		return nil, fmt.Errorf("ticker '%s' not found in watchlist", ticker)
	}

	existing := &wl.Items[idx]
	now := time.Now()

	// Merge: only overwrite non-zero fields
	if update.Name != "" {
		existing.Name = update.Name
	}
	if update.Verdict != "" {
		if existing.Verdict != update.Verdict {
			existing.ReviewedAt = now
		}
		existing.Verdict = update.Verdict
	}
	if update.Reason != "" {
		existing.Reason = update.Reason
	}
	if update.KeyMetrics != "" {
		existing.KeyMetrics = update.KeyMetrics
	}
	if update.Notes != "" {
		existing.Notes = update.Notes
	}
	existing.UpdatedAt = now

	if err := s.storage.WatchlistStorage().SaveWatchlist(ctx, wl); err != nil {
		return nil, fmt.Errorf("failed to save watchlist: %w", err)
	}

	s.logger.Info().Str("portfolio", portfolioName).Str("ticker", ticker).Msg("Watchlist item updated")
	return wl, nil
}

// RemoveItem removes a stock from the watchlist by ticker
func (s *Service) RemoveItem(ctx context.Context, portfolioName, ticker string) (*models.PortfolioWatchlist, error) {
	wl, err := s.storage.WatchlistStorage().GetWatchlist(ctx, portfolioName)
	if err != nil {
		return nil, fmt.Errorf("failed to get watchlist: %w", err)
	}

	ticker = strings.ToUpper(strings.TrimSpace(ticker))
	_, idx := wl.FindByTicker(ticker)
	if idx < 0 {
		return nil, fmt.Errorf("ticker '%s' not found in watchlist", ticker)
	}

	wl.Items = append(wl.Items[:idx], wl.Items[idx+1:]...)

	if err := s.storage.WatchlistStorage().SaveWatchlist(ctx, wl); err != nil {
		return nil, fmt.Errorf("failed to save watchlist: %w", err)
	}

	s.logger.Info().Str("portfolio", portfolioName).Str("ticker", ticker).Msg("Watchlist item removed")
	return wl, nil
}
