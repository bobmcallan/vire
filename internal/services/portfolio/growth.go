package portfolio

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bobmccarthy/vire/internal/interfaces"
	"github.com/bobmccarthy/vire/internal/models"
)

// GetDailyGrowth returns daily portfolio value data points for a date range.
// It bulk-loads all data once then iterates dates in memory — O(holdings) reads
// instead of O(days × holdings).
func (s *Service) GetDailyGrowth(ctx context.Context, name string, opts interfaces.GrowthOptions) ([]models.GrowthDataPoint, error) {
	s.logger.Info().Str("name", name).Msg("Computing daily portfolio growth")

	// 1. Load portfolio once
	p, err := s.storage.PortfolioStorage().GetPortfolio(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("portfolio '%s' not found — sync it first: %w", name, err)
	}

	earliest := findEarliestTradeDate(p.Holdings)
	if earliest.IsZero() {
		return nil, fmt.Errorf("no trades found in portfolio '%s'", name)
	}

	// 2. Determine date range
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
		Time("from", from).
		Time("to", to).
		Int("days", len(dates)).
		Msg("Daily growth date range")

	if len(dates) == 0 {
		return nil, nil
	}

	// 3. Bulk-load all market data
	tickers := make([]string, 0, len(p.Holdings))
	for _, h := range p.Holdings {
		if len(h.Trades) > 0 {
			tickers = append(tickers, h.Ticker+".AU")
		}
	}

	allMarketData, err := s.storage.MarketDataStorage().GetMarketDataBatch(ctx, tickers)
	if err != nil {
		return nil, fmt.Errorf("failed to bulk-load market data: %w", err)
	}

	// 4. Index market data by ticker
	mdByTicker := make(map[string]*models.MarketData, len(allMarketData))
	for _, md := range allMarketData {
		mdByTicker[md.Ticker] = md
	}

	// 5. Iterate dates and compute portfolio value
	points := make([]models.GrowthDataPoint, 0, len(dates))
	for _, date := range dates {
		var totalValue, totalCost float64
		holdingCount := 0

		for _, h := range p.Holdings {
			if len(h.Trades) == 0 {
				continue
			}

			units, _, cost := replayTradesAsOf(h.Trades, date)
			if units <= 0 {
				continue
			}

			ticker := h.Ticker + ".AU"
			md := mdByTicker[ticker]
			if md == nil || len(md.EOD) == 0 {
				continue
			}

			closePrice, _, found := findClosingPriceAsOf(md.EOD, date)
			if !found {
				continue
			}

			totalValue += units * closePrice
			totalCost += cost
			holdingCount++
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
			Date:         date,
			TotalValue:   totalValue,
			TotalCost:    totalCost,
			GainLoss:     gainLoss,
			GainLossPct:  gainLossPct,
			HoldingCount: holdingCount,
		})
	}

	s.logger.Info().Str("name", name).Int("points", len(points)).Msg("Daily portfolio growth complete")
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
