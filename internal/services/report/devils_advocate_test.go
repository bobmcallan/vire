package report

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

// Devils-advocate stress tests for GenerateTickerReport.
// Validates edge cases, failure modes, and behavioral correctness
// of the CollectMarketData -> CollectCoreMarketData change.

// ============================================================================
// Mocks — shared by service_test.go (same package)
// ============================================================================

type mockMarketService struct {
	collectMarketDataCalls     atomic.Int64
	collectCoreMarketDataCalls atomic.Int64
	collectMarketDataFn        func(ctx context.Context, tickers []string, includeNews bool, force bool) error
	collectCoreMarketDataFn    func(ctx context.Context, tickers []string, force bool) error
}

func (m *mockMarketService) CollectMarketData(ctx context.Context, tickers []string, includeNews bool, force bool) error {
	m.collectMarketDataCalls.Add(1)
	if m.collectMarketDataFn != nil {
		return m.collectMarketDataFn(ctx, tickers, includeNews, force)
	}
	return nil
}
func (m *mockMarketService) CollectCoreMarketData(ctx context.Context, tickers []string, force bool) error {
	m.collectCoreMarketDataCalls.Add(1)
	if m.collectCoreMarketDataFn != nil {
		return m.collectCoreMarketDataFn(ctx, tickers, force)
	}
	return nil
}
func (m *mockMarketService) CollectEOD(_ context.Context, _ string, _ bool) error { return nil }
func (m *mockMarketService) CollectFundamentals(_ context.Context, _ string, _ bool) error {
	return nil
}
func (m *mockMarketService) CollectFilingsIndex(_ context.Context, _ string, _ bool) error {
	return nil
}
func (m *mockMarketService) CollectFilingPdfs(_ context.Context, _ string, _ bool) error { return nil }
func (m *mockMarketService) CollectNews(_ context.Context, _ string, _ bool) error       { return nil }
func (m *mockMarketService) CollectFilingSummaries(_ context.Context, _ string, _ bool) error {
	return nil
}
func (m *mockMarketService) CollectTimeline(_ context.Context, _ string, _ bool) error { return nil }
func (m *mockMarketService) CollectNewsIntelligence(_ context.Context, _ string, _ bool) error {
	return nil
}
func (m *mockMarketService) CollectBulkEOD(_ context.Context, _ string, _ bool) error { return nil }
func (m *mockMarketService) GetStockData(_ context.Context, _ string, _ interfaces.StockDataInclude) (*models.StockData, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *mockMarketService) FindSnipeBuys(_ context.Context, _ interfaces.SnipeOptions) ([]*models.SnipeBuy, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *mockMarketService) ScreenStocks(_ context.Context, _ interfaces.ScreenOptions) ([]*models.ScreenCandidate, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *mockMarketService) FunnelScreen(_ context.Context, _ interfaces.FunnelOptions) (*models.FunnelResult, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *mockMarketService) RefreshStaleData(_ context.Context, _ string) error { return nil }
func (m *mockMarketService) ScanMarket(_ context.Context, _ models.ScanQuery) (*models.ScanResponse, error) {
	return nil, nil
}
func (m *mockMarketService) ScanFields() *models.ScanFieldsResponse { return nil }
func (m *mockMarketService) ReadFiling(_ context.Context, _, _ string) (*models.FilingContent, error) {
	return nil, fmt.Errorf("not implemented")
}

type mockPortfolioService struct {
	getPortfolioFn    func(ctx context.Context, name string) (*models.Portfolio, error)
	syncPortfolioFn   func(ctx context.Context, name string, force bool) (*models.Portfolio, error)
	reviewPortfolioFn func(ctx context.Context, name string, options interfaces.ReviewOptions) (*models.PortfolioReview, error)
}

func (m *mockPortfolioService) SyncPortfolio(ctx context.Context, name string, force bool) (*models.Portfolio, error) {
	if m.syncPortfolioFn != nil {
		return m.syncPortfolioFn(ctx, name, force)
	}
	return nil, fmt.Errorf("not implemented")
}
func (m *mockPortfolioService) GetPortfolio(ctx context.Context, name string) (*models.Portfolio, error) {
	if m.getPortfolioFn != nil {
		return m.getPortfolioFn(ctx, name)
	}
	return nil, fmt.Errorf("not implemented")
}
func (m *mockPortfolioService) ListPortfolios(_ context.Context) ([]string, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *mockPortfolioService) ReviewPortfolio(ctx context.Context, name string, options interfaces.ReviewOptions) (*models.PortfolioReview, error) {
	if m.reviewPortfolioFn != nil {
		return m.reviewPortfolioFn(ctx, name, options)
	}
	return nil, fmt.Errorf("not implemented")
}
func (m *mockPortfolioService) ReviewWatchlist(_ context.Context, _ string, _ interfaces.ReviewOptions) (*models.WatchlistReview, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *mockPortfolioService) GetPortfolioSnapshot(_ context.Context, _ string, _ time.Time) (*models.PortfolioSnapshot, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *mockPortfolioService) GetPortfolioGrowth(_ context.Context, _ string) ([]models.GrowthDataPoint, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *mockPortfolioService) GetDailyGrowth(_ context.Context, _ string, _ interfaces.GrowthOptions) ([]models.GrowthDataPoint, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *mockPortfolioService) GetPortfolioIndicators(_ context.Context, _ string) (*models.PortfolioIndicators, error) {
	return nil, fmt.Errorf("not implemented")
}

type mockSignalService struct {
	detectSignalsCalls atomic.Int64
	detectSignalsFn    func(ctx context.Context, tickers []string, signalTypes []string, force bool) ([]*models.TickerSignals, error)
}

func (m *mockSignalService) DetectSignals(ctx context.Context, tickers []string, signalTypes []string, force bool) ([]*models.TickerSignals, error) {
	m.detectSignalsCalls.Add(1)
	if m.detectSignalsFn != nil {
		return m.detectSignalsFn(ctx, tickers, signalTypes, force)
	}
	return nil, nil
}
func (m *mockSignalService) ComputeSignals(_ context.Context, _ string, _ *models.MarketData) (*models.TickerSignals, error) {
	return nil, fmt.Errorf("not implemented")
}

type mockUserDataStore struct {
	data map[string]*models.UserRecord // keyed by "userID:subject:key"
}

func newMockUserDataStore() *mockUserDataStore {
	return &mockUserDataStore{data: make(map[string]*models.UserRecord)}
}

func (m *mockUserDataStore) compositeKey(userID, subject, key string) string {
	return userID + ":" + subject + ":" + key
}
func (m *mockUserDataStore) Get(_ context.Context, userID, subject, key string) (*models.UserRecord, error) {
	rec, ok := m.data[m.compositeKey(userID, subject, key)]
	if !ok {
		return nil, fmt.Errorf("%s '%s' not found", subject, key)
	}
	return rec, nil
}
func (m *mockUserDataStore) Put(_ context.Context, record *models.UserRecord) error {
	m.data[m.compositeKey(record.UserID, record.Subject, record.Key)] = record
	return nil
}
func (m *mockUserDataStore) Delete(_ context.Context, userID, subject, key string) error {
	delete(m.data, m.compositeKey(userID, subject, key))
	return nil
}
func (m *mockUserDataStore) List(_ context.Context, _ string, _ string) ([]*models.UserRecord, error) {
	return nil, nil
}
func (m *mockUserDataStore) Query(_ context.Context, _, _ string, _ interfaces.QueryOptions) ([]*models.UserRecord, error) {
	return nil, nil
}
func (m *mockUserDataStore) DeleteBySubject(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (m *mockUserDataStore) Close() error { return nil }

type mockStorageManager struct {
	userData *mockUserDataStore
}

func (m *mockStorageManager) InternalStore() interfaces.InternalStore         { return nil }
func (m *mockStorageManager) UserDataStore() interfaces.UserDataStore         { return m.userData }
func (m *mockStorageManager) MarketDataStorage() interfaces.MarketDataStorage { return nil }
func (m *mockStorageManager) SignalStorage() interfaces.SignalStorage         { return nil }
func (m *mockStorageManager) StockIndexStore() interfaces.StockIndexStore     { return nil }
func (m *mockStorageManager) JobQueueStore() interfaces.JobQueueStore         { return nil }
func (m *mockStorageManager) FileStore() interfaces.FileStore                 { return nil }
func (m *mockStorageManager) FeedbackStore() interfaces.FeedbackStore         { return nil }
func (m *mockStorageManager) OAuthStore() interfaces.OAuthStore               { return nil }
func (m *mockStorageManager) DataPath() string                                { return "/tmp/test" }
func (m *mockStorageManager) WriteRaw(_, _ string, _ []byte) error            { return nil }
func (m *mockStorageManager) PurgeDerivedData(_ context.Context) (map[string]int, error) {
	return nil, nil
}
func (m *mockStorageManager) PurgeReports(_ context.Context) (int, error) { return 0, nil }
func (m *mockStorageManager) Close() error                                { return nil }

// ============================================================================
// Test helpers — shared by service_test.go
// ============================================================================

func makeTestHolding(ticker, exchange string) models.Holding {
	return models.Holding{
		Ticker:       ticker,
		Name:         ticker + " Group",
		Exchange:     exchange,
		Units:        100,
		CurrentPrice: 42.0,
		MarketValue:  4200.0,
		AvgCost:      40.0,
		NetReturn:    200.0,
	}
}

func makeTestPortfolio(name string, holdings ...models.Holding) *models.Portfolio {
	return &models.Portfolio{
		Name:     name,
		Holdings: holdings,
	}
}

func makeTestReview(name string, ticker string) *models.PortfolioReview {
	return &models.PortfolioReview{
		PortfolioName: name,
		ReviewDate:    time.Now(),
		HoldingReviews: []models.HoldingReview{
			{
				Holding:        makeTestHolding(ticker, "ASX"),
				ActionRequired: "HOLD",
				ActionReason:   "No signals",
			},
		},
	}
}

// ============================================================================
// DA-1. CollectCoreMarketData failure does not abort ticker report generation
// ============================================================================
//
// When CollectCoreMarketData returns an error, GenerateTickerReport should
// log a warning and continue (same behavior as before with CollectMarketData).

func TestDA_GenerateTickerReport_CollectionFailure_Continues(t *testing.T) {
	logger := common.NewLogger("error")
	userData := newMockUserDataStore()
	sm := &mockStorageManager{userData: userData}
	market := &mockMarketService{
		collectCoreMarketDataFn: func(_ context.Context, _ []string, _ bool) error {
			return fmt.Errorf("EODHD API down")
		},
	}
	portfolio := &mockPortfolioService{
		getPortfolioFn: func(_ context.Context, _ string) (*models.Portfolio, error) {
			return makeTestPortfolio("SMSF", makeTestHolding("BHP", "ASX")), nil
		},
		reviewPortfolioFn: func(_ context.Context, _ string, _ interfaces.ReviewOptions) (*models.PortfolioReview, error) {
			return makeTestReview("SMSF", "BHP"), nil
		},
	}
	signal := &mockSignalService{}
	svc := NewService(portfolio, market, signal, sm, logger)

	ctx := context.Background()
	storeTestReport(ctx, userData, "SMSF", "BHP")

	// Should NOT return an error — collection failure is non-fatal
	report, err := svc.GenerateTickerReport(ctx, "SMSF", "BHP")
	if err != nil {
		t.Fatalf("collection failure should not abort report generation, got: %v", err)
	}
	if report == nil {
		t.Fatal("report should not be nil")
	}
}

// ============================================================================
// DA-2. Ticker not found in portfolio falls back to .AU suffix
// ============================================================================

func TestDA_GenerateTickerReport_FallbackTicker(t *testing.T) {
	logger := common.NewLogger("error")
	userData := newMockUserDataStore()
	sm := &mockStorageManager{userData: userData}

	var collectedTickers []string
	market := &mockMarketService{
		collectCoreMarketDataFn: func(_ context.Context, tickers []string, _ bool) error {
			collectedTickers = tickers
			return nil
		},
	}
	portfolio := &mockPortfolioService{
		getPortfolioFn: func(_ context.Context, _ string) (*models.Portfolio, error) {
			return nil, fmt.Errorf("portfolio not found") // force fallback
		},
		reviewPortfolioFn: func(_ context.Context, _ string, _ interfaces.ReviewOptions) (*models.PortfolioReview, error) {
			return makeTestReview("SMSF", "XYZ"), nil
		},
	}
	signal := &mockSignalService{}
	svc := NewService(portfolio, market, signal, sm, logger)

	ctx := context.Background()
	storeTestReport(ctx, userData, "SMSF", "XYZ")

	_, err := svc.GenerateTickerReport(ctx, "SMSF", "XYZ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(collectedTickers) != 1 || collectedTickers[0] != "XYZ.AU" {
		t.Errorf("expected fallback ticker XYZ.AU, got %v", collectedTickers)
	}
}

// ============================================================================
// DA-3. Ticker not in existing report — append behavior
// ============================================================================

func TestDA_GenerateTickerReport_NewTickerAppended(t *testing.T) {
	logger := common.NewLogger("error")
	userData := newMockUserDataStore()
	sm := &mockStorageManager{userData: userData}
	market := &mockMarketService{}
	portfolio := &mockPortfolioService{
		getPortfolioFn: func(_ context.Context, _ string) (*models.Portfolio, error) {
			return makeTestPortfolio("SMSF",
				makeTestHolding("BHP", "ASX"),
				makeTestHolding("RIO", "ASX"),
			), nil
		},
		reviewPortfolioFn: func(_ context.Context, _ string, _ interfaces.ReviewOptions) (*models.PortfolioReview, error) {
			return &models.PortfolioReview{
				PortfolioName: "SMSF",
				ReviewDate:    time.Now(),
				HoldingReviews: []models.HoldingReview{
					{Holding: makeTestHolding("BHP", "ASX"), ActionRequired: "HOLD"},
					{Holding: makeTestHolding("RIO", "ASX"), ActionRequired: "BUY", ActionReason: "signal"},
				},
			}, nil
		},
	}
	signal := &mockSignalService{}
	svc := NewService(portfolio, market, signal, sm, logger)

	// Seed report with only BHP
	storeTestReport(context.Background(), userData, "SMSF", "BHP")

	report, err := svc.GenerateTickerReport(context.Background(), "SMSF", "RIO")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, tr := range report.TickerReports {
		if tr.Ticker == "RIO" {
			found = true
			if tr.Markdown == "" {
				t.Error("appended ticker should have non-empty markdown")
			}
		}
	}
	if !found {
		t.Error("RIO should be appended to TickerReports")
	}

	rioInTickers := false
	for _, tk := range report.Tickers {
		if tk == "RIO" {
			rioInTickers = true
		}
	}
	if !rioInTickers {
		t.Error("RIO should be appended to Tickers list")
	}
}

// ============================================================================
// DA-4. Ticker not found in portfolio review — returns error
// ============================================================================

func TestDA_GenerateTickerReport_TickerNotInReview(t *testing.T) {
	logger := common.NewLogger("error")
	userData := newMockUserDataStore()
	sm := &mockStorageManager{userData: userData}
	market := &mockMarketService{}
	portfolio := &mockPortfolioService{
		getPortfolioFn: func(_ context.Context, _ string) (*models.Portfolio, error) {
			return makeTestPortfolio("SMSF", makeTestHolding("BHP", "ASX")), nil
		},
		reviewPortfolioFn: func(_ context.Context, _ string, _ interfaces.ReviewOptions) (*models.PortfolioReview, error) {
			return makeTestReview("SMSF", "BHP"), nil // only BHP, not GHOST
		},
	}
	signal := &mockSignalService{}
	svc := NewService(portfolio, market, signal, sm, logger)

	storeTestReport(context.Background(), userData, "SMSF", "GHOST")

	_, err := svc.GenerateTickerReport(context.Background(), "SMSF", "GHOST")
	if err == nil {
		t.Fatal("expected error when ticker not found in portfolio review")
	}
	if !strings.Contains(err.Error(), "not found in portfolio") {
		t.Errorf("error should mention 'not found in portfolio', got: %v", err)
	}
}

// ============================================================================
// DA-5. Case-insensitive ticker matching
// ============================================================================

func TestDA_GenerateTickerReport_CaseInsensitiveMatch(t *testing.T) {
	logger := common.NewLogger("error")
	userData := newMockUserDataStore()
	sm := &mockStorageManager{userData: userData}
	market := &mockMarketService{}
	portfolio := &mockPortfolioService{
		getPortfolioFn: func(_ context.Context, _ string) (*models.Portfolio, error) {
			return makeTestPortfolio("SMSF", makeTestHolding("BHP", "ASX")), nil
		},
		reviewPortfolioFn: func(_ context.Context, _ string, _ interfaces.ReviewOptions) (*models.PortfolioReview, error) {
			return makeTestReview("SMSF", "BHP"), nil
		},
	}
	signal := &mockSignalService{}
	svc := NewService(portfolio, market, signal, sm, logger)

	storeTestReport(context.Background(), userData, "SMSF", "BHP")

	// Request with lowercase ticker — should match
	report, err := svc.GenerateTickerReport(context.Background(), "SMSF", "bhp")
	if err != nil {
		t.Fatalf("case-insensitive match failed: %v", err)
	}

	for _, tr := range report.TickerReports {
		if strings.EqualFold(tr.Ticker, "BHP") && tr.Markdown == "old report" {
			t.Error("report markdown should have been regenerated")
		}
	}
}

// ============================================================================
// DA-6. ReviewPortfolio failure is fatal — returns error
// ============================================================================

func TestDA_GenerateTickerReport_ReviewFailure_Fatal(t *testing.T) {
	logger := common.NewLogger("error")
	userData := newMockUserDataStore()
	sm := &mockStorageManager{userData: userData}
	market := &mockMarketService{}
	portfolio := &mockPortfolioService{
		getPortfolioFn: func(_ context.Context, _ string) (*models.Portfolio, error) {
			return makeTestPortfolio("SMSF", makeTestHolding("BHP", "ASX")), nil
		},
		reviewPortfolioFn: func(_ context.Context, _ string, _ interfaces.ReviewOptions) (*models.PortfolioReview, error) {
			return nil, fmt.Errorf("navexa API down")
		},
	}
	signal := &mockSignalService{}
	svc := NewService(portfolio, market, signal, sm, logger)

	storeTestReport(context.Background(), userData, "SMSF", "BHP")

	_, err := svc.GenerateTickerReport(context.Background(), "SMSF", "BHP")
	if err == nil {
		t.Fatal("expected error when ReviewPortfolio fails")
	}
	if !strings.Contains(err.Error(), "review portfolio") {
		t.Errorf("error should mention review failure, got: %v", err)
	}
}

// ============================================================================
// DA-7. Signal detection failure is non-fatal
// ============================================================================

func TestDA_GenerateTickerReport_SignalDetectionFailure_NonFatal(t *testing.T) {
	logger := common.NewLogger("error")
	userData := newMockUserDataStore()
	sm := &mockStorageManager{userData: userData}
	market := &mockMarketService{}
	portfolio := &mockPortfolioService{
		getPortfolioFn: func(_ context.Context, _ string) (*models.Portfolio, error) {
			return makeTestPortfolio("SMSF", makeTestHolding("BHP", "ASX")), nil
		},
		reviewPortfolioFn: func(_ context.Context, _ string, _ interfaces.ReviewOptions) (*models.PortfolioReview, error) {
			return makeTestReview("SMSF", "BHP"), nil
		},
	}
	signal := &mockSignalService{
		detectSignalsFn: func(_ context.Context, _ []string, _ []string, _ bool) ([]*models.TickerSignals, error) {
			return nil, fmt.Errorf("signal computation crashed")
		},
	}
	svc := NewService(portfolio, market, signal, sm, logger)

	storeTestReport(context.Background(), userData, "SMSF", "BHP")

	report, err := svc.GenerateTickerReport(context.Background(), "SMSF", "BHP")
	if err != nil {
		t.Fatalf("signal detection failure should not abort report generation, got: %v", err)
	}
	if report == nil {
		t.Fatal("report should not be nil")
	}
}

// ============================================================================
// DA-8. Empty ticker string — hostile input
// ============================================================================

func TestDA_GenerateTickerReport_EmptyTicker(t *testing.T) {
	logger := common.NewLogger("error")
	userData := newMockUserDataStore()
	sm := &mockStorageManager{userData: userData}
	market := &mockMarketService{}
	portfolio := &mockPortfolioService{
		getPortfolioFn: func(_ context.Context, _ string) (*models.Portfolio, error) {
			return makeTestPortfolio("SMSF", makeTestHolding("BHP", "ASX")), nil
		},
		reviewPortfolioFn: func(_ context.Context, _ string, _ interfaces.ReviewOptions) (*models.PortfolioReview, error) {
			return makeTestReview("SMSF", "BHP"), nil
		},
	}
	signal := &mockSignalService{}
	svc := NewService(portfolio, market, signal, sm, logger)

	storeTestReport(context.Background(), userData, "SMSF", "BHP")

	_, err := svc.GenerateTickerReport(context.Background(), "SMSF", "")
	if err == nil {
		t.Error("expected error for empty ticker")
	}
}

// ============================================================================
// DA-9. Empty portfolio name — hostile input
// ============================================================================

func TestDA_GenerateTickerReport_EmptyPortfolio(t *testing.T) {
	logger := common.NewLogger("error")
	userData := newMockUserDataStore()
	sm := &mockStorageManager{userData: userData}
	market := &mockMarketService{}
	portfolio := &mockPortfolioService{}
	signal := &mockSignalService{}
	svc := NewService(portfolio, market, signal, sm, logger)

	_, err := svc.GenerateTickerReport(context.Background(), "", "BHP")
	if err == nil {
		t.Error("expected error for empty portfolio name")
	}
}

// ============================================================================
// DA-10. Verify DetectSignals is still called after the change
// ============================================================================

func TestDA_GenerateTickerReport_DetectSignals_StillCalled(t *testing.T) {
	logger := common.NewLogger("error")
	userData := newMockUserDataStore()
	sm := &mockStorageManager{userData: userData}
	market := &mockMarketService{}
	portfolio := &mockPortfolioService{
		getPortfolioFn: func(_ context.Context, _ string) (*models.Portfolio, error) {
			return makeTestPortfolio("SMSF", makeTestHolding("BHP", "ASX")), nil
		},
		reviewPortfolioFn: func(_ context.Context, _ string, _ interfaces.ReviewOptions) (*models.PortfolioReview, error) {
			return makeTestReview("SMSF", "BHP"), nil
		},
	}
	signal := &mockSignalService{}
	svc := NewService(portfolio, market, signal, sm, logger)

	storeTestReport(context.Background(), userData, "SMSF", "BHP")

	_, err := svc.GenerateTickerReport(context.Background(), "SMSF", "BHP")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if signal.detectSignalsCalls.Load() == 0 {
		t.Error("DetectSignals should still be called after the CollectCoreMarketData change")
	}
}

// ============================================================================
// DA-11. Concurrent GenerateTickerReport calls — last-writer-wins documented
// ============================================================================

func TestDA_GenerateTickerReport_ConcurrentCalls_LastWriterWins(t *testing.T) {
	logger := common.NewLogger("error")
	userData := newMockUserDataStore()
	sm := &mockStorageManager{userData: userData}
	market := &mockMarketService{}

	tickers := []string{"BHP", "RIO", "CBA"}
	holdings := make([]models.Holding, len(tickers))
	reviews := make([]models.HoldingReview, len(tickers))
	for i, tk := range tickers {
		holdings[i] = models.Holding{Ticker: tk, Exchange: "ASX", Name: tk + " Corp"}
		reviews[i] = models.HoldingReview{Holding: holdings[i], ActionRequired: "HOLD"}
	}

	portfolio := &mockPortfolioService{
		getPortfolioFn: func(_ context.Context, _ string) (*models.Portfolio, error) {
			return &models.Portfolio{Name: "SMSF", Holdings: holdings}, nil
		},
		reviewPortfolioFn: func(_ context.Context, _ string, _ interfaces.ReviewOptions) (*models.PortfolioReview, error) {
			return &models.PortfolioReview{PortfolioName: "SMSF", HoldingReviews: reviews}, nil
		},
	}
	signal := &mockSignalService{}
	svc := NewService(portfolio, market, signal, sm, logger)

	// Seed report with all tickers
	tickerReports := make([]models.TickerReport, len(tickers))
	for i, tk := range tickers {
		tickerReports[i] = models.TickerReport{Ticker: tk, Name: tk + " Corp", Markdown: "old"}
	}
	report := &models.PortfolioReport{
		Portfolio:     "SMSF",
		GeneratedAt:   time.Now().Add(-1 * time.Hour),
		TickerReports: tickerReports,
		Tickers:       tickers,
	}
	data, _ := json.Marshal(report)
	userData.Put(context.Background(), &models.UserRecord{
		UserID: "default", Subject: "report", Key: "SMSF", Value: string(data),
	})

	errCh := make(chan error, len(tickers))
	for _, tk := range tickers {
		go func(ticker string) {
			_, err := svc.GenerateTickerReport(context.Background(), "SMSF", ticker)
			errCh <- err
		}(tk)
	}

	errors := 0
	for range tickers {
		if err := <-errCh; err != nil {
			errors++
		}
	}

	rec, _ := userData.Get(context.Background(), "default", "report", "SMSF")
	var finalReport models.PortfolioReport
	json.Unmarshal([]byte(rec.Value), &finalReport)

	updated := 0
	for _, tr := range finalReport.TickerReports {
		if tr.Markdown != "old" {
			updated++
		}
	}

	if updated < len(tickers) {
		t.Logf("INFO: %d/%d tickers updated in concurrent scenario (expected — last-writer-wins). "+
			"GenerateTickerReport is not designed for concurrent same-portfolio calls.", updated, len(tickers))
	}
}

// ============================================================================
// DA-12. Verify force=false is passed to CollectCoreMarketData
// ============================================================================

func TestDA_GenerateTickerReport_ForceIsFalse(t *testing.T) {
	logger := common.NewLogger("error")
	userData := newMockUserDataStore()
	sm := &mockStorageManager{userData: userData}

	var capturedForce *bool
	market := &mockMarketService{
		collectCoreMarketDataFn: func(_ context.Context, _ []string, force bool) error {
			capturedForce = &force
			return nil
		},
	}
	portfolio := &mockPortfolioService{
		getPortfolioFn: func(_ context.Context, _ string) (*models.Portfolio, error) {
			return makeTestPortfolio("SMSF", makeTestHolding("BHP", "ASX")), nil
		},
		reviewPortfolioFn: func(_ context.Context, _ string, _ interfaces.ReviewOptions) (*models.PortfolioReview, error) {
			return makeTestReview("SMSF", "BHP"), nil
		},
	}
	signal := &mockSignalService{}
	svc := NewService(portfolio, market, signal, sm, logger)

	storeTestReport(context.Background(), userData, "SMSF", "BHP")

	_, _ = svc.GenerateTickerReport(context.Background(), "SMSF", "BHP")

	if capturedForce == nil {
		t.Fatal("CollectCoreMarketData was not called")
	}
	if *capturedForce {
		t.Error("CollectCoreMarketData should be called with force=false")
	}
}

// ============================================================================
// DA-13. Corrupted stored report — malformed JSON
// ============================================================================

func TestDA_GenerateTickerReport_CorruptedStoredReport(t *testing.T) {
	logger := common.NewLogger("error")
	userData := newMockUserDataStore()
	sm := &mockStorageManager{userData: userData}
	market := &mockMarketService{}
	portfolio := &mockPortfolioService{
		getPortfolioFn: func(_ context.Context, _ string) (*models.Portfolio, error) {
			return makeTestPortfolio("SMSF", makeTestHolding("BHP", "ASX")), nil
		},
	}
	signal := &mockSignalService{}
	svc := NewService(portfolio, market, signal, sm, logger)

	userData.Put(context.Background(), &models.UserRecord{
		UserID: "default", Subject: "report", Key: "SMSF", Value: "{corrupted json!!!",
	})

	_, err := svc.GenerateTickerReport(context.Background(), "SMSF", "BHP")
	if err == nil {
		t.Fatal("expected error for corrupted stored report")
	}
}

// ============================================================================
// DA-14. Report with large ticker list — performance edge case
// ============================================================================

func TestDA_GenerateTickerReport_LargeTickerList(t *testing.T) {
	logger := common.NewLogger("error")
	userData := newMockUserDataStore()
	sm := &mockStorageManager{userData: userData}
	market := &mockMarketService{}

	n := 200
	holdings := make([]models.Holding, n)
	reviews := make([]models.HoldingReview, n)
	tickerReports := make([]models.TickerReport, n)
	tickerList := make([]string, n)
	for i := 0; i < n; i++ {
		tk := fmt.Sprintf("T%03d", i)
		holdings[i] = models.Holding{Ticker: tk, Exchange: "ASX", Name: tk}
		reviews[i] = models.HoldingReview{Holding: holdings[i], ActionRequired: "HOLD"}
		tickerReports[i] = models.TickerReport{Ticker: tk, Name: tk, Markdown: "old"}
		tickerList[i] = tk
	}

	portfolio := &mockPortfolioService{
		getPortfolioFn: func(_ context.Context, _ string) (*models.Portfolio, error) {
			return &models.Portfolio{Name: "SMSF", Holdings: holdings}, nil
		},
		reviewPortfolioFn: func(_ context.Context, _ string, _ interfaces.ReviewOptions) (*models.PortfolioReview, error) {
			return &models.PortfolioReview{PortfolioName: "SMSF", HoldingReviews: reviews}, nil
		},
	}
	signal := &mockSignalService{}
	svc := NewService(portfolio, market, signal, sm, logger)

	report := &models.PortfolioReport{
		Portfolio:     "SMSF",
		GeneratedAt:   time.Now().Add(-1 * time.Hour),
		TickerReports: tickerReports,
		Tickers:       tickerList,
	}
	data, _ := json.Marshal(report)
	userData.Put(context.Background(), &models.UserRecord{
		UserID: "default", Subject: "report", Key: "SMSF", Value: string(data),
	})

	start := time.Now()
	result, err := svc.GenerateTickerReport(context.Background(), "SMSF", "T199")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if elapsed > 5*time.Second {
		t.Errorf("GenerateTickerReport took %v for %d tickers — possible quadratic behavior", elapsed, n)
	}

	for _, tr := range result.TickerReports {
		if tr.Ticker == "T199" && tr.Markdown == "old" {
			t.Error("T199 should have been regenerated")
		}
	}
}

// ============================================================================
// Helper
// ============================================================================

func storeTestReport(ctx context.Context, store *mockUserDataStore, portfolioName, ticker string) {
	report := &models.PortfolioReport{
		Portfolio:   portfolioName,
		GeneratedAt: time.Now().Add(-1 * time.Hour),
		Tickers:     []string{ticker},
		TickerReports: []models.TickerReport{
			{Ticker: ticker, Name: ticker + " Group", Markdown: "old report"},
		},
	}
	data, _ := json.Marshal(report)
	store.Put(ctx, &models.UserRecord{
		UserID:  "default",
		Subject: "report",
		Key:     portfolioName,
		Value:   string(data),
	})
}
