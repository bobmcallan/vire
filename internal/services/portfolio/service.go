// Package portfolio provides portfolio management services
package portfolio

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
	strategypkg "github.com/bobmcallan/vire/internal/services/strategy"
	"github.com/bobmcallan/vire/internal/signals"
)

// Service implements PortfolioService
type Service struct {
	storage        interfaces.StorageManager
	navexa         interfaces.NavexaClient
	eodhd          interfaces.EODHDClient
	gemini         interfaces.GeminiClient
	signalComputer *signals.Computer
	logger         *common.Logger
	syncMu         sync.Mutex // serializes SyncPortfolio to prevent warm cache overwriting force sync
}

// NewService creates a new portfolio service
func NewService(
	storage interfaces.StorageManager,
	navexa interfaces.NavexaClient,
	eodhd interfaces.EODHDClient,
	gemini interfaces.GeminiClient,
	logger *common.Logger,
) *Service {
	return &Service{
		storage:        storage,
		navexa:         navexa,
		eodhd:          eodhd,
		gemini:         gemini,
		signalComputer: signals.NewComputer(),
		logger:         logger,
	}
}

// resolveNavexaClient returns a per-request Navexa client override from context.
// Returns an error if no client is available in the context.
func (s *Service) resolveNavexaClient(ctx context.Context) (interfaces.NavexaClient, error) {
	if override := common.NavexaClientFromContext(ctx); override != nil {
		return override, nil
	}
	return nil, fmt.Errorf("navexa client not available: portal headers required")
}

// SyncPortfolio refreshes portfolio data from Navexa
func (s *Service) SyncPortfolio(ctx context.Context, name string, force bool) (*models.Portfolio, error) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()

	navexaClient, err := s.resolveNavexaClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve navexa client: %w", err)
	}

	s.logger.Info().Str("name", name).Bool("force", force).Msg("Syncing portfolio")

	// Check if we need to sync
	if !force {
		existing, err := s.getPortfolioRecord(ctx, name)
		if err == nil && common.IsFresh(existing.LastSynced, common.FreshnessPortfolio) {
			s.logger.Debug().Str("name", name).Msg("Portfolio recently synced, skipping")
			return existing, nil
		}
	}

	// Get portfolios from Navexa
	navexaPortfolios, err := navexaClient.GetPortfolios(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get portfolios from Navexa: %w", err)
	}

	// Find matching portfolio
	var navexaPortfolio *models.NavexaPortfolio
	for _, p := range navexaPortfolios {
		if p.Name == name {
			navexaPortfolio = p
			break
		}
	}

	if navexaPortfolio == nil {
		return nil, fmt.Errorf("portfolio '%s' not found in Navexa", name)
	}

	// Use performance endpoint to get enriched holdings with financial data
	fromDate := navexaPortfolio.DateCreated
	if fromDate == "" {
		fromDate = "2020-01-01" // fallback
	}
	toDate := time.Now().Format("2006-01-02")

	s.logger.Info().
		Str("navexa_id", navexaPortfolio.ID).
		Str("from", fromDate).
		Str("to", toDate).
		Msg("Fetching enriched holdings from Navexa")

	navexaHoldings, err := navexaClient.GetEnrichedHoldings(ctx, navexaPortfolio.ID, fromDate, toDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get enriched holdings from Navexa: %w", err)
	}

	// Fetch trades per holding to compute accurate cost basis
	// (performance endpoint returns annualized values, not actual cost)
	holdingTrades := make(map[string][]*models.NavexaTrade) // ticker -> trades
	holdingMetrics := make(map[string]*holdingCalcMetrics)  // ticker -> computed return metrics
	for _, h := range navexaHoldings {
		if h.ID == "" {
			continue
		}
		trades, err := navexaClient.GetHoldingTrades(ctx, h.ID)
		if err != nil {
			s.logger.Warn().Err(err).Str("ticker", h.Ticker).Str("holdingID", h.ID).Msg("Failed to get trades for holding")
			continue
		}
		if len(trades) > 0 {
			holdingTrades[h.Ticker] = append(holdingTrades[h.Ticker], trades...)

			// Calculate average cost, remaining cost, and units from trades.
			// Trade-derived units are authoritative — Navexa performance endpoint
			// can return stale or rounded unit counts.
			avgCost, remainingCost, tradeUnits := calculateAvgCostFromTrades(trades)
			h.AvgCost = avgCost
			if math.Abs(tradeUnits-h.Units) > 0.01 {
				s.logger.Warn().
					Str("ticker", h.Ticker).
					Float64("navexa_units", h.Units).
					Float64("trade_units", tradeUnits).
					Msg("Units mismatch: overriding Navexa value with trade-derived units")
			}
			h.Units = tradeUnits
			h.MarketValue = h.CurrentPrice * h.Units

			// Calculate gain/loss using the simple, correct formula:
			// GainLoss = (proceeds from sells) + (current market value) - (total invested)
			totalInvested, totalProceeds, gainLoss := calculateGainLossFromTrades(trades, h.MarketValue)

			// TotalCost represents remaining cost basis (for position sizing)
			// For closed positions, use totalInvested; for open, use remainingCost
			if h.Units <= 0 {
				h.TotalCost = totalInvested
			} else {
				h.TotalCost = remainingCost
			}

			h.GainLoss = gainLoss

			// Simple percentage returns — denominator is total capital invested
			if totalInvested > 0 {
				h.GainLossPct = (h.GainLoss / totalInvested) * 100
			} else {
				h.GainLossPct = 0
			}

			// Realized/unrealized breakdown
			realizedGL := totalProceeds - (totalInvested - remainingCost)
			unrealizedGL := h.MarketValue - remainingCost
			holdingMetrics[h.Ticker] = &holdingCalcMetrics{
				totalInvested:      totalInvested,
				realizedGainLoss:   realizedGL,
				unrealizedGainLoss: unrealizedGL,
			}

			// XIRR annualised returns
			now := time.Now()
			h.CapitalGainPct = CalculateXIRR(trades, h.MarketValue, h.DividendReturn, false, now)
			h.TotalReturnPctIRR = CalculateXIRR(trades, h.MarketValue, h.DividendReturn, true, now)
		}
	}

	// Cross-check Navexa prices against EODHD close prices.
	// Navexa's performance API can return stale currentPrice values
	// (e.g. Friday's close on Monday evening). If EODHD has a more
	// recent bar, use its close price instead.
	for _, h := range navexaHoldings {
		if h.Units <= 0 {
			continue // skip closed positions
		}
		ticker := h.EODHDTicker()
		md, err := s.storage.MarketDataStorage().GetMarketData(ctx, ticker)
		if err != nil || md == nil || len(md.EOD) == 0 {
			continue
		}
		latestBar := md.EOD[0] // EOD is sorted descending (most recent first)

		// Use EODHD close if the bar is recent (within 24h) and differs from Navexa.
		// We use time.Since rather than date equality to avoid UTC vs AEST timezone
		// issues — the Docker container runs in UTC but ASX trades in AEST.
		// Prefer AdjClose over Close to handle corporate actions (e.g. consolidations).
		eodhPrice := eodClosePrice(latestBar)
		if time.Since(latestBar.Date) < 24*time.Hour && eodhPrice != h.CurrentPrice {
			s.logger.Info().
				Str("ticker", h.Ticker).
				Float64("navexa_price", h.CurrentPrice).
				Float64("eodhd_close", latestBar.Close).
				Float64("eodhd_adj_close", latestBar.AdjClose).
				Float64("eodhd_price_used", eodhPrice).
				Str("eodhd_bar_date", latestBar.Date.Format("2006-01-02")).
				Msg("Price refresh: using EODHD adjusted close (more recent than Navexa)")
			oldMarketValue := h.MarketValue
			h.CurrentPrice = eodhPrice
			h.MarketValue = h.CurrentPrice * h.Units
			// Adjust gain/loss by price change — preserves realised component
			h.GainLoss += h.MarketValue - oldMarketValue
			// Recompute simple percentages after price update
			if m, ok := holdingMetrics[h.Ticker]; ok && m.totalInvested > 0 {
				h.GainLossPct = (h.GainLoss / m.totalInvested) * 100
				// Update unrealized gain/loss by price delta
				m.unrealizedGainLoss += h.MarketValue - oldMarketValue
			} else if h.TotalCost > 0 {
				h.GainLossPct = (h.GainLoss / h.TotalCost) * 100
			} else {
				h.GainLossPct = 0
			}
			// Recompute XIRR with updated market value
			if trades := holdingTrades[h.Ticker]; len(trades) > 0 {
				now := time.Now()
				h.CapitalGainPct = CalculateXIRR(trades, h.MarketValue, h.DividendReturn, false, now)
				h.TotalReturnPctIRR = CalculateXIRR(trades, h.MarketValue, h.DividendReturn, true, now)
			}
		}
	}

	// Convert to internal model
	holdings := make([]models.Holding, len(navexaHoldings))
	hasUSD := false

	for i, h := range navexaHoldings {
		// Currency: default to AUD if empty
		currency := strings.ToUpper(h.Currency)
		if currency == "" {
			currency = "AUD"
		}
		if currency == "USD" {
			hasUSD = true
		}

		holdings[i] = models.Holding{
			Ticker:          h.Ticker,
			Exchange:        h.Exchange,
			Name:            h.Name,
			Units:           h.Units,
			AvgCost:         h.AvgCost,
			CurrentPrice:    h.CurrentPrice,
			MarketValue:     h.MarketValue,
			NetReturn:       h.GainLoss,
			NetReturnPct:    h.GainLossPct,
			TotalCost:       h.TotalCost,
			DividendReturn:  h.DividendReturn,
			CapitalGainPct:  h.CapitalGainPct,
			NetReturnPctIRR: h.TotalReturnPctIRR,
			Currency:        currency,
			Trades:          holdingTrades[h.Ticker],
			LastUpdated:     h.LastUpdated,
		}
		// Populate return breakdown from side map
		if m, ok := holdingMetrics[h.Ticker]; ok {
			holdings[i].TotalInvested = m.totalInvested
			holdings[i].RealizedNetReturn = m.realizedGainLoss
			holdings[i].UnrealizedNetReturn = m.unrealizedGainLoss
		}

		// Derived breakeven field (open positions only)
		if holdings[i].Units > 0 {
			trueBreakeven := (holdings[i].TotalCost - holdings[i].RealizedNetReturn) / holdings[i].Units
			holdings[i].TrueBreakevenPrice = &trueBreakeven
		}
	}

	// Fetch FX rate if any holdings are in USD (for AUD portfolio conversion)
	var fxRate float64
	if hasUSD && s.eodhd != nil {
		quote, err := s.eodhd.GetRealTimeQuote(ctx, "AUDUSD.FOREX")
		if err != nil {
			s.logger.Warn().Err(err).Msg("Failed to fetch AUDUSD forex rate; USD holdings will not be converted")
		} else if quote.Close > 0 {
			fxRate = quote.Close
			s.logger.Info().Float64("audusd_rate", fxRate).Msg("Fetched AUDUSD forex rate")
		}
	}

	// Compute TWRR and populate Country from stored fundamentals
	now := time.Now()
	for i := range holdings {
		ticker := holdings[i].EODHDTicker()
		md, err := s.storage.MarketDataStorage().GetMarketData(ctx, ticker)
		if err == nil && md != nil {
			// Populate country from stored fundamentals
			if md.Fundamentals != nil && md.Fundamentals.CountryISO != "" {
				holdings[i].Country = md.Fundamentals.CountryISO
			}
		}

		trades := holdings[i].Trades
		if len(trades) == 0 {
			continue
		}
		if err != nil {
			// No market data: TWRR will use fallback (trade price to current price)
			holdings[i].NetReturnPctTWRR = CalculateTWRR(trades, nil, holdings[i].CurrentPrice, now)
			continue
		}
		holdings[i].NetReturnPctTWRR = CalculateTWRR(trades, md.EOD, holdings[i].CurrentPrice, now)
	}

	// Convert USD holding values to AUD when FX rate is available.
	// AUDUSD rate means "how many USD per 1 AUD", so USD→AUD = value / rate.
	if fxRate > 0 {
		fxDiv := fxRate // USD to AUD divisor
		for i := range holdings {
			if holdings[i].Currency != "USD" {
				continue
			}
			holdings[i].OriginalCurrency = "USD"
			holdings[i].CurrentPrice /= fxDiv
			holdings[i].AvgCost /= fxDiv
			holdings[i].MarketValue /= fxDiv
			holdings[i].TotalCost /= fxDiv
			holdings[i].TotalInvested /= fxDiv
			holdings[i].NetReturn /= fxDiv
			holdings[i].RealizedNetReturn /= fxDiv
			holdings[i].UnrealizedNetReturn /= fxDiv
			holdings[i].DividendReturn /= fxDiv
			if holdings[i].TrueBreakevenPrice != nil {
				converted := *holdings[i].TrueBreakevenPrice / fxDiv
				holdings[i].TrueBreakevenPrice = &converted
			}
			holdings[i].Currency = "AUD"
		}
	}

	// Compute portfolio-level totals — all holdings are now in AUD (or unconverted if FX failed).
	var totalValue, totalCost, totalGain, totalDividends float64
	var totalRealizedNetReturn, totalUnrealizedNetReturn float64
	for _, h := range holdings {
		totalValue += h.MarketValue
		totalDividends += h.DividendReturn
		totalGain += h.NetReturn
		totalRealizedNetReturn += h.RealizedNetReturn
		totalUnrealizedNetReturn += h.UnrealizedNetReturn
		if h.Units > 0 {
			totalCost += h.TotalCost
		}
	}
	totalGain += totalDividends

	// Preserve external balances from existing portfolio across re-syncs
	var existingExternalBalances []models.ExternalBalance
	var existingExternalBalanceTotal float64
	if existing, err := s.getPortfolioRecord(ctx, name); err == nil {
		existingExternalBalances = existing.ExternalBalances
		existingExternalBalanceTotal = existing.ExternalBalanceTotal
	}

	// Calculate weights using total value + external balance total as denominator
	weightDenom := totalValue + existingExternalBalanceTotal
	for i := range holdings {
		if weightDenom > 0 {
			holdings[i].Weight = (holdings[i].MarketValue / weightDenom) * 100
		}
	}

	totalGainPct := 0.0
	if totalCost > 0 {
		// Percentage return relative to currently deployed capital
		totalGainPct = (totalGain / totalCost) * 100
	}

	portfolio := &models.Portfolio{
		ID:                       name,
		Name:                     name,
		NavexaID:                 navexaPortfolio.ID,
		Holdings:                 holdings,
		TotalValue:               totalValue,
		TotalCost:                totalCost,
		TotalNetReturn:           totalGain,
		TotalNetReturnPct:        totalGainPct,
		Currency:                 navexaPortfolio.Currency,
		FXRate:                   fxRate,
		TotalRealizedNetReturn:   totalRealizedNetReturn,
		TotalUnrealizedNetReturn: totalUnrealizedNetReturn,
		CalculationMethod:        "average_cost",
		ExternalBalances:         existingExternalBalances,
		ExternalBalanceTotal:     existingExternalBalanceTotal,
		LastSynced:               time.Now(),
	}

	// Save portfolio
	if err := s.savePortfolioRecord(ctx, portfolio); err != nil {
		return nil, fmt.Errorf("failed to save portfolio: %w", err)
	}

	// Upsert tickers to stock index for job manager tracking
	stockIndex := s.storage.StockIndexStore()
	for _, h := range holdings {
		entry := &models.StockIndexEntry{
			Ticker:   h.EODHDTicker(),
			Code:     h.Ticker,
			Exchange: h.Exchange,
			Name:     h.Name,
			Source:   "portfolio",
		}
		if err := stockIndex.Upsert(ctx, entry); err != nil {
			s.logger.Warn().Str("ticker", h.EODHDTicker()).Err(err).Msg("Failed to upsert stock index")
		}
	}

	s.logger.Info().Str("name", name).Int("holdings", len(holdings)).Msg("Portfolio synced")

	return portfolio, nil
}

// GetPortfolio retrieves a portfolio with current data
func (s *Service) GetPortfolio(ctx context.Context, name string) (*models.Portfolio, error) {
	portfolio, err := s.getPortfolioRecord(ctx, name)
	if err != nil {
		// Auto-sync on first access if Navexa client available
		if synced, syncErr := s.SyncPortfolio(ctx, name, false); syncErr == nil {
			return synced, nil
		}
		return nil, err
	}

	// Auto-refresh if stale
	if !common.IsFresh(portfolio.LastSynced, common.FreshnessPortfolio) {
		if synced, syncErr := s.SyncPortfolio(ctx, name, false); syncErr == nil {
			return synced, nil
		}
	}

	return portfolio, nil
}

// ListPortfolios returns available portfolio names
func (s *Service) ListPortfolios(ctx context.Context) ([]string, error) {
	userID := common.ResolveUserID(ctx)
	records, err := s.storage.UserDataStore().List(ctx, userID, "portfolio")
	if err != nil {
		return nil, err
	}
	names := make([]string, len(records))
	for i, r := range records {
		names[i] = r.Key
	}
	return names, nil
}

// ReviewPortfolio generates a portfolio review with signals
func (s *Service) ReviewPortfolio(ctx context.Context, name string, options interfaces.ReviewOptions) (*models.PortfolioReview, error) {
	serviceStart := time.Now()
	s.logger.Info().Str("name", name).Msg("Generating portfolio review")

	// Phase 1: Get portfolio and strategy
	phaseStart := time.Now()
	portfolio, err := s.GetPortfolio(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get portfolio: %w", err)
	}

	// Load strategy (nil if none exists — all behaviour unchanged)
	var strategy *models.PortfolioStrategy
	if strat, err := s.getStrategyRecord(ctx, name); err == nil {
		strategy = strat
	}
	s.logger.Info().Dur("elapsed", time.Since(phaseStart)).Msg("ReviewPortfolio: portfolio+strategy load complete")

	review := &models.PortfolioReview{
		PortfolioName:     name,
		ReviewDate:        time.Now(),
		TotalValue:        portfolio.TotalValue + portfolio.ExternalBalanceTotal,
		TotalCost:         portfolio.TotalCost,
		TotalNetReturn:    portfolio.TotalNetReturn,
		TotalNetReturnPct: portfolio.TotalNetReturnPct,
		FXRate:            portfolio.FXRate,
	}

	// Separate active and closed positions
	activeHoldings, closedHoldings := filterClosedPositions(portfolio.Holdings)
	if len(closedHoldings) > 0 {
		s.logger.Info().
			Int("closed", len(closedHoldings)).
			Int("active", len(activeHoldings)).
			Msg("Separated closed positions (0 units)")
	}

	holdingReviews := make([]models.HoldingReview, 0, len(activeHoldings))
	alerts := make([]models.Alert, 0)
	dayChange := 0.0

	// Phase 2: Batch load all market data
	phaseStart = time.Now()
	tickers := make([]string, 0, len(activeHoldings))
	for _, h := range activeHoldings {
		tickers = append(tickers, h.EODHDTicker())
	}
	allMarketData, err := s.storage.MarketDataStorage().GetMarketDataBatch(ctx, tickers)
	if err != nil {
		s.logger.Warn().Err(err).Msg("Failed to batch load market data")
	}
	mdByTicker := make(map[string]*models.MarketData, len(allMarketData))
	for _, md := range allMarketData {
		mdByTicker[md.Ticker] = md
	}
	s.logger.Info().Dur("elapsed", time.Since(phaseStart)).Int("tickers", len(tickers)).Msg("ReviewPortfolio: market data batch load complete")

	// Phase 2b: Fetch real-time quotes for active holdings
	phaseStart = time.Now()
	liveQuotes := make(map[string]*models.RealTimeQuote, len(tickers))
	if s.eodhd != nil {
		for _, ticker := range tickers {
			if quote, err := s.eodhd.GetRealTimeQuote(ctx, ticker); err == nil && quote.Close > 0 {
				liveQuotes[ticker] = quote
			} else if err != nil {
				s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Real-time quote unavailable for holding")
			}
		}
	}
	s.logger.Info().Dur("elapsed", time.Since(phaseStart)).Int("live_quotes", len(liveQuotes)).Msg("ReviewPortfolio: real-time quotes complete")

	// Phase 3: Holdings loop (signals + review)
	phaseStart = time.Now()
	for _, holding := range activeHoldings {
		ticker := holding.EODHDTicker()

		// Get market data from pre-loaded batch
		marketData := mdByTicker[ticker]
		if marketData == nil {
			s.logger.Warn().Str("ticker", ticker).Msg("No market data in batch — including holding without signals")
			holdingReviews = append(holdingReviews, models.HoldingReview{
				Holding:        holding,
				ActionRequired: "HOLD",
				ActionReason:   "Market data unavailable — signals and compliance pending data collection",
			})
			continue
		}

		// Get or compute signals (persist computed signals for future reuse)
		tickerSignals, err := s.storage.SignalStorage().GetSignals(ctx, ticker)
		if err != nil {
			tickerSignals = s.signalComputer.Compute(marketData)
			if saveErr := s.storage.SignalStorage().SaveSignals(ctx, tickerSignals); saveErr != nil {
				s.logger.Warn().Err(saveErr).Str("ticker", ticker).Msg("Failed to persist computed signals")
			}
		}

		// Calculate overnight movement — prefer real-time price over EOD[0].Close.
		// Live quotes and EOD bars are in native currency; holding values may be
		// AUD-converted. Apply FX conversion for originally-USD holdings.
		overnightMove := 0.0
		overnightPct := 0.0
		fxDiv := 1.0
		if holding.OriginalCurrency == "USD" && portfolio.FXRate > 0 {
			fxDiv = portfolio.FXRate
		}
		if quote, ok := liveQuotes[ticker]; ok && len(marketData.EOD) > 1 {
			prevClose := marketData.EOD[1].Close
			overnightMove = (quote.Close - prevClose) / fxDiv
			overnightPct = (overnightMove / (prevClose / fxDiv)) * 100
			// Update holding with live price for the review (converted to AUD)
			holding.CurrentPrice = quote.Close / fxDiv
			holding.MarketValue = holding.CurrentPrice * holding.Units
		} else if len(marketData.EOD) > 1 {
			overnightMove = (marketData.EOD[0].Close - marketData.EOD[1].Close) / fxDiv
			overnightPct = (overnightMove / (marketData.EOD[1].Close / fxDiv)) * 100
		}

		// Determine action (strategy-aware thresholds)
		action, reason := determineAction(tickerSignals, options.FocusSignals, strategy, &holding, marketData.Fundamentals)

		holdingReview := models.HoldingReview{
			Holding:        holding,
			Signals:        tickerSignals,
			Fundamentals:   marketData.Fundamentals,
			OvernightMove:  overnightMove,
			OvernightPct:   overnightPct,
			ActionRequired: action,
			ActionReason:   reason,
		}

		// Compliance check
		if strategy != nil {
			sectorWeight := computeHoldingSectorWeight(holding, activeHoldings, marketData.Fundamentals)
			holdingReview.Compliance = strategypkg.CheckCompliance(
				strategy, &holding, tickerSignals, marketData.Fundamentals, sectorWeight)
		}

		// Add news impact if available and requested
		if options.IncludeNews && len(marketData.News) > 0 {
			holdingReview.NewsImpact = summarizeNewsImpact(marketData.News)
		}

		// Attach news intelligence if available
		if marketData.NewsIntelligence != nil {
			holdingReview.NewsIntelligence = marketData.NewsIntelligence
		}

		// Attach filings intelligence (deprecated) if available
		if marketData.FilingsIntelligence != nil {
			holdingReview.FilingsIntelligence = marketData.FilingsIntelligence
		}

		// Attach 3-layer assessment data
		holdingReview.FilingSummaries = marketData.FilingSummaries
		holdingReview.Timeline = marketData.CompanyTimeline

		holdingReviews = append(holdingReviews, holdingReview)

		// Track day change
		dayChange += overnightMove * holding.Units

		// Generate alerts (strategy-aware)
		holdingAlerts := generateAlerts(holding, tickerSignals, options.FocusSignals, strategy)
		alerts = append(alerts, holdingAlerts...)
	}
	s.logger.Info().Dur("elapsed", time.Since(phaseStart)).Int("holdings", len(activeHoldings)).Msg("ReviewPortfolio: holdings loop complete")

	// Add closed positions (no market data or signals needed)
	for _, holding := range closedHoldings {
		holdingReviews = append(holdingReviews, models.HoldingReview{
			Holding:        holding,
			ActionRequired: "CLOSED",
			ActionReason:   "Position exited",
		})
	}

	review.HoldingReviews = holdingReviews
	review.Alerts = alerts
	review.DayChange = dayChange

	// Recompute TotalValue from live-updated holdings (all values already in AUD)
	liveTotal := 0.0
	for _, hr := range holdingReviews {
		if hr.ActionRequired != "CLOSED" {
			liveTotal += hr.Holding.MarketValue
		}
	}
	if liveTotal > 0 {
		review.TotalValue = liveTotal + portfolio.ExternalBalanceTotal
	}

	if review.TotalValue > 0 {
		review.DayChangePct = (dayChange / review.TotalValue) * 100
	}

	// Phase 3: Generate AI summary if available
	if s.gemini != nil {
		phaseStart = time.Now()
		summary, err := s.generateReviewSummary(ctx, review, strategy)
		if err != nil {
			s.logger.Warn().Err(err).Msg("Failed to generate AI summary")
		} else {
			review.Summary = summary
		}
		s.logger.Info().Dur("elapsed", time.Since(phaseStart)).Msg("ReviewPortfolio: AI summary complete")
	}

	// Generate observations (strategy-aware)
	review.Recommendations = generateRecommendations(review, strategy)

	// Generate portfolio balance analysis
	review.PortfolioBalance = analyzePortfolioBalance(review.HoldingReviews)

	// Update strategy LastReviewedAt
	if strategy != nil {
		strategy.LastReviewedAt = time.Now()
		if err := s.saveStrategyRecord(ctx, strategy); err != nil {
			s.logger.Warn().Err(err).Msg("Failed to update strategy LastReviewedAt")
		}
	}

	s.logger.Info().
		Str("name", name).
		Int("holdings", len(holdingReviews)).
		Int("alerts", len(alerts)).
		Dur("elapsed", time.Since(serviceStart)).
		Msg("ReviewPortfolio: TOTAL")

	return review, nil
}

// filterClosedPositions separates holdings into active (units > 0) and closed (units == 0).
func filterClosedPositions(holdings []models.Holding) (active, closed []models.Holding) {
	active = make([]models.Holding, 0, len(holdings))
	closed = make([]models.Holding, 0)
	for _, h := range holdings {
		if h.Units > 0 {
			active = append(active, h)
		} else {
			closed = append(closed, h)
		}
	}
	return active, closed
}

// strategyRSIThresholds returns the RSI overbought (sell) and oversold (buy) thresholds
// adjusted for the portfolio strategy's risk appetite level.
func strategyRSIThresholds(strategy *models.PortfolioStrategy) (overboughtSell float64, oversoldBuy float64) {
	if strategy == nil || strategy.RiskAppetite.Level == "" {
		return 70, 30 // defaults (moderate)
	}
	switch strings.ToLower(strategy.RiskAppetite.Level) {
	case "conservative":
		return 65, 35 // sell earlier, buy later (more cautious)
	case "aggressive":
		return 80, 25 // sell later, buy earlier (more risk-tolerant)
	default: // "moderate" or unknown
		return 70, 30
	}
}

// determineAction determines the compliance status for a holding.
// Strategy-aware: adjusts RSI and SMA thresholds based on risk appetite.
// User-defined rules at priority >0 override hardcoded indicator logic.
func determineAction(signals *models.TickerSignals, focusSignals []string, strategy *models.PortfolioStrategy, holding *models.Holding, fundamentals *models.Fundamentals) (string, string) {
	if signals == nil {
		return "COMPLIANT", "Insufficient data"
	}

	// Evaluate user-defined rules (priority > 0 overrides hardcoded logic)
	if strategy != nil && len(strategy.Rules) > 0 {
		ruleCtx := strategypkg.RuleContext{Holding: holding, Signals: signals, Fundamentals: fundamentals}
		results := strategypkg.EvaluateRules(strategy.Rules, ruleCtx)
		if len(results) > 0 && results[0].Rule.Priority > 0 {
			return string(results[0].Rule.Action), results[0].Reason
		}
	}

	rsiOverbought, rsiOversold := strategyRSIThresholds(strategy)

	// Strategy: position weight exceeds max
	if strategy != nil && holding != nil && strategy.PositionSizing.MaxPositionPct > 0 {
		if holding.Weight > strategy.PositionSizing.MaxPositionPct {
			return "WATCH", fmt.Sprintf("Position weight %.1f%% exceeds strategy max %.1f%%",
				holding.Weight, strategy.PositionSizing.MaxPositionPct)
		}
	}

	// Check for exit triggers
	if signals.Technical.RSI > rsiOverbought {
		return "EXIT TRIGGER", fmt.Sprintf("RSI overbought (>%.0f)", rsiOverbought)
	}
	if signals.Technical.SMA20CrossSMA50 == "death_cross" {
		return "EXIT TRIGGER", "Recent death cross (SMA20 below SMA50)"
	}
	if signals.Trend == models.TrendBearish && signals.Price.DistanceToSMA200 < -20 {
		return "EXIT TRIGGER", "Extended below 200-day SMA in downtrend (>20%)"
	}

	// Check for entry criteria
	if signals.Technical.RSI < rsiOversold {
		return "ENTRY CRITERIA MET", fmt.Sprintf("RSI oversold (<%.0f)", rsiOversold)
	}
	if signals.Technical.SMA20CrossSMA50 == "golden_cross" {
		return "ENTRY CRITERIA MET", "Recent golden cross (SMA20 above SMA50)"
	}

	// Check for watch signals
	if signals.Technical.NearSupport {
		return "WATCH", "Testing support level"
	}
	if signals.Technical.NearResistance {
		return "WATCH", "Testing resistance level"
	}

	return "COMPLIANT", "All indicators within tolerance"
}

// generateAlerts creates alerts for a holding (strategy-aware)
func generateAlerts(holding models.Holding, signals *models.TickerSignals, focusSignals []string, strategy *models.PortfolioStrategy) []models.Alert {
	alerts := make([]models.Alert, 0)

	if signals == nil {
		return alerts
	}

	// RSI alerts (thresholds adjusted by strategy risk level)
	rsiOverbought, rsiOversold := strategyRSIThresholds(strategy)
	if signals.Technical.RSI > rsiOverbought {
		alerts = append(alerts, models.Alert{
			Type:     models.AlertTypeSignal,
			Severity: "high",
			Ticker:   holding.Ticker,
			Message:  fmt.Sprintf("%s RSI is overbought at %.1f (threshold: %.0f)", holding.Ticker, signals.Technical.RSI, rsiOverbought),
			Signal:   "rsi_overbought",
		})
	} else if signals.Technical.RSI < rsiOversold {
		alerts = append(alerts, models.Alert{
			Type:     models.AlertTypeSignal,
			Severity: "medium",
			Ticker:   holding.Ticker,
			Message:  fmt.Sprintf("%s RSI is oversold at %.1f (threshold: %.0f)", holding.Ticker, signals.Technical.RSI, rsiOversold),
			Signal:   "rsi_oversold",
		})
	}

	// SMA crossover alerts
	if signals.Technical.SMA20CrossSMA50 == "death_cross" {
		alerts = append(alerts, models.Alert{
			Type:     models.AlertTypeSignal,
			Severity: "high",
			Ticker:   holding.Ticker,
			Message:  fmt.Sprintf("%s has a death cross (SMA20 below SMA50)", holding.Ticker),
			Signal:   "death_cross",
		})
	} else if signals.Technical.SMA20CrossSMA50 == "golden_cross" {
		alerts = append(alerts, models.Alert{
			Type:     models.AlertTypeSignal,
			Severity: "medium",
			Ticker:   holding.Ticker,
			Message:  fmt.Sprintf("%s has a golden cross (SMA20 above SMA50)", holding.Ticker),
			Signal:   "golden_cross",
		})
	}

	// Volume alerts
	if signals.Technical.VolumeSignal == "spike" {
		alerts = append(alerts, models.Alert{
			Type:     models.AlertTypeVolume,
			Severity: "medium",
			Ticker:   holding.Ticker,
			Message:  fmt.Sprintf("%s has unusual volume (%.1fx average)", holding.Ticker, signals.Technical.VolumeRatio),
			Signal:   "volume_spike",
		})
	}

	// Risk alerts
	for _, flag := range signals.RiskFlags {
		alerts = append(alerts, models.Alert{
			Type:     models.AlertTypeRisk,
			Severity: "low",
			Ticker:   holding.Ticker,
			Message:  fmt.Sprintf("%s: %s", holding.Ticker, flag),
			Signal:   flag,
		})
	}

	// Strategy-alignment alerts
	if strategy != nil {
		// Position size exceeds strategy max
		if strategy.PositionSizing.MaxPositionPct > 0 && holding.Weight > strategy.PositionSizing.MaxPositionPct {
			alerts = append(alerts, models.Alert{
				Type:     models.AlertTypeStrategy,
				Severity: "medium",
				Ticker:   holding.Ticker,
				Message: fmt.Sprintf("%s weight %.1f%% exceeds strategy max position size of %.1f%%",
					holding.Ticker, holding.Weight, strategy.PositionSizing.MaxPositionPct),
				Signal: "strategy_position_size",
			})
		}
	}

	return alerts
}

// summarizeNewsImpact creates a brief news impact summary
func summarizeNewsImpact(news []*models.NewsItem) string {
	if len(news) == 0 {
		return ""
	}

	// Count sentiment
	positive := 0
	negative := 0
	for _, item := range news {
		switch item.Sentiment {
		case "positive":
			positive++
		case "negative":
			negative++
		}
	}

	if positive > negative {
		return fmt.Sprintf("Positive news sentiment (%d positive, %d negative)", positive, negative)
	} else if negative > positive {
		return fmt.Sprintf("Negative news sentiment (%d positive, %d negative)", positive, negative)
	}
	return fmt.Sprintf("Mixed news sentiment (%d positive, %d negative)", positive, negative)
}

// generateRecommendations creates observations based on compliance status (strategy-aware)
func generateRecommendations(review *models.PortfolioReview, strategy *models.PortfolioStrategy) []string {
	recommendations := make([]string, 0)

	// Prompt the user when no strategy is defined — compliance checks require one
	if strategy == nil {
		recommendations = append(recommendations,
			"No portfolio strategy defined — compliance review requires a strategy. "+
				"Would you like to define one now, or just review the stock positions without compliance?")
	}

	// Count actions
	sellCount := 0
	buyCount := 0
	watchCount := 0

	for _, hr := range review.HoldingReviews {
		switch hr.ActionRequired {
		case "EXIT TRIGGER":
			sellCount++
		case "ENTRY CRITERIA MET":
			buyCount++
		case "WATCH":
			watchCount++
		}
	}

	if sellCount > 0 {
		recommendations = append(recommendations,
			fmt.Sprintf("Review %d holdings with exit triggers active", sellCount))
	}

	if buyCount > 0 {
		recommendations = append(recommendations,
			fmt.Sprintf("Review %d holdings where entry criteria are met", buyCount))
	}

	if watchCount > 0 {
		recommendations = append(recommendations,
			fmt.Sprintf("Monitor %d holdings at key support/resistance levels", watchCount))
	}

	// Portfolio-level observations
	highAlerts := 0
	strategyAlerts := 0
	for _, alert := range review.Alerts {
		if alert.Severity == "high" {
			highAlerts++
		}
		if alert.Type == models.AlertTypeStrategy {
			strategyAlerts++
		}
	}

	if highAlerts > 0 {
		recommendations = append(recommendations,
			fmt.Sprintf("Address %d high-priority alerts requiring immediate attention", highAlerts))
	}

	// Strategy-specific observations
	if strategy != nil {
		if strategyAlerts > 0 {
			recommendations = append(recommendations,
				fmt.Sprintf("Review %d strategy-alignment issues (position sizing, sector limits)", strategyAlerts))
		}

		// Check sector concentration against strategy
		if review.PortfolioBalance != nil && strategy.PositionSizing.MaxSectorPct > 0 {
			for _, sa := range review.PortfolioBalance.SectorAllocations {
				if sa.Weight > strategy.PositionSizing.MaxSectorPct {
					recommendations = append(recommendations,
						fmt.Sprintf("Sector '%s' at %.1f%% exceeds strategy limit of %.1f%%",
							sa.Sector, sa.Weight, strategy.PositionSizing.MaxSectorPct))
					break // Only report the first one
				}
			}
		}
	}

	return recommendations
}

// generateReviewSummary creates an AI summary of the portfolio review
func (s *Service) generateReviewSummary(ctx context.Context, review *models.PortfolioReview, strategy *models.PortfolioStrategy) (string, error) {
	prompt := buildReviewSummaryPrompt(review, strategy)
	return s.gemini.GenerateContent(ctx, prompt)
}

func buildReviewSummaryPrompt(review *models.PortfolioReview, strategy *models.PortfolioStrategy) string {
	prompt := fmt.Sprintf(`Summarize this portfolio review for %s:

Portfolio Value: $%.2f
Day Change: $%.2f (%.2f%%)
Holdings: %d
Alerts: %d

`,
		review.PortfolioName,
		review.TotalValue,
		review.DayChange,
		review.DayChangePct,
		len(review.HoldingReviews),
		len(review.Alerts),
	)

	// Add structured strategy context (no free-text fields to avoid prompt injection)
	if strategy != nil {
		prompt += fmt.Sprintf("Portfolio strategy: %s risk appetite, targeting %.1f%% annual return",
			strategy.RiskAppetite.Level, strategy.TargetReturns.AnnualPct)
		if strategy.AccountType != "" {
			prompt += fmt.Sprintf(", %s account", string(strategy.AccountType))
		}
		prompt += "\n\n"
	}

	// Add holding summaries
	prompt += "Holdings requiring action:\n"
	for _, hr := range review.HoldingReviews {
		if hr.ActionRequired != "COMPLIANT" {
			prompt += fmt.Sprintf("- %s: %s (%s)\n", hr.Holding.Ticker, hr.ActionRequired, hr.ActionReason)
		}
	}

	// Add high-priority alerts
	prompt += "\nHigh-priority alerts:\n"
	for _, alert := range review.Alerts {
		if alert.Severity == "high" {
			prompt += fmt.Sprintf("- %s\n", alert.Message)
		}
	}

	prompt += "\nProvide a 2-3 sentence executive summary highlighting the most important actions to take today."

	return prompt
}

// calculateAvgCostFromTrades computes the weighted-average cost and remaining units from trade history.
// Handles Buy, Sell, Cost Base Increase/Decrease, and Opening Balance trade types.
func calculateAvgCostFromTrades(trades []*models.NavexaTrade) (avgCost, totalCost, units float64) {
	totalCost = 0.0
	units = 0.0

	for _, t := range trades {
		switch strings.ToLower(t.Type) {
		case "buy", "opening balance":
			cost := t.Units*t.Price + t.Fees
			totalCost += cost
			units += t.Units
		case "sell":
			if units > 0 {
				// Reduce cost proportionally
				costPerUnit := totalCost / units
				totalCost -= t.Units * costPerUnit
				units -= t.Units
			}
		case "cost base increase":
			totalCost += t.Value
		case "cost base decrease":
			totalCost -= t.Value
		}
	}

	if units > 0 {
		avgCost = totalCost / units
	}

	return avgCost, totalCost, units
}

// calculateRealizedFromTrades computes realized gain/loss for fully-sold positions.
// It sums total invested (all buys + fees) and total proceeds (all sells - fees) independently.
func calculateRealizedFromTrades(trades []*models.NavexaTrade) (avgBuyPrice, totalInvested, totalProceeds, realizedGain float64) {
	var totalBuyUnits float64
	for _, t := range trades {
		switch strings.ToLower(t.Type) {
		case "buy", "opening balance":
			totalInvested += t.Units*t.Price + t.Fees
			totalBuyUnits += t.Units
		case "sell":
			totalProceeds += t.Units*t.Price - t.Fees
		case "cost base increase":
			totalInvested += t.Value
		case "cost base decrease":
			totalInvested -= t.Value
		}
	}
	if totalBuyUnits > 0 {
		avgBuyPrice = totalInvested / totalBuyUnits
	}
	realizedGain = totalProceeds - totalInvested
	return
}

// calculateGainLossFromTrades computes the total gain/loss for a position.
// Works for both open and closed positions:
//   - GainLoss = (proceeds from sells) + (current market value) - (total invested)
//
// This is the simple, correct calculation for share/stock gain/loss.
func calculateGainLossFromTrades(trades []*models.NavexaTrade, currentMarketValue float64) (totalInvested, totalProceeds, gainLoss float64) {
	for _, t := range trades {
		switch strings.ToLower(t.Type) {
		case "buy", "opening balance":
			totalInvested += t.Units*t.Price + t.Fees
		case "sell":
			totalProceeds += t.Units*t.Price - t.Fees
		case "cost base increase":
			totalInvested += t.Value
		case "cost base decrease":
			totalInvested -= t.Value
		}
	}
	gainLoss = (totalProceeds + currentMarketValue) - totalInvested
	return
}

// analyzePortfolioBalance calculates sector allocation and diversification metrics
func analyzePortfolioBalance(holdings []models.HoldingReview) *models.PortfolioBalance {
	if len(holdings) == 0 {
		return nil
	}

	// Sector classification
	sectorMap := make(map[string][]string) // sector -> tickers
	sectorWeight := make(map[string]float64)
	totalWeight := 0.0

	// Defensive sectors (lower beta, stable earnings)
	defensiveSectors := map[string]bool{
		"Consumer Defensive": true, "Utilities": true, "Healthcare": true,
		"Consumer Staples": true, "Real Estate": true,
	}
	// Growth sectors (higher beta, earnings growth focus)
	growthSectors := map[string]bool{
		"Technology": true, "Consumer Cyclical": true, "Communication Services": true,
		"Industrials": true,
	}

	defensiveWeight := 0.0
	growthWeight := 0.0
	incomeWeight := 0.0

	for _, hr := range holdings {
		// Skip closed positions (no market value to allocate)
		if hr.ActionRequired == "CLOSED" {
			continue
		}

		weight := hr.Holding.Weight
		totalWeight += weight

		sector := "Unknown"
		if hr.Fundamentals != nil && hr.Fundamentals.Sector != "" {
			sector = hr.Fundamentals.Sector
		}

		sectorMap[sector] = append(sectorMap[sector], hr.Holding.Ticker)
		sectorWeight[sector] += weight

		if defensiveSectors[sector] {
			defensiveWeight += weight
		}
		if growthSectors[sector] {
			growthWeight += weight
		}

		// High dividend yield (>4%) considered income-focused
		if hr.Fundamentals != nil && hr.Fundamentals.DividendYield > 0.04 {
			incomeWeight += weight
		}
	}

	// Build sector allocations
	allocations := make([]models.SectorAllocation, 0, len(sectorMap))
	for sector, tickers := range sectorMap {
		allocations = append(allocations, models.SectorAllocation{
			Sector:   sector,
			Weight:   sectorWeight[sector],
			Holdings: tickers,
		})
	}

	// Sort by weight descending
	for i := 0; i < len(allocations)-1; i++ {
		for j := i + 1; j < len(allocations); j++ {
			if allocations[j].Weight > allocations[i].Weight {
				allocations[i], allocations[j] = allocations[j], allocations[i]
			}
		}
	}

	// Concentration risk: if top sector > 40% or top 2 sectors > 60%
	concentrationRisk := "low"
	if len(allocations) > 0 && allocations[0].Weight > 40 {
		concentrationRisk = "high"
	} else if len(allocations) > 1 && allocations[0].Weight+allocations[1].Weight > 60 {
		concentrationRisk = "medium"
	}

	// Diversification note
	note := generateDiversificationNote(len(holdings), len(allocations), defensiveWeight, growthWeight, concentrationRisk)

	return &models.PortfolioBalance{
		SectorAllocations:   allocations,
		DefensiveWeight:     defensiveWeight,
		GrowthWeight:        growthWeight,
		IncomeWeight:        incomeWeight,
		ConcentrationRisk:   concentrationRisk,
		DiversificationNote: note,
	}
}

// generateDiversificationNote creates analysis commentary without prescriptive advice
func generateDiversificationNote(holdingCount, sectorCount int, defensive, growth float64, risk string) string {
	notes := []string{}

	// Holding count observation
	if holdingCount < 5 {
		notes = append(notes, fmt.Sprintf("Concentrated portfolio with %d holdings", holdingCount))
	} else if holdingCount > 20 {
		notes = append(notes, fmt.Sprintf("Diversified across %d holdings", holdingCount))
	}

	// Sector spread
	if sectorCount < 3 {
		notes = append(notes, fmt.Sprintf("exposed to %d sectors", sectorCount))
	} else {
		notes = append(notes, fmt.Sprintf("spread across %d sectors", sectorCount))
	}

	// Style tilt
	if defensive > growth+15 {
		notes = append(notes, "tilted toward defensive positioning")
	} else if growth > defensive+15 {
		notes = append(notes, "tilted toward growth positioning")
	} else if defensive > 0 && growth > 0 {
		notes = append(notes, "balanced between growth and defensive")
	}

	// Concentration
	if risk == "high" {
		notes = append(notes, "with significant sector concentration")
	}

	if len(notes) == 0 {
		return "Portfolio composition analysis pending fundamental data."
	}

	// Capitalize first note
	if len(notes) > 0 && len(notes[0]) > 0 {
		notes[0] = strings.ToUpper(notes[0][:1]) + notes[0][1:]
	}

	return strings.Join(notes, ", ") + "."
}

// computeHoldingSectorWeight sums the portfolio weight for all holdings in the same sector.
// Uses the fundamentals of the target holding to identify its sector.
func computeHoldingSectorWeight(holding models.Holding, allHoldings []models.Holding, fundamentals *models.Fundamentals) float64 {
	if fundamentals == nil || fundamentals.Sector == "" {
		return 0
	}
	// For a proper implementation we would look up each holding's fundamentals.
	// Since we only have the current holding's fundamentals available in this loop,
	// we approximate by returning the holding's own weight. The full sector weight
	// is already computed in analyzePortfolioBalance for the portfolio-level view.
	// Compliance will flag only when the sector allocation from the balance analysis
	// exceeds the limit.
	return holding.Weight
}

// --- UserDataStore helpers ---

func (s *Service) getPortfolioRecord(ctx context.Context, name string) (*models.Portfolio, error) {
	userID := common.ResolveUserID(ctx)
	rec, err := s.storage.UserDataStore().Get(ctx, userID, "portfolio", name)
	if err != nil {
		return nil, fmt.Errorf("portfolio '%s' not found: %w", name, err)
	}
	var portfolio models.Portfolio
	if err := json.Unmarshal([]byte(rec.Value), &portfolio); err != nil {
		return nil, fmt.Errorf("failed to unmarshal portfolio '%s': %w", name, err)
	}
	if portfolio.DataVersion != common.SchemaVersion {
		return nil, fmt.Errorf("portfolio '%s' has stale schema version %s (current: %s)", name, portfolio.DataVersion, common.SchemaVersion)
	}
	return &portfolio, nil
}

func (s *Service) savePortfolioRecord(ctx context.Context, portfolio *models.Portfolio) error {
	portfolio.DataVersion = common.SchemaVersion
	userID := common.ResolveUserID(ctx)
	data, err := json.Marshal(portfolio)
	if err != nil {
		return fmt.Errorf("failed to marshal portfolio: %w", err)
	}
	return s.storage.UserDataStore().Put(ctx, &models.UserRecord{
		UserID:  userID,
		Subject: "portfolio",
		Key:     portfolio.Name,
		Value:   string(data),
	})
}

func (s *Service) getStrategyRecord(ctx context.Context, portfolioName string) (*models.PortfolioStrategy, error) {
	userID := common.ResolveUserID(ctx)
	rec, err := s.storage.UserDataStore().Get(ctx, userID, "strategy", portfolioName)
	if err != nil {
		return nil, fmt.Errorf("strategy for '%s' not found: %w", portfolioName, err)
	}
	var strategy models.PortfolioStrategy
	if err := json.Unmarshal([]byte(rec.Value), &strategy); err != nil {
		return nil, fmt.Errorf("failed to unmarshal strategy: %w", err)
	}
	return &strategy, nil
}

func (s *Service) saveStrategyRecord(ctx context.Context, strategy *models.PortfolioStrategy) error {
	userID := common.ResolveUserID(ctx)
	data, err := json.Marshal(strategy)
	if err != nil {
		return fmt.Errorf("failed to marshal strategy: %w", err)
	}
	return s.storage.UserDataStore().Put(ctx, &models.UserRecord{
		UserID:  userID,
		Subject: "strategy",
		Key:     strategy.PortfolioName,
		Value:   string(data),
	})
}

// eodClosePrice returns the best available close price from an EOD bar.
// Prefers AdjClose (adjusted for corporate actions like consolidations) when
// available and positive; falls back to Close otherwise.
// Includes a sanity check: if AdjClose diverges from Close by more than 50%,
// it indicates bad corporate action data (e.g., consolidation where EODHD
// back-adjusted AdjClose but Close reflects the actual trading price).
func eodClosePrice(bar models.EODBar) float64 {
	if bar.AdjClose > 0 && !math.IsInf(bar.AdjClose, 0) && !math.IsNaN(bar.AdjClose) {
		// Sanity check: for recent bars, AdjClose should be close to Close.
		// A large divergence (>50%) indicates bad corporate action data.
		if bar.Close > 0 {
			ratio := bar.AdjClose / bar.Close
			if ratio < 0.5 || ratio > 2.0 {
				return bar.Close
			}
		}
		return bar.AdjClose
	}
	return bar.Close
}

// holdingCalcMetrics stores per-holding calculation results computed during
// trade processing, used to populate Holding model fields after conversion.
type holdingCalcMetrics struct {
	totalInvested      float64
	realizedGainLoss   float64
	unrealizedGainLoss float64
}

// Ensure Service implements PortfolioService
var _ interfaces.PortfolioService = (*Service)(nil)
