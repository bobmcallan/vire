// Package interfaces defines service contracts for Vire
package interfaces

import (
	"context"
	"time"

	"github.com/bobmcallan/vire/internal/models"
)

// EODHDClient provides access to EODHD API
type EODHDClient interface {
	// GetRealTimeQuote retrieves a live OHLCV snapshot for a ticker
	GetRealTimeQuote(ctx context.Context, ticker string) (*models.RealTimeQuote, error)

	// GetEOD retrieves end-of-day price data
	GetEOD(ctx context.Context, ticker string, opts ...EODOption) (*models.EODResponse, error)

	// GetBulkEOD retrieves EOD data for multiple tickers in one request.
	// Uses EODHD bulk API: /eod-bulk-last-day/{exchange}?symbols=A,B,C
	// More efficient than calling GetEOD for each ticker.
	GetBulkEOD(ctx context.Context, exchange string, tickers []string) (map[string]models.EODBar, error)

	// GetFundamentals retrieves fundamental data
	GetFundamentals(ctx context.Context, ticker string) (*models.Fundamentals, error)

	// GetTechnicals retrieves technical indicators
	GetTechnicals(ctx context.Context, ticker string, function string) (*models.TechnicalResponse, error)

	// GetNews retrieves news for a ticker
	GetNews(ctx context.Context, ticker string, limit int) ([]*models.NewsItem, error)

	// GetExchangeSymbols retrieves all symbols for an exchange
	GetExchangeSymbols(ctx context.Context, exchange string) ([]*models.Symbol, error)

	// ScreenStocks uses the EODHD Screener API to find stocks matching filters
	ScreenStocks(ctx context.Context, options models.ScreenerOptions) ([]*models.ScreenerResult, error)
}

// EODOption configures EOD data requests
type EODOption func(*EODParams)

// EODParams holds EOD query parameters
type EODParams struct {
	From   time.Time
	To     time.Time
	Period string // d=daily, w=weekly, m=monthly
	Order  string // a=ascending, d=descending
	Limit  int
}

// WithDateRange sets the date range for EOD query
func WithDateRange(from, to time.Time) EODOption {
	return func(p *EODParams) {
		p.From = from
		p.To = to
	}
}

// WithPeriod sets the period for EOD query
func WithPeriod(period string) EODOption {
	return func(p *EODParams) {
		p.Period = period
	}
}

// WithLimit sets the limit for EOD query
func WithLimit(limit int) EODOption {
	return func(p *EODParams) {
		p.Limit = limit
	}
}

// ASXClient provides access to the ASX Markit Digital API for real-time quotes
type ASXClient interface {
	// GetRealTimeQuote retrieves a live price snapshot for an ASX-listed ticker
	GetRealTimeQuote(ctx context.Context, ticker string) (*models.RealTimeQuote, error)
}

// NavexaClient provides access to Navexa API
type NavexaClient interface {
	// GetPortfolios retrieves all portfolios
	GetPortfolios(ctx context.Context) ([]*models.NavexaPortfolio, error)

	// GetPortfolio retrieves a specific portfolio by ID
	GetPortfolio(ctx context.Context, portfolioID string) (*models.NavexaPortfolio, error)

	// GetHoldings retrieves holdings for a portfolio
	GetHoldings(ctx context.Context, portfolioID string) ([]*models.NavexaHolding, error)

	// GetPerformance retrieves portfolio performance metrics grouped by holding
	GetPerformance(ctx context.Context, portfolioID, fromDate, toDate string) (*models.NavexaPerformance, error)

	// GetEnrichedHoldings retrieves holdings with financial data via the performance endpoint
	GetEnrichedHoldings(ctx context.Context, portfolioID, fromDate, toDate string) ([]*models.NavexaHolding, error)

	// GetHoldingTrades retrieves all trades for a specific holding
	GetHoldingTrades(ctx context.Context, holdingID string) ([]*models.NavexaTrade, error)
}

// GeminiClient provides access to Gemini API
type GeminiClient interface {
	// GenerateContent generates AI content from a prompt
	GenerateContent(ctx context.Context, prompt string) (string, error)

	// GenerateWithURLContext generates content using Gemini's URL context tool
	GenerateWithURLContext(ctx context.Context, prompt string, urls ...string) (string, error)

	// AnalyzeStock generates AI analysis for a stock
	AnalyzeStock(ctx context.Context, ticker string, data *models.StockData) (string, error)

	// SummariseFilingPDF uploads a PDF to the Gemini Files API for native PDF
	// comprehension, sends it with a text prompt, and returns the response.
	SummariseFilingPDF(ctx context.Context, pdfPath string, prompt string) (string, error)
}
