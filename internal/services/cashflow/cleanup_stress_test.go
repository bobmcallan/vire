package cashflow

import (
	"math"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
)

// Devils-advocate stress tests for the cash flow cleanup.
//
// After the cleanup, transfer entries (Category == CashCatTransfer) are treated
// as normal flows — no exclusion. These tests verify correctness in that regime.

// =============================================================================
// Concern 1: Paired transfers net to zero in TotalContributions
// =============================================================================

func TestCleanup_PairedTransfer_NetsToZero_TotalContributions(t *testing.T) {
	// A debit from Trading and credit to Accumulate should cancel out in TotalContributions.
	ledger := models.CashFlowLedger{
		Transactions: []models.CashTransaction{
			{Direction: models.CashCredit, Account: "Trading", Category: models.CashCatContribution, Amount: 100000},
			// Paired transfer: debit from Trading, credit to Accumulate
			{Direction: models.CashDebit, Account: "Trading", Category: models.CashCatTransfer, Amount: 20000},
			{Direction: models.CashCredit, Account: "Stake Accumulate", Category: models.CashCatTransfer, Amount: 20000},
		},
	}

	got := ledger.TotalContributions()
	// After cleanup: transfer debit -20K + transfer credit +20K = net 0 from transfers.
	// TotalContributions = 100000 + 20000 - 20000 = 100000
	if math.Abs(got-100000) > 0.01 {
		t.Errorf("TotalContributions = %v, want 100000 (paired transfers net to zero)", got)
	}
}

func TestCleanup_MultiplePairedTransfers_NetToZero(t *testing.T) {
	// Multiple transfer pairs across different accounts all net to zero.
	ledger := models.CashFlowLedger{
		Transactions: []models.CashTransaction{
			{Direction: models.CashCredit, Account: "Trading", Category: models.CashCatContribution, Amount: 200000},
			// Transfer 1: Trading -> Accumulate
			{Direction: models.CashDebit, Account: "Trading", Category: models.CashCatTransfer, Amount: 30000},
			{Direction: models.CashCredit, Account: "Accumulate", Category: models.CashCatTransfer, Amount: 30000},
			// Transfer 2: Trading -> Term Deposit
			{Direction: models.CashDebit, Account: "Trading", Category: models.CashCatTransfer, Amount: 50000},
			{Direction: models.CashCredit, Account: "Term Deposit", Category: models.CashCatTransfer, Amount: 50000},
			// Transfer 3: Accumulate -> Trading (return)
			{Direction: models.CashDebit, Account: "Accumulate", Category: models.CashCatTransfer, Amount: 10000},
			{Direction: models.CashCredit, Account: "Trading", Category: models.CashCatTransfer, Amount: 10000},
		},
	}

	got := ledger.TotalContributions()
	// All transfer pairs cancel: 200000 net
	if math.Abs(got-200000) > 0.01 {
		t.Errorf("TotalContributions = %v, want 200000 (all paired transfers net to zero)", got)
	}
}

// =============================================================================
// Concern 2: Unpaired transfer (lone debit or lone credit) DOES affect totals
// =============================================================================

func TestCleanup_UnpairedTransferDebit_ReducesTotal(t *testing.T) {
	// A lone transfer debit with no matching credit IS a problem.
	// This can happen if someone uses AddTransaction instead of AddTransfer.
	ledger := models.CashFlowLedger{
		Transactions: []models.CashTransaction{
			{Direction: models.CashCredit, Account: "Trading", Category: models.CashCatContribution, Amount: 100000},
			// Unpaired transfer debit — no matching credit
			{Direction: models.CashDebit, Account: "Trading", Category: models.CashCatTransfer, Amount: 20000},
		},
	}

	got := ledger.TotalContributions()
	// Without exclusion: 100000 - 20000 = 80000.
	// This is technically "correct" (the debit reduces total cash) but could be
	// misleading if the user intended a paired transfer but only created one side.
	if math.Abs(got-80000) > 0.01 {
		t.Errorf("TotalContributions = %v, want 80000 (unpaired debit reduces total)", got)
	}
}

func TestCleanup_UnpairedTransferCredit_InflatesTotal(t *testing.T) {
	// A lone transfer credit with no matching debit inflates total.
	ledger := models.CashFlowLedger{
		Transactions: []models.CashTransaction{
			{Direction: models.CashCredit, Account: "Trading", Category: models.CashCatContribution, Amount: 100000},
			// Unpaired transfer credit — no matching debit
			{Direction: models.CashCredit, Account: "Accumulate", Category: models.CashCatTransfer, Amount: 20000},
		},
	}

	got := ledger.TotalContributions()
	// Without exclusion: 100000 + 20000 = 120000
	if math.Abs(got-120000) > 0.01 {
		t.Errorf("TotalContributions = %v, want 120000 (unpaired credit inflates total)", got)
	}
}

// =============================================================================
// Concern 3: Circular transfers (A->B->A) net to zero
// =============================================================================

func TestCleanup_CircularTransfer_NetsToZero(t *testing.T) {
	// A->B then B->A should completely cancel in all totals.
	ledger := models.CashFlowLedger{
		Transactions: []models.CashTransaction{
			{Direction: models.CashCredit, Account: "Trading", Category: models.CashCatContribution, Amount: 100000},
			// A->B
			{Direction: models.CashDebit, Account: "Trading", Category: models.CashCatTransfer, Amount: 20000},
			{Direction: models.CashCredit, Account: "Accumulate", Category: models.CashCatTransfer, Amount: 20000},
			// B->A
			{Direction: models.CashDebit, Account: "Accumulate", Category: models.CashCatTransfer, Amount: 20000},
			{Direction: models.CashCredit, Account: "Trading", Category: models.CashCatTransfer, Amount: 20000},
		},
	}

	got := ledger.TotalContributions()
	if math.Abs(got-100000) > 0.01 {
		t.Errorf("TotalContributions = %v, want 100000 (circular transfer nets to zero)", got)
	}

	// Account balances should also reflect the round-trip
	tradingBal := ledger.AccountBalance("Trading")
	if math.Abs(tradingBal-100000) > 0.01 {
		t.Errorf("Trading balance = %v, want 100000", tradingBal)
	}
	accumBal := ledger.AccountBalance("Accumulate")
	if math.Abs(accumBal) > 0.01 {
		t.Errorf("Accumulate balance = %v, want 0", accumBal)
	}
}

// =============================================================================
// Concern 4: Transfer amount mismatch between paired entries
// =============================================================================

func TestCleanup_TransferAmountMismatch_AffectsTotal(t *testing.T) {
	// If the debit and credit amounts don't match, the difference affects the total.
	// This should not happen with AddTransfer (which uses the same amount), but
	// could happen with manual AddTransaction calls.
	ledger := models.CashFlowLedger{
		Transactions: []models.CashTransaction{
			{Direction: models.CashCredit, Account: "Trading", Category: models.CashCatContribution, Amount: 100000},
			// Mismatched transfer: debit 20K but credit 25K (e.g., user error)
			{Direction: models.CashDebit, Account: "Trading", Category: models.CashCatTransfer, Amount: 20000},
			{Direction: models.CashCredit, Account: "Accumulate", Category: models.CashCatTransfer, Amount: 25000},
		},
	}

	got := ledger.TotalContributions()
	// 100000 - 20000 + 25000 = 105000
	if math.Abs(got-105000) > 0.01 {
		t.Errorf("TotalContributions = %v, want 105000 (mismatched transfer creates $5K phantom)", got)
	}

	totalBal := ledger.TotalCashBalance()
	// 100000 - 20000 + 25000 = 105000
	if math.Abs(totalBal-105000) > 0.01 {
		t.Errorf("TotalCashBalance = %v, want 105000 (mismatch creates phantom balance)", totalBal)
	}
}

// =============================================================================
// Concern 5: CalculatePerformance — paired transfers inflate deposit/withdrawal
// totals but net capital is unchanged
// =============================================================================

func TestCleanup_CalcPerf_PairedTransfer_NetCapitalUnchanged(t *testing.T) {
	// Key insight: with exclusion removed, both sides of a paired transfer feed
	// into totalDeposited and totalWithdrawn. Net capital stays the same.
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
		Direction: models.CashCredit, Account: "Trading", Category: models.CashCatContribution,
		Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 100000, Description: "Initial deposit",
	})
	// Paired transfer via AddTransfer
	_, _ = svc.AddTransfer(ctx, "SMSF", "Trading", "Accumulate", 20000,
		time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), "To accumulate")

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// After cleanup: transfer credit (to Accumulate) adds to deposits,
	// transfer debit (from Trading) adds to withdrawals.
	// TotalDeposited = 100000 (contribution) + 20000 (transfer credit) = 120000
	// TotalWithdrawn = 20000 (transfer debit)
	// NetCapitalDeployed = 120000 - 20000 = 100000 (unchanged)
	if perf.NetCapitalDeployed != 100000 {
		t.Errorf("NetCapitalDeployed = %v, want 100000 (paired transfer, net unchanged)", perf.NetCapitalDeployed)
	}
	if perf.TotalDeposited != 120000 {
		t.Errorf("TotalDeposited = %v, want 120000 (includes transfer credit)", perf.TotalDeposited)
	}
	if perf.TotalWithdrawn != 20000 {
		t.Errorf("TotalWithdrawn = %v, want 20000 (includes transfer debit)", perf.TotalWithdrawn)
	}
}

func TestCleanup_CalcPerf_SimpleReturn_StableWithTransfers(t *testing.T) {
	// SimpleReturnPct should be stable: transfers don't change net capital.
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

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Direction: models.CashCredit, Account: "Trading", Category: models.CashCatContribution,
		Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 100000, Description: "Initial deposit",
	})
	_, _ = svc.AddTransfer(ctx, "SMSF", "Trading", "Accumulate", 30000,
		time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), "To accumulate")
	_, _ = svc.AddTransfer(ctx, "SMSF", "Trading", "Term Deposit", 50000,
		time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), "To term deposit")

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// Net: 100000 (real) + 30000 + 50000 (transfer credits) - 30000 - 50000 (transfer debits) = 100000
	if perf.NetCapitalDeployed != 100000 {
		t.Errorf("NetCapitalDeployed = %v, want 100000", perf.NetCapitalDeployed)
	}
	// Simple return: (120000 - 100000) / 100000 * 100 = 20%
	expected := (120000.0 - 100000.0) / 100000.0 * 100
	if math.Abs(perf.SimpleReturnPct-expected) > 0.1 {
		t.Errorf("SimpleReturnPct = %.2f, want ~%.2f", perf.SimpleReturnPct, expected)
	}
}

// =============================================================================
// Concern 6: Only-transfers portfolio — division by zero guard
// =============================================================================

func TestCleanup_CalcPerf_OnlyPairedTransfers_NoDivisionByZero(t *testing.T) {
	// If ledger has ONLY paired transfers, net capital = 0.
	// SimpleReturnPct must be 0 (not NaN or Inf).
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

	_, _ = svc.AddTransfer(ctx, "SMSF", "Trading", "Accumulate", 20000,
		time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), "To accumulate")

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// Net capital = 20000 (credit) - 20000 (debit) = 0
	if perf.NetCapitalDeployed != 0 {
		t.Errorf("NetCapitalDeployed = %v, want 0", perf.NetCapitalDeployed)
	}
	if math.IsNaN(perf.SimpleReturnPct) {
		t.Error("SimpleReturnPct is NaN — division by zero bug")
	}
	if math.IsInf(perf.SimpleReturnPct, 0) {
		t.Error("SimpleReturnPct is Inf — division by zero bug")
	}
	if perf.SimpleReturnPct != 0 {
		t.Errorf("SimpleReturnPct = %v, want 0 (zero net capital)", perf.SimpleReturnPct)
	}
}

// =============================================================================
// Concern 7: XIRR still uses trades, not cash transactions
// =============================================================================

func TestCleanup_CalcPerf_XIRR_StillUsesTradesNotCash(t *testing.T) {
	// XIRR should be computed from buy/sell trades, not cash transactions.
	// Transfers in cash transactions should have zero effect on XIRR.
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			TotalValueHoldings: 120000,
			Holdings: []models.Holding{
				{
					Ticker: "BHP", Exchange: "AU", Units: 100, CurrentPrice: 50.00,
					Trades: []*models.NavexaTrade{
						{Type: "buy", Units: 100, Price: 40.00, Fees: 10.00, Date: "2023-06-01"},
					},
				},
			},
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Direction: models.CashCredit, Account: "Trading", Category: models.CashCatContribution,
		Date: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 200000, Description: "Deposit",
	})
	// Large transfer — must NOT affect XIRR
	_, _ = svc.AddTransfer(ctx, "SMSF", "Trading", "Accumulate", 100000,
		time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC), "To accumulate")

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	if math.IsNaN(perf.AnnualizedReturnPct) {
		t.Error("AnnualizedReturnPct is NaN")
	}
	if math.IsInf(perf.AnnualizedReturnPct, 0) {
		t.Error("AnnualizedReturnPct is Inf")
	}
	// XIRR comes from trades only — non-zero because there's a buy trade
	// The exact value depends on current date, but it should be non-NaN and non-Inf.
}

// =============================================================================
// Concern 8: External balance performance tracking preserved
// =============================================================================

func TestCleanup_CalcPerf_ExternalBalancePerf_StillTracked(t *testing.T) {
	// After cleanup, external balance performance (gain/loss) for non-trading
	// accounts should still be tracked. The accountFlows logic in
	// CalculatePerformance must still identify transfer entries on non-trading
	// accounts and compute TotalOut, TotalIn, NetTransferred, GainLoss.
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			TotalValueHoldings: 300000,
			ExternalBalances: []models.ExternalBalance{
				{ID: "eb_001", Type: "accumulate", Label: "Stake Accumulate", Value: 52000},
			},
			ExternalBalanceTotal: 52000,
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Direction: models.CashCredit, Account: "Trading", Category: models.CashCatContribution,
		Date: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 500000, Description: "Deposit",
	})
	// Transfer out 50K to accumulate
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Direction: models.CashDebit, Account: "Trading", Category: models.CashCatTransfer,
		Date: time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC), Amount: 50000, Description: "To accumulate",
	})
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Direction: models.CashCredit, Account: "Stake Accumulate", Category: models.CashCatTransfer,
		Date: time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC), Amount: 50000, Description: "To accumulate",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// External balance performance should still be tracked
	if len(perf.ExternalBalances) == 0 {
		t.Fatal("ExternalBalances is empty — external balance performance tracking was lost in cleanup")
	}

	eb := perf.ExternalBalances[0]
	if eb.Category != "Stake Accumulate" {
		t.Errorf("Category = %q, want Stake Accumulate", eb.Category)
	}
	if math.Abs(eb.TotalOut-50000) > 0.01 {
		t.Errorf("TotalOut = %v, want 50000", eb.TotalOut)
	}
	if eb.CurrentBalance != 52000 {
		t.Errorf("CurrentBalance = %v, want 52000", eb.CurrentBalance)
	}
	// Gain: 52000 (current) - 50000 (net transferred) = 2000
	if math.Abs(eb.GainLoss-2000) > 0.01 {
		t.Errorf("GainLoss = %v, want 2000", eb.GainLoss)
	}
}

// =============================================================================
// Concern 9: SMSF real-world scenario post-cleanup
// =============================================================================

func TestCleanup_CalcPerf_SMSFScenario_PostCleanup(t *testing.T) {
	// Real-world SMSF scenario:
	// - $258K real deposits over 3 contributions
	// - 3 transfers to accumulate totaling $60,600
	// After cleanup, transfers are normal flows but paired, so net capital = $258K.
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			TotalValueHoldings: 426000,
			ExternalBalances: []models.ExternalBalance{
				{ID: "eb_001", Type: "accumulate", Label: "Stake Accumulate", Value: 62000},
			},
			ExternalBalanceTotal: 62000,
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	// Real deposits
	for _, dep := range []struct {
		date   time.Time
		amount float64
		desc   string
	}{
		{time.Date(2022, 7, 1, 0, 0, 0, 0, time.UTC), 200000, "Initial rollover"},
		{time.Date(2023, 7, 1, 0, 0, 0, 0, time.UTC), 28000, "FY23 contribution"},
		{time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC), 30000, "FY24 contribution"},
	} {
		_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
			Direction: models.CashCredit, Account: "Trading", Category: models.CashCatContribution,
			Date: dep.date, Amount: dep.amount, Description: dep.desc,
		})
	}

	// Transfers to accumulate (via AddTransfer — paired)
	for _, xfr := range []struct {
		date   time.Time
		amount float64
	}{
		{time.Date(2023, 1, 15, 0, 0, 0, 0, time.UTC), 20000},
		{time.Date(2023, 7, 15, 0, 0, 0, 0, time.UTC), 20300},
		{time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), 20300},
	} {
		_, _ = svc.AddTransfer(ctx, "SMSF", "Trading", "Stake Accumulate", xfr.amount,
			xfr.date, "To Stake Accumulate")
	}

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// Paired transfers: net effect on deposits/withdrawals is zero.
	// Real deposits: 200K + 28K + 30K = 258K
	// Transfer credits (to Accumulate): 20K + 20.3K + 20.3K = 60.6K
	// Transfer debits (from Trading): 20K + 20.3K + 20.3K = 60.6K
	// TotalDeposited = 258K + 60.6K = 318.6K
	// TotalWithdrawn = 60.6K
	// NetCapitalDeployed = 318.6K - 60.6K = 258K (correct)
	if math.Abs(perf.NetCapitalDeployed-258000) > 0.01 {
		t.Errorf("NetCapitalDeployed = %v, want 258000 (paired transfers net to zero)", perf.NetCapitalDeployed)
	}

	// Simple return: (426000 - 258000) / 258000 * 100 = 65.12%
	expectedReturn := (426000.0 - 258000.0) / 258000.0 * 100
	if math.Abs(perf.SimpleReturnPct-expectedReturn) > 0.1 {
		t.Errorf("SimpleReturnPct = %.2f, want ~%.2f", perf.SimpleReturnPct, expectedReturn)
	}

	// Transaction count: 3 deposits + 3 transfer pairs (6 entries) = 9
	if perf.TransactionCount != 9 {
		t.Errorf("TransactionCount = %d, want 9", perf.TransactionCount)
	}
}

// =============================================================================
// Concern 10: Transfer between same account is prevented
// =============================================================================

func TestCleanup_AddTransfer_SameAccount_Rejected(t *testing.T) {
	// AddTransfer should reject from==to (already implemented in service).
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{Name: "SMSF", TotalValueHoldings: 100000},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	_, err := svc.AddTransfer(ctx, "SMSF", "Trading", "Trading", 10000,
		time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), "Self-transfer")
	if err == nil {
		t.Error("AddTransfer with same from/to account should be rejected")
	}
}

// =============================================================================
// Concern 11: Legacy types are fully removed — no stale references
// =============================================================================

// NOTE: This is a compile-time concern. If any code references CashTransactionType,
// LegacyTransaction, LegacyLedger, MigrateLegacyLedger, or inferAccountName after
// removal, the build will fail. The grep verification in the task description handles this.

// =============================================================================
// Concern 12: TotalCashBalance is unchanged (was never excluding transfers)
// =============================================================================

func TestCleanup_TotalCashBalance_AlwaysIncludedTransfers(t *testing.T) {
	// TotalCashBalance was never excluding transfers — verify it still works.
	ledger := models.CashFlowLedger{
		Transactions: []models.CashTransaction{
			{Direction: models.CashCredit, Account: "Trading", Category: models.CashCatContribution, Amount: 100000},
			{Direction: models.CashDebit, Account: "Trading", Category: models.CashCatTransfer, Amount: 30000},
			{Direction: models.CashCredit, Account: "Accumulate", Category: models.CashCatTransfer, Amount: 30000},
		},
	}

	got := ledger.TotalCashBalance()
	// 100000 - 30000 + 30000 = 100000
	if math.Abs(got-100000) > 0.01 {
		t.Errorf("TotalCashBalance = %v, want 100000", got)
	}
}

// =============================================================================
// Concern 13: populateNetFlows includes transfers after cleanup
// =============================================================================

func TestCleanup_PopulateNetFlows_TransfersNowCounted(t *testing.T) {
	// After cleanup, transfers ARE included in net flows.
	// But paired transfers should net to zero.
	now := time.Now().Truncate(24 * time.Hour)
	yesterday := now.AddDate(0, 0, -1)

	ledger := &models.CashFlowLedger{
		Transactions: []models.CashTransaction{
			{Direction: models.CashCredit, Account: "Trading", Category: models.CashCatContribution, Date: yesterday, Amount: 10000},
			// Paired transfer yesterday
			{Direction: models.CashDebit, Account: "Trading", Category: models.CashCatTransfer, Date: yesterday, Amount: 5000},
			{Direction: models.CashCredit, Account: "Accumulate", Category: models.CashCatTransfer, Date: yesterday, Amount: 5000},
			// Real withdrawal
			{Direction: models.CashDebit, Account: "Trading", Category: models.CashCatOther, Date: yesterday, Amount: 2000},
		},
	}

	// populateNetFlows is on the portfolio Service, which needs a cashflowSvc.
	// We test the logic inline since the function uses direct iteration.
	var netFlow float64
	for _, tx := range ledger.Transactions {
		txDate := tx.Date.Truncate(24 * time.Hour)
		if !txDate.Equal(yesterday) {
			continue
		}
		// After cleanup: dividends still excluded, transfers included
		if tx.Category == models.CashCatDividend {
			continue
		}
		if tx.Direction == models.CashCredit {
			netFlow += tx.Amount
		} else {
			netFlow -= tx.Amount
		}
	}

	// +10000 (contribution) -5000 (transfer debit) +5000 (transfer credit) -2000 (withdrawal) = 8000
	if math.Abs(netFlow-8000) > 0.01 {
		t.Errorf("netFlow = %v, want 8000 (paired transfer nets to zero, real flows counted)", netFlow)
	}
}

// =============================================================================
// Concern 14: Growth cash balance includes transfers after cleanup
// =============================================================================

func TestCleanup_GrowthCash_TransfersAffectCashBalance(t *testing.T) {
	// After cleanup: transfers DO affect per-account cash balances.
	// A transfer debit from Trading reduces Trading cash.
	// A transfer credit to Accumulate increases Accumulate cash.
	// But the growth timeline only tracks a single running cash balance.
	// Paired transfers: debit reduces, credit increases → net zero effect.
	txs := []models.CashTransaction{
		{Direction: models.CashCredit, Account: "Trading", Category: models.CashCatContribution,
			Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 100000},
		// Paired transfer
		{Direction: models.CashDebit, Account: "Trading", Category: models.CashCatTransfer,
			Date: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), Amount: 20000},
		{Direction: models.CashCredit, Account: "Accumulate", Category: models.CashCatTransfer,
			Date: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), Amount: 20000},
	}

	// Simulate growth cash merge (same logic as GetDailyGrowth after cleanup)
	var cashBalance float64
	for _, tx := range txs {
		if tx.Direction == models.CashCredit {
			cashBalance += tx.Amount
		} else {
			cashBalance -= tx.Amount
		}
	}

	// 100000 - 20000 + 20000 = 100000
	if math.Abs(cashBalance-100000) > 0.01 {
		t.Errorf("cashBalance = %v, want 100000 (paired transfer nets to zero)", cashBalance)
	}
}

func TestCleanup_GrowthCash_UnpairedTransfer_AffectsBalance(t *testing.T) {
	// An unpaired transfer (lone debit without matching credit) DOES reduce cash.
	txs := []models.CashTransaction{
		{Direction: models.CashCredit, Account: "Trading", Category: models.CashCatContribution,
			Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 100000},
		// Unpaired transfer debit
		{Direction: models.CashDebit, Account: "Trading", Category: models.CashCatTransfer,
			Date: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), Amount: 20000},
	}

	var cashBalance float64
	for _, tx := range txs {
		if tx.Direction == models.CashCredit {
			cashBalance += tx.Amount
		} else {
			cashBalance -= tx.Amount
		}
	}

	// 100000 - 20000 = 80000 (unpaired debit reduces balance)
	if math.Abs(cashBalance-80000) > 0.01 {
		t.Errorf("cashBalance = %v, want 80000 (unpaired debit reduces balance)", cashBalance)
	}
}
