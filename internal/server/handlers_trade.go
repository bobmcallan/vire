package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

// handlePortfolioCreate creates a new manually-managed portfolio.
// POST /api/portfolios
func (s *Server) handlePortfolioCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	var req struct {
		Name       string            `json:"name"`
		SourceType models.SourceType `json:"source_type"`
		Currency   string            `json:"currency"`
	}
	if !DecodeJSON(w, r, &req) {
		return
	}
	ctx := s.app.InjectNavexaClient(r.Context())
	portfolio, err := s.app.PortfolioService.CreatePortfolio(ctx, req.Name, req.SourceType, req.Currency)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "too long") {
			WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error creating portfolio: %v", err))
		return
	}
	WriteJSON(w, http.StatusCreated, portfolio)
}

// handleTrades handles GET/POST /api/portfolios/{name}/trades
func (s *Server) handleTrades(w http.ResponseWriter, r *http.Request, portfolioName string) {
	ctx := s.app.InjectNavexaClient(r.Context())
	switch r.Method {
	case http.MethodGet:
		filter := interfaces.TradeFilter{
			Ticker: r.URL.Query().Get("ticker"),
			Limit:  50,
		}
		if action := r.URL.Query().Get("action"); action != "" {
			filter.Action = models.TradeAction(action)
		}
		if dateFrom := r.URL.Query().Get("date_from"); dateFrom != "" {
			if t, err := time.Parse("2006-01-02", dateFrom); err == nil {
				filter.DateFrom = t
			}
		}
		if dateTo := r.URL.Query().Get("date_to"); dateTo != "" {
			if t, err := time.Parse("2006-01-02", dateTo); err == nil {
				filter.DateTo = t
			}
		}
		if src := r.URL.Query().Get("source_type"); src != "" {
			filter.SourceType = models.SourceType(src)
		}
		if limit := r.URL.Query().Get("limit"); limit != "" {
			if n, err := strconv.Atoi(limit); err == nil && n > 0 {
				filter.Limit = n
			}
		}
		if offset := r.URL.Query().Get("offset"); offset != "" {
			if n, err := strconv.Atoi(offset); err == nil && n >= 0 {
				filter.Offset = n
			}
		}
		trades, total, err := s.app.TradeService.ListTrades(ctx, portfolioName, filter)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error listing trades: %v", err))
			return
		}
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"trades": trades,
			"total":  total,
		})

	case http.MethodPost:
		var req struct {
			Ticker     string  `json:"ticker"`
			Action     string  `json:"action"`
			Units      float64 `json:"units"`
			Price      float64 `json:"price"`
			Fees       float64 `json:"fees"`
			Date       string  `json:"date"`
			SettleDate string  `json:"settle_date"`
			SourceType string  `json:"source_type"`
			SourceRef  string  `json:"source_ref"`
			Notes      string  `json:"notes"`
		}
		if !DecodeJSON(w, r, &req) {
			return
		}
		date, err := time.Parse("2006-01-02", req.Date)
		if err != nil {
			WriteError(w, http.StatusBadRequest, fmt.Sprintf("invalid date format: %v (expected YYYY-MM-DD)", err))
			return
		}
		trade := models.Trade{
			Ticker:     req.Ticker,
			Action:     models.TradeAction(req.Action),
			Units:      req.Units,
			Price:      req.Price,
			Fees:       req.Fees,
			Date:       date,
			SettleDate: req.SettleDate,
			SourceType: models.SourceType(req.SourceType),
			SourceRef:  req.SourceRef,
			Notes:      req.Notes,
		}
		created, holding, err := s.app.TradeService.AddTrade(ctx, portfolioName, trade)
		if err != nil {
			if strings.Contains(err.Error(), "insufficient") || strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "required") {
				WriteError(w, http.StatusBadRequest, err.Error())
				return
			}
			WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error adding trade: %v", err))
			return
		}
		WriteJSON(w, http.StatusCreated, map[string]interface{}{
			"trade":   created,
			"holding": holding,
		})

	default:
		WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleTradeItem handles PUT/DELETE /api/portfolios/{name}/trades/{id}
func (s *Server) handleTradeItem(w http.ResponseWriter, r *http.Request, portfolioName, tradeID string) {
	ctx := s.app.InjectNavexaClient(r.Context())
	switch r.Method {
	case http.MethodPut:
		var req models.Trade
		if !DecodeJSON(w, r, &req) {
			return
		}
		updated, err := s.app.TradeService.UpdateTrade(ctx, portfolioName, tradeID, req)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				WriteError(w, http.StatusNotFound, err.Error())
				return
			}
			WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error updating trade: %v", err))
			return
		}
		WriteJSON(w, http.StatusOK, updated)

	case http.MethodDelete:
		tb, err := s.app.TradeService.RemoveTrade(ctx, portfolioName, tradeID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				WriteError(w, http.StatusNotFound, err.Error())
				return
			}
			WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error removing trade: %v", err))
			return
		}
		WriteJSON(w, http.StatusOK, tb)

	default:
		WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handlePortfolioSnapshotImport handles POST /api/portfolios/{name}/snapshot for bulk position imports.
func (s *Server) handlePortfolioSnapshotImport(w http.ResponseWriter, r *http.Request, portfolioName string) {
	var req struct {
		Positions    []models.SnapshotPosition `json:"positions"`
		Mode         string                    `json:"mode"` // "replace" (default) or "merge"
		SourceRef    string                    `json:"source_ref"`
		SnapshotDate string                    `json:"snapshot_date"` // YYYY-MM-DD, default today
	}
	if !DecodeJSON(w, r, &req) {
		return
	}
	if len(req.Positions) == 0 {
		WriteError(w, http.StatusBadRequest, "positions array is required and cannot be empty")
		return
	}
	if req.Mode == "" {
		req.Mode = "replace"
	}
	if req.Mode != "replace" && req.Mode != "merge" {
		WriteError(w, http.StatusBadRequest, fmt.Sprintf("invalid mode: %q (valid: replace, merge)", req.Mode))
		return
	}

	ctx := s.app.InjectNavexaClient(r.Context())

	// Auto-create portfolio if it doesn't exist
	if _, err := s.app.PortfolioService.GetPortfolio(ctx, portfolioName); err != nil {
		if _, createErr := s.app.PortfolioService.CreatePortfolio(ctx, portfolioName, models.SourceSnapshot, "AUD"); createErr != nil {
			if !strings.Contains(createErr.Error(), "already exists") {
				WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error creating portfolio: %v", createErr))
				return
			}
		}
	}

	tb, err := s.app.TradeService.SnapshotPositions(ctx, portfolioName, req.Positions, req.Mode, req.SourceRef, req.SnapshotDate)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error importing snapshot: %v", err))
		return
	}

	WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"portfolio_name": portfolioName,
		"positions":      len(tb.SnapshotPositions),
		"mode":           req.Mode,
	})
}
