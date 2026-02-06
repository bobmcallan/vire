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
			mcp.Required(),
			mcp.Description("Name of the portfolio to review (e.g., 'SMSF', 'Personal')"),
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
			mcp.Required(),
			mcp.Description("Name of the portfolio to sync"),
		),
		mcp.WithBoolean("force",
			mcp.Description("Force sync even if recently synced (default: false)"),
		),
	)
}

// createGenerateReportTool returns the generate_report tool definition
func createGenerateReportTool() mcp.Tool {
	return mcp.NewTool("generate_report",
		mcp.WithDescription("Generate a full portfolio report (sync, collect, signals, review) and store it. Returns cached report if fresh (<1hr) unless force_refresh=true. Smart caching respects per-component data freshness."),
		mcp.WithString("portfolio_name",
			mcp.Required(),
			mcp.Description("Name of the portfolio to generate a report for (e.g., 'SMSF', 'Personal')"),
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
		mcp.WithDescription("Regenerate the report for a single ticker within an existing portfolio report. Refreshes market data and signals for just that ticker."),
		mcp.WithString("portfolio_name",
			mcp.Required(),
			mcp.Description("Name of the portfolio (e.g., 'SMSF')"),
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
		mcp.WithDescription("Get the summary markdown for a portfolio report. Auto-generates if no cached report exists or the cached report is stale (>1hr). Contains holdings tables, portfolio balance, alerts, and recommendations (no per-ticker details)."),
		mcp.WithString("portfolio_name",
			mcp.Required(),
			mcp.Description("Name of the portfolio (e.g., 'SMSF')"),
		),
	)
}

// createGetTickerReportTool returns the get_ticker_report tool definition
func createGetTickerReportTool() mcp.Tool {
	return mcp.NewTool("get_ticker_report",
		mcp.WithDescription("Get the per-ticker report markdown for a specific holding. Auto-generates if no cached report exists or the cached report is stale (>1hr). Contains position, fundamentals/fund metrics, technical signals, and risk flags."),
		mcp.WithString("portfolio_name",
			mcp.Required(),
			mcp.Description("Name of the portfolio (e.g., 'SMSF')"),
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
			mcp.Required(),
			mcp.Description("Name of the portfolio (e.g., 'SMSF')"),
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
