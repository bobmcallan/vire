package cashflow

import (
	"math"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
)

// Devils-advocate stress tests for TotalDeposited/TotalWithdrawn category filtering.
// After the fix, only category=contribution transactions count as deposits/withdrawals.
// Transfers, dividends, fees, and other categories are excluded.

var baseDate = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

func ledgerWith(txs ...models.CashTransaction) *models.CashFlowLedger {
	return &models.CashFlowLedger{
		PortfolioName: "SMSF",
		Accounts:      []models.CashAccount{{Name: "Trading", Type: "trading", IsTransactional: true}},
		Transactions:  txs,
	}
}

func tx(cat models.CashCategory, amount float64) models.CashTransaction {
	return models.CashTransaction{
		Account:     "Trading",
		Category:    cat,
		Date:        baseDate,
		Amount:      amount,
		Description: string(cat),
	}
}

// --- 1. Ledger with ONLY transfers (no contributions) → both should be 0 ---

func TestCategoryFilter_OnlyTransfers_BothZero(t *testing.T) {
	l := ledgerWith(
		tx(models.CashCatTransfer, 10000),  // transfer credit
		tx(models.CashCatTransfer, -10000), // transfer debit
		tx(models.CashCatTransfer, 5000),   // another credit
		tx(models.CashCatTransfer, -3000),  // another debit
	)
	if d := l.TotalDeposited(); d != 0 {
		t.Errorf("TotalDeposited with only transfers = %v, want 0", d)
	}
	if w := l.TotalWithdrawn(); w != 0 {
		t.Errorf("TotalWithdrawn with only transfers = %v, want 0", w)
	}
}

// --- 2. Ledger with ONLY dividends → both should be 0 ---

func TestCategoryFilter_OnlyDividends_BothZero(t *testing.T) {
	l := ledgerWith(
		tx(models.CashCatDividend, 1500),
		tx(models.CashCatDividend, 2300),
		tx(models.CashCatDividend, 750),
	)
	if d := l.TotalDeposited(); d != 0 {
		t.Errorf("TotalDeposited with only dividends = %v, want 0", d)
	}
	if w := l.TotalWithdrawn(); w != 0 {
		t.Errorf("TotalWithdrawn with only dividends = %v, want 0", w)
	}
}

// --- 3. Ledger with ONLY fees → both should be 0 ---

func TestCategoryFilter_OnlyFees_BothZero(t *testing.T) {
	l := ledgerWith(
		tx(models.CashCatFee, -150),  // management fee
		tx(models.CashCatFee, -75),   // brokerage
		tx(models.CashCatFee, -1200), // annual admin fee
	)
	if d := l.TotalDeposited(); d != 0 {
		t.Errorf("TotalDeposited with only fees = %v, want 0", d)
	}
	if w := l.TotalWithdrawn(); w != 0 {
		t.Errorf("TotalWithdrawn with only fees = %v, want 0", w)
	}
}

// --- 4. Mixed categories — verify only contributions counted ---

func TestCategoryFilter_MixedCategories_OnlyContributionsCounted(t *testing.T) {
	l := ledgerWith(
		tx(models.CashCatContribution, 50000),  // deposit — counts
		tx(models.CashCatContribution, -10000), // withdrawal — counts
		tx(models.CashCatDividend, 3000),       // dividend — excluded
		tx(models.CashCatTransfer, 20000),      // transfer credit — excluded
		tx(models.CashCatTransfer, -20000),     // transfer debit — excluded
		tx(models.CashCatFee, -500),            // fee — excluded
		tx(models.CashCatOther, 1000),          // other credit — excluded
		tx(models.CashCatOther, -200),          // other debit — excluded
	)
	if d := l.TotalDeposited(); d != 50000 {
		t.Errorf("TotalDeposited = %v, want 50000 (only contribution deposit)", d)
	}
	if w := l.TotalWithdrawn(); w != 10000 {
		t.Errorf("TotalWithdrawn = %v, want 10000 (only contribution withdrawal)", w)
	}
}

// --- 5. Zero-amount contributions → should not count ---

func TestCategoryFilter_ZeroAmountContributions_NotCounted(t *testing.T) {
	l := ledgerWith(
		tx(models.CashCatContribution, 0), // zero — neither > 0 nor < 0
		tx(models.CashCatContribution, 0),
		tx(models.CashCatContribution, 0),
	)
	if d := l.TotalDeposited(); d != 0 {
		t.Errorf("TotalDeposited with zero contributions = %v, want 0", d)
	}
	if w := l.TotalWithdrawn(); w != 0 {
		t.Errorf("TotalWithdrawn with zero contributions = %v, want 0", w)
	}
}

// --- 6. Very large amounts (overflow check) ---

func TestCategoryFilter_VeryLargeAmounts_NoOverflow(t *testing.T) {
	// Two deposits near float64 significant range but not near MaxFloat64
	large := 1e15 // 1 quadrillion
	l := ledgerWith(
		tx(models.CashCatContribution, large),
		tx(models.CashCatContribution, large),
		tx(models.CashCatContribution, -large), // withdrawal
		tx(models.CashCatDividend, large),      // excluded
	)
	wantDeposited := 2 * large
	if d := l.TotalDeposited(); d != wantDeposited {
		t.Errorf("TotalDeposited = %v, want %v", d, wantDeposited)
	}
	if w := l.TotalWithdrawn(); w != large {
		t.Errorf("TotalWithdrawn = %v, want %v", w, large)
	}
}

// --- 7. Many small contributions (precision check) ---

func TestCategoryFilter_ManySmallContributions_Precision(t *testing.T) {
	var txs []models.CashTransaction
	// 10000 deposits of 0.01 — tests float64 summation precision
	for i := 0; i < 10000; i++ {
		txs = append(txs, tx(models.CashCatContribution, 0.01))
	}
	// Also add non-contribution noise that should be excluded
	for i := 0; i < 1000; i++ {
		txs = append(txs, tx(models.CashCatDividend, 0.01))
		txs = append(txs, tx(models.CashCatTransfer, 0.01))
	}
	l := ledgerWith(txs...)

	// 10000 * 0.01 = 100.0 (with float tolerance)
	if d := l.TotalDeposited(); math.Abs(d-100.0) > 0.01 {
		t.Errorf("TotalDeposited = %v, want ~100.0 (precision)", d)
	}
	if w := l.TotalWithdrawn(); w != 0 {
		t.Errorf("TotalWithdrawn = %v, want 0 (no negative contributions)", w)
	}
}

// --- 8. Empty ledger → both should be 0 ---

func TestCategoryFilter_EmptyLedger_BothZero(t *testing.T) {
	l := ledgerWith() // no transactions
	if d := l.TotalDeposited(); d != 0 {
		t.Errorf("TotalDeposited on empty ledger = %v, want 0", d)
	}
	if w := l.TotalWithdrawn(); w != 0 {
		t.Errorf("TotalWithdrawn on empty ledger = %v, want 0", w)
	}
}

// --- 9. Single contribution deposit ---

func TestCategoryFilter_SingleDeposit(t *testing.T) {
	l := ledgerWith(tx(models.CashCatContribution, 25000))
	if d := l.TotalDeposited(); d != 25000 {
		t.Errorf("TotalDeposited = %v, want 25000", d)
	}
	if w := l.TotalWithdrawn(); w != 0 {
		t.Errorf("TotalWithdrawn = %v, want 0", w)
	}
}

// --- 10. Single contribution withdrawal ---

func TestCategoryFilter_SingleWithdrawal(t *testing.T) {
	l := ledgerWith(tx(models.CashCatContribution, -15000))
	if d := l.TotalDeposited(); d != 0 {
		t.Errorf("TotalDeposited = %v, want 0", d)
	}
	if w := l.TotalWithdrawn(); w != 15000 {
		t.Errorf("TotalWithdrawn = %v, want 15000", w)
	}
}

// --- 11. Transfer credit + contribution deposit → only contribution counts ---

func TestCategoryFilter_TransferCreditPlusContribution(t *testing.T) {
	l := ledgerWith(
		tx(models.CashCatTransfer, 20000),     // transfer credit — excluded
		tx(models.CashCatContribution, 50000), // contribution deposit — counts
	)
	if d := l.TotalDeposited(); d != 50000 {
		t.Errorf("TotalDeposited = %v, want 50000 (transfer credit excluded)", d)
	}
	if w := l.TotalWithdrawn(); w != 0 {
		t.Errorf("TotalWithdrawn = %v, want 0", w)
	}
}

// --- 12. Dividend + fee + contribution → only contribution counts ---

func TestCategoryFilter_DividendFeePlusContribution(t *testing.T) {
	l := ledgerWith(
		tx(models.CashCatDividend, 5000),       // dividend — excluded
		tx(models.CashCatFee, -300),            // fee — excluded
		tx(models.CashCatContribution, 100000), // deposit — counts
		tx(models.CashCatContribution, -25000), // withdrawal — counts
	)
	if d := l.TotalDeposited(); d != 100000 {
		t.Errorf("TotalDeposited = %v, want 100000", d)
	}
	if w := l.TotalWithdrawn(); w != 25000 {
		t.Errorf("TotalWithdrawn = %v, want 25000", w)
	}
}

// --- 13. Multiple categories with same amounts → verify filtering ---

func TestCategoryFilter_SameAmountsDifferentCategories(t *testing.T) {
	// Each category has an identical +10000 credit
	l := ledgerWith(
		tx(models.CashCatContribution, 10000),
		tx(models.CashCatDividend, 10000),
		tx(models.CashCatTransfer, 10000),
		tx(models.CashCatFee, 10000), // unusual but possible (fee reversal)
		tx(models.CashCatOther, 10000),
	)
	// Only the contribution counts
	if d := l.TotalDeposited(); d != 10000 {
		t.Errorf("TotalDeposited = %v, want 10000 (only contribution)", d)
	}
	// Same for debits
	l2 := ledgerWith(
		tx(models.CashCatContribution, -10000),
		tx(models.CashCatDividend, -10000),
		tx(models.CashCatTransfer, -10000),
		tx(models.CashCatFee, -10000),
		tx(models.CashCatOther, -10000),
	)
	if w := l2.TotalWithdrawn(); w != 10000 {
		t.Errorf("TotalWithdrawn = %v, want 10000 (only contribution)", w)
	}
}

// --- 14. Negative contribution (withdrawal) counted in TotalWithdrawn, not TotalDeposited ---

func TestCategoryFilter_NegativeContribution_IsWithdrawal(t *testing.T) {
	l := ledgerWith(
		tx(models.CashCatContribution, 100000),
		tx(models.CashCatContribution, -30000), // withdrawal of capital
	)
	if d := l.TotalDeposited(); d != 100000 {
		t.Errorf("TotalDeposited = %v, want 100000 (only positive contributions)", d)
	}
	if w := l.TotalWithdrawn(); w != 30000 {
		t.Errorf("TotalWithdrawn = %v, want 30000 (absolute value of negative contribution)", w)
	}
}

// --- 15. NetDeployedImpact consistency for contribution-only ledger ---

func TestCategoryFilter_NetDeployedImpact_ConsistencyWithContributionsOnly(t *testing.T) {
	// For a ledger with ONLY contributions, the sum of NetDeployedImpact
	// should equal TotalDeposited - TotalWithdrawn (since contributions with
	// positive amount have NetDeployedImpact = amount, but negative contributions
	// have NetDeployedImpact = 0 per the implementation).
	// Wait — let's verify the actual behavior of NetDeployedImpact for negative contributions.
	//
	// NetDeployedImpact for CashCatContribution: only counts Amount > 0.
	// So for contribution-only ledgers:
	//   sum(NetDeployedImpact) = TotalDeposited (not TotalDeposited - TotalWithdrawn)
	// This is actually correct: withdrawals of capital reduce deployment differently.
	l := ledgerWith(
		tx(models.CashCatContribution, 50000),
		tx(models.CashCatContribution, 30000),
		tx(models.CashCatContribution, -10000),
	)

	var sumNDI float64
	for _, t := range l.Transactions {
		sumNDI += t.NetDeployedImpact()
	}

	deposited := l.TotalDeposited()
	withdrawn := l.TotalWithdrawn()

	// For contribution-only: sum of NDI = deposited only (negative contributions have NDI=0)
	if sumNDI != deposited {
		t.Errorf("sum(NetDeployedImpact) = %v, TotalDeposited = %v — expected equal for contribution-only ledger", sumNDI, deposited)
	}

	// Verify NetCapitalDeployed = TotalDeposited - TotalWithdrawn
	netCapital := deposited - withdrawn
	if netCapital != 70000 {
		t.Errorf("NetCapitalDeployed = %v, want 70000 (80000 deposited - 10000 withdrawn)", netCapital)
	}
}

// --- 16. All categories mixed — end-to-end via CalculatePerformance ---

func TestCalculatePerformance_CategoryFiltering_EndToEnd(t *testing.T) {
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:                 "SMSF",
			TotalValueHoldings:   120000,
			ExternalBalanceTotal: 0,
			TotalValue:           120000,
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	// Add mixed transactions
	txns := []models.CashTransaction{
		{Account: "Trading", Category: models.CashCatContribution, Date: baseDate, Amount: 100000, Description: "Initial deposit"},
		{Account: "Trading", Category: models.CashCatDividend, Date: baseDate.AddDate(0, 3, 0), Amount: 3000, Description: "BHP div"},
		{Account: "Trading", Category: models.CashCatTransfer, Date: baseDate.AddDate(0, 3, 0), Amount: 20000, Description: "Transfer from savings"},
		{Account: "Trading", Category: models.CashCatTransfer, Date: baseDate.AddDate(0, 3, 0), Amount: -20000, Description: "Transfer to term deposit"},
		{Account: "Trading", Category: models.CashCatFee, Date: baseDate.AddDate(0, 6, 0), Amount: -1500, Description: "Admin fee"},
		{Account: "Trading", Category: models.CashCatContribution, Date: baseDate.AddDate(0, 6, 0), Amount: 25000, Description: "Top up"},
		{Account: "Trading", Category: models.CashCatContribution, Date: baseDate.AddDate(0, 9, 0), Amount: -15000, Description: "Partial withdrawal"},
	}

	for _, txn := range txns {
		if _, err := svc.AddTransaction(ctx, "SMSF", txn); err != nil {
			t.Fatalf("AddTransaction(%s): %v", txn.Description, err)
		}
	}

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// Only contributions count:
	// Deposits: 100000 + 25000 = 125000
	// Withdrawals: |−15000| = 15000
	// Net capital deployed: 125000 - 15000 = 110000
	if perf.TotalDeposited != 125000 {
		t.Errorf("TotalDeposited = %v, want 125000 (only contributions)", perf.TotalDeposited)
	}
	if perf.TotalWithdrawn != 15000 {
		t.Errorf("TotalWithdrawn = %v, want 15000 (only contribution withdrawal)", perf.TotalWithdrawn)
	}
	if perf.NetCapitalDeployed != 110000 {
		t.Errorf("NetCapitalDeployed = %v, want 110000", perf.NetCapitalDeployed)
	}

	// Simple return: (120000 - 110000) / 110000 * 100 ≈ 9.09%
	expectedReturn := (120000.0 - 110000.0) / 110000.0 * 100
	if math.Abs(perf.SimpleReturnPct-expectedReturn) > 0.01 {
		t.Errorf("SimpleReturnPct = %v, want ~%v", perf.SimpleReturnPct, expectedReturn)
	}

	// Transaction count includes ALL transactions, not just contributions
	if perf.TransactionCount != 7 {
		t.Errorf("TransactionCount = %v, want 7 (all transactions)", perf.TransactionCount)
	}
}

// --- 17. Dividends NOT counted as deposits via CalculatePerformance ---

func TestCalculatePerformance_DividendsNotCountedAsDeposit(t *testing.T) {
	// Regression test: before the fix, dividends inflated TotalDeposited
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			TotalValueHoldings: 85000,
			TotalValue:         85000,
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account: "Trading", Category: models.CashCatContribution,
		Date: baseDate, Amount: 80000, Description: "Initial deposit",
	})
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account: "Trading", Category: models.CashCatDividend,
		Date: baseDate.AddDate(0, 6, 0), Amount: 5000, Description: "BHP div",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// Dividend is NOT a deposit — only the 80000 contribution counts
	if perf.TotalDeposited != 80000 {
		t.Errorf("TotalDeposited = %v, want 80000 (dividend excluded)", perf.TotalDeposited)
	}
	if perf.TotalWithdrawn != 0 {
		t.Errorf("TotalWithdrawn = %v, want 0", perf.TotalWithdrawn)
	}
	if perf.NetCapitalDeployed != 80000 {
		t.Errorf("NetCapitalDeployed = %v, want 80000", perf.NetCapitalDeployed)
	}
}

// --- 18. Transfer credits NOT counted as deposits via CalculatePerformance ---

func TestCalculatePerformance_TransferCreditsNotCountedAsDeposit(t *testing.T) {
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			TotalValueHoldings: 100000,
			TotalValue:         100000,
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account: "Trading", Category: models.CashCatContribution,
		Date: baseDate, Amount: 100000, Description: "Deposit",
	})
	// Internal transfer: move money between accounts (net zero, not capital)
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account: "Trading", Category: models.CashCatTransfer,
		Date: baseDate.AddDate(0, 1, 0), Amount: -30000, Description: "Transfer to term deposit",
	})
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account: "Term Deposit", Category: models.CashCatTransfer,
		Date: baseDate.AddDate(0, 1, 0), Amount: 30000, Description: "Transfer from trading",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// Only the contribution counts — transfers are internal movement
	if perf.TotalDeposited != 100000 {
		t.Errorf("TotalDeposited = %v, want 100000 (transfer credits excluded)", perf.TotalDeposited)
	}
	if perf.TotalWithdrawn != 0 {
		t.Errorf("TotalWithdrawn = %v, want 0 (transfer debits excluded)", perf.TotalWithdrawn)
	}
}

// --- 19. Fees NOT counted as withdrawals via CalculatePerformance ---

func TestCalculatePerformance_FeesNotCountedAsWithdrawal(t *testing.T) {
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			TotalValueHoldings: 98000,
			TotalValue:         98000,
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account: "Trading", Category: models.CashCatContribution,
		Date: baseDate, Amount: 100000, Description: "Deposit",
	})
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account: "Trading", Category: models.CashCatFee,
		Date: baseDate.AddDate(0, 6, 0), Amount: -2000, Description: "Management fee",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	if perf.TotalDeposited != 100000 {
		t.Errorf("TotalDeposited = %v, want 100000", perf.TotalDeposited)
	}
	// Fee is NOT a withdrawal of capital
	if perf.TotalWithdrawn != 0 {
		t.Errorf("TotalWithdrawn = %v, want 0 (fees excluded)", perf.TotalWithdrawn)
	}
	if perf.NetCapitalDeployed != 100000 {
		t.Errorf("NetCapitalDeployed = %v, want 100000", perf.NetCapitalDeployed)
	}
}

// --- 20. "Other" category NOT counted ---

func TestCategoryFilter_OtherCategory_Excluded(t *testing.T) {
	l := ledgerWith(
		tx(models.CashCatOther, 5000),  // other credit
		tx(models.CashCatOther, -2000), // other debit
	)
	if d := l.TotalDeposited(); d != 0 {
		t.Errorf("TotalDeposited with only 'other' = %v, want 0", d)
	}
	if w := l.TotalWithdrawn(); w != 0 {
		t.Errorf("TotalWithdrawn with only 'other' = %v, want 0", w)
	}
}
