package report

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
)

// formatReportSummary generates the summary markdown (no ETF details or stock fundamentals)
func formatReportSummary(review *models.PortfolioReview) string {
	var sb strings.Builder

	// Header
	sb.WriteString(fmt.Sprintf("# Portfolio Review: %s\n\n", review.PortfolioName))
	sb.WriteString(fmt.Sprintf("**Date:** %s\n", review.ReviewDate.Format("2006-01-02 15:04")))
	sb.WriteString(fmt.Sprintf("**Total Value:** %s\n", common.FormatMoney(review.TotalValue)))
	sb.WriteString(fmt.Sprintf("**Total Cost:** %s\n", common.FormatMoney(review.TotalCost)))
	sb.WriteString(fmt.Sprintf("**Total Gain:** %s (%s)\n", common.FormatSignedMoney(review.TotalGain), common.FormatSignedPct(review.TotalGainPct)))
	sb.WriteString(fmt.Sprintf("**Day Change:** %s (%s)\n\n", common.FormatSignedMoney(review.DayChange), common.FormatSignedPct(review.DayChangePct)))

	// Separate active holdings from closed, then split active into stocks/ETFs
	var stocks, etfs, closed []models.HoldingReview
	for _, hr := range review.HoldingReviews {
		if hr.ActionRequired == "CLOSED" {
			closed = append(closed, hr)
		} else if common.IsETF(&hr) {
			etfs = append(etfs, hr)
		} else {
			stocks = append(stocks, hr)
		}
	}
	sort.Slice(stocks, func(i, j int) bool { return stocks[i].Holding.Ticker < stocks[j].Holding.Ticker })
	sort.Slice(etfs, func(i, j int) bool { return etfs[i].Holding.Ticker < etfs[j].Holding.Ticker })
	sort.Slice(closed, func(i, j int) bool { return closed[i].Holding.Ticker < closed[j].Holding.Ticker })

	sb.WriteString("## Holdings\n\n")

	// Stocks table
	if len(stocks) > 0 {
		sb.WriteString("### Stocks\n\n")
		sb.WriteString("| Symbol | Weight | Avg Buy | Qty | Price | Value | Capital Gain % | Total Return | Total Return % | TWRR % | Action |\n")
		sb.WriteString("|--------|--------|---------|-----|-------|-------|----------------|--------------|----------------|--------|--------|\n")

		stocksTotal := 0.0
		stocksGain := 0.0
		for _, hr := range stocks {
			h := hr.Holding
			stocksTotal += h.MarketValue
			stocksGain += h.TotalReturnValue
			sb.WriteString(fmt.Sprintf("| %s | %.1f%% | %s | %.0f | %s | %s | %s | %s | %s | %s | %s |\n",
				h.Ticker, h.Weight, common.FormatMoney(h.AvgCost), h.Units,
				common.FormatMoney(h.CurrentPrice), common.FormatMoney(h.MarketValue),
				common.FormatSignedPct(h.CapitalGainPct),
				common.FormatSignedMoney(h.TotalReturnValue), common.FormatSignedPct(h.TotalReturnPct), common.FormatSignedPct(h.TotalReturnPctTWRR),
				formatAction(hr.ActionRequired),
			))
		}
		stocksGainPct := 0.0
		if stocksTotal-stocksGain > 0 {
			stocksGainPct = (stocksGain / (stocksTotal - stocksGain)) * 100
		}
		sb.WriteString(fmt.Sprintf("| **Stocks Total** | | | | | **%s** | | **%s** | **%s** | | |\n\n",
			common.FormatMoney(stocksTotal), common.FormatSignedMoney(stocksGain), common.FormatSignedPct(stocksGainPct)))
	}

	// ETFs table
	if len(etfs) > 0 {
		sb.WriteString("### ETFs\n\n")
		sb.WriteString("| Symbol | Weight | Avg Buy | Qty | Price | Value | Capital Gain % | Total Return | Total Return % | TWRR % | Action |\n")
		sb.WriteString("|--------|--------|---------|-----|-------|-------|----------------|--------------|----------------|--------|--------|\n")

		etfsTotal := 0.0
		etfsGain := 0.0
		for _, hr := range etfs {
			h := hr.Holding
			etfsTotal += h.MarketValue
			etfsGain += h.TotalReturnValue
			sb.WriteString(fmt.Sprintf("| %s | %.1f%% | %s | %.0f | %s | %s | %s | %s | %s | %s | %s |\n",
				h.Ticker, h.Weight, common.FormatMoney(h.AvgCost), h.Units,
				common.FormatMoney(h.CurrentPrice), common.FormatMoney(h.MarketValue),
				common.FormatSignedPct(h.CapitalGainPct),
				common.FormatSignedMoney(h.TotalReturnValue), common.FormatSignedPct(h.TotalReturnPct), common.FormatSignedPct(h.TotalReturnPctTWRR),
				formatAction(hr.ActionRequired),
			))
		}
		etfsGainPct := 0.0
		if etfsTotal-etfsGain > 0 {
			etfsGainPct = (etfsGain / (etfsTotal - etfsGain)) * 100
		}
		sb.WriteString(fmt.Sprintf("| **ETFs Total** | | | | | **%s** | | **%s** | **%s** | | |\n\n",
			common.FormatMoney(etfsTotal), common.FormatSignedMoney(etfsGain), common.FormatSignedPct(etfsGainPct)))
	}

	// Closed Positions table (same format as stocks/ETFs)
	if len(closed) > 0 {
		sb.WriteString("### Closed Positions\n\n")
		sb.WriteString("| Symbol | Weight | Avg Buy | Qty | Price | Value | Capital Gain % | Total Return | Total Return % | TWRR % | Action |\n")
		sb.WriteString("|--------|--------|---------|-----|-------|-------|----------------|--------------|----------------|--------|--------|\n")

		closedGain := 0.0
		for _, hr := range closed {
			h := hr.Holding
			closedGain += h.TotalReturnValue
			sb.WriteString(fmt.Sprintf("| %s | %.1f%% | %s | %.0f | %s | %s | %s | %s | %s | %s | %s |\n",
				h.Ticker, h.Weight, common.FormatMoney(h.AvgCost), h.Units,
				common.FormatMoney(h.CurrentPrice), common.FormatMoney(h.MarketValue),
				common.FormatSignedPct(h.CapitalGainPct),
				common.FormatSignedMoney(h.TotalReturnValue), common.FormatSignedPct(h.TotalReturnPct), common.FormatSignedPct(h.TotalReturnPctTWRR),
				formatAction(hr.ActionRequired),
			))
		}
		closedCost := 0.0
		for _, hr := range closed {
			closedCost += hr.Holding.TotalCost
		}
		closedGainPct := 0.0
		if closedCost > 0 {
			closedGainPct = (closedGain / closedCost) * 100
		}
		sb.WriteString(fmt.Sprintf("| **Closed Total** | | | | | | | **%s** | **%s** | | |\n\n",
			common.FormatSignedMoney(closedGain), common.FormatSignedPct(closedGainPct)))
	}

	// Grand total
	sb.WriteString(fmt.Sprintf("**Portfolio Total:** %s | **Total Return:** %s (%s)\n\n",
		common.FormatMoney(review.TotalValue), common.FormatSignedMoney(review.TotalGain), common.FormatSignedPct(review.TotalGainPct)))

	// Portfolio Balance
	if review.PortfolioBalance != nil {
		sb.WriteString("## Portfolio Balance\n\n")
		pb := review.PortfolioBalance

		sb.WriteString("### Sector Allocation\n\n")
		sb.WriteString("| Sector | Weight | Holdings |\n")
		sb.WriteString("|--------|--------|----------|\n")
		for _, sa := range pb.SectorAllocations {
			sb.WriteString(fmt.Sprintf("| %s | %.1f%% | %s |\n", sa.Sector, sa.Weight, strings.Join(sa.Holdings, ", ")))
		}
		sb.WriteString("\n")

		sb.WriteString("### Portfolio Style\n\n")
		sb.WriteString("| Style | Weight |\n")
		sb.WriteString("|-------|--------|\n")
		sb.WriteString(fmt.Sprintf("| Defensive | %.1f%% |\n", pb.DefensiveWeight))
		sb.WriteString(fmt.Sprintf("| Growth | %.1f%% |\n", pb.GrowthWeight))
		sb.WriteString(fmt.Sprintf("| Income (>4%% yield) | %.1f%% |\n", pb.IncomeWeight))
		sb.WriteString("\n")

		sb.WriteString(fmt.Sprintf("**Concentration Risk:** %s\n\n", pb.ConcentrationRisk))
		sb.WriteString(fmt.Sprintf("**Analysis:** %s\n\n", pb.DiversificationNote))
	}

	// AI Summary
	if review.Summary != "" {
		sb.WriteString("## Summary\n\n")
		sb.WriteString(review.Summary)
		sb.WriteString("\n\n")
	}

	// Alerts & Observations
	if len(review.Alerts) > 0 || len(review.Recommendations) > 0 {
		sb.WriteString("## Alerts & Observations\n\n")

		if len(review.Alerts) > 0 {
			sb.WriteString("### Alerts\n\n")
			for _, alert := range review.Alerts {
				icon := "info"
				if alert.Severity == "high" {
					icon = "HIGH"
				} else if alert.Severity == "medium" {
					icon = "MEDIUM"
				}
				sb.WriteString(fmt.Sprintf("- [%s] **%s**: %s\n", icon, alert.Ticker, alert.Message))
			}
			sb.WriteString("\n")
		}

		if len(review.Recommendations) > 0 {
			sb.WriteString("### Observations\n\n")
			for i, rec := range review.Recommendations {
				sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, rec))
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// formatETFReport generates per-ticker markdown for an ETF holding
func formatETFReport(hr *models.HoldingReview, review *models.PortfolioReview) string {
	var sb strings.Builder
	h := hr.Holding
	f := hr.Fundamentals

	sb.WriteString(fmt.Sprintf("# %s - %s\n\n", h.Ticker, h.Name))
	sb.WriteString(fmt.Sprintf("**Action:** %s | **Reason:** %s\n\n", hr.ActionRequired, hr.ActionReason))

	// About
	if f != nil && f.Description != "" && f.Description != "NA" {
		sb.WriteString("## About\n\n")
		sb.WriteString(f.Description + "\n\n")
	}

	// Position
	sb.WriteString("## Position\n\n")
	sb.WriteString("| Metric | Value |\n")
	sb.WriteString("|--------|-------|\n")
	sb.WriteString(fmt.Sprintf("| Weight | %.1f%% |\n", h.Weight))
	sb.WriteString(fmt.Sprintf("| Avg Buy | %s |\n", common.FormatMoney(h.AvgCost)))
	sb.WriteString(fmt.Sprintf("| Quantity | %.0f |\n", h.Units))
	sb.WriteString(fmt.Sprintf("| Price | %s |\n", common.FormatMoney(h.CurrentPrice)))
	sb.WriteString(fmt.Sprintf("| Value | %s |\n", common.FormatMoney(h.MarketValue)))
	sb.WriteString(fmt.Sprintf("| Total Cost | %s |\n", common.FormatMoney(h.TotalCost)))
	sb.WriteString(fmt.Sprintf("| Capital Gain | %s (%s) |\n",
		common.FormatSignedMoney(h.GainLoss), common.FormatSignedPct(h.CapitalGainPct)))
	sb.WriteString(fmt.Sprintf("| Dividend Return | %s |\n", common.FormatSignedMoney(h.DividendReturn)))
	sb.WriteString(fmt.Sprintf("| Total Return | %s (%s) |\n",
		common.FormatSignedMoney(h.TotalReturnValue), common.FormatSignedPct(h.TotalReturnPct)))
	sb.WriteString(fmt.Sprintf("| TWRR | %s |\n", common.FormatSignedPct(h.TotalReturnPctTWRR)))
	sb.WriteString("\n")

	// Trade History
	sb.WriteString(formatTradeHistory(h))

	// EODHD Market Analysis — wraps Fund Metrics and Technical Signals
	sb.WriteString("## EODHD Market Analysis\n\n")

	// Fund Metrics
	if f != nil {
		sb.WriteString("### Fund Metrics\n\n")
		sb.WriteString("| Metric | Value |\n")
		sb.WriteString("|--------|-------|\n")
		sb.WriteString(fmt.Sprintf("| Beta | %.2f |\n", f.Beta))
		if f.ExpenseRatio > 0 {
			sb.WriteString(fmt.Sprintf("| Expense Ratio | %.2f%% |\n", f.ExpenseRatio*100))
		}
		if f.ManagementStyle != "" {
			sb.WriteString(fmt.Sprintf("| Management Style | %s |\n", f.ManagementStyle))
		}
		sb.WriteString("\n")
	}

	// Top Holdings
	if f != nil && len(f.TopHoldings) > 0 {
		sb.WriteString("### Top Holdings\n\n")
		sb.WriteString("| Holding | Weight |\n")
		sb.WriteString("|---------|--------|\n")
		for _, th := range f.TopHoldings {
			name := th.Name
			if name == "" {
				name = th.Ticker
			}
			sb.WriteString(fmt.Sprintf("| %s | %.2f%% |\n", name, th.Weight))
		}
		sb.WriteString("\n")
	}

	// Sector Breakdown
	if f != nil && len(f.SectorWeights) > 0 {
		sb.WriteString("### Sector Breakdown\n\n")
		sb.WriteString("| Sector | Weight |\n")
		sb.WriteString("|--------|--------|\n")
		for _, sw := range f.SectorWeights {
			sb.WriteString(fmt.Sprintf("| %s | %.2f%% |\n", sw.Sector, sw.Weight))
		}
		sb.WriteString("\n")
	}

	// Country Exposure
	if f != nil && len(f.CountryWeights) > 0 {
		sb.WriteString("### Country Exposure\n\n")
		sb.WriteString("| Country | Weight |\n")
		sb.WriteString("|---------|--------|\n")
		for _, cw := range f.CountryWeights {
			sb.WriteString(fmt.Sprintf("| %s | %.2f%% |\n", cw.Country, cw.Weight))
		}
		sb.WriteString("\n")
	}

	// Technical Signals
	sb.WriteString(formatSignalsTable(hr.Signals))

	// News Intelligence
	sb.WriteString(formatNewsIntelligence(hr.NewsIntelligence))

	// Filings Intelligence (deprecated — kept for backwards compatibility with older reports)
	sb.WriteString(formatFilingsIntelligence(hr.FilingsIntelligence))

	// Company Releases (per-filing summaries)
	if len(hr.FilingSummaries) > 0 {
		sb.WriteString(formatCompanyReleases(hr.FilingSummaries))
	}

	// Company Timeline
	if hr.Timeline != nil {
		sb.WriteString(formatCompanyTimeline(hr.Timeline))
	}

	// Risk Flags
	sb.WriteString(formatRiskFlags(hr, review))

	return sb.String()
}

// formatStockReport generates per-ticker markdown for a stock holding
func formatStockReport(hr *models.HoldingReview, review *models.PortfolioReview) string {
	var sb strings.Builder
	h := hr.Holding
	f := hr.Fundamentals

	sb.WriteString(fmt.Sprintf("# %s - %s\n\n", h.Ticker, h.Name))
	sb.WriteString(fmt.Sprintf("**Action:** %s | **Reason:** %s\n\n", hr.ActionRequired, hr.ActionReason))

	if f != nil && (f.Sector != "" || f.Industry != "") {
		sb.WriteString(fmt.Sprintf("**Sector:** %s | **Industry:** %s\n\n", f.Sector, f.Industry))
	}

	// About
	if f != nil && f.Description != "" && f.Description != "NA" {
		sb.WriteString("## About\n\n")
		sb.WriteString(f.Description + "\n\n")
	}

	// Position
	sb.WriteString("## Position\n\n")
	sb.WriteString("| Metric | Value |\n")
	sb.WriteString("|--------|-------|\n")
	sb.WriteString(fmt.Sprintf("| Weight | %.1f%% |\n", h.Weight))
	sb.WriteString(fmt.Sprintf("| Avg Buy | %s |\n", common.FormatMoney(h.AvgCost)))
	sb.WriteString(fmt.Sprintf("| Quantity | %.0f |\n", h.Units))
	sb.WriteString(fmt.Sprintf("| Price | %s |\n", common.FormatMoney(h.CurrentPrice)))
	sb.WriteString(fmt.Sprintf("| Value | %s |\n", common.FormatMoney(h.MarketValue)))
	sb.WriteString(fmt.Sprintf("| Total Cost | %s |\n", common.FormatMoney(h.TotalCost)))
	sb.WriteString(fmt.Sprintf("| Capital Gain | %s (%s) |\n",
		common.FormatSignedMoney(h.GainLoss), common.FormatSignedPct(h.CapitalGainPct)))
	sb.WriteString(fmt.Sprintf("| Dividend Return | %s |\n", common.FormatSignedMoney(h.DividendReturn)))
	sb.WriteString(fmt.Sprintf("| Total Return | %s (%s) |\n",
		common.FormatSignedMoney(h.TotalReturnValue), common.FormatSignedPct(h.TotalReturnPct)))
	sb.WriteString(fmt.Sprintf("| TWRR | %s |\n", common.FormatSignedPct(h.TotalReturnPctTWRR)))
	sb.WriteString("\n")

	// Trade History
	sb.WriteString(formatTradeHistory(h))

	// EODHD Market Analysis — wraps Fundamentals and Technical Signals
	sb.WriteString("## EODHD Market Analysis\n\n")

	// Fundamentals
	if f != nil {
		sb.WriteString("### Fundamentals\n\n")

		// Valuation
		sb.WriteString("#### Valuation\n\n")
		sb.WriteString("| Metric | Value |\n")
		sb.WriteString("|--------|-------|\n")
		sb.WriteString(fmt.Sprintf("| Market Cap | %s |\n", common.FormatMarketCap(f.MarketCap)))
		if f.PE != 0 {
			sb.WriteString(fmt.Sprintf("| P/E Ratio (Trailing) | %.2f |\n", f.PE))
		}
		if f.ForwardPE != 0 {
			sb.WriteString(fmt.Sprintf("| P/E Ratio (Forward) | %.2f |\n", f.ForwardPE))
		}
		if f.PEGRatio != 0 {
			sb.WriteString(fmt.Sprintf("| PEG Ratio | %.2f |\n", f.PEGRatio))
		}
		if f.PB != 0 {
			sb.WriteString(fmt.Sprintf("| P/B Ratio | %.2f |\n", f.PB))
		}
		sb.WriteString(fmt.Sprintf("| EPS | $%.2f |\n", f.EPS))
		sb.WriteString(fmt.Sprintf("| Dividend Yield | %.2f%% |\n", f.DividendYield*100))
		sb.WriteString(fmt.Sprintf("| Beta | %.2f |\n", f.Beta))
		sb.WriteString("\n")

		// Profitability (only if any non-zero)
		hasProfitability := f.ProfitMargin != 0 || f.OperatingMarginTTM != 0 || f.ReturnOnEquityTTM != 0 || f.ReturnOnAssetsTTM != 0
		if hasProfitability {
			sb.WriteString("#### Profitability\n\n")
			sb.WriteString("| Metric | Value |\n")
			sb.WriteString("|--------|-------|\n")
			if f.ProfitMargin != 0 {
				sb.WriteString(fmt.Sprintf("| Profit Margin | %.2f%% |\n", f.ProfitMargin*100))
			}
			if f.OperatingMarginTTM != 0 {
				sb.WriteString(fmt.Sprintf("| Operating Margin | %.2f%% |\n", f.OperatingMarginTTM*100))
			}
			if f.ReturnOnEquityTTM != 0 {
				sb.WriteString(fmt.Sprintf("| ROE | %.2f%% |\n", f.ReturnOnEquityTTM*100))
			}
			if f.ReturnOnAssetsTTM != 0 {
				sb.WriteString(fmt.Sprintf("| ROA | %.2f%% |\n", f.ReturnOnAssetsTTM*100))
			}
			sb.WriteString("\n")
		}

		// Growth & Scale (only if any non-zero)
		hasGrowth := f.RevenueTTM != 0 || f.EBITDA != 0 || f.GrossProfitTTM != 0 || f.RevGrowthYOY != 0 || f.EarningsGrowthYOY != 0
		if hasGrowth {
			sb.WriteString("#### Growth & Scale\n\n")
			sb.WriteString("| Metric | Value |\n")
			sb.WriteString("|--------|-------|\n")
			if f.RevenueTTM != 0 {
				sb.WriteString(fmt.Sprintf("| Revenue TTM | %s |\n", common.FormatMarketCap(f.RevenueTTM)))
			}
			if f.GrossProfitTTM != 0 {
				sb.WriteString(fmt.Sprintf("| Gross Profit TTM | %s |\n", common.FormatMarketCap(f.GrossProfitTTM)))
			}
			if f.EBITDA != 0 {
				sb.WriteString(fmt.Sprintf("| EBITDA | %s |\n", common.FormatMarketCap(f.EBITDA)))
			}
			if f.RevGrowthYOY != 0 {
				sb.WriteString(fmt.Sprintf("| Revenue Growth (QoQ YoY) | %.2f%% |\n", f.RevGrowthYOY*100))
			}
			if f.EarningsGrowthYOY != 0 {
				sb.WriteString(fmt.Sprintf("| Earnings Growth (QoQ YoY) | %.2f%% |\n", f.EarningsGrowthYOY*100))
			}
			sb.WriteString("\n")
		}

		// Estimates (only if any non-zero)
		hasEstimates := f.EPSEstimateCurrent != 0 || f.EPSEstimateNext != 0 || f.MostRecentQuarter != ""
		if hasEstimates {
			sb.WriteString("#### Estimates\n\n")
			sb.WriteString("| Metric | Value |\n")
			sb.WriteString("|--------|-------|\n")
			if f.EPSEstimateCurrent != 0 {
				sb.WriteString(fmt.Sprintf("| EPS Estimate (Current Year) | $%.2f |\n", f.EPSEstimateCurrent))
			}
			if f.EPSEstimateNext != 0 {
				sb.WriteString(fmt.Sprintf("| EPS Estimate (Next Year) | $%.2f |\n", f.EPSEstimateNext))
			}
			if f.MostRecentQuarter != "" {
				sb.WriteString(fmt.Sprintf("| Most Recent Quarter | %s |\n", f.MostRecentQuarter))
			}
			sb.WriteString("\n")
		}

		// Analyst Consensus (if available)
		if f.AnalystRatings != nil {
			ar := f.AnalystRatings
			sb.WriteString("#### Analyst Consensus\n\n")
			sb.WriteString("| Metric | Value |\n")
			sb.WriteString("|--------|-------|\n")
			if ar.Rating != "" {
				sb.WriteString(fmt.Sprintf("| Rating | %s |\n", ar.Rating))
			}
			if ar.TargetPrice > 0 {
				sb.WriteString(fmt.Sprintf("| Target Price | $%.2f |\n", ar.TargetPrice))
			}
			sb.WriteString(fmt.Sprintf("| Strong Buy | %d |\n", ar.StrongBuy))
			sb.WriteString(fmt.Sprintf("| Buy | %d |\n", ar.Buy))
			sb.WriteString(fmt.Sprintf("| Hold | %d |\n", ar.Hold))
			sb.WriteString(fmt.Sprintf("| Sell | %d |\n", ar.Sell))
			sb.WriteString(fmt.Sprintf("| Strong Sell | %d |\n", ar.StrongSell))
			sb.WriteString("\n")
		}
	}

	// Technical Signals
	sb.WriteString(formatSignalsTable(hr.Signals))

	// News Intelligence
	sb.WriteString(formatNewsIntelligence(hr.NewsIntelligence))

	// Filings Intelligence (deprecated — kept for backwards compatibility with older reports)
	sb.WriteString(formatFilingsIntelligence(hr.FilingsIntelligence))

	// Company Releases (per-filing summaries)
	if len(hr.FilingSummaries) > 0 {
		sb.WriteString(formatCompanyReleases(hr.FilingSummaries))
	}

	// Company Timeline
	if hr.Timeline != nil {
		sb.WriteString(formatCompanyTimeline(hr.Timeline))
	}

	// Risk Flags
	sb.WriteString(formatRiskFlags(hr, review))

	return sb.String()
}

// formatCompanyReleases formats per-filing summaries as a table
func formatCompanyReleases(summaries []models.FilingSummary) string {
	var sb strings.Builder

	sb.WriteString("## Company Releases\n\n")

	// Show financial results as a table
	sb.WriteString("| Date | Filing | Type | Revenue | Profit | Key Detail |\n")
	sb.WriteString("|------|--------|------|---------|--------|------------|\n")

	shown := 0
	for _, fs := range summaries {
		if shown >= 15 {
			break
		}
		keyDetail := ""
		if fs.ContractValue != "" {
			keyDetail = "Contract: " + fs.ContractValue
			if fs.Customer != "" {
				keyDetail += " (" + fs.Customer + ")"
			}
		} else if fs.GuidanceRevenue != "" || fs.GuidanceProfit != "" {
			keyDetail = "Guidance: " + fs.GuidanceRevenue
			if fs.GuidanceProfit != "" {
				if keyDetail != "Guidance: " {
					keyDetail += " / "
				}
				keyDetail += fs.GuidanceProfit
			}
		} else if len(fs.KeyFacts) > 0 {
			keyDetail = fs.KeyFacts[0]
			if len(keyDetail) > 60 {
				keyDetail = keyDetail[:57] + "..."
			}
		}

		rev := fs.Revenue
		if fs.RevenueGrowth != "" {
			rev += " (" + fs.RevenueGrowth + ")"
		}
		profit := fs.Profit
		if fs.ProfitGrowth != "" {
			profit += " (" + fs.ProfitGrowth + ")"
		}

		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s |\n",
			fs.Date.Format("2006-01-02"), truncateStr(fs.Headline, 40), fs.Type,
			rev, profit, keyDetail))
		shown++
	}
	sb.WriteString("\n")

	// Show detailed key facts for most recent financial results
	factsShown := 0
	for _, fs := range summaries {
		if factsShown >= 3 {
			break
		}
		if fs.Type != "financial_results" || len(fs.KeyFacts) == 0 {
			continue
		}
		period := fs.Period
		if period == "" {
			period = fs.Date.Format("2006-01-02")
		}
		sb.WriteString(fmt.Sprintf("### %s — %s\n\n", period, fs.Headline))
		for _, kf := range fs.KeyFacts {
			sb.WriteString(fmt.Sprintf("- %s\n", kf))
		}
		sb.WriteString("\n")
		factsShown++
	}

	sb.WriteString(fmt.Sprintf("*%d filings analyzed*\n\n", len(summaries)))
	return sb.String()
}

// formatCompanyTimeline formats the structured timeline as markdown
func formatCompanyTimeline(tl *models.CompanyTimeline) string {
	var sb strings.Builder

	sb.WriteString("## Company Timeline\n\n")

	if tl.BusinessModel != "" {
		sb.WriteString("**Business Model:** " + tl.BusinessModel + "\n\n")
	}

	if len(tl.Periods) > 0 {
		sb.WriteString("### Financial History\n\n")
		sb.WriteString("| Period | Revenue | Growth | Profit | Growth | Margin | EPS | Dividend |\n")
		sb.WriteString("|--------|---------|--------|--------|--------|--------|-----|----------|\n")
		for _, p := range tl.Periods {
			sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s | %s | %s |\n",
				p.Period, p.Revenue, p.RevenueGrowth, p.Profit, p.ProfitGrowth,
				p.Margin, p.EPS, p.Dividend))
		}
		sb.WriteString("\n")

		// Show guidance tracking inline
		for _, p := range tl.Periods {
			if p.GuidanceGiven != "" || p.GuidanceOutcome != "" {
				sb.WriteString(fmt.Sprintf("**%s Guidance:** ", p.Period))
				if p.GuidanceGiven != "" {
					sb.WriteString(p.GuidanceGiven)
				}
				if p.GuidanceOutcome != "" {
					sb.WriteString(" | Outcome: " + p.GuidanceOutcome)
				}
				sb.WriteString("\n")
			}
		}
		sb.WriteString("\n")
	}

	if len(tl.KeyEvents) > 0 {
		sb.WriteString("### Key Events\n\n")
		for _, e := range tl.KeyEvents {
			impact := ""
			if e.Impact != "" && e.Impact != "neutral" {
				impact = " [" + e.Impact + "]"
			}
			sb.WriteString(fmt.Sprintf("- **%s** %s%s", e.Date, e.Event, impact))
			if e.Detail != "" {
				sb.WriteString(": " + e.Detail)
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	// Operational metrics
	if tl.WorkOnHand != "" || tl.RepeatBusinessRate != "" || tl.NextReportingDate != "" {
		sb.WriteString("### Operational\n\n")
		if tl.WorkOnHand != "" {
			sb.WriteString(fmt.Sprintf("- **Work on Hand:** %s\n", tl.WorkOnHand))
		}
		if tl.RepeatBusinessRate != "" {
			sb.WriteString(fmt.Sprintf("- **Repeat Business Rate:** %s\n", tl.RepeatBusinessRate))
		}
		if tl.NextReportingDate != "" {
			sb.WriteString(fmt.Sprintf("- **Next Reporting Date:** %s\n", tl.NextReportingDate))
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("*Generated %s*\n\n", tl.GeneratedAt.Format("2006-01-02")))
	return sb.String()
}

// truncateStr shortens a string to max length with ellipsis
func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// formatTradeHistory renders a date-ordered table of buys, sells, and dividends with totals
func formatTradeHistory(h models.Holding) string {
	if len(h.Trades) == 0 {
		return ""
	}

	// Sort trades by date
	trades := make([]*models.NavexaTrade, len(h.Trades))
	copy(trades, h.Trades)
	sort.Slice(trades, func(i, j int) bool {
		return trades[i].Date < trades[j].Date
	})

	var sb strings.Builder
	sb.WriteString("## Trade History\n\n")
	sb.WriteString("| Date | Type | Units | Price | Fees | Value |\n")
	sb.WriteString("|------|------|-------|-------|------|-------|\n")

	totalBuyUnits := 0.0
	totalBuyCost := 0.0
	totalBuyFees := 0.0
	totalSellUnits := 0.0
	totalSellValue := 0.0
	totalSellFees := 0.0
	lastTradeDate := ""

	for _, t := range trades {
		tradeType := strings.ToUpper(t.Type)
		date := t.Date
		if len(date) > 10 {
			date = date[:10] // trim to YYYY-MM-DD
		}
		lastTradeDate = date

		// Compute value for display
		value := t.Units * t.Price
		if t.Value != 0 {
			value = t.Value
		}

		switch strings.ToLower(t.Type) {
		case "buy", "opening balance":
			totalBuyUnits += t.Units
			totalBuyCost += t.Units*t.Price + t.Fees
			totalBuyFees += t.Fees
			sb.WriteString(fmt.Sprintf("| %s | %s | %.0f | %s | %s | %s |\n",
				date, tradeType, t.Units,
				common.FormatMoney(t.Price),
				common.FormatMoney(t.Fees),
				common.FormatMoney(t.Units*t.Price),
			))
		case "sell":
			totalSellUnits += t.Units
			totalSellValue += t.Units * t.Price
			totalSellFees += t.Fees
			sb.WriteString(fmt.Sprintf("| %s | %s | %.0f | %s | %s | %s |\n",
				date, tradeType, t.Units,
				common.FormatMoney(t.Price),
				common.FormatMoney(t.Fees),
				common.FormatMoney(t.Units*t.Price),
			))
		case "cost base increase", "cost base decrease":
			sb.WriteString(fmt.Sprintf("| %s | %s | | | | %s |\n",
				date, tradeType, common.FormatSignedMoney(value),
			))
		default:
			sb.WriteString(fmt.Sprintf("| %s | %s | %.0f | %s | %s | %s |\n",
				date, tradeType, t.Units,
				common.FormatMoney(t.Price),
				common.FormatMoney(t.Fees),
				common.FormatMoney(value),
			))
		}
	}

	// Add dividend row if there's dividend income
	if h.DividendReturn != 0 {
		divDate := lastTradeDate
		if divDate == "" {
			divDate = time.Now().Format("2006-01-02")
		}
		sb.WriteString(fmt.Sprintf("| %s | DIVIDEND | | | | %s |\n",
			divDate, common.FormatSignedMoney(h.DividendReturn),
		))
	}

	// Totals row
	sb.WriteString(fmt.Sprintf("| | **Total Bought** | **%.0f** | | **%s** | **%s** |\n",
		totalBuyUnits, common.FormatMoney(totalBuyFees), common.FormatMoney(totalBuyCost),
	))
	if totalSellUnits > 0 {
		sb.WriteString(fmt.Sprintf("| | **Total Sold** | **%.0f** | | **%s** | **%s** |\n",
			totalSellUnits, common.FormatMoney(totalSellFees), common.FormatMoney(totalSellValue),
		))
	}
	sb.WriteString(fmt.Sprintf("| | **Capital Gain** | | | | **%s (%s)** |\n",
		common.FormatSignedMoney(h.GainLoss), common.FormatSignedPct(h.CapitalGainPct),
	))
	if h.DividendReturn != 0 {
		sb.WriteString(fmt.Sprintf("| | **Dividends** | | | | **%s** |\n",
			common.FormatSignedMoney(h.DividendReturn),
		))
	}
	sb.WriteString(fmt.Sprintf("| | **Total Return** | | | | **%s (%s)** |\n",
		common.FormatSignedMoney(h.TotalReturnValue), common.FormatSignedPct(h.TotalReturnPct),
	))
	sb.WriteString("\n")

	return sb.String()
}

// formatSignalsTable renders the common technical signals table
func formatSignalsTable(signals *models.TickerSignals) string {
	var sb strings.Builder

	sb.WriteString("### Technical Signals\n\n")

	if signals == nil {
		sb.WriteString("*Signal data not available*\n\n")
		return sb.String()
	}

	sb.WriteString("| Signal | Value | Status |\n")
	sb.WriteString("|--------|-------|--------|\n")
	sb.WriteString(fmt.Sprintf("| Trend | %s | %s |\n", signals.Trend, signals.TrendDescription))
	sb.WriteString(fmt.Sprintf("| SMA 20 | $%.2f | %s |\n", signals.Price.SMA20, formatSMAStatus(signals.Price.Current, signals.Price.SMA20)))
	sb.WriteString(fmt.Sprintf("| SMA 50 | $%.2f | %s |\n", signals.Price.SMA50, formatSMAStatus(signals.Price.Current, signals.Price.SMA50)))
	sb.WriteString(fmt.Sprintf("| SMA 200 | $%.2f | %s |\n", signals.Price.SMA200, formatSMAStatus(signals.Price.Current, signals.Price.SMA200)))
	sb.WriteString(fmt.Sprintf("| RSI | %.2f | %s |\n", signals.Technical.RSI, signals.Technical.RSISignal))
	sb.WriteString(fmt.Sprintf("| MACD | %.4f | %s |\n", signals.Technical.MACD, signals.Technical.MACDCrossover))
	sb.WriteString(fmt.Sprintf("| Volume | %.2fx avg | %s |\n", signals.Technical.VolumeRatio, signals.Technical.VolumeSignal))
	sb.WriteString(fmt.Sprintf("| PBAS | %.2f | %s |\n", signals.PBAS.Score, signals.PBAS.Interpretation))
	sb.WriteString(fmt.Sprintf("| VLI | %.2f | %s |\n", signals.VLI.Score, signals.VLI.Interpretation))
	sb.WriteString(fmt.Sprintf("| Regime | %s | %s |\n", signals.Regime.Current, signals.Regime.Description))
	if signals.Technical.SupportLevel > 0 {
		sb.WriteString(fmt.Sprintf("| Support | $%.2f | |\n", signals.Technical.SupportLevel))
	}
	if signals.Technical.ResistanceLevel > 0 {
		sb.WriteString(fmt.Sprintf("| Resistance | $%.2f | |\n", signals.Technical.ResistanceLevel))
	}
	sb.WriteString("\n")

	return sb.String()
}

// formatRiskFlags renders the risk flags section for a ticker
func formatRiskFlags(hr *models.HoldingReview, review *models.PortfolioReview) string {
	var sb strings.Builder

	sb.WriteString("## Risk Flags\n\n")

	flags := make([]string, 0)

	// From signals
	if hr.Signals != nil && len(hr.Signals.RiskFlags) > 0 {
		flags = append(flags, hr.Signals.RiskFlags...)
	}

	// From portfolio alerts for this ticker
	for _, alert := range review.Alerts {
		if alert.Ticker == hr.Holding.Ticker {
			flags = append(flags, alert.Message)
		}
	}

	if len(flags) == 0 {
		sb.WriteString("None\n\n")
	} else {
		for _, flag := range flags {
			sb.WriteString(fmt.Sprintf("- %s\n", flag))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// formatNewsIntelligence renders the news intelligence section for stored reports.
func formatNewsIntelligence(intel *models.NewsIntelligence) string {
	if intel == nil {
		return ""
	}

	var sb strings.Builder

	sb.WriteString("## News Intelligence\n\n")
	sb.WriteString(fmt.Sprintf("**Sentiment:** %s\n\n", intel.OverallSentiment))
	sb.WriteString(intel.Summary + "\n\n")

	if len(intel.KeyThemes) > 0 {
		sb.WriteString("**Key Themes:** ")
		sb.WriteString(strings.Join(intel.KeyThemes, ", "))
		sb.WriteString("\n\n")
	}

	sb.WriteString("### Impact Assessment\n\n")
	sb.WriteString("| Timeframe | Outlook |\n")
	sb.WriteString("|-----------|----------|\n")
	sb.WriteString(fmt.Sprintf("| This Week | %s |\n", intel.ImpactWeek))
	sb.WriteString(fmt.Sprintf("| This Month | %s |\n", intel.ImpactMonth))
	sb.WriteString(fmt.Sprintf("| This Year | %s |\n", intel.ImpactYear))
	sb.WriteString("\n")

	if len(intel.Articles) > 0 {
		sb.WriteString("### Sources\n\n")
		for _, a := range intel.Articles {
			cred := "[credible]"
			switch a.Credibility {
			case "fluff":
				cred = "[fluff]"
			case "promotional":
				cred = "[promotional]"
			case "speculative":
				cred = "[speculative]"
			}
			sb.WriteString(fmt.Sprintf("- %s [%s](%s) (%s) — %s\n", cred, a.Title, a.URL, a.Source, a.Summary))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// formatFilingsIntelligence renders the filings intelligence section for stored reports.
func formatFilingsIntelligence(intel *models.FilingsIntelligence) string {
	if intel == nil {
		return ""
	}

	var sb strings.Builder

	sb.WriteString("## Company Filings Intelligence\n\n")

	sb.WriteString(fmt.Sprintf("**Financial Health:** %s | **Growth Outlook:** %s\n\n",
		intel.FinancialHealth, intel.GrowthOutlook))

	if intel.GrowthRationale != "" {
		sb.WriteString("### Growth Outlook\n\n")
		sb.WriteString(intel.GrowthRationale + "\n\n")
	}

	// Summary
	sb.WriteString("### Summary\n\n")
	sb.WriteString(intel.Summary + "\n\n")

	// Key Metrics
	if len(intel.KeyMetrics) > 0 {
		sb.WriteString("### Key Metrics\n\n")
		sb.WriteString("| Metric | Value | Period | Trend |\n")
		sb.WriteString("|--------|-------|--------|-------|\n")
		for _, m := range intel.KeyMetrics {
			sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n", m.Name, m.Value, m.Period, m.Trend))
		}
		sb.WriteString("\n")
	}

	// Year-over-Year
	if len(intel.YearOverYear) > 0 {
		sb.WriteString("### Year-over-Year\n\n")
		sb.WriteString("| Period | Revenue | Profit | Outlook | Key Changes |\n")
		sb.WriteString("|--------|---------|--------|---------|-------------|\n")
		for _, y := range intel.YearOverYear {
			sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
				y.Period, y.Revenue, y.Profit, y.Outlook, y.KeyChanges))
		}
		sb.WriteString("\n")
	}

	// Positive Factors
	if len(intel.PositiveFactors) > 0 {
		sb.WriteString("### Positive Factors\n\n")
		for _, f := range intel.PositiveFactors {
			sb.WriteString(fmt.Sprintf("- %s\n", f))
		}
		sb.WriteString("\n")
	}

	// Risk Factors
	if len(intel.RiskFactors) > 0 {
		sb.WriteString("### Risk Factors\n\n")
		for _, f := range intel.RiskFactors {
			sb.WriteString(fmt.Sprintf("- %s\n", f))
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("*Based on %d filings analyzed | Generated %s*\n\n",
		intel.FilingsAnalyzed, intel.GeneratedAt.Format("2006-01-02")))

	return sb.String()
}

// formatSMAStatus returns "above" or "below" based on price vs SMA
func formatSMAStatus(price, sma float64) string {
	if sma == 0 {
		return "N/A"
	}
	if price >= sma {
		return "above"
	}
	return "below"
}

// formatAction formats the action without emojis (for stored reports)
func formatAction(action string) string {
	return action
}
