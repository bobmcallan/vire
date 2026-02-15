package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
)

func TestUserContextMiddleware_AllHeaders(t *testing.T) {
	handler := userContextMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uc := common.UserContextFromContext(r.Context())
		if uc == nil {
			t.Fatal("Expected UserContext to be present")
		}
		if len(uc.Portfolios) != 2 || uc.Portfolios[0] != "SMSF" || uc.Portfolios[1] != "Trading" {
			t.Errorf("Expected portfolios [SMSF, Trading], got %v", uc.Portfolios)
		}
		if uc.DisplayCurrency != "AUD" {
			t.Errorf("Expected display_currency=AUD, got %s", uc.DisplayCurrency)
		}
		if uc.NavexaAPIKey != "test-key-123" {
			t.Errorf("Expected navexa key=test-key-123, got %s", uc.NavexaAPIKey)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.Header.Set("X-Vire-Portfolios", "SMSF, Trading")
	req.Header.Set("X-Vire-Display-Currency", "AUD")
	req.Header.Set("X-Vire-Navexa-Key", "test-key-123")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rr.Code)
	}
}

func TestUserContextMiddleware_NoHeaders(t *testing.T) {
	handler := userContextMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uc := common.UserContextFromContext(r.Context())
		if uc != nil {
			t.Error("Expected nil UserContext when no headers present")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rr.Code)
	}
}

func TestUserContextMiddleware_PartialHeaders(t *testing.T) {
	handler := userContextMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uc := common.UserContextFromContext(r.Context())
		if uc == nil {
			t.Fatal("Expected UserContext to be present")
		}
		if len(uc.Portfolios) != 1 || uc.Portfolios[0] != "SMSF" {
			t.Errorf("Expected portfolios [SMSF], got %v", uc.Portfolios)
		}
		if uc.DisplayCurrency != "" {
			t.Errorf("Expected empty display_currency, got %s", uc.DisplayCurrency)
		}
		if uc.NavexaAPIKey != "" {
			t.Errorf("Expected empty navexa key, got %s", uc.NavexaAPIKey)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.Header.Set("X-Vire-Portfolios", "SMSF")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rr.Code)
	}
}

func TestUserContextMiddleware_CommaSeparatedPortfolios(t *testing.T) {
	handler := userContextMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uc := common.UserContextFromContext(r.Context())
		if uc == nil {
			t.Fatal("Expected UserContext to be present")
		}
		if len(uc.Portfolios) != 3 {
			t.Fatalf("Expected 3 portfolios, got %d: %v", len(uc.Portfolios), uc.Portfolios)
		}
		if uc.Portfolios[0] != "A" || uc.Portfolios[1] != "B" || uc.Portfolios[2] != "C" {
			t.Errorf("Expected [A, B, C], got %v", uc.Portfolios)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.Header.Set("X-Vire-Portfolios", "A, B, C")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
}

func TestUserContextMiddleware_UserIDHeader(t *testing.T) {
	handler := userContextMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uc := common.UserContextFromContext(r.Context())
		if uc == nil {
			t.Fatal("Expected UserContext to be present")
		}
		if uc.UserID != "user-456" {
			t.Errorf("Expected UserID=user-456, got %s", uc.UserID)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.Header.Set("X-Vire-User-ID", "user-456")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rr.Code)
	}
}

func TestUserContextMiddleware_AllHeadersIncludingUserID(t *testing.T) {
	handler := userContextMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uc := common.UserContextFromContext(r.Context())
		if uc == nil {
			t.Fatal("Expected UserContext to be present")
		}
		if uc.UserID != "user-789" {
			t.Errorf("Expected UserID=user-789, got %s", uc.UserID)
		}
		if uc.NavexaAPIKey != "key-abc" {
			t.Errorf("Expected NavexaAPIKey=key-abc, got %s", uc.NavexaAPIKey)
		}
		if len(uc.Portfolios) != 1 || uc.Portfolios[0] != "SMSF" {
			t.Errorf("Expected portfolios [SMSF], got %v", uc.Portfolios)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.Header.Set("X-Vire-Portfolios", "SMSF")
	req.Header.Set("X-Vire-Navexa-Key", "key-abc")
	req.Header.Set("X-Vire-User-ID", "user-789")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rr.Code)
	}
}

// logLevelCapture wraps a writer to capture raw JSON log events and extract levels.
type logLevelCapture struct {
	buf bytes.Buffer
}

func (c *logLevelCapture) Write(p []byte) (int, error) {
	return c.buf.Write(p)
}

// lastLevel parses the captured output and returns the log level from the last JSON event.
// The writerAdapter reformats JSON to text, so we check for level keywords in the text output.
// arbor's writerAdapter doesn't include the level prefix, so we use a different approach:
// we check whether the event was logged at all (level filtering) to infer the level used.
func (c *logLevelCapture) output() string {
	return c.buf.String()
}

func TestLoggingMiddleware_4xxUsesInfoLevel(t *testing.T) {
	// Verify that 4xx responses call logger.Info(), not logger.Warn().
	// We test this by creating a logger at WARN level — at WARN, Info() events are filtered out.
	// Before the fix: 4xx uses Warn() → event passes the WARN filter → output is non-empty
	// After the fix: 4xx uses Info() → event fails the WARN filter → output is empty

	capture := &logLevelCapture{}
	logger := common.NewLoggerWithOutput("warn", capture)

	handler := loggingMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/missing", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	output := capture.output()
	if strings.Contains(output, "HTTP request") {
		t.Errorf("Expected 404 log to be filtered at WARN level (should use INFO), but it passed through: %s", output)
	}
}

func TestLoggingMiddleware_5xxUsesErrorLevel(t *testing.T) {
	// Verify that 5xx responses call logger.Error(), not a lower level.
	// At WARN level, Error() events should pass through.
	capture := &logLevelCapture{}
	logger := common.NewLoggerWithOutput("warn", capture)

	handler := loggingMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/broken", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	output := capture.output()
	if !strings.Contains(output, "HTTP request") {
		t.Errorf("Expected 500 log to pass WARN filter (should use ERROR), got: %q", output)
	}
}

func TestLoggingMiddleware_2xxUsesTraceLevel(t *testing.T) {
	// Verify that 2xx responses call logger.Trace(), which is below INFO.
	// At INFO level, Trace() events should be filtered out.
	capture := &logLevelCapture{}
	logger := common.NewLoggerWithOutput("info", capture)

	handler := loggingMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	output := capture.output()
	if strings.Contains(output, "HTTP request") {
		t.Errorf("Expected 200 log to be filtered at INFO level (should use TRACE), but it passed through: %s", output)
	}
}

func TestCORSMiddleware_AllowsVireHeaders(t *testing.T) {
	handler := corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/api/config", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	allowHeaders := rr.Header().Get("Access-Control-Allow-Headers")
	for _, h := range []string{"X-Vire-Portfolios", "X-Vire-Display-Currency", "X-Vire-Navexa-Key", "X-Vire-User-ID"} {
		if !contains(allowHeaders, h) {
			t.Errorf("Expected %s in Access-Control-Allow-Headers, got: %s", h, allowHeaders)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestMiddleware_ResolvesDisplayCurrencyFromUserStorage(t *testing.T) {
	srv := newTestServerWithStorage(t)
	ctx := context.Background()

	srv.app.Storage.UserStorage().SaveUser(ctx, &models.User{
		Username:        "user-dc",
		Email:           "u@x.com",
		PasswordHash:    "hash",
		Role:            "user",
		DisplayCurrency: "USD",
	})

	var capturedUC *common.UserContext
	handler := userContextMiddleware(srv.app.Storage.UserStorage())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUC = common.UserContextFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.Header.Set("X-Vire-User-ID", "user-dc")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if capturedUC == nil {
		t.Fatal("expected UserContext")
	}
	if capturedUC.DisplayCurrency != "USD" {
		t.Errorf("expected display_currency resolved from storage, got %q", capturedUC.DisplayCurrency)
	}
}

func TestMiddleware_ResolvesPortfoliosFromUserStorage(t *testing.T) {
	srv := newTestServerWithStorage(t)
	ctx := context.Background()

	srv.app.Storage.UserStorage().SaveUser(ctx, &models.User{
		Username:     "user-pf",
		Email:        "u@x.com",
		PasswordHash: "hash",
		Role:         "user",
		Portfolios:   []string{"SMSF", "Trading"},
	})

	var capturedUC *common.UserContext
	handler := userContextMiddleware(srv.app.Storage.UserStorage())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUC = common.UserContextFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.Header.Set("X-Vire-User-ID", "user-pf")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if capturedUC == nil {
		t.Fatal("expected UserContext")
	}
	if len(capturedUC.Portfolios) != 2 || capturedUC.Portfolios[0] != "SMSF" || capturedUC.Portfolios[1] != "Trading" {
		t.Errorf("expected portfolios resolved from storage, got %v", capturedUC.Portfolios)
	}
}

func TestMiddleware_HeaderOverridesStoredDisplayCurrency(t *testing.T) {
	srv := newTestServerWithStorage(t)
	ctx := context.Background()

	srv.app.Storage.UserStorage().SaveUser(ctx, &models.User{
		Username:        "user-override",
		Email:           "u@x.com",
		PasswordHash:    "hash",
		Role:            "user",
		DisplayCurrency: "USD",
		Portfolios:      []string{"SMSF"},
	})

	var capturedUC *common.UserContext
	handler := userContextMiddleware(srv.app.Storage.UserStorage())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUC = common.UserContextFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.Header.Set("X-Vire-User-ID", "user-override")
	req.Header.Set("X-Vire-Display-Currency", "AUD")
	req.Header.Set("X-Vire-Portfolios", "Trading, Growth")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if capturedUC == nil {
		t.Fatal("expected UserContext")
	}
	// Headers should override stored values
	if capturedUC.DisplayCurrency != "AUD" {
		t.Errorf("expected header display_currency to override, got %q", capturedUC.DisplayCurrency)
	}
	if len(capturedUC.Portfolios) != 2 || capturedUC.Portfolios[0] != "Trading" || capturedUC.Portfolios[1] != "Growth" {
		t.Errorf("expected header portfolios to override, got %v", capturedUC.Portfolios)
	}
}

func TestMiddleware_OnlyUserIDResolvesAllFields(t *testing.T) {
	srv := newTestServerWithStorage(t)
	ctx := context.Background()

	srv.app.Storage.UserStorage().SaveUser(ctx, &models.User{
		Username:        "full-profile",
		Email:           "u@x.com",
		PasswordHash:    "hash",
		Role:            "user",
		NavexaKey:       "nk-full-key-9999",
		DisplayCurrency: "USD",
		Portfolios:      []string{"SMSF", "Trading"},
	})

	var capturedUC *common.UserContext
	handler := userContextMiddleware(srv.app.Storage.UserStorage())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUC = common.UserContextFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// Only send X-Vire-User-ID — all fields should resolve from profile
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.Header.Set("X-Vire-User-ID", "full-profile")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if capturedUC == nil {
		t.Fatal("expected UserContext")
	}
	if capturedUC.UserID != "full-profile" {
		t.Errorf("expected UserID=full-profile, got %q", capturedUC.UserID)
	}
	if capturedUC.NavexaAPIKey != "nk-full-key-9999" {
		t.Errorf("expected navexa_key from storage, got %q", capturedUC.NavexaAPIKey)
	}
	if capturedUC.DisplayCurrency != "USD" {
		t.Errorf("expected display_currency from storage, got %q", capturedUC.DisplayCurrency)
	}
	if len(capturedUC.Portfolios) != 2 {
		t.Errorf("expected 2 portfolios from storage, got %d", len(capturedUC.Portfolios))
	}
}
