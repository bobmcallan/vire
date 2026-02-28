package server

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"time"

	"github.com/bobmcallan/vire/internal/models"
)

// handleServiceRegister handles POST /api/services/register — register a service user.
func (s *Server) handleServiceRegister(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodPost) {
		return
	}

	var req struct {
		ServiceID   string `json:"service_id"`
		ServiceKey  string `json:"service_key"`
		ServiceType string `json:"service_type"`
	}
	if !DecodeJSON(w, r, &req) {
		return
	}

	// Validate: server key configured
	serverKey := s.app.Config.Auth.ServiceKey
	if serverKey == "" {
		WriteError(w, http.StatusNotImplemented, "service registration not configured")
		return
	}

	// Validate: service_id non-empty
	if req.ServiceID == "" {
		WriteError(w, http.StatusBadRequest, "service_id is required")
		return
	}

	// Validate: key length >= 32 (both sides)
	if len(serverKey) < 32 || len(req.ServiceKey) < 32 {
		WriteError(w, http.StatusBadRequest, "service_key must be at least 32 characters")
		return
	}

	// Validate: key match (constant-time comparison to prevent timing attacks)
	if subtle.ConstantTimeCompare([]byte(req.ServiceKey), []byte(serverKey)) != 1 {
		WriteError(w, http.StatusForbidden, "invalid service key")
		return
	}

	ctx := r.Context()
	store := s.app.Storage.InternalStore()
	now := time.Now()

	serviceUserID := "service:" + req.ServiceID

	// Idempotent: check if user exists, only update ModifiedAt
	existing, err := store.GetUser(ctx, serviceUserID)
	if err == nil {
		existing.ModifiedAt = now
		if err := store.SaveUser(ctx, existing); err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to update service user: "+err.Error())
			return
		}
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"status":          "ok",
			"service_user_id": serviceUserID,
			"registered_at":   existing.CreatedAt.Format(time.RFC3339),
		})
		return
	}

	// Create new service user
	user := &models.InternalUser{
		UserID:     serviceUserID,
		Email:      fmt.Sprintf("%s@service.vire.local", req.ServiceID),
		Name:       fmt.Sprintf("Service: %s", req.ServiceID),
		Provider:   "service",
		Role:       models.RoleService,
		CreatedAt:  now,
		ModifiedAt: now,
	}

	if err := store.SaveUser(ctx, user); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to create service user: "+err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status":          "ok",
		"service_user_id": serviceUserID,
		"registered_at":   now.Format(time.RFC3339),
	})
}

// handleServiceTidy handles POST /api/admin/services/tidy — purge stale service users.
func (s *Server) handleServiceTidy(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodPost) {
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}

	ctx := r.Context()
	store := s.app.Storage.InternalStore()

	ids, err := store.ListUsers(ctx)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to list users: "+err.Error())
		return
	}

	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	purged := 0
	remaining := 0

	for _, id := range ids {
		u, err := store.GetUser(ctx, id)
		if err != nil {
			continue
		}
		if u.Provider != "service" {
			continue
		}
		if u.ModifiedAt.Before(cutoff) {
			if err := store.DeleteUser(ctx, id); err == nil {
				purged++
			}
		} else {
			remaining++
		}
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"purged":    purged,
		"remaining": remaining,
	})
}
