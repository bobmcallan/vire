package server

import (
	"encoding/json"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

// handleFeedbackRoot dispatches GET /api/feedback (list) and POST /api/feedback (submit).
func (s *Server) handleFeedbackRoot(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleFeedbackList(w, r)
	case http.MethodPost:
		s.handleFeedbackSubmit(w, r)
	default:
		w.Header().Set("Allow", "GET, POST")
		WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// routeFeedback dispatches /api/feedback/{sub} to the appropriate handler.
func (s *Server) routeFeedback(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/feedback/")
	if path == "" {
		s.handleFeedbackRoot(w, r)
		return
	}

	switch path {
	case "summary":
		s.handleFeedbackSummary(w, r)
	case "bulk":
		s.handleFeedbackBulkUpdate(w, r)
	default:
		// Treat as feedback ID
		switch r.Method {
		case http.MethodGet:
			s.handleFeedbackGet(w, r, path)
		case http.MethodPatch:
			s.handleFeedbackUpdate(w, r, path)
		case http.MethodDelete:
			s.handleFeedbackDelete(w, r, path)
		default:
			w.Header().Set("Allow", "GET, PATCH, DELETE")
			WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
	}
}

// handleFeedbackSubmit handles POST /api/feedback.
func (s *Server) handleFeedbackSubmit(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SessionID     string      `json:"session_id"`
		ClientType    string      `json:"client_type"`
		Category      string      `json:"category"`
		Severity      string      `json:"severity"`
		Description   string      `json:"description"`
		Ticker        string      `json:"ticker"`
		PortfolioName string      `json:"portfolio_name"`
		ToolName      string      `json:"tool_name"`
		ObservedValue interface{} `json:"observed_value"`
		ExpectedValue interface{} `json:"expected_value"`
	}
	if !DecodeJSON(w, r, &body) {
		return
	}

	// Validate required fields
	if body.Category == "" {
		WriteError(w, http.StatusBadRequest, "category is required")
		return
	}
	if !models.ValidFeedbackCategories[body.Category] {
		WriteError(w, http.StatusBadRequest, "invalid category: must be one of data_anomaly, sync_delay, calculation_error, missing_data, schema_change, tool_error, observation")
		return
	}
	if strings.TrimSpace(body.Description) == "" {
		WriteError(w, http.StatusBadRequest, "description is required")
		return
	}
	if body.Severity != "" && !models.ValidFeedbackSeverities[body.Severity] {
		WriteError(w, http.StatusBadRequest, "invalid severity: must be one of low, medium, high")
		return
	}

	fb := &models.Feedback{
		SessionID:     body.SessionID,
		ClientType:    body.ClientType,
		Category:      body.Category,
		Severity:      body.Severity,
		Description:   strings.TrimSpace(body.Description),
		Ticker:        body.Ticker,
		PortfolioName: body.PortfolioName,
		ToolName:      body.ToolName,
	}

	// Marshal observed/expected values to json.RawMessage
	if body.ObservedValue != nil {
		fb.ObservedValue = marshalRaw(body.ObservedValue)
	}
	if body.ExpectedValue != nil {
		fb.ExpectedValue = marshalRaw(body.ExpectedValue)
	}

	ctx := r.Context()
	store := s.app.Storage.FeedbackStore()

	if err := store.Create(ctx, fb); err != nil {
		WriteError(w, http.StatusInternalServerError, "Failed to store feedback: "+err.Error())
		return
	}

	WriteJSON(w, http.StatusAccepted, map[string]interface{}{
		"accepted":    true,
		"feedback_id": fb.ID,
	})
}

// handleFeedbackList handles GET /api/feedback.
func (s *Server) handleFeedbackList(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodGet) {
		return
	}

	q := r.URL.Query()
	opts := interfaces.FeedbackListOptions{
		Status:        q.Get("status"),
		Severity:      q.Get("severity"),
		Category:      q.Get("category"),
		Ticker:        q.Get("ticker"),
		PortfolioName: q.Get("portfolio_name"),
		SessionID:     q.Get("session_id"),
		Sort:          q.Get("sort"),
	}

	if since := q.Get("since"); since != "" {
		if t, err := time.Parse(time.RFC3339, since); err == nil {
			opts.Since = &t
		}
	}
	if before := q.Get("before"); before != "" {
		if t, err := time.Parse(time.RFC3339, before); err == nil {
			opts.Before = &t
		}
	}

	opts.Page = 1
	if p := q.Get("page"); p != "" {
		if v, err := parseInt(p); err == nil && v > 0 {
			opts.Page = v
		}
	}
	opts.PerPage = 20
	if pp := q.Get("per_page"); pp != "" {
		if v, err := parseInt(pp); err == nil && v > 0 && v <= 100 {
			opts.PerPage = v
		}
	}

	ctx := r.Context()
	store := s.app.Storage.FeedbackStore()

	items, total, err := store.List(ctx, opts)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "Failed to list feedback: "+err.Error())
		return
	}

	pages := int(math.Ceil(float64(total) / float64(opts.PerPage)))

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"items":    items,
		"total":    total,
		"page":     opts.Page,
		"per_page": opts.PerPage,
		"pages":    pages,
	})
}

// handleFeedbackSummary handles GET /api/feedback/summary.
func (s *Server) handleFeedbackSummary(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodGet) {
		return
	}

	ctx := r.Context()
	store := s.app.Storage.FeedbackStore()

	summary, err := store.Summary(ctx)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "Failed to get feedback summary: "+err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, summary)
}

// handleFeedbackGet handles GET /api/feedback/{id}.
func (s *Server) handleFeedbackGet(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()
	store := s.app.Storage.FeedbackStore()

	fb, err := store.Get(ctx, id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "Failed to get feedback: "+err.Error())
		return
	}
	if fb == nil {
		WriteError(w, http.StatusNotFound, "Feedback not found")
		return
	}

	WriteJSON(w, http.StatusOK, fb)
}

// handleFeedbackUpdate handles PATCH /api/feedback/{id}.
func (s *Server) handleFeedbackUpdate(w http.ResponseWriter, r *http.Request, id string) {
	if !s.requireAdmin(w, r) {
		return
	}

	var body struct {
		Status          string `json:"status"`
		ResolutionNotes string `json:"resolution_notes"`
	}
	if !DecodeJSON(w, r, &body) {
		return
	}

	if body.Status != "" && !models.ValidFeedbackStatuses[body.Status] {
		WriteError(w, http.StatusBadRequest, "invalid status: must be one of new, acknowledged, resolved, dismissed")
		return
	}

	ctx := r.Context()
	store := s.app.Storage.FeedbackStore()

	// Verify it exists
	existing, err := store.Get(ctx, id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "Failed to get feedback: "+err.Error())
		return
	}
	if existing == nil {
		WriteError(w, http.StatusNotFound, "Feedback not found")
		return
	}

	status := body.Status
	if status == "" {
		status = existing.Status
	}

	if err := store.Update(ctx, id, status, body.ResolutionNotes); err != nil {
		WriteError(w, http.StatusInternalServerError, "Failed to update feedback: "+err.Error())
		return
	}

	// Fetch updated record
	updated, err := store.Get(ctx, id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "Failed to get updated feedback: "+err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, updated)
}

// handleFeedbackBulkUpdate handles PATCH /api/feedback/bulk.
func (s *Server) handleFeedbackBulkUpdate(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodPatch) {
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}

	var body struct {
		IDs             []string `json:"ids"`
		Status          string   `json:"status"`
		ResolutionNotes string   `json:"resolution_notes"`
	}
	if !DecodeJSON(w, r, &body) {
		return
	}

	if len(body.IDs) == 0 {
		WriteError(w, http.StatusBadRequest, "ids is required")
		return
	}
	if body.Status == "" {
		WriteError(w, http.StatusBadRequest, "status is required")
		return
	}
	if !models.ValidFeedbackStatuses[body.Status] {
		WriteError(w, http.StatusBadRequest, "invalid status: must be one of new, acknowledged, resolved, dismissed")
		return
	}

	ctx := r.Context()
	store := s.app.Storage.FeedbackStore()

	updated, err := store.BulkUpdateStatus(ctx, body.IDs, body.Status, body.ResolutionNotes)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "Failed to bulk update feedback: "+err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"updated": updated,
	})
}

// handleFeedbackDelete handles DELETE /api/feedback/{id}.
func (s *Server) handleFeedbackDelete(w http.ResponseWriter, r *http.Request, id string) {
	if !s.requireAdmin(w, r) {
		return
	}

	ctx := r.Context()
	store := s.app.Storage.FeedbackStore()

	if err := store.Delete(ctx, id); err != nil {
		WriteError(w, http.StatusInternalServerError, "Failed to delete feedback: "+err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// marshalRaw converts an interface{} value to json.RawMessage.
func marshalRaw(v interface{}) json.RawMessage {
	if v == nil {
		return nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return data
}
