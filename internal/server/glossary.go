package server

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/bobmcallan/vire/internal/models"
)

// handleGlossary returns an active glossary of portfolio terms with live examples.
func (s *Server) handleGlossary(w http.ResponseWriter, r *http.Request, name string) {
	if !RequireMethod(w, r, http.MethodGet) {
		return
	}

	ctx := s.app.InjectNavexaClient(r.Context())

	portfolio, err := s.app.PortfolioService.GetPortfolio(ctx, name)
	if err != nil {
		WriteError(w, http.StatusNotFound, fmt.Sprintf("Portfolio '%s' not found", name))
		return
	}

	// Non-fatal enrichment: capital performance and indicators
	var capitalPerf *models.CapitalPerformance
	if perf, err := s.app.CashFlowService.CalculatePerformance(ctx, name); err == nil {
		capitalPerf = perf
	}

	var indicators *models.PortfolioIndicators
	if ind, err := s.app.PortfolioService.GetPortfolioIndicators(ctx, name); err == nil {
		indicators = ind
	}

	resp := buildGlossary(portfolio, capitalPerf, indicators)
	WriteJSON(w, http.StatusOK, resp)
}

// buildGlossary constructs the glossary response from portfolio data.
func buildGlossary(p *models.Portfolio, cp *models.CapitalPerformance, ind *models.PortfolioIndicators) *models.GlossaryResponse {
	resp := &models.GlossaryResponse{
		PortfolioName: p.Name,
		GeneratedAt:   time.Now(),
	}

	resp.Categories = append(resp.Categories, buildValuationCategory(p))

	if len(p.Holdings) > 0 {
		resp.Categories = append(resp.Categories, buildHoldingCategory(p))
	}

	if cp != nil && cp.TransactionCount > 0 {
		resp.Categories = append(resp.Categories, buildCapitalCategory(cp))
		if len(cp.ExternalBalances) > 0 {
			resp.Categories = append(resp.Categories, buildExternalBalanceCategory(cp))
		}
	}

	if ind != nil && ind.DataPoints > 0 {
		resp.Categories = append(resp.Categories, buildIndicatorCategory(ind))
	}

	resp.Categories = append(resp.Categories, buildGrowthCategory(p, cp))

	return resp
}

func buildValuationCategory(p *models.Portfolio) models.GlossaryCategory {
	return models.GlossaryCategory{
		Name: "Portfolio Valuation",
		Terms: []models.GlossaryTerm{
			{
				Term:       "total_value",
				Label:      "Total Value",
				Definition: "Current market value of all equity holdings at today's prices.",
				Formula:    "sum(units * current_price) for each holding",
				Value:      p.TotalValueHoldings,
				Example:    fmtMoney(p.TotalValueHoldings),
			},
			{
				Term:       "total_cost",
				Label:      "Total Cost",
				Definition: "Total cost basis of all holdings (average cost * units).",
				Formula:    "sum(avg_cost * units) for each holding",
				Value:      p.TotalCost,
				Example:    fmtMoney(p.TotalCost),
			},
			{
				Term:       "net_return",
				Label:      "Net Return",
				Definition: "Unrealised gain or loss across the portfolio.",
				Formula:    "total_value - total_cost",
				Value:      p.TotalNetReturn,
				Example:    fmt.Sprintf("%s - %s = %s", fmtMoney(p.TotalValueHoldings), fmtMoney(p.TotalCost), fmtMoney(p.TotalNetReturn)),
			},
			{
				Term:       "net_return_pct",
				Label:      "Net Return %",
				Definition: "Portfolio return as a percentage of cost.",
				Formula:    "(net_return / total_cost) * 100",
				Value:      p.TotalNetReturnPct,
				Example:    fmt.Sprintf("(%s / %s) * 100 = %.2f%%", fmtMoney(p.TotalNetReturn), fmtMoney(p.TotalCost), p.TotalNetReturnPct),
			},
			{
				Term:       "external_balance_total",
				Label:      "External Balance Total",
				Definition: "Sum of all external balance accounts (cash, accumulate, term deposits, offset).",
				Value:      p.ExternalBalanceTotal,
				Example:    fmtExternalBalances(p.ExternalBalances, p.ExternalBalanceTotal),
			},
			{
				Term:       "total_capital",
				Label:      "Total Capital",
				Definition: "Total value of all assets: equity holdings plus external balances.",
				Formula:    "total_value + external_balance_total",
				Value:      p.TotalValue,
				Example:    fmt.Sprintf("%s + %s = %s", fmtMoney(p.TotalValueHoldings), fmtMoney(p.ExternalBalanceTotal), fmtMoney(p.TotalValue)),
			},
		},
	}
}

func buildHoldingCategory(p *models.Portfolio) models.GlossaryCategory {
	// Sort holdings by weight descending and take top 3 for examples
	top := topHoldings(p.Holdings, 3)

	terms := []models.GlossaryTerm{
		{
			Term:       "market_value",
			Label:      "Market Value",
			Definition: "Current value of a holding position.",
			Formula:    "units * current_price",
			Value:      topVal(top, func(h models.Holding) float64 { return h.MarketValue }),
			Example: fmtHoldingCalc(top, "market_value", func(h models.Holding) string {
				return fmt.Sprintf("%.2f units * %s = %s", h.Units, fmtMoney(h.CurrentPrice), fmtMoney(h.MarketValue))
			}),
		},
		{
			Term:       "avg_cost",
			Label:      "Average Cost",
			Definition: "Average purchase price per unit, including brokerage fees.",
			Formula:    "total_cost / units",
			Value:      topVal(top, func(h models.Holding) float64 { return h.AvgCost }),
			Example:    fmtHoldingCalc(top, "avg_cost", func(h models.Holding) string { return fmt.Sprintf("%s per unit", fmtMoney(h.AvgCost)) }),
		},
		{
			Term:       "weight",
			Label:      "Weight %",
			Definition: "Holding's proportion of the total portfolio value.",
			Formula:    "(market_value / total_portfolio_value) * 100",
			Value:      topVal(top, func(h models.Holding) float64 { return h.Weight }),
			Example: fmtHoldingCalc(top, "weight", func(h models.Holding) string {
				return fmt.Sprintf("(%s / %s) * 100 = %.2f%%", fmtMoney(h.MarketValue), fmtMoney(p.TotalValueHoldings), h.Weight)
			}),
		},
		{
			Term:       "net_return",
			Label:      "Net Return",
			Definition: "Unrealised gain or loss on a single holding.",
			Formula:    "market_value - total_cost",
			Value:      topVal(top, func(h models.Holding) float64 { return h.NetReturn }),
			Example: fmtHoldingCalc(top, "net_return", func(h models.Holding) string {
				return fmt.Sprintf("%s - %s = %s", fmtMoney(h.MarketValue), fmtMoney(h.TotalCost), fmtMoney(h.NetReturn))
			}),
		},
		{
			Term:       "net_return_pct",
			Label:      "Net Return %",
			Definition: "Holding return as a percentage of total invested capital.",
			Formula:    "(net_return / total_invested) * 100",
			Value:      topVal(top, func(h models.Holding) float64 { return h.NetReturnPct }),
			Example:    fmtHoldingCalc(top, "net_return_pct", func(h models.Holding) string { return fmt.Sprintf("%.2f%%", h.NetReturnPct) }),
		},
	}

	return models.GlossaryCategory{
		Name:  "Holding Metrics",
		Terms: terms,
	}
}

func buildCapitalCategory(cp *models.CapitalPerformance) models.GlossaryCategory {
	return models.GlossaryCategory{
		Name: "Capital Performance",
		Terms: []models.GlossaryTerm{
			{
				Term:       "total_deposited",
				Label:      "Total Deposited",
				Definition: "Sum of all credits into the portfolio (deposits, contributions, transfers in, dividends).",
				Value:      cp.TotalDeposited,
				Example:    fmtMoney(cp.TotalDeposited),
			},
			{
				Term:       "total_withdrawn",
				Label:      "Total Withdrawn",
				Definition: "Sum of all debits from the portfolio (withdrawals, fees, transfers out).",
				Formula:    "sum(debits)",
				Value:      cp.TotalWithdrawn,
				Example:    fmtMoney(cp.TotalWithdrawn),
			},
			{
				Term:       "net_capital_deployed",
				Label:      "Net Capital Deployed",
				Definition: "Net capital currently deployed in the portfolio.",
				Formula:    "total_deposited - total_withdrawn",
				Value:      cp.NetCapitalDeployed,
				Example:    fmt.Sprintf("%s - %s = %s", fmtMoney(cp.TotalDeposited), fmtMoney(cp.TotalWithdrawn), fmtMoney(cp.NetCapitalDeployed)),
			},
			{
				Term:       "simple_return_pct",
				Label:      "Simple Return %",
				Definition: "Simple return on deployed capital (not time-weighted).",
				Formula:    "(current_portfolio_value - net_capital_deployed) / net_capital_deployed * 100",
				Value:      cp.SimpleReturnPct,
				Example:    fmt.Sprintf("(%s - %s) / %s * 100 = %.2f%%", fmtMoney(cp.CurrentPortfolioValue), fmtMoney(cp.NetCapitalDeployed), fmtMoney(cp.NetCapitalDeployed), cp.SimpleReturnPct),
			},
			{
				Term:       "annualized_return_pct",
				Label:      "Annualized Return % (XIRR)",
				Definition: "Time-weighted annualized return using the XIRR method. Accounts for the timing and size of each cash flow.",
				Formula:    "XIRR(cash_flows, current_value)",
				Value:      cp.AnnualizedReturnPct,
				Example:    fmt.Sprintf("%.2f%% annualized", cp.AnnualizedReturnPct),
			},
		},
	}
}

func buildExternalBalanceCategory(cp *models.CapitalPerformance) models.GlossaryCategory {
	terms := make([]models.GlossaryTerm, 0, len(cp.ExternalBalances)*2)

	for _, eb := range cp.ExternalBalances {
		terms = append(terms, models.GlossaryTerm{
			Term:       fmt.Sprintf("%s_net_transferred", eb.Category),
			Label:      fmt.Sprintf("%s — Net Transferred", fmtCategoryLabel(eb.Category)),
			Definition: fmt.Sprintf("Net amount transferred to the %s external balance.", eb.Category),
			Formula:    "total_out - total_in",
			Value:      eb.NetTransferred,
			Example:    fmt.Sprintf("Out: %s - In: %s = %s", fmtMoney(eb.TotalOut), fmtMoney(eb.TotalIn), fmtMoney(eb.NetTransferred)),
		})
		terms = append(terms, models.GlossaryTerm{
			Term:       fmt.Sprintf("%s_gain_loss", eb.Category),
			Label:      fmt.Sprintf("%s — Gain/Loss", fmtCategoryLabel(eb.Category)),
			Definition: fmt.Sprintf("Investment gain or loss on the %s balance. Positive means the balance grew beyond what was transferred in.", eb.Category),
			Formula:    "current_balance - net_transferred",
			Value:      eb.GainLoss,
			Example:    fmt.Sprintf("Current: %s - Transferred: %s = %s", fmtMoney(eb.CurrentBalance), fmtMoney(eb.NetTransferred), fmtMoney(eb.GainLoss)),
		})
	}

	return models.GlossaryCategory{
		Name:  "External Balance Performance",
		Terms: terms,
	}
}

func buildIndicatorCategory(ind *models.PortfolioIndicators) models.GlossaryCategory {
	return models.GlossaryCategory{
		Name: "Technical Indicators",
		Terms: []models.GlossaryTerm{
			{
				Term:       "ema_20",
				Label:      "EMA 20",
				Definition: "20-day Exponential Moving Average of portfolio value. Short-term trend indicator.",
				Value:      ind.EMA20,
				Example:    fmt.Sprintf("%s (current value %s is %s)", fmtMoney(ind.EMA20), fmtMoney(ind.CurrentValue), aboveBelow(ind.AboveEMA20)),
			},
			{
				Term:       "ema_50",
				Label:      "EMA 50",
				Definition: "50-day Exponential Moving Average. Medium-term trend indicator.",
				Value:      ind.EMA50,
				Example:    fmt.Sprintf("%s (current value is %s)", fmtMoney(ind.EMA50), aboveBelow(ind.AboveEMA50)),
			},
			{
				Term:       "ema_200",
				Label:      "EMA 200",
				Definition: "200-day Exponential Moving Average. Long-term trend indicator.",
				Value:      ind.EMA200,
				Example:    fmt.Sprintf("%s (current value is %s)", fmtMoney(ind.EMA200), aboveBelow(ind.AboveEMA200)),
			},
			{
				Term:       "rsi",
				Label:      "RSI",
				Definition: "Relative Strength Index (0-100). Below 30 is oversold, above 70 is overbought.",
				Formula:    "100 - (100 / (1 + average_gain / average_loss))",
				Value:      ind.RSI,
				Example:    fmt.Sprintf("%.1f — %s", ind.RSI, ind.RSISignal),
			},
			{
				Term:       "trend",
				Label:      "Trend",
				Definition: "Overall portfolio trend direction based on EMA crossovers and RSI.",
				Value:      string(ind.Trend),
				Example:    fmt.Sprintf("%s — %s", string(ind.Trend), ind.TrendDescription),
			},
		},
	}
}

func buildGrowthCategory(p *models.Portfolio, cp *models.CapitalPerformance) models.GlossaryCategory {
	yesterdayChange := p.TotalValueHoldings - p.YesterdayTotal
	lastWeekChange := p.TotalValueHoldings - p.LastWeekTotal

	cashBalance := 0.0
	netDeployed := 0.0
	if cp != nil {
		netDeployed = cp.NetCapitalDeployed
	}

	return models.GlossaryCategory{
		Name: "Growth Metrics",
		Terms: []models.GlossaryTerm{
			{
				Term:       "yesterday_change",
				Label:      "Yesterday Change",
				Definition: "Value change since yesterday's close.",
				Formula:    "current_value - yesterday_close",
				Value:      yesterdayChange,
				Example:    fmt.Sprintf("%s - %s = %s (%.2f%%)", fmtMoney(p.TotalValueHoldings), fmtMoney(p.YesterdayTotal), fmtMoney(yesterdayChange), p.YesterdayTotalPct),
			},
			{
				Term:       "last_week_change",
				Label:      "Last Week Change",
				Definition: "Value change since last week's close.",
				Formula:    "current_value - last_week_close",
				Value:      lastWeekChange,
				Example:    fmt.Sprintf("%s - %s = %s (%.2f%%)", fmtMoney(p.TotalValueHoldings), fmtMoney(p.LastWeekTotal), fmtMoney(lastWeekChange), p.LastWeekTotalPct),
			},
			{
				Term:       "cash_balance",
				Label:      "Cash Balance",
				Definition: "Running cash balance from the cash transactions ledger.",
				Value:      cashBalance,
				Example:    fmtMoney(cashBalance),
			},
			{
				Term:       "net_deployed",
				Label:      "Net Deployed",
				Definition: "Net capital deployed into the portfolio (deposits + contributions - withdrawals).",
				Formula:    "total_deposited - total_withdrawn",
				Value:      netDeployed,
				Example:    fmtMoney(netDeployed),
			},
		},
	}
}

// --- Helpers ---

func fmtMoney(v float64) string {
	if v < 0 {
		return fmt.Sprintf("-$%.2f", -v)
	}
	return fmt.Sprintf("$%.2f", v)
}

func fmtCategoryLabel(cat string) string {
	switch cat {
	case "accumulate":
		return "Accumulate"
	case "cash":
		return "Cash"
	case "term_deposit":
		return "Term Deposit"
	case "offset":
		return "Offset"
	default:
		return cat
	}
}

func aboveBelow(above bool) string {
	if above {
		return "above"
	}
	return "below"
}

func topHoldings(holdings []models.Holding, n int) []models.Holding {
	if len(holdings) == 0 {
		return nil
	}
	sorted := make([]models.Holding, len(holdings))
	copy(sorted, holdings)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Weight > sorted[j].Weight
	})
	if n > len(sorted) {
		n = len(sorted)
	}
	return sorted[:n]
}

func topVal(holdings []models.Holding, fn func(models.Holding) float64) float64 {
	if len(holdings) == 0 {
		return 0
	}
	return fn(holdings[0])
}

func fmtHoldingCalc(holdings []models.Holding, _ string, fn func(models.Holding) string) string {
	if len(holdings) == 0 {
		return ""
	}
	result := ""
	for i, h := range holdings {
		if i > 0 {
			result += " | "
		}
		result += fmt.Sprintf("%s: %s", h.Ticker, fn(h))
	}
	return result
}

func fmtExternalBalances(balances []models.ExternalBalance, total float64) string {
	if len(balances) == 0 {
		return fmtMoney(total)
	}
	result := ""
	for i, eb := range balances {
		if i > 0 {
			result += " + "
		}
		result += fmt.Sprintf("%s: %s", eb.Label, fmtMoney(eb.Value))
	}
	result += fmt.Sprintf(" = %s", fmtMoney(total))
	return result
}
