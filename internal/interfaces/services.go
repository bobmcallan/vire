// Package interfaces defines service contracts for Vire
package interfaces

import (
	"context"
	"time"

	"github.com/bobmcallan/vire/internal/models"
)

// QuoteService provides real-time quotes with automatic fallback across sources
type QuoteService interface {
	// GetRealTimeQuote retrieves a live OHLCV snapshot, falling back to ASX
	// Markit Digital API when the primary EODHD source returns stale data
	// for ASX-listed tickers during market hours.
	GetRealTimeQuote(ctx context.Context, ticker string) (*models.RealTimeQuote, error)
}

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

	// ReviewWatchlist generates a review with signals for watchlist tickers
	ReviewWatchlist(ctx context.Context, name string, options ReviewOptions) (*models.WatchlistReview, error)

	// GetPortfolioSnapshot reconstructs portfolio state as of a historical date
	GetPortfolioSnapshot(ctx context.Context, name string, asOf time.Time) (*models.PortfolioSnapshot, error)

	// GetPortfolioGrowth returns monthly growth data points from inception to now
	GetPortfolioGrowth(ctx context.Context, name string) ([]models.GrowthDataPoint, error)

	// GetDailyGrowth returns daily portfolio value data points for a date range.
	// From/To zero values default to inception and yesterday respectively.
	GetDailyGrowth(ctx context.Context, name string, opts GrowthOptions) ([]models.GrowthDataPoint, error)

	// GetExternalBalances returns the external balances for a portfolio.
	GetExternalBalances(ctx context.Context, portfolioName string) ([]models.ExternalBalance, error)

	// SetExternalBalances replaces all external balances, recomputes totals and weights, and saves.
	SetExternalBalances(ctx context.Context, portfolioName string, balances []models.ExternalBalance) (*models.Portfolio, error)

	// AddExternalBalance appends an external balance, recomputes totals and weights, and saves.
	AddExternalBalance(ctx context.Context, portfolioName string, balance models.ExternalBalance) (*models.Portfolio, error)

	// RemoveExternalBalance removes an external balance by ID, recomputes totals and weights, and saves.
	RemoveExternalBalance(ctx context.Context, portfolioName string, balanceID string) (*models.Portfolio, error)
}

// GrowthOptions configures the date range for daily growth queries
type GrowthOptions struct {
	From time.Time // Start date (zero = inception)
	To   time.Time // End date (zero = yesterday)
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

	// CollectCoreMarketData fetches only EOD bars and fundamentals (fast path).
	// Uses bulk EOD API where possible. Skips filings, news, AI summaries.
	// When force is true, all data is re-fetched regardless of freshness.
	CollectCoreMarketData(ctx context.Context, tickers []string, force bool) error

	// CollectBulkEOD fetches last-day EOD bars for all tickers on an exchange
	// via the bulk API, merges into existing data, and falls back to individual
	// CollectEOD for tickers with no existing EOD history.
	CollectBulkEOD(ctx context.Context, exchange string, force bool) error

	// Individual collection methods — each handles a single data component for a single ticker.
	CollectEOD(ctx context.Context, ticker string, force bool) error
	CollectFundamentals(ctx context.Context, ticker string, force bool) error
	CollectFilings(ctx context.Context, ticker string, force bool) error
	CollectNews(ctx context.Context, ticker string, force bool) error
	CollectFilingSummaries(ctx context.Context, ticker string, force bool) error
	CollectTimeline(ctx context.Context, ticker string, force bool) error
	CollectNewsIntelligence(ctx context.Context, ticker string, force bool) error

	// ReadFiling retrieves the text content of a filing PDF by ticker and document key.
	ReadFiling(ctx context.Context, ticker, documentKey string) (*models.FilingContent, error)

	// GetStockData retrieves stock data with optional components
	GetStockData(ctx context.Context, ticker string, include StockDataInclude) (*models.StockData, error)

	// FindSnipeBuys scans for tickers matching strategy entry criteria
	FindSnipeBuys(ctx context.Context, options SnipeOptions) ([]*models.SnipeBuy, error)

	// ScreenStocks filters stocks by quantitative criteria
	ScreenStocks(ctx context.Context, options ScreenOptions) ([]*models.ScreenCandidate, error)

	// FunnelScreen runs a 3-stage funnel: EODHD screener (100) -> fundamental refinement (25) -> technical scoring (5)
	FunnelScreen(ctx context.Context, options FunnelOptions) (*models.FunnelResult, error)

	// RefreshStaleData updates outdated market data
	RefreshStaleData(ctx context.Context, exchange string) error

	// ScanMarket executes a flexible market scan query
	ScanMarket(ctx context.Context, query models.ScanQuery) (*models.ScanResponse, error)

	// ScanFields returns the available scan field definitions
	ScanFields() *models.ScanFieldsResponse
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
	Exchange    string                    // Exchange to scan (e.g., "ASX")
	Limit       int                       // Max results to return
	Criteria    []string                  // Filter criteria
	Sector      string                    // Optional sector filter
	IncludeNews bool                      // Include news sentiment analysis
	Strategy    *models.PortfolioStrategy // Optional portfolio strategy for filtering/scoring
}

// FunnelOptions configures the multi-stage funnel screen
type FunnelOptions struct {
	Exchange    string                    // Exchange to scan (e.g., "AU", "US")
	Limit       int                       // Final result count (default: 5, max: 10)
	Sector      string                    // Optional sector filter
	IncludeNews bool                      // Include news sentiment
	Strategy    *models.PortfolioStrategy // Optional portfolio strategy
}

// ScreenOptions configures the stock screen
type ScreenOptions struct {
	Exchange        string                    // Exchange to scan (e.g., "AU", "US")
	Limit           int                       // Max results to return
	MaxPE           float64                   // Maximum P/E ratio (default: 20)
	MinQtrReturnPct float64                   // Minimum annualised quarterly return % (default: 10)
	Sector          string                    // Optional sector filter
	IncludeNews     bool                      // Include news sentiment analysis
	Strategy        *models.PortfolioStrategy // Optional portfolio strategy for filtering/scoring
}

// ReportService handles report generation and storage
type ReportService interface {
	// GetReport retrieves a cached report for a portfolio
	GetReport(ctx context.Context, portfolioName string) (*models.PortfolioReport, error)

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

// StrategyService manages portfolio strategy operations
type StrategyService interface {
	// GetStrategy retrieves the strategy for a portfolio
	GetStrategy(ctx context.Context, portfolioName string) (*models.PortfolioStrategy, error)

	// SaveStrategy saves a strategy with devil's advocate validation
	SaveStrategy(ctx context.Context, strategy *models.PortfolioStrategy) ([]models.StrategyWarning, error)

	// DeleteStrategy removes a strategy
	DeleteStrategy(ctx context.Context, portfolioName string) error

	// ValidateStrategy checks for unrealistic goals and internal contradictions
	ValidateStrategy(ctx context.Context, strategy *models.PortfolioStrategy) []models.StrategyWarning
}

// SignalService handles signal detection
type SignalService interface {
	// DetectSignals computes signals for tickers.
	// When force is true, signals are recomputed regardless of freshness.
	DetectSignals(ctx context.Context, tickers []string, signalTypes []string, force bool) ([]*models.TickerSignals, error)

	// ComputeSignals calculates all signals for a ticker
	ComputeSignals(ctx context.Context, ticker string, marketData *models.MarketData) (*models.TickerSignals, error)
}

// WatchlistService manages portfolio watchlist operations
type WatchlistService interface {
	// GetWatchlist retrieves the watchlist for a portfolio
	GetWatchlist(ctx context.Context, portfolioName string) (*models.PortfolioWatchlist, error)

	// SaveWatchlist saves a watchlist with version increment
	SaveWatchlist(ctx context.Context, watchlist *models.PortfolioWatchlist) error

	// DeleteWatchlist removes a watchlist
	DeleteWatchlist(ctx context.Context, portfolioName string) error

	// AddOrUpdateItem adds a new item or updates an existing one (upsert keyed on ticker)
	AddOrUpdateItem(ctx context.Context, portfolioName string, item *models.WatchlistItem) (*models.PortfolioWatchlist, error)

	// UpdateItem updates an existing item by ticker (merge semantics)
	UpdateItem(ctx context.Context, portfolioName, ticker string, update *models.WatchlistItem) (*models.PortfolioWatchlist, error)

	// RemoveItem removes a stock from the watchlist by ticker
	RemoveItem(ctx context.Context, portfolioName, ticker string) (*models.PortfolioWatchlist, error)
}

// PlanService manages portfolio plan operations
type PlanService interface {
	// GetPlan retrieves the plan for a portfolio
	GetPlan(ctx context.Context, portfolioName string) (*models.PortfolioPlan, error)

	// SavePlan saves a plan with version increment
	SavePlan(ctx context.Context, plan *models.PortfolioPlan) error

	// DeletePlan removes a plan
	DeletePlan(ctx context.Context, portfolioName string) error

	// AddPlanItem adds a single item to a portfolio plan
	AddPlanItem(ctx context.Context, portfolioName string, item *models.PlanItem) (*models.PortfolioPlan, error)

	// UpdatePlanItem updates an existing plan item by ID
	UpdatePlanItem(ctx context.Context, portfolioName, itemID string, update *models.PlanItem) (*models.PortfolioPlan, error)

	// RemovePlanItem removes an item from a plan by ID
	RemovePlanItem(ctx context.Context, portfolioName, itemID string) (*models.PortfolioPlan, error)

	// CheckPlanEvents evaluates event-based pending items, returns triggered items
	CheckPlanEvents(ctx context.Context, portfolioName string) ([]models.PlanItem, error)

	// CheckPlanDeadlines marks overdue time-based items as expired, returns expired items
	CheckPlanDeadlines(ctx context.Context, portfolioName string) ([]models.PlanItem, error)

	// ValidatePlanAgainstStrategy checks plan items against portfolio strategy
	ValidatePlanAgainstStrategy(ctx context.Context, plan *models.PortfolioPlan, strategy *models.PortfolioStrategy) []models.StrategyWarning
}

// CashFlowService manages cash flow transactions and capital performance calculations
type CashFlowService interface {
	// GetLedger retrieves all cash transactions for a portfolio
	GetLedger(ctx context.Context, portfolioName string) (*models.CashFlowLedger, error)

	// AddTransaction adds a new cash transaction to the ledger
	AddTransaction(ctx context.Context, portfolioName string, tx models.CashTransaction) (*models.CashFlowLedger, error)

	// UpdateTransaction updates an existing transaction by ID (merge semantics)
	UpdateTransaction(ctx context.Context, portfolioName string, txID string, tx models.CashTransaction) (*models.CashFlowLedger, error)

	// RemoveTransaction removes a transaction by ID
	RemoveTransaction(ctx context.Context, portfolioName string, txID string) (*models.CashFlowLedger, error)

	// CalculatePerformance computes capital deployment performance (XIRR, simple return)
	CalculatePerformance(ctx context.Context, portfolioName string) (*models.CapitalPerformance, error)
}
