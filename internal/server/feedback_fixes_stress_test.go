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
		Name:                "SMSF",
		PortfolioValue:      50000,
		EquityHoldingsValue: 50000,
	}
	portfolioSvc := &mockPortfolioService{
		getPortfolio:  func(_ context.Context, _ string) (*models.Portfolio, error) { return portfolio, nil },
		syncPortfolio: func(_ context.Context, _ string, _ bool) (*models.Portfolio, error) { return portfolio, nil },
	}
	cashFlowSvc := &mockCashFlowService{
		calculatePerformance: func(_ context.Context, _ string) (*models.CapitalPerformance, error) {
			return &models.CapitalPerformance{
				ContributionsNet: -50000, // negative: more withdrawn than deposited
				TransactionCount: 3,
				CurrentValue:     50000,
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
	assert.Equal(t, 100000.0, result.PortfolioReturn,
		"negative deployed should still compute return")
	assert.InDelta(t, -200.0, result.PortfolioReturnPct, 0.01,
		"negative deployed produces negative return pct")
}

func TestHandlerPortfolioGet_ZeroNetCapitalDeployed_NoReturn(t *testing.T) {
	// Zero NetCapitalDeployed should NOT compute return (division by zero).
	portfolio := &models.Portfolio{
		Name:                "SMSF",
		PortfolioValue:      50000,
		EquityHoldingsValue: 50000,
	}
	portfolioSvc := &mockPortfolioService{
		getPortfolio:  func(_ context.Context, _ string) (*models.Portfolio, error) { return portfolio, nil },
		syncPortfolio: func(_ context.Context, _ string, _ bool) (*models.Portfolio, error) { return portfolio, nil },
	}
	cashFlowSvc := &mockCashFlowService{
		calculatePerformance: func(_ context.Context, _ string) (*models.CapitalPerformance, error) {
			return &models.CapitalPerformance{
				ContributionsNet: 0,
				TransactionCount: 2,
				CurrentValue:     50000,
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
	assert.Equal(t, 0.0, result.PortfolioReturn)
	assert.Equal(t, 0.0, result.PortfolioReturnPct)
}

func TestHandlerPortfolioGet_TinyNetCapitalDeployed_HugeReturn(t *testing.T) {
	// Edge case: very small positive deployed with large portfolio value.
	// The handler should compute a huge return without NaN/Inf.
	portfolio := &models.Portfolio{
		Name:                "SMSF",
		PortfolioValue:      500000,
		EquityHoldingsValue: 500000,
	}
	portfolioSvc := &mockPortfolioService{
		getPortfolio:  func(_ context.Context, _ string) (*models.Portfolio, error) { return portfolio, nil },
		syncPortfolio: func(_ context.Context, _ string, _ bool) (*models.Portfolio, error) { return portfolio, nil },
	}
	cashFlowSvc := &mockCashFlowService{
		calculatePerformance: func(_ context.Context, _ string) (*models.CapitalPerformance, error) {
			return &models.CapitalPerformance{
				ContributionsNet: 0.01, // tiny
				TransactionCount: 1,
				CurrentValue:     500000,
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
	assert.False(t, math.IsNaN(result.PortfolioReturnPct), "tiny deployed must not NaN")
	assert.False(t, math.IsInf(result.PortfolioReturnPct, 0), "tiny deployed must not Inf")
	assert.Greater(t, result.PortfolioReturnPct, 1e6,
		"FINDING: tiny deployed capital produces astronomically high return pct")
}

func TestHandlerPortfolioGet_TransactionCountZero_NoPerf(t *testing.T) {
	// If CalculatePerformance returns TransactionCount=0, the handler
	// should NOT attach CapitalPerformance.
	portfolio := &models.Portfolio{
		Name:                "SMSF",
		PortfolioValue:      50000,
		EquityHoldingsValue: 50000,
	}
	portfolioSvc := &mockPortfolioService{
		getPortfolio:  func(_ context.Context, _ string) (*models.Portfolio, error) { return portfolio, nil },
		syncPortfolio: func(_ context.Context, _ string, _ bool) (*models.Portfolio, error) { return portfolio, nil },
	}
	cashFlowSvc := &mockCashFlowService{
		calculatePerformance: func(_ context.Context, _ string) (*models.CapitalPerformance, error) {
			return &models.CapitalPerformance{
				ContributionsNet: 100000,
				TransactionCount: 0, // no transactions
				CurrentValue:     50000,
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
	assert.Equal(t, 0.0, result.PortfolioReturn)
}

// --- Fix 3: Glossary growth uses PortfolioValue ---

func TestGlossaryGrowth_UsesPortfolioValue(t *testing.T) {
	// Fix 3: buildGrowthCategory documents the changes structure and populates live examples from Changes data.
	p := &models.Portfolio{
		EquityHoldingsValue: 265000,
		PortfolioValue:      471000,
		Changes: &models.PortfolioChanges{
			Yesterday: models.PeriodChanges{
				PortfolioValue:      models.MetricChange{Current: 471000, Previous: 470000, HasPrevious: true, RawChange: 1000, PctChange: 0.21},
				EquityHoldingsValue: models.MetricChange{Current: 265000, Previous: 264000, HasPrevious: true, RawChange: 1000, PctChange: 0.38},
			},
			Week: models.PeriodChanges{
				PortfolioValue: models.MetricChange{Current: 471000, Previous: 465000, HasPrevious: true, RawChange: 6000, PctChange: 1.29},
			},
			Month: models.PeriodChanges{},
		},
	}

	cat := buildGrowthCategory(p)

	require.Len(t, cat.Terms, 5, "Growth category should have 5 terms (changes overview + 4 metric docs)")

	// terms[1] = changes.portfolio_value — yesterday raw change = 1000
	assert.Equal(t, 1000.0, cat.Terms[1].Value,
		"changes.portfolio_value must use PortfolioValue change (1000)")
}

func TestGlossaryGrowth_NoDuplicateTerms(t *testing.T) {
	// Growth category documents the changes structure — should not contain standalone capital or income terms.
	p := &models.Portfolio{
		PortfolioValue: 100000,
		CapitalGross:   50000,
	}

	cat := buildGrowthCategory(p)

	// All term names should be "changes" or "changes.*" prefixed
	for _, term := range cat.Terms {
		assert.Contains(t, term.Term, "changes",
			"All growth terms should be changes-related, got: %s", term.Term)
	}

	assert.Len(t, cat.Terms, 5, "Growth should have 5 terms (changes overview + 4 metric docs)")
}

func TestGlossaryGrowth_ZeroYesterdayValue(t *testing.T) {
	// Edge case: no Changes data populated — terms exist but no live values
	p := &models.Portfolio{
		PortfolioValue: 100000,
	}

	cat := buildGrowthCategory(p)

	// Without Changes data, terms still have definitions but no live values
	require.Len(t, cat.Terms, 5)
	assert.Equal(t, "changes", cat.Terms[0].Term)
	assert.Nil(t, cat.Terms[0].Value, "No live value without Changes data")
}

// --- Fix 4: Glossary corrections ---

func TestGlossaryCapital_SimpleReturnFormula_UsesPortfolioValue(t *testing.T) {
	// Fix 4: simple_capital_return_pct formula should reference "portfolio_value",
	// not "equity_holdings_value".
	cp := &models.CapitalPerformance{
		ContributionsNet: 100000,
		CurrentValue:     110000,
		ReturnSimplePct:  10.0,
	}

	cat := buildCapitalCategory(cp)

	var found bool
	for _, term := range cat.Terms {
		if term.Term == "capital_return_simple_pct" {
			found = true
			assert.Contains(t, term.Formula, "portfolio_value",
				"formula should reference portfolio_value")
			assert.NotContains(t, term.Formula, "equity_holdings_value",
				"formula should NOT reference equity_value")
			break
		}
	}
	assert.True(t, found, "simple_capital_return_pct term should exist in Capital category")
}

func TestGlossaryCapital_SimpleReturnExample_UsesCurrentValue(t *testing.T) {
	// Fix 4: Example should use cp.CurrentValue (renamed from EquityValue).
	cp := &models.CapitalPerformance{
		ContributionsNet: 477000,
		CurrentValue:     471000,
		ReturnSimplePct:  -1.26,
	}

	cat := buildCapitalCategory(cp)

	for _, term := range cat.Terms {
		if term.Term == "capital_return_simple_pct" {
			// Example should contain "$471,000.00" (CurrentValue), not "$265,000.00" (EquityValue)
			assert.Contains(t, term.Example, "$471000.00",
				"example should use CurrentValue ($471000)")
			break
		}
	}
}
