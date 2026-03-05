package server

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/bobmcallan/vire/internal/models"
)

// handleGlossaryRoot handles GET /api/glossary?portfolio_name=X — top-level glossary endpoint.
func (s *Server) handleGlossaryRoot(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodGet) {
		return
	}
	name := s.resolvePortfolio(r.Context(), r.URL.Query().Get("portfolio_name"))
	s.handleGlossary(w, r, name)
}

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

	resp.Categories = append(resp.Categories, buildGrowthCategory(p))

	return resp
}

func buildValuationCategory(p *models.Portfolio) models.GlossaryCategory {
	return models.GlossaryCategory{
		Name: "Portfolio Valuation",
		Terms: []models.GlossaryTerm{
			{
				Term:       "equity_holdings_value",
				Label:      "Equity Holdings Value",
				Definition: "Total market value of all equity holdings, FX-adjusted to portfolio base currency.",
				Formula:    "sum(holding_value_market) for all holdings",
				Value:      p.EquityHoldingsValue,
				Example:    fmtMoney(p.EquityHoldingsValue),
			},
			{
				Term:       "portfolio_value",
				Label:      "Portfolio Value",
				Definition: "Portfolio value: equity holdings plus available (uninvested) cash.",
				Formula:    "equity_holdings_value + capital_available",
				Value:      p.PortfolioValue,
				Example:    fmt.Sprintf("%s + %s = %s", fmtMoney(p.EquityHoldingsValue), fmtMoney(p.CapitalAvailable), fmtMoney(p.PortfolioValue)),
			},
			{
				Term:       "equity_holdings_cost",
				Label:      "Equity Holdings Cost",
				Definition: "Net capital deployed in equities (buy costs minus sell proceeds, FX-adjusted).",
				Formula:    "sum(gross_invested - gross_proceeds) for all holdings",
				Value:      p.EquityHoldingsCost,
				Example:    fmtMoney(p.EquityHoldingsCost),
			},
			{
				Term:       "equity_holdings_return",
				Label:      "Equity Holdings Return",
				Definition: "Net return on all capital deployed into equities, including realised gains/losses from closed positions.",
				Formula:    "equity_holdings_value - equity_holdings_cost",
				Value:      p.EquityHoldingsReturn,
				Example:    fmt.Sprintf("%s - %s = %s", fmtMoney(p.EquityHoldingsValue), fmtMoney(p.EquityHoldingsCost), fmtMoney(p.EquityHoldingsReturn)),
			},
			{
				Term:       "equity_holdings_return_pct",
				Label:      "Equity Holdings Return %",
				Definition: "Portfolio equity return as a percentage of cost.",
				Formula:    "(equity_holdings_return / equity_holdings_cost) * 100",
				Value:      p.EquityHoldingsReturnPct,
				Example:    fmt.Sprintf("(%s / %s) * 100 = %.2f%%", fmtMoney(p.EquityHoldingsReturn), fmtMoney(p.EquityHoldingsCost), p.EquityHoldingsReturnPct),
			},
			{
				Term:       "equity_holdings_realized",
				Label:      "Equity Holdings Realized",
				Definition: "Cumulative profit or loss from sold portions of all holdings.",
				Formula:    "sum(realized_return) for all holdings",
				Value:      p.EquityHoldingsRealized,
				Example:    fmtMoney(p.EquityHoldingsRealized),
			},
			{
				Term:       "equity_holdings_unrealized",
				Label:      "Equity Holdings Unrealized",
				Definition: "Paper profit or loss on remaining open positions across all holdings.",
				Formula:    "sum(unrealized_return) for all holdings",
				Value:      p.EquityHoldingsUnrealized,
				Example:    fmtMoney(p.EquityHoldingsUnrealized),
			},
			{
				Term:       "capital_gross",
				Label:      "Capital Gross",
				Definition: "Sum of all cash account balances (trading, accumulate, term deposits, offset).",
				Value:      p.CapitalGross,
				Example:    fmtMoney(p.CapitalGross),
			},
			{
				Term:       "capital_available",
				Label:      "Capital Available",
				Definition: "Uninvested cash: gross cash balance minus capital locked in equities.",
				Formula:    "capital_gross - equity_holdings_cost",
				Value:      p.CapitalAvailable,
				Example:    fmt.Sprintf("%s - %s = %s", fmtMoney(p.CapitalGross), fmtMoney(p.EquityHoldingsCost), fmtMoney(p.CapitalAvailable)),
			},
			{
				Term:       "portfolio_return",
				Label:      "Portfolio Return",
				Definition: "Overall portfolio gain: portfolio value minus net capital deployed.",
				Formula:    "portfolio_value - capital_contributions_net",
				Value:      p.PortfolioReturn,
				Example:    fmtMoney(p.PortfolioReturn),
			},
			{
				Term:       "portfolio_return_pct",
				Label:      "Portfolio Return %",
				Definition: "Overall portfolio gain as a percentage of net capital deployed.",
				Formula:    "(portfolio_return / capital_contributions_net) × 100",
				Value:      p.PortfolioReturnPct,
				Example:    fmt.Sprintf("%.2f%%", p.PortfolioReturnPct),
			},
			{
				Term:       "currency",
				Label:      "Currency",
				Definition: "Portfolio base currency. All holding values are converted to this currency.",
				Value:      p.Currency,
				Example:    p.Currency,
			},
			{
				Term:       "fx_rate",
				Label:      "FX Rate",
				Definition: "AUDUSD exchange rate at last sync. Used to convert USD-denominated holdings to AUD.",
				Value:      p.FXRate,
				Example:    fmt.Sprintf("%.4f", p.FXRate),
			},
			{
				Term:       "income_dividends_forecast",
				Label:      "Income Dividends Forecast",
				Definition: "Forecasted future dividends: Navexa total dividends minus forecast amounts for holdings with confirmed ledger payments.",
				Formula:    "navexa_total_dividends - paid_forecast",
				Value:      p.IncomeDividendsForecast,
				Example:    fmtMoney(p.IncomeDividendsForecast),
			},
			{
				Term:       "income_dividends_received",
				Label:      "Income Dividends Received",
				Definition: "Confirmed dividend income recorded in the cash flow ledger. Distinct from Navexa-calculated dividend returns.",
				Formula:    "sum(dividend category) from cash transactions",
				Value:      p.IncomeDividendsReceived,
				Example:    fmtMoney(p.IncomeDividendsReceived),
			},
			{
				Term:       "calculation_method",
				Label:      "Calculation Method",
				Definition: "Methodology for return calculations. Average cost divides total cost by total units to determine per-unit cost basis.",
				Value:      p.CalculationMethod,
				Example:    p.CalculationMethod,
			},
			{
				Term:       "data_version",
				Label:      "Data Version",
				Definition: "Schema version at save time. A version mismatch between stored data and server triggers automatic re-sync on next portfolio load.",
				Value:      p.DataVersion,
				Example:    p.DataVersion,
			},
		},
	}
}

func buildHoldingCategory(p *models.Portfolio) models.GlossaryCategory {
	// Sort holdings by weight descending and take top 3 for examples
	top := topHoldings(p.Holdings, 3)

	terms := []models.GlossaryTerm{
		{
			Term:       "holding_value_market",
			Label:      "Holding Value (Market)",
			Definition: "Current value of a holding position.",
			Formula:    "units * current_price",
			Value:      topVal(top, func(h models.Holding) float64 { return h.MarketValue }),
			Example: fmtHoldingCalc(top, "holding_value_market", func(h models.Holding) string {
				return fmt.Sprintf("%.2f units * %s = %s", h.Units, fmtMoney(h.CurrentPrice), fmtMoney(h.MarketValue))
			}),
		},
		{
			Term:       "holding_cost_avg",
			Label:      "Holding Cost (Avg)",
			Definition: "Average purchase price per unit, including brokerage fees.",
			Formula:    "total_cost / units",
			Value:      topVal(top, func(h models.Holding) float64 { return h.AvgCost }),
			Example:    fmtHoldingCalc(top, "holding_cost_avg", func(h models.Holding) string { return fmt.Sprintf("%s per unit", fmtMoney(h.AvgCost)) }),
		},
		{
			Term:       "holding_weight_pct",
			Label:      "Holding Weight %",
			Definition: "Holding's proportion of the total portfolio value.",
			Formula:    "(holding_value_market / portfolio_value) * 100",
			Value:      topVal(top, func(h models.Holding) float64 { return h.WeightPct }),
			Example: fmtHoldingCalc(top, "holding_weight_pct", func(h models.Holding) string {
				return fmt.Sprintf("(%s / %s) * 100 = %.2f%%", fmtMoney(h.MarketValue), fmtMoney(p.EquityHoldingsValue), h.WeightPct)
			}),
		},
		{
			Term:       "holding_return_net",
			Label:      "Holding Return (Net)",
			Definition: "Unrealised gain or loss on a single holding.",
			Formula:    "holding_value_market - cost_basis",
			Value:      topVal(top, func(h models.Holding) float64 { return h.ReturnNet }),
			Example: fmtHoldingCalc(top, "holding_return_net", func(h models.Holding) string {
				return fmt.Sprintf("%s - %s = %s", fmtMoney(h.MarketValue), fmtMoney(h.CostBasis), fmtMoney(h.ReturnNet))
			}),
		},
		{
			Term:       "holding_return_net_pct",
			Label:      "Holding Return (Net) %",
			Definition: "Holding return as a percentage of total invested capital.",
			Formula:    "(holding_return_net / total_invested) * 100",
			Value:      topVal(top, func(h models.Holding) float64 { return h.ReturnNetPct }),
			Example:    fmtHoldingCalc(top, "holding_return_net_pct", func(h models.Holding) string { return fmt.Sprintf("%.2f%%", h.ReturnNetPct) }),
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
				Term:       "capital_contributions_gross",
				Label:      "Capital Contributions (Gross)",
				Definition: "Sum of all credits into the portfolio (deposits, contributions, transfers in, dividends).",
				Value:      cp.ContributionsGross,
				Example:    fmtMoney(cp.ContributionsGross),
			},
			{
				Term:       "capital_withdrawals_gross",
				Label:      "Capital Withdrawals (Gross)",
				Definition: "Sum of all debits from the portfolio (withdrawals, fees, transfers out).",
				Formula:    "sum(debits)",
				Value:      cp.WithdrawalsGross,
				Example:    fmtMoney(cp.WithdrawalsGross),
			},
			{
				Term:       "capital_contributions_net",
				Label:      "Capital Contributions (Net)",
				Definition: "Net capital currently deployed in the portfolio.",
				Formula:    "capital_contributions_gross - capital_withdrawals_gross",
				Value:      cp.ContributionsNet,
				Example:    fmt.Sprintf("%s - %s = %s", fmtMoney(cp.ContributionsGross), fmtMoney(cp.WithdrawalsGross), fmtMoney(cp.ContributionsNet)),
			},
			{
				Term:       "capital_return_simple_pct",
				Label:      "Capital Return (Simple) %",
				Definition: "Simple return on deployed capital (not time-weighted).",
				Formula:    "(portfolio_value - capital_contributions_net) / capital_contributions_net * 100",
				Value:      cp.ReturnSimplePct,
				Example:    fmt.Sprintf("(%s - %s) / %s * 100 = %.2f%%", fmtMoney(cp.CurrentValue), fmtMoney(cp.ContributionsNet), fmtMoney(cp.ContributionsNet), cp.ReturnSimplePct),
			},
			{
				Term:       "capital_return_xirr_pct",
				Label:      "Capital Return (XIRR) %",
				Definition: "Time-weighted annualized return using the XIRR method. Accounts for the timing and size of each cash flow.",
				Formula:    "XIRR(cash_flows, current_value)",
				Value:      cp.ReturnXirrPct,
				Example:    fmt.Sprintf("%.2f%% annualized", cp.ReturnXirrPct),
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

func buildGrowthCategory(p *models.Portfolio) models.GlossaryCategory {
	terms := []models.GlossaryTerm{
		{
			Term:       "changes",
			Label:      "Period Changes",
			Definition: "D/W/M change tracking for four metrics: portfolio_value, equity_holdings_value, capital_gross, income_dividends. Each metric is a MetricChange object with current, previous, raw_change, pct_change, and has_previous fields.",
			Formula:    "changes.{yesterday|week|month}.{portfolio_value|equity_holdings_value|capital_gross|income_dividends}",
		},
		{
			Term:       "changes.portfolio_value",
			Label:      "Portfolio Value Change",
			Definition: "D/W/M change in total portfolio value (equity + cash). Use for overall portfolio movement.",
			Formula:    "portfolio_value_today - portfolio_value_at_reference_date",
		},
		{
			Term:       "changes.equity_holdings_value",
			Label:      "Equity Holdings Value Change",
			Definition: "D/W/M change in market value of equity holdings. Reflects price movement only — unaffected by cash flows or trade settlements. Use for market movement display.",
			Formula:    "equity_holdings_value_today - equity_holdings_value_at_reference_date",
		},
		{
			Term:       "changes.capital_gross",
			Label:      "Cash Balance Change",
			Definition: "D/W/M change in gross cash balance across all accounts.",
			Formula:    "capital_gross_today - capital_gross_at_reference_date",
		},
		{
			Term:       "changes.income_dividends",
			Label:      "Dividend Change",
			Definition: "D/W/M change in cumulative confirmed dividends from cash flow ledger.",
			Formula:    "income_dividends_received_today - income_dividends_cumulative_at_reference_date",
		},
	}

	// Add live examples if changes data is available
	if p.Changes != nil {
		yd := p.Changes.Yesterday
		if yd.PortfolioValue.HasPrevious {
			terms[1].Value = yd.PortfolioValue.RawChange
			terms[1].Example = fmt.Sprintf("D: %s (%.2f%%) | W: %s (%.2f%%) | M: %s (%.2f%%)",
				fmtMoney(p.Changes.Yesterday.PortfolioValue.RawChange), p.Changes.Yesterday.PortfolioValue.PctChange,
				fmtMoney(p.Changes.Week.PortfolioValue.RawChange), p.Changes.Week.PortfolioValue.PctChange,
				fmtMoney(p.Changes.Month.PortfolioValue.RawChange), p.Changes.Month.PortfolioValue.PctChange)
		}
		if yd.EquityHoldingsValue.HasPrevious {
			terms[2].Value = yd.EquityHoldingsValue.RawChange
			terms[2].Example = fmt.Sprintf("D: %s (%.2f%%) | W: %s (%.2f%%) | M: %s (%.2f%%)",
				fmtMoney(p.Changes.Yesterday.EquityHoldingsValue.RawChange), p.Changes.Yesterday.EquityHoldingsValue.PctChange,
				fmtMoney(p.Changes.Week.EquityHoldingsValue.RawChange), p.Changes.Week.EquityHoldingsValue.PctChange,
				fmtMoney(p.Changes.Month.EquityHoldingsValue.RawChange), p.Changes.Month.EquityHoldingsValue.PctChange)
		}
		if yd.CapitalGross.HasPrevious {
			terms[3].Value = yd.CapitalGross.RawChange
			terms[3].Example = fmt.Sprintf("D: %s | W: %s | M: %s",
				fmtMoney(p.Changes.Yesterday.CapitalGross.RawChange),
				fmtMoney(p.Changes.Week.CapitalGross.RawChange),
				fmtMoney(p.Changes.Month.CapitalGross.RawChange))
		}
		if yd.IncomeDividends.HasPrevious {
			terms[4].Value = yd.IncomeDividends.RawChange
			terms[4].Example = fmt.Sprintf("D: %s | W: %s | M: %s",
				fmtMoney(p.Changes.Yesterday.IncomeDividends.RawChange),
				fmtMoney(p.Changes.Week.IncomeDividends.RawChange),
				fmtMoney(p.Changes.Month.IncomeDividends.RawChange))
		}
	}

	return models.GlossaryCategory{
		Name:  "Growth Metrics",
		Terms: terms,
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
		return sorted[i].WeightPct > sorted[j].WeightPct
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
