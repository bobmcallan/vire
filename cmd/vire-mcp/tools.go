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
