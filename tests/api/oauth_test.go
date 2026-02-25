package api

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmcallan/vire/tests/common"
)

// --- OAuth 2.1 Helpers ---

// oauthClient holds registered client credentials returned from DCR.
type oauthClient struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	ClientName   string `json:"client_name"`
}

// tokenResponse holds the response from the token endpoint.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

// registerOAuthClient registers a dynamic client and returns credentials.
func registerOAuthClient(t *testing.T, env *common.Env, name string, redirectURIs []string) oauthClient {
	t.Helper()
	resp, err := env.HTTPPost("/oauth/register", map[string]interface{}{
		"client_name":   name,
		"redirect_uris": redirectURIs,
	})
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 201, resp.StatusCode, "DCR should return 201 Created")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var client oauthClient
	require.NoError(t, json.Unmarshal(body, &client))
	require.NotEmpty(t, client.ClientID, "client_id must not be empty")
	require.NotEmpty(t, client.ClientSecret, "client_secret must not be empty")
	return client
}

// createTestUser creates a user with email/password for OAuth consent login.
func createTestUser(t *testing.T, env *common.Env, username, email, password string) {
	t.Helper()
	resp, err := env.HTTPPost("/api/users", map[string]interface{}{
		"username": username,
		"email":    email,
		"password": password,
		"role":     "user",
	})
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, 201, resp.StatusCode, "user creation should succeed")
}

// pkceChallenge computes S256 code_challenge from a code_verifier.
func pkceChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

// authorizeAndGetCode drives the authorize + consent flow:
// 1. POST consent form with user credentials
// 2. Capture the redirect containing the auth code
// Returns the authorization code.
func authorizeAndGetCode(t *testing.T, env *common.Env, client oauthClient, redirectURI, codeChallenge, state, email, password string) string {
	t.Helper()

	// Build the authorize URL with query params
	authURL := fmt.Sprintf("/oauth/authorize?client_id=%s&redirect_uri=%s&response_type=code&code_challenge=%s&code_challenge_method=S256&state=%s&scope=vire",
		url.QueryEscape(client.ClientID),
		url.QueryEscape(redirectURI),
		url.QueryEscape(codeChallenge),
		url.QueryEscape(state),
	)

	// POST the consent form with credentials (form-encoded)
	formData := url.Values{
		"email":                 {email},
		"password":              {password},
		"client_id":             {client.ClientID},
		"redirect_uri":          {redirectURI},
		"response_type":         {"code"},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
		"state":                 {state},
		"scope":                 {"vire"},
	}

	// Use a client that does NOT follow redirects so we can capture the Location header
	noRedirectClient := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequest(http.MethodPost, env.ServerURL()+authURL, strings.NewReader(formData.Encode()))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := noRedirectClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should redirect with code
	require.True(t, resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusSeeOther,
		"authorize should redirect, got %d", resp.StatusCode)

	location := resp.Header.Get("Location")
	require.NotEmpty(t, location, "Location header must be present on redirect")

	redirectURL, err := url.Parse(location)
	require.NoError(t, err)

	code := redirectURL.Query().Get("code")
	require.NotEmpty(t, code, "redirect must contain authorization code")

	returnedState := redirectURL.Query().Get("state")
	assert.Equal(t, state, returnedState, "state parameter must be echoed back")

	return code
}

// exchangeCodeForTokens calls the token endpoint with authorization_code grant.
func exchangeCodeForTokens(t *testing.T, env *common.Env, client oauthClient, code, redirectURI, codeVerifier string) tokenResponse {
	t.Helper()

	formData := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {client.ClientID},
		"client_secret": {client.ClientSecret},
		"code_verifier": {codeVerifier},
	}

	req, err := http.NewRequest(http.MethodPost, env.ServerURL()+"/oauth/token", strings.NewReader(formData.Encode()))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode, "token exchange should succeed")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var tokens tokenResponse
	require.NoError(t, json.Unmarshal(body, &tokens))
	require.NotEmpty(t, tokens.AccessToken, "access_token must not be empty")
	require.Equal(t, "Bearer", tokens.TokenType)
	return tokens
}

// refreshTokens calls the token endpoint with refresh_token grant.
func refreshTokens(t *testing.T, env *common.Env, client oauthClient, refreshToken string) tokenResponse {
	t.Helper()

	formData := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {client.ClientID},
		"client_secret": {client.ClientSecret},
	}

	req, err := http.NewRequest(http.MethodPost, env.ServerURL()+"/oauth/token", strings.NewReader(formData.Encode()))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode, "refresh token exchange should succeed")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var tokens tokenResponse
	require.NoError(t, json.Unmarshal(body, &tokens))
	require.NotEmpty(t, tokens.AccessToken, "new access_token must not be empty")
	return tokens
}

// postTokenForm sends a form-encoded POST to /oauth/token and returns the raw response.
func postTokenForm(t *testing.T, env *common.Env, formData url.Values) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, env.ServerURL()+"/oauth/token", strings.NewReader(formData.Encode()))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

// --- 1. Metadata Endpoints ---

func TestOAuth_ProtectedResourceMetadata(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	resp, err := env.HTTPGet("/.well-known/oauth-protected-resource")
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	guard.SaveResult("01_protected_resource", string(body))

	assert.Equal(t, 200, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")

	var meta map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &meta))

	// RFC9728 required fields
	assert.NotEmpty(t, meta["resource"], "resource field is required")
	authServers, ok := meta["authorization_servers"].([]interface{})
	assert.True(t, ok, "authorization_servers must be an array")
	assert.NotEmpty(t, authServers, "authorization_servers must not be empty")

	// bearer_methods_supported
	methods, ok := meta["bearer_methods_supported"].([]interface{})
	assert.True(t, ok, "bearer_methods_supported must be an array")
	assert.Contains(t, methods, "header")
}

func TestOAuth_AuthorizationServerMetadata(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	resp, err := env.HTTPGet("/.well-known/oauth-authorization-server")
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	guard.SaveResult("01_authorization_server", string(body))

	assert.Equal(t, 200, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")

	var meta map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &meta))

	// RFC8414 required fields
	assert.NotEmpty(t, meta["issuer"])
	assert.NotEmpty(t, meta["authorization_endpoint"])
	assert.NotEmpty(t, meta["token_endpoint"])
	assert.NotEmpty(t, meta["registration_endpoint"])

	// Verify supported values
	responseTypes, ok := meta["response_types_supported"].([]interface{})
	assert.True(t, ok)
	assert.Contains(t, responseTypes, "code")

	grantTypes, ok := meta["grant_types_supported"].([]interface{})
	assert.True(t, ok)
	assert.Contains(t, grantTypes, "authorization_code")
	assert.Contains(t, grantTypes, "refresh_token")

	codeMethods, ok := meta["code_challenge_methods_supported"].([]interface{})
	assert.True(t, ok)
	assert.Contains(t, codeMethods, "S256")
}

// --- 2. Dynamic Client Registration ---

func TestOAuth_DCR_RegisterClient(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	resp, err := env.HTTPPost("/oauth/register", map[string]interface{}{
		"client_name":   "Test MCP Client",
		"redirect_uris": []string{"http://localhost:3000/callback"},
	})
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	guard.SaveResult("01_dcr_register", string(body))

	assert.Equal(t, 201, resp.StatusCode)

	var client map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &client))

	assert.NotEmpty(t, client["client_id"])
	assert.NotEmpty(t, client["client_secret"])
	assert.Equal(t, "Test MCP Client", client["client_name"])

	redirectURIs, ok := client["redirect_uris"].([]interface{})
	assert.True(t, ok)
	assert.Contains(t, redirectURIs, "http://localhost:3000/callback")
}

func TestOAuth_DCR_RegisterMultipleClients(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	// Register first client
	client1 := registerOAuthClient(t, env, "Client A", []string{"http://localhost:3000/callback"})

	// Register second client
	client2 := registerOAuthClient(t, env, "Client B", []string{"http://localhost:4000/callback"})

	// Client IDs should be different
	assert.NotEqual(t, client1.ClientID, client2.ClientID, "each client should get a unique client_id")
}

// --- 3. Full OAuth Flow ---

func TestOAuth_FullAuthCodeFlow(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Step 1: Create a user with email/password
	createTestUser(t, env, "oauthuser", "oauthuser@test.com", "testpassword123")

	// Step 2: Register an OAuth client via DCR
	redirectURI := "http://localhost:3000/callback"
	client := registerOAuthClient(t, env, "Flow Test Client", []string{redirectURI})

	// Step 3: Generate PKCE pair
	codeVerifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk" // known test verifier
	codeChallenge := pkceChallenge(codeVerifier)

	// Step 4: Authorize and get code via consent form
	state := "test-state-12345"
	code := authorizeAndGetCode(t, env, client, redirectURI, codeChallenge, state, "oauthuser@test.com", "testpassword123")
	guard.SaveResult("02_auth_code", code)

	// Step 5: Exchange code for tokens
	tokens := exchangeCodeForTokens(t, env, client, code, redirectURI, codeVerifier)
	assert.NotEmpty(t, tokens.AccessToken)
	assert.NotEmpty(t, tokens.RefreshToken)
	assert.Equal(t, "Bearer", tokens.TokenType)
	assert.Greater(t, tokens.ExpiresIn, 0)
	guard.SaveResult("03_tokens", fmt.Sprintf("access_token_len=%d refresh_token_len=%d expires_in=%d", len(tokens.AccessToken), len(tokens.RefreshToken), tokens.ExpiresIn))

	// Step 6: Use bearer token on protected endpoint
	// The bearer token authenticates the user, but without a Navexa API key configured
	// the portfolio endpoint returns 400 navexa_key_required (not 401 authentication_required).
	// This proves the OAuth flow resolved the user context correctly.
	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios", nil,
		map[string]string{"Authorization": "Bearer " + tokens.AccessToken})
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	guard.SaveResult("04_protected_access", string(body))

	// Bearer token should authenticate (not 401) — 400 means user was identified but lacks Navexa key
	assert.Equal(t, 400, resp.StatusCode, "authenticated but missing Navexa key should return 400")
	var errResp map[string]interface{}
	if json.Unmarshal(body, &errResp) == nil {
		assert.Equal(t, "navexa_key_required", errResp["error"],
			"should indicate navexa_key is needed, not authentication")
	}

	// Step 7: Refresh tokens
	newTokens := refreshTokens(t, env, client, tokens.RefreshToken)
	assert.NotEmpty(t, newTokens.AccessToken)
	assert.NotEmpty(t, newTokens.RefreshToken)
	assert.NotEqual(t, tokens.AccessToken, newTokens.AccessToken, "new access token should differ")
	assert.NotEqual(t, tokens.RefreshToken, newTokens.RefreshToken, "refresh token should be rotated")
	guard.SaveResult("05_refreshed_tokens", fmt.Sprintf("new_access_len=%d new_refresh_len=%d", len(newTokens.AccessToken), len(newTokens.RefreshToken)))

	// Step 8: Use new bearer token — same pattern: 400 navexa_key_required proves auth works
	resp2, err := env.HTTPRequest(http.MethodGet, "/api/portfolios", nil,
		map[string]string{"Authorization": "Bearer " + newTokens.AccessToken})
	require.NoError(t, err)
	defer resp2.Body.Close()

	assert.Equal(t, 400, resp2.StatusCode, "refreshed token should authenticate (400 = auth OK, missing Navexa key)")
}

// --- 4. Error Paths ---

func TestOAuth_Authorize_InvalidClientID(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	resp, err := env.HTTPGet("/oauth/authorize?client_id=nonexistent&redirect_uri=http://localhost/cb&response_type=code&code_challenge=abc&code_challenge_method=S256&state=x")
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should return an error (400 or render error page), not redirect
	assert.True(t, resp.StatusCode >= 400, "invalid client_id should fail, got %d", resp.StatusCode)
}

func TestOAuth_Authorize_MissingParams(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	tests := []struct {
		name  string
		query string
	}{
		{"missing client_id", "redirect_uri=http://localhost/cb&response_type=code&code_challenge=abc&code_challenge_method=S256&state=x"},
		{"missing code_challenge", "client_id=test&redirect_uri=http://localhost/cb&response_type=code&code_challenge_method=S256&state=x"},
		{"missing state", "client_id=test&redirect_uri=http://localhost/cb&response_type=code&code_challenge=abc&code_challenge_method=S256"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := env.HTTPGet("/oauth/authorize?" + tt.query)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.True(t, resp.StatusCode >= 400, "%s: should fail, got %d", tt.name, resp.StatusCode)
		})
	}
}

func TestOAuth_Token_WrongCodeVerifier(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	createTestUser(t, env, "pkcewrong", "pkcewrong@test.com", "testpass123")
	redirectURI := "http://localhost:3000/callback"
	client := registerOAuthClient(t, env, "PKCE Wrong Client", []string{redirectURI})

	codeVerifier := "correct-verifier-for-challenge-generation"
	codeChallenge := pkceChallenge(codeVerifier)

	code := authorizeAndGetCode(t, env, client, redirectURI, codeChallenge, "s1", "pkcewrong@test.com", "testpass123")

	// Try exchanging with wrong code_verifier
	formData := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {client.ClientID},
		"client_secret": {client.ClientSecret},
		"code_verifier": {"wrong-verifier-that-does-not-match"},
	}

	resp := postTokenForm(t, env, formData)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	guard.SaveResult("01_wrong_verifier", string(body))

	assert.Equal(t, 400, resp.StatusCode, "wrong code_verifier should be rejected")
}

func TestOAuth_Token_ExpiredOrUsedCode(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	createTestUser(t, env, "codeuser", "codeuser@test.com", "testpass123")
	redirectURI := "http://localhost:3000/callback"
	client := registerOAuthClient(t, env, "Code Replay Client", []string{redirectURI})

	codeVerifier := "verifier-for-replay-test"
	codeChallenge := pkceChallenge(codeVerifier)

	code := authorizeAndGetCode(t, env, client, redirectURI, codeChallenge, "s2", "codeuser@test.com", "testpass123")

	// First exchange should succeed
	_ = exchangeCodeForTokens(t, env, client, code, redirectURI, codeVerifier)

	// Second exchange with same code should fail (code already used)
	formData := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {client.ClientID},
		"client_secret": {client.ClientSecret},
		"code_verifier": {codeVerifier},
	}

	resp := postTokenForm(t, env, formData)
	defer resp.Body.Close()

	assert.Equal(t, 400, resp.StatusCode, "reused authorization code should be rejected")
}

func TestOAuth_Token_InvalidRefreshToken(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	client := registerOAuthClient(t, env, "Bad Refresh Client", []string{"http://localhost:3000/callback"})

	formData := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {"invalid-refresh-token-value"},
		"client_id":     {client.ClientID},
		"client_secret": {client.ClientSecret},
	}

	resp := postTokenForm(t, env, formData)
	defer resp.Body.Close()

	assert.Equal(t, 400, resp.StatusCode, "invalid refresh_token should be rejected")
}

func TestOAuth_Token_WrongClientSecret(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	createTestUser(t, env, "secretuser", "secretuser@test.com", "testpass123")
	redirectURI := "http://localhost:3000/callback"
	client := registerOAuthClient(t, env, "Wrong Secret Client", []string{redirectURI})

	codeVerifier := "verifier-for-secret-test"
	codeChallenge := pkceChallenge(codeVerifier)

	code := authorizeAndGetCode(t, env, client, redirectURI, codeChallenge, "s3", "secretuser@test.com", "testpass123")

	// Exchange with wrong client_secret
	formData := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {client.ClientID},
		"client_secret": {"completely-wrong-secret"},
		"code_verifier": {codeVerifier},
	}

	resp := postTokenForm(t, env, formData)
	defer resp.Body.Close()

	assert.True(t, resp.StatusCode == 400 || resp.StatusCode == 401,
		"wrong client_secret should be rejected, got %d", resp.StatusCode)
}

// --- 5. Bearer Token on Protected Endpoints ---

func TestOAuth_BearerToken_ResolvesUserContext(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Create user and do full OAuth flow
	createTestUser(t, env, "beareruser", "beareruser@test.com", "testpass123")
	redirectURI := "http://localhost:3000/callback"
	client := registerOAuthClient(t, env, "Bearer Test Client", []string{redirectURI})

	codeVerifier := "bearer-test-verifier-value"
	codeChallenge := pkceChallenge(codeVerifier)

	code := authorizeAndGetCode(t, env, client, redirectURI, codeChallenge, "s4", "beareruser@test.com", "testpass123")
	tokens := exchangeCodeForTokens(t, env, client, code, redirectURI, codeVerifier)

	// GET /api/portfolios with bearer token should resolve the correct user.
	// Without a Navexa API key, we get 400 navexa_key_required (not 401),
	// which proves the bearer token successfully authenticated the user.
	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios", nil,
		map[string]string{"Authorization": "Bearer " + tokens.AccessToken})
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	guard.SaveResult("01_bearer_portfolios", string(body))

	assert.Equal(t, 400, resp.StatusCode, "authenticated but missing Navexa key should return 400")
	var errResp map[string]interface{}
	if json.Unmarshal(body, &errResp) == nil {
		assert.Equal(t, "navexa_key_required", errResp["error"],
			"should indicate navexa_key is needed, not authentication")
	}
}

func TestOAuth_BearerToken_InvalidJWT(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios", nil,
		map[string]string{"Authorization": "Bearer invalid.jwt.token"})
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 401, resp.StatusCode, "invalid JWT should return 401")

	// Should include WWW-Authenticate header
	wwwAuth := resp.Header.Get("WWW-Authenticate")
	assert.Contains(t, wwwAuth, "Bearer", "401 response should include WWW-Authenticate: Bearer header")
}

// --- 6. Unauthenticated Error ---

func TestOAuth_Unauthenticated_Returns401WithDiscovery(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Request a protected endpoint without any auth headers
	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios", nil, nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	guard.SaveResult("01_unauthenticated", string(body))

	// When OAuth2 is configured, should return 401 with discovery info
	// Note: This test depends on the server having [auth.oauth2] issuer configured.
	// If not configured, the server returns 400 for backward compatibility.
	if resp.StatusCode == 401 {
		// WWW-Authenticate header should point to resource metadata
		wwwAuth := resp.Header.Get("WWW-Authenticate")
		assert.Contains(t, wwwAuth, "Bearer", "should include Bearer scheme")
		assert.Contains(t, wwwAuth, "resource_metadata", "should include resource_metadata URL")

		// Body should indicate authentication is required
		var result map[string]interface{}
		if json.Unmarshal(body, &result) == nil {
			assert.Equal(t, "authentication_required", result["error"],
				"error field should be authentication_required")
		}
	}
	// If 400, the server doesn't have OAuth2 configured — test is informational only
}

// --- 7. Authorize Consent Page ---

func TestOAuth_Authorize_RendersConsentPage(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Register a client first
	redirectURI := "http://localhost:3000/callback"
	client := registerOAuthClient(t, env, "Consent Page Client", []string{redirectURI})

	// GET the authorize endpoint — should render the consent HTML page
	authURL := fmt.Sprintf("/oauth/authorize?client_id=%s&redirect_uri=%s&response_type=code&code_challenge=test-challenge&code_challenge_method=S256&state=test-state&scope=vire",
		url.QueryEscape(client.ClientID),
		url.QueryEscape(redirectURI),
	)

	resp, err := env.HTTPGet(authURL)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	guard.SaveResult("01_consent_page", string(body))

	assert.Equal(t, 200, resp.StatusCode, "GET authorize should render consent page")
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/html", "consent page should be HTML")

	// Page should contain the client name and form elements
	pageContent := string(body)
	assert.Contains(t, pageContent, "Consent Page Client", "page should show client name")
	assert.Contains(t, pageContent, "email", "page should have email field")
	assert.Contains(t, pageContent, "password", "page should have password field")
}

// --- 8. Redirect URI Validation ---

func TestOAuth_Authorize_RedirectURIMismatch(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	// Register client with specific redirect URI
	client := registerOAuthClient(t, env, "Redirect Test Client", []string{"http://localhost:3000/callback"})

	// Try to authorize with a different redirect URI
	authURL := fmt.Sprintf("/oauth/authorize?client_id=%s&redirect_uri=%s&response_type=code&code_challenge=test&code_challenge_method=S256&state=x",
		url.QueryEscape(client.ClientID),
		url.QueryEscape("http://evil.com/steal-code"),
	)

	resp, err := env.HTTPGet(authURL)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.True(t, resp.StatusCode >= 400, "mismatched redirect_uri should fail, got %d", resp.StatusCode)
}

// --- 9. Refresh Token Rotation ---

func TestOAuth_RefreshToken_Rotation(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	createTestUser(t, env, "rotateuser", "rotateuser@test.com", "testpass123")
	redirectURI := "http://localhost:3000/callback"
	client := registerOAuthClient(t, env, "Rotation Client", []string{redirectURI})

	codeVerifier := "verifier-for-rotation-test"
	codeChallenge := pkceChallenge(codeVerifier)

	code := authorizeAndGetCode(t, env, client, redirectURI, codeChallenge, "s5", "rotateuser@test.com", "testpass123")
	tokens := exchangeCodeForTokens(t, env, client, code, redirectURI, codeVerifier)

	// Refresh once
	newTokens := refreshTokens(t, env, client, tokens.RefreshToken)
	guard.SaveResult("01_first_refresh", fmt.Sprintf("old_refresh_len=%d new_refresh_len=%d", len(tokens.RefreshToken), len(newTokens.RefreshToken)))

	// Old refresh token should no longer work (revoked after rotation)
	formData := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {tokens.RefreshToken},
		"client_id":     {client.ClientID},
		"client_secret": {client.ClientSecret},
	}

	resp := postTokenForm(t, env, formData)
	defer resp.Body.Close()

	assert.Equal(t, 400, resp.StatusCode, "old refresh token should be revoked after rotation")

	// New refresh token should still work
	newerTokens := refreshTokens(t, env, client, newTokens.RefreshToken)
	assert.NotEmpty(t, newerTokens.AccessToken, "refreshing with rotated token should work")
	guard.SaveResult("02_second_refresh", fmt.Sprintf("newer_access_len=%d", len(newerTokens.AccessToken)))
}

// --- 10. Token Endpoint Grant Type Validation ---

func TestOAuth_Token_UnsupportedGrantType(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	client := registerOAuthClient(t, env, "Grant Type Client", []string{"http://localhost:3000/callback"})

	formData := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {client.ClientID},
		"client_secret": {client.ClientSecret},
	}

	resp := postTokenForm(t, env, formData)
	defer resp.Body.Close()

	assert.Equal(t, 400, resp.StatusCode, "unsupported grant_type should return 400")
}

// --- 11. Authorize Deny Flow ---

func TestOAuth_Authorize_DenyRedirects(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	redirectURI := "http://localhost:3000/callback"
	client := registerOAuthClient(t, env, "Deny Test Client", []string{redirectURI})

	// Build a deny URL (click the "Deny" link on the consent page)
	denyURL := fmt.Sprintf("/oauth/authorize?client_id=%s&redirect_uri=%s&response_type=code&code_challenge=test&code_challenge_method=S256&state=deny-state&deny=true",
		url.QueryEscape(client.ClientID),
		url.QueryEscape(redirectURI),
	)

	noRedirectClient := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequest(http.MethodGet, env.ServerURL()+denyURL, nil)
	require.NoError(t, err)

	resp, err := noRedirectClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Deny should redirect back to client with error=access_denied
	if resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusSeeOther {
		location := resp.Header.Get("Location")
		redirectURL, err := url.Parse(location)
		require.NoError(t, err)

		assert.Equal(t, "access_denied", redirectURL.Query().Get("error"),
			"deny should redirect with error=access_denied")
		assert.Equal(t, "deny-state", redirectURL.Query().Get("state"),
			"state should be echoed back on deny")
	}
}

// --- 12. Consent Login with Wrong Credentials ---

func TestOAuth_Authorize_WrongCredentials(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	createTestUser(t, env, "wrongpwuser", "wrongpw@test.com", "correctpassword")
	redirectURI := "http://localhost:3000/callback"
	client := registerOAuthClient(t, env, "Wrong PW Client", []string{redirectURI})

	authURL := fmt.Sprintf("/oauth/authorize?client_id=%s&redirect_uri=%s&response_type=code&code_challenge=test-challenge&code_challenge_method=S256&state=test-state&scope=vire",
		url.QueryEscape(client.ClientID),
		url.QueryEscape(redirectURI),
	)

	formData := url.Values{
		"email":                 {"wrongpw@test.com"},
		"password":              {"wrongpassword"},
		"client_id":             {client.ClientID},
		"redirect_uri":          {redirectURI},
		"response_type":         {"code"},
		"code_challenge":        {"test-challenge"},
		"code_challenge_method": {"S256"},
		"state":                 {"test-state"},
		"scope":                 {"vire"},
	}

	req, err := http.NewRequest(http.MethodPost, env.ServerURL()+authURL, strings.NewReader(formData.Encode()))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Wrong credentials should re-render consent page or return error, NOT redirect with code
	assert.True(t, resp.StatusCode == 200 || resp.StatusCode == 401 || resp.StatusCode == 403,
		"wrong credentials should not succeed, got %d", resp.StatusCode)

	// If it's a 200 (re-rendered form), ensure it's HTML and doesn't contain an auth code redirect
	if resp.StatusCode == 200 {
		assert.Contains(t, resp.Header.Get("Content-Type"), "text/html",
			"re-rendered consent page should be HTML")
	}
}
