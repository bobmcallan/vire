// Package storage provides blob-based persistence with pluggable backends.
package storage

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
	bstore "github.com/bobmcallan/vire/internal/storage/badger"
)

// Manager coordinates all storage backends.
// User data (portfolios, strategies, plans, etc.) is stored in BadgerHold.
// Shared reference data (market, signals, charts) is stored in FileStore.
type Manager struct {
	badgerStore *bstore.Store // Per-user data (BadgerHold)
	dataStore   *FileStore    // Shared reference data (file-based)

	// Domain storage — each backed by the appropriate store
	portfolio     interfaces.PortfolioStorage     // → badgerStore
	strategy      interfaces.StrategyStorage      // → badgerStore
	plan          interfaces.PlanStorage          // → badgerStore
	watchlist     interfaces.WatchlistStorage     // → badgerStore
	report        interfaces.ReportStorage        // → badgerStore
	searchHistory interfaces.SearchHistoryStorage // → badgerStore
	kv            interfaces.KeyValueStorage      // → badgerStore
	user          interfaces.UserStorage          // → badgerStore

	marketData interfaces.MarketDataStorage // → dataStore
	signal     interfaces.SignalStorage     // → dataStore

	logger *common.Logger
}

// NewStorageManager creates a new storage manager with BadgerHold for user data
// and FileStore for shared reference data.
func NewStorageManager(logger *common.Logger, config *common.Config) (interfaces.StorageManager, error) {
	// Run migration from old flat layout if needed
	if err := MigrateToSplitLayout(logger, config); err != nil {
		logger.Warn().Err(err).Msg("Storage migration failed (continuing with new layout)")
	}

	// Create BadgerHold store for user data
	badgerStore, err := bstore.NewStore(logger, config.Storage.UserData.Path)
	if err != nil {
		return nil, fmt.Errorf("badger store: %w", err)
	}

	// Migrate file-based user data to BadgerDB if old directories exist
	if err := bstore.MigrateFromFiles(logger, badgerStore, config.Storage.UserData.Path); err != nil {
		logger.Warn().Err(err).Msg("File-to-BadgerDB migration failed (continuing)")
	}

	// Create FileStore for shared reference data
	dataStore, err := NewFileStore(logger, &config.Storage.Data)
	if err != nil {
		badgerStore.Close()
		return nil, fmt.Errorf("data store: %w", err)
	}

	manager := &Manager{
		badgerStore: badgerStore,
		dataStore:   dataStore,

		// User data — backed by BadgerHold
		portfolio:     bstore.NewPortfolioStorage(badgerStore, logger),
		strategy:      bstore.NewStrategyStorage(badgerStore, logger),
		plan:          bstore.NewPlanStorage(badgerStore, logger),
		watchlist:     bstore.NewWatchlistStorage(badgerStore, logger),
		report:        bstore.NewReportStorage(badgerStore, logger),
		searchHistory: bstore.NewSearchHistoryStorage(badgerStore, logger),
		kv:            bstore.NewKVStorage(badgerStore, logger),
		user:          bstore.NewUserStorage(badgerStore, logger),

		// Shared data — backed by FileStore
		marketData: newMarketDataStorage(dataStore, logger),
		signal:     newSignalStorage(dataStore, logger),

		logger: logger,
	}

	logger.Debug().
		Str("backend", config.Storage.Backend).
		Str("user_data_path", config.Storage.UserData.Path).
		Str("data_path", config.Storage.Data.Path).
		Msg("Storage manager initialized")
	return manager, nil
}

// PortfolioStorage returns the portfolio storage backend
func (m *Manager) PortfolioStorage() interfaces.PortfolioStorage {
	return m.portfolio
}

// MarketDataStorage returns the market data storage backend
func (m *Manager) MarketDataStorage() interfaces.MarketDataStorage {
	return m.marketData
}

// SignalStorage returns the signal storage backend
func (m *Manager) SignalStorage() interfaces.SignalStorage {
	return m.signal
}

// KeyValueStorage returns the key-value storage backend
func (m *Manager) KeyValueStorage() interfaces.KeyValueStorage {
	return m.kv
}

// ReportStorage returns the report storage backend
func (m *Manager) ReportStorage() interfaces.ReportStorage {
	return m.report
}

// StrategyStorage returns the strategy storage backend
func (m *Manager) StrategyStorage() interfaces.StrategyStorage {
	return m.strategy
}

// PlanStorage returns the plan storage backend
func (m *Manager) PlanStorage() interfaces.PlanStorage {
	return m.plan
}

// SearchHistoryStorage returns the search history storage backend
func (m *Manager) SearchHistoryStorage() interfaces.SearchHistoryStorage {
	return m.searchHistory
}

// WatchlistStorage returns the watchlist storage backend
func (m *Manager) WatchlistStorage() interfaces.WatchlistStorage {
	return m.watchlist
}

// UserStorage returns the user storage backend
func (m *Manager) UserStorage() interfaces.UserStorage {
	return m.user
}

// PurgeDerivedData deletes all derived/cached data while preserving user data.
// Derived: portfolios, market data, signals, reports, search history, charts.
// Preserved: strategies, KV entries, plans, watchlists.
func (m *Manager) PurgeDerivedData(ctx context.Context) (map[string]int, error) {
	// Purge BadgerHold user-domain derived data
	portfolioCount := deleteAllByType[models.Portfolio](m.badgerStore)
	reportCount := deleteAllByType[models.PortfolioReport](m.badgerStore)
	searchCount := deleteAllByType[models.SearchRecord](m.badgerStore)

	counts := map[string]int{
		// User-store derived data (BadgerHold)
		"portfolios":     portfolioCount,
		"reports":        reportCount,
		"search_history": searchCount,
		// Data-store derived data (file-based)
		"market_data": m.dataStore.purgeDir(filepath.Join(m.dataStore.basePath, "market")),
		"signals":     m.dataStore.purgeDir(filepath.Join(m.dataStore.basePath, "signals")),
		"charts":      m.dataStore.purgeAllFiles(filepath.Join(m.dataStore.basePath, "charts")),
	}

	total := counts["portfolios"] + counts["market_data"] + counts["signals"] + counts["reports"] + counts["search_history"] + counts["charts"]
	m.logger.Info().
		Int("portfolios", counts["portfolios"]).
		Int("market_data", counts["market_data"]).
		Int("signals", counts["signals"]).
		Int("reports", counts["reports"]).
		Int("search_history", counts["search_history"]).
		Int("charts", counts["charts"]).
		Int("total", total).
		Msg("Derived data purged")

	return counts, nil
}

// DataPath returns the shared data directory path (used for chart output).
func (m *Manager) DataPath() string {
	return m.dataStore.basePath
}

// PurgeReports deletes only cached reports (used by dev mode on build change).
// Returns count of deleted reports.
func (m *Manager) PurgeReports(ctx context.Context) (int, error) {
	count := deleteAllByType[models.PortfolioReport](m.badgerStore)
	m.logger.Info().Int("reports", count).Msg("Reports purged")
	return count, nil
}

// WriteRaw writes arbitrary binary data to a subdirectory atomically.
// Routes through the data store (used for charts).
func (m *Manager) WriteRaw(subdir, key string, data []byte) error {
	return m.dataStore.WriteRaw(subdir, key, data)
}

// Close closes all storage backends.
func (m *Manager) Close() error {
	var errs []error
	if err := m.badgerStore.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := m.dataStore.Close(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// deleteAllByType deletes all records of a given type from BadgerHold.
// Returns the count of deleted records.
func deleteAllByType[T any](store *bstore.Store) int {
	var items []T
	if err := store.DB().Find(&items, nil); err != nil {
		return 0
	}
	count := len(items)
	if count > 0 {
		var zero T
		store.DB().DeleteMatching(zero, nil)
	}
	return count
}
