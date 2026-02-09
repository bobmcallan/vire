package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/bobmccarthy/vire/internal/common"
	"github.com/bobmccarthy/vire/internal/interfaces"
	"github.com/bobmccarthy/vire/internal/models"
	"github.com/bobmccarthy/vire/internal/services/portfolio"
)

// validateTicker checks a ticker has an exchange suffix and returns an error message if ambiguous.
func validateTicker(ticker string) (string, string) {
	ticker = strings.ToUpper(strings.TrimSpace(ticker))
	if ticker != "" && !strings.Contains(ticker, ".") {
		return "", fmt.Sprintf("Ambiguous ticker %q — did you mean %s.AU (ASX) or %s.US (NYSE/NASDAQ)? Please include the exchange suffix.", ticker, ticker, ticker)
	}
	return ticker, ""
}

// validateTickers checks all tickers have exchange suffixes. Returns the first ambiguous ticker as an error.
func validateTickers(tickers []string) ([]string, string) {
	for i, t := range tickers {
		normalized, errMsg := validateTicker(t)
		if errMsg != "" {
			return nil, errMsg
		}
		tickers[i] = normalized
	}
	return tickers, ""
}

// resolvePortfolioName resolves portfolio_name from request, falling back to the configured default.
func resolvePortfolioName(ctx context.Context, request mcp.CallToolRequest, kvStorage interfaces.KeyValueStorage, configDefault string) string {
	name := request.GetString("portfolio_name", "")
	if name != "" {
		return name
	}
	return common.ResolveDefaultPortfolio(ctx, kvStorage, configDefault)
}

// handleGetVersion implements the get_version tool
func handleGetVersion() server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		result := fmt.Sprintf("Vire MCP Server\nVersion: %s\nBuild: %s\nCommit: %s\nStatus: OK",
			common.GetVersion(), common.GetBuild(), common.GetGitCommit())
		return textResult(result), nil
	}
}

// handlePortfolioReview implements the portfolio_review tool
func handlePortfolioReview(portfolioService interfaces.PortfolioService, storage interfaces.StorageManager, configDefault string, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioName(ctx, request, storage.KeyValueStorage(), configDefault)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
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

		// Add strategy context section if strategy exists
		if strat, err := storage.StrategyStorage().GetStrategy(ctx, portfolioName); err == nil {
			markdown += formatStrategyContext(review, strat)
		}

		// Compute daily growth once, downsample for table, use daily for chart
		dailyPoints, err := portfolioService.GetDailyGrowth(ctx, portfolioName, interfaces.GrowthOptions{})
		if err != nil {
			logger.Warn().Err(err).Msg("Failed to compute portfolio growth")
		}

		// Warn about tickers skipped due to missing market data
		if p, pErr := storage.PortfolioStorage().GetPortfolio(ctx, portfolioName); pErr == nil {
			var missingTickers []string
			for _, h := range p.Holdings {
				if len(h.Trades) == 0 {
					continue
				}
				ticker := h.Ticker + ".AU"
				md, mdErr := storage.MarketDataStorage().GetMarketData(ctx, ticker)
				if mdErr != nil || md == nil || len(md.EOD) == 0 {
					missingTickers = append(missingTickers, h.Ticker)
				}
			}
			if len(missingTickers) > 0 {
				logger.Warn().
					Strs("tickers", missingTickers).
					Msg("Growth chart excludes tickers with missing market data — run generate_report or collect_market_data to fix")
			}
		}

		content := []mcp.Content{mcp.NewTextContent(markdown)}

		if len(dailyPoints) > 0 {
			monthlyPoints := portfolio.DownsampleToMonthly(dailyPoints)

			if len(dailyPoints) >= 2 {
				pngBytes, err := portfolio.RenderGrowthChart(dailyPoints)
				if err != nil {
					logger.Warn().Err(err).Msg("Failed to render growth chart")
				} else {
					b64 := base64.StdEncoding.EncodeToString(pngBytes)
					content = append(content, mcp.NewImageContent(b64, "image/png"))
				}
			}

			growthMarkdown := formatPortfolioGrowth(monthlyPoints, "")
			content = append(content, mcp.NewTextContent(growthMarkdown))
		}

		return &mcp.CallToolResult{Content: content}, nil
	}
}

// handleGetPortfolioHistory implements the get_portfolio_history tool
func handleGetPortfolioHistory(portfolioService interfaces.PortfolioService, storage interfaces.StorageManager, configDefault string, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioName(ctx, request, storage.KeyValueStorage(), configDefault)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
		}

		opts := interfaces.GrowthOptions{}

		if fromStr := request.GetString("from", ""); fromStr != "" {
			t, err := time.Parse("2006-01-02", fromStr)
			if err != nil {
				return errorResult(fmt.Sprintf("Error: invalid from date '%s' — use YYYY-MM-DD", fromStr)), nil
			}
			opts.From = t
		}

		if toStr := request.GetString("to", ""); toStr != "" {
			t, err := time.Parse("2006-01-02", toStr)
			if err != nil {
				return errorResult(fmt.Sprintf("Error: invalid to date '%s' — use YYYY-MM-DD", toStr)), nil
			}
			opts.To = t
		}

		format := request.GetString("format", "auto")

		points, err := portfolioService.GetDailyGrowth(ctx, portfolioName, opts)
		if err != nil {
			logger.Error().Err(err).Str("portfolio", portfolioName).Msg("Portfolio history failed")
			return errorResult(fmt.Sprintf("History error: %v", err)), nil
		}

		if len(points) == 0 {
			return textResult("No portfolio history data available for the specified date range."), nil
		}

		// Determine granularity and apply downsampling
		granularity := format
		switch format {
		case "auto":
			if len(points) <= 90 {
				granularity = "daily"
			} else {
				granularity = "weekly"
				points = portfolio.DownsampleToWeekly(points)
			}
		case "daily":
			// No downsampling
		case "weekly":
			points = portfolio.DownsampleToWeekly(points)
		case "monthly":
			points = portfolio.DownsampleToMonthly(points)
		default:
			return errorResult(fmt.Sprintf("Error: invalid format '%s' — use 'daily', 'weekly', 'monthly', or 'auto'", format)), nil
		}

		markdown := formatPortfolioHistory(points, granularity)
		jsonData := "<!-- CHART_DATA -->\n" + formatHistoryJSON(points)

		content := []mcp.Content{
			mcp.NewTextContent(markdown),
			mcp.NewTextContent(jsonData),
		}
		return &mcp.CallToolResult{Content: content}, nil
	}
}

// handleMarketSnipe implements the market_snipe tool
func handleMarketSnipe(marketService interfaces.MarketService, storage interfaces.StorageManager, configDefault string, logger *common.Logger) server.ToolHandlerFunc {
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

		// Auto-load portfolio strategy (nil if none exists)
		portfolioName := resolvePortfolioName(ctx, request, storage.KeyValueStorage(), configDefault)
		strategy, _ := storage.StrategyStorage().GetStrategy(ctx, portfolioName)

		includeNews := request.GetBool("include_news", false)

		snipeBuys, err := marketService.FindSnipeBuys(ctx, interfaces.SnipeOptions{
			Exchange:    exchange,
			Limit:       limit,
			Criteria:    criteria,
			Sector:      sector,
			IncludeNews: includeNews,
			Strategy:    strategy,
		})
		if err != nil {
			logger.Error().Err(err).Str("exchange", exchange).Msg("Market snipe failed")
			return errorResult(fmt.Sprintf("Snipe error: %v", err)), nil
		}

		// Auto-save to search history
		searchID := autoSaveSnipeSearch(ctx, storage, snipeBuys, exchange, criteria, sector, strategy, logger)

		markdown := formatSnipeBuys(snipeBuys, exchange)
		if searchID != "" {
			markdown += fmt.Sprintf("\n*Search saved: `%s` — use `get_search` to recall*\n", searchID)
		}
		if strategy != nil {
			strategyNote := fmt.Sprintf("\n---\n*Filtered for your %s", strategy.RiskAppetite.Level)
			if strategy.AccountType != "" {
				strategyNote += " " + string(strategy.AccountType)
			}
			strategyNote += " strategy*\n"
			markdown += strategyNote
		}
		return textResult(markdown), nil
	}
}

// handleStockScreen implements the stock_screen tool
func handleStockScreen(marketService interfaces.MarketService, storage interfaces.StorageManager, configDefault string, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		exchange, err := request.RequireString("exchange")
		if err != nil || exchange == "" {
			return errorResult("Error: exchange parameter is required"), nil
		}

		limit := request.GetInt("limit", 5)
		if limit > 15 {
			limit = 15
		}

		maxPE := request.GetFloat("max_pe", 0) // 0 = let strategy/defaults decide
		minReturn := request.GetFloat("min_return", 0)
		sector := request.GetString("sector", "")

		// Auto-load portfolio strategy (nil if none exists)
		portfolioName := resolvePortfolioName(ctx, request, storage.KeyValueStorage(), configDefault)
		strategy, _ := storage.StrategyStorage().GetStrategy(ctx, portfolioName)

		includeNews := request.GetBool("include_news", false)

		candidates, err := marketService.ScreenStocks(ctx, interfaces.ScreenOptions{
			Exchange:        exchange,
			Limit:           limit,
			MaxPE:           maxPE,
			MinQtrReturnPct: minReturn,
			Sector:          sector,
			IncludeNews:     includeNews,
			Strategy:        strategy,
		})
		if err != nil {
			logger.Error().Err(err).Str("exchange", exchange).Msg("Stock screen failed")
			return errorResult(fmt.Sprintf("Screen error: %v", err)), nil
		}

		// Determine effective maxPE/minReturn for display
		effectiveMaxPE := maxPE
		if effectiveMaxPE <= 0 {
			effectiveMaxPE = 20.0
			if strategy != nil {
				switch strategy.RiskAppetite.Level {
				case "conservative":
					effectiveMaxPE = 15.0
				case "aggressive":
					effectiveMaxPE = 25.0
				}
			}
		}
		effectiveMinReturn := minReturn
		if effectiveMinReturn <= 0 {
			effectiveMinReturn = 10.0
		}

		// Auto-save to search history
		searchID := autoSaveScreenSearch(ctx, storage, candidates, exchange, effectiveMaxPE, effectiveMinReturn, sector, strategy, logger)

		markdown := formatScreenCandidates(candidates, exchange, effectiveMaxPE, effectiveMinReturn)
		if searchID != "" {
			markdown += fmt.Sprintf("\n*Search saved: `%s` — use `get_search` to recall*\n", searchID)
		}
		if strategy != nil {
			strategyNote := fmt.Sprintf("\n---\n*Filtered for your %s", strategy.RiskAppetite.Level)
			if strategy.AccountType != "" {
				strategyNote += " " + string(strategy.AccountType)
			}
			strategyNote += " strategy*\n"
			markdown += strategyNote
		}
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
		ticker, errMsg := validateTicker(ticker)
		if errMsg != "" {
			return errorResult(errMsg), nil
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
		tickers, errMsg := validateTickers(tickers)
		if errMsg != "" {
			return errorResult(errMsg), nil
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
func handleSyncPortfolio(portfolioService interfaces.PortfolioService, storage interfaces.StorageManager, configDefault string, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioName(ctx, request, storage.KeyValueStorage(), configDefault)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
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

// handleRebuildData implements the rebuild_data tool
func handleRebuildData(portfolioService interfaces.PortfolioService, marketService interfaces.MarketService, storage interfaces.StorageManager, configDefault string, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioName(ctx, request, storage.KeyValueStorage(), configDefault)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
		}

		// Step 1: Purge all derived data
		counts, err := storage.PurgeDerivedData(ctx)
		if err != nil {
			logger.Error().Err(err).Msg("Rebuild: purge failed")
			return errorResult(fmt.Sprintf("Rebuild failed during purge: %v", err)), nil
		}

		// Step 2: Update schema version
		if err := storage.KeyValueStorage().Set(ctx, schemaVersionKey, common.SchemaVersion); err != nil {
			logger.Warn().Err(err).Msg("Rebuild: failed to update schema version")
		}

		// Step 3: Re-sync portfolio from Navexa
		p, err := portfolioService.SyncPortfolio(ctx, portfolioName, true)
		if err != nil {
			logger.Error().Err(err).Str("portfolio", portfolioName).Msg("Rebuild: portfolio sync failed")
			return errorResult(fmt.Sprintf("Rebuild: purge succeeded but portfolio sync failed: %v", err)), nil
		}

		// Step 4: Collect market data for all holdings with trades
		tickers := make([]string, 0, len(p.Holdings))
		for _, h := range p.Holdings {
			if len(h.Trades) > 0 {
				tickers = append(tickers, h.Ticker+".AU")
			}
		}

		marketCount := 0
		if len(tickers) > 0 {
			if err := marketService.CollectMarketData(ctx, tickers, false, true); err != nil {
				logger.Warn().Err(err).Msg("Rebuild: market data collection failed")
			} else {
				marketCount = len(tickers)
			}
		}

		// Step 5: Return summary
		var sb strings.Builder
		sb.WriteString("# Data Rebuild Complete\n\n")
		sb.WriteString("## Purged\n\n")
		sb.WriteString(fmt.Sprintf("| Type | Count |\n"))
		sb.WriteString(fmt.Sprintf("|------|-------|\n"))
		sb.WriteString(fmt.Sprintf("| Portfolios | %d |\n", counts["portfolios"]))
		sb.WriteString(fmt.Sprintf("| Market Data | %d |\n", counts["market_data"]))
		sb.WriteString(fmt.Sprintf("| Signals | %d |\n", counts["signals"]))
		sb.WriteString(fmt.Sprintf("| Reports | %d |\n", counts["reports"]))
		sb.WriteString("\n")
		sb.WriteString("## Rebuilt\n\n")
		sb.WriteString(fmt.Sprintf("- Portfolio **%s** synced (%d holdings)\n", portfolioName, len(p.Holdings)))
		sb.WriteString(fmt.Sprintf("- Market data collected for **%d** tickers\n", marketCount))
		sb.WriteString(fmt.Sprintf("- Schema version set to **%s**\n", common.SchemaVersion))
		sb.WriteString("\n*Signals and reports will regenerate lazily on next query.*\n")

		return textResult(sb.String()), nil
	}
}

// handleCollectMarketData implements the collect_market_data tool
func handleCollectMarketData(marketService interfaces.MarketService, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tickers := request.GetStringSlice("tickers", nil)
		if len(tickers) == 0 {
			return errorResult("Error: tickers parameter is required"), nil
		}
		tickers, errMsg := validateTickers(tickers)
		if errMsg != "" {
			return errorResult(errMsg), nil
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
func handleGenerateReport(reportService interfaces.ReportService, storage interfaces.StorageManager, configDefault string, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioName(ctx, request, storage.KeyValueStorage(), configDefault)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
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
func handleGenerateTickerReport(reportService interfaces.ReportService, storage interfaces.StorageManager, configDefault string, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioName(ctx, request, storage.KeyValueStorage(), configDefault)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
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
func handleGetSummary(storage interfaces.StorageManager, reportService interfaces.ReportService, configDefault string, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioName(ctx, request, storage.KeyValueStorage(), configDefault)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
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
func handleGetTickerReport(storage interfaces.StorageManager, reportService interfaces.ReportService, configDefault string, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioName(ctx, request, storage.KeyValueStorage(), configDefault)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
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
func handleListTickers(storage interfaces.StorageManager, configDefault string, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioName(ctx, request, storage.KeyValueStorage(), configDefault)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
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

// handleGetPortfolioSnapshot implements the get_portfolio_snapshot tool
func handleGetPortfolioSnapshot(portfolioService interfaces.PortfolioService, storage interfaces.StorageManager, configDefault string, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioName(ctx, request, storage.KeyValueStorage(), configDefault)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
		}

		dateStr, err := request.RequireString("date")
		if err != nil || dateStr == "" {
			return errorResult("Error: date parameter is required (format: YYYY-MM-DD)"), nil
		}

		asOf, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			return errorResult(fmt.Sprintf("Error: invalid date format '%s' — use YYYY-MM-DD", dateStr)), nil
		}

		if asOf.After(time.Now()) {
			return errorResult("Error: date must be in the past"), nil
		}

		snapshot, err := portfolioService.GetPortfolioSnapshot(ctx, portfolioName, asOf)
		if err != nil {
			logger.Error().Err(err).Str("portfolio", portfolioName).Str("date", dateStr).Msg("Portfolio snapshot failed")
			return errorResult(fmt.Sprintf("Snapshot error: %v", err)), nil
		}

		markdown := formatPortfolioSnapshot(snapshot)
		return textResult(markdown), nil
	}
}

// handleSetDefaultPortfolio implements the set_default_portfolio tool.
// When called without portfolio_name, lists available portfolios with the current default first.
func handleSetDefaultPortfolio(storage interfaces.StorageManager, portfolioService interfaces.PortfolioService, configDefault string, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := request.GetString("portfolio_name", "")

		// If no name provided, list available portfolios
		if portfolioName == "" {
			currentDefault := common.ResolveDefaultPortfolio(ctx, storage.KeyValueStorage(), configDefault)

			portfolios, err := portfolioService.ListPortfolios(ctx)
			if err != nil || len(portfolios) == 0 {
				return errorResult("No portfolios found. Use sync_portfolio to add portfolios from Navexa first."), nil
			}

			// Sort: current default first, then alphabetical
			ordered := make([]string, 0, len(portfolios))
			for _, p := range portfolios {
				if strings.EqualFold(p, currentDefault) {
					ordered = append([]string{p}, ordered...)
				} else {
					ordered = append(ordered, p)
				}
			}

			var sb strings.Builder
			sb.WriteString("# Available Portfolios\n\n")
			if currentDefault != "" {
				sb.WriteString(fmt.Sprintf("**Current default:** %s\n\n", currentDefault))
			} else {
				sb.WriteString("**Current default:** *(not set)*\n\n")
			}
			sb.WriteString("| # | Portfolio | Default |\n")
			sb.WriteString("|---|----------|---------|\n")
			for i, p := range ordered {
				marker := ""
				if strings.EqualFold(p, currentDefault) {
					marker = "**current**"
				}
				sb.WriteString(fmt.Sprintf("| %d | %s | %s |\n", i+1, p, marker))
			}
			sb.WriteString("\nTo set the default, call `set_default_portfolio` with the portfolio_name parameter.")
			return textResult(sb.String()), nil
		}

		// Set the default
		if err := storage.KeyValueStorage().Set(ctx, "default_portfolio", portfolioName); err != nil {
			logger.Error().Err(err).Msg("Failed to set default portfolio")
			return errorResult(fmt.Sprintf("Failed to set default portfolio: %v", err)), nil
		}

		return textResult(fmt.Sprintf("Default portfolio set to **%s**.\n\nTools that accept portfolio_name will now use '%s' when no portfolio is specified.", portfolioName, portfolioName)), nil
	}
}

// handleGetConfig implements the get_config tool
func handleGetConfig(storage interfaces.StorageManager, config *common.Config, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var sb strings.Builder
		sb.WriteString("# Vire Configuration\n\n")

		// Section 1: Runtime (KV store)
		sb.WriteString("## Runtime Settings (KV Store)\n\n")
		sb.WriteString("*Set via MCP tools, highest priority. Persists across restarts.*\n\n")
		sb.WriteString("| Key | Value |\n")
		sb.WriteString("|-----|-------|\n")

		kvStorage := storage.KeyValueStorage()
		kvAll, err := kvStorage.GetAll(ctx)
		if err != nil {
			kvAll = map[string]string{}
		}
		if len(kvAll) == 0 {
			sb.WriteString("| *(none set)* | |\n")
		} else {
			for k, v := range kvAll {
				display := v
				if strings.Contains(k, "api_key") {
					display = maskSecret(v)
				}
				sb.WriteString(fmt.Sprintf("| %s | %s |\n", k, display))
			}
		}
		sb.WriteString("\n")

		// Section 2: Environment Variables
		sb.WriteString("## Environment Variables\n\n")
		sb.WriteString("*Set via shell environment, overrides TOML config.*\n\n")
		sb.WriteString("| Variable | Value |\n")
		sb.WriteString("|----------|-------|\n")
		envVars := []struct{ name, key string }{
			{"VIRE_DEFAULT_PORTFOLIO", "VIRE_DEFAULT_PORTFOLIO"},
			{"VIRE_ENV", "VIRE_ENV"},
			{"VIRE_DATA_PATH", "VIRE_DATA_PATH"},
			{"VIRE_LOG_LEVEL", "VIRE_LOG_LEVEL"},
			{"EODHD_API_KEY", "EODHD_API_KEY"},
			{"NAVEXA_API_KEY", "NAVEXA_API_KEY"},
			{"GEMINI_API_KEY", "GEMINI_API_KEY"},
		}
		anyEnvSet := false
		for _, ev := range envVars {
			val := os.Getenv(ev.key)
			if val != "" {
				display := val
				if strings.Contains(strings.ToLower(ev.name), "api_key") || strings.Contains(strings.ToLower(ev.name), "key") {
					display = maskSecret(val)
				}
				sb.WriteString(fmt.Sprintf("| %s | %s |\n", ev.name, display))
				anyEnvSet = true
			}
		}
		if !anyEnvSet {
			sb.WriteString("| *(none set)* | |\n")
		}
		sb.WriteString("\n")

		// Section 3: Config File (TOML)
		sb.WriteString("## Config File (TOML)\n\n")
		sb.WriteString("*Loaded at startup, lowest priority.*\n\n")
		sb.WriteString("| Setting | Value |\n")
		sb.WriteString("|---------|-------|\n")
		portfoliosStr := "-"
		if len(config.Portfolios) > 0 {
			portfoliosStr = strings.Join(config.Portfolios, ", ")
		}
		sb.WriteString(fmt.Sprintf("| portfolios | %s |\n", portfoliosStr))
		sb.WriteString(fmt.Sprintf("| environment | %s |\n", valueOrDash(config.Environment)))
		sb.WriteString(fmt.Sprintf("| storage.file.path | %s |\n", valueOrDash(config.Storage.File.Path)))
		sb.WriteString(fmt.Sprintf("| storage.file.versions | %d |\n", config.Storage.File.Versions))
		sb.WriteString(fmt.Sprintf("| clients.eodhd.base_url | %s |\n", valueOrDash(config.Clients.EODHD.BaseURL)))
		sb.WriteString(fmt.Sprintf("| clients.eodhd.api_key | %s |\n", maskSecret(config.Clients.EODHD.APIKey)))
		sb.WriteString(fmt.Sprintf("| clients.eodhd.rate_limit | %d |\n", config.Clients.EODHD.RateLimit))
		sb.WriteString(fmt.Sprintf("| clients.navexa.base_url | %s |\n", valueOrDash(config.Clients.Navexa.BaseURL)))
		sb.WriteString(fmt.Sprintf("| clients.navexa.api_key | %s |\n", maskSecret(config.Clients.Navexa.APIKey)))
		sb.WriteString(fmt.Sprintf("| clients.navexa.rate_limit | %d |\n", config.Clients.Navexa.RateLimit))
		sb.WriteString(fmt.Sprintf("| clients.gemini.api_key | %s |\n", maskSecret(config.Clients.Gemini.APIKey)))
		sb.WriteString(fmt.Sprintf("| clients.gemini.model | %s |\n", valueOrDash(config.Clients.Gemini.Model)))
		sb.WriteString(fmt.Sprintf("| logging.level | %s |\n", valueOrDash(config.Logging.Level)))
		sb.WriteString(fmt.Sprintf("| logging.format | %s |\n", valueOrDash(config.Logging.Format)))
		sb.WriteString("\n")

		// Resolved defaults section
		sb.WriteString("## Resolved Defaults\n\n")
		sb.WriteString("*Effective values after resolving KV > env > config priority.*\n\n")
		resolvedPortfolio := common.ResolveDefaultPortfolio(ctx, kvStorage, config.DefaultPortfolio())
		if resolvedPortfolio == "" {
			resolvedPortfolio = "*(not set — will prompt user)*"
		}
		sb.WriteString(fmt.Sprintf("| Setting | Value |\n"))
		sb.WriteString(fmt.Sprintf("|---------|-------|\n"))
		sb.WriteString(fmt.Sprintf("| default_portfolio | %s |\n", resolvedPortfolio))
		sb.WriteString("\n")

		return textResult(sb.String()), nil
	}
}

// maskSecret masks an API key or secret, showing only the first 4 characters.
func maskSecret(s string) string {
	if s == "" {
		return "-"
	}
	if len(s) <= 4 {
		return "****"
	}
	return s[:4] + "****"
}

// valueOrDash returns the string or "-" if empty.
func valueOrDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// handleGetStrategyTemplate implements the get_strategy_template tool
func handleGetStrategyTemplate() server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		accountType := request.GetString("account_type", "")

		var b strings.Builder
		b.WriteString("# Portfolio Strategy Template\n\n")
		b.WriteString("Use `set_portfolio_strategy` with a JSON object containing any of these fields.\n")
		b.WriteString("Only include fields you want to set — unspecified fields keep defaults.\n\n")

		b.WriteString("## Fields\n\n")

		// Account Type
		b.WriteString("### account_type (string)\n")
		b.WriteString("- `\"smsf\"` — Self-managed super fund (Australian)\n")
		b.WriteString("- `\"trading\"` — Standard trading account\n\n")

		// Investment Universe
		b.WriteString("### investment_universe (array of strings)\n")
		b.WriteString("Exchange codes: `[\"AU\", \"US\"]`\n")
		b.WriteString("- `\"AU\"` — Australian Securities Exchange (ASX)\n")
		b.WriteString("- `\"US\"` — US exchanges (NYSE, NASDAQ)\n\n")

		// Risk Appetite
		b.WriteString("### risk_appetite (object)\n")
		b.WriteString("- `level` (string): `\"conservative\"`, `\"moderate\"`, or `\"aggressive\"`\n")
		b.WriteString("- `max_drawdown_pct` (number): Maximum acceptable portfolio drawdown, e.g. `15`\n")
		b.WriteString("- `description` (string): Free-text description of risk tolerance\n\n")
		b.WriteString("**Guidance:**\n")
		b.WriteString("| Level | Typical Return | Max Drawdown | Suits |\n")
		b.WriteString("|-------|---------------|-------------|-------|\n")
		b.WriteString("| Conservative | 5-8% | 5-10% | Capital preservation, near retirement |\n")
		b.WriteString("| Moderate | 8-12% | 10-20% | Balanced growth and income |\n")
		b.WriteString("| Aggressive | 12%+ | 20-40% | Long time horizon, growth focus |\n\n")

		// Target Returns
		b.WriteString("### target_returns (object)\n")
		b.WriteString("- `annual_pct` (number): Annual return target, e.g. `10`\n")
		b.WriteString("- `timeframe` (string): Investment horizon, e.g. `\"3-5 years\"`\n\n")
		b.WriteString("**Reference:** ASX200 long-term average ~10%, S&P500 ~10-12%\n\n")

		// Income Requirements
		b.WriteString("### income_requirements (object)\n")
		b.WriteString("- `dividend_yield_pct` (number): Target portfolio dividend yield, e.g. `4.0`\n")
		b.WriteString("- `description` (string): Income strategy notes\n\n")
		b.WriteString("**Reference:** ASX200 yield ~4%, S&P500 yield ~1.5%\n\n")

		// Sector Preferences
		b.WriteString("### sector_preferences (object)\n")
		b.WriteString("- `preferred` (array): Sectors to favour, e.g. `[\"Financials\", \"Healthcare\"]`\n")
		b.WriteString("- `excluded` (array): Sectors to avoid, e.g. `[\"Gambling\", \"Tobacco\"]`\n\n")

		// Position Sizing
		b.WriteString("### position_sizing (object)\n")
		b.WriteString("- `max_position_pct` (number): Max single stock weight, e.g. `10`\n")
		b.WriteString("- `max_sector_pct` (number): Max sector weight, e.g. `30`\n\n")

		// Reference Strategies
		b.WriteString("### reference_strategies (array of objects)\n")
		b.WriteString("Named investment approaches that guide analysis:\n")
		b.WriteString("- `name` (string): Strategy name\n")
		b.WriteString("- `description` (string): How it influences decisions\n\n")
		b.WriteString("**Examples:** `\"dividend growth\"`, `\"value investing\"`, `\"momentum\"`, `\"index tracking\"`\n\n")

		// Rebalance
		b.WriteString("### rebalance_frequency (string)\n")
		b.WriteString("`\"monthly\"`, `\"quarterly\"`, or `\"annually\"`\n\n")

		// Trading Rules
		b.WriteString("### rules (array of objects)\n")
		b.WriteString("Declarative trading rules evaluated against live signals, fundamentals, and holdings.\n")
		b.WriteString("Each rule has AND-conditions — for OR logic, create multiple rules.\n\n")
		b.WriteString("- `name` (string): Human-readable rule name\n")
		b.WriteString("- `conditions` (array): AND'd conditions, each with `field`, `operator`, `value`\n")
		b.WriteString("- `action` (string): `\"SELL\"`, `\"BUY\"`, `\"WATCH\"`, `\"HOLD\"`, or `\"ALERT\"`\n")
		b.WriteString("- `reason` (string): Template with `{field}` placeholders for actual values\n")
		b.WriteString("- `priority` (number): >0 overrides hardcoded signal logic (priority 0)\n")
		b.WriteString("- `enabled` (boolean): Set false to disable without deleting\n\n")
		b.WriteString("**Available condition fields:**\n\n")
		b.WriteString("| Prefix | Fields |\n")
		b.WriteString("|--------|--------|\n")
		b.WriteString("| `signals.` | `rsi`, `volume_ratio`, `macd`, `macd_histogram`, `atr_pct`, `near_support`, `near_resistance` |\n")
		b.WriteString("| `signals.price.` | `distance_to_sma20`, `distance_to_sma50`, `distance_to_sma200` |\n")
		b.WriteString("| `signals.pbas.` | `score`, `interpretation` |\n")
		b.WriteString("| `signals.vli.` | `score`, `interpretation` |\n")
		b.WriteString("| `signals.regime.` | `current` |\n")
		b.WriteString("| `signals.` | `trend` (bullish/bearish/neutral) |\n")
		b.WriteString("| `fundamentals.` | `pe`, `pb`, `eps`, `dividend_yield`, `beta`, `market_cap`, `sector`, `industry` |\n")
		b.WriteString("| `holding.` | `weight`, `gain_loss_pct`, `total_return_pct`, `capital_gain_pct`, `units`, `market_value` |\n\n")
		b.WriteString("**Operators:** `>`, `>=`, `<`, `<=`, `==`, `!=`, `in`, `not_in`\n\n")
		b.WriteString("**Example rule:**\n")
		b.WriteString("```json\n")
		b.WriteString("{\n")
		b.WriteString("  \"name\": \"Overbought sell\",\n")
		b.WriteString("  \"conditions\": [{\"field\": \"signals.rsi\", \"operator\": \">\", \"value\": 70}],\n")
		b.WriteString("  \"action\": \"SELL\",\n")
		b.WriteString("  \"reason\": \"RSI overbought at {signals.rsi}\",\n")
		b.WriteString("  \"priority\": 1,\n")
		b.WriteString("  \"enabled\": true\n")
		b.WriteString("}\n")
		b.WriteString("```\n\n")

		// Company Filter
		b.WriteString("### company_filter (object)\n")
		b.WriteString("Stock selection criteria applied during screening and compliance checks.\n\n")
		b.WriteString("- `min_market_cap` (number): Minimum market capitalisation, e.g. `1000000000` ($1B)\n")
		b.WriteString("- `max_pe` (number): Maximum P/E ratio, e.g. `25`\n")
		b.WriteString("- `min_dividend_yield` (number): Minimum dividend yield %, e.g. `2.0`\n")
		b.WriteString("- `allowed_sectors` (array): Whitelist of sectors (stricter than sector_preferences)\n")
		b.WriteString("- `excluded_sectors` (array): Blacklist of sectors\n\n")

		// Notes
		b.WriteString("### notes (string)\n")
		b.WriteString("Free-form markdown for anything else. Tax considerations, life events, etc.\n\n")

		// SMSF-specific guidance
		if strings.ToLower(accountType) == "smsf" {
			b.WriteString("---\n\n")
			b.WriteString("## SMSF-Specific Considerations\n\n")
			b.WriteString("- SMSF trustees must maintain a documented investment strategy (SIS Regulation 4.09)\n")
			b.WriteString("- Strategy must consider: risk, return, diversification, liquidity, and ability to pay benefits\n")
			b.WriteString("- Funds in pension phase need sufficient income/liquidity for minimum pension payments\n")
			b.WriteString("- Consider franking credits for tax efficiency (franked dividends refund 30% company tax)\n")
			b.WriteString("- Aggressive strategies may not meet the 'prudent person' test\n\n")
		}

		// Example
		b.WriteString("---\n\n")
		b.WriteString("## Example\n\n")
		b.WriteString("```json\n")
		if strings.ToLower(accountType) == "smsf" {
			b.WriteString(`{
  "account_type": "smsf",
  "investment_universe": ["AU"],
  "risk_appetite": {
    "level": "moderate",
    "max_drawdown_pct": 15,
    "description": "Balanced approach suitable for accumulation phase"
  },
  "target_returns": {
    "annual_pct": 8,
    "timeframe": "5-10 years"
  },
  "income_requirements": {
    "dividend_yield_pct": 4.0,
    "description": "Focus on franked dividends for tax efficiency"
  },
  "sector_preferences": {
    "preferred": ["Financials", "Healthcare", "Utilities"],
    "excluded": ["Gambling"]
  },
  "position_sizing": {
    "max_position_pct": 10,
    "max_sector_pct": 30
  },
  "reference_strategies": [
    {"name": "Dividend Growth", "description": "Companies with growing, sustainable dividends"}
  ],
  "rebalance_frequency": "quarterly"
}`)
		} else {
			b.WriteString(`{
  "account_type": "trading",
  "investment_universe": ["AU", "US"],
  "risk_appetite": {
    "level": "moderate",
    "max_drawdown_pct": 20,
    "description": "Growth-oriented with reasonable risk management"
  },
  "target_returns": {
    "annual_pct": 12,
    "timeframe": "3-5 years"
  },
  "position_sizing": {
    "max_position_pct": 15,
    "max_sector_pct": 35
  },
  "reference_strategies": [
    {"name": "Value Investing", "description": "Buy undervalued companies with strong fundamentals"},
    {"name": "Momentum", "description": "Follow strong price trends with risk management"}
  ],
  "rebalance_frequency": "monthly"
}`)
		}
		b.WriteString("\n```\n")

		return textResult(b.String()), nil
	}
}

// handleSetPortfolioStrategy implements the set_portfolio_strategy tool
func handleSetPortfolioStrategy(strategyService interfaces.StrategyService, storage interfaces.StorageManager, configDefault string, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioName(ctx, request, storage.KeyValueStorage(), configDefault)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
		}

		strategyJSON := request.GetString("strategy_json", "")
		if strategyJSON == "" {
			return errorResult("Error: strategy_json parameter is required. Use get_strategy_template to see available fields."), nil
		}

		// Load existing strategy or create new with defaults
		existing, err := strategyService.GetStrategy(ctx, portfolioName)
		if err != nil {
			// New strategy — start with defaults
			existing = &models.PortfolioStrategy{
				PortfolioName: portfolioName,
			}
		}

		// Merge: unmarshal incoming JSON on top of existing strategy.
		// json.Unmarshal only overwrites fields present in the JSON.
		if err := json.Unmarshal([]byte(strategyJSON), existing); err != nil {
			return errorResult(fmt.Sprintf("Error parsing strategy_json: %v. Use get_strategy_template to see valid fields.", err)), nil
		}

		// Ensure portfolio name is always set correctly (cannot be overridden via JSON)
		existing.PortfolioName = portfolioName

		// Save with validation
		warnings, err := strategyService.SaveStrategy(ctx, existing)
		if err != nil {
			logger.Error().Err(err).Str("portfolio", portfolioName).Msg("Failed to save strategy")
			return errorResult(fmt.Sprintf("Error saving strategy: %v", err)), nil
		}

		// Reload saved strategy to get auto-set fields (version, timestamps, disclaimer)
		saved, err := strategyService.GetStrategy(ctx, portfolioName)
		if err != nil {
			return errorResult(fmt.Sprintf("Strategy saved but failed to reload: %v", err)), nil
		}

		// Build response
		var b strings.Builder
		b.WriteString(saved.ToMarkdown())

		// Devil's advocate warnings
		if len(warnings) > 0 {
			b.WriteString("\n---\n\n")
			b.WriteString("## Devil's Advocate Warnings\n\n")
			for _, w := range warnings {
				icon := "**[" + strings.ToUpper(w.Severity) + "]**"
				b.WriteString(fmt.Sprintf("%s %s\n\n", icon, w.Message))
			}
			b.WriteString("*These warnings highlight potential issues with your strategy. They are challenges to consider, not errors to fix. You may proceed with your strategy as-is if you understand the trade-offs.*\n")
		}

		return textResult(b.String()), nil
	}
}

// handleGetPortfolioStrategy implements the get_portfolio_strategy tool
func handleGetPortfolioStrategy(strategyService interfaces.StrategyService, storage interfaces.StorageManager, configDefault string, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioName(ctx, request, storage.KeyValueStorage(), configDefault)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
		}

		strategy, err := strategyService.GetStrategy(ctx, portfolioName)
		if err != nil {
			return textResult(fmt.Sprintf("No strategy found for portfolio '%s'.\n\n"+
				"Use `set_portfolio_strategy` to create one, or `get_strategy_template` to see available options.",
				portfolioName)), nil
		}

		return textResult(strategy.ToMarkdown()), nil
	}
}

// handleDeletePortfolioStrategy implements the delete_portfolio_strategy tool
func handleDeletePortfolioStrategy(strategyService interfaces.StrategyService, storage interfaces.StorageManager, configDefault string, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioName(ctx, request, storage.KeyValueStorage(), configDefault)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
		}

		err := strategyService.DeleteStrategy(ctx, portfolioName)
		if err != nil {
			logger.Error().Err(err).Str("portfolio", portfolioName).Msg("Failed to delete strategy")
			return errorResult(fmt.Sprintf("Error deleting strategy: %v", err)), nil
		}

		return textResult(fmt.Sprintf("Strategy for portfolio '%s' has been deleted.", portfolioName)), nil
	}
}

// handleGetPortfolioPlan implements the get_portfolio_plan tool
func handleGetPortfolioPlan(planService interfaces.PlanService, storage interfaces.StorageManager, configDefault string, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioName(ctx, request, storage.KeyValueStorage(), configDefault)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
		}

		plan, err := planService.GetPlan(ctx, portfolioName)
		if err != nil {
			return textResult(fmt.Sprintf("No plan found for portfolio '%s'.\n\n"+
				"Use `add_plan_item` to create action items, or `set_portfolio_plan` to set an entire plan.",
				portfolioName)), nil
		}

		return textResult(plan.ToMarkdown()), nil
	}
}

// handleSetPortfolioPlan implements the set_portfolio_plan tool
func handleSetPortfolioPlan(planService interfaces.PlanService, storage interfaces.StorageManager, configDefault string, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioName(ctx, request, storage.KeyValueStorage(), configDefault)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
		}

		planJSON := request.GetString("plan_json", "")
		if planJSON == "" {
			return errorResult("Error: plan_json parameter is required."), nil
		}

		plan := &models.PortfolioPlan{
			PortfolioName: portfolioName,
		}
		if err := json.Unmarshal([]byte(planJSON), plan); err != nil {
			return errorResult(fmt.Sprintf("Error parsing plan_json: %v", err)), nil
		}

		plan.PortfolioName = portfolioName

		if err := planService.SavePlan(ctx, plan); err != nil {
			logger.Error().Err(err).Str("portfolio", portfolioName).Msg("Failed to save plan")
			return errorResult(fmt.Sprintf("Error saving plan: %v", err)), nil
		}

		// Reload to get version/timestamps
		saved, err := planService.GetPlan(ctx, portfolioName)
		if err != nil {
			return errorResult(fmt.Sprintf("Plan saved but failed to reload: %v", err)), nil
		}

		return textResult(saved.ToMarkdown()), nil
	}
}

// handleAddPlanItem implements the add_plan_item tool
func handleAddPlanItem(planService interfaces.PlanService, storage interfaces.StorageManager, configDefault string, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioName(ctx, request, storage.KeyValueStorage(), configDefault)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
		}

		itemJSON := request.GetString("item_json", "")
		if itemJSON == "" {
			return errorResult("Error: item_json parameter is required."), nil
		}

		var item models.PlanItem
		if err := json.Unmarshal([]byte(itemJSON), &item); err != nil {
			return errorResult(fmt.Sprintf("Error parsing item_json: %v", err)), nil
		}

		plan, err := planService.AddPlanItem(ctx, portfolioName, &item)
		if err != nil {
			logger.Error().Err(err).Str("portfolio", portfolioName).Msg("Failed to add plan item")
			return errorResult(fmt.Sprintf("Error adding plan item: %v", err)), nil
		}

		return textResult(plan.ToMarkdown()), nil
	}
}

// handleUpdatePlanItem implements the update_plan_item tool
func handleUpdatePlanItem(planService interfaces.PlanService, storage interfaces.StorageManager, configDefault string, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioName(ctx, request, storage.KeyValueStorage(), configDefault)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
		}

		itemID, err := request.RequireString("item_id")
		if err != nil || itemID == "" {
			return errorResult("Error: item_id parameter is required"), nil
		}

		itemJSON := request.GetString("item_json", "")
		if itemJSON == "" {
			return errorResult("Error: item_json parameter is required."), nil
		}

		var update models.PlanItem
		if err := json.Unmarshal([]byte(itemJSON), &update); err != nil {
			return errorResult(fmt.Sprintf("Error parsing item_json: %v", err)), nil
		}

		plan, err := planService.UpdatePlanItem(ctx, portfolioName, itemID, &update)
		if err != nil {
			logger.Error().Err(err).Str("portfolio", portfolioName).Str("item_id", itemID).Msg("Failed to update plan item")
			return errorResult(fmt.Sprintf("Error updating plan item: %v", err)), nil
		}

		return textResult(plan.ToMarkdown()), nil
	}
}

// handleRemovePlanItem implements the remove_plan_item tool
func handleRemovePlanItem(planService interfaces.PlanService, storage interfaces.StorageManager, configDefault string, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioName(ctx, request, storage.KeyValueStorage(), configDefault)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
		}

		itemID, err := request.RequireString("item_id")
		if err != nil || itemID == "" {
			return errorResult("Error: item_id parameter is required"), nil
		}

		plan, err := planService.RemovePlanItem(ctx, portfolioName, itemID)
		if err != nil {
			logger.Error().Err(err).Str("portfolio", portfolioName).Str("item_id", itemID).Msg("Failed to remove plan item")
			return errorResult(fmt.Sprintf("Error removing plan item: %v", err)), nil
		}

		return textResult(plan.ToMarkdown()), nil
	}
}

// handleCheckPlanStatus implements the check_plan_status tool
func handleCheckPlanStatus(planService interfaces.PlanService, storage interfaces.StorageManager, configDefault string, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioName(ctx, request, storage.KeyValueStorage(), configDefault)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
		}

		// Check events
		triggered, err := planService.CheckPlanEvents(ctx, portfolioName)
		if err != nil {
			logger.Error().Err(err).Str("portfolio", portfolioName).Msg("Failed to check plan events")
			return errorResult(fmt.Sprintf("Error checking plan events: %v", err)), nil
		}

		// Check deadlines
		expired, err := planService.CheckPlanDeadlines(ctx, portfolioName)
		if err != nil {
			logger.Error().Err(err).Str("portfolio", portfolioName).Msg("Failed to check plan deadlines")
			return errorResult(fmt.Sprintf("Error checking plan deadlines: %v", err)), nil
		}

		var b strings.Builder
		b.WriteString(fmt.Sprintf("# Plan Status: %s\n\n", portfolioName))

		if len(triggered) > 0 {
			b.WriteString("## Triggered Events\n\n")
			for _, item := range triggered {
				b.WriteString(fmt.Sprintf("- **[%s]** %s", item.ID, item.Description))
				if item.Ticker != "" {
					b.WriteString(fmt.Sprintf(" | Ticker: %s", item.Ticker))
				}
				if item.Action != "" {
					b.WriteString(fmt.Sprintf(" | Action: %s", string(item.Action)))
				}
				b.WriteString("\n")
			}
			b.WriteString("\n")
		}

		if len(expired) > 0 {
			b.WriteString("## Expired Deadlines\n\n")
			for _, item := range expired {
				b.WriteString(fmt.Sprintf("- **[%s]** %s", item.ID, item.Description))
				if item.Deadline != nil {
					b.WriteString(fmt.Sprintf(" (was due %s)", item.Deadline.Format("2006-01-02")))
				}
				b.WriteString("\n")
			}
			b.WriteString("\n")
		}

		if len(triggered) == 0 && len(expired) == 0 {
			b.WriteString("No triggered events or expired deadlines.\n\n")
		}

		// Show full plan
		plan, err := planService.GetPlan(ctx, portfolioName)
		if err == nil {
			b.WriteString("---\n\n")
			b.WriteString(plan.ToMarkdown())
		}

		return textResult(b.String()), nil
	}
}

// handleFunnelScreen implements the funnel_screen tool
func handleFunnelScreen(marketService interfaces.MarketService, storage interfaces.StorageManager, configDefault string, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		exchange, err := request.RequireString("exchange")
		if err != nil || exchange == "" {
			return errorResult("Error: exchange parameter is required"), nil
		}

		limit := request.GetInt("limit", 5)
		if limit > 10 {
			limit = 10
		}

		sector := request.GetString("sector", "")
		includeNews := request.GetBool("include_news", false)

		// Auto-load portfolio strategy
		portfolioName := resolvePortfolioName(ctx, request, storage.KeyValueStorage(), configDefault)
		strategy, _ := storage.StrategyStorage().GetStrategy(ctx, portfolioName)

		result, err := marketService.FunnelScreen(ctx, interfaces.FunnelOptions{
			Exchange:    exchange,
			Limit:       limit,
			Sector:      sector,
			IncludeNews: includeNews,
			Strategy:    strategy,
		})
		if err != nil {
			logger.Error().Err(err).Str("exchange", exchange).Msg("Funnel screen failed")
			return errorResult(fmt.Sprintf("Funnel screen error: %v", err)), nil
		}

		// Auto-save to search history
		searchID := autoSaveFunnelSearch(ctx, storage, result, exchange, sector, strategy, logger)

		markdown := formatFunnelResult(result)
		if searchID != "" {
			markdown += fmt.Sprintf("\n*Search saved: `%s` — use `get_search` to recall*\n", searchID)
		}
		if strategy != nil {
			strategyNote := fmt.Sprintf("\n---\n*Filtered for your %s", strategy.RiskAppetite.Level)
			if strategy.AccountType != "" {
				strategyNote += " " + string(strategy.AccountType)
			}
			strategyNote += " strategy*\n"
			markdown += strategyNote
		}
		return textResult(markdown), nil
	}
}

// handleListSearches implements the list_searches tool
func handleListSearches(storage interfaces.StorageManager, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		searchType := request.GetString("type", "")
		exchange := request.GetString("exchange", "")
		limit := request.GetInt("limit", 10)

		records, err := storage.SearchHistoryStorage().ListSearches(ctx, interfaces.SearchListOptions{
			Type:     searchType,
			Exchange: exchange,
			Limit:    limit,
		})
		if err != nil {
			logger.Error().Err(err).Msg("List searches failed")
			return errorResult(fmt.Sprintf("Error listing searches: %v", err)), nil
		}

		markdown := formatSearchList(records)
		return textResult(markdown), nil
	}
}

// handleGetSearch implements the get_search tool
func handleGetSearch(storage interfaces.StorageManager, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		searchID, err := request.RequireString("search_id")
		if err != nil || searchID == "" {
			return errorResult("Error: search_id parameter is required"), nil
		}

		record, err := storage.SearchHistoryStorage().GetSearch(ctx, searchID)
		if err != nil {
			return errorResult(fmt.Sprintf("Search record not found: %v", err)), nil
		}

		markdown := formatSearchDetail(record)
		return textResult(markdown), nil
	}
}

// handleGetWatchlist implements the get_watchlist tool
func handleGetWatchlist(watchlistService interfaces.WatchlistService, storage interfaces.StorageManager, configDefault string, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioName(ctx, request, storage.KeyValueStorage(), configDefault)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
		}

		wl, err := watchlistService.GetWatchlist(ctx, portfolioName)
		if err != nil {
			return textResult(fmt.Sprintf("No watchlist found for portfolio '%s'.\n\n"+
				"Use `add_watchlist_item` to add stocks, or `set_watchlist` to set an entire watchlist.",
				portfolioName)), nil
		}

		return textResult(wl.ToMarkdown()), nil
	}
}

// handleAddWatchlistItem implements the add_watchlist_item tool
func handleAddWatchlistItem(watchlistService interfaces.WatchlistService, storage interfaces.StorageManager, configDefault string, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioName(ctx, request, storage.KeyValueStorage(), configDefault)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
		}

		itemJSON := request.GetString("item_json", "")
		if itemJSON == "" {
			return errorResult("Error: item_json parameter is required."), nil
		}

		var item models.WatchlistItem
		if err := json.Unmarshal([]byte(itemJSON), &item); err != nil {
			return errorResult(fmt.Sprintf("Error parsing item_json: %v", err)), nil
		}

		if item.Ticker == "" {
			return errorResult("Error: ticker is required in item_json"), nil
		}

		ticker, errMsg := validateTicker(item.Ticker)
		if errMsg != "" {
			return errorResult(errMsg), nil
		}
		item.Ticker = ticker

		if item.Verdict != "" && !models.ValidWatchlistVerdict(item.Verdict) {
			return errorResult(fmt.Sprintf("Error: invalid verdict '%s' — must be PASS, WATCH, or FAIL", item.Verdict)), nil
		}

		wl, err := watchlistService.AddOrUpdateItem(ctx, portfolioName, &item)
		if err != nil {
			logger.Error().Err(err).Str("portfolio", portfolioName).Msg("Failed to add/update watchlist item")
			return errorResult(fmt.Sprintf("Error adding watchlist item: %v", err)), nil
		}

		return textResult(wl.ToMarkdown()), nil
	}
}

// handleUpdateWatchlistItem implements the update_watchlist_item tool
func handleUpdateWatchlistItem(watchlistService interfaces.WatchlistService, storage interfaces.StorageManager, configDefault string, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioName(ctx, request, storage.KeyValueStorage(), configDefault)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
		}

		ticker, err := request.RequireString("ticker")
		if err != nil || ticker == "" {
			return errorResult("Error: ticker parameter is required"), nil
		}
		ticker, errMsg := validateTicker(ticker)
		if errMsg != "" {
			return errorResult(errMsg), nil
		}

		itemJSON := request.GetString("item_json", "")
		if itemJSON == "" {
			return errorResult("Error: item_json parameter is required."), nil
		}

		var update models.WatchlistItem
		if err := json.Unmarshal([]byte(itemJSON), &update); err != nil {
			return errorResult(fmt.Sprintf("Error parsing item_json: %v", err)), nil
		}

		if update.Verdict != "" && !models.ValidWatchlistVerdict(update.Verdict) {
			return errorResult(fmt.Sprintf("Error: invalid verdict '%s' — must be PASS, WATCH, or FAIL", update.Verdict)), nil
		}

		wl, err := watchlistService.UpdateItem(ctx, portfolioName, ticker, &update)
		if err != nil {
			logger.Error().Err(err).Str("portfolio", portfolioName).Str("ticker", ticker).Msg("Failed to update watchlist item")
			return errorResult(fmt.Sprintf("Error updating watchlist item: %v", err)), nil
		}

		return textResult(wl.ToMarkdown()), nil
	}
}

// handleRemoveWatchlistItem implements the remove_watchlist_item tool
func handleRemoveWatchlistItem(watchlistService interfaces.WatchlistService, storage interfaces.StorageManager, configDefault string, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioName(ctx, request, storage.KeyValueStorage(), configDefault)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
		}

		ticker, err := request.RequireString("ticker")
		if err != nil || ticker == "" {
			return errorResult("Error: ticker parameter is required"), nil
		}
		ticker, errMsg := validateTicker(ticker)
		if errMsg != "" {
			return errorResult(errMsg), nil
		}

		wl, err := watchlistService.RemoveItem(ctx, portfolioName, ticker)
		if err != nil {
			logger.Error().Err(err).Str("portfolio", portfolioName).Str("ticker", ticker).Msg("Failed to remove watchlist item")
			return errorResult(fmt.Sprintf("Error removing watchlist item: %v", err)), nil
		}

		return textResult(wl.ToMarkdown()), nil
	}
}

// handleSetWatchlist implements the set_watchlist tool
func handleSetWatchlist(watchlistService interfaces.WatchlistService, storage interfaces.StorageManager, configDefault string, logger *common.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioName(ctx, request, storage.KeyValueStorage(), configDefault)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
		}

		watchlistJSON := request.GetString("watchlist_json", "")
		if watchlistJSON == "" {
			return errorResult("Error: watchlist_json parameter is required."), nil
		}

		wl := &models.PortfolioWatchlist{
			PortfolioName: portfolioName,
		}
		if err := json.Unmarshal([]byte(watchlistJSON), wl); err != nil {
			return errorResult(fmt.Sprintf("Error parsing watchlist_json: %v", err)), nil
		}

		wl.PortfolioName = portfolioName

		// Validate all items
		for i, item := range wl.Items {
			if item.Ticker == "" {
				return errorResult(fmt.Sprintf("Error: item %d is missing ticker", i)), nil
			}
			ticker, errMsg := validateTicker(item.Ticker)
			if errMsg != "" {
				return errorResult(errMsg), nil
			}
			wl.Items[i].Ticker = ticker

			if item.Verdict != "" && !models.ValidWatchlistVerdict(item.Verdict) {
				return errorResult(fmt.Sprintf("Error: item %d has invalid verdict '%s' — must be PASS, WATCH, or FAIL", i, item.Verdict)), nil
			}
		}

		if err := watchlistService.SaveWatchlist(ctx, wl); err != nil {
			logger.Error().Err(err).Str("portfolio", portfolioName).Msg("Failed to save watchlist")
			return errorResult(fmt.Sprintf("Error saving watchlist: %v", err)), nil
		}

		// Reload to get version/timestamps
		saved, err := watchlistService.GetWatchlist(ctx, portfolioName)
		if err != nil {
			return errorResult(fmt.Sprintf("Watchlist saved but failed to reload: %v", err)), nil
		}

		return textResult(saved.ToMarkdown()), nil
	}
}

// autoSaveScreenSearch saves a stock_screen result to search history
func autoSaveScreenSearch(ctx context.Context, storage interfaces.StorageManager, candidates []*models.ScreenCandidate, exchange string, maxPE, minReturn float64, sector string, strategy *models.PortfolioStrategy, logger *common.Logger) string {
	filtersJSON, _ := json.Marshal(map[string]interface{}{
		"exchange":   exchange,
		"max_pe":     maxPE,
		"min_return": minReturn,
		"sector":     sector,
	})
	resultsJSON, _ := json.Marshal(candidates)

	record := &models.SearchRecord{
		Type:        "screen",
		Exchange:    exchange,
		Filters:     string(filtersJSON),
		ResultCount: len(candidates),
		Results:     string(resultsJSON),
		CreatedAt:   time.Now(),
	}
	if strategy != nil {
		record.StrategyName = strategy.PortfolioName
		record.StrategyVer = strategy.Version
	}

	if err := storage.SearchHistoryStorage().SaveSearch(ctx, record); err != nil {
		logger.Warn().Err(err).Msg("Failed to auto-save screen search")
		return ""
	}
	return record.ID
}

// autoSaveSnipeSearch saves a market_snipe result to search history
func autoSaveSnipeSearch(ctx context.Context, storage interfaces.StorageManager, buys []*models.SnipeBuy, exchange string, criteria []string, sector string, strategy *models.PortfolioStrategy, logger *common.Logger) string {
	filtersJSON, _ := json.Marshal(map[string]interface{}{
		"exchange": exchange,
		"criteria": criteria,
		"sector":   sector,
	})
	resultsJSON, _ := json.Marshal(buys)

	record := &models.SearchRecord{
		Type:        "snipe",
		Exchange:    exchange,
		Filters:     string(filtersJSON),
		ResultCount: len(buys),
		Results:     string(resultsJSON),
		CreatedAt:   time.Now(),
	}
	if strategy != nil {
		record.StrategyName = strategy.PortfolioName
		record.StrategyVer = strategy.Version
	}

	if err := storage.SearchHistoryStorage().SaveSearch(ctx, record); err != nil {
		logger.Warn().Err(err).Msg("Failed to auto-save snipe search")
		return ""
	}
	return record.ID
}

// autoSaveFunnelSearch saves a funnel_screen result to search history
func autoSaveFunnelSearch(ctx context.Context, storage interfaces.StorageManager, result *models.FunnelResult, exchange, sector string, strategy *models.PortfolioStrategy, logger *common.Logger) string {
	filtersJSON, _ := json.Marshal(map[string]interface{}{
		"exchange": exchange,
		"sector":   sector,
	})
	resultsJSON, _ := json.Marshal(result.Candidates)
	stagesJSON, _ := json.Marshal(result.Stages)

	record := &models.SearchRecord{
		Type:        "funnel",
		Exchange:    exchange,
		Filters:     string(filtersJSON),
		ResultCount: len(result.Candidates),
		Results:     string(resultsJSON),
		Stages:      string(stagesJSON),
		CreatedAt:   time.Now(),
	}
	if strategy != nil {
		record.StrategyName = strategy.PortfolioName
		record.StrategyVer = strategy.Version
	}

	if err := storage.SearchHistoryStorage().SaveSearch(ctx, record); err != nil {
		logger.Warn().Err(err).Msg("Failed to auto-save funnel search")
		return ""
	}
	return record.ID
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
