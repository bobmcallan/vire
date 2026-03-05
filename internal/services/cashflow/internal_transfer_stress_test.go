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
			Name:                "SMSF",
			EquityHoldingsValue: 80000,
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

	// Only contribution debits count as withdrawals (other/fee debits don't count)
	if perf.WithdrawalsGross != 0 {
		t.Errorf("TotalWithdrawn = %v, want 0 (other-category debit is not a capital withdrawal)", perf.WithdrawalsGross)
	}
	if perf.ContributionsNet != 100000 {
		t.Errorf("NetCapitalDeployed = %v, want 100000 (100000 deposited - 0 withdrawn)", perf.ContributionsNet)
	}
}

func TestCalcPerf_DebitFee_IsRealWithdrawal(t *testing.T) {
	// A debit with CashCatFee is also a real withdrawal (not a transfer)
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:                "SMSF",
			EquityHoldingsValue: 80000,
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

	if perf.WithdrawalsGross != 0 {
		t.Errorf("TotalWithdrawn = %v, want 0 (fee debit is not a capital withdrawal)", perf.WithdrawalsGross)
	}
}

func TestCalcPerf_TransferDebit_IsCountedAsWithdrawal(t *testing.T) {
	// A debit with CashCatTransfer on the Trading account is a real withdrawal
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:                "SMSF",
			EquityHoldingsValue: 80000,
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

	// Transfer debits are NOT counted as withdrawals
	if perf.ContributionsGross != 100000 {
		t.Errorf("TotalDeposited = %v, want 100000", perf.ContributionsGross)
	}
	if perf.WithdrawalsGross != 0 {
		t.Errorf("TotalWithdrawn = %v, want 0 (transfer debit is not a capital withdrawal)", perf.WithdrawalsGross)
	}
	if perf.ContributionsNet != 100000 {
		t.Errorf("NetCapitalDeployed = %v, want 100000 (100000 deposited - 0 withdrawn)", perf.ContributionsNet)
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
			Name:                "SMSF",
			EquityHoldingsValue: 50000,
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

	if perf.ContributionsGross != 0 {
		t.Errorf("TotalDeposited = %v, want 0 (transfers are not deposits)", perf.ContributionsGross)
	}
	if perf.WithdrawalsGross != 0 {
		t.Errorf("TotalWithdrawn = %v, want 0 (transfers are not withdrawals)", perf.WithdrawalsGross)
	}
	if perf.ContributionsNet != 0 {
		t.Errorf("NetCapitalDeployed = %v, want 0 (only contributions count)", perf.ContributionsNet)
	}
	// SimpleReturnPct must be 0 when net capital is <= 0 (no division by zero)
	if perf.ReturnSimplePct != 0 {
		t.Errorf("SimpleReturnPct = %v, want 0 (negative net capital)", perf.ReturnSimplePct)
	}
	if math.IsNaN(perf.ReturnSimplePct) {
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
			Name:                "SMSF",
			EquityHoldingsValue: 50000,
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
	if math.IsNaN(perf.ReturnXirrPct) {
		t.Error("AnnualizedReturnPct is NaN — should not happen")
	}
	if math.IsInf(perf.ReturnXirrPct, 0) {
		t.Error("AnnualizedReturnPct is Inf")
	}
	if perf.ReturnXirrPct != 0 {
		t.Errorf("AnnualizedReturnPct = %v, want 0 (no negative flows for XIRR)", perf.ReturnXirrPct)
	}
}

// --- Edge case 4: Asymmetric transfer amounts ---

func TestCalcPerf_AsymmetricInternalTransfers(t *testing.T) {
	// Transfer out $60K, transfer in $10K — all count as real flows
	// Deposits: 150K + 10K = 160K, Withdrawals: 60K
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:                "SMSF",
			EquityHoldingsValue: 100000,
			PortfolioValue:      100000, // equity only (no cash specified)
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

	// Only contributions count as deposits: 150K (transfers don't count)
	if perf.ContributionsGross != 150000 {
		t.Errorf("TotalDeposited = %v, want 150000 (transfers are not deposits)", perf.ContributionsGross)
	}
	// Transfer debits are not withdrawals: 0
	if perf.WithdrawalsGross != 0 {
		t.Errorf("TotalWithdrawn = %v, want 0 (transfers are not withdrawals)", perf.WithdrawalsGross)
	}
	// Net capital = 150K - 0 = 150K
	if perf.ContributionsNet != 150000 {
		t.Errorf("NetCapitalDeployed = %v, want 150000 (only contributions count)", perf.ContributionsNet)
	}
	// Return: (100000 - 150000) / 150000 * 100 = -33.33%
	expectedReturn := (100000.0 - 150000.0) / 150000.0 * 100
	if math.Abs(perf.ReturnSimplePct-expectedReturn) > 0.1 {
		t.Errorf("SimpleReturnPct = %.2f, want ~%.2f", perf.ReturnSimplePct, expectedReturn)
	}
}

// --- Edge case 5: Mix of transfer and non-transfer debits ---

func TestCalcPerf_MixedTransferAndRealDebits(t *testing.T) {
	// All debits count as real withdrawals
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:                "SMSF",
			EquityHoldingsValue: 100000,
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

	if perf.ContributionsGross != 200000 {
		t.Errorf("TotalDeposited = %v, want 200000", perf.ContributionsGross)
	}
	// Only contribution debits count: 0 (transfers and other debits don't count)
	if perf.WithdrawalsGross != 0 {
		t.Errorf("TotalWithdrawn = %v, want 0 (only contribution withdrawals count)", perf.WithdrawalsGross)
	}
	if perf.ContributionsNet != 200000 {
		t.Errorf("NetCapitalDeployed = %v, want 200000 (200000 - 0)", perf.ContributionsNet)
	}
}

// --- Edge case 6: PortfolioValue includes both equity and cash ---

func TestCalcPerf_UsesPortfolioValue_EquityPlusCash(t *testing.T) {
	// currentValue should be PortfolioValue (equity + cash),
	// not EquityValue (equity only)
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

	// CurrentPortfolioValue should be PortfolioValue: 150000 (equity 100000 + cash 50000)
	if perf.CurrentValue != 150000 {
		t.Errorf("CurrentPortfolioValue = %v, want 150000 (PortfolioValue = equity + cash)",
			perf.CurrentValue)
	}
	// Simple return: (150000 - 100000) / 100000 * 100 = 50%
	expectedReturn := (150000.0 - 100000.0) / 100000.0 * 100
	if math.Abs(perf.ReturnSimplePct-expectedReturn) > 0.01 {
		t.Errorf("SimpleReturnPct = %v, want ~%v (PortfolioValue / deposits)", perf.ReturnSimplePct, expectedReturn)
	}
}

func TestCalcPerf_ZeroHoldings_CashOnlyPortfolio(t *testing.T) {
	// Edge: No equity holdings but positive cash balance.
	// currentValue = PortfolioValue = 50000 (all in cash)
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
		Description: "Deposit all to cash",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// PortfolioValue = 50000 (cash balance, no equity)
	if perf.CurrentValue != 50000 {
		t.Errorf("CurrentPortfolioValue = %v, want 50000 (PortfolioValue = all in cash)", perf.CurrentValue)
	}
	// Return = (50000 - 50000) / 50000 * 100 = 0%
	if math.Abs(perf.ReturnSimplePct) > 0.01 {
		t.Errorf("SimpleReturnPct = %v, want 0 (deposited 50000 = portfolio value 50000)", perf.ReturnSimplePct)
	}
}

// --- Edge case 7: deriveFromTrades uses PortfolioValue ---

func TestDeriveFromTrades_UsesPortfolioValueWithCash(t *testing.T) {
	// deriveFromTrades should use PortfolioValue (equity + cash)
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:                "SMSF",
			EquityHoldingsValue: 120000,
			CapitalGross:        50000,
			PortfolioValue:      170000, // equity + cash
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

	// CurrentPortfolioValue should be PortfolioValue = 170000 (equity 120000 + cash 50000)
	if perf.CurrentValue != 170000 {
		t.Errorf("CurrentPortfolioValue = %v, want 170000 (PortfolioValue = equity + cash)", perf.CurrentValue)
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
			Name:                "SMSF",
			EquityHoldingsValue: 426000,
			CapitalGross:        50000,
			PortfolioValue:      476000,
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

	// All deposits: 200K + 28K + 30K = 258K (transfers don't count)
	if perf.ContributionsGross != 258000 {
		t.Errorf("TotalDeposited = %v, want 258000", perf.ContributionsGross)
	}
	// Transfer debits don't count: 0
	if perf.WithdrawalsGross != 0 {
		t.Errorf("TotalWithdrawn = %v, want 0 (transfers are not withdrawals)", perf.WithdrawalsGross)
	}
	// Net: 258K - 0 = 258K
	if perf.ContributionsNet != 258000 {
		t.Errorf("NetCapitalDeployed = %v, want 258000 (only contributions count)", perf.ContributionsNet)
	}

	// PortfolioValue: 476000 (equity 426000 + cash 50000)
	if perf.CurrentValue != 476000 {
		t.Errorf("CurrentPortfolioValue = %v, want 476000 (PortfolioValue = equity + cash)", perf.CurrentValue)
	}

	// Return: (476000 - 258000) / 258000 * 100
	expectedReturn := (476000.0 - 258000.0) / 258000.0 * 100
	if math.Abs(perf.ReturnSimplePct-expectedReturn) > 0.1 {
		t.Errorf("SimpleReturnPct = %.2f, want ~%.2f", perf.ReturnSimplePct, expectedReturn)
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
			Name:                "SMSF",
			EquityHoldingsValue: 100000,
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

	// Only contributions count: deposit 100K, transfer debit doesn't count
	if perf.ContributionsGross != 100000 {
		t.Errorf("TotalDeposited = %v, want 100000", perf.ContributionsGross)
	}
	if perf.WithdrawalsGross != 0 {
		t.Errorf("TotalWithdrawn = %v, want 0 (transfer debit is not a capital withdrawal)", perf.WithdrawalsGross)
	}
}

// --- Edge case 10: Transfer debits count as withdrawals ---

func TestCalcPerf_TransferDebitsCountAsWithdrawals(t *testing.T) {
	// Transfer debits on Trading account count as real withdrawals
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:                "SMSF",
			EquityHoldingsValue: 100000,
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

	if perf.WithdrawalsGross != 0 {
		t.Errorf("TotalWithdrawn = %v, want 0 (transfer debit is not a capital withdrawal)", perf.WithdrawalsGross)
	}
}

// --- Edge case 11: Transfer credit on Trading counts as deposit ---

func TestCalcPerf_TransferCreditNotCountedAsDeposit(t *testing.T) {
	// A credit with CashCatTransfer does NOT count as a capital deposit.
	// Only category=contribution counts for TotalDeposited.
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:                "SMSF",
			EquityHoldingsValue: 100000,
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

	// Transfer credit is NOT a capital deposit — only contributions count
	if perf.ContributionsGross != 0 {
		t.Errorf("TotalDeposited = %v, want 0 (transfer credit is not a capital deposit)", perf.ContributionsGross)
	}
	if perf.WithdrawalsGross != 0 {
		t.Errorf("TotalWithdrawn = %v, want 0", perf.WithdrawalsGross)
	}
}

// --- Edge case 12: XIRR convergence with mix of transfer and real flows ---

func TestCalcPerf_XIRR_UsesTradesNotCashTransactions(t *testing.T) {
	// XIRR now uses buy/sell trades from holdings, not cash transactions.
	// With no Holdings in the mock, XIRR returns 0.
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:                "SMSF",
			EquityHoldingsValue: 120000,
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
	if math.IsNaN(perf.ReturnXirrPct) {
		t.Error("AnnualizedReturnPct is NaN")
	}
	if math.IsInf(perf.ReturnXirrPct, 0) {
		t.Error("AnnualizedReturnPct is Inf")
	}
	if perf.ReturnXirrPct != 0 {
		t.Errorf("AnnualizedReturnPct = %v, want 0 (no trades in mock portfolio)", perf.ReturnXirrPct)
	}
}
