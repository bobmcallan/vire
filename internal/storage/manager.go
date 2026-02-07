// Package storage provides BadgerDB-based persistence
package storage

import (
	"github.com/bobmccarthy/vire/internal/common"
	"github.com/bobmccarthy/vire/internal/interfaces"
)

// Manager coordinates all storage backends
type Manager struct {
	db         *BadgerDB
	portfolio  interfaces.PortfolioStorage
	marketData interfaces.MarketDataStorage
	signal     interfaces.SignalStorage
	kv         interfaces.KeyValueStorage
	report     interfaces.ReportStorage
	strategy   interfaces.StrategyStorage
	logger     *common.Logger
}

// NewStorageManager creates a new storage manager
func NewStorageManager(logger *common.Logger, config *common.Config) (interfaces.StorageManager, error) {
	db, err := NewBadgerDB(logger, &config.Storage.Badger)
	if err != nil {
		return nil, err
	}

	manager := &Manager{
		db:         db,
		portfolio:  newPortfolioStorage(db, logger),
		marketData: newMarketDataStorage(db, logger),
		signal:     newSignalStorage(db, logger),
		kv:         newKVStorage(db, logger),
		report:     newReportStorage(db, logger),
		strategy:   newStrategyStorage(db, logger),
		logger:     logger,
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

// Close closes all storage backends
func (m *Manager) Close() error {
	if m.db != nil {
		return m.db.Close()
	}
	return nil
}
