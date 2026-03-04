package server

import (
	"math"
	"net/http"
	"strings"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

// handleChangelogRoot dispatches GET /api/changelog (list) and POST /api/changelog (create).
func (s *Server) handleChangelogRoot(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleChangelogList(w, r)
	case http.MethodPost:
		s.handleChangelogCreate(w, r)
	default:
		w.Header().Set("Allow", "GET, POST")
		WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// routeChangelog dispatches /api/changelog/{id} to the appropriate handler.
func (s *Server) routeChangelog(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/changelog/")
	if id == "" {
		s.handleChangelogRoot(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleChangelogGet(w, r, id)
	case http.MethodPatch:
		s.handleChangelogUpdate(w, r, id)
	case http.MethodDelete:
		s.handleChangelogDelete(w, r, id)
	default:
		w.Header().Set("Allow", "GET, PATCH, DELETE")
		WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleChangelogCreate handles POST /api/changelog.
func (s *Server) handleChangelogCreate(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminOrService(w, r) {
		return
	}

	var body struct {
		Service        string `json:"service"`
		ServiceVersion string `json:"service_version"`
		ServiceBuild   string `json:"service_build"`
		Content        string `json:"content"`
	}
	if !DecodeJSON(w, r, &body) {
		return
	}

	// Validate required fields
	if strings.TrimSpace(body.Service) == "" {
		WriteError(w, http.StatusBadRequest, "service is required")
		return
	}
	if len(body.Service) > 100 {
		WriteError(w, http.StatusBadRequest, "service must be 100 characters or less")
		return
	}
	if strings.TrimSpace(body.Content) == "" {
		WriteError(w, http.StatusBadRequest, "content is required")
		return
	}
	if len(body.Content) > 50000 {
		WriteError(w, http.StatusBadRequest, "content must be 50000 characters or less")
		return
	}
	if len(body.ServiceVersion) > 50 {
		WriteError(w, http.StatusBadRequest, "service_version must be 50 characters or less")
		return
	}
	if len(body.ServiceBuild) > 50 {
		WriteError(w, http.StatusBadRequest, "service_build must be 50 characters or less")
		return
	}

	entry := &models.ChangelogEntry{
		Service:        strings.TrimSpace(body.Service),
		ServiceVersion: body.ServiceVersion,
		ServiceBuild:   body.ServiceBuild,
		Content:        body.Content,
	}

	ctx := r.Context()

	// Capture creator identity from UserContext
	if uc := common.UserContextFromContext(ctx); uc != nil && strings.TrimSpace(uc.UserID) != "" {
		entry.CreatedByID = strings.TrimSpace(uc.UserID)
		if user, err := s.app.Storage.InternalStore().GetUser(ctx, entry.CreatedByID); err == nil && user != nil {
			entry.CreatedByName = user.Name
		}
	}

	store := s.app.Storage.ChangelogStore()

	if err := store.Create(ctx, entry); err != nil {
		WriteError(w, http.StatusInternalServerError, "Failed to create changelog entry: "+err.Error())
		return
	}

	WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"id":    entry.ID,
		"entry": entry,
	})
}

// handleChangelogList handles GET /api/changelog.
func (s *Server) handleChangelogList(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodGet) {
		return
	}

	q := r.URL.Query()

	opts := interfaces.ChangelogListOptions{
		Service: q.Get("service"),
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
	store := s.app.Storage.ChangelogStore()

	items, total, err := store.List(ctx, opts)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "Failed to list changelog entries: "+err.Error())
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

// handleChangelogGet handles GET /api/changelog/{id}.
func (s *Server) handleChangelogGet(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()
	store := s.app.Storage.ChangelogStore()

	entry, err := store.Get(ctx, id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "Failed to get changelog entry: "+err.Error())
		return
	}
	if entry == nil {
		WriteError(w, http.StatusNotFound, "Changelog entry not found")
		return
	}

	WriteJSON(w, http.StatusOK, entry)
}

// handleChangelogUpdate handles PATCH /api/changelog/{id}.
func (s *Server) handleChangelogUpdate(w http.ResponseWriter, r *http.Request, id string) {
	if !s.requireAdmin(w, r) {
		return
	}

	var body struct {
		Service        string `json:"service"`
		ServiceVersion string `json:"service_version"`
		ServiceBuild   string `json:"service_build"`
		Content        string `json:"content"`
	}
	if !DecodeJSON(w, r, &body) {
		return
	}

	// Validate optional field lengths
	if len(body.Service) > 100 {
		WriteError(w, http.StatusBadRequest, "service must be 100 characters or less")
		return
	}
	if len(body.Content) > 50000 {
		WriteError(w, http.StatusBadRequest, "content must be 50000 characters or less")
		return
	}
	if len(body.ServiceVersion) > 50 {
		WriteError(w, http.StatusBadRequest, "service_version must be 50 characters or less")
		return
	}
	if len(body.ServiceBuild) > 50 {
		WriteError(w, http.StatusBadRequest, "service_build must be 50 characters or less")
		return
	}

	ctx := r.Context()
	store := s.app.Storage.ChangelogStore()

	// Verify entry exists
	existing, err := store.Get(ctx, id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "Failed to get changelog entry: "+err.Error())
		return
	}
	if existing == nil {
		WriteError(w, http.StatusNotFound, "Changelog entry not found")
		return
	}

	entry := &models.ChangelogEntry{
		ID:             id,
		Service:        body.Service,
		ServiceVersion: body.ServiceVersion,
		ServiceBuild:   body.ServiceBuild,
		Content:        body.Content,
	}

	// Capture updater identity
	if uc := common.UserContextFromContext(ctx); uc != nil && strings.TrimSpace(uc.UserID) != "" {
		entry.UpdatedByID = strings.TrimSpace(uc.UserID)
		if user, err := s.app.Storage.InternalStore().GetUser(ctx, entry.UpdatedByID); err == nil && user != nil {
			entry.UpdatedByName = user.Name
		}
	}

	if err := store.Update(ctx, entry); err != nil {
		WriteError(w, http.StatusInternalServerError, "Failed to update changelog entry: "+err.Error())
		return
	}

	// Fetch updated record
	updated, err := store.Get(ctx, id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "Failed to get updated changelog entry: "+err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, updated)
}

// handleChangelogDelete handles DELETE /api/changelog/{id}.
func (s *Server) handleChangelogDelete(w http.ResponseWriter, r *http.Request, id string) {
	if !s.requireAdmin(w, r) {
		return
	}

	ctx := r.Context()
	store := s.app.Storage.ChangelogStore()

	if err := store.Delete(ctx, id); err != nil {
		WriteError(w, http.StatusInternalServerError, "Failed to delete changelog entry: "+err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
