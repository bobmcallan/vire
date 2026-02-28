package portfolio

import (
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
)

// Stress tests for growth timeline: all transactions (including transfers)
// affect cash balance and net deployed calculations.

// --- Edge case: transfer entries affect cash balance ---

func TestGrowthCash_TransferEntries_AffectCashBalance(t *testing.T) {
	// Transfer debit reduces cash balance, paired credit adds it back
	// Net effect of paired transfer is zero on total cash
	txs := []models.CashTransaction{
		{Direction: models.CashCredit, Account: "Trading", Category: models.CashCatContribution, Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 100000},
		{Direction: models.CashDebit, Account: "Trading", Category: models.CashCatTransfer, Date: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), Amount: 20000, LinkedID: "pair1"},
		{Direction: models.CashCredit, Account: "Stake Accumulate", Category: models.CashCatTransfer, Date: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), Amount: 20000, LinkedID: "pair1_src"},
	}

	result := simulateGrowthCashMerge(txs, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 6, 30, 0, 0, 0, 0, time.UTC))

	// Paired transfer: -20K debit + 20K credit = net 0 on cash balance
	assert.Equal(t, 100000.0, result.cashBalance,
		"paired transfer nets to zero on cash balance")
	// Net deployed: contribution 100K, transfer debit -20K = 80K
	assert.Equal(t, 80000.0, result.netDeployed,
		"transfer debit reduces net deployed")
}

func TestGrowthCash_TransferCredit_AffectsCashBalance(t *testing.T) {
	// Transfer credit + debit pair: net zero on cash balance
	txs := []models.CashTransaction{
		{Direction: models.CashCredit, Account: "Trading", Category: models.CashCatContribution, Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 50000},
		{Direction: models.CashCredit, Account: "Trading", Category: models.CashCatTransfer, Date: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), Amount: 10000, LinkedID: "pair2"},
		{Direction: models.CashDebit, Account: "Cash", Category: models.CashCatTransfer, Date: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), Amount: 10000, LinkedID: "pair2_src"},
	}

	result := simulateGrowthCashMerge(txs, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 6, 30, 0, 0, 0, 0, time.UTC))

	// Paired transfer: +10K credit - 10K debit = net 0 on cash balance
	assert.Equal(t, 50000.0, result.cashBalance,
		"paired transfer nets to zero on cash balance")
	// Net deployed: contribution 50K, transfer debit -10K = 40K
	assert.Equal(t, 40000.0, result.netDeployed,
		"transfer debit reduces net deployed")
}

// --- Edge case: non-transfer debits ARE counted ---

func TestGrowthCash_OtherDebit_Counted(t *testing.T) {
	// A debit with category "other" is a real withdrawal
	txs := []models.CashTransaction{
		{Direction: models.CashCredit, Account: "Trading", Category: models.CashCatContribution, Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 100000},
		{Direction: models.CashDebit, Account: "Trading", Category: models.CashCatOther, Date: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), Amount: 20000},
	}

	result := simulateGrowthCashMerge(txs, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 6, 30, 0, 0, 0, 0, time.UTC))

	assert.Equal(t, 80000.0, result.cashBalance,
		"real debit (category other) should reduce cash balance")
}

func TestGrowthCash_FeeDebit_Counted(t *testing.T) {
	txs := []models.CashTransaction{
		{Direction: models.CashCredit, Account: "Trading", Category: models.CashCatContribution, Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 100000},
		{Direction: models.CashDebit, Account: "Trading", Category: models.CashCatFee, Date: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), Amount: 500},
	}

	result := simulateGrowthCashMerge(txs, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 6, 30, 0, 0, 0, 0, time.UTC))

	assert.Equal(t, 99500.0, result.cashBalance,
		"fee debit should reduce cash balance")
}

// --- Edge case: only transfers => cash balance reflects them ---

func TestGrowthCash_OnlyTransfers_CashBalanceReflectsFlows(t *testing.T) {
	txs := []models.CashTransaction{
		{Direction: models.CashDebit, Account: "Trading", Category: models.CashCatTransfer, Date: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), Amount: 20000},
		{Direction: models.CashCredit, Account: "Trading", Category: models.CashCatTransfer, Date: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), Amount: 10000},
	}

	result := simulateGrowthCashMerge(txs, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC))

	// Cash balance: -20000 + 10000 = -10000
	assert.Equal(t, -10000.0, result.cashBalance,
		"transfers affect cash balance: -20K + 10K = -10K")
	// Net deployed: debit -20K (transfer debits reduce net deployed)
	assert.Equal(t, -20000.0, result.netDeployed,
		"transfer debit reduces net deployed")
}

// --- Edge case: first transaction is a transfer ---

func TestGrowthCash_FirstTransactionIsTransfer(t *testing.T) {
	txs := []models.CashTransaction{
		{Direction: models.CashDebit, Account: "Trading", Category: models.CashCatTransfer, Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 30000},
		{Direction: models.CashCredit, Account: "Trading", Category: models.CashCatContribution, Date: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), Amount: 100000},
	}

	// Check at Jan 15 (after transfer debit, before contribution)
	resultJan := simulateGrowthCashMerge(txs, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC))
	assert.Equal(t, -30000.0, resultJan.cashBalance,
		"after transfer debit, cash balance = -30000")

	// Check at Mar 15 (after both transactions)
	resultMar := simulateGrowthCashMerge(txs, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 3, 31, 0, 0, 0, 0, time.UTC))
	assert.Equal(t, 70000.0, resultMar.cashBalance,
		"after contribution, cash balance = -30000 + 100000 = 70000")
}

// --- Edge case: mix of transfers and real transactions ---

func TestGrowthCash_MixedTransfersAndRealTransactions(t *testing.T) {
	txs := []models.CashTransaction{
		{Direction: models.CashCredit, Account: "Trading", Category: models.CashCatContribution, Date: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 200000},
		{Direction: models.CashDebit, Account: "Trading", Category: models.CashCatTransfer, Date: time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC), Amount: 30000}, // transfer
		{Direction: models.CashDebit, Account: "Trading", Category: models.CashCatOther, Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 25000},    // real withdrawal
		{Direction: models.CashCredit, Account: "Trading", Category: models.CashCatDividend, Date: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC), Amount: 5000}, // dividend
	}

	result := simulateGrowthCashMerge(txs, time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC))

	// Cash balance: +200000 -30000 (transfer) -25000 (withdrawal) +5000 (dividend) = 150000
	assert.Equal(t, 150000.0, result.cashBalance,
		"cash balance includes all flows (transfers, dividends, withdrawals)")
	// Net deployed: +200000 (contribution) -30000 (transfer debit) -25000 (withdrawal) = 145000
	// Dividends don't count as contributions for net deployed
	assert.Equal(t, 145000.0, result.netDeployed,
		"net deployed = contributions - all debits (except dividends)")
}

// --- Edge case: SMSF scenario — the false crash bug ---

func TestGrowthCash_SMSFScenario(t *testing.T) {
	txs := []models.CashTransaction{
		{Direction: models.CashCredit, Account: "Trading", Category: models.CashCatContribution, Date: time.Date(2022, 7, 1, 0, 0, 0, 0, time.UTC), Amount: 200000},
		{Direction: models.CashDebit, Account: "Trading", Category: models.CashCatTransfer, Date: time.Date(2023, 1, 15, 0, 0, 0, 0, time.UTC), Amount: 20000},
		{Direction: models.CashCredit, Account: "Trading", Category: models.CashCatContribution, Date: time.Date(2023, 7, 1, 0, 0, 0, 0, time.UTC), Amount: 28000},
		{Direction: models.CashDebit, Account: "Trading", Category: models.CashCatTransfer, Date: time.Date(2023, 7, 15, 0, 0, 0, 0, time.UTC), Amount: 20300},
		{Direction: models.CashDebit, Account: "Trading", Category: models.CashCatTransfer, Date: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), Amount: 20300},
		{Direction: models.CashCredit, Account: "Trading", Category: models.CashCatContribution, Date: time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC), Amount: 30000},
	}

	// After deposit (Jul 2022): 200000
	resultAfterDeposit := simulateGrowthCashMerge(txs, time.Date(2022, 7, 1, 0, 0, 0, 0, time.UTC), time.Date(2022, 12, 31, 0, 0, 0, 0, time.UTC))
	assert.Equal(t, 200000.0, resultAfterDeposit.cashBalance, "after initial contribution, cash = 200000")

	// After first transfer (Jan 2023): 200000 - 20000 = 180000
	resultAfterTransfer := simulateGrowthCashMerge(txs, time.Date(2022, 7, 1, 0, 0, 0, 0, time.UTC), time.Date(2023, 1, 31, 0, 0, 0, 0, time.UTC))
	assert.Equal(t, 180000.0, resultAfterTransfer.cashBalance, "after transfer debit, cash drops by 20000")

	// After all transactions: +200K +28K +30K -20K -20.3K -20.3K = 197400
	resultAll := simulateGrowthCashMerge(txs, time.Date(2022, 7, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC))
	assert.Equal(t, 197400.0, resultAll.cashBalance, "final cash = all credits - all debits = 197400")
	// Net deployed: contributions 258K - transfer debits 60.6K = 197400
	assert.Equal(t, 197400.0, resultAll.netDeployed, "net deployed = contributions - debits = 197400")
}

// --- Edge case: populateNetFlows includes transfers ---

func TestPopulateNetFlows_TransfersIncluded(t *testing.T) {
	now := time.Now().Truncate(24 * time.Hour)
	yesterday := now.AddDate(0, 0, -1)

	svc := &Service{
		cashflowSvc: &mockCashFlowService{
			ledger: &models.CashFlowLedger{
				Transactions: []models.CashTransaction{
					{Direction: models.CashCredit, Account: "Trading", Category: models.CashCatContribution, Date: yesterday, Amount: 10000},
					{Direction: models.CashDebit, Account: "Trading", Category: models.CashCatTransfer, Date: yesterday, Amount: 5000}, // transfer counts
					{Direction: models.CashDebit, Account: "Trading", Category: models.CashCatOther, Date: yesterday, Amount: 2000},    // real withdrawal
				},
			},
		},
	}
	portfolio := &models.Portfolio{Name: "SMSF"}
	svc.populateNetFlows(testCtx(), portfolio)

	// Net flow includes transfers: +10000 -5000 -2000 = 3000
	assert.Equal(t, 3000.0, portfolio.YesterdayNetFlow,
		"yesterday net flow should include transfer entries")
	assert.Equal(t, 3000.0, portfolio.LastWeekNetFlow,
		"last week net flow should include transfer entries")
}

// =============================================================================
// Test helper: simulate the growth.go cash merge logic
// =============================================================================

type cashMergeResult struct {
	cashBalance float64
	netDeployed float64
	txProcessed int
}

func simulateGrowthCashMerge(txs []models.CashTransaction, from, to time.Time) cashMergeResult {
	// Sort by date ascending (same as GetDailyGrowth)
	sorted := make([]models.CashTransaction, len(txs))
	copy(sorted, txs)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].Date.Before(sorted[i].Date) {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	dates := generateCalendarDates(from, to)
	txCursor := 0
	var runningCashBalance, runningNetDeployed float64

	for _, date := range dates {
		endOfDay := date.AddDate(0, 0, 1)
		for txCursor < len(sorted) && sorted[txCursor].Date.Before(endOfDay) {
			tx := sorted[txCursor]
			txCursor++
			runningCashBalance += tx.SignedAmount()
			runningNetDeployed += tx.NetDeployedImpact()
		}
	}

	return cashMergeResult{
		cashBalance: runningCashBalance,
		netDeployed: runningNetDeployed,
		txProcessed: txCursor,
	}
}
