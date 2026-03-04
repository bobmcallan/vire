package server

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Devils-advocate stress tests for feedback batch fixes (handler/glossary level).
// Targets: Fix 2 (handler guard), Fix 3 (glossary growth), Fix 4 (glossary duplicates).

// --- Fix 2: Handler NetCapitalDeployed != 0 guard ---

func TestHandlerPortfolioGet_NegativeNetCapitalDeployed_ComputesReturn(t *testing.T) {
	// Fix 2: After changing guard from > 0 to != 0, negative NetCapitalDeployed
	// should still produce NetCapitalReturn and NetCapitalReturnPct.
	portfolio := &models.Portfolio{
		Name:           "SMSF",
		PortfolioValue: 50000,
		EquityValue:    50000,
	}
	portfolioSvc := &mockPortfolioService{
		getPortfolio:  func(_ context.Context, _ string) (*models.Portfolio, error) { return portfolio, nil },
		syncPortfolio: func(_ context.Context, _ string, _ bool) (*models.Portfolio, error) { return portfolio, nil },
	}
	cashFlowSvc := &mockCashFlowService{
		calculatePerformance: func(_ context.Context, _ string) (*models.CapitalPerformance, error) {
			return &models.CapitalPerformance{
				NetCapitalDeployed: -50000, // negative: more withdrawn than deposited
				TransactionCount:   3,
				CurrentValue:       50000,
			}, nil
		},
	}
	srv := newTestServerWithCashFlow(portfolioSvc, cashFlowSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/portfolios/SMSF", nil)
	w := httptest.NewRecorder()
	srv.handlePortfolioGet(w, req, "SMSF")

	require.Equal(t, http.StatusOK, w.Code)

	var result models.Portfolio
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))

	// NetCapitalDeployed = -50000, PortfolioValue = 50000
	// NetCapitalReturn = 50000 - (-50000) = 100000
	// NetCapitalReturnPct = 100000 / (-50000) * 100 = -200%
	assert.Equal(t, 100000.0, result.NetCapitalReturn,
		"negative deployed should still compute return")
	assert.InDelta(t, -200.0, result.NetCapitalReturnPct, 0.01,
		"negative deployed produces negative return pct")
}

func TestHandlerPortfolioGet_ZeroNetCapitalDeployed_NoReturn(t *testing.T) {
	// Zero NetCapitalDeployed should NOT compute return (division by zero).
	portfolio := &models.Portfolio{
		Name:           "SMSF",
		PortfolioValue: 50000,
		EquityValue:    50000,
	}
	portfolioSvc := &mockPortfolioService{
		getPortfolio:  func(_ context.Context, _ string) (*models.Portfolio, error) { return portfolio, nil },
		syncPortfolio: func(_ context.Context, _ string, _ bool) (*models.Portfolio, error) { return portfolio, nil },
	}
	cashFlowSvc := &mockCashFlowService{
		calculatePerformance: func(_ context.Context, _ string) (*models.CapitalPerformance, error) {
			return &models.CapitalPerformance{
				NetCapitalDeployed: 0,
				TransactionCount:   2,
				CurrentValue:       50000,
			}, nil
		},
	}
	srv := newTestServerWithCashFlow(portfolioSvc, cashFlowSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/portfolios/SMSF", nil)
	w := httptest.NewRecorder()
	srv.handlePortfolioGet(w, req, "SMSF")

	require.Equal(t, http.StatusOK, w.Code)

	var result models.Portfolio
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))

	// != 0 guard: 0 does NOT pass, so no return computed.
	assert.Equal(t, 0.0, result.NetCapitalReturn)
	assert.Equal(t, 0.0, result.NetCapitalReturnPct)
}

func TestHandlerPortfolioGet_TinyNetCapitalDeployed_HugeReturn(t *testing.T) {
	// Edge case: very small positive deployed with large portfolio value.
	// The handler should compute a huge return without NaN/Inf.
	portfolio := &models.Portfolio{
		Name:           "SMSF",
		PortfolioValue: 500000,
		EquityValue:    500000,
	}
	portfolioSvc := &mockPortfolioService{
		getPortfolio:  func(_ context.Context, _ string) (*models.Portfolio, error) { return portfolio, nil },
		syncPortfolio: func(_ context.Context, _ string, _ bool) (*models.Portfolio, error) { return portfolio, nil },
	}
	cashFlowSvc := &mockCashFlowService{
		calculatePerformance: func(_ context.Context, _ string) (*models.CapitalPerformance, error) {
			return &models.CapitalPerformance{
				NetCapitalDeployed: 0.01, // tiny
				TransactionCount:   1,
				CurrentValue:       500000,
			}, nil
		},
	}
	srv := newTestServerWithCashFlow(portfolioSvc, cashFlowSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/portfolios/SMSF", nil)
	w := httptest.NewRecorder()
	srv.handlePortfolioGet(w, req, "SMSF")

	require.Equal(t, http.StatusOK, w.Code)

	var result models.Portfolio
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))

	// (500000 - 0.01) / 0.01 * 100 = ~5,000,000,000%
	assert.False(t, math.IsNaN(result.NetCapitalReturnPct), "tiny deployed must not NaN")
	assert.False(t, math.IsInf(result.NetCapitalReturnPct, 0), "tiny deployed must not Inf")
	assert.Greater(t, result.NetCapitalReturnPct, 1e6,
		"FINDING: tiny deployed capital produces astronomically high return pct")
}

func TestHandlerPortfolioGet_TransactionCountZero_NoPerf(t *testing.T) {
	// If CalculatePerformance returns TransactionCount=0, the handler
	// should NOT attach CapitalPerformance.
	portfolio := &models.Portfolio{
		Name:           "SMSF",
		PortfolioValue: 50000,
		EquityValue:    50000,
	}
	portfolioSvc := &mockPortfolioService{
		getPortfolio:  func(_ context.Context, _ string) (*models.Portfolio, error) { return portfolio, nil },
		syncPortfolio: func(_ context.Context, _ string, _ bool) (*models.Portfolio, error) { return portfolio, nil },
	}
	cashFlowSvc := &mockCashFlowService{
		calculatePerformance: func(_ context.Context, _ string) (*models.CapitalPerformance, error) {
			return &models.CapitalPerformance{
				NetCapitalDeployed: 100000,
				TransactionCount:   0, // no transactions
				CurrentValue:       50000,
			}, nil
		},
	}
	srv := newTestServerWithCashFlow(portfolioSvc, cashFlowSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/portfolios/SMSF", nil)
	w := httptest.NewRecorder()
	srv.handlePortfolioGet(w, req, "SMSF")

	require.Equal(t, http.StatusOK, w.Code)

	var result models.Portfolio
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))

	// TransactionCount=0 → guard skips attaching perf
	assert.Nil(t, result.CapitalPerformance,
		"zero transactions should not attach CapitalPerformance")
	assert.Equal(t, 0.0, result.NetCapitalReturn)
}

// --- Fix 3: Glossary growth uses PortfolioValue ---

func TestGlossaryGrowth_UsesPortfolioValue(t *testing.T) {
	// Fix 3: buildGrowthCategory must use p.PortfolioValue, not p.EquityValue.
	p := &models.Portfolio{
		EquityValue:                 265000,
		PortfolioValue:              471000,
		PortfolioYesterdayValue:     470000,
		PortfolioLastWeekValue:      465000,
		PortfolioYesterdayChangePct: 0.21,
		PortfolioLastWeekChangePct:  1.29,
	}

	cat := buildGrowthCategory(p)

	require.Len(t, cat.Terms, 2, "Growth category should have exactly 2 terms (no duplicates)")

	// yesterday_change = PortfolioValue - PortfolioYesterdayValue = 471000 - 470000 = 1000
	assert.Equal(t, 1000.0, cat.Terms[0].Value,
		"yesterday_change must use PortfolioValue (1000), not EquityValue (-205000)")

	// last_week_change = PortfolioValue - PortfolioLastWeekValue = 471000 - 465000 = 6000
	assert.Equal(t, 6000.0, cat.Terms[1].Value,
		"last_week_change must use PortfolioValue (6000), not EquityValue (-200000)")
}

func TestGlossaryGrowth_NoDuplicateTerms(t *testing.T) {
	// Fix 4: Growth category should NOT contain gross_cash_balance or net_capital_deployed
	// (those belong in Valuation and Capital categories).
	p := &models.Portfolio{
		PortfolioValue:          100000,
		PortfolioYesterdayValue: 99000,
		PortfolioLastWeekValue:  98000,
		GrossCashBalance:        50000,
	}

	cat := buildGrowthCategory(p)

	for _, term := range cat.Terms {
		assert.NotEqual(t, "gross_cash_balance", term.Term,
			"gross_cash_balance should NOT be in Growth category")
		assert.NotEqual(t, "net_capital_deployed", term.Term,
			"net_capital_deployed should NOT be in Growth category")
	}

	assert.Len(t, cat.Terms, 2, "Growth should have exactly yesterday_change and last_week_change")
}

func TestGlossaryGrowth_ZeroYesterdayValue(t *testing.T) {
	// Edge case: PortfolioYesterdayValue = 0 (no data yet)
	p := &models.Portfolio{
		PortfolioValue:          100000,
		PortfolioYesterdayValue: 0,
		PortfolioLastWeekValue:  0,
	}

	cat := buildGrowthCategory(p)

	// Change = 100000 - 0 = 100000 (full portfolio value as "change")
	assert.Equal(t, 100000.0, cat.Terms[0].Value)
}

// --- Fix 4: Glossary corrections ---

func TestGlossaryCapital_SimpleReturnFormula_UsesPortfolioValue(t *testing.T) {
	// Fix 4: simple_capital_return_pct formula should reference "portfolio_value",
	// not "equity_value".
	cp := &models.CapitalPerformance{
		NetCapitalDeployed:     100000,
		CurrentValue:           110000,
		SimpleCapitalReturnPct: 10.0,
	}

	cat := buildCapitalCategory(cp)

	var found bool
	for _, term := range cat.Terms {
		if term.Term == "simple_capital_return_pct" {
			found = true
			assert.Contains(t, term.Formula, "portfolio_value",
				"formula should reference portfolio_value")
			assert.NotContains(t, term.Formula, "equity_value",
				"formula should NOT reference equity_value")
			break
		}
	}
	assert.True(t, found, "simple_capital_return_pct term should exist in Capital category")
}

func TestGlossaryCapital_SimpleReturnExample_UsesCurrentValue(t *testing.T) {
	// Fix 4: Example should use cp.CurrentValue (renamed from EquityValue).
	cp := &models.CapitalPerformance{
		NetCapitalDeployed:     477000,
		CurrentValue:           471000,
		SimpleCapitalReturnPct: -1.26,
	}

	cat := buildCapitalCategory(cp)

	for _, term := range cat.Terms {
		if term.Term == "simple_capital_return_pct" {
			// Example should contain "$471,000.00" (CurrentValue), not "$265,000.00" (EquityValue)
			assert.Contains(t, term.Example, "$471000.00",
				"example should use CurrentValue ($471000)")
			break
		}
	}
}
