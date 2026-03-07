// Package portfolio provides portfolio management services
package portfolio

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"sort"
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
	storage            interfaces.StorageManager
	navexa             interfaces.NavexaClient
	eodhd              interfaces.EODHDClient
	gemini             interfaces.GeminiClient
	cashflowSvc        interfaces.CashFlowService
	tradeService       interfaces.TradeService
	signalComputer     *signals.Computer
	holdingNoteService interfaces.HoldingNoteService
	assetSetSvc        interfaces.AssetSetService
	logger             *common.Logger
	syncMu             sync.Mutex // serializes SyncPortfolio to prevent warm cache overwriting force sync
	timelineRebuilding sync.Map   // map[string]bool — true while a rebuild goroutine runs
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

// SetCashFlowService sets the cash flow service dependency.
// Called after construction to break the circular dependency between
// portfolio and cashflow services.
func (s *Service) SetCashFlowService(svc interfaces.CashFlowService) {
	s.cashflowSvc = svc
}

// SetTradeService sets the trade service dependency.
func (s *Service) SetTradeService(svc interfaces.TradeService) {
	s.tradeService = svc
}

// SetHoldingNoteService injects the holding note service (setter injection to avoid circular deps)
func (s *Service) SetHoldingNoteService(svc interfaces.HoldingNoteService) {
	s.holdingNoteService = svc
}

// SetAssetSetService injects the asset set service (setter injection to avoid circular deps)
func (s *Service) SetAssetSetService(svc interfaces.AssetSetService) {
	s.assetSetSvc = svc
}

// populateAssetSetValues loads non-equity asset set values and adds them to portfolio totals.
func (s *Service) populateAssetSetValues(ctx context.Context, portfolio *models.Portfolio) {
	if s.assetSetSvc == nil {
		return
	}
	assetSets, err := s.assetSetSvc.GetAssetSets(ctx, portfolio.Name)
	if err != nil || assetSets == nil {
		return
	}
	portfolio.AssetSetsValue = assetSets.TotalValue()
	portfolio.PortfolioValue += portfolio.AssetSetsValue
}

// IsTimelineRebuilding returns true when a full timeline rebuild is in progress
// for the named portfolio.
func (s *Service) IsTimelineRebuilding(name string) bool {
	v, ok := s.timelineRebuilding.Load(name)
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

// rebuildTimelineWithCash loads the cash ledger and recomputes the full portfolio
// timeline with cash transactions included. This ensures persisted snapshots have
// correct portfolio_value = equity_value + net_cash_balance.
//
// GetDailyGrowth now auto-loads cash transactions internally, so this method
// is now just a convenience wrapper. Callers can directly call GetDailyGrowth
// if preferred.
func (s *Service) rebuildTimelineWithCash(ctx context.Context, name string) ([]models.GrowthDataPoint, error) {
	return s.GetDailyGrowth(ctx, name, interfaces.GrowthOptions{})
}

// triggerTimelineRebuildAsync spawns a background goroutine to fully recompute
// the portfolio timeline. Sets the rebuilding flag for the duration.
// Call when the trade hash changes and the timeline cache has been invalidated.
func (s *Service) triggerTimelineRebuildAsync(ctx context.Context, name string) {
	// Dedup: skip if a rebuild is already in progress for this portfolio.
	// Prevents concurrent full recomputes which waste resources and produce the same result.
	if s.IsTimelineRebuilding(name) {
		s.logger.Info().Str("portfolio", name).Msg("Timeline rebuild already in progress — skipping")
		return
	}
	s.timelineRebuilding.Store(name, true)
	go func() {
		defer func() {
			s.timelineRebuilding.Store(name, false)
			if r := recover(); r != nil {
				s.logger.Warn().Str("portfolio", name).Msgf("Timeline rebuild panic recovered: %v", r)
			}
		}()
		bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		bgCtx = common.WithUserContext(bgCtx, common.UserContextFromContext(ctx))
		// CRITICAL: use rebuildTimelineWithCash to include cash transactions.
		// Bare GetDailyGrowth with empty GrowthOptions excludes cash from persisted snapshots.
		if _, err := s.rebuildTimelineWithCash(bgCtx, name); err != nil {
			s.logger.Warn().Err(err).Str("portfolio", name).Msg("Timeline rebuild after trade change failed")
			return
		}
		s.logger.Info().Str("portfolio", name).Msg("Timeline rebuild after trade change complete")
	}()
}

// InvalidateAndRebuildTimeline deletes persisted timeline data and triggers an
// async rebuild with cash transactions. Used when external data (e.g. cash ledger)
// changes that affect historical portfolio values.
func (s *Service) InvalidateAndRebuildTimeline(ctx context.Context, name string) {
	if s.IsTimelineRebuilding(name) {
		s.logger.Info().Str("portfolio", name).Msg("Timeline rebuild already in progress — skipping invalidation")
		return
	}

	userID := common.ResolveUserID(ctx)
	if tl := s.storage.TimelineStore(); tl != nil {
		if _, err := tl.DeleteAll(ctx, userID, name); err != nil {
			s.logger.Warn().Err(err).Str("portfolio", name).Msg("Timeline invalidation: delete failed")
		}
	}

	s.triggerTimelineRebuildAsync(ctx, name)
}

// ForceRebuildTimeline is an admin-only operation that deletes ALL timeline data
// for a portfolio and triggers a full from-scratch recompute. Unlike
// InvalidateAndRebuildTimeline, this does NOT skip if a rebuild is in progress —
// it forcefully clears everything and starts fresh.
func (s *Service) ForceRebuildTimeline(ctx context.Context, name string) error {
	userID := common.ResolveUserID(ctx)
	tl := s.storage.TimelineStore()
	if tl == nil {
		return fmt.Errorf("timeline store not available")
	}

	deleted, err := tl.DeleteAll(ctx, userID, name)
	if err != nil {
		return fmt.Errorf("failed to delete timeline data: %w", err)
	}
	s.logger.Info().Str("portfolio", name).Int("deleted", deleted).Msg("Admin force rebuild: timeline data deleted")

	// Force: bypass dedup check by resetting flag first
	s.timelineRebuilding.Store(name, false)
	s.triggerTimelineRebuildAsync(ctx, name)
	return nil
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

	// Check freshness: force=false uses standard TTL (30 min),
	// force=true uses shorter cooldown (5 min) to prevent rapid re-syncs.
	// Capture existing trade hash for timeline invalidation detection later.
	var existingTradeHash string
	if existing, err := s.getPortfolioRecord(ctx, name); err == nil {
		existingTradeHash = existing.TradeHash
		ttl := common.FreshnessPortfolio
		if force {
			ttl = common.FreshnessSyncCooldown
		}
		if common.IsFresh(existing.LastSynced, ttl) {
			s.logger.Debug().Str("name", name).Bool("force", force).
				Dur("ttl", ttl).Msg("Portfolio within sync cooldown, returning cached")
			s.populateHistoricalValues(ctx, existing)
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

	// Fetch trades per holding concurrently to compute accurate cost basis.
	// (performance endpoint returns annualized values, not actual cost)
	// Sequential fetching at 5 req/s across 40+ holdings exceeds typical
	// request timeouts, so we fan out with bounded concurrency.
	type tradeResult struct {
		holding *models.NavexaHolding
		trades  []*models.NavexaTrade
	}

	const tradeFetchWorkers = 10
	tradeCh := make(chan *models.NavexaHolding, len(navexaHoldings))
	resultCh := make(chan tradeResult, len(navexaHoldings))

	var tradeWg sync.WaitGroup
	for i := 0; i < tradeFetchWorkers; i++ {
		tradeWg.Add(1)
		go func() {
			defer tradeWg.Done()
			for h := range tradeCh {
				trades, err := navexaClient.GetHoldingTrades(ctx, h.ID)
				if err != nil {
					s.logger.Warn().Err(err).Str("ticker", h.Ticker).Str("holdingID", h.ID).Msg("Failed to get trades for holding")
					continue
				}
				if len(trades) > 0 {
					resultCh <- tradeResult{holding: h, trades: trades}
				}
			}
		}()
	}

	for _, h := range navexaHoldings {
		if h.ID == "" {
			continue
		}
		tradeCh <- h
	}
	close(tradeCh)

	go func() {
		tradeWg.Wait()
		close(resultCh)
	}()

	holdingTrades := make(map[string][]*models.NavexaTrade) // ticker -> trades
	holdingMetrics := make(map[string]*holdingCalcMetrics)  // ticker -> computed return metrics
	for res := range resultCh {
		h := res.holding
		trades := res.trades
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
			totalProceeds:      totalProceeds,
			realizedGainLoss:   realizedGL,
			unrealizedGainLoss: unrealizedGL,
		}

		// XIRR annualised returns
		now := time.Now()
		h.CapitalGainPct = CalculateXIRR(trades, h.MarketValue, h.DividendReturn, false, now)
		h.TotalReturnPctIRR = CalculateXIRR(trades, h.MarketValue, h.DividendReturn, true, now)
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
			// Guard: reject EODHD price if it diverges >50% from Navexa — indicates
			// wrong instrument mapping in EODHD (e.g. ticker resolves to different security).
			if h.CurrentPrice > 0 {
				divergencePct := math.Abs(eodhPrice-h.CurrentPrice) / h.CurrentPrice * 100
				if divergencePct > 50.0 {
					s.logger.Warn().
						Str("ticker", h.Ticker).
						Float64("navexa_price", h.CurrentPrice).
						Float64("eodhd_price", eodhPrice).
						Float64("divergence_pct", divergencePct).
						Msg("EODHD price rejected: >50% divergence suggests wrong instrument mapping")
					continue
				}
			}
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
			Ticker:                     h.Ticker,
			Exchange:                   h.Exchange,
			Name:                       h.Name,
			Units:                      h.Units,
			AvgCost:                    h.AvgCost,
			CurrentPrice:               h.CurrentPrice,
			MarketValue:                h.MarketValue,
			ReturnNet:                  h.GainLoss,
			ReturnNetPct:               h.GainLossPct,
			CostBasis:                  h.TotalCost,
			DividendReturn:             h.DividendReturn,
			AnnualizedCapitalReturnPct: h.CapitalGainPct,
			AnnualizedTotalReturnPct:   h.TotalReturnPctIRR,
			Currency:                   currency,
			Trades:                     holdingTrades[h.Ticker],
			LastUpdated:                h.LastUpdated,
		}
		// Populate return breakdown from side map
		if m, ok := holdingMetrics[h.Ticker]; ok {
			holdings[i].GrossInvested = m.totalInvested
			holdings[i].GrossProceeds = m.totalProceeds
			holdings[i].RealizedReturn = m.realizedGainLoss
			holdings[i].UnrealizedReturn = m.unrealizedGainLoss
		}

		// Mark position status and compute breakeven for open positions
		if holdings[i].Units > 0 {
			if holdings[i].CurrentPrice == 0 {
				holdings[i].Status = "delisted"
			} else {
				holdings[i].Status = "open"
			}
			trueBreakeven := (holdings[i].CostBasis - holdings[i].RealizedReturn) / holdings[i].Units
			holdings[i].TrueBreakevenPrice = &trueBreakeven
		} else {
			holdings[i].Status = "closed"
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

	// Batch-fetch market data for TWRR and country population
	syncTickers := make([]string, 0, len(holdings))
	for _, h := range holdings {
		if h.Units > 0 || len(h.Trades) > 0 {
			syncTickers = append(syncTickers, h.EODHDTicker())
		}
	}
	mdByTicker := make(map[string]*models.MarketData)
	if mds := s.storage.MarketDataStorage(); mds != nil && len(syncTickers) > 0 {
		allMD, _ := mds.GetMarketDataBatch(ctx, syncTickers)
		for _, md := range allMD {
			mdByTicker[md.Ticker] = md
		}
	}

	// Compute TWRR and populate Country from stored fundamentals
	now := time.Now()
	for i := range holdings {
		ticker := holdings[i].EODHDTicker()
		md := mdByTicker[ticker]
		if md != nil {
			// Populate country from stored fundamentals
			if md.Fundamentals != nil && md.Fundamentals.CountryISO != "" {
				holdings[i].Country = md.Fundamentals.CountryISO
			}
		}

		trades := holdings[i].Trades
		if len(trades) == 0 {
			continue
		}
		if md == nil {
			// No market data: TWRR will use fallback (trade price to current price)
			holdings[i].TimeWeightedReturnPct = CalculateTWRR(trades, nil, holdings[i].CurrentPrice, now)
			continue
		}
		holdings[i].TimeWeightedReturnPct = CalculateTWRR(trades, md.EOD, holdings[i].CurrentPrice, now)
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
			holdings[i].CostBasis /= fxDiv
			holdings[i].GrossInvested /= fxDiv
			holdings[i].GrossProceeds /= fxDiv
			holdings[i].ReturnNet /= fxDiv
			holdings[i].RealizedReturn /= fxDiv
			holdings[i].UnrealizedReturn /= fxDiv
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
		totalGain += h.ReturnNet
		totalRealizedNetReturn += h.RealizedReturn
		totalUnrealizedNetReturn += h.UnrealizedReturn
		// Net capital in equities: buys - sells (all holdings, open + closed)
		totalCost += h.GrossInvested - h.GrossProceeds
	}
	totalGain += totalDividends

	// Compute total cash balance, ledger dividend total, and dividend forecast from cashflow ledger.
	var totalCash, ledgerDividends, dividendForecast float64
	dividendForecast = totalDividends // default: all Navexa dividends are forecasted
	if s.cashflowSvc != nil {
		if ledger, err := s.cashflowSvc.GetLedger(ctx, name); err == nil && ledger != nil {
			totalCash = ledger.TotalCashBalance()
			summary := ledger.Summary()
			ledgerDividends = summary.NetCashByCategory[string(models.CashCatDividend)]

			// Compute dividend forecast: Navexa total minus Navexa forecast for holdings
			// that have confirmed dividend payments in the ledger. We subtract the FORECAST
			// amount (not the actual), because the actual may differ from the forecast.
			// e.g. TWR forecast $895.86, actual $761.48 → subtract $895.86 from forecast.
			paidTickers := make(map[string]bool)
			for _, tx := range ledger.Transactions {
				if tx.Category == models.CashCatDividend && tx.Ticker != "" {
					paidTickers[tx.Ticker] = true
				}
			}
			if len(paidTickers) > 0 {
				var paidForecast float64
				for _, h := range holdings {
					if paidTickers[h.EODHDTicker()] && h.DividendReturn > 0 {
						paidForecast += h.DividendReturn
					}
				}
				dividendForecast = totalDividends - paidForecast
			}
		}
	}

	// Available cash = ledger balance minus capital locked in equities.
	// Only meaningful when cash transactions have been recorded (totalCash > 0).
	// With no ledger entries, available cash is 0 — the concept doesn't apply.
	availableCash := 0.0
	if totalCash != 0 {
		availableCash = totalCash - totalCost
	}

	// Calculate weights using total value + available cash as denominator
	weightDenom := totalValue + availableCash
	for i := range holdings {
		if weightDenom > 0 {
			holdings[i].WeightPct = (holdings[i].MarketValue / weightDenom) * 100
		}
	}

	totalGainPct := 0.0
	if totalCost > 0 {
		// Percentage return relative to currently deployed capital
		totalGainPct = (totalGain / totalCost) * 100
	}

	// Compute trade hash for timeline invalidation detection.
	// If trades or cash transactions have changed since last sync,
	// the persisted timeline is stale and must be recomputed.
	tradeHash := computeTradeHash(holdings)

	portfolio := &models.Portfolio{
		ID:                       name,
		Name:                     name,
		NavexaID:                 navexaPortfolio.ID,
		Holdings:                 holdings,
		EquityHoldingsValue:      totalValue,
		PortfolioValue:           totalValue + availableCash,
		EquityHoldingsCost:       totalCost,
		EquityHoldingsReturn:     totalGain,
		EquityHoldingsReturnPct:  totalGainPct,
		Currency:                 navexaPortfolio.Currency,
		FXRate:                   fxRate,
		EquityHoldingsRealized:   totalRealizedNetReturn,
		EquityHoldingsUnrealized: totalUnrealizedNetReturn,
		IncomeDividendsForecast:  dividendForecast,
		IncomeDividendsReceived:  ledgerDividends,
		CalculationMethod:        "average_cost",
		TradeHash:                tradeHash,
		CapitalGross:             totalCash,
		CapitalAvailable:         availableCash,
		LastSynced:               time.Now(),
	}

	// Invalidate persisted timeline if trade data changed since last sync.
	tradeHashChanged := existingTradeHash != "" && existingTradeHash != tradeHash
	if tradeHashChanged {
		s.logger.Info().Str("portfolio", name).Msg("Trade data changed — invalidating timeline cache")
		userID := common.ResolveUserID(ctx)
		if tl := s.storage.TimelineStore(); tl != nil {
			if _, err := tl.DeleteAll(ctx, userID, name); err != nil {
				s.logger.Warn().Err(err).Str("portfolio", name).Msg("Failed to invalidate timeline cache")
			}
		}
	}

	// Save portfolio
	if err := s.savePortfolioRecord(ctx, portfolio); err != nil {
		return nil, fmt.Errorf("failed to save portfolio: %w", err)
	}

	// Trigger explicit timeline rebuild when trades changed. Must happen after
	// savePortfolioRecord so GetDailyGrowth reads the new trade data.
	if tradeHashChanged {
		s.triggerTimelineRebuildAsync(ctx, name)
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

	// Write today's timeline snapshot synchronously.
	// This is the authoritative "today" value, updated every sync cycle (~5-30 min).
	s.writeTodaySnapshot(ctx, portfolio)

	// Backfill historical timeline if empty (e.g. after a schema rebuild).
	// Runs in background to avoid blocking the sync response.
	s.backfillTimelineIfEmpty(ctx, portfolio)

	// Include non-equity asset set values in portfolio totals
	s.populateAssetSetValues(ctx, portfolio)

	// Populate historical values (yesterday/last week) from EOD market data
	s.populateHistoricalValues(ctx, portfolio)

	return portfolio, nil
}

// GetPortfolio retrieves a portfolio with current data
func (s *Service) GetPortfolio(ctx context.Context, name string) (*models.Portfolio, error) {
	portfolio, err := s.getPortfolioRecord(ctx, name)
	if err != nil {
		// For Navexa portfolios: auto-sync on first access
		if synced, syncErr := s.SyncPortfolio(ctx, name, false); syncErr == nil {
			return synced, nil
		}
		return nil, err
	}

	// Populate timeline rebuilding flag for all return paths
	portfolio.TimelineRebuilding = s.IsTimelineRebuilding(name)

	// Route by source type
	switch portfolio.SourceType {
	case models.SourceManual:
		return s.assembleManualPortfolio(ctx, portfolio)
	case models.SourceSnapshot:
		return s.assembleSnapshotPortfolio(ctx, portfolio)
	case models.SourceNavexa, "":
		// Existing Navexa behaviour
		if !common.IsFresh(portfolio.LastSynced, common.FreshnessPortfolio) {
			if synced, syncErr := s.SyncPortfolio(ctx, name, false); syncErr == nil {
				synced.TimelineRebuilding = s.IsTimelineRebuilding(name)
				return synced, nil
			}
		}
		s.populateAssetSetValues(ctx, portfolio)
		s.populateHistoricalValues(ctx, portfolio)
		return portfolio, nil
	default:
		// Unknown source type, return as-is
		s.populateAssetSetValues(ctx, portfolio)
		s.populateHistoricalValues(ctx, portfolio)
		return portfolio, nil
	}
}

// assembleManualPortfolio builds a portfolio from trade history.
func (s *Service) assembleManualPortfolio(ctx context.Context, portfolio *models.Portfolio) (*models.Portfolio, error) {
	if s.tradeService == nil {
		return portfolio, nil
	}
	derived, err := s.tradeService.DeriveHoldings(ctx, portfolio.Name)
	if err != nil {
		return nil, fmt.Errorf("deriving holdings from trades: %w", err)
	}

	holdings := make([]models.Holding, 0, len(derived))
	var totalEquityValue, totalCost, totalRealized, totalUnrealized, totalGrossInvested float64
	for _, dh := range derived {
		h := models.Holding{
			Ticker:           dh.Ticker,
			Exchange:         models.EodhExchange(tickerExchange(dh.Ticker)),
			Name:             dh.Ticker, // will be enriched with market data if available
			Units:            dh.Units,
			AvgCost:          dh.AvgCost,
			CostBasis:        dh.CostBasis,
			GrossInvested:    dh.GrossInvested,
			GrossProceeds:    dh.GrossProceeds,
			RealizedReturn:   dh.RealizedReturn,
			UnrealizedReturn: dh.UnrealizedReturn,
			SourceType:       models.SourceManual,
			Currency:         portfolio.Currency,
		}

		if dh.Units > 0 {
			h.Status = "open"
			h.CurrentPrice = dh.AvgCost
			h.MarketValue = dh.AvgCost * dh.Units

			// Try to enrich with current market price
			if s.eodhd != nil {
				if md, err := s.storage.MarketDataStorage().GetMarketData(ctx, dh.Ticker); err == nil && md != nil && len(md.EOD) > 0 {
					latestPrice := md.EOD[len(md.EOD)-1].Close
					if latestPrice > 0 {
						h.CurrentPrice = latestPrice
						h.MarketValue = latestPrice * dh.Units
						h.UnrealizedReturn = h.MarketValue - dh.CostBasis
					}
				}
			}

			h.ReturnNet = h.RealizedReturn + h.UnrealizedReturn
			if h.GrossInvested > 0 {
				h.ReturnNetPct = (h.ReturnNet / h.GrossInvested) * 100
			}

			// Only open positions count toward aggregates
			totalEquityValue += h.MarketValue
			totalCost += h.CostBasis
			totalRealized += h.RealizedReturn
			totalUnrealized += h.UnrealizedReturn
			totalGrossInvested += h.GrossInvested
		} else {
			h.Status = "closed"
			// Closed: realized return is the final P&L
			h.ReturnNet = h.RealizedReturn
			if h.GrossInvested > 0 {
				h.ReturnNetPct = (h.ReturnNet / h.GrossInvested) * 100
			}
		}

		holdings = append(holdings, h)
	}

	// Compute portfolio weights
	for i := range holdings {
		if totalEquityValue > 0 {
			holdings[i].WeightPct = (holdings[i].MarketValue / totalEquityValue) * 100
		}
	}

	portfolio.Holdings = holdings
	portfolio.EquityHoldingsValue = totalEquityValue
	portfolio.EquityHoldingsCost = totalCost
	portfolio.EquityHoldingsReturn = totalRealized + totalUnrealized
	if totalGrossInvested > 0 {
		portfolio.EquityHoldingsReturnPct = (portfolio.EquityHoldingsReturn / totalGrossInvested) * 100
	}
	portfolio.EquityHoldingsRealized = totalRealized
	portfolio.EquityHoldingsUnrealized = totalUnrealized
	portfolio.PortfolioValue = totalEquityValue + portfolio.CapitalGross
	portfolio.CalculationMethod = "average_cost"

	s.populateAssetSetValues(ctx, portfolio)
	s.populateHistoricalValues(ctx, portfolio)
	return portfolio, nil
}

// assembleSnapshotPortfolio builds a portfolio from snapshot positions.
func (s *Service) assembleSnapshotPortfolio(ctx context.Context, portfolio *models.Portfolio) (*models.Portfolio, error) {
	if s.tradeService == nil {
		return portfolio, nil
	}
	tb, err := s.tradeService.GetTradeBook(ctx, portfolio.Name)
	if err != nil {
		return nil, err
	}

	holdings := make([]models.Holding, 0, len(tb.SnapshotPositions))
	var totalEquityValue, totalCost float64
	for _, sp := range tb.SnapshotPositions {
		costBasis := sp.AvgCost * sp.Units
		marketValue := sp.MarketValue
		if marketValue == 0 && sp.CurrentPrice > 0 {
			marketValue = sp.CurrentPrice * sp.Units
		}
		if marketValue == 0 {
			marketValue = costBasis // fallback
		}

		h := models.Holding{
			Ticker:           sp.Ticker,
			Exchange:         models.EodhExchange(tickerExchange(sp.Ticker)),
			Name:             sp.Name,
			Status:           "open",
			Units:            sp.Units,
			AvgCost:          sp.AvgCost,
			CurrentPrice:     sp.CurrentPrice,
			MarketValue:      marketValue,
			CostBasis:        costBasis,
			GrossInvested:    costBasis + sp.FeesTotal,
			UnrealizedReturn: marketValue - costBasis,
			SourceType:       models.SourceSnapshot,
			SourceRef:        sp.SourceRef,
			Currency:         portfolio.Currency,
		}
		h.ReturnNet = h.UnrealizedReturn
		if h.GrossInvested > 0 {
			h.ReturnNetPct = (h.ReturnNet / h.GrossInvested) * 100
		}

		totalEquityValue += h.MarketValue
		totalCost += h.CostBasis
		holdings = append(holdings, h)
	}

	// Compute weights
	for i := range holdings {
		if totalEquityValue > 0 {
			holdings[i].WeightPct = (holdings[i].MarketValue / totalEquityValue) * 100
		}
	}

	portfolio.Holdings = holdings
	portfolio.EquityHoldingsValue = totalEquityValue
	portfolio.EquityHoldingsCost = totalCost
	portfolio.EquityHoldingsReturn = totalEquityValue - totalCost
	if totalCost > 0 {
		portfolio.EquityHoldingsReturnPct = ((totalEquityValue - totalCost) / totalCost) * 100
	}
	portfolio.EquityHoldingsUnrealized = totalEquityValue - totalCost
	portfolio.PortfolioValue = totalEquityValue + portfolio.CapitalGross
	portfolio.CalculationMethod = "snapshot"

	s.populateAssetSetValues(ctx, portfolio)
	s.populateHistoricalValues(ctx, portfolio)
	return portfolio, nil
}

// tickerExchange extracts the exchange suffix from a ticker (e.g. "BHP.AU" → "AU").
func tickerExchange(ticker string) string {
	parts := strings.SplitN(ticker, ".", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return "AU"
}

// CreatePortfolio creates a new manually-managed portfolio.
func (s *Service) CreatePortfolio(ctx context.Context, name string, sourceType models.SourceType, currency string) (*models.Portfolio, error) {
	if !models.ValidPortfolioSourceTypes[sourceType] {
		return nil, fmt.Errorf("invalid source_type: %q (valid: manual, snapshot, hybrid)", sourceType)
	}
	if name == "" {
		return nil, fmt.Errorf("portfolio name is required")
	}
	if len(name) > 100 {
		return nil, fmt.Errorf("portfolio name too long (max 100 chars)")
	}

	// Check if already exists
	existing, _ := s.getPortfolioRecord(ctx, name)
	if existing != nil {
		return nil, fmt.Errorf("portfolio %q already exists", name)
	}

	// Default currency
	if currency == "" {
		currency = "AUD"
	}

	now := time.Now()
	portfolio := &models.Portfolio{
		Name:        name,
		SourceType:  sourceType,
		Currency:    currency,
		DataVersion: common.SchemaVersion,
		LastSynced:  now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.savePortfolioRecord(ctx, portfolio); err != nil {
		return nil, err
	}
	return portfolio, nil
}

// populateHistoricalValues adds yesterday and last week values to holdings and portfolio totals.
// Tries timeline snapshots first for portfolio-level aggregates (avoids market data batch load).
// Falls back to EOD bars for per-holding price changes and when no timeline data exists.
func (s *Service) populateHistoricalValues(ctx context.Context, portfolio *models.Portfolio) {
	now := time.Now().Truncate(24 * time.Hour)
	yesterday := now.AddDate(0, 0, -1)
	lastWeek := now.AddDate(0, 0, -7)

	// Try timeline-sourced portfolio aggregates first.
	timelineHit := s.populateFromTimeline(ctx, portfolio, yesterday, lastWeek)
	if timelineHit {
		s.logger.Debug().Str("portfolio", portfolio.Name).Msg("Portfolio aggregates populated from timeline")
	}

	// Per-holding historical prices always need market data (timeline doesn't store per-holding data).
	// Also compute portfolio aggregates from market data if timeline wasn't available.
	s.populateFromMarketData(ctx, portfolio, !timelineHit)

	// Compute breadth summary from holdings with trend data
	s.computeBreadth(portfolio)

	// Compute net flow fields from cash flow ledger
	s.populateNetFlows(ctx, portfolio)

	// Add changes section
	s.populateChanges(ctx, portfolio)
}

// populateFromTimeline sets portfolio-level yesterday/lastWeek values from timeline snapshots.
// Returns true if at least yesterday's snapshot was found.
func (s *Service) populateFromTimeline(ctx context.Context, portfolio *models.Portfolio, yesterday, lastWeek time.Time) bool {
	tl := s.storage.TimelineStore()
	if tl == nil {
		return false
	}

	userID := common.ResolveUserID(ctx)

	// Get yesterday's snapshot
	yesterdaySnaps, err := tl.GetRange(ctx, userID, portfolio.Name, yesterday, yesterday)
	if err != nil || len(yesterdaySnaps) == 0 {
		return false
	}

	ySnap := yesterdaySnaps[0]
	portfolio.PortfolioYesterdayValue = ySnap.PortfolioValue
	if portfolio.PortfolioYesterdayValue > 0 {
		portfolio.PortfolioYesterdayChangePct = ((portfolio.PortfolioValue - portfolio.PortfolioYesterdayValue) / portfolio.PortfolioYesterdayValue) * 100
	}

	// Get last week's snapshot
	lastWeekSnaps, err := tl.GetRange(ctx, userID, portfolio.Name, lastWeek, lastWeek)
	if err == nil && len(lastWeekSnaps) > 0 {
		lwSnap := lastWeekSnaps[0]
		portfolio.PortfolioLastWeekValue = lwSnap.PortfolioValue
		if portfolio.PortfolioLastWeekValue > 0 {
			portfolio.PortfolioLastWeekChangePct = ((portfolio.PortfolioValue - portfolio.PortfolioLastWeekValue) / portfolio.PortfolioLastWeekValue) * 100
		}
	}

	return true
}

// populateFromMarketData loads EOD bars and sets per-holding historical prices.
// When computeAggregates is true, also computes portfolio-level yesterday/lastWeek totals.
func (s *Service) populateFromMarketData(ctx context.Context, portfolio *models.Portfolio, computeAggregates bool) {
	tickers := make([]string, 0, len(portfolio.Holdings))
	for _, h := range portfolio.Holdings {
		if h.Units > 0 {
			tickers = append(tickers, h.EODHDTicker())
		}
	}
	if len(tickers) == 0 {
		return
	}

	allMarketData, err := s.storage.MarketDataStorage().GetMarketDataBatch(ctx, tickers)
	if err != nil {
		s.logger.Warn().Err(err).Msg("Failed to load market data for historical values")
		return
	}

	mdByTicker := make(map[string]*models.MarketData, len(allMarketData))
	for _, md := range allMarketData {
		mdByTicker[md.Ticker] = md
	}

	// Batch-fetch signals for all open holdings
	var signalsByTicker map[string]*models.TickerSignals
	if ss := s.storage.SignalStorage(); ss != nil {
		if allSignals, err := ss.GetSignalsBatch(ctx, tickers); err == nil {
			signalsByTicker = make(map[string]*models.TickerSignals, len(allSignals))
			for _, sig := range allSignals {
				signalsByTicker[sig.Ticker] = sig
			}
		}
	}

	var yesterdayTotal, lastWeekTotal float64

	for i := range portfolio.Holdings {
		h := &portfolio.Holdings[i]
		if h.Units <= 0 {
			continue
		}

		ticker := h.EODHDTicker()
		md := mdByTicker[ticker]
		if md == nil || len(md.EOD) < 2 {
			continue
		}

		fxDiv := 1.0
		if h.OriginalCurrency == "USD" && portfolio.FXRate > 0 {
			fxDiv = portfolio.FXRate
		}

		currentPrice := h.CurrentPrice

		// EOD[0] is the most recent completed trading day (yesterday's close).
		// EOD bars are only collected after market close, so EOD[0] is never "today".
		yesterdayClose := eodClosePrice(md.EOD[0]) / fxDiv
		h.YesterdayClosePrice = yesterdayClose
		if yesterdayClose > 0 {
			h.YesterdayPriceChangePct = ((currentPrice - yesterdayClose) / yesterdayClose) * 100
		}
		yesterdayTotal += yesterdayClose * h.Units

		if bar := findEODBarByOffset(md.EOD, 4); bar != nil {
			lastWeekClose := eodClosePrice(*bar) / fxDiv
			h.LastWeekClosePrice = lastWeekClose
			if lastWeekClose > 0 {
				h.LastWeekPriceChangePct = ((currentPrice - lastWeekClose) / lastWeekClose) * 100
			}
			lastWeekTotal += lastWeekClose * h.Units
		}

		if bar := findEODBarByOffset(md.EOD, 21); bar != nil {
			lastMonthClose := eodClosePrice(*bar) / fxDiv
			h.LastMonthClosePrice = lastMonthClose
			if lastMonthClose > 0 {
				h.LastMonthPriceChangePct = ((currentPrice - lastMonthClose) / lastMonthClose) * 100
			}
		}

		if sigs := signalsByTicker[ticker]; sigs != nil && sigs.TrendMomentum.Level != "" {
			h.TrendLabel = trendMomentumLabel(sigs.TrendMomentum.Level)
			h.TrendScore = sigs.TrendMomentum.Score
		}
	}

	// Only set portfolio aggregates from market data if timeline wasn't available
	if computeAggregates {
		if yesterdayTotal > 0 {
			portfolio.PortfolioYesterdayValue = yesterdayTotal + portfolio.CapitalAvailable
			if portfolio.PortfolioYesterdayValue > 0 {
				portfolio.PortfolioYesterdayChangePct = ((portfolio.PortfolioValue - portfolio.PortfolioYesterdayValue) / portfolio.PortfolioYesterdayValue) * 100
			}
		}
		if lastWeekTotal > 0 {
			portfolio.PortfolioLastWeekValue = lastWeekTotal + portfolio.CapitalAvailable
			if portfolio.PortfolioLastWeekValue > 0 {
				portfolio.PortfolioLastWeekChangePct = ((portfolio.PortfolioValue - portfolio.PortfolioLastWeekValue) / portfolio.PortfolioLastWeekValue) * 100
			}
		}
	}
}

// trendMomentumLabel maps a TrendMomentumLevel to a human-readable label.
func trendMomentumLabel(level models.TrendMomentumLevel) string {
	switch level {
	case models.TrendMomentumStrongUp:
		return "Strong Uptrend"
	case models.TrendMomentumUp:
		return "Uptrend"
	case models.TrendMomentumFlat:
		return "Consolidating"
	case models.TrendMomentumDown:
		return "Downtrend"
	case models.TrendMomentumStrongDown:
		return "Strong Downtrend"
	default:
		return ""
	}
}

// populateNetFlows computes yesterday and last-week net cash flow from the ledger.
// Delegates to CashFlowLedger.NetFlowForPeriod — no inline direction logic.
func (s *Service) populateNetFlows(ctx context.Context, portfolio *models.Portfolio) {
	if s.cashflowSvc == nil {
		return
	}

	ledger, err := s.cashflowSvc.GetLedger(ctx, portfolio.Name)
	if err != nil || ledger == nil || len(ledger.Transactions) == 0 {
		return
	}

	now := time.Now().Truncate(24 * time.Hour)
	yesterday := now.AddDate(0, 0, -1)
	lastWeek := now.AddDate(0, 0, -7)

	// Net flow excludes dividends (investment returns, not capital movements)
	portfolio.NetCashYesterdayFlow = ledger.NetFlowForPeriod(yesterday, now, models.CashCatDividend)
	portfolio.NetCashLastWeekFlow = ledger.NetFlowForPeriod(lastWeek, now, models.CashCatDividend)
}

// computeBreadth aggregates holding trend signals into a portfolio-level breadth summary.
// Only considers open holdings with trend data.
func (s *Service) computeBreadth(portfolio *models.Portfolio) {
	var (
		risingVal, flatVal, fallingVal       float64
		risingCount, flatCount, fallingCount int
		weightedScore, totalWeight           float64
		todayChange                          float64
	)

	for i := range portfolio.Holdings {
		h := &portfolio.Holdings[i]
		if h.Status != "open" || h.MarketValue == 0 {
			continue
		}

		mv := h.MarketValue

		// Classify direction from TrendLabel
		switch h.TrendLabel {
		case "Strong Uptrend", "Uptrend":
			risingVal += mv
			risingCount++
		case "Downtrend", "Strong Downtrend":
			fallingVal += mv
			fallingCount++
		default: // "Consolidating" or no signal
			flatVal += mv
			flatCount++
		}

		// Dollar-weighted trend score
		if h.TrendScore != 0 {
			weightedScore += h.TrendScore * mv
			totalWeight += mv
		}

		// Today's dollar change
		if h.YesterdayClosePrice > 0 {
			todayChange += (h.CurrentPrice - h.YesterdayClosePrice) * h.Units
		}
	}

	total := risingVal + flatVal + fallingVal
	if total == 0 {
		return // No open holdings with market value
	}

	score := 0.0
	if totalWeight > 0 {
		score = weightedScore / totalWeight
	}

	portfolio.Breadth = &models.PortfolioBreadth{
		RisingCount:    risingCount,
		FlatCount:      flatCount,
		FallingCount:   fallingCount,
		RisingWeight:   risingVal / total,
		FlatWeight:     flatVal / total,
		FallingWeight:  fallingVal / total,
		RisingValue:    risingVal,
		FlatValue:      flatVal,
		FallingValue:   fallingVal,
		TrendLabel:     breadthTrendLabel(score),
		TrendScore:     score,
		TodayChange:    todayChange,
		TodayChangePct: todayChange / total * 100,
	}
}

// breadthTrendLabel maps a dollar-weighted trend score to a plain-English label.
func breadthTrendLabel(score float64) string {
	switch {
	case score >= 0.4:
		return "Strong Uptrend"
	case score >= 0.15:
		return "Uptrend"
	case score > -0.15:
		return "Mixed"
	case score > -0.4:
		return "Downtrend"
	default:
		return "Strong Downtrend"
	}
}

// populateChanges computes the Changes section from timeline snapshots and ledger.
// Uses timeline cache (fast) when available; falls back to ledger computation.
func (s *Service) populateChanges(ctx context.Context, portfolio *models.Portfolio) {
	tl := s.storage.TimelineStore()
	userID := common.ResolveUserID(ctx)

	now := time.Now().Truncate(24 * time.Hour)
	dates := struct {
		yesterday time.Time
		week      time.Time
		month     time.Time
	}{
		yesterday: now.AddDate(0, 0, -1),
		week:      now.AddDate(0, 0, -7),
		month:     now.AddDate(0, 0, -30),
	}

	changes := &models.PortfolioChanges{}

	// Populate each period
	changes.Yesterday = s.computePeriodChanges(ctx, userID, portfolio, tl, dates.yesterday)
	changes.Week = s.computePeriodChanges(ctx, userID, portfolio, tl, dates.week)
	changes.Month = s.computePeriodChanges(ctx, userID, portfolio, tl, dates.month)

	portfolio.Changes = changes
}

// computePeriodChanges calculates metric changes for a single reference date.
func (s *Service) computePeriodChanges(ctx context.Context, userID string, portfolio *models.Portfolio, tl interfaces.TimelineStore, refDate time.Time) models.PeriodChanges {
	current := models.PeriodChanges{
		PortfolioValue: models.MetricChange{
			Current:     portfolio.PortfolioValue,
			HasPrevious: false,
		},
		EquityHoldingsValue: models.MetricChange{
			Current:     portfolio.EquityHoldingsValue,
			HasPrevious: false,
		},
		CapitalGross: models.MetricChange{
			Current:     portfolio.CapitalGross,
			HasPrevious: false,
		},
		IncomeDividends: models.MetricChange{
			Current:     portfolio.IncomeDividendsReceived,
			HasPrevious: false,
		},
	}

	// Try timeline store first
	if tl != nil {
		snaps, err := tl.GetRange(ctx, userID, portfolio.Name, refDate, refDate)
		if err == nil && len(snaps) > 0 {
			snap := snaps[0]
			current.PortfolioValue = buildMetricChange(portfolio.PortfolioValue, snap.PortfolioValue)
			current.EquityHoldingsValue = buildMetricChange(portfolio.EquityHoldingsValue, snap.EquityHoldingsValue)
			current.CapitalGross = buildMetricChange(portfolio.CapitalGross, snap.CapitalGross)
			// Dividend: use snapshot if available, else compute from ledger below
			if snap.IncomeDividendsCumulative > 0 || portfolio.IncomeDividendsReceived > 0 {
				current.IncomeDividends = buildMetricChange(portfolio.IncomeDividendsReceived, snap.IncomeDividendsCumulative)
			}
			return current
		}
	}

	// Fallback: compute dividend from ledger for the period
	if s.cashflowSvc != nil {
		ledger, err := s.cashflowSvc.GetLedger(ctx, portfolio.Name)
		if err == nil && ledger != nil {
			// Compute cumulative dividends up to refDate
			divToDate := cumulativeDividendsByDate(ledger, refDate)
			current.IncomeDividends = buildMetricChange(portfolio.IncomeDividendsReceived, divToDate)
		}
	}

	return current
}

// buildMetricChange creates a MetricChange from current and previous values.
func buildMetricChange(current, previous float64) models.MetricChange {
	mc := models.MetricChange{
		Current:     current,
		Previous:    previous,
		HasPrevious: previous > 0,
		RawChange:   current - previous,
	}
	if previous > 0 {
		mc.PctChange = ((current - previous) / previous) * 100
	}
	return mc
}

// buildSignedMetricChange creates a MetricChange for values that can be negative (e.g. P&L).
// Unlike buildMetricChange, hasPrevious is explicit (not derived from previous > 0)
// and PctChange uses math.Abs(previous) as denominator to handle sign correctly.
func buildSignedMetricChange(current, previous float64, hasPrevious bool) models.MetricChange {
	mc := models.MetricChange{
		Current:     current,
		Previous:    previous,
		HasPrevious: hasPrevious,
		RawChange:   current - previous,
	}
	if hasPrevious && previous != 0 {
		mc.PctChange = ((current - previous) / math.Abs(previous)) * 100
	}
	return mc
}

// cumulativeDividendsByDate returns total dividends received up to (and including) refDate.
func cumulativeDividendsByDate(ledger *models.CashFlowLedger, refDate time.Time) float64 {
	var total float64
	for _, tx := range ledger.Transactions {
		if tx.Category != models.CashCatDividend {
			continue
		}
		txDate := tx.Date.Truncate(24 * time.Hour)
		if txDate.Before(refDate) || txDate.Equal(refDate) {
			total += tx.Amount // dividends are positive credits
		}
	}
	return total
}

// RefreshTodaySnapshot reads the cached portfolio from storage and writes today's timeline
// snapshot. Safe for background use — does not require a Navexa client.
func (s *Service) RefreshTodaySnapshot(ctx context.Context, name string) error {
	portfolio, err := s.getPortfolioRecord(ctx, name)
	if err != nil {
		return fmt.Errorf("portfolio '%s' not found in storage: %w", name, err)
	}
	s.writeTodaySnapshot(ctx, portfolio)
	return nil
}

// writeTodaySnapshot persists today's timeline snapshot from the computed portfolio header.
// This is synchronous (not fire-and-forget) because it's the authoritative "today" value
// that gets overwritten on each sync cycle as intraday prices update.
func (s *Service) writeTodaySnapshot(ctx context.Context, portfolio *models.Portfolio) {
	tl := s.storage.TimelineStore()
	if tl == nil {
		return
	}

	userID := common.ResolveUserID(ctx)
	today := time.Now().Truncate(24 * time.Hour)
	now := time.Now()

	// Compute timeline-consistent cost and return from open holdings only.
	// The portfolio header's EquityHoldingsCost uses GrossInvested-GrossProceeds
	// across ALL holdings (including closed), which differs from the timeline's
	// trade-replay cost (sum of open positions' CostBasis). Use CostBasis here
	// to avoid a discontinuity on the last timeline data point.
	var holdingCount int
	var timelineCost float64
	for _, h := range portfolio.Holdings {
		if h.Units > 0 {
			holdingCount++
			timelineCost += h.CostBasis
		}
	}
	timelineReturn := portfolio.EquityHoldingsValue - timelineCost
	timelineReturnPct := 0.0
	if timelineCost > 0 {
		timelineReturnPct = (timelineReturn / timelineCost) * 100
	}

	snap := models.TimelineSnapshot{
		UserID:                    userID,
		PortfolioName:             portfolio.Name,
		Date:                      today,
		EquityHoldingsValue:       portfolio.EquityHoldingsValue,
		EquityHoldingsCost:        timelineCost,
		EquityHoldingsReturn:      timelineReturn,
		EquityHoldingsReturnPct:   timelineReturnPct,
		HoldingCount:              holdingCount,
		CapitalGross:              portfolio.CapitalGross,
		CapitalAvailable:          portfolio.CapitalAvailable,
		AssetSetsValue:            portfolio.AssetSetsValue,
		PortfolioValue:            portfolio.PortfolioValue,
		CapitalContributionsNet:   0, // computed by GetDailyGrowth from trade replay, not available here
		FXRate:                    portfolio.FXRate,
		DataVersion:               common.SchemaVersion,
		ComputedAt:                now,
		IncomeDividendsCumulative: portfolio.IncomeDividendsReceived,
	}

	if err := tl.SaveBatch(ctx, []models.TimelineSnapshot{snap}); err != nil {
		s.logger.Warn().Err(err).Str("portfolio", portfolio.Name).Msg("Failed to write today's timeline snapshot")
	}
}

// backfillTimelineIfEmpty triggers a background GetDailyGrowth computation when the
// timeline store has no historical snapshots (e.g. after a schema rebuild or first sync).
// This ensures the portfolio timeline chart populates automatically without requiring
// a manual API call to /timeline.
func (s *Service) backfillTimelineIfEmpty(ctx context.Context, portfolio *models.Portfolio) {
	tl := s.storage.TimelineStore()
	if tl == nil {
		return
	}

	// Only backfill if the portfolio has historical trades
	earliest := findEarliestTradeDate(portfolio.Holdings)
	if earliest.IsZero() {
		return
	}
	today := time.Now().Truncate(24 * time.Hour)
	if !earliest.Before(today) {
		return // no historical dates to backfill
	}

	// Check if historical snapshots sufficiently cover the expected date range.
	// A handful of snapshots covering a multi-month range is insufficient —
	// require at least 50% of the expected days to consider history populated.
	userID := common.ResolveUserID(ctx)
	yesterday := today.AddDate(0, 0, -1)
	snapshots, err := tl.GetRange(ctx, userID, portfolio.Name, earliest, yesterday)
	if err == nil && len(snapshots) > 0 {
		expectedDays := int(yesterday.Sub(earliest).Hours()/24) + 1
		if len(snapshots) >= expectedDays/2 {
			return // history sufficiently populated
		}
		s.logger.Info().Str("portfolio", portfolio.Name).Int("snapshots", len(snapshots)).Int("expected_days", expectedDays).Msg("Timeline history sparse — triggering backfill")
	}

	// Skip backfill if a rebuild is already in progress
	if s.IsTimelineRebuilding(portfolio.Name) {
		return
	}

	s.logger.Info().Str("portfolio", portfolio.Name).Msg("Timeline history empty — triggering background backfill")
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.logger.Warn().Str("portfolio", portfolio.Name).Msgf("Timeline backfill panic recovered: %v", r)
			}
		}()
		bgCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		// Inject user context for the background goroutine
		bgCtx = common.WithUserContext(bgCtx, common.UserContextFromContext(ctx))
		if _, err := s.rebuildTimelineWithCash(bgCtx, portfolio.Name); err != nil {
			s.logger.Warn().Err(err).Str("portfolio", portfolio.Name).Msg("Timeline backfill failed")
		}
	}()
}

// computeTradeHash creates a deterministic hash of all trade data for change detection.
// The hash captures trade dates, types, amounts, and units — if any trade is added,
// removed, or modified, the hash changes and triggers timeline invalidation.
func computeTradeHash(holdings []models.Holding) string {
	h := sha256.New()
	// Sort holdings by ticker for deterministic ordering
	tickers := make([]string, 0, len(holdings))
	tradesByTicker := make(map[string][]*models.NavexaTrade)
	for i := range holdings {
		ticker := holdings[i].EODHDTicker()
		tickers = append(tickers, ticker)
		tradesByTicker[ticker] = holdings[i].Trades
	}
	sort.Strings(tickers)

	for _, ticker := range tickers {
		trades := tradesByTicker[ticker]
		for _, t := range trades {
			fmt.Fprintf(h, "%s|%s|%s|%.6f|%.6f|%.6f|%.6f\n",
				ticker, t.Date, t.Type, t.Units, t.Price, t.Fees, t.Value)
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:16] // 16-char hex prefix is sufficient
}

// findEODBarByOffset returns the EOD bar approximately N trading days back.
// EOD slice is sorted descending (index 0 = most recent).
// Returns nil if not enough bars available.
func findEODBarByOffset(eod []models.EODBar, offset int) *models.EODBar {
	if offset < 0 || len(eod) <= offset {
		return nil
	}
	return &eod[offset]
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

	// Load holding notes (nil if none exist)
	var noteMap map[string]*models.HoldingNote
	if s.holdingNoteService != nil {
		if hn, err := s.holdingNoteService.GetNotes(ctx, name); err == nil && hn != nil {
			noteMap = hn.NoteMap()
		}
	}
	s.logger.Info().Dur("elapsed", time.Since(phaseStart)).Msg("ReviewPortfolio: portfolio+strategy load complete")

	review := &models.PortfolioReview{
		PortfolioName:           name,
		ReviewDate:              time.Now(),
		PortfolioValue:          portfolio.PortfolioValue,
		EquityHoldingsCost:      portfolio.EquityHoldingsCost,
		EquityHoldingsReturn:    portfolio.EquityHoldingsReturn,
		EquityHoldingsReturnPct: portfolio.EquityHoldingsReturnPct,
		FXRate:                  portfolio.FXRate,
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

		// Attach holding note and derive signal confidence
		if note, ok := noteMap[strings.ToUpper(holding.Ticker)]; ok {
			holdingReview.HoldingNote = note
			holdingReview.SignalConfidence = note.DeriveSignalConfidence()
			holdingReview.NoteStale = note.IsStale()
		} else {
			holdingReview.SignalConfidence = models.SignalConfidenceMedium
		}

		// Add news impact if available and requested
		if options.IncludeNews && len(marketData.News) > 0 {
			holdingReview.NewsImpact = summarizeNewsImpact(marketData.News)
		}

		// Attach news intelligence if available
		if marketData.NewsIntelligence != nil {
			holdingReview.NewsIntelligence = marketData.NewsIntelligence
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

		// Stale note alert
		if holdingReview.NoteStale {
			alerts = append(alerts, models.Alert{
				Type:     models.AlertTypeSignal,
				Severity: "low",
				Ticker:   holding.Ticker,
				Message:  fmt.Sprintf("%s holding note is stale (last reviewed %s)", holding.Ticker, holdingReview.HoldingNote.ReviewedAt.Format("2006-01-02")),
				Signal:   "note_stale",
			})
		}
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
	review.PortfolioDayChange = dayChange

	// Recompute PortfolioValue from live-updated holdings (all values already in AUD)
	liveTotal := 0.0
	for _, hr := range holdingReviews {
		if hr.ActionRequired != "CLOSED" {
			liveTotal += hr.Holding.MarketValue
		}
	}
	if liveTotal > 0 {
		review.PortfolioValue = liveTotal + portfolio.CapitalAvailable
	}

	if review.PortfolioValue > 0 {
		review.PortfolioDayChangePct = (dayChange / review.PortfolioValue) * 100
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

	// Compute portfolio-level indicators
	if indicators, err := s.GetPortfolioIndicators(ctx, name); err == nil {
		review.PortfolioIndicators = indicators
	} else {
		s.logger.Warn().Err(err).Msg("Failed to compute portfolio indicators")
	}

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

// ReviewWatchlist generates a review with signals for watchlist tickers.
// Follows the same pattern as ReviewPortfolio but operates on watchlist items
// rather than portfolio holdings.
func (s *Service) ReviewWatchlist(ctx context.Context, name string, options interfaces.ReviewOptions) (*models.WatchlistReview, error) {
	s.logger.Info().Str("name", name).Msg("Generating watchlist review")

	// 1. Load watchlist from storage
	userID := common.ResolveUserID(ctx)
	rec, err := s.storage.UserDataStore().Get(ctx, userID, "watchlist", name)
	if err != nil {
		return nil, fmt.Errorf("watchlist for '%s' not found: %w", name, err)
	}
	var watchlist models.PortfolioWatchlist
	if err := json.Unmarshal([]byte(rec.Value), &watchlist); err != nil {
		return nil, fmt.Errorf("failed to unmarshal watchlist: %w", err)
	}
	if len(watchlist.Items) == 0 {
		return nil, fmt.Errorf("watchlist '%s' is empty", name)
	}

	// 2. Load strategy (optional, nil if none)
	var strategy *models.PortfolioStrategy
	if strat, err := s.getStrategyRecord(ctx, name); err == nil {
		strategy = strat
	}

	// 2b. Load holding notes (optional, nil if none)
	var noteMap map[string]*models.HoldingNote
	if s.holdingNoteService != nil {
		if hn, err := s.holdingNoteService.GetNotes(ctx, name); err == nil && hn != nil {
			noteMap = hn.NoteMap()
		}
	}

	// 3. Extract tickers from watchlist items
	tickers := make([]string, 0, len(watchlist.Items))
	for _, item := range watchlist.Items {
		tickers = append(tickers, item.Ticker)
	}

	// 4. Batch load market data
	allMarketData, _ := s.storage.MarketDataStorage().GetMarketDataBatch(ctx, tickers)
	mdByTicker := make(map[string]*models.MarketData, len(allMarketData))
	for _, md := range allMarketData {
		mdByTicker[md.Ticker] = md
	}

	// 5. Fetch real-time quotes (non-fatal)
	liveQuotes := make(map[string]*models.RealTimeQuote, len(tickers))
	if s.eodhd != nil {
		for _, ticker := range tickers {
			if quote, err := s.eodhd.GetRealTimeQuote(ctx, ticker); err == nil && quote.Close > 0 {
				liveQuotes[ticker] = quote
			}
		}
	}

	// 6. For each watchlist item, compute review
	itemReviews := make([]models.WatchlistItemReview, 0, len(watchlist.Items))
	alerts := make([]models.Alert, 0)

	for _, item := range watchlist.Items {
		marketData := mdByTicker[item.Ticker]
		if marketData == nil {
			itemReviews = append(itemReviews, models.WatchlistItemReview{
				Item:           item,
				ActionRequired: "WATCH",
				ActionReason:   "Market data unavailable — signals pending data collection",
			})
			continue
		}

		// Get or compute signals
		tickerSignals, err := s.storage.SignalStorage().GetSignals(ctx, item.Ticker)
		if err != nil {
			tickerSignals = s.signalComputer.Compute(marketData)
			if saveErr := s.storage.SignalStorage().SaveSignals(ctx, tickerSignals); saveErr != nil {
				s.logger.Warn().Err(saveErr).Str("ticker", item.Ticker).Msg("Failed to persist computed signals")
			}
		}

		// Overnight movement
		overnightMove := 0.0
		overnightPct := 0.0
		if quote, ok := liveQuotes[item.Ticker]; ok && len(marketData.EOD) > 1 {
			prevClose := marketData.EOD[1].Close
			overnightMove = quote.Close - prevClose
			if prevClose > 0 {
				overnightPct = (overnightMove / prevClose) * 100
			}
		} else if len(marketData.EOD) > 1 {
			overnightMove = marketData.EOD[0].Close - marketData.EOD[1].Close
			if marketData.EOD[1].Close > 0 {
				overnightPct = (overnightMove / marketData.EOD[1].Close) * 100
			}
		}

		// Action determination — pass nil for holding (watchlist items aren't held)
		action, reason := determineAction(tickerSignals, options.FocusSignals, strategy, nil, marketData.Fundamentals)

		review := models.WatchlistItemReview{
			Item:           item,
			Signals:        tickerSignals,
			Fundamentals:   marketData.Fundamentals,
			OvernightMove:  overnightMove,
			OvernightPct:   overnightPct,
			ActionRequired: action,
			ActionReason:   reason,
		}

		// Compliance (strategy-aware) — pass nil holding, zero sector weight
		if strategy != nil {
			review.Compliance = strategypkg.CheckCompliance(strategy, nil, tickerSignals, marketData.Fundamentals, 0)
		}

		// Attach holding note and derive signal confidence
		if note, ok := noteMap[strings.ToUpper(item.Ticker)]; ok {
			review.HoldingNote = note
			review.SignalConfidence = note.DeriveSignalConfidence()
			review.NoteStale = note.IsStale()
		} else {
			review.SignalConfidence = models.SignalConfidenceMedium
		}

		itemReviews = append(itemReviews, review)

		// Generate alerts — construct minimal Holding for ticker identification
		minimalHolding := models.Holding{Ticker: item.Ticker}
		holdingAlerts := generateAlerts(minimalHolding, tickerSignals, options.FocusSignals, strategy)
		alerts = append(alerts, holdingAlerts...)

		// Stale note alert
		if review.NoteStale {
			alerts = append(alerts, models.Alert{
				Type:     models.AlertTypeSignal,
				Severity: "low",
				Ticker:   item.Ticker,
				Message:  fmt.Sprintf("%s holding note is stale (last reviewed %s)", item.Ticker, review.HoldingNote.ReviewedAt.Format("2006-01-02")),
				Signal:   "note_stale",
			})
		}
	}

	return &models.WatchlistReview{
		PortfolioName: name,
		ReviewDate:    time.Now(),
		ItemReviews:   itemReviews,
		Alerts:        alerts,
	}, nil
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
		if holding.WeightPct > strategy.PositionSizing.MaxPositionPct {
			return "WATCH", fmt.Sprintf("Position weight %.1f%% exceeds strategy max %.1f%%",
				holding.WeightPct, strategy.PositionSizing.MaxPositionPct)
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

	// Trend momentum — early warning for deteriorating positions
	if signals.TrendMomentum.Level == models.TrendMomentumStrongDown {
		return "EXIT TRIGGER", fmt.Sprintf("Strong downtrend: %.1f%% over 3d, %.1f%% over 10d",
			signals.TrendMomentum.PriceChange3D, signals.TrendMomentum.PriceChange10D)
	}
	if signals.TrendMomentum.Level == models.TrendMomentumDown {
		return "WATCH", fmt.Sprintf("Deteriorating trend: %.1f%% over 3d, %.1f%% over 10d",
			signals.TrendMomentum.PriceChange3D, signals.TrendMomentum.PriceChange10D)
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

	// Trend momentum alerts
	if signals.TrendMomentum.Level == models.TrendMomentumStrongDown {
		alerts = append(alerts, models.Alert{
			Type:     models.AlertTypeSignal,
			Severity: "high",
			Ticker:   holding.Ticker,
			Message:  fmt.Sprintf("%s strong downtrend: %s", holding.Ticker, signals.TrendMomentum.Description),
			Signal:   "trend_momentum_strong_down",
		})
	} else if signals.TrendMomentum.Level == models.TrendMomentumDown {
		alerts = append(alerts, models.Alert{
			Type:     models.AlertTypeSignal,
			Severity: "medium",
			Ticker:   holding.Ticker,
			Message:  fmt.Sprintf("%s deteriorating: %s", holding.Ticker, signals.TrendMomentum.Description),
			Signal:   "trend_momentum_down",
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
		if strategy.PositionSizing.MaxPositionPct > 0 && holding.WeightPct > strategy.PositionSizing.MaxPositionPct {
			alerts = append(alerts, models.Alert{
				Type:     models.AlertTypeStrategy,
				Severity: "medium",
				Ticker:   holding.Ticker,
				Message: fmt.Sprintf("%s weight %.1f%% exceeds strategy max position size of %.1f%%",
					holding.Ticker, holding.WeightPct, strategy.PositionSizing.MaxPositionPct),
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
		review.PortfolioValue,
		review.PortfolioDayChange,
		review.PortfolioDayChangePct,
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

	// Sort by date ascending — Navexa API may return trades in arbitrary order
	// (e.g. sell before buy) which produces wrong unit counts.
	sorted := make([]*models.NavexaTrade, len(trades))
	copy(sorted, trades)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Date < sorted[j].Date
	})

	for _, t := range sorted {
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
				if math.Abs(units) < 1e-9 {
					units = 0
				}
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

		weight := hr.Holding.WeightPct
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
	return holding.WeightPct
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
	totalProceeds      float64 // sum of sell proceeds
	realizedGainLoss   float64
	unrealizedGainLoss float64
}

// Ensure Service implements PortfolioService
var _ interfaces.PortfolioService = (*Service)(nil)
