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

func (m *mockPortfolioService) GetPortfolioSnapshot(ctx context.Context, name string, asOf time.Time) (*models.PortfolioSnapshot, error) {
	return nil, nil
}

func (m *mockPortfolioService) GetPortfolioGrowth(ctx context.Context, name string) ([]models.GrowthDataPoint, error) {
	return nil, nil
}

func (m *mockPortfolioService) GetDailyGrowth(ctx context.Context, name string, opts interfaces.GrowthOptions) ([]models.GrowthDataPoint, error) {
	return nil, nil
}

func newTestServer(portfolioSvc interfaces.PortfolioService) *Server {
	logger := common.NewLoggerFromConfig(common.LoggingConfig{Level: "disabled"})
	a := &app.App{
		PortfolioService: portfolioSvc,
		Logger:           logger,
	}
	return &Server{app: a, logger: logger}
}

func TestHandlePortfolioGet_FreshPortfolio_NoSync(t *testing.T) {
	syncCalled := false
	freshPortfolio := &models.Portfolio{
		Name:       "test",
		LastSynced: time.Now(), // fresh
	}

	svc := &mockPortfolioService{
		getPortfolio: func(ctx context.Context, name string) (*models.Portfolio, error) {
			return freshPortfolio, nil
		},
		syncPortfolio: func(ctx context.Context, name string, force bool) (*models.Portfolio, error) {
			syncCalled = true
			return nil, errors.New("should not be called")
		},
	}

	srv := newTestServer(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/portfolios/test", nil)
	rec := httptest.NewRecorder()

	srv.handlePortfolioGet(rec, req, "test")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if syncCalled {
		t.Fatal("SyncPortfolio should not be called for a fresh portfolio")
	}

	var got models.Portfolio
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.Name != "test" {
		t.Errorf("expected portfolio name 'test', got %q", got.Name)
	}
}

func TestHandlePortfolioGet_StalePortfolio_TriggersSync(t *testing.T) {
	syncCalled := false
	stalePortfolio := &models.Portfolio{
		Name:       "test",
		TotalValue: 100.0,
		LastSynced: time.Now().Add(-2 * common.FreshnessPortfolio), // stale
	}
	freshPortfolio := &models.Portfolio{
		Name:       "test",
		TotalValue: 200.0,
		LastSynced: time.Now(),
	}

	svc := &mockPortfolioService{
		getPortfolio: func(ctx context.Context, name string) (*models.Portfolio, error) {
			return stalePortfolio, nil
		},
		syncPortfolio: func(ctx context.Context, name string, force bool) (*models.Portfolio, error) {
			syncCalled = true
			if force {
				t.Error("SyncPortfolio should be called with force=false")
			}
			return freshPortfolio, nil
		},
	}

	srv := newTestServer(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/portfolios/test", nil)
	rec := httptest.NewRecorder()

	srv.handlePortfolioGet(rec, req, "test")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if !syncCalled {
		t.Fatal("SyncPortfolio should be called for a stale portfolio")
	}

	var got models.Portfolio
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.TotalValue != 200.0 {
		t.Errorf("expected synced portfolio value 200.0, got %f", got.TotalValue)
	}
}

func TestHandlePortfolioGet_SyncFails_ReturnsStaleData(t *testing.T) {
	stalePortfolio := &models.Portfolio{
		Name:       "test",
		TotalValue: 100.0,
		LastSynced: time.Now().Add(-2 * common.FreshnessPortfolio), // stale
	}

	svc := &mockPortfolioService{
		getPortfolio: func(ctx context.Context, name string) (*models.Portfolio, error) {
			return stalePortfolio, nil
		},
		syncPortfolio: func(ctx context.Context, name string, force bool) (*models.Portfolio, error) {
			return nil, errors.New("navexa unavailable")
		},
	}

	srv := newTestServer(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/portfolios/test", nil)
	rec := httptest.NewRecorder()

	srv.handlePortfolioGet(rec, req, "test")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 (stale fallback), got %d", rec.Code)
	}

	var got models.Portfolio
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.TotalValue != 100.0 {
		t.Errorf("expected stale portfolio value 100.0, got %f", got.TotalValue)
	}
}

func TestHandlePortfolioGet_NotFound(t *testing.T) {
	svc := &mockPortfolioService{
		getPortfolio: func(ctx context.Context, name string) (*models.Portfolio, error) {
			return nil, errors.New("not found")
		},
		syncPortfolio: func(ctx context.Context, name string, force bool) (*models.Portfolio, error) {
			t.Fatal("SyncPortfolio should not be called when portfolio not found")
			return nil, nil
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
