package portfolio

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

// holdingGrowthState tracks incremental trade replay state for a holding.
// Trades are sorted by date ascending; cursor advances as dates progress.
type holdingGrowthState struct {
	Ticker       string
	SortedTrades []*models.NavexaTrade // sorted by date ascending
	Cursor       int                   // next trade index to process
	Units        float64
	TotalCost    float64
	FXDiv        float64 // USD→AUD divisor (AUDUSD rate for USD holdings, 1.0 for AUD)
	LastPrice    float64 // last known closing price, carried forward across dates
	HasPrice     bool    // true once at least one closing price has been found
}

// advanceTo processes all trades with date <= cutoff, updating units and cost.
// Returns the net cash impact of trades processed (negative for buys, positive for sells).
func (s *holdingGrowthState) advanceTo(cutoff time.Time) (cashDelta float64) {
	cutoffStr := cutoff.Format("2006-01-02")

	for s.Cursor < len(s.SortedTrades) {
		t := s.SortedTrades[s.Cursor]
		tradeDate := normalizeDateStr(strings.TrimSpace(t.Date))
		if tradeDate == "" || tradeDate > cutoffStr {
			break // trade is in the future, stop
		}

		// Process this trade — all monetary values are converted from trade
		// currency to portfolio base currency (AUD) using FXDiv.
		fx := s.FXDiv
		switch strings.ToLower(t.Type) {
		case "buy", "opening balance":
			cost := (t.Units*t.Price + t.Fees) / fx
			s.TotalCost += cost
			s.Units += t.Units
			cashDelta -= cost
		case "sell":
			if s.Units > 0 {
				costPerUnit := s.TotalCost / s.Units
				s.TotalCost -= t.Units * costPerUnit
				s.Units -= t.Units
				proceeds := (t.Units*t.Price - t.Fees) / fx
				if proceeds > 0 {
					cashDelta += proceeds
				}
			}
		case "cost base increase":
			s.TotalCost += t.Value / fx
		case "cost base decrease":
			s.TotalCost -= t.Value / fx
		}
		s.Cursor++
	}
	return cashDelta
}

// newHoldingGrowthState creates a state for a holding with trades sorted by date.
// fxDiv is the USD→AUD divisor (AUDUSD rate for USD holdings, 1.0 for AUD).
func newHoldingGrowthState(ticker string, trades []*models.NavexaTrade, fxDiv float64) *holdingGrowthState {
	// Copy and sort trades by date ascending
	sorted := make([]*models.NavexaTrade, len(trades))
	copy(sorted, trades)
	sort.Slice(sorted, func(i, j int) bool {
		di := normalizeDateStr(strings.TrimSpace(sorted[i].Date))
		dj := normalizeDateStr(strings.TrimSpace(sorted[j].Date))
		return di < dj
	})
	return &holdingGrowthState{
		Ticker:       ticker,
		SortedTrades: sorted,
		Cursor:       0,
		Units:        0,
		TotalCost:    0,
		FXDiv:        fxDiv,
	}
}

// GetDailyGrowth returns daily portfolio value data points for a date range.
// It bulk-loads all data once then iterates dates in memory — O(holdings) reads
// instead of O(days × holdings).
func (s *Service) GetDailyGrowth(ctx context.Context, name string, opts interfaces.GrowthOptions) ([]models.GrowthDataPoint, error) {
	funcStart := time.Now()
	s.logger.Info().Str("name", name).Msg("Computing daily portfolio growth")

	// Phase 1: Load portfolio once
	phaseStart := time.Now()
	p, err := s.getPortfolioRecord(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("portfolio '%s' not found — sync it first: %w", name, err)
	}

	earliest := findEarliestTradeDate(p.Holdings)
	if earliest.IsZero() {
		return nil, fmt.Errorf("no trades found in portfolio '%s'", name)
	}
	s.logger.Info().Dur("elapsed", time.Since(phaseStart)).Msg("GetDailyGrowth: portfolio load complete")

	// Auto-load cash transactions if not provided by caller.
	// This ensures all code paths (handler, scheduler, internal) include cash
	// in timeline computations without requiring explicit injection.
	if opts.Transactions == nil && s.cashflowSvc != nil {
		if ledger, err := s.cashflowSvc.GetLedger(ctx, name); err == nil && ledger != nil {
			opts.Transactions = ledger.Transactions
		}
	}

	// Phase 2: Determine date range
	from := opts.From
	if from.IsZero() {
		from = earliest
	}
	to := opts.To
	if to.IsZero() {
		to = time.Now().Truncate(24 * time.Hour)
	}
	if to.After(time.Now()) {
		to = time.Now().Truncate(24 * time.Hour)
	}

	dates := generateCalendarDates(from, to)
	s.logger.Info().
		Str("name", name).
		Str("from", from.Format(time.RFC3339)).
		Str("to", to.Format(time.RFC3339)).
		Int("days", len(dates)).
		Msg("Daily growth date range")

	if len(dates) == 0 {
		return nil, nil
	}

	// Timeline cache: if persisted snapshots cover the full requested range,
	// return them directly — no market data load or trade replay needed.
	userID := common.ResolveUserID(ctx)
	if cached, ok := s.tryTimelineCache(ctx, userID, name, from, to); ok {
		s.logger.Info().Str("name", name).Int("points", len(cached)).Dur("elapsed", time.Since(funcStart)).Msg("GetDailyGrowth: served from timeline cache")
		return cached, nil
	}

	// Phase 3: Bulk-load all market data
	phaseStart = time.Now()
	tickers := make([]string, 0, len(p.Holdings))
	for _, h := range p.Holdings {
		if len(h.Trades) > 0 {
			tickers = append(tickers, h.EODHDTicker())
		}
	}

	allMarketData, err := s.storage.MarketDataStorage().GetMarketDataBatch(ctx, tickers)
	if err != nil {
		return nil, fmt.Errorf("failed to bulk-load market data: %w", err)
	}

	// Index market data by ticker
	mdByTicker := make(map[string]*models.MarketData, len(allMarketData))
	for _, md := range allMarketData {
		mdByTicker[md.Ticker] = md
	}
	s.logger.Info().Dur("elapsed", time.Since(phaseStart)).Int("tickers", len(tickers)).Msg("GetDailyGrowth: market data batch load complete")

	// Phase 4: Determine FX rate for USD→AUD conversion.
	// Holdings may be in different currencies (e.g. CBOE in USD, BHP in AUD).
	// Trade prices and EOD close prices are in the holding's native currency
	// and must be converted to the portfolio base currency (AUD).
	phaseStart = time.Now()
	fxDivByTicker := make(map[string]float64, len(p.Holdings))
	hasUSD := false
	for _, h := range p.Holdings {
		if len(h.Trades) == 0 {
			continue
		}
		ticker := h.EODHDTicker()
		currency := h.OriginalCurrency
		if currency == "" {
			currency = h.Currency
		}
		if currency == "USD" {
			hasUSD = true
			fxDivByTicker[ticker] = 0 // placeholder, set below
		} else {
			fxDivByTicker[ticker] = 1.0
		}
	}

	if hasUSD {
		// Use persisted FX rate from last sync; fall back to live quote.
		fxDiv := p.FXRate
		if fxDiv <= 0 && s.eodhd != nil {
			if quote, err := s.eodhd.GetRealTimeQuote(ctx, "AUDUSD.FOREX"); err == nil && quote.Close > 0 {
				fxDiv = quote.Close
			} else {
				s.logger.Warn().Err(err).Msg("GetDailyGrowth: failed to fetch AUDUSD rate; USD values will be unconverted")
			}
		}
		if fxDiv <= 0 {
			fxDiv = 1.0 // fallback: no conversion
		}
		for ticker, v := range fxDivByTicker {
			if v == 0 {
				fxDivByTicker[ticker] = fxDiv
			}
		}
		s.logger.Info().Float64("fx_div", fxDiv).Msg("GetDailyGrowth: FX divisor for USD holdings")
	}

	// Initialize incremental trade replay states.
	// All holdings with trades are included — even those without market data —
	// so that trade settlements (buys/sells) correctly affect runningNetCash.
	// Holdings without any price data will be skipped for equity valuation
	// but their cost-basis and cash impact are still tracked.
	holdingStates := make([]*holdingGrowthState, 0, len(p.Holdings))
	var noMarketDataCount int
	for _, h := range p.Holdings {
		if len(h.Trades) == 0 {
			continue
		}
		ticker := h.EODHDTicker()
		if md := mdByTicker[ticker]; md == nil || len(md.EOD) == 0 {
			noMarketDataCount++
		}
		holdingStates = append(holdingStates, newHoldingGrowthState(ticker, h.Trades, fxDivByTicker[ticker]))
	}
	if noMarketDataCount > 0 {
		s.logger.Warn().Int("count", noMarketDataCount).Msg("GetDailyGrowth: holdings without market data (trades tracked, equity unpriced)")
	}

	// Phase 5: Prepare cash flow cursor for single-pass merge
	// Transactions must be date-sorted ascending for cursor-based iteration.
	txs := opts.Transactions
	sort.Slice(txs, func(i, j int) bool { return txs[i].Date.Before(txs[j].Date) })
	txCursor := 0
	hasCashTxs := len(txs) > 0
	// Track gross (cash transactions only) and net (cash + trade settlements) separately.
	// GrossCash matches ledger.TotalCashBalance(); NetCash = uninvested cash after equity purchases.
	var runningGrossCash, runningNetCash, runningNetDeployed float64

	// Phase 6: Iterate dates and compute portfolio value using incremental replay
	points := make([]models.GrowthDataPoint, 0, len(dates))
	for _, date := range dates {
		var totalValue, totalCost float64
		holdingCount := 0

		for _, hs := range holdingStates {
			// Advance state to include all trades up to this date;
			// cashDelta reflects money spent on buys (negative) and received from sells (positive).
			tradeCashDelta := hs.advanceTo(date)
			runningNetCash += tradeCashDelta // trade settlements affect net cash only

			if hs.Units <= 0 {
				continue
			}

			md := mdByTicker[hs.Ticker]
			var closePrice float64
			var found bool
			if md != nil && len(md.EOD) > 0 {
				closePrice, _, found = findClosingPriceAsOf(md.EOD, date)
			}
			if found {
				hs.LastPrice = closePrice
				hs.HasPrice = true
			} else if hs.HasPrice {
				// Carry forward last known price — EOD data may have gaps
				// (incomplete collection, weekends, holidays) but the holding
				// still exists and should be valued.
				closePrice = hs.LastPrice
			} else {
				continue // no price ever seen for this holding yet
			}

			// Convert EOD close price to portfolio base currency (AUD)
			totalValue += hs.Units * closePrice / hs.FXDiv
			totalCost += hs.TotalCost
			holdingCount++
		}

		// Outlier detection: cap day-over-day swings exceeding 50%.
		// Corrupted EOD data (e.g. bad EODHD price) can produce implausible
		// portfolio values that distort charts and derived indicators.
		if len(points) > 0 && totalValue > 0 {
			prevValue := points[len(points)-1].EquityHoldingsValue
			if prevValue > 0 {
				ratio := totalValue / prevValue
				if ratio > 1.5 || ratio < 0.5 {
					totalValue = prevValue
				}
			}
		}

		// Advance cash flow cursor: process all transactions up to this date.
		// SignedAmount() and NetDeployedImpact() are the authoritative source
		// for how each transaction affects balances — no inline direction logic.
		endOfDay := date.AddDate(0, 0, 1) // exclusive upper bound
		for txCursor < len(txs) && txs[txCursor].Date.Before(endOfDay) {
			tx := txs[txCursor]
			txCursor++
			amount := tx.SignedAmount()
			runningGrossCash += amount // cash transactions affect both gross and net
			runningNetCash += amount
			runningNetDeployed += tx.NetDeployedImpact()
		}

		if totalValue == 0 && totalCost == 0 {
			continue
		}

		gainLoss := totalValue - totalCost
		gainLossPct := 0.0
		if totalCost > 0 {
			gainLossPct = (gainLoss / totalCost) * 100
		}

		// Without cash transactions, cash position is unknown.
		// PortfolioValue = EquityValue (consistent with service.go portfolio sync).
		netCash := runningNetCash
		portfolioVal := totalValue + runningNetCash
		if !hasCashTxs {
			netCash = 0
			portfolioVal = totalValue
		}

		points = append(points, models.GrowthDataPoint{
			Date:                    date,
			EquityHoldingsValue:     totalValue,
			EquityHoldingsCost:      totalCost,
			EquityHoldingsReturn:    gainLoss,
			EquityHoldingsReturnPct: gainLossPct,
			HoldingCount:            holdingCount,
			CapitalGross:            runningGrossCash,
			CapitalAvailable:        netCash,
			PortfolioValue:          portfolioVal,
			CapitalContributionsNet: runningNetDeployed,
		})
	}
	s.logger.Info().Dur("elapsed", time.Since(phaseStart)).Int("days", len(dates)).Int("holdings", len(holdingStates)).Msg("GetDailyGrowth: date iteration complete")

	s.logger.Info().Str("name", name).Int("points", len(points)).Dur("elapsed", time.Since(funcStart)).Msg("GetDailyGrowth: TOTAL")

	// Write-behind: persist timeline snapshots for historical dates (fire-and-forget).
	// Today's snapshot is written synchronously by SyncPortfolio with live header values.
	if len(points) > 0 {
		today := time.Now().Truncate(24 * time.Hour)
		go s.persistTimelineSnapshots(userID, name, points, today, p.FXRate)
	}

	return points, nil
}

// GetPortfolioGrowth returns monthly growth data points from inception to now.
// Delegates to GetDailyGrowth and downsamples to monthly.
func (s *Service) GetPortfolioGrowth(ctx context.Context, name string) ([]models.GrowthDataPoint, error) {
	daily, err := s.GetDailyGrowth(ctx, name, interfaces.GrowthOptions{})
	if err != nil {
		return nil, err
	}
	return DownsampleToMonthly(daily), nil
}

// DownsampleToWeekly keeps the last data point per ISO week.
func DownsampleToWeekly(points []models.GrowthDataPoint) []models.GrowthDataPoint {
	if len(points) == 0 {
		return nil
	}

	weekly := make([]models.GrowthDataPoint, 0)
	for i, p := range points {
		if i == len(points)-1 {
			weekly = append(weekly, p)
			continue
		}
		y1, w1 := p.Date.ISOWeek()
		y2, w2 := points[i+1].Date.ISOWeek()
		if w1 != w2 || y1 != y2 {
			weekly = append(weekly, p)
		}
	}

	return weekly
}

// DownsampleToMonthly keeps the last data point per calendar month.
func DownsampleToMonthly(points []models.GrowthDataPoint) []models.GrowthDataPoint {
	if len(points) == 0 {
		return nil
	}

	monthly := make([]models.GrowthDataPoint, 0)
	for i, p := range points {
		// Keep this point if it's the last one, or if the next point is in a different month
		if i == len(points)-1 || points[i+1].Date.Month() != p.Date.Month() || points[i+1].Date.Year() != p.Date.Year() {
			monthly = append(monthly, p)
		}
	}

	return monthly
}

// DownsampleStockTimelineWeekly keeps the last data point per ISO week.
func DownsampleStockTimelineWeekly(points []models.StockTimelinePoint) []models.StockTimelinePoint {
	if len(points) == 0 {
		return nil
	}
	out := make([]models.StockTimelinePoint, 0)
	for i, p := range points {
		if i == len(points)-1 {
			out = append(out, p)
			continue
		}
		y1, w1 := p.Date.ISOWeek()
		y2, w2 := points[i+1].Date.ISOWeek()
		if w1 != w2 || y1 != y2 {
			out = append(out, p)
		}
	}
	return out
}

// DownsampleStockTimelineMonthly keeps the last data point per calendar month.
func DownsampleStockTimelineMonthly(points []models.StockTimelinePoint) []models.StockTimelinePoint {
	if len(points) == 0 {
		return nil
	}
	out := make([]models.StockTimelinePoint, 0)
	for i, p := range points {
		if i == len(points)-1 || points[i+1].Date.Month() != p.Date.Month() || points[i+1].Date.Year() != p.Date.Year() {
			out = append(out, p)
		}
	}
	return out
}

// GetStockTimeline returns daily value data points for a single holding within a portfolio.
// Reuses trade replay (holdingGrowthState) and EOD price lookup from GetDailyGrowth.
func (s *Service) GetStockTimeline(ctx context.Context, portfolioName, ticker string, from, to time.Time) ([]models.StockTimelinePoint, error) {
	p, err := s.getPortfolioRecord(ctx, portfolioName)
	if err != nil {
		return nil, fmt.Errorf("portfolio '%s' not found: %w", portfolioName, err)
	}

	// Find matching holding
	var holding *models.Holding
	tickerUpper := strings.ToUpper(strings.TrimSpace(ticker))
	for i := range p.Holdings {
		h := &p.Holdings[i]
		ht := strings.ToUpper(h.Ticker)
		eodhd := strings.ToUpper(h.EODHDTicker())
		if tickerUpper == ht || tickerUpper == eodhd {
			holding = h
			break
		}
		if base, _, ok := strings.Cut(tickerUpper, "."); ok && base == ht {
			holding = h
			break
		}
		if base, _, ok := strings.Cut(eodhd, "."); ok && base == tickerUpper {
			holding = h
			break
		}
	}
	if holding == nil {
		return nil, fmt.Errorf("holding '%s' not found in portfolio '%s'", ticker, portfolioName)
	}
	if len(holding.Trades) == 0 {
		return nil, fmt.Errorf("holding '%s' has no trades", ticker)
	}

	// Load EOD bars
	eohdTicker := holding.EODHDTicker()
	md, _ := s.storage.MarketDataStorage().GetMarketData(ctx, eohdTicker)
	var bars []models.EODBar
	if md != nil {
		bars = md.EOD
	}

	// FX divisor (same logic as GetDailyGrowth Phase 4)
	fxDiv := 1.0
	currency := holding.OriginalCurrency
	if currency == "" {
		currency = holding.Currency
	}
	if currency == "USD" {
		fxDiv = p.FXRate
		if fxDiv <= 0 && s.eodhd != nil {
			if quote, qErr := s.eodhd.GetRealTimeQuote(ctx, "AUDUSD.FOREX"); qErr == nil && quote.Close > 0 {
				fxDiv = quote.Close
			}
		}
		if fxDiv <= 0 {
			fxDiv = 1.0
		}
	}

	// Init trade replay state
	state := newHoldingGrowthState(eohdTicker, holding.Trades, fxDiv)

	// Determine date range
	earliest := findEarliestTradeDateForHolding(holding)
	if earliest.IsZero() {
		return nil, fmt.Errorf("no parseable trade dates for '%s'", ticker)
	}
	if from.IsZero() {
		from = earliest
	}
	if to.IsZero() {
		to = time.Now().Truncate(24 * time.Hour)
	}
	if to.After(time.Now()) {
		to = time.Now().Truncate(24 * time.Hour)
	}

	dates := generateCalendarDates(from, to)
	if len(dates) == 0 {
		return nil, nil
	}

	points := make([]models.StockTimelinePoint, 0, len(dates))
	for _, date := range dates {
		state.advanceTo(date)

		if state.Units <= 0 && state.TotalCost <= 0 {
			continue
		}

		var closePrice float64
		var found bool
		if len(bars) > 0 {
			closePrice, _, found = findClosingPriceAsOf(bars, date)
		}
		if found {
			state.LastPrice = closePrice
			state.HasPrice = true
		} else if state.HasPrice {
			closePrice = state.LastPrice
		} else {
			continue // no price ever seen yet
		}

		closePriceAUD := closePrice / fxDiv
		marketValue := state.Units * closePriceAUD
		costBasis := state.TotalCost
		netReturn := marketValue - costBasis
		netReturnPct := 0.0
		if costBasis > 0 {
			netReturnPct = (netReturn / costBasis) * 100
		}

		points = append(points, models.StockTimelinePoint{
			Date:         date,
			Units:        state.Units,
			CostBasis:    costBasis,
			ClosePrice:   closePriceAUD,
			MarketValue:  marketValue,
			NetReturn:    netReturn,
			NetReturnPct: netReturnPct,
		})
	}

	return points, nil
}

// findEarliestTradeDateForHolding scans a single holding for the oldest trade date.
func findEarliestTradeDateForHolding(h *models.Holding) time.Time {
	var earliest time.Time
	for _, t := range h.Trades {
		parsed := parseTradeDate(t.Date)
		if parsed.IsZero() {
			continue
		}
		if earliest.IsZero() || parsed.Before(earliest) {
			earliest = parsed
		}
	}
	return earliest
}

// generateCalendarDates produces one date per day from start to end (inclusive).
func generateCalendarDates(start, end time.Time) []time.Time {
	start = start.Truncate(24 * time.Hour)
	end = end.Truncate(24 * time.Hour)

	if end.Before(start) {
		return nil
	}

	days := int(end.Sub(start).Hours()/24) + 1
	dates := make([]time.Time, 0, days)
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		dates = append(dates, d)
	}
	return dates
}

// findEarliestTradeDate scans all holdings for the oldest trade date.
func findEarliestTradeDate(holdings []models.Holding) time.Time {
	var earliest time.Time

	for _, h := range holdings {
		for _, t := range h.Trades {
			parsed := parseTradeDate(t.Date)
			if parsed.IsZero() {
				continue
			}
			if earliest.IsZero() || parsed.Before(earliest) {
				earliest = parsed
			}
		}
	}

	return earliest
}

// parseTradeDate parses a trade date string which may be "2006-01-02" or "2006-01-02T15:04:05".
func parseTradeDate(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	// Try date-only first, then with time component
	for _, layout := range []string{"2006-01-02", "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// tryTimelineCache checks if persisted timeline snapshots cover the full requested range.
// Returns converted GrowthDataPoints and true on cache hit; nil and false on miss.
func (s *Service) tryTimelineCache(ctx context.Context, userID, name string, from, to time.Time) ([]models.GrowthDataPoint, bool) {
	tl := s.storage.TimelineStore()
	if tl == nil {
		return nil, false
	}

	latest, err := tl.GetLatest(ctx, userID, name)
	if err != nil || latest == nil {
		return nil, false
	}

	// Quick check: if even the latest snapshot is stale, skip the GetRange call entirely.
	// This doesn't catch mixed-version scenarios (today current, historical stale) —
	// that's handled after GetRange below.
	if latest.DataVersion != common.SchemaVersion {
		s.logger.Info().
			Str("cached_version", latest.DataVersion).
			Str("current_version", common.SchemaVersion).
			Msg("Timeline cache stale: schema version mismatch, forcing rebuild")
		return nil, false
	}

	// Cache must cover through at least the requested end date
	latestDate := latest.Date.Truncate(24 * time.Hour)
	toDate := to.Truncate(24 * time.Hour)
	if latestDate.Before(toDate) {
		s.logger.Debug().
			Str("latest", latestDate.Format("2006-01-02")).
			Str("to", toDate.Format("2006-01-02")).
			Msg("Timeline cache partial miss: latest < requested end")
		return nil, false
	}

	snapshots, err := tl.GetRange(ctx, userID, name, from, to)
	if err != nil || len(snapshots) == 0 {
		return nil, false
	}

	// Check oldest snapshot for stale schema — writeTodaySnapshot may have updated
	// today to current version while historical snapshots retain old field names.
	// Stale field names cause renamed fields to deserialize as zero.
	if snapshots[0].DataVersion != common.SchemaVersion {
		s.logger.Info().
			Str("cached_version", snapshots[0].DataVersion).
			Str("current_version", common.SchemaVersion).
			Str("oldest_date", snapshots[0].Date.Format("2006-01-02")).
			Msg("Timeline cache stale: oldest snapshot has old schema version, forcing rebuild")
		return nil, false
	}

	// Data integrity guard: if any snapshot has active holdings but zero equity,
	// the data is corrupt (e.g. stale JSON field names that deserialized as zero).
	// Reject the entire cache to force a fresh trade replay.
	for _, snap := range snapshots {
		if snap.HoldingCount > 0 && snap.EquityHoldingsValue == 0 {
			s.logger.Warn().
				Str("date", snap.Date.Format("2006-01-02")).
				Int("holding_count", snap.HoldingCount).
				Msg("Timeline cache corrupt: holdings with zero equity, forcing rebuild")
			return nil, false
		}
	}

	// Validate cache completeness: the first snapshot must be near the requested
	// start date. Without this check, a cache with only 2 snapshots covering a
	// 70-day range would be returned as a "hit" with severely incomplete data.
	firstSnapDate := snapshots[0].Date.Truncate(24 * time.Hour)
	fromDate := from.Truncate(24 * time.Hour)
	if firstSnapDate.Sub(fromDate) > 7*24*time.Hour {
		s.logger.Debug().
			Str("first_snap", firstSnapDate.Format("2006-01-02")).
			Str("from", fromDate.Format("2006-01-02")).
			Int("snapshots", len(snapshots)).
			Msg("Timeline cache partial miss: first snapshot too far from requested start")
		return nil, false
	}

	return snapshotsToGrowthPoints(snapshots), true
}

// snapshotsToGrowthPoints converts TimelineSnapshots to GrowthDataPoints.
func snapshotsToGrowthPoints(snapshots []models.TimelineSnapshot) []models.GrowthDataPoint {
	points := make([]models.GrowthDataPoint, len(snapshots))
	for i, snap := range snapshots {
		points[i] = models.GrowthDataPoint{
			Date:                    snap.Date,
			EquityHoldingsValue:     snap.EquityHoldingsValue,
			EquityHoldingsCost:      snap.EquityHoldingsCost,
			EquityHoldingsReturn:    snap.EquityHoldingsReturn,
			EquityHoldingsReturnPct: snap.EquityHoldingsReturnPct,
			HoldingCount:            snap.HoldingCount,
			CapitalGross:            snap.CapitalGross,
			CapitalAvailable:        snap.CapitalAvailable,
			PortfolioValue:          snap.PortfolioValue,
			CapitalContributionsNet: snap.CapitalContributionsNet,
		}
	}
	return points
}

// persistTimelineSnapshots converts GrowthDataPoints to TimelineSnapshots and saves them.
// Only persists points with Date < today — today's snapshot is managed by SyncPortfolio.
// Runs in a background goroutine; errors are logged, not returned.
func (s *Service) persistTimelineSnapshots(userID, portfolioName string, points []models.GrowthDataPoint, today time.Time, fxRate float64) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tl := s.storage.TimelineStore()
	if tl == nil {
		return
	}

	snapshots := make([]models.TimelineSnapshot, 0, len(points))
	now := time.Now()
	for _, p := range points {
		if !p.Date.Before(today) {
			continue // skip today — SyncPortfolio owns it
		}
		snapshots = append(snapshots, models.TimelineSnapshot{
			UserID:                  userID,
			PortfolioName:           portfolioName,
			Date:                    p.Date,
			EquityHoldingsValue:     p.EquityHoldingsValue,
			EquityHoldingsCost:      p.EquityHoldingsCost,
			EquityHoldingsReturn:    p.EquityHoldingsReturn,
			EquityHoldingsReturnPct: p.EquityHoldingsReturnPct,
			HoldingCount:            p.HoldingCount,
			CapitalGross:            p.CapitalGross,
			CapitalAvailable:        p.CapitalAvailable,
			PortfolioValue:          p.PortfolioValue,
			CapitalContributionsNet: p.CapitalContributionsNet,
			FXRate:                  fxRate,
			DataVersion:             common.SchemaVersion,
			ComputedAt:              now,
		})
	}

	if len(snapshots) == 0 {
		return
	}

	if err := tl.SaveBatch(ctx, snapshots); err != nil {
		s.logger.Warn().Err(err).Str("portfolio", portfolioName).Int("count", len(snapshots)).Msg("Failed to persist timeline snapshots")
	} else {
		s.logger.Info().Str("portfolio", portfolioName).Int("count", len(snapshots)).Msg("Timeline snapshots persisted (write-behind)")
	}
}
