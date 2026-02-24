package market

import (
	"context"
	"fmt"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

// CollectBulkEOD fetches last-day EOD bars for all tickers on an exchange via
// the bulk API, merges into existing data, and falls back to individual
// CollectEOD for tickers with no existing EOD history.
func (s *Service) CollectBulkEOD(ctx context.Context, exchange string, force bool) error {
	now := time.Now()

	if s.eodhd == nil {
		return fmt.Errorf("EODHD client not configured")
	}

	// Get all tickers for this exchange from the stock index
	entries, err := s.storage.StockIndexStore().List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list stock index: %w", err)
	}

	var tickers []string
	for _, entry := range entries {
		if extractExchange(entry.Ticker) == exchange {
			tickers = append(tickers, entry.Ticker)
		}
	}
	if len(tickers) == 0 {
		s.logger.Debug().Str("exchange", exchange).Msg("No tickers for exchange in stock index")
		return nil
	}

	// Fetch bulk EOD (last day) for all tickers on this exchange
	bulkBars, err := s.eodhd.GetBulkEOD(ctx, exchange, tickers)
	if err != nil {
		return fmt.Errorf("bulk EOD fetch failed for exchange %s: %w", exchange, err)
	}

	processed := 0
	fallbacks := 0

	for _, ticker := range tickers {
		existing, _ := s.storage.MarketDataStorage().GetMarketData(ctx, ticker)

		// No existing EOD data — fall back to individual full-history fetch
		if existing == nil || len(existing.EOD) == 0 {
			s.logger.Debug().Str("ticker", ticker).Msg("No existing EOD, falling back to individual CollectEOD")
			if err := s.CollectEOD(ctx, ticker, force); err != nil {
				s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Individual EOD fallback failed")
			} else {
				if err := s.storage.StockIndexStore().UpdateTimestamp(ctx, ticker, "eod_collected_at", now); err != nil {
					s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to update stock index EOD timestamp")
				}
				fallbacks++
			}
			continue
		}

		// Check freshness (skip if already fresh and not forced)
		if !force && common.IsFresh(existing.EODUpdatedAt, common.FreshnessTodayBar) {
			continue
		}

		bar, ok := bulkBars[ticker]
		if !ok {
			// Ticker not in bulk response, skip
			s.logger.Debug().Str("ticker", ticker).Msg("Ticker not in bulk EOD response")
			continue
		}

		marketData := existing
		eodChanged := false

		// Merge the bulk bar into existing EOD data
		barDate := bar.Date.Format("2006-01-02")
		latestDate := existing.EOD[0].Date.Format("2006-01-02")
		if barDate != latestDate {
			marketData.EOD = mergeEODBars([]models.EODBar{bar}, existing.EOD)
			eodChanged = true
		}
		marketData.EODUpdatedAt = now
		marketData.DataVersion = common.SchemaVersion
		marketData.LastUpdated = now

		if err := s.storage.MarketDataStorage().SaveMarketData(ctx, marketData); err != nil {
			s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to save market data after bulk EOD merge")
			continue
		}

		// Compute signals when EOD changed
		if eodChanged {
			tickerSignals := s.signalComputer.Compute(marketData)
			if err := s.storage.SignalStorage().SaveSignals(ctx, tickerSignals); err != nil {
				s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to save signals after bulk EOD")
			}
		}

		// Update stock index timestamp per-ticker
		if err := s.storage.StockIndexStore().UpdateTimestamp(ctx, ticker, "eod_collected_at", now); err != nil {
			s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to update stock index EOD timestamp")
		}

		processed++
	}

	s.logger.Info().
		Str("exchange", exchange).
		Int("tickers", len(tickers)).
		Int("processed", processed).
		Int("fallbacks", fallbacks).
		Msg("Bulk EOD collection complete")

	return nil
}

// CollectEOD fetches and stores EOD bar data for a single ticker.
func (s *Service) CollectEOD(ctx context.Context, ticker string, force bool) error {
	now := time.Now()

	existing, _ := s.storage.MarketDataStorage().GetMarketData(ctx, ticker)
	marketData := &models.MarketData{
		Ticker:   ticker,
		Exchange: extractExchange(ticker),
	}
	if existing != nil {
		marketData = existing
	}

	if !force && existing != nil && common.IsFresh(existing.EODUpdatedAt, common.FreshnessTodayBar) {
		return nil
	}

	if s.eodhd == nil {
		return fmt.Errorf("EODHD client not configured")
	}

	eodChanged := false

	// Incremental fetch: only bars after the latest stored date
	if !force && existing != nil && len(existing.EOD) > 0 {
		latestDate := existing.EOD[0].Date
		fromDate := latestDate.AddDate(0, 0, 1)
		if fromDate.Before(now) {
			eodResp, err := s.eodhd.GetEOD(ctx, ticker, interfaces.WithDateRange(fromDate, now))
			if err != nil {
				return fmt.Errorf("failed to fetch incremental EOD data: %w", err)
			}
			if len(eodResp.Data) > 0 {
				marketData.EOD = mergeEODBars(eodResp.Data, existing.EOD)
				eodChanged = true
			}
		}
		marketData.EODUpdatedAt = now
	} else {
		// Full fetch
		eodResp, err := s.eodhd.GetEOD(ctx, ticker, interfaces.WithDateRange(now.AddDate(-3, 0, 0), now))
		if err != nil {
			return fmt.Errorf("failed to fetch EOD data: %w", err)
		}
		marketData.EOD = eodResp.Data
		marketData.EODUpdatedAt = now
		eodChanged = true
	}

	marketData.DataVersion = common.SchemaVersion
	marketData.LastUpdated = now

	if err := s.storage.MarketDataStorage().SaveMarketData(ctx, marketData); err != nil {
		return fmt.Errorf("failed to save market data: %w", err)
	}

	// Compute and save signals when EOD data changed
	if eodChanged {
		tickerSignals := s.signalComputer.Compute(marketData)
		if err := s.storage.SignalStorage().SaveSignals(ctx, tickerSignals); err != nil {
			s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to save signals after EOD collect")
		}
	}

	return nil
}

// CollectFundamentals fetches and stores fundamental data for a single ticker.
func (s *Service) CollectFundamentals(ctx context.Context, ticker string, force bool) error {
	now := time.Now()

	existing, _ := s.storage.MarketDataStorage().GetMarketData(ctx, ticker)
	marketData := &models.MarketData{
		Ticker:   ticker,
		Exchange: extractExchange(ticker),
	}
	if existing != nil {
		marketData = existing
	}

	needFundamentals := force || existing == nil || !common.IsFresh(existing.FundamentalsUpdatedAt, common.FreshnessFundamentals) ||
		(existing != nil && existing.Fundamentals != nil && existing.Fundamentals.ISIN == "")
	if !needFundamentals {
		return nil
	}

	if s.eodhd == nil {
		return fmt.Errorf("EODHD client not configured")
	}

	fundamentals, err := s.eodhd.GetFundamentals(ctx, ticker)
	if err != nil {
		return fmt.Errorf("failed to fetch fundamentals: %w", err)
	}

	if fundamentals != nil {
		s.enrichFundamentals(ctx, fundamentals)
	}
	marketData.Fundamentals = fundamentals
	marketData.FundamentalsUpdatedAt = now
	marketData.DataVersion = common.SchemaVersion
	marketData.LastUpdated = now

	// Recompute quality assessment when fundamentals are refreshed
	if fundamentals != nil {
		marketData.QualityAssessment = computeQualityAssessment(fundamentals)
	}

	if err := s.storage.MarketDataStorage().SaveMarketData(ctx, marketData); err != nil {
		return fmt.Errorf("failed to save market data: %w", err)
	}

	return nil
}

// CollectFilings fetches and stores filings for a single ticker.
func (s *Service) CollectFilings(ctx context.Context, ticker string, force bool) error {
	now := time.Now()

	existing, _ := s.storage.MarketDataStorage().GetMarketData(ctx, ticker)
	marketData := &models.MarketData{
		Ticker:   ticker,
		Exchange: extractExchange(ticker),
	}
	if existing != nil {
		marketData = existing
	}

	if !force && existing != nil && common.IsFresh(existing.FilingsUpdatedAt, common.FreshnessFilings) {
		return nil
	}

	filings, err := s.collectFilings(ctx, ticker)
	if err != nil {
		return fmt.Errorf("failed to collect filings: %w", err)
	}

	filings = s.downloadFilingPDFs(ctx, extractCode(ticker), filings)
	marketData.Filings = filings
	marketData.FilingsUpdatedAt = now
	marketData.DataVersion = common.SchemaVersion
	marketData.LastUpdated = now

	if err := s.storage.MarketDataStorage().SaveMarketData(ctx, marketData); err != nil {
		return fmt.Errorf("failed to save market data: %w", err)
	}

	return nil
}

// CollectNews fetches and stores news articles for a single ticker.
func (s *Service) CollectNews(ctx context.Context, ticker string, force bool) error {
	now := time.Now()

	existing, _ := s.storage.MarketDataStorage().GetMarketData(ctx, ticker)
	marketData := &models.MarketData{
		Ticker:   ticker,
		Exchange: extractExchange(ticker),
	}
	if existing != nil {
		marketData = existing
	}

	if !force && existing != nil && common.IsFresh(existing.NewsUpdatedAt, common.FreshnessNews) {
		return nil
	}

	if s.eodhd == nil {
		return fmt.Errorf("EODHD client not configured")
	}

	news, err := s.eodhd.GetNews(ctx, ticker, 10)
	if err != nil {
		return fmt.Errorf("failed to fetch news: %w", err)
	}

	marketData.News = news
	marketData.NewsUpdatedAt = now
	marketData.DataVersion = common.SchemaVersion
	marketData.LastUpdated = now

	if err := s.storage.MarketDataStorage().SaveMarketData(ctx, marketData); err != nil {
		return fmt.Errorf("failed to save market data: %w", err)
	}

	return nil
}

// CollectFilingSummaries generates AI summaries for unsummarized filings.
func (s *Service) CollectFilingSummaries(ctx context.Context, ticker string, force bool) error {
	now := time.Now()

	existing, _ := s.storage.MarketDataStorage().GetMarketData(ctx, ticker)
	if existing == nil || len(existing.Filings) == 0 {
		return nil // No filings to summarize
	}

	if s.gemini == nil {
		return nil // AI not configured
	}

	marketData := existing

	// Save references to fields we'll nil for memory reduction during PDF processing.
	// These must be restored before any SaveMarketData call to avoid data loss.
	savedEOD := marketData.EOD
	savedNews := marketData.News
	savedNewsIntel := marketData.NewsIntelligence
	savedTimeline := marketData.CompanyTimeline

	// Free fields not needed for summarization to reduce memory footprint
	marketData.EOD = nil
	marketData.News = nil
	marketData.NewsIntelligence = nil
	marketData.CompanyTimeline = nil

	// restoreFields puts back the saved references before persisting
	restoreFields := func() {
		marketData.EOD = savedEOD
		marketData.News = savedNews
		marketData.NewsIntelligence = savedNewsIntel
		marketData.CompanyTimeline = savedTimeline
	}

	// Check if prompt template changed — if so, force regeneration
	currentHash := filingSummaryPromptHash()
	if marketData.FilingSummaryPromptHash != currentHash {
		force = true
		s.logger.Info().
			Str("ticker", ticker).
			Str("old_hash", marketData.FilingSummaryPromptHash).
			Str("new_hash", currentHash).
			Msg("Filing summary prompt changed, forcing regeneration")
	}

	if force {
		marketData.FilingSummaries = nil
		marketData.FilingSummariesUpdatedAt = time.Time{}
	}

	// Save callback: persist intermediate results after each batch.
	// Restores nil'd fields before saving to avoid data loss.
	saveFn := func(summaries []models.FilingSummary) error {
		restoreFields()
		marketData.FilingSummaries = summaries
		marketData.FilingSummariesUpdatedAt = now
		marketData.FilingSummaryPromptHash = currentHash
		marketData.DataVersion = common.SchemaVersion
		marketData.LastUpdated = now
		err := s.storage.MarketDataStorage().SaveMarketData(ctx, marketData)
		// Re-nil after save to keep memory low for next batch
		marketData.EOD = nil
		marketData.News = nil
		marketData.NewsIntelligence = nil
		marketData.CompanyTimeline = nil
		return err
	}

	newSummaries, changed := s.summarizeNewFilings(ctx, ticker, marketData.Filings, marketData.FilingSummaries, saveFn)
	if changed {
		restoreFields()
		marketData.FilingSummaries = newSummaries
		marketData.FilingSummariesUpdatedAt = now
		marketData.FilingSummaryPromptHash = currentHash
		marketData.DataVersion = common.SchemaVersion
		marketData.LastUpdated = now

		if err := s.storage.MarketDataStorage().SaveMarketData(ctx, marketData); err != nil {
			return fmt.Errorf("failed to save market data: %w", err)
		}
	} else {
		// Even if unchanged, restore fields on the shared pointer
		restoreFields()
	}

	return nil
}

// CollectTimeline generates or refreshes the company timeline.
func (s *Service) CollectTimeline(ctx context.Context, ticker string, force bool) error {
	now := time.Now()

	existing, _ := s.storage.MarketDataStorage().GetMarketData(ctx, ticker)
	if existing == nil || len(existing.FilingSummaries) == 0 {
		return nil // No summaries to build timeline from
	}

	if s.gemini == nil {
		return nil // AI not configured
	}

	marketData := existing

	if !force && common.IsFresh(marketData.CompanyTimelineUpdatedAt, common.FreshnessTimeline) {
		return nil
	}

	if force {
		marketData.CompanyTimeline = nil
		marketData.CompanyTimelineUpdatedAt = time.Time{}
	}

	timeline := s.generateCompanyTimeline(ctx, ticker, marketData.FilingSummaries, marketData.Fundamentals)
	if timeline != nil {
		marketData.CompanyTimeline = timeline
		marketData.CompanyTimelineUpdatedAt = now
		marketData.DataVersion = common.SchemaVersion
		marketData.LastUpdated = now

		if err := s.storage.MarketDataStorage().SaveMarketData(ctx, marketData); err != nil {
			return fmt.Errorf("failed to save market data: %w", err)
		}
	}

	return nil
}

// CollectNewsIntelligence generates AI news intelligence for a single ticker.
func (s *Service) CollectNewsIntelligence(ctx context.Context, ticker string, force bool) error {
	now := time.Now()

	existing, _ := s.storage.MarketDataStorage().GetMarketData(ctx, ticker)
	if existing == nil || len(existing.News) == 0 {
		return nil // No news to analyze
	}

	if s.gemini == nil {
		return nil // AI not configured
	}

	marketData := existing

	if !force && common.IsFresh(marketData.NewsIntelUpdatedAt, common.FreshnessNewsIntel) {
		return nil
	}

	intel := s.generateNewsIntelligence(ctx, ticker, marketData.Name, marketData.News)
	if intel != nil {
		marketData.NewsIntelligence = intel
		marketData.NewsIntelUpdatedAt = now
		marketData.DataVersion = common.SchemaVersion
		marketData.LastUpdated = now

		if err := s.storage.MarketDataStorage().SaveMarketData(ctx, marketData); err != nil {
			return fmt.Errorf("failed to save market data: %w", err)
		}
	}

	return nil
}
