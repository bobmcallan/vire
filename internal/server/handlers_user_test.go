package server

import (
	"bytes"
	"context"
	"encoding/json"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/app"
	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/bobmcallan/vire/internal/storage"
	"golang.org/x/crypto/bcrypt"
)

// newTestServerWithStorage creates a test server backed by real storage.
func newTestServerWithStorage(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	logger := common.NewLoggerFromConfig(common.LoggingConfig{Level: "disabled"})
	cfg := common.NewDefaultConfig()
	cfg.Environment = "development"
	cfg.Storage.Address = "ws://localhost:8000/rpc"
	cfg.Storage.Namespace = "test"
	cfg.Storage.Database = "test_" + strconv.FormatInt(time.Now().UnixNano(), 10) + "_" + strconv.Itoa(rand.Intn(10000)) + strconv.Itoa(time.Now().Nanosecond())
	cfg.Storage.DataPath = filepath.Join(dir, "market")

	mgr, err := storage.NewManager(logger, cfg)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
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

// createTestUser is a helper that creates a user directly in the store.
// This bypasses handler-level restrictions (e.g. role is ignored on user
// endpoints) so tests can set up users with any role.
func createTestUser(t *testing.T, srv *Server, username, email, password, role string) {
	t.Helper()
	ctx := context.Background()

	passwordBytes := []byte(password)
	if len(passwordBytes) > 72 {
		passwordBytes = passwordBytes[:72]
	}
	hash, err := bcrypt.GenerateFromPassword(passwordBytes, 10)
	if err != nil {
		t.Fatalf("createTestUser: failed to hash password: %v", err)
	}

	user := &models.InternalUser{
		UserID:       username,
		Email:        email,
		PasswordHash: string(hash),
		Role:         role,
		CreatedAt:    time.Now(),
	}
	if err := srv.app.Storage.InternalStore().SaveUser(ctx, user); err != nil {
		t.Fatalf("createTestUser: failed to save user: %v", err)
	}
}

func TestHandleUserCreate_Success(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]string{
		"username": "alice",
		"email":    "alice@example.com",
		"password": "secretpass",
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
	if data["role"] != "user" {
		t.Errorf("expected role 'user', got %v", data["role"])
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

	// Directly set navexa_key via UserKV storage
	ctx := context.Background()
	srv.app.Storage.InternalStore().SetUserKV(ctx, "alice", "navexa_key", "nk-secret-api-key-12345678")

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
	if data["role"] != "user" {
		t.Errorf("expected role to remain 'user', got %v", data["role"])
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
	user, _ := srv.app.Storage.InternalStore().GetUser(ctx, "alice")
	if user.Email != "alice@x.com" {
		t.Errorf("expected email preserved, got %q", user.Email)
	}
	if user.Role != "user" {
		t.Errorf("expected role preserved, got %q", user.Role)
	}
	// Verify navexa_key was stored as UserKV
	kv, err := srv.app.Storage.InternalStore().GetUserKV(ctx, "alice", "navexa_key")
	if err != nil {
		t.Fatalf("expected navexa_key KV, got error: %v", err)
	}
	if kv.Value != "nk-new-key-1234" {
		t.Errorf("expected navexa_key updated, got %q", kv.Value)
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
	_, err := srv.app.Storage.InternalStore().GetUser(context.Background(), "alice")
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

func TestHandleUsernameCheck_Available(t *testing.T) {
	srv := newTestServerWithStorage(t)

	req := httptest.NewRequest(http.MethodGet, "/api/users/check/newuser", nil)
	rec := httptest.NewRecorder()
	srv.handleUsernameCheck(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	data := resp["data"].(map[string]interface{})
	if data["available"] != true {
		t.Errorf("expected available=true, got %v", data["available"])
	}
	if data["username"] != "newuser" {
		t.Errorf("expected username='newuser', got %v", data["username"])
	}
}

func TestHandleUsernameCheck_Taken(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "a@x.com", "pass", "user")

	req := httptest.NewRequest(http.MethodGet, "/api/users/check/alice", nil)
	rec := httptest.NewRecorder()
	srv.handleUsernameCheck(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	data := resp["data"].(map[string]interface{})
	if data["available"] != false {
		t.Errorf("expected available=false for existing user, got %v", data["available"])
	}
}

func TestHandleUsernameCheck_EmptyUsername(t *testing.T) {
	srv := newTestServerWithStorage(t)

	req := httptest.NewRequest(http.MethodGet, "/api/users/check/", nil)
	rec := httptest.NewRecorder()
	srv.handleUsernameCheck(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty username, got %d", rec.Code)
	}
}

func TestHandleUserUpsert_CreatesNewUser(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]interface{}{
		"username": "newuser",
		"email":    "new@x.com",
		"password": "pass123",
		"role":     "user",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/users/upsert", body)
	rec := httptest.NewRecorder()
	srv.handleUserUpsert(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	data := resp["data"].(map[string]interface{})
	if data["username"] != "newuser" {
		t.Errorf("expected username=newuser, got %v", data["username"])
	}
	if data["email"] != "new@x.com" {
		t.Errorf("expected email=new@x.com, got %v", data["email"])
	}

	// Verify login works
	loginBody := jsonBody(t, map[string]string{"username": "newuser", "password": "pass123"})
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", loginBody)
	loginRec := httptest.NewRecorder()
	srv.handleAuthLogin(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Errorf("expected login to succeed, got %d", loginRec.Code)
	}
}

func TestHandleUserUpsert_UpdatesExistingUser(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "alice@x.com", "oldpass", "admin")

	body := jsonBody(t, map[string]interface{}{
		"username": "alice",
		"email":    "newalice@x.com",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/users/upsert", body)
	rec := httptest.NewRecorder()
	srv.handleUserUpsert(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for update, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	data := resp["data"].(map[string]interface{})
	if data["email"] != "newalice@x.com" {
		t.Errorf("expected updated email, got %v", data["email"])
	}
	if data["role"] != "admin" {
		t.Errorf("expected role to remain unchanged, got %v", data["role"])
	}

	// Old password should still work (password not changed)
	loginBody := jsonBody(t, map[string]string{"username": "alice", "password": "oldpass"})
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", loginBody)
	loginRec := httptest.NewRecorder()
	srv.handleAuthLogin(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Errorf("expected old password to still work, got %d", loginRec.Code)
	}
}

func TestHandleUserUpsert_UpdatesPassword(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "alice@x.com", "oldpass", "user")

	body := jsonBody(t, map[string]interface{}{
		"username": "alice",
		"password": "newpass",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/users/upsert", body)
	rec := httptest.NewRecorder()
	srv.handleUserUpsert(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// New password should work
	loginBody := jsonBody(t, map[string]string{"username": "alice", "password": "newpass"})
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", loginBody)
	loginRec := httptest.NewRecorder()
	srv.handleAuthLogin(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Errorf("expected new password to work, got %d", loginRec.Code)
	}

	// Old password should fail
	loginBody = jsonBody(t, map[string]string{"username": "alice", "password": "oldpass"})
	loginReq = httptest.NewRequest(http.MethodPost, "/api/auth/login", loginBody)
	loginRec = httptest.NewRecorder()
	srv.handleAuthLogin(loginRec, loginReq)
	if loginRec.Code != http.StatusUnauthorized {
		t.Errorf("expected old password to fail, got %d", loginRec.Code)
	}
}

func TestHandleUserUpsert_NewUserRequiresPassword(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]interface{}{
		"username": "nopass",
		"email":    "n@x.com",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/users/upsert", body)
	rec := httptest.NewRecorder()
	srv.handleUserUpsert(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for new user without password, got %d", rec.Code)
	}
}

func TestHandleUserUpsert_SetsPreferences(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]interface{}{
		"username":         "prefuser",
		"password":         "pass123",
		"display_currency": "USD",
		"portfolios":       []string{"Growth", "Income"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/users/upsert", body)
	rec := httptest.NewRecorder()
	srv.handleUserUpsert(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify preferences stored
	ctx := context.Background()
	dc, _ := srv.app.Storage.InternalStore().GetUserKV(ctx, "prefuser", "display_currency")
	if dc.Value != "USD" {
		t.Errorf("expected display_currency=USD, got %q", dc.Value)
	}
	pf, _ := srv.app.Storage.InternalStore().GetUserKV(ctx, "prefuser", "portfolios")
	if pf.Value != "Growth,Income" {
		t.Errorf("expected portfolios=Growth,Income, got %q", pf.Value)
	}
}

func TestHandlePasswordReset_Success(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "alice@x.com", "oldpass", "user")

	body := jsonBody(t, map[string]string{
		"username":     "alice",
		"new_password": "newpass123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/password-reset", body)
	rec := httptest.NewRecorder()
	srv.handlePasswordReset(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Old password should fail
	loginBody := jsonBody(t, map[string]string{"username": "alice", "password": "oldpass"})
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", loginBody)
	loginRec := httptest.NewRecorder()
	srv.handleAuthLogin(loginRec, loginReq)
	if loginRec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with old password, got %d", loginRec.Code)
	}

	// New password should work
	loginBody = jsonBody(t, map[string]string{"username": "alice", "password": "newpass123"})
	loginReq = httptest.NewRequest(http.MethodPost, "/api/auth/login", loginBody)
	loginRec = httptest.NewRecorder()
	srv.handleAuthLogin(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Errorf("expected 200 with new password, got %d", loginRec.Code)
	}
}

func TestHandlePasswordReset_UserNotFound(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]string{
		"username":     "nobody",
		"new_password": "newpass",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/password-reset", body)
	rec := httptest.NewRecorder()
	srv.handlePasswordReset(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestHandlePasswordReset_MissingFields(t *testing.T) {
	srv := newTestServerWithStorage(t)

	// Missing new_password
	body := jsonBody(t, map[string]string{"username": "alice"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/password-reset", body)
	rec := httptest.NewRecorder()
	srv.handlePasswordReset(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing new_password, got %d", rec.Code)
	}

	// Missing username
	body = jsonBody(t, map[string]string{"new_password": "newpass"})
	req = httptest.NewRequest(http.MethodPost, "/api/auth/password-reset", body)
	rec = httptest.NewRecorder()
	srv.handlePasswordReset(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing username, got %d", rec.Code)
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
	user := &models.InternalUser{
		UserID:       "alice",
		Email:        "alice@x.com",
		PasswordHash: "$2a$10$somereallylonghash",
		Role:         "admin",
	}
	kvs := []*models.UserKeyValue{
		{UserID: "alice", Key: "navexa_key", Value: "nk-secret-key-1234"},
	}
	resp := userResponse(user, kvs)

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
		"display_currency": "USD",
		"portfolios":       []string{"Growth", "Income"},
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
	portfolios := data["portfolios"].([]interface{})
	if len(portfolios) != 2 || portfolios[0] != "Growth" || portfolios[1] != "Income" {
		t.Errorf("expected portfolios=[Growth, Income], got %v", portfolios)
	}

	// Verify stored correctly as UserKV
	ctx := context.Background()
	dcKV, _ := srv.app.Storage.InternalStore().GetUserKV(ctx, "alice", "display_currency")
	if dcKV.Value != "USD" {
		t.Errorf("expected stored display_currency=USD, got %q", dcKV.Value)
	}
	pfKV, _ := srv.app.Storage.InternalStore().GetUserKV(ctx, "alice", "portfolios")
	if pfKV.Value != "Growth,Income" {
		t.Errorf("expected stored portfolios=Growth,Income, got %q", pfKV.Value)
	}
}

func TestHandleUserGet_IncludesNewFields(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "alice@x.com", "pass", "user")

	// Set new fields via UserKV storage
	ctx := context.Background()
	srv.app.Storage.InternalStore().SetUserKV(ctx, "alice", "display_currency", "USD")
	srv.app.Storage.InternalStore().SetUserKV(ctx, "alice", "portfolios", "SMSF,Trading")

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
	portfolios := data["portfolios"].([]interface{})
	if len(portfolios) != 2 {
		t.Errorf("expected 2 portfolios, got %d", len(portfolios))
	}
}

func TestHandleAuthLogin_IncludesNewFields(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "alice@x.com", "pass", "admin")

	// Set profile fields via UserKV
	ctx := context.Background()
	srv.app.Storage.InternalStore().SetUserKV(ctx, "alice", "display_currency", "USD")
	srv.app.Storage.InternalStore().SetUserKV(ctx, "alice", "portfolios", "SMSF,Trading")

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
	portfolios := userData["portfolios"].([]interface{})
	if len(portfolios) != 2 {
		t.Errorf("expected 2 portfolios, got %d", len(portfolios))
	}
}

func TestUserResponse_IncludesNewFields(t *testing.T) {
	user := &models.InternalUser{
		UserID:       "alice",
		Email:        "alice@x.com",
		PasswordHash: "$2a$10$hash",
		Role:         "admin",
	}
	kvs := []*models.UserKeyValue{
		{UserID: "alice", Key: "display_currency", Value: "USD"},
		{UserID: "alice", Key: "portfolios", Value: "Growth,Income"},
	}
	resp := userResponse(user, kvs)

	if resp["display_currency"] != "USD" {
		t.Errorf("expected display_currency=USD, got %v", resp["display_currency"])
	}
	portfolios := resp["portfolios"].([]string)
	if len(portfolios) != 2 {
		t.Errorf("expected 2 portfolios, got %d", len(portfolios))
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
	kv, _ := srv.app.Storage.InternalStore().GetUserKV(ctx, "alice", "portfolios")
	if kv.Value != "" {
		t.Errorf("expected empty portfolios KV after clearing, got %q", kv.Value)
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
	kv, _ := srv.app.Storage.InternalStore().GetUserKV(ctx, "alice", "display_currency")
	if kv.Value != "" {
		t.Errorf("expected empty display_currency after clearing, got %q", kv.Value)
	}
}

func TestHandleUserUpdate_PreserveNewFieldsOnPartialUpdate(t *testing.T) {
	// Updating email should NOT wipe display_currency, portfolios, etc.
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "alice@x.com", "pass", "user")

	// Set profile fields
	body := jsonBody(t, map[string]interface{}{
		"display_currency": "USD",
		"portfolios":       []string{"SMSF", "Trading"},
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

	// Verify profile fields preserved in UserKV
	ctx := context.Background()
	user, _ := srv.app.Storage.InternalStore().GetUser(ctx, "alice")
	if user.Email != "alice@new.com" {
		t.Errorf("expected email updated, got %q", user.Email)
	}
	dcKV, _ := srv.app.Storage.InternalStore().GetUserKV(ctx, "alice", "display_currency")
	if dcKV.Value != "USD" {
		t.Errorf("expected display_currency preserved, got %q", dcKV.Value)
	}
	pfKV, _ := srv.app.Storage.InternalStore().GetUserKV(ctx, "alice", "portfolios")
	if pfKV.Value != "SMSF,Trading" {
		t.Errorf("expected portfolios preserved, got %q", pfKV.Value)
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

	// display_currency should be present in response (empty string)
	if _, exists := data["display_currency"]; !exists {
		t.Error("expected display_currency key in response even when empty")
	}
	// portfolios should be present (nil renders as null in JSON)
	if _, exists := data["portfolios"]; !exists {
		t.Error("expected portfolios key in response even when nil")
	}
}

func TestHandleAuthLogin_NewFieldsNeverExposeSecrets(t *testing.T) {
	// Login response should contain new fields but never password_hash or navexa_key
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "alice@x.com", "pass", "admin")

	ctx := context.Background()
	srv.app.Storage.InternalStore().SetUserKV(ctx, "alice", "navexa_key", "nk-super-secret-key-9999")
	srv.app.Storage.InternalStore().SetUserKV(ctx, "alice", "display_currency", "USD")
	srv.app.Storage.InternalStore().SetUserKV(ctx, "alice", "portfolios", "Growth")

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

	// Create a user with a navexa_key in UserKV
	srv.app.Storage.InternalStore().SaveUser(ctx, &models.InternalUser{
		UserID:       "user-123",
		Email:        "u@x.com",
		PasswordHash: "hash",
		Role:         "user",
	})
	srv.app.Storage.InternalStore().SetUserKV(ctx, "user-123", "navexa_key", "nk-stored-key-5678")

	var capturedUC *common.UserContext
	handler := userContextMiddleware(srv.app.Storage.InternalStore())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	// Create a user with a stored navexa_key in UserKV
	srv.app.Storage.InternalStore().SaveUser(ctx, &models.InternalUser{
		UserID:       "user-123",
		Email:        "u@x.com",
		PasswordHash: "hash",
		Role:         "user",
	})
	srv.app.Storage.InternalStore().SetUserKV(ctx, "user-123", "navexa_key", "nk-stored-key")

	var capturedUC *common.UserContext
	handler := userContextMiddleware(srv.app.Storage.InternalStore())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	handler := userContextMiddleware(srv.app.Storage.InternalStore())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
