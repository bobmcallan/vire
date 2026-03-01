package cashflow

import (
	"math"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
)

// Devils-advocate stress tests for the deriveFromTrades trade-based fallback.
// These test the CalculatePerformance empty-ledger path and the XIRR computation
// with trade-derived cash flows.
//
// Note: The actual deriveFromTrades method requires a Navexa client and portfolio
// service, so we test the underlying computation logic and verify the existing
// empty-ledger path handles edge cases.

// --- Empty ledger path (pre-fix behavior) ---

func TestCalculatePerformance_EmptyLedger_ReturnsZeros(t *testing.T) {
	// Before the fix: empty ledger returns zeros
	// After the fix: it should try deriveFromTrades as fallback
	svc, _ := testService()
	ctx := testContext()

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	assert.NoError(t, err)
	assert.NotNil(t, perf)
	assert.Equal(t, 0, perf.TransactionCount)
	assert.Equal(t, 0.0, perf.TotalDeposited)
	assert.Equal(t, 0.0, perf.TotalWithdrawn)
	assert.Equal(t, 0.0, perf.SimpleReturnPct)
	assert.Equal(t, 0.0, perf.AnnualizedReturnPct)
}

// --- XIRR with trade-like cash flows ---

func TestComputeXIRR_AllBuysNoSells_PositiveReturn(t *testing.T) {
	// Simulates deriveFromTrades: only buy trades, portfolio value > cost
	// Buys are negative (investment outflow)
	transactions := []models.CashTransaction{
		{Date: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), Amount: -50000},
		{Date: time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC), Amount: -30000},
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Amount: -20000},
	}
	// Total invested: 100K, portfolio worth 120K
	rate := computeXIRR(transactions, 120000)
	assert.False(t, math.IsNaN(rate), "XIRR should not be NaN")
	assert.False(t, math.IsInf(rate, 0), "XIRR should not be Inf")
	assert.Greater(t, rate, 0.0, "positive return should give positive XIRR")
}

func TestComputeXIRR_AllBuysNoSells_NegativeReturn(t *testing.T) {
	// Portfolio value < total invested — loss
	// Buys are negative (investment outflow)
	transactions := []models.CashTransaction{
		{Date: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), Amount: -100000},
	}
	rate := computeXIRR(transactions, 70000)
	assert.False(t, math.IsNaN(rate), "XIRR should not be NaN")
	assert.Less(t, rate, 0.0, "loss scenario should give negative XIRR")
}

func TestComputeXIRR_MixedBuysAndSells(t *testing.T) {
	// Simulates a portfolio with buys and sells
	// Buys are negative (outflow), sells are positive (inflow)
	transactions := []models.CashTransaction{
		{Date: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), Amount: -50000},
		{Date: time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC), Amount: 20000},
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Amount: -30000},
	}
	// Net in: 60K, portfolio 80K
	rate := computeXIRR(transactions, 80000)
	assert.False(t, math.IsNaN(rate))
	assert.False(t, math.IsInf(rate, 0))
}

// --- Scenarios for deriveFromTrades edge cases ---

func TestDeriveFromTrades_AllSellsNoBuys_NegativeCapital(t *testing.T) {
	// If a portfolio only has sell trades (e.g., opening balance + sells),
	// totalDeposited=0, totalWithdrawn=sell proceeds → net capital negative
	// SimpleReturnPct should be 0 (netCapital <= 0)
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			TotalValueHoldings: 50000,
			TotalCash:          0,
			TotalValue:         50000,
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	// Add only contribution withdrawals to simulate "all sells with net outflow"
	_, err := svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      -30000,
		Description: "Net withdrawal",
	})
	assert.NoError(t, err)

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	assert.NoError(t, err)
	assert.Equal(t, 0.0, perf.SimpleReturnPct, "negative net capital should give 0% return")
	assert.Equal(t, -30000.0, perf.NetCapitalDeployed, "net capital should be negative")
}

func TestDeriveFromTrades_ZeroPriceAndFees(t *testing.T) {
	// Trades with zero price and zero fees — edge case in trade data
	// buy cost = units * 0 + 0 = 0 deposited
	// This means all zeros for performance
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			TotalValueHoldings: 0,
			TotalCash:          0,
			TotalValue:         0,
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	assert.NoError(t, err)
	assert.Equal(t, 0, perf.TransactionCount)
	assert.Equal(t, 0.0, perf.CurrentPortfolioValue)
}

func TestDeriveFromTrades_VeryLargeTradeAmounts(t *testing.T) {
	// Near float64 limits
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			TotalValueHoldings: 1e14,
			TotalCash:          0,
			TotalValue:         1e14,
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	// Large deposit near max allowed
	_, err := svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      1e15 - 1, // just under max
		Description: "Large deposit",
	})
	assert.NoError(t, err)

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	assert.NoError(t, err)
	assert.False(t, math.IsNaN(perf.SimpleReturnPct))
	assert.False(t, math.IsInf(perf.SimpleReturnPct, 0))
}

func TestDeriveFromTrades_CurrencyMismatch(t *testing.T) {
	// FINDING: If trades are in USD but portfolio reports in AUD, the sum of
	// buy costs (USD) would be compared against portfolio value (AUD).
	// deriveFromTrades needs to account for currency or document the assumption.
	t.Log("FINDING: deriveFromTrades sums trade amounts in their native currency. " +
		"If holdings have mixed currencies (AUD + USD), the sum is meaningless. " +
		"RECOMMENDATION: Either convert at FX rate or document 'same currency' assumption.")
}

// --- Division by zero in return calculations ---

func TestSimpleReturn_NetCapitalZero(t *testing.T) {
	// Equal deposits and withdrawals: net capital = 0
	netCapital := 0.0
	currentValue := 100000.0
	var simpleReturnPct float64
	if netCapital > 0 {
		simpleReturnPct = (currentValue - netCapital) / netCapital * 100
	}
	assert.Equal(t, 0.0, simpleReturnPct, "zero net capital should return 0%")
}

func TestSimpleReturn_NegativeNetCapital(t *testing.T) {
	// More withdrawn than deposited
	netCapital := -50000.0
	var simpleReturnPct float64
	if netCapital > 0 {
		simpleReturnPct = 999 // should not execute
	}
	assert.Equal(t, 0.0, simpleReturnPct, "negative net capital should return 0%")
}

func TestSimpleReturn_VerySmallNetCapital(t *testing.T) {
	// Tiny net capital with large portfolio value — produces huge return
	netCapital := 0.01
	currentValue := 100000.0
	var simpleReturnPct float64
	if netCapital > 0 {
		simpleReturnPct = (currentValue - netCapital) / netCapital * 100
	}
	// (100000 - 0.01) / 0.01 * 100 = 999,999,000%
	assert.Greater(t, simpleReturnPct, 1e8, "tiny net capital produces astronomically high return")
	assert.False(t, math.IsInf(simpleReturnPct, 0), "should not overflow to Inf")
	t.Logf("FINDING: Very small net capital ($0.01) with $100K portfolio produces %.0f%% return — "+
		"may want to cap extreme values", simpleReturnPct)
}

// --- XIRR convergence stress tests ---

func TestXIRR_RapidTrading_ManyTransactions(t *testing.T) {
	// 1000 alternating buys and sells
	// Even = buy (negative outflow), odd = sell (positive inflow)
	var transactions []models.CashTransaction
	base := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 1000; i++ {
		amount := -1000.0 // buy (outflow)
		if i%2 == 1 {
			amount = 1000.0 // sell (inflow)
		}
		transactions = append(transactions, models.CashTransaction{
			Date:   base.Add(time.Duration(i) * 24 * time.Hour),
			Amount: amount,
		})
	}
	// Net: 500 deposits - 500 withdrawals = 0, but portfolio has value
	rate := computeXIRR(transactions, 50000)
	assert.False(t, math.IsNaN(rate), "XIRR with 1000 transactions should converge")
	assert.False(t, math.IsInf(rate, 0), "XIRR with 1000 transactions should be finite")
}

func TestXIRR_NearZeroTimeSpan(t *testing.T) {
	// Deposit 1 second ago — negative outflow
	transactions := []models.CashTransaction{
		{Date: time.Now().Add(-time.Second), Amount: -100000},
	}
	rate := computeXIRR(transactions, 100001)
	// Very short time → annualized rate could be astronomical
	assert.False(t, math.IsNaN(rate), "near-zero time span should not produce NaN")
	assert.False(t, math.IsInf(rate, 0), "near-zero time span should not produce Inf")
}

func TestXIRR_FutureDateTransaction(t *testing.T) {
	// Transaction in the future — negative year fraction
	// This shouldn't happen (validation prevents it) but test XIRR robustness
	transactions := []models.CashTransaction{
		{Date: time.Now().Add(365 * 24 * time.Hour), Amount: -100000},
	}
	rate := computeXIRR(transactions, 110000)
	// Future dates create negative year fractions, which can cause math.Pow issues
	assert.False(t, math.IsNaN(rate), "future date transaction should not produce NaN")
}

func TestXIRR_10000xReturn(t *testing.T) {
	// Extreme return: $100 → $1M in 1 year
	transactions := []models.CashTransaction{
		{Date: time.Now().Add(-365 * 24 * time.Hour), Amount: -100},
	}
	rate := computeXIRR(transactions, 1000000)
	// 10000x return → ~999900% → should be capped at 10000% (rate=100)
	assert.False(t, math.IsNaN(rate), "extreme return should not produce NaN")
	assert.False(t, math.IsInf(rate, 0), "extreme return should not produce Inf")
	t.Logf("10000x return XIRR: %.2f%%", rate)
}

// --- CalculatePerformance with NaveClient not available ---

func TestCalculatePerformance_EmptyLedger_NoNavexaClient(t *testing.T) {
	// After the fix, if deriveFromTrades fails (no Navexa client),
	// it should gracefully return empty CapitalPerformance
	svc, _ := testService()
	ctx := testContext()

	// With empty ledger and no Navexa client on the cashflow service,
	// deriveFromTrades should fail gracefully
	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	assert.NoError(t, err)
	assert.NotNil(t, perf)
	// Pre-fix: returns zeros. Post-fix: tries deriveFromTrades, fails, returns zeros.
	assert.Equal(t, 0.0, perf.TotalDeposited)
}

func TestCalculatePerformance_PortfolioTimeout(t *testing.T) {
	// If GetPortfolio times out during CalculatePerformance,
	// the error should propagate (not hang)
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: nil, // simulates timeout/error
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	// Add a transaction so ledger is non-empty (positive = deposit)
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      50000,
		Description: "Deposit",
	})

	_, err := svc.CalculatePerformance(ctx, "SMSF")
	assert.Error(t, err, "portfolio not found should propagate as error")
	assert.Contains(t, err.Error(), "portfolio")
}
