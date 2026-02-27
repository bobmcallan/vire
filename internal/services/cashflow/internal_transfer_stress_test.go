package cashflow

import (
	"math"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
)

// Devils-advocate stress tests for the internal transfer filtering in
// CalculatePerformance (Fix 2) and XIRR (Fix 2).
//
// These tests verify that:
// 1. Internal transfers (transfer_out/in with external balance categories) are excluded
// 2. Non-internal transfers still count as capital flows
// 3. Edge cases around empty/unknown categories, all-internal portfolios, etc.

// --- Edge case 1: transfer_out with empty/missing/unknown category ---

func TestCalcPerf_TransferOut_EmptyCategory_NotInternal(t *testing.T) {
	// transfer_out with empty category should be treated as a REAL withdrawal,
	// not an internal transfer. Empty category means "uncategorized external transfer".
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			TotalValueHoldings: 80000,
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
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxTransferOut,
		Date:        time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		Amount:      20000,
		Description: "Transfer out - no category",
		Category:    "", // empty — NOT an internal transfer
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// Empty-category transfer_out is a REAL withdrawal, should be counted
	if perf.TotalWithdrawn != 20000 {
		t.Errorf("TotalWithdrawn = %v, want 20000 (empty category transfer_out is real withdrawal)", perf.TotalWithdrawn)
	}
	if perf.NetCapitalDeployed != 80000 {
		t.Errorf("NetCapitalDeployed = %v, want 80000", perf.NetCapitalDeployed)
	}
}

func TestCalcPerf_TransferOut_UnknownCategory_NotInternal(t *testing.T) {
	// transfer_out with unknown category (e.g. "groceries") is a real withdrawal
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			TotalValueHoldings: 80000,
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
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxTransferOut,
		Date:        time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		Amount:      15000,
		Description: "Transfer to personal account",
		Category:    "personal", // unknown category — NOT internal
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	if perf.TotalWithdrawn != 15000 {
		t.Errorf("TotalWithdrawn = %v, want 15000 (unknown category is real withdrawal)", perf.TotalWithdrawn)
	}
}

func TestCalcPerf_TransferOut_AccumulateCategory_IsInternal(t *testing.T) {
	// transfer_out with "accumulate" category IS internal — should be excluded
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			TotalValueHoldings: 80000,
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
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxTransferOut,
		Date:        time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		Amount:      20000,
		Description: "Transfer to accumulate",
		Category:    "accumulate", // internal transfer
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// Internal transfer should be EXCLUDED from deposits/withdrawals
	if perf.TotalDeposited != 100000 {
		t.Errorf("TotalDeposited = %v, want 100000", perf.TotalDeposited)
	}
	if perf.TotalWithdrawn != 0 {
		t.Errorf("TotalWithdrawn = %v, want 0 (internal transfer excluded)", perf.TotalWithdrawn)
	}
	if perf.NetCapitalDeployed != 100000 {
		t.Errorf("NetCapitalDeployed = %v, want 100000 (internal transfer excluded)", perf.NetCapitalDeployed)
	}
	// Transaction count should still include internal transfers (they exist in ledger)
	if perf.TransactionCount != 2 {
		t.Errorf("TransactionCount = %v, want 2 (all transactions counted)", perf.TransactionCount)
	}
}

// --- Edge case 2: ONLY internal transfers, no real deposits ---

func TestCalcPerf_OnlyInternalTransfers_NoDivisionByZero(t *testing.T) {
	// If the ledger contains ONLY internal transfers, after filtering there are
	// zero real capital flows. NetCapitalDeployed=0, SimpleReturnPct should be 0
	// (not NaN from division by zero).
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			TotalValueHoldings: 50000,
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	// Only internal transfers — no real deposits
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxTransferOut,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      20000,
		Description: "To accumulate",
		Category:    "accumulate",
	})
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxTransferIn,
		Date:        time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		Amount:      10000,
		Description: "From cash account",
		Category:    "cash",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// Both filtered out: deposited=0, withdrawn=0, net=0
	if perf.TotalDeposited != 0 {
		t.Errorf("TotalDeposited = %v, want 0 (all internal)", perf.TotalDeposited)
	}
	if perf.TotalWithdrawn != 0 {
		t.Errorf("TotalWithdrawn = %v, want 0 (all internal)", perf.TotalWithdrawn)
	}
	if perf.NetCapitalDeployed != 0 {
		t.Errorf("NetCapitalDeployed = %v, want 0", perf.NetCapitalDeployed)
	}
	// SimpleReturnPct must be 0, NOT NaN (division by zero)
	if perf.SimpleReturnPct != 0 {
		t.Errorf("SimpleReturnPct = %v, want 0 (zero net capital, no division by zero)", perf.SimpleReturnPct)
	}
	if math.IsNaN(perf.SimpleReturnPct) {
		t.Error("SimpleReturnPct is NaN — division by zero bug")
	}
}

// --- Edge case 3: XIRR with no cashflows after filtering ---

func TestCalcPerf_XIRR_AllInternalTransfers_Returns0(t *testing.T) {
	// After filtering internal transfers, XIRR has zero flows → should return 0
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			TotalValueHoldings: 50000,
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxTransferOut,
		Date:        time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
		Amount:      30000,
		Description: "To term deposit",
		Category:    "term_deposit",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// XIRR with no real flows should be 0, not NaN/Inf
	if math.IsNaN(perf.AnnualizedReturnPct) {
		t.Error("AnnualizedReturnPct is NaN — XIRR should handle empty flow list")
	}
	if math.IsInf(perf.AnnualizedReturnPct, 0) {
		t.Error("AnnualizedReturnPct is Inf")
	}
	if perf.AnnualizedReturnPct != 0 {
		t.Errorf("AnnualizedReturnPct = %v, want 0 (no real flows)", perf.AnnualizedReturnPct)
	}
}

// --- Edge case 4: Asymmetric transfer_in/transfer_out amounts ---

func TestCalcPerf_AsymmetricInternalTransfers(t *testing.T) {
	// transfer_out $60K to accumulate, transfer_in $10K from cash
	// Both are internal — the asymmetry should NOT affect capital performance
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			TotalValueHoldings: 100000,
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxDeposit,
		Date:        time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      150000,
		Description: "Initial deposit",
	})
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxTransferOut,
		Date:        time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC),
		Amount:      60000,
		Description: "Move to accumulate",
		Category:    "accumulate",
	})
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxTransferIn,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      10000,
		Description: "Return from cash",
		Category:    "cash",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// Only the deposit counts: deposited=150000, withdrawn=0
	if perf.TotalDeposited != 150000 {
		t.Errorf("TotalDeposited = %v, want 150000 (only real deposit)", perf.TotalDeposited)
	}
	if perf.TotalWithdrawn != 0 {
		t.Errorf("TotalWithdrawn = %v, want 0 (internal transfers excluded)", perf.TotalWithdrawn)
	}
	// Net capital = 150000
	if perf.NetCapitalDeployed != 150000 {
		t.Errorf("NetCapitalDeployed = %v, want 150000", perf.NetCapitalDeployed)
	}
	// Return: (100000 - 150000) / 150000 * 100 = -33.33%
	expectedReturn := (100000.0 - 150000.0) / 150000.0 * 100
	if math.Abs(perf.SimpleReturnPct-expectedReturn) > 0.01 {
		t.Errorf("SimpleReturnPct = %v, want ~%.2f", perf.SimpleReturnPct, expectedReturn)
	}
}

// --- Edge case 5: Mix of internal and real transfers ---

func TestCalcPerf_MixedInternalAndRealTransfers(t *testing.T) {
	// Some transfer_outs are internal (category=accumulate), some are real (no category)
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			TotalValueHoldings: 100000,
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxDeposit,
		Date:        time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      200000,
		Description: "Initial deposit",
	})
	// Internal transfer — excluded
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxTransferOut,
		Date:        time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC),
		Amount:      30000,
		Description: "To accumulate",
		Category:    "accumulate",
	})
	// Real withdrawal — included
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxWithdrawal,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      25000,
		Description: "Living expenses",
	})
	// Real transfer out (no category) — included
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxTransferOut,
		Date:        time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
		Amount:      10000,
		Description: "Transfer to spouse",
		Category:    "", // no category — real withdrawal
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	if perf.TotalDeposited != 200000 {
		t.Errorf("TotalDeposited = %v, want 200000", perf.TotalDeposited)
	}
	// Only real withdrawals: 25000 + 10000 = 35000 (not the 30000 internal)
	if perf.TotalWithdrawn != 35000 {
		t.Errorf("TotalWithdrawn = %v, want 35000 (excludes internal 30K)", perf.TotalWithdrawn)
	}
	if perf.NetCapitalDeployed != 165000 {
		t.Errorf("NetCapitalDeployed = %v, want 165000", perf.NetCapitalDeployed)
	}
}

// --- Edge case 6: Holdings-only value (not total with external balances) ---

func TestCalcPerf_UsesHoldingsOnly_NotTotalValue(t *testing.T) {
	// After Fix 2, currentValue should be TotalValueHoldings ONLY,
	// not TotalValueHoldings + ExternalBalanceTotal
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:                 "SMSF",
			TotalValueHoldings:   100000,
			ExternalBalanceTotal: 50000,  // should be IGNORED
			TotalValue:           150000, // should be IGNORED
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

	// CurrentPortfolioValue should be holdings-only: 100000
	if perf.CurrentPortfolioValue != 100000 {
		t.Errorf("CurrentPortfolioValue = %v, want 100000 (holdings-only, not %v with external balances)",
			perf.CurrentPortfolioValue, 150000.0)
	}
	// Simple return: (100000 - 100000) / 100000 * 100 = 0%
	if math.Abs(perf.SimpleReturnPct) > 0.01 {
		t.Errorf("SimpleReturnPct = %v, want ~0 (holdings = deposits)", perf.SimpleReturnPct)
	}
}

func TestCalcPerf_HoldingsOnly_ZeroHoldings_PositiveExternal(t *testing.T) {
	// Edge: No equity holdings but positive external balances.
	// After fix: currentValue = 0 (holdings-only), not 50000 (external)
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
		Description: "Deposit all to cash",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// Holdings-only means 0
	if perf.CurrentPortfolioValue != 0 {
		t.Errorf("CurrentPortfolioValue = %v, want 0 (no holdings, external balances excluded)", perf.CurrentPortfolioValue)
	}
	// Return = (0 - 50000) / 50000 * 100 = -100%
	if perf.SimpleReturnPct != -100 {
		t.Errorf("SimpleReturnPct = %v, want -100 (all money in external, not holdings)", perf.SimpleReturnPct)
	}
}

// --- Edge case 7: deriveFromTrades uses holdings-only ---

func TestDeriveFromTrades_UsesHoldingsOnly(t *testing.T) {
	// deriveFromTrades should also use TotalValueHoldings only
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:                 "SMSF",
			TotalValueHoldings:   120000,
			ExternalBalanceTotal: 50000, // should be IGNORED
			TotalValue:           170000,
			Holdings: []models.Holding{
				{
					Ticker: "BHP", Exchange: "AU", Units: 100, CurrentPrice: 50.00,
					Trades: []*models.NavexaTrade{
						{Type: "buy", Units: 100, Price: 40.00, Fees: 10.00, Date: "2023-01-10"},
					},
				},
			},
		},
	}
	storage := newMockStorageManager()
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// CurrentPortfolioValue should be 120000 (holdings-only), not 170000
	if perf.CurrentPortfolioValue != 120000 {
		t.Errorf("CurrentPortfolioValue = %v, want 120000 (holdings-only in deriveFromTrades)", perf.CurrentPortfolioValue)
	}
}

// --- Edge case 8: The SMSF scenario from the bug report ---

func TestCalcPerf_SMSFScenario_ThreeAccumulateTransfers(t *testing.T) {
	// Exact scenario from fb_65070e71:
	// - 3 transfer_out with category "accumulate" totaling $60,600
	// - These are internal reallocations, not money leaving the fund
	// - Without the fix, net_capital_deployed is understated by ~$50K
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:                 "SMSF",
			TotalValueHoldings:   426000,
			ExternalBalanceTotal: 50000,
			TotalValue:           476000,
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	// Real deposits
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxDeposit,
		Date:        time.Date(2022, 7, 1, 0, 0, 0, 0, time.UTC),
		Amount:      200000,
		Description: "Initial rollover",
	})
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxContribution,
		Date:        time.Date(2023, 7, 1, 0, 0, 0, 0, time.UTC),
		Amount:      28000,
		Description: "FY23 contribution",
	})
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxContribution,
		Date:        time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC),
		Amount:      30000,
		Description: "FY24 contribution",
	})

	// Internal transfers to accumulate — should be excluded
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxTransferOut,
		Date:        time.Date(2023, 1, 15, 0, 0, 0, 0, time.UTC),
		Amount:      20000,
		Description: "To Stake Accumulate",
		Category:    "accumulate",
	})
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxTransferOut,
		Date:        time.Date(2023, 7, 15, 0, 0, 0, 0, time.UTC),
		Amount:      20300,
		Description: "To Stake Accumulate",
		Category:    "accumulate",
	})
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxTransferOut,
		Date:        time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
		Amount:      20300,
		Description: "To Stake Accumulate",
		Category:    "accumulate",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// Real capital: 200000 + 28000 + 30000 = 258000 deposited, 0 withdrawn
	if perf.TotalDeposited != 258000 {
		t.Errorf("TotalDeposited = %v, want 258000 (real deposits only)", perf.TotalDeposited)
	}
	if perf.TotalWithdrawn != 0 {
		t.Errorf("TotalWithdrawn = %v, want 0 (accumulate transfers excluded)", perf.TotalWithdrawn)
	}
	if perf.NetCapitalDeployed != 258000 {
		t.Errorf("NetCapitalDeployed = %v, want 258000", perf.NetCapitalDeployed)
	}

	// Holdings-only value: 426000
	if perf.CurrentPortfolioValue != 426000 {
		t.Errorf("CurrentPortfolioValue = %v, want 426000 (holdings-only)", perf.CurrentPortfolioValue)
	}

	// Return: (426000 - 258000) / 258000 * 100 = 65.12%
	expectedReturn := (426000.0 - 258000.0) / 258000.0 * 100
	if math.Abs(perf.SimpleReturnPct-expectedReturn) > 0.1 {
		t.Errorf("SimpleReturnPct = %.2f, want ~%.2f", perf.SimpleReturnPct, expectedReturn)
	}

	// All 6 transactions in ledger
	if perf.TransactionCount != 6 {
		t.Errorf("TransactionCount = %d, want 6", perf.TransactionCount)
	}
}

// --- Edge case 9: FirstTransactionDate with internal transfer first ---

func TestCalcPerf_FirstTransactionIsInternal(t *testing.T) {
	// If the earliest transaction is an internal transfer, it should still
	// be used for FirstTransactionDate (it exists in the ledger).
	// But it should NOT affect deposit/withdrawal sums.
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			TotalValueHoldings: 100000,
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	// Internal transfer is the FIRST transaction
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxTransferOut,
		Date:        time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      10000,
		Description: "Early reallocation",
		Category:    "offset",
	})
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxDeposit,
		Date:        time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC),
		Amount:      100000,
		Description: "Real deposit",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// FirstTransactionDate should be the internal transfer (earliest in ledger)
	expectedFirst := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	if perf.FirstTransactionDate == nil {
		t.Fatal("FirstTransactionDate should not be nil")
	}
	if !perf.FirstTransactionDate.Equal(expectedFirst) {
		t.Errorf("FirstTransactionDate = %v, want %v", perf.FirstTransactionDate, expectedFirst)
	}

	// Only real deposit counted
	if perf.TotalDeposited != 100000 {
		t.Errorf("TotalDeposited = %v, want 100000", perf.TotalDeposited)
	}
	if perf.TotalWithdrawn != 0 {
		t.Errorf("TotalWithdrawn = %v, want 0", perf.TotalWithdrawn)
	}
}

// --- Edge case 10: All four external balance categories as internal ---

func TestCalcPerf_AllExternalBalanceCategories(t *testing.T) {
	// All four external balance types should be treated as internal
	categories := []string{"cash", "accumulate", "term_deposit", "offset"}

	for _, cat := range categories {
		t.Run(cat, func(t *testing.T) {
			storage := newMockStorageManager()
			portfolioSvc := &mockPortfolioService{
				portfolio: &models.Portfolio{
					Name:               "SMSF",
					TotalValueHoldings: 100000,
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
			_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
				Type:        models.CashTxTransferOut,
				Date:        time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
				Amount:      20000,
				Description: "Transfer to " + cat,
				Category:    cat,
			})

			perf, err := svc.CalculatePerformance(ctx, "SMSF")
			if err != nil {
				t.Fatalf("CalculatePerformance: %v", err)
			}

			if perf.TotalWithdrawn != 0 {
				t.Errorf("category=%q: TotalWithdrawn = %v, want 0 (internal)", cat, perf.TotalWithdrawn)
			}
		})
	}
}

// --- Edge case 11: transfer_in with external balance category ---

func TestCalcPerf_TransferIn_AccumulateCategory_IsInternal(t *testing.T) {
	// transfer_in from an external balance account (category=accumulate) is also internal
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			TotalValueHoldings: 100000,
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxTransferIn,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      30000,
		Description: "Return from accumulate",
		Category:    "accumulate",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// Internal transfer_in is excluded from deposits
	if perf.TotalDeposited != 0 {
		t.Errorf("TotalDeposited = %v, want 0 (internal transfer_in excluded)", perf.TotalDeposited)
	}
}

// --- Edge case 12: XIRR convergence with mix of internal and real flows ---

func TestCalcPerf_XIRR_MixedFlows_ConvergesToRealFlowsOnly(t *testing.T) {
	// XIRR should only use real flows, excluding internal transfers
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			TotalValueHoldings: 120000,
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	// Real deposit
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxDeposit,
		Date:        time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      100000,
		Description: "Initial deposit",
	})
	// Internal transfer (should be excluded from XIRR)
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxTransferOut,
		Date:        time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC),
		Amount:      50000,
		Description: "To offset",
		Category:    "offset",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// XIRR should compute based on: -100000 at 2023-01-01, +120000 at today
	// ~10% annualized (depending on exact date)
	if math.IsNaN(perf.AnnualizedReturnPct) {
		t.Error("AnnualizedReturnPct is NaN")
	}
	if math.IsInf(perf.AnnualizedReturnPct, 0) {
		t.Error("AnnualizedReturnPct is Inf")
	}
	// Should be positive (120K > 100K)
	if perf.AnnualizedReturnPct <= 0 {
		t.Errorf("AnnualizedReturnPct = %v, should be positive (portfolio grew)", perf.AnnualizedReturnPct)
	}
}
