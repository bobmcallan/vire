package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/bobmcallan/vire/internal/models"
	"golang.org/x/crypto/bcrypt"
)

// --- User handlers ---

// validateUsername checks that a username is safe for storage.
// Rejects empty, too long, null bytes, and control characters.
func validateUsername(username string) string {
	if username == "" {
		return "username is required"
	}
	if len(username) > 128 {
		return "username must be 128 characters or fewer"
	}
	for _, c := range username {
		if c < 0x20 || c == 0x7f {
			return "username contains invalid control characters"
		}
	}
	return ""
}

// handleUserCreate handles POST /api/users — create a new user.
func (s *Server) handleUserCreate(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodPost) {
		return
	}

	var req struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if !DecodeJSON(w, r, &req) {
		return
	}

	if errMsg := validateUsername(req.Username); errMsg != "" {
		WriteError(w, http.StatusBadRequest, errMsg)
		return
	}
	if req.Password == "" {
		WriteError(w, http.StatusBadRequest, "password is required")
		return
	}

	ctx := r.Context()
	userStore := s.app.Storage.UserStorage()

	// Check if user already exists
	if _, err := userStore.GetUser(ctx, req.Username); err == nil {
		WriteError(w, http.StatusConflict, fmt.Sprintf("user '%s' already exists", req.Username))
		return
	}

	// Hash password with bcrypt (truncate to 72 bytes like portal does)
	passwordBytes := []byte(req.Password)
	if len(passwordBytes) > 72 {
		passwordBytes = passwordBytes[:72]
	}
	hash, err := bcrypt.GenerateFromPassword(passwordBytes, 10)
	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to hash password")
		WriteError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	user := &models.User{
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: string(hash),
		Role:         req.Role,
	}

	if err := userStore.SaveUser(ctx, user); err != nil {
		s.logger.Error().Err(err).Str("username", req.Username).Msg("Failed to save user")
		WriteError(w, http.StatusInternalServerError, "failed to save user")
		return
	}

	WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"status": "ok",
		"data": map[string]interface{}{
			"username": user.Username,
			"email":    user.Email,
			"role":     user.Role,
		},
	})
}

// routeUsers dispatches GET/PUT/DELETE for /api/users/{id}.
func (s *Server) routeUsers(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimPrefix(r.URL.Path, "/api/users/")
	if username == "" {
		WriteError(w, http.StatusBadRequest, "username is required in path")
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleUserGet(w, r, username)
	case http.MethodPut:
		s.handleUserUpdate(w, r, username)
	case http.MethodDelete:
		s.handleUserDelete(w, r, username)
	default:
		RequireMethod(w, r, http.MethodGet, http.MethodPut, http.MethodDelete)
	}
}

// handleUserGet handles GET /api/users/{id}.
func (s *Server) handleUserGet(w http.ResponseWriter, r *http.Request, username string) {
	user, err := s.app.Storage.UserStorage().GetUser(r.Context(), username)
	if err != nil {
		WriteError(w, http.StatusNotFound, fmt.Sprintf("user '%s' not found", username))
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
		"data":   userResponse(user),
	})
}

// handleUserUpdate handles PUT /api/users/{id}.
func (s *Server) handleUserUpdate(w http.ResponseWriter, r *http.Request, username string) {
	var req struct {
		Email            *string   `json:"email"`
		Role             *string   `json:"role"`
		NavexaKey        *string   `json:"navexa_key"`
		Password         *string   `json:"password"`
		DisplayCurrency  *string   `json:"display_currency"`
		DefaultPortfolio *string   `json:"default_portfolio"`
		Portfolios       *[]string `json:"portfolios"`
	}
	if !DecodeJSON(w, r, &req) {
		return
	}

	ctx := r.Context()
	userStore := s.app.Storage.UserStorage()

	user, err := userStore.GetUser(ctx, username)
	if err != nil {
		WriteError(w, http.StatusNotFound, fmt.Sprintf("user '%s' not found", username))
		return
	}

	// Merge only provided fields
	if req.Email != nil {
		user.Email = *req.Email
	}
	if req.Role != nil {
		user.Role = *req.Role
	}
	if req.NavexaKey != nil {
		user.NavexaKey = *req.NavexaKey
	}
	if req.DisplayCurrency != nil {
		user.DisplayCurrency = *req.DisplayCurrency
	}
	if req.DefaultPortfolio != nil {
		user.DefaultPortfolio = *req.DefaultPortfolio
	}
	if req.Portfolios != nil {
		user.Portfolios = *req.Portfolios
	}
	if req.Password != nil {
		passwordBytes := []byte(*req.Password)
		if len(passwordBytes) > 72 {
			passwordBytes = passwordBytes[:72]
		}
		hash, err := bcrypt.GenerateFromPassword(passwordBytes, 10)
		if err != nil {
			s.logger.Error().Err(err).Msg("Failed to hash password")
			WriteError(w, http.StatusInternalServerError, "failed to update user")
			return
		}
		user.PasswordHash = string(hash)
	}

	if err := userStore.SaveUser(ctx, user); err != nil {
		s.logger.Error().Err(err).Str("username", username).Msg("Failed to save user")
		WriteError(w, http.StatusInternalServerError, "failed to save user")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
		"data":   userResponse(user),
	})
}

// handleUserDelete handles DELETE /api/users/{id}.
func (s *Server) handleUserDelete(w http.ResponseWriter, r *http.Request, username string) {
	ctx := r.Context()
	userStore := s.app.Storage.UserStorage()

	if _, err := userStore.GetUser(ctx, username); err != nil {
		WriteError(w, http.StatusNotFound, fmt.Sprintf("user '%s' not found", username))
		return
	}

	if err := userStore.DeleteUser(ctx, username); err != nil {
		s.logger.Error().Err(err).Str("username", username).Msg("Failed to delete user")
		WriteError(w, http.StatusInternalServerError, "failed to delete user")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
	})
}

// handleUserImport handles POST /api/users/import — bulk import users.
func (s *Server) handleUserImport(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodPost) {
		return
	}

	var req struct {
		Users []struct {
			Username         string   `json:"username"`
			Email            string   `json:"email"`
			Password         string   `json:"password"`
			Role             string   `json:"role"`
			NavexaKey        string   `json:"navexa_key"`
			DisplayCurrency  string   `json:"display_currency"`
			DefaultPortfolio string   `json:"default_portfolio"`
			Portfolios       []string `json:"portfolios"`
		} `json:"users"`
	}
	if !DecodeJSON(w, r, &req) {
		return
	}

	ctx := r.Context()
	userStore := s.app.Storage.UserStorage()

	imported := 0
	skipped := 0

	for _, u := range req.Users {
		// Skip users with invalid usernames
		if validateUsername(u.Username) != "" {
			skipped++
			continue
		}

		// Skip if user already exists
		if _, err := userStore.GetUser(ctx, u.Username); err == nil {
			skipped++
			continue
		}

		passwordBytes := []byte(u.Password)
		if len(passwordBytes) > 72 {
			passwordBytes = passwordBytes[:72]
		}
		hash, err := bcrypt.GenerateFromPassword(passwordBytes, 10)
		if err != nil {
			s.logger.Warn().Err(err).Str("username", u.Username).Msg("Failed to hash password during import")
			skipped++
			continue
		}

		user := &models.User{
			Username:         u.Username,
			Email:            u.Email,
			PasswordHash:     string(hash),
			Role:             u.Role,
			NavexaKey:        u.NavexaKey,
			DisplayCurrency:  u.DisplayCurrency,
			DefaultPortfolio: u.DefaultPortfolio,
			Portfolios:       u.Portfolios,
		}

		if err := userStore.SaveUser(ctx, user); err != nil {
			s.logger.Warn().Err(err).Str("username", u.Username).Msg("Failed to save user during import")
			skipped++
			continue
		}
		imported++
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
		"data": map[string]interface{}{
			"imported": imported,
			"skipped":  skipped,
		},
	})
}

// handleAuthLogin handles POST /api/auth/login — authenticate a user.
func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodPost) {
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !DecodeJSON(w, r, &req) {
		return
	}

	ctx := r.Context()
	user, err := s.app.Storage.UserStorage().GetUser(ctx, req.Username)
	if err != nil {
		WriteError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	passwordBytes := []byte(req.Password)
	if len(passwordBytes) > 72 {
		passwordBytes = passwordBytes[:72]
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), passwordBytes); err != nil {
		WriteError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
		"data": map[string]interface{}{
			"user": map[string]interface{}{
				"username":          user.Username,
				"email":             user.Email,
				"role":              user.Role,
				"navexa_key_set":    user.NavexaKey != "",
				"display_currency":  user.DisplayCurrency,
				"default_portfolio": user.DefaultPortfolio,
				"portfolios":        user.Portfolios,
			},
		},
	})
}

// navexaKeyPreview returns "****" + last 4 chars, or "" if empty.
func navexaKeyPreview(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 4 {
		return "****"
	}
	return "****" + key[len(key)-4:]
}

// userResponse builds a safe response without secrets.
func userResponse(user *models.User) map[string]interface{} {
	resp := map[string]interface{}{
		"username":           user.Username,
		"email":              user.Email,
		"role":               user.Role,
		"navexa_key_set":     user.NavexaKey != "",
		"navexa_key_preview": navexaKeyPreview(user.NavexaKey),
		"display_currency":   user.DisplayCurrency,
		"default_portfolio":  user.DefaultPortfolio,
		"portfolios":         user.Portfolios,
	}
	return resp
}
