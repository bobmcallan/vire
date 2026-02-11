package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/bobmccarthy/vire/internal/common"
	"github.com/bobmccarthy/vire/internal/interfaces"
	"github.com/bobmccarthy/vire/internal/models"
)

// --- Portfolio handlers ---

func (s *Server) handlePortfolioList(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodGet) {
		return
	}

	portfolios, err := s.app.PortfolioService.ListPortfolios(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error listing portfolios: %v", err))
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"portfolios": portfolios,
	})
}

func (s *Server) handlePortfolioGet(w http.ResponseWriter, r *http.Request, name string) {
	if !RequireMethod(w, r, http.MethodGet) {
		return
	}

	portfolio, err := s.app.PortfolioService.GetPortfolio(r.Context(), name)
	if err != nil {
		WriteError(w, http.StatusNotFound, fmt.Sprintf("Portfolio not found: %v", err))
		return
	}

	WriteJSON(w, http.StatusOK, portfolio)
}

func (s *Server) handlePortfolioReview(w http.ResponseWriter, r *http.Request, name string) {
	if !RequireMethod(w, r, http.MethodPost) {
		return
	}

	var req struct {
		FocusSignals []string `json:"focus_signals"`
		IncludeNews  bool     `json:"include_news"`
	}
	if r.Body != nil {
		json.NewDecoder(r.Body).Decode(&req)
	}

	review, err := s.app.PortfolioService.ReviewPortfolio(r.Context(), name, interfaces.ReviewOptions{
		FocusSignals: req.FocusSignals,
		IncludeNews:  req.IncludeNews,
	})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Review error: %v", err))
		return
	}

	// Append strategy context if available
	var strategyContext *models.PortfolioStrategy
	if strat, err := s.app.Storage.StrategyStorage().GetStrategy(r.Context(), name); err == nil {
		strategyContext = strat
	}

	// Get growth data
	dailyPoints, _ := s.app.PortfolioService.GetDailyGrowth(r.Context(), name, interfaces.GrowthOptions{})

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"review":   review,
		"strategy": strategyContext,
		"growth":   dailyPoints,
	})
}

func (s *Server) handlePortfolioSync(w http.ResponseWriter, r *http.Request, name string) {
	if !RequireMethod(w, r, http.MethodPost) {
		return
	}

	var req struct {
		Force bool `json:"force"`
	}
	if r.Body != nil {
		json.NewDecoder(r.Body).Decode(&req)
	}

	portfolio, err := s.app.PortfolioService.SyncPortfolio(r.Context(), name, req.Force)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Sync error: %v", err))
		return
	}

	WriteJSON(w, http.StatusOK, portfolio)
}

func (s *Server) handlePortfolioRebuild(w http.ResponseWriter, r *http.Request, name string) {
	if !RequireMethod(w, r, http.MethodPost) {
		return
	}

	ctx := r.Context()

	// Step 1: Purge all derived data
	counts, err := s.app.Storage.PurgeDerivedData(ctx)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Rebuild failed during purge: %v", err))
		return
	}

	// Step 2: Update schema version
	s.app.Storage.KeyValueStorage().Set(ctx, "vire_schema_version", common.SchemaVersion)

	// Step 3: Re-sync portfolio
	p, err := s.app.PortfolioService.SyncPortfolio(ctx, name, true)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Rebuild: sync failed: %v", err))
		return
	}

	// Step 4: Collect market data
	tickers := extractTickers(p)
	marketCount := 0
	if len(tickers) > 0 {
		if err := s.app.MarketService.CollectMarketData(ctx, tickers, false, true); err == nil {
			marketCount = len(tickers)
		}
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"purged": counts,
		"rebuilt": map[string]interface{}{
			"portfolio":      name,
			"holdings":       len(p.Holdings),
			"market_tickers": marketCount,
			"schema_version": common.SchemaVersion,
		},
	})
}

func (s *Server) handlePortfolioDefault(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		current := common.ResolveDefaultPortfolio(ctx, s.app.Storage.KeyValueStorage(), s.app.DefaultPortfolio)
		portfolios, _ := s.app.PortfolioService.ListPortfolios(ctx)
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"default":    current,
			"portfolios": portfolios,
		})

	case http.MethodPut:
		var req struct {
			Name string `json:"name"`
		}
		if !DecodeJSON(w, r, &req) {
			return
		}
		if req.Name == "" {
			WriteError(w, http.StatusBadRequest, "name is required")
			return
		}
		if err := s.app.Storage.KeyValueStorage().Set(ctx, "default_portfolio", req.Name); err != nil {
			WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to set default: %v", err))
			return
		}
		WriteJSON(w, http.StatusOK, map[string]string{
			"default": req.Name,
		})

	default:
		RequireMethod(w, r, http.MethodGet, http.MethodPut)
	}
}

func (s *Server) handlePortfolioSnapshot(w http.ResponseWriter, r *http.Request, name string) {
	if !RequireMethod(w, r, http.MethodGet) {
		return
	}

	dateStr := r.URL.Query().Get("date")
	if dateStr == "" {
		WriteError(w, http.StatusBadRequest, "date query parameter is required (format: YYYY-MM-DD)")
		return
	}

	asOf, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, fmt.Sprintf("Invalid date format '%s' — use YYYY-MM-DD", dateStr))
		return
	}

	if asOf.After(time.Now()) {
		WriteError(w, http.StatusBadRequest, "date must be in the past")
		return
	}

	snapshot, err := s.app.PortfolioService.GetPortfolioSnapshot(r.Context(), name, asOf)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Snapshot error: %v", err))
		return
	}

	WriteJSON(w, http.StatusOK, snapshot)
}

func (s *Server) handlePortfolioHistory(w http.ResponseWriter, r *http.Request, name string) {
	if !RequireMethod(w, r, http.MethodGet) {
		return
	}

	opts := interfaces.GrowthOptions{}

	if fromStr := r.URL.Query().Get("from"); fromStr != "" {
		t, err := time.Parse("2006-01-02", fromStr)
		if err != nil {
			WriteError(w, http.StatusBadRequest, fmt.Sprintf("Invalid from date '%s' — use YYYY-MM-DD", fromStr))
			return
		}
		opts.From = t
	}

	if toStr := r.URL.Query().Get("to"); toStr != "" {
		t, err := time.Parse("2006-01-02", toStr)
		if err != nil {
			WriteError(w, http.StatusBadRequest, fmt.Sprintf("Invalid to date '%s' — use YYYY-MM-DD", toStr))
			return
		}
		opts.To = t
	}

	format := r.URL.Query().Get("format")
	if format == "" {
		format = "auto"
	}

	points, err := s.app.PortfolioService.GetDailyGrowth(r.Context(), name, opts)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("History error: %v", err))
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"portfolio":   name,
		"format":      format,
		"data_points": points,
		"count":       len(points),
	})
}

// --- Market Data handlers ---

func (s *Server) handleMarketStocks(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodGet) {
		return
	}

	ticker := strings.TrimPrefix(r.URL.Path, "/api/market/stocks/")
	if ticker == "" {
		WriteError(w, http.StatusBadRequest, "ticker is required in path")
		return
	}

	ticker, errMsg := validateTicker(ticker)
	if errMsg != "" {
		WriteError(w, http.StatusBadRequest, errMsg)
		return
	}

	includeParam := r.URL.Query().Get("include")
	include := interfaces.StockDataInclude{
		Price: true, Fundamentals: true, Signals: true, News: true,
	}
	if includeParam != "" {
		include = interfaces.StockDataInclude{}
		for _, inc := range strings.Split(includeParam, ",") {
			switch strings.TrimSpace(inc) {
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
	}

	stockData, err := s.app.MarketService.GetStockData(r.Context(), ticker, include)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error getting stock data: %v", err))
		return
	}

	WriteJSON(w, http.StatusOK, stockData)
}

func (s *Server) handleMarketSignals(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodPost) {
		return
	}

	var req struct {
		Tickers     []string `json:"tickers"`
		SignalTypes []string `json:"signal_types"`
	}
	if !DecodeJSON(w, r, &req) {
		return
	}

	if len(req.Tickers) == 0 {
		WriteError(w, http.StatusBadRequest, "tickers is required")
		return
	}

	tickers, errMsg := validateTickers(req.Tickers)
	if errMsg != "" {
		WriteError(w, http.StatusBadRequest, errMsg)
		return
	}

	signals, err := s.app.SignalService.DetectSignals(r.Context(), tickers, req.SignalTypes, false)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Signal detection error: %v", err))
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"signals": signals,
	})
}

func (s *Server) handleMarketCollect(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodPost) {
		return
	}

	var req struct {
		Tickers     []string `json:"tickers"`
		IncludeNews bool     `json:"include_news"`
	}
	if !DecodeJSON(w, r, &req) {
		return
	}

	if len(req.Tickers) == 0 {
		WriteError(w, http.StatusBadRequest, "tickers is required")
		return
	}

	tickers, errMsg := validateTickers(req.Tickers)
	if errMsg != "" {
		WriteError(w, http.StatusBadRequest, errMsg)
		return
	}

	err := s.app.MarketService.CollectMarketData(r.Context(), tickers, req.IncludeNews, false)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Collection error: %v", err))
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"collected": len(tickers),
		"tickers":   tickers,
	})
}

// --- Screening handlers ---

func (s *Server) handleScreen(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodPost) {
		return
	}

	var req struct {
		Exchange    string  `json:"exchange"`
		Limit       int     `json:"limit"`
		MaxPE       float64 `json:"max_pe"`
		MinReturn   float64 `json:"min_return"`
		Sector      string  `json:"sector"`
		IncludeNews bool    `json:"include_news"`
		Portfolio   string  `json:"portfolio_name"`
	}
	if !DecodeJSON(w, r, &req) {
		return
	}

	if req.Exchange == "" {
		WriteError(w, http.StatusBadRequest, "exchange is required")
		return
	}

	if req.Limit <= 0 {
		req.Limit = 5
	}
	if req.Limit > 15 {
		req.Limit = 15
	}

	ctx := r.Context()

	// Auto-load strategy
	portfolioName := s.resolvePortfolio(ctx, req.Portfolio)
	strategy, _ := s.app.Storage.StrategyStorage().GetStrategy(ctx, portfolioName)

	candidates, err := s.app.MarketService.ScreenStocks(ctx, interfaces.ScreenOptions{
		Exchange:        req.Exchange,
		Limit:           req.Limit,
		MaxPE:           req.MaxPE,
		MinQtrReturnPct: req.MinReturn,
		Sector:          req.Sector,
		IncludeNews:     req.IncludeNews,
		Strategy:        strategy,
	})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Screen error: %v", err))
		return
	}

	// Auto-save search
	searchID := s.autoSaveScreenSearch(ctx, candidates, req.Exchange, req.MaxPE, req.MinReturn, req.Sector, strategy)

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"candidates": candidates,
		"count":      len(candidates),
		"search_id":  searchID,
	})
}

func (s *Server) handleScreenSnipe(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodPost) {
		return
	}

	var req struct {
		Exchange    string   `json:"exchange"`
		Limit       int      `json:"limit"`
		Criteria    []string `json:"criteria"`
		Sector      string   `json:"sector"`
		IncludeNews bool     `json:"include_news"`
		Portfolio   string   `json:"portfolio_name"`
	}
	if !DecodeJSON(w, r, &req) {
		return
	}

	if req.Exchange == "" {
		WriteError(w, http.StatusBadRequest, "exchange is required")
		return
	}

	if req.Limit <= 0 {
		req.Limit = 3
	}
	if req.Limit > 10 {
		req.Limit = 10
	}

	ctx := r.Context()
	portfolioName := s.resolvePortfolio(ctx, req.Portfolio)
	strategy, _ := s.app.Storage.StrategyStorage().GetStrategy(ctx, portfolioName)

	snipeBuys, err := s.app.MarketService.FindSnipeBuys(ctx, interfaces.SnipeOptions{
		Exchange:    req.Exchange,
		Limit:       req.Limit,
		Criteria:    req.Criteria,
		Sector:      req.Sector,
		IncludeNews: req.IncludeNews,
		Strategy:    strategy,
	})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Snipe error: %v", err))
		return
	}

	searchID := s.autoSaveSnipeSearch(ctx, snipeBuys, req.Exchange, req.Criteria, req.Sector, strategy)

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"snipe_buys": snipeBuys,
		"count":      len(snipeBuys),
		"search_id":  searchID,
	})
}

func (s *Server) handleScreenFunnel(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodPost) {
		return
	}

	var req struct {
		Exchange    string `json:"exchange"`
		Limit       int    `json:"limit"`
		Sector      string `json:"sector"`
		IncludeNews bool   `json:"include_news"`
		Portfolio   string `json:"portfolio_name"`
	}
	if !DecodeJSON(w, r, &req) {
		return
	}

	if req.Exchange == "" {
		WriteError(w, http.StatusBadRequest, "exchange is required")
		return
	}

	if req.Limit <= 0 {
		req.Limit = 5
	}
	if req.Limit > 10 {
		req.Limit = 10
	}

	ctx := r.Context()
	portfolioName := s.resolvePortfolio(ctx, req.Portfolio)
	strategy, _ := s.app.Storage.StrategyStorage().GetStrategy(ctx, portfolioName)

	result, err := s.app.MarketService.FunnelScreen(ctx, interfaces.FunnelOptions{
		Exchange:    req.Exchange,
		Limit:       req.Limit,
		Sector:      req.Sector,
		IncludeNews: req.IncludeNews,
		Strategy:    strategy,
	})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Funnel screen error: %v", err))
		return
	}

	searchID := s.autoSaveFunnelSearch(ctx, result, req.Exchange, req.Sector, strategy)

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"candidates": result.Candidates,
		"stages":     result.Stages,
		"count":      len(result.Candidates),
		"search_id":  searchID,
	})
}

// --- Report handlers ---

func (s *Server) handleReportList(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodGet) {
		return
	}

	filterName := r.URL.Query().Get("portfolio_name")

	reports, err := s.app.Storage.ReportStorage().ListReports(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error listing reports: %v", err))
		return
	}

	type reportInfo struct {
		PortfolioName string    `json:"portfolio_name"`
		GeneratedAt   time.Time `json:"generated_at"`
		TickerCount   int       `json:"ticker_count"`
	}

	var result []reportInfo
	for _, name := range reports {
		if filterName != "" && !strings.EqualFold(name, filterName) {
			continue
		}
		report, err := s.app.Storage.ReportStorage().GetReport(r.Context(), name)
		if err != nil {
			continue
		}
		result = append(result, reportInfo{
			PortfolioName: name,
			GeneratedAt:   report.GeneratedAt,
			TickerCount:   len(report.TickerReports),
		})
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"reports": result,
	})
}

func (s *Server) handlePortfolioReport(w http.ResponseWriter, r *http.Request, name string) {
	if !RequireMethod(w, r, http.MethodPost) {
		return
	}

	var req struct {
		ForceRefresh bool `json:"force_refresh"`
		IncludeNews  bool `json:"include_news"`
	}
	if r.Body != nil {
		json.NewDecoder(r.Body).Decode(&req)
	}

	ctx := r.Context()

	// Smart caching
	if !req.ForceRefresh {
		existing, err := s.app.Storage.ReportStorage().GetReport(ctx, name)
		if err == nil && common.IsFresh(existing.GeneratedAt, common.FreshnessReport) {
			WriteJSON(w, http.StatusOK, map[string]interface{}{
				"cached":       true,
				"generated_at": existing.GeneratedAt,
				"tickers":      existing.Tickers,
				"ticker_count": len(existing.TickerReports),
			})
			return
		}
	}

	report, err := s.app.ReportService.GenerateReport(ctx, name, interfaces.ReportOptions{
		ForceRefresh: req.ForceRefresh,
		IncludeNews:  req.IncludeNews,
	})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Report generation error: %v", err))
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"cached":       false,
		"generated_at": report.GeneratedAt,
		"tickers":      report.Tickers,
		"ticker_count": len(report.TickerReports),
	})
}

func (s *Server) handlePortfolioTickerReport(w http.ResponseWriter, r *http.Request, portfolioName, ticker string) {
	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		// Get existing report
		report, err := s.app.Storage.ReportStorage().GetReport(ctx, portfolioName)
		if err != nil || !common.IsFresh(report.GeneratedAt, common.FreshnessReport) {
			// Auto-generate
			report, err = s.app.ReportService.GenerateReport(ctx, portfolioName, interfaces.ReportOptions{})
			if err != nil {
				WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to generate report: %v", err))
				return
			}
		}

		for _, tr := range report.TickerReports {
			if strings.EqualFold(tr.Ticker, ticker) {
				WriteJSON(w, http.StatusOK, tr)
				return
			}
		}
		WriteError(w, http.StatusNotFound, fmt.Sprintf("Ticker '%s' not found in report for '%s'", ticker, portfolioName))

	case http.MethodPost:
		// Generate/regenerate ticker report
		report, err := s.app.ReportService.GenerateTickerReport(ctx, portfolioName, ticker)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Ticker report error: %v", err))
			return
		}
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"generated_at": report.GeneratedAt,
			"ticker":       ticker,
		})

	default:
		RequireMethod(w, r, http.MethodGet, http.MethodPost)
	}
}

func (s *Server) handlePortfolioSummary(w http.ResponseWriter, r *http.Request, name string) {
	if !RequireMethod(w, r, http.MethodGet) {
		return
	}

	ctx := r.Context()

	// Try cached report
	report, err := s.app.Storage.ReportStorage().GetReport(ctx, name)
	if err != nil || !common.IsFresh(report.GeneratedAt, common.FreshnessReport) {
		// Auto-generate
		report, err = s.app.ReportService.GenerateReport(ctx, name, interfaces.ReportOptions{})
		if err != nil {
			WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to generate report: %v", err))
			return
		}
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"summary":      report.SummaryMarkdown,
		"generated_at": report.GeneratedAt,
		"tickers":      report.Tickers,
	})
}

func (s *Server) handlePortfolioTickers(w http.ResponseWriter, r *http.Request, name string) {
	if !RequireMethod(w, r, http.MethodGet) {
		return
	}

	report, err := s.app.Storage.ReportStorage().GetReport(r.Context(), name)
	if err != nil {
		WriteError(w, http.StatusNotFound, fmt.Sprintf("Report not found for '%s': %v", name, err))
		return
	}

	type tickerInfo struct {
		Ticker string `json:"ticker"`
		Name   string `json:"name"`
		IsETF  bool   `json:"is_etf"`
	}

	var tickers []tickerInfo
	for _, tr := range report.TickerReports {
		tickers = append(tickers, tickerInfo{
			Ticker: tr.Ticker,
			Name:   tr.Name,
			IsETF:  tr.IsETF,
		})
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"portfolio":    name,
		"generated_at": report.GeneratedAt,
		"tickers":      tickers,
	})
}

// --- Strategy handlers ---

func (s *Server) handleStrategyTemplate(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodGet) {
		return
	}

	accountType := r.URL.Query().Get("account_type")

	// Return the strategy template as structured JSON
	template := map[string]interface{}{
		"account_type":        "smsf | trading",
		"investment_universe": []string{"AU", "US"},
		"risk_appetite": map[string]interface{}{
			"level":            "conservative | moderate | aggressive",
			"max_drawdown_pct": 15,
			"description":      "Free-text risk tolerance description",
		},
		"target_returns": map[string]interface{}{
			"annual_pct": 10,
			"timeframe":  "3-5 years",
		},
		"income_requirements": map[string]interface{}{
			"dividend_yield_pct": 4.0,
			"description":        "Income strategy notes",
		},
		"sector_preferences": map[string]interface{}{
			"preferred": []string{"Financials", "Healthcare"},
			"excluded":  []string{"Gambling"},
		},
		"position_sizing": map[string]interface{}{
			"max_position_pct": 10,
			"max_sector_pct":   30,
		},
		"company_filter": map[string]interface{}{
			"min_market_cap":      500000000,
			"max_market_cap":      50000000000,
			"max_pe":              25,
			"min_qtr_return_pct":  10,
			"min_dividend_yield":  0.02,
			"max_beta":            1.3,
			"allowed_sectors":     []string{"Technology", "Healthcare"},
			"excluded_sectors":    []string{"Banks", "REITs"},
			"allowed_countries":   []string{"US", "AU"},
			"_description":        "Stock screening filters used by stock_screen, funnel_screen, and market_snipe. allowed_countries uses ISO 2-letter codes.",
		},
		"rebalance_frequency": "quarterly",
		"notes":               "Free-form markdown for tax considerations, life events, etc.",
	}

	if strings.ToLower(accountType) == "smsf" {
		template["smsf_notes"] = "SMSF trustees must maintain a documented investment strategy (SIS Regulation 4.09)"
	}

	WriteJSON(w, http.StatusOK, template)
}

func (s *Server) handlePortfolioStrategy(w http.ResponseWriter, r *http.Request, name string) {
	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		strategy, err := s.app.StrategyService.GetStrategy(ctx, name)
		if err != nil {
			WriteJSON(w, http.StatusOK, map[string]interface{}{
				"exists":  false,
				"message": fmt.Sprintf("No strategy found for portfolio '%s'", name),
			})
			return
		}
		WriteJSON(w, http.StatusOK, strategy)

	case http.MethodPut:
		var req struct {
			StrategyJSON json.RawMessage `json:"strategy"`
		}
		if !DecodeJSON(w, r, &req) {
			return
		}

		// Load existing or create new
		existing, err := s.app.StrategyService.GetStrategy(ctx, name)
		if err != nil {
			existing = &models.PortfolioStrategy{PortfolioName: name}
		}

		// Merge incoming JSON on top
		if err := json.Unmarshal(req.StrategyJSON, existing); err != nil {
			WriteError(w, http.StatusBadRequest, fmt.Sprintf("Error parsing strategy: %v", err))
			return
		}
		existing.PortfolioName = name

		warnings, err := s.app.StrategyService.SaveStrategy(ctx, existing)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error saving strategy: %v", err))
			return
		}

		saved, err := s.app.StrategyService.GetStrategy(ctx, name)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Saved but failed to reload: %v", err))
			return
		}

		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"strategy": saved,
			"warnings": warnings,
		})

	case http.MethodDelete:
		if err := s.app.StrategyService.DeleteStrategy(ctx, name); err != nil {
			WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error deleting strategy: %v", err))
			return
		}
		WriteJSON(w, http.StatusOK, map[string]string{"deleted": name})

	default:
		RequireMethod(w, r, http.MethodGet, http.MethodPut, http.MethodDelete)
	}
}

// --- Plan handlers ---

func (s *Server) handlePortfolioPlan(w http.ResponseWriter, r *http.Request, name string) {
	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		plan, err := s.app.PlanService.GetPlan(ctx, name)
		if err != nil {
			WriteJSON(w, http.StatusOK, map[string]interface{}{
				"exists":  false,
				"message": fmt.Sprintf("No plan found for portfolio '%s'", name),
			})
			return
		}
		WriteJSON(w, http.StatusOK, plan)

	case http.MethodPut:
		var plan models.PortfolioPlan
		if !DecodeJSON(w, r, &plan) {
			return
		}
		plan.PortfolioName = name

		if err := s.app.PlanService.SavePlan(ctx, &plan); err != nil {
			WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error saving plan: %v", err))
			return
		}

		saved, err := s.app.PlanService.GetPlan(ctx, name)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Saved but failed to reload: %v", err))
			return
		}
		WriteJSON(w, http.StatusOK, saved)

	default:
		RequireMethod(w, r, http.MethodGet, http.MethodPut)
	}
}

func (s *Server) handlePlanStatus(w http.ResponseWriter, r *http.Request, name string) {
	if !RequireMethod(w, r, http.MethodGet) {
		return
	}

	ctx := r.Context()

	triggered, err := s.app.PlanService.CheckPlanEvents(ctx, name)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error checking events: %v", err))
		return
	}

	expired, err := s.app.PlanService.CheckPlanDeadlines(ctx, name)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error checking deadlines: %v", err))
		return
	}

	plan, _ := s.app.PlanService.GetPlan(ctx, name)

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"triggered": triggered,
		"expired":   expired,
		"plan":      plan,
	})
}

func (s *Server) handlePlanItemAdd(w http.ResponseWriter, r *http.Request, name string) {
	if !RequireMethod(w, r, http.MethodPost) {
		return
	}

	var item models.PlanItem
	if !DecodeJSON(w, r, &item) {
		return
	}

	plan, err := s.app.PlanService.AddPlanItem(r.Context(), name, &item)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error adding plan item: %v", err))
		return
	}

	WriteJSON(w, http.StatusCreated, plan)
}

func (s *Server) handlePlanItem(w http.ResponseWriter, r *http.Request, name, itemID string) {
	ctx := r.Context()

	switch r.Method {
	case http.MethodPatch:
		var update models.PlanItem
		if !DecodeJSON(w, r, &update) {
			return
		}

		plan, err := s.app.PlanService.UpdatePlanItem(ctx, name, itemID, &update)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error updating plan item: %v", err))
			return
		}
		WriteJSON(w, http.StatusOK, plan)

	case http.MethodDelete:
		plan, err := s.app.PlanService.RemovePlanItem(ctx, name, itemID)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error removing plan item: %v", err))
			return
		}
		WriteJSON(w, http.StatusOK, plan)

	default:
		RequireMethod(w, r, http.MethodPatch, http.MethodDelete)
	}
}

// --- Watchlist handlers ---

func (s *Server) handlePortfolioWatchlist(w http.ResponseWriter, r *http.Request, name string) {
	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		wl, err := s.app.WatchlistService.GetWatchlist(ctx, name)
		if err != nil {
			WriteJSON(w, http.StatusOK, map[string]interface{}{
				"exists":  false,
				"message": fmt.Sprintf("No watchlist found for portfolio '%s'", name),
			})
			return
		}
		WriteJSON(w, http.StatusOK, wl)

	case http.MethodPut:
		var wl models.PortfolioWatchlist
		if !DecodeJSON(w, r, &wl) {
			return
		}
		wl.PortfolioName = name

		// Validate items
		for i, item := range wl.Items {
			if item.Ticker == "" {
				WriteError(w, http.StatusBadRequest, fmt.Sprintf("item %d is missing ticker", i))
				return
			}
			ticker, errMsg := validateTicker(item.Ticker)
			if errMsg != "" {
				WriteError(w, http.StatusBadRequest, errMsg)
				return
			}
			wl.Items[i].Ticker = ticker
			if item.Verdict != "" && !models.ValidWatchlistVerdict(item.Verdict) {
				WriteError(w, http.StatusBadRequest, fmt.Sprintf("item %d has invalid verdict '%s'", i, item.Verdict))
				return
			}
		}

		if err := s.app.WatchlistService.SaveWatchlist(ctx, &wl); err != nil {
			WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error saving watchlist: %v", err))
			return
		}

		saved, err := s.app.WatchlistService.GetWatchlist(ctx, name)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "Saved but failed to reload")
			return
		}
		WriteJSON(w, http.StatusOK, saved)

	default:
		RequireMethod(w, r, http.MethodGet, http.MethodPut)
	}
}

func (s *Server) handleWatchlistItemAdd(w http.ResponseWriter, r *http.Request, name string) {
	if !RequireMethod(w, r, http.MethodPost) {
		return
	}

	var item models.WatchlistItem
	if !DecodeJSON(w, r, &item) {
		return
	}

	if item.Ticker == "" {
		WriteError(w, http.StatusBadRequest, "ticker is required")
		return
	}

	ticker, errMsg := validateTicker(item.Ticker)
	if errMsg != "" {
		WriteError(w, http.StatusBadRequest, errMsg)
		return
	}
	item.Ticker = ticker

	if item.Verdict != "" && !models.ValidWatchlistVerdict(item.Verdict) {
		WriteError(w, http.StatusBadRequest, fmt.Sprintf("invalid verdict '%s'", item.Verdict))
		return
	}

	wl, err := s.app.WatchlistService.AddOrUpdateItem(r.Context(), name, &item)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error adding watchlist item: %v", err))
		return
	}

	WriteJSON(w, http.StatusCreated, wl)
}

func (s *Server) handleWatchlistItem(w http.ResponseWriter, r *http.Request, name, ticker string) {
	ctx := r.Context()

	ticker, errMsg := validateTicker(ticker)
	if errMsg != "" {
		WriteError(w, http.StatusBadRequest, errMsg)
		return
	}

	switch r.Method {
	case http.MethodPatch:
		var update models.WatchlistItem
		if !DecodeJSON(w, r, &update) {
			return
		}

		if update.Verdict != "" && !models.ValidWatchlistVerdict(update.Verdict) {
			WriteError(w, http.StatusBadRequest, fmt.Sprintf("invalid verdict '%s'", update.Verdict))
			return
		}

		wl, err := s.app.WatchlistService.UpdateItem(ctx, name, ticker, &update)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error updating watchlist item: %v", err))
			return
		}
		WriteJSON(w, http.StatusOK, wl)

	case http.MethodDelete:
		wl, err := s.app.WatchlistService.RemoveItem(ctx, name, ticker)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error removing watchlist item: %v", err))
			return
		}
		WriteJSON(w, http.StatusOK, wl)

	default:
		RequireMethod(w, r, http.MethodPatch, http.MethodDelete)
	}
}

// --- Search handlers ---

func (s *Server) handleSearchList(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodGet) {
		return
	}

	searchType := r.URL.Query().Get("type")
	exchange := r.URL.Query().Get("exchange")
	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := parseInt(l); err == nil && v > 0 {
			limit = v
		}
	}

	records, err := s.app.Storage.SearchHistoryStorage().ListSearches(r.Context(), interfaces.SearchListOptions{
		Type:     searchType,
		Exchange: exchange,
		Limit:    limit,
	})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error listing searches: %v", err))
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"searches": records,
		"count":    len(records),
	})
}

func (s *Server) handleSearchByID(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodGet) {
		return
	}

	searchID := strings.TrimPrefix(r.URL.Path, "/api/searches/")
	if searchID == "" {
		WriteError(w, http.StatusBadRequest, "search_id is required")
		return
	}

	record, err := s.app.Storage.SearchHistoryStorage().GetSearch(r.Context(), searchID)
	if err != nil {
		WriteError(w, http.StatusNotFound, fmt.Sprintf("Search record not found: %v", err))
		return
	}

	WriteJSON(w, http.StatusOK, record)
}

// --- Helper methods ---

func (s *Server) resolvePortfolio(ctx context.Context, requested string) string {
	if requested != "" {
		return requested
	}
	return common.ResolveDefaultPortfolio(ctx, s.app.Storage.KeyValueStorage(), s.app.DefaultPortfolio)
}

// validateTicker checks a ticker has an exchange suffix.
func validateTicker(ticker string) (string, string) {
	ticker = strings.ToUpper(strings.TrimSpace(ticker))
	if ticker != "" && !strings.Contains(ticker, ".") {
		return "", fmt.Sprintf("Ambiguous ticker %q — did you mean %s.AU (ASX) or %s.US (NYSE/NASDAQ)? Please include the exchange suffix.", ticker, ticker, ticker)
	}
	return ticker, ""
}

// validateTickers validates all tickers have exchange suffixes.
func validateTickers(tickers []string) ([]string, string) {
	for i, t := range tickers {
		normalized, errMsg := validateTicker(t)
		if errMsg != "" {
			return nil, errMsg
		}
		tickers[i] = normalized
	}
	return tickers, ""
}

// extractTickers returns tickers with exchange suffixes from portfolio holdings.
func extractTickers(p *models.Portfolio) []string {
	tickers := make([]string, 0, len(p.Holdings))
	for _, h := range p.Holdings {
		if len(h.Trades) > 0 {
			tickers = append(tickers, h.Ticker+".AU")
		}
	}
	return tickers
}

// --- Search auto-save helpers ---

func (s *Server) autoSaveScreenSearch(ctx context.Context, candidates []*models.ScreenCandidate, exchange string, maxPE, minReturn float64, sector string, strategy *models.PortfolioStrategy) string {
	filtersJSON, _ := json.Marshal(map[string]interface{}{
		"exchange":   exchange,
		"max_pe":     maxPE,
		"min_return": minReturn,
		"sector":     sector,
	})
	resultsJSON, _ := json.Marshal(candidates)

	record := &models.SearchRecord{
		Type:        "screen",
		Exchange:    exchange,
		Filters:     string(filtersJSON),
		ResultCount: len(candidates),
		Results:     string(resultsJSON),
		CreatedAt:   time.Now(),
	}
	if strategy != nil {
		record.StrategyName = strategy.PortfolioName
		record.StrategyVer = strategy.Version
	}

	if err := s.app.Storage.SearchHistoryStorage().SaveSearch(ctx, record); err != nil {
		s.logger.Warn().Err(err).Msg("Failed to auto-save screen search")
		return ""
	}
	return record.ID
}

func (s *Server) autoSaveSnipeSearch(ctx context.Context, buys []*models.SnipeBuy, exchange string, criteria []string, sector string, strategy *models.PortfolioStrategy) string {
	filtersJSON, _ := json.Marshal(map[string]interface{}{
		"exchange": exchange,
		"criteria": criteria,
		"sector":   sector,
	})
	resultsJSON, _ := json.Marshal(buys)

	record := &models.SearchRecord{
		Type:        "snipe",
		Exchange:    exchange,
		Filters:     string(filtersJSON),
		ResultCount: len(buys),
		Results:     string(resultsJSON),
		CreatedAt:   time.Now(),
	}
	if strategy != nil {
		record.StrategyName = strategy.PortfolioName
		record.StrategyVer = strategy.Version
	}

	if err := s.app.Storage.SearchHistoryStorage().SaveSearch(ctx, record); err != nil {
		s.logger.Warn().Err(err).Msg("Failed to auto-save snipe search")
		return ""
	}
	return record.ID
}

func (s *Server) autoSaveFunnelSearch(ctx context.Context, result *models.FunnelResult, exchange, sector string, strategy *models.PortfolioStrategy) string {
	filtersJSON, _ := json.Marshal(map[string]interface{}{
		"exchange": exchange,
		"sector":   sector,
	})
	resultsJSON, _ := json.Marshal(result.Candidates)
	stagesJSON, _ := json.Marshal(result.Stages)

	record := &models.SearchRecord{
		Type:        "funnel",
		Exchange:    exchange,
		Filters:     string(filtersJSON),
		ResultCount: len(result.Candidates),
		Results:     string(resultsJSON),
		Stages:      string(stagesJSON),
		CreatedAt:   time.Now(),
	}
	if strategy != nil {
		record.StrategyName = strategy.PortfolioName
		record.StrategyVer = strategy.Version
	}

	if err := s.app.Storage.SearchHistoryStorage().SaveSearch(ctx, record); err != nil {
		s.logger.Warn().Err(err).Msg("Failed to auto-save funnel search")
		return ""
	}
	return record.ID
}
