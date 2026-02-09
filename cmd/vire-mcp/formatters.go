package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bobmccarthy/vire/internal/common"
	"github.com/bobmccarthy/vire/internal/models"
)

// Delegate to common format helpers
func formatMoney(v float64) string       { return common.FormatMoney(v) }
func formatSignedMoney(v float64) string { return common.FormatSignedMoney(v) }
func formatSignedPct(v float64) string   { return common.FormatSignedPct(v) }

// formatPortfolioReview formats a portfolio review as markdown
func formatPortfolioReview(review *models.PortfolioReview) string {
	var sb strings.Builder

	// Header
	sb.WriteString(fmt.Sprintf("# Portfolio Review: %s\n\n", review.PortfolioName))
	sb.WriteString(fmt.Sprintf("**Date:** %s\n", review.ReviewDate.Format("2006-01-02 15:04")))
	sb.WriteString(fmt.Sprintf("**Total Value:** %s\n", formatMoney(review.TotalValue)))
	sb.WriteString(fmt.Sprintf("**Total Cost:** %s\n", formatMoney(review.TotalCost)))
	sb.WriteString(fmt.Sprintf("**Total Gain:** %s (%s)\n", formatSignedMoney(review.TotalGain), formatSignedPct(review.TotalGainPct)))
	sb.WriteString(fmt.Sprintf("**Day Change:** %s (%s)\n\n", formatSignedMoney(review.DayChange), formatSignedPct(review.DayChangePct)))

	// Separate active holdings from closed, then split active into stocks/ETFs
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

	// Sort all lists by symbol
	sort.Slice(stocks, func(i, j int) bool {
		return stocks[i].Holding.Ticker < stocks[j].Holding.Ticker
	})
	sort.Slice(etfs, func(i, j int) bool {
		return etfs[i].Holding.Ticker < etfs[j].Holding.Ticker
	})
	sort.Slice(closed, func(i, j int) bool {
		return closed[i].Holding.Ticker < closed[j].Holding.Ticker
	})

	// Holdings Section
	sb.WriteString("## Holdings\n\n")

	// Check if any holding has compliance data
	hasCompliance := false
	for _, hr := range review.HoldingReviews {
		if hr.Compliance != nil {
			hasCompliance = true
			break
		}
	}

	// Stocks Table
	if len(stocks) > 0 {
		sb.WriteString("### Stocks\n\n")
		if hasCompliance {
			sb.WriteString("| Symbol | Weight | Avg Buy | Qty | Price | Value | Capital Gain % | Income Return | Total Return | Total Return % | Action | C |\n")
			sb.WriteString("|--------|--------|---------|-----|-------|-------|----------------|---------------|--------------|----------------|--------|---|\n")
		} else {
			sb.WriteString("| Symbol | Weight | Avg Buy | Qty | Price | Value | Capital Gain % | Income Return | Total Return | Total Return % | Action |\n")
			sb.WriteString("|--------|--------|---------|-----|-------|-------|----------------|---------------|--------------|----------------|--------|\n")
		}

		stocksTotal := 0.0
		stocksGain := 0.0
		for _, hr := range stocks {
			h := hr.Holding
			stocksTotal += h.MarketValue
			stocksGain += h.TotalReturnValue
			if hasCompliance {
				sb.WriteString(fmt.Sprintf("| %s | %.1f%% | %s | %.0f | %s | %s | %s | %s | %s | %s | %s | %s |\n",
					h.Ticker, h.Weight, formatMoney(h.AvgCost), h.Units,
					formatMoney(h.CurrentPrice), formatMoney(h.MarketValue),
					formatSignedPct(h.CapitalGainPct), formatSignedMoney(h.DividendReturn),
					formatSignedMoney(h.TotalReturnValue), formatSignedPct(h.TotalReturnPct),
					formatActionWithReason(hr.ActionRequired, hr.ActionReason),
					formatCompliance(hr.Compliance),
				))
			} else {
				sb.WriteString(fmt.Sprintf("| %s | %.1f%% | %s | %.0f | %s | %s | %s | %s | %s | %s | %s |\n",
					h.Ticker, h.Weight, formatMoney(h.AvgCost), h.Units,
					formatMoney(h.CurrentPrice), formatMoney(h.MarketValue),
					formatSignedPct(h.CapitalGainPct), formatSignedMoney(h.DividendReturn),
					formatSignedMoney(h.TotalReturnValue), formatSignedPct(h.TotalReturnPct),
					formatActionWithReason(hr.ActionRequired, hr.ActionReason),
				))
			}
		}
		stocksGainPct := 0.0
		if stocksTotal-stocksGain > 0 {
			stocksGainPct = (stocksGain / (stocksTotal - stocksGain)) * 100
		}
		if hasCompliance {
			sb.WriteString(fmt.Sprintf("| **Stocks Total** | | | | | **%s** | | | **%s** | **%s** | | |\n\n",
				formatMoney(stocksTotal), formatSignedMoney(stocksGain), formatSignedPct(stocksGainPct)))
		} else {
			sb.WriteString(fmt.Sprintf("| **Stocks Total** | | | | | **%s** | | | **%s** | **%s** | |\n\n",
				formatMoney(stocksTotal), formatSignedMoney(stocksGain), formatSignedPct(stocksGainPct)))
		}
	}

	// ETFs Table
	if len(etfs) > 0 {
		sb.WriteString("### ETFs\n\n")
		if hasCompliance {
			sb.WriteString("| Symbol | Weight | Avg Buy | Qty | Price | Value | Capital Gain % | Income Return | Total Return | Total Return % | Action | C |\n")
			sb.WriteString("|--------|--------|---------|-----|-------|-------|----------------|---------------|--------------|----------------|--------|---|\n")
		} else {
			sb.WriteString("| Symbol | Weight | Avg Buy | Qty | Price | Value | Capital Gain % | Income Return | Total Return | Total Return % | Action |\n")
			sb.WriteString("|--------|--------|---------|-----|-------|-------|----------------|---------------|--------------|----------------|--------|\n")
		}

		etfsTotal := 0.0
		etfsGain := 0.0
		for _, hr := range etfs {
			h := hr.Holding
			etfsTotal += h.MarketValue
			etfsGain += h.TotalReturnValue
			if hasCompliance {
				sb.WriteString(fmt.Sprintf("| %s | %.1f%% | %s | %.0f | %s | %s | %s | %s | %s | %s | %s | %s |\n",
					h.Ticker, h.Weight, formatMoney(h.AvgCost), h.Units,
					formatMoney(h.CurrentPrice), formatMoney(h.MarketValue),
					formatSignedPct(h.CapitalGainPct), formatSignedMoney(h.DividendReturn),
					formatSignedMoney(h.TotalReturnValue), formatSignedPct(h.TotalReturnPct),
					formatActionWithReason(hr.ActionRequired, hr.ActionReason),
					formatCompliance(hr.Compliance),
				))
			} else {
				sb.WriteString(fmt.Sprintf("| %s | %.1f%% | %s | %.0f | %s | %s | %s | %s | %s | %s | %s |\n",
					h.Ticker, h.Weight, formatMoney(h.AvgCost), h.Units,
					formatMoney(h.CurrentPrice), formatMoney(h.MarketValue),
					formatSignedPct(h.CapitalGainPct), formatSignedMoney(h.DividendReturn),
					formatSignedMoney(h.TotalReturnValue), formatSignedPct(h.TotalReturnPct),
					formatActionWithReason(hr.ActionRequired, hr.ActionReason),
				))
			}
		}
		etfsGainPct := 0.0
		if etfsTotal-etfsGain > 0 {
			etfsGainPct = (etfsGain / (etfsTotal - etfsGain)) * 100
		}
		if hasCompliance {
			sb.WriteString(fmt.Sprintf("| **ETFs Total** | | | | | **%s** | | | **%s** | **%s** | | |\n\n",
				formatMoney(etfsTotal), formatSignedMoney(etfsGain), formatSignedPct(etfsGainPct)))
		} else {
			sb.WriteString(fmt.Sprintf("| **ETFs Total** | | | | | **%s** | | | **%s** | **%s** | |\n\n",
				formatMoney(etfsTotal), formatSignedMoney(etfsGain), formatSignedPct(etfsGainPct)))
		}
	}

	// Closed Positions table (same format as stocks/ETFs)
	if len(closed) > 0 {
		sb.WriteString("### Closed Positions\n\n")
		sb.WriteString("| Symbol | Weight | Avg Buy | Qty | Price | Value | Capital Gain % | Income Return | Total Return | Total Return % | Action |\n")
		sb.WriteString("|--------|--------|---------|-----|-------|-------|----------------|---------------|--------------|----------------|--------|\n")

		closedGain := 0.0
		for _, hr := range closed {
			h := hr.Holding
			closedGain += h.TotalReturnValue
			sb.WriteString(fmt.Sprintf("| %s | %.1f%% | %s | %.0f | %s | %s | %s | %s | %s | %s | %s |\n",
				h.Ticker,
				h.Weight,
				formatMoney(h.AvgCost),
				h.Units,
				formatMoney(h.CurrentPrice),
				formatMoney(h.MarketValue),
				formatSignedPct(h.CapitalGainPct),
				formatSignedMoney(h.DividendReturn),
				formatSignedMoney(h.TotalReturnValue),
				formatSignedPct(h.TotalReturnPct),
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
		sb.WriteString(fmt.Sprintf("| **Closed Total** | | | | | | | | **%s** | **%s** | |\n\n",
			formatSignedMoney(closedGain),
			formatSignedPct(closedGainPct),
		))
	}

	// Grand total row
	sb.WriteString(fmt.Sprintf("**Portfolio Total:** %s | **Total Return:** %s (%s)\n\n",
		formatMoney(review.TotalValue),
		formatSignedMoney(review.TotalGain),
		formatSignedPct(review.TotalGainPct),
	))

	// Portfolio Balance section
	if review.PortfolioBalance != nil {
		sb.WriteString("## Portfolio Balance\n\n")
		pb := review.PortfolioBalance

		// Sector allocation table
		sb.WriteString("### Sector Allocation\n\n")
		sb.WriteString("| Sector | Weight | Holdings |\n")
		sb.WriteString("|--------|--------|----------|\n")
		for _, sa := range pb.SectorAllocations {
			sb.WriteString(fmt.Sprintf("| %s | %.1f%% | %s |\n",
				sa.Sector, sa.Weight, strings.Join(sa.Holdings, ", ")))
		}
		sb.WriteString("\n")

		// Style breakdown
		sb.WriteString("### Portfolio Style\n\n")
		sb.WriteString("| Style | Weight |\n")
		sb.WriteString("|-------|--------|\n")
		sb.WriteString(fmt.Sprintf("| Defensive | %.1f%% |\n", pb.DefensiveWeight))
		sb.WriteString(fmt.Sprintf("| Growth | %.1f%% |\n", pb.GrowthWeight))
		sb.WriteString(fmt.Sprintf("| Income (>4%% yield) | %.1f%% |\n", pb.IncomeWeight))
		sb.WriteString("\n")

		// Risk and analysis
		sb.WriteString(fmt.Sprintf("**Concentration Risk:** %s\n\n", pb.ConcentrationRisk))
		sb.WriteString(fmt.Sprintf("**Analysis:** %s\n\n", pb.DiversificationNote))
	}

	// Strategy Compliance section (non-compliant holdings with reasons)
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
				sb.WriteString(fmt.Sprintf("| %s | ❌ Non-compliant | %s |\n",
					hr.Holding.Ticker, strings.Join(hr.Compliance.Reasons, "; ")))
			}
			sb.WriteString("\n")
		}
	}

	// AI Summary
	if review.Summary != "" {
		sb.WriteString("## Summary\n\n")
		sb.WriteString(review.Summary)
		sb.WriteString("\n\n")
	}

	// Consolidated Alerts and Recommendations at the end
	if len(review.Alerts) > 0 || len(review.Recommendations) > 0 {
		sb.WriteString("## Alerts & Recommendations\n\n")

		// Alerts
		if len(review.Alerts) > 0 {
			sb.WriteString("### Alerts\n\n")
			for _, alert := range review.Alerts {
				icon := "ℹ️"
				if alert.Severity == "high" {
					icon = "🔴"
				} else if alert.Severity == "medium" {
					icon = "🟡"
				}
				sb.WriteString(fmt.Sprintf("- %s **%s**: %s\n", icon, alert.Ticker, alert.Message))
			}
			sb.WriteString("\n")
		}

		// Recommendations
		if len(review.Recommendations) > 0 {
			sb.WriteString("### Recommendations\n\n")
			for i, rec := range review.Recommendations {
				sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, rec))
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// formatPortfolioHoldings formats portfolio holdings as markdown (no signals, AI, or charts)
// Used by the fast get_portfolio tool
func formatPortfolioHoldings(p *models.Portfolio) string {
	var sb strings.Builder

	// Header
	sb.WriteString(fmt.Sprintf("# Portfolio: %s\n\n", p.Name))
	sb.WriteString(fmt.Sprintf("**Total Value:** %s\n", formatMoney(p.TotalValue)))
	sb.WriteString(fmt.Sprintf("**Total Cost:** %s\n", formatMoney(p.TotalCost)))
	sb.WriteString(fmt.Sprintf("**Total Gain:** %s (%s)\n", formatSignedMoney(p.TotalGain), formatSignedPct(p.TotalGainPct)))
	sb.WriteString(fmt.Sprintf("**Last Synced:** %s\n\n", p.LastSynced.Format("2006-01-02 15:04")))

	// Separate active and closed holdings
	var active, closed []models.Holding
	for _, h := range p.Holdings {
		if h.Units > 0 {
			active = append(active, h)
		} else {
			closed = append(closed, h)
		}
	}

	// Sort by ticker
	sort.Slice(active, func(i, j int) bool { return active[i].Ticker < active[j].Ticker })
	sort.Slice(closed, func(i, j int) bool { return closed[i].Ticker < closed[j].Ticker })

	// Active Holdings table
	if len(active) > 0 {
		sb.WriteString("## Holdings\n\n")
		sb.WriteString("| Symbol | Name | Units | Avg Cost | Price | Value | Weight | Gain | Gain % |\n")
		sb.WriteString("|--------|------|-------|----------|-------|-------|--------|------|--------|\n")

		for _, h := range active {
			name := h.Name
			if len(name) > 25 {
				name = name[:22] + "..."
			}
			sb.WriteString(fmt.Sprintf("| %s | %s | %.2f | %s | %s | %s | %.1f%% | %s | %s |\n",
				h.Ticker, name, h.Units,
				formatMoney(h.AvgCost), formatMoney(h.CurrentPrice),
				formatMoney(h.MarketValue), h.Weight,
				formatSignedMoney(h.GainLoss), formatSignedPct(h.GainLossPct),
			))
		}
		sb.WriteString("\n")
	}

	// Closed positions table
	if len(closed) > 0 {
		sb.WriteString("## Closed Positions\n\n")
		sb.WriteString("| Symbol | Name | Realized Gain | Gain % |\n")
		sb.WriteString("|--------|------|---------------|--------|\n")

		for _, h := range closed {
			name := h.Name
			if len(name) > 25 {
				name = name[:22] + "..."
			}
			sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
				h.Ticker, name, formatSignedMoney(h.GainLoss), formatSignedPct(h.GainLossPct),
			))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func formatMarketCap(v float64) string    { return common.FormatMarketCap(v) }
func isETF(hr *models.HoldingReview) bool { return common.IsETF(hr) }

// formatAction formats the action required with emoji
func formatAction(action string) string {
	switch action {
	case "SELL":
		return "🔴 SELL"
	case "BUY":
		return "🟢 BUY"
	case "WATCH":
		return "🟡 WATCH"
	case "CLOSED":
		return "⬛ CLOSED"
	case "ALERT":
		return "🟠 ALERT"
	default:
		return "⚪ HOLD"
	}
}

// formatActionWithReason formats action + reason for table display
func formatActionWithReason(action, reason string) string {
	base := formatAction(action)
	if reason == "" || reason == "No significant signals" {
		return base
	}
	return base + ": " + reason
}

// formatCompliance formats compliance status for table display
func formatCompliance(compliance *models.ComplianceResult) string {
	if compliance == nil {
		return "—"
	}
	switch compliance.Status {
	case models.ComplianceStatusCompliant:
		return "✅"
	case models.ComplianceStatusNonCompliant:
		return "❌"
	default:
		return "—"
	}
}

// formatPortfolioSnapshot formats a historical portfolio snapshot as markdown
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
			h.Ticker,
			h.Name,
			h.Units,
			formatMoney(h.AvgCost),
			formatMoney(h.ClosePrice),
			formatMoney(h.MarketValue),
			formatSignedMoney(h.GainLoss),
			formatSignedPct(h.GainLossPct),
			h.Weight,
		))
	}

	sb.WriteString(fmt.Sprintf("| **Total** | | | | | **%s** | **%s** | **%s** | |\n",
		formatMoney(snapshot.TotalValue),
		formatSignedMoney(snapshot.TotalGain),
		formatSignedPct(snapshot.TotalGainPct),
	))

	return sb.String()
}

// formatPortfolioGrowth formats growth data points as a markdown table.
// If chartURL is non-empty, it appends a line with the chart image URL.
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
			p.Date.Format("Jan 2006"),
			formatMoney(p.TotalValue),
			formatSignedMoney(p.GainLoss),
			formatSignedPct(p.GainLossPct),
			p.HoldingCount,
		))
	}

	sb.WriteString("\n")
	if chartURL != "" {
		sb.WriteString(fmt.Sprintf("_Chart: %s_\n\n", chartURL))
	}
	return sb.String()
}

// formatPortfolioHistory formats growth data as markdown with period summary.
// granularity controls the table format: "daily", "weekly", or "monthly".
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
		sb.WriteString("| Date | Value | Gain/Loss | Gain % | Tickers | Day Change |\n")
		sb.WriteString("|------|-------|-----------|--------|---------|------------|\n")

		for i, p := range points {
			dayChange := ""
			if i > 0 {
				dc := p.TotalValue - points[i-1].TotalValue
				dayChange = formatSignedMoney(dc)
			}
			sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %d | %s |\n",
				p.Date.Format("2006-01-02"),
				formatMoney(p.TotalValue),
				formatSignedMoney(p.GainLoss),
				formatSignedPct(p.GainLossPct),
				p.HoldingCount,
				dayChange,
			))
		}

	case "weekly":
		sb.WriteString("| Week Ending | Value | Gain/Loss | Gain % | Tickers | Week Change |\n")
		sb.WriteString("|-------------|-------|-----------|--------|---------|-------------|\n")

		for i, p := range points {
			weekChange := ""
			if i > 0 {
				wc := p.TotalValue - points[i-1].TotalValue
				weekChange = formatSignedMoney(wc)
			}
			sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %d | %s |\n",
				p.Date.Format("2006-01-02"),
				formatMoney(p.TotalValue),
				formatSignedMoney(p.GainLoss),
				formatSignedPct(p.GainLossPct),
				p.HoldingCount,
				weekChange,
			))
		}

	case "monthly":
		sb.WriteString("| Month | Value | Gain/Loss | Gain % | Tickers | Month Change |\n")
		sb.WriteString("|-------|-------|-----------|--------|---------|--------------|")
		sb.WriteString("\n")

		for i, p := range points {
			monthChange := ""
			if i > 0 {
				mc := p.TotalValue - points[i-1].TotalValue
				monthChange = formatSignedMoney(mc)
			}
			sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %d | %s |\n",
				p.Date.Format("2006-01-02"),
				formatMoney(p.TotalValue),
				formatSignedMoney(p.GainLoss),
				formatSignedPct(p.GainLossPct),
				p.HoldingCount,
				monthChange,
			))
		}
	}

	sb.WriteString("\n")
	return sb.String()
}

// formatHistoryJSON serializes growth data points to a JSON array string.
func formatHistoryJSON(points []models.GrowthDataPoint) string {
	type jsonPoint struct {
		Date     string  `json:"date"`
		Value    float64 `json:"value"`
		Cost     float64 `json:"cost"`
		Gain     float64 `json:"gain"`
		GainPct  float64 `json:"gain_pct"`
		Holdings int     `json:"holding_count"`
	}

	out := make([]jsonPoint, len(points))
	for i, p := range points {
		out[i] = jsonPoint{
			Date:     p.Date.Format("2006-01-02"),
			Value:    p.TotalValue,
			Cost:     p.TotalCost,
			Gain:     p.GainLoss,
			GainPct:  p.GainLossPct,
			Holdings: p.HoldingCount,
		}
	}

	data, _ := json.Marshal(out)
	return string(data)
}

// formatSnipeBuys formats snipe buy results as markdown
func formatSnipeBuys(snipeBuys []*models.SnipeBuy, exchange string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Market Snipe: Top Turnaround Candidates (%s)\n\n", exchange))
	sb.WriteString(fmt.Sprintf("**Scan Date:** %s\n\n", time.Now().Format("2006-01-02 15:04")))

	if len(snipeBuys) == 0 {
		sb.WriteString("No candidates matching criteria found.\n")
		return sb.String()
	}

	for i, snipe := range snipeBuys {
		sb.WriteString(fmt.Sprintf("## %d. %s - %s\n\n", i+1, snipe.Ticker, snipe.Name))
		sb.WriteString(fmt.Sprintf("**Score:** %.0f/100 | **Sector:** %s\n\n", snipe.Score*100, snipe.Sector))
		sb.WriteString(fmt.Sprintf("| Current Price | Target Price | Upside |\n"))
		sb.WriteString(fmt.Sprintf("|---------------|--------------|--------|\n"))
		sb.WriteString(fmt.Sprintf("| $%.2f | $%.2f | %.1f%% |\n\n", snipe.Price, snipe.TargetPrice, snipe.UpsidePct))

		if len(snipe.Reasons) > 0 {
			sb.WriteString("**Bullish Signals:**\n")
			for _, reason := range snipe.Reasons {
				sb.WriteString(fmt.Sprintf("- ✅ %s\n", reason))
			}
			sb.WriteString("\n")
		}

		if len(snipe.RiskFactors) > 0 {
			sb.WriteString("**Risk Factors:**\n")
			for _, risk := range snipe.RiskFactors {
				sb.WriteString(fmt.Sprintf("- ⚠️ %s\n", risk))
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

// formatStockData formats stock data as markdown
func formatStockData(data *models.StockData) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# %s - %s\n\n", data.Ticker, data.Name))

	// Sector/Industry and About (from fundamentals)
	if data.Fundamentals != nil {
		if data.Fundamentals.Sector != "" || data.Fundamentals.Industry != "" {
			sb.WriteString(fmt.Sprintf("**Sector:** %s | **Industry:** %s\n\n", data.Fundamentals.Sector, data.Fundamentals.Industry))
		}
		if data.Fundamentals.Description != "" && data.Fundamentals.Description != "NA" {
			sb.WriteString(data.Fundamentals.Description + "\n\n")
		}
	}

	// Price Data
	if data.Price != nil {
		sb.WriteString("## Price\n\n")
		sb.WriteString(fmt.Sprintf("| Metric | Value |\n"))
		sb.WriteString(fmt.Sprintf("|--------|-------|\n"))
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

	// Fundamentals
	if data.Fundamentals != nil {
		sb.WriteString("## Fundamentals\n\n")
		sb.WriteString(fmt.Sprintf("| Metric | Value |\n"))
		sb.WriteString(fmt.Sprintf("|--------|-------|\n"))
		sb.WriteString(fmt.Sprintf("| Market Cap | $%.2fM |\n", data.Fundamentals.MarketCap/1000000))
		sb.WriteString(fmt.Sprintf("| P/E Ratio | %.2f |\n", data.Fundamentals.PE))
		sb.WriteString(fmt.Sprintf("| P/B Ratio | %.2f |\n", data.Fundamentals.PB))
		sb.WriteString(fmt.Sprintf("| EPS | $%.2f |\n", data.Fundamentals.EPS))
		sb.WriteString(fmt.Sprintf("| Dividend Yield | %.2f%% |\n", data.Fundamentals.DividendYield*100))
		sb.WriteString(fmt.Sprintf("| Beta | %.2f |\n", data.Fundamentals.Beta))
		sb.WriteString("\n")
	}

	// Signals
	if data.Signals != nil {
		sb.WriteString("## Technical Signals\n\n")
		sb.WriteString(fmt.Sprintf("**Trend:** %s - %s\n\n", data.Signals.Trend, data.Signals.TrendDescription))

		sb.WriteString("### Moving Averages\n\n")
		sb.WriteString(fmt.Sprintf("| SMA | Value | Distance |\n"))
		sb.WriteString(fmt.Sprintf("|-----|-------|----------|\n"))
		sb.WriteString(fmt.Sprintf("| SMA20 | $%.2f | %.2f%% |\n", data.Signals.Price.SMA20, data.Signals.Price.DistanceToSMA20))
		sb.WriteString(fmt.Sprintf("| SMA50 | $%.2f | %.2f%% |\n", data.Signals.Price.SMA50, data.Signals.Price.DistanceToSMA50))
		sb.WriteString(fmt.Sprintf("| SMA200 | $%.2f | %.2f%% |\n", data.Signals.Price.SMA200, data.Signals.Price.DistanceToSMA200))
		sb.WriteString("\n")

		sb.WriteString("### Indicators\n\n")
		sb.WriteString(fmt.Sprintf("| Indicator | Value | Signal |\n"))
		sb.WriteString(fmt.Sprintf("|-----------|-------|--------|\n"))
		sb.WriteString(fmt.Sprintf("| RSI | %.2f | %s |\n", data.Signals.Technical.RSI, data.Signals.Technical.RSISignal))
		sb.WriteString(fmt.Sprintf("| MACD | %.4f | %s |\n", data.Signals.Technical.MACD, data.Signals.Technical.MACDCrossover))
		sb.WriteString(fmt.Sprintf("| Volume | %.2fx | %s |\n", data.Signals.Technical.VolumeRatio, data.Signals.Technical.VolumeSignal))
		sb.WriteString(fmt.Sprintf("| ATR | $%.2f (%.2f%%) | - |\n", data.Signals.Technical.ATR, data.Signals.Technical.ATRPct))
		sb.WriteString("\n")

		// Advanced Signals
		sb.WriteString("### Advanced Signals\n\n")
		sb.WriteString(fmt.Sprintf("| Signal | Score | Interpretation |\n"))
		sb.WriteString(fmt.Sprintf("|--------|-------|----------------|\n"))
		sb.WriteString(fmt.Sprintf("| PBAS | %.2f | %s |\n", data.Signals.PBAS.Score, data.Signals.PBAS.Interpretation))
		sb.WriteString(fmt.Sprintf("| VLI | %.2f | %s |\n", data.Signals.VLI.Score, data.Signals.VLI.Interpretation))
		sb.WriteString(fmt.Sprintf("| Regime | - | %s |\n", data.Signals.Regime.Current))
		sb.WriteString(fmt.Sprintf("| RS | %.2f | %s |\n", data.Signals.RS.Score, data.Signals.RS.Interpretation))
		sb.WriteString("\n")

		// Risk Flags
		if len(data.Signals.RiskFlags) > 0 {
			sb.WriteString("### Risk Flags\n\n")
			for _, flag := range data.Signals.RiskFlags {
				sb.WriteString(fmt.Sprintf("- ⚠️ %s\n", flag))
			}
			sb.WriteString("\n")
		}
	}

	// News
	if len(data.News) > 0 {
		sb.WriteString("## Recent News\n\n")
		for _, news := range data.News {
			sentiment := ""
			if news.Sentiment == "positive" {
				sentiment = " 🟢"
			} else if news.Sentiment == "negative" {
				sentiment = " 🔴"
			}
			sb.WriteString(fmt.Sprintf("- **%s**%s (%s)\n", news.Title, sentiment, news.PublishedAt.Format("Jan 2")))
		}
		sb.WriteString("\n")
	}

	// News Intelligence
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

		if len(data.NewsIntelligence.Articles) > 0 {
			sb.WriteString("### Sources\n\n")
			for _, a := range data.NewsIntelligence.Articles {
				credIcon := "✅"
				switch a.Credibility {
				case "fluff":
					credIcon = "🗑️"
				case "promotional":
					credIcon = "📢"
				case "speculative":
					credIcon = "❓"
				}
				sb.WriteString(fmt.Sprintf("- %s [%s](%s) (%s) — %s\n", credIcon, a.Title, a.URL, a.Source, a.Summary))
			}
			sb.WriteString("\n")
		}
	}

	// Company Filings Intelligence
	if data.FilingsIntelligence != nil {
		sb.WriteString(formatFilingsIntelligenceMCP(data.FilingsIntelligence))
	}

	// Recent Filings (top 10 HIGH/MEDIUM)
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
				ps := ""
				if f.PriceSensitive {
					ps = " ⚡"
				}
				sb.WriteString(fmt.Sprintf("| %s | %s%s | %s | %s |\n",
					f.Date.Format("2006-01-02"), f.Headline, ps, f.Type, f.Relevance))
				shown++
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// formatFilingsIntelligenceMCP formats filings intelligence for MCP output (with emojis).
func formatFilingsIntelligenceMCP(intel *models.FilingsIntelligence) string {
	var sb strings.Builder

	sb.WriteString("## Company Filings Intelligence\n\n")

	// Health and outlook badges
	healthIcon := "⚪"
	switch intel.FinancialHealth {
	case "strong":
		healthIcon = "🟢"
	case "stable":
		healthIcon = "🔵"
	case "concerning":
		healthIcon = "🟡"
	case "weak":
		healthIcon = "🔴"
	}

	outlookIcon := "⚪"
	switch intel.GrowthOutlook {
	case "positive":
		outlookIcon = "📈"
	case "negative":
		outlookIcon = "📉"
	case "neutral":
		outlookIcon = "➡️"
	}

	sb.WriteString(fmt.Sprintf("**Financial Health:** %s %s | **Growth Outlook:** %s %s\n\n",
		healthIcon, intel.FinancialHealth, outlookIcon, intel.GrowthOutlook))

	// 10% Growth Assessment — prominent
	growthIcon := "❌"
	if intel.CanSupport10PctPA {
		growthIcon = "✅"
	}
	sb.WriteString(fmt.Sprintf("### 10%% Annual Growth Assessment: %s\n\n", growthIcon))
	sb.WriteString(intel.GrowthRationale + "\n\n")

	// Executive Summary
	sb.WriteString("### Summary\n\n")
	sb.WriteString(intel.Summary + "\n\n")

	// Key Metrics
	if len(intel.KeyMetrics) > 0 {
		sb.WriteString("### Key Metrics\n\n")
		sb.WriteString("| Metric | Value | Period | Trend |\n")
		sb.WriteString("|--------|-------|--------|-------|\n")
		for _, m := range intel.KeyMetrics {
			trendIcon := "➡️"
			switch m.Trend {
			case "up":
				trendIcon = "📈"
			case "down":
				trendIcon = "📉"
			}
			sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n", m.Name, m.Value, m.Period, trendIcon))
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

	// Strategy
	if intel.StrategyNotes != "" {
		sb.WriteString("### Strategy\n\n")
		sb.WriteString(intel.StrategyNotes + "\n\n")
	}

	// Positive Factors
	if len(intel.PositiveFactors) > 0 {
		sb.WriteString("### Positive Factors\n\n")
		for _, f := range intel.PositiveFactors {
			sb.WriteString(fmt.Sprintf("- ✅ %s\n", f))
		}
		sb.WriteString("\n")
	}

	// Risk Factors
	if len(intel.RiskFactors) > 0 {
		sb.WriteString("### Risk Factors\n\n")
		for _, f := range intel.RiskFactors {
			sb.WriteString(fmt.Sprintf("- ⚠️ %s\n", f))
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("*Based on %d filings analyzed | Generated %s*\n\n",
		intel.FilingsAnalyzed, intel.GeneratedAt.Format("2006-01-02")))

	return sb.String()
}

// formatSignals formats signal detection results as markdown
func formatSignals(signals []*models.TickerSignals) string {
	var sb strings.Builder

	sb.WriteString("# Signal Detection Results\n\n")
	sb.WriteString(fmt.Sprintf("**Tickers Analyzed:** %d\n\n", len(signals)))

	for _, sig := range signals {
		sb.WriteString(fmt.Sprintf("## %s\n\n", sig.Ticker))
		sb.WriteString(fmt.Sprintf("**Trend:** %s\n", sig.Trend))
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

// formatPortfolioList formats the portfolio list as markdown
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

// formatSyncResult formats sync results as markdown
func formatSyncResult(portfolio *models.Portfolio) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Portfolio Synced: %s\n\n", portfolio.Name))
	sb.WriteString(fmt.Sprintf("**Holdings:** %d\n", len(portfolio.Holdings)))
	sb.WriteString(fmt.Sprintf("**Total Value:** $%.2f\n", portfolio.TotalValue))
	sb.WriteString(fmt.Sprintf("**Currency:** %s\n", portfolio.Currency))
	sb.WriteString(fmt.Sprintf("**Last Synced:** %s\n\n", portfolio.LastSynced.Format("2006-01-02 15:04")))

	sb.WriteString("## Holdings Summary\n\n")
	sb.WriteString("| Ticker | Units | Price | Value | Weight |\n")
	sb.WriteString("|--------|-------|-------|-------|--------|\n")

	for _, h := range portfolio.Holdings {
		sb.WriteString(fmt.Sprintf("| %s | %.0f | $%.2f | $%.2f | %.1f%% |\n",
			h.Ticker, h.Units, h.CurrentPrice, h.MarketValue, h.Weight))
	}

	return sb.String()
}

// formatCollectResult formats collection results as markdown
func formatCollectResult(tickers []string) string {
	var sb strings.Builder

	sb.WriteString("# Market Data Collection Complete\n\n")
	sb.WriteString(fmt.Sprintf("**Tickers Collected:** %d\n\n", len(tickers)))

	for _, ticker := range tickers {
		sb.WriteString(fmt.Sprintf("- ✅ %s\n", ticker))
	}

	sb.WriteString("\nData is now available for analysis with `get_stock_data` or `detect_signals`.\n")

	return sb.String()
}

// formatStrategyContext generates the strategy context section for Claude's adversarial use
func formatStrategyContext(review *models.PortfolioReview, strategy *models.PortfolioStrategy) string {
	if strategy == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Strategy Context\n\n")

	// Strategy summary line
	sb.WriteString(fmt.Sprintf("**Strategy v%d** | %s risk", strategy.Version, strategy.RiskAppetite.Level))
	if strategy.TargetReturns.AnnualPct > 0 {
		sb.WriteString(fmt.Sprintf(" | %.1f%% target", strategy.TargetReturns.AnnualPct))
	}
	if strategy.AccountType != "" {
		sb.WriteString(fmt.Sprintf(" | %s account", string(strategy.AccountType)))
	}
	sb.WriteString("\n\n")

	// Non-compliant holdings summary
	nonCompliant := make([]string, 0)
	for _, hr := range review.HoldingReviews {
		if hr.Compliance != nil && hr.Compliance.Status == models.ComplianceStatusNonCompliant {
			reasons := make([]string, 0)
			for _, r := range hr.Compliance.Reasons {
				// Shorten for summary
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

// formatFunnelResult formats a funnel screen result as markdown
func formatFunnelResult(result *models.FunnelResult) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Funnel Screen: %s\n\n", result.Exchange))
	sb.WriteString(fmt.Sprintf("**Scan Date:** %s\n", time.Now().Format("2006-01-02 15:04")))
	if result.Sector != "" {
		sb.WriteString(fmt.Sprintf("**Sector Filter:** %s\n", result.Sector))
	}
	sb.WriteString(fmt.Sprintf("**Total Duration:** %s\n\n", result.Duration.Round(time.Millisecond)))

	// Stage summary table
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

	// Final candidates
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
				sb.WriteString(fmt.Sprintf("- %s\n", s))
			}
			sb.WriteString("\n")
		}
		if len(c.Concerns) > 0 {
			for _, con := range c.Concerns {
				sb.WriteString(fmt.Sprintf("- %s\n", con))
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

	sb.WriteString("> Funnel screen results based on EODHD screener data, fundamentals, and technical analysis. Past performance does not guarantee future results.\n")

	return sb.String()
}

// formatSearchList formats a list of search records as markdown
func formatSearchList(records []*models.SearchRecord) string {
	var sb strings.Builder

	sb.WriteString("# Search History\n\n")

	if len(records) == 0 {
		sb.WriteString("No search history found.\n\n")
		sb.WriteString("Run `stock_screen`, `market_snipe`, or `funnel_screen` to create search records.\n")
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

// formatSearchDetail formats a single search record as markdown
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

	// Decode and display results based on type
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

		// Show funnel stages if available
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

// formatScreenCandidates formats stock screen results as markdown
func formatScreenCandidates(candidates []*models.ScreenCandidate, exchange string, maxPE, minReturn float64) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Stock Screen: Quality-Value Candidates (%s)\n\n", exchange))
	sb.WriteString(fmt.Sprintf("**Scan Date:** %s\n", time.Now().Format("2006-01-02 15:04")))
	sb.WriteString(fmt.Sprintf("**Criteria:** P/E ≤ %.0f | Quarterly return ≥ %.0f%% annualised | Positive earnings | Not story stocks\n\n", maxPE, minReturn))

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

		// Key metrics table
		sb.WriteString("| Metric | Value |\n")
		sb.WriteString("|--------|-------|\n")
		sb.WriteString(fmt.Sprintf("| Price | $%.2f |\n", c.Price))
		sb.WriteString(fmt.Sprintf("| P/E Ratio | %.1f |\n", c.PE))
		sb.WriteString(fmt.Sprintf("| EPS | $%.2f |\n", c.EPS))
		sb.WriteString(fmt.Sprintf("| Market Cap | %s |\n", formatMarketCap(c.MarketCap)))
		sb.WriteString(fmt.Sprintf("| Dividend Yield | %.2f%% |\n", c.DividendYield*100))
		sb.WriteString(fmt.Sprintf("| News Sentiment | %s |\n", c.NewsSentiment))
		sb.WriteString(fmt.Sprintf("| News Credibility | %s |\n", c.NewsCredibility))
		sb.WriteString("\n")

		// Quarterly returns
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

		// Strengths
		if len(c.Strengths) > 0 {
			sb.WriteString("**Strengths:**\n")
			for _, s := range c.Strengths {
				sb.WriteString(fmt.Sprintf("- ✅ %s\n", s))
			}
			sb.WriteString("\n")
		}

		// Concerns
		if len(c.Concerns) > 0 {
			sb.WriteString("**Concerns:**\n")
			for _, con := range c.Concerns {
				sb.WriteString(fmt.Sprintf("- ⚠️ %s\n", con))
			}
			sb.WriteString("\n")
		}

		// AI Analysis
		if c.Analysis != "" {
			sb.WriteString("**Analysis:**\n")
			sb.WriteString(c.Analysis)
			sb.WriteString("\n\n")
		}

		sb.WriteString("---\n\n")
	}

	sb.WriteString("> These are quality-value screens based on historical data and fundamentals. Past performance does not guarantee future results. Always conduct your own due diligence.\n")

	return sb.String()
}
