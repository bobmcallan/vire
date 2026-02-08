package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/mark3labs/mcp-go/server"

	"github.com/bobmccarthy/vire/internal/clients/eodhd"
	"github.com/bobmccarthy/vire/internal/clients/gemini"
	"github.com/bobmccarthy/vire/internal/clients/navexa"
	"github.com/bobmccarthy/vire/internal/common"
	"github.com/bobmccarthy/vire/internal/services/market"
	"github.com/bobmccarthy/vire/internal/services/plan"
	"github.com/bobmccarthy/vire/internal/services/portfolio"
	"github.com/bobmccarthy/vire/internal/services/report"
	"github.com/bobmccarthy/vire/internal/services/signal"
	"github.com/bobmccarthy/vire/internal/services/strategy"
	"github.com/bobmccarthy/vire/internal/storage"
)

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

	// Initialize logger
	logger := common.NewLogger("warn")

	// Initialize storage
	storageManager, err := storage.NewStorageManager(logger, config)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to initialize storage")
	}
	defer storageManager.Close()

	// Check schema version — purge derived data on mismatch
	ctx := context.Background()
	checkSchemaVersion(ctx, storageManager, logger)

	// Resolve API keys
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
	strategyService := strategy.NewService(storageManager, logger)
	planService := plan.NewService(storageManager, strategyService, logger)

	// Create MCP server
	mcpServer := server.NewMCPServer(
		"vire",
		common.GetVersion(),
		server.WithToolCapabilities(true),
	)

	// Register tools
	defaultPortfolio := config.DefaultPortfolio()
	mcpServer.AddTool(createGetVersionTool(), handleGetVersion())
	// Image cache for chart PNGs
	imageCache := NewImageCache(filepath.Join(config.Storage.Badger.Path, "..", "images"), config.Server.Port, logger)

	mcpServer.AddTool(createPortfolioReviewTool(), handlePortfolioReview(portfolioService, storageManager, defaultPortfolio, imageCache, logger))
	mcpServer.AddTool(createMarketSnipeTool(), handleMarketSnipe(marketService, storageManager, defaultPortfolio, logger))
	mcpServer.AddTool(createStockScreenTool(), handleStockScreen(marketService, storageManager, defaultPortfolio, logger))
	mcpServer.AddTool(createGetStockDataTool(), handleGetStockData(marketService, logger))
	mcpServer.AddTool(createDetectSignalsTool(), handleDetectSignals(signalService, logger))
	mcpServer.AddTool(createListPortfoliosTool(), handleListPortfolios(portfolioService, logger))
	mcpServer.AddTool(createSyncPortfolioTool(), handleSyncPortfolio(portfolioService, storageManager, defaultPortfolio, logger))
	mcpServer.AddTool(createRebuildDataTool(), handleRebuildData(portfolioService, marketService, storageManager, defaultPortfolio, logger))
	mcpServer.AddTool(createCollectMarketDataTool(), handleCollectMarketData(marketService, logger))
	mcpServer.AddTool(createGenerateReportTool(), handleGenerateReport(reportService, storageManager, defaultPortfolio, logger))
	mcpServer.AddTool(createGenerateTickerReportTool(), handleGenerateTickerReport(reportService, storageManager, defaultPortfolio, logger))
	mcpServer.AddTool(createListReportsTool(), handleListReports(storageManager, logger))
	mcpServer.AddTool(createGetSummaryTool(), handleGetSummary(storageManager, reportService, defaultPortfolio, logger))
	mcpServer.AddTool(createGetTickerReportTool(), handleGetTickerReport(storageManager, reportService, defaultPortfolio, logger))
	mcpServer.AddTool(createListTickersTool(), handleListTickers(storageManager, defaultPortfolio, logger))
	mcpServer.AddTool(createGetPortfolioSnapshotTool(), handleGetPortfolioSnapshot(portfolioService, storageManager, defaultPortfolio, logger))
	mcpServer.AddTool(createGetPortfolioHistoryTool(), handleGetPortfolioHistory(portfolioService, storageManager, defaultPortfolio, logger))
	mcpServer.AddTool(createSetDefaultPortfolioTool(), handleSetDefaultPortfolio(storageManager, portfolioService, defaultPortfolio, logger))
	mcpServer.AddTool(createGetConfigTool(), handleGetConfig(storageManager, config, logger))
	mcpServer.AddTool(createGetStrategyTemplateTool(), handleGetStrategyTemplate())
	mcpServer.AddTool(createSetPortfolioStrategyTool(), handleSetPortfolioStrategy(strategyService, storageManager, defaultPortfolio, logger))
	mcpServer.AddTool(createGetPortfolioStrategyTool(), handleGetPortfolioStrategy(strategyService, storageManager, defaultPortfolio, logger))
	mcpServer.AddTool(createDeletePortfolioStrategyTool(), handleDeletePortfolioStrategy(strategyService, storageManager, defaultPortfolio, logger))
	mcpServer.AddTool(createGetPortfolioPlanTool(), handleGetPortfolioPlan(planService, storageManager, defaultPortfolio, logger))
	mcpServer.AddTool(createSetPortfolioPlanTool(), handleSetPortfolioPlan(planService, storageManager, defaultPortfolio, logger))
	mcpServer.AddTool(createAddPlanItemTool(), handleAddPlanItem(planService, storageManager, defaultPortfolio, logger))
	mcpServer.AddTool(createUpdatePlanItemTool(), handleUpdatePlanItem(planService, storageManager, defaultPortfolio, logger))
	mcpServer.AddTool(createRemovePlanItemTool(), handleRemovePlanItem(planService, storageManager, defaultPortfolio, logger))
	mcpServer.AddTool(createCheckPlanStatusTool(), handleCheckPlanStatus(planService, storageManager, defaultPortfolio, logger))
	mcpServer.AddTool(createFunnelScreenTool(), handleFunnelScreen(marketService, storageManager, defaultPortfolio, logger))
	mcpServer.AddTool(createListSearchesTool(), handleListSearches(storageManager, logger))
	mcpServer.AddTool(createGetSearchTool(), handleGetSearch(storageManager, logger))

	// Warm cache: pre-fetch portfolio and market data in the background
	warmCtx, warmCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	go func() {
		defer warmCancel()
		warmCache(warmCtx, portfolioService, marketService, storageManager, defaultPortfolio, logger)
	}()

	// Scheduled price refresh: update EOD prices hourly
	schedulerCtx, schedulerCancel := context.WithCancel(context.Background())
	defer schedulerCancel()
	go startPriceScheduler(schedulerCtx, portfolioService, marketService, storageManager, defaultPortfolio, logger, common.FreshnessTodayBar)

	// Start streamable HTTP server
	addr := fmt.Sprintf("%s:%d", config.Server.Host, config.Server.Port)
	httpServer := server.NewStreamableHTTPServer(mcpServer,
		server.WithEndpointPath("/mcp"),
		server.WithStateLess(true),
	)

	mux := http.NewServeMux()
	mux.Handle("/mcp", httpServer)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	mux.Handle("/images/", imageCache.Handler())

	srv := &http.Server{Addr: addr, Handler: mux}
	logger.Info().Str("addr", addr).Msg("Starting MCP HTTP server")
	if err := srv.ListenAndServe(); err != nil {
		logger.Fatal().Err(err).Msg("MCP server failed")
	}
}
