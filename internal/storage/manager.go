// Package storage provides the top-level StorageManager that coordinates
// the 3 storage areas: internaldb, userdb, and marketfs.
package storage

import (
	"context"
	"fmt"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/storage/internaldb"
	"github.com/bobmcallan/vire/internal/storage/marketfs"
	"github.com/bobmcallan/vire/internal/storage/userdb"
)

// Manager implements interfaces.StorageManager using 3 storage areas.
type Manager struct {
	internal *internaldb.Store
	user     *userdb.Store
	market   *marketfs.Store
	logger   *common.Logger
}

// NewManager creates a new StorageManager with the 3 storage areas.
func NewManager(logger *common.Logger, config *common.Config) (*Manager, error) {
	internalStore, err := internaldb.NewStore(logger, config.Storage.Internal.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to create internal store: %w", err)
	}

	userStore, err := userdb.NewStore(logger, config.Storage.User.Path)
	if err != nil {
		internalStore.Close()
		return nil, fmt.Errorf("failed to create user store: %w", err)
	}

	marketStore, err := marketfs.NewMarketStore(logger, config.Storage.Market.Path)
	if err != nil {
		internalStore.Close()
		userStore.Close()
		return nil, fmt.Errorf("failed to create market store: %w", err)
	}

	logger.Info().
		Str("internal", config.Storage.Internal.Path).
		Str("user", config.Storage.User.Path).
		Str("market", config.Storage.Market.Path).
		Msg("Storage manager initialized (3 areas)")

	return &Manager{
		internal: internalStore,
		user:     userStore,
		market:   marketStore,
		logger:   logger,
	}, nil
}

func (m *Manager) InternalStore() interfaces.InternalStore {
	return m.internal
}

func (m *Manager) UserDataStore() interfaces.UserDataStore {
	return m.user
}

func (m *Manager) MarketDataStorage() interfaces.MarketDataStorage {
	return m.market.MarketDataStorage()
}

func (m *Manager) SignalStorage() interfaces.SignalStorage {
	return m.market.SignalStorage()
}

func (m *Manager) DataPath() string {
	return m.market.DataPath()
}

func (m *Manager) WriteRaw(subdir, key string, data []byte) error {
	return m.market.WriteRaw(subdir, key, data)
}

func (m *Manager) PurgeDerivedData(ctx context.Context) (map[string]int, error) {
	counts := make(map[string]int)

	// Purge user domain records: portfolio, report, search
	userCount, err := m.user.DeleteBySubjects(ctx, "portfolio", "report", "search")
	if err != nil {
		return counts, fmt.Errorf("failed to purge user data: %w", err)
	}
	counts["user_records"] = userCount

	// Purge market data files
	counts["market"] = m.market.PurgeMarket()
	counts["signals"] = m.market.PurgeSignals()
	counts["charts"] = m.market.PurgeCharts()

	m.logger.Info().
		Int("user_records", counts["user_records"]).
		Int("market", counts["market"]).
		Int("signals", counts["signals"]).
		Int("charts", counts["charts"]).
		Msg("Derived data purged")

	return counts, nil
}

func (m *Manager) PurgeReports(ctx context.Context) (int, error) {
	count, err := m.user.DeleteBySubject(ctx, "report")
	if err != nil {
		return 0, fmt.Errorf("failed to purge reports: %w", err)
	}
	m.logger.Info().Int("count", count).Msg("Reports purged")
	return count, nil
}

func (m *Manager) Close() error {
	var firstErr error
	if err := m.internal.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := m.user.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := m.market.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

// Compile-time check
var _ interfaces.StorageManager = (*Manager)(nil)
