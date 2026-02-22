package server

import "github.com/bobmcallan/vire/internal/models"

// buildToolCatalog returns the full MCP tool catalog describing all active tools
// and their HTTP mappings. Used by GET /api/mcp/tools for dynamic tool registration.
func buildToolCatalog() []models.ToolDefinition {
	portfolioParam := models.ParamDefinition{
		Name:        "portfolio_name",
		Type:        "string",
		Description: "Name of the portfolio. Uses default portfolio if not specified.",
		In:          "path",
		DefaultFrom: "user_config.default_portfolio",
	}

	return []models.ToolDefinition{
		// --- System ---
		{
			Name:        "get_version",
			Description: "Get the Vire MCP server version and status. Use this to verify connectivity.",
			Method:      "GET",
			Path:        "/api/version",
		},
		{
			Name:        "get_config",
			Description: "List all Vire configuration settings.",
			Method:      "GET",
			Path:        "/api/config",
		},
		{
			Name:        "get_diagnostics",
			Description: "Get server diagnostics: uptime, version, recent log entries.",
			Method:      "GET",
			Path:        "/api/diagnostics",
			Params: []models.ParamDefinition{
				{
					Name:        "correlation_id",
					Type:        "string",
					Description: "If provided, returns logs for a specific correlation ID",
					In:          "query",
				},
				{
					Name:        "limit",
					Type:        "number",
					Description: "Maximum recent log entries (default: 50)",
					In:          "query",
				},
			},
		},

		// --- Portfolios ---
		{
			Name:        "list_portfolios",
			Description: "List all available portfolios that can be reviewed.",
			Method:      "GET",
			Path:        "/api/portfolios",
		},
		{
			Name:        "set_default_portfolio",
			Description: "Set the default portfolio name. Call without portfolio_name to list available portfolios.",
			Method:      "PUT",
			Path:        "/api/portfolios/default",
			Params: []models.ParamDefinition{
				{
					Name:        "name",
					Type:        "string",
					Description: "Portfolio name to set as default. Omit to list available portfolios.",
					In:          "body",
				},
			},
		},
		{
			Name:        "get_portfolio",
			Description: "FAST: Get current portfolio holdings \u2014 tickers, names, values, weights, and gains. Return percentages use total capital invested as denominator (average cost basis for partial sells). Includes realized/unrealized gain breakdown, true breakeven price (accounts for prior realized P&L), net P&L if sold today, price targets and stop losses. No signals, charts, or AI analysis. Use portfolio_compliance for full analysis.",
			Method:      "GET",
			Path:        "/api/portfolios/{portfolio_name}",
			Params: []models.ParamDefinition{
				{
					Name:        "portfolio_name",
					Type:        "string",
					Description: "Name of the portfolio (e.g., 'SMSF', 'Personal'). Uses default portfolio if not specified.",
					In:          "path",
					DefaultFrom: "user_config.default_portfolio",
				},
			},
		},
		{
			Name:        "get_portfolio_stock",
			Description: "FAST: Get portfolio position data for a single holding \u2014 position details, trade history, dividends, and returns. Return percentages use total capital invested as denominator (average cost basis for partial sells). Includes realized/unrealized gain breakdown, true breakeven price (accounts for prior realized P&L), net P&L if sold today, price targets and stop losses. No market data or signals. Use get_stock_data for market analysis.",
			Method:      "GET",
			Path:        "/api/portfolios/{portfolio_name}/stock/{ticker}",
			Params: []models.ParamDefinition{
				portfolioParam,
				{
					Name:        "ticker",
					Type:        "string",
					Description: "Ticker symbol (e.g., 'BHP', 'BHP.AU', 'NVDA.US')",
					Required:    true,
					In:          "path",
				},
				{
					Name:        "force_refresh",
					Type:        "boolean",
					Description: "Force a fresh sync from Navexa, ignoring cache (default: false)",
					In:          "query",
				},
			},
		},
		{
			Name:        "portfolio_compliance",
			Description: "Review a portfolio for signals, overnight movement, and actionable observations. Returns a comprehensive analysis of holdings with compliance status classifications.",
			Method:      "POST",
			Path:        "/api/portfolios/{portfolio_name}/review",
			Params: []models.ParamDefinition{
				{
					Name:        "portfolio_name",
					Type:        "string",
					Description: "Name of the portfolio to review (e.g., 'SMSF', 'Personal'). Uses default portfolio if not specified.",
					In:          "path",
					DefaultFrom: "user_config.default_portfolio",
				},
				{
					Name:        "focus_signals",
					Type:        "array",
					Description: "Signal types to focus on: sma, rsi, volume, pbas, vli, regime, trend, support_resistance, macd",
					In:          "body",
				},
				{
					Name:        "include_news",
					Type:        "boolean",
					Description: "Include news sentiment analysis (default: false)",
					In:          "body",
				},
			},
		},
		{
			Name:        "generate_report",
			Description: "SLOW: Generate a full portfolio report from scratch \u2014 syncs holdings, collects market data, runs signals for every ticker. Takes several minutes.",
			Method:      "POST",
			Path:        "/api/portfolios/{portfolio_name}/report",
			Params: []models.ParamDefinition{
				{
					Name:        "portfolio_name",
					Type:        "string",
					Description: "Name of the portfolio to generate a report for. Uses default portfolio if not specified.",
					In:          "path",
					DefaultFrom: "user_config.default_portfolio",
				},
				{
					Name:        "force_refresh",
					Type:        "boolean",
					Description: "Force refresh of portfolio data even if recently synced (default: false)",
					In:          "body",
				},
				{
					Name:        "include_news",
					Type:        "boolean",
					Description: "Include news sentiment analysis (default: false)",
					In:          "body",
				},
			},
		},
		{
			Name:        "get_summary",
			Description: "FAST: Get portfolio summary. Auto-generates if no cached report exists.",
			Method:      "GET",
			Path:        "/api/portfolios/{portfolio_name}/summary",
			Params: []models.ParamDefinition{
				portfolioParam,
			},
		},

		// --- Strategy ---
		{
			Name:        "get_portfolio_strategy",
			Description: "FAST: Get the investment strategy document for a portfolio.",
			Method:      "GET",
			Path:        "/api/portfolios/{portfolio_name}/strategy",
			Params: []models.ParamDefinition{
				portfolioParam,
			},
		},
		{
			Name:        "set_portfolio_strategy",
			Description: "Set or update the investment strategy for a portfolio. Uses MERGE semantics.",
			Method:      "PUT",
			Path:        "/api/portfolios/{portfolio_name}/strategy",
			Params: []models.ParamDefinition{
				portfolioParam,
				{
					Name: "strategy",
					Type: "object",
					Description: "Strategy fields as a JSON object (merged with existing). " +
						"Optional fields: account_type (smsf|trading), investment_universe ([\"AU\",\"US\"]), " +
						"risk_appetite {level, max_drawdown_pct, description}, " +
						"target_returns {annual_pct, timeframe}, income_requirements {dividend_yield_pct, description}, " +
						"sector_preferences {preferred [], excluded []}, position_sizing {max_position_pct, max_sector_pct}, " +
						"company_filter {min_market_cap, max_market_cap, max_pe, min_dividend_yield, allowed_sectors [], excluded_sectors []}, " +
						"rules [{name, conditions [{field, operator, value}], action (SELL|BUY|HOLD|WATCH), reason, priority, enabled}], " +
						"rebalance_frequency, notes (free-form markdown).",
					Required: true,
					In:       "body",
				},
			},
		},
		{
			Name:        "delete_portfolio_strategy",
			Description: "Delete the investment strategy for a portfolio.",
			Method:      "DELETE",
			Path:        "/api/portfolios/{portfolio_name}/strategy",
			Params: []models.ParamDefinition{
				portfolioParam,
			},
		},

		// --- Plan ---
		{
			Name:        "get_portfolio_plan",
			Description: "Get the current investment plan for a portfolio.",
			Method:      "GET",
			Path:        "/api/portfolios/{portfolio_name}/plan",
			Params: []models.ParamDefinition{
				portfolioParam,
			},
		},
		{
			Name:        "set_portfolio_plan",
			Description: "Set or update the investment plan for a portfolio.",
			Method:      "PUT",
			Path:        "/api/portfolios/{portfolio_name}/plan",
			Params: []models.ParamDefinition{
				portfolioParam,
				{
					Name: "items",
					Type: "array",
					Description: "Plan action items. Array of objects: " +
						"{type (time|event), description, status (pending|triggered|completed|expired|cancelled), " +
						"deadline (ISO date, time-based), ticker (event-based), " +
						"conditions [{field, operator, value}] (event-based), " +
						"action (SELL|BUY|HOLD|WATCH), target_value, notes}.",
					Required: true,
					In:       "body",
				},
				{
					Name:        "notes",
					Type:        "string",
					Description: "Free-form plan notes.",
					In:          "body",
				},
			},
		},
		{
			Name:        "add_plan_item",
			Description: "Add a single action item to a portfolio plan.",
			Method:      "POST",
			Path:        "/api/portfolios/{portfolio_name}/plan/items",
			Params: []models.ParamDefinition{
				portfolioParam,
				{
					Name:        "type",
					Type:        "string",
					Description: "Item type: time_based or event_based.",
					Required:    true,
					In:          "body",
				},
				{
					Name:        "description",
					Type:        "string",
					Description: "Description of the action item.",
					Required:    true,
					In:          "body",
				},
				{
					Name:        "deadline",
					Type:        "string",
					Description: "Deadline in ISO 8601 format (for time-based items).",
					In:          "body",
				},
				{
					Name:        "ticker",
					Type:        "string",
					Description: "Target ticker symbol (for event-based items).",
					In:          "body",
				},
				{
					Name:        "conditions",
					Type:        "array",
					Description: "Trigger conditions (for event-based items).",
					In:          "body",
				},
				{
					Name:        "action",
					Type:        "object",
					Description: "Action to take when triggered.",
					In:          "body",
				},
				{
					Name:        "target_value",
					Type:        "number",
					Description: "Target value for the action.",
					In:          "body",
				},
				{
					Name:        "notes",
					Type:        "string",
					Description: "Free-form notes.",
					In:          "body",
				},
			},
		},
		{
			Name:        "update_plan_item",
			Description: "Update an existing plan item by ID. Uses merge semantics.",
			Method:      "PATCH",
			Path:        "/api/portfolios/{portfolio_name}/plan/items/{item_id}",
			Params: []models.ParamDefinition{
				portfolioParam,
				{
					Name:        "item_id",
					Type:        "string",
					Description: "ID of the plan item to update",
					Required:    true,
					In:          "path",
				},
				{
					Name:        "status",
					Type:        "string",
					Description: "New status: pending, in_progress, completed, cancelled.",
					In:          "body",
				},
				{
					Name:        "description",
					Type:        "string",
					Description: "Updated description.",
					In:          "body",
				},
				{
					Name:        "notes",
					Type:        "string",
					Description: "Updated notes.",
					In:          "body",
				},
			},
		},
		{
			Name:        "remove_plan_item",
			Description: "Remove a plan item by ID.",
			Method:      "DELETE",
			Path:        "/api/portfolios/{portfolio_name}/plan/items/{item_id}",
			Params: []models.ParamDefinition{
				portfolioParam,
				{
					Name:        "item_id",
					Type:        "string",
					Description: "ID of the plan item to remove",
					Required:    true,
					In:          "path",
				},
			},
		},
		{
			Name:        "check_plan_status",
			Description: "Evaluate plan status: checks event triggers and deadline expiry.",
			Method:      "GET",
			Path:        "/api/portfolios/{portfolio_name}/plan/status",
			Params: []models.ParamDefinition{
				portfolioParam,
			},
		},

		// --- Market Data ---
		{
			Name:        "get_quote",
			Description: "FAST: Get a real-time price quote for a single ticker. Returns OHLCV, change%, and previous close. Use for spot-checking 1-3 prices \u2014 for broad analysis prefer get_stock_data. Supports stocks (BHP.AU, AAPL.US), forex (AUDUSD.FOREX, EURUSD.FOREX), and commodities (XAUUSD.FOREX for gold, XAGUSD.FOREX for silver).",
			Method:      "GET",
			Path:        "/api/market/quote/{ticker}",
			Params: []models.ParamDefinition{
				{
					Name:        "ticker",
					Type:        "string",
					Description: "Ticker with exchange suffix (e.g., 'BHP.AU', 'AAPL.US', 'AUDUSD.FOREX', 'XAUUSD.FOREX')",
					Required:    true,
					In:          "path",
				},
			},
		},
		{
			Name:        "get_stock_data",
			Description: "Get comprehensive stock data including price, fundamentals, signals, and news for a specific ticker.",
			Method:      "GET",
			Path:        "/api/market/stocks/{ticker}",
			Params: []models.ParamDefinition{
				{
					Name:        "ticker",
					Type:        "string",
					Description: "Stock ticker with exchange suffix (e.g., 'BHP.AU', 'AAPL.US')",
					Required:    true,
					In:          "path",
				},
				{
					Name:        "include",
					Type:        "array",
					Description: "Data to include: price, fundamentals, signals, news (default: all)",
					In:          "query",
				},
			},
		},
		{
			Name:        "compute_indicators",
			Description: "Compute technical indicators for specified tickers. Returns raw indicator values, trend classification, and risk flags.",
			Method:      "POST",
			Path:        "/api/market/signals",
			Params: []models.ParamDefinition{
				{
					Name:        "tickers",
					Type:        "array",
					Description: "List of tickers to analyze (e.g., ['BHP.AU', 'CBA.AU'])",
					Required:    true,
					In:          "body",
				},
				{
					Name:        "signal_types",
					Type:        "array",
					Description: "Signal types to compute: sma, rsi, volume, pbas, vli, regime, trend (default: all)",
					In:          "body",
				},
			},
		},

		// --- Screening ---
		{
			Name:        "strategy_scanner",
			Description: "Scan for tickers matching strategy entry criteria. Filters by technical indicators, volume patterns, and price levels.",
			Method:      "POST",
			Path:        "/api/screen/snipe",
			Params: []models.ParamDefinition{
				{
					Name:        "exchange",
					Type:        "string",
					Description: "Exchange to scan (e.g., 'AU' for ASX, 'US' for NYSE/NASDAQ)",
					Required:    true,
					In:          "body",
				},
				{
					Name:        "limit",
					Type:        "number",
					Description: "Maximum results to return (default: 3, max: 10)",
					In:          "body",
				},
				{
					Name:        "criteria",
					Type:        "array",
					Description: "Filter criteria: oversold_rsi, near_support, underpriced, accumulating, regime_shift",
					In:          "body",
				},
				{
					Name:        "sector",
					Type:        "string",
					Description: "Filter by sector (e.g., 'Technology', 'Healthcare', 'Mining')",
					In:          "body",
				},
				{
					Name:        "include_news",
					Type:        "boolean",
					Description: "Include news sentiment analysis (default: false)",
					In:          "body",
				},
				{
					Name:        "portfolio_name",
					Type:        "string",
					Description: "Name of the portfolio for strategy loading. Uses default portfolio if not specified.",
					In:          "body",
					DefaultFrom: "user_config.default_portfolio",
				},
			},
		},
		{
			Name:        "stock_screen",
			Description: "Screen for stocks matching quantitative filters: low P/E, positive earnings, consistent quarterly returns (10%+ annualised), upward price trajectory, and credible news support.",
			Method:      "POST",
			Path:        "/api/screen",
			Params: []models.ParamDefinition{
				{
					Name:        "exchange",
					Type:        "string",
					Description: "Exchange to scan (e.g., 'AU' for ASX, 'US' for NYSE/NASDAQ)",
					Required:    true,
					In:          "body",
				},
				{
					Name:        "limit",
					Type:        "number",
					Description: "Maximum results to return (default: 5, max: 15)",
					In:          "body",
				},
				{
					Name:        "max_pe",
					Type:        "number",
					Description: "Maximum P/E ratio filter (default: 20)",
					In:          "body",
				},
				{
					Name:        "min_return",
					Type:        "number",
					Description: "Minimum annualised quarterly return percentage (default: 10)",
					In:          "body",
				},
				{
					Name:        "sector",
					Type:        "string",
					Description: "Filter by sector (e.g., 'Technology', 'Healthcare', 'Financials')",
					In:          "body",
				},
				{
					Name:        "include_news",
					Type:        "boolean",
					Description: "Include news sentiment analysis (default: false)",
					In:          "body",
				},
				{
					Name:        "portfolio_name",
					Type:        "string",
					Description: "Name of the portfolio for strategy loading. Uses default portfolio if not specified.",
					In:          "body",
					DefaultFrom: "user_config.default_portfolio",
				},
			},
		},

		// --- Reports ---
		{
			Name:        "list_reports",
			Description: "List available portfolio reports with their generation timestamps.",
			Method:      "GET",
			Path:        "/api/reports",
			Params: []models.ParamDefinition{
				{
					Name:        "portfolio_name",
					Type:        "string",
					Description: "Optional: filter to a specific portfolio name",
					In:          "query",
				},
			},
		},

		// --- Strategy template ---
		{
			Name:        "get_strategy_template",
			Description: "Get a template showing all available strategy fields and examples.",
			Method:      "GET",
			Path:        "/api/strategies/template",
			Params: []models.ParamDefinition{
				{
					Name:        "account_type",
					Type:        "string",
					Description: "Account type for tailored guidance: 'smsf' or 'trading'.",
					In:          "query",
				},
			},
		},
	}
}
