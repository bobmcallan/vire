package market

import (
	"context"
	"fmt"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

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

	if force {
		marketData.FilingSummaries = nil
		marketData.FilingSummariesUpdatedAt = time.Time{}
	}

	newSummaries, changed := s.summarizeNewFilings(ctx, ticker, marketData.Filings, marketData.FilingSummaries)
	if changed {
		marketData.FilingSummaries = newSummaries
		marketData.FilingSummariesUpdatedAt = now
		marketData.DataVersion = common.SchemaVersion
		marketData.LastUpdated = now

		if err := s.storage.MarketDataStorage().SaveMarketData(ctx, marketData); err != nil {
			return fmt.Errorf("failed to save market data: %w", err)
		}
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
