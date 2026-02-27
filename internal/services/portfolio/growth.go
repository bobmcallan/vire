package portfolio

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

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

		// Process this trade
		switch strings.ToLower(t.Type) {
		case "buy", "opening balance":
			cost := t.Units*t.Price + t.Fees
			s.TotalCost += cost
			s.Units += t.Units
			cashDelta -= cost
		case "sell":
			if s.Units > 0 {
				costPerUnit := s.TotalCost / s.Units
				s.TotalCost -= t.Units * costPerUnit
				s.Units -= t.Units
				proceeds := t.Units*t.Price - t.Fees
				if proceeds > 0 {
					cashDelta += proceeds
				}
			}
		case "cost base increase":
			s.TotalCost += t.Value
		case "cost base decrease":
			s.TotalCost -= t.Value
		}
		s.Cursor++
	}
	return cashDelta
}

// newHoldingGrowthState creates a state for a holding with trades sorted by date.
func newHoldingGrowthState(ticker string, trades []*models.NavexaTrade) *holdingGrowthState {
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

	// Phase 2: Determine date range
	from := opts.From
	if from.IsZero() {
		from = earliest
	}
	to := opts.To
	if to.IsZero() {
		to = time.Now().AddDate(0, 0, -1).Truncate(24 * time.Hour)
	}
	if to.After(time.Now()) {
		to = time.Now().AddDate(0, 0, -1).Truncate(24 * time.Hour)
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

	// Phase 4: Initialize incremental trade replay states
	phaseStart = time.Now()
	holdingStates := make([]*holdingGrowthState, 0, len(p.Holdings))
	for _, h := range p.Holdings {
		if len(h.Trades) == 0 {
			continue
		}
		ticker := h.EODHDTicker()
		// Only include holdings with market data
		if md := mdByTicker[ticker]; md != nil && len(md.EOD) > 0 {
			holdingStates = append(holdingStates, newHoldingGrowthState(ticker, h.Trades))
		}
	}

	// Phase 5: Prepare cash flow cursor for single-pass merge
	// Transactions must be date-sorted ascending for cursor-based iteration.
	txs := opts.Transactions
	sort.Slice(txs, func(i, j int) bool { return txs[i].Date.Before(txs[j].Date) })
	txCursor := 0
	var runningCashBalance, runningNetDeployed float64

	// Phase 6: Iterate dates and compute portfolio value using incremental replay
	points := make([]models.GrowthDataPoint, 0, len(dates))
	for _, date := range dates {
		var totalValue, totalCost float64
		holdingCount := 0

		for _, hs := range holdingStates {
			// Advance state to include all trades up to this date;
			// cashDelta reflects money spent on buys (negative) and received from sells (positive).
			tradeCashDelta := hs.advanceTo(date)
			runningCashBalance += tradeCashDelta

			if hs.Units <= 0 {
				continue
			}

			md := mdByTicker[hs.Ticker]
			closePrice, _, found := findClosingPriceAsOf(md.EOD, date)
			if !found {
				continue
			}

			totalValue += hs.Units * closePrice
			totalCost += hs.TotalCost
			holdingCount++
		}

		// Outlier detection: cap day-over-day swings exceeding 50%.
		// Corrupted EOD data (e.g. bad EODHD price) can produce implausible
		// portfolio values that distort charts and derived indicators.
		if len(points) > 0 && totalValue > 0 {
			prevValue := points[len(points)-1].TotalValue
			if prevValue > 0 {
				ratio := totalValue / prevValue
				if ratio > 1.5 || ratio < 0.5 {
					totalValue = prevValue
				}
			}
		}

		// Advance cash flow cursor: process all transactions up to this date
		endOfDay := date.AddDate(0, 0, 1) // exclusive upper bound
		for txCursor < len(txs) && txs[txCursor].Date.Before(endOfDay) {
			tx := txs[txCursor]
			txCursor++
			// Skip internal transfers — these are rebalancing between portfolio cash
			// and external balance accounts, not real cash flows.
			if tx.IsInternalTransfer() {
				continue
			}
			if models.IsInflowType(tx.Type) {
				runningCashBalance += tx.Amount
			} else {
				runningCashBalance -= tx.Amount
			}
			// Net deployed tracks only deposits/contributions minus withdrawals
			switch tx.Type {
			case models.CashTxDeposit, models.CashTxContribution:
				runningNetDeployed += tx.Amount
			case models.CashTxWithdrawal:
				runningNetDeployed -= tx.Amount
			}
		}

		if totalValue == 0 && totalCost == 0 {
			continue
		}

		gainLoss := totalValue - totalCost
		gainLossPct := 0.0
		if totalCost > 0 {
			gainLossPct = (gainLoss / totalCost) * 100
		}

		points = append(points, models.GrowthDataPoint{
			Date:            date,
			TotalValue:      totalValue,
			TotalCost:       totalCost,
			NetReturn:       gainLoss,
			NetReturnPct:    gainLossPct,
			HoldingCount:    holdingCount,
			CashBalance:     runningCashBalance,
			ExternalBalance: p.ExternalBalanceTotal,
			TotalCapital:    totalValue + runningCashBalance + p.ExternalBalanceTotal,
			NetDeployed:     runningNetDeployed,
		})
	}
	s.logger.Info().Dur("elapsed", time.Since(phaseStart)).Int("days", len(dates)).Int("holdings", len(holdingStates)).Msg("GetDailyGrowth: date iteration complete")

	s.logger.Info().Str("name", name).Int("points", len(points)).Dur("elapsed", time.Since(funcStart)).Msg("GetDailyGrowth: TOTAL")
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
