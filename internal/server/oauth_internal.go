package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/bobmcallan/vire/internal/models"
)

// routeInternalOAuth dispatches /api/internal/oauth/* to the appropriate handler.
func (s *Server) routeInternalOAuth(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/internal/oauth/")

	switch {
	// Sessions
	case path == "sessions":
		s.handleInternalOAuthSessions(w, r)
	case path == "sessions/purge":
		s.handleInternalOAuthSessionPurge(w, r)
	case strings.HasPrefix(path, "sessions/"):
		id := strings.TrimPrefix(path, "sessions/")
		s.handleInternalOAuthSessionByID(w, r, id)

	// Clients
	case path == "clients":
		s.handleInternalOAuthClients(w, r)
	case strings.HasPrefix(path, "clients/"):
		id := strings.TrimPrefix(path, "clients/")
		s.handleInternalOAuthClientByID(w, r, id)

	// Codes
	case strings.HasPrefix(path, "codes/") && strings.HasSuffix(path, "/used"):
		code := strings.TrimPrefix(path, "codes/")
		code = strings.TrimSuffix(code, "/used")
		s.handleInternalOAuthCodeMarkUsed(w, r, code)
	case path == "codes":
		s.handleInternalOAuthCodes(w, r)
	case strings.HasPrefix(path, "codes/"):
		code := strings.TrimPrefix(path, "codes/")
		s.handleInternalOAuthCodeByCode(w, r, code)

	// Tokens
	case path == "tokens/lookup":
		s.handleInternalOAuthTokenLookup(w, r)
	case path == "tokens/revoke":
		s.handleInternalOAuthTokenRevoke(w, r)
	case path == "tokens/purge":
		s.handleInternalOAuthTokenPurge(w, r)
	case path == "tokens":
		s.handleInternalOAuthTokenSave(w, r)

	default:
		WriteError(w, http.StatusNotFound, "Not found")
	}
}

// --- Sessions ---

func (s *Server) handleInternalOAuthSessions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	store := s.app.Storage.OAuthStore()

	switch r.Method {
	case http.MethodPost:
		var body models.OAuthSession
		if !DecodeJSON(w, r, &body) {
			return
		}
		if body.SessionID == "" || body.ClientID == "" {
			WriteError(w, http.StatusBadRequest, "session_id and client_id are required")
			return
		}
		if body.CreatedAt.IsZero() {
			body.CreatedAt = time.Now()
		}
		if err := store.SaveSession(ctx, &body); err != nil {
			WriteError(w, http.StatusInternalServerError, "Failed to save session: "+err.Error())
			return
		}
		WriteJSON(w, http.StatusCreated, body)

	case http.MethodGet:
		clientID := r.URL.Query().Get("client_id")
		if clientID == "" {
			WriteError(w, http.StatusBadRequest, "client_id query parameter is required")
			return
		}
		sess, err := store.GetSessionByClientID(ctx, clientID)
		if err != nil {
			WriteError(w, http.StatusNotFound, "Session not found")
			return
		}
		WriteJSON(w, http.StatusOK, sess)

	default:
		w.Header().Set("Allow", "GET, POST")
		WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (s *Server) handleInternalOAuthSessionByID(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()
	store := s.app.Storage.OAuthStore()

	switch r.Method {
	case http.MethodGet:
		sess, err := store.GetSession(ctx, id)
		if err != nil {
			WriteError(w, http.StatusNotFound, "Session not found")
			return
		}
		WriteJSON(w, http.StatusOK, sess)

	case http.MethodPatch:
		var body struct {
			UserID string `json:"user_id"`
		}
		if !DecodeJSON(w, r, &body) {
			return
		}
		if body.UserID == "" {
			WriteError(w, http.StatusBadRequest, "user_id is required")
			return
		}
		if err := store.UpdateSessionUserID(ctx, id, body.UserID); err != nil {
			WriteError(w, http.StatusInternalServerError, "Failed to update session: "+err.Error())
			return
		}
		WriteJSON(w, http.StatusOK, map[string]string{"status": "updated"})

	case http.MethodDelete:
		if err := store.DeleteSession(ctx, id); err != nil {
			WriteError(w, http.StatusInternalServerError, "Failed to delete session: "+err.Error())
			return
		}
		WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

	default:
		w.Header().Set("Allow", "GET, PATCH, DELETE")
		WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// --- Clients ---

func (s *Server) handleInternalOAuthClients(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodPost) {
		return
	}
	ctx := r.Context()
	store := s.app.Storage.OAuthStore()

	var body models.OAuthClient
	if !DecodeJSON(w, r, &body) {
		return
	}
	if body.ClientID == "" {
		WriteError(w, http.StatusBadRequest, "client_id is required")
		return
	}
	if body.CreatedAt.IsZero() {
		body.CreatedAt = time.Now()
	}
	if err := store.SaveClient(ctx, &body); err != nil {
		WriteError(w, http.StatusInternalServerError, "Failed to save client: "+err.Error())
		return
	}
	WriteJSON(w, http.StatusCreated, body)
}

func (s *Server) handleInternalOAuthClientByID(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()
	store := s.app.Storage.OAuthStore()

	switch r.Method {
	case http.MethodGet:
		client, err := store.GetClient(ctx, id)
		if err != nil {
			WriteError(w, http.StatusNotFound, "Client not found")
			return
		}
		WriteJSON(w, http.StatusOK, client)

	case http.MethodDelete:
		if err := store.DeleteClient(ctx, id); err != nil {
			WriteError(w, http.StatusInternalServerError, "Failed to delete client: "+err.Error())
			return
		}
		WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

	default:
		w.Header().Set("Allow", "GET, DELETE")
		WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// --- Codes ---

func (s *Server) handleInternalOAuthCodes(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodPost) {
		return
	}
	ctx := r.Context()
	store := s.app.Storage.OAuthStore()

	var body models.OAuthCode
	if !DecodeJSON(w, r, &body) {
		return
	}
	if body.Code == "" || body.ClientID == "" {
		WriteError(w, http.StatusBadRequest, "code and client_id are required")
		return
	}
	if body.CreatedAt.IsZero() {
		body.CreatedAt = time.Now()
	}
	if err := store.SaveCode(ctx, &body); err != nil {
		WriteError(w, http.StatusInternalServerError, "Failed to save code: "+err.Error())
		return
	}
	WriteJSON(w, http.StatusCreated, body)
}

func (s *Server) handleInternalOAuthCodeByCode(w http.ResponseWriter, r *http.Request, code string) {
	if !RequireMethod(w, r, http.MethodGet) {
		return
	}
	ctx := r.Context()
	store := s.app.Storage.OAuthStore()

	c, err := store.GetCode(ctx, code)
	if err != nil {
		WriteError(w, http.StatusNotFound, "Code not found")
		return
	}
	WriteJSON(w, http.StatusOK, c)
}

func (s *Server) handleInternalOAuthCodeMarkUsed(w http.ResponseWriter, r *http.Request, code string) {
	if !RequireMethod(w, r, http.MethodPatch) {
		return
	}
	ctx := r.Context()
	store := s.app.Storage.OAuthStore()

	if err := store.MarkCodeUsed(ctx, code); err != nil {
		WriteError(w, http.StatusInternalServerError, "Failed to mark code used: "+err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, map[string]string{"status": "used"})
}

// --- Tokens ---

func (s *Server) handleInternalOAuthTokenSave(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodPost) {
		return
	}
	ctx := r.Context()
	store := s.app.Storage.OAuthStore()

	var body struct {
		Token     string    `json:"token"`
		ClientID  string    `json:"client_id"`
		UserID    string    `json:"user_id"`
		Scope     string    `json:"scope"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if !DecodeJSON(w, r, &body) {
		return
	}
	if body.Token == "" || body.ClientID == "" {
		WriteError(w, http.StatusBadRequest, "token and client_id are required")
		return
	}

	hash := hashRefreshToken(body.Token)
	token := &models.OAuthRefreshToken{
		TokenHash: hash,
		ClientID:  body.ClientID,
		UserID:    body.UserID,
		Scope:     body.Scope,
		ExpiresAt: body.ExpiresAt,
		CreatedAt: time.Now(),
	}
	if err := store.SaveRefreshToken(ctx, token); err != nil {
		WriteError(w, http.StatusInternalServerError, "Failed to save token: "+err.Error())
		return
	}
	WriteJSON(w, http.StatusCreated, map[string]string{"status": "saved"})
}

func (s *Server) handleInternalOAuthTokenLookup(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodPost) {
		return
	}
	ctx := r.Context()
	store := s.app.Storage.OAuthStore()

	var body struct {
		Token string `json:"token"`
	}
	if !DecodeJSON(w, r, &body) {
		return
	}
	if body.Token == "" {
		WriteError(w, http.StatusBadRequest, "token is required")
		return
	}

	hash := hashRefreshToken(body.Token)
	token, err := store.GetRefreshToken(ctx, hash)
	if err != nil {
		WriteError(w, http.StatusNotFound, "Token not found")
		return
	}
	WriteJSON(w, http.StatusOK, token)
}

func (s *Server) handleInternalOAuthTokenRevoke(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodPost) {
		return
	}
	ctx := r.Context()
	store := s.app.Storage.OAuthStore()

	var body struct {
		Token string `json:"token"`
	}
	if !DecodeJSON(w, r, &body) {
		return
	}
	if body.Token == "" {
		WriteError(w, http.StatusBadRequest, "token is required")
		return
	}

	hash := hashRefreshToken(body.Token)
	if err := store.RevokeRefreshToken(ctx, hash); err != nil {
		WriteError(w, http.StatusInternalServerError, "Failed to revoke token: "+err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

func (s *Server) handleInternalOAuthTokenPurge(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodPost) {
		return
	}
	ctx := r.Context()
	store := s.app.Storage.OAuthStore()

	count, err := store.PurgeExpiredTokens(ctx)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "Failed to purge tokens: "+err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, map[string]int{"purged": count})
}

// --- Session Purge ---

func (s *Server) handleInternalOAuthSessionPurge(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodPost) {
		return
	}
	ctx := r.Context()
	store := s.app.Storage.OAuthStore()

	count, err := store.PurgeExpiredSessions(ctx)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "Failed to purge sessions: "+err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, map[string]int{"purged": count})
}
