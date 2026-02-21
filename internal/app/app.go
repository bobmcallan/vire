package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/bobmcallan/vire/internal/clients/asx"
	"github.com/bobmcallan/vire/internal/clients/eodhd"
	"github.com/bobmcallan/vire/internal/clients/gemini"
	"github.com/bobmcallan/vire/internal/clients/navexa"
	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/services/jobmanager"
	"github.com/bobmcallan/vire/internal/services/market"
	"github.com/bobmcallan/vire/internal/services/plan"
	"github.com/bobmcallan/vire/internal/services/portfolio"
	"github.com/bobmcallan/vire/internal/services/quote"
	"github.com/bobmcallan/vire/internal/services/report"
	"github.com/bobmcallan/vire/internal/services/signal"
	"github.com/bobmcallan/vire/internal/services/strategy"
	"github.com/bobmcallan/vire/internal/services/watchlist"
	"github.com/bobmcallan/vire/internal/storage"
)

// App holds all initialized services, clients, and configuration.
// It is the shared core used by cmd/vire-server.
type App struct {
	Config           *common.Config
	Logger           *common.Logger
	Storage          interfaces.StorageManager
	EODHDClient      interfaces.EODHDClient
	ASXClient        interfaces.ASXClient
	GeminiClient     interfaces.GeminiClient
	QuoteService     interfaces.QuoteService
	MarketService    interfaces.MarketService
	PortfolioService interfaces.PortfolioService
	ReportService    interfaces.ReportService
	SignalService    interfaces.SignalService
	StrategyService  interfaces.StrategyService
	PlanService      interfaces.PlanService
	WatchlistService interfaces.WatchlistService
	JobManager       *jobmanager.JobManager
	StartupTime      time.Time

	schedulerCancel context.CancelFunc
	warmCacheCancel context.CancelFunc
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
		configPath = filepath.Join(binDir, "vire-service.toml")
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			configPath = "config/vire-service.toml" // fallback for development
		}
	}

	config, err := common.LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Resolve relative storage paths to binary directory
	if config.Storage.DataPath != "" && !filepath.IsAbs(config.Storage.DataPath) {
		config.Storage.DataPath = filepath.Join(binDir, config.Storage.DataPath)
	}

	// Resolve relative log file path to binary directory
	if config.Logging.FilePath != "" && !filepath.IsAbs(config.Logging.FilePath) {
		config.Logging.FilePath = filepath.Join(binDir, config.Logging.FilePath)
	}

	// Initialize logger
	logger := common.NewLoggerFromConfig(config.Logging)

	// Initialize storage
	storageManager, err := storage.NewManager(logger, config)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}

	// Check schema version — purge derived data on mismatch
	ctx := context.Background()
	checkSchemaVersion(ctx, storageManager, logger)

	// Dev mode: purge reports on build change (so code changes are immediately visible)
	checkDevBuildChange(ctx, storageManager, config, logger)

	// Resolve API keys
	internalStore := storageManager.InternalStore()

	eodhdKey, err := common.ResolveAPIKey(ctx, internalStore, "eodhd_api_key", config.Clients.EODHD.APIKey)
	if err != nil {
		logger.Warn().Msg("EODHD API key not configured - some features may be limited")
	}

	geminiKey, err := common.ResolveAPIKey(ctx, internalStore, "gemini_api_key", config.Clients.Gemini.APIKey)
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

	// Initialize ASX Markit Digital client (public API, no key required)
	asxClient := asx.NewClient(asx.WithLogger(logger))

	// Initialize quote service with EODHD primary and ASX fallback
	var quoteService *quote.Service
	if eodhdClient != nil {
		quoteService = quote.NewService(eodhdClient, asxClient, logger)
	}

	// Initialize services
	signalService := signal.NewService(storageManager, logger)
	marketService := market.NewService(storageManager, eodhdClient, geminiClient, logger)
	portfolioService := portfolio.NewService(storageManager, nil, eodhdClient, geminiClient, logger)
	reportService := report.NewService(portfolioService, marketService, signalService, storageManager, logger)
	strategyService := strategy.NewService(storageManager, logger)
	planService := plan.NewService(storageManager, strategyService, logger)
	watchlistService := watchlist.NewService(storageManager, logger)

	// Initialize job manager
	var jobMgr *jobmanager.JobManager
	if config.JobManager.Enabled {
		jobMgr = jobmanager.NewJobManager(
			marketService,
			signalService,
			storageManager,
			logger,
			config.JobManager,
		)
	}

	a := &App{
		Config:           config,
		Logger:           logger,
		Storage:          storageManager,
		EODHDClient:      eodhdClient,
		ASXClient:        asxClient,
		GeminiClient:     geminiClient,
		QuoteService:     quoteService,
		MarketService:    marketService,
		PortfolioService: portfolioService,
		ReportService:    reportService,
		SignalService:    signalService,
		StrategyService:  strategyService,
		PlanService:      planService,
		WatchlistService: watchlistService,
		JobManager:       jobMgr,
		StartupTime:      startupStart,
	}

	logger.Info().Dur("startup", time.Since(startupStart)).Msg("App initialized")

	return a, nil
}

// InjectNavexaClient creates a per-request Navexa client from the user context
// API key and stores it in context for downstream services.
// The caller must validate that the user context has a NavexaAPIKey before calling.
func (a *App) InjectNavexaClient(ctx context.Context) context.Context {
	if uc := common.UserContextFromContext(ctx); uc != nil && uc.NavexaAPIKey != "" {
		client := navexa.NewClient(uc.NavexaAPIKey,
			navexa.WithLogger(a.Logger),
			navexa.WithRateLimit(a.Config.Clients.Navexa.RateLimit),
		)
		return common.WithNavexaClient(ctx, client)
	}
	return ctx
}

// Close releases all resources held by the App.
// Shutdown order: stop job manager, cancel scheduler, cancel warm cache, close storage.
func (a *App) Close() {
	if a.JobManager != nil {
		a.JobManager.Stop()
		a.JobManager = nil
	}
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

// StartJobManager launches the background job manager.
func (a *App) StartJobManager() {
	if a.JobManager != nil {
		a.JobManager.Start()
	}
}

// StartWarmCache launches the background cache warming goroutine.
func (a *App) StartWarmCache() {
	warmCtx, warmCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	a.warmCacheCancel = warmCancel
	go func() {
		defer warmCancel()
		warmCache(warmCtx, a.PortfolioService, a.MarketService, a.Storage, a.Logger)
	}()
}

// StartPriceScheduler launches the background price refresh goroutine.
func (a *App) StartPriceScheduler() {
	schedulerCtx, schedulerCancel := context.WithCancel(context.Background())
	a.schedulerCancel = schedulerCancel
	go startPriceScheduler(schedulerCtx, a.PortfolioService, a.MarketService, a.Storage, a.Logger, common.FreshnessTodayBar)
}
