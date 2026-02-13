// Package storage provides blob-based persistence with pluggable backends.
package storage

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
)

// Manager coordinates all storage backends.
// User data (portfolios, strategies, plans, etc.) is stored in userStore.
// Shared reference data (market, signals, charts) is stored in dataStore.
type Manager struct {
	userStore *FileStore // Per-user data
	dataStore *FileStore // Shared reference data

	// Domain storage — each backed by the appropriate store
	portfolio     interfaces.PortfolioStorage     // → userStore
	strategy      interfaces.StrategyStorage      // → userStore
	plan          interfaces.PlanStorage          // → userStore
	watchlist     interfaces.WatchlistStorage     // → userStore
	report        interfaces.ReportStorage        // → userStore
	searchHistory interfaces.SearchHistoryStorage // → userStore
	kv            interfaces.KeyValueStorage      // → userStore

	marketData interfaces.MarketDataStorage // → dataStore
	signal     interfaces.SignalStorage     // → dataStore

	logger *common.Logger
}

// NewStorageManager creates a new storage manager with separate user and data stores.
func NewStorageManager(logger *common.Logger, config *common.Config) (interfaces.StorageManager, error) {
	// Run migration from old flat layout if needed
	if err := MigrateToSplitLayout(logger, config); err != nil {
		logger.Warn().Err(err).Msg("Storage migration failed (continuing with new layout)")
	}

	userStore, err := NewFileStore(logger, &config.Storage.UserData)
	if err != nil {
		return nil, fmt.Errorf("user store: %w", err)
	}

	dataStore, err := NewFileStore(logger, &config.Storage.Data)
	if err != nil {
		return nil, fmt.Errorf("data store: %w", err)
	}

	manager := &Manager{
		userStore: userStore,
		dataStore: dataStore,

		// User data — backed by userStore
		portfolio:     newPortfolioStorage(userStore, logger),
		strategy:      newStrategyStorage(userStore, logger),
		plan:          newPlanStorage(userStore, logger),
		watchlist:     newWatchlistStorage(userStore, logger),
		report:        newReportStorage(userStore, logger),
		searchHistory: newSearchHistoryStorage(userStore, logger),
		kv:            newKVStorage(userStore, logger),

		// Shared data — backed by dataStore
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

// PurgeDerivedData deletes all derived/cached data while preserving user data.
// Derived: portfolios, market data, signals, reports, search history, charts.
// Preserved: strategies, KV entries, plans, watchlists.
func (m *Manager) PurgeDerivedData(ctx context.Context) (map[string]int, error) {
	counts := map[string]int{
		// User-store derived data
		"portfolios":     m.userStore.purgeDir(filepath.Join(m.userStore.basePath, "portfolios")),
		"reports":        m.userStore.purgeDir(filepath.Join(m.userStore.basePath, "reports")),
		"search_history": m.userStore.purgeDir(filepath.Join(m.userStore.basePath, "searches")),
		// Data-store derived data
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
	count := m.userStore.purgeDir(filepath.Join(m.userStore.basePath, "reports"))
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
	if err := m.userStore.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := m.dataStore.Close(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
