package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
)

// Delegate to common format helpers
func formatMoney(v float64) string        { return common.FormatMoney(v) }
func formatSignedMoney(v float64) string  { return common.FormatSignedMoney(v) }
func formatSignedPct(v float64) string    { return common.FormatSignedPct(v) }
func formatMarketCap(v float64) string    { return common.FormatMarketCap(v) }
func isETF(hr *models.HoldingReview) bool { return common.IsETF(hr) }

func formatMoneyWithCcy(v float64, ccy string) string { return common.FormatMoneyWithCurrency(v, ccy) }
func formatSignedMoneyWithCcy(v float64, ccy string) string {
	return common.FormatSignedMoneyWithCurrency(v, ccy)
}

// formatPortfolioReview formats a portfolio review as markdown
func formatPortfolioReview(review *models.PortfolioReview) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Portfolio Compliance: %s\n\n", review.PortfolioName))
	sb.WriteString(fmt.Sprintf("**Date:** %s\n", review.ReviewDate.Format("2006-01-02 15:04")))
	sb.WriteString(fmt.Sprintf("**Total Value:** %s\n", formatMoney(review.TotalValue)))
	sb.WriteString(fmt.Sprintf("**Total Cost:** %s\n", formatMoney(review.TotalCost)))
	sb.WriteString(fmt.Sprintf("**Total Gain:** %s (%s)\n", formatSignedMoney(review.TotalGain), formatSignedPct(review.TotalGainPct)))
	sb.WriteString(fmt.Sprintf("**Day Change:** %s (%s)\n", formatSignedMoney(review.DayChange), formatSignedPct(review.DayChangePct)))
	if review.FXRate > 0 {
		sb.WriteString(fmt.Sprintf("**FX Rate (AUDUSD):** %.4f — USD holdings converted to AUD\n", review.FXRate))
	}
	sb.WriteString("\n")

	var stocks, etfs, closed []models.HoldingReview
	for _, hr := range review.HoldingReviews {
		if hr.ActionRequired == "CLOSED" {
			closed = append(closed, hr)
		} else if isETF(&hr) {
			etfs = append(etfs, hr)
		} else {
			stocks = append(stocks, hr)
		}
	}

	sort.Slice(stocks, func(i, j int) bool { return stocks[i].Holding.Ticker < stocks[j].Holding.Ticker })
	sort.Slice(etfs, func(i, j int) bool { return etfs[i].Holding.Ticker < etfs[j].Holding.Ticker })
	sort.Slice(closed, func(i, j int) bool { return closed[i].Holding.Ticker < closed[j].Holding.Ticker })

	sb.WriteString("## Holdings\n\n")

	hasCompliance := false
	for _, hr := range review.HoldingReviews {
		if hr.Compliance != nil {
			hasCompliance = true
			break
		}
	}

	// toAUD converts a holding value from its native currency to AUD.
	toAUD := func(v float64, ccy string) float64 {
		if ccy == "USD" && review.FXRate > 0 {
			return v / review.FXRate
		}
		return v
	}

	if len(stocks) > 0 {
		sb.WriteString("### Stocks\n\n")
		if hasCompliance {
			sb.WriteString("| Symbol | Weight | Avg Buy | Qty | Price | Value | Capital Gain % | Income Return | Total Return | Total Return % | TWRR % | Action | C |\n")
			sb.WriteString("|--------|--------|---------|-----|-------|-------|----------------|---------------|--------------|----------------------|--------|--------|---|\n")
		} else {
			sb.WriteString("| Symbol | Weight | Avg Buy | Qty | Price | Value | Capital Gain % | Income Return | Total Return | Total Return % | TWRR % | Action |\n")
			sb.WriteString("|--------|--------|---------|-----|-------|-------|----------------|---------------|--------------|----------------------|--------|--------|\n")
		}

		stocksTotal := 0.0
		stocksGain := 0.0
		for _, hr := range stocks {
			h := hr.Holding
			stocksTotal += toAUD(h.MarketValue, h.Currency)
			stocksGain += toAUD(h.TotalReturnValue, h.Currency)
			if hasCompliance {
				sb.WriteString(fmt.Sprintf("| %s | %.1f%% | %s | %.0f | %s | %s | %s | %s | %s | %s | %s | %s | %s |\n",
					h.Ticker, h.Weight, formatMoney(toAUD(h.AvgCost, h.Currency)), h.Units,
					formatMoney(toAUD(h.CurrentPrice, h.Currency)), formatMoney(toAUD(h.MarketValue, h.Currency)),
					formatSignedPct(h.CapitalGainPct), formatSignedMoney(toAUD(h.DividendReturn, h.Currency)),
					formatSignedMoney(toAUD(h.TotalReturnValue, h.Currency)), formatSignedPct(h.TotalReturnPct), formatSignedPct(h.TotalReturnPctTWRR),
					formatActionWithReason(hr.ActionRequired, hr.ActionReason),
					formatCompliance(hr.Compliance)))
			} else {
				sb.WriteString(fmt.Sprintf("| %s | %.1f%% | %s | %.0f | %s | %s | %s | %s | %s | %s | %s | %s |\n",
					h.Ticker, h.Weight, formatMoney(toAUD(h.AvgCost, h.Currency)), h.Units,
					formatMoney(toAUD(h.CurrentPrice, h.Currency)), formatMoney(toAUD(h.MarketValue, h.Currency)),
					formatSignedPct(h.CapitalGainPct), formatSignedMoney(toAUD(h.DividendReturn, h.Currency)),
					formatSignedMoney(toAUD(h.TotalReturnValue, h.Currency)), formatSignedPct(h.TotalReturnPct), formatSignedPct(h.TotalReturnPctTWRR),
					formatActionWithReason(hr.ActionRequired, hr.ActionReason)))
			}
		}
		stocksGainPct := 0.0
		if stocksTotal-stocksGain > 0 {
			stocksGainPct = (stocksGain / (stocksTotal - stocksGain)) * 100
		}
		if hasCompliance {
			sb.WriteString(fmt.Sprintf("| **Stocks Total** | | | | | **%s** | | | **%s** | **%s** | | | |\n\n",
				formatMoney(stocksTotal), formatSignedMoney(stocksGain), formatSignedPct(stocksGainPct)))
		} else {
			sb.WriteString(fmt.Sprintf("| **Stocks Total** | | | | | **%s** | | | **%s** | **%s** | | |\n\n",
				formatMoney(stocksTotal), formatSignedMoney(stocksGain), formatSignedPct(stocksGainPct)))
		}
	}

	if len(etfs) > 0 {
		sb.WriteString("### ETFs\n\n")
		if hasCompliance {
			sb.WriteString("| Symbol | Weight | Avg Buy | Qty | Price | Value | Capital Gain % | Income Return | Total Return | Total Return % | TWRR % | Action | C |\n")
			sb.WriteString("|--------|--------|---------|-----|-------|-------|----------------|---------------|--------------|----------------------|--------|--------|---|\n")
		} else {
			sb.WriteString("| Symbol | Weight | Avg Buy | Qty | Price | Value | Capital Gain % | Income Return | Total Return | Total Return % | TWRR % | Action |\n")
			sb.WriteString("|--------|--------|---------|-----|-------|-------|----------------|---------------|--------------|----------------------|--------|--------|\n")
		}

		etfsTotal := 0.0
		etfsGain := 0.0
		for _, hr := range etfs {
			h := hr.Holding
			etfsTotal += toAUD(h.MarketValue, h.Currency)
			etfsGain += toAUD(h.TotalReturnValue, h.Currency)
			if hasCompliance {
				sb.WriteString(fmt.Sprintf("| %s | %.1f%% | %s | %.0f | %s | %s | %s | %s | %s | %s | %s | %s | %s |\n",
					h.Ticker, h.Weight, formatMoney(toAUD(h.AvgCost, h.Currency)), h.Units,
					formatMoney(toAUD(h.CurrentPrice, h.Currency)), formatMoney(toAUD(h.MarketValue, h.Currency)),
					formatSignedPct(h.CapitalGainPct), formatSignedMoney(toAUD(h.DividendReturn, h.Currency)),
					formatSignedMoney(toAUD(h.TotalReturnValue, h.Currency)), formatSignedPct(h.TotalReturnPct), formatSignedPct(h.TotalReturnPctTWRR),
					formatActionWithReason(hr.ActionRequired, hr.ActionReason),
					formatCompliance(hr.Compliance)))
			} else {
				sb.WriteString(fmt.Sprintf("| %s | %.1f%% | %s | %.0f | %s | %s | %s | %s | %s | %s | %s | %s |\n",
					h.Ticker, h.Weight, formatMoney(toAUD(h.AvgCost, h.Currency)), h.Units,
					formatMoney(toAUD(h.CurrentPrice, h.Currency)), formatMoney(toAUD(h.MarketValue, h.Currency)),
					formatSignedPct(h.CapitalGainPct), formatSignedMoney(toAUD(h.DividendReturn, h.Currency)),
					formatSignedMoney(toAUD(h.TotalReturnValue, h.Currency)), formatSignedPct(h.TotalReturnPct), formatSignedPct(h.TotalReturnPctTWRR),
					formatActionWithReason(hr.ActionRequired, hr.ActionReason)))
			}
		}
		etfsGainPct := 0.0
		if etfsTotal-etfsGain > 0 {
			etfsGainPct = (etfsGain / (etfsTotal - etfsGain)) * 100
		}
		if hasCompliance {
			sb.WriteString(fmt.Sprintf("| **ETFs Total** | | | | | **%s** | | | **%s** | **%s** | | | |\n\n",
				formatMoney(etfsTotal), formatSignedMoney(etfsGain), formatSignedPct(etfsGainPct)))
		} else {
			sb.WriteString(fmt.Sprintf("| **ETFs Total** | | | | | **%s** | | | **%s** | **%s** | | |\n\n",
				formatMoney(etfsTotal), formatSignedMoney(etfsGain), formatSignedPct(etfsGainPct)))
		}
	}

	if len(closed) > 0 {
		sb.WriteString("### Closed Positions\n\n")
		sb.WriteString("| Symbol | Weight | Avg Buy | Qty | Price | Value | Capital Gain % | Income Return | Total Return | Total Return % | TWRR % | Action |\n")
		sb.WriteString("|--------|--------|---------|-----|-------|-------|----------------|---------------|--------------|----------------------|--------|--------|\n")

		closedGain := 0.0
		for _, hr := range closed {
			h := hr.Holding
			closedGain += toAUD(h.TotalReturnValue, h.Currency)
			sb.WriteString(fmt.Sprintf("| %s | %.1f%% | %s | %.0f | %s | %s | %s | %s | %s | %s | %s | %s |\n",
				h.Ticker, h.Weight, formatMoney(toAUD(h.AvgCost, h.Currency)), h.Units,
				formatMoney(toAUD(h.CurrentPrice, h.Currency)), formatMoney(toAUD(h.MarketValue, h.Currency)),
				formatSignedPct(h.CapitalGainPct), formatSignedMoney(toAUD(h.DividendReturn, h.Currency)),
				formatSignedMoney(toAUD(h.TotalReturnValue, h.Currency)), formatSignedPct(h.TotalReturnPct), formatSignedPct(h.TotalReturnPctTWRR),
				formatAction(hr.ActionRequired)))
		}
		closedCost := 0.0
		for _, hr := range closed {
			closedCost += toAUD(hr.Holding.TotalCost, hr.Holding.Currency)
		}
		closedGainPct := 0.0
		if closedCost > 0 {
			closedGainPct = (closedGain / closedCost) * 100
		}
		sb.WriteString(fmt.Sprintf("| **Closed Total** | | | | | | | | **%s** | **%s** | | |\n\n",
			formatSignedMoney(closedGain), formatSignedPct(closedGainPct)))
	}

	sb.WriteString(fmt.Sprintf("**Portfolio Total:** %s | **Total Return:** %s (%s)\n\n",
		formatMoney(review.TotalValue), formatSignedMoney(review.TotalGain), formatSignedPct(review.TotalGainPct)))

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

	if hasCompliance {
		nonCompliant := make([]models.HoldingReview, 0)
		for _, hr := range review.HoldingReviews {
			if hr.Compliance != nil && hr.Compliance.Status == models.ComplianceStatusNonCompliant {
				nonCompliant = append(nonCompliant, hr)
			}
		}
		if len(nonCompliant) > 0 {
			sb.WriteString("## Strategy Compliance\n\n")
			sb.WriteString("| Symbol | Status | Reasons |\n")
			sb.WriteString("|--------|--------|---------|\n")
			for _, hr := range nonCompliant {
				sb.WriteString(fmt.Sprintf("| %s | Non-compliant | %s |\n",
					hr.Holding.Ticker, strings.Join(hr.Compliance.Reasons, "; ")))
			}
			sb.WriteString("\n")
		}
	}

	if review.Summary != "" {
		sb.WriteString("## Summary\n\n")
		sb.WriteString(review.Summary)
		sb.WriteString("\n\n")
	}

	if len(review.Alerts) > 0 || len(review.Recommendations) > 0 {
		sb.WriteString("## Alerts & Observations\n\n")
		if len(review.Alerts) > 0 {
			sb.WriteString("### Alerts\n\n")
			for _, alert := range review.Alerts {
				sb.WriteString(fmt.Sprintf("- **%s**: %s\n", alert.Ticker, alert.Message))
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

func formatPortfolioHoldings(p *models.Portfolio) string {
	var sb strings.Builder

	// toAUD converts a holding value from its native currency to AUD.
	toAUD := func(v float64, ccy string) float64 {
		if ccy == "USD" && p.FXRate > 0 {
			return v / p.FXRate
		}
		return v
	}

	sb.WriteString(fmt.Sprintf("# Portfolio: %s\n\n", p.Name))
	sb.WriteString(fmt.Sprintf("**Market Value:** %s *(current worth if sold today)*\n", formatMoney(p.TotalValue)))
	sb.WriteString(fmt.Sprintf("**Cost Basis:** %s *(amount paid for current holdings)*\n", formatMoney(p.TotalCost)))
	sb.WriteString(fmt.Sprintf("**Total Gain:** %s (%s) *(simple return: gain ÷ cost basis)*\n", formatSignedMoney(p.TotalGain), formatSignedPct(p.TotalGainPct)))
	sb.WriteString(fmt.Sprintf("**Last Synced:** %s\n", p.LastSynced.Format("2006-01-02 15:04")))
	if p.FXRate > 0 {
		sb.WriteString(fmt.Sprintf("**FX Rate (AUDUSD):** %.4f — USD holdings converted to AUD\n", p.FXRate))
	}
	sb.WriteString("\n")

	var active, closedHoldings []models.Holding
	for _, h := range p.Holdings {
		if h.Units > 0 {
			active = append(active, h)
		} else {
			closedHoldings = append(closedHoldings, h)
		}
	}

	sort.Slice(active, func(i, j int) bool { return active[i].Ticker < active[j].Ticker })
	sort.Slice(closedHoldings, func(i, j int) bool { return closedHoldings[i].Ticker < closedHoldings[j].Ticker })

	if len(active) > 0 {
		sb.WriteString("## Holdings\n\n")
		sb.WriteString("| Symbol | Name | Country | Units | Avg Cost | Price | Value | Weight | Gain | Gain % | TWRR % |\n")
		sb.WriteString("|--------|------|---------|-------|----------|-------|-------|--------|------|--------------|--------|\n")
		for _, h := range active {
			name := h.Name
			if len(name) > 25 {
				name = name[:22] + "..."
			}
			sb.WriteString(fmt.Sprintf("| %s | %s | %s | %.2f | %s | %s | %s | %.1f%% | %s | %s | %s |\n",
				h.Ticker, name, h.Country, h.Units,
				formatMoney(toAUD(h.AvgCost, h.Currency)), formatMoney(toAUD(h.CurrentPrice, h.Currency)),
				formatMoney(toAUD(h.MarketValue, h.Currency)), h.Weight,
				formatSignedMoney(toAUD(h.GainLoss, h.Currency)), formatSignedPct(h.GainLossPct), formatSignedPct(h.TotalReturnPctTWRR)))
		}
		sb.WriteString("\n")
	}

	if len(closedHoldings) > 0 {
		sb.WriteString("## Closed Positions\n\n")
		sb.WriteString("| Symbol | Name | Realized Gain | Gain % | TWRR % |\n")
		sb.WriteString("|--------|------|---------------|--------------|--------|\n")
		for _, h := range closedHoldings {
			name := h.Name
			if len(name) > 25 {
				name = name[:22] + "..."
			}
			sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
				h.Ticker, name,
				formatSignedMoney(toAUD(h.GainLoss, h.Currency)), formatSignedPct(h.GainLossPct), formatSignedPct(h.TotalReturnPctTWRR)))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func formatAction(action string) string {
	switch action {
	case "SELL", "EXIT TRIGGER":
		return "EXIT TRIGGER"
	case "BUY", "ENTRY CRITERIA MET":
		return "ENTRY CRITERIA MET"
	case "WATCH":
		return "WATCH"
	case "CLOSED":
		return "CLOSED"
	case "ALERT":
		return "ALERT"
	default:
		return "COMPLIANT"
	}
}

func formatActionWithReason(action, reason string) string {
	base := formatAction(action)
	if reason == "" || reason == "All indicators within tolerance" {
		return base
	}
	return base + ": " + reason
}

// formatTrend translates internal trend labels to neutral display language
func formatTrend(trend models.TrendType) string {
	switch trend {
	case "bullish":
		return "upward trend"
	case "bearish":
		return "downward trend"
	default:
		return string(trend)
	}
}

// formatTrendDescription translates internal trend descriptions to neutral display language
func formatTrendDescription(desc string) string {
	r := strings.NewReplacer(
		"Bullish trend:", "Upward trend:",
		"Bearish trend:", "Downward trend:",
		"bullish momentum", "upward momentum",
		"bearish momentum", "downward momentum",
		"Bullish MACD crossover", "Positive MACD crossover",
		"Bearish MACD crossover", "Negative MACD crossover",
		"Bullish trend confirms", "Upward trend aligns with",
		"Bearish trend contradicts", "Downward trend diverges from",
		"Bullish news sentiment", "Positive news sentiment",
		"Bearish news sentiment", "Negative news sentiment",
	)
	return r.Replace(desc)
}

func formatCompliance(compliance *models.ComplianceResult) string {
	if compliance == nil {
		return "-"
	}
	switch compliance.Status {
	case models.ComplianceStatusCompliant:
		return "OK"
	case models.ComplianceStatusNonCompliant:
		return "FAIL"
	default:
		return "-"
	}
}

func formatPortfolioSnapshot(snapshot *models.PortfolioSnapshot) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Portfolio Snapshot: %s\n\n", snapshot.PortfolioName))
	sb.WriteString(fmt.Sprintf("**As-of Date:** %s\n", snapshot.AsOfDate.Format("2006-01-02")))
	sb.WriteString(fmt.Sprintf("**Price Date:** %s", snapshot.PriceDate.Format("2006-01-02")))

	asOfDay := snapshot.AsOfDate.Truncate(24 * time.Hour)
	priceDay := snapshot.PriceDate.Truncate(24 * time.Hour)
	if !asOfDay.Equal(priceDay) {
		sb.WriteString(" *(closest trading day)*")
	}
	sb.WriteString("\n\n")

	sb.WriteString(fmt.Sprintf("**Total Value:** %s\n", formatMoney(snapshot.TotalValue)))
	sb.WriteString(fmt.Sprintf("**Total Cost:** %s\n", formatMoney(snapshot.TotalCost)))
	sb.WriteString(fmt.Sprintf("**Total Gain:** %s (%s)\n\n", formatSignedMoney(snapshot.TotalGain), formatSignedPct(snapshot.TotalGainPct)))

	if len(snapshot.Holdings) == 0 {
		sb.WriteString("No holdings found at this date.\n")
		return sb.String()
	}

	sb.WriteString("| Ticker | Name | Units | Avg Cost | Close Price | Market Value | Gain/Loss | Gain % | Weight |\n")
	sb.WriteString("|--------|------|-------|----------|-------------|-------------|-----------|--------|--------|\n")

	for _, h := range snapshot.Holdings {
		sb.WriteString(fmt.Sprintf("| %s | %s | %.0f | %s | %s | %s | %s | %s | %.1f%% |\n",
			h.Ticker, h.Name, h.Units, formatMoney(h.AvgCost), formatMoney(h.ClosePrice),
			formatMoney(h.MarketValue), formatSignedMoney(h.GainLoss),
			formatSignedPct(h.GainLossPct), h.Weight))
	}

	sb.WriteString(fmt.Sprintf("| **Total** | | | | | **%s** | **%s** | **%s** | |\n",
		formatMoney(snapshot.TotalValue), formatSignedMoney(snapshot.TotalGain), formatSignedPct(snapshot.TotalGainPct)))

	return sb.String()
}

func formatPortfolioGrowth(points []models.GrowthDataPoint, chartURL string) string {
	var sb strings.Builder

	sb.WriteString("## Portfolio Growth\n\n")
	if len(points) == 0 {
		sb.WriteString("No growth data available.\n")
		return sb.String()
	}

	sb.WriteString("| Date | Portfolio Value | Gain/Loss | Gain % | Tickers |\n")
	sb.WriteString("|------|----------------|-----------|--------|----------|\n")

	for _, p := range points {
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %d |\n",
			p.Date.Format("Jan 2006"), formatMoney(p.TotalValue),
			formatSignedMoney(p.GainLoss), formatSignedPct(p.GainLossPct), p.HoldingCount))
	}

	sb.WriteString("\n")
	if chartURL != "" {
		sb.WriteString(fmt.Sprintf("_Chart: %s_\n\n", chartURL))
	}
	return sb.String()
}

func formatPortfolioHistory(points []models.GrowthDataPoint, granularity string) string {
	var sb strings.Builder

	if len(points) == 0 {
		sb.WriteString("No portfolio history data available.\n")
		return sb.String()
	}

	first := points[0]
	last := points[len(points)-1]
	netChange := last.TotalValue - first.TotalValue
	changePct := 0.0
	if first.TotalValue > 0 {
		changePct = (netChange / first.TotalValue) * 100
	}

	sb.WriteString("# Portfolio History\n\n")
	sb.WriteString(fmt.Sprintf("**Period:** %s to %s (%d data points, %s)\n", first.Date.Format("2006-01-02"), last.Date.Format("2006-01-02"), len(points), granularity))
	sb.WriteString(fmt.Sprintf("**Start Value:** %s\n", formatMoney(first.TotalValue)))
	sb.WriteString(fmt.Sprintf("**End Value:** %s\n", formatMoney(last.TotalValue)))
	sb.WriteString(fmt.Sprintf("**Net Change:** %s (%s)\n\n", formatSignedMoney(netChange), formatSignedPct(changePct)))

	switch granularity {
	case "daily":
		sb.WriteString("| Date | Value | Gain/Loss | Gain % | Tickers | Day Change | Day % |\n")
		sb.WriteString("|------|-------|-----------|--------|---------|------------|-------|\n")
		for i, p := range points {
			dayChange := ""
			dayChangePct := ""
			if i > 0 {
				dc := p.TotalValue - points[i-1].TotalValue
				dayChange = formatSignedMoney(dc)
				if prev := points[i-1].TotalValue; prev > 0 {
					dcPct := (dc / prev) * 100
					dayChangePct = formatSignedPct(dcPct)
				}
			}
			sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %d | %s | %s |\n",
				p.Date.Format("2006-01-02"), formatMoney(p.TotalValue),
				formatSignedMoney(p.GainLoss), formatSignedPct(p.GainLossPct),
				p.HoldingCount, dayChange, dayChangePct))
		}

	case "weekly":
		sb.WriteString("| Week Ending | Value | Gain/Loss | Gain % | Tickers | Week Change | Week % |\n")
		sb.WriteString("|-------------|-------|-----------|--------|---------|-------------|--------|\n")
		for i, p := range points {
			weekChange := ""
			weekChangePct := ""
			if i > 0 {
				wc := p.TotalValue - points[i-1].TotalValue
				weekChange = formatSignedMoney(wc)
				if prev := points[i-1].TotalValue; prev > 0 {
					wcPct := (wc / prev) * 100
					weekChangePct = formatSignedPct(wcPct)
				}
			}
			sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %d | %s | %s |\n",
				p.Date.Format("2006-01-02"), formatMoney(p.TotalValue),
				formatSignedMoney(p.GainLoss), formatSignedPct(p.GainLossPct),
				p.HoldingCount, weekChange, weekChangePct))
		}

	case "monthly":
		sb.WriteString("| Month | Value | Gain/Loss | Gain % | Tickers | Month Change | Month % |\n")
		sb.WriteString("|-------|-------|-----------|--------|---------|--------------|--------|\n")
		for i, p := range points {
			monthChange := ""
			monthChangePct := ""
			if i > 0 {
				mc := p.TotalValue - points[i-1].TotalValue
				monthChange = formatSignedMoney(mc)
				if prev := points[i-1].TotalValue; prev > 0 {
					mcPct := (mc / prev) * 100
					monthChangePct = formatSignedPct(mcPct)
				}
			}
			sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %d | %s | %s |\n",
				p.Date.Format("2006-01-02"), formatMoney(p.TotalValue),
				formatSignedMoney(p.GainLoss), formatSignedPct(p.GainLossPct),
				p.HoldingCount, monthChange, monthChangePct))
		}
	}

	sb.WriteString("\n")
	return sb.String()
}

func formatHistoryJSON(points []models.GrowthDataPoint) string {
	type jsonPoint struct {
		Date            string  `json:"date"`
		Value           float64 `json:"value"`
		Cost            float64 `json:"cost"`
		Gain            float64 `json:"gain"`
		GainPct         float64 `json:"gain_pct"`
		Holdings        int     `json:"holding_count"`
		PeriodChange    float64 `json:"period_change"`
		PeriodChangePct float64 `json:"period_change_pct"`
	}

	out := make([]jsonPoint, len(points))
	for i, p := range points {
		var periodChange, periodChangePct float64
		if i > 0 {
			periodChange = p.TotalValue - points[i-1].TotalValue
			if prev := points[i-1].TotalValue; prev > 0 {
				periodChangePct = (periodChange / prev) * 100
			}
		}
		out[i] = jsonPoint{
			Date:            p.Date.Format("2006-01-02"),
			Value:           p.TotalValue,
			Cost:            p.TotalCost,
			Gain:            p.GainLoss,
			GainPct:         p.GainLossPct,
			Holdings:        p.HoldingCount,
			PeriodChange:    periodChange,
			PeriodChangePct: periodChangePct,
		}
	}

	data, _ := json.Marshal(out)
	return string(data)
}

func formatSnipeBuys(snipeBuys []*models.SnipeBuy, exchange string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Strategy Scanner: Entry Criteria Matches (%s)\n\n", exchange))
	sb.WriteString(fmt.Sprintf("**Scan Date:** %s\n\n", time.Now().Format("2006-01-02 15:04")))

	if len(snipeBuys) == 0 {
		sb.WriteString("No candidates matching criteria found.\n")
		return sb.String()
	}

	for i, snipe := range snipeBuys {
		sb.WriteString(fmt.Sprintf("## %d. %s - %s\n\n", i+1, snipe.Ticker, snipe.Name))
		sb.WriteString(fmt.Sprintf("**Score:** %.0f/100 | **Sector:** %s\n\n", snipe.Score*100, snipe.Sector))
		sb.WriteString("| Current Price | Target Price | Upside |\n")
		sb.WriteString("|---------------|--------------|--------|\n")
		sb.WriteString(fmt.Sprintf("| $%.2f | $%.2f | %.1f%% |\n\n", snipe.Price, snipe.TargetPrice, snipe.UpsidePct))

		if len(snipe.Reasons) > 0 {
			sb.WriteString("**Entry Criteria Matched:**\n")
			for _, reason := range snipe.Reasons {
				sb.WriteString(fmt.Sprintf("- %s\n", reason))
			}
			sb.WriteString("\n")
		}

		if len(snipe.RiskFactors) > 0 {
			sb.WriteString("**Risk Factors:**\n")
			for _, risk := range snipe.RiskFactors {
				sb.WriteString(fmt.Sprintf("- %s\n", risk))
			}
			sb.WriteString("\n")
		}

		if snipe.Analysis != "" {
			sb.WriteString("**Analysis:**\n")
			sb.WriteString(snipe.Analysis)
			sb.WriteString("\n\n")
		}

		sb.WriteString("---\n\n")
	}

	return sb.String()
}

func formatStockData(data *models.StockData) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# %s - %s\n\n", data.Ticker, data.Name))

	if data.Fundamentals != nil {
		if data.Fundamentals.Sector != "" || data.Fundamentals.Industry != "" {
			sb.WriteString(fmt.Sprintf("**Sector:** %s | **Industry:** %s\n\n", data.Fundamentals.Sector, data.Fundamentals.Industry))
		}
		if data.Fundamentals.Description != "" && data.Fundamentals.Description != "NA" {
			sb.WriteString(data.Fundamentals.Description + "\n\n")
		}
	}

	if data.Price != nil {
		sb.WriteString("## Price\n\n")
		sb.WriteString("| Metric | Value |\n")
		sb.WriteString("|--------|-------|\n")
		sb.WriteString(fmt.Sprintf("| Current | $%.2f |\n", data.Price.Current))
		sb.WriteString(fmt.Sprintf("| Change | $%.2f (%.2f%%) |\n", data.Price.Change, data.Price.ChangePct))
		sb.WriteString(fmt.Sprintf("| Open | $%.2f |\n", data.Price.Open))
		sb.WriteString(fmt.Sprintf("| High | $%.2f |\n", data.Price.High))
		sb.WriteString(fmt.Sprintf("| Low | $%.2f |\n", data.Price.Low))
		sb.WriteString(fmt.Sprintf("| Volume | %d |\n", data.Price.Volume))
		sb.WriteString(fmt.Sprintf("| Avg Volume | %d |\n", data.Price.AvgVolume))
		sb.WriteString(fmt.Sprintf("| 52-Week High | $%.2f |\n", data.Price.High52Week))
		sb.WriteString(fmt.Sprintf("| 52-Week Low | $%.2f |\n", data.Price.Low52Week))
		sb.WriteString("\n")
	}

	if data.Fundamentals != nil {
		f := data.Fundamentals
		sb.WriteString("## Fundamentals\n\n")

		// Valuation
		sb.WriteString("### Valuation\n\n")
		sb.WriteString("| Metric | Value |\n")
		sb.WriteString("|--------|-------|\n")
		sb.WriteString(fmt.Sprintf("| Market Cap | %s |\n", formatMarketCap(f.MarketCap)))
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
			sb.WriteString("### Profitability\n\n")
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
			sb.WriteString("### Growth & Scale\n\n")
			sb.WriteString("| Metric | Value |\n")
			sb.WriteString("|--------|-------|\n")
			if f.RevenueTTM != 0 {
				sb.WriteString(fmt.Sprintf("| Revenue TTM | %s |\n", formatMarketCap(f.RevenueTTM)))
			}
			if f.GrossProfitTTM != 0 {
				sb.WriteString(fmt.Sprintf("| Gross Profit TTM | %s |\n", formatMarketCap(f.GrossProfitTTM)))
			}
			if f.EBITDA != 0 {
				sb.WriteString(fmt.Sprintf("| EBITDA | %s |\n", formatMarketCap(f.EBITDA)))
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
			sb.WriteString("### Estimates\n\n")
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

		// Analyst Ratings (from fundamentals, if available)
		if f.AnalystRatings != nil {
			ar := f.AnalystRatings
			sb.WriteString("### Analyst Consensus\n\n")
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

	if data.Signals != nil {
		sb.WriteString("## Technical Signals\n\n")
		sb.WriteString(fmt.Sprintf("**Trend:** %s - %s\n\n", formatTrend(data.Signals.Trend), formatTrendDescription(data.Signals.TrendDescription)))

		sb.WriteString("### Moving Averages\n\n")
		sb.WriteString("| SMA | Value | Distance |\n")
		sb.WriteString("|-----|-------|----------|\n")
		sb.WriteString(fmt.Sprintf("| SMA20 | $%.2f | %.2f%% |\n", data.Signals.Price.SMA20, data.Signals.Price.DistanceToSMA20))
		sb.WriteString(fmt.Sprintf("| SMA50 | $%.2f | %.2f%% |\n", data.Signals.Price.SMA50, data.Signals.Price.DistanceToSMA50))
		sb.WriteString(fmt.Sprintf("| SMA200 | $%.2f | %.2f%% |\n", data.Signals.Price.SMA200, data.Signals.Price.DistanceToSMA200))
		sb.WriteString("\n")

		sb.WriteString("### Indicators\n\n")
		sb.WriteString("| Indicator | Value | Signal |\n")
		sb.WriteString("|-----------|-------|--------|\n")
		sb.WriteString(fmt.Sprintf("| RSI | %.2f | %s |\n", data.Signals.Technical.RSI, data.Signals.Technical.RSISignal))
		macdCrossover := data.Signals.Technical.MACDCrossover
		switch macdCrossover {
		case "bullish":
			macdCrossover = "positive"
		case "bearish":
			macdCrossover = "negative"
		}
		sb.WriteString(fmt.Sprintf("| MACD | %.4f | %s |\n", data.Signals.Technical.MACD, macdCrossover))
		sb.WriteString(fmt.Sprintf("| Volume | %.2fx | %s |\n", data.Signals.Technical.VolumeRatio, data.Signals.Technical.VolumeSignal))
		sb.WriteString(fmt.Sprintf("| ATR | $%.2f (%.2f%%) | - |\n", data.Signals.Technical.ATR, data.Signals.Technical.ATRPct))
		sb.WriteString("\n")

		sb.WriteString("### Advanced Signals\n\n")
		sb.WriteString("| Signal | Score | Interpretation |\n")
		sb.WriteString("|--------|-------|----------------|\n")
		sb.WriteString(fmt.Sprintf("| PBAS | %.2f | %s |\n", data.Signals.PBAS.Score, data.Signals.PBAS.Interpretation))
		sb.WriteString(fmt.Sprintf("| VLI | %.2f | %s |\n", data.Signals.VLI.Score, data.Signals.VLI.Interpretation))
		sb.WriteString(fmt.Sprintf("| Regime | - | %s |\n", data.Signals.Regime.Current))
		sb.WriteString(fmt.Sprintf("| RS | %.2f | %s |\n", data.Signals.RS.Score, data.Signals.RS.Interpretation))
		sb.WriteString("\n")

		if len(data.Signals.RiskFlags) > 0 {
			sb.WriteString("### Risk Flags\n\n")
			for _, flag := range data.Signals.RiskFlags {
				sb.WriteString(fmt.Sprintf("- %s\n", flag))
			}
			sb.WriteString("\n")
		}
	}

	if len(data.News) > 0 {
		sb.WriteString("## Recent News\n\n")
		for _, news := range data.News {
			sb.WriteString(fmt.Sprintf("- **%s** (%s)\n", news.Title, news.PublishedAt.Format("Jan 2")))
		}
		sb.WriteString("\n")
	}

	if data.NewsIntelligence != nil {
		sb.WriteString("## News Intelligence\n\n")
		sb.WriteString(fmt.Sprintf("**Sentiment:** %s\n\n", data.NewsIntelligence.OverallSentiment))
		sb.WriteString(data.NewsIntelligence.Summary + "\n\n")

		if len(data.NewsIntelligence.KeyThemes) > 0 {
			sb.WriteString("**Key Themes:** ")
			sb.WriteString(strings.Join(data.NewsIntelligence.KeyThemes, ", "))
			sb.WriteString("\n\n")
		}

		sb.WriteString("### Impact Assessment\n\n")
		sb.WriteString("| Timeframe | Outlook |\n")
		sb.WriteString("|-----------|----------|\n")
		sb.WriteString(fmt.Sprintf("| This Week | %s |\n", data.NewsIntelligence.ImpactWeek))
		sb.WriteString(fmt.Sprintf("| This Month | %s |\n", data.NewsIntelligence.ImpactMonth))
		sb.WriteString(fmt.Sprintf("| This Year | %s |\n", data.NewsIntelligence.ImpactYear))
		sb.WriteString("\n")
	}

	// Layer 2: Company Releases (per-filing summaries)
	if len(data.FilingSummaries) > 0 {
		sb.WriteString(formatCompanyReleases(data.FilingSummaries))
	}

	// Layer 3: Company Timeline
	if data.Timeline != nil {
		sb.WriteString(formatCompanyTimeline(data.Timeline))
	}

	if len(data.Filings) > 0 {
		sb.WriteString("## Recent Announcements\n\n")
		sb.WriteString("| Date | Headline | Type | Relevance |\n")
		sb.WriteString("|------|----------|------|-----------|\n")
		shown := 0
		for _, f := range data.Filings {
			if shown >= 10 {
				break
			}
			if f.Relevance == "HIGH" || f.Relevance == "MEDIUM" {
				sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
					f.Date.Format("2006-01-02"), f.Headline, f.Type, f.Relevance))
				shown++
			}
		}
		sb.WriteString("\n")
	}

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
			fs.Date.Format("2006-01-02"), truncate(fs.Headline, 40), fs.Type,
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

// truncate shortens a string to max length with ellipsis
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func formatSignals(signals []*models.TickerSignals) string {
	var sb strings.Builder

	sb.WriteString("# Computed Indicators\n\n")
	sb.WriteString(fmt.Sprintf("**Tickers Analyzed:** %d\n\n", len(signals)))

	for _, sig := range signals {
		sb.WriteString(fmt.Sprintf("## %s\n\n", sig.Ticker))
		sb.WriteString(fmt.Sprintf("**Trend:** %s\n", formatTrend(sig.Trend)))
		sb.WriteString(fmt.Sprintf("**Computed:** %s\n\n", sig.ComputeTimestamp.Format("2006-01-02 15:04")))

		sb.WriteString("| Signal | Value | Status |\n")
		sb.WriteString("|--------|-------|--------|\n")
		sb.WriteString(fmt.Sprintf("| RSI | %.2f | %s |\n", sig.Technical.RSI, sig.Technical.RSISignal))
		sb.WriteString(fmt.Sprintf("| Volume | %.2fx | %s |\n", sig.Technical.VolumeRatio, sig.Technical.VolumeSignal))
		sb.WriteString(fmt.Sprintf("| SMA20 Cross | - | %s |\n", sig.Technical.SMA20CrossSMA50))
		sb.WriteString(fmt.Sprintf("| Price vs SMA200 | %.2f%% | %s |\n", sig.Price.DistanceToSMA200, sig.Technical.PriceCrossSMA200))
		sb.WriteString(fmt.Sprintf("| PBAS | %.2f | %s |\n", sig.PBAS.Score, sig.PBAS.Interpretation))
		sb.WriteString(fmt.Sprintf("| VLI | %.2f | %s |\n", sig.VLI.Score, sig.VLI.Interpretation))
		sb.WriteString(fmt.Sprintf("| Regime | - | %s |\n", sig.Regime.Current))
		sb.WriteString("\n")

		if len(sig.RiskFlags) > 0 {
			sb.WriteString("**Risk Flags:** ")
			sb.WriteString(strings.Join(sig.RiskFlags, ", "))
			sb.WriteString("\n")
		}
		sb.WriteString("\n---\n\n")
	}

	return sb.String()
}

func formatPortfolioList(portfolios []string) string {
	var sb strings.Builder

	sb.WriteString("# Available Portfolios\n\n")
	if len(portfolios) == 0 {
		sb.WriteString("No portfolios found. Use `sync_portfolio` to add portfolios from Navexa.\n")
		return sb.String()
	}

	for i, name := range portfolios {
		sb.WriteString(fmt.Sprintf("%d. **%s**\n", i+1, name))
	}

	return sb.String()
}

func formatSyncResult(portfolio *models.Portfolio) string {
	var sb strings.Builder

	// toAUD converts a holding value from its native currency to AUD.
	toAUD := func(v float64, ccy string) float64 {
		if ccy == "USD" && portfolio.FXRate > 0 {
			return v / portfolio.FXRate
		}
		return v
	}

	sb.WriteString(fmt.Sprintf("# Portfolio Synced: %s\n\n", portfolio.Name))
	sb.WriteString(fmt.Sprintf("**Holdings:** %d\n", len(portfolio.Holdings)))
	sb.WriteString(fmt.Sprintf("**Total Value:** $%.2f\n", portfolio.TotalValue))
	sb.WriteString(fmt.Sprintf("**Currency:** %s\n", portfolio.Currency))
	if portfolio.FXRate > 0 {
		sb.WriteString(fmt.Sprintf("**FX Rate (AUDUSD):** %.4f — USD holdings converted to AUD\n", portfolio.FXRate))
	}
	sb.WriteString(fmt.Sprintf("**Last Synced:** %s\n\n", portfolio.LastSynced.Format("2006-01-02 15:04")))

	sb.WriteString("## Holdings Summary\n\n")
	sb.WriteString("| Ticker | Units | Price | Value | Weight |\n")
	sb.WriteString("|--------|-------|-------|-------|--------|\n")
	for _, h := range portfolio.Holdings {
		sb.WriteString(fmt.Sprintf("| %s | %.0f | %s | %s | %.1f%% |\n",
			h.Ticker, h.Units,
			formatMoney(toAUD(h.CurrentPrice, h.Currency)),
			formatMoney(toAUD(h.MarketValue, h.Currency)), h.Weight))
	}

	return sb.String()
}

func formatCollectResult(tickers []string) string {
	var sb strings.Builder

	sb.WriteString("# Market Data Collection Complete\n\n")
	sb.WriteString(fmt.Sprintf("**Tickers Collected:** %d\n\n", len(tickers)))

	for _, ticker := range tickers {
		sb.WriteString(fmt.Sprintf("- %s\n", ticker))
	}

	sb.WriteString("\nData is now available for analysis with `get_stock_data` or `compute_indicators`.\n")

	return sb.String()
}

func formatStrategyContext(review *models.PortfolioReview, strategy *models.PortfolioStrategy) string {
	if strategy == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Strategy Context\n\n")

	sb.WriteString(fmt.Sprintf("**Strategy v%d** | %s risk", strategy.Version, strategy.RiskAppetite.Level))
	if strategy.TargetReturns.AnnualPct > 0 {
		sb.WriteString(fmt.Sprintf(" | %.1f%% target", strategy.TargetReturns.AnnualPct))
	}
	if strategy.AccountType != "" {
		sb.WriteString(fmt.Sprintf(" | %s account", string(strategy.AccountType)))
	}
	sb.WriteString("\n\n")

	nonCompliant := make([]string, 0)
	for _, hr := range review.HoldingReviews {
		if hr.Compliance != nil && hr.Compliance.Status == models.ComplianceStatusNonCompliant {
			reasons := make([]string, 0)
			for _, r := range hr.Compliance.Reasons {
				if len(r) > 40 {
					r = r[:40] + "..."
				}
				reasons = append(reasons, r)
			}
			nonCompliant = append(nonCompliant, fmt.Sprintf("%s (%s)", hr.Holding.Ticker, strings.Join(reasons, ", ")))
		}
	}

	if len(nonCompliant) > 0 {
		sb.WriteString("Non-compliant: " + strings.Join(nonCompliant, ", ") + "\n\n")
	}

	sb.WriteString("When the user proposes actions deviating from this strategy, challenge with specific data.\n")
	sb.WriteString("Strategy deviations are permitted but must be conscious decisions — do not change the strategy.\n\n")

	return sb.String()
}

func formatFunnelResult(result *models.FunnelResult) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Funnel Screen: %s\n\n", result.Exchange))
	sb.WriteString(fmt.Sprintf("**Scan Date:** %s\n", time.Now().Format("2006-01-02 15:04")))
	if result.Sector != "" {
		sb.WriteString(fmt.Sprintf("**Sector Filter:** %s\n", result.Sector))
	}
	sb.WriteString(fmt.Sprintf("**Total Duration:** %s\n\n", result.Duration.Round(time.Millisecond)))

	sb.WriteString("## Funnel Stages\n\n")
	sb.WriteString("| Stage | Input | Output | Duration | Filters |\n")
	sb.WriteString("|-------|-------|--------|----------|---------|\n")
	for i, stage := range result.Stages {
		inputStr := "-"
		if stage.InputCount > 0 {
			inputStr = fmt.Sprintf("%d", stage.InputCount)
		}
		sb.WriteString(fmt.Sprintf("| %d. %s | %s | %d | %s | %s |\n",
			i+1, stage.Name, inputStr, stage.OutputCount,
			stage.Duration.Round(time.Millisecond), stage.Filters))
	}
	sb.WriteString("\n")

	if len(result.Candidates) == 0 {
		sb.WriteString("No candidates survived all funnel stages.\n\n")
		sb.WriteString("**Suggestions:**\n")
		sb.WriteString("- Try a different exchange or sector\n")
		sb.WriteString("- Relax your strategy constraints\n")
		return sb.String()
	}

	sb.WriteString("## Final Candidates\n\n")
	for i, c := range result.Candidates {
		sb.WriteString(fmt.Sprintf("### %d. %s - %s\n\n", i+1, c.Ticker, c.Name))
		sb.WriteString(fmt.Sprintf("**Score:** %.0f/100 | **Sector:** %s | **Industry:** %s\n\n", c.Score*100, c.Sector, c.Industry))

		sb.WriteString("| Metric | Value |\n")
		sb.WriteString("|--------|-------|\n")
		sb.WriteString(fmt.Sprintf("| Price | $%.2f |\n", c.Price))
		sb.WriteString(fmt.Sprintf("| P/E Ratio | %.1f |\n", c.PE))
		sb.WriteString(fmt.Sprintf("| EPS | $%.2f |\n", c.EPS))
		sb.WriteString(fmt.Sprintf("| Market Cap | %s |\n", formatMarketCap(c.MarketCap)))
		sb.WriteString(fmt.Sprintf("| Dividend Yield | %.2f%% |\n", c.DividendYield*100))
		sb.WriteString("\n")

		if len(c.QuarterlyReturns) > 0 {
			sb.WriteString("**Quarterly Returns (annualised):** ")
			parts := make([]string, 0, len(c.QuarterlyReturns))
			for _, r := range c.QuarterlyReturns {
				parts = append(parts, formatSignedPct(r))
			}
			sb.WriteString(strings.Join(parts, " | "))
			sb.WriteString(fmt.Sprintf(" | Avg: **%s**\n\n", formatSignedPct(c.AvgQtrReturn)))
		}

		if len(c.Strengths) > 0 {
			for _, s := range c.Strengths {
				sb.WriteString(fmt.Sprintf("- %s\n", formatTrendDescription(s)))
			}
			sb.WriteString("\n")
		}
		if len(c.Concerns) > 0 {
			for _, con := range c.Concerns {
				sb.WriteString(fmt.Sprintf("- %s\n", formatTrendDescription(con)))
			}
			sb.WriteString("\n")
		}

		if c.Analysis != "" {
			sb.WriteString("**Analysis:**\n")
			sb.WriteString(c.Analysis)
			sb.WriteString("\n\n")
		}

		sb.WriteString("---\n\n")
	}

	return sb.String()
}

func formatSearchList(records []*models.SearchRecord) string {
	var sb strings.Builder

	sb.WriteString("# Search History\n\n")
	if len(records) == 0 {
		sb.WriteString("No search history found.\n\n")
		sb.WriteString("Run `stock_screen`, `strategy_scanner`, or `funnel_screen` to create search records.\n")
		return sb.String()
	}

	sb.WriteString("| ID | Type | Exchange | Results | Date |\n")
	sb.WriteString("|----|------|----------|---------|------|\n")
	for _, r := range records {
		sb.WriteString(fmt.Sprintf("| `%s` | %s | %s | %d | %s |\n",
			r.ID, r.Type, r.Exchange, r.ResultCount,
			r.CreatedAt.Format("2006-01-02 15:04")))
	}
	sb.WriteString("\n")
	sb.WriteString("Use `get_search` with a search ID to recall full results.\n")

	return sb.String()
}

func formatSearchDetail(record *models.SearchRecord) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Search: %s\n\n", record.ID))
	sb.WriteString(fmt.Sprintf("**Type:** %s\n", record.Type))
	sb.WriteString(fmt.Sprintf("**Exchange:** %s\n", record.Exchange))
	sb.WriteString(fmt.Sprintf("**Results:** %d\n", record.ResultCount))
	sb.WriteString(fmt.Sprintf("**Date:** %s\n", record.CreatedAt.Format("2006-01-02 15:04:05")))
	if record.StrategyName != "" {
		sb.WriteString(fmt.Sprintf("**Strategy:** %s v%d\n", record.StrategyName, record.StrategyVer))
	}
	sb.WriteString(fmt.Sprintf("**Filters:** %s\n\n", record.Filters))

	switch record.Type {
	case "screen", "funnel":
		var candidates []*models.ScreenCandidate
		if err := json.Unmarshal([]byte(record.Results), &candidates); err == nil && len(candidates) > 0 {
			sb.WriteString("## Results\n\n")
			for i, c := range candidates {
				sb.WriteString(fmt.Sprintf("### %d. %s - %s\n\n", i+1, c.Ticker, c.Name))
				sb.WriteString(fmt.Sprintf("**Score:** %.0f/100 | **Sector:** %s\n", c.Score*100, c.Sector))
				sb.WriteString(fmt.Sprintf("**Price:** $%.2f | **P/E:** %.1f | **Market Cap:** %s\n\n",
					c.Price, c.PE, formatMarketCap(c.MarketCap)))
				if c.Analysis != "" {
					sb.WriteString(c.Analysis + "\n\n")
				}
				sb.WriteString("---\n\n")
			}
		}

		if record.Stages != "" {
			var stages []models.FunnelStage
			if err := json.Unmarshal([]byte(record.Stages), &stages); err == nil && len(stages) > 0 {
				sb.WriteString("## Funnel Stages\n\n")
				for i, stage := range stages {
					sb.WriteString(fmt.Sprintf("%d. **%s**: %d -> %d (%s)\n",
						i+1, stage.Name, stage.InputCount, stage.OutputCount, stage.Filters))
				}
				sb.WriteString("\n")
			}
		}

	case "snipe":
		var buys []*models.SnipeBuy
		if err := json.Unmarshal([]byte(record.Results), &buys); err == nil && len(buys) > 0 {
			sb.WriteString("## Results\n\n")
			for i, b := range buys {
				sb.WriteString(fmt.Sprintf("### %d. %s - %s\n\n", i+1, b.Ticker, b.Name))
				sb.WriteString(fmt.Sprintf("**Score:** %.0f/100 | **Sector:** %s\n", b.Score*100, b.Sector))
				sb.WriteString(fmt.Sprintf("**Price:** $%.2f | **Target:** $%.2f | **Upside:** %.1f%%\n\n",
					b.Price, b.TargetPrice, b.UpsidePct))
				if b.Analysis != "" {
					sb.WriteString(b.Analysis + "\n\n")
				}
				sb.WriteString("---\n\n")
			}
		}
	}

	return sb.String()
}

func formatScreenCandidates(candidates []*models.ScreenCandidate, exchange string, maxPE, minReturn float64) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Stock Screen: Strategy Filter Results (%s)\n\n", exchange))
	sb.WriteString(fmt.Sprintf("**Scan Date:** %s\n", time.Now().Format("2006-01-02 15:04")))
	sb.WriteString(fmt.Sprintf("**Criteria:** P/E <= %.0f | Quarterly return >= %.0f%% annualised | Positive earnings\n\n", maxPE, minReturn))

	if len(candidates) == 0 {
		sb.WriteString("No candidates matching all criteria found.\n\n")
		sb.WriteString("**Suggestions:**\n")
		sb.WriteString("- Increase `max_pe` to broaden the P/E filter\n")
		sb.WriteString("- Decrease `min_return` to accept lower quarterly returns\n")
		sb.WriteString("- Try a different exchange or sector\n")
		return sb.String()
	}

	for i, c := range candidates {
		sb.WriteString(fmt.Sprintf("## %d. %s - %s\n\n", i+1, c.Ticker, c.Name))
		sb.WriteString(fmt.Sprintf("**Score:** %.0f/100 | **Sector:** %s | **Industry:** %s\n\n", c.Score*100, c.Sector, c.Industry))

		sb.WriteString("| Metric | Value |\n")
		sb.WriteString("|--------|-------|\n")
		sb.WriteString(fmt.Sprintf("| Price | $%.2f |\n", c.Price))
		sb.WriteString(fmt.Sprintf("| P/E Ratio | %.1f |\n", c.PE))
		sb.WriteString(fmt.Sprintf("| EPS | $%.2f |\n", c.EPS))
		sb.WriteString(fmt.Sprintf("| Market Cap | %s |\n", formatMarketCap(c.MarketCap)))
		sb.WriteString(fmt.Sprintf("| Dividend Yield | %.2f%% |\n", c.DividendYield*100))
		sb.WriteString("\n")

		if len(c.QuarterlyReturns) > 0 {
			sb.WriteString("**Quarterly Returns (annualised):**\n\n")
			sb.WriteString("| Quarter | Return |\n")
			sb.WriteString("|---------|--------|\n")
			labels := []string{"Most Recent", "Previous", "Earliest"}
			for j, r := range c.QuarterlyReturns {
				if j < len(labels) {
					sb.WriteString(fmt.Sprintf("| %s | %s |\n", labels[j], formatSignedPct(r)))
				}
			}
			sb.WriteString(fmt.Sprintf("| **Average** | **%s** |\n", formatSignedPct(c.AvgQtrReturn)))
			sb.WriteString("\n")
		}

		if len(c.Strengths) > 0 {
			sb.WriteString("**Strengths:**\n")
			for _, s := range c.Strengths {
				sb.WriteString(fmt.Sprintf("- %s\n", formatTrendDescription(s)))
			}
			sb.WriteString("\n")
		}

		if len(c.Concerns) > 0 {
			sb.WriteString("**Concerns:**\n")
			for _, con := range c.Concerns {
				sb.WriteString(fmt.Sprintf("- %s\n", formatTrendDescription(con)))
			}
			sb.WriteString("\n")
		}

		if c.Analysis != "" {
			sb.WriteString("**Analysis:**\n")
			sb.WriteString(c.Analysis)
			sb.WriteString("\n\n")
		}

		sb.WriteString("---\n\n")
	}

	return sb.String()
}

func formatQuote(q *models.RealTimeQuote) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Quote: %s\n\n", q.Code))

	// Compute staleness at display time using the existing IsFresh pattern
	var dataAge time.Duration
	isStale := false
	if !q.Timestamp.IsZero() {
		dataAge = time.Since(q.Timestamp)
		if dataAge < 0 {
			dataAge = 0
		}
		isStale = !common.IsFresh(q.Timestamp, common.FreshnessRealTimeQuote)
	}

	if isStale {
		sb.WriteString(fmt.Sprintf("**STALE DATA** — price is %s old (as of %s). Market may be closed or data delayed. Verify with a live source.\n\n",
			formatDuration(dataAge), q.Timestamp.Format("2006-01-02 15:04:05")))
	}

	sb.WriteString("| Field | Value |\n")
	sb.WriteString("|-------|-------|\n")
	sb.WriteString(fmt.Sprintf("| Price | %.4f |\n", q.Close))
	if q.ChangePct != 0 {
		sign := ""
		if q.Change > 0 {
			sign = "+"
		}
		sb.WriteString(fmt.Sprintf("| Change | %s%.4f (%s%.2f%%) |\n", sign, q.Change, sign, q.ChangePct))
	}
	if q.PreviousClose != 0 {
		sb.WriteString(fmt.Sprintf("| Prev Close | %.4f |\n", q.PreviousClose))
	}
	sb.WriteString(fmt.Sprintf("| Open | %.4f |\n", q.Open))
	sb.WriteString(fmt.Sprintf("| High | %.4f |\n", q.High))
	sb.WriteString(fmt.Sprintf("| Low | %.4f |\n", q.Low))
	sb.WriteString(fmt.Sprintf("| Volume | %d |\n", q.Volume))
	if !q.Timestamp.IsZero() {
		sb.WriteString(fmt.Sprintf("| Timestamp | %s |\n", q.Timestamp.Format("2006-01-02 15:04:05")))
	}
	if dataAge > 0 {
		sb.WriteString(fmt.Sprintf("| Data Age | %s |\n", formatDuration(dataAge)))
	}
	sb.WriteString("\n")

	return sb.String()
}

// formatDuration renders a duration as a human-readable string (e.g. "2m 15s", "3h 20m").
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		m := int(d.Minutes())
		s := int(d.Seconds()) - m*60
		if s > 0 {
			return fmt.Sprintf("%dm %ds", m, s)
		}
		return fmt.Sprintf("%dm", m)
	}
	h := int(d.Hours())
	m := int(d.Minutes()) - h*60
	if m > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	return fmt.Sprintf("%dh", h)
}

// downsampleToWeekly takes daily data points and returns the last point of each week.
func downsampleToWeekly(points []models.GrowthDataPoint) []models.GrowthDataPoint {
	if len(points) <= 1 {
		return points
	}

	var result []models.GrowthDataPoint
	for i, p := range points {
		if i == len(points)-1 {
			result = append(result, p)
			continue
		}
		next := points[i+1]
		_, thisWeek := p.Date.ISOWeek()
		_, nextWeek := next.Date.ISOWeek()
		if thisWeek != nextWeek {
			result = append(result, p)
		}
	}
	return result
}

// downsampleToMonthly takes daily data points and returns the last point of each month.
func downsampleToMonthly(points []models.GrowthDataPoint) []models.GrowthDataPoint {
	if len(points) <= 1 {
		return points
	}

	var result []models.GrowthDataPoint
	for i, p := range points {
		if i == len(points)-1 {
			result = append(result, p)
			continue
		}
		next := points[i+1]
		if p.Date.Month() != next.Date.Month() || p.Date.Year() != next.Date.Year() {
			result = append(result, p)
		}
	}
	return result
}

// matchTicker checks whether an input ticker matches a holding ticker.
// Handles cases like "SKS" matching "SKS.AU", "BHP.AU" matching "BHP.AU",
// and "BHP.AU" matching "BHP".
func matchTicker(input, holdingTicker string) bool {
	input = strings.ToUpper(input)
	holdingTicker = strings.ToUpper(holdingTicker)

	if input == holdingTicker {
		return true
	}
	// Strip exchange suffix from holding ticker
	if base, _, ok := strings.Cut(holdingTicker, "."); ok && base == input {
		return true
	}
	// Strip exchange suffix from input ticker
	if base, _, ok := strings.Cut(input, "."); ok && base == holdingTicker {
		return true
	}
	return false
}

// formatPortfolioStock formats a single holding's position data and trade history as markdown.
func formatPortfolioStock(h *models.Holding, p *models.Portfolio) string {
	var sb strings.Builder

	// toAUD converts a holding value from its native currency to AUD.
	toAUD := func(v float64, ccy string) float64 {
		if ccy == "USD" && p.FXRate > 0 {
			return v / p.FXRate
		}
		return v
	}

	sb.WriteString(fmt.Sprintf("# %s — %s\n\n", h.Ticker, h.Name))
	sb.WriteString(fmt.Sprintf("**Portfolio:** %s\n", p.Name))
	if h.Units == 0 {
		sb.WriteString("**Status:** Position Closed\n")
	}
	if h.Currency != "" && h.Currency != "AUD" {
		sb.WriteString(fmt.Sprintf("**Currency:** %s (converted to AUD at %.4f)\n", h.Currency, p.FXRate))
	}
	sb.WriteString("\n")

	// Position table
	sb.WriteString("## Position\n\n")
	sb.WriteString("| Metric | Value |\n")
	sb.WriteString("|--------|-------|\n")
	sb.WriteString(fmt.Sprintf("| Units | %.2f |\n", h.Units))
	sb.WriteString(fmt.Sprintf("| Avg Cost | %s |\n", formatMoney(toAUD(h.AvgCost, h.Currency))))
	sb.WriteString(fmt.Sprintf("| Current Price | %s |\n", formatMoney(toAUD(h.CurrentPrice, h.Currency))))
	sb.WriteString(fmt.Sprintf("| Market Value | %s |\n", formatMoney(toAUD(h.MarketValue, h.Currency))))
	sb.WriteString(fmt.Sprintf("| Total Cost | %s |\n", formatMoney(toAUD(h.TotalCost, h.Currency))))
	sb.WriteString(fmt.Sprintf("| Weight | %.1f%% |\n", h.Weight))
	sb.WriteString(fmt.Sprintf("| Capital Gain | %s |\n", formatSignedPct(h.CapitalGainPct)))
	sb.WriteString(fmt.Sprintf("| Income Return | %s |\n", formatSignedMoney(toAUD(h.DividendReturn, h.Currency))))
	sb.WriteString(fmt.Sprintf("| Total Return | %s (%s) |\n", formatSignedMoney(toAUD(h.TotalReturnValue, h.Currency)), formatSignedPct(h.TotalReturnPct)))
	sb.WriteString(fmt.Sprintf("| TWRR | %s |\n", formatSignedPct(h.TotalReturnPctTWRR)))
	sb.WriteString("\n")

	// Trade history
	sb.WriteString(formatTradeHistory(*h))

	return sb.String()
}

// formatTradeHistory renders a date-ordered table of buys, sells, and dividends with totals.
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
				formatMoney(t.Price),
				formatMoney(t.Fees),
				formatMoney(t.Units*t.Price),
			))
		case "sell":
			totalSellUnits += t.Units
			totalSellValue += t.Units * t.Price
			totalSellFees += t.Fees
			sb.WriteString(fmt.Sprintf("| %s | %s | %.0f | %s | %s | %s |\n",
				date, tradeType, t.Units,
				formatMoney(t.Price),
				formatMoney(t.Fees),
				formatMoney(t.Units*t.Price),
			))
		case "cost base increase", "cost base decrease":
			sb.WriteString(fmt.Sprintf("| %s | %s | | | | %s |\n",
				date, tradeType, formatSignedMoney(value),
			))
		default:
			sb.WriteString(fmt.Sprintf("| %s | %s | %.0f | %s | %s | %s |\n",
				date, tradeType, t.Units,
				formatMoney(t.Price),
				formatMoney(t.Fees),
				formatMoney(value),
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
			divDate, formatSignedMoney(h.DividendReturn),
		))
	}

	// Totals row
	sb.WriteString(fmt.Sprintf("| | **Total Bought** | **%.0f** | | **%s** | **%s** |\n",
		totalBuyUnits, formatMoney(totalBuyFees), formatMoney(totalBuyCost),
	))
	if totalSellUnits > 0 {
		sb.WriteString(fmt.Sprintf("| | **Total Sold** | **%.0f** | | **%s** | **%s** |\n",
			totalSellUnits, formatMoney(totalSellFees), formatMoney(totalSellValue),
		))
	}
	sb.WriteString(fmt.Sprintf("| | **Capital Gain** | | | | **%s (%s)** |\n",
		formatSignedMoney(h.GainLoss), formatSignedPct(h.CapitalGainPct),
	))
	if h.DividendReturn != 0 {
		sb.WriteString(fmt.Sprintf("| | **Dividends** | | | | **%s** |\n",
			formatSignedMoney(h.DividendReturn),
		))
	}
	sb.WriteString(fmt.Sprintf("| | **Total Return** | | | | **%s (%s)** |\n",
		formatSignedMoney(h.TotalReturnValue), formatSignedPct(h.TotalReturnPct),
	))
	sb.WriteString("\n")

	return sb.String()
}
