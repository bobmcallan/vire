package main

import (
	"github.com/mark3labs/mcp-go/mcp"
)

// createGetVersionTool returns the get_version tool definition
func createGetVersionTool() mcp.Tool {
	return mcp.NewTool("get_version",
		mcp.WithDescription("Get the Vire MCP server version and status. Use this to verify connectivity."),
	)
}

// createPortfolioReviewTool returns the portfolio_review tool definition
func createPortfolioReviewTool() mcp.Tool {
	return mcp.NewTool("portfolio_review",
		mcp.WithDescription("Review a portfolio for signals, overnight movement, and actionable recommendations. Returns a comprehensive analysis of holdings with buy/sell/hold recommendations."),
		mcp.WithString("portfolio_name",
			mcp.Description("Name of the portfolio to review (e.g., 'SMSF', 'Personal'). Uses default portfolio if not specified."),
		),
		mcp.WithArray("focus_signals",
			mcp.WithStringItems(),
			mcp.Description("Signal types to focus on: sma, rsi, volume, pbas, vli, regime, trend, support_resistance, macd"),
		),
		mcp.WithBoolean("include_news",
			mcp.Description("Include news sentiment analysis (default: false)"),
		),
	)
}

// createGetPortfolioTool returns the get_portfolio tool definition
// FAST: Returns cached portfolio holdings without signals, AI, or growth chart.
func createGetPortfolioTool() mcp.Tool {
	return mcp.NewTool("get_portfolio",
		mcp.WithDescription("FAST: Get current portfolio holdings — tickers, names, values, weights, and gains. No signals, charts, or AI analysis. Use portfolio_review for full analysis."),
		mcp.WithString("portfolio_name",
			mcp.Description("Name of the portfolio (e.g., 'SMSF', 'Personal'). Uses default portfolio if not specified."),
		),
	)
}

// createMarketSnipeTool returns the market_snipe tool definition
func createMarketSnipeTool() mcp.Tool {
	return mcp.NewTool("market_snipe",
		mcp.WithDescription("Find turnaround stock opportunities showing buy signals. Scans the market for oversold stocks with accumulation patterns and good upside potential."),
		mcp.WithString("exchange",
			mcp.Required(),
			mcp.Description("Exchange to scan (e.g., 'AU' for ASX, 'US' for NYSE/NASDAQ)"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum results to return (default: 3, max: 10)"),
		),
		mcp.WithArray("criteria",
			mcp.WithStringItems(),
			mcp.Description("Filter criteria: oversold_rsi, near_support, underpriced, accumulating, regime_shift"),
		),
		mcp.WithString("sector",
			mcp.Description("Filter by sector (e.g., 'Technology', 'Healthcare', 'Mining')"),
		),
		mcp.WithBoolean("include_news",
			mcp.Description("Include news sentiment analysis (default: false)"),
		),
		mcp.WithString("portfolio_name",
			mcp.Description("Name of the portfolio for strategy loading. Uses default portfolio if not specified."),
		),
	)
}

// createGetStockDataTool returns the get_stock_data tool definition
func createGetStockDataTool() mcp.Tool {
	return mcp.NewTool("get_stock_data",
		mcp.WithDescription("Get comprehensive stock data including price, fundamentals, signals, and news for a specific ticker."),
		mcp.WithString("ticker",
			mcp.Required(),
			mcp.Description("Stock ticker with exchange suffix (e.g., 'BHP.AU', 'AAPL.US')"),
		),
		mcp.WithArray("include",
			mcp.WithStringItems(),
			mcp.Description("Data to include: price, fundamentals, signals, news (default: all)"),
		),
	)
}

// createDetectSignalsTool returns the detect_signals tool definition
func createDetectSignalsTool() mcp.Tool {
	return mcp.NewTool("detect_signals",
		mcp.WithDescription("Detect and compute trading signals for specified tickers. Returns technical indicators, trend classification, and risk flags."),
		mcp.WithArray("tickers",
			mcp.WithStringItems(),
			mcp.Required(),
			mcp.Description("List of tickers to analyze (e.g., ['BHP.AU', 'CBA.AU'])"),
		),
		mcp.WithArray("signal_types",
			mcp.WithStringItems(),
			mcp.Description("Signal types to compute: sma, rsi, volume, pbas, vli, regime, trend (default: all)"),
		),
	)
}

// createListPortfoliosTool returns the list_portfolios tool definition
func createListPortfoliosTool() mcp.Tool {
	return mcp.NewTool("list_portfolios",
		mcp.WithDescription("List all available portfolios that can be reviewed."),
	)
}

// createSyncPortfolioTool returns the sync_portfolio tool definition
func createSyncPortfolioTool() mcp.Tool {
	return mcp.NewTool("sync_portfolio",
		mcp.WithDescription("Synchronize portfolio holdings from Navexa. Use this to refresh portfolio data before a review."),
		mcp.WithString("portfolio_name",
			mcp.Description("Name of the portfolio to sync. Uses default portfolio if not specified."),
		),
		mcp.WithBoolean("force",
			mcp.Description("Force sync even if recently synced (default: false)"),
		),
	)
}

// createGenerateReportTool returns the generate_report tool definition
func createGenerateReportTool() mcp.Tool {
	return mcp.NewTool("generate_report",
		mcp.WithDescription("SLOW: Generate a full portfolio report from scratch — syncs holdings, collects market data, runs signals for every ticker. Takes several minutes. Only use when explicitly asked to regenerate or refresh a report. For reading existing reports, use get_summary or get_ticker_report instead."),
		mcp.WithString("portfolio_name",
			mcp.Description("Name of the portfolio to generate a report for (e.g., 'SMSF', 'Personal'). Uses default portfolio if not specified."),
		),
		mcp.WithBoolean("force_refresh",
			mcp.Description("Force refresh of portfolio data even if recently synced (default: false)"),
		),
		mcp.WithBoolean("include_news",
			mcp.Description("Include news sentiment analysis (default: false)"),
		),
	)
}

// createGenerateTickerReportTool returns the generate_ticker_report tool definition
func createGenerateTickerReportTool() mcp.Tool {
	return mcp.NewTool("generate_ticker_report",
		mcp.WithDescription("SLOW: Regenerate report for a single ticker — refreshes market data and signals. Only use when asked to refresh a specific ticker. For reading existing reports, use get_ticker_report instead."),
		mcp.WithString("portfolio_name",
			mcp.Description("Name of the portfolio (e.g., 'SMSF'). Uses default portfolio if not specified."),
		),
		mcp.WithString("ticker",
			mcp.Required(),
			mcp.Description("Ticker symbol to regenerate (e.g., 'BHP', 'ACDC')"),
		),
	)
}

// createListReportsTool returns the list_reports tool definition
func createListReportsTool() mcp.Tool {
	return mcp.NewTool("list_reports",
		mcp.WithDescription("List available portfolio reports with their generation timestamps."),
		mcp.WithString("portfolio_name",
			mcp.Description("Optional: filter to a specific portfolio name"),
		),
	)
}

// createGetSummaryTool returns the get_summary tool definition
func createGetSummaryTool() mcp.Tool {
	return mcp.NewTool("get_summary",
		mcp.WithDescription("FAST: Get portfolio summary — holdings, market values, portfolio balance, alerts, and recommendations. This is the default tool for portfolio questions. Auto-generates if no cached report exists."),
		mcp.WithString("portfolio_name",
			mcp.Description("Name of the portfolio (e.g., 'SMSF'). Uses default portfolio if not specified."),
		),
	)
}

// createGetTickerReportTool returns the get_ticker_report tool definition
func createGetTickerReportTool() mcp.Tool {
	return mcp.NewTool("get_ticker_report",
		mcp.WithDescription("FAST: Get detailed report for a single ticker — position, fundamentals, technical signals, filings intelligence, and risk flags. Use this when asked about a specific stock. Auto-generates if no cached report exists."),
		mcp.WithString("portfolio_name",
			mcp.Description("Name of the portfolio (e.g., 'SMSF'). Uses default portfolio if not specified."),
		),
		mcp.WithString("ticker",
			mcp.Required(),
			mcp.Description("Ticker symbol (e.g., 'BHP', 'ACDC')"),
		),
	)
}

// createListTickersTool returns the list_tickers tool definition
func createListTickersTool() mcp.Tool {
	return mcp.NewTool("list_tickers",
		mcp.WithDescription("List all ticker reports available in a portfolio report."),
		mcp.WithString("portfolio_name",
			mcp.Description("Name of the portfolio (e.g., 'SMSF'). Uses default portfolio if not specified."),
		),
	)
}

// createStockScreenTool returns the stock_screen tool definition
func createStockScreenTool() mcp.Tool {
	return mcp.NewTool("stock_screen",
		mcp.WithDescription("Screen for quality-value stocks with low P/E, positive earnings, consistent quarterly returns (10%+ annualised), bullish price trajectory, and credible news support. Filters out story stocks and speculative plays."),
		mcp.WithString("exchange",
			mcp.Required(),
			mcp.Description("Exchange to scan (e.g., 'AU' for ASX, 'US' for NYSE/NASDAQ)"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum results to return (default: 5, max: 15)"),
		),
		mcp.WithNumber("max_pe",
			mcp.Description("Maximum P/E ratio filter (default: 20)"),
		),
		mcp.WithNumber("min_return",
			mcp.Description("Minimum annualised quarterly return percentage (default: 10)"),
		),
		mcp.WithString("sector",
			mcp.Description("Filter by sector (e.g., 'Technology', 'Healthcare', 'Financials')"),
		),
		mcp.WithBoolean("include_news",
			mcp.Description("Include news sentiment analysis (default: false)"),
		),
		mcp.WithString("portfolio_name",
			mcp.Description("Name of the portfolio for strategy loading. Uses default portfolio if not specified."),
		),
	)
}

// createGetPortfolioSnapshotTool returns the get_portfolio_snapshot tool definition
func createGetPortfolioSnapshotTool() mcp.Tool {
	return mcp.NewTool("get_portfolio_snapshot",
		mcp.WithDescription("Reconstruct portfolio state as of a historical date — shows holdings, quantities, close prices, and gains at that point in time. Requires portfolio to have been synced with trades and market data to cover the requested date."),
		mcp.WithString("portfolio_name",
			mcp.Description("Name of the portfolio (e.g., 'SMSF'). Uses default portfolio if not specified."),
		),
		mcp.WithString("date",
			mcp.Required(),
			mcp.Description("Historical date in YYYY-MM-DD format (e.g., '2025-01-30')"),
		),
	)
}

// createSetDefaultPortfolioTool returns the set_default_portfolio tool definition
func createSetDefaultPortfolioTool() mcp.Tool {
	return mcp.NewTool("set_default_portfolio",
		mcp.WithDescription("Set the default portfolio name. Call without portfolio_name to list available portfolios and see the current default. Once set, tools that accept portfolio_name will use this default when no portfolio is specified."),
		mcp.WithString("portfolio_name",
			mcp.Description("Portfolio name to set as default (e.g., 'SMSF', 'Personal'). Omit to list available portfolios."),
		),
	)
}

// createGetConfigTool returns the get_config tool definition
func createGetConfigTool() mcp.Tool {
	return mcp.NewTool("get_config",
		mcp.WithDescription("List all Vire configuration settings ordered by source: runtime (KV store), environment variables, then config file (TOML) defaults."),
	)
}

// createGetPortfolioHistoryTool returns the get_portfolio_history tool definition
func createGetPortfolioHistoryTool() mcp.Tool {
	return mcp.NewTool("get_portfolio_history",
		mcp.WithDescription("Get daily portfolio value history for a date range. Returns both a markdown summary table and a JSON data array for programmatic use (charting, analysis). Use format parameter to control granularity. Use for questions like 'How much have I lost this week?' or 'What was my portfolio value last month?'"),
		mcp.WithString("portfolio_name",
			mcp.Description("Name of the portfolio (e.g., 'SMSF'). Uses default portfolio if not specified."),
		),
		mcp.WithString("from",
			mcp.Description("Start date in YYYY-MM-DD format (default: portfolio inception)"),
		),
		mcp.WithString("to",
			mcp.Description("End date in YYYY-MM-DD format (default: yesterday)"),
		),
		mcp.WithString("format",
			mcp.Description("Output granularity: 'daily', 'weekly', 'monthly', or 'auto' (default: auto — daily if <=90 days, weekly otherwise)"),
		),
	)
}

// createGetStrategyTemplateTool returns the get_strategy_template tool definition
func createGetStrategyTemplateTool() mcp.Tool {
	return mcp.NewTool("get_strategy_template",
		mcp.WithDescription("Get a template showing all available strategy fields, valid values, and examples. Use this before setting a strategy to understand what options are available. Returns a structured questionnaire that guides strategy creation. Optionally specify account_type for tailored guidance (SMSF accounts include regulatory considerations)."),
		mcp.WithString("account_type",
			mcp.Description("Account type for tailored guidance: 'smsf' (self-managed super fund) or 'trading' (standard trading account). Omit for generic template."),
		),
	)
}

// createSetPortfolioStrategyTool returns the set_portfolio_strategy tool definition
func createSetPortfolioStrategyTool() mcp.Tool {
	return mcp.NewTool("set_portfolio_strategy",
		mcp.WithDescription("Set or update the investment strategy for a portfolio. Uses MERGE semantics: only include fields you want to change, unspecified fields keep their current values (or sensible defaults for new strategies). IMPORTANT: When updating a nested object (e.g., risk_appetite), include ALL sub-fields you want to keep, not just the ones you're changing — nested objects are replaced atomically. Returns the complete saved strategy as markdown plus any devil's advocate warnings about unrealistic goals or internal contradictions. Call get_strategy_template first to see available fields and valid values."),
		mcp.WithString("portfolio_name",
			mcp.Description("Name of the portfolio (e.g., 'SMSF', 'Personal'). Uses default portfolio if not specified."),
		),
		mcp.WithString("strategy_json",
			mcp.Required(),
			mcp.Description("JSON object with strategy fields to set. Supports partial updates. Example: {\"account_type\":\"smsf\",\"risk_appetite\":{\"level\":\"moderate\",\"max_drawdown_pct\":15},\"target_returns\":{\"annual_pct\":8.5,\"timeframe\":\"3-5 years\"},\"investment_universe\":[\"AU\",\"US\"],\"position_sizing\":{\"max_position_pct\":10,\"max_sector_pct\":30}}"),
		),
	)
}

// createGetPortfolioStrategyTool returns the get_portfolio_strategy tool definition
func createGetPortfolioStrategyTool() mcp.Tool {
	return mcp.NewTool("get_portfolio_strategy",
		mcp.WithDescription("FAST: Get the investment strategy document for a portfolio. Returns the strategy as a formatted markdown document including account type, risk appetite, target returns, income requirements, sector preferences, position sizing rules, reference strategies, and rebalancing frequency. Shows version, creation date, and last review date. If no strategy exists, returns guidance on how to create one."),
		mcp.WithString("portfolio_name",
			mcp.Description("Name of the portfolio (e.g., 'SMSF'). Uses default portfolio if not specified."),
		),
	)
}

// createDeletePortfolioStrategyTool returns the delete_portfolio_strategy tool definition
func createDeletePortfolioStrategyTool() mcp.Tool {
	return mcp.NewTool("delete_portfolio_strategy",
		mcp.WithDescription("Delete the investment strategy for a portfolio. This action cannot be undone. The strategy version history will be lost."),
		mcp.WithString("portfolio_name",
			mcp.Description("Name of the portfolio (e.g., 'SMSF'). Uses default portfolio if not specified."),
		),
	)
}

// createRebuildDataTool returns the rebuild_data tool definition
func createRebuildDataTool() mcp.Tool {
	return mcp.NewTool("rebuild_data",
		mcp.WithDescription("Purge all cached/derived data (portfolios, market data, signals, reports) and rebuild from source. Use when data appears stale or incorrect. Takes several minutes — re-syncs portfolio from Navexa and re-collects market data for all holdings. Signals and reports regenerate lazily on next query."),
		mcp.WithString("portfolio_name",
			mcp.Description("Name of the portfolio (e.g., 'SMSF'). Uses default portfolio if not specified."),
		),
	)
}

// createGetPortfolioPlanTool returns the get_portfolio_plan tool definition
func createGetPortfolioPlanTool() mcp.Tool {
	return mcp.NewTool("get_portfolio_plan",
		mcp.WithDescription("Get the current investment plan for a portfolio. Returns time-based and event-based action items as a formatted markdown document with status indicators."),
		mcp.WithString("portfolio_name",
			mcp.Description("Name of the portfolio (e.g., 'SMSF'). Uses default portfolio if not specified."),
		),
	)
}

// createSetPortfolioPlanTool returns the set_portfolio_plan tool definition
func createSetPortfolioPlanTool() mcp.Tool {
	return mcp.NewTool("set_portfolio_plan",
		mcp.WithDescription("Set or update the investment plan for a portfolio. Replaces the entire plan with the provided items. Use add_plan_item/update_plan_item/remove_plan_item for incremental changes."),
		mcp.WithString("portfolio_name",
			mcp.Description("Name of the portfolio (e.g., 'SMSF'). Uses default portfolio if not specified."),
		),
		mcp.WithString("plan_json",
			mcp.Required(),
			mcp.Description("JSON object with plan fields. Example: {\"items\":[{\"type\":\"time\",\"description\":\"Rebalance portfolio\",\"deadline\":\"2026-06-01\"}],\"notes\":\"Q2 rebalance plan\"}"),
		),
	)
}

// createAddPlanItemTool returns the add_plan_item tool definition
func createAddPlanItemTool() mcp.Tool {
	return mcp.NewTool("add_plan_item",
		mcp.WithDescription("Add a single action item to a portfolio plan. Supports time-based items (with deadlines) and event-based items (with conditions that trigger on market data). Auto-generates an ID if not provided."),
		mcp.WithString("portfolio_name",
			mcp.Description("Name of the portfolio (e.g., 'SMSF'). Uses default portfolio if not specified."),
		),
		mcp.WithString("item_json",
			mcp.Required(),
			mcp.Description("JSON object for the plan item. Time-based: {\"type\":\"time\",\"description\":\"Rebalance\",\"deadline\":\"2026-06-01\",\"action\":\"WATCH\"}. Event-based: {\"type\":\"event\",\"description\":\"Buy BHP on dip\",\"ticker\":\"BHP.AU\",\"action\":\"BUY\",\"conditions\":[{\"field\":\"signals.rsi\",\"operator\":\"<\",\"value\":30}]}"),
		),
	)
}

// createUpdatePlanItemTool returns the update_plan_item tool definition
func createUpdatePlanItemTool() mcp.Tool {
	return mcp.NewTool("update_plan_item",
		mcp.WithDescription("Update an existing plan item by ID. Uses merge semantics — only include fields you want to change."),
		mcp.WithString("portfolio_name",
			mcp.Description("Name of the portfolio (e.g., 'SMSF'). Uses default portfolio if not specified."),
		),
		mcp.WithString("item_id",
			mcp.Required(),
			mcp.Description("ID of the plan item to update (e.g., 'plan-1')"),
		),
		mcp.WithString("item_json",
			mcp.Required(),
			mcp.Description("JSON object with fields to update. Example: {\"status\":\"completed\",\"notes\":\"Done on 2026-02-08\"}"),
		),
	)
}

// createRemovePlanItemTool returns the remove_plan_item tool definition
func createRemovePlanItemTool() mcp.Tool {
	return mcp.NewTool("remove_plan_item",
		mcp.WithDescription("Remove a plan item by ID."),
		mcp.WithString("portfolio_name",
			mcp.Description("Name of the portfolio (e.g., 'SMSF'). Uses default portfolio if not specified."),
		),
		mcp.WithString("item_id",
			mcp.Required(),
			mcp.Description("ID of the plan item to remove (e.g., 'plan-1')"),
		),
	)
}

// createCheckPlanStatusTool returns the check_plan_status tool definition
func createCheckPlanStatusTool() mcp.Tool {
	return mcp.NewTool("check_plan_status",
		mcp.WithDescription("Evaluate plan status: checks event-based items against current market data for triggers, and time-based items for deadline expiry. Returns triggered and expired items."),
		mcp.WithString("portfolio_name",
			mcp.Description("Name of the portfolio (e.g., 'SMSF'). Uses default portfolio if not specified."),
		),
	)
}

// createFunnelScreenTool returns the funnel_screen tool definition
func createFunnelScreenTool() mcp.Tool {
	return mcp.NewTool("funnel_screen",
		mcp.WithDescription("SLOW: Extended version of stock_screen with stage-by-stage visibility and timing. Runs a 3-stage funnel: broad EODHD screener (100) -> fundamental refinement (25) -> technical + signal scoring (5). Same pipeline as stock_screen but with wider funnel and detailed stage breakdown. Costs ~50 EODHD API calls + 5 Gemini calls. Results are saved to search history for recall."),
		mcp.WithString("exchange",
			mcp.Required(),
			mcp.Description("Exchange to scan (e.g., 'AU' for ASX, 'US' for NYSE/NASDAQ)"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum final results to return (default: 5, max: 10)"),
		),
		mcp.WithString("sector",
			mcp.Description("Filter by sector (e.g., 'Technology', 'Healthcare', 'Financials')"),
		),
		mcp.WithBoolean("include_news",
			mcp.Description("Include news sentiment analysis (default: false)"),
		),
		mcp.WithString("portfolio_name",
			mcp.Description("Name of the portfolio for strategy loading. Uses default portfolio if not specified."),
		),
	)
}

// createListSearchesTool returns the list_searches tool definition
func createListSearchesTool() mcp.Tool {
	return mcp.NewTool("list_searches",
		mcp.WithDescription("List recent stock screen and search history. Shows past funnel_screen, stock_screen, and market_snipe results with timestamps."),
		mcp.WithString("type",
			mcp.Description("Filter by search type: 'screen', 'snipe', 'funnel' (default: all)"),
		),
		mcp.WithString("exchange",
			mcp.Description("Filter by exchange (e.g., 'AU', 'US')"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum results to return (default: 10)"),
		),
	)
}

// createGetSearchTool returns the get_search tool definition
func createGetSearchTool() mcp.Tool {
	return mcp.NewTool("get_search",
		mcp.WithDescription("Retrieve a specific past search result by ID. Use list_searches to find IDs."),
		mcp.WithString("search_id",
			mcp.Required(),
			mcp.Description("The search record ID (e.g., 'search-1707350400-AU')"),
		),
	)
}

// createGetWatchlistTool returns the get_watchlist tool definition
func createGetWatchlistTool() mcp.Tool {
	return mcp.NewTool("get_watchlist",
		mcp.WithDescription("Get the stock watchlist for a portfolio. Returns verdicts (PASS/WATCH/FAIL) grouped by verdict with review dates and reasoning."),
		mcp.WithString("portfolio_name",
			mcp.Description("Name of the portfolio (e.g., 'SMSF'). Uses default portfolio if not specified."),
		),
	)
}

// createAddWatchlistItemTool returns the add_watchlist_item tool definition
func createAddWatchlistItemTool() mcp.Tool {
	return mcp.NewTool("add_watchlist_item",
		mcp.WithDescription("Add or update a stock on the watchlist with a verdict (upsert keyed on ticker). If the ticker already exists, it updates the existing entry. Use this after reviewing a stock against the portfolio strategy."),
		mcp.WithString("portfolio_name",
			mcp.Description("Name of the portfolio (e.g., 'SMSF'). Uses default portfolio if not specified."),
		),
		mcp.WithString("item_json",
			mcp.Required(),
			mcp.Description("JSON object for the watchlist item. Example: {\"ticker\":\"SGI.AU\",\"name\":\"Stealth Global\",\"verdict\":\"WATCH\",\"reason\":\"Revenue growing but acquisition-driven\",\"key_metrics\":\"PE 12, Rev $180M\",\"notes\":\"Re-check after Feb results\"}"),
		),
	)
}

// createUpdateWatchlistItemTool returns the update_watchlist_item tool definition
func createUpdateWatchlistItemTool() mcp.Tool {
	return mcp.NewTool("update_watchlist_item",
		mcp.WithDescription("Update an existing watchlist item by ticker. Uses merge semantics — only include fields you want to change."),
		mcp.WithString("portfolio_name",
			mcp.Description("Name of the portfolio (e.g., 'SMSF'). Uses default portfolio if not specified."),
		),
		mcp.WithString("ticker",
			mcp.Required(),
			mcp.Description("Ticker to update (e.g., 'SGI.AU')"),
		),
		mcp.WithString("item_json",
			mcp.Required(),
			mcp.Description("JSON object with fields to update. Example: {\"verdict\":\"PASS\",\"reason\":\"Strong H1 results confirmed growth thesis\"}"),
		),
	)
}

// createRemoveWatchlistItemTool returns the remove_watchlist_item tool definition
func createRemoveWatchlistItemTool() mcp.Tool {
	return mcp.NewTool("remove_watchlist_item",
		mcp.WithDescription("Remove a stock from the watchlist by ticker."),
		mcp.WithString("portfolio_name",
			mcp.Description("Name of the portfolio (e.g., 'SMSF'). Uses default portfolio if not specified."),
		),
		mcp.WithString("ticker",
			mcp.Required(),
			mcp.Description("Ticker to remove (e.g., 'SGI.AU')"),
		),
	)
}

// createSetWatchlistTool returns the set_watchlist tool definition
func createSetWatchlistTool() mcp.Tool {
	return mcp.NewTool("set_watchlist",
		mcp.WithDescription("Replace the entire watchlist for a portfolio. Use add_watchlist_item for incremental changes."),
		mcp.WithString("portfolio_name",
			mcp.Description("Name of the portfolio (e.g., 'SMSF'). Uses default portfolio if not specified."),
		),
		mcp.WithString("watchlist_json",
			mcp.Required(),
			mcp.Description("JSON object with watchlist fields. Example: {\"items\":[{\"ticker\":\"SGI.AU\",\"verdict\":\"WATCH\",\"reason\":\"Under review\"}],\"notes\":\"Q2 watchlist\"}"),
		),
	)
}

// createCollectMarketDataTool returns the collect_market_data tool definition
func createCollectMarketDataTool() mcp.Tool {
	return mcp.NewTool("collect_market_data",
		mcp.WithDescription("Collect and store market data for specified tickers. Use this to pre-fetch data for analysis."),
		mcp.WithArray("tickers",
			mcp.WithStringItems(),
			mcp.Required(),
			mcp.Description("List of tickers to collect data for (e.g., ['BHP.AU', 'CBA.AU'])"),
		),
		mcp.WithBoolean("include_news",
			mcp.Description("Include news articles (default: false)"),
		),
	)
}
