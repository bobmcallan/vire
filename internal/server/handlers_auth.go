package server

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/golang-jwt/jwt/v5"
)

// --- JWT helpers ---

// signJWT creates a signed HMAC-SHA256 JWT for the given user and provider.
func signJWT(user *models.InternalUser, provider string, config *common.AuthConfig) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"sub":      user.UserID,
		"email":    user.Email,
		"name":     user.Name,
		"provider": provider,
		"iss":      "vire-server",
		"iat":      now.Unix(),
		"exp":      now.Add(config.GetTokenExpiry()).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(config.JWTSecret))
}

// validateJWT parses and validates a JWT token string using the given secret.
func validateJWT(tokenString string, secret []byte) (*jwt.Token, jwt.MapClaims, error) {
	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return secret, nil
	})
	if err != nil {
		return nil, nil, err
	}
	return token, claims, nil
}

// --- OAuth state parameter encoding ---

type oauthStatePayload struct {
	Callback string `json:"callback"`
	Nonce    string `json:"nonce"`
	TS       int64  `json:"ts"`
}

// encodeOAuthState encodes a callback URL into a signed state parameter.
func encodeOAuthState(callback string, secret []byte) (string, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}
	payload := oauthStatePayload{
		Callback: callback,
		Nonce:    base64.RawURLEncoding.EncodeToString(nonce),
		TS:       time.Now().Unix(),
	}
	return encodeOAuthStateFromPayload(payload, secret)
}

// encodeOAuthStateFromPayload encodes a pre-built payload into a signed state parameter.
func encodeOAuthStateFromPayload(payload oauthStatePayload, secret []byte) (string, error) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal state: %w", err)
	}
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payloadB64))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return payloadB64 + "." + sig, nil
}

// decodeOAuthState validates and decodes a state parameter, returning the callback URL.
func decodeOAuthState(state string, secret []byte) (string, error) {
	parts := strings.SplitN(state, ".", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid state format")
	}
	payloadB64, sigB64 := parts[0], parts[1]

	// Verify HMAC
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payloadB64))
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(sigB64), []byte(expectedSig)) {
		return "", fmt.Errorf("invalid state signature")
	}

	// Decode payload
	payloadJSON, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return "", fmt.Errorf("invalid state encoding: %w", err)
	}
	var payload oauthStatePayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return "", fmt.Errorf("invalid state payload: %w", err)
	}

	// Check expiry (10 minutes)
	if time.Since(time.Unix(payload.TS, 0)) > 10*time.Minute {
		return "", fmt.Errorf("state expired")
	}

	return payload.Callback, nil
}

// --- OAuth handlers ---

// handleAuthOAuth handles POST /api/auth/oauth — exchange provider code for JWT.
func (s *Server) handleAuthOAuth(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodPost) {
		return
	}

	var req struct {
		Provider string `json:"provider"`
		Code     string `json:"code"`
		State    string `json:"state"`
	}
	if !DecodeJSON(w, r, &req) {
		return
	}

	ctx := r.Context()
	store := s.app.Storage.InternalStore()

	switch req.Provider {
	case "dev":
		if s.app.Config.IsProduction() {
			WriteError(w, http.StatusForbidden, "dev provider is not available in production")
			return
		}
		// Create or get dev user
		user, err := store.GetUser(ctx, "dev_user")
		if err != nil {
			user = &models.InternalUser{
				UserID:    "dev_user",
				Email:     "dev@vire.local",
				Provider:  "dev",
				Role:      "admin",
				CreatedAt: time.Now(),
			}
			if err := store.SaveUser(ctx, user); err != nil {
				s.logger.Error().Err(err).Msg("Failed to create dev user")
				WriteError(w, http.StatusInternalServerError, "failed to create dev user")
				return
			}
		}
		token, err := signJWT(user, "dev", &s.app.Config.Auth)
		if err != nil {
			s.logger.Error().Err(err).Msg("Failed to sign JWT")
			WriteError(w, http.StatusInternalServerError, "failed to sign token")
			return
		}
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"status": "ok",
			"data": map[string]interface{}{
				"token": token,
				"user":  oauthUserResponse(user),
			},
		})

	case "google":
		s.handleGoogleCodeExchange(w, r, req.Code)

	case "github":
		s.handleGitHubCodeExchange(w, r, req.Code)

	default:
		WriteError(w, http.StatusBadRequest, fmt.Sprintf("unsupported provider: %s", req.Provider))
	}
}

// handleGoogleCodeExchange exchanges a Google auth code for user info and returns a JWT.
func (s *Server) handleGoogleCodeExchange(w http.ResponseWriter, r *http.Request, code string) {
	cfg := s.app.Config.Auth.Google
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		WriteError(w, http.StatusInternalServerError, "Google OAuth not configured")
		return
	}

	// Exchange code for token
	tokenResp, err := http.PostForm("https://oauth2.googleapis.com/token", url.Values{
		"code":          {code},
		"client_id":     {cfg.ClientID},
		"client_secret": {cfg.ClientSecret},
		"redirect_uri":  {s.oauthRedirectURI(r, "google")},
		"grant_type":    {"authorization_code"},
	})
	if err != nil {
		s.logger.Error().Err(err).Msg("Google token exchange failed")
		WriteError(w, http.StatusBadGateway, "failed to exchange code with Google")
		return
	}
	defer tokenResp.Body.Close()

	var tokenData struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(tokenResp.Body).Decode(&tokenData); err != nil || tokenData.AccessToken == "" {
		errMsg := "failed to get access token from Google"
		if tokenData.Error != "" {
			errMsg = "Google error: " + tokenData.Error
		}
		WriteError(w, http.StatusBadGateway, errMsg)
		return
	}

	// Get user info
	infoReq, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://www.googleapis.com/oauth2/v2/userinfo", nil)
	infoReq.Header.Set("Authorization", "Bearer "+tokenData.AccessToken)
	infoResp, err := http.DefaultClient.Do(infoReq)
	if err != nil {
		s.logger.Error().Err(err).Msg("Google userinfo request failed")
		WriteError(w, http.StatusBadGateway, "failed to get user info from Google")
		return
	}
	defer infoResp.Body.Close()

	var userInfo struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := json.NewDecoder(infoResp.Body).Decode(&userInfo); err != nil {
		WriteError(w, http.StatusBadGateway, "failed to parse Google user info")
		return
	}

	user := s.findOrCreateOAuthUser(r.Context(), "google_"+userInfo.ID, userInfo.Email, userInfo.Name, "google")
	if user == nil {
		WriteError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	token, err := signJWT(user, "google", &s.app.Config.Auth)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to sign token")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
		"data": map[string]interface{}{
			"token": token,
			"user":  oauthUserResponse(user),
		},
	})
}

// handleGitHubCodeExchange exchanges a GitHub auth code for user info and returns a JWT.
func (s *Server) handleGitHubCodeExchange(w http.ResponseWriter, r *http.Request, code string) {
	cfg := s.app.Config.Auth.GitHub
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		WriteError(w, http.StatusInternalServerError, "GitHub OAuth not configured")
		return
	}

	// Exchange code for token
	tokenReqBody := url.Values{
		"code":          {code},
		"client_id":     {cfg.ClientID},
		"client_secret": {cfg.ClientSecret},
		"redirect_uri":  {s.oauthRedirectURI(r, "github")},
	}
	tokenReq, _ := http.NewRequestWithContext(r.Context(), http.MethodPost, "https://github.com/login/oauth/access_token", strings.NewReader(tokenReqBody.Encode()))
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenReq.Header.Set("Accept", "application/json")
	tokenResp, err := http.DefaultClient.Do(tokenReq)
	if err != nil {
		s.logger.Error().Err(err).Msg("GitHub token exchange failed")
		WriteError(w, http.StatusBadGateway, "failed to exchange code with GitHub")
		return
	}
	defer tokenResp.Body.Close()

	var tokenData struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(tokenResp.Body).Decode(&tokenData); err != nil || tokenData.AccessToken == "" {
		errMsg := "failed to get access token from GitHub"
		if tokenData.Error != "" {
			errMsg = "GitHub error: " + tokenData.Error
		}
		WriteError(w, http.StatusBadGateway, errMsg)
		return
	}

	// Get user info
	userReq, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://api.github.com/user", nil)
	userReq.Header.Set("Authorization", "Bearer "+tokenData.AccessToken)
	userResp, err := http.DefaultClient.Do(userReq)
	if err != nil {
		s.logger.Error().Err(err).Msg("GitHub user request failed")
		WriteError(w, http.StatusBadGateway, "failed to get user info from GitHub")
		return
	}
	defer userResp.Body.Close()

	var ghUser struct {
		ID    int    `json:"id"`
		Login string `json:"login"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := json.NewDecoder(userResp.Body).Decode(&ghUser); err != nil {
		WriteError(w, http.StatusBadGateway, "failed to parse GitHub user info")
		return
	}

	// If email is empty, fetch from /user/emails
	email := ghUser.Email
	if email == "" {
		emailReq, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://api.github.com/user/emails", nil)
		emailReq.Header.Set("Authorization", "Bearer "+tokenData.AccessToken)
		emailResp, err := http.DefaultClient.Do(emailReq)
		if err == nil {
			defer emailResp.Body.Close()
			var emails []struct {
				Email    string `json:"email"`
				Primary  bool   `json:"primary"`
				Verified bool   `json:"verified"`
			}
			if err := json.NewDecoder(emailResp.Body).Decode(&emails); err == nil {
				for _, e := range emails {
					if e.Primary && e.Verified {
						email = e.Email
						break
					}
				}
			}
		}
	}

	// Fall back to Login if Name is empty
	name := ghUser.Name
	if name == "" {
		name = ghUser.Login
	}

	userID := fmt.Sprintf("github_%d", ghUser.ID)
	user := s.findOrCreateOAuthUser(r.Context(), userID, email, name, "github")
	if user == nil {
		WriteError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	token, err := signJWT(user, "github", &s.app.Config.Auth)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to sign token")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
		"data": map[string]interface{}{
			"token": token,
			"user":  oauthUserResponse(user),
		},
	})
}

// findOrCreateOAuthUser looks up or creates a user for an OAuth provider.
// It first checks by provider-specific userID, then by email for account linking.
func (s *Server) findOrCreateOAuthUser(ctx context.Context, userID, email, name, provider string) *models.InternalUser {
	store := s.app.Storage.InternalStore()

	// 1. Check by provider-specific userID
	user, err := store.GetUser(ctx, userID)
	if err == nil {
		// Update email and name if changed
		changed := false
		if user.Email != email {
			user.Email = email
			changed = true
		}
		if name != "" && user.Name != name {
			user.Name = name
			changed = true
		}
		if changed {
			user.ModifiedAt = time.Now()
			store.SaveUser(ctx, user)
		}
		return user
	}

	// 2. Check by email for account linking
	if email != "" {
		existing, err := store.GetUserByEmail(ctx, email)
		if err == nil {
			// Update name if changed
			if name != "" && existing.Name != name {
				existing.Name = name
				existing.ModifiedAt = time.Now()
				store.SaveUser(ctx, existing)
			}
			return existing
		}
	}

	// 3. Create new user
	user = &models.InternalUser{
		UserID:    userID,
		Email:     email,
		Name:      name,
		Provider:  provider,
		Role:      "user",
		CreatedAt: time.Now(),
	}
	if err := store.SaveUser(ctx, user); err != nil {
		s.logger.Error().Err(err).Str("user_id", userID).Msg("Failed to create OAuth user")
		return nil
	}
	return user
}

// oauthUserResponse builds a response map for an OAuth user.
func oauthUserResponse(user *models.InternalUser) map[string]interface{} {
	return map[string]interface{}{
		"user_id":  user.UserID,
		"email":    user.Email,
		"name":     user.Name,
		"provider": user.Provider,
		"role":     user.Role,
	}
}

// --- OAuth login redirects ---

// handleOAuthLoginGoogle handles GET /api/auth/login/google — redirect to Google OAuth.
func (s *Server) handleOAuthLoginGoogle(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodGet) {
		return
	}

	cfg := s.app.Config.Auth.Google
	if cfg.ClientID == "" {
		WriteError(w, http.StatusInternalServerError, "Google OAuth not configured")
		return
	}

	callback := r.URL.Query().Get("callback")
	if err := validateCallbackURL(callback, s.app.Config.IsProduction()); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid callback URL")
		return
	}

	state, err := encodeOAuthState(callback, []byte(s.app.Config.Auth.JWTSecret))
	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to encode OAuth state")
		WriteError(w, http.StatusInternalServerError, "failed to generate state")
		return
	}

	redirectURI := s.oauthRedirectURI(r, "google")

	authURL := fmt.Sprintf(
		"https://accounts.google.com/o/oauth2/v2/auth?client_id=%s&redirect_uri=%s&response_type=code&scope=openid%%20email%%20profile&state=%s",
		url.QueryEscape(cfg.ClientID),
		url.QueryEscape(redirectURI),
		url.QueryEscape(state),
	)

	http.Redirect(w, r, authURL, http.StatusFound)
}

// handleOAuthLoginGitHub handles GET /api/auth/login/github — redirect to GitHub OAuth.
func (s *Server) handleOAuthLoginGitHub(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodGet) {
		return
	}

	cfg := s.app.Config.Auth.GitHub
	if cfg.ClientID == "" {
		WriteError(w, http.StatusInternalServerError, "GitHub OAuth not configured")
		return
	}

	callback := r.URL.Query().Get("callback")
	if err := validateCallbackURL(callback, s.app.Config.IsProduction()); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid callback URL")
		return
	}

	state, err := encodeOAuthState(callback, []byte(s.app.Config.Auth.JWTSecret))
	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to encode OAuth state")
		WriteError(w, http.StatusInternalServerError, "failed to generate state")
		return
	}

	redirectURI := s.oauthRedirectURI(r, "github")

	authURL := fmt.Sprintf(
		"https://github.com/login/oauth/authorize?client_id=%s&redirect_uri=%s&scope=read:user%%20user:email&state=%s",
		url.QueryEscape(cfg.ClientID),
		url.QueryEscape(redirectURI),
		url.QueryEscape(state),
	)

	http.Redirect(w, r, authURL, http.StatusFound)
}

// --- OAuth callbacks ---

// handleOAuthCallbackGoogle handles GET /api/auth/callback/google.
func (s *Server) handleOAuthCallbackGoogle(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodGet) {
		return
	}

	// Check if provider sent an error (e.g., user denied consent)
	if errParam := r.URL.Query().Get("error"); errParam != "" {
		stateParam := r.URL.Query().Get("state")
		if callback, err := decodeOAuthState(stateParam, []byte(s.app.Config.Auth.JWTSecret)); err == nil {
			redirectWithError(w, r, callback, errParam)
		} else {
			WriteError(w, http.StatusBadRequest, "OAuth error: "+errParam)
		}
		return
	}

	code := r.URL.Query().Get("code")
	stateParam := r.URL.Query().Get("state")

	callback, err := decodeOAuthState(stateParam, []byte(s.app.Config.Auth.JWTSecret))
	if err != nil {
		s.logger.Error().Err(err).Msg("Invalid OAuth state")
		WriteError(w, http.StatusBadRequest, "invalid state parameter")
		return
	}

	cfg := s.app.Config.Auth.Google
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		redirectWithError(w, r, callback, "provider_not_configured")
		return
	}

	// Exchange code for token
	tokenResp, err := http.PostForm("https://oauth2.googleapis.com/token", url.Values{
		"code":          {code},
		"client_id":     {cfg.ClientID},
		"client_secret": {cfg.ClientSecret},
		"redirect_uri":  {s.oauthRedirectURI(r, "google")},
		"grant_type":    {"authorization_code"},
	})
	if err != nil {
		s.logger.Error().Err(err).Msg("Google token exchange failed")
		redirectWithError(w, r, callback, "exchange_failed")
		return
	}
	defer tokenResp.Body.Close()

	var tokenData struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(tokenResp.Body).Decode(&tokenData); err != nil || tokenData.AccessToken == "" {
		redirectWithError(w, r, callback, "exchange_failed")
		return
	}

	// Get user info
	infoReq, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://www.googleapis.com/oauth2/v2/userinfo", nil)
	infoReq.Header.Set("Authorization", "Bearer "+tokenData.AccessToken)
	infoResp, err := http.DefaultClient.Do(infoReq)
	if err != nil {
		redirectWithError(w, r, callback, "profile_failed")
		return
	}
	defer infoResp.Body.Close()

	var userInfo struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := json.NewDecoder(infoResp.Body).Decode(&userInfo); err != nil {
		redirectWithError(w, r, callback, "profile_failed")
		return
	}

	user := s.findOrCreateOAuthUser(r.Context(), "google_"+userInfo.ID, userInfo.Email, userInfo.Name, "google")
	if user == nil {
		redirectWithError(w, r, callback, "user_creation_failed")
		return
	}

	jwtToken, err := signJWT(user, "google", &s.app.Config.Auth)
	if err != nil {
		redirectWithError(w, r, callback, "token_failed")
		return
	}

	if err := validateCallbackURL(callback, s.app.Config.IsProduction()); err != nil {
		s.logger.Error().Err(err).Str("callback", callback).Msg("Invalid callback URL in OAuth state")
		WriteError(w, http.StatusBadRequest, "invalid callback URL in state")
		return
	}

	redirectURL, err := buildCallbackRedirectURL(callback, jwtToken)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to build redirect URL")
		return
	}
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// handleOAuthCallbackGitHub handles GET /api/auth/callback/github.
func (s *Server) handleOAuthCallbackGitHub(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodGet) {
		return
	}

	// Check if provider sent an error (e.g., user denied consent)
	if errParam := r.URL.Query().Get("error"); errParam != "" {
		stateParam := r.URL.Query().Get("state")
		if callback, err := decodeOAuthState(stateParam, []byte(s.app.Config.Auth.JWTSecret)); err == nil {
			redirectWithError(w, r, callback, errParam)
		} else {
			WriteError(w, http.StatusBadRequest, "OAuth error: "+errParam)
		}
		return
	}

	code := r.URL.Query().Get("code")
	stateParam := r.URL.Query().Get("state")

	callback, err := decodeOAuthState(stateParam, []byte(s.app.Config.Auth.JWTSecret))
	if err != nil {
		s.logger.Error().Err(err).Msg("Invalid OAuth state")
		WriteError(w, http.StatusBadRequest, "invalid state parameter")
		return
	}

	cfg := s.app.Config.Auth.GitHub
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		redirectWithError(w, r, callback, "provider_not_configured")
		return
	}

	// Exchange code for token
	tokenReqBody := url.Values{
		"code":          {code},
		"client_id":     {cfg.ClientID},
		"client_secret": {cfg.ClientSecret},
		"redirect_uri":  {s.oauthRedirectURI(r, "github")},
	}
	tokenReq, _ := http.NewRequestWithContext(r.Context(), http.MethodPost, "https://github.com/login/oauth/access_token", strings.NewReader(tokenReqBody.Encode()))
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenReq.Header.Set("Accept", "application/json")
	tokenResp, err := http.DefaultClient.Do(tokenReq)
	if err != nil {
		redirectWithError(w, r, callback, "exchange_failed")
		return
	}
	defer tokenResp.Body.Close()

	var tokenData struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(tokenResp.Body).Decode(&tokenData); err != nil || tokenData.AccessToken == "" {
		redirectWithError(w, r, callback, "exchange_failed")
		return
	}

	// Get user info
	userReq, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://api.github.com/user", nil)
	userReq.Header.Set("Authorization", "Bearer "+tokenData.AccessToken)
	userResp, err := http.DefaultClient.Do(userReq)
	if err != nil {
		redirectWithError(w, r, callback, "profile_failed")
		return
	}
	defer userResp.Body.Close()

	var ghUser struct {
		ID    int    `json:"id"`
		Login string `json:"login"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := json.NewDecoder(userResp.Body).Decode(&ghUser); err != nil {
		redirectWithError(w, r, callback, "profile_failed")
		return
	}

	email := ghUser.Email
	if email == "" {
		emailReq, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://api.github.com/user/emails", nil)
		emailReq.Header.Set("Authorization", "Bearer "+tokenData.AccessToken)
		emailResp, err := http.DefaultClient.Do(emailReq)
		if err == nil {
			defer emailResp.Body.Close()
			var emails []struct {
				Email    string `json:"email"`
				Primary  bool   `json:"primary"`
				Verified bool   `json:"verified"`
			}
			if err := json.NewDecoder(emailResp.Body).Decode(&emails); err == nil {
				for _, e := range emails {
					if e.Primary && e.Verified {
						email = e.Email
						break
					}
				}
			}
		}
	}

	// Fall back to Login if Name is empty
	name := ghUser.Name
	if name == "" {
		name = ghUser.Login
	}

	userID := fmt.Sprintf("github_%d", ghUser.ID)
	user := s.findOrCreateOAuthUser(r.Context(), userID, email, name, "github")
	if user == nil {
		redirectWithError(w, r, callback, "user_creation_failed")
		return
	}

	jwtToken, err := signJWT(user, "github", &s.app.Config.Auth)
	if err != nil {
		redirectWithError(w, r, callback, "token_failed")
		return
	}

	if err := validateCallbackURL(callback, s.app.Config.IsProduction()); err != nil {
		s.logger.Error().Err(err).Str("callback", callback).Msg("Invalid callback URL in OAuth state")
		WriteError(w, http.StatusBadRequest, "invalid callback URL in state")
		return
	}

	redirectURL, err := buildCallbackRedirectURL(callback, jwtToken)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to build redirect URL")
		return
	}
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// --- Token validation ---

// handleAuthValidate handles POST /api/auth/validate — validate a JWT token.
func (s *Server) handleAuthValidate(w http.ResponseWriter, r *http.Request) {
	if !RequireMethod(w, r, http.MethodPost) {
		return
	}

	authHeader := r.Header.Get("Authorization")
	if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
		WriteError(w, http.StatusUnauthorized, "missing or invalid Authorization header")
		return
	}
	tokenString := strings.TrimPrefix(authHeader, "Bearer ")

	_, claims, err := validateJWT(tokenString, []byte(s.app.Config.Auth.JWTSecret))
	if err != nil {
		WriteError(w, http.StatusUnauthorized, "invalid or expired token")
		return
	}

	sub, _ := claims["sub"].(string)
	if sub == "" {
		WriteError(w, http.StatusUnauthorized, "invalid token claims")
		return
	}

	ctx := r.Context()
	store := s.app.Storage.InternalStore()
	user, err := store.GetUser(ctx, sub)
	if err != nil {
		WriteError(w, http.StatusUnauthorized, "invalid or expired token")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
		"data": map[string]interface{}{
			"user": oauthUserResponse(user),
		},
	})
}

// --- Helpers ---

// validateCallbackURL checks that a callback URL is safe to redirect to.
// It rejects non-http(s) schemes (e.g. javascript:, data:), protocol-relative
// URLs, and URLs with no host. In production, only https is allowed.
func validateCallbackURL(callback string, isProduction bool) error {
	if callback == "" {
		return fmt.Errorf("empty callback URL")
	}

	// Block protocol-relative URLs
	if strings.HasPrefix(callback, "//") {
		return fmt.Errorf("protocol-relative URLs not allowed")
	}

	u, err := url.Parse(callback)
	if err != nil {
		return fmt.Errorf("invalid callback URL: %w", err)
	}

	// Only allow http and https schemes
	switch u.Scheme {
	case "https":
		// Always allowed
	case "http":
		if isProduction {
			return fmt.Errorf("http callbacks not allowed in production")
		}
	default:
		return fmt.Errorf("callback scheme %q not allowed", u.Scheme)
	}

	// Must have a host
	if u.Host == "" {
		return fmt.Errorf("callback URL must have a host")
	}

	return nil
}

// buildCallbackRedirectURL safely appends a token query parameter to a callback URL.
func buildCallbackRedirectURL(callback, jwtToken string) (string, error) {
	u, err := url.Parse(callback)
	if err != nil {
		return "", fmt.Errorf("invalid callback URL: %w", err)
	}
	q := u.Query()
	q.Set("token", jwtToken)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// redirectWithError redirects to the callback URL with an error query parameter.
func redirectWithError(w http.ResponseWriter, r *http.Request, callback, errorCode string) {
	u, err := url.Parse(callback)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "invalid callback URL")
		return
	}
	q := u.Query()
	q.Set("error", errorCode)
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

// oauthRedirectURI builds the server-side redirect URI for OAuth callbacks.
func (s *Server) oauthRedirectURI(r *http.Request, provider string) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := r.Host
	return fmt.Sprintf("%s://%s/api/auth/callback/%s", scheme, host, provider)
}
