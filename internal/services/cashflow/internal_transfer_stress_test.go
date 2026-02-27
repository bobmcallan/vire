package cashflow

import (
	"math"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
)

// Devils-advocate stress tests for internal transfer exclusion in
// CalculatePerformance and XIRR.
//
// These tests verify that:
// 1. Internal transfers (transfer_out/in with external balance categories) are excluded from withdrawal totals
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
	// transfer_out with "accumulate" category IS internal — excluded from withdrawals
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

	// Internal transfer_out excluded from withdrawals (capital reallocation)
	if perf.TotalDeposited != 100000 {
		t.Errorf("TotalDeposited = %v, want 100000", perf.TotalDeposited)
	}
	if perf.TotalWithdrawn != 0 {
		t.Errorf("TotalWithdrawn = %v, want 0 (internal transfer_out excluded)", perf.TotalWithdrawn)
	}
	if perf.NetCapitalDeployed != 100000 {
		t.Errorf("NetCapitalDeployed = %v, want 100000 (internal transfers excluded)", perf.NetCapitalDeployed)
	}
	// Transaction count should still include internal transfers (they exist in ledger)
	if perf.TransactionCount != 2 {
		t.Errorf("TransactionCount = %v, want 2 (all transactions counted)", perf.TransactionCount)
	}
}

// --- Edge case 2: ONLY internal transfers, no real deposits ---

func TestCalcPerf_OnlyInternalTransfers_NoDivisionByZero(t *testing.T) {
	// If the ledger contains ONLY internal transfers, they are excluded.
	// Net capital is 0. SimpleReturnPct should be 0 (not NaN from division by zero).
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

	// Internal transfers excluded: TotalWithdrawn = 0
	if perf.TotalDeposited != 0 {
		t.Errorf("TotalDeposited = %v, want 0 (no real deposits)", perf.TotalDeposited)
	}
	if perf.TotalWithdrawn != 0 {
		t.Errorf("TotalWithdrawn = %v, want 0 (internal transfers excluded)", perf.TotalWithdrawn)
	}
	if perf.NetCapitalDeployed != 0 {
		t.Errorf("NetCapitalDeployed = %v, want 0 (no real flows)", perf.NetCapitalDeployed)
	}
	// SimpleReturnPct must be 0 when net capital is 0 (no division by zero)
	if perf.SimpleReturnPct != 0 {
		t.Errorf("SimpleReturnPct = %v, want 0 (zero net capital, no division by zero)", perf.SimpleReturnPct)
	}
	if math.IsNaN(perf.SimpleReturnPct) {
		t.Error("SimpleReturnPct is NaN — division by zero bug")
	}
}

// --- Edge case 3: XIRR with no cashflows after filtering ---

func TestCalcPerf_XIRR_AllInternalTransfers_NoNaN(t *testing.T) {
	// Only internal transfer_out (positive flow) + terminal value (positive).
	// No negative flow → XIRR returns 0 (can't compute rate without both signs).
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

	// XIRR: only positive flows (transfer_out + terminal) → no negative flow → returns 0
	if math.IsNaN(perf.AnnualizedReturnPct) {
		t.Error("AnnualizedReturnPct is NaN — should not happen")
	}
	if math.IsInf(perf.AnnualizedReturnPct, 0) {
		t.Error("AnnualizedReturnPct is Inf")
	}
	if perf.AnnualizedReturnPct != 0 {
		t.Errorf("AnnualizedReturnPct = %v, want 0 (no negative flows for XIRR)", perf.AnnualizedReturnPct)
	}
}

// --- Edge case 4: Asymmetric transfer_in/transfer_out amounts ---

func TestCalcPerf_AsymmetricInternalTransfers(t *testing.T) {
	// transfer_out $60K to accumulate, transfer_in $10K from cash
	// Both are internal — excluded from withdrawal totals
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

	// Deposit: 150000, Internal transfers excluded
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
	if math.Abs(perf.SimpleReturnPct-expectedReturn) > 0.1 {
		t.Errorf("SimpleReturnPct = %.2f, want ~%.2f", perf.SimpleReturnPct, expectedReturn)
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
	// Internal transfer — excluded from withdrawal total
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxTransferOut,
		Date:        time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC),
		Amount:      30000,
		Description: "To accumulate",
		Category:    "accumulate",
	})
	// Real withdrawal — counted
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxWithdrawal,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      25000,
		Description: "Living expenses",
	})
	// Real transfer out (no category) — counted
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
	// Only real withdrawals: 25K (withdrawal) + 10K (real transfer_out) = 35K
	if perf.TotalWithdrawn != 35000 {
		t.Errorf("TotalWithdrawn = %v, want 35000 (25K + 10K real, 30K internal excluded)", perf.TotalWithdrawn)
	}
	if perf.NetCapitalDeployed != 165000 {
		t.Errorf("NetCapitalDeployed = %v, want 165000 (200K - 35K)", perf.NetCapitalDeployed)
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
	// SMSF scenario:
	// - 3 transfer_out with category "accumulate" totaling $60,600
	// - These are excluded from withdrawals (internal capital reallocation)
	// - Total withdrawn = 0
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

	// Internal transfers to accumulate — excluded from withdrawal total
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

	// Real deposits: 200K + 28K + 30K = 258K
	if perf.TotalDeposited != 258000 {
		t.Errorf("TotalDeposited = %v, want 258000 (real deposits only)", perf.TotalDeposited)
	}
	// Internal transfers excluded: TotalWithdrawn = 0
	if perf.TotalWithdrawn != 0 {
		t.Errorf("TotalWithdrawn = %v, want 0 (accumulate transfers excluded)", perf.TotalWithdrawn)
	}
	// Net: 258K - 0 = 258K
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

	// Real deposit only; internal transfer_out excluded
	if perf.TotalDeposited != 100000 {
		t.Errorf("TotalDeposited = %v, want 100000", perf.TotalDeposited)
	}
	if perf.TotalWithdrawn != 0 {
		t.Errorf("TotalWithdrawn = %v, want 0 (internal transfer_out excluded)", perf.TotalWithdrawn)
	}
}

// --- Edge case 10: All four external balance categories as internal ---

func TestCalcPerf_AllExternalBalanceCategories(t *testing.T) {
	// All four external balance types should be excluded from withdrawals
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
				t.Errorf("category=%q: TotalWithdrawn = %v, want 0 (internal transfer excluded)", cat, perf.TotalWithdrawn)
			}
		})
	}
}

// --- Edge case 11: transfer_in with external balance category ---

func TestCalcPerf_TransferIn_AccumulateCategory_IsInternal(t *testing.T) {
	// transfer_in from an external balance account (category=accumulate) is internal
	// Excluded from both deposit and withdrawal totals
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

	// Internal transfer_in is excluded from both deposits and withdrawals
	if perf.TotalDeposited != 0 {
		t.Errorf("TotalDeposited = %v, want 0 (internal transfer_in excluded)", perf.TotalDeposited)
	}
	if perf.TotalWithdrawn != 0 {
		t.Errorf("TotalWithdrawn = %v, want 0 (internal transfer_in excluded)", perf.TotalWithdrawn)
	}
}

// --- Edge case 12: XIRR convergence with mix of internal and real flows ---

func TestCalcPerf_XIRR_UsesTradesNotCashTransactions(t *testing.T) {
	// XIRR now uses buy/sell trades from holdings, not cash transactions.
	// With no Holdings in the mock, XIRR returns 0.
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
	// Internal transfer (excluded from XIRR entirely since XIRR uses trades)
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

	// XIRR uses trades (none in mock) → returns 0
	if math.IsNaN(perf.AnnualizedReturnPct) {
		t.Error("AnnualizedReturnPct is NaN")
	}
	if math.IsInf(perf.AnnualizedReturnPct, 0) {
		t.Error("AnnualizedReturnPct is Inf")
	}
	if perf.AnnualizedReturnPct != 0 {
		t.Errorf("AnnualizedReturnPct = %v, want 0 (no trades in mock portfolio)", perf.AnnualizedReturnPct)
	}
}

// --- Edge case 13: External balance gain/loss tracking by category ---

func TestCalcPerf_ExternalBalanceGainLoss(t *testing.T) {
	// Scenario: accumulate account earns interest
	// Transfer out $600, $10,000 to accumulate, then $10,619.79 comes back (gain $19.79)
	// Then another $50,000 out to accumulate.
	// Net in accumulate: 60,600 - 10,619.79 = 49,980.21
	// Current accumulate balance: 50,000 (grew from 49,980.21)
	// Gain: 50,000 - 49,980.21 = $19.79
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			TotalValueHoldings: 400000,
			ExternalBalances: []models.ExternalBalance{
				{ID: "eb_001", Type: "accumulate", Label: "Stake Accumulate", Value: 50000},
			},
			ExternalBalanceTotal: 50000,
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type: models.CashTxDeposit, Date: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount: 500000, Description: "Initial deposit",
	})
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type: models.CashTxTransferOut, Date: time.Date(2023, 3, 1, 0, 0, 0, 0, time.UTC),
		Amount: 600, Description: "To accumulate", Category: "accumulate",
	})
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type: models.CashTxTransferOut, Date: time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC),
		Amount: 10000, Description: "To accumulate", Category: "accumulate",
	})
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type: models.CashTxTransferIn, Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount: 10619.79, Description: "From accumulate", Category: "accumulate",
	})
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type: models.CashTxTransferOut, Date: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
		Amount: 50000, Description: "To accumulate", Category: "accumulate",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// Internal transfers excluded: TotalWithdrawn = 0
	if perf.TotalWithdrawn != 0 {
		t.Errorf("TotalWithdrawn = %v, want 0 (internal transfers excluded)", perf.TotalWithdrawn)
	}

	// External balance performance
	if len(perf.ExternalBalances) != 1 {
		t.Fatalf("ExternalBalances len = %d, want 1", len(perf.ExternalBalances))
	}

	eb := perf.ExternalBalances[0]
	if eb.Category != "accumulate" {
		t.Errorf("Category = %q, want accumulate", eb.Category)
	}
	if math.Abs(eb.TotalOut-60600) > 0.01 {
		t.Errorf("TotalOut = %v, want 60600", eb.TotalOut)
	}
	if math.Abs(eb.TotalIn-10619.79) > 0.01 {
		t.Errorf("TotalIn = %v, want 10619.79", eb.TotalIn)
	}
	if math.Abs(eb.NetTransferred-49980.21) > 0.01 {
		t.Errorf("NetTransferred = %v, want ~49980.21", eb.NetTransferred)
	}
	if eb.CurrentBalance != 50000 {
		t.Errorf("CurrentBalance = %v, want 50000", eb.CurrentBalance)
	}
	// Gain: 50000 - 49980.21 = 19.79
	if math.Abs(eb.GainLoss-19.79) > 0.01 {
		t.Errorf("GainLoss = %v, want ~19.79", eb.GainLoss)
	}
}

func TestCalcPerf_ExternalBalanceMultipleCategories(t *testing.T) {
	// Multiple external balance categories with different performance
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			TotalValueHoldings: 300000,
			ExternalBalances: []models.ExternalBalance{
				{ID: "eb_001", Type: "accumulate", Label: "Stake Accumulate", Value: 52000},
				{ID: "eb_002", Type: "term_deposit", Label: "ING Term", Value: 100500},
			},
			ExternalBalanceTotal: 152500,
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type: models.CashTxDeposit, Date: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount: 600000, Description: "Initial deposit",
	})
	// Accumulate transfers: out 50K, current 52K → gain 2K
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type: models.CashTxTransferOut, Date: time.Date(2023, 3, 1, 0, 0, 0, 0, time.UTC),
		Amount: 50000, Description: "To accumulate", Category: "accumulate",
	})
	// Term deposit transfers: out 100K, current 100.5K → gain 500
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type: models.CashTxTransferOut, Date: time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC),
		Amount: 100000, Description: "To term deposit", Category: "term_deposit",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	if len(perf.ExternalBalances) != 2 {
		t.Fatalf("ExternalBalances len = %d, want 2", len(perf.ExternalBalances))
	}

	// Find each category
	ebMap := make(map[string]models.ExternalBalancePerformance)
	for _, eb := range perf.ExternalBalances {
		ebMap[eb.Category] = eb
	}

	acc := ebMap["accumulate"]
	if acc.TotalOut != 50000 {
		t.Errorf("accumulate TotalOut = %v, want 50000", acc.TotalOut)
	}
	if acc.CurrentBalance != 52000 {
		t.Errorf("accumulate CurrentBalance = %v, want 52000", acc.CurrentBalance)
	}
	if math.Abs(acc.GainLoss-2000) > 0.01 {
		t.Errorf("accumulate GainLoss = %v, want 2000", acc.GainLoss)
	}

	td := ebMap["term_deposit"]
	if td.TotalOut != 100000 {
		t.Errorf("term_deposit TotalOut = %v, want 100000", td.TotalOut)
	}
	if td.CurrentBalance != 100500 {
		t.Errorf("term_deposit CurrentBalance = %v, want 100500", td.CurrentBalance)
	}
	if math.Abs(td.GainLoss-500) > 0.01 {
		t.Errorf("term_deposit GainLoss = %v, want 500", td.GainLoss)
	}
}
