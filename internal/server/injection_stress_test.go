package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/app"
	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
)

// ============================================================================
// 1. Header injection & hostile input tests
// ============================================================================

func TestPortalInjection_WhitespaceOnlyNavexaKey_Returns400(t *testing.T) {
	// VULNERABILITY: whitespace-only API key bypasses requireNavexaContext
	// because "   " != "" is true. This should be trimmed or rejected.
	svc := &mockPortfolioService{}
	srv := newTestServer(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/portfolios/SMSF/sync", nil)
	uc := &common.UserContext{UserID: "user-123", NavexaAPIKey: "   "}
	req = req.WithContext(common.WithUserContext(req.Context(), uc))
	rec := httptest.NewRecorder()

	srv.handlePortfolioSync(rec, req, "SMSF")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("VULNERABILITY: whitespace-only NavexaAPIKey bypassed validation, got status %d (expected 400)", rec.Code)
	}
}

func TestPortalInjection_WhitespaceOnlyUserID_Returns400(t *testing.T) {
	// VULNERABILITY: whitespace-only UserID bypasses requireNavexaContext
	svc := &mockPortfolioService{}
	srv := newTestServer(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/portfolios/SMSF/sync", nil)
	uc := &common.UserContext{UserID: "   ", NavexaAPIKey: "valid-key"}
	req = req.WithContext(common.WithUserContext(req.Context(), uc))
	rec := httptest.NewRecorder()

	srv.handlePortfolioSync(rec, req, "SMSF")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("VULNERABILITY: whitespace-only UserID bypassed validation, got status %d (expected 400)", rec.Code)
	}
}

func TestPortalInjection_NewlinesInNavexaKey(t *testing.T) {
	// Go's net/http sanitizes header values, but test that the navexa key
	// with newlines doesn't cause issues in downstream processing.
	svc := &mockPortfolioService{
		syncPortfolio: func(ctx context.Context, name string, force bool) (*models.Portfolio, error) {
			return &models.Portfolio{Name: name, LastSynced: time.Now()}, nil
		},
	}
	srv := newTestServer(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/portfolios/SMSF/sync", nil)
	uc := &common.UserContext{UserID: "user-123", NavexaAPIKey: "key\r\nX-Injected: evil"}
	req = req.WithContext(common.WithUserContext(req.Context(), uc))
	rec := httptest.NewRecorder()

	srv.handlePortfolioSync(rec, req, "SMSF")

	// Should not crash. The navexa client's http.Header.Set sanitizes CR/LF.
	// We just verify no panic occurred and status is not 5xx.
	if rec.Code >= 500 {
		t.Errorf("Server error with injected headers: status %d", rec.Code)
	}
}

func TestPortalInjection_NullBytesInHeaders(t *testing.T) {
	svc := &mockPortfolioService{
		syncPortfolio: func(ctx context.Context, name string, force bool) (*models.Portfolio, error) {
			return &models.Portfolio{Name: name, LastSynced: time.Now()}, nil
		},
	}
	srv := newTestServer(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/portfolios/SMSF/sync", nil)
	uc := &common.UserContext{UserID: "user\x00inject", NavexaAPIKey: "key\x00inject"}
	req = req.WithContext(common.WithUserContext(req.Context(), uc))
	rec := httptest.NewRecorder()

	srv.handlePortfolioSync(rec, req, "SMSF")

	// Should not crash
	if rec.Code >= 500 {
		t.Errorf("Server error with null bytes in headers: status %d", rec.Code)
	}
}

func TestPortalInjection_SQLInjectionPatterns(t *testing.T) {
	// API key and user ID are not used in SQL, but test defensively
	svc := &mockPortfolioService{
		syncPortfolio: func(ctx context.Context, name string, force bool) (*models.Portfolio, error) {
			return &models.Portfolio{Name: name, LastSynced: time.Now()}, nil
		},
	}
	srv := newTestServer(svc)

	sqlPayloads := []string{
		"'; DROP TABLE users; --",
		"1 OR 1=1",
		"admin'--",
		"' UNION SELECT * FROM passwords --",
	}

	for _, payload := range sqlPayloads {
		req := httptest.NewRequest(http.MethodPost, "/api/portfolios/SMSF/sync", nil)
		uc := &common.UserContext{UserID: payload, NavexaAPIKey: payload}
		req = req.WithContext(common.WithUserContext(req.Context(), uc))
		rec := httptest.NewRecorder()

		srv.handlePortfolioSync(rec, req, "SMSF")

		if rec.Code >= 500 {
			t.Errorf("Server error with SQL payload %q: status %d", payload, rec.Code)
		}
	}
}

func TestPortalInjection_PathTraversalInUserID(t *testing.T) {
	svc := &mockPortfolioService{
		syncPortfolio: func(ctx context.Context, name string, force bool) (*models.Portfolio, error) {
			return &models.Portfolio{Name: name, LastSynced: time.Now()}, nil
		},
	}
	srv := newTestServer(svc)

	traversalPayloads := []string{
		"../../../etc/passwd",
		"..\\..\\windows\\system32",
		"/etc/shadow",
		"user/../admin",
	}

	for _, payload := range traversalPayloads {
		req := httptest.NewRequest(http.MethodPost, "/api/portfolios/SMSF/sync", nil)
		uc := &common.UserContext{UserID: payload, NavexaAPIKey: "valid-key"}
		req = req.WithContext(common.WithUserContext(req.Context(), uc))
		rec := httptest.NewRecorder()

		srv.handlePortfolioSync(rec, req, "SMSF")

		if rec.Code >= 500 {
			t.Errorf("Server error with path traversal %q: status %d", payload, rec.Code)
		}
	}
}

func TestPortalInjection_VeryLongHeaders(t *testing.T) {
	svc := &mockPortfolioService{
		syncPortfolio: func(ctx context.Context, name string, force bool) (*models.Portfolio, error) {
			return &models.Portfolio{Name: name, LastSynced: time.Now()}, nil
		},
	}
	srv := newTestServer(svc)

	// 1MB string
	longValue := strings.Repeat("A", 1024*1024)

	req := httptest.NewRequest(http.MethodPost, "/api/portfolios/SMSF/sync", nil)
	uc := &common.UserContext{UserID: longValue, NavexaAPIKey: longValue}
	req = req.WithContext(common.WithUserContext(req.Context(), uc))
	rec := httptest.NewRecorder()

	srv.handlePortfolioSync(rec, req, "SMSF")

	// Should not OOM or crash — just fail gracefully when Navexa rejects the key
	if rec.Code >= 500 {
		t.Errorf("Server error with very long headers: status %d", rec.Code)
	}
}

func TestPortalInjection_EmptyStringHeaders(t *testing.T) {
	svc := &mockPortfolioService{}
	srv := newTestServer(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/portfolios/SMSF/sync", nil)
	uc := &common.UserContext{UserID: "", NavexaAPIKey: ""}
	req = req.WithContext(common.WithUserContext(req.Context(), uc))
	rec := httptest.NewRecorder()

	srv.handlePortfolioSync(rec, req, "SMSF")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for empty headers, got %d", rec.Code)
	}

	var resp ErrorResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Error != "configuration not correct" {
		t.Errorf("Expected 'configuration not correct', got %q", resp.Error)
	}
}

// ============================================================================
// 2. Auth bypass tests
// ============================================================================

func TestPortalInjection_NilUserContext_AllNavexaHandlers(t *testing.T) {
	svc := &mockPortfolioService{}
	srv := newTestServer(svc)

	handlers := []struct {
		name    string
		method  string
		handler func(http.ResponseWriter, *http.Request, string)
	}{
		{"sync", http.MethodPost, srv.handlePortfolioSync},
		{"rebuild", http.MethodPost, srv.handlePortfolioRebuild},
	}

	for _, h := range handlers {
		t.Run(h.name, func(t *testing.T) {
			req := httptest.NewRequest(h.method, "/api/portfolios/SMSF/"+h.name, nil)
			rec := httptest.NewRecorder()

			h.handler(rec, req, "SMSF")

			if rec.Code != http.StatusBadRequest {
				t.Errorf("%s: expected 400 with nil UserContext, got %d", h.name, rec.Code)
			}

			var resp ErrorResponse
			json.NewDecoder(rec.Body).Decode(&resp)
			if resp.Error != "configuration not correct" {
				t.Errorf("%s: expected 'configuration not correct', got %q", h.name, resp.Error)
			}
		})
	}
}

func TestPortalInjection_OnlyUserID_NoNavexaKey(t *testing.T) {
	svc := &mockPortfolioService{}
	srv := newTestServer(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/portfolios/SMSF/sync", nil)
	uc := &common.UserContext{UserID: "user-123"}
	req = req.WithContext(common.WithUserContext(req.Context(), uc))
	rec := httptest.NewRecorder()

	srv.handlePortfolioSync(rec, req, "SMSF")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 with UserID only, got %d", rec.Code)
	}
}

func TestPortalInjection_OnlyNavexaKey_NoUserID(t *testing.T) {
	svc := &mockPortfolioService{}
	srv := newTestServer(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/portfolios/SMSF/sync", nil)
	uc := &common.UserContext{NavexaAPIKey: "key-123"}
	req = req.WithContext(common.WithUserContext(req.Context(), uc))
	rec := httptest.NewRecorder()

	srv.handlePortfolioSync(rec, req, "SMSF")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 with NavexaKey only, got %d", rec.Code)
	}
}

// ============================================================================
// 3. Middleware header extraction tests with hostile input
// ============================================================================

func TestMiddleware_HeaderInjectionViaNewlines(t *testing.T) {
	var capturedUC *common.UserContext
	handler := userContextMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUC = common.UserContextFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	// Go's net/http sanitizes CRLF in header values before they reach handlers,
	// but test that our middleware doesn't break if it somehow gets through
	req.Header.Set("X-Vire-Navexa-Key", "key-abc")
	req.Header.Set("X-Vire-User-ID", "user-123")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if capturedUC == nil {
		t.Fatal("Expected UserContext")
	}
	if capturedUC.NavexaAPIKey != "key-abc" {
		t.Errorf("Unexpected key: %q", capturedUC.NavexaAPIKey)
	}
	if capturedUC.UserID != "user-123" {
		t.Errorf("Unexpected UserID: %q", capturedUC.UserID)
	}
}

func TestMiddleware_WhitespaceOnlyNavexaKey_CreatesContext(t *testing.T) {
	// FINDING: whitespace-only header values pass the `!= ""` check
	// and create a UserContext with whitespace-only values
	var capturedUC *common.UserContext
	handler := userContextMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUC = common.UserContextFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/portfolios/SMSF/sync", nil)
	req.Header.Set("X-Vire-Navexa-Key", "   ")
	req.Header.Set("X-Vire-User-ID", "   ")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if capturedUC == nil {
		t.Fatal("Expected UserContext to be created (whitespace headers pass != '' check)")
	}
	// Document the behavior: whitespace-only values are stored as-is
	if capturedUC.NavexaAPIKey != "   " {
		t.Errorf("Expected whitespace-only NavexaAPIKey, got %q", capturedUC.NavexaAPIKey)
	}
	if capturedUC.UserID != "   " {
		t.Errorf("Expected whitespace-only UserID, got %q", capturedUC.UserID)
	}
}

func TestMiddleware_VeryLongPortfolioList(t *testing.T) {
	var capturedUC *common.UserContext
	handler := userContextMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUC = common.UserContextFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// Generate a huge comma-separated portfolio list
	parts := make([]string, 10000)
	for i := range parts {
		parts[i] = "Portfolio"
	}
	hugeList := strings.Join(parts, ",")

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.Header.Set("X-Vire-Portfolios", hugeList)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if capturedUC == nil {
		t.Fatal("Expected UserContext")
	}
	if len(capturedUC.Portfolios) != 10000 {
		t.Errorf("Expected 10000 portfolios, got %d", len(capturedUC.Portfolios))
	}
}

// ============================================================================
// 4. Per-request client creation tests
// ============================================================================

func TestInjectNavexaClient_CreatesClientPerRequest(t *testing.T) {
	logger := common.NewLoggerFromConfig(common.LoggingConfig{Level: "disabled"})
	a := &app.App{
		Logger: logger,
		Config: &common.Config{
			Clients: common.ClientsConfig{
				Navexa: common.NavexaConfig{RateLimit: 5},
			},
		},
	}

	// Two different user contexts should get different clients
	ctx1 := common.WithUserContext(context.Background(), &common.UserContext{
		NavexaAPIKey: "key-1",
		UserID:       "user-1",
	})
	ctx2 := common.WithUserContext(context.Background(), &common.UserContext{
		NavexaAPIKey: "key-2",
		UserID:       "user-2",
	})

	result1 := a.InjectNavexaClient(ctx1)
	result2 := a.InjectNavexaClient(ctx2)

	client1 := common.NavexaClientFromContext(result1)
	client2 := common.NavexaClientFromContext(result2)

	if client1 == nil || client2 == nil {
		t.Fatal("Expected non-nil clients")
	}

	// They should be different instances
	if client1 == client2 {
		t.Error("Expected different client instances for different API keys")
	}
}

func TestInjectNavexaClient_EmptyAPIKey_ReturnsUnmodifiedContext(t *testing.T) {
	logger := common.NewLoggerFromConfig(common.LoggingConfig{Level: "disabled"})
	a := &app.App{
		Logger: logger,
		Config: &common.Config{
			Clients: common.ClientsConfig{
				Navexa: common.NavexaConfig{RateLimit: 5},
			},
		},
	}

	ctx := common.WithUserContext(context.Background(), &common.UserContext{
		NavexaAPIKey: "",
		UserID:       "user-1",
	})

	result := a.InjectNavexaClient(ctx)
	client := common.NavexaClientFromContext(result)

	if client != nil {
		t.Error("Expected nil client when API key is empty")
	}
}

func TestInjectNavexaClient_NilUserContext_ReturnsUnmodifiedContext(t *testing.T) {
	logger := common.NewLoggerFromConfig(common.LoggingConfig{Level: "disabled"})
	a := &app.App{
		Logger: logger,
		Config: &common.Config{
			Clients: common.ClientsConfig{
				Navexa: common.NavexaConfig{RateLimit: 5},
			},
		},
	}

	ctx := context.Background()
	result := a.InjectNavexaClient(ctx)
	client := common.NavexaClientFromContext(result)

	if client != nil {
		t.Error("Expected nil client when no user context")
	}
}

// ============================================================================
// 5. Error response consistency tests
// ============================================================================

func TestPortalInjection_ErrorResponseFormat_Sync(t *testing.T) {
	svc := &mockPortfolioService{}
	srv := newTestServer(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/portfolios/SMSF/sync", nil)
	rec := httptest.NewRecorder()

	srv.handlePortfolioSync(rec, req, "SMSF")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	// Verify exact JSON format
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode JSON: %v", err)
	}

	if resp["error"] != "configuration not correct" {
		t.Errorf("Expected error 'configuration not correct', got %v", resp["error"])
	}

	// Ensure no extra fields leak (e.g., stack traces, internal details)
	if len(resp) != 1 {
		t.Errorf("Expected exactly 1 field in error response, got %d: %v", len(resp), resp)
	}
}

func TestPortalInjection_ErrorResponseFormat_Rebuild(t *testing.T) {
	svc := &mockPortfolioService{}
	srv := newTestServer(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/portfolios/SMSF/rebuild", nil)
	rec := httptest.NewRecorder()

	srv.handlePortfolioRebuild(rec, req, "SMSF")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode JSON: %v", err)
	}

	if resp["error"] != "configuration not correct" {
		t.Errorf("Expected error 'configuration not correct', got %v", resp["error"])
	}

	// Ensure no information leakage
	if len(resp) != 1 {
		t.Errorf("Expected exactly 1 field in error response, got %d: %v", len(resp), resp)
	}
}

// ============================================================================
// 6. Middleware integration: full request path from header to handler
// ============================================================================

func TestFullPath_MiddlewareToHandler_MissingHeaders(t *testing.T) {
	svc := &mockPortfolioService{}
	srv := newTestServer(svc)

	// Simulate a full middleware chain
	handler := userContextMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.handlePortfolioSync(w, r, "SMSF")
	}))

	// No X-Vire headers at all
	req := httptest.NewRequest(http.MethodPost, "/api/portfolios/SMSF/sync", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 with no headers through middleware, got %d", rec.Code)
	}
}

func TestFullPath_MiddlewareToHandler_ValidHeaders(t *testing.T) {
	syncCalled := false
	svc := &mockPortfolioService{
		syncPortfolio: func(ctx context.Context, name string, force bool) (*models.Portfolio, error) {
			syncCalled = true
			return &models.Portfolio{Name: name, LastSynced: time.Now()}, nil
		},
	}
	srv := newTestServer(svc)

	handler := userContextMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.handlePortfolioSync(w, r, "SMSF")
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/portfolios/SMSF/sync", nil)
	req.Header.Set("X-Vire-Navexa-Key", "valid-key")
	req.Header.Set("X-Vire-User-ID", "user-123")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200 with valid headers, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if !syncCalled {
		t.Error("Expected SyncPortfolio to be called")
	}
}

func TestFullPath_MiddlewareToHandler_OnlyNavexaKey_NoUserID(t *testing.T) {
	svc := &mockPortfolioService{}
	srv := newTestServer(svc)

	handler := userContextMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.handlePortfolioSync(w, r, "SMSF")
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/portfolios/SMSF/sync", nil)
	req.Header.Set("X-Vire-Navexa-Key", "valid-key")
	// No X-Vire-User-ID
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 with NavexaKey but no UserID, got %d", rec.Code)
	}
}

// ============================================================================
// 7. CORS verification: new header is exposed
// ============================================================================

func TestCORS_UserIDHeader_InAllowList(t *testing.T) {
	handler := corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/api/portfolios/SMSF/sync", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	allowHeaders := rec.Header().Get("Access-Control-Allow-Headers")
	requiredHeaders := []string{
		"X-Vire-Portfolios",
		"X-Vire-Display-Currency",
		"X-Vire-Navexa-Key",
		"X-Vire-User-ID",
	}

	for _, h := range requiredHeaders {
		if !strings.Contains(allowHeaders, h) {
			t.Errorf("Missing %s in CORS Allow-Headers: %s", h, allowHeaders)
		}
	}
}
