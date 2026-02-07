// Package storage provides BadgerDB-based persistence
package storage

import (
	"context"

	"github.com/bobmccarthy/vire/internal/common"
	"github.com/bobmccarthy/vire/internal/interfaces"
	"github.com/bobmccarthy/vire/internal/models"
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

// PurgeDerivedData deletes all derived/cached data while preserving user data.
// Derived types: Portfolio, MarketData, TickerSignals, PortfolioReport.
// Preserved types: PortfolioStrategy, KV entries.
func (m *Manager) PurgeDerivedData(ctx context.Context) (map[string]int, error) {
	counts := map[string]int{
		"portfolios":  0,
		"market_data": 0,
		"signals":     0,
		"reports":     0,
	}

	store := m.db.Store()

	// Purge portfolios
	var portfolios []models.Portfolio
	if err := store.Find(&portfolios, nil); err == nil {
		for _, p := range portfolios {
			if err := store.Delete(p.ID, models.Portfolio{}); err == nil {
				counts["portfolios"]++
			}
		}
	}

	// Purge market data
	var marketData []models.MarketData
	if err := store.Find(&marketData, nil); err == nil {
		for _, md := range marketData {
			if err := store.Delete(md.Ticker, models.MarketData{}); err == nil {
				counts["market_data"]++
			}
		}
	}

	// Purge signals
	var signals []models.TickerSignals
	if err := store.Find(&signals, nil); err == nil {
		for _, s := range signals {
			if err := store.Delete(s.Ticker, models.TickerSignals{}); err == nil {
				counts["signals"]++
			}
		}
	}

	// Purge reports
	var reports []models.PortfolioReport
	if err := store.Find(&reports, nil); err == nil {
		for _, r := range reports {
			if err := store.Delete(r.Portfolio, models.PortfolioReport{}); err == nil {
				counts["reports"]++
			}
		}
	}

	total := counts["portfolios"] + counts["market_data"] + counts["signals"] + counts["reports"]
	m.logger.Info().
		Int("portfolios", counts["portfolios"]).
		Int("market_data", counts["market_data"]).
		Int("signals", counts["signals"]).
		Int("reports", counts["reports"]).
		Int("total", total).
		Msg("Derived data purged")

	return counts, nil
}

// Close closes all storage backends
func (m *Manager) Close() error {
	if m.db != nil {
		return m.db.Close()
	}
	return nil
}
