package server

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bobmcallan/vire/internal/models"
)

// requireAdmin checks that the user has admin role. Returns false if not admin.
func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	// Check for admin role from JWT claims or user context
	// For now, check X-Vire-User-ID and look up role
	userID := r.Header.Get("X-Vire-User-ID")
	if userID == "" {
		WriteError(w, http.StatusUnauthorized, "Authentication required")
		return false
	}

	user, err := s.app.Storage.InternalStore().GetUser(r.Context(), userID)
	if err != nil {
		WriteError(w, http.StatusUnauthorized, "User not found")
		return false
	}

	if user.Role != "admin" {
		WriteError(w, http.StatusForbidden, "Admin access required")
		return false
	}

	return true
}

// handleAdminJobs handles GET /api/admin/jobs — list jobs with optional filters.
func (s *Server) handleAdminJobs(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodGet) {
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}

	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 1000 {
			limit = v
		}
	}

	ctx := r.Context()
	store := s.app.Storage.JobQueueStore()

	ticker := r.URL.Query().Get("ticker")
	if ticker != "" {
		jobs, err := store.ListByTicker(ctx, ticker)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "Failed to list jobs: "+err.Error())
			return
		}
		WriteJSON(w, http.StatusOK, map[string]interface{}{"jobs": jobs})
		return
	}

	status := r.URL.Query().Get("status")
	if status == "pending" {
		jobs, err := store.ListPending(ctx, limit)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "Failed to list pending jobs: "+err.Error())
			return
		}
		WriteJSON(w, http.StatusOK, map[string]interface{}{"jobs": jobs})
		return
	}

	jobs, err := store.ListAll(ctx, limit)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "Failed to list jobs: "+err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, map[string]interface{}{"jobs": jobs})
}

// handleAdminJobQueue handles GET /api/admin/jobs/queue — pending jobs ordered by priority.
func (s *Server) handleAdminJobQueue(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodGet) {
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}

	ctx := r.Context()
	store := s.app.Storage.JobQueueStore()

	jobs, err := store.ListPending(ctx, 100)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "Failed to list queue: "+err.Error())
		return
	}

	pending, _ := store.CountPending(ctx)
	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"jobs":    jobs,
		"pending": pending,
	})
}

// handleAdminJobPriority handles PUT /api/admin/jobs/{id}/priority.
func (s *Server) handleAdminJobPriority(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodPut) {
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}

	// Extract job ID from path: /api/admin/jobs/{id}/priority
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/jobs/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 || parts[1] != "priority" {
		WriteError(w, http.StatusNotFound, "Not found")
		return
	}
	jobID := parts[0]

	var body struct {
		Priority interface{} `json:"priority"` // int or "top"
	}
	if !DecodeJSON(w, r, &body) {
		return
	}

	ctx := r.Context()

	switch v := body.Priority.(type) {
	case string:
		if v == "top" {
			if s.app.JobManager == nil {
				WriteError(w, http.StatusServiceUnavailable, "Job manager not running")
				return
			}
			if err := s.app.JobManager.PushToTop(ctx, jobID); err != nil {
				WriteError(w, http.StatusInternalServerError, "Failed to push to top: "+err.Error())
				return
			}
		} else {
			WriteError(w, http.StatusBadRequest, "Invalid priority: use a number or \"top\"")
			return
		}
	case float64:
		store := s.app.Storage.JobQueueStore()
		if err := store.SetPriority(ctx, jobID, int(v)); err != nil {
			WriteError(w, http.StatusInternalServerError, "Failed to set priority: "+err.Error())
			return
		}
	default:
		WriteError(w, http.StatusBadRequest, "Invalid priority: use a number or \"top\"")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleAdminJobCancel handles POST /api/admin/jobs/{id}/cancel.
func (s *Server) handleAdminJobCancel(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodPost) {
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}

	// Extract job ID from path: /api/admin/jobs/{id}/cancel
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/jobs/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 || parts[1] != "cancel" {
		WriteError(w, http.StatusNotFound, "Not found")
		return
	}
	jobID := parts[0]

	ctx := r.Context()
	store := s.app.Storage.JobQueueStore()

	if err := store.Cancel(ctx, jobID); err != nil {
		WriteError(w, http.StatusInternalServerError, "Failed to cancel job: "+err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

// handleAdminJobEnqueue handles POST /api/admin/jobs/enqueue — manually enqueue a job.
func (s *Server) handleAdminJobEnqueue(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodPost) {
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}

	var body struct {
		JobType  string `json:"job_type"`
		Ticker   string `json:"ticker"`
		Priority int    `json:"priority"`
	}
	if !DecodeJSON(w, r, &body) {
		return
	}

	if body.JobType == "" || body.Ticker == "" {
		WriteError(w, http.StatusBadRequest, "job_type and ticker are required")
		return
	}

	if body.Priority == 0 {
		body.Priority = models.DefaultPriority(body.JobType)
	}

	job := &models.Job{
		JobType:     body.JobType,
		Ticker:      body.Ticker,
		Priority:    body.Priority,
		Status:      models.JobStatusPending,
		CreatedAt:   time.Now(),
		MaxAttempts: 3,
	}

	ctx := r.Context()
	store := s.app.Storage.JobQueueStore()

	if err := store.Enqueue(ctx, job); err != nil {
		WriteError(w, http.StatusInternalServerError, "Failed to enqueue job: "+err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{"job": job})
}

// handleAdminStockIndex handles GET/POST /api/admin/stock-index.
func (s *Server) handleAdminStockIndex(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}

	ctx := r.Context()
	store := s.app.Storage.StockIndexStore()

	switch r.Method {
	case http.MethodGet:
		entries, err := store.List(ctx)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "Failed to list stock index: "+err.Error())
			return
		}
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"entries": entries,
			"count":   len(entries),
		})

	case http.MethodPost:
		var body struct {
			Ticker   string `json:"ticker"`
			Code     string `json:"code"`
			Exchange string `json:"exchange"`
			Name     string `json:"name"`
		}
		if !DecodeJSON(w, r, &body) {
			return
		}

		if body.Ticker == "" {
			WriteError(w, http.StatusBadRequest, "ticker is required")
			return
		}

		entry := &models.StockIndexEntry{
			Ticker:   body.Ticker,
			Code:     body.Code,
			Exchange: body.Exchange,
			Name:     body.Name,
			Source:   "manual",
		}

		if err := store.Upsert(ctx, entry); err != nil {
			WriteError(w, http.StatusInternalServerError, "Failed to add to stock index: "+err.Error())
			return
		}

		WriteJSON(w, http.StatusOK, map[string]interface{}{"entry": entry})

	default:
		w.Header().Set("Allow", "GET, POST")
		WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleAdminJobsWS handles GET /api/admin/ws/jobs — WebSocket upgrade.
func (s *Server) handleAdminJobsWS(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodGet) {
		return
	}

	// Admin check for WebSocket — must authenticate before upgrade
	userID := r.Header.Get("X-Vire-User-ID")
	if userID == "" {
		WriteError(w, http.StatusUnauthorized, "Authentication required")
		return
	}
	user, err := s.app.Storage.InternalStore().GetUser(r.Context(), userID)
	if err != nil || user.Role != "admin" {
		WriteError(w, http.StatusForbidden, "Admin access required")
		return
	}

	if s.app.JobManager == nil {
		WriteError(w, http.StatusServiceUnavailable, "Job manager not running")
		return
	}

	s.app.JobManager.Hub().ServeWS(w, r)
}
