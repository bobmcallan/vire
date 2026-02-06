// Package interfaces defines service contracts for Vire
package interfaces

import (
	"context"

	"github.com/bobmccarthy/vire/internal/models"
)

// StorageManager coordinates all storage backends
type StorageManager interface {
	// Storage accessors
	PortfolioStorage() PortfolioStorage
	MarketDataStorage() MarketDataStorage
	SignalStorage() SignalStorage
	KeyValueStorage() KeyValueStorage
	ReportStorage() ReportStorage

	// Lifecycle
	Close() error
}

// PortfolioStorage handles portfolio persistence
type PortfolioStorage interface {
	// GetPortfolio retrieves a portfolio by name
	GetPortfolio(ctx context.Context, name string) (*models.Portfolio, error)

	// SavePortfolio persists a portfolio
	SavePortfolio(ctx context.Context, portfolio *models.Portfolio) error

	// ListPortfolios returns all portfolio names
	ListPortfolios(ctx context.Context) ([]string, error)

	// DeletePortfolio removes a portfolio
	DeletePortfolio(ctx context.Context, name string) error
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

// ReportStorage handles report persistence
type ReportStorage interface {
	// GetReport retrieves a report by portfolio name
	GetReport(ctx context.Context, portfolio string) (*models.PortfolioReport, error)

	// SaveReport persists a report
	SaveReport(ctx context.Context, report *models.PortfolioReport) error

	// ListReports returns all portfolio names that have reports
	ListReports(ctx context.Context) ([]string, error)

	// DeleteReport removes a report
	DeleteReport(ctx context.Context, portfolio string) error
}

// KeyValueStorage provides generic key-value storage
type KeyValueStorage interface {
	// Get retrieves a value by key
	Get(ctx context.Context, key string) (string, error)

	// Set stores a value
	Set(ctx context.Context, key, value string) error

	// Delete removes a key
	Delete(ctx context.Context, key string) error

	// GetAll returns all key-value pairs
	GetAll(ctx context.Context) (map[string]string, error)
}
