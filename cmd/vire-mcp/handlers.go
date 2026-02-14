package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/bobmcallan/vire/internal/models"
)

// --- Helpers ---

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

func getString(request mcp.CallToolRequest, key, defaultVal string) string {
	v := request.GetString(key, defaultVal)
	return v
}

func getInt(request mcp.CallToolRequest, key string, defaultVal int) int {
	return request.GetInt(key, defaultVal)
}

func getFloat(request mcp.CallToolRequest, key string, defaultVal float64) float64 {
	return request.GetFloat(key, defaultVal)
}

func getBool(request mcp.CallToolRequest, key string, defaultVal bool) bool {
	return request.GetBool(key, defaultVal)
}

func getStringSlice(request mcp.CallToolRequest, key string) []string {
	return request.GetStringSlice(key, nil)
}

func requireString(request mcp.CallToolRequest, key string) (string, error) {
	return request.RequireString(key)
}

// resolvePortfolioViaAPI resolves the portfolio name: if provided in the request, use it;
// otherwise call the server's default endpoint.
func resolvePortfolioViaAPI(p *MCPProxy, request mcp.CallToolRequest) string {
	name := getString(request, "portfolio_name", "")
	if name != "" {
		return name
	}

	// Ask the server for the default
	body, err := p.get("/api/portfolios/default")
	if err != nil {
		return ""
	}
	var resp struct {
		Default string `json:"default"`
	}
	if json.Unmarshal(body, &resp) == nil && resp.Default != "" {
		return resp.Default
	}
	return ""
}

// --- Handlers ---

func handleGetVersion(p *MCPProxy) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		body, err := p.get("/api/version")
		if err != nil {
			return errorResult(fmt.Sprintf("Error: %v", err)), nil
		}

		var resp struct {
			Version string `json:"version"`
			Build   string `json:"build"`
			Commit  string `json:"commit"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return errorResult(fmt.Sprintf("Error parsing response: %v", err)), nil
		}

		result := fmt.Sprintf("Vire MCP Server\nVersion: %s\nBuild: %s\nCommit: %s\nStatus: OK",
			resp.Version, resp.Build, resp.Commit)
		return textResult(result), nil
	}
}

func handlePortfolioCompliance(p *MCPProxy) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioViaAPI(p, request)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
		}

		reqBody := map[string]interface{}{}
		if fs := getStringSlice(request, "focus_signals"); len(fs) > 0 {
			reqBody["focus_signals"] = fs
		}
		if getBool(request, "include_news", false) {
			reqBody["include_news"] = true
		}

		body, err := p.post(fmt.Sprintf("/api/portfolios/%s/review", url.PathEscape(portfolioName)), reqBody)
		if err != nil {
			return errorResult(fmt.Sprintf("Review error: %v", err)), nil
		}

		var resp struct {
			Review   *models.PortfolioReview   `json:"review"`
			Strategy *models.PortfolioStrategy `json:"strategy"`
			Growth   []models.GrowthDataPoint  `json:"growth"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return errorResult(fmt.Sprintf("Error parsing response: %v", err)), nil
		}

		markdown := formatPortfolioReview(resp.Review)

		if resp.Strategy != nil {
			markdown += formatStrategyContext(resp.Review, resp.Strategy)
		}

		content := []mcp.Content{mcp.NewTextContent(markdown)}

		if len(resp.Growth) > 0 {
			monthlyPoints := downsampleToMonthly(resp.Growth)
			growthMarkdown := formatPortfolioGrowth(monthlyPoints, "")
			content = append(content, mcp.NewTextContent(growthMarkdown))
		}

		return &mcp.CallToolResult{Content: content}, nil
	}
}

func handleGetPortfolio(p *MCPProxy) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioViaAPI(p, request)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
		}

		body, err := p.get(fmt.Sprintf("/api/portfolios/%s", url.PathEscape(portfolioName)))
		if err != nil {
			return errorResult(fmt.Sprintf("Error: %v", err)), nil
		}

		var portfolio models.Portfolio
		if err := json.Unmarshal(body, &portfolio); err != nil {
			return errorResult(fmt.Sprintf("Error parsing response: %v", err)), nil
		}

		return textResult(formatPortfolioHoldings(&portfolio)), nil
	}
}

func handleStrategyScanner(p *MCPProxy) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		exchange, err := requireString(request, "exchange")
		if err != nil || exchange == "" {
			return errorResult("Error: exchange parameter is required"), nil
		}

		portfolioName := resolvePortfolioViaAPI(p, request)

		reqBody := map[string]interface{}{
			"exchange":       exchange,
			"limit":          getInt(request, "limit", 3),
			"include_news":   getBool(request, "include_news", false),
			"portfolio_name": portfolioName,
		}
		if criteria := getStringSlice(request, "criteria"); len(criteria) > 0 {
			reqBody["criteria"] = criteria
		}
		if sector := getString(request, "sector", ""); sector != "" {
			reqBody["sector"] = sector
		}

		body, err := p.post("/api/screen/snipe", reqBody)
		if err != nil {
			return errorResult(fmt.Sprintf("Strategy scan error: %v", err)), nil
		}

		var resp struct {
			SnipeBuys []*models.SnipeBuy `json:"snipe_buys"`
			SearchID  string             `json:"search_id"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return errorResult(fmt.Sprintf("Error parsing response: %v", err)), nil
		}

		markdown := formatSnipeBuys(resp.SnipeBuys, exchange)
		if resp.SearchID != "" {
			markdown += fmt.Sprintf("\n*Search saved: `%s` — use `get_search` to recall*\n", resp.SearchID)
		}
		return textResult(markdown), nil
	}
}

func handleStockScreen(p *MCPProxy) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		exchange, err := requireString(request, "exchange")
		if err != nil || exchange == "" {
			return errorResult("Error: exchange parameter is required"), nil
		}

		portfolioName := resolvePortfolioViaAPI(p, request)

		reqBody := map[string]interface{}{
			"exchange":       exchange,
			"limit":          getInt(request, "limit", 5),
			"max_pe":         getFloat(request, "max_pe", 0),
			"min_return":     getFloat(request, "min_return", 0),
			"include_news":   getBool(request, "include_news", false),
			"portfolio_name": portfolioName,
		}
		if sector := getString(request, "sector", ""); sector != "" {
			reqBody["sector"] = sector
		}

		body, err := p.post("/api/screen", reqBody)
		if err != nil {
			return errorResult(fmt.Sprintf("Screen error: %v", err)), nil
		}

		var resp struct {
			Candidates []*models.ScreenCandidate `json:"candidates"`
			SearchID   string                    `json:"search_id"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return errorResult(fmt.Sprintf("Error parsing response: %v", err)), nil
		}

		// Calculate effective display values
		maxPE := getFloat(request, "max_pe", 0)
		if maxPE <= 0 {
			maxPE = 20.0
		}
		minReturn := getFloat(request, "min_return", 0)
		if minReturn <= 0 {
			minReturn = 10.0
		}

		markdown := formatScreenCandidates(resp.Candidates, exchange, maxPE, minReturn)
		if resp.SearchID != "" {
			markdown += fmt.Sprintf("\n*Search saved: `%s` — use `get_search` to recall*\n", resp.SearchID)
		}
		return textResult(markdown), nil
	}
}

func handleGetStockData(p *MCPProxy) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ticker, err := requireString(request, "ticker")
		if err != nil || ticker == "" {
			return errorResult("Error: ticker parameter is required"), nil
		}

		includes := getStringSlice(request, "include")
		includeParam := ""
		if len(includes) > 0 {
			includeParam = strings.Join(includes, ",")
		}

		path := fmt.Sprintf("/api/market/stocks/%s", url.PathEscape(ticker))
		if includeParam != "" {
			path += "?include=" + url.QueryEscape(includeParam)
		}

		body, err := p.get(path)
		if err != nil {
			return errorResult(fmt.Sprintf("Error getting stock data: %v", err)), nil
		}

		var stockData models.StockData
		if err := json.Unmarshal(body, &stockData); err != nil {
			return errorResult(fmt.Sprintf("Error parsing response: %v", err)), nil
		}

		return textResult(formatStockData(&stockData)), nil
	}
}

func handleComputeIndicators(p *MCPProxy) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tickers := getStringSlice(request, "tickers")
		if len(tickers) == 0 {
			return errorResult("Error: tickers parameter is required"), nil
		}

		reqBody := map[string]interface{}{
			"tickers": tickers,
		}
		if st := getStringSlice(request, "signal_types"); len(st) > 0 {
			reqBody["signal_types"] = st
		}

		body, err := p.post("/api/market/signals", reqBody)
		if err != nil {
			return errorResult(fmt.Sprintf("Indicator computation error: %v", err)), nil
		}

		var resp struct {
			Signals []*models.TickerSignals `json:"signals"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return errorResult(fmt.Sprintf("Error parsing response: %v", err)), nil
		}

		return textResult(formatSignals(resp.Signals)), nil
	}
}

func handleListPortfolios(p *MCPProxy) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		body, err := p.get("/api/portfolios")
		if err != nil {
			return errorResult(fmt.Sprintf("Error listing portfolios: %v", err)), nil
		}

		var resp struct {
			Portfolios []string `json:"portfolios"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return errorResult(fmt.Sprintf("Error parsing response: %v", err)), nil
		}

		return textResult(formatPortfolioList(resp.Portfolios)), nil
	}
}

func handleSyncPortfolio(p *MCPProxy) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioViaAPI(p, request)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
		}

		reqBody := map[string]interface{}{
			"force": getBool(request, "force", false),
		}

		body, err := p.post(fmt.Sprintf("/api/portfolios/%s/sync", url.PathEscape(portfolioName)), reqBody)
		if err != nil {
			return errorResult(fmt.Sprintf("Sync error: %v", err)), nil
		}

		var portfolio models.Portfolio
		if err := json.Unmarshal(body, &portfolio); err != nil {
			return errorResult(fmt.Sprintf("Error parsing response: %v", err)), nil
		}

		return textResult(formatSyncResult(&portfolio)), nil
	}
}

func handleRebuildData(p *MCPProxy) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioViaAPI(p, request)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
		}

		body, err := p.post(fmt.Sprintf("/api/portfolios/%s/rebuild", url.PathEscape(portfolioName)), nil)
		if err != nil {
			return errorResult(fmt.Sprintf("Rebuild error: %v", err)), nil
		}

		var resp struct {
			Purged  map[string]int `json:"purged"`
			Rebuilt struct {
				Portfolio     string `json:"portfolio"`
				Holdings      int    `json:"holdings"`
				MarketTickers int    `json:"market_tickers"`
				SchemaVersion string `json:"schema_version"`
			} `json:"rebuilt"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return errorResult(fmt.Sprintf("Error parsing response: %v", err)), nil
		}

		var sb strings.Builder
		sb.WriteString("# Data Rebuild Complete\n\n")
		sb.WriteString("## Purged\n\n")
		sb.WriteString("| Type | Count |\n")
		sb.WriteString("|------|-------|\n")
		for k, v := range resp.Purged {
			sb.WriteString(fmt.Sprintf("| %s | %d |\n", k, v))
		}
		sb.WriteString("\n## Rebuilt\n\n")
		sb.WriteString(fmt.Sprintf("- Portfolio **%s** synced (%d holdings)\n", resp.Rebuilt.Portfolio, resp.Rebuilt.Holdings))
		sb.WriteString(fmt.Sprintf("- Market data collected for **%d** tickers\n", resp.Rebuilt.MarketTickers))
		sb.WriteString(fmt.Sprintf("- Schema version set to **%s**\n", resp.Rebuilt.SchemaVersion))
		sb.WriteString("\n*Signals and reports will regenerate lazily on next query.*\n")

		return textResult(sb.String()), nil
	}
}

func handleCollectMarketData(p *MCPProxy) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tickers := getStringSlice(request, "tickers")
		if len(tickers) == 0 {
			return errorResult("Error: tickers parameter is required"), nil
		}

		reqBody := map[string]interface{}{
			"tickers":      tickers,
			"include_news": getBool(request, "include_news", false),
		}

		body, err := p.post("/api/market/collect", reqBody)
		if err != nil {
			return errorResult(fmt.Sprintf("Collection error: %v", err)), nil
		}

		var resp struct {
			Collected int      `json:"collected"`
			Tickers   []string `json:"tickers"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return errorResult(fmt.Sprintf("Error parsing response: %v", err)), nil
		}

		return textResult(formatCollectResult(resp.Tickers)), nil
	}
}

func handleGenerateReport(p *MCPProxy) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioViaAPI(p, request)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
		}

		reqBody := map[string]interface{}{
			"force_refresh": getBool(request, "force_refresh", false),
			"include_news":  getBool(request, "include_news", false),
		}

		body, err := p.post(fmt.Sprintf("/api/portfolios/%s/report", url.PathEscape(portfolioName)), reqBody)
		if err != nil {
			return errorResult(fmt.Sprintf("Report generation error: %v", err)), nil
		}

		var resp struct {
			Cached      bool     `json:"cached"`
			GeneratedAt string   `json:"generated_at"`
			Tickers     []string `json:"tickers"`
			TickerCount int      `json:"ticker_count"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return errorResult(fmt.Sprintf("Error parsing response: %v", err)), nil
		}

		var result string
		if resp.Cached {
			result = fmt.Sprintf("Report is current for %s (cached)\n\nTickers: %d\nGenerated at: %s\nTickers: %s",
				portfolioName, resp.TickerCount, resp.GeneratedAt, strings.Join(resp.Tickers, ", "))
		} else {
			result = fmt.Sprintf("Report generated for %s\n\nTickers: %d\nGenerated at: %s\nTickers: %s",
				portfolioName, resp.TickerCount, resp.GeneratedAt, strings.Join(resp.Tickers, ", "))
		}
		return textResult(result), nil
	}
}

func handleGetPortfolioStock(p *MCPProxy) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioViaAPI(p, request)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
		}

		ticker, err := requireString(request, "ticker")
		if err != nil || ticker == "" {
			return errorResult("Error: ticker parameter is required"), nil
		}

		body, err := p.get(fmt.Sprintf("/api/portfolios/%s", url.PathEscape(portfolioName)))
		if err != nil {
			return errorResult(fmt.Sprintf("Error: %v", err)), nil
		}

		var portfolio models.Portfolio
		if err := json.Unmarshal(body, &portfolio); err != nil {
			return errorResult(fmt.Sprintf("Error parsing response: %v", err)), nil
		}

		// Find matching holding — match against both bare ticker and qualified EODHD ticker
		var found *models.Holding
		for i := range portfolio.Holdings {
			h := &portfolio.Holdings[i]
			if matchTicker(ticker, h.Ticker) || matchTicker(ticker, h.EODHDTicker()) {
				found = h
				break
			}
		}
		if found == nil {
			available := make([]string, 0, len(portfolio.Holdings))
			for _, h := range portfolio.Holdings {
				available = append(available, h.Ticker)
			}
			return errorResult(fmt.Sprintf("Ticker '%s' not found in portfolio '%s'. Available: %s",
				ticker, portfolioName, strings.Join(available, ", "))), nil
		}

		return textResult(formatPortfolioStock(found, &portfolio)), nil
	}
}

func handleListReports(p *MCPProxy) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path := "/api/reports"
		if filterName := getString(request, "portfolio_name", ""); filterName != "" {
			path += "?portfolio_name=" + url.QueryEscape(filterName)
		}

		body, err := p.get(path)
		if err != nil {
			return errorResult(fmt.Sprintf("Error listing reports: %v", err)), nil
		}

		var resp struct {
			Reports []struct {
				PortfolioName string `json:"portfolio_name"`
				GeneratedAt   string `json:"generated_at"`
				TickerCount   int    `json:"ticker_count"`
			} `json:"reports"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return errorResult(fmt.Sprintf("Error parsing response: %v", err)), nil
		}

		if len(resp.Reports) == 0 {
			return textResult("No reports available. Use `generate_report` to create one."), nil
		}

		var sb strings.Builder
		sb.WriteString("# Available Reports\n\n")
		for _, r := range resp.Reports {
			sb.WriteString(fmt.Sprintf("- **%s** — Generated: %s — Tickers: %d\n",
				r.PortfolioName, r.GeneratedAt, r.TickerCount))
		}
		return textResult(sb.String()), nil
	}
}

func handleGetSummary(p *MCPProxy) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioViaAPI(p, request)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
		}

		body, err := p.get(fmt.Sprintf("/api/portfolios/%s/summary", url.PathEscape(portfolioName)))
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to get summary: %v", err)), nil
		}

		var resp struct {
			Summary string `json:"summary"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return errorResult(fmt.Sprintf("Error parsing response: %v", err)), nil
		}

		return textResult(resp.Summary), nil
	}
}

func handleGetPortfolioSnapshot(p *MCPProxy) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioViaAPI(p, request)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
		}

		dateStr, err := requireString(request, "date")
		if err != nil || dateStr == "" {
			return errorResult("Error: date parameter is required (format: YYYY-MM-DD)"), nil
		}

		body, err := p.get(fmt.Sprintf("/api/portfolios/%s/snapshot?date=%s",
			url.PathEscape(portfolioName), url.QueryEscape(dateStr)))
		if err != nil {
			return errorResult(fmt.Sprintf("Snapshot error: %v", err)), nil
		}

		var snapshot models.PortfolioSnapshot
		if err := json.Unmarshal(body, &snapshot); err != nil {
			return errorResult(fmt.Sprintf("Error parsing response: %v", err)), nil
		}

		return textResult(formatPortfolioSnapshot(&snapshot)), nil
	}
}

func handleGetPortfolioHistory(p *MCPProxy) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioViaAPI(p, request)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
		}

		params := url.Values{}
		if from := getString(request, "from", ""); from != "" {
			params.Set("from", from)
		}
		if to := getString(request, "to", ""); to != "" {
			params.Set("to", to)
		}
		format := getString(request, "format", "auto")
		params.Set("format", format)

		path := fmt.Sprintf("/api/portfolios/%s/history", url.PathEscape(portfolioName))
		if len(params) > 0 {
			path += "?" + params.Encode()
		}

		body, err := p.get(path)
		if err != nil {
			return errorResult(fmt.Sprintf("History error: %v", err)), nil
		}

		var resp struct {
			Portfolio  string                   `json:"portfolio"`
			Format     string                   `json:"format"`
			DataPoints []models.GrowthDataPoint `json:"data_points"`
			Count      int                      `json:"count"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return errorResult(fmt.Sprintf("Error parsing response: %v", err)), nil
		}

		if len(resp.DataPoints) == 0 {
			return textResult("No portfolio history data available for the specified date range."), nil
		}

		// Apply downsampling on the MCP side for display formatting
		points := resp.DataPoints
		granularity := format
		switch format {
		case "auto":
			if len(points) <= 90 {
				granularity = "daily"
			} else {
				granularity = "weekly"
				points = downsampleToWeekly(points)
			}
		case "weekly":
			points = downsampleToWeekly(points)
		case "monthly":
			points = downsampleToMonthly(points)
		default:
			granularity = "daily"
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

func handleSetDefaultPortfolio(p *MCPProxy) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := getString(request, "portfolio_name", "")

		if portfolioName == "" {
			// List available portfolios
			body, err := p.get("/api/portfolios/default")
			if err != nil {
				return errorResult("No portfolios found."), nil
			}

			var resp struct {
				Default    string   `json:"default"`
				Portfolios []string `json:"portfolios"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return errorResult(fmt.Sprintf("Error: %v", err)), nil
			}

			var sb strings.Builder
			sb.WriteString("# Available Portfolios\n\n")
			if resp.Default != "" {
				sb.WriteString(fmt.Sprintf("**Current default:** %s\n\n", resp.Default))
			} else {
				sb.WriteString("**Current default:** *(not set)*\n\n")
			}
			sb.WriteString("| # | Portfolio | Default |\n")
			sb.WriteString("|---|----------|---------|\n")
			for i, name := range resp.Portfolios {
				marker := ""
				if strings.EqualFold(name, resp.Default) {
					marker = "**current**"
				}
				sb.WriteString(fmt.Sprintf("| %d | %s | %s |\n", i+1, name, marker))
			}
			sb.WriteString("\nTo set the default, call `set_default_portfolio` with the portfolio_name parameter.")
			return textResult(sb.String()), nil
		}

		// Set the default
		body, err := p.put("/api/portfolios/default", map[string]string{"name": portfolioName})
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to set default portfolio: %v", err)), nil
		}

		var resp struct {
			Default string `json:"default"`
		}
		json.Unmarshal(body, &resp)

		return textResult(fmt.Sprintf("Default portfolio set to **%s**.\n\nTools that accept portfolio_name will now use '%s' when no portfolio is specified.",
			portfolioName, portfolioName)), nil
	}
}

func handleGetConfig(p *MCPProxy) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		body, err := p.get("/api/config")
		if err != nil {
			return errorResult(fmt.Sprintf("Error: %v", err)), nil
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(body, &resp); err != nil {
			return errorResult(fmt.Sprintf("Error parsing response: %v", err)), nil
		}

		var sb strings.Builder
		sb.WriteString("# Vire Configuration\n\n")

		// Runtime settings
		sb.WriteString("## Runtime Settings\n\n")
		sb.WriteString("| Key | Value |\n")
		sb.WriteString("|-----|-------|\n")
		if rs, ok := resp["runtime_settings"].(map[string]interface{}); ok {
			if len(rs) == 0 {
				sb.WriteString("| *(none set)* | |\n")
			}
			for k, v := range rs {
				sb.WriteString(fmt.Sprintf("| %s | %v |\n", k, v))
			}
		}
		sb.WriteString("\n")

		// Key fields
		if dp, ok := resp["default_portfolio"]; ok && dp != nil && dp != "" {
			sb.WriteString(fmt.Sprintf("**Default Portfolio:** %v\n", dp))
		}
		if sp, ok := resp["storage_path"]; ok {
			sb.WriteString(fmt.Sprintf("**Storage Path:** %v\n", sp))
		}
		if env, ok := resp["environment"]; ok {
			sb.WriteString(fmt.Sprintf("**Environment:** %v\n", env))
		}
		sb.WriteString(fmt.Sprintf("**EODHD Configured:** %v\n", resp["eodhd_configured"]))
		sb.WriteString(fmt.Sprintf("**Navexa Configured:** %v\n", resp["navexa_configured"]))
		sb.WriteString(fmt.Sprintf("**Gemini Configured:** %v\n", resp["gemini_configured"]))
		sb.WriteString("\n")

		return textResult(sb.String()), nil
	}
}

func handleGetStrategyTemplate(p *MCPProxy) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path := "/api/strategies/template"
		if at := getString(request, "account_type", ""); at != "" {
			path += "?account_type=" + url.QueryEscape(at)
		}

		body, err := p.get(path)
		if err != nil {
			return errorResult(fmt.Sprintf("Error: %v", err)), nil
		}

		// The REST API returns structured JSON template; format it for MCP display
		var template map[string]interface{}
		if err := json.Unmarshal(body, &template); err != nil {
			return errorResult(fmt.Sprintf("Error parsing response: %v", err)), nil
		}

		// Pretty-print as JSON for the user
		pretty, _ := json.MarshalIndent(template, "", "  ")
		return textResult(fmt.Sprintf("# Strategy Template\n\nUse `set_portfolio_strategy` with a JSON object containing these fields:\n\n```json\n%s\n```\n", string(pretty))), nil
	}
}

func handleSetPortfolioStrategy(p *MCPProxy) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioViaAPI(p, request)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
		}

		strategyJSON := getString(request, "strategy_json", "")
		if strategyJSON == "" {
			return errorResult("Error: strategy_json parameter is required. Use get_strategy_template to see available fields."), nil
		}

		// Parse the strategy JSON to ensure it's valid
		var strategyData json.RawMessage
		if err := json.Unmarshal([]byte(strategyJSON), &strategyData); err != nil {
			return errorResult(fmt.Sprintf("Error parsing strategy_json: %v", err)), nil
		}

		body, err := p.put(fmt.Sprintf("/api/portfolios/%s/strategy", url.PathEscape(portfolioName)),
			map[string]json.RawMessage{"strategy": strategyData})
		if err != nil {
			return errorResult(fmt.Sprintf("Error saving strategy: %v", err)), nil
		}

		var resp struct {
			Strategy *models.PortfolioStrategy `json:"strategy"`
			Warnings []struct {
				Severity string `json:"severity"`
				Message  string `json:"message"`
			} `json:"warnings"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return errorResult(fmt.Sprintf("Error parsing response: %v", err)), nil
		}

		var sb strings.Builder
		if resp.Strategy != nil {
			sb.WriteString(resp.Strategy.ToMarkdown())
		}

		if len(resp.Warnings) > 0 {
			sb.WriteString("\n---\n\n## Devil's Advocate Warnings\n\n")
			for _, w := range resp.Warnings {
				sb.WriteString(fmt.Sprintf("**[%s]** %s\n\n", strings.ToUpper(w.Severity), w.Message))
			}
			sb.WriteString("*These warnings highlight potential issues with your strategy.*\n")
		}

		return textResult(sb.String()), nil
	}
}

func handleGetPortfolioStrategy(p *MCPProxy) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioViaAPI(p, request)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
		}

		body, err := p.get(fmt.Sprintf("/api/portfolios/%s/strategy", url.PathEscape(portfolioName)))
		if err != nil {
			return errorResult(fmt.Sprintf("Error: %v", err)), nil
		}

		// Check if exists=false response
		var check struct {
			Exists  *bool  `json:"exists"`
			Message string `json:"message"`
		}
		if json.Unmarshal(body, &check) == nil && check.Exists != nil && !*check.Exists {
			return textResult(fmt.Sprintf("No strategy found for portfolio '%s'.\n\n"+
				"Use `set_portfolio_strategy` to create one, or `get_strategy_template` to see available options.",
				portfolioName)), nil
		}

		var strategy models.PortfolioStrategy
		if err := json.Unmarshal(body, &strategy); err != nil {
			return errorResult(fmt.Sprintf("Error parsing response: %v", err)), nil
		}

		return textResult(strategy.ToMarkdown()), nil
	}
}

func handleDeletePortfolioStrategy(p *MCPProxy) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioViaAPI(p, request)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
		}

		_, err := p.del(fmt.Sprintf("/api/portfolios/%s/strategy", url.PathEscape(portfolioName)))
		if err != nil {
			return errorResult(fmt.Sprintf("Error deleting strategy: %v", err)), nil
		}

		return textResult(fmt.Sprintf("Strategy for portfolio '%s' has been deleted.", portfolioName)), nil
	}
}

func handleGetPortfolioPlan(p *MCPProxy) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioViaAPI(p, request)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
		}

		body, err := p.get(fmt.Sprintf("/api/portfolios/%s/plan", url.PathEscape(portfolioName)))
		if err != nil {
			return errorResult(fmt.Sprintf("Error: %v", err)), nil
		}

		// Check if exists=false response
		var check struct {
			Exists  *bool  `json:"exists"`
			Message string `json:"message"`
		}
		if json.Unmarshal(body, &check) == nil && check.Exists != nil && !*check.Exists {
			return textResult(fmt.Sprintf("No plan found for portfolio '%s'.\n\n"+
				"Use `add_plan_item` to create action items, or `set_portfolio_plan` to set an entire plan.",
				portfolioName)), nil
		}

		var plan models.PortfolioPlan
		if err := json.Unmarshal(body, &plan); err != nil {
			return errorResult(fmt.Sprintf("Error parsing response: %v", err)), nil
		}

		return textResult(plan.ToMarkdown()), nil
	}
}

func handleSetPortfolioPlan(p *MCPProxy) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioViaAPI(p, request)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
		}

		planJSON := getString(request, "plan_json", "")
		if planJSON == "" {
			return errorResult("Error: plan_json parameter is required."), nil
		}

		var planData json.RawMessage
		if err := json.Unmarshal([]byte(planJSON), &planData); err != nil {
			return errorResult(fmt.Sprintf("Error parsing plan_json: %v", err)), nil
		}

		body, err := p.put(fmt.Sprintf("/api/portfolios/%s/plan", url.PathEscape(portfolioName)), planData)
		if err != nil {
			return errorResult(fmt.Sprintf("Error saving plan: %v", err)), nil
		}

		var plan models.PortfolioPlan
		if err := json.Unmarshal(body, &plan); err != nil {
			return errorResult(fmt.Sprintf("Error parsing response: %v", err)), nil
		}

		return textResult(plan.ToMarkdown()), nil
	}
}

func handleAddPlanItem(p *MCPProxy) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioViaAPI(p, request)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
		}

		itemJSON := getString(request, "item_json", "")
		if itemJSON == "" {
			return errorResult("Error: item_json parameter is required."), nil
		}

		var itemData json.RawMessage
		if err := json.Unmarshal([]byte(itemJSON), &itemData); err != nil {
			return errorResult(fmt.Sprintf("Error parsing item_json: %v", err)), nil
		}

		body, err := p.post(fmt.Sprintf("/api/portfolios/%s/plan/items", url.PathEscape(portfolioName)), itemData)
		if err != nil {
			return errorResult(fmt.Sprintf("Error adding plan item: %v", err)), nil
		}

		var plan models.PortfolioPlan
		if err := json.Unmarshal(body, &plan); err != nil {
			return errorResult(fmt.Sprintf("Error parsing response: %v", err)), nil
		}

		return textResult(plan.ToMarkdown()), nil
	}
}

func handleUpdatePlanItem(p *MCPProxy) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioViaAPI(p, request)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
		}

		itemID, err := requireString(request, "item_id")
		if err != nil || itemID == "" {
			return errorResult("Error: item_id parameter is required"), nil
		}

		itemJSON := getString(request, "item_json", "")
		if itemJSON == "" {
			return errorResult("Error: item_json parameter is required."), nil
		}

		var itemData json.RawMessage
		if err := json.Unmarshal([]byte(itemJSON), &itemData); err != nil {
			return errorResult(fmt.Sprintf("Error parsing item_json: %v", err)), nil
		}

		body, err := p.patch(fmt.Sprintf("/api/portfolios/%s/plan/items/%s",
			url.PathEscape(portfolioName), url.PathEscape(itemID)), itemData)
		if err != nil {
			return errorResult(fmt.Sprintf("Error updating plan item: %v", err)), nil
		}

		var plan models.PortfolioPlan
		if err := json.Unmarshal(body, &plan); err != nil {
			return errorResult(fmt.Sprintf("Error parsing response: %v", err)), nil
		}

		return textResult(plan.ToMarkdown()), nil
	}
}

func handleRemovePlanItem(p *MCPProxy) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioViaAPI(p, request)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
		}

		itemID, err := requireString(request, "item_id")
		if err != nil || itemID == "" {
			return errorResult("Error: item_id parameter is required"), nil
		}

		body, err := p.del(fmt.Sprintf("/api/portfolios/%s/plan/items/%s",
			url.PathEscape(portfolioName), url.PathEscape(itemID)))
		if err != nil {
			return errorResult(fmt.Sprintf("Error removing plan item: %v", err)), nil
		}

		var plan models.PortfolioPlan
		if err := json.Unmarshal(body, &plan); err != nil {
			return errorResult(fmt.Sprintf("Error parsing response: %v", err)), nil
		}

		return textResult(plan.ToMarkdown()), nil
	}
}

func handleCheckPlanStatus(p *MCPProxy) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioViaAPI(p, request)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
		}

		body, err := p.get(fmt.Sprintf("/api/portfolios/%s/plan/status", url.PathEscape(portfolioName)))
		if err != nil {
			return errorResult(fmt.Sprintf("Error checking plan status: %v", err)), nil
		}

		var resp struct {
			Triggered []models.PlanItem     `json:"triggered"`
			Expired   []models.PlanItem     `json:"expired"`
			Plan      *models.PortfolioPlan `json:"plan"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return errorResult(fmt.Sprintf("Error parsing response: %v", err)), nil
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("# Plan Status: %s\n\n", portfolioName))

		if len(resp.Triggered) > 0 {
			sb.WriteString("## Triggered Events\n\n")
			for _, item := range resp.Triggered {
				sb.WriteString(fmt.Sprintf("- **[%s]** %s", item.ID, item.Description))
				if item.Ticker != "" {
					sb.WriteString(fmt.Sprintf(" | Ticker: %s", item.Ticker))
				}
				if item.Action != "" {
					sb.WriteString(fmt.Sprintf(" | Action: %s", string(item.Action)))
				}
				sb.WriteString("\n")
			}
			sb.WriteString("\n")
		}

		if len(resp.Expired) > 0 {
			sb.WriteString("## Expired Deadlines\n\n")
			for _, item := range resp.Expired {
				sb.WriteString(fmt.Sprintf("- **[%s]** %s", item.ID, item.Description))
				if item.Deadline != nil {
					sb.WriteString(fmt.Sprintf(" (was due %s)", item.Deadline.Format("2006-01-02")))
				}
				sb.WriteString("\n")
			}
			sb.WriteString("\n")
		}

		if len(resp.Triggered) == 0 && len(resp.Expired) == 0 {
			sb.WriteString("No triggered events or expired deadlines.\n\n")
		}

		if resp.Plan != nil {
			sb.WriteString("---\n\n")
			sb.WriteString(resp.Plan.ToMarkdown())
		}

		return textResult(sb.String()), nil
	}
}

func handleFunnelScreen(p *MCPProxy) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		exchange, err := requireString(request, "exchange")
		if err != nil || exchange == "" {
			return errorResult("Error: exchange parameter is required"), nil
		}

		portfolioName := resolvePortfolioViaAPI(p, request)

		reqBody := map[string]interface{}{
			"exchange":       exchange,
			"limit":          getInt(request, "limit", 5),
			"include_news":   getBool(request, "include_news", false),
			"portfolio_name": portfolioName,
		}
		if sector := getString(request, "sector", ""); sector != "" {
			reqBody["sector"] = sector
		}

		body, err := p.post("/api/screen/funnel", reqBody)
		if err != nil {
			return errorResult(fmt.Sprintf("Funnel screen error: %v", err)), nil
		}

		var resp struct {
			Candidates []*models.ScreenCandidate `json:"candidates"`
			Stages     []models.FunnelStage      `json:"stages"`
			SearchID   string                    `json:"search_id"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return errorResult(fmt.Sprintf("Error parsing response: %v", err)), nil
		}

		// Build a FunnelResult for formatting
		funnelResult := &models.FunnelResult{
			Exchange:   exchange,
			Candidates: resp.Candidates,
			Stages:     resp.Stages,
		}

		markdown := formatFunnelResult(funnelResult)
		if resp.SearchID != "" {
			markdown += fmt.Sprintf("\n*Search saved: `%s` — use `get_search` to recall*\n", resp.SearchID)
		}
		return textResult(markdown), nil
	}
}

func handleListSearches(p *MCPProxy) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := url.Values{}
		if t := getString(request, "type", ""); t != "" {
			params.Set("type", t)
		}
		if e := getString(request, "exchange", ""); e != "" {
			params.Set("exchange", e)
		}
		if l := getInt(request, "limit", 10); l != 10 {
			params.Set("limit", fmt.Sprintf("%d", l))
		}

		path := "/api/searches"
		if len(params) > 0 {
			path += "?" + params.Encode()
		}

		body, err := p.get(path)
		if err != nil {
			return errorResult(fmt.Sprintf("Error listing searches: %v", err)), nil
		}

		var resp struct {
			Searches []*models.SearchRecord `json:"searches"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return errorResult(fmt.Sprintf("Error parsing response: %v", err)), nil
		}

		return textResult(formatSearchList(resp.Searches)), nil
	}
}

func handleGetSearch(p *MCPProxy) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		searchID, err := requireString(request, "search_id")
		if err != nil || searchID == "" {
			return errorResult("Error: search_id parameter is required"), nil
		}

		body, err := p.get(fmt.Sprintf("/api/searches/%s", url.PathEscape(searchID)))
		if err != nil {
			return errorResult(fmt.Sprintf("Search record not found: %v", err)), nil
		}

		var record models.SearchRecord
		if err := json.Unmarshal(body, &record); err != nil {
			return errorResult(fmt.Sprintf("Error parsing response: %v", err)), nil
		}

		return textResult(formatSearchDetail(&record)), nil
	}
}

func handleGetWatchlist(p *MCPProxy) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioViaAPI(p, request)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
		}

		body, err := p.get(fmt.Sprintf("/api/portfolios/%s/watchlist", url.PathEscape(portfolioName)))
		if err != nil {
			return errorResult(fmt.Sprintf("Error: %v", err)), nil
		}

		// Check if exists=false response
		var check struct {
			Exists  *bool  `json:"exists"`
			Message string `json:"message"`
		}
		if json.Unmarshal(body, &check) == nil && check.Exists != nil && !*check.Exists {
			return textResult(fmt.Sprintf("No watchlist found for portfolio '%s'.\n\n"+
				"Use `add_watchlist_item` to add stocks, or `set_watchlist` to set an entire watchlist.",
				portfolioName)), nil
		}

		var wl models.PortfolioWatchlist
		if err := json.Unmarshal(body, &wl); err != nil {
			return errorResult(fmt.Sprintf("Error parsing response: %v", err)), nil
		}

		return textResult(wl.ToMarkdown()), nil
	}
}

func handleAddWatchlistItem(p *MCPProxy) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioViaAPI(p, request)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
		}

		itemJSON := getString(request, "item_json", "")
		if itemJSON == "" {
			return errorResult("Error: item_json parameter is required."), nil
		}

		var itemData json.RawMessage
		if err := json.Unmarshal([]byte(itemJSON), &itemData); err != nil {
			return errorResult(fmt.Sprintf("Error parsing item_json: %v", err)), nil
		}

		body, err := p.post(fmt.Sprintf("/api/portfolios/%s/watchlist/items", url.PathEscape(portfolioName)), itemData)
		if err != nil {
			return errorResult(fmt.Sprintf("Error adding watchlist item: %v", err)), nil
		}

		var wl models.PortfolioWatchlist
		if err := json.Unmarshal(body, &wl); err != nil {
			return errorResult(fmt.Sprintf("Error parsing response: %v", err)), nil
		}

		return textResult(wl.ToMarkdown()), nil
	}
}

func handleUpdateWatchlistItem(p *MCPProxy) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioViaAPI(p, request)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
		}

		ticker, err := requireString(request, "ticker")
		if err != nil || ticker == "" {
			return errorResult("Error: ticker parameter is required"), nil
		}

		itemJSON := getString(request, "item_json", "")
		if itemJSON == "" {
			return errorResult("Error: item_json parameter is required."), nil
		}

		var itemData json.RawMessage
		if err := json.Unmarshal([]byte(itemJSON), &itemData); err != nil {
			return errorResult(fmt.Sprintf("Error parsing item_json: %v", err)), nil
		}

		body, err := p.patch(fmt.Sprintf("/api/portfolios/%s/watchlist/items/%s",
			url.PathEscape(portfolioName), url.PathEscape(ticker)), itemData)
		if err != nil {
			return errorResult(fmt.Sprintf("Error updating watchlist item: %v", err)), nil
		}

		var wl models.PortfolioWatchlist
		if err := json.Unmarshal(body, &wl); err != nil {
			return errorResult(fmt.Sprintf("Error parsing response: %v", err)), nil
		}

		return textResult(wl.ToMarkdown()), nil
	}
}

func handleRemoveWatchlistItem(p *MCPProxy) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioViaAPI(p, request)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
		}

		ticker, err := requireString(request, "ticker")
		if err != nil || ticker == "" {
			return errorResult("Error: ticker parameter is required"), nil
		}

		body, err := p.del(fmt.Sprintf("/api/portfolios/%s/watchlist/items/%s",
			url.PathEscape(portfolioName), url.PathEscape(ticker)))
		if err != nil {
			return errorResult(fmt.Sprintf("Error removing watchlist item: %v", err)), nil
		}

		var wl models.PortfolioWatchlist
		if err := json.Unmarshal(body, &wl); err != nil {
			return errorResult(fmt.Sprintf("Error parsing response: %v", err)), nil
		}

		return textResult(wl.ToMarkdown()), nil
	}
}

func handleSetWatchlist(p *MCPProxy) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		portfolioName := resolvePortfolioViaAPI(p, request)
		if portfolioName == "" {
			return errorResult("Error: portfolio_name parameter is required (no default portfolio configured — use set_default_portfolio to set one)"), nil
		}

		watchlistJSON := getString(request, "watchlist_json", "")
		if watchlistJSON == "" {
			return errorResult("Error: watchlist_json parameter is required."), nil
		}

		var wlData json.RawMessage
		if err := json.Unmarshal([]byte(watchlistJSON), &wlData); err != nil {
			return errorResult(fmt.Sprintf("Error parsing watchlist_json: %v", err)), nil
		}

		body, err := p.put(fmt.Sprintf("/api/portfolios/%s/watchlist", url.PathEscape(portfolioName)), wlData)
		if err != nil {
			return errorResult(fmt.Sprintf("Error saving watchlist: %v", err)), nil
		}

		var wl models.PortfolioWatchlist
		if err := json.Unmarshal(body, &wl); err != nil {
			return errorResult(fmt.Sprintf("Error parsing response: %v", err)), nil
		}

		return textResult(wl.ToMarkdown()), nil
	}
}

func handleGetQuote(p *MCPProxy) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ticker, err := requireString(request, "ticker")
		if err != nil || ticker == "" {
			return errorResult("Error: ticker parameter is required"), nil
		}

		body, err := p.get(fmt.Sprintf("/api/market/quote/%s", url.PathEscape(ticker)))
		if err != nil {
			return errorResult(fmt.Sprintf("Quote error: %v", err)), nil
		}

		var envelope struct {
			Quote models.RealTimeQuote `json:"quote"`
		}
		if err := json.Unmarshal(body, &envelope); err != nil {
			return errorResult(fmt.Sprintf("Error parsing response: %v", err)), nil
		}

		return textResult(formatQuote(&envelope.Quote)), nil
	}
}

func handleGetDiagnostics(p *MCPProxy) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := url.Values{}
		if cid := getString(request, "correlation_id", ""); cid != "" {
			params.Set("correlation_id", cid)
		}
		if l := getInt(request, "limit", 50); l != 50 {
			params.Set("limit", fmt.Sprintf("%d", l))
		}

		path := "/api/diagnostics"
		if len(params) > 0 {
			path += "?" + params.Encode()
		}

		body, err := p.get(path)
		if err != nil {
			return errorResult(fmt.Sprintf("Error: %v", err)), nil
		}

		var resp struct {
			Version   string `json:"version"`
			Build     string `json:"build"`
			Commit    string `json:"commit"`
			Uptime    string `json:"uptime"`
			StartedAt string `json:"started_at"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return errorResult(fmt.Sprintf("Error parsing response: %v", err)), nil
		}

		var sb strings.Builder
		sb.WriteString("# Server Diagnostics\n\n")
		sb.WriteString("## Server Info\n\n")
		sb.WriteString("| Field | Value |\n")
		sb.WriteString("|-------|-------|\n")
		sb.WriteString(fmt.Sprintf("| Version | %s |\n", resp.Version))
		sb.WriteString(fmt.Sprintf("| Build | %s |\n", resp.Build))
		sb.WriteString(fmt.Sprintf("| Commit | %s |\n", resp.Commit))
		sb.WriteString(fmt.Sprintf("| Uptime | %s |\n", resp.Uptime))
		sb.WriteString(fmt.Sprintf("| Started | %s |\n", resp.StartedAt))
		sb.WriteString("\n")

		return textResult(sb.String()), nil
	}
}
