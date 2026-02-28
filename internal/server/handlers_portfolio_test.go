package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/app"
	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

// mockPortfolioService implements interfaces.PortfolioService for testing.
type mockPortfolioService struct {
	getPortfolio           func(ctx context.Context, name string) (*models.Portfolio, error)
	syncPortfolio          func(ctx context.Context, name string, force bool) (*models.Portfolio, error)
	getPortfolioIndicators func(ctx context.Context, name string) (*models.PortfolioIndicators, error)
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

func (m *mockPortfolioService) GetPortfolioIndicators(ctx context.Context, name string) (*models.PortfolioIndicators, error) {
	if m.getPortfolioIndicators != nil {
		return m.getPortfolioIndicators(ctx, name)
	}
	return nil, nil
}

// mockCashFlowService implements interfaces.CashFlowService for testing.
type mockCashFlowService struct {
	calculatePerformance func(ctx context.Context, portfolioName string) (*models.CapitalPerformance, error)
}

func (m *mockCashFlowService) GetLedger(ctx context.Context, portfolioName string) (*models.CashFlowLedger, error) {
	return nil, nil
}

func (m *mockCashFlowService) AddTransaction(ctx context.Context, portfolioName string, tx models.CashTransaction) (*models.CashFlowLedger, error) {
	return nil, nil
}

func (m *mockCashFlowService) AddTransfer(ctx context.Context, portfolioName string, fromAccount, toAccount string, amount float64, date time.Time, description string) (*models.CashFlowLedger, error) {
	return nil, nil
}

func (m *mockCashFlowService) UpdateTransaction(ctx context.Context, portfolioName string, txID string, tx models.CashTransaction) (*models.CashFlowLedger, error) {
	return nil, nil
}

func (m *mockCashFlowService) RemoveTransaction(ctx context.Context, portfolioName string, txID string) (*models.CashFlowLedger, error) {
	return nil, nil
}

func (m *mockCashFlowService) CalculatePerformance(ctx context.Context, portfolioName string) (*models.CapitalPerformance, error) {
	if m.calculatePerformance != nil {
		return m.calculatePerformance(ctx, portfolioName)
	}
	return &models.CapitalPerformance{}, nil
}

func newTestServer(portfolioSvc interfaces.PortfolioService) *Server {
	return newTestServerWithCashFlow(portfolioSvc, nil)
}

func newTestServerWithCashFlow(portfolioSvc interfaces.PortfolioService, cashFlowSvc interfaces.CashFlowService) *Server {
	logger := common.NewLoggerFromConfig(common.LoggingConfig{Level: "disabled"})
	cfg := common.NewDefaultConfig()
	if cashFlowSvc == nil {
		cashFlowSvc = &mockCashFlowService{}
	}
	a := &app.App{
		Config:           cfg,
		PortfolioService: portfolioSvc,
		CashFlowService:  cashFlowSvc,
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

// --- Capital performance embedding tests ---

func TestHandlePortfolioGet_IncludesCapitalPerformance(t *testing.T) {
	now := time.Now()
	portfolio := &models.Portfolio{
		Name:       "test",
		TotalValue: 500000.0,
		LastSynced: now,
	}

	portfolioSvc := &mockPortfolioService{
		getPortfolio: func(ctx context.Context, name string) (*models.Portfolio, error) {
			return portfolio, nil
		},
	}

	cashFlowSvc := &mockCashFlowService{
		calculatePerformance: func(ctx context.Context, portfolioName string) (*models.CapitalPerformance, error) {
			firstDate := now.Add(-90 * 24 * time.Hour)
			return &models.CapitalPerformance{
				TotalDeposited:        471000.0,
				TotalWithdrawn:        0,
				NetCapitalDeployed:    471000.0,
				CurrentPortfolioValue: 500000.0,
				SimpleReturnPct:       6.16,
				AnnualizedReturnPct:   15.2,
				FirstTransactionDate:  &firstDate,
				TransactionCount:      5,
			}, nil
		},
	}

	srv := newTestServerWithCashFlow(portfolioSvc, cashFlowSvc)
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
	if got.CapitalPerformance == nil {
		t.Fatal("expected capital_performance to be present")
	}
	if got.CapitalPerformance.AnnualizedReturnPct != 15.2 {
		t.Errorf("expected annualized return 15.2, got %f", got.CapitalPerformance.AnnualizedReturnPct)
	}
	if got.CapitalPerformance.TransactionCount != 5 {
		t.Errorf("expected transaction count 5, got %d", got.CapitalPerformance.TransactionCount)
	}
}

func TestHandlePortfolioGet_OmitsCapitalPerformanceWhenNoTransactions(t *testing.T) {
	portfolio := &models.Portfolio{
		Name:       "test",
		TotalValue: 100000.0,
		LastSynced: time.Now(),
	}

	portfolioSvc := &mockPortfolioService{
		getPortfolio: func(ctx context.Context, name string) (*models.Portfolio, error) {
			return portfolio, nil
		},
	}

	cashFlowSvc := &mockCashFlowService{
		calculatePerformance: func(ctx context.Context, portfolioName string) (*models.CapitalPerformance, error) {
			return &models.CapitalPerformance{}, nil // TransactionCount == 0
		},
	}

	srv := newTestServerWithCashFlow(portfolioSvc, cashFlowSvc)
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
	if got.CapitalPerformance != nil {
		t.Error("expected capital_performance to be nil when no transactions exist")
	}
}

func TestHandlePortfolioGet_CapitalPerformanceErrorDoesNotBreakResponse(t *testing.T) {
	portfolio := &models.Portfolio{
		Name:       "test",
		TotalValue: 100000.0,
		LastSynced: time.Now(),
	}

	portfolioSvc := &mockPortfolioService{
		getPortfolio: func(ctx context.Context, name string) (*models.Portfolio, error) {
			return portfolio, nil
		},
	}

	cashFlowSvc := &mockCashFlowService{
		calculatePerformance: func(ctx context.Context, portfolioName string) (*models.CapitalPerformance, error) {
			return nil, errors.New("storage unavailable")
		},
	}

	srv := newTestServerWithCashFlow(portfolioSvc, cashFlowSvc)
	req := httptest.NewRequest(http.MethodGet, "/api/portfolios/test", nil)
	rec := httptest.NewRecorder()

	srv.handlePortfolioGet(rec, req, "test")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 even when capital performance fails, got %d", rec.Code)
	}

	var got models.Portfolio
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.CapitalPerformance != nil {
		t.Error("expected capital_performance to be nil when calculation fails")
	}
	if got.Name != "test" {
		t.Errorf("expected portfolio name 'test', got %q", got.Name)
	}
}

// --- Capital performance stress tests ---

func TestHandlePortfolioGet_CapitalPerformanceNilReturn(t *testing.T) {
	// CalculatePerformance returns nil, nil (no error but nil result)
	portfolio := &models.Portfolio{
		Name:       "test",
		TotalValue: 100000.0,
		LastSynced: time.Now(),
	}

	portfolioSvc := &mockPortfolioService{
		getPortfolio: func(ctx context.Context, name string) (*models.Portfolio, error) {
			return portfolio, nil
		},
	}

	cashFlowSvc := &mockCashFlowService{
		calculatePerformance: func(ctx context.Context, portfolioName string) (*models.CapitalPerformance, error) {
			return nil, nil // nil perf, no error
		},
	}

	srv := newTestServerWithCashFlow(portfolioSvc, cashFlowSvc)
	req := httptest.NewRequest(http.MethodGet, "/api/portfolios/test", nil)
	rec := httptest.NewRecorder()

	srv.handlePortfolioGet(rec, req, "test")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 when perf is nil, got %d", rec.Code)
	}

	var got models.Portfolio
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.CapitalPerformance != nil {
		t.Error("expected capital_performance to be nil when CalculatePerformance returns nil")
	}
}

func TestHandlePortfolioGet_CapitalPerformanceExtremeValues(t *testing.T) {
	// Very large return values — verify JSON serialization doesn't produce NaN/Inf
	portfolio := &models.Portfolio{
		Name:       "test",
		TotalValue: 1e12,
		LastSynced: time.Now(),
	}

	portfolioSvc := &mockPortfolioService{
		getPortfolio: func(ctx context.Context, name string) (*models.Portfolio, error) {
			return portfolio, nil
		},
	}

	firstDate := time.Now().AddDate(-10, 0, 0)
	cashFlowSvc := &mockCashFlowService{
		calculatePerformance: func(ctx context.Context, portfolioName string) (*models.CapitalPerformance, error) {
			return &models.CapitalPerformance{
				TotalDeposited:        1e6,
				TotalWithdrawn:        0,
				NetCapitalDeployed:    1e6,
				CurrentPortfolioValue: 1e12,
				SimpleReturnPct:       99999900.0, // 1e12/1e6 - 1 * 100
				AnnualizedReturnPct:   999.99,
				FirstTransactionDate:  &firstDate,
				TransactionCount:      1,
			}, nil
		},
	}

	srv := newTestServerWithCashFlow(portfolioSvc, cashFlowSvc)
	req := httptest.NewRequest(http.MethodGet, "/api/portfolios/test", nil)
	rec := httptest.NewRecorder()

	srv.handlePortfolioGet(rec, req, "test")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	// Verify response is valid JSON
	var raw map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&raw); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	cp, ok := raw["capital_performance"]
	if !ok || cp == nil {
		t.Fatal("expected capital_performance in response")
	}
}

func TestHandlePortfolioGet_CapitalPerformanceNegativeReturns(t *testing.T) {
	// Portfolio has lost money — verify negative percentages are serialized correctly
	portfolio := &models.Portfolio{
		Name:       "test",
		TotalValue: 50000.0,
		LastSynced: time.Now(),
	}

	portfolioSvc := &mockPortfolioService{
		getPortfolio: func(ctx context.Context, name string) (*models.Portfolio, error) {
			return portfolio, nil
		},
	}

	firstDate := time.Now().AddDate(-1, 0, 0)
	cashFlowSvc := &mockCashFlowService{
		calculatePerformance: func(ctx context.Context, portfolioName string) (*models.CapitalPerformance, error) {
			return &models.CapitalPerformance{
				TotalDeposited:        100000,
				TotalWithdrawn:        0,
				NetCapitalDeployed:    100000,
				CurrentPortfolioValue: 50000,
				SimpleReturnPct:       -50.0,
				AnnualizedReturnPct:   -50.0,
				FirstTransactionDate:  &firstDate,
				TransactionCount:      1,
			}, nil
		},
	}

	srv := newTestServerWithCashFlow(portfolioSvc, cashFlowSvc)
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
	if got.CapitalPerformance == nil {
		t.Fatal("expected capital_performance to be present")
	}
	if got.CapitalPerformance.SimpleReturnPct != -50.0 {
		t.Errorf("SimpleReturnPct = %f, want -50.0", got.CapitalPerformance.SimpleReturnPct)
	}
}

func TestPortfolio_CapitalPerformanceOmittedInJSON(t *testing.T) {
	// When CapitalPerformance is nil, the field should be omitted from JSON
	p := models.Portfolio{
		Name:               "test",
		TotalValue:         100000,
		CapitalPerformance: nil,
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	raw := string(data)
	if strings.Contains(raw, "capital_performance") {
		t.Error("nil CapitalPerformance should be omitted from JSON via omitempty")
	}
}

func TestPortfolio_CapitalPerformancePresentInJSON(t *testing.T) {
	// When CapitalPerformance is set, the field should appear
	p := models.Portfolio{
		Name:       "test",
		TotalValue: 100000,
		CapitalPerformance: &models.CapitalPerformance{
			TransactionCount: 3,
			SimpleReturnPct:  12.5,
		},
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	raw := string(data)
	if !strings.Contains(raw, "capital_performance") {
		t.Error("non-nil CapitalPerformance should be present in JSON")
	}
	if !strings.Contains(raw, `"transaction_count":3`) {
		t.Error("transaction_count should be serialized")
	}
}

func TestPortfolio_BackwardCompatibility_NoCapitalPerformance(t *testing.T) {
	// Old JSON without capital_performance field should deserialize cleanly
	oldJSON := `{
		"id": "test",
		"name": "SMSF",
		"holdings": [],
		"total_value": 100000,
		"total_value_holdings": 100000,
		"total_cost": 90000,
		"currency": "AUD",
		"external_balance_total": 0,
		"last_synced": "2025-01-01T00:00:00Z",
		"created_at": "2025-01-01T00:00:00Z",
		"updated_at": "2025-01-01T00:00:00Z"
	}`

	var p models.Portfolio
	if err := json.Unmarshal([]byte(oldJSON), &p); err != nil {
		t.Fatalf("failed to unmarshal old JSON: %v", err)
	}
	if p.CapitalPerformance != nil {
		t.Error("CapitalPerformance should be nil when not present in JSON")
	}
	if p.TotalValue != 100000 {
		t.Errorf("TotalValue = %v, want 100000", p.TotalValue)
	}
}

func TestHandlePortfolioGet_ConcurrentCapitalPerformance(t *testing.T) {
	// Concurrent portfolio gets should not race on CapitalPerformance attachment
	portfolio := &models.Portfolio{
		Name:       "test",
		TotalValue: 200000.0,
		LastSynced: time.Now(),
	}

	portfolioSvc := &mockPortfolioService{
		getPortfolio: func(ctx context.Context, name string) (*models.Portfolio, error) {
			// Return a fresh copy each time to avoid shared state
			p := *portfolio
			return &p, nil
		},
	}

	firstDate := time.Now().AddDate(-1, 0, 0)
	cashFlowSvc := &mockCashFlowService{
		calculatePerformance: func(ctx context.Context, portfolioName string) (*models.CapitalPerformance, error) {
			return &models.CapitalPerformance{
				TransactionCount:     5,
				SimpleReturnPct:      10.0,
				FirstTransactionDate: &firstDate,
			}, nil
		},
	}

	srv := newTestServerWithCashFlow(portfolioSvc, cashFlowSvc)

	const goroutines = 20
	results := make(chan int, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			req := httptest.NewRequest(http.MethodGet, "/api/portfolios/test", nil)
			rec := httptest.NewRecorder()
			srv.handlePortfolioGet(rec, req, "test")
			results <- rec.Code
		}()
	}

	for i := 0; i < goroutines; i++ {
		code := <-results
		if code != http.StatusOK {
			t.Errorf("concurrent request returned %d, want 200", code)
		}
	}
}
