package portfolio

import (
	"math"
	"sort"
	"strings"
	"time"

	"github.com/bobmcallan/vire/internal/models"
)

// cashFlow represents a single cash flow for XIRR calculation.
// Negative values = money out (buys), positive values = money in (sells, current value).
type cashFlow struct {
	date   time.Time
	amount float64
}

// CalculateXIRR computes the annualised internal rate of return (XIRR) for a holding
// using Newton-Raphson iteration. Cash flows are derived from trades:
//   - Buy/Opening Balance → negative cash flow (money invested)
//   - Sell → positive cash flow (money received)
//   - Final: current market value as a positive cash flow at today's date
//
// If includeDividends is true, dividend income is added as a positive cash flow at today's date.
// Returns the XIRR as a percentage, or 0 if it cannot be computed.
func CalculateXIRR(trades []*models.NavexaTrade, currentMarketValue float64, dividends float64, includeDividends bool, now time.Time) float64 {
	if len(trades) == 0 {
		return 0
	}

	var flows []cashFlow

	for _, t := range trades {
		tt := strings.ToLower(t.Type)
		d := parseTradeDate(t.Date)
		if d.IsZero() {
			continue
		}

		switch tt {
		case "buy", "opening balance":
			// Money out: negative
			invested := t.Units*t.Price + t.Fees
			flows = append(flows, cashFlow{date: d, amount: -invested})
		case "sell":
			// Money in: positive (proceeds minus fees)
			proceeds := t.Units*t.Price - t.Fees
			flows = append(flows, cashFlow{date: d, amount: proceeds})
		case "cost base increase":
			// Additional cost: negative
			flows = append(flows, cashFlow{date: d, amount: -t.Value})
		case "cost base decrease":
			// Returned cost: positive
			flows = append(flows, cashFlow{date: d, amount: t.Value})
		}
	}

	if len(flows) == 0 {
		return 0
	}

	// Add current market value as final positive cash flow
	terminalValue := currentMarketValue
	if includeDividends {
		terminalValue += dividends
	}
	if terminalValue > 0 {
		flows = append(flows, cashFlow{date: now, amount: terminalValue})
	}

	// Sort by date
	sort.Slice(flows, func(i, j int) bool {
		return flows[i].date.Before(flows[j].date)
	})

	// Need at least one negative and one positive flow
	hasNeg, hasPos := false, false
	for _, f := range flows {
		if f.amount < 0 {
			hasNeg = true
		}
		if f.amount > 0 {
			hasPos = true
		}
	}
	if !hasNeg || !hasPos {
		return 0
	}

	rate := solveXIRR(flows)
	if math.IsNaN(rate) || math.IsInf(rate, 0) {
		return 0
	}

	return rate * 100
}

// solveXIRR uses Newton-Raphson to find the rate r such that NPV(r) = 0.
// NPV(r) = sum of amount_i / (1 + r)^(years_i) where years_i = days from first date / 365.25
// Returns the rate as a decimal (e.g., 0.12 for 12%).
func solveXIRR(flows []cashFlow) float64 {
	const (
		maxIter = 100
		tol     = 1e-7
		minRate = -0.999 // rate can't go below -99.9%
	)

	baseDate := flows[0].date

	// Convert dates to year fractions
	years := make([]float64, len(flows))
	for i, f := range flows {
		days := f.date.Sub(baseDate).Hours() / 24
		years[i] = days / 365.25
	}

	// Initial guess: use simple return as starting point
	totalInvested := 0.0
	totalReceived := 0.0
	for _, f := range flows {
		if f.amount < 0 {
			totalInvested -= f.amount
		} else {
			totalReceived += f.amount
		}
	}

	guess := 0.1 // default 10%
	if totalInvested > 0 {
		simpleReturn := totalReceived/totalInvested - 1
		// Clamp initial guess to reasonable range
		if simpleReturn > -0.9 && simpleReturn < 10 {
			guess = simpleReturn
		}
	}

	rate := guess

	for iter := 0; iter < maxIter; iter++ {
		npv := 0.0
		dnpv := 0.0 // derivative of NPV with respect to rate

		for i, f := range flows {
			y := years[i]
			base := 1 + rate
			if base <= 0 {
				// Avoid negative base with fractional exponent
				rate = minRate
				base = 1 + rate
			}
			discount := math.Pow(base, y)
			if discount == 0 {
				continue
			}
			npv += f.amount / discount
			if y != 0 {
				dnpv -= y * f.amount / (discount * base)
			}
		}

		if math.Abs(npv) < tol {
			return rate
		}

		if dnpv == 0 {
			// Derivative is zero — can't continue Newton-Raphson
			break
		}

		newRate := rate - npv/dnpv

		// Clamp to prevent wild oscillation
		if newRate < minRate {
			newRate = minRate
		}
		if newRate > 100 { // 10000% annual return cap
			newRate = 100
		}

		rate = newRate
	}

	// Fallback: bisection method if Newton-Raphson didn't converge
	return bisectXIRR(flows, years)
}

// bisectXIRR uses bisection as a fallback solver for XIRR.
func bisectXIRR(flows []cashFlow, years []float64) float64 {
	const (
		maxIter = 200
		tol     = 1e-6
	)

	npvAt := func(rate float64) float64 {
		sum := 0.0
		for i, f := range flows {
			base := 1 + rate
			if base <= 0 {
				return math.NaN()
			}
			sum += f.amount / math.Pow(base, years[i])
		}
		return sum
	}

	// Find bracket [lo, hi] where NPV changes sign
	lo, hi := -0.99, 10.0
	npvLo := npvAt(lo)
	npvHi := npvAt(hi)

	if math.IsNaN(npvLo) || math.IsNaN(npvHi) {
		return math.NaN()
	}
	if npvLo*npvHi > 0 {
		// Same sign — no root in this bracket
		return math.NaN()
	}

	for iter := 0; iter < maxIter; iter++ {
		mid := (lo + hi) / 2
		npvMid := npvAt(mid)
		if math.IsNaN(npvMid) {
			return math.NaN()
		}
		if math.Abs(npvMid) < tol {
			return mid
		}
		if npvMid*npvLo < 0 {
			hi = mid
			// npvHi = npvMid (not needed but conceptually)
		} else {
			lo = mid
			npvLo = npvMid
		}
	}

	return (lo + hi) / 2
}
