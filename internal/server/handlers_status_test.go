package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/app"
	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

// --- mock storage for status handler tests ---

type mockStatusStockIndexStore struct {
	mu      sync.Mutex
	entries map[string]*models.StockIndexEntry
}

func (m *mockStatusStockIndexStore) Upsert(_ context.Context, entry *models.StockIndexEntry) error {
	return nil
}
func (m *mockStatusStockIndexStore) Get(_ context.Context, ticker string) (*models.StockIndexEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e, ok := m.entries[ticker]; ok {
		return e, nil
	}
	return nil, fmt.Errorf("not found")
}
func (m *mockStatusStockIndexStore) List(_ context.Context) ([]*models.StockIndexEntry, error) {
	return nil, nil
}
func (m *mockStatusStockIndexStore) UpdateTimestamp(_ context.Context, _, _ string, _ time.Time) error {
	return nil
}
func (m *mockStatusStockIndexStore) ResetCollectionTimestamps(_ context.Context) (int, error) {
	return 0, nil
}
func (m *mockStatusStockIndexStore) Delete(_ context.Context, _ string) error { return nil }

type mockStatusJobQueueStore struct {
	mu   sync.Mutex
	jobs []*models.Job
}

func (m *mockStatusJobQueueStore) Enqueue(_ context.Context, _ *models.Job) error { return nil }
func (m *mockStatusJobQueueStore) Dequeue(_ context.Context) (*models.Job, error) { return nil, nil }
func (m *mockStatusJobQueueStore) Complete(_ context.Context, _ string, _ error, _ int64) error {
	return nil
}
func (m *mockStatusJobQueueStore) Cancel(_ context.Context, _ string) error             { return nil }
func (m *mockStatusJobQueueStore) SetPriority(_ context.Context, _ string, _ int) error { return nil }
func (m *mockStatusJobQueueStore) GetMaxPriority(_ context.Context) (int, error)        { return 0, nil }
func (m *mockStatusJobQueueStore) ListPending(_ context.Context, _ int) ([]*models.Job, error) {
	return nil, nil
}
func (m *mockStatusJobQueueStore) ListAll(_ context.Context, _ int) ([]*models.Job, error) {
	return nil, nil
}
func (m *mockStatusJobQueueStore) ListByTicker(_ context.Context, ticker string) ([]*models.Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*models.Job
	for _, j := range m.jobs {
		if j.Ticker == ticker {
			result = append(result, j)
		}
	}
	return result, nil
}
func (m *mockStatusJobQueueStore) ListByBatchID(_ context.Context, _ string) ([]*models.Job, error) {
	return nil, nil
}
func (m *mockStatusJobQueueStore) CountPending(_ context.Context) (int, error) { return 0, nil }
func (m *mockStatusJobQueueStore) HasPendingJob(_ context.Context, _, _ string) (bool, error) {
	return false, nil
}
func (m *mockStatusJobQueueStore) PurgeCompleted(_ context.Context, _ time.Time) (int, error) {
	return 0, nil
}
func (m *mockStatusJobQueueStore) CancelByTicker(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (m *mockStatusJobQueueStore) ResetRunningJobs(_ context.Context) (int, error) { return 0, nil }

type mockStatusTimelineStore struct {
	snapshots []models.TimelineSnapshot
}

func (m *mockStatusTimelineStore) GetRange(_ context.Context, _, _ string, _, _ time.Time) ([]models.TimelineSnapshot, error) {
	return m.snapshots, nil
}
func (m *mockStatusTimelineStore) GetLatest(_ context.Context, _, _ string) (*models.TimelineSnapshot, error) {
	if len(m.snapshots) > 0 {
		return &m.snapshots[len(m.snapshots)-1], nil
	}
	return nil, fmt.Errorf("not found")
}
func (m *mockStatusTimelineStore) SaveBatch(_ context.Context, _ []models.TimelineSnapshot) error {
	return nil
}
func (m *mockStatusTimelineStore) DeleteRange(_ context.Context, _, _ string, _, _ time.Time) (int, error) {
	return 0, nil
}
func (m *mockStatusTimelineStore) DeleteAll(_ context.Context, _, _ string) (int, error) {
	return 0, nil
}

type mockStatusStorageManager struct {
	stockIndex *mockStatusStockIndexStore
	jobQueue   *mockStatusJobQueueStore
	timeline   *mockStatusTimelineStore
}

func (m *mockStatusStorageManager) InternalStore() interfaces.InternalStore         { return nil }
func (m *mockStatusStorageManager) UserDataStore() interfaces.UserDataStore         { return nil }
func (m *mockStatusStorageManager) MarketDataStorage() interfaces.MarketDataStorage { return nil }
func (m *mockStatusStorageManager) SignalStorage() interfaces.SignalStorage         { return nil }
func (m *mockStatusStorageManager) StockIndexStore() interfaces.StockIndexStore     { return m.stockIndex }
func (m *mockStatusStorageManager) JobQueueStore() interfaces.JobQueueStore         { return m.jobQueue }
func (m *mockStatusStorageManager) FileStore() interfaces.FileStore                 { return nil }
func (m *mockStatusStorageManager) FeedbackStore() interfaces.FeedbackStore         { return nil }
func (m *mockStatusStorageManager) ChangelogStore() interfaces.ChangelogStore       { return nil }
func (m *mockStatusStorageManager) OAuthStore() interfaces.OAuthStore               { return nil }
func (m *mockStatusStorageManager) TimelineStore() interfaces.TimelineStore         { return m.timeline }
func (m *mockStatusStorageManager) DataPath() string                                { return "" }
func (m *mockStatusStorageManager) WriteRaw(_, _ string, _ []byte) error            { return nil }
func (m *mockStatusStorageManager) PurgeDerivedData(_ context.Context) (map[string]int, error) {
	return nil, nil
}
func (m *mockStatusStorageManager) PurgeReports(_ context.Context) (int, error) { return 0, nil }
func (m *mockStatusStorageManager) Close() error                                { return nil }

func newStatusTestServer(
	portfolioSvc interfaces.PortfolioService,
	storage interfaces.StorageManager,
) *Server {
	logger := common.NewLoggerFromConfig(common.LoggingConfig{Level: "disabled"})
	cfg := common.NewDefaultConfig()
	a := &app.App{
		Config:           cfg,
		PortfolioService: portfolioSvc,
		CashFlowService:  &mockCashFlowService{},
		Storage:          storage,
		Logger:           logger,
	}
	return &Server{app: a, logger: logger}
}

// injectUserContext creates a request context with a valid user to pass requireNavexaContext.
func injectUserContext(r *http.Request) *http.Request {
	uc := &common.UserContext{
		UserID:       "test-user",
		NavexaAPIKey: "test-key",
	}
	ctx := common.WithUserContext(r.Context(), uc)
	return r.WithContext(ctx)
}

// --- Tests ---

func TestHandlePortfolioStatus_ReturnsReadiness(t *testing.T) {
	now := time.Now()
	portfolio := &models.Portfolio{
		Name: "main",
		Holdings: []models.Holding{
			{Ticker: "BHP", Exchange: "ASX", Name: "BHP Group", Units: 100},
			{Ticker: "CBA", Exchange: "ASX", Name: "Commonwealth Bank", Units: 50},
			{Ticker: "NAB", Exchange: "ASX", Name: "NAB", Units: 25},
		},
	}

	svc := &mockPortfolioService{
		getPortfolio: func(_ context.Context, _ string) (*models.Portfolio, error) {
			return portfolio, nil
		},
	}

	stockIdx := &mockStatusStockIndexStore{
		entries: map[string]*models.StockIndexEntry{
			"BHP.AU": {
				Ticker:                  "BHP.AU",
				Name:                    "BHP Group Limited",
				EODCollectedAt:          now.Add(-1 * time.Hour),
				FundamentalsCollectedAt: now.Add(-2 * time.Hour),
			},
			"CBA.AU": {
				Ticker:         "CBA.AU",
				Name:           "Commonwealth Bank",
				EODCollectedAt: now.Add(-1 * time.Hour),
				// FundamentalsCollectedAt is zero
			},
			// NAB.AU is missing from stock index (not yet indexed)
		},
	}

	jobQueue := &mockStatusJobQueueStore{
		jobs: []*models.Job{
			{Ticker: "BHP.AU", Status: models.JobStatusPending, JobType: models.JobTypeCollectFilingPdfs},
			{Ticker: "CBA.AU", Status: models.JobStatusRunning, JobType: models.JobTypeCollectFundamentals},
			{Ticker: "CBA.AU", Status: models.JobStatusPending, JobType: models.JobTypeCollectFilingSummaries},
			{Ticker: "NAB.AU", Status: models.JobStatusPending, JobType: models.JobTypeCollectEOD},
		},
	}

	timeline := &mockStatusTimelineStore{
		snapshots: []models.TimelineSnapshot{
			{Date: now.Add(-48 * time.Hour), ComputedAt: now.Add(-48 * time.Hour)},
			{Date: now.Add(-24 * time.Hour), ComputedAt: now.Add(-24 * time.Hour)},
			{Date: now, ComputedAt: now},
		},
	}

	storage := &mockStatusStorageManager{
		stockIndex: stockIdx,
		jobQueue:   jobQueue,
		timeline:   timeline,
	}

	srv := newStatusTestServer(svc, storage)
	req := httptest.NewRequest(http.MethodGet, "/api/portfolios/main/status", nil)
	req = injectUserContext(req)
	rec := httptest.NewRecorder()

	srv.handlePortfolioStatus(rec, req, "main")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Check portfolio name
	if resp["portfolio_name"] != "main" {
		t.Errorf("expected portfolio_name=main, got %v", resp["portfolio_name"])
	}

	// Check holdings counts
	holdings := resp["holdings"].(map[string]interface{})
	if holdings["total"] != float64(3) {
		t.Errorf("expected total=3, got %v", holdings["total"])
	}
	if holdings["eod_ready"] != float64(2) {
		t.Errorf("expected eod_ready=2 (BHP + CBA), got %v", holdings["eod_ready"])
	}
	if holdings["fundamentals_ready"] != float64(1) {
		t.Errorf("expected fundamentals_ready=1 (BHP only), got %v", holdings["fundamentals_ready"])
	}

	// Check job counts
	jobs := resp["jobs"].(map[string]interface{})
	if jobs["pending"] != float64(3) {
		t.Errorf("expected pending=3, got %v", jobs["pending"])
	}
	if jobs["running"] != float64(1) {
		t.Errorf("expected running=1, got %v", jobs["running"])
	}

	// Check timeline
	tl := resp["timeline"].(map[string]interface{})
	if tl["snapshots"] != float64(3) {
		t.Errorf("expected snapshots=3, got %v", tl["snapshots"])
	}

	// Check tickers array
	tickers := resp["tickers"].([]interface{})
	if len(tickers) != 3 {
		t.Fatalf("expected 3 tickers, got %d", len(tickers))
	}
}

func TestHandlePortfolioStatus_EmptyPortfolio(t *testing.T) {
	portfolio := &models.Portfolio{
		Name:     "empty",
		Holdings: []models.Holding{},
	}

	svc := &mockPortfolioService{
		getPortfolio: func(_ context.Context, _ string) (*models.Portfolio, error) {
			return portfolio, nil
		},
	}

	storage := &mockStatusStorageManager{
		stockIndex: &mockStatusStockIndexStore{entries: map[string]*models.StockIndexEntry{}},
		jobQueue:   &mockStatusJobQueueStore{},
		timeline:   &mockStatusTimelineStore{},
	}

	srv := newStatusTestServer(svc, storage)
	req := httptest.NewRequest(http.MethodGet, "/api/portfolios/empty/status", nil)
	req = injectUserContext(req)
	rec := httptest.NewRecorder()

	srv.handlePortfolioStatus(rec, req, "empty")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)

	holdings := resp["holdings"].(map[string]interface{})
	if holdings["total"] != float64(0) {
		t.Errorf("expected total=0, got %v", holdings["total"])
	}
}

func TestHandlePortfolioStatus_PortfolioNotFound(t *testing.T) {
	svc := &mockPortfolioService{
		getPortfolio: func(_ context.Context, _ string) (*models.Portfolio, error) {
			return nil, fmt.Errorf("not found")
		},
	}

	storage := &mockStatusStorageManager{
		stockIndex: &mockStatusStockIndexStore{entries: map[string]*models.StockIndexEntry{}},
		jobQueue:   &mockStatusJobQueueStore{},
		timeline:   &mockStatusTimelineStore{},
	}

	srv := newStatusTestServer(svc, storage)
	req := httptest.NewRequest(http.MethodGet, "/api/portfolios/missing/status", nil)
	req = injectUserContext(req)
	rec := httptest.NewRecorder()

	srv.handlePortfolioStatus(rec, req, "missing")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rec.Code)
	}
}

func TestHandlePortfolioStatus_MethodNotAllowed(t *testing.T) {
	svc := &mockPortfolioService{
		getPortfolio: func(_ context.Context, _ string) (*models.Portfolio, error) {
			return &models.Portfolio{Name: "test"}, nil
		},
	}

	storage := &mockStatusStorageManager{
		stockIndex: &mockStatusStockIndexStore{entries: map[string]*models.StockIndexEntry{}},
		jobQueue:   &mockStatusJobQueueStore{},
		timeline:   &mockStatusTimelineStore{},
	}

	srv := newStatusTestServer(svc, storage)
	req := httptest.NewRequest(http.MethodPost, "/api/portfolios/test/status", nil)
	rec := httptest.NewRecorder()

	srv.handlePortfolioStatus(rec, req, "test")

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", rec.Code)
	}
}

func TestHandlePortfolioStatus_SkipsZeroUnits(t *testing.T) {
	portfolio := &models.Portfolio{
		Name: "test",
		Holdings: []models.Holding{
			{Ticker: "BHP", Exchange: "ASX", Units: 100},
			{Ticker: "SOLD", Exchange: "ASX", Units: 0}, // sold position — should be excluded
		},
	}

	svc := &mockPortfolioService{
		getPortfolio: func(_ context.Context, _ string) (*models.Portfolio, error) {
			return portfolio, nil
		},
	}

	storage := &mockStatusStorageManager{
		stockIndex: &mockStatusStockIndexStore{entries: map[string]*models.StockIndexEntry{
			"BHP.AU": {Ticker: "BHP.AU", EODCollectedAt: time.Now()},
		}},
		jobQueue: &mockStatusJobQueueStore{},
		timeline: &mockStatusTimelineStore{},
	}

	srv := newStatusTestServer(svc, storage)
	req := httptest.NewRequest(http.MethodGet, "/api/portfolios/test/status", nil)
	req = injectUserContext(req)
	rec := httptest.NewRecorder()

	srv.handlePortfolioStatus(rec, req, "test")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)

	holdings := resp["holdings"].(map[string]interface{})
	if holdings["total"] != float64(1) {
		t.Errorf("expected total=1 (SOLD should be excluded), got %v", holdings["total"])
	}

	tickers := resp["tickers"].([]interface{})
	if len(tickers) != 1 {
		t.Errorf("expected 1 ticker in response, got %d", len(tickers))
	}
}
