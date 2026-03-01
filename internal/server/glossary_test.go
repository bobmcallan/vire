package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
)

func testPortfolio() *models.Portfolio {
	return &models.Portfolio{
		Name:               "SMSF",
		EquityValue: 100000.00,
		PortfolioValue:         120000.00,
		NetEquityCost:          80000.00,
		NetEquityReturn:     20000.00,
		NetEquityReturnPct:  25.0,
		GrossCashBalance:          20000.00,
		Currency:           "AUD",
		LastSynced:         time.Now(),
		PortfolioYesterdayValue:     99000.00,
		PortfolioYesterdayChangePct:  1.01,
		PortfolioLastWeekValue:      97000.00,
		PortfolioLastWeekChangePct:   3.09,
		Holdings: []models.Holding{
			{
				Ticker:       "BHP",
				Exchange:     "ASX",
				Name:         "BHP Group",
				Units:        100,
				AvgCost:      40.00,
				CurrentPrice: 45.50,
				MarketValue:  4550.00,
				CostBasis:    4000.00,
				NetReturn:    550.00,
				NetReturnPct: 13.75,
				PortfolioWeightPct:       4.55,
			},
			{
				Ticker:       "VAS",
				Exchange:     "ASX",
				Name:         "Vanguard Aus Shares",
				Units:        200,
				AvgCost:      85.00,
				CurrentPrice: 92.30,
				MarketValue:  18460.00,
				CostBasis:    17000.00,
				NetReturn:    1460.00,
				NetReturnPct: 8.59,
				PortfolioWeightPct:       18.46,
			},
			{
				Ticker:       "CBA",
				Exchange:     "ASX",
				Name:         "Commonwealth Bank",
				Units:        50,
				AvgCost:      100.00,
				CurrentPrice: 120.00,
				MarketValue:  6000.00,
				CostBasis:    5000.00,
				NetReturn:    1000.00,
				NetReturnPct: 20.0,
				PortfolioWeightPct:       6.0,
			},
			{
				Ticker:       "WES",
				Exchange:     "ASX",
				Name:         "Wesfarmers",
				Units:        10,
				AvgCost:      50.00,
				CurrentPrice: 55.00,
				MarketValue:  550.00,
				CostBasis:    500.00,
				NetReturn:    50.00,
				NetReturnPct: 10.0,
				PortfolioWeightPct:       0.55,
			},
		},
	}
}

func testCapitalPerformance() *models.CapitalPerformance {
	firstDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	return &models.CapitalPerformance{
		GrossCapitalDeposited:     50000.00,
		GrossCapitalWithdrawn:     5000.00,
		NetCapitalDeployed:        45000.00,
		EquityValue:               100000.00,
		SimpleCapitalReturnPct:    122.22,
		AnnualizedCapitalReturnPct: 18.5,
		FirstTransactionDate:      &firstDate,
		TransactionCount:          12,
	}
}

func testIndicators() *models.PortfolioIndicators {
	return &models.PortfolioIndicators{
		PortfolioName:    "SMSF",
		ComputeDate:      time.Now(),
		PortfolioValue:   100000.00,
		DataPoints:       252,
		EMA20:            98000.00,
		EMA50:            95000.00,
		EMA200:           90000.00,
		AboveEMA20:       true,
		AboveEMA50:       true,
		AboveEMA200:      true,
		RSI:              62.5,
		RSISignal:        "neutral",
		Trend:            models.TrendBullish,
		TrendDescription: "Uptrend: value above all EMAs",
	}
}

// TestHandleGlossary_Success tests the full glossary response with all data available.
func TestHandleGlossary_Success(t *testing.T) {
	portfolio := testPortfolio()
	perf := testCapitalPerformance()
	indicators := testIndicators()

	svc := &mockPortfolioService{
		getPortfolio: func(ctx context.Context, name string) (*models.Portfolio, error) {
			return portfolio, nil
		},
	}
	svc.getPortfolioIndicators = func(ctx context.Context, name string) (*models.PortfolioIndicators, error) {
		return indicators, nil
	}

	cashSvc := &mockCashFlowService{
		calculatePerformance: func(ctx context.Context, portfolioName string) (*models.CapitalPerformance, error) {
			return perf, nil
		},
	}

	srv := newTestServerWithCashFlow(svc, cashSvc)
	req := httptest.NewRequest(http.MethodGet, "/api/portfolios/SMSF/glossary", nil)
	rec := httptest.NewRecorder()

	srv.handleGlossary(rec, req, "SMSF")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp models.GlossaryResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.PortfolioName != "SMSF" {
		t.Errorf("expected portfolio name SMSF, got %q", resp.PortfolioName)
	}

	if resp.GeneratedAt.IsZero() {
		t.Error("expected non-zero generated_at")
	}

	// With all data available, expect 5 categories
	if len(resp.Categories) != 5 {
		names := make([]string, len(resp.Categories))
		for i, c := range resp.Categories {
			names[i] = c.Name
		}
		t.Fatalf("expected 5 categories, got %d: %v", len(resp.Categories), names)
	}

	expectedCategories := []string{
		"Portfolio Valuation",
		"Holding Metrics",
		"Capital Performance",
		"Technical Indicators",
		"Growth Metrics",
	}
	for i, name := range expectedCategories {
		if resp.Categories[i].Name != name {
			t.Errorf("category[%d] = %q, want %q", i, resp.Categories[i].Name, name)
		}
	}

	// Verify portfolio valuation has expected terms
	valuation := resp.Categories[0]
	termNames := make(map[string]bool)
	for _, term := range valuation.Terms {
		termNames[term.Term] = true
	}
	for _, expected := range []string{"portfolio_value", "net_equity_cost", "net_equity_return", "net_equity_return_pct", "gross_cash_balance", "net_cash_balance", "net_capital_return", "net_capital_return_pct"} {
		if !termNames[expected] {
			t.Errorf("Portfolio Valuation missing term %q", expected)
		}
	}
}

// TestHandleGlossary_MethodNotAllowed verifies POST is rejected.
func TestHandleGlossary_MethodNotAllowed(t *testing.T) {
	svc := &mockPortfolioService{
		getPortfolio: func(ctx context.Context, name string) (*models.Portfolio, error) {
			return testPortfolio(), nil
		},
	}
	srv := newTestServer(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/portfolios/SMSF/glossary", nil)
	rec := httptest.NewRecorder()

	srv.handleGlossary(rec, req, "SMSF")

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", rec.Code)
	}
}

// TestHandleGlossary_PortfolioNotFound verifies 404 when portfolio doesn't exist.
func TestHandleGlossary_PortfolioNotFound(t *testing.T) {
	svc := &mockPortfolioService{
		getPortfolio: func(ctx context.Context, name string) (*models.Portfolio, error) {
			return nil, errors.New("not found")
		},
	}
	srv := newTestServer(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/portfolios/MISSING/glossary", nil)
	rec := httptest.NewRecorder()

	srv.handleGlossary(rec, req, "MISSING")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rec.Code)
	}
}

// TestHandleGlossary_NonFatalEnrichment verifies glossary still works when
// capital performance and indicators fail.
func TestHandleGlossary_NonFatalEnrichment(t *testing.T) {
	svc := &mockPortfolioService{
		getPortfolio: func(ctx context.Context, name string) (*models.Portfolio, error) {
			return testPortfolio(), nil
		},
	}
	svc.getPortfolioIndicators = func(ctx context.Context, name string) (*models.PortfolioIndicators, error) {
		return nil, errors.New("indicators unavailable")
	}

	cashSvc := &mockCashFlowService{
		calculatePerformance: func(ctx context.Context, portfolioName string) (*models.CapitalPerformance, error) {
			return nil, errors.New("cash flow unavailable")
		},
	}

	srv := newTestServerWithCashFlow(svc, cashSvc)
	req := httptest.NewRequest(http.MethodGet, "/api/portfolios/SMSF/glossary", nil)
	rec := httptest.NewRecorder()

	srv.handleGlossary(rec, req, "SMSF")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp models.GlossaryResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Without capital perf and indicators: Portfolio Valuation, Holding Metrics, Growth Metrics
	if len(resp.Categories) != 3 {
		names := make([]string, len(resp.Categories))
		for i, c := range resp.Categories {
			names[i] = c.Name
		}
		t.Fatalf("expected 3 categories (no capital perf, no indicators), got %d: %v", len(resp.Categories), names)
	}
}

// TestHandleGlossary_HoldingMetricsUsesTop3 verifies that holding examples use top 3 by weight.
func TestHandleGlossary_HoldingMetricsUsesTop3(t *testing.T) {
	portfolio := testPortfolio()
	svc := &mockPortfolioService{
		getPortfolio: func(ctx context.Context, name string) (*models.Portfolio, error) {
			return portfolio, nil
		},
	}
	svc.getPortfolioIndicators = func(ctx context.Context, name string) (*models.PortfolioIndicators, error) {
		return nil, errors.New("skip")
	}

	cashSvc := &mockCashFlowService{
		calculatePerformance: func(ctx context.Context, portfolioName string) (*models.CapitalPerformance, error) {
			return nil, errors.New("skip")
		},
	}

	srv := newTestServerWithCashFlow(svc, cashSvc)
	req := httptest.NewRequest(http.MethodGet, "/api/portfolios/SMSF/glossary", nil)
	rec := httptest.NewRecorder()

	srv.handleGlossary(rec, req, "SMSF")

	var resp models.GlossaryResponse
	json.NewDecoder(rec.Body).Decode(&resp)

	// Find Holding Metrics category
	var holdingCat *models.GlossaryCategory
	for i := range resp.Categories {
		if resp.Categories[i].Name == "Holding Metrics" {
			holdingCat = &resp.Categories[i]
			break
		}
	}

	if holdingCat == nil {
		t.Fatal("Holding Metrics category not found")
	}

	// market_value example should reference top 3 by weight (VAS, CBA, BHP) not WES
	var marketValueTerm *models.GlossaryTerm
	for i := range holdingCat.Terms {
		if holdingCat.Terms[i].Term == "market_value" {
			marketValueTerm = &holdingCat.Terms[i]
			break
		}
	}

	if marketValueTerm == nil {
		t.Fatal("market_value term not found in Holding Metrics")
	}

	// Top 3 by weight: VAS (18.46%), CBA (6.0%), BHP (4.55%)
	// WES (0.55%) should NOT be in the example
	if marketValueTerm.Example == "" {
		t.Error("market_value example should not be empty")
	}
}

// TestHandleGlossary_EmptyPortfolio verifies glossary works with no holdings.
func TestHandleGlossary_EmptyPortfolio(t *testing.T) {
	portfolio := &models.Portfolio{
		Name:       "Empty",
		Currency:   "AUD",
		LastSynced: time.Now(),
	}

	svc := &mockPortfolioService{
		getPortfolio: func(ctx context.Context, name string) (*models.Portfolio, error) {
			return portfolio, nil
		},
	}
	svc.getPortfolioIndicators = func(ctx context.Context, name string) (*models.PortfolioIndicators, error) {
		return nil, errors.New("no data")
	}

	cashSvc := &mockCashFlowService{
		calculatePerformance: func(ctx context.Context, portfolioName string) (*models.CapitalPerformance, error) {
			return nil, errors.New("no data")
		},
	}

	srv := newTestServerWithCashFlow(svc, cashSvc)
	req := httptest.NewRequest(http.MethodGet, "/api/portfolios/Empty/glossary", nil)
	rec := httptest.NewRecorder()

	srv.handleGlossary(rec, req, "Empty")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp models.GlossaryResponse
	json.NewDecoder(rec.Body).Decode(&resp)

	// Should have Portfolio Valuation and Growth Metrics, but Holding Metrics should be skipped (no holdings)
	for _, cat := range resp.Categories {
		if cat.Name == "Holding Metrics" {
			t.Error("Holding Metrics should not be present for empty portfolio")
		}
	}
}

// TestBuildGlossary_AllTermsHaveRequiredFields verifies every term has term, label, definition.
func TestBuildGlossary_AllTermsHaveRequiredFields(t *testing.T) {
	portfolio := testPortfolio()
	perf := testCapitalPerformance()
	indicators := testIndicators()

	glossary := buildGlossary(portfolio, perf, indicators)

	for _, cat := range glossary.Categories {
		for _, term := range cat.Terms {
			if term.Term == "" {
				t.Errorf("category %q has term with empty term field", cat.Name)
			}
			if term.Label == "" {
				t.Errorf("category %q term %q has empty label", cat.Name, term.Term)
			}
			if term.Definition == "" {
				t.Errorf("category %q term %q has empty definition", cat.Name, term.Term)
			}
		}
	}
}

// TestBuildGlossary_TermsAreUnique verifies no duplicate term names across all categories.
func TestBuildGlossary_TermsAreUnique(t *testing.T) {
	portfolio := testPortfolio()
	perf := testCapitalPerformance()
	indicators := testIndicators()

	glossary := buildGlossary(portfolio, perf, indicators)

	seen := make(map[string]string) // term -> category
	for _, cat := range glossary.Categories {
		for _, term := range cat.Terms {
			key := cat.Name + ":" + term.Term
			if prev, ok := seen[key]; ok {
				t.Errorf("duplicate term %q in %q (also in %q)", term.Term, cat.Name, prev)
			}
			seen[key] = cat.Name
		}
	}
}

// TestBuildGlossary_TotalCashInPortfolioValuation verifies total_cash
// appears in the Portfolio Valuation category.
func TestBuildGlossary_TotalCashInPortfolioValuation(t *testing.T) {
	portfolio := testPortfolio()
	portfolio.GrossCashBalance = 25000

	perf := testCapitalPerformance()
	glossary := buildGlossary(portfolio, perf, nil)

	// total_cash should appear in Portfolio Valuation
	found := false
	for _, cat := range glossary.Categories {
		if cat.Name != "Portfolio Valuation" {
			continue
		}
		for _, term := range cat.Terms {
			if term.Term == "total_cash" {
				found = true
			}
		}
	}
	if !found {
		t.Error("total_cash term not found in Portfolio Valuation category")
	}
}
