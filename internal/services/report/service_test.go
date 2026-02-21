package report

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

// Tests use the mock types defined in devils_advocate_test.go (same package).

// --- helpers ---

func newTestServiceForUnit() (*Service, *mockUserDataStore, *mockMarketService, *mockPortfolioService) {
	logger := common.NewLogger("error")
	userData := newMockUserDataStore()
	sm := &mockStorageManager{userData: userData}
	market := &mockMarketService{}
	portfolio := &mockPortfolioService{
		getPortfolioFn: func(_ context.Context, _ string) (*models.Portfolio, error) {
			return makeTestPortfolio("SMSF", makeTestHolding("BHP", "ASX")), nil
		},
		syncPortfolioFn: func(_ context.Context, _ string, _ bool) (*models.Portfolio, error) {
			return makeTestPortfolio("SMSF", makeTestHolding("BHP", "ASX")), nil
		},
		reviewPortfolioFn: func(_ context.Context, _ string, _ interfaces.ReviewOptions) (*models.PortfolioReview, error) {
			return makeTestReview("SMSF", "BHP"), nil
		},
	}
	signal := &mockSignalService{}
	svc := NewService(portfolio, market, signal, sm, logger)
	return svc, userData, market, portfolio
}

func seedReportForUnit(ctx context.Context, store *mockUserDataStore, portfolioName string) {
	report := &models.PortfolioReport{
		Portfolio:   portfolioName,
		GeneratedAt: time.Now().Add(-1 * time.Hour),
		Tickers:     []string{"BHP"},
		TickerReports: []models.TickerReport{
			{Ticker: "BHP", Name: "BHP Group", Markdown: "old report"},
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

// --- tests ---

func TestGenerateTickerReport_UsesCorePath(t *testing.T) {
	svc, userData, market, _ := newTestServiceForUnit()
	ctx := context.Background()

	seedReportForUnit(ctx, userData, "SMSF")

	_, err := svc.GenerateTickerReport(ctx, "SMSF", "BHP")
	if err != nil {
		t.Fatalf("GenerateTickerReport failed: %v", err)
	}

	if market.collectMarketDataCalls.Load() > 0 {
		t.Error("expected CollectMarketData NOT to be called, but it was")
	}
	if market.collectCoreMarketDataCalls.Load() == 0 {
		t.Error("expected CollectCoreMarketData to be called, but it was not")
	}
}

func TestGenerateReport_UsesCorePath(t *testing.T) {
	svc, _, market, _ := newTestServiceForUnit()
	ctx := context.Background()

	_, err := svc.GenerateReport(ctx, "SMSF", interfaces.ReportOptions{})
	if err != nil {
		t.Fatalf("GenerateReport failed: %v", err)
	}

	if market.collectMarketDataCalls.Load() > 0 {
		t.Error("expected CollectMarketData NOT to be called, but it was")
	}
	if market.collectCoreMarketDataCalls.Load() == 0 {
		t.Error("expected CollectCoreMarketData to be called, but it was not")
	}
}

func TestGenerateTickerReport_UpdatesExistingReport(t *testing.T) {
	svc, userData, _, _ := newTestServiceForUnit()
	ctx := context.Background()

	seedReportForUnit(ctx, userData, "SMSF")

	report, err := svc.GenerateTickerReport(ctx, "SMSF", "BHP")
	if err != nil {
		t.Fatalf("GenerateTickerReport failed: %v", err)
	}

	if len(report.TickerReports) != 1 {
		t.Fatalf("expected 1 ticker report, got %d", len(report.TickerReports))
	}
	if report.TickerReports[0].Ticker != "BHP" {
		t.Errorf("expected ticker BHP, got %s", report.TickerReports[0].Ticker)
	}
	if report.TickerReports[0].Markdown == "old report" {
		t.Error("expected ticker report markdown to be updated")
	}
}

func TestGenerateTickerReport_NoExistingReport(t *testing.T) {
	svc, _, _, _ := newTestServiceForUnit()
	ctx := context.Background()

	_, err := svc.GenerateTickerReport(ctx, "SMSF", "BHP")
	if err == nil {
		t.Fatal("expected error when no existing report")
	}
}
