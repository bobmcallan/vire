// Package portfolio provides portfolio management services
package portfolio

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bobmccarthy/vire/internal/common"
	"github.com/bobmccarthy/vire/internal/interfaces"
	"github.com/bobmccarthy/vire/internal/models"
	"github.com/bobmccarthy/vire/internal/signals"
)

// Service implements PortfolioService
type Service struct {
	storage        interfaces.StorageManager
	navexa         interfaces.NavexaClient
	eodhd          interfaces.EODHDClient
	gemini         interfaces.GeminiClient
	signalComputer *signals.Computer
	logger         *common.Logger
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

// SyncPortfolio refreshes portfolio data from Navexa
func (s *Service) SyncPortfolio(ctx context.Context, name string, force bool) (*models.Portfolio, error) {
	s.logger.Info().Str("name", name).Bool("force", force).Msg("Syncing portfolio")

	// Check if we need to sync
	if !force {
		existing, err := s.storage.PortfolioStorage().GetPortfolio(ctx, name)
		if err == nil && time.Since(existing.LastSynced) < time.Hour {
			s.logger.Debug().Str("name", name).Msg("Portfolio recently synced, skipping")
			return existing, nil
		}
	}

	// Get portfolios from Navexa
	navexaPortfolios, err := s.navexa.GetPortfolios(ctx)
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

	navexaHoldings, err := s.navexa.GetEnrichedHoldings(ctx, navexaPortfolio.ID, fromDate, toDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get enriched holdings from Navexa: %w", err)
	}

	// Fetch trades per holding to compute accurate cost basis
	// (performance endpoint returns annualized values, not actual cost)
	holdingTrades := make(map[string][]*models.NavexaTrade) // ticker -> trades
	for _, h := range navexaHoldings {
		if h.ID == "" {
			continue
		}
		trades, err := s.navexa.GetHoldingTrades(ctx, h.ID)
		if err != nil {
			s.logger.Warn().Err(err).Str("ticker", h.Ticker).Str("holdingID", h.ID).Msg("Failed to get trades for holding")
			continue
		}
		if len(trades) > 0 {
			holdingTrades[h.Ticker] = trades
			avgCost, totalCost := calculateAvgCostFromTrades(trades)
			h.AvgCost = avgCost
			h.TotalCost = totalCost
			h.GainLoss = h.MarketValue - totalCost
			if totalCost > 0 {
				h.GainLossPct = (h.GainLoss / totalCost) * 100
				h.CapitalGainPct = h.GainLossPct
			}
			h.TotalReturnValue = h.GainLoss + h.DividendReturn
			if totalCost > 0 {
				h.TotalReturnPct = (h.TotalReturnValue / totalCost) * 100
			}

			// For closed positions (fully sold), override with realized gain/loss
			if h.Units <= 0 {
				avgBuy, invested, _, realized := calculateRealizedFromTrades(trades)
				h.AvgCost = avgBuy
				h.TotalCost = invested
				h.GainLoss = realized
				if invested > 0 {
					h.GainLossPct = (realized / invested) * 100
					h.CapitalGainPct = h.GainLossPct
				}
				h.TotalReturnValue = realized + h.DividendReturn
				if invested > 0 {
					h.TotalReturnPct = (h.TotalReturnValue / invested) * 100
				}
			}
		}
	}

	// Convert to internal model
	holdings := make([]models.Holding, len(navexaHoldings))
	totalValue := 0.0

	for i, h := range navexaHoldings {
		holdings[i] = models.Holding{
			Ticker:           h.Ticker,
			Exchange:         h.Exchange,
			Name:             h.Name,
			Units:            h.Units,
			AvgCost:          h.AvgCost,
			CurrentPrice:     h.CurrentPrice,
			MarketValue:      h.MarketValue,
			GainLoss:         h.GainLoss,
			GainLossPct:      h.GainLossPct,
			TotalCost:        h.TotalCost,
			DividendReturn:   h.DividendReturn,
			CapitalGainPct:   h.CapitalGainPct,
			TotalReturnValue: h.TotalReturnValue,
			TotalReturnPct:   h.TotalReturnPct,
			Trades:           holdingTrades[h.Ticker],
			LastUpdated:      h.LastUpdated,
		}
		totalValue += h.MarketValue
	}

	// Calculate weights
	for i := range holdings {
		if totalValue > 0 {
			holdings[i].Weight = (holdings[i].MarketValue / totalValue) * 100
		}
	}

	// Compute portfolio-level totals from holdings (not from performance endpoint which is annualized)
	var totalCost, totalGain, totalGainPct float64
	for _, h := range holdings {
		totalCost += h.TotalCost
		totalGain += h.GainLoss + h.DividendReturn
	}
	if totalCost > 0 {
		totalGainPct = (totalGain / totalCost) * 100
	}

	portfolio := &models.Portfolio{
		ID:           name,
		Name:         name,
		NavexaID:     navexaPortfolio.ID,
		Holdings:     holdings,
		TotalValue:   totalValue,
		TotalCost:    totalCost,
		TotalGain:    totalGain,
		TotalGainPct: totalGainPct,
		Currency:     navexaPortfolio.Currency,
		LastSynced:   time.Now(),
	}

	// Save portfolio
	if err := s.storage.PortfolioStorage().SavePortfolio(ctx, portfolio); err != nil {
		return nil, fmt.Errorf("failed to save portfolio: %w", err)
	}

	s.logger.Info().Str("name", name).Int("holdings", len(holdings)).Msg("Portfolio synced")

	return portfolio, nil
}

// GetPortfolio retrieves a portfolio with current data
func (s *Service) GetPortfolio(ctx context.Context, name string) (*models.Portfolio, error) {
	return s.storage.PortfolioStorage().GetPortfolio(ctx, name)
}

// ListPortfolios returns available portfolio names
func (s *Service) ListPortfolios(ctx context.Context) ([]string, error) {
	return s.storage.PortfolioStorage().ListPortfolios(ctx)
}

// ReviewPortfolio generates a portfolio review with signals
func (s *Service) ReviewPortfolio(ctx context.Context, name string, options interfaces.ReviewOptions) (*models.PortfolioReview, error) {
	s.logger.Info().Str("name", name).Msg("Generating portfolio review")

	// Get portfolio
	portfolio, err := s.GetPortfolio(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get portfolio: %w", err)
	}

	review := &models.PortfolioReview{
		PortfolioName: name,
		ReviewDate:    time.Now(),
		TotalValue:    portfolio.TotalValue,
		TotalCost:     portfolio.TotalCost,
		TotalGain:     portfolio.TotalGain,
		TotalGainPct:  portfolio.TotalGainPct,
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

	for _, holding := range activeHoldings {
		ticker := holding.Ticker + ".AU"

		// Get market data
		marketData, err := s.storage.MarketDataStorage().GetMarketData(ctx, ticker)
		if err != nil {
			s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to get market data")
			continue
		}

		// Get or compute signals
		tickerSignals, err := s.storage.SignalStorage().GetSignals(ctx, ticker)
		if err != nil {
			tickerSignals = s.signalComputer.Compute(marketData)
		}

		// Calculate overnight movement
		overnightMove := 0.0
		overnightPct := 0.0
		if len(marketData.EOD) > 1 {
			overnightMove = marketData.EOD[0].Close - marketData.EOD[1].Close
			overnightPct = (overnightMove / marketData.EOD[1].Close) * 100
		}

		// Determine action
		action, reason := determineAction(tickerSignals, options.FocusSignals)

		holdingReview := models.HoldingReview{
			Holding:        holding,
			Signals:        tickerSignals,
			Fundamentals:   marketData.Fundamentals,
			OvernightMove:  overnightMove,
			OvernightPct:   overnightPct,
			ActionRequired: action,
			ActionReason:   reason,
		}

		// Add news impact if available and requested
		if options.IncludeNews && len(marketData.News) > 0 {
			holdingReview.NewsImpact = summarizeNewsImpact(marketData.News)
		}

		// Attach news intelligence if available
		if marketData.NewsIntelligence != nil {
			holdingReview.NewsIntelligence = marketData.NewsIntelligence
		}

		// Attach filings intelligence if available
		if marketData.FilingsIntelligence != nil {
			holdingReview.FilingsIntelligence = marketData.FilingsIntelligence
		}

		holdingReviews = append(holdingReviews, holdingReview)

		// Track day change
		dayChange += overnightMove * holding.Units

		// Generate alerts
		holdingAlerts := generateAlerts(holding, tickerSignals, options.FocusSignals)
		alerts = append(alerts, holdingAlerts...)
	}

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
	if portfolio.TotalValue > 0 {
		review.DayChangePct = (dayChange / portfolio.TotalValue) * 100
	}

	// Generate AI summary if available
	if s.gemini != nil {
		summary, err := s.generateReviewSummary(ctx, review)
		if err != nil {
			s.logger.Warn().Err(err).Msg("Failed to generate AI summary")
		} else {
			review.Summary = summary
		}
	}

	// Generate recommendations
	review.Recommendations = generateRecommendations(review)

	// Generate portfolio balance analysis
	review.PortfolioBalance = analyzePortfolioBalance(review.HoldingReviews)

	s.logger.Info().
		Str("name", name).
		Int("holdings", len(holdingReviews)).
		Int("alerts", len(alerts)).
		Msg("Portfolio review complete")

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

// determineAction determines the recommended action for a holding
func determineAction(signals *models.TickerSignals, focusSignals []string) (string, string) {
	if signals == nil {
		return "HOLD", "Insufficient data"
	}

	// Check for sell signals
	if signals.Technical.RSI > 70 {
		return "SELL", "RSI overbought (>70)"
	}
	if signals.Technical.SMA20CrossSMA50 == "death_cross" {
		return "SELL", "Recent death cross (SMA20 below SMA50)"
	}
	if signals.Trend == models.TrendBearish && signals.Price.DistanceToSMA200 < -20 {
		return "SELL", "Extended below 200-day SMA in downtrend"
	}

	// Check for buy signals
	if signals.Technical.RSI < 30 {
		return "BUY", "RSI oversold (<30)"
	}
	if signals.Technical.SMA20CrossSMA50 == "golden_cross" {
		return "BUY", "Recent golden cross (SMA20 above SMA50)"
	}

	// Check for watch signals
	if signals.Technical.NearSupport {
		return "WATCH", "Testing support level"
	}
	if signals.Technical.NearResistance {
		return "WATCH", "Testing resistance level"
	}

	return "HOLD", "No significant signals"
}

// generateAlerts creates alerts for a holding
func generateAlerts(holding models.Holding, signals *models.TickerSignals, focusSignals []string) []models.Alert {
	alerts := make([]models.Alert, 0)

	if signals == nil {
		return alerts
	}

	// RSI alerts
	if signals.Technical.RSI > 70 {
		alerts = append(alerts, models.Alert{
			Type:     models.AlertTypeSignal,
			Severity: "high",
			Ticker:   holding.Ticker,
			Message:  fmt.Sprintf("%s RSI is overbought at %.1f", holding.Ticker, signals.Technical.RSI),
			Signal:   "rsi_overbought",
		})
	} else if signals.Technical.RSI < 30 {
		alerts = append(alerts, models.Alert{
			Type:     models.AlertTypeSignal,
			Severity: "medium",
			Ticker:   holding.Ticker,
			Message:  fmt.Sprintf("%s RSI is oversold at %.1f", holding.Ticker, signals.Technical.RSI),
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

// generateRecommendations creates actionable recommendations
func generateRecommendations(review *models.PortfolioReview) []string {
	recommendations := make([]string, 0)

	// Count actions
	sellCount := 0
	buyCount := 0
	watchCount := 0

	for _, hr := range review.HoldingReviews {
		switch hr.ActionRequired {
		case "SELL":
			sellCount++
		case "BUY":
			buyCount++
		case "WATCH":
			watchCount++
		}
	}

	if sellCount > 0 {
		recommendations = append(recommendations,
			fmt.Sprintf("Review %d holdings showing sell signals for potential profit-taking or loss mitigation", sellCount))
	}

	if buyCount > 0 {
		recommendations = append(recommendations,
			fmt.Sprintf("Consider adding to %d holdings showing buy signals", buyCount))
	}

	if watchCount > 0 {
		recommendations = append(recommendations,
			fmt.Sprintf("Monitor %d holdings at key support/resistance levels", watchCount))
	}

	// Portfolio-level recommendations
	highAlerts := 0
	for _, alert := range review.Alerts {
		if alert.Severity == "high" {
			highAlerts++
		}
	}

	if highAlerts > 0 {
		recommendations = append(recommendations,
			fmt.Sprintf("Address %d high-priority alerts requiring immediate attention", highAlerts))
	}

	return recommendations
}

// generateReviewSummary creates an AI summary of the portfolio review
func (s *Service) generateReviewSummary(ctx context.Context, review *models.PortfolioReview) (string, error) {
	prompt := buildReviewSummaryPrompt(review)
	return s.gemini.GenerateContent(ctx, prompt)
}

func buildReviewSummaryPrompt(review *models.PortfolioReview) string {
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

	// Add holding summaries
	prompt += "Holdings requiring action:\n"
	for _, hr := range review.HoldingReviews {
		if hr.ActionRequired != "HOLD" {
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

// calculateAvgCostFromTrades computes the weighted-average cost from trade history.
// Handles Buy, Sell, Cost Base Increase/Decrease, and Opening Balance trade types.
func calculateAvgCostFromTrades(trades []*models.NavexaTrade) (avgCost, totalCost float64) {
	totalUnits := 0.0
	totalCost = 0.0

	for _, t := range trades {
		switch strings.ToLower(t.Type) {
		case "buy", "opening balance":
			cost := t.Units*t.Price + t.Fees
			totalCost += cost
			totalUnits += t.Units
		case "sell":
			if totalUnits > 0 {
				// Reduce cost proportionally
				costPerUnit := totalCost / totalUnits
				totalCost -= t.Units * costPerUnit
				totalUnits -= t.Units
			}
		case "cost base increase":
			totalCost += t.Value
		case "cost base decrease":
			totalCost -= t.Value
		}
	}

	if totalUnits > 0 {
		avgCost = totalCost / totalUnits
	}

	return avgCost, totalCost
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

// Ensure Service implements PortfolioService
var _ interfaces.PortfolioService = (*Service)(nil)
