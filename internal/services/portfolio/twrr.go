package portfolio

import (
	"math"
	"sort"
	"strings"
	"time"

	"github.com/bobmcallan/vire/internal/models"
)

// CalculateTWRR computes the time-weighted rate of return for a holding.
// It splits the holding period into sub-periods at each buy/sell trade date,
// computes the price return for each sub-period using EOD close prices,
// and geometrically links them.
//
// Returns the TWRR as a percentage.
// If the holding period >= 365 days, the result is annualised.
// If < 365 days, the result is cumulative (not annualised).
func CalculateTWRR(trades []*models.NavexaTrade, eodBars []models.EODBar, currentPrice float64, now time.Time) float64 {
	if len(trades) == 0 {
		return 0
	}

	// Filter to cash-flow trades only (buy/sell/opening balance).
	// Cost base adjustments are accounting entries, not cash flows.
	type tradeEvent struct {
		date  time.Time
		ttype string
		units float64
		price float64
	}
	var events []tradeEvent
	for _, t := range trades {
		tt := strings.ToLower(t.Type)
		if tt != "buy" && tt != "sell" && tt != "opening balance" {
			continue
		}
		d := parseTradeDate(t.Date)
		if d.IsZero() {
			continue
		}
		events = append(events, tradeEvent{date: d, ttype: tt, units: t.Units, price: t.Price})
	}
	if len(events) == 0 {
		return 0
	}

	// Sort events by date
	sort.Slice(events, func(i, j int) bool {
		return events[i].date.Before(events[j].date)
	})

	firstTradeDate := events[0].date

	// Collect unique trade dates as sub-period boundaries
	dateSeen := make(map[time.Time]bool)
	var tradeDates []time.Time
	for _, e := range events {
		d := e.date.Truncate(24 * time.Hour)
		if !dateSeen[d] {
			dateSeen[d] = true
			tradeDates = append(tradeDates, d)
		}
	}
	sort.Slice(tradeDates, func(i, j int) bool { return tradeDates[i].Before(tradeDates[j]) })

	// Helper to find close price on or before a date
	findClose := func(target time.Time) (float64, bool) {
		price, _, found := findClosingPriceAsOf(eodBars, target)
		return price, found
	}

	// Fallback: if no EOD data, compute simple return from first trade price to current price
	if len(eodBars) == 0 {
		// Use the first buy trade price as the starting price
		var startPrice float64
		for _, e := range events {
			if e.ttype == "buy" || e.ttype == "opening balance" {
				startPrice = e.price
				break
			}
		}
		if startPrice <= 0 || currentPrice <= 0 {
			return 0
		}
		cumulative := currentPrice/startPrice - 1
		days := now.Sub(firstTradeDate).Hours() / 24
		return annualiseTWRR(cumulative, days) * 100
	}

	// Build sub-period returns by geometric linking of close-to-close ratios
	chainedReturn := 1.0

	// Track running units to know when position is open
	runningUnits := 0.0

	for i, tradeDate := range tradeDates {
		// Process all events on this date to get units before the trade
		unitsBefore := runningUnits

		for _, e := range events {
			ed := e.date.Truncate(24 * time.Hour)
			if !ed.Equal(tradeDate) {
				continue
			}
			switch e.ttype {
			case "buy", "opening balance":
				runningUnits += e.units
			case "sell":
				runningUnits -= e.units
			}
		}

		// Sub-period: from previous trade date (or first trade date) to this trade date
		if i == 0 {
			// No sub-period before the first trade
			continue
		}

		prevDate := tradeDates[i-1]

		// Only compute sub-period if position was open (units > 0)
		if unitsBefore <= 0 {
			continue
		}

		beginPrice, beginFound := findClose(prevDate)
		endPrice, endFound := findClose(tradeDate)

		if !beginFound || !endFound || beginPrice <= 0 {
			continue
		}

		subReturn := endPrice / beginPrice
		chainedReturn *= subReturn
	}

	// Final sub-period: from last trade date to now (if position still open)
	if runningUnits > 0 && currentPrice > 0 {
		lastTradeDate := tradeDates[len(tradeDates)-1]
		beginPrice, beginFound := findClose(lastTradeDate)
		if beginFound && beginPrice > 0 {
			subReturn := currentPrice / beginPrice
			chainedReturn *= subReturn
		}
	} else if runningUnits <= 0 {
		// Closed position: last sub-period ends at the last sell date
		// (already handled in the loop above)
	}

	cumulative := chainedReturn - 1

	// Determine holding period for annualisation
	var endDate time.Time
	if runningUnits > 0 {
		endDate = now
	} else {
		// Closed position: use the last trade date
		endDate = tradeDates[len(tradeDates)-1]
	}
	days := endDate.Sub(firstTradeDate).Hours() / 24

	return annualiseTWRR(cumulative, days) * 100
}

// annualiseTWRR converts a cumulative return to annualised if >= 365 days,
// otherwise returns cumulative. Input and output are decimal (not percentage).
func annualiseTWRR(cumulative float64, days float64) float64 {
	if days < 365 {
		return cumulative
	}
	// Annualise: (1 + cumulative)^(365/days) - 1
	base := 1 + cumulative
	if base <= 0 {
		// Total loss or worse — can't raise negative to fractional power
		return cumulative
	}
	return math.Pow(base, 365/days) - 1
}
