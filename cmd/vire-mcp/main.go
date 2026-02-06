package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mark3labs/mcp-go/server"

	"github.com/bobmccarthy/vire/internal/clients/eodhd"
	"github.com/bobmccarthy/vire/internal/clients/gemini"
	"github.com/bobmccarthy/vire/internal/clients/navexa"
	"github.com/bobmccarthy/vire/internal/common"
	"github.com/bobmccarthy/vire/internal/services/market"
	"github.com/bobmccarthy/vire/internal/services/portfolio"
	"github.com/bobmccarthy/vire/internal/services/report"
	"github.com/bobmccarthy/vire/internal/services/signal"
	"github.com/bobmccarthy/vire/internal/storage"
)

// isStdioMode checks if --stdio flag is present in command-line arguments
func isStdioMode() bool {
	for _, arg := range os.Args[1:] {
		if arg == "--stdio" {
			return true
		}
	}
	return false
}

// getBinaryDir returns the directory containing the executable
func getBinaryDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

func main() {
	// Load version from .version file (fallback if ldflags not set)
	common.LoadVersionFromFile()

	// Get binary directory for self-contained operation
	binDir := getBinaryDir()

	// Load configuration - check VIRE_CONFIG, then binary dir, then fallback
	configPath := os.Getenv("VIRE_CONFIG")
	if configPath == "" {
		configPath = filepath.Join(binDir, "vire.toml")
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			configPath = "config/vire.toml" // fallback for development
		}
	}

	config, err := common.LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Resolve relative storage path to binary directory
	if config.Storage.Badger.Path != "" && !filepath.IsAbs(config.Storage.Badger.Path) {
		config.Storage.Badger.Path = filepath.Join(binDir, config.Storage.Badger.Path)
	}

	// Initialize minimal logger for MCP server (warn level to avoid cluttering stdio)
	logger := common.NewLogger("warn")

	// Initialize storage
	storageManager, err := storage.NewStorageManager(logger, config)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to initialize storage")
	}
	defer storageManager.Close()

	// Resolve API keys
	ctx := context.Background()
	kvStorage := storageManager.KeyValueStorage()

	eodhdKey, err := common.ResolveAPIKey(ctx, kvStorage, "eodhd_api_key", config.Clients.EODHD.APIKey)
	if err != nil {
		logger.Warn().Msg("EODHD API key not configured - some features may be limited")
	}

	navexaKey, err := common.ResolveAPIKey(ctx, kvStorage, "navexa_api_key", config.Clients.Navexa.APIKey)
	if err != nil {
		logger.Warn().Msg("Navexa API key not configured - portfolio sync will be unavailable")
	}

	geminiKey, err := common.ResolveAPIKey(ctx, kvStorage, "gemini_api_key", config.Clients.Gemini.APIKey)
	if err != nil {
		logger.Warn().Msg("Gemini API key not configured - AI analysis will be unavailable")
	}

	// Initialize API clients
	var eodhdClient *eodhd.Client
	if eodhdKey != "" {
		eodhdClient = eodhd.NewClient(eodhdKey,
			eodhd.WithLogger(logger),
			eodhd.WithRateLimit(config.Clients.EODHD.RateLimit),
		)
	}

	var navexaClient *navexa.Client
	if navexaKey != "" {
		navexaClient = navexa.NewClient(navexaKey,
			navexa.WithLogger(logger),
			navexa.WithRateLimit(config.Clients.Navexa.RateLimit),
		)
	}

	var geminiClient *gemini.Client
	if geminiKey != "" {
		geminiClient, err = gemini.NewClient(ctx, geminiKey,
			gemini.WithLogger(logger),
			gemini.WithModel(config.Clients.Gemini.Model),
		)
		if err != nil {
			logger.Warn().Err(err).Msg("Failed to initialize Gemini client")
		}
	}

	// Initialize services
	signalService := signal.NewService(storageManager, logger)
	marketService := market.NewService(storageManager, eodhdClient, geminiClient, logger)
	portfolioService := portfolio.NewService(storageManager, navexaClient, eodhdClient, geminiClient, logger)
	reportService := report.NewService(portfolioService, marketService, signalService, storageManager, logger)

	// Create MCP server
	mcpServer := server.NewMCPServer(
		"vire",
		common.GetVersion(),
		server.WithToolCapabilities(true),
	)

	// Register tools
	mcpServer.AddTool(createGetVersionTool(), handleGetVersion())
	mcpServer.AddTool(createPortfolioReviewTool(), handlePortfolioReview(portfolioService, logger))
	mcpServer.AddTool(createMarketSnipeTool(), handleMarketSnipe(marketService, logger))
	mcpServer.AddTool(createStockScreenTool(), handleStockScreen(marketService, logger))
	mcpServer.AddTool(createGetStockDataTool(), handleGetStockData(marketService, logger))
	mcpServer.AddTool(createDetectSignalsTool(), handleDetectSignals(signalService, logger))
	mcpServer.AddTool(createListPortfoliosTool(), handleListPortfolios(portfolioService, logger))
	mcpServer.AddTool(createSyncPortfolioTool(), handleSyncPortfolio(portfolioService, logger))
	mcpServer.AddTool(createCollectMarketDataTool(), handleCollectMarketData(marketService, logger))
	mcpServer.AddTool(createGenerateReportTool(), handleGenerateReport(reportService, storageManager, logger))
	mcpServer.AddTool(createGenerateTickerReportTool(), handleGenerateTickerReport(reportService, logger))
	mcpServer.AddTool(createListReportsTool(), handleListReports(storageManager, logger))
	mcpServer.AddTool(createGetSummaryTool(), handleGetSummary(storageManager, reportService, logger))
	mcpServer.AddTool(createGetTickerReportTool(), handleGetTickerReport(storageManager, reportService, logger))
	mcpServer.AddTool(createListTickersTool(), handleListTickers(storageManager, logger))

	// Start server in the appropriate transport mode
	if isStdioMode() {
		// stdio transport — for Claude Desktop via "docker run --rm -i"
		logger.Info().Msg("Starting MCP stdio server")
		if err := server.ServeStdio(mcpServer); err != nil {
			logger.Fatal().Err(err).Msg("MCP stdio server failed")
		}
	} else {
		// SSE transport — for Claude Code and HTTP clients
		addr := fmt.Sprintf("%s:%d", config.Server.Host, config.Server.Port)
		baseHost := config.Server.Host
		if baseHost == "0.0.0.0" {
			baseHost = "localhost"
		}
		sseServer := server.NewSSEServer(mcpServer,
			server.WithBaseURL(fmt.Sprintf("http://%s:%d", baseHost, config.Server.Port)),
		)

		logger.Info().Str("addr", addr).Msg("Starting MCP SSE server")
		if err := sseServer.Start(addr); err != nil {
			logger.Fatal().Err(err).Msg("MCP server failed")
		}
	}
}
