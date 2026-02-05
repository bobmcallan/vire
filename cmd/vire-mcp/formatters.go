package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/bobmccarthy/vire/internal/models"
)

// formatPortfolioReview formats a portfolio review as markdown
func formatPortfolioReview(review *models.PortfolioReview) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Portfolio Review: %s\n\n", review.PortfolioName))
	sb.WriteString(fmt.Sprintf("**Date:** %s\n", review.ReviewDate.Format("2006-01-02 15:04")))
	sb.WriteString(fmt.Sprintf("**Total Value:** $%.2f\n", review.TotalValue))
	sb.WriteString(fmt.Sprintf("**Day Change:** $%.2f (%.2f%%)\n\n", review.DayChange, review.DayChangePct))

	// AI Summary
	if review.Summary != "" {
		sb.WriteString("## Summary\n\n")
		sb.WriteString(review.Summary)
		sb.WriteString("\n\n")
	}

	// Alerts
	if len(review.Alerts) > 0 {
		sb.WriteString("## Alerts\n\n")
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

	// Holdings Table
	sb.WriteString("## Holdings\n\n")
	sb.WriteString("| Ticker | Price | Change | Weight | Action | Reason |\n")
	sb.WriteString("|--------|-------|--------|--------|--------|--------|\n")

	for _, hr := range review.HoldingReviews {
		action := hr.ActionRequired
		switch action {
		case "SELL":
			action = "🔴 SELL"
		case "BUY":
			action = "🟢 BUY"
		case "WATCH":
			action = "🟡 WATCH"
		default:
			action = "⚪ HOLD"
		}

		sb.WriteString(fmt.Sprintf("| %s | $%.2f | %.2f%% | %.1f%% | %s | %s |\n",
			hr.Holding.Ticker,
			hr.Holding.CurrentPrice,
			hr.OvernightPct,
			hr.Holding.Weight,
			action,
			hr.ActionReason,
		))
	}
	sb.WriteString("\n")

	// Recommendations
	if len(review.Recommendations) > 0 {
		sb.WriteString("## Recommendations\n\n")
		for i, rec := range review.Recommendations {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, rec))
		}
		sb.WriteString("\n")
	}

	return sb.String()
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
		sb.WriteString(fmt.Sprintf("| Sector | %s |\n", data.Fundamentals.Sector))
		sb.WriteString(fmt.Sprintf("| Industry | %s |\n", data.Fundamentals.Industry))
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
	sb.WriteString("| Ticker | Units | Value | Weight |\n")
	sb.WriteString("|--------|-------|-------|--------|\n")

	for _, h := range portfolio.Holdings {
		sb.WriteString(fmt.Sprintf("| %s | %.2f | $%.2f | %.1f%% |\n",
			h.Ticker, h.Units, h.MarketValue, h.Weight))
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
