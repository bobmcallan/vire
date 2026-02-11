// Package storage provides blob-based persistence with pluggable backends.
package storage

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/bobmccarthy/vire/internal/common"
	"github.com/bobmccarthy/vire/internal/interfaces"
)

// Manager coordinates all storage backends.
// It provides both high-level domain storage (portfolios, strategies, etc.)
// and low-level blob storage for raw data access.
type Manager struct {
	blob          BlobStore  // Provider-agnostic blob storage
	fs            *FileStore // Legacy file store (used by domain storage)
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

// NewStorageManager creates a new storage manager.
// The storage backend is determined by config.Storage.Backend:
//   - "file" (default): Local filesystem storage
//   - "gcs": Google Cloud Storage (future)
//   - "s3": AWS S3 or S3-compatible storage (future)
func NewStorageManager(logger *common.Logger, config *common.Config) (interfaces.StorageManager, error) {
	// Create the blob store based on backend configuration
	blobConfig := &BlobStoreConfig{
		Backend: config.Storage.Backend,
		File: FileBlobConfig{
			BasePath: config.Storage.File.Path,
		},
		GCS: GCSBlobConfig{
			Bucket:          config.Storage.GCS.Bucket,
			Prefix:          config.Storage.GCS.Prefix,
			CredentialsFile: config.Storage.GCS.CredentialsFile,
		},
		S3: S3BlobConfig{
			Bucket:    config.Storage.S3.Bucket,
			Prefix:    config.Storage.S3.Prefix,
			Region:    config.Storage.S3.Region,
			Endpoint:  config.Storage.S3.Endpoint,
			AccessKey: config.Storage.S3.AccessKey,
			SecretKey: config.Storage.S3.SecretKey,
		},
	}

	blob, err := NewBlobStore(logger, blobConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create blob store: %w", err)
	}

	// Create legacy FileStore for domain storage (maintains backward compatibility)
	// The FileStore uses the same base path as the blob store
	fs, err := NewFileStore(logger, &config.Storage.File)
	if err != nil {
		blob.Close()
		return nil, fmt.Errorf("failed to create file store: %w", err)
	}

	manager := &Manager{
		blob:          blob,
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

	logger.Debug().
		Str("backend", config.Storage.Backend).
		Str("path", config.Storage.File.Path).
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

// PurgeReports deletes only cached reports (used by dev mode on build change).
// Returns count of deleted reports.
func (m *Manager) PurgeReports(ctx context.Context) (int, error) {
	count := m.fs.purgeDir(filepath.Join(m.fs.basePath, "reports"))
	m.logger.Info().Int("reports", count).Msg("Reports purged")
	return count, nil
}

// WriteRaw writes arbitrary binary data to a subdirectory atomically.
func (m *Manager) WriteRaw(subdir, key string, data []byte) error {
	return m.fs.WriteRaw(subdir, key, data)
}

// BlobStore returns the underlying blob store for raw access.
// Use this for direct blob operations or when implementing new storage features.
func (m *Manager) BlobStore() BlobStore {
	return m.blob
}

// Close closes all storage backends.
func (m *Manager) Close() error {
	if m.blob != nil {
		return m.blob.Close()
	}
	return nil
}
