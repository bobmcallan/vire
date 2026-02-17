package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"

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
		Username        string   `json:"username"`
		Email           string   `json:"email"`
		Password        string   `json:"password"`
		Role            string   `json:"role"`
		NavexaKey       string   `json:"navexa_key"`
		DisplayCurrency string   `json:"display_currency"`
		Portfolios      []string `json:"portfolios"`
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
	store := s.app.Storage.InternalStore()

	// Check if user already exists
	if _, err := store.GetUser(ctx, req.Username); err == nil {
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

	user := &models.InternalUser{
		UserID:       req.Username,
		Email:        req.Email,
		PasswordHash: string(hash),
		Role:         req.Role,
		CreatedAt:    time.Now(),
	}

	if err := store.SaveUser(ctx, user); err != nil {
		s.logger.Error().Err(err).Str("username", req.Username).Msg("Failed to save user")
		WriteError(w, http.StatusInternalServerError, "failed to save user")
		return
	}

	// Save preferences as UserKV entries
	if req.NavexaKey != "" {
		store.SetUserKV(ctx, req.Username, "navexa_key", req.NavexaKey)
	}
	if req.DisplayCurrency != "" {
		store.SetUserKV(ctx, req.Username, "display_currency", req.DisplayCurrency)
	}
	if len(req.Portfolios) > 0 {
		store.SetUserKV(ctx, req.Username, "portfolios", strings.Join(req.Portfolios, ","))
	}

	WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"status": "ok",
		"data": map[string]interface{}{
			"username": user.UserID,
			"email":    user.Email,
			"role":     user.Role,
		},
	})
}

// handleUserUpsert handles POST /api/users/upsert — create or update a user.
// If the user exists, updates the provided fields (merge semantics).
// If the user does not exist, creates a new user (password required).
func (s *Server) handleUserUpsert(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodPost) {
		return
	}

	var req struct {
		Username        string   `json:"username"`
		Email           string   `json:"email"`
		Password        string   `json:"password"`
		Role            string   `json:"role"`
		NavexaKey       string   `json:"navexa_key"`
		DisplayCurrency string   `json:"display_currency"`
		Portfolios      []string `json:"portfolios"`
	}
	if !DecodeJSON(w, r, &req) {
		return
	}

	if errMsg := validateUsername(req.Username); errMsg != "" {
		WriteError(w, http.StatusBadRequest, errMsg)
		return
	}

	ctx := r.Context()
	store := s.app.Storage.InternalStore()

	user, err := store.GetUser(ctx, req.Username)
	isNew := err != nil

	if isNew {
		// Create: password is required
		if req.Password == "" {
			WriteError(w, http.StatusBadRequest, "password is required for new users")
			return
		}

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

		user = &models.InternalUser{
			UserID:       req.Username,
			Email:        req.Email,
			PasswordHash: string(hash),
			Role:         req.Role,
			CreatedAt:    time.Now(),
		}
	} else {
		// Update: merge provided fields
		if req.Email != "" {
			user.Email = req.Email
		}
		if req.Role != "" {
			user.Role = req.Role
		}
		if req.Password != "" {
			passwordBytes := []byte(req.Password)
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
	}

	if err := store.SaveUser(ctx, user); err != nil {
		s.logger.Error().Err(err).Str("username", req.Username).Msg("Failed to save user")
		WriteError(w, http.StatusInternalServerError, "failed to save user")
		return
	}

	// Save preferences as UserKV entries
	if req.NavexaKey != "" {
		store.SetUserKV(ctx, req.Username, "navexa_key", req.NavexaKey)
	}
	if req.DisplayCurrency != "" {
		store.SetUserKV(ctx, req.Username, "display_currency", req.DisplayCurrency)
	}
	if len(req.Portfolios) > 0 {
		store.SetUserKV(ctx, req.Username, "portfolios", strings.Join(req.Portfolios, ","))
	}

	kvs, _ := store.ListUserKV(ctx, req.Username)

	status := http.StatusOK
	if isNew {
		status = http.StatusCreated
	}

	WriteJSON(w, status, map[string]interface{}{
		"status": "ok",
		"data":   userResponse(user, kvs),
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
	ctx := r.Context()
	store := s.app.Storage.InternalStore()

	user, err := store.GetUser(ctx, username)
	if err != nil {
		WriteError(w, http.StatusNotFound, fmt.Sprintf("user '%s' not found", username))
		return
	}

	kvs, _ := store.ListUserKV(ctx, username)

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
		"data":   userResponse(user, kvs),
	})
}

// handleUserUpdate handles PUT /api/users/{id}.
func (s *Server) handleUserUpdate(w http.ResponseWriter, r *http.Request, username string) {
	var req struct {
		Email           *string   `json:"email"`
		Role            *string   `json:"role"`
		NavexaKey       *string   `json:"navexa_key"`
		Password        *string   `json:"password"`
		DisplayCurrency *string   `json:"display_currency"`
		Portfolios      *[]string `json:"portfolios"`
	}
	if !DecodeJSON(w, r, &req) {
		return
	}

	ctx := r.Context()
	store := s.app.Storage.InternalStore()

	user, err := store.GetUser(ctx, username)
	if err != nil {
		WriteError(w, http.StatusNotFound, fmt.Sprintf("user '%s' not found", username))
		return
	}

	// Update InternalUser fields
	if req.Email != nil {
		user.Email = *req.Email
	}
	if req.Role != nil {
		user.Role = *req.Role
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

	if err := store.SaveUser(ctx, user); err != nil {
		s.logger.Error().Err(err).Str("username", username).Msg("Failed to save user")
		WriteError(w, http.StatusInternalServerError, "failed to save user")
		return
	}

	// Update UserKV preferences
	if req.NavexaKey != nil {
		store.SetUserKV(ctx, username, "navexa_key", *req.NavexaKey)
	}
	if req.DisplayCurrency != nil {
		store.SetUserKV(ctx, username, "display_currency", *req.DisplayCurrency)
	}
	if req.Portfolios != nil {
		store.SetUserKV(ctx, username, "portfolios", strings.Join(*req.Portfolios, ","))
	}

	kvs, _ := store.ListUserKV(ctx, username)

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
		"data":   userResponse(user, kvs),
	})
}

// handleUserDelete handles DELETE /api/users/{id}.
func (s *Server) handleUserDelete(w http.ResponseWriter, r *http.Request, username string) {
	ctx := r.Context()
	store := s.app.Storage.InternalStore()

	if _, err := store.GetUser(ctx, username); err != nil {
		WriteError(w, http.StatusNotFound, fmt.Sprintf("user '%s' not found", username))
		return
	}

	if err := store.DeleteUser(ctx, username); err != nil {
		s.logger.Error().Err(err).Str("username", username).Msg("Failed to delete user")
		WriteError(w, http.StatusInternalServerError, "failed to delete user")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
	})
}

// handleUsernameCheck handles GET /api/users/check/{username} — check if a username is available.
func (s *Server) handleUsernameCheck(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodGet) {
		return
	}

	username := strings.TrimPrefix(r.URL.Path, "/api/users/check/")
	if username == "" {
		WriteError(w, http.StatusBadRequest, "username is required in path")
		return
	}

	ctx := r.Context()
	store := s.app.Storage.InternalStore()

	_, err := store.GetUser(ctx, username)
	available := err != nil

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
		"data": map[string]interface{}{
			"username":  username,
			"available": available,
		},
	})
}

// handlePasswordReset handles POST /api/auth/password-reset — reset a user's password.
func (s *Server) handlePasswordReset(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodPost) {
		return
	}

	var req struct {
		Username    string `json:"username"`
		NewPassword string `json:"new_password"`
	}
	if !DecodeJSON(w, r, &req) {
		return
	}

	if req.Username == "" {
		WriteError(w, http.StatusBadRequest, "username is required")
		return
	}
	if req.NewPassword == "" {
		WriteError(w, http.StatusBadRequest, "new_password is required")
		return
	}

	ctx := r.Context()
	store := s.app.Storage.InternalStore()

	user, err := store.GetUser(ctx, req.Username)
	if err != nil {
		WriteError(w, http.StatusNotFound, fmt.Sprintf("user '%s' not found", req.Username))
		return
	}

	passwordBytes := []byte(req.NewPassword)
	if len(passwordBytes) > 72 {
		passwordBytes = passwordBytes[:72]
	}
	hash, err := bcrypt.GenerateFromPassword(passwordBytes, 10)
	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to hash password")
		WriteError(w, http.StatusInternalServerError, "failed to reset password")
		return
	}
	user.PasswordHash = string(hash)

	if err := store.SaveUser(ctx, user); err != nil {
		s.logger.Error().Err(err).Str("username", req.Username).Msg("Failed to save user during password reset")
		WriteError(w, http.StatusInternalServerError, "failed to reset password")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
		"data": map[string]interface{}{
			"username": user.UserID,
			"message":  "password reset successfully",
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
	store := s.app.Storage.InternalStore()

	user, err := store.GetUser(ctx, req.Username)
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

	kvs, _ := store.ListUserKV(ctx, req.Username)
	kvMap := kvToMap(kvs)

	// Sign JWT for the authenticated user
	token, err := signJWT(user, "email", &s.app.Config.Auth)
	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to sign JWT for login")
		WriteError(w, http.StatusInternalServerError, "failed to sign token")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
		"data": map[string]interface{}{
			"token": token,
			"user": map[string]interface{}{
				"username":         user.UserID,
				"email":            user.Email,
				"role":             user.Role,
				"navexa_key_set":   kvMap["navexa_key"] != "",
				"display_currency": kvMap["display_currency"],
				"portfolios":       splitCSV(kvMap["portfolios"]),
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

// userResponse builds a safe response from InternalUser + UserKV entries.
func userResponse(user *models.InternalUser, kvs []*models.UserKeyValue) map[string]interface{} {
	kvMap := kvToMap(kvs)
	resp := map[string]interface{}{
		"username":           user.UserID,
		"email":              user.Email,
		"role":               user.Role,
		"navexa_key_set":     kvMap["navexa_key"] != "",
		"navexa_key_preview": navexaKeyPreview(kvMap["navexa_key"]),
		"display_currency":   kvMap["display_currency"],
		"portfolios":         splitCSV(kvMap["portfolios"]),
	}
	return resp
}

func kvToMap(kvs []*models.UserKeyValue) map[string]string {
	m := make(map[string]string)
	for _, kv := range kvs {
		m[kv.Key] = kv.Value
	}
	return m
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}
