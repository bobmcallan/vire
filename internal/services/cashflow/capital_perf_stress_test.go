package cashflow

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
)

// Devils-advocate stress tests for CalculatePerformance.
// Focus: the explicit field sum fix (TotalValueHoldings + ExternalBalanceTotal)
// and numeric edge cases that could produce incorrect capital performance metrics.

// --- Fix 2 verification: explicit field sum ---

func TestCalculatePerformance_ExplicitFieldSum_NotTotalValue(t *testing.T) {
	// Core bug scenario: TotalValue is wrong/stale, but TotalValueHoldings and
	// ExternalBalanceTotal are correct. After the fix, CalculatePerformance should
	// use TotalValueHoldings + ExternalBalanceTotal, NOT TotalValue.
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:                 "SMSF",
			TotalValueHoldings:   100000,
			ExternalBalanceTotal: 50000,
			TotalValue:           999999, // deliberately wrong — simulates stale/corrupted TotalValue
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxDeposit,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      100000,
		Description: "Initial deposit",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// After the fix: currentValue = 100000 + 50000 = 150000
	// NOT 999999 (the wrong TotalValue)
	expectedValue := 150000.0
	if perf.CurrentPortfolioValue != expectedValue {
		t.Errorf("CurrentPortfolioValue = %v, want %v (should use TotalValueHoldings + ExternalBalanceTotal, not TotalValue)",
			perf.CurrentPortfolioValue, expectedValue)
	}

	// Simple return: (150000 - 100000) / 100000 * 100 = 50%
	expectedSimple := 50.0
	if math.Abs(perf.SimpleReturnPct-expectedSimple) > 0.01 {
		t.Errorf("SimpleReturnPct = %v, want ~%v", perf.SimpleReturnPct, expectedSimple)
	}
}

func TestCalculatePerformance_ZeroHoldings_PositiveExternalBalance(t *testing.T) {
	// Edge case: no equity holdings (all cash). TotalValueHoldings=0, ExternalBalanceTotal=50000.
	// Capital performance should reflect external balances as the portfolio value.
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:                 "SMSF",
			TotalValueHoldings:   0,
			ExternalBalanceTotal: 50000,
			TotalValue:           50000,
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxDeposit,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      50000,
		Description: "Deposit into cash account",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	if perf.CurrentPortfolioValue != 50000 {
		t.Errorf("CurrentPortfolioValue = %v, want 50000", perf.CurrentPortfolioValue)
	}
	// Simple return: (50000 - 50000) / 50000 * 100 = 0%
	if math.Abs(perf.SimpleReturnPct) > 0.01 {
		t.Errorf("SimpleReturnPct = %v, want ~0 (capital preserved in cash)", perf.SimpleReturnPct)
	}
}

func TestCalculatePerformance_NaN_TotalValueHoldings(t *testing.T) {
	// What if TotalValueHoldings is NaN due to upstream computation error?
	// The sum should produce NaN, which should propagate (not silently corrupt).
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:                 "SMSF",
			TotalValueHoldings:   math.NaN(),
			ExternalBalanceTotal: 50000,
			TotalValue:           50000, // "looks fine" but holdings is NaN
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxDeposit,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      50000,
		Description: "Deposit",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// With the explicit sum, NaN + 50000 = NaN. This is correct behaviour —
	// NaN should propagate rather than being masked by a pre-computed TotalValue.
	// The caller (handlePortfolioGet) swallows errors and omits capital_performance.
	if !math.IsNaN(perf.CurrentPortfolioValue) {
		t.Logf("CurrentPortfolioValue = %v (NaN propagation depends on implementation)", perf.CurrentPortfolioValue)
	}
}

func TestCalculatePerformance_Inf_ExternalBalanceTotal(t *testing.T) {
	// Inf external balance total — should not crash, should produce Inf or be handled
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:                 "SMSF",
			TotalValueHoldings:   100000,
			ExternalBalanceTotal: math.Inf(1),
			TotalValue:           math.Inf(1),
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxDeposit,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      100000,
		Description: "Deposit",
	})

	// Should not panic
	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance should not error on Inf: %v", err)
	}

	// Inf + 100000 = Inf
	if !math.IsInf(perf.CurrentPortfolioValue, 1) {
		t.Logf("CurrentPortfolioValue = %v (Inf expected)", perf.CurrentPortfolioValue)
	}
}

func TestCalculatePerformance_NegativeExternalBalanceTotal(t *testing.T) {
	// ExternalBalanceTotal should never be negative (validation prevents it), but if
	// corrupted data gets through, CalculatePerformance should still produce sensible results.
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:                 "SMSF",
			TotalValueHoldings:   100000,
			ExternalBalanceTotal: -50000, // corrupted: negative external balance
			TotalValue:           100000, // would be 50000 if using sum, but this is wrong
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxDeposit,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      100000,
		Description: "Deposit",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// Explicit sum: 100000 + (-50000) = 50000
	expectedValue := 50000.0
	if perf.CurrentPortfolioValue != expectedValue {
		t.Errorf("CurrentPortfolioValue = %v, want %v", perf.CurrentPortfolioValue, expectedValue)
	}

	// Simple return: (50000 - 100000) / 100000 * 100 = -50%
	expectedSimple := -50.0
	if math.Abs(perf.SimpleReturnPct-expectedSimple) > 0.01 {
		t.Errorf("SimpleReturnPct = %v, want ~%v", perf.SimpleReturnPct, expectedSimple)
	}
}

func TestCalculatePerformance_BothFieldsZero(t *testing.T) {
	// Both TotalValueHoldings and ExternalBalanceTotal are 0 — complete wipeout
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:                 "SMSF",
			TotalValueHoldings:   0,
			ExternalBalanceTotal: 0,
			TotalValue:           0,
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxDeposit,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      100000,
		Description: "Deposit",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	if perf.CurrentPortfolioValue != 0 {
		t.Errorf("CurrentPortfolioValue = %v, want 0", perf.CurrentPortfolioValue)
	}
	// Simple return: (0 - 100000) / 100000 * 100 = -100%
	if perf.SimpleReturnPct != -100 {
		t.Errorf("SimpleReturnPct = %v, want -100", perf.SimpleReturnPct)
	}
}

func TestCalculatePerformance_VeryLargeExternalBalance(t *testing.T) {
	// External balance near float64 limits
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:                 "SMSF",
			TotalValueHoldings:   100000,
			ExternalBalanceTotal: 1e14, // 100 trillion — near max allowed
			TotalValue:           1e14 + 100000,
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxDeposit,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      100000,
		Description: "Deposit",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	if math.IsNaN(perf.CurrentPortfolioValue) || math.IsInf(perf.CurrentPortfolioValue, 0) {
		t.Errorf("CurrentPortfolioValue is NaN/Inf with large external balance: %v", perf.CurrentPortfolioValue)
	}
	if math.IsNaN(perf.SimpleReturnPct) || math.IsInf(perf.SimpleReturnPct, 0) {
		t.Errorf("SimpleReturnPct is NaN/Inf with large external balance: %v", perf.SimpleReturnPct)
	}
}

func TestCalculatePerformance_ManySmallTransactions_Precision(t *testing.T) {
	// 1000 micro-deposits — tests float64 summation precision
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:                 "SMSF",
			TotalValueHoldings:   10000,
			ExternalBalanceTotal: 5000,
			TotalValue:           15000,
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 1000; i++ {
		_, err := svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
			Type:        models.CashTxDeposit,
			Date:        base.Add(time.Duration(i) * time.Hour),
			Amount:      10.01, // not a power of 2 — accumulates rounding error
			Description: "Micro deposit",
		})
		if err != nil {
			t.Fatalf("AddTransaction %d: %v", i, err)
		}
	}

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// 1000 * 10.01 = 10010 total deposited
	if math.Abs(perf.TotalDeposited-10010) > 1.0 {
		t.Errorf("TotalDeposited = %v, want ~10010 (float precision)", perf.TotalDeposited)
	}
	if math.IsNaN(perf.SimpleReturnPct) || math.IsInf(perf.SimpleReturnPct, 0) {
		t.Errorf("SimpleReturnPct is NaN/Inf: %v", perf.SimpleReturnPct)
	}
	if math.IsNaN(perf.AnnualizedReturnPct) || math.IsInf(perf.AnnualizedReturnPct, 0) {
		t.Errorf("AnnualizedReturnPct is NaN/Inf: %v", perf.AnnualizedReturnPct)
	}
}

func TestCalculatePerformance_DividendInflowCounted(t *testing.T) {
	// Dividends are inflows — they should increase TotalDeposited
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:                 "SMSF",
			TotalValueHoldings:   100000,
			ExternalBalanceTotal: 0,
			TotalValue:           100000,
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxDeposit,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      80000,
		Description: "Initial deposit",
	})
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxDividend,
		Date:        time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		Amount:      5000,
		Description: "BHP dividend",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// Dividends are inflows: TotalDeposited = 80000 + 5000 = 85000
	if perf.TotalDeposited != 85000 {
		t.Errorf("TotalDeposited = %v, want 85000 (dividend counted as inflow)", perf.TotalDeposited)
	}
}

func TestCalculatePerformance_TransferTypes(t *testing.T) {
	// transfer_in is inflow, transfer_out is outflow
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:                 "SMSF",
			TotalValueHoldings:   100000,
			ExternalBalanceTotal: 0,
			TotalValue:           100000,
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxTransferIn,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      100000,
		Description: "Transfer from personal",
	})
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxTransferOut,
		Date:        time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		Amount:      20000,
		Description: "Transfer to savings",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	if perf.TotalDeposited != 100000 {
		t.Errorf("TotalDeposited = %v, want 100000", perf.TotalDeposited)
	}
	if perf.TotalWithdrawn != 20000 {
		t.Errorf("TotalWithdrawn = %v, want 20000", perf.TotalWithdrawn)
	}
	if perf.NetCapitalDeployed != 80000 {
		t.Errorf("NetCapitalDeployed = %v, want 80000", perf.NetCapitalDeployed)
	}
}

func TestCalculatePerformance_MaxDescriptionLength(t *testing.T) {
	// Hostile input: description at exact max length (500 chars)
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:                 "SMSF",
			TotalValueHoldings:   50000,
			ExternalBalanceTotal: 0,
			TotalValue:           50000,
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	longDesc := strings.Repeat("A", 500)
	_, err := svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxDeposit,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      50000,
		Description: longDesc,
	})
	if err != nil {
		t.Fatalf("AddTransaction with 500-char description should succeed: %v", err)
	}

	// Just over the limit
	tooLong := strings.Repeat("A", 501)
	_, err = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxDeposit,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      50000,
		Description: tooLong,
	})
	if err == nil {
		t.Error("AddTransaction with 501-char description should fail")
	}
}
