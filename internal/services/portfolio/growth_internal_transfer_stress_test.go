package portfolio

import (
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
)

// Devils-advocate stress tests for Fix 3: growth timeline excludes internal
// transfers from cash balance calculation.
//
// After the fix, GetDailyGrowth should skip transactions where
// tx.IsInternalTransfer() == true when computing runningCashBalance
// and runningNetDeployed.

// --- Edge case: transfer_out with accumulate category excluded from cash balance ---

func TestGrowthCash_InternalTransferOut_ExcludedFromCashBalance(t *testing.T) {
	// A transfer_out to "accumulate" is internal — it should NOT reduce cash balance
	txs := []models.CashTransaction{
		{Type: models.CashTxDeposit, Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 100000},
		{Type: models.CashTxTransferOut, Date: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), Amount: 20000, Category: "accumulate"},
	}

	result := simulateGrowthCashMerge(txs, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 6, 30, 0, 0, 0, 0, time.UTC))

	assert.Equal(t, 100000.0, result.cashBalance,
		"internal transfer_out should not reduce cash balance (was 100000, stays 100000)")
	assert.Equal(t, 100000.0, result.netDeployed,
		"internal transfer_out should not affect net deployed")
}

func TestGrowthCash_InternalTransferIn_ExcludedFromCashBalance(t *testing.T) {
	// A transfer_in from "cash" external balance is internal — should NOT increase cash balance
	txs := []models.CashTransaction{
		{Type: models.CashTxDeposit, Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 50000},
		{Type: models.CashTxTransferIn, Date: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), Amount: 10000, Category: "cash"},
	}

	result := simulateGrowthCashMerge(txs, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 6, 30, 0, 0, 0, 0, time.UTC))

	assert.Equal(t, 50000.0, result.cashBalance,
		"internal transfer_in should not increase cash balance")
	assert.Equal(t, 50000.0, result.netDeployed,
		"internal transfer_in should not affect net deployed")
}

// --- Edge case: empty/unknown category transfer IS counted ---

func TestGrowthCash_TransferOut_EmptyCategory_Counted(t *testing.T) {
	// transfer_out with empty category is a real withdrawal — SHOULD reduce cash balance
	txs := []models.CashTransaction{
		{Type: models.CashTxDeposit, Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 100000},
		{Type: models.CashTxTransferOut, Date: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), Amount: 20000, Category: ""},
	}

	result := simulateGrowthCashMerge(txs, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 6, 30, 0, 0, 0, 0, time.UTC))

	assert.Equal(t, 80000.0, result.cashBalance,
		"real transfer_out (empty category) should reduce cash balance")
}

func TestGrowthCash_TransferOut_UnknownCategory_Counted(t *testing.T) {
	txs := []models.CashTransaction{
		{Type: models.CashTxDeposit, Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 100000},
		{Type: models.CashTxTransferOut, Date: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), Amount: 15000, Category: "personal"},
	}

	result := simulateGrowthCashMerge(txs, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 6, 30, 0, 0, 0, 0, time.UTC))

	assert.Equal(t, 85000.0, result.cashBalance,
		"real transfer_out (unknown category) should reduce cash balance")
}

// --- Edge case: only internal transfers => cash balance stays 0 ---

func TestGrowthCash_OnlyInternalTransfers_ZeroCashBalance(t *testing.T) {
	txs := []models.CashTransaction{
		{Type: models.CashTxTransferOut, Date: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), Amount: 20000, Category: "accumulate"},
		{Type: models.CashTxTransferIn, Date: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), Amount: 10000, Category: "offset"},
	}

	result := simulateGrowthCashMerge(txs, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC))

	assert.Equal(t, 0.0, result.cashBalance,
		"only internal transfers should leave cash balance at 0")
	assert.Equal(t, 0.0, result.netDeployed,
		"only internal transfers should leave net deployed at 0")
}

// --- Edge case: first transaction is an internal transfer ---

func TestGrowthCash_FirstTransactionIsInternal(t *testing.T) {
	// The first transaction in the timeline is an internal transfer.
	// It should be skipped, so the first data point has zero cash balance.
	txs := []models.CashTransaction{
		{Type: models.CashTxTransferOut, Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 30000, Category: "term_deposit"},
		{Type: models.CashTxDeposit, Date: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), Amount: 100000},
	}

	// Check at Jan 15 (after internal, before deposit)
	resultJan := simulateGrowthCashMerge(txs, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC))
	assert.Equal(t, 0.0, resultJan.cashBalance,
		"after internal transfer only, cash balance should be 0")

	// Check at Mar 15 (after both transactions)
	resultMar := simulateGrowthCashMerge(txs, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 3, 31, 0, 0, 0, 0, time.UTC))
	assert.Equal(t, 100000.0, resultMar.cashBalance,
		"after deposit, cash balance should reflect only the real deposit")
}

// --- Edge case: mix of internal and real transfers ---

func TestGrowthCash_MixedInternalAndRealTransfers(t *testing.T) {
	txs := []models.CashTransaction{
		{Type: models.CashTxDeposit, Date: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 200000},
		{Type: models.CashTxTransferOut, Date: time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC), Amount: 30000, Category: "accumulate"}, // internal
		{Type: models.CashTxWithdrawal, Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 25000},                          // real
		{Type: models.CashTxTransferOut, Date: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), Amount: 10000, Category: ""},           // real (empty cat)
	}

	result := simulateGrowthCashMerge(txs, time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC))

	// Cash balance: +200000 (deposit) -25000 (withdrawal) -10000 (real transfer) = 165000
	// The internal 30000 is excluded
	assert.Equal(t, 165000.0, result.cashBalance,
		"cash balance should exclude internal transfer_out (30K) but include real ones")
	// Net deployed: +200000 (deposit) -25000 (withdrawal) = 175000
	// transfer_out is not deposit/contribution/withdrawal, so doesn't affect net deployed
	assert.Equal(t, 175000.0, result.netDeployed,
		"net deployed should be deposits - withdrawals only")
}

// --- Edge case: all four external balance categories excluded ---

func TestGrowthCash_AllCategoriesExcluded(t *testing.T) {
	categories := []string{"cash", "accumulate", "term_deposit", "offset"}

	for _, cat := range categories {
		t.Run(cat, func(t *testing.T) {
			txs := []models.CashTransaction{
				{Type: models.CashTxDeposit, Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 100000},
				{Type: models.CashTxTransferOut, Date: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), Amount: 20000, Category: cat},
			}

			result := simulateGrowthCashMerge(txs, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC))

			assert.Equal(t, 100000.0, result.cashBalance,
				"category=%q transfer_out should be excluded (cash stays at 100000)", cat)
		})
	}
}

// --- Edge case: SMSF scenario — the false crash bug ---

func TestGrowthCash_SMSFScenario_NoFalseCrash(t *testing.T) {
	// From fb_2f9c18fe: when transfer_out to accumulate fires, the running
	// cash balance drops, making the chart show a false crash.
	// After the fix, internal transfers should be excluded.
	txs := []models.CashTransaction{
		{Type: models.CashTxDeposit, Date: time.Date(2022, 7, 1, 0, 0, 0, 0, time.UTC), Amount: 200000},
		{Type: models.CashTxTransferOut, Date: time.Date(2023, 1, 15, 0, 0, 0, 0, time.UTC), Amount: 20000, Category: "accumulate"},
		{Type: models.CashTxContribution, Date: time.Date(2023, 7, 1, 0, 0, 0, 0, time.UTC), Amount: 28000},
		{Type: models.CashTxTransferOut, Date: time.Date(2023, 7, 15, 0, 0, 0, 0, time.UTC), Amount: 20300, Category: "accumulate"},
		{Type: models.CashTxContribution, Date: time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC), Amount: 30000},
		{Type: models.CashTxTransferOut, Date: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), Amount: 20300, Category: "accumulate"},
	}

	// Track cash balance at key dates
	// After deposit (Jul 2022): 200000
	resultAfterDeposit := simulateGrowthCashMerge(txs, time.Date(2022, 7, 1, 0, 0, 0, 0, time.UTC), time.Date(2022, 12, 31, 0, 0, 0, 0, time.UTC))
	assert.Equal(t, 200000.0, resultAfterDeposit.cashBalance,
		"after initial deposit, cash = 200000")

	// After first internal transfer (Jan 2023): still 200000 (transfer excluded)
	resultAfterTransfer := simulateGrowthCashMerge(txs, time.Date(2022, 7, 1, 0, 0, 0, 0, time.UTC), time.Date(2023, 1, 31, 0, 0, 0, 0, time.UTC))
	assert.Equal(t, 200000.0, resultAfterTransfer.cashBalance,
		"after internal transfer, cash should NOT drop — no false crash")

	// After all transactions
	resultAll := simulateGrowthCashMerge(txs, time.Date(2022, 7, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC))
	// Real flows: +200000 +28000 +30000 = 258000
	assert.Equal(t, 258000.0, resultAll.cashBalance,
		"final cash = deposits + contributions (258000), internal transfers excluded")
	assert.Equal(t, 258000.0, resultAll.netDeployed,
		"net deployed = deposits + contributions = 258000")
}

// --- Edge case: populateNetFlows should also exclude internal transfers ---

func TestPopulateNetFlows_InternalTransfersExcluded(t *testing.T) {
	now := time.Now().Truncate(24 * time.Hour)
	yesterday := now.AddDate(0, 0, -1)

	svc := &Service{
		cashflowSvc: &mockCashFlowService{
			ledger: &models.CashFlowLedger{
				Transactions: []models.CashTransaction{
					{Type: models.CashTxDeposit, Date: yesterday, Amount: 10000},
					{Type: models.CashTxTransferOut, Date: yesterday, Amount: 5000, Category: "accumulate"}, // internal
					{Type: models.CashTxWithdrawal, Date: yesterday, Amount: 2000},                          // real
				},
			},
		},
	}
	portfolio := &models.Portfolio{Name: "SMSF"}
	svc.populateNetFlows(testCtx(), portfolio)

	// Net flow should exclude internal transfer:
	// +10000 (deposit) -2000 (withdrawal) = 8000
	// The 5000 internal transfer_out is excluded
	assert.Equal(t, 8000.0, portfolio.YesterdayNetFlow,
		"yesterday net flow should exclude internal transfer_out")
	assert.Equal(t, 8000.0, portfolio.LastWeekNetFlow,
		"last week net flow should exclude internal transfer_out")
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

			// FIX 3: Skip internal transfers
			if tx.IsInternalTransfer() {
				continue
			}

			if models.IsInflowType(tx.Type) {
				runningCashBalance += tx.Amount
			} else {
				runningCashBalance -= tx.Amount
			}
			switch tx.Type {
			case models.CashTxDeposit, models.CashTxContribution:
				runningNetDeployed += tx.Amount
			case models.CashTxWithdrawal:
				runningNetDeployed -= tx.Amount
			}
		}
	}

	return cashMergeResult{
		cashBalance: runningCashBalance,
		netDeployed: runningNetDeployed,
		txProcessed: txCursor,
	}
}
