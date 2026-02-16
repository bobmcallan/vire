package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/bobmcallan/vire/internal/common"
)

// handleShutdown handles POST /api/shutdown (dev mode only).
func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodPost) {
		return
	}

	if s.app.Config.IsProduction() {
		WriteError(w, http.StatusForbidden, "Shutdown endpoint disabled in production")
		return
	}

	s.logger.Info().Msg("Shutdown requested via HTTP endpoint")

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Shutting down gracefully...\n"))

	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	if s.shutdownChan != nil {
		go func() {
			time.Sleep(100 * time.Millisecond)
			s.shutdownChan <- struct{}{}
		}()
	}
}

// registerRoutes sets up all REST API routes on the mux.
func (s *Server) registerRoutes(mux *http.ServeMux) {
	// System
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/version", s.handleVersion)
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/diagnostics", s.handleDiagnostics)
	mux.HandleFunc("/api/mcp/tools", s.handleToolCatalog)
	mux.HandleFunc("/api/shutdown", s.handleShutdown)

	// Users
	mux.HandleFunc("/api/users/import", s.handleUserImport)
	mux.HandleFunc("/api/users/", s.routeUsers)
	mux.HandleFunc("/api/users", s.handleUserCreate)

	// Auth
	mux.HandleFunc("/api/auth/login", s.handleAuthLogin)
	mux.HandleFunc("/api/auth/oauth", s.handleAuthOAuth)
	mux.HandleFunc("/api/auth/validate", s.handleAuthValidate)
	mux.HandleFunc("/api/auth/login/google", s.handleOAuthLoginGoogle)
	mux.HandleFunc("/api/auth/login/github", s.handleOAuthLoginGitHub)
	mux.HandleFunc("/api/auth/callback/google", s.handleOAuthCallbackGoogle)
	mux.HandleFunc("/api/auth/callback/github", s.handleOAuthCallbackGitHub)

	// Portfolios
	mux.HandleFunc("/api/portfolios/default", s.handlePortfolioDefault)
	mux.HandleFunc("/api/portfolios/", s.routePortfolios)
	mux.HandleFunc("/api/portfolios", s.handlePortfolioList)

	// Market Data
	mux.HandleFunc("/api/market/quote/", s.handleMarketQuote)
	mux.HandleFunc("/api/market/stocks/", s.handleMarketStocks)
	mux.HandleFunc("/api/market/signals", s.handleMarketSignals)
	mux.HandleFunc("/api/market/collect", s.handleMarketCollect)

	// Screening
	mux.HandleFunc("/api/screen/snipe", s.handleScreenSnipe)
	mux.HandleFunc("/api/screen/funnel", s.handleScreenFunnel)
	mux.HandleFunc("/api/screen", s.handleScreen)

	// Searches
	mux.HandleFunc("/api/searches/", s.handleSearchByID)
	mux.HandleFunc("/api/searches", s.handleSearchList)

	// Reports (non-portfolio)
	mux.HandleFunc("/api/reports", s.handleReportList)

	// Strategy template
	mux.HandleFunc("/api/strategies/template", s.handleStrategyTemplate)
}

// routePortfolios dispatches /api/portfolios/{name}/* to the appropriate handler.
func (s *Server) routePortfolios(w http.ResponseWriter, r *http.Request) {
	// Extract portfolio name from path
	path := strings.TrimPrefix(r.URL.Path, "/api/portfolios/")
	if path == "" {
		s.handlePortfolioList(w, r)
		return
	}

	// Split into name and sub-path
	parts := strings.SplitN(path, "/", 2)
	name := parts[0]
	subpath := ""
	if len(parts) > 1 {
		subpath = parts[1]
	}

	switch subpath {
	case "":
		s.handlePortfolioGet(w, r, name)
	case "review":
		s.handlePortfolioReview(w, r, name)
	case "sync":
		s.handlePortfolioSync(w, r, name)
	case "rebuild":
		s.handlePortfolioRebuild(w, r, name)
	case "snapshot":
		s.handlePortfolioSnapshot(w, r, name)
	case "history":
		s.handlePortfolioHistory(w, r, name)
	case "report":
		s.handlePortfolioReport(w, r, name)
	case "summary":
		s.handlePortfolioSummary(w, r, name)
	case "tickers":
		s.handlePortfolioTickers(w, r, name)
	case "strategy":
		s.handlePortfolioStrategy(w, r, name)
	case "plan":
		s.handlePortfolioPlan(w, r, name)
	case "watchlist":
		s.handlePortfolioWatchlist(w, r, name)
	default:
		// Check for nested paths: plan/items, plan/items/{id}, plan/status
		// reports/{ticker}, stock/{ticker}, watchlist/items, watchlist/items/{ticker}
		if strings.HasPrefix(subpath, "plan/") {
			s.routePlan(w, r, name, strings.TrimPrefix(subpath, "plan/"))
		} else if strings.HasPrefix(subpath, "reports/") {
			ticker := strings.TrimPrefix(subpath, "reports/")
			s.handlePortfolioTickerReport(w, r, name, ticker)
		} else if strings.HasPrefix(subpath, "stock/") {
			ticker := strings.TrimPrefix(subpath, "stock/")
			s.handlePortfolioStock(w, r, name, ticker)
		} else if strings.HasPrefix(subpath, "watchlist/") {
			s.routeWatchlist(w, r, name, strings.TrimPrefix(subpath, "watchlist/"))
		} else {
			WriteError(w, http.StatusNotFound, "Not found")
		}
	}
}

// routePlan dispatches /api/portfolios/{name}/plan/* sub-routes.
func (s *Server) routePlan(w http.ResponseWriter, r *http.Request, portfolioName, subpath string) {
	switch {
	case subpath == "status":
		s.handlePlanStatus(w, r, portfolioName)
	case subpath == "items":
		s.handlePlanItemAdd(w, r, portfolioName)
	case strings.HasPrefix(subpath, "items/"):
		itemID := strings.TrimPrefix(subpath, "items/")
		s.handlePlanItem(w, r, portfolioName, itemID)
	default:
		WriteError(w, http.StatusNotFound, "Not found")
	}
}

// routeWatchlist dispatches /api/portfolios/{name}/watchlist/* sub-routes.
func (s *Server) routeWatchlist(w http.ResponseWriter, r *http.Request, portfolioName, subpath string) {
	switch {
	case subpath == "items":
		s.handleWatchlistItemAdd(w, r, portfolioName)
	case strings.HasPrefix(subpath, "items/"):
		ticker := strings.TrimPrefix(subpath, "items/")
		s.handleWatchlistItem(w, r, portfolioName, ticker)
	default:
		WriteError(w, http.StatusNotFound, "Not found")
	}
}

// --- System handlers ---

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodGet, http.MethodHead) {
		return
	}
	WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodGet, http.MethodHead) {
		return
	}
	WriteJSON(w, http.StatusOK, map[string]string{
		"version": common.GetVersion(),
		"build":   common.GetBuild(),
		"commit":  common.GetGitCommit(),
	})
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodGet) {
		return
	}
	ctx := r.Context()

	store := s.app.Storage.InternalStore()

	// Build runtime settings from system KV
	kvAll := map[string]string{}
	for _, key := range []string{"vire_schema_version", "vire_build_timestamp", "default_portfolio", "eodhd_api_key", "gemini_api_key"} {
		if val, err := store.GetSystemKV(ctx, key); err == nil && val != "" {
			kvAll[key] = val
		}
	}
	// Mask secrets
	for k, v := range kvAll {
		if strings.Contains(k, "api_key") {
			kvAll[k] = maskSecret(v)
		}
	}

	resolvedPortfolios := common.ResolvePortfolios(ctx)
	resolvedCurrency := common.ResolveDisplayCurrency(ctx)
	resolvedPortfolio := common.ResolveDefaultPortfolio(ctx, store)

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"runtime_settings":      kvAll,
		"default_portfolio":     resolvedPortfolio,
		"portfolios":            resolvedPortfolios,
		"display_currency":      resolvedCurrency,
		"environment":           s.app.Config.Environment,
		"storage_internal_path": s.app.Config.Storage.Internal.Path,
		"storage_user_path":     s.app.Config.Storage.User.Path,
		"storage_market_path":   s.app.Config.Storage.Market.Path,
		"logging_level":         s.app.Config.Logging.Level,
		"eodhd_configured":      s.app.EODHDClient != nil,
		"navexa_configured":     true, // always available via portal injection
		"gemini_configured":     s.app.GeminiClient != nil,
	})
}

func (s *Server) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodGet) {
		return
	}

	correlationID := r.URL.Query().Get("correlation_id")
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := parseInt(l); err == nil && v > 0 && v <= 500 {
			limit = v
		}
	}

	uptime := time.Since(s.app.StartupTime).Round(time.Second)

	resp := map[string]interface{}{
		"version":    common.GetVersion(),
		"build":      common.GetBuild(),
		"commit":     common.GetGitCommit(),
		"uptime":     uptime.String(),
		"started_at": s.app.StartupTime,
	}

	if correlationID != "" {
		logs, err := s.app.Logger.GetMemoryLogsForCorrelation(correlationID)
		if err == nil {
			resp["correlation_logs"] = logs
		}
	}

	logs, err := s.app.Logger.GetMemoryLogsWithLimit(limit)
	if err == nil {
		resp["recent_logs"] = logs
	}

	WriteJSON(w, http.StatusOK, resp)
}

func (s *Server) handleToolCatalog(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodGet) {
		return
	}
	WriteJSON(w, http.StatusOK, buildToolCatalog())
}

func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 4 {
		return "****"
	}
	return s[:4] + "****"
}

func parseInt(s string) (int, error) {
	var v int
	_, err := json.Number(s).Int64()
	if err != nil {
		return 0, err
	}
	n, _ := json.Number(s).Int64()
	v = int(n)
	return v, nil
}
