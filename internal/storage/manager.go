// Package storage provides file-based JSON persistence
package storage

import (
	"context"
	"path/filepath"

	"github.com/bobmccarthy/vire/internal/common"
	"github.com/bobmccarthy/vire/internal/interfaces"
)

// Manager coordinates all storage backends
type Manager struct {
	fs            *FileStore
	portfolio     interfaces.PortfolioStorage
	marketData    interfaces.MarketDataStorage
	signal        interfaces.SignalStorage
	kv            interfaces.KeyValueStorage
	report        interfaces.ReportStorage
	strategy      interfaces.StrategyStorage
	plan          interfaces.PlanStorage
	searchHistory interfaces.SearchHistoryStorage
	watchlist     interfaces.WatchlistStorage
	logger        *common.Logger
}

// NewStorageManager creates a new storage manager
func NewStorageManager(logger *common.Logger, config *common.Config) (interfaces.StorageManager, error) {
	fs, err := NewFileStore(logger, &config.Storage.File)
	if err != nil {
		return nil, err
	}

	manager := &Manager{
		fs:            fs,
		portfolio:     newPortfolioStorage(fs, logger),
		marketData:    newMarketDataStorage(fs, logger),
		signal:        newSignalStorage(fs, logger),
		kv:            newKVStorage(fs, logger),
		report:        newReportStorage(fs, logger),
		strategy:      newStrategyStorage(fs, logger),
		plan:          newPlanStorage(fs, logger),
		searchHistory: newSearchHistoryStorage(fs, logger),
		watchlist:     newWatchlistStorage(fs, logger),
		logger:        logger,
	}

	logger.Debug().Msg("Storage manager initialized")
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
		"portfolios":     m.fs.purgeDir(filepath.Join(m.fs.basePath, "portfolios")),
		"market_data":    m.fs.purgeDir(filepath.Join(m.fs.basePath, "market")),
		"signals":        m.fs.purgeDir(filepath.Join(m.fs.basePath, "signals")),
		"reports":        m.fs.purgeDir(filepath.Join(m.fs.basePath, "reports")),
		"search_history": m.fs.purgeDir(filepath.Join(m.fs.basePath, "searches")),
		"charts":         m.fs.purgeAllFiles(filepath.Join(m.fs.basePath, "charts")),
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

// DataPath returns the base data directory path.
func (m *Manager) DataPath() string {
	return m.fs.basePath
}

// WriteRaw writes arbitrary binary data to a subdirectory atomically.
func (m *Manager) WriteRaw(subdir, key string, data []byte) error {
	return m.fs.WriteRaw(subdir, key, data)
}

// Close closes all storage backends (no-op for file storage)
func (m *Manager) Close() error {
	return nil
}
