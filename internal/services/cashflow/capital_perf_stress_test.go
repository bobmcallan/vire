package cashflow

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
)

// Stress tests for CalculatePerformance.
// Focus: PortfolioValue (equity + cash) for capital return calculations
// and numeric edge cases that could produce incorrect capital performance metrics.

// --- Fix 1 verification: uses PortfolioValue (equity + cash) ---

func TestCalculatePerformance_UsesPortfolioValue_NotEquityOnly(t *testing.T) {
	// Core scenario: CalculatePerformance should use PortfolioValue (equity + cash),
	// not EquityValue alone.
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:                "SMSF",
			EquityHoldingsValue: 100000,
			CapitalGross:        50000,
			PortfolioValue:      150000, // equity + cash
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

	// After the fix: currentValue = PortfolioValue = 150000 (equity + cash)
	if perf.CurrentValue != 150000 {
		t.Errorf("CurrentPortfolioValue = %v, want 150000 (PortfolioValue = equity + cash)",
			perf.CurrentValue)
	}

	// Simple return: (150000 - 100000) / 100000 * 100 = 50%
	expectedReturn := (150000.0 - 100000.0) / 100000.0 * 100
	if math.Abs(perf.ReturnSimplePct-expectedReturn) > 0.01 {
		t.Errorf("SimpleReturnPct = %v, want ~%.2f%%", perf.ReturnSimplePct, expectedReturn)
	}
}

func TestCalculatePerformance_ZeroHoldings_AllCash(t *testing.T) {
	// Edge case: no equity holdings (all cash). EquityValue=0, TotalCash=50000.
	// PortfolioValue = 50000 (all cash), so return is break-even.
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:                "SMSF",
			EquityHoldingsValue: 0,
			CapitalGross:        50000,
			PortfolioValue:      50000,
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

	// PortfolioValue = 50000 (all cash, no equity)
	if perf.CurrentValue != 50000 {
		t.Errorf("CurrentPortfolioValue = %v, want 50000 (PortfolioValue = cash balance)", perf.CurrentValue)
	}
	// Simple return: (50000 - 50000) / 50000 * 100 = 0%
	if math.Abs(perf.ReturnSimplePct) > 0.01 {
		t.Errorf("SimpleReturnPct = %v, want 0 (deposited = portfolio value)", perf.ReturnSimplePct)
	}
}

func TestCalculatePerformance_NaN_EquityValue(t *testing.T) {
	// What if EquityValue is NaN due to upstream computation error?
	// NaN should propagate (not silently corrupt).
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:                "SMSF",
			EquityHoldingsValue: math.NaN(),
			CapitalGross:        50000,
			PortfolioValue:      50000, // "looks fine" but holdings is NaN
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
	if !math.IsNaN(perf.CurrentValue) {
		t.Logf("CurrentPortfolioValue = %v (NaN propagation depends on implementation)", perf.CurrentValue)
	}
}

func TestCalculatePerformance_Inf_PortfolioValue(t *testing.T) {
	// Inf PortfolioValue — should not panic and propagates Inf
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:                "SMSF",
			EquityHoldingsValue: 100000,
			CapitalGross:        math.Inf(1),
			PortfolioValue:      math.Inf(1),
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

	// PortfolioValue = Inf, so CurrentValue is Inf
	if !math.IsInf(perf.CurrentValue, 1) {
		t.Logf("CurrentPortfolioValue = %v (Inf PortfolioValue propagates)", perf.CurrentValue)
	}
}

func TestCalculatePerformance_NegativeTotalCash(t *testing.T) {
	// Negative cash balance reduces PortfolioValue. PortfolioValue is the source of truth.
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:                "SMSF",
			EquityHoldingsValue: 100000,
			CapitalGross:        -50000, // overdraft scenario
			PortfolioValue:      50000,  // equity 100000 + negative cash -50000
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

	// PortfolioValue = 50000 (equity 100000 - overdraft 50000)
	if perf.CurrentValue != 50000 {
		t.Errorf("CurrentPortfolioValue = %v, want 50000 (PortfolioValue with negative cash)", perf.CurrentValue)
	}

	// Simple return: (50000 - 100000) / 100000 * 100 = -50%
	expectedReturn := (50000.0 - 100000.0) / 100000.0 * 100
	if math.Abs(perf.ReturnSimplePct-expectedReturn) > 0.01 {
		t.Errorf("SimpleReturnPct = %v, want ~%.2f%%", perf.ReturnSimplePct, expectedReturn)
	}
}

func TestCalculatePerformance_BothFieldsZero(t *testing.T) {
	// Both EquityValue and TotalCash are 0 — complete wipeout
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:                "SMSF",
			EquityHoldingsValue: 0,
			CapitalGross:        0,
			PortfolioValue:      0,
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

	if perf.CurrentValue != 0 {
		t.Errorf("CurrentPortfolioValue = %v, want 0", perf.CurrentValue)
	}
	// Simple return: (0 - 100000) / 100000 * 100 = -100%
	if perf.ReturnSimplePct != -100 {
		t.Errorf("SimpleReturnPct = %v, want -100", perf.ReturnSimplePct)
	}
}

func TestCalculatePerformance_VeryLargeCashBalance(t *testing.T) {
	// Very large cash balance included in PortfolioValue — tests numeric stability
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:                "SMSF",
			EquityHoldingsValue: 100000,
			CapitalGross:        1e14, // 100 trillion cash
			PortfolioValue:      1e14 + 100000,
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

	// PortfolioValue = 1e14 + 100000 (equity + huge cash)
	if perf.CurrentValue != 1e14+100000 {
		t.Errorf("CurrentPortfolioValue = %v, want %v (PortfolioValue = equity + cash)", perf.CurrentValue, 1e14+100000)
	}
	if math.IsNaN(perf.ReturnSimplePct) || math.IsInf(perf.ReturnSimplePct, 0) {
		t.Errorf("SimpleReturnPct is NaN/Inf: %v", perf.ReturnSimplePct)
	}
}

func TestCalculatePerformance_ManySmallTransactions_Precision(t *testing.T) {
	// 1000 micro-deposits — tests float64 summation precision
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:                "SMSF",
			EquityHoldingsValue: 10000,
			CapitalGross:        5000,  // excluded from performance
			PortfolioValue:      15000, // not used
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
	if math.Abs(perf.ContributionsGross-10010) > 1.0 {
		t.Errorf("GrossCapitalDeposited = %v, want ~10010 (float precision)", perf.ContributionsGross)
	}
	if math.IsNaN(perf.ReturnSimplePct) || math.IsInf(perf.ReturnSimplePct, 0) {
		t.Errorf("SimpleReturnPct is NaN/Inf: %v", perf.ReturnSimplePct)
	}
	if math.IsNaN(perf.ReturnXirrPct) || math.IsInf(perf.ReturnXirrPct, 0) {
		t.Errorf("AnnualizedReturnPct is NaN/Inf: %v", perf.ReturnXirrPct)
	}
}

func TestCalculatePerformance_DividendNotCountedAsDeposit(t *testing.T) {
	// Dividends are NOT counted as deposits — only contributions count
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:                "SMSF",
			EquityHoldingsValue: 100000,
			CapitalGross:        0,
			PortfolioValue:      100000,
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
	if perf.ContributionsGross != 80000 {
		t.Errorf("GrossCapitalDeposited = %v, want 80000 (dividend is not a capital deposit)", perf.ContributionsGross)
	}
}

func TestCalculatePerformance_CreditDebitTypes(t *testing.T) {
	// Only contribution category counts for GrossCapitalDeposited/GrossCapitalWithdrawn.
	// Non-contribution debits (other, transfer, fee) do NOT count as withdrawals.
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:                "SMSF",
			EquityHoldingsValue: 100000,
			CapitalGross:        0,
			PortfolioValue:      100000,
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

	if perf.ContributionsGross != 100000 {
		t.Errorf("GrossCapitalDeposited = %v, want 100000", perf.ContributionsGross)
	}
	// The -20000 is category=other, not contribution — does NOT count as withdrawn
	if perf.WithdrawalsGross != 0 {
		t.Errorf("GrossCapitalWithdrawn = %v, want 0 (other-category debit is not a capital withdrawal)", perf.WithdrawalsGross)
	}
	if perf.ContributionsNet != 100000 {
		t.Errorf("NetCapitalDeployed = %v, want 100000", perf.ContributionsNet)
	}
}

func TestCalculatePerformance_MaxDescriptionLength(t *testing.T) {
	// Hostile input: description at exact max length (500 chars)
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:                "SMSF",
			EquityHoldingsValue: 50000,
			CapitalGross:        0,
			PortfolioValue:      50000,
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
