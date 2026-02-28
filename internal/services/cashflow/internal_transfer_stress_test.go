package cashflow

import (
	"math"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
)

// Devils-advocate stress tests for CalculatePerformance and XIRR.
//
// These tests verify that:
// 1. All transactions count as real flows (credits=deposits, debits=withdrawals)
// 2. Edge cases around all-transfer portfolios, division by zero, etc.
// 3. External balance gain/loss tracking still works via per-account flows

// --- Edge case 1: debit with non-transfer category counts as real withdrawal ---

func TestCalcPerf_DebitOther_IsRealWithdrawal(t *testing.T) {
	// A debit with CashCatOther is a REAL withdrawal, not excluded.
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
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      100000,
		Description: "Initial deposit",
	})
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatOther,
		Date:        time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		Amount:      -20000,
		Description: "Withdrawal - real outflow",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// Non-transfer debit is a REAL withdrawal, should be counted
	if perf.TotalWithdrawn != 20000 {
		t.Errorf("TotalWithdrawn = %v, want 20000 (non-transfer debit is real withdrawal)", perf.TotalWithdrawn)
	}
	if perf.NetCapitalDeployed != 80000 {
		t.Errorf("NetCapitalDeployed = %v, want 80000", perf.NetCapitalDeployed)
	}
}

func TestCalcPerf_DebitFee_IsRealWithdrawal(t *testing.T) {
	// A debit with CashCatFee is also a real withdrawal (not a transfer)
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
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      100000,
		Description: "Initial deposit",
	})
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatFee,
		Date:        time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		Amount:      -15000,
		Description: "Management fee",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	if perf.TotalWithdrawn != 15000 {
		t.Errorf("TotalWithdrawn = %v, want 15000 (fee debit is real withdrawal)", perf.TotalWithdrawn)
	}
}

func TestCalcPerf_TransferDebit_IsCountedAsWithdrawal(t *testing.T) {
	// A debit with CashCatTransfer on the Trading account is a real withdrawal
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
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      100000,
		Description: "Initial deposit",
	})
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatTransfer,
		Date:        time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		Amount:      -20000,
		Description: "Transfer to accumulate",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// Transfer debit counts as a real withdrawal
	if perf.TotalDeposited != 100000 {
		t.Errorf("TotalDeposited = %v, want 100000", perf.TotalDeposited)
	}
	if perf.TotalWithdrawn != 20000 {
		t.Errorf("TotalWithdrawn = %v, want 20000 (transfer debit is a real withdrawal)", perf.TotalWithdrawn)
	}
	if perf.NetCapitalDeployed != 80000 {
		t.Errorf("NetCapitalDeployed = %v, want 80000", perf.NetCapitalDeployed)
	}
	if perf.TransactionCount != 2 {
		t.Errorf("TransactionCount = %v, want 2", perf.TransactionCount)
	}
}

// --- Edge case 2: ONLY internal transfers, no real deposits ---

func TestCalcPerf_OnlyInternalTransfers_NegativeNetCapital(t *testing.T) {
	// If the ledger contains ONLY transfer entries, they count as real flows.
	// Debit 20K, credit 10K → net = -10K. SimpleReturnPct should be 0 (negative net capital).
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
		Account:     "Trading",
		Category:    models.CashCatTransfer,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      -20000,
		Description: "To accumulate",
	})
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatTransfer,
		Date:        time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		Amount:      10000,
		Description: "From cash account",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	if perf.TotalDeposited != 10000 {
		t.Errorf("TotalDeposited = %v, want 10000", perf.TotalDeposited)
	}
	if perf.TotalWithdrawn != 20000 {
		t.Errorf("TotalWithdrawn = %v, want 20000", perf.TotalWithdrawn)
	}
	if perf.NetCapitalDeployed != -10000 {
		t.Errorf("NetCapitalDeployed = %v, want -10000", perf.NetCapitalDeployed)
	}
	// SimpleReturnPct must be 0 when net capital is <= 0 (no division by zero)
	if perf.SimpleReturnPct != 0 {
		t.Errorf("SimpleReturnPct = %v, want 0 (negative net capital)", perf.SimpleReturnPct)
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
		Account:     "Trading",
		Category:    models.CashCatTransfer,
		Date:        time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
		Amount:      -30000,
		Description: "To term deposit",
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

// --- Edge case 4: Asymmetric transfer amounts ---

func TestCalcPerf_AsymmetricInternalTransfers(t *testing.T) {
	// Transfer out $60K, transfer in $10K — all count as real flows
	// Deposits: 150K + 10K = 160K, Withdrawals: 60K
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
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      150000,
		Description: "Initial deposit",
	})
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatTransfer,
		Date:        time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC),
		Amount:      -60000,
		Description: "Move to accumulate",
	})
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatTransfer,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      10000,
		Description: "Return from cash",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// All credits are deposits: 150K + 10K = 160K
	if perf.TotalDeposited != 160000 {
		t.Errorf("TotalDeposited = %v, want 160000", perf.TotalDeposited)
	}
	// Transfer debit counts: 60K
	if perf.TotalWithdrawn != 60000 {
		t.Errorf("TotalWithdrawn = %v, want 60000", perf.TotalWithdrawn)
	}
	// Net capital = 160K - 60K = 100K
	if perf.NetCapitalDeployed != 100000 {
		t.Errorf("NetCapitalDeployed = %v, want 100000", perf.NetCapitalDeployed)
	}
	// Return: (100000 - 100000) / 100000 * 100 = 0%
	if math.Abs(perf.SimpleReturnPct) > 0.1 {
		t.Errorf("SimpleReturnPct = %.2f, want ~0", perf.SimpleReturnPct)
	}
}

// --- Edge case 5: Mix of transfer and non-transfer debits ---

func TestCalcPerf_MixedTransferAndRealDebits(t *testing.T) {
	// All debits count as real withdrawals
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
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      200000,
		Description: "Initial deposit",
	})
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatTransfer,
		Date:        time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC),
		Amount:      -30000,
		Description: "To accumulate",
	})
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatOther,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      -25000,
		Description: "Living expenses",
	})
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatOther,
		Date:        time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
		Amount:      -10000,
		Description: "Transfer to spouse",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	if perf.TotalDeposited != 200000 {
		t.Errorf("TotalDeposited = %v, want 200000", perf.TotalDeposited)
	}
	// All debits count: 30K + 25K + 10K = 65K
	if perf.TotalWithdrawn != 65000 {
		t.Errorf("TotalWithdrawn = %v, want 65000 (30K + 25K + 10K)", perf.TotalWithdrawn)
	}
	if perf.NetCapitalDeployed != 135000 {
		t.Errorf("NetCapitalDeployed = %v, want 135000 (200K - 65K)", perf.NetCapitalDeployed)
	}
}

// --- Edge case 6: Holdings-only value (not total with external balances) ---

func TestCalcPerf_UsesHoldingsOnly_NotTotalValue(t *testing.T) {
	// currentValue should be TotalValueHoldings ONLY,
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
	// currentValue = 0 (holdings-only), not 50000 (external)
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
		Account:     "Trading",
		Category:    models.CashCatContribution,
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
	// - 3 contribution credits totaling $258K
	// - 3 transfer debits totaling $60,600
	// - All count as real flows
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
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2022, 7, 1, 0, 0, 0, 0, time.UTC),
		Amount:      200000,
		Description: "Initial rollover",
	})
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2023, 7, 1, 0, 0, 0, 0, time.UTC),
		Amount:      28000,
		Description: "FY23 contribution",
	})
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC),
		Amount:      30000,
		Description: "FY24 contribution",
	})

	// Transfer debits — now count as real withdrawals
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatTransfer,
		Date:        time.Date(2023, 1, 15, 0, 0, 0, 0, time.UTC),
		Amount:      -20000,
		Description: "To Stake Accumulate",
	})
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatTransfer,
		Date:        time.Date(2023, 7, 15, 0, 0, 0, 0, time.UTC),
		Amount:      -20300,
		Description: "To Stake Accumulate",
	})
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatTransfer,
		Date:        time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
		Amount:      -20300,
		Description: "To Stake Accumulate",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// All deposits: 200K + 28K + 30K = 258K
	if perf.TotalDeposited != 258000 {
		t.Errorf("TotalDeposited = %v, want 258000", perf.TotalDeposited)
	}
	// Transfer debits count: 20K + 20.3K + 20.3K = 60.6K
	if perf.TotalWithdrawn != 60600 {
		t.Errorf("TotalWithdrawn = %v, want 60600", perf.TotalWithdrawn)
	}
	// Net: 258K - 60.6K = 197.4K
	if perf.NetCapitalDeployed != 197400 {
		t.Errorf("NetCapitalDeployed = %v, want 197400", perf.NetCapitalDeployed)
	}

	// Holdings-only value: 426000
	if perf.CurrentPortfolioValue != 426000 {
		t.Errorf("CurrentPortfolioValue = %v, want 426000 (holdings-only)", perf.CurrentPortfolioValue)
	}

	// Return: (426000 - 197400) / 197400 * 100
	expectedReturn := (426000.0 - 197400.0) / 197400.0 * 100
	if math.Abs(perf.SimpleReturnPct-expectedReturn) > 0.1 {
		t.Errorf("SimpleReturnPct = %.2f, want ~%.2f", perf.SimpleReturnPct, expectedReturn)
	}

	// All 6 transactions in ledger
	if perf.TransactionCount != 6 {
		t.Errorf("TransactionCount = %d, want 6", perf.TransactionCount)
	}
}

// --- Edge case 9: FirstTransactionDate with transfer first ---

func TestCalcPerf_FirstTransactionIsTransfer(t *testing.T) {
	// If the earliest transaction is a transfer, it should still
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

	// Transfer is the FIRST transaction
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatTransfer,
		Date:        time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      -10000,
		Description: "Early reallocation",
	})
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC),
		Amount:      100000,
		Description: "Real deposit",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// FirstTransactionDate should be the transfer (earliest in ledger)
	expectedFirst := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	if perf.FirstTransactionDate == nil {
		t.Fatal("FirstTransactionDate should not be nil")
	}
	if !perf.FirstTransactionDate.Equal(expectedFirst) {
		t.Errorf("FirstTransactionDate = %v, want %v", perf.FirstTransactionDate, expectedFirst)
	}

	// All transactions count: deposit 100K, transfer debit 10K
	if perf.TotalDeposited != 100000 {
		t.Errorf("TotalDeposited = %v, want 100000", perf.TotalDeposited)
	}
	if perf.TotalWithdrawn != 10000 {
		t.Errorf("TotalWithdrawn = %v, want 10000 (transfer debit counts)", perf.TotalWithdrawn)
	}
}

// --- Edge case 10: Transfer debits count as withdrawals ---

func TestCalcPerf_TransferDebitsCountAsWithdrawals(t *testing.T) {
	// Transfer debits on Trading account count as real withdrawals
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
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      100000,
		Description: "Deposit",
	})
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatTransfer,
		Date:        time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		Amount:      -20000,
		Description: "Transfer to external",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	if perf.TotalWithdrawn != 20000 {
		t.Errorf("TotalWithdrawn = %v, want 20000 (transfer debit counts)", perf.TotalWithdrawn)
	}
}

// --- Edge case 11: Transfer credit on Trading counts as deposit ---

func TestCalcPerf_TransferCreditOnTrading_CountsAsDeposit(t *testing.T) {
	// A credit with CashCatTransfer on Trading account counts as a deposit
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
		Account:     "Trading",
		Category:    models.CashCatTransfer,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      30000,
		Description: "Return from accumulate",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	if perf.TotalDeposited != 30000 {
		t.Errorf("TotalDeposited = %v, want 30000 (transfer credit counts as deposit)", perf.TotalDeposited)
	}
	if perf.TotalWithdrawn != 0 {
		t.Errorf("TotalWithdrawn = %v, want 0", perf.TotalWithdrawn)
	}
}

// --- Edge case 12: XIRR convergence with mix of transfer and real flows ---

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
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      100000,
		Description: "Initial deposit",
	})
	// Transfer (excluded from XIRR entirely since XIRR uses trades)
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatTransfer,
		Date:        time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC),
		Amount:      -50000,
		Description: "To offset",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// XIRR uses trades (none in mock) -> returns 0
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
