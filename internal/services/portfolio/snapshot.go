package portfolio

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bobmcallan/vire/internal/models"
)

// replayTradesAsOf replays trade history up to cutoff (inclusive) to compute units held and cost basis.
// Mirrors calculateAvgCostFromTrades but adds a date filter.
func replayTradesAsOf(trades []*models.NavexaTrade, cutoff time.Time) (units, avgCost, totalCost float64) {
	cutoffStr := cutoff.Format("2006-01-02")

	for _, t := range trades {
		tradeDate := normalizeDateStr(strings.TrimSpace(t.Date))
		if tradeDate == "" || tradeDate > cutoffStr {
			continue
		}

		switch strings.ToLower(t.Type) {
		case "buy", "opening balance":
			cost := t.Units*t.Price + t.Fees
			totalCost += cost
			units += t.Units
		case "sell":
			if units > 0 {
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

	return units, avgCost, totalCost
}

// normalizeDateStr strips a time component (e.g. "T00:00:00") from a date string,
// returning just the "YYYY-MM-DD" portion for reliable string comparison.
func normalizeDateStr(s string) string {
	if idx := strings.IndexByte(s, 'T'); idx == 10 {
		return s[:10]
	}
	return s
}

// findClosingPriceAsOf uses binary search on EOD bars (descending by date) to find the first bar
// at or before the target date. Returns the close price and the actual bar date.
// O(log N) complexity instead of O(N) linear scan.
func findClosingPriceAsOf(bars []models.EODBar, asOf time.Time) (closePrice float64, barDate time.Time, found bool) {
	if len(bars) == 0 {
		return 0, time.Time{}, false
	}

	target := asOf.Truncate(24 * time.Hour)

	// Bars are sorted descending (newest first).
	// We want the first bar where bar.Date <= target.
	// Binary search for the smallest index where bar.Date <= target.
	// Since bars are descending, we search for the first index where bar.Date <= target.
	idx := sort.Search(len(bars), func(i int) bool {
		return !bars[i].Date.Truncate(24 * time.Hour).After(target)
	})

	if idx >= len(bars) {
		return 0, time.Time{}, false
	}

	bar := bars[idx]
	barDay := bar.Date.Truncate(24 * time.Hour)
	return bar.Close, barDay, true
}

// GetPortfolioSnapshot reconstructs portfolio state as of a historical date.
func (s *Service) GetPortfolioSnapshot(ctx context.Context, name string, asOf time.Time) (*models.PortfolioSnapshot, error) {
	s.logger.Info().Str("name", name).Str("asOf", asOf.Format(time.RFC3339)).Msg("Building portfolio snapshot")

	portfolio, err := s.storage.PortfolioStorage().GetPortfolio(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("portfolio '%s' not found — sync it first with sync_portfolio: %w", name, err)
	}

	snapshot := &models.PortfolioSnapshot{
		PortfolioName: name,
		AsOfDate:      asOf,
	}

	var earliestPriceDate time.Time

	for _, h := range portfolio.Holdings {
		if len(h.Trades) == 0 {
			continue
		}

		units, avgCost, totalCost := replayTradesAsOf(h.Trades, asOf)
		if units <= 0 {
			continue
		}

		// Look up EOD close price from stored market data
		ticker := h.Ticker + ".AU"
		marketData, err := s.storage.MarketDataStorage().GetMarketData(ctx, ticker)
		if err != nil || len(marketData.EOD) == 0 {
			s.logger.Warn().Str("ticker", ticker).Msg("No market data for snapshot — skipping holding")
			continue
		}

		closePrice, barDate, found := findClosingPriceAsOf(marketData.EOD, asOf)
		if !found {
			s.logger.Warn().Str("ticker", ticker).Msg("No EOD bar at or before snapshot date — skipping")
			continue
		}

		if earliestPriceDate.IsZero() || barDate.Before(earliestPriceDate) {
			earliestPriceDate = barDate
		}

		marketValue := units * closePrice
		gainLoss := marketValue - totalCost
		gainLossPct := 0.0
		if totalCost > 0 {
			gainLossPct = (gainLoss / totalCost) * 100
		}

		snapshot.Holdings = append(snapshot.Holdings, models.SnapshotHolding{
			Ticker:      h.Ticker,
			Name:        h.Name,
			Units:       units,
			AvgCost:     avgCost,
			TotalCost:   totalCost,
			ClosePrice:  closePrice,
			MarketValue: marketValue,
			GainLoss:    gainLoss,
			GainLossPct: gainLossPct,
		})

		snapshot.TotalValue += marketValue
		snapshot.TotalCost += totalCost
	}

	// Compute weights and portfolio-level totals
	for i := range snapshot.Holdings {
		if snapshot.TotalValue > 0 {
			snapshot.Holdings[i].Weight = (snapshot.Holdings[i].MarketValue / snapshot.TotalValue) * 100
		}
	}

	snapshot.TotalGain = snapshot.TotalValue - snapshot.TotalCost
	if snapshot.TotalCost > 0 {
		snapshot.TotalGainPct = (snapshot.TotalGain / snapshot.TotalCost) * 100
	}

	if !earliestPriceDate.IsZero() {
		snapshot.PriceDate = earliestPriceDate
	} else {
		snapshot.PriceDate = asOf
	}

	s.logger.Info().
		Str("name", name).
		Int("holdings", len(snapshot.Holdings)).
		Float64("totalValue", snapshot.TotalValue).
		Msg("Portfolio snapshot complete")

	return snapshot, nil
}
