package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mark3labs/mcp-go/server"

	"github.com/bobmccarthy/vire/internal/clients/eodhd"
	"github.com/bobmccarthy/vire/internal/clients/gemini"
	"github.com/bobmccarthy/vire/internal/clients/navexa"
	"github.com/bobmccarthy/vire/internal/common"
	"github.com/bobmccarthy/vire/internal/interfaces"
	"github.com/bobmccarthy/vire/internal/services/market"
	"github.com/bobmccarthy/vire/internal/services/plan"
	"github.com/bobmccarthy/vire/internal/services/portfolio"
	"github.com/bobmccarthy/vire/internal/services/report"
	"github.com/bobmccarthy/vire/internal/services/signal"
	"github.com/bobmccarthy/vire/internal/services/strategy"
	"github.com/bobmccarthy/vire/internal/services/watchlist"
	"github.com/bobmccarthy/vire/internal/storage"
)

// App holds all initialized services, clients, and the MCP server.
// It is the shared core used by both cmd/vire-server and cmd/vire-mcp.
type App struct {
	Config           *common.Config
	Logger           *common.Logger
	Storage          interfaces.StorageManager
	EODHDClient      interfaces.EODHDClient
	NavexaClient     interfaces.NavexaClient
	GeminiClient     interfaces.GeminiClient
	MarketService    interfaces.MarketService
	PortfolioService interfaces.PortfolioService
	ReportService    interfaces.ReportService
	SignalService    interfaces.SignalService
	StrategyService  interfaces.StrategyService
	PlanService      interfaces.PlanService
	WatchlistService interfaces.WatchlistService
	MCPServer        *server.MCPServer
	DefaultPortfolio string
	StartupTime      time.Time

	schedulerCancel  context.CancelFunc
	warmCacheCancel  context.CancelFunc
}

// getBinaryDir returns the directory containing the executable.
func getBinaryDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

// NewApp initializes all services, clients, storage, and the MCP server.
// configPath may be empty, in which case the default resolution logic is used.
func NewApp(configPath string) (*App, error) {
	startupStart := time.Now()

	// Load version from .version file (fallback if ldflags not set)
	common.LoadVersionFromFile()

	// Get binary directory for self-contained operation
	binDir := getBinaryDir()

	// Load configuration - check provided path, VIRE_CONFIG, then binary dir, then fallback
	if configPath == "" {
		configPath = os.Getenv("VIRE_CONFIG")
	}
	if configPath == "" {
		configPath = filepath.Join(binDir, "vire.toml")
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			configPath = "config/vire.toml" // fallback for development
		}
	}

	config, err := common.LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Resolve relative storage path to binary directory
	if config.Storage.File.Path != "" && !filepath.IsAbs(config.Storage.File.Path) {
		config.Storage.File.Path = filepath.Join(binDir, config.Storage.File.Path)
	}

	// Resolve relative log file path to binary directory
	if config.Logging.FilePath != "" && !filepath.IsAbs(config.Logging.FilePath) {
		config.Logging.FilePath = filepath.Join(binDir, config.Logging.FilePath)
	}

	// Initialize logger
	logger := common.NewLoggerFromConfig(config.Logging)

	// Initialize storage
	storageManager, err := storage.NewStorageManager(logger, config)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}

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
	watchlistService := watchlist.NewService(storageManager, logger)

	// Create MCP server
	mcpServer := server.NewMCPServer(
		"vire",
		common.GetVersion(),
		server.WithToolCapabilities(true),
	)

	defaultPortfolio := config.DefaultPortfolio()

	a := &App{
		Config:           config,
		Logger:           logger,
		Storage:          storageManager,
		EODHDClient:      eodhdClient,
		NavexaClient:     navexaClient,
		GeminiClient:     geminiClient,
		MarketService:    marketService,
		PortfolioService: portfolioService,
		ReportService:    reportService,
		SignalService:    signalService,
		StrategyService:  strategyService,
		PlanService:      planService,
		WatchlistService: watchlistService,
		MCPServer:        mcpServer,
		DefaultPortfolio: defaultPortfolio,
		StartupTime:      startupStart,
	}

	// Register all MCP tools
	a.registerTools()

	logger.Info().Dur("startup", time.Since(startupStart)).Msg("App initialized")

	return a, nil
}

// Close releases all resources held by the App.
// Shutdown order: cancel scheduler, cancel warm cache, close storage.
func (a *App) Close() {
	if a.schedulerCancel != nil {
		a.schedulerCancel()
		a.schedulerCancel = nil
	}
	if a.warmCacheCancel != nil {
		a.warmCacheCancel()
		a.warmCacheCancel = nil
	}
	if a.Storage != nil {
		a.Storage.Close()
		a.Storage = nil
	}
}

// StartWarmCache launches the background cache warming goroutine.
func (a *App) StartWarmCache() {
	warmCtx, warmCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	a.warmCacheCancel = warmCancel
	go func() {
		defer warmCancel()
		warmCache(warmCtx, a.PortfolioService, a.MarketService, a.Storage, a.DefaultPortfolio, a.Logger)
	}()
}

// StartPriceScheduler launches the background price refresh goroutine.
func (a *App) StartPriceScheduler() {
	schedulerCtx, schedulerCancel := context.WithCancel(context.Background())
	a.schedulerCancel = schedulerCancel
	go startPriceScheduler(schedulerCtx, a.PortfolioService, a.MarketService, a.Storage, a.DefaultPortfolio, a.Logger, common.FreshnessTodayBar)
}

// registerTools registers all MCP tools on the App's MCPServer.
func (a *App) registerTools() {
	s := a.MCPServer
	dp := a.DefaultPortfolio
	sm := a.Storage
	logger := a.Logger

	s.AddTool(createGetVersionTool(), handleGetVersion())
	s.AddTool(createPortfolioReviewTool(), handlePortfolioReview(a.PortfolioService, sm, dp, logger))
	s.AddTool(createGetPortfolioTool(), handleGetPortfolio(a.PortfolioService, sm, dp, logger))
	s.AddTool(createMarketSnipeTool(), handleMarketSnipe(a.MarketService, sm, dp, logger))
	s.AddTool(createStockScreenTool(), handleStockScreen(a.MarketService, sm, dp, logger))
	s.AddTool(createGetStockDataTool(), handleGetStockData(a.MarketService, logger))
	s.AddTool(createDetectSignalsTool(), handleDetectSignals(a.SignalService, logger))
	s.AddTool(createListPortfoliosTool(), handleListPortfolios(a.PortfolioService, logger))
	s.AddTool(createSyncPortfolioTool(), handleSyncPortfolio(a.PortfolioService, sm, dp, logger))
	s.AddTool(createRebuildDataTool(), handleRebuildData(a.PortfolioService, a.MarketService, sm, dp, logger))
	s.AddTool(createCollectMarketDataTool(), handleCollectMarketData(a.MarketService, logger))
	s.AddTool(createGenerateReportTool(), handleGenerateReport(a.ReportService, sm, dp, logger))
	s.AddTool(createGenerateTickerReportTool(), handleGenerateTickerReport(a.ReportService, sm, dp, logger))
	s.AddTool(createListReportsTool(), handleListReports(sm, logger))
	s.AddTool(createGetSummaryTool(), handleGetSummary(sm, a.ReportService, dp, logger))
	s.AddTool(createGetTickerReportTool(), handleGetTickerReport(sm, a.ReportService, dp, logger))
	s.AddTool(createListTickersTool(), handleListTickers(sm, dp, logger))
	s.AddTool(createGetPortfolioSnapshotTool(), handleGetPortfolioSnapshot(a.PortfolioService, sm, dp, logger))
	s.AddTool(createGetPortfolioHistoryTool(), handleGetPortfolioHistory(a.PortfolioService, sm, dp, logger))
	s.AddTool(createSetDefaultPortfolioTool(), handleSetDefaultPortfolio(sm, a.PortfolioService, dp, logger))
	s.AddTool(createGetConfigTool(), handleGetConfig(sm, a.Config, logger))
	s.AddTool(createGetStrategyTemplateTool(), handleGetStrategyTemplate())
	s.AddTool(createSetPortfolioStrategyTool(), handleSetPortfolioStrategy(a.StrategyService, sm, dp, logger))
	s.AddTool(createGetPortfolioStrategyTool(), handleGetPortfolioStrategy(a.StrategyService, sm, dp, logger))
	s.AddTool(createDeletePortfolioStrategyTool(), handleDeletePortfolioStrategy(a.StrategyService, sm, dp, logger))
	s.AddTool(createGetPortfolioPlanTool(), handleGetPortfolioPlan(a.PlanService, sm, dp, logger))
	s.AddTool(createSetPortfolioPlanTool(), handleSetPortfolioPlan(a.PlanService, sm, dp, logger))
	s.AddTool(createAddPlanItemTool(), handleAddPlanItem(a.PlanService, sm, dp, logger))
	s.AddTool(createUpdatePlanItemTool(), handleUpdatePlanItem(a.PlanService, sm, dp, logger))
	s.AddTool(createRemovePlanItemTool(), handleRemovePlanItem(a.PlanService, sm, dp, logger))
	s.AddTool(createCheckPlanStatusTool(), handleCheckPlanStatus(a.PlanService, sm, dp, logger))
	s.AddTool(createFunnelScreenTool(), handleFunnelScreen(a.MarketService, sm, dp, logger))
	s.AddTool(createListSearchesTool(), handleListSearches(sm, logger))
	s.AddTool(createGetSearchTool(), handleGetSearch(sm, logger))
	s.AddTool(createGetWatchlistTool(), handleGetWatchlist(a.WatchlistService, sm, dp, logger))
	s.AddTool(createAddWatchlistItemTool(), handleAddWatchlistItem(a.WatchlistService, sm, dp, logger))
	s.AddTool(createUpdateWatchlistItemTool(), handleUpdateWatchlistItem(a.WatchlistService, sm, dp, logger))
	s.AddTool(createRemoveWatchlistItemTool(), handleRemoveWatchlistItem(a.WatchlistService, sm, dp, logger))
	s.AddTool(createSetWatchlistTool(), handleSetWatchlist(a.WatchlistService, sm, dp, logger))
	s.AddTool(createGetDiagnosticsTool(), handleGetDiagnostics(logger, a.StartupTime))
}
