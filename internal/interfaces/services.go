// Package interfaces defines service contracts for Vire
package interfaces

import (
	"context"

	"github.com/bobmccarthy/vire/internal/models"
)

// PortfolioService manages portfolio operations
type PortfolioService interface {
	// SyncPortfolio refreshes portfolio data from Navexa
	SyncPortfolio(ctx context.Context, name string, force bool) (*models.Portfolio, error)

	// GetPortfolio retrieves a portfolio with current data
	GetPortfolio(ctx context.Context, name string) (*models.Portfolio, error)

	// ListPortfolios returns available portfolio names
	ListPortfolios(ctx context.Context) ([]string, error)

	// ReviewPortfolio generates a portfolio review with signals
	ReviewPortfolio(ctx context.Context, name string, options ReviewOptions) (*models.PortfolioReview, error)
}

// ReviewOptions configures portfolio review
type ReviewOptions struct {
	FocusSignals []string // Signal types to focus on
	IncludeNews  bool     // Include news in analysis
}

// MarketService handles market data operations
type MarketService interface {
	// CollectMarketData fetches and stores market data for tickers.
	// When force is true, all data is re-fetched regardless of freshness.
	CollectMarketData(ctx context.Context, tickers []string, includeNews bool, force bool) error

	// GetStockData retrieves stock data with optional components
	GetStockData(ctx context.Context, ticker string, include StockDataInclude) (*models.StockData, error)

	// FindSnipeBuys identifies turnaround stocks
	FindSnipeBuys(ctx context.Context, options SnipeOptions) ([]*models.SnipeBuy, error)

	// ScreenStocks finds quality-value stocks with low P/E, consistent returns, and credible news
	ScreenStocks(ctx context.Context, options ScreenOptions) ([]*models.ScreenCandidate, error)

	// RefreshStaleData updates outdated market data
	RefreshStaleData(ctx context.Context, exchange string) error
}

// StockDataInclude specifies what to include in stock data
type StockDataInclude struct {
	Price        bool
	Fundamentals bool
	Signals      bool
	News         bool
}

// SnipeOptions configures snipe buy search
type SnipeOptions struct {
	Exchange string   // Exchange to scan (e.g., "ASX")
	Limit    int      // Max results to return
	Criteria []string // Filter criteria
	Sector   string   // Optional sector filter
}

// ScreenOptions configures the quality-value stock screen
type ScreenOptions struct {
	Exchange        string  // Exchange to scan (e.g., "AU", "US")
	Limit           int     // Max results to return
	MaxPE           float64 // Maximum P/E ratio (default: 20)
	MinQtrReturnPct float64 // Minimum annualised quarterly return % (default: 10)
	Sector          string  // Optional sector filter
}

// ReportService handles report generation and storage
type ReportService interface {
	// GenerateReport runs the full pipeline and stores the result
	GenerateReport(ctx context.Context, portfolioName string, options ReportOptions) (*models.PortfolioReport, error)

	// GenerateTickerReport refreshes a single ticker's report within an existing portfolio report
	GenerateTickerReport(ctx context.Context, portfolioName, ticker string) (*models.PortfolioReport, error)
}

// ReportOptions configures report generation
type ReportOptions struct {
	ForceRefresh bool
	IncludeNews  bool
	FocusSignals []string
}

// SignalService handles signal detection
type SignalService interface {
	// DetectSignals computes signals for tickers.
	// When force is true, signals are recomputed regardless of freshness.
	DetectSignals(ctx context.Context, tickers []string, signalTypes []string, force bool) ([]*models.TickerSignals, error)

	// ComputeSignals calculates all signals for a ticker
	ComputeSignals(ctx context.Context, ticker string, marketData *models.MarketData) (*models.TickerSignals, error)
}
