// Package interfaces defines service contracts for Vire
package interfaces

import (
	"context"

	"github.com/bobmcallan/vire/internal/models"
)

// StorageManager coordinates all storage backends
type StorageManager interface {
	// Storage accessors
	InternalStore() InternalStore
	UserDataStore() UserDataStore
	MarketDataStorage() MarketDataStorage
	SignalStorage() SignalStorage

	// DataPath returns the base data directory path (e.g. /app/data/market).
	DataPath() string

	// WriteRaw writes arbitrary binary data to a subdirectory atomically.
	// Key is sanitized for safe filenames (e.g. "smsf-growth.png").
	WriteRaw(subdir, key string, data []byte) error

	// PurgeDerivedData deletes all derived/cached data (Portfolio, MarketData,
	// Signals, Reports) while preserving user data (Strategy, Plans, Watchlists).
	// Returns counts of deleted items per type.
	PurgeDerivedData(ctx context.Context) (map[string]int, error)

	// PurgeReports deletes only cached reports (used by dev mode on build change).
	// Returns count of deleted reports.
	PurgeReports(ctx context.Context) (int, error)

	// Lifecycle
	Close() error
}

// InternalStore manages user accounts, per-user config, and system-level KV.
type InternalStore interface {
	// User accounts
	GetUser(ctx context.Context, userID string) (*models.InternalUser, error)
	SaveUser(ctx context.Context, user *models.InternalUser) error
	DeleteUser(ctx context.Context, userID string) error
	ListUsers(ctx context.Context) ([]string, error)

	// Per-user key-value config
	GetUserKV(ctx context.Context, userID, key string) (*models.UserKeyValue, error)
	SetUserKV(ctx context.Context, userID, key, value string) error
	DeleteUserKV(ctx context.Context, userID, key string) error
	ListUserKV(ctx context.Context, userID string) ([]*models.UserKeyValue, error)

	// System key-value (non-user-scoped)
	GetSystemKV(ctx context.Context, key string) (string, error)
	SetSystemKV(ctx context.Context, key, value string) error

	Close() error
}

// UserDataStore manages all user domain data via generic records.
type UserDataStore interface {
	Get(ctx context.Context, userID, subject, key string) (*models.UserRecord, error)
	Put(ctx context.Context, record *models.UserRecord) error
	Delete(ctx context.Context, userID, subject, key string) error
	List(ctx context.Context, userID, subject string) ([]*models.UserRecord, error)
	Query(ctx context.Context, userID, subject string, opts QueryOptions) ([]*models.UserRecord, error)
	DeleteBySubject(ctx context.Context, subject string) (int, error)
	Close() error
}

// QueryOptions configures query behavior for UserDataStore.
type QueryOptions struct {
	Limit   int
	OrderBy string // "datetime_desc" (default), "datetime_asc"
}

// MarketDataStorage handles market data persistence
type MarketDataStorage interface {
	// GetMarketData retrieves market data for a ticker
	GetMarketData(ctx context.Context, ticker string) (*models.MarketData, error)

	// SaveMarketData persists market data
	SaveMarketData(ctx context.Context, data *models.MarketData) error

	// GetMarketDataBatch retrieves market data for multiple tickers
	GetMarketDataBatch(ctx context.Context, tickers []string) ([]*models.MarketData, error)

	// GetStaleTickers returns tickers that need refreshing
	GetStaleTickers(ctx context.Context, exchange string, maxAge int64) ([]string, error)
}

// SignalStorage handles signal persistence
type SignalStorage interface {
	// GetSignals retrieves signals for a ticker
	GetSignals(ctx context.Context, ticker string) (*models.TickerSignals, error)

	// SaveSignals persists signals
	SaveSignals(ctx context.Context, signals *models.TickerSignals) error

	// GetSignalsBatch retrieves signals for multiple tickers
	GetSignalsBatch(ctx context.Context, tickers []string) ([]*models.TickerSignals, error)
}
