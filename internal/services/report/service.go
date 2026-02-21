// Package report provides report generation services
package report

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
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

// GetReport retrieves a cached report for a portfolio
func (s *Service) GetReport(ctx context.Context, portfolioName string) (*models.PortfolioReport, error) {
	return s.getReportRecord(ctx, portfolioName)
}

// GenerateReport runs the fast pipeline: sync, collect core data, review, format, store
func (s *Service) GenerateReport(ctx context.Context, portfolioName string, options interfaces.ReportOptions) (*models.PortfolioReport, error) {
	s.logger.Info().Str("portfolio", portfolioName).Msg("Generating portfolio report")

	// Step 1: Sync portfolio
	portfolio, err := s.portfolio.SyncPortfolio(ctx, portfolioName, options.ForceRefresh)
	if err != nil {
		return nil, fmt.Errorf("sync portfolio: %w", err)
	}

	// Extract tickers (include closed positions — they need market data for growth calculations)
	tickers := make([]string, 0, len(portfolio.Holdings))
	for _, h := range portfolio.Holdings {
		tickers = append(tickers, h.EODHDTicker())
	}

	// Step 2: Collect core market data (fast path — EOD + fundamentals only)
	if err := s.market.CollectCoreMarketData(ctx, tickers, options.ForceRefresh); err != nil {
		s.logger.Warn().Err(err).Msg("Core market data collection had errors (continuing)")
	}

	// Step 3: Review portfolio
	review, err := s.portfolio.ReviewPortfolio(ctx, portfolioName, interfaces.ReviewOptions{
		FocusSignals: options.FocusSignals,
		IncludeNews:  options.IncludeNews,
	})
	if err != nil {
		return nil, fmt.Errorf("review portfolio: %w", err)
	}

	// Step 4: Format and build report
	report := s.buildReport(portfolioName, review)

	// Step 5: Store report
	if err := s.saveReportRecord(ctx, report); err != nil {
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
	existing, err := s.getReportRecord(ctx, portfolioName)
	if err != nil {
		return nil, fmt.Errorf("no existing report for '%s': %w", portfolioName, err)
	}

	// Resolve EODHD ticker by finding the holding's exchange from the portfolio
	portfolio, err := s.portfolio.GetPortfolio(ctx, portfolioName)
	eodhdTicker := ticker + ".AU" // fallback
	if err == nil {
		for _, h := range portfolio.Holdings {
			if strings.EqualFold(h.Ticker, ticker) {
				eodhdTicker = h.EODHDTicker()
				break
			}
		}
	}

	// Collect + detect for just this ticker
	if err := s.market.CollectCoreMarketData(ctx, []string{eodhdTicker}, false); err != nil {
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
	if err := s.saveReportRecord(ctx, existing); err != nil {
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
		if hr.ActionRequired == "CLOSED" {
			continue
		}
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
		if hr.ActionRequired == "CLOSED" {
			continue
		}
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

func (s *Service) getReportRecord(ctx context.Context, portfolioName string) (*models.PortfolioReport, error) {
	userID := common.ResolveUserID(ctx)
	rec, err := s.storage.UserDataStore().Get(ctx, userID, "report", portfolioName)
	if err != nil {
		return nil, fmt.Errorf("report for '%s' not found: %w", portfolioName, err)
	}
	var report models.PortfolioReport
	if err := json.Unmarshal([]byte(rec.Value), &report); err != nil {
		return nil, fmt.Errorf("failed to unmarshal report: %w", err)
	}
	return &report, nil
}

func (s *Service) saveReportRecord(ctx context.Context, report *models.PortfolioReport) error {
	userID := common.ResolveUserID(ctx)
	data, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("failed to marshal report: %w", err)
	}
	return s.storage.UserDataStore().Put(ctx, &models.UserRecord{
		UserID:  userID,
		Subject: "report",
		Key:     report.Portfolio,
		Value:   string(data),
	})
}

// Ensure Service implements ReportService
var _ interfaces.ReportService = (*Service)(nil)
