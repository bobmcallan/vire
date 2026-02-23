package server

import (
	"net/http"

	"github.com/bobmcallan/vire/internal/models"
)

// handleScan handles POST /api/scan
func (s *Server) handleScan(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodPost) {
		return
	}

	var query models.ScanQuery
	if !DecodeJSON(w, r, &query) {
		return
	}

	resp, err := s.app.MarketService.ScanMarket(r.Context(), query)
	if err != nil {
		// Validation errors
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, resp)
}

// handleScanFields handles GET /api/scan/fields
func (s *Server) handleScanFields(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodGet) {
		return
	}

	resp := s.app.MarketService.ScanFields()
	WriteJSON(w, http.StatusOK, resp)
}
