package server

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/app"
	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// --- In-memory mock stores for OAuth tests ---

// memOAuthStore is a minimal in-memory OAuthStore for tests.
type memOAuthStore struct {
	mu      sync.Mutex
	clients map[string]*models.OAuthClient
	codes   map[string]*models.OAuthCode
	tokens  map[string]*models.OAuthRefreshToken
}

func newMemOAuthStore() *memOAuthStore {
	return &memOAuthStore{
		clients: make(map[string]*models.OAuthClient),
		codes:   make(map[string]*models.OAuthCode),
		tokens:  make(map[string]*models.OAuthRefreshToken),
	}
}

func (s *memOAuthStore) SaveClient(_ context.Context, c *models.OAuthClient) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clients[c.ClientID] = c
	return nil
}
func (s *memOAuthStore) GetClient(_ context.Context, id string) (*models.OAuthClient, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.clients[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return c, nil
}
func (s *memOAuthStore) DeleteClient(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.clients, id)
	return nil
}
func (s *memOAuthStore) SaveCode(_ context.Context, c *models.OAuthCode) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.codes[c.Code] = c
	return nil
}
func (s *memOAuthStore) GetCode(_ context.Context, code string) (*models.OAuthCode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.codes[code]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return c, nil
}
func (s *memOAuthStore) MarkCodeUsed(_ context.Context, code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, ok := s.codes[code]; ok {
		c.Used = true
	}
	return nil
}
func (s *memOAuthStore) PurgeExpiredCodes(_ context.Context) (int, error) { return 0, nil }
func (s *memOAuthStore) SaveRefreshToken(_ context.Context, t *models.OAuthRefreshToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[t.TokenHash] = t
	return nil
}
func (s *memOAuthStore) GetRefreshToken(_ context.Context, h string) (*models.OAuthRefreshToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tokens[h]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return t, nil
}
func (s *memOAuthStore) RevokeRefreshToken(_ context.Context, h string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.tokens[h]; ok {
		t.Revoked = true
	}
	return nil
}
func (s *memOAuthStore) RevokeRefreshTokensByClient(_ context.Context, userID, clientID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.tokens {
		if t.UserID == userID && t.ClientID == clientID {
			t.Revoked = true
		}
	}
	return nil
}
func (s *memOAuthStore) PurgeExpiredTokens(_ context.Context) (int, error) { return 0, nil }
func (s *memOAuthStore) UpdateRefreshTokenLastUsed(_ context.Context, tokenHash string, lastUsedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.tokens[tokenHash]; ok {
		t.LastUsedAt = lastUsedAt
	}
	return nil
}

var _ interfaces.OAuthStore = (*memOAuthStore)(nil)

// memInternalStore is a minimal in-memory InternalStore for OAuth tests.
type memInternalStore struct {
	mu    sync.Mutex
	users map[string]*models.InternalUser
	kv    map[string][]models.UserKeyValue
	sysKV map[string]string
}

func newMemInternalStore() *memInternalStore {
	return &memInternalStore{
		users: make(map[string]*models.InternalUser),
		kv:    make(map[string][]models.UserKeyValue),
		sysKV: make(map[string]string),
	}
}

func (s *memInternalStore) SaveUser(_ context.Context, u *models.InternalUser) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users[u.UserID] = u
	return nil
}
func (s *memInternalStore) GetUser(_ context.Context, id string) (*models.InternalUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return u, nil
}
func (s *memInternalStore) GetUserByEmail(_ context.Context, email string) (*models.InternalUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	email = strings.ToLower(email)
	for _, u := range s.users {
		if strings.ToLower(u.Email) == email {
			return u, nil
		}
	}
	return nil, fmt.Errorf("not found")
}
func (s *memInternalStore) DeleteUser(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.users, id)
	return nil
}
func (s *memInternalStore) ListUsers(_ context.Context) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var ids []string
	for id := range s.users {
		ids = append(ids, id)
	}
	return ids, nil
}
func (s *memInternalStore) GetUserKV(_ context.Context, uid, key string) (*models.UserKeyValue, error) {
	return nil, fmt.Errorf("not found")
}
func (s *memInternalStore) SetUserKV(_ context.Context, uid, key, value string) error { return nil }
func (s *memInternalStore) DeleteUserKV(_ context.Context, uid, key string) error     { return nil }
func (s *memInternalStore) ListUserKV(_ context.Context, uid string) ([]*models.UserKeyValue, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	kvs := s.kv[uid]
	var result []*models.UserKeyValue
	for i := range kvs {
		result = append(result, &kvs[i])
	}
	return result, nil
}
func (s *memInternalStore) GetSystemKV(_ context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.sysKV[key]
	if !ok {
		return "", fmt.Errorf("not found")
	}
	return v, nil
}
func (s *memInternalStore) SetSystemKV(_ context.Context, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sysKV[key] = value
	return nil
}
func (s *memInternalStore) Close() error { return nil }

var _ interfaces.InternalStore = (*memInternalStore)(nil)

// oauthTestStorageManager wraps the mem stores into a StorageManager.
type oauthTestStorageManager struct {
	internal *memInternalStore
	oauth    *memOAuthStore
}

func (m *oauthTestStorageManager) InternalStore() interfaces.InternalStore         { return m.internal }
func (m *oauthTestStorageManager) OAuthStore() interfaces.OAuthStore               { return m.oauth }
func (m *oauthTestStorageManager) UserDataStore() interfaces.UserDataStore         { return nil }
func (m *oauthTestStorageManager) MarketDataStorage() interfaces.MarketDataStorage { return nil }
func (m *oauthTestStorageManager) SignalStorage() interfaces.SignalStorage         { return nil }
func (m *oauthTestStorageManager) StockIndexStore() interfaces.StockIndexStore     { return nil }
func (m *oauthTestStorageManager) JobQueueStore() interfaces.JobQueueStore         { return nil }
func (m *oauthTestStorageManager) FileStore() interfaces.FileStore                 { return nil }
func (m *oauthTestStorageManager) FeedbackStore() interfaces.FeedbackStore         { return nil }
func (m *oauthTestStorageManager) DataPath() string                                { return "" }
func (m *oauthTestStorageManager) WriteRaw(_, _ string, _ []byte) error            { return nil }
func (m *oauthTestStorageManager) PurgeDerivedData(_ context.Context) (map[string]int, error) {
	return nil, nil
}
func (m *oauthTestStorageManager) PurgeReports(_ context.Context) (int, error) { return 0, nil }
func (m *oauthTestStorageManager) Close() error                                { return nil }

var _ interfaces.StorageManager = (*oauthTestStorageManager)(nil)

// --- Test server factory ---

func newOAuthTestServer(t *testing.T) *Server {
	t.Helper()
	internalStore := newMemInternalStore()
	oauthStore := newMemOAuthStore()
	mgr := &oauthTestStorageManager{internal: internalStore, oauth: oauthStore}

	logger := common.NewLoggerFromConfig(common.LoggingConfig{Level: "disabled"})
	cfg := common.NewDefaultConfig()
	cfg.Environment = "development"
	cfg.Auth.JWTSecret = "test-secret-key-for-oauth-tests"
	cfg.Auth.OAuth2.Issuer = "https://auth.vire.test"

	a := &app.App{
		Config:  cfg,
		Logger:  logger,
		Storage: mgr,
	}
	return &Server{app: a, logger: logger}
}

func newOAuthTestServerNoIssuer(t *testing.T) *Server {
	t.Helper()
	mgr := &oauthTestStorageManager{internal: newMemInternalStore(), oauth: newMemOAuthStore()}
	logger := common.NewLoggerFromConfig(common.LoggingConfig{Level: "disabled"})
	cfg := common.NewDefaultConfig()
	cfg.Environment = "development"
	cfg.Auth.JWTSecret = "test-secret"
	a := &app.App{Config: cfg, Logger: logger, Storage: mgr}
	return &Server{app: a, logger: logger}
}

// registerTestClient is an alias for registerTestOAuthClient used by stress tests.
func registerTestClient(t *testing.T, srv *Server) (string, string) {
	return registerTestOAuthClient(t, srv)
}

// registerTestOAuthClient registers a client via the handler and returns (clientID, clientSecret).
func registerTestOAuthClient(t *testing.T, srv *Server) (string, string) {
	t.Helper()
	body := jsonBody(t, map[string]interface{}{
		"client_name":   "test-client",
		"redirect_uris": []string{"http://localhost:3000/callback"},
	})
	req := httptest.NewRequest(http.MethodPost, "/oauth/register", body)
	rec := httptest.NewRecorder()
	srv.handleOAuthRegister(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp["client_id"].(string), resp["client_secret"].(string)
}

// createOAuthTestUser creates a user with email auth for the OAuth consent flow.
func createOAuthTestUser(t *testing.T, srv *Server, userID, email, password string) {
	t.Helper()
	passwordBytes := []byte(password)
	if len(passwordBytes) > 72 {
		passwordBytes = passwordBytes[:72]
	}
	hash, err := bcrypt.GenerateFromPassword(passwordBytes, 10)
	require.NoError(t, err)

	user := &models.InternalUser{
		UserID:       userID,
		Email:        email,
		Name:         "Test User",
		PasswordHash: string(hash),
		Provider:     "email",
		Role:         models.RoleUser,
		CreatedAt:    time.Now(),
	}
	require.NoError(t, srv.app.Storage.InternalStore().SaveUser(context.Background(), user))
}

// pkceChallenge generates a code_verifier and code_challenge (S256).
func pkceChallenge() (verifier, challenge string) {
	verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	h := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(h[:])
	return
}

// createTestJWT creates a JWT from claims for tests.
func createTestJWT(t *testing.T, claims map[string]interface{}, secret string) string {
	t.Helper()
	mapClaims := jwt.MapClaims{}
	for k, v := range claims {
		mapClaims[k] = v
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, mapClaims)
	s, err := token.SignedString([]byte(secret))
	require.NoError(t, err)
	return s
}

// --- Metadata endpoint tests ---

func TestOAuthProtectedResource_ReturnsMetadata(t *testing.T) {
	srv := newOAuthTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)
	rec := httptest.NewRecorder()
	srv.handleOAuthProtectedResource(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "https://auth.vire.test", resp["resource"])
	assert.Contains(t, resp["authorization_servers"], "https://auth.vire.test")
	assert.Contains(t, resp["bearer_methods_supported"], "header")
}

func TestOAuthProtectedResource_NotConfigured(t *testing.T) {
	srv := newOAuthTestServerNoIssuer(t)
	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)
	rec := httptest.NewRecorder()
	srv.handleOAuthProtectedResource(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestOAuthAuthorizationServer_ReturnsMetadata(t *testing.T) {
	srv := newOAuthTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil)
	rec := httptest.NewRecorder()
	srv.handleOAuthAuthorizationServer(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "https://auth.vire.test", resp["issuer"])
	assert.Equal(t, "https://auth.vire.test/oauth/authorize", resp["authorization_endpoint"])
	assert.Equal(t, "https://auth.vire.test/oauth/token", resp["token_endpoint"])
	assert.Equal(t, "https://auth.vire.test/oauth/register", resp["registration_endpoint"])
	assert.Contains(t, resp["code_challenge_methods_supported"], "S256")
	assert.Contains(t, resp["grant_types_supported"], "authorization_code")
	assert.Contains(t, resp["grant_types_supported"], "refresh_token")
}

func TestOAuthProtectedResource_MethodNotAllowed(t *testing.T) {
	srv := newOAuthTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/.well-known/oauth-protected-resource", nil)
	rec := httptest.NewRecorder()
	srv.handleOAuthProtectedResource(rec, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

// --- DCR tests ---

func TestOAuthRegister_CreatesClient(t *testing.T) {
	srv := newOAuthTestServer(t)
	clientID, clientSecret := registerTestOAuthClient(t, srv)
	assert.NotEmpty(t, clientID)
	assert.NotEmpty(t, clientSecret)
	assert.Len(t, clientSecret, 64) // 32 bytes hex
}

func TestOAuthRegister_MissingClientName(t *testing.T) {
	srv := newOAuthTestServer(t)
	body := jsonBody(t, map[string]interface{}{
		"redirect_uris": []string{"http://localhost/cb"},
	})
	req := httptest.NewRequest(http.MethodPost, "/oauth/register", body)
	rec := httptest.NewRecorder()
	srv.handleOAuthRegister(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestOAuthRegister_MissingRedirectURIs(t *testing.T) {
	srv := newOAuthTestServer(t)
	body := jsonBody(t, map[string]interface{}{
		"client_name": "test",
	})
	req := httptest.NewRequest(http.MethodPost, "/oauth/register", body)
	rec := httptest.NewRecorder()
	srv.handleOAuthRegister(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestOAuthRegister_InvalidRedirectURI(t *testing.T) {
	srv := newOAuthTestServer(t)
	body := jsonBody(t, map[string]interface{}{
		"client_name":   "test",
		"redirect_uris": []string{"not-a-uri"},
	})
	req := httptest.NewRequest(http.MethodPost, "/oauth/register", body)
	rec := httptest.NewRecorder()
	srv.handleOAuthRegister(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestOAuthRegister_NotConfigured(t *testing.T) {
	srv := newOAuthTestServerNoIssuer(t)
	body := jsonBody(t, map[string]interface{}{
		"client_name":   "test",
		"redirect_uris": []string{"http://localhost/cb"},
	})
	req := httptest.NewRequest(http.MethodPost, "/oauth/register", body)
	rec := httptest.NewRecorder()
	srv.handleOAuthRegister(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// --- Authorize GET tests ---

func TestOAuthAuthorizeGET_RendersForm(t *testing.T) {
	srv := newOAuthTestServer(t)
	clientID, _ := registerTestOAuthClient(t, srv)
	_, challenge := pkceChallenge()

	u := "/oauth/authorize?" + url.Values{
		"client_id":             {clientID},
		"redirect_uri":          {"http://localhost:3000/callback"},
		"response_type":         {"code"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {"xyz"},
		"scope":                 {"vire"},
	}.Encode()
	req := httptest.NewRequest(http.MethodGet, u, nil)
	rec := httptest.NewRecorder()
	srv.handleOAuthAuthorize(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "text/html")
	assert.Contains(t, rec.Body.String(), "test-client")
	assert.Contains(t, rec.Body.String(), "Grant Access")
}

func TestOAuthAuthorizeGET_MissingParams(t *testing.T) {
	srv := newOAuthTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?client_id=nope", nil)
	rec := httptest.NewRecorder()
	srv.handleOAuthAuthorize(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestOAuthAuthorizeGET_UnknownClient(t *testing.T) {
	srv := newOAuthTestServer(t)
	_, challenge := pkceChallenge()
	u := "/oauth/authorize?" + url.Values{
		"client_id":             {"nonexistent"},
		"redirect_uri":          {"http://localhost:3000/callback"},
		"response_type":         {"code"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {"xyz"},
	}.Encode()
	req := httptest.NewRequest(http.MethodGet, u, nil)
	rec := httptest.NewRecorder()
	srv.handleOAuthAuthorize(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// --- Authorize POST + Token Exchange (full auth code flow) ---

func TestOAuthFullFlow_AuthCodeWithPKCE(t *testing.T) {
	srv := newOAuthTestServer(t)
	clientID, clientSecret := registerTestOAuthClient(t, srv)
	createOAuthTestUser(t, srv, "oauth_user", "user@example.com", "password123")
	verifier, challenge := pkceChallenge()

	// Step 1: POST /oauth/authorize (consent form submit)
	form := url.Values{
		"client_id":             {clientID},
		"redirect_uri":          {"http://localhost:3000/callback"},
		"response_type":         {"code"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {"my-state"},
		"scope":                 {"vire"},
		"email":                 {"user@example.com"},
		"password":              {"password123"},
	}
	req := httptest.NewRequest(http.MethodPost, "/oauth/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.handleOAuthAuthorize(rec, req)

	require.Equal(t, http.StatusFound, rec.Code, rec.Body.String())
	loc, err := url.Parse(rec.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "my-state", loc.Query().Get("state"))
	code := loc.Query().Get("code")
	require.NotEmpty(t, code)

	// Step 2: POST /oauth/token (exchange code for tokens)
	tokenForm := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"redirect_uri":  {"http://localhost:3000/callback"},
		"code_verifier": {verifier},
	}
	req = httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(tokenForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	srv.handleOAuthToken(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var tokenResp map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &tokenResp))
	assert.NotEmpty(t, tokenResp["access_token"])
	assert.NotEmpty(t, tokenResp["refresh_token"])
	assert.Equal(t, "Bearer", tokenResp["token_type"])
	assert.NotZero(t, tokenResp["expires_in"])

	// Verify the access token is a valid JWT
	accessToken := tokenResp["access_token"].(string)
	_, claims, err := validateJWT(accessToken, []byte(srv.app.Config.Auth.JWTSecret))
	require.NoError(t, err)
	assert.Equal(t, "oauth_user", claims["sub"])
	assert.Equal(t, clientID, claims["client_id"])
	assert.Equal(t, "vire", claims["scope"])
}

func TestOAuthToken_UsedCodeRejected(t *testing.T) {
	srv := newOAuthTestServer(t)
	clientID, clientSecret := registerTestOAuthClient(t, srv)
	createOAuthTestUser(t, srv, "u1", "u1@example.com", "pass")
	verifier, challenge := pkceChallenge()

	// Get a code
	form := url.Values{
		"client_id": {clientID}, "redirect_uri": {"http://localhost:3000/callback"},
		"response_type": {"code"}, "code_challenge": {challenge},
		"code_challenge_method": {"S256"}, "state": {"s"},
		"email": {"u1@example.com"}, "password": {"pass"},
	}
	req := httptest.NewRequest(http.MethodPost, "/oauth/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.handleOAuthAuthorize(rec, req)
	loc, _ := url.Parse(rec.Header().Get("Location"))
	code := loc.Query().Get("code")

	// Use it once
	tokenForm := url.Values{
		"grant_type": {"authorization_code"}, "code": {code},
		"client_id": {clientID}, "client_secret": {clientSecret},
		"redirect_uri": {"http://localhost:3000/callback"}, "code_verifier": {verifier},
	}
	req = httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(tokenForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	srv.handleOAuthToken(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Use it again — should fail
	req = httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(tokenForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	srv.handleOAuthToken(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "already used")
}

func TestOAuthToken_WrongCodeVerifier(t *testing.T) {
	srv := newOAuthTestServer(t)
	clientID, clientSecret := registerTestOAuthClient(t, srv)
	createOAuthTestUser(t, srv, "u2", "u2@example.com", "pass")
	_, challenge := pkceChallenge()

	// Get code
	form := url.Values{
		"client_id": {clientID}, "redirect_uri": {"http://localhost:3000/callback"},
		"response_type": {"code"}, "code_challenge": {challenge},
		"code_challenge_method": {"S256"}, "state": {"s"},
		"email": {"u2@example.com"}, "password": {"pass"},
	}
	req := httptest.NewRequest(http.MethodPost, "/oauth/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.handleOAuthAuthorize(rec, req)
	loc, _ := url.Parse(rec.Header().Get("Location"))
	code := loc.Query().Get("code")

	// Wrong verifier
	tokenForm := url.Values{
		"grant_type": {"authorization_code"}, "code": {code},
		"client_id": {clientID}, "client_secret": {clientSecret},
		"redirect_uri": {"http://localhost:3000/callback"}, "code_verifier": {"wrong-verifier"},
	}
	req = httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(tokenForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	srv.handleOAuthToken(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "code_verifier")
}

func TestOAuthToken_WrongClientSecret(t *testing.T) {
	srv := newOAuthTestServer(t)
	clientID, _ := registerTestOAuthClient(t, srv)

	tokenForm := url.Values{
		"grant_type": {"authorization_code"}, "code": {"fake"},
		"client_id": {clientID}, "client_secret": {"wrong-secret"},
		"redirect_uri": {"http://localhost:3000/callback"}, "code_verifier": {"x"},
	}
	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(tokenForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.handleOAuthToken(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid_client")
}

func TestOAuthToken_ExpiredCode(t *testing.T) {
	srv := newOAuthTestServer(t)
	clientID, clientSecret := registerTestOAuthClient(t, srv)
	createOAuthTestUser(t, srv, "expired_u", "exp@example.com", "pass")
	verifier, challenge := pkceChallenge()

	// Save an already-expired code directly in the store
	oauthStore := srv.app.Storage.OAuthStore()
	expiredCode := &models.OAuthCode{
		Code:                "expired-code-123",
		ClientID:            clientID,
		UserID:              "expired_u",
		RedirectURI:         "http://localhost:3000/callback",
		CodeChallenge:       challenge,
		CodeChallengeMethod: "S256",
		Scope:               "vire",
		ExpiresAt:           time.Now().Add(-1 * time.Hour), // expired
		Used:                false,
		CreatedAt:           time.Now().Add(-2 * time.Hour),
	}
	require.NoError(t, oauthStore.SaveCode(context.Background(), expiredCode))
	_ = verifier

	tokenForm := url.Values{
		"grant_type": {"authorization_code"}, "code": {"expired-code-123"},
		"client_id": {clientID}, "client_secret": {clientSecret},
		"redirect_uri": {"http://localhost:3000/callback"}, "code_verifier": {verifier},
	}
	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(tokenForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.handleOAuthToken(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "expired")
}

// --- Refresh token tests ---

func TestOAuthRefreshToken_RotatesTokens(t *testing.T) {
	srv := newOAuthTestServer(t)
	clientID, clientSecret := registerTestOAuthClient(t, srv)
	createOAuthTestUser(t, srv, "rt_user", "rt@example.com", "pass")
	verifier, challenge := pkceChallenge()

	// Get auth code
	form := url.Values{
		"client_id": {clientID}, "redirect_uri": {"http://localhost:3000/callback"},
		"response_type": {"code"}, "code_challenge": {challenge},
		"code_challenge_method": {"S256"}, "state": {"s"},
		"email": {"rt@example.com"}, "password": {"pass"},
	}
	req := httptest.NewRequest(http.MethodPost, "/oauth/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.handleOAuthAuthorize(rec, req)
	loc, _ := url.Parse(rec.Header().Get("Location"))
	code := loc.Query().Get("code")

	// Exchange for tokens
	tokenForm := url.Values{
		"grant_type": {"authorization_code"}, "code": {code},
		"client_id": {clientID}, "client_secret": {clientSecret},
		"redirect_uri": {"http://localhost:3000/callback"}, "code_verifier": {verifier},
	}
	req = httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(tokenForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	srv.handleOAuthToken(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp1 map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp1))
	refreshToken1 := resp1["refresh_token"].(string)

	// Use refresh token to get new tokens
	refreshForm := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken1},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
	}
	req = httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(refreshForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	srv.handleOAuthToken(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp2 map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp2))
	assert.NotEmpty(t, resp2["access_token"])
	refreshToken2 := resp2["refresh_token"].(string)
	assert.NotEqual(t, refreshToken1, refreshToken2, "refresh token should rotate")

	// Old refresh token should be revoked
	oldRefreshForm := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken1},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
	}
	req = httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(oldRefreshForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	srv.handleOAuthToken(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "revoked")
}

// --- Bearer middleware tests ---

func TestBearerMiddleware_ValidJWTPopulatesContext(t *testing.T) {
	srv := newOAuthTestServer(t)
	createOAuthTestUser(t, srv, "bearer_user", "b@example.com", "pass")

	// Create a valid JWT
	user := &models.InternalUser{
		UserID:   "bearer_user",
		Email:    "b@example.com",
		Name:     "Bearer User",
		Role:     models.RoleUser,
		Provider: "email",
	}
	token, err := signJWT(user, "email", &srv.app.Config.Auth)
	require.NoError(t, err)

	// Use the middleware
	var capturedUC *common.UserContext
	handler := bearerTokenMiddleware(srv.app.Config, srv.app.Storage.InternalStore())(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedUC = common.UserContextFromContext(r.Context())
			w.WriteHeader(http.StatusOK)
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, capturedUC)
	assert.Equal(t, "bearer_user", capturedUC.UserID)
	assert.Equal(t, models.RoleUser, capturedUC.Role)
}

func TestBearerMiddleware_ExpiredJWTReturns401(t *testing.T) {
	srv := newOAuthTestServer(t)

	// Create an expired JWT
	token := createTestJWT(t, map[string]interface{}{
		"sub":  "u",
		"exp":  time.Now().Add(-1 * time.Hour).Unix(),
		"iat":  time.Now().Add(-2 * time.Hour).Unix(),
		"iss":  "vire-server",
		"role": "user",
	}, srv.app.Config.Auth.JWTSecret)

	handler := bearerTokenMiddleware(srv.app.Config, srv.app.Storage.InternalStore())(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("handler should not be called for expired token")
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, "Bearer", rec.Header().Get("WWW-Authenticate"))
}

func TestBearerMiddleware_NoAuthHeaderPassesThrough(t *testing.T) {
	srv := newOAuthTestServer(t)

	called := false
	handler := bearerTokenMiddleware(srv.app.Config, srv.app.Storage.InternalStore())(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			uc := common.UserContextFromContext(r.Context())
			assert.Nil(t, uc, "no UserContext should be set without auth header")
			w.WriteHeader(http.StatusOK)
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestBearerMiddleware_InvalidTokenReturns401(t *testing.T) {
	srv := newOAuthTestServer(t)

	handler := bearerTokenMiddleware(srv.app.Config, srv.app.Storage.InternalStore())(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("handler should not be called for invalid token")
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer totally-not-a-jwt")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, "Bearer", rec.Header().Get("WWW-Authenticate"))
}

// --- requireNavexaContext with OAuth2 configured ---

func TestRequireNavexaContext_Returns401WithWWWAuthenticate(t *testing.T) {
	srv := newOAuthTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	result := srv.requireNavexaContext(rec, req)
	assert.False(t, result)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Header().Get("WWW-Authenticate"), "Bearer")
	assert.Contains(t, rec.Header().Get("WWW-Authenticate"), ".well-known/oauth-protected-resource")
}

func TestRequireNavexaContext_Returns400WithoutOAuth(t *testing.T) {
	srv := newOAuthTestServerNoIssuer(t)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	result := srv.requireNavexaContext(rec, req)
	assert.False(t, result)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// --- Authorize POST: wrong credentials ---

func TestOAuthAuthorizePOST_WrongPassword(t *testing.T) {
	srv := newOAuthTestServer(t)
	clientID, _ := registerTestOAuthClient(t, srv)
	createOAuthTestUser(t, srv, "bad_pw_user", "bp@example.com", "correct")
	_, challenge := pkceChallenge()

	form := url.Values{
		"client_id": {clientID}, "redirect_uri": {"http://localhost:3000/callback"},
		"response_type": {"code"}, "code_challenge": {challenge},
		"code_challenge_method": {"S256"}, "state": {"s"},
		"email": {"bp@example.com"}, "password": {"wrong"},
	}
	req := httptest.NewRequest(http.MethodPost, "/oauth/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.handleOAuthAuthorize(rec, req)

	// Should re-render the form with an error, not redirect
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Invalid email or password")
}

func TestOAuthAuthorizePOST_MissingCredentials(t *testing.T) {
	srv := newOAuthTestServer(t)
	clientID, _ := registerTestOAuthClient(t, srv)
	_, challenge := pkceChallenge()

	form := url.Values{
		"client_id": {clientID}, "redirect_uri": {"http://localhost:3000/callback"},
		"response_type": {"code"}, "code_challenge": {challenge},
		"code_challenge_method": {"S256"}, "state": {"s"},
	}
	req := httptest.NewRequest(http.MethodPost, "/oauth/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.handleOAuthAuthorize(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Email and password are required")
}

func TestOAuthToken_UnsupportedGrantType(t *testing.T) {
	srv := newOAuthTestServer(t)
	form := url.Values{"grant_type": {"client_credentials"}}
	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.handleOAuthToken(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "unsupported_grant_type")
}

func TestOAuthToken_RedirectURIMismatch(t *testing.T) {
	srv := newOAuthTestServer(t)
	clientID, clientSecret := registerTestOAuthClient(t, srv)
	createOAuthTestUser(t, srv, "redir_u", "redir@example.com", "pass")
	verifier, challenge := pkceChallenge()

	// Get code with correct redirect_uri
	form := url.Values{
		"client_id": {clientID}, "redirect_uri": {"http://localhost:3000/callback"},
		"response_type": {"code"}, "code_challenge": {challenge},
		"code_challenge_method": {"S256"}, "state": {"s"},
		"email": {"redir@example.com"}, "password": {"pass"},
	}
	req := httptest.NewRequest(http.MethodPost, "/oauth/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.handleOAuthAuthorize(rec, req)
	loc, _ := url.Parse(rec.Header().Get("Location"))
	code := loc.Query().Get("code")

	// Use different redirect_uri at token endpoint
	tokenForm := url.Values{
		"grant_type": {"authorization_code"}, "code": {code},
		"client_id": {clientID}, "client_secret": {clientSecret},
		"redirect_uri": {"http://evil.example.com/callback"}, "code_verifier": {verifier},
	}
	req = httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(tokenForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	srv.handleOAuthToken(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "redirect_uri")
}
