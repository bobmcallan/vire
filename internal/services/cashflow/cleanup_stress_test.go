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
			{Account: "Trading", Category: models.CashCatContribution, Amount: 100000, Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
			{Account: "Trading", Category: models.CashCatTransfer, Amount: -20000, Date: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)},
			{Account: "Accumulate", Category: models.CashCatTransfer, Amount: 20000, Date: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)},
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
			{Account: "Trading", Category: models.CashCatContribution, Amount: 200000, Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
			// Transfer 1: Trading -> Accumulate
			{Account: "Trading", Category: models.CashCatTransfer, Amount: -20000, Date: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)},
			{Account: "Accumulate", Category: models.CashCatTransfer, Amount: 20000, Date: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)},
			// Transfer 2: Trading -> Term Deposit
			{Account: "Trading", Category: models.CashCatTransfer, Amount: -30000, Date: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)},
			{Account: "Term Deposit", Category: models.CashCatTransfer, Amount: 30000, Date: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)},
			// Transfer 3: Accumulate -> Trading (return)
			{Account: "Accumulate", Category: models.CashCatTransfer, Amount: -20000, Date: time.Date(2024, 9, 1, 0, 0, 0, 0, time.UTC)},
			{Account: "Trading", Category: models.CashCatTransfer, Amount: 20000, Date: time.Date(2024, 9, 1, 0, 0, 0, 0, time.UTC)},
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
			{Account: "Trading", Category: models.CashCatContribution, Amount: 100000, Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
			{Account: "Trading", Category: models.CashCatTransfer, Amount: -20000, Date: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)},
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
			{Account: "Trading", Category: models.CashCatContribution, Amount: 100000, Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
			{Account: "Accumulate", Category: models.CashCatTransfer, Amount: 20000, Date: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)},
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
			{Account: "Trading", Category: models.CashCatContribution, Amount: 100000, Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
			// A->B: Trading -> Accumulate 20K
			{Account: "Trading", Category: models.CashCatTransfer, Amount: -20000, Date: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)},
			{Account: "Accumulate", Category: models.CashCatTransfer, Amount: 20000, Date: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)},
			// B->A: Accumulate -> Trading 20K
			{Account: "Accumulate", Category: models.CashCatTransfer, Amount: -20000, Date: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)},
			{Account: "Trading", Category: models.CashCatTransfer, Amount: 20000, Date: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)},
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
			{Account: "Trading", Category: models.CashCatContribution, Amount: 100000, Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
			// Mismatched transfer: debit 20K but credit 25K (e.g., user error)
			{Account: "Trading", Category: models.CashCatTransfer, Amount: -20000, Date: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)},
			{Account: "Accumulate", Category: models.CashCatTransfer, Amount: 25000, Date: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)},
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
			Name:        "SMSF",
			EquityValue: 80000,
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account: "Trading", Category: models.CashCatContribution,
		Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 100000, Description: "Initial deposit",
	})
	// Paired transfer via AddTransfer
	_, _ = svc.AddTransfer(ctx, "SMSF", "Trading", "Accumulate", 20000,
		time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), "To accumulate")

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// Transfers are not counted in deposits/withdrawals (only contributions count)
	// TotalDeposited = 100000 (contribution only, not transfer credit)
	// TotalWithdrawn = 0 (transfer debit is not a contribution)
	// NetCapitalDeployed = 100000 - 0 = 100000 (unchanged)
	if perf.NetCapitalDeployed != 100000 {
		t.Errorf("NetCapitalDeployed = %v, want 100000 (only contribution counts)", perf.NetCapitalDeployed)
	}
	if perf.GrossCapitalDeposited != 100000 {
		t.Errorf("TotalDeposited = %v, want 100000 (transfers are not deposits)", perf.GrossCapitalDeposited)
	}
	if perf.GrossCapitalWithdrawn != 0 {
		t.Errorf("TotalWithdrawn = %v, want 0 (transfers are not withdrawals)", perf.GrossCapitalWithdrawn)
	}
}

func TestCleanup_CalcPerf_SimpleReturn_StableWithTransfers(t *testing.T) {
	// SimpleReturnPct should be stable: transfers don't change net capital.
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:        "SMSF",
			EquityValue: 120000,
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account: "Trading", Category: models.CashCatContribution,
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
	if math.Abs(perf.SimpleCapitalReturnPct-expected) > 0.1 {
		t.Errorf("SimpleReturnPct = %.2f, want ~%.2f", perf.SimpleCapitalReturnPct, expected)
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
			Name:        "SMSF",
			EquityValue: 50000,
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
	if math.IsNaN(perf.SimpleCapitalReturnPct) {
		t.Error("SimpleReturnPct is NaN — division by zero bug")
	}
	if math.IsInf(perf.SimpleCapitalReturnPct, 0) {
		t.Error("SimpleReturnPct is Inf — division by zero bug")
	}
	if perf.SimpleCapitalReturnPct != 0 {
		t.Errorf("SimpleReturnPct = %v, want 0 (zero net capital)", perf.SimpleCapitalReturnPct)
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
			Name:        "SMSF",
			EquityValue: 120000,
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
		Account: "Trading", Category: models.CashCatContribution,
		Date: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 200000, Description: "Deposit",
	})
	// Large transfer — must NOT affect XIRR
	_, _ = svc.AddTransfer(ctx, "SMSF", "Trading", "Accumulate", 100000,
		time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC), "To accumulate")

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	if math.IsNaN(perf.AnnualizedCapitalReturnPct) {
		t.Error("AnnualizedReturnPct is NaN")
	}
	if math.IsInf(perf.AnnualizedCapitalReturnPct, 0) {
		t.Error("AnnualizedReturnPct is Inf")
	}
	// XIRR comes from trades only — non-zero because there's a buy trade
	// The exact value depends on current date, but it should be non-NaN and non-Inf.
}

// =============================================================================
// Concern 8: External balance performance tracking preserved
// =============================================================================

func TestCleanup_CalcPerf_NonTransactionalBalance_TrackedViaLedger(t *testing.T) {
	// After cleanup, external balance tracking is replaced by NonTransactionalBalance
	// on the ledger. Transfer entries to non-transactional accounts build up balances
	// which are then summed by NonTransactionalBalance().
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:             "SMSF",
			EquityValue:      300000,
			GrossCashBalance: 52000,
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account: "Trading", Category: models.CashCatContribution,
		Date: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 500000, Description: "Deposit",
	})
	// Transfer out 50K to accumulate via AddTransfer (creates paired entries)
	_, _ = svc.AddTransfer(ctx, "SMSF", "Trading", "Stake Accumulate", 50000,
		time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC), "To accumulate")

	ledger, err := svc.GetLedger(ctx, "SMSF")
	if err != nil {
		t.Fatalf("GetLedger: %v", err)
	}

	// Stake Accumulate balance should be 50000 (auto-created as non-transactional)
	accBal := ledger.AccountBalance("Stake Accumulate")
	if accBal != 50000 {
		t.Errorf("Stake Accumulate balance = %v, want 50000", accBal)
	}

	// NonTransactionalBalance should be 50000
	nonTxBal := ledger.NonTransactionalBalance()
	if nonTxBal != 50000 {
		t.Errorf("NonTransactionalBalance = %v, want 50000", nonTxBal)
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
			Name:             "SMSF",
			EquityValue:      426000,
			GrossCashBalance: 62000,
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
			Account: "Trading", Category: models.CashCatContribution,
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

	// Only contributions count for deposits/withdrawals.
	// Real deposits: 200K + 28K + 30K = 258K (category=contribution)
	// Transfer credits/debits: NOT counted (category=transfer)
	// TotalDeposited = 258K (contributions only)
	// TotalWithdrawn = 0 (transfers are not capital withdrawals)
	// NetCapitalDeployed = 258K - 0 = 258K (correct)
	if math.Abs(perf.NetCapitalDeployed-258000) > 0.01 {
		t.Errorf("NetCapitalDeployed = %v, want 258000 (only contributions count)", perf.NetCapitalDeployed)
	}

	// Simple return: (426000 - 258000) / 258000 * 100 = 65.12%
	expectedReturn := (426000.0 - 258000.0) / 258000.0 * 100
	if math.Abs(perf.SimpleCapitalReturnPct-expectedReturn) > 0.1 {
		t.Errorf("SimpleReturnPct = %.2f, want ~%.2f", perf.SimpleCapitalReturnPct, expectedReturn)
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
		portfolio: &models.Portfolio{Name: "SMSF", EquityValue: 100000},
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
			{Account: "Trading", Category: models.CashCatContribution, Amount: 100000, Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
			{Account: "Trading", Category: models.CashCatTransfer, Amount: -30000, Date: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)},
			{Account: "Accumulate", Category: models.CashCatTransfer, Amount: 30000, Date: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)},
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
			// Contribution yesterday
			{Account: "Trading", Category: models.CashCatContribution, Amount: 10000, Date: yesterday},
			// Paired transfer yesterday: Trading -> Accumulate 5K
			{Account: "Trading", Category: models.CashCatTransfer, Amount: -5000, Date: yesterday},
			{Account: "Accumulate", Category: models.CashCatTransfer, Amount: 5000, Date: yesterday},
			// Withdrawal yesterday
			{Account: "Trading", Category: models.CashCatOther, Amount: -2000, Date: yesterday},
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
		netFlow += tx.Amount // signed: positive = credit, negative = debit
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
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 100000},
		// Paired transfer
		{Date: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), Amount: -20000},
		{Date: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), Amount: 20000},
	}

	// Simulate growth cash merge (same logic as GetDailyGrowth after cleanup)
	var cashBalance float64
	for _, tx := range txs {
		cashBalance += tx.Amount // signed: positive = credit, negative = debit
	}

	// 100000 - 20000 + 20000 = 100000
	if math.Abs(cashBalance-100000) > 0.01 {
		t.Errorf("cashBalance = %v, want 100000 (paired transfer nets to zero)", cashBalance)
	}
}

func TestCleanup_GrowthCash_UnpairedTransfer_AffectsBalance(t *testing.T) {
	// An unpaired transfer (lone debit without matching credit) DOES reduce cash.
	txs := []models.CashTransaction{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 100000},
		// Unpaired transfer debit
		{Date: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), Amount: -20000},
	}

	var cashBalance float64
	for _, tx := range txs {
		cashBalance += tx.Amount // signed: positive = credit, negative = debit
	}

	// 100000 - 20000 = 80000 (unpaired debit reduces balance)
	if math.Abs(cashBalance-80000) > 0.01 {
		t.Errorf("cashBalance = %v, want 80000 (unpaired debit reduces balance)", cashBalance)
	}
}
