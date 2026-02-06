// Package report provides report generation services
package report

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bobmccarthy/vire/internal/common"
	"github.com/bobmccarthy/vire/internal/interfaces"
	"github.com/bobmccarthy/vire/internal/models"
)

// Service implements ReportService
type Service struct {
	portfolio interfaces.PortfolioService
	market    interfaces.MarketService
	signal    interfaces.SignalService
	storage   interfaces.StorageManager
	logger    *common.Logger
}

// NewService creates a new report service
func NewService(
	portfolio interfaces.PortfolioService,
	market interfaces.MarketService,
	signal interfaces.SignalService,
	storage interfaces.StorageManager,
	logger *common.Logger,
) *Service {
	return &Service{
		portfolio: portfolio,
		market:    market,
		signal:    signal,
		storage:   storage,
		logger:    logger,
	}
}

// GenerateReport runs the full pipeline: sync, collect, detect, review, format, store
func (s *Service) GenerateReport(ctx context.Context, portfolioName string, options interfaces.ReportOptions) (*models.PortfolioReport, error) {
	s.logger.Info().Str("portfolio", portfolioName).Msg("Generating portfolio report")

	// Step 1: Sync portfolio
	portfolio, err := s.portfolio.SyncPortfolio(ctx, portfolioName, options.ForceRefresh)
	if err != nil {
		return nil, fmt.Errorf("sync portfolio: %w", err)
	}

	// Extract tickers
	tickers := make([]string, 0, len(portfolio.Holdings))
	for _, h := range portfolio.Holdings {
		if h.Units > 0 {
			tickers = append(tickers, h.Ticker+".AU")
		}
	}

	// Step 2: Collect market data
	if err := s.market.CollectMarketData(ctx, tickers, options.IncludeNews, options.ForceRefresh); err != nil {
		s.logger.Warn().Err(err).Msg("Market data collection had errors (continuing)")
	}

	// Step 3: Detect signals
	if _, err := s.signal.DetectSignals(ctx, tickers, options.FocusSignals, options.ForceRefresh); err != nil {
		s.logger.Warn().Err(err).Msg("Signal detection had errors (continuing)")
	}

	// Step 4: Review portfolio
	review, err := s.portfolio.ReviewPortfolio(ctx, portfolioName, interfaces.ReviewOptions{
		FocusSignals: options.FocusSignals,
		IncludeNews:  options.IncludeNews,
	})
	if err != nil {
		return nil, fmt.Errorf("review portfolio: %w", err)
	}

	// Step 5: Format and build report
	report := s.buildReport(portfolioName, review)

	// Step 6: Store report
	if err := s.storage.ReportStorage().SaveReport(ctx, report); err != nil {
		return nil, fmt.Errorf("save report: %w", err)
	}

	s.logger.Info().
		Str("portfolio", portfolioName).
		Int("tickers", len(report.TickerReports)).
		Msg("Report generated and stored")

	return report, nil
}

// GenerateTickerReport refreshes a single ticker's report
func (s *Service) GenerateTickerReport(ctx context.Context, portfolioName, ticker string) (*models.PortfolioReport, error) {
	s.logger.Info().Str("portfolio", portfolioName).Str("ticker", ticker).Msg("Regenerating ticker report")

	// Load existing report
	existing, err := s.storage.ReportStorage().GetReport(ctx, portfolioName)
	if err != nil {
		return nil, fmt.Errorf("no existing report for '%s': %w", portfolioName, err)
	}

	// Collect + detect for just this ticker
	eodhdTicker := ticker + ".AU"
	if err := s.market.CollectMarketData(ctx, []string{eodhdTicker}, false, false); err != nil {
		s.logger.Warn().Err(err).Str("ticker", ticker).Msg("Market data collection had errors")
	}
	if _, err := s.signal.DetectSignals(ctx, []string{eodhdTicker}, nil, false); err != nil {
		s.logger.Warn().Err(err).Str("ticker", ticker).Msg("Signal detection had errors")
	}

	// Run full review (needs portfolio context for weights/actions)
	review, err := s.portfolio.ReviewPortfolio(ctx, portfolioName, interfaces.ReviewOptions{})
	if err != nil {
		return nil, fmt.Errorf("review portfolio: %w", err)
	}

	// Find the target holding and regenerate its markdown
	found := false
	for _, hr := range review.HoldingReviews {
		if strings.EqualFold(hr.Holding.Ticker, ticker) {
			isETF := common.IsETF(&hr)
			var md string
			if isETF {
				md = formatETFReport(&hr, review)
			} else {
				md = formatStockReport(&hr, review)
			}

			// Replace in existing report
			for i, tr := range existing.TickerReports {
				if strings.EqualFold(tr.Ticker, ticker) {
					existing.TickerReports[i].Markdown = md
					existing.TickerReports[i].IsETF = isETF
					existing.TickerReports[i].Name = hr.Holding.Name
					found = true
					break
				}
			}
			if !found {
				existing.TickerReports = append(existing.TickerReports, models.TickerReport{
					Ticker:   hr.Holding.Ticker,
					Name:     hr.Holding.Name,
					IsETF:    isETF,
					Markdown: md,
				})
				existing.Tickers = append(existing.Tickers, hr.Holding.Ticker)
				found = true
			}
			break
		}
	}

	if !found {
		return nil, fmt.Errorf("ticker '%s' not found in portfolio '%s'", ticker, portfolioName)
	}

	// Regenerate summary with updated review data
	existing.SummaryMarkdown = formatReportSummary(review)
	existing.GeneratedAt = time.Now()

	// Save back
	if err := s.storage.ReportStorage().SaveReport(ctx, existing); err != nil {
		return nil, fmt.Errorf("save report: %w", err)
	}

	s.logger.Info().Str("portfolio", portfolioName).Str("ticker", ticker).Msg("Ticker report regenerated")
	return existing, nil
}

// buildReport creates a PortfolioReport from a review
func (s *Service) buildReport(portfolioName string, review *models.PortfolioReview) *models.PortfolioReport {
	// Sort holdings: stocks first, then ETFs, alphabetically within each group
	var stocks, etfs []models.HoldingReview
	for _, hr := range review.HoldingReviews {
		if common.IsETF(&hr) {
			etfs = append(etfs, hr)
		} else {
			stocks = append(stocks, hr)
		}
	}
	sort.Slice(stocks, func(i, j int) bool { return stocks[i].Holding.Ticker < stocks[j].Holding.Ticker })
	sort.Slice(etfs, func(i, j int) bool { return etfs[i].Holding.Ticker < etfs[j].Holding.Ticker })

	// Build ticker reports
	tickerReports := make([]models.TickerReport, 0, len(review.HoldingReviews))
	tickerNames := make([]string, 0, len(review.HoldingReviews))

	for _, hr := range stocks {
		md := formatStockReport(&hr, review)
		tickerReports = append(tickerReports, models.TickerReport{
			Ticker:   hr.Holding.Ticker,
			Name:     hr.Holding.Name,
			IsETF:    false,
			Markdown: md,
		})
		tickerNames = append(tickerNames, hr.Holding.Ticker)
	}
	for _, hr := range etfs {
		md := formatETFReport(&hr, review)
		tickerReports = append(tickerReports, models.TickerReport{
			Ticker:   hr.Holding.Ticker,
			Name:     hr.Holding.Name,
			IsETF:    true,
			Markdown: md,
		})
		tickerNames = append(tickerNames, hr.Holding.Ticker)
	}

	return &models.PortfolioReport{
		Portfolio:       portfolioName,
		GeneratedAt:     time.Now(),
		SummaryMarkdown: formatReportSummary(review),
		TickerReports:   tickerReports,
		Tickers:         tickerNames,
	}
}

// Ensure Service implements ReportService
var _ interfaces.ReportService = (*Service)(nil)
