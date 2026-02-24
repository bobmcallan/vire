package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

// slimHoldingReview strips heavy analysis data from a HoldingReview,
// keeping only position calculations and action fields.
type slimHoldingReview struct {
	Holding        models.Holding           `json:"holding"`
	OvernightMove  float64                  `json:"overnight_move"`
	OvernightPct   float64                  `json:"overnight_pct"`
	NewsImpact     string                   `json:"news_impact,omitempty"`
	ActionRequired string                   `json:"action_required"`
	ActionReason   string                   `json:"action_reason"`
	Compliance     *models.ComplianceResult `json:"compliance,omitempty"`
}

// slimPortfolioReview mirrors PortfolioReview but uses slimHoldingReview
// to exclude signals, fundamentals, and intelligence data from the API response.
type slimPortfolioReview struct {
	PortfolioName       string                      `json:"portfolio_name"`
	ReviewDate          time.Time                   `json:"review_date"`
	TotalValue          float64                     `json:"total_value"`
	TotalCost           float64                     `json:"total_cost"`
	TotalNetReturn      float64                     `json:"total_net_return"`
	TotalNetReturnPct   float64                     `json:"total_net_return_pct"`
	DayChange           float64                     `json:"day_change"`
	DayChangePct        float64                     `json:"day_change_pct"`
	FXRate              float64                     `json:"fx_rate,omitempty"`
	HoldingReviews      []slimHoldingReview         `json:"holding_reviews"`
	Alerts              []models.Alert              `json:"alerts"`
	Summary             string                      `json:"summary"`
	Recommendations     []string                    `json:"recommendations"`
	PortfolioBalance    *models.PortfolioBalance    `json:"portfolio_balance,omitempty"`
	PortfolioIndicators *models.PortfolioIndicators `json:"portfolio_indicators,omitempty"`
}

// toSlimReview converts a full PortfolioReview to a slimPortfolioReview,
// stripping heavy analysis fields from each holding review.
func toSlimReview(review *models.PortfolioReview) slimPortfolioReview {
	slim := slimPortfolioReview{
		PortfolioName:       review.PortfolioName,
		ReviewDate:          review.ReviewDate,
		TotalValue:          review.TotalValue,
		TotalCost:           review.TotalCost,
		TotalNetReturn:      review.TotalNetReturn,
		TotalNetReturnPct:   review.TotalNetReturnPct,
		DayChange:           review.DayChange,
		DayChangePct:        review.DayChangePct,
		FXRate:              review.FXRate,
		Alerts:              review.Alerts,
		Summary:             review.Summary,
		Recommendations:     review.Recommendations,
		PortfolioBalance:    review.PortfolioBalance,
		PortfolioIndicators: review.PortfolioIndicators,
	}

	slim.HoldingReviews = make([]slimHoldingReview, len(review.HoldingReviews))
	for i, hr := range review.HoldingReviews {
		slim.HoldingReviews[i] = slimHoldingReview{
			Holding:        hr.Holding,
			OvernightMove:  hr.OvernightMove,
			OvernightPct:   hr.OvernightPct,
			NewsImpact:     hr.NewsImpact,
			ActionRequired: hr.ActionRequired,
			ActionReason:   hr.ActionReason,
			Compliance:     hr.Compliance,
		}
	}

	return slim
}

// --- Portfolio handlers ---

func (s *Server) handlePortfolioList(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodGet) {
		return
	}

	if !s.requireNavexaContext(w, r) {
		return
	}

	ctx := s.app.InjectNavexaClient(r.Context())
	nxClient := common.NavexaClientFromContext(ctx)

	portfolios, err := nxClient.GetPortfolios(ctx)
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

	ctx := s.app.InjectNavexaClient(r.Context())
	forceRefresh := r.URL.Query().Get("force_refresh") == "true"

	var portfolio *models.Portfolio
	var err error
	if forceRefresh {
		portfolio, err = s.app.PortfolioService.SyncPortfolio(ctx, name, true)
	} else {
		portfolio, err = s.app.PortfolioService.GetPortfolio(ctx, name)
	}
	if err != nil {
		WriteError(w, http.StatusNotFound, fmt.Sprintf("Portfolio not found: %v", err))
		return
	}

	// Strip trades from portfolio-level response; individual stock endpoint returns them
	for i := range portfolio.Holdings {
		portfolio.Holdings[i].Trades = nil
	}

	// Attach capital performance if cash transactions exist (non-fatal on error)
	if perf, err := s.app.CashFlowService.CalculatePerformance(ctx, name); err == nil && perf != nil && perf.TransactionCount > 0 {
		portfolio.CapitalPerformance = perf
	}

	WriteJSON(w, http.StatusOK, portfolio)
}

func (s *Server) handlePortfolioStock(w http.ResponseWriter, r *http.Request, name, ticker string) {
	if !RequireMethod(w, r, http.MethodGet) {
		return
	}

	if ticker == "" {
		WriteError(w, http.StatusBadRequest, "ticker is required in path")
		return
	}

	ctx := s.app.InjectNavexaClient(r.Context())
	forceRefresh := r.URL.Query().Get("force_refresh") == "true"

	var portfolio *models.Portfolio
	var err error
	if forceRefresh {
		portfolio, err = s.app.PortfolioService.SyncPortfolio(ctx, name, true)
	} else {
		portfolio, err = s.app.PortfolioService.GetPortfolio(ctx, name)
	}
	if err != nil {
		WriteError(w, http.StatusNotFound, fmt.Sprintf("Portfolio not found: %v", err))
		return
	}

	// Find matching holding — match against both bare ticker and qualified EODHD ticker
	var found *models.Holding
	for i := range portfolio.Holdings {
		h := &portfolio.Holdings[i]
		if matchHoldingTicker(ticker, h) {
			found = h
			break
		}
	}

	if found == nil {
		available := make([]string, 0, len(portfolio.Holdings))
		for _, h := range portfolio.Holdings {
			available = append(available, h.Ticker)
		}
		WriteError(w, http.StatusNotFound, fmt.Sprintf("Ticker '%s' not found in portfolio '%s'. Available: %s",
			ticker, name, strings.Join(available, ", ")))
		return
	}

	WriteJSON(w, http.StatusOK, found)
}

// matchHoldingTicker checks whether the input ticker matches a holding's bare ticker
// or its qualified EODHD ticker (e.g., "BHP" matches "BHP" and "BHP.AU").
// Comparison is case-insensitive.
func matchHoldingTicker(input string, h *models.Holding) bool {
	input = strings.ToUpper(strings.TrimSpace(input))
	holdingTicker := strings.ToUpper(h.Ticker)
	eodhd := strings.ToUpper(h.EODHDTicker())

	if input == holdingTicker || input == eodhd {
		return true
	}
	// Strip exchange suffix from input and compare to bare holding ticker
	if base, _, ok := strings.Cut(input, "."); ok && base == holdingTicker {
		return true
	}
	// Strip exchange suffix from holding EODHD ticker and compare to input
	if base, _, ok := strings.Cut(eodhd, "."); ok && base == input {
		return true
	}
	return false
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

	ctx := s.app.InjectNavexaClient(r.Context())

	// Ensure market data exists for portfolio holdings before review.
	// ReviewPortfolio reads from storage but doesn't collect — mirror the
	// warm-cache pattern: get portfolio → extract tickers → collect.
	if portfolio, err := s.app.PortfolioService.GetPortfolio(ctx, name); err == nil {
		tickers := make([]string, 0, len(portfolio.Holdings))
		for _, h := range portfolio.Holdings {
			if h.Units > 0 {
				tickers = append(tickers, h.EODHDTicker())
			}
		}
		if len(tickers) > 0 {
			if err := s.app.MarketService.CollectMarketData(ctx, tickers, req.IncludeNews, false); err != nil {
				s.logger.Warn().Err(err).Msg("Pre-review market data collection failed")
			}
		}
	}

	review, err := s.app.PortfolioService.ReviewPortfolio(ctx, name, interfaces.ReviewOptions{
		FocusSignals: req.FocusSignals,
		IncludeNews:  req.IncludeNews,
	})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Review error: %v", err))
		return
	}

	// Append strategy context if available
	var strategyContext *models.PortfolioStrategy
	if strat, err := s.app.StrategyService.GetStrategy(ctx, name); err == nil {
		strategyContext = strat
	}

	// Get growth data
	dailyPoints, _ := s.app.PortfolioService.GetDailyGrowth(ctx, name, interfaces.GrowthOptions{})

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"review":   toSlimReview(review),
		"strategy": strategyContext,
		"growth":   dailyPoints,
	})
}

func (s *Server) handlePortfolioSync(w http.ResponseWriter, r *http.Request, name string) {
	if !RequireMethod(w, r, http.MethodPost) {
		return
	}

	if !s.requireNavexaContext(w, r) {
		return
	}

	var req struct {
		Force bool `json:"force"`
	}
	if r.Body != nil {
		json.NewDecoder(r.Body).Decode(&req)
	}

	ctx := s.app.InjectNavexaClient(r.Context())
	portfolio, err := s.app.PortfolioService.SyncPortfolio(ctx, name, req.Force)
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

	if !s.requireNavexaContext(w, r) {
		return
	}

	ctx := s.app.InjectNavexaClient(r.Context())

	// Step 1: Purge all derived data
	counts, err := s.app.Storage.PurgeDerivedData(ctx)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Rebuild failed during purge: %v", err))
		return
	}

	// Step 2: Update schema version
	s.app.Storage.InternalStore().SetSystemKV(ctx, "vire_schema_version", common.SchemaVersion)

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
		// Check user context for default portfolio override (multi-tenant)
		current := ""
		if uc := common.UserContextFromContext(ctx); uc != nil && len(uc.Portfolios) > 0 {
			current = uc.Portfolios[0]
		}
		if current == "" {
			current = common.ResolveDefaultPortfolio(ctx, s.app.Storage.InternalStore())
		}
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
		if err := s.app.Storage.InternalStore().SetSystemKV(ctx, "default_portfolio", req.Name); err != nil {
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

	ctx := s.app.InjectNavexaClient(r.Context())
	snapshot, err := s.app.PortfolioService.GetPortfolioSnapshot(ctx, name, asOf)
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

	ctx := s.app.InjectNavexaClient(r.Context())
	points, err := s.app.PortfolioService.GetDailyGrowth(ctx, name, opts)
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

func (s *Server) handleMarketQuote(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodGet) {
		return
	}

	ticker := strings.TrimPrefix(r.URL.Path, "/api/market/quote/")
	ticker, errMsg := validateQuoteTicker(ticker)
	if errMsg != "" {
		WriteError(w, http.StatusBadRequest, errMsg)
		return
	}

	if s.app.QuoteService == nil {
		WriteError(w, http.StatusServiceUnavailable, "Quote service not configured")
		return
	}

	quote, err := s.app.QuoteService.GetRealTimeQuote(r.Context(), ticker)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Quote error: %v", err))
		return
	}

	dataAge := time.Since(quote.Timestamp)
	if dataAge < 0 {
		dataAge = 0
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"quote":            quote,
		"data_age_seconds": int64(dataAge.Seconds()),
		"is_stale":         !common.IsFresh(quote.Timestamp, common.FreshnessRealTimeQuote),
		"source":           quote.Source,
	})
}

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

func (s *Server) handleFilingSummaries(w http.ResponseWriter, r *http.Request, ticker string) {
	if !RequireMethod(w, r, http.MethodGet) {
		return
	}

	ticker, errMsg := validateTicker(ticker)
	if errMsg != "" {
		WriteError(w, http.StatusBadRequest, errMsg)
		return
	}

	marketData, err := s.app.Storage.MarketDataStorage().GetMarketData(r.Context(), ticker)
	if err != nil || marketData == nil {
		WriteError(w, http.StatusNotFound, fmt.Sprintf("No market data for %s", ticker))
		return
	}

	resp := map[string]interface{}{
		"ticker":             ticker,
		"filing_summaries":   marketData.FilingSummaries,
		"quality_assessment": marketData.QualityAssessment,
		"summary_count":      len(marketData.FilingSummaries),
		"last_updated":       marketData.FilingSummariesUpdatedAt,
	}

	WriteJSON(w, http.StatusOK, resp)
}

func (s *Server) handleReadFiling(w http.ResponseWriter, r *http.Request, ticker, documentKey string) {
	if !RequireMethod(w, r, http.MethodGet) {
		return
	}

	ticker, errMsg := validateTicker(ticker)
	if errMsg != "" {
		WriteError(w, http.StatusBadRequest, errMsg)
		return
	}
	if documentKey == "" {
		WriteError(w, http.StatusBadRequest, "document_key is required")
		return
	}

	result, err := s.app.MarketService.ReadFiling(r.Context(), ticker, documentKey)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			WriteError(w, http.StatusNotFound, err.Error())
		} else {
			WriteError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	WriteJSON(w, http.StatusOK, result)
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
	strategy, _ := s.app.StrategyService.GetStrategy(ctx, portfolioName)

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
	strategy, _ := s.app.StrategyService.GetStrategy(ctx, portfolioName)

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
	strategy, _ := s.app.StrategyService.GetStrategy(ctx, portfolioName)

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
	userID := common.ResolveUserID(r.Context())

	records, err := s.app.Storage.UserDataStore().List(r.Context(), userID, "report")
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
	for _, rec := range records {
		if filterName != "" && !strings.EqualFold(rec.Key, filterName) {
			continue
		}
		var report models.PortfolioReport
		if err := json.Unmarshal([]byte(rec.Value), &report); err != nil {
			continue
		}
		result = append(result, reportInfo{
			PortfolioName: rec.Key,
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

	ctx := s.app.InjectNavexaClient(r.Context())

	// Smart caching
	if !req.ForceRefresh {
		existing, err := s.app.ReportService.GetReport(ctx, name)
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
	ctx := s.app.InjectNavexaClient(r.Context())

	switch r.Method {
	case http.MethodGet:
		// Get existing report
		report, err := s.app.ReportService.GetReport(ctx, portfolioName)
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

	ctx := s.app.InjectNavexaClient(r.Context())

	// Try cached report
	report, err := s.app.ReportService.GetReport(ctx, name)
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

	report, err := s.app.ReportService.GetReport(r.Context(), name)
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
			"min_market_cap":     500000000,
			"max_market_cap":     50000000000,
			"max_pe":             25,
			"min_qtr_return_pct": 10,
			"min_dividend_yield": 0.02,
			"max_beta":           1.3,
			"allowed_sectors":    []string{"Technology", "Healthcare"},
			"excluded_sectors":   []string{"Banks", "REITs"},
			"allowed_countries":  []string{"US", "AU"},
			"_description":       "Stock screening filters used by stock_screen, funnel_screen, and strategy_scanner. allowed_countries uses ISO 2-letter codes.",
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

		// Unwrap string-encoded JSON: MCP proxies may send the strategy
		// as a JSON string ("{ ... }") instead of a raw JSON object ({ ... }).
		strategyBytes := []byte(req.StrategyJSON)
		if len(strategyBytes) > 0 && strategyBytes[0] == '"' {
			var unwrapped string
			if err := json.Unmarshal(strategyBytes, &unwrapped); err == nil {
				strategyBytes = []byte(unwrapped)
			}
		}

		// Merge incoming JSON on top
		if err := json.Unmarshal(strategyBytes, existing); err != nil {
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
		var raw struct {
			Items json.RawMessage `json:"items"`
			Notes string          `json:"notes"`
		}
		if !DecodeJSON(w, r, &raw) {
			return
		}
		var plan models.PortfolioPlan
		plan.PortfolioName = name
		plan.Notes = raw.Notes
		if len(raw.Items) > 0 {
			if err := UnmarshalArrayParam(raw.Items, &plan.Items); err != nil {
				WriteError(w, http.StatusBadRequest, "Invalid items: "+err.Error())
				return
			}
		}

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
		var raw struct {
			Items json.RawMessage `json:"items"`
			Notes string          `json:"notes"`
		}
		if !DecodeJSON(w, r, &raw) {
			return
		}
		var wl models.PortfolioWatchlist
		wl.PortfolioName = name
		wl.Notes = raw.Notes
		if len(raw.Items) > 0 {
			if err := UnmarshalArrayParam(raw.Items, &wl.Items); err != nil {
				WriteError(w, http.StatusBadRequest, "Invalid items: "+err.Error())
				return
			}
		}

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

func (s *Server) handleWatchlistReview(w http.ResponseWriter, r *http.Request, name string) {
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

	ctx := s.app.InjectNavexaClient(r.Context())
	review, err := s.app.PortfolioService.ReviewWatchlist(ctx, name, interfaces.ReviewOptions{
		FocusSignals: req.FocusSignals,
		IncludeNews:  req.IncludeNews,
	})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Watchlist review error: %v", err))
		return
	}

	WriteJSON(w, http.StatusOK, review)
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

	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := parseInt(l); err == nil && v > 0 {
			limit = v
		}
	}

	userID := common.ResolveUserID(r.Context())
	records, err := s.app.Storage.UserDataStore().Query(r.Context(), userID, "search", interfaces.QueryOptions{
		Limit:   limit,
		OrderBy: "datetime_desc",
	})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error listing searches: %v", err))
		return
	}

	// Unmarshal and filter
	searchType := r.URL.Query().Get("type")
	exchange := r.URL.Query().Get("exchange")
	var results []models.SearchRecord
	for _, rec := range records {
		var sr models.SearchRecord
		if err := json.Unmarshal([]byte(rec.Value), &sr); err != nil {
			continue
		}
		sr.ID = rec.Key
		if searchType != "" && sr.Type != searchType {
			continue
		}
		if exchange != "" && sr.Exchange != exchange {
			continue
		}
		results = append(results, sr)
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"searches": results,
		"count":    len(results),
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

	userID := common.ResolveUserID(r.Context())
	rec, err := s.app.Storage.UserDataStore().Get(r.Context(), userID, "search", searchID)
	if err != nil {
		WriteError(w, http.StatusNotFound, fmt.Sprintf("Search record not found: %v", err))
		return
	}

	var record models.SearchRecord
	if err := json.Unmarshal([]byte(rec.Value), &record); err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to parse search record: %v", err))
		return
	}
	record.ID = rec.Key

	WriteJSON(w, http.StatusOK, record)
}

// --- External Balance handlers ---

func (s *Server) handleExternalBalances(w http.ResponseWriter, r *http.Request, name string) {
	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		balances, err := s.app.PortfolioService.GetExternalBalances(ctx, name)
		if err != nil {
			WriteError(w, http.StatusNotFound, fmt.Sprintf("Portfolio not found: %v", err))
			return
		}
		total := 0.0
		for _, b := range balances {
			total += b.Value
		}
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"external_balances": balances,
			"total":             total,
		})

	case http.MethodPut:
		var req struct {
			ExternalBalances json.RawMessage `json:"external_balances"`
		}
		if !DecodeJSON(w, r, &req) {
			return
		}
		var balances []models.ExternalBalance
		if err := UnmarshalArrayParam(req.ExternalBalances, &balances); err != nil {
			WriteError(w, http.StatusBadRequest, "Invalid external_balances: "+err.Error())
			return
		}
		portfolio, err := s.app.PortfolioService.SetExternalBalances(ctx, name, balances)
		if err != nil {
			if strings.Contains(err.Error(), "external balance") {
				WriteError(w, http.StatusBadRequest, err.Error())
				return
			}
			WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error setting external balances: %v", err))
			return
		}
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"external_balances": portfolio.ExternalBalances,
			"total":             portfolio.ExternalBalanceTotal,
		})

	case http.MethodPost:
		var balance models.ExternalBalance
		if !DecodeJSON(w, r, &balance) {
			return
		}
		portfolio, err := s.app.PortfolioService.AddExternalBalance(ctx, name, balance)
		if err != nil {
			if strings.Contains(err.Error(), "external balance") {
				WriteError(w, http.StatusBadRequest, err.Error())
				return
			}
			WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error adding external balance: %v", err))
			return
		}
		// Return the newly added balance (last in the list)
		added := portfolio.ExternalBalances[len(portfolio.ExternalBalances)-1]
		WriteJSON(w, http.StatusCreated, added)

	default:
		RequireMethod(w, r, http.MethodGet, http.MethodPut, http.MethodPost)
	}
}

func (s *Server) handleExternalBalanceDelete(w http.ResponseWriter, r *http.Request, name, balanceID string) {
	if !RequireMethod(w, r, http.MethodDelete) {
		return
	}

	_, err := s.app.PortfolioService.RemoveExternalBalance(r.Context(), name, balanceID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error removing external balance: %v", err))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Portfolio indicators handler ---

func (s *Server) handlePortfolioIndicators(w http.ResponseWriter, r *http.Request, name string) {
	if !RequireMethod(w, r, http.MethodGet) {
		return
	}
	ctx := s.app.InjectNavexaClient(r.Context())
	indicators, err := s.app.PortfolioService.GetPortfolioIndicators(ctx, name)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Portfolio indicators error: %v", err))
		return
	}
	WriteJSON(w, http.StatusOK, indicators)
}

// --- Cash flow handlers ---

func (s *Server) handleCashFlows(w http.ResponseWriter, r *http.Request, name string) {
	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		if _, err := s.app.PortfolioService.GetPortfolio(ctx, name); err != nil {
			WriteError(w, http.StatusNotFound, fmt.Sprintf("Portfolio not found: %v", err))
			return
		}
		ledger, err := s.app.CashFlowService.GetLedger(ctx, name)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error getting cash flows: %v", err))
			return
		}
		WriteJSON(w, http.StatusOK, ledger)

	case http.MethodPost:
		var tx models.CashTransaction
		if !DecodeJSON(w, r, &tx) {
			return
		}
		ledger, err := s.app.CashFlowService.AddTransaction(ctx, name, tx)
		if err != nil {
			if strings.Contains(err.Error(), "invalid cash transaction") {
				WriteError(w, http.StatusBadRequest, err.Error())
				return
			}
			WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error adding cash transaction: %v", err))
			return
		}
		WriteJSON(w, http.StatusCreated, ledger)

	default:
		RequireMethod(w, r, http.MethodGet, http.MethodPost)
	}
}

func (s *Server) handleCashFlowItem(w http.ResponseWriter, r *http.Request, name, txID string) {
	ctx := r.Context()

	switch r.Method {
	case http.MethodPut:
		var tx models.CashTransaction
		if !DecodeJSON(w, r, &tx) {
			return
		}
		ledger, err := s.app.CashFlowService.UpdateTransaction(ctx, name, txID, tx)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				WriteError(w, http.StatusNotFound, err.Error())
				return
			}
			if strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "exceeds") {
				WriteError(w, http.StatusBadRequest, err.Error())
				return
			}
			WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error updating cash transaction: %v", err))
			return
		}
		WriteJSON(w, http.StatusOK, ledger)

	case http.MethodDelete:
		_, err := s.app.CashFlowService.RemoveTransaction(ctx, name, txID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				WriteError(w, http.StatusNotFound, err.Error())
				return
			}
			WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error removing cash transaction: %v", err))
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		RequireMethod(w, r, http.MethodPut, http.MethodDelete)
	}
}

func (s *Server) handleCashFlowPerformance(w http.ResponseWriter, r *http.Request, name string) {
	if !RequireMethod(w, r, http.MethodGet) {
		return
	}

	ctx := r.Context()
	if _, err := s.app.PortfolioService.GetPortfolio(ctx, name); err != nil {
		WriteError(w, http.StatusNotFound, fmt.Sprintf("Portfolio not found: %v", err))
		return
	}

	perf, err := s.app.CashFlowService.CalculatePerformance(ctx, name)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error calculating performance: %v", err))
		return
	}
	WriteJSON(w, http.StatusOK, perf)
}

// --- Helper methods ---

func (s *Server) resolvePortfolio(ctx context.Context, requested string) string {
	if requested != "" {
		return requested
	}
	// Check user context for default portfolio (multi-tenant)
	if uc := common.UserContextFromContext(ctx); uc != nil && len(uc.Portfolios) > 0 {
		return uc.Portfolios[0]
	}
	return common.ResolveDefaultPortfolio(ctx, s.app.Storage.InternalStore())
}

// requireNavexaContext validates that the request has both a UserID and NavexaAPIKey
// in the user context. Returns false and writes a 400 error if not.
func (s *Server) requireNavexaContext(w http.ResponseWriter, r *http.Request) bool {
	uc := common.UserContextFromContext(r.Context())
	if uc == nil || strings.TrimSpace(uc.UserID) == "" || strings.TrimSpace(uc.NavexaAPIKey) == "" {
		WriteError(w, http.StatusBadRequest, "configuration not correct")
		return false
	}
	return true
}

// validateQuoteTicker validates a ticker for the real-time quote endpoint.
// Accepts any EODHD ticker format: stocks (BHP.AU), forex (AUDUSD.FOREX),
// commodities (XAUUSD.FOREX). Enforces a character whitelist and requires
// an exchange suffix.
func validateQuoteTicker(ticker string) (string, string) {
	ticker = strings.ToUpper(strings.TrimSpace(ticker))
	if ticker == "" {
		return "", "ticker is required"
	}
	for _, c := range ticker {
		if !((c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '.' || c == '_' || c == '-') {
			return "", fmt.Sprintf("invalid character %q in ticker %q — only A-Z, 0-9, '.', '_', '-' are allowed", string(c), ticker)
		}
	}
	if !strings.Contains(ticker, ".") {
		return "", fmt.Sprintf("ticker %q requires an exchange suffix (e.g., %s.AU, %s.US, %s.FOREX)", ticker, ticker, ticker, ticker)
	}
	return ticker, ""
}

// validateTicker checks a ticker has an exchange suffix and only contains safe characters.
func validateTicker(ticker string) (string, string) {
	ticker = strings.ToUpper(strings.TrimSpace(ticker))
	if ticker == "" {
		return "", "ticker is required"
	}
	for _, c := range ticker {
		if !((c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '.' || c == '_' || c == '-') {
			return "", fmt.Sprintf("invalid character %q in ticker %q", string(c), ticker)
		}
	}
	if !strings.Contains(ticker, ".") {
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
			tickers = append(tickers, h.EODHDTicker())
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

	return s.saveSearchRecord(ctx, record)
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

	return s.saveSearchRecord(ctx, record)
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

	return s.saveSearchRecord(ctx, record)
}

func (s *Server) saveSearchRecord(ctx context.Context, record *models.SearchRecord) string {
	searchID := fmt.Sprintf("%s-%d", record.Type, time.Now().UnixNano())
	record.ID = searchID

	data, err := json.Marshal(record)
	if err != nil {
		s.logger.Warn().Err(err).Msg("Failed to marshal search record")
		return ""
	}

	userID := common.ResolveUserID(ctx)
	if err := s.app.Storage.UserDataStore().Put(ctx, &models.UserRecord{
		UserID:  userID,
		Subject: "search",
		Key:     searchID,
		Value:   string(data),
	}); err != nil {
		s.logger.Warn().Err(err).Msg("Failed to auto-save search")
		return ""
	}
	return searchID
}
