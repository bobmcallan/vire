package main

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/bobmccarthy/vire/internal/common"
	"github.com/bobmccarthy/vire/internal/interfaces"
)

// handleGetVersion implements the get_version tool
func handleGetVersion() server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		version := common.GetVersion()
		result := fmt.Sprintf("Vire MCP Server\nVersion: %s\nStatus: OK", version)
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

		signals, err := signalService.DetectSignals(ctx, tickers, signalTypes)
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

		err := marketService.CollectMarketData(ctx, tickers, includeNews)
		if err != nil {
			logger.Error().Err(err).Msg("Collect market data failed")
			return errorResult(fmt.Sprintf("Collection error: %v", err)), nil
		}

		markdown := formatCollectResult(tickers)
		return textResult(markdown), nil
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
