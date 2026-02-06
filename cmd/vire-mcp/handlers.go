package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/bobmccarthy/vire/internal/common"
	"github.com/bobmccarthy/vire/internal/interfaces"
)

// handleGetVersion implements the get_version tool
func handleGetVersion() server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		result := fmt.Sprintf("Vire MCP Server\nVersion: %s\nBuild: %s\nCommit: %s\nStatus: OK",
			common.GetVersion(), common.GetBuild(), common.GetGitCommit())
		return textResult(result), nil
	}
}

// handlePortfolioReview implements the portfolio_review tool
func handlePortfolioReview(portfolioService interfaces.PortfolioService, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName, err := request.RequireString("portfolio_name")
		if err != nil || portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required"), nil
		}

		focusSignals := request.GetStringSlice("focus_signals", nil)
		includeNews := request.GetBool("include_news", false)

		review, err := portfolioService.ReviewPortfolio(ctx, portfolioName, interfaces.ReviewOptions{
			FocusSignals: focusSignals,
			IncludeNews:  includeNews,
		})
		if err != nil {
			logger.Error().Err(err).Str("portfolio", portfolioName).Msg("Portfolio review failed")
			return errorResult(fmt.Sprintf("Review error: %v", err)), nil
		}

		markdown := formatPortfolioReview(review)
		return textResult(markdown), nil
	}
}

// handleMarketSnipe implements the market_snipe tool
func handleMarketSnipe(marketService interfaces.MarketService, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		exchange, err := request.RequireString("exchange")
		if err != nil || exchange == "" {
			return errorResult("Error: exchange parameter is required"), nil
		}

		limit := request.GetInt("limit", 3)
		if limit > 10 {
			limit = 10
		}

		criteria := request.GetStringSlice("criteria", nil)
		sector := request.GetString("sector", "")

		snipeBuys, err := marketService.FindSnipeBuys(ctx, interfaces.SnipeOptions{
			Exchange: exchange,
			Limit:    limit,
			Criteria: criteria,
			Sector:   sector,
		})
		if err != nil {
			logger.Error().Err(err).Str("exchange", exchange).Msg("Market snipe failed")
			return errorResult(fmt.Sprintf("Snipe error: %v", err)), nil
		}

		markdown := formatSnipeBuys(snipeBuys, exchange)
		return textResult(markdown), nil
	}
}

// handleStockScreen implements the stock_screen tool
func handleStockScreen(marketService interfaces.MarketService, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		exchange, err := request.RequireString("exchange")
		if err != nil || exchange == "" {
			return errorResult("Error: exchange parameter is required"), nil
		}

		limit := request.GetInt("limit", 5)
		if limit > 15 {
			limit = 15
		}

		maxPE := request.GetFloat("max_pe", 20.0)
		minReturn := request.GetFloat("min_return", 10.0)
		sector := request.GetString("sector", "")

		candidates, err := marketService.ScreenStocks(ctx, interfaces.ScreenOptions{
			Exchange:        exchange,
			Limit:           limit,
			MaxPE:           maxPE,
			MinQtrReturnPct: minReturn,
			Sector:          sector,
		})
		if err != nil {
			logger.Error().Err(err).Str("exchange", exchange).Msg("Stock screen failed")
			return errorResult(fmt.Sprintf("Screen error: %v", err)), nil
		}

		markdown := formatScreenCandidates(candidates, exchange, maxPE, minReturn)
		return textResult(markdown), nil
	}
}

// handleGetStockData implements the get_stock_data tool
func handleGetStockData(marketService interfaces.MarketService, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ticker, err := request.RequireString("ticker")
		if err != nil || ticker == "" {
			return errorResult("Error: ticker parameter is required"), nil
		}

		includes := request.GetStringSlice("include", []string{"price", "fundamentals", "signals", "news"})

		include := interfaces.StockDataInclude{}
		for _, inc := range includes {
			switch inc {
			case "price":
				include.Price = true
			case "fundamentals":
				include.Fundamentals = true
			case "signals":
				include.Signals = true
			case "news":
				include.News = true
			}
		}

		// Default to all if nothing specified
		if !include.Price && !include.Fundamentals && !include.Signals && !include.News {
			include = interfaces.StockDataInclude{
				Price:        true,
				Fundamentals: true,
				Signals:      true,
				News:         true,
			}
		}

		stockData, err := marketService.GetStockData(ctx, ticker, include)
		if err != nil {
			logger.Error().Err(err).Str("ticker", ticker).Msg("Get stock data failed")
			return errorResult(fmt.Sprintf("Error getting stock data: %v", err)), nil
		}

		markdown := formatStockData(stockData)
		return textResult(markdown), nil
	}
}

// handleDetectSignals implements the detect_signals tool
func handleDetectSignals(signalService interfaces.SignalService, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tickers := request.GetStringSlice("tickers", nil)
		if len(tickers) == 0 {
			return errorResult("Error: tickers parameter is required"), nil
		}

		signalTypes := request.GetStringSlice("signal_types", nil)

		signals, err := signalService.DetectSignals(ctx, tickers, signalTypes, false)
		if err != nil {
			logger.Error().Err(err).Msg("Detect signals failed")
			return errorResult(fmt.Sprintf("Signal detection error: %v", err)), nil
		}

		markdown := formatSignals(signals)
		return textResult(markdown), nil
	}
}

// handleListPortfolios implements the list_portfolios tool
func handleListPortfolios(portfolioService interfaces.PortfolioService, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolios, err := portfolioService.ListPortfolios(ctx)
		if err != nil {
			logger.Error().Err(err).Msg("List portfolios failed")
			return errorResult(fmt.Sprintf("Error listing portfolios: %v", err)), nil
		}

		markdown := formatPortfolioList(portfolios)
		return textResult(markdown), nil
	}
}

// handleSyncPortfolio implements the sync_portfolio tool
func handleSyncPortfolio(portfolioService interfaces.PortfolioService, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName, err := request.RequireString("portfolio_name")
		if err != nil || portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required"), nil
		}

		force := request.GetBool("force", false)

		portfolio, err := portfolioService.SyncPortfolio(ctx, portfolioName, force)
		if err != nil {
			logger.Error().Err(err).Str("portfolio", portfolioName).Msg("Sync portfolio failed")
			return errorResult(fmt.Sprintf("Sync error: %v", err)), nil
		}

		markdown := formatSyncResult(portfolio)
		return textResult(markdown), nil
	}
}

// handleCollectMarketData implements the collect_market_data tool
func handleCollectMarketData(marketService interfaces.MarketService, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tickers := request.GetStringSlice("tickers", nil)
		if len(tickers) == 0 {
			return errorResult("Error: tickers parameter is required"), nil
		}

		includeNews := request.GetBool("include_news", false)

		err := marketService.CollectMarketData(ctx, tickers, includeNews, false)
		if err != nil {
			logger.Error().Err(err).Msg("Collect market data failed")
			return errorResult(fmt.Sprintf("Collection error: %v", err)), nil
		}

		markdown := formatCollectResult(tickers)
		return textResult(markdown), nil
	}
}

// handleGenerateReport implements the generate_report tool.
// Returns cached report if fresh unless force_refresh is true.
func handleGenerateReport(reportService interfaces.ReportService, storage interfaces.StorageManager, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName, err := request.RequireString("portfolio_name")
		if err != nil || portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required"), nil
		}

		forceRefresh := request.GetBool("force_refresh", false)
		includeNews := request.GetBool("include_news", false)

		// Smart caching: skip regeneration if report is fresh and not forced
		if !forceRefresh {
			existing, err := storage.ReportStorage().GetReport(ctx, portfolioName)
			if err == nil && common.IsFresh(existing.GeneratedAt, common.FreshnessReport) {
				ago := time.Since(existing.GeneratedAt).Round(time.Minute)
				result := fmt.Sprintf("Report is current for %s (generated %s ago)\n\nTickers: %d\nGenerated at: %s\nTickers: %s",
					portfolioName,
					ago,
					len(existing.TickerReports),
					existing.GeneratedAt.Format("2006-01-02 15:04:05"),
					strings.Join(existing.Tickers, ", "),
				)
				return textResult(result), nil
			}
		}

		report, err := reportService.GenerateReport(ctx, portfolioName, interfaces.ReportOptions{
			ForceRefresh: forceRefresh,
			IncludeNews:  includeNews,
		})
		if err != nil {
			logger.Error().Err(err).Str("portfolio", portfolioName).Msg("Report generation failed")
			return errorResult(fmt.Sprintf("Report generation error: %v", err)), nil
		}

		result := fmt.Sprintf("Report generated for %s\n\nTickers: %d\nGenerated at: %s\nTickers: %s",
			portfolioName,
			len(report.TickerReports),
			report.GeneratedAt.Format("2006-01-02 15:04:05"),
			strings.Join(report.Tickers, ", "),
		)
		return textResult(result), nil
	}
}

// handleGenerateTickerReport implements the generate_ticker_report tool
func handleGenerateTickerReport(reportService interfaces.ReportService, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName, err := request.RequireString("portfolio_name")
		if err != nil || portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required"), nil
		}

		ticker, err := request.RequireString("ticker")
		if err != nil || ticker == "" {
			return errorResult("Error: ticker parameter is required"), nil
		}

		report, err := reportService.GenerateTickerReport(ctx, portfolioName, ticker)
		if err != nil {
			logger.Error().Err(err).Str("portfolio", portfolioName).Str("ticker", ticker).Msg("Ticker report generation failed")
			return errorResult(fmt.Sprintf("Ticker report error: %v", err)), nil
		}

		result := fmt.Sprintf("Ticker report regenerated for %s in %s\n\nGenerated at: %s",
			ticker, portfolioName, report.GeneratedAt.Format("2006-01-02 15:04:05"))
		return textResult(result), nil
	}
}

// handleListReports implements the list_reports tool
func handleListReports(storage interfaces.StorageManager, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filterName := request.GetString("portfolio_name", "")

		reports, err := storage.ReportStorage().ListReports(ctx)
		if err != nil {
			return errorResult(fmt.Sprintf("Error listing reports: %v", err)), nil
		}

		if len(reports) == 0 {
			return textResult("No reports available. Use `generate_report` to create one."), nil
		}

		var sb strings.Builder
		sb.WriteString("# Available Reports\n\n")

		for _, name := range reports {
			if filterName != "" && !strings.EqualFold(name, filterName) {
				continue
			}
			report, err := storage.ReportStorage().GetReport(ctx, name)
			if err != nil {
				continue
			}
			sb.WriteString(fmt.Sprintf("- **%s** — Generated: %s — Tickers: %d\n",
				name, report.GeneratedAt.Format("2006-01-02 15:04:05"), len(report.TickerReports)))
		}

		return textResult(sb.String()), nil
	}
}

// handleGetSummary implements the get_summary tool.
// Auto-generates a report if none exists or the cached report is stale.
func handleGetSummary(storage interfaces.StorageManager, reportService interfaces.ReportService, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName, err := request.RequireString("portfolio_name")
		if err != nil || portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required"), nil
		}

		// Try cached report first
		report, err := storage.ReportStorage().GetReport(ctx, portfolioName)
		if err == nil && common.IsFresh(report.GeneratedAt, common.FreshnessReport) {
			return textResult(report.SummaryMarkdown), nil
		}

		// Auto-generate (stale or missing)
		logger.Info().Str("portfolio", portfolioName).Msg("Auto-generating report for get_summary")
		report, err = reportService.GenerateReport(ctx, portfolioName, interfaces.ReportOptions{
			ForceRefresh: false,
			IncludeNews:  false,
		})
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to generate report for '%s': %v", portfolioName, err)), nil
		}

		return textResult(report.SummaryMarkdown), nil
	}
}

// handleGetTickerReport implements the get_ticker_report tool.
// Auto-generates a report if none exists or the cached report is stale.
func handleGetTickerReport(storage interfaces.StorageManager, reportService interfaces.ReportService, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName, err := request.RequireString("portfolio_name")
		if err != nil || portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required"), nil
		}

		ticker, err := request.RequireString("ticker")
		if err != nil || ticker == "" {
			return errorResult("Error: ticker parameter is required"), nil
		}

		// Try cached report first
		report, err := storage.ReportStorage().GetReport(ctx, portfolioName)
		if err != nil || !common.IsFresh(report.GeneratedAt, common.FreshnessReport) {
			// Auto-generate (stale or missing)
			logger.Info().Str("portfolio", portfolioName).Msg("Auto-generating report for get_ticker_report")
			report, err = reportService.GenerateReport(ctx, portfolioName, interfaces.ReportOptions{
				ForceRefresh: false,
				IncludeNews:  false,
			})
			if err != nil {
				return errorResult(fmt.Sprintf("Failed to generate report for '%s': %v", portfolioName, err)), nil
			}
		}

		for _, tr := range report.TickerReports {
			if strings.EqualFold(tr.Ticker, ticker) {
				return textResult(tr.Markdown), nil
			}
		}

		return errorResult(fmt.Sprintf("Ticker '%s' not found in report for '%s'. Available: %s",
			ticker, portfolioName, strings.Join(report.Tickers, ", "))), nil
	}
}

// handleListTickers implements the list_tickers tool
func handleListTickers(storage interfaces.StorageManager, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName, err := request.RequireString("portfolio_name")
		if err != nil || portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required"), nil
		}

		report, err := storage.ReportStorage().GetReport(ctx, portfolioName)
		if err != nil {
			return errorResult(fmt.Sprintf("Report not found for '%s': %v", portfolioName, err)), nil
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("# Ticker Reports: %s\n\n", portfolioName))
		sb.WriteString(fmt.Sprintf("**Generated:** %s\n\n", report.GeneratedAt.Format("2006-01-02 15:04:05")))

		for _, tr := range report.TickerReports {
			typeLabel := "Stock"
			if tr.IsETF {
				typeLabel = "ETF"
			}
			sb.WriteString(fmt.Sprintf("- **%s** — %s (%s)\n", tr.Ticker, tr.Name, typeLabel))
		}

		return textResult(sb.String()), nil
	}
}

// Helper functions

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.NewTextContent(text),
		},
	}
}

func errorResult(message string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.NewTextContent(message),
		},
		IsError: true,
	}
}
