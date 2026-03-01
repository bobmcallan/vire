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
				Term:       "portfolio_value",
				Label:      "Portfolio Value",
				Definition: "Portfolio value: equity holdings plus available (uninvested) cash.",
				Formula:    "equity_value + net_cash_balance",
				Value:      p.PortfolioValue,
				Example:    fmt.Sprintf("%s + %s = %s", fmtMoney(p.EquityValue), fmtMoney(p.NetCashBalance), fmtMoney(p.PortfolioValue)),
			},
			{
				Term:       "net_equity_cost",
				Label:      "Net Equity Cost",
				Definition: "Net capital deployed in equities (buy costs minus sell proceeds, FX-adjusted).",
				Formula:    "sum(gross_invested - gross_proceeds) for all holdings",
				Value:      p.NetEquityCost,
				Example:    fmtMoney(p.NetEquityCost),
			},
			{
				Term:       "net_equity_return",
				Label:      "Net Equity Return",
				Definition: "Unrealised gain or loss across the portfolio.",
				Formula:    "equity_value - net_equity_cost",
				Value:      p.NetEquityReturn,
				Example:    fmt.Sprintf("%s - %s = %s", fmtMoney(p.EquityValue), fmtMoney(p.NetEquityCost), fmtMoney(p.NetEquityReturn)),
			},
			{
				Term:       "net_equity_return_pct",
				Label:      "Net Equity Return %",
				Definition: "Portfolio equity return as a percentage of cost.",
				Formula:    "(net_equity_return / net_equity_cost) * 100",
				Value:      p.NetEquityReturnPct,
				Example:    fmt.Sprintf("(%s / %s) * 100 = %.2f%%", fmtMoney(p.NetEquityReturn), fmtMoney(p.NetEquityCost), p.NetEquityReturnPct),
			},
			{
				Term:       "gross_cash_balance",
				Label:      "Gross Cash Balance",
				Definition: "Sum of all cash account balances (trading, accumulate, term deposits, offset).",
				Value:      p.GrossCashBalance,
				Example:    fmtMoney(p.GrossCashBalance),
			},
			{
				Term:       "net_cash_balance",
				Label:      "Net Cash Balance",
				Definition: "Uninvested cash: gross cash balance minus capital locked in equities.",
				Formula:    "gross_cash_balance - net_equity_cost",
				Value:      p.NetCashBalance,
				Example:    fmt.Sprintf("%s - %s = %s", fmtMoney(p.GrossCashBalance), fmtMoney(p.NetEquityCost), fmtMoney(p.NetCashBalance)),
			},
			{
				Term:       "net_capital_return",
				Label:      "Net Capital Return",
				Definition: "Overall portfolio gain: portfolio value minus net capital deployed.",
				Formula:    "portfolio_value - net_capital_deployed",
				Value:      p.NetCapitalReturn,
				Example:    fmtMoney(p.NetCapitalReturn),
			},
			{
				Term:       "net_capital_return_pct",
				Label:      "Net Capital Return %",
				Definition: "Overall portfolio gain as a percentage of net capital deployed.",
				Formula:    "(net_capital_return / net_capital_deployed) × 100",
				Value:      p.NetCapitalReturnPct,
				Example:    fmt.Sprintf("%.2f%%", p.NetCapitalReturnPct),
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
			Term:       "portfolio_weight_pct",
			Label:      "Portfolio Weight %",
			Definition: "Holding's proportion of the total portfolio value.",
			Formula:    "(market_value / portfolio_value) * 100",
			Value:      topVal(top, func(h models.Holding) float64 { return h.PortfolioWeightPct }),
			Example: fmtHoldingCalc(top, "portfolio_weight_pct", func(h models.Holding) string {
				return fmt.Sprintf("(%s / %s) * 100 = %.2f%%", fmtMoney(h.MarketValue), fmtMoney(p.EquityValue), h.PortfolioWeightPct)
			}),
		},
		{
			Term:       "net_return",
			Label:      "Net Return",
			Definition: "Unrealised gain or loss on a single holding.",
			Formula:    "market_value - cost_basis",
			Value:      topVal(top, func(h models.Holding) float64 { return h.NetReturn }),
			Example: fmtHoldingCalc(top, "net_return", func(h models.Holding) string {
				return fmt.Sprintf("%s - %s = %s", fmtMoney(h.MarketValue), fmtMoney(h.CostBasis), fmtMoney(h.NetReturn))
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
				Term:       "gross_capital_deposited",
				Label:      "Gross Capital Deposited",
				Definition: "Sum of all credits into the portfolio (deposits, contributions, transfers in, dividends).",
				Value:      cp.GrossCapitalDeposited,
				Example:    fmtMoney(cp.GrossCapitalDeposited),
			},
			{
				Term:       "gross_capital_withdrawn",
				Label:      "Gross Capital Withdrawn",
				Definition: "Sum of all debits from the portfolio (withdrawals, fees, transfers out).",
				Formula:    "sum(debits)",
				Value:      cp.GrossCapitalWithdrawn,
				Example:    fmtMoney(cp.GrossCapitalWithdrawn),
			},
			{
				Term:       "net_capital_deployed",
				Label:      "Net Capital Deployed",
				Definition: "Net capital currently deployed in the portfolio.",
				Formula:    "gross_capital_deposited - gross_capital_withdrawn",
				Value:      cp.NetCapitalDeployed,
				Example:    fmt.Sprintf("%s - %s = %s", fmtMoney(cp.GrossCapitalDeposited), fmtMoney(cp.GrossCapitalWithdrawn), fmtMoney(cp.NetCapitalDeployed)),
			},
			{
				Term:       "simple_capital_return_pct",
				Label:      "Simple Capital Return %",
				Definition: "Simple return on deployed capital (not time-weighted).",
				Formula:    "(equity_value - net_capital_deployed) / net_capital_deployed * 100",
				Value:      cp.SimpleCapitalReturnPct,
				Example:    fmt.Sprintf("(%s - %s) / %s * 100 = %.2f%%", fmtMoney(cp.EquityValue), fmtMoney(cp.NetCapitalDeployed), fmtMoney(cp.NetCapitalDeployed), cp.SimpleCapitalReturnPct),
			},
			{
				Term:       "annualized_capital_return_pct",
				Label:      "Annualized Capital Return % (XIRR)",
				Definition: "Time-weighted annualized return using the XIRR method. Accounts for the timing and size of each cash flow.",
				Formula:    "XIRR(cash_flows, current_value)",
				Value:      cp.AnnualizedCapitalReturnPct,
				Example:    fmt.Sprintf("%.2f%% annualized", cp.AnnualizedCapitalReturnPct),
			},
		},
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
				Example:    fmt.Sprintf("%s (current value %s is %s)", fmtMoney(ind.EMA20), fmtMoney(ind.PortfolioValue), aboveBelow(ind.AboveEMA20)),
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
	yesterdayChange := p.EquityValue - p.PortfolioYesterdayValue
	lastWeekChange := p.EquityValue - p.PortfolioLastWeekValue

	grossCashBalance := 0.0
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
				Example:    fmt.Sprintf("%s - %s = %s (%.2f%%)", fmtMoney(p.EquityValue), fmtMoney(p.PortfolioYesterdayValue), fmtMoney(yesterdayChange), p.PortfolioYesterdayChangePct),
			},
			{
				Term:       "last_week_change",
				Label:      "Last Week Change",
				Definition: "Value change since last week's close.",
				Formula:    "current_value - last_week_close",
				Value:      lastWeekChange,
				Example:    fmt.Sprintf("%s - %s = %s (%.2f%%)", fmtMoney(p.EquityValue), fmtMoney(p.PortfolioLastWeekValue), fmtMoney(lastWeekChange), p.PortfolioLastWeekChangePct),
			},
			{
				Term:       "gross_cash_balance",
				Label:      "Gross Cash Balance",
				Definition: "Running cash balance from the cash transactions ledger.",
				Value:      grossCashBalance,
				Example:    fmtMoney(grossCashBalance),
			},
			{
				Term:       "net_capital_deployed",
				Label:      "Net Capital Deployed",
				Definition: "Net capital deployed into the portfolio (deposits + contributions - withdrawals).",
				Formula:    "gross_capital_deposited - gross_capital_withdrawn",
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
		return sorted[i].PortfolioWeightPct > sorted[j].PortfolioWeightPct
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
