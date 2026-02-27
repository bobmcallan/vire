package surrealdb

import (
	"context"
	"fmt"
	"os"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/surrealdb/surrealdb.go"
)

// Manager implements interfaces.StorageManager using SurrealDB.
type Manager struct {
	db       *surrealdb.DB
	logger   *common.Logger
	dataPath string

	internalStore   *InternalStore
	userStore       *UserStore
	marketStore     *MarketStore
	stockIndexStore *StockIndexStore
	jobQueueStore   *JobQueueStore
	fileStore       *FileStore
	feedbackStore   *FeedbackStore
	oauthStore      *OAuthStore
}

// NewManager creates a new StorageManager connected to SurrealDB.
func NewManager(logger *common.Logger, config *common.Config) (*Manager, error) {
	ctx := context.Background()

	// Connect to SurrealDB
	db, err := surrealdb.New(config.Storage.Address)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to SurrealDB: %w", err)
	}

	// Sign in
	if _, err := db.SignIn(ctx, map[string]interface{}{
		"user": config.Storage.Username,
		"pass": config.Storage.Password,
	}); err != nil {
		return nil, fmt.Errorf("failed to sign in to SurrealDB: %w", err)
	}

	// Select namespace and database
	if err := db.Use(ctx, config.Storage.Namespace, config.Storage.Database); err != nil {
		return nil, fmt.Errorf("failed to select namespace/database: %w", err)
	}

	// Define tables to ensure they exist (SurrealDB v3 errors on querying non-existent tables)
	tables := []string{"user", "user_kv", "system_kv", "user_data", "market_data", "signals", "job_runs", "stock_index", "job_queue", "files", "mcp_feedback", "oauth_client", "oauth_code", "oauth_refresh_token", "mcp_auth_session"}
	for _, table := range tables {
		sql := fmt.Sprintf("DEFINE TABLE IF NOT EXISTS %s SCHEMALESS", table)
		if _, err := surrealdb.Query[any](ctx, db, sql, nil); err != nil {
			return nil, fmt.Errorf("failed to define table %s: %w", table, err)
		}
	}

	// Ensure DataPath exists (for fallback raw writes like charts)
	dataPath := config.Storage.DataPath
	if dataPath == "" {
		dataPath = "data/market"
	}
	if err := os.MkdirAll(dataPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data path: %w", err)
	}

	m := &Manager{
		db:       db,
		logger:   logger,
		dataPath: dataPath,
	}

	// Init stores
	m.internalStore = NewInternalStore(db, logger)
	m.userStore = NewUserStore(db, logger)
	m.marketStore = NewMarketStore(db, logger, dataPath)
	m.stockIndexStore = NewStockIndexStore(db, logger)
	m.jobQueueStore = NewJobQueueStore(db, logger)
	m.fileStore = NewFileStore(db, logger)
	m.feedbackStore = NewFeedbackStore(db, logger)
	m.oauthStore = NewOAuthStore(db, logger)

	logger.Info().
		Str("address", config.Storage.Address).
		Str("namespace", config.Storage.Namespace).
		Str("database", config.Storage.Database).
		Msg("SurrealDB storage manager initialized")

	return m, nil
}

func (m *Manager) InternalStore() interfaces.InternalStore {
	return m.internalStore
}

func (m *Manager) UserDataStore() interfaces.UserDataStore {
	return m.userStore
}

func (m *Manager) MarketDataStorage() interfaces.MarketDataStorage {
	return m.marketStore
}

func (m *Manager) SignalStorage() interfaces.SignalStorage {
	return m.marketStore
}

func (m *Manager) StockIndexStore() interfaces.StockIndexStore {
	return m.stockIndexStore
}

func (m *Manager) JobQueueStore() interfaces.JobQueueStore {
	return m.jobQueueStore
}

func (m *Manager) FileStore() interfaces.FileStore {
	return m.fileStore
}

func (m *Manager) FeedbackStore() interfaces.FeedbackStore {
	return m.feedbackStore
}

func (m *Manager) OAuthStore() interfaces.OAuthStore {
	return m.oauthStore
}

func (m *Manager) DataPath() string {
	return m.dataPath
}

// WriteRaw stores binary data (e.g. charts) in the database via FileStore.
func (m *Manager) WriteRaw(subdir, key string, data []byte) error {
	return m.fileStore.SaveFile(context.Background(), "chart", subdir+"/"+key, data, "application/octet-stream")
}

func (m *Manager) PurgeDerivedData(ctx context.Context) (map[string]int, error) {
	counts := make(map[string]int)

	// In SurrealDB, we delete records by their table name.
	// We'll let UserStore, MarketStore handle these.

	// User data purge: portfolio, report, search
	userCount, err := m.userStore.DeleteBySubjects(ctx, "portfolio", "report", "search")
	if err != nil {
		return counts, fmt.Errorf("failed to purge user data: %w", err)
	}
	counts["user_records"] = userCount

	// Market & Signal purge
	marketCount, err := m.marketStore.PurgeMarketData(ctx)
	if err != nil {
		m.logger.Warn().Err(err).Msg("Failed to purge market data")
	}
	counts["market"] = marketCount

	signalCount, err := m.marketStore.PurgeSignalsData(ctx)
	if err != nil {
		m.logger.Warn().Err(err).Msg("Failed to purge signals data")
	}
	counts["signals"] = signalCount

	// Charts purge
	chartsCount, err := m.marketStore.PurgeCharts()
	if err != nil {
		m.logger.Warn().Err(err).Msg("Failed to purge charts")
	}
	counts["charts"] = chartsCount

	m.logger.Info().
		Int("user_records", counts["user_records"]).
		Int("market", counts["market"]).
		Int("signals", counts["signals"]).
		Int("charts", counts["charts"]).
		Msg("Derived data purged")

	return counts, nil
}

func (m *Manager) PurgeReports(ctx context.Context) (int, error) {
	count, err := m.userStore.DeleteBySubject(ctx, "report")
	if err != nil {
		return 0, fmt.Errorf("failed to purge reports: %w", err)
	}
	m.logger.Info().Int("count", count).Msg("Reports purged")
	return count, nil
}

func (m *Manager) Close() error {
	m.db.Close(context.Background())
	return nil
}

// Compile-time check
var _ interfaces.StorageManager = (*Manager)(nil)
