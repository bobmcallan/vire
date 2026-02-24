package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/app"
	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

// mockPortfolioService implements interfaces.PortfolioService for testing.
type mockPortfolioService struct {
	getPortfolio  func(ctx context.Context, name string) (*models.Portfolio, error)
	syncPortfolio func(ctx context.Context, name string, force bool) (*models.Portfolio, error)
}

func (m *mockPortfolioService) GetPortfolio(ctx context.Context, name string) (*models.Portfolio, error) {
	return m.getPortfolio(ctx, name)
}

func (m *mockPortfolioService) SyncPortfolio(ctx context.Context, name string, force bool) (*models.Portfolio, error) {
	return m.syncPortfolio(ctx, name, force)
}

func (m *mockPortfolioService) ListPortfolios(ctx context.Context) ([]string, error) {
	return nil, nil
}

func (m *mockPortfolioService) ReviewPortfolio(ctx context.Context, name string, options interfaces.ReviewOptions) (*models.PortfolioReview, error) {
	return nil, nil
}

func (m *mockPortfolioService) ReviewWatchlist(ctx context.Context, name string, options interfaces.ReviewOptions) (*models.WatchlistReview, error) {
	return nil, nil
}

func (m *mockPortfolioService) GetPortfolioSnapshot(ctx context.Context, name string, asOf time.Time) (*models.PortfolioSnapshot, error) {
	return nil, nil
}

func (m *mockPortfolioService) GetPortfolioGrowth(ctx context.Context, name string) ([]models.GrowthDataPoint, error) {
	return nil, nil
}

func (m *mockPortfolioService) GetExternalBalances(ctx context.Context, portfolioName string) ([]models.ExternalBalance, error) {
	return nil, nil
}

func (m *mockPortfolioService) SetExternalBalances(ctx context.Context, portfolioName string, balances []models.ExternalBalance) (*models.Portfolio, error) {
	return nil, nil
}

func (m *mockPortfolioService) AddExternalBalance(ctx context.Context, portfolioName string, balance models.ExternalBalance) (*models.Portfolio, error) {
	return nil, nil
}

func (m *mockPortfolioService) RemoveExternalBalance(ctx context.Context, portfolioName string, balanceID string) (*models.Portfolio, error) {
	return nil, nil
}

func (m *mockPortfolioService) GetDailyGrowth(ctx context.Context, name string, opts interfaces.GrowthOptions) ([]models.GrowthDataPoint, error) {
	return nil, nil
}

func newTestServer(portfolioSvc interfaces.PortfolioService) *Server {
	logger := common.NewLoggerFromConfig(common.LoggingConfig{Level: "disabled"})
	cfg := common.NewDefaultConfig()
	a := &app.App{
		Config:           cfg,
		PortfolioService: portfolioSvc,
		Logger:           logger,
	}
	return &Server{app: a, logger: logger}
}

func TestHandlePortfolioGet_ReturnsPortfolio(t *testing.T) {
	portfolio := &models.Portfolio{
		Name:       "test",
		TotalValue: 200.0,
		LastSynced: time.Now(),
	}

	svc := &mockPortfolioService{
		getPortfolio: func(ctx context.Context, name string) (*models.Portfolio, error) {
			return portfolio, nil
		},
	}

	srv := newTestServer(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/portfolios/test", nil)
	rec := httptest.NewRecorder()

	srv.handlePortfolioGet(rec, req, "test")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var got models.Portfolio
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.Name != "test" {
		t.Errorf("expected portfolio name 'test', got %q", got.Name)
	}
	if got.TotalValue != 200.0 {
		t.Errorf("expected total value 200.0, got %f", got.TotalValue)
	}
}

func TestHandlePortfolioGet_NotFound(t *testing.T) {
	svc := &mockPortfolioService{
		getPortfolio: func(ctx context.Context, name string) (*models.Portfolio, error) {
			return nil, errors.New("not found")
		},
	}

	srv := newTestServer(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/portfolios/missing", nil)
	rec := httptest.NewRecorder()

	srv.handlePortfolioGet(rec, req, "missing")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rec.Code)
	}
}

// --- Portal injection validation tests ---

func TestHandlePortfolioSync_MissingUserContext_Returns400(t *testing.T) {
	svc := &mockPortfolioService{}
	srv := newTestServer(svc)

	// No user context headers at all
	req := httptest.NewRequest(http.MethodPost, "/api/portfolios/SMSF/sync", nil)
	rec := httptest.NewRecorder()

	srv.handlePortfolioSync(rec, req, "SMSF")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}

	var resp ErrorResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Error != "configuration not correct" {
		t.Errorf("expected error 'configuration not correct', got %q", resp.Error)
	}
}

func TestHandlePortfolioSync_MissingUserID_Returns400(t *testing.T) {
	svc := &mockPortfolioService{}
	srv := newTestServer(svc)

	// Has navexa key but no user ID
	req := httptest.NewRequest(http.MethodPost, "/api/portfolios/SMSF/sync", nil)
	uc := &common.UserContext{NavexaAPIKey: "some-key"}
	req = req.WithContext(common.WithUserContext(req.Context(), uc))
	rec := httptest.NewRecorder()

	srv.handlePortfolioSync(rec, req, "SMSF")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}

	var resp ErrorResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Error != "configuration not correct" {
		t.Errorf("expected error 'configuration not correct', got %q", resp.Error)
	}
}

func TestHandlePortfolioSync_MissingNavexaKey_Returns400(t *testing.T) {
	svc := &mockPortfolioService{}
	srv := newTestServer(svc)

	// Has user ID but no navexa key
	req := httptest.NewRequest(http.MethodPost, "/api/portfolios/SMSF/sync", nil)
	uc := &common.UserContext{UserID: "user-123"}
	req = req.WithContext(common.WithUserContext(req.Context(), uc))
	rec := httptest.NewRecorder()

	srv.handlePortfolioSync(rec, req, "SMSF")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}

	var resp ErrorResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Error != "configuration not correct" {
		t.Errorf("expected error 'configuration not correct', got %q", resp.Error)
	}
}

func TestHandlePortfolioSync_BothPresent_Succeeds(t *testing.T) {
	syncCalled := false
	svc := &mockPortfolioService{
		syncPortfolio: func(ctx context.Context, name string, force bool) (*models.Portfolio, error) {
			syncCalled = true
			return &models.Portfolio{Name: name, LastSynced: time.Now()}, nil
		},
	}
	srv := newTestServer(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/portfolios/SMSF/sync", nil)
	uc := &common.UserContext{UserID: "user-123", NavexaAPIKey: "key-abc"}
	req = req.WithContext(common.WithUserContext(req.Context(), uc))
	rec := httptest.NewRecorder()

	srv.handlePortfolioSync(rec, req, "SMSF")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if !syncCalled {
		t.Error("expected SyncPortfolio to be called")
	}
}

func TestHandlePortfolioRebuild_MissingUserContext_Returns400(t *testing.T) {
	svc := &mockPortfolioService{}
	srv := newTestServer(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/portfolios/SMSF/rebuild", nil)
	rec := httptest.NewRecorder()

	srv.handlePortfolioRebuild(rec, req, "SMSF")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}

	var resp ErrorResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Error != "configuration not correct" {
		t.Errorf("expected error 'configuration not correct', got %q", resp.Error)
	}
}
