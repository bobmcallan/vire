package main

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerTools registers all MCP tools on the server, wiring each to a handler
// that calls the REST API via the proxy.
func registerTools(s *server.MCPServer, p *MCPProxy) {
	s.AddTool(createGetVersionTool(), handleGetVersion(p))
	s.AddTool(createPortfolioComplianceTool(), handlePortfolioCompliance(p))
	s.AddTool(createGetPortfolioTool(), handleGetPortfolio(p))
	s.AddTool(createStrategyScannerTool(), handleStrategyScanner(p))
	s.AddTool(createStockScreenTool(), handleStockScreen(p))
	s.AddTool(createGetStockDataTool(), handleGetStockData(p))
	s.AddTool(createComputeIndicatorsTool(), handleComputeIndicators(p))
	s.AddTool(createListPortfoliosTool(), handleListPortfolios(p))
	// s.AddTool(createSyncPortfolioTool(), handleSyncPortfolio(p))
	// s.AddTool(createRebuildDataTool(), handleRebuildData(p))
	// s.AddTool(createCollectMarketDataTool(), handleCollectMarketData(p))
	s.AddTool(createGetPortfolioStockTool(), handleGetPortfolioStock(p))
	s.AddTool(createGenerateReportTool(), handleGenerateReport(p))
	s.AddTool(createListReportsTool(), handleListReports(p))
	s.AddTool(createGetSummaryTool(), handleGetSummary(p))
	// s.AddTool(createGetPortfolioSnapshotTool(), handleGetPortfolioSnapshot(p))
	// s.AddTool(createGetPortfolioHistoryTool(), handleGetPortfolioHistory(p))
	s.AddTool(createSetDefaultPortfolioTool(), handleSetDefaultPortfolio(p))
	s.AddTool(createGetConfigTool(), handleGetConfig(p))
	s.AddTool(createGetStrategyTemplateTool(), handleGetStrategyTemplate(p))
	s.AddTool(createSetPortfolioStrategyTool(), handleSetPortfolioStrategy(p))
	s.AddTool(createGetPortfolioStrategyTool(), handleGetPortfolioStrategy(p))
	s.AddTool(createDeletePortfolioStrategyTool(), handleDeletePortfolioStrategy(p))
	s.AddTool(createGetPortfolioPlanTool(), handleGetPortfolioPlan(p))
	s.AddTool(createSetPortfolioPlanTool(), handleSetPortfolioPlan(p))
	s.AddTool(createAddPlanItemTool(), handleAddPlanItem(p))
	s.AddTool(createUpdatePlanItemTool(), handleUpdatePlanItem(p))
	s.AddTool(createRemovePlanItemTool(), handleRemovePlanItem(p))
	s.AddTool(createCheckPlanStatusTool(), handleCheckPlanStatus(p))
	// s.AddTool(createFunnelScreenTool(), handleFunnelScreen(p))
	// s.AddTool(createListSearchesTool(), handleListSearches(p))
	// s.AddTool(createGetSearchTool(), handleGetSearch(p))
	// s.AddTool(createGetWatchlistTool(), handleGetWatchlist(p))
	// s.AddTool(createAddWatchlistItemTool(), handleAddWatchlistItem(p))
	// s.AddTool(createUpdateWatchlistItemTool(), handleUpdateWatchlistItem(p))
	// s.AddTool(createRemoveWatchlistItemTool(), handleRemoveWatchlistItem(p))
	// s.AddTool(createSetWatchlistTool(), handleSetWatchlist(p))
	s.AddTool(createGetQuoteTool(), handleGetQuote(p))
	s.AddTool(createGetDiagnosticsTool(), handleGetDiagnostics(p))
}

// --- Tool definitions (unchanged from internal/app/tools.go) ---

func createGetVersionTool() mcp.Tool {
	return mcp.NewTool("get_version",
		mcp.WithDescription("Get the Vire MCP server version and status. Use this to verify connectivity."),
	)
}

func createPortfolioComplianceTool() mcp.Tool {
	return mcp.NewTool("portfolio_compliance",
		mcp.WithDescription("Review a portfolio for signals, overnight movement, and actionable observations. Returns a comprehensive analysis of holdings with compliance status classifications."),
		mcp.WithString("portfolio_name", mcp.Description("Name of the portfolio to review (e.g., 'SMSF', 'Personal'). Uses default portfolio if not specified.")),
		mcp.WithArray("focus_signals", mcp.WithStringItems(), mcp.Description("Signal types to focus on: sma, rsi, volume, pbas, vli, regime, trend, support_resistance, macd")),
		mcp.WithBoolean("include_news", mcp.Description("Include news sentiment analysis (default: false)")),
	)
}

func createGetPortfolioTool() mcp.Tool {
	return mcp.NewTool("get_portfolio",
		mcp.WithDescription("FAST: Get current portfolio holdings — tickers, names, values, weights, and gains. No signals, charts, or AI analysis. Use portfolio_compliance for full analysis."),
		mcp.WithString("portfolio_name", mcp.Description("Name of the portfolio (e.g., 'SMSF', 'Personal'). Uses default portfolio if not specified.")),
	)
}

func createStrategyScannerTool() mcp.Tool {
	return mcp.NewTool("strategy_scanner",
		mcp.WithDescription("Scan for tickers matching strategy entry criteria. Filters by technical indicators, volume patterns, and price levels."),
		mcp.WithString("exchange", mcp.Required(), mcp.Description("Exchange to scan (e.g., 'AU' for ASX, 'US' for NYSE/NASDAQ)")),
		mcp.WithNumber("limit", mcp.Description("Maximum results to return (default: 3, max: 10)")),
		mcp.WithArray("criteria", mcp.WithStringItems(), mcp.Description("Filter criteria: oversold_rsi, near_support, underpriced, accumulating, regime_shift")),
		mcp.WithString("sector", mcp.Description("Filter by sector (e.g., 'Technology', 'Healthcare', 'Mining')")),
		mcp.WithBoolean("include_news", mcp.Description("Include news sentiment analysis (default: false)")),
		mcp.WithString("portfolio_name", mcp.Description("Name of the portfolio for strategy loading. Uses default portfolio if not specified.")),
	)
}

func createStockScreenTool() mcp.Tool {
	return mcp.NewTool("stock_screen",
		mcp.WithDescription("Screen for stocks matching quantitative filters: low P/E, positive earnings, consistent quarterly returns (10%+ annualised), upward price trajectory, and credible news support."),
		mcp.WithString("exchange", mcp.Required(), mcp.Description("Exchange to scan (e.g., 'AU' for ASX, 'US' for NYSE/NASDAQ)")),
		mcp.WithNumber("limit", mcp.Description("Maximum results to return (default: 5, max: 15)")),
		mcp.WithNumber("max_pe", mcp.Description("Maximum P/E ratio filter (default: 20)")),
		mcp.WithNumber("min_return", mcp.Description("Minimum annualised quarterly return percentage (default: 10)")),
		mcp.WithString("sector", mcp.Description("Filter by sector (e.g., 'Technology', 'Healthcare', 'Financials')")),
		mcp.WithBoolean("include_news", mcp.Description("Include news sentiment analysis (default: false)")),
		mcp.WithString("portfolio_name", mcp.Description("Name of the portfolio for strategy loading. Uses default portfolio if not specified.")),
	)
}

func createGetStockDataTool() mcp.Tool {
	return mcp.NewTool("get_stock_data",
		mcp.WithDescription("Get comprehensive stock data including price, fundamentals, signals, and news for a specific ticker."),
		mcp.WithString("ticker", mcp.Required(), mcp.Description("Stock ticker with exchange suffix (e.g., 'BHP.AU', 'AAPL.US')")),
		mcp.WithArray("include", mcp.WithStringItems(), mcp.Description("Data to include: price, fundamentals, signals, news (default: all)")),
	)
}

func createGetQuoteTool() mcp.Tool {
	return mcp.NewTool("get_quote",
		mcp.WithDescription("FAST: Get a real-time price quote for a single ticker. Returns OHLCV, change%, and previous close. Use for spot-checking 1-3 prices — for broad analysis prefer get_stock_data. Supports stocks (BHP.AU, AAPL.US), forex (AUDUSD.FOREX, EURUSD.FOREX), and commodities (XAUUSD.FOREX for gold, XAGUSD.FOREX for silver)."),
		mcp.WithString("ticker", mcp.Required(), mcp.Description("Ticker with exchange suffix (e.g., 'BHP.AU', 'AAPL.US', 'AUDUSD.FOREX', 'XAUUSD.FOREX')")),
	)
}

func createComputeIndicatorsTool() mcp.Tool {
	return mcp.NewTool("compute_indicators",
		mcp.WithDescription("Compute technical indicators for specified tickers. Returns raw indicator values, trend classification, and risk flags."),
		mcp.WithArray("tickers", mcp.WithStringItems(), mcp.Required(), mcp.Description("List of tickers to analyze (e.g., ['BHP.AU', 'CBA.AU'])")),
		mcp.WithArray("signal_types", mcp.WithStringItems(), mcp.Description("Signal types to compute: sma, rsi, volume, pbas, vli, regime, trend (default: all)")),
	)
}

func createListPortfoliosTool() mcp.Tool {
	return mcp.NewTool("list_portfolios",
		mcp.WithDescription("List all available portfolios that can be reviewed."),
	)
}

func createSyncPortfolioTool() mcp.Tool {
	return mcp.NewTool("sync_portfolio",
		mcp.WithDescription("Synchronize portfolio holdings from Navexa. Use this to refresh portfolio data before a review."),
		mcp.WithString("portfolio_name", mcp.Description("Name of the portfolio to sync. Uses default portfolio if not specified.")),
		mcp.WithBoolean("force", mcp.Description("Force sync even if recently synced (default: false)")),
	)
}

func createGenerateReportTool() mcp.Tool {
	return mcp.NewTool("generate_report",
		mcp.WithDescription("SLOW: Generate a full portfolio report from scratch — syncs holdings, collects market data, runs signals for every ticker. Takes several minutes."),
		mcp.WithString("portfolio_name", mcp.Description("Name of the portfolio to generate a report for. Uses default portfolio if not specified.")),
		mcp.WithBoolean("force_refresh", mcp.Description("Force refresh of portfolio data even if recently synced (default: false)")),
		mcp.WithBoolean("include_news", mcp.Description("Include news sentiment analysis (default: false)")),
	)
}

func createGetPortfolioStockTool() mcp.Tool {
	return mcp.NewTool("get_portfolio_stock",
		mcp.WithDescription("FAST: Get portfolio position data for a single holding — position details, trade history, dividends, and returns. No market data or signals. Use get_stock_data for market analysis."),
		mcp.WithString("portfolio_name", mcp.Description("Name of the portfolio. Uses default portfolio if not specified.")),
		mcp.WithString("ticker", mcp.Required(), mcp.Description("Ticker symbol (e.g., 'BHP', 'BHP.AU', 'NVDA.US')")),
	)
}

func createListReportsTool() mcp.Tool {
	return mcp.NewTool("list_reports",
		mcp.WithDescription("List available portfolio reports with their generation timestamps."),
		mcp.WithString("portfolio_name", mcp.Description("Optional: filter to a specific portfolio name")),
	)
}

func createGetSummaryTool() mcp.Tool {
	return mcp.NewTool("get_summary",
		mcp.WithDescription("FAST: Get portfolio summary. Auto-generates if no cached report exists."),
		mcp.WithString("portfolio_name", mcp.Description("Name of the portfolio. Uses default portfolio if not specified.")),
	)
}

func createGetPortfolioSnapshotTool() mcp.Tool {
	return mcp.NewTool("get_portfolio_snapshot",
		mcp.WithDescription("Reconstruct portfolio state as of a historical date."),
		mcp.WithString("portfolio_name", mcp.Description("Name of the portfolio. Uses default portfolio if not specified.")),
		mcp.WithString("date", mcp.Required(), mcp.Description("Historical date in YYYY-MM-DD format")),
	)
}

func createGetPortfolioHistoryTool() mcp.Tool {
	return mcp.NewTool("get_portfolio_history",
		mcp.WithDescription("Get daily portfolio value history for a date range. Returns both a markdown summary table and a JSON data array."),
		mcp.WithString("portfolio_name", mcp.Description("Name of the portfolio. Uses default portfolio if not specified.")),
		mcp.WithString("from", mcp.Description("Start date in YYYY-MM-DD format (default: portfolio inception)")),
		mcp.WithString("to", mcp.Description("End date in YYYY-MM-DD format (default: yesterday)")),
		mcp.WithString("format", mcp.Description("Output granularity: 'daily', 'weekly', 'monthly', or 'auto' (default: auto)")),
	)
}

func createSetDefaultPortfolioTool() mcp.Tool {
	return mcp.NewTool("set_default_portfolio",
		mcp.WithDescription("Set the default portfolio name. Call without portfolio_name to list available portfolios."),
		mcp.WithString("portfolio_name", mcp.Description("Portfolio name to set as default. Omit to list available portfolios.")),
	)
}

func createGetConfigTool() mcp.Tool {
	return mcp.NewTool("get_config",
		mcp.WithDescription("List all Vire configuration settings."),
	)
}

func createRebuildDataTool() mcp.Tool {
	return mcp.NewTool("rebuild_data",
		mcp.WithDescription("Purge all cached/derived data and rebuild from source. Takes several minutes."),
		mcp.WithString("portfolio_name", mcp.Description("Name of the portfolio. Uses default portfolio if not specified.")),
	)
}

func createGetStrategyTemplateTool() mcp.Tool {
	return mcp.NewTool("get_strategy_template",
		mcp.WithDescription("Get a template showing all available strategy fields and examples."),
		mcp.WithString("account_type", mcp.Description("Account type for tailored guidance: 'smsf' or 'trading'.")),
	)
}

func createSetPortfolioStrategyTool() mcp.Tool {
	return mcp.NewTool("set_portfolio_strategy",
		mcp.WithDescription("Set or update the investment strategy for a portfolio. Uses MERGE semantics."),
		mcp.WithString("portfolio_name", mcp.Description("Name of the portfolio. Uses default portfolio if not specified.")),
		mcp.WithString("strategy_json", mcp.Required(), mcp.Description("JSON object with strategy fields to set.")),
	)
}

func createGetPortfolioStrategyTool() mcp.Tool {
	return mcp.NewTool("get_portfolio_strategy",
		mcp.WithDescription("FAST: Get the investment strategy document for a portfolio."),
		mcp.WithString("portfolio_name", mcp.Description("Name of the portfolio. Uses default portfolio if not specified.")),
	)
}

func createDeletePortfolioStrategyTool() mcp.Tool {
	return mcp.NewTool("delete_portfolio_strategy",
		mcp.WithDescription("Delete the investment strategy for a portfolio."),
		mcp.WithString("portfolio_name", mcp.Description("Name of the portfolio. Uses default portfolio if not specified.")),
	)
}

func createGetPortfolioPlanTool() mcp.Tool {
	return mcp.NewTool("get_portfolio_plan",
		mcp.WithDescription("Get the current investment plan for a portfolio."),
		mcp.WithString("portfolio_name", mcp.Description("Name of the portfolio. Uses default portfolio if not specified.")),
	)
}

func createSetPortfolioPlanTool() mcp.Tool {
	return mcp.NewTool("set_portfolio_plan",
		mcp.WithDescription("Set or update the investment plan for a portfolio."),
		mcp.WithString("portfolio_name", mcp.Description("Name of the portfolio. Uses default portfolio if not specified.")),
		mcp.WithString("plan_json", mcp.Required(), mcp.Description("JSON object with plan fields.")),
	)
}

func createAddPlanItemTool() mcp.Tool {
	return mcp.NewTool("add_plan_item",
		mcp.WithDescription("Add a single action item to a portfolio plan."),
		mcp.WithString("portfolio_name", mcp.Description("Name of the portfolio. Uses default portfolio if not specified.")),
		mcp.WithString("item_json", mcp.Required(), mcp.Description("JSON object for the plan item.")),
	)
}

func createUpdatePlanItemTool() mcp.Tool {
	return mcp.NewTool("update_plan_item",
		mcp.WithDescription("Update an existing plan item by ID. Uses merge semantics."),
		mcp.WithString("portfolio_name", mcp.Description("Name of the portfolio. Uses default portfolio if not specified.")),
		mcp.WithString("item_id", mcp.Required(), mcp.Description("ID of the plan item to update")),
		mcp.WithString("item_json", mcp.Required(), mcp.Description("JSON object with fields to update.")),
	)
}

func createRemovePlanItemTool() mcp.Tool {
	return mcp.NewTool("remove_plan_item",
		mcp.WithDescription("Remove a plan item by ID."),
		mcp.WithString("portfolio_name", mcp.Description("Name of the portfolio. Uses default portfolio if not specified.")),
		mcp.WithString("item_id", mcp.Required(), mcp.Description("ID of the plan item to remove")),
	)
}

func createCheckPlanStatusTool() mcp.Tool {
	return mcp.NewTool("check_plan_status",
		mcp.WithDescription("Evaluate plan status: checks event triggers and deadline expiry."),
		mcp.WithString("portfolio_name", mcp.Description("Name of the portfolio. Uses default portfolio if not specified.")),
	)
}

func createFunnelScreenTool() mcp.Tool {
	return mcp.NewTool("funnel_screen",
		mcp.WithDescription("SLOW: Extended stock screen with 3-stage funnel and stage-by-stage visibility."),
		mcp.WithString("exchange", mcp.Required(), mcp.Description("Exchange to scan (e.g., 'AU', 'US')")),
		mcp.WithNumber("limit", mcp.Description("Maximum final results (default: 5, max: 10)")),
		mcp.WithString("sector", mcp.Description("Filter by sector")),
		mcp.WithBoolean("include_news", mcp.Description("Include news sentiment analysis (default: false)")),
		mcp.WithString("portfolio_name", mcp.Description("Name of the portfolio for strategy loading.")),
	)
}

func createListSearchesTool() mcp.Tool {
	return mcp.NewTool("list_searches",
		mcp.WithDescription("List recent stock screen and search history."),
		mcp.WithString("type", mcp.Description("Filter by type: 'screen', 'snipe', 'funnel'")),
		mcp.WithString("exchange", mcp.Description("Filter by exchange")),
		mcp.WithNumber("limit", mcp.Description("Maximum results (default: 10)")),
	)
}

func createGetSearchTool() mcp.Tool {
	return mcp.NewTool("get_search",
		mcp.WithDescription("Retrieve a specific past search result by ID."),
		mcp.WithString("search_id", mcp.Required(), mcp.Description("The search record ID")),
	)
}

func createGetWatchlistTool() mcp.Tool {
	return mcp.NewTool("get_watchlist",
		mcp.WithDescription("Get the stock watchlist for a portfolio."),
		mcp.WithString("portfolio_name", mcp.Description("Name of the portfolio. Uses default portfolio if not specified.")),
	)
}

func createAddWatchlistItemTool() mcp.Tool {
	return mcp.NewTool("add_watchlist_item",
		mcp.WithDescription("Add or update a stock on the watchlist with a verdict."),
		mcp.WithString("portfolio_name", mcp.Description("Name of the portfolio. Uses default portfolio if not specified.")),
		mcp.WithString("item_json", mcp.Required(), mcp.Description("JSON object for the watchlist item.")),
	)
}

func createUpdateWatchlistItemTool() mcp.Tool {
	return mcp.NewTool("update_watchlist_item",
		mcp.WithDescription("Update an existing watchlist item by ticker."),
		mcp.WithString("portfolio_name", mcp.Description("Name of the portfolio. Uses default portfolio if not specified.")),
		mcp.WithString("ticker", mcp.Required(), mcp.Description("Ticker to update")),
		mcp.WithString("item_json", mcp.Required(), mcp.Description("JSON object with fields to update.")),
	)
}

func createRemoveWatchlistItemTool() mcp.Tool {
	return mcp.NewTool("remove_watchlist_item",
		mcp.WithDescription("Remove a stock from the watchlist by ticker."),
		mcp.WithString("portfolio_name", mcp.Description("Name of the portfolio. Uses default portfolio if not specified.")),
		mcp.WithString("ticker", mcp.Required(), mcp.Description("Ticker to remove")),
	)
}

func createSetWatchlistTool() mcp.Tool {
	return mcp.NewTool("set_watchlist",
		mcp.WithDescription("Replace the entire watchlist for a portfolio."),
		mcp.WithString("portfolio_name", mcp.Description("Name of the portfolio. Uses default portfolio if not specified.")),
		mcp.WithString("watchlist_json", mcp.Required(), mcp.Description("JSON object with watchlist fields.")),
	)
}

func createCollectMarketDataTool() mcp.Tool {
	return mcp.NewTool("collect_market_data",
		mcp.WithDescription("Collect and store market data for specified tickers."),
		mcp.WithArray("tickers", mcp.WithStringItems(), mcp.Required(), mcp.Description("List of tickers to collect data for")),
		mcp.WithBoolean("include_news", mcp.Description("Include news articles (default: false)")),
	)
}

func createGetDiagnosticsTool() mcp.Tool {
	return mcp.NewTool("get_diagnostics",
		mcp.WithDescription("Get server diagnostics: uptime, version, recent log entries."),
		mcp.WithString("correlation_id", mcp.Description("If provided, returns logs for a specific correlation ID")),
		mcp.WithNumber("limit", mcp.Description("Maximum recent log entries (default: 50)")),
	)
}
