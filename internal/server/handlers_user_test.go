package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/bobmcallan/vire/internal/app"
	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/bobmcallan/vire/internal/storage"
)

// newTestServerWithStorage creates a test server backed by real file storage.
func newTestServerWithStorage(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	logger := common.NewLoggerFromConfig(common.LoggingConfig{Level: "disabled"})
	cfg := common.NewDefaultConfig()
	cfg.Storage.UserData = common.FileConfig{Path: filepath.Join(dir, "user"), Versions: 0}
	cfg.Storage.Data = common.FileConfig{Path: filepath.Join(dir, "data"), Versions: 0}

	mgr, err := storage.NewStorageManager(logger, cfg)
	if err != nil {
		t.Fatalf("NewStorageManager failed: %v", err)
	}
	t.Cleanup(func() { mgr.Close() })

	a := &app.App{
		Config:  cfg,
		Logger:  logger,
		Storage: mgr,
	}
	return &Server{app: a, logger: logger}
}

func jsonBody(t *testing.T, v interface{}) *bytes.Buffer {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal JSON: %v", err)
	}
	return bytes.NewBuffer(data)
}

// createTestUser is a helper that creates a user via the handler.
func createTestUser(t *testing.T, srv *Server, username, email, password, role string) {
	t.Helper()
	body := jsonBody(t, map[string]string{
		"username": username,
		"email":    email,
		"password": password,
		"role":     role,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/users", body)
	rec := httptest.NewRecorder()
	srv.handleUserCreate(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("createTestUser: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleUserCreate_Success(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]string{
		"username": "alice",
		"email":    "alice@example.com",
		"password": "secretpass",
		"role":     "admin",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/users", body)
	rec := httptest.NewRecorder()
	srv.handleUserCreate(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp["status"] != "ok" {
		t.Errorf("expected status 'ok', got %v", resp["status"])
	}
	data := resp["data"].(map[string]interface{})
	if data["username"] != "alice" {
		t.Errorf("expected username 'alice', got %v", data["username"])
	}
	if data["email"] != "alice@example.com" {
		t.Errorf("expected email 'alice@example.com', got %v", data["email"])
	}
	if data["role"] != "admin" {
		t.Errorf("expected role 'admin', got %v", data["role"])
	}
}

func TestHandleUserCreate_MissingUsername(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]string{
		"password": "secret",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/users", body)
	rec := httptest.NewRecorder()
	srv.handleUserCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandleUserCreate_MissingPassword(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]string{
		"username": "alice",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/users", body)
	rec := httptest.NewRecorder()
	srv.handleUserCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandleUserCreate_DuplicateUsername(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "a@x.com", "pass1", "user")

	body := jsonBody(t, map[string]string{
		"username": "alice",
		"password": "pass2",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/users", body)
	rec := httptest.NewRecorder()
	srv.handleUserCreate(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409 for duplicate, got %d", rec.Code)
	}
}

func TestHandleUserGet_Success(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "alice@x.com", "pass", "admin")

	req := httptest.NewRequest(http.MethodGet, "/api/users/alice", nil)
	rec := httptest.NewRecorder()
	srv.handleUserGet(rec, req, "alice")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	data := resp["data"].(map[string]interface{})

	if data["username"] != "alice" {
		t.Errorf("expected username 'alice', got %v", data["username"])
	}

	// password_hash should never appear in response
	if _, exists := data["password_hash"]; exists {
		t.Error("password_hash should not appear in response")
	}
}

func TestHandleUserGet_NotFound(t *testing.T) {
	srv := newTestServerWithStorage(t)

	req := httptest.NewRequest(http.MethodGet, "/api/users/nobody", nil)
	rec := httptest.NewRecorder()
	srv.handleUserGet(rec, req, "nobody")

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestHandleUserGet_NavexaKeyNeverExposed(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "alice@x.com", "pass", "user")

	// Directly set navexa_key via storage
	ctx := context.Background()
	user, _ := srv.app.Storage.UserStorage().GetUser(ctx, "alice")
	user.NavexaKey = "nk-secret-api-key-12345678"
	srv.app.Storage.UserStorage().SaveUser(ctx, user)

	req := httptest.NewRequest(http.MethodGet, "/api/users/alice", nil)
	rec := httptest.NewRecorder()
	srv.handleUserGet(rec, req, "alice")

	body := rec.Body.String()
	// Full navexa_key should not appear in the response
	if bytes.Contains([]byte(body), []byte("nk-secret-api-key-12345678")) {
		t.Error("full navexa_key should not appear in response")
	}

	var resp map[string]interface{}
	json.NewDecoder(bytes.NewBufferString(body)).Decode(&resp)
	data := resp["data"].(map[string]interface{})

	if data["navexa_key_set"] != true {
		t.Error("expected navexa_key_set to be true")
	}
	if data["navexa_key_preview"] != "****5678" {
		t.Errorf("expected navexa_key_preview '****5678', got %v", data["navexa_key_preview"])
	}
}

func TestHandleUserUpdate_Success(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "alice@old.com", "pass", "user")

	body := jsonBody(t, map[string]interface{}{
		"email": "alice@new.com",
		"role":  "admin",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/users/alice", body)
	rec := httptest.NewRecorder()
	srv.handleUserUpdate(rec, req, "alice")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	data := resp["data"].(map[string]interface{})
	if data["email"] != "alice@new.com" {
		t.Errorf("expected updated email, got %v", data["email"])
	}
	if data["role"] != "admin" {
		t.Errorf("expected updated role, got %v", data["role"])
	}
}

func TestHandleUserUpdate_NotFound(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]interface{}{
		"email": "new@x.com",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/users/nobody", body)
	rec := httptest.NewRecorder()
	srv.handleUserUpdate(rec, req, "nobody")

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestHandleUserUpdate_PartialUpdate(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "alice@x.com", "pass", "user")

	// Only update navexa_key, leave everything else unchanged
	body := jsonBody(t, map[string]interface{}{
		"navexa_key": "nk-new-key-1234",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/users/alice", body)
	rec := httptest.NewRecorder()
	srv.handleUserUpdate(rec, req, "alice")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify original fields are preserved
	ctx := context.Background()
	user, _ := srv.app.Storage.UserStorage().GetUser(ctx, "alice")
	if user.Email != "alice@x.com" {
		t.Errorf("expected email preserved, got %q", user.Email)
	}
	if user.Role != "user" {
		t.Errorf("expected role preserved, got %q", user.Role)
	}
	if user.NavexaKey != "nk-new-key-1234" {
		t.Errorf("expected navexa_key updated, got %q", user.NavexaKey)
	}
}

func TestHandleUserDelete_Success(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "a@x.com", "pass", "user")

	req := httptest.NewRequest(http.MethodDelete, "/api/users/alice", nil)
	rec := httptest.NewRecorder()
	srv.handleUserDelete(rec, req, "alice")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Verify user is gone
	_, err := srv.app.Storage.UserStorage().GetUser(context.Background(), "alice")
	if err == nil {
		t.Error("expected user to be deleted")
	}
}

func TestHandleUserDelete_NotFound(t *testing.T) {
	srv := newTestServerWithStorage(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/users/nobody", nil)
	rec := httptest.NewRecorder()
	srv.handleUserDelete(rec, req, "nobody")

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestHandleUserImport_Success(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]interface{}{
		"users": []map[string]string{
			{"username": "alice", "email": "a@x.com", "password": "pass1", "role": "admin"},
			{"username": "bob", "email": "b@x.com", "password": "pass2", "role": "user"},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/users/import", body)
	rec := httptest.NewRecorder()
	srv.handleUserImport(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	data := resp["data"].(map[string]interface{})
	if data["imported"] != float64(2) {
		t.Errorf("expected 2 imported, got %v", data["imported"])
	}
	if data["skipped"] != float64(0) {
		t.Errorf("expected 0 skipped, got %v", data["skipped"])
	}
}

func TestHandleUserImport_Idempotent(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "a@x.com", "pass", "admin")

	body := jsonBody(t, map[string]interface{}{
		"users": []map[string]string{
			{"username": "alice", "email": "new@x.com", "password": "newpass", "role": "user"},
			{"username": "bob", "email": "b@x.com", "password": "pass2", "role": "user"},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/users/import", body)
	rec := httptest.NewRecorder()
	srv.handleUserImport(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	data := resp["data"].(map[string]interface{})
	if data["imported"] != float64(1) {
		t.Errorf("expected 1 imported, got %v", data["imported"])
	}
	if data["skipped"] != float64(1) {
		t.Errorf("expected 1 skipped, got %v", data["skipped"])
	}

	// Verify alice was NOT overwritten
	ctx := context.Background()
	user, _ := srv.app.Storage.UserStorage().GetUser(ctx, "alice")
	if user.Email != "a@x.com" {
		t.Errorf("expected alice's email unchanged, got %q", user.Email)
	}
}

func TestHandleAuthLogin_Success(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "alice@x.com", "correctpassword", "admin")

	body := jsonBody(t, map[string]string{
		"username": "alice",
		"password": "correctpassword",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
	rec := httptest.NewRecorder()
	srv.handleAuthLogin(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Errorf("expected status 'ok', got %v", resp["status"])
	}
	data := resp["data"].(map[string]interface{})
	user := data["user"].(map[string]interface{})
	if user["username"] != "alice" {
		t.Errorf("expected username 'alice', got %v", user["username"])
	}
}

func TestHandleAuthLogin_WrongPassword(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "alice@x.com", "correctpassword", "admin")

	body := jsonBody(t, map[string]string{
		"username": "alice",
		"password": "wrongpassword",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
	rec := httptest.NewRecorder()
	srv.handleAuthLogin(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestHandleAuthLogin_UserNotFound(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]string{
		"username": "nobody",
		"password": "pass",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
	rec := httptest.NewRecorder()
	srv.handleAuthLogin(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestHandleAuthLogin_PasswordHashNeverInResponse(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "alice@x.com", "mypassword", "user")

	body := jsonBody(t, map[string]string{
		"username": "alice",
		"password": "mypassword",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
	rec := httptest.NewRecorder()
	srv.handleAuthLogin(rec, req)

	respBody := rec.Body.String()
	if bytes.Contains([]byte(respBody), []byte("$2a$")) {
		t.Error("password hash should never appear in login response")
	}
	if bytes.Contains([]byte(respBody), []byte("password_hash")) {
		t.Error("password_hash field should never appear in login response")
	}
}

func TestRouteUsers_MethodDispatch(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "a@x.com", "pass", "user")

	// GET should work
	req := httptest.NewRequest(http.MethodGet, "/api/users/alice", nil)
	rec := httptest.NewRecorder()
	srv.routeUsers(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("GET expected 200, got %d", rec.Code)
	}

	// DELETE should work
	req = httptest.NewRequest(http.MethodDelete, "/api/users/alice", nil)
	rec = httptest.NewRecorder()
	srv.routeUsers(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("DELETE expected 200, got %d", rec.Code)
	}

	// POST should return 405
	req = httptest.NewRequest(http.MethodPost, "/api/users/alice", nil)
	rec = httptest.NewRecorder()
	srv.routeUsers(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST expected 405, got %d", rec.Code)
	}
}

func TestRouteUsers_EmptyUsername(t *testing.T) {
	srv := newTestServerWithStorage(t)

	req := httptest.NewRequest(http.MethodGet, "/api/users/", nil)
	rec := httptest.NewRecorder()
	srv.routeUsers(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty username, got %d", rec.Code)
	}
}

func TestNavexaKeyPreview(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"abc", "****"},
		{"abcd", "****"},
		{"abcde", "****bcde"},
		{"nk-12345678", "****5678"},
	}
	for _, tt := range tests {
		got := navexaKeyPreview(tt.input)
		if got != tt.expected {
			t.Errorf("navexaKeyPreview(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestUserResponse_NoSecrets(t *testing.T) {
	user := &models.User{
		Username:     "alice",
		Email:        "alice@x.com",
		PasswordHash: "$2a$10$somereallylonghash",
		Role:         "admin",
		NavexaKey:    "nk-secret-key-1234",
	}
	resp := userResponse(user)

	if _, exists := resp["password_hash"]; exists {
		t.Error("password_hash should not be in response")
	}
	if _, exists := resp["navexa_key"]; exists {
		t.Error("navexa_key should not be in response")
	}
	if resp["navexa_key_set"] != true {
		t.Error("expected navexa_key_set = true")
	}
	if resp["navexa_key_preview"] != "****1234" {
		t.Errorf("expected preview '****1234', got %v", resp["navexa_key_preview"])
	}
}

func TestHandleUserUpdate_NewProfileFields(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "alice@x.com", "pass", "user")

	// Update with new profile fields
	body := jsonBody(t, map[string]interface{}{
		"display_currency":  "USD",
		"default_portfolio": "Growth",
		"portfolios":        []string{"Growth", "Income"},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/users/alice", body)
	rec := httptest.NewRecorder()
	srv.handleUserUpdate(rec, req, "alice")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	data := resp["data"].(map[string]interface{})
	if data["display_currency"] != "USD" {
		t.Errorf("expected display_currency=USD, got %v", data["display_currency"])
	}
	if data["default_portfolio"] != "Growth" {
		t.Errorf("expected default_portfolio=Growth, got %v", data["default_portfolio"])
	}
	portfolios := data["portfolios"].([]interface{})
	if len(portfolios) != 2 || portfolios[0] != "Growth" || portfolios[1] != "Income" {
		t.Errorf("expected portfolios=[Growth, Income], got %v", portfolios)
	}

	// Verify stored correctly
	ctx := context.Background()
	user, _ := srv.app.Storage.UserStorage().GetUser(ctx, "alice")
	if user.DisplayCurrency != "USD" {
		t.Errorf("expected stored display_currency=USD, got %q", user.DisplayCurrency)
	}
	if user.DefaultPortfolio != "Growth" {
		t.Errorf("expected stored default_portfolio=Growth, got %q", user.DefaultPortfolio)
	}
	if len(user.Portfolios) != 2 {
		t.Errorf("expected stored 2 portfolios, got %d", len(user.Portfolios))
	}
}

func TestHandleUserGet_IncludesNewFields(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "alice@x.com", "pass", "user")

	// Set new fields via storage
	ctx := context.Background()
	user, _ := srv.app.Storage.UserStorage().GetUser(ctx, "alice")
	user.DisplayCurrency = "USD"
	user.DefaultPortfolio = "SMSF"
	user.Portfolios = []string{"SMSF", "Trading"}
	srv.app.Storage.UserStorage().SaveUser(ctx, user)

	req := httptest.NewRequest(http.MethodGet, "/api/users/alice", nil)
	rec := httptest.NewRecorder()
	srv.handleUserGet(rec, req, "alice")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	data := resp["data"].(map[string]interface{})
	if data["display_currency"] != "USD" {
		t.Errorf("expected display_currency=USD, got %v", data["display_currency"])
	}
	if data["default_portfolio"] != "SMSF" {
		t.Errorf("expected default_portfolio=SMSF, got %v", data["default_portfolio"])
	}
	portfolios := data["portfolios"].([]interface{})
	if len(portfolios) != 2 {
		t.Errorf("expected 2 portfolios, got %d", len(portfolios))
	}
}

func TestHandleUserImport_WithNewFields(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]interface{}{
		"users": []map[string]interface{}{
			{
				"username":          "alice",
				"email":             "a@x.com",
				"password":          "pass1",
				"role":              "admin",
				"display_currency":  "USD",
				"default_portfolio": "Growth",
				"portfolios":        []string{"Growth", "Income"},
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/users/import", body)
	rec := httptest.NewRecorder()
	srv.handleUserImport(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify the user has the new fields stored
	ctx := context.Background()
	user, err := srv.app.Storage.UserStorage().GetUser(ctx, "alice")
	if err != nil {
		t.Fatalf("GetUser failed: %v", err)
	}
	if user.DisplayCurrency != "USD" {
		t.Errorf("expected display_currency=USD, got %q", user.DisplayCurrency)
	}
	if user.DefaultPortfolio != "Growth" {
		t.Errorf("expected default_portfolio=Growth, got %q", user.DefaultPortfolio)
	}
	if len(user.Portfolios) != 2 || user.Portfolios[0] != "Growth" {
		t.Errorf("expected portfolios=[Growth, Income], got %v", user.Portfolios)
	}
}

func TestHandleAuthLogin_IncludesNewFields(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "alice@x.com", "pass", "admin")

	// Set profile fields
	ctx := context.Background()
	user, _ := srv.app.Storage.UserStorage().GetUser(ctx, "alice")
	user.DisplayCurrency = "USD"
	user.DefaultPortfolio = "SMSF"
	user.Portfolios = []string{"SMSF", "Trading"}
	srv.app.Storage.UserStorage().SaveUser(ctx, user)

	body := jsonBody(t, map[string]string{
		"username": "alice",
		"password": "pass",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
	rec := httptest.NewRecorder()
	srv.handleAuthLogin(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	data := resp["data"].(map[string]interface{})
	userData := data["user"].(map[string]interface{})
	if userData["display_currency"] != "USD" {
		t.Errorf("expected display_currency=USD, got %v", userData["display_currency"])
	}
	if userData["default_portfolio"] != "SMSF" {
		t.Errorf("expected default_portfolio=SMSF, got %v", userData["default_portfolio"])
	}
	portfolios := userData["portfolios"].([]interface{})
	if len(portfolios) != 2 {
		t.Errorf("expected 2 portfolios, got %d", len(portfolios))
	}
}

func TestUserResponse_IncludesNewFields(t *testing.T) {
	user := &models.User{
		Username:         "alice",
		Email:            "alice@x.com",
		PasswordHash:     "$2a$10$hash",
		Role:             "admin",
		DisplayCurrency:  "USD",
		DefaultPortfolio: "Growth",
		Portfolios:       []string{"Growth", "Income"},
	}
	resp := userResponse(user)

	if resp["display_currency"] != "USD" {
		t.Errorf("expected display_currency=USD, got %v", resp["display_currency"])
	}
	if resp["default_portfolio"] != "Growth" {
		t.Errorf("expected default_portfolio=Growth, got %v", resp["default_portfolio"])
	}
	portfolios := resp["portfolios"].([]string)
	if len(portfolios) != 2 {
		t.Errorf("expected 2 portfolios, got %d", len(portfolios))
	}
}

// --- Stress tests: hostile inputs, edge cases, security ---

func TestHandleUserImport_EmptyPassword(t *testing.T) {
	// Empty passwords are accepted by import — verify they hash and that login works with ""
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]interface{}{
		"users": []map[string]interface{}{
			{"username": "emptypass", "email": "e@x.com", "password": "", "role": "user"},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/users/import", body)
	rec := httptest.NewRecorder()
	srv.handleUserImport(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// User should exist
	ctx := context.Background()
	user, err := srv.app.Storage.UserStorage().GetUser(ctx, "emptypass")
	if err != nil {
		t.Fatalf("expected user to exist: %v", err)
	}
	// Password hash should be non-empty (bcrypt of empty string)
	if user.PasswordHash == "" {
		t.Error("expected password_hash to be set even for empty password")
	}
}

func TestHandleUserUpdate_ClearPortfoliosToEmptySlice(t *testing.T) {
	// Setting portfolios to [] should clear the list, not leave stale data
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "alice@x.com", "pass", "user")

	// First set portfolios
	body := jsonBody(t, map[string]interface{}{
		"portfolios": []string{"Growth", "Income"},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/users/alice", body)
	rec := httptest.NewRecorder()
	srv.handleUserUpdate(rec, req, "alice")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Now clear them
	body = jsonBody(t, map[string]interface{}{
		"portfolios": []string{},
	})
	req = httptest.NewRequest(http.MethodPut, "/api/users/alice", body)
	rec = httptest.NewRecorder()
	srv.handleUserUpdate(rec, req, "alice")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	ctx := context.Background()
	user, _ := srv.app.Storage.UserStorage().GetUser(ctx, "alice")
	if len(user.Portfolios) != 0 {
		t.Errorf("expected empty portfolios after clearing, got %v", user.Portfolios)
	}
}

func TestHandleUserUpdate_ClearDisplayCurrency(t *testing.T) {
	// Setting display_currency to "" should clear it
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "alice@x.com", "pass", "user")

	// Set display_currency
	body := jsonBody(t, map[string]interface{}{
		"display_currency": "USD",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/users/alice", body)
	rec := httptest.NewRecorder()
	srv.handleUserUpdate(rec, req, "alice")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Clear it
	body = jsonBody(t, map[string]interface{}{
		"display_currency": "",
	})
	req = httptest.NewRequest(http.MethodPut, "/api/users/alice", body)
	rec = httptest.NewRecorder()
	srv.handleUserUpdate(rec, req, "alice")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	ctx := context.Background()
	user, _ := srv.app.Storage.UserStorage().GetUser(ctx, "alice")
	if user.DisplayCurrency != "" {
		t.Errorf("expected empty display_currency after clearing, got %q", user.DisplayCurrency)
	}
}

func TestHandleUserImport_ControlCharUsername(t *testing.T) {
	// HTTP import should reject usernames with control characters
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]interface{}{
		"users": []map[string]interface{}{
			{"username": "evil\x00user", "email": "e@x.com", "password": "pass", "role": "user"},
			{"username": "evil\nuser", "email": "e@x.com", "password": "pass", "role": "user"},
			{"username": "valid", "email": "v@x.com", "password": "pass", "role": "user"},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/users/import", body)
	rec := httptest.NewRecorder()
	srv.handleUserImport(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	data := resp["data"].(map[string]interface{})
	if data["imported"] != float64(1) {
		t.Errorf("expected 1 imported (only 'valid'), got %v", data["imported"])
	}
	if data["skipped"] != float64(2) {
		t.Errorf("expected 2 skipped (control char usernames), got %v", data["skipped"])
	}
}

func TestHandleUserImport_OversizedUsername(t *testing.T) {
	// HTTP import should reject usernames longer than 128 chars
	srv := newTestServerWithStorage(t)

	longName := ""
	for i := 0; i < 200; i++ {
		longName += "a"
	}

	body := jsonBody(t, map[string]interface{}{
		"users": []map[string]interface{}{
			{"username": longName, "email": "e@x.com", "password": "pass", "role": "user"},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/users/import", body)
	rec := httptest.NewRecorder()
	srv.handleUserImport(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	data := resp["data"].(map[string]interface{})
	if data["imported"] != float64(0) {
		t.Errorf("expected 0 imported for oversized username, got %v", data["imported"])
	}
	if data["skipped"] != float64(1) {
		t.Errorf("expected 1 skipped, got %v", data["skipped"])
	}
}

func TestHandleUserImport_EmptyUsersArray(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]interface{}{
		"users": []map[string]interface{}{},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/users/import", body)
	rec := httptest.NewRecorder()
	srv.handleUserImport(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	data := resp["data"].(map[string]interface{})
	if data["imported"] != float64(0) {
		t.Errorf("expected 0 imported, got %v", data["imported"])
	}
}

func TestHandleUserUpdate_PreserveNewFieldsOnPartialUpdate(t *testing.T) {
	// Updating email should NOT wipe display_currency, portfolios, etc.
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "alice@x.com", "pass", "user")

	// Set profile fields
	body := jsonBody(t, map[string]interface{}{
		"display_currency":  "USD",
		"default_portfolio": "SMSF",
		"portfolios":        []string{"SMSF", "Trading"},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/users/alice", body)
	rec := httptest.NewRecorder()
	srv.handleUserUpdate(rec, req, "alice")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Now update only email
	body = jsonBody(t, map[string]interface{}{
		"email": "alice@new.com",
	})
	req = httptest.NewRequest(http.MethodPut, "/api/users/alice", body)
	rec = httptest.NewRecorder()
	srv.handleUserUpdate(rec, req, "alice")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Verify profile fields preserved
	ctx := context.Background()
	user, _ := srv.app.Storage.UserStorage().GetUser(ctx, "alice")
	if user.Email != "alice@new.com" {
		t.Errorf("expected email updated, got %q", user.Email)
	}
	if user.DisplayCurrency != "USD" {
		t.Errorf("expected display_currency preserved, got %q", user.DisplayCurrency)
	}
	if user.DefaultPortfolio != "SMSF" {
		t.Errorf("expected default_portfolio preserved, got %q", user.DefaultPortfolio)
	}
	if len(user.Portfolios) != 2 {
		t.Errorf("expected portfolios preserved, got %v", user.Portfolios)
	}
}

func TestHandleUserGet_EmptyProfileFields(t *testing.T) {
	// GET should return new fields even when they're empty (zero values)
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "alice@x.com", "pass", "user")

	req := httptest.NewRequest(http.MethodGet, "/api/users/alice", nil)
	rec := httptest.NewRecorder()
	srv.handleUserGet(rec, req, "alice")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	data := resp["data"].(map[string]interface{})

	// display_currency and default_portfolio should be present in response (empty string)
	if _, exists := data["display_currency"]; !exists {
		t.Error("expected display_currency key in response even when empty")
	}
	if _, exists := data["default_portfolio"]; !exists {
		t.Error("expected default_portfolio key in response even when empty")
	}
	// portfolios should be present (nil renders as null in JSON)
	if _, exists := data["portfolios"]; !exists {
		t.Error("expected portfolios key in response even when nil")
	}
}

func TestHandleUserImport_LargePayload(t *testing.T) {
	// Import 100 users — verifies no off-by-one or resource exhaustion
	srv := newTestServerWithStorage(t)

	users := make([]map[string]interface{}, 100)
	for i := 0; i < 100; i++ {
		users[i] = map[string]interface{}{
			"username": fmt.Sprintf("user-%03d", i),
			"email":    fmt.Sprintf("user%d@x.com", i),
			"password": "testpass",
			"role":     "user",
		}
	}

	body := jsonBody(t, map[string]interface{}{"users": users})
	req := httptest.NewRequest(http.MethodPost, "/api/users/import", body)
	rec := httptest.NewRecorder()
	srv.handleUserImport(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	data := resp["data"].(map[string]interface{})
	if data["imported"] != float64(100) {
		t.Errorf("expected 100 imported, got %v", data["imported"])
	}
}

func TestHandleAuthLogin_NewFieldsNeverExposeSecrets(t *testing.T) {
	// Login response should contain new fields but never password_hash or navexa_key
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "alice@x.com", "pass", "admin")

	ctx := context.Background()
	user, _ := srv.app.Storage.UserStorage().GetUser(ctx, "alice")
	user.NavexaKey = "nk-super-secret-key-9999"
	user.DisplayCurrency = "USD"
	user.DefaultPortfolio = "Growth"
	user.Portfolios = []string{"Growth"}
	srv.app.Storage.UserStorage().SaveUser(ctx, user)

	body := jsonBody(t, map[string]string{
		"username": "alice",
		"password": "pass",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
	rec := httptest.NewRecorder()
	srv.handleAuthLogin(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	respBody := rec.Body.String()
	// Navexa key must NOT appear in login response
	if bytes.Contains([]byte(respBody), []byte("nk-super-secret-key-9999")) {
		t.Error("full navexa_key should not appear in login response")
	}
	if bytes.Contains([]byte(respBody), []byte("password_hash")) {
		t.Error("password_hash should not appear in login response")
	}

	// But new fields should be present
	var resp map[string]interface{}
	json.NewDecoder(bytes.NewBufferString(respBody)).Decode(&resp)
	data := resp["data"].(map[string]interface{})
	userData := data["user"].(map[string]interface{})
	if userData["display_currency"] != "USD" {
		t.Errorf("expected display_currency=USD, got %v", userData["display_currency"])
	}
}

// --- Middleware navexa_key resolution tests ---

func TestMiddleware_ResolvesNavexaKeyFromUserStorage(t *testing.T) {
	srv := newTestServerWithStorage(t)
	ctx := context.Background()

	// Create a user with a navexa_key
	srv.app.Storage.UserStorage().SaveUser(ctx, &models.User{
		Username:     "user-123",
		Email:        "u@x.com",
		PasswordHash: "hash",
		Role:         "user",
		NavexaKey:    "nk-stored-key-5678",
	})

	var capturedUC *common.UserContext
	handler := userContextMiddleware(srv.app.Storage.UserStorage())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUC = common.UserContextFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// Send request with User-ID but no Navexa-Key header
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.Header.Set("X-Vire-User-ID", "user-123")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if capturedUC == nil {
		t.Fatal("expected UserContext")
	}
	if capturedUC.NavexaAPIKey != "nk-stored-key-5678" {
		t.Errorf("expected navexa key resolved from storage, got %q", capturedUC.NavexaAPIKey)
	}
}

func TestMiddleware_HeaderNavexaKeyTakesPrecedence(t *testing.T) {
	srv := newTestServerWithStorage(t)
	ctx := context.Background()

	// Create a user with a stored navexa_key
	srv.app.Storage.UserStorage().SaveUser(ctx, &models.User{
		Username:     "user-123",
		Email:        "u@x.com",
		PasswordHash: "hash",
		Role:         "user",
		NavexaKey:    "nk-stored-key",
	})

	var capturedUC *common.UserContext
	handler := userContextMiddleware(srv.app.Storage.UserStorage())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUC = common.UserContextFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// Send request with BOTH User-ID and Navexa-Key header
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.Header.Set("X-Vire-User-ID", "user-123")
	req.Header.Set("X-Vire-Navexa-Key", "header-key")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if capturedUC == nil {
		t.Fatal("expected UserContext")
	}
	// Header value should take precedence over stored value
	if capturedUC.NavexaAPIKey != "header-key" {
		t.Errorf("expected header key to take precedence, got %q", capturedUC.NavexaAPIKey)
	}
}

func TestMiddleware_UserNotFoundNoNavexaKey(t *testing.T) {
	srv := newTestServerWithStorage(t)

	var capturedUC *common.UserContext
	handler := userContextMiddleware(srv.app.Storage.UserStorage())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUC = common.UserContextFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// Send request with User-ID but user doesn't exist in storage
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.Header.Set("X-Vire-User-ID", "nonexistent-user")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if capturedUC == nil {
		t.Fatal("expected UserContext")
	}
	if capturedUC.NavexaAPIKey != "" {
		t.Errorf("expected empty navexa key for unknown user, got %q", capturedUC.NavexaAPIKey)
	}
}
