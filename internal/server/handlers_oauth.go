package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/bobmcallan/vire/internal/models"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// --- Well-Known Metadata Endpoints ---

// handleOAuthProtectedResource handles GET /.well-known/oauth-protected-resource (RFC 9728).
func (s *Server) handleOAuthProtectedResource(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodGet) {
		return
	}
	issuer := s.app.Config.Auth.OAuth2.Issuer
	if issuer == "" {
		WriteError(w, http.StatusNotFound, "OAuth 2.1 not configured")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"resource":                 issuer,
		"authorization_servers":    []string{issuer},
		"bearer_methods_supported": []string{"header"},
	})
}

// handleOAuthAuthorizationServer handles GET /.well-known/oauth-authorization-server (RFC 8414).
func (s *Server) handleOAuthAuthorizationServer(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodGet) {
		return
	}
	issuer := s.app.Config.Auth.OAuth2.Issuer
	if issuer == "" {
		WriteError(w, http.StatusNotFound, "OAuth 2.1 not configured")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"issuer":                                issuer,
		"authorization_endpoint":                issuer + "/oauth/authorize",
		"token_endpoint":                        issuer + "/oauth/token",
		"registration_endpoint":                 issuer + "/oauth/register",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_post"},
		"scopes_supported":                      []string{"vire"},
	})
}

// --- Dynamic Client Registration (RFC 7591) ---

// handleOAuthRegister handles POST /oauth/register — register a new OAuth client.
func (s *Server) handleOAuthRegister(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodPost) {
		return
	}
	issuer := s.app.Config.Auth.OAuth2.Issuer
	if issuer == "" {
		WriteError(w, http.StatusNotFound, "OAuth 2.1 not configured")
		return
	}

	var req struct {
		ClientName   string   `json:"client_name"`
		RedirectURIs []string `json:"redirect_uris"`
	}
	if !DecodeJSON(w, r, &req) {
		return
	}
	if req.ClientName == "" {
		WriteError(w, http.StatusBadRequest, "client_name is required")
		return
	}
	if len(req.ClientName) > 200 {
		WriteError(w, http.StatusBadRequest, "client_name must not exceed 200 characters")
		return
	}
	if len(req.RedirectURIs) == 0 {
		WriteError(w, http.StatusBadRequest, "redirect_uris is required and must contain at least one URI")
		return
	}
	if len(req.RedirectURIs) > 10 {
		WriteError(w, http.StatusBadRequest, "redirect_uris must not contain more than 10 URIs")
		return
	}
	for _, uri := range req.RedirectURIs {
		if uri == "" {
			WriteError(w, http.StatusBadRequest, "redirect_uris must not contain empty strings")
			return
		}
		u, err := url.Parse(uri)
		if err != nil || u.Host == "" {
			WriteError(w, http.StatusBadRequest, fmt.Sprintf("invalid redirect_uri: %s", uri))
			return
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			WriteError(w, http.StatusBadRequest, fmt.Sprintf("redirect_uri must use http or https scheme: %s", uri))
			return
		}
	}

	clientID := uuid.New().String()
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		s.logger.Error().Err(err).Msg("Failed to generate client secret")
		WriteError(w, http.StatusInternalServerError, "failed to generate credentials")
		return
	}
	clientSecret := hex.EncodeToString(secretBytes)

	hash, err := bcrypt.GenerateFromPassword([]byte(clientSecret), bcrypt.DefaultCost)
	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to hash client secret")
		WriteError(w, http.StatusInternalServerError, "failed to generate credentials")
		return
	}

	client := &models.OAuthClient{
		ClientID:         clientID,
		ClientSecretHash: string(hash),
		ClientName:       req.ClientName,
		RedirectURIs:     req.RedirectURIs,
		CreatedAt:        time.Now(),
	}

	if err := s.app.Storage.OAuthStore().SaveClient(r.Context(), client); err != nil {
		s.logger.Error().Err(err).Msg("Failed to save OAuth client")
		WriteError(w, http.StatusInternalServerError, "failed to register client")
		return
	}

	WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"client_id":     clientID,
		"client_secret": clientSecret,
		"client_name":   req.ClientName,
		"redirect_uris": req.RedirectURIs,
	})
}

// --- Authorization Endpoint ---

// handleOAuthAuthorize handles GET and POST /oauth/authorize.
func (s *Server) handleOAuthAuthorize(w http.ResponseWriter, r *http.Request) {
	issuer := s.app.Config.Auth.OAuth2.Issuer
	if issuer == "" {
		WriteError(w, http.StatusNotFound, "OAuth 2.1 not configured")
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleOAuthAuthorizeGET(w, r)
	case http.MethodPost:
		s.handleOAuthAuthorizePOST(w, r)
	default:
		RequireMethod(w, r, http.MethodGet, http.MethodPost)
	}
}

func (s *Server) handleOAuthAuthorizeGET(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	clientID := q.Get("client_id")
	redirectURI := q.Get("redirect_uri")
	responseType := q.Get("response_type")
	codeChallenge := q.Get("code_challenge")
	codeChallengeMethod := q.Get("code_challenge_method")
	state := q.Get("state")
	scope := q.Get("scope")

	// Phase 1: Validate client and redirect_uri. Per OAuth 2.1, if the client_id
	// is unknown or redirect_uri is not registered, we MUST NOT redirect — display
	// the error directly instead.
	client, redirectVerified := s.verifyClientAndRedirect(w, r, clientID, redirectURI)
	if client == nil {
		return // error already written
	}

	// Handle deny: if the user clicked "Deny", redirect with access_denied
	if q.Get("deny") == "true" && redirectVerified {
		s.redirectWithError(w, r, redirectURI, "access_denied", "The user denied the request", state)
		return
	}

	// Phase 2: Validate remaining params. Since the client and redirect_uri are
	// verified, we can safely redirect errors to the redirect_uri per OAuth spec.
	errMsg := s.validateAuthorizeParams(responseType, codeChallenge, codeChallengeMethod, state)
	if errMsg != "" {
		if redirectVerified {
			s.redirectWithError(w, r, redirectURI, "invalid_request", errMsg, state)
			return
		}
		WriteError(w, http.StatusBadRequest, errMsg)
		return
	}

	renderConsentPage(w, oauthConsentData{
		ClientName:          client.ClientName,
		ClientID:            clientID,
		RedirectURI:         redirectURI,
		State:               state,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
		Scope:               scope,
		ResponseType:        responseType,
	})
}

func (s *Server) handleOAuthAuthorizePOST(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid form data")
		return
	}

	clientID := r.FormValue("client_id")
	redirectURI := r.FormValue("redirect_uri")
	responseType := r.FormValue("response_type")
	codeChallenge := r.FormValue("code_challenge")
	codeChallengeMethod := r.FormValue("code_challenge_method")
	state := r.FormValue("state")
	scope := r.FormValue("scope")
	email := r.FormValue("email")
	password := r.FormValue("password")

	client, _ := s.verifyClientAndRedirect(w, r, clientID, redirectURI)
	if client == nil {
		return
	}

	errMsg := s.validateAuthorizeParams(responseType, codeChallenge, codeChallengeMethod, state)
	if errMsg != "" {
		WriteError(w, http.StatusBadRequest, errMsg)
		return
	}

	// Normalize scope: only "vire" is accepted; empty defaults to "vire"
	scope = normalizeScope(scope)

	// Validate credentials
	if email == "" || password == "" {
		renderConsentPage(w, oauthConsentData{
			ClientName:          client.ClientName,
			ClientID:            clientID,
			RedirectURI:         redirectURI,
			State:               state,
			CodeChallenge:       codeChallenge,
			CodeChallengeMethod: codeChallengeMethod,
			Scope:               scope,
			ResponseType:        responseType,
			Error:               "Email and password are required",
		})
		return
	}

	store := s.app.Storage.InternalStore()
	user, err := store.GetUserByEmail(r.Context(), email)
	if err != nil {
		renderConsentPage(w, oauthConsentData{
			ClientName:          client.ClientName,
			ClientID:            clientID,
			RedirectURI:         redirectURI,
			State:               state,
			CodeChallenge:       codeChallenge,
			CodeChallengeMethod: codeChallengeMethod,
			Scope:               scope,
			ResponseType:        responseType,
			Error:               "Invalid email or password",
		})
		return
	}

	passwordBytes := []byte(password)
	if len(passwordBytes) > 72 {
		passwordBytes = passwordBytes[:72]
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), passwordBytes); err != nil {
		renderConsentPage(w, oauthConsentData{
			ClientName:          client.ClientName,
			ClientID:            clientID,
			RedirectURI:         redirectURI,
			State:               state,
			CodeChallenge:       codeChallenge,
			CodeChallengeMethod: codeChallengeMethod,
			Scope:               scope,
			ResponseType:        responseType,
			Error:               "Invalid email or password",
		})
		return
	}

	// Generate authorization code
	codeBytes := make([]byte, 32)
	if _, err := rand.Read(codeBytes); err != nil {
		s.logger.Error().Err(err).Msg("Failed to generate auth code")
		WriteError(w, http.StatusInternalServerError, "failed to generate authorization code")
		return
	}
	code := hex.EncodeToString(codeBytes)

	oauthCode := &models.OAuthCode{
		Code:                code,
		ClientID:            clientID,
		UserID:              user.UserID,
		RedirectURI:         redirectURI,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
		Scope:               scope,
		ExpiresAt:           time.Now().Add(s.app.Config.Auth.OAuth2.GetCodeExpiry()),
		Used:                false,
		CreatedAt:           time.Now(),
	}

	if err := s.app.Storage.OAuthStore().SaveCode(r.Context(), oauthCode); err != nil {
		s.logger.Error().Err(err).Msg("Failed to save auth code")
		WriteError(w, http.StatusInternalServerError, "failed to generate authorization code")
		return
	}

	// Redirect back to client with code
	u, _ := url.Parse(redirectURI)
	q := u.Query()
	q.Set("code", code)
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

// validateAuthorizeParams checks the required OAuth authorize parameters.
// verifyClientAndRedirect validates the client_id and redirect_uri. Per OAuth 2.1 spec,
// if the client is unknown or the redirect_uri is not registered, we MUST NOT redirect
// to the redirect_uri — the error is displayed directly. Returns (client, redirectVerified).
// If client is nil, an error response has already been written.
func (s *Server) verifyClientAndRedirect(w http.ResponseWriter, r *http.Request, clientID, redirectURI string) (*models.OAuthClient, bool) {
	if clientID == "" {
		WriteError(w, http.StatusBadRequest, "client_id is required")
		return nil, false
	}
	if redirectURI == "" {
		WriteError(w, http.StatusBadRequest, "redirect_uri is required")
		return nil, false
	}
	client, err := s.app.Storage.OAuthStore().GetClient(r.Context(), clientID)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "unknown client_id")
		return nil, false
	}
	uriMatch := false
	for _, uri := range client.RedirectURIs {
		if uri == redirectURI {
			uriMatch = true
			break
		}
	}
	if !uriMatch {
		WriteError(w, http.StatusBadRequest, "redirect_uri does not match any registered URIs")
		return nil, false
	}
	return client, true
}

// redirectWithError redirects to the redirect_uri with error parameters per OAuth 2.1 spec.
func (s *Server) redirectWithError(w http.ResponseWriter, r *http.Request, redirectURI, errCode, errDescription, state string) {
	u, err := url.Parse(redirectURI)
	if err != nil {
		WriteError(w, http.StatusBadRequest, errDescription)
		return
	}
	q := u.Query()
	q.Set("error", errCode)
	q.Set("error_description", errDescription)
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

// validateAuthorizeParams validates the non-client/redirect authorize parameters.
func (s *Server) validateAuthorizeParams(responseType, codeChallenge, codeChallengeMethod, state string) string {
	if responseType != "code" {
		return "response_type must be 'code'"
	}
	if codeChallenge == "" {
		return "code_challenge is required (PKCE is mandatory)"
	}
	if codeChallengeMethod != "S256" {
		return "code_challenge_method must be 'S256'"
	}
	if state == "" {
		return "state is required"
	}
	return ""
}

// normalizeScope validates and normalizes the OAuth scope. Only "vire" is accepted;
// empty or unrecognized values default to "vire".
func normalizeScope(scope string) string {
	if scope == "" || scope == "vire" {
		return "vire"
	}
	// Reject anything other than "vire" — default to "vire"
	return "vire"
}

// --- Token Endpoint ---

// handleOAuthToken handles POST /oauth/token — exchange code or refresh token for tokens.
func (s *Server) handleOAuthToken(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodPost) {
		return
	}
	issuer := s.app.Config.Auth.OAuth2.Issuer
	if issuer == "" {
		WriteError(w, http.StatusNotFound, "OAuth 2.1 not configured")
		return
	}

	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "invalid form data")
		return
	}

	grantType := r.FormValue("grant_type")
	switch grantType {
	case "authorization_code":
		s.handleOAuthTokenAuthCode(w, r)
	case "refresh_token":
		s.handleOAuthTokenRefresh(w, r)
	default:
		writeOAuthError(w, http.StatusBadRequest, "unsupported_grant_type", "grant_type must be 'authorization_code' or 'refresh_token'")
	}
}

func (s *Server) handleOAuthTokenAuthCode(w http.ResponseWriter, r *http.Request) {
	code := r.FormValue("code")
	clientID := r.FormValue("client_id")
	clientSecret := r.FormValue("client_secret")
	redirectURI := r.FormValue("redirect_uri")
	codeVerifier := r.FormValue("code_verifier")

	if code == "" || clientID == "" || clientSecret == "" || redirectURI == "" || codeVerifier == "" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "code, client_id, client_secret, redirect_uri, and code_verifier are all required")
		return
	}

	ctx := r.Context()
	oauthStore := s.app.Storage.OAuthStore()

	// Validate client
	client, err := oauthStore.GetClient(ctx, clientID)
	if err != nil {
		writeOAuthError(w, http.StatusUnauthorized, "invalid_client", "unknown client_id")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(client.ClientSecretHash), []byte(clientSecret)); err != nil {
		writeOAuthError(w, http.StatusUnauthorized, "invalid_client", "invalid client_secret")
		return
	}

	// Validate code
	oauthCode, err := oauthStore.GetCode(ctx, code)
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "invalid authorization code")
		return
	}
	if oauthCode.Used {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "authorization code already used")
		return
	}
	if time.Now().After(oauthCode.ExpiresAt) {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "authorization code expired")
		return
	}
	if oauthCode.ClientID != clientID {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "client_id mismatch")
		return
	}
	if oauthCode.RedirectURI != redirectURI {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "redirect_uri mismatch")
		return
	}

	// Verify PKCE: SHA256(code_verifier) must match code_challenge
	h := sha256.Sum256([]byte(codeVerifier))
	computedChallenge := base64.RawURLEncoding.EncodeToString(h[:])
	if computedChallenge != oauthCode.CodeChallenge {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "code_verifier does not match code_challenge")
		return
	}

	// Mark code used — abort if this fails to prevent replay attacks
	if err := oauthStore.MarkCodeUsed(ctx, code); err != nil {
		s.logger.Error().Err(err).Msg("Failed to mark code used")
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "failed to process authorization code")
		return
	}

	// Load user
	user, err := s.app.Storage.InternalStore().GetUser(ctx, oauthCode.UserID)
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "failed to load user")
		return
	}

	// Generate access token JWT
	accessToken, err := s.signOAuthAccessToken(user, clientID, oauthCode.Scope)
	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to sign access token")
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "failed to generate access token")
		return
	}

	// Generate refresh token
	refreshToken, refreshHash, err := s.generateRefreshToken()
	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to generate refresh token")
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "failed to generate refresh token")
		return
	}

	rt := &models.OAuthRefreshToken{
		TokenHash: refreshHash,
		ClientID:  clientID,
		UserID:    user.UserID,
		Scope:     oauthCode.Scope,
		ExpiresAt: time.Now().Add(s.app.Config.Auth.OAuth2.GetRefreshTokenExpiry()),
		Revoked:   false,
		CreatedAt: time.Now(),
	}
	if err := oauthStore.SaveRefreshToken(ctx, rt); err != nil {
		s.logger.Error().Err(err).Msg("Failed to save refresh token")
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "failed to store refresh token")
		return
	}

	expiresIn := int(s.app.Config.Auth.OAuth2.GetAccessTokenExpiry().Seconds())

	w.Header().Set("Cache-Control", "no-store")
	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"token_type":    "Bearer",
		"expires_in":    expiresIn,
	})
}

func (s *Server) handleOAuthTokenRefresh(w http.ResponseWriter, r *http.Request) {
	refreshToken := r.FormValue("refresh_token")
	clientID := r.FormValue("client_id")
	clientSecret := r.FormValue("client_secret")

	if refreshToken == "" || clientID == "" || clientSecret == "" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "refresh_token, client_id, and client_secret are required")
		return
	}

	ctx := r.Context()
	oauthStore := s.app.Storage.OAuthStore()

	// Validate client
	client, err := oauthStore.GetClient(ctx, clientID)
	if err != nil {
		writeOAuthError(w, http.StatusUnauthorized, "invalid_client", "unknown client_id")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(client.ClientSecretHash), []byte(clientSecret)); err != nil {
		writeOAuthError(w, http.StatusUnauthorized, "invalid_client", "invalid client_secret")
		return
	}
	_ = client // used for validation

	// Look up refresh token by hash
	tokenHash := hashRefreshToken(refreshToken)
	storedToken, err := oauthStore.GetRefreshToken(ctx, tokenHash)
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "invalid refresh token")
		return
	}
	if storedToken.Revoked {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "refresh token has been revoked")
		return
	}
	if time.Now().After(storedToken.ExpiresAt) {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "refresh token expired")
		return
	}
	if storedToken.ClientID != clientID {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "client_id mismatch")
		return
	}

	// Revoke old refresh token (rotation)
	if err := oauthStore.RevokeRefreshToken(ctx, tokenHash); err != nil {
		s.logger.Error().Err(err).Msg("Failed to revoke old refresh token")
	}

	// Load user
	user, err := s.app.Storage.InternalStore().GetUser(ctx, storedToken.UserID)
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "failed to load user")
		return
	}

	// Generate new access token
	accessToken, err := s.signOAuthAccessToken(user, clientID, storedToken.Scope)
	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to sign access token")
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "failed to generate access token")
		return
	}

	// Generate new refresh token (rotation)
	newRefreshToken, newRefreshHash, err := s.generateRefreshToken()
	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to generate refresh token")
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "failed to generate refresh token")
		return
	}

	rt := &models.OAuthRefreshToken{
		TokenHash: newRefreshHash,
		ClientID:  clientID,
		UserID:    storedToken.UserID,
		Scope:     storedToken.Scope,
		ExpiresAt: time.Now().Add(s.app.Config.Auth.OAuth2.GetRefreshTokenExpiry()),
		Revoked:   false,
		CreatedAt: time.Now(),
	}
	if err := oauthStore.SaveRefreshToken(ctx, rt); err != nil {
		s.logger.Error().Err(err).Msg("Failed to save new refresh token")
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "failed to store refresh token")
		return
	}

	expiresIn := int(s.app.Config.Auth.OAuth2.GetAccessTokenExpiry().Seconds())

	w.Header().Set("Cache-Control", "no-store")
	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"access_token":  accessToken,
		"refresh_token": newRefreshToken,
		"token_type":    "Bearer",
		"expires_in":    expiresIn,
	})
}

// --- Helpers ---

// signOAuthAccessToken creates a JWT access token with OAuth 2.1 claims.
func (s *Server) signOAuthAccessToken(user *models.InternalUser, clientID, scope string) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"jti":       uuid.New().String(),
		"sub":       user.UserID,
		"email":     user.Email,
		"name":      user.Name,
		"role":      user.Role,
		"client_id": clientID,
		"scope":     scope,
		"iss":       "vire-server",
		"iat":       now.Unix(),
		"exp":       now.Add(s.app.Config.Auth.OAuth2.GetAccessTokenExpiry()).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.app.Config.Auth.JWTSecret))
}

// generateRefreshToken creates a random refresh token and its SHA-256 hash.
// Returns (plaintext, hash, error).
func (s *Server) generateRefreshToken() (string, string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	plaintext := hex.EncodeToString(b)
	return plaintext, hashRefreshToken(plaintext), nil
}

// hashRefreshToken computes the SHA-256 hash of a refresh token for storage lookup.
func hashRefreshToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// writeOAuthError writes an OAuth 2.0 error response (RFC 6749 section 5.2).
func writeOAuthError(w http.ResponseWriter, statusCode int, errorCode, description string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]string{
		"error":             errorCode,
		"error_description": description,
	})
}
