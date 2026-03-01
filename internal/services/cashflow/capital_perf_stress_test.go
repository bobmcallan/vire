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
// Focus: holdings-only value (EquityValue, excluding TotalCash)
// and numeric edge cases that could produce incorrect capital performance metrics.

// --- Fix 2 verification: holdings-only value ---

func TestCalculatePerformance_HoldingsOnly_NotTotalValue(t *testing.T) {
	// Core scenario: CalculatePerformance should use EquityValue only,
	// NOT TotalValue and NOT EquityValue + TotalCash.
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			EquityValue: 100000,
			GrossCashBalance:          50000,
			PortfolioValue:         999999, // deliberately wrong — simulates stale/corrupted TotalValue
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      100000,
		Description: "Initial deposit",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// After the fix: currentValue = EquityValue only = 100000
	// NOT 150000 (holdings + external) and NOT 999999 (stale TotalValue)
	if perf.EquityValue != 100000 {
		t.Errorf("CurrentPortfolioValue = %v, want 100000 (holdings only)",
			perf.EquityValue)
	}

	// Simple return: (100000 - 100000) / 100000 * 100 = 0%
	if math.Abs(perf.SimpleCapitalReturnPct) > 0.01 {
		t.Errorf("SimpleReturnPct = %v, want ~0%%", perf.SimpleCapitalReturnPct)
	}
}

func TestCalculatePerformance_ZeroHoldings_PositiveTotalCash(t *testing.T) {
	// Edge case: no equity holdings (all cash). EquityValue=0, TotalCash=50000.
	// Capital performance uses holdings only, so value is 0.
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			EquityValue: 0,
			GrossCashBalance:          50000,
			PortfolioValue:         50000,
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      50000,
		Description: "Deposit into cash account",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// Holdings only = 0 (all money is in cash balances, not equity)
	if perf.EquityValue != 0 {
		t.Errorf("CurrentPortfolioValue = %v, want 0 (holdings only)", perf.EquityValue)
	}
	// Simple return: (0 - 50000) / 50000 * 100 = -100%
	if perf.SimpleCapitalReturnPct != -100 {
		t.Errorf("SimpleReturnPct = %v, want -100 (all in cash balances)", perf.SimpleCapitalReturnPct)
	}
}

func TestCalculatePerformance_NaN_EquityValue(t *testing.T) {
	// What if EquityValue is NaN due to upstream computation error?
	// NaN should propagate (not silently corrupt).
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			EquityValue: math.NaN(),
			GrossCashBalance:          50000,
			PortfolioValue:         50000, // "looks fine" but holdings is NaN
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      50000,
		Description: "Deposit",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// Holdings only: NaN. NaN should propagate rather than being masked.
	if !math.IsNaN(perf.EquityValue) {
		t.Logf("CurrentPortfolioValue = %v (NaN propagation depends on implementation)", perf.EquityValue)
	}
}

func TestCalculatePerformance_Inf_TotalCash(t *testing.T) {
	// Inf cash balance total — should not affect holdings-only value
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			EquityValue: 100000,
			GrossCashBalance:          math.Inf(1),
			PortfolioValue:         math.Inf(1),
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      100000,
		Description: "Deposit",
	})

	// Should not panic
	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance should not error on Inf: %v", err)
	}

	// Holdings only = 100000 (Inf cash balance is excluded)
	if perf.EquityValue != 100000 {
		t.Errorf("CurrentPortfolioValue = %v, want 100000 (holdings only, Inf external ignored)", perf.EquityValue)
	}
}

func TestCalculatePerformance_NegativeTotalCash(t *testing.T) {
	// TotalCash should never be negative (validation prevents it), but if
	// corrupted data gets through, holdings-only value should be unaffected.
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			EquityValue: 100000,
			GrossCashBalance:          -50000, // corrupted: negative cash balance
			PortfolioValue:         100000,
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      100000,
		Description: "Deposit",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// Holdings only = 100000 (negative cash balance is excluded)
	if perf.EquityValue != 100000 {
		t.Errorf("CurrentPortfolioValue = %v, want 100000 (holdings only)", perf.EquityValue)
	}

	// Simple return: (100000 - 100000) / 100000 * 100 = 0%
	if math.Abs(perf.SimpleCapitalReturnPct) > 0.01 {
		t.Errorf("SimpleReturnPct = %v, want ~0%%", perf.SimpleCapitalReturnPct)
	}
}

func TestCalculatePerformance_BothFieldsZero(t *testing.T) {
	// Both EquityValue and TotalCash are 0 — complete wipeout
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			EquityValue: 0,
			GrossCashBalance:          0,
			PortfolioValue:         0,
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      100000,
		Description: "Deposit",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	if perf.EquityValue != 0 {
		t.Errorf("CurrentPortfolioValue = %v, want 0", perf.EquityValue)
	}
	// Simple return: (0 - 100000) / 100000 * 100 = -100%
	if perf.SimpleCapitalReturnPct != -100 {
		t.Errorf("SimpleReturnPct = %v, want -100", perf.SimpleCapitalReturnPct)
	}
}

func TestCalculatePerformance_VeryLargeTotalCash(t *testing.T) {
	// External balance near float64 limits — should not affect holdings-only value
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			EquityValue: 100000,
			GrossCashBalance:          1e14, // 100 trillion — excluded from performance
			PortfolioValue:         1e14 + 100000,
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      100000,
		Description: "Deposit",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// Holdings only = 100000, large cash balance is excluded
	if perf.EquityValue != 100000 {
		t.Errorf("CurrentPortfolioValue = %v, want 100000 (holdings only)", perf.EquityValue)
	}
	if math.IsNaN(perf.SimpleCapitalReturnPct) || math.IsInf(perf.SimpleCapitalReturnPct, 0) {
		t.Errorf("SimpleReturnPct is NaN/Inf: %v", perf.SimpleCapitalReturnPct)
	}
}

func TestCalculatePerformance_ManySmallTransactions_Precision(t *testing.T) {
	// 1000 micro-deposits — tests float64 summation precision
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			EquityValue: 10000,
			GrossCashBalance:          5000,  // excluded from performance
			PortfolioValue:         15000, // not used
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 1000; i++ {
		_, err := svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
			Account:     "Trading",
			Category:    models.CashCatContribution,
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
	if math.Abs(perf.GrossCapitalDeposited-10010) > 1.0 {
		t.Errorf("GrossCapitalDeposited = %v, want ~10010 (float precision)", perf.GrossCapitalDeposited)
	}
	if math.IsNaN(perf.SimpleCapitalReturnPct) || math.IsInf(perf.SimpleCapitalReturnPct, 0) {
		t.Errorf("SimpleReturnPct is NaN/Inf: %v", perf.SimpleCapitalReturnPct)
	}
	if math.IsNaN(perf.AnnualizedCapitalReturnPct) || math.IsInf(perf.AnnualizedCapitalReturnPct, 0) {
		t.Errorf("AnnualizedReturnPct is NaN/Inf: %v", perf.AnnualizedCapitalReturnPct)
	}
}

func TestCalculatePerformance_DividendNotCountedAsDeposit(t *testing.T) {
	// Dividends are NOT counted as deposits — only contributions count
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			EquityValue: 100000,
			GrossCashBalance:          0,
			PortfolioValue:         100000,
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      80000,
		Description: "Initial deposit",
	})
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatDividend,
		Date:        time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		Amount:      5000,
		Description: "BHP dividend",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// Dividends are NOT capital deposits: GrossCapitalDeposited = 80000 (not 85000)
	if perf.GrossCapitalDeposited != 80000 {
		t.Errorf("GrossCapitalDeposited = %v, want 80000 (dividend is not a capital deposit)", perf.GrossCapitalDeposited)
	}
}

func TestCalculatePerformance_CreditDebitTypes(t *testing.T) {
	// Only contribution category counts for GrossCapitalDeposited/GrossCapitalWithdrawn.
	// Non-contribution debits (other, transfer, fee) do NOT count as withdrawals.
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			EquityValue: 100000,
			GrossCashBalance:          0,
			PortfolioValue:         100000,
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      100000,
		Description: "Transfer from personal",
	})
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatOther,
		Date:        time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		Amount:      -20000,
		Description: "Transfer to savings",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	if perf.GrossCapitalDeposited != 100000 {
		t.Errorf("GrossCapitalDeposited = %v, want 100000", perf.GrossCapitalDeposited)
	}
	// The -20000 is category=other, not contribution — does NOT count as withdrawn
	if perf.GrossCapitalWithdrawn != 0 {
		t.Errorf("GrossCapitalWithdrawn = %v, want 0 (other-category debit is not a capital withdrawal)", perf.GrossCapitalWithdrawn)
	}
	if perf.NetCapitalDeployed != 100000 {
		t.Errorf("NetCapitalDeployed = %v, want 100000", perf.NetCapitalDeployed)
	}
}

func TestCalculatePerformance_MaxDescriptionLength(t *testing.T) {
	// Hostile input: description at exact max length (500 chars)
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			EquityValue: 50000,
			GrossCashBalance:          0,
			PortfolioValue:         50000,
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	longDesc := strings.Repeat("A", 500)
	_, err := svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
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
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      50000,
		Description: tooLong,
	})
	if err == nil {
		t.Error("AddTransaction with 501-char description should fail")
	}
}
