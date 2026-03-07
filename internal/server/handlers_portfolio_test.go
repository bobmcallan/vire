package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func (m *mockPortfolioService) GetDailyGrowth(ctx context.Context, name string, opts interfaces.GrowthOptions) ([]models.GrowthDataPoint, error) {
	return nil, nil
}

func (m *mockPortfolioService) GetStockTimeline(_ context.Context, _, _ string, _, _ time.Time) ([]models.StockTimelinePoint, error) {
	return nil, nil
}
func (m *mockPortfolioService) GetPortfolioIndicators(ctx context.Context, name string) (*models.PortfolioIndicators, error) {
	if m.getPortfolioIndicators != nil {
		return m.getPortfolioIndicators(ctx, name)
	}
	return nil, nil
}
func (m *mockPortfolioService) RefreshTodaySnapshot(_ context.Context, _ string) error {
	return nil
}

func (m *mockPortfolioService) CreatePortfolio(_ context.Context, _ string, _ models.SourceType, _ string) (*models.Portfolio, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockPortfolioService) IsTimelineRebuilding(_ string) bool                       { return false }
func (m *mockPortfolioService) InvalidateAndRebuildTimeline(_ context.Context, _ string) {}
func (m *mockPortfolioService) ForceRebuildTimeline(_ context.Context, _ string) error   { return nil }
func (m *mockPortfolioService) SetHoldingNoteService(_ interfaces.HoldingNoteService)    {}
func (m *mockPortfolioService) SetAssetSetService(_ interfaces.AssetSetService)          {}

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

func (m *mockCashFlowService) UpdateAccount(ctx context.Context, portfolioName string, accountName string, update models.CashAccountUpdate) (*models.CashFlowLedger, error) {
	return nil, nil
}

func (m *mockCashFlowService) SetTransactions(ctx context.Context, portfolioName string, transactions []models.CashTransaction, notes string) (*models.CashFlowLedger, error) {
	return &models.CashFlowLedger{PortfolioName: portfolioName, Transactions: transactions, Notes: notes}, nil
}

func (m *mockCashFlowService) ClearLedger(ctx context.Context, portfolioName string) (*models.CashFlowLedger, error) {
	return &models.CashFlowLedger{
		PortfolioName: portfolioName,
		Accounts:      []models.CashAccount{{Name: models.DefaultTradingAccount, Type: "trading", IsTransactional: true}},
		Transactions:  []models.CashTransaction{},
	}, nil
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
		Name:           "test",
		PortfolioValue: 200.0,
		LastSynced:     time.Now(),
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
	if got.PortfolioValue != 200.0 {
		t.Errorf("expected total value 200.0, got %f", got.PortfolioValue)
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
	if resp.Error != "navexa_key_required" {
		t.Errorf("expected error 'navexa_key_required', got %q", resp.Error)
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
		Name:           "test",
		PortfolioValue: 500000.0,
		LastSynced:     now,
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
				ContributionsGross:   471000.0,
				WithdrawalsGross:     0,
				ContributionsNet:     471000.0,
				CurrentValue:         500000.0,
				ReturnSimplePct:      6.16,
				ReturnXirrPct:        15.2,
				FirstTransactionDate: &firstDate,
				TransactionCount:     5,
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
	if got.CapitalPerformance.ReturnXirrPct != 15.2 {
		t.Errorf("expected annualized return 15.2, got %f", got.CapitalPerformance.ReturnXirrPct)
	}
	if got.CapitalPerformance.TransactionCount != 5 {
		t.Errorf("expected transaction count 5, got %d", got.CapitalPerformance.TransactionCount)
	}
}

func TestHandlePortfolioGet_OmitsCapitalPerformanceWhenNoTransactions(t *testing.T) {
	portfolio := &models.Portfolio{
		Name:           "test",
		PortfolioValue: 100000.0,
		LastSynced:     time.Now(),
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
		Name:           "test",
		PortfolioValue: 100000.0,
		LastSynced:     time.Now(),
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
		Name:           "test",
		PortfolioValue: 100000.0,
		LastSynced:     time.Now(),
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
		Name:           "test",
		PortfolioValue: 1e12,
		LastSynced:     time.Now(),
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
				ContributionsGross:   1e6,
				WithdrawalsGross:     0,
				ContributionsNet:     1e6,
				CurrentValue:         1e12,
				ReturnSimplePct:      99999900.0, // 1e12/1e6 - 1 * 100
				ReturnXirrPct:        999.99,
				FirstTransactionDate: &firstDate,
				TransactionCount:     1,
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
		Name:           "test",
		PortfolioValue: 50000.0,
		LastSynced:     time.Now(),
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
				ContributionsGross:   100000,
				WithdrawalsGross:     0,
				ContributionsNet:     100000,
				CurrentValue:         50000,
				ReturnSimplePct:      -50.0,
				ReturnXirrPct:        -50.0,
				FirstTransactionDate: &firstDate,
				TransactionCount:     1,
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
	if got.CapitalPerformance.ReturnSimplePct != -50.0 {
		t.Errorf("SimpleReturnPct = %f, want -50.0", got.CapitalPerformance.ReturnSimplePct)
	}
}

func TestPortfolio_CapitalPerformanceOmittedInJSON(t *testing.T) {
	// When CapitalPerformance is nil, the field should be omitted from JSON
	p := models.Portfolio{
		Name:               "test",
		PortfolioValue:     100000,
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
		Name:           "test",
		PortfolioValue: 100000,
		CapitalPerformance: &models.CapitalPerformance{
			TransactionCount: 3,
			ReturnSimplePct:  12.5,
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
	// JSON without capital_performance field should deserialize cleanly
	jsonData := `{
		"id": "test",
		"name": "SMSF",
		"holdings": [],
		"equity_holdings_value": 100000,
		"equity_holdings_cost": 90000,
		"portfolio_value": 100000,
		"currency": "AUD",
		"capital_gross": 0,
		"last_synced": "2025-01-01T00:00:00Z",
		"created_at": "2025-01-01T00:00:00Z",
		"updated_at": "2025-01-01T00:00:00Z"
	}`

	var p models.Portfolio
	if err := json.Unmarshal([]byte(jsonData), &p); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}
	if p.CapitalPerformance != nil {
		t.Error("CapitalPerformance should be nil when not present in JSON")
	}
	if p.EquityHoldingsValue != 100000 {
		t.Errorf("EquityHoldingsValue = %v, want 100000", p.EquityHoldingsValue)
	}
	if p.EquityHoldingsCost != 90000 {
		t.Errorf("EquityHoldingsCost = %v, want 90000", p.EquityHoldingsCost)
	}
}

func TestHandlePortfolioGet_ExcludesClosedByDefault(t *testing.T) {
	portfolio := &models.Portfolio{
		Name: "test",
		Holdings: []models.Holding{
			{Ticker: "BHP.AU", Units: 100, Status: "open"},
			{Ticker: "CBA.AU", Units: 0, Status: "closed"},
			{Ticker: "NAB.AU", Units: 50, Status: "open"},
		},
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
	if len(got.Holdings) != 2 {
		t.Errorf("expected 2 holdings (closed filtered out), got %d", len(got.Holdings))
	}
	for _, h := range got.Holdings {
		if h.Units <= 0 {
			t.Errorf("expected all returned holdings to have units > 0, got %s with units %f", h.Ticker, h.Units)
		}
	}
}

func TestHandlePortfolioGet_IncludesClosedWhenRequested(t *testing.T) {
	portfolio := &models.Portfolio{
		Name: "test",
		Holdings: []models.Holding{
			{Ticker: "BHP.AU", Units: 100, Status: "open"},
			{Ticker: "CBA.AU", Units: 0, Status: "closed"},
			{Ticker: "NAB.AU", Units: 50, Status: "open"},
		},
	}
	svc := &mockPortfolioService{
		getPortfolio: func(ctx context.Context, name string) (*models.Portfolio, error) {
			return portfolio, nil
		},
	}
	srv := newTestServer(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/portfolios/test?include_closed=true", nil)
	rec := httptest.NewRecorder()
	srv.handlePortfolioGet(rec, req, "test")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	var got models.Portfolio
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(got.Holdings) != 3 {
		t.Errorf("expected 3 holdings (all included), got %d", len(got.Holdings))
	}
	var foundClosed bool
	for _, h := range got.Holdings {
		if h.Status == "closed" {
			foundClosed = true
		}
	}
	if !foundClosed {
		t.Error("expected to find a closed position when include_closed=true")
	}
}

func TestHandlePortfolioGet_ConcurrentCapitalPerformance(t *testing.T) {
	// Concurrent portfolio gets should not race on CapitalPerformance attachment
	portfolio := &models.Portfolio{
		Name:           "test",
		PortfolioValue: 200000.0,
		LastSynced:     time.Now(),
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
				ReturnSimplePct:      10.0,
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
