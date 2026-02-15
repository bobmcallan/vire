package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bobmcallan/vire/internal/common"
)

func TestUserContextMiddleware_AllHeaders(t *testing.T) {
	handler := userContextMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	handler := userContextMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	handler := userContextMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	handler := userContextMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	handler := userContextMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	handler := userContextMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
