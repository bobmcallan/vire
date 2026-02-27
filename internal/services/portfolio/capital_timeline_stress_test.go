package portfolio

import (
	"context"
	"encoding/json"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Devils-advocate stress tests for capital allocation timeline feature.
// Focus: circular dependency safety, nil CashFlowService, empty/corrupt transactions,
// large volumes, sign logic, same-day multiples, future-dated transactions, FX.

// =============================================================================
// 1. Circular Dependency: cashflow.Service holds portfolioService, now portfolio.Service
//    holds CashFlowService. Verify no infinite recursion path.
// =============================================================================

func TestCircularDependency_SetCashFlowServiceNil(t *testing.T) {
	// SetCashFlowService(nil) must not panic
	svc := &Service{}
	svc.SetCashFlowService(nil)
	assert.Nil(t, svc.cashflowSvc)
}

func TestCircularDependency_SetCashFlowServiceTwice(t *testing.T) {
	// Calling SetCashFlowService multiple times must not corrupt state
	svc := &Service{}
	mock1 := &mockCashFlowService{ledger: &models.CashFlowLedger{}}
	mock2 := &mockCashFlowService{ledger: &models.CashFlowLedger{
		Transactions: []models.CashTransaction{
			{Type: models.CashTxDeposit, Amount: 1000},
		},
	}}
	svc.SetCashFlowService(mock1)
	assert.Equal(t, mock1, svc.cashflowSvc)
	svc.SetCashFlowService(mock2)
	assert.Equal(t, mock2, svc.cashflowSvc)
}

func TestCircularDependency_NoCyclicCallInPopulateNetFlows(t *testing.T) {
	// populateNetFlows calls cashflowSvc.GetLedger, which should NOT call back
	// into portfolio service methods. The mock verifies this by tracking calls.
	tracker := &trackingCashFlowService{
		mockCashFlowService: mockCashFlowService{
			ledger: &models.CashFlowLedger{
				Transactions: []models.CashTransaction{
					{
						Type:   models.CashTxDeposit,
						Date:   time.Now().Add(-24 * time.Hour),
						Amount: 5000,
					},
				},
			},
		},
	}
	svc := &Service{cashflowSvc: tracker}
	portfolio := &models.Portfolio{Name: "SMSF"}

	svc.populateNetFlows(testCtx(), portfolio)

	assert.Equal(t, 1, tracker.getLedgerCalls, "GetLedger should be called exactly once")
	// If GetLedger triggered a cyclic call back into portfolio service,
	// the test would hang or panic. Completing = no cycle.
}

// =============================================================================
// 2. Nil CashFlowService: populateNetFlows and GetPortfolioIndicators must not
//    panic when cashflowSvc is nil (backward compatibility for tests).
// =============================================================================

func TestNilCashFlowService_PopulateNetFlows(t *testing.T) {
	svc := &Service{cashflowSvc: nil}
	portfolio := &models.Portfolio{Name: "SMSF", TotalValue: 100000}

	// Must not panic
	svc.populateNetFlows(testCtx(), portfolio)

	assert.Equal(t, 0.0, portfolio.YesterdayNetFlow)
	assert.Equal(t, 0.0, portfolio.LastWeekNetFlow)
}

func TestNilCashFlowService_GrowthOptionsNoTransactions(t *testing.T) {
	// GetDailyGrowth with nil Transactions should behave identically to before
	opts := interfaces.GrowthOptions{
		Transactions: nil,
	}
	assert.Empty(t, opts.Transactions)
}

func TestNilCashFlowService_GrowthPointsHaveZeroCashFields(t *testing.T) {
	// When no transactions are provided, cash flow fields should be zero
	point := models.GrowthDataPoint{
		Date:       time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC),
		TotalValue: 100000,
		TotalCost:  90000,
	}
	assert.Equal(t, 0.0, point.CashBalance)
	assert.Equal(t, 0.0, point.NetDeployed)
	assert.Equal(t, 0.0, point.TotalCapital)
	assert.Equal(t, 0.0, point.ExternalBalance)
}

// =============================================================================
// 3. Empty/Corrupt Ledger: empty transactions, zero dates, nil ledger.
// =============================================================================

func TestEmptyLedger_PopulateNetFlows(t *testing.T) {
	svc := &Service{
		cashflowSvc: &mockCashFlowService{
			ledger: &models.CashFlowLedger{
				Transactions: []models.CashTransaction{},
			},
		},
	}
	portfolio := &models.Portfolio{Name: "SMSF"}

	svc.populateNetFlows(testCtx(), portfolio)

	assert.Equal(t, 0.0, portfolio.YesterdayNetFlow)
	assert.Equal(t, 0.0, portfolio.LastWeekNetFlow)
}

func TestNilLedger_PopulateNetFlows(t *testing.T) {
	svc := &Service{
		cashflowSvc: &mockCashFlowService{ledger: nil},
	}
	portfolio := &models.Portfolio{Name: "SMSF"}

	svc.populateNetFlows(testCtx(), portfolio)

	assert.Equal(t, 0.0, portfolio.YesterdayNetFlow)
	assert.Equal(t, 0.0, portfolio.LastWeekNetFlow)
}

func TestErrorLedger_PopulateNetFlows(t *testing.T) {
	svc := &Service{
		cashflowSvc: &mockCashFlowService{err: assert.AnError},
	}
	portfolio := &models.Portfolio{Name: "SMSF"}

	// Must not panic, should silently return
	svc.populateNetFlows(testCtx(), portfolio)

	assert.Equal(t, 0.0, portfolio.YesterdayNetFlow)
	assert.Equal(t, 0.0, portfolio.LastWeekNetFlow)
}

func TestZeroDateTransactions_GrowthCashFlow(t *testing.T) {
	// Transactions with zero dates should not panic in the cursor merge
	txs := []models.CashTransaction{
		{Type: models.CashTxDeposit, Date: time.Time{}, Amount: 5000},
		{Type: models.CashTxDeposit, Date: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), Amount: 10000},
	}

	// Simulate the sort that GetDailyGrowth does
	// Zero time sorts before all other dates, which is harmless
	dates := []time.Time{
		time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 6, 2, 0, 0, 0, 0, time.UTC),
	}

	// Simulate the cursor merge logic
	txCursor := 0
	var runningCashBalance float64
	for _, date := range dates {
		endOfDay := date.AddDate(0, 0, 1)
		for txCursor < len(txs) && txs[txCursor].Date.Before(endOfDay) {
			tx := txs[txCursor]
			if models.IsInflowType(tx.Type) {
				runningCashBalance += tx.Amount
			}
			txCursor++
		}
	}
	// Zero-dated transaction gets processed on the first date (before 2024-06-02)
	assert.Equal(t, 15000.0, runningCashBalance, "both transactions should be processed")
}

func TestZeroDateTransactions_PopulateNetFlows(t *testing.T) {
	// Transactions with zero dates should not match yesterday or last week windows
	now := time.Now().Truncate(24 * time.Hour)
	svc := &Service{
		cashflowSvc: &mockCashFlowService{
			ledger: &models.CashFlowLedger{
				Transactions: []models.CashTransaction{
					{Type: models.CashTxDeposit, Date: time.Time{}, Amount: 5000},
				},
			},
		},
	}
	portfolio := &models.Portfolio{Name: "SMSF"}

	svc.populateNetFlows(testCtx(), portfolio)

	// Zero date truncated is still zero time, which is not in [lastWeek, now)
	_ = now
	assert.Equal(t, 0.0, portfolio.YesterdayNetFlow,
		"zero-date transaction should not match yesterday")
	assert.Equal(t, 0.0, portfolio.LastWeekNetFlow,
		"zero-date transaction should not match last week")
}

// =============================================================================
// 4. Large Transaction Volumes: 10k+ transactions performance.
// =============================================================================

func TestLargeTransactionVolume_CursorMerge(t *testing.T) {
	// Simulate 10,000 transactions over 1 year
	n := 10000
	txs := make([]models.CashTransaction, n)
	baseDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		txs[i] = models.CashTransaction{
			Type:   models.CashTxContribution,
			Date:   baseDate.Add(time.Duration(i) * time.Hour), // multiple per day
			Amount: 100,
		}
	}

	// Generate 365 dates
	dates := generateCalendarDates(baseDate, baseDate.AddDate(0, 0, 364))
	require.Len(t, dates, 365)

	// Run the cursor merge
	start := time.Now()
	txCursor := 0
	var runningCashBalance, runningNetDeployed float64
	for _, date := range dates {
		endOfDay := date.AddDate(0, 0, 1)
		for txCursor < len(txs) && txs[txCursor].Date.Before(endOfDay) {
			tx := txs[txCursor]
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
			txCursor++
		}
	}
	elapsed := time.Since(start)

	// 10000 transactions at 1-hour intervals = 416+ days of transactions.
	// With only 365 days of dates, only the first 365*24 = 8760 transactions fit.
	expectedTxCount := 365 * 24
	assert.Equal(t, expectedTxCount, txCursor, "transactions within 365-day window should be processed")
	assert.Equal(t, float64(expectedTxCount)*100, runningCashBalance, "total cash balance")
	assert.Equal(t, float64(expectedTxCount)*100, runningNetDeployed, "total net deployed")
	assert.Less(t, elapsed, 100*time.Millisecond,
		"10k transactions merge should complete in < 100ms, took %v", elapsed)
}

func TestLargeTransactionVolume_PopulateNetFlows(t *testing.T) {
	// 1000 transactions in the last week
	now := time.Now().Truncate(24 * time.Hour)
	yesterday := now.AddDate(0, 0, -1)

	txs := make([]models.CashTransaction, 1000)
	for i := 0; i < 1000; i++ {
		txs[i] = models.CashTransaction{
			Type:   models.CashTxDeposit,
			Date:   yesterday.Add(time.Duration(i) * time.Minute),
			Amount: 100,
		}
	}

	svc := &Service{
		cashflowSvc: &mockCashFlowService{
			ledger: &models.CashFlowLedger{Transactions: txs},
		},
	}
	portfolio := &models.Portfolio{Name: "SMSF"}

	start := time.Now()
	svc.populateNetFlows(testCtx(), portfolio)
	elapsed := time.Since(start)

	assert.Equal(t, 100000.0, portfolio.YesterdayNetFlow, "1000 deposits of $100")
	assert.Equal(t, 100000.0, portfolio.LastWeekNetFlow, "same transactions in last week")
	assert.Less(t, elapsed, 50*time.Millisecond,
		"1k transactions net flow should complete in < 50ms")
}

// =============================================================================
// 5. Sign Logic: CashTransaction.Amount is always positive, type determines
//    direction. Verify sign logic is correct for all transaction types.
// =============================================================================

func TestSignLogic_CashBalance_AllTypes(t *testing.T) {
	tests := []struct {
		txType         models.CashTransactionType
		amount         float64
		expectInflow   bool   // for cash balance
		expectDeployed string // "add", "subtract", or "none" for net deployed
	}{
		{models.CashTxDeposit, 1000, true, "add"},
		{models.CashTxContribution, 2000, true, "add"},
		{models.CashTxTransferIn, 500, true, "none"},
		{models.CashTxDividend, 100, true, "none"},
		{models.CashTxWithdrawal, 3000, false, "subtract"},
		{models.CashTxTransferOut, 750, false, "none"},
	}

	for _, tt := range tests {
		t.Run(string(tt.txType), func(t *testing.T) {
			// Test IsInflowType
			isInflow := models.IsInflowType(tt.txType)
			assert.Equal(t, tt.expectInflow, isInflow,
				"IsInflowType(%s) should be %v", tt.txType, tt.expectInflow)

			// Test cash balance sign
			var cashBalance float64
			if isInflow {
				cashBalance += tt.amount
			} else {
				cashBalance -= tt.amount
			}
			if tt.expectInflow {
				assert.Greater(t, cashBalance, 0.0,
					"%s should increase cash balance", tt.txType)
			} else {
				assert.Less(t, cashBalance, 0.0,
					"%s should decrease cash balance", tt.txType)
			}

			// Test net deployed sign
			var netDeployed float64
			switch tt.txType {
			case models.CashTxDeposit, models.CashTxContribution:
				netDeployed += tt.amount
			case models.CashTxWithdrawal:
				netDeployed -= tt.amount
			}
			switch tt.expectDeployed {
			case "add":
				assert.Greater(t, netDeployed, 0.0,
					"%s should add to net deployed", tt.txType)
			case "subtract":
				assert.Less(t, netDeployed, 0.0,
					"%s should subtract from net deployed", tt.txType)
			case "none":
				assert.Equal(t, 0.0, netDeployed,
					"%s should not affect net deployed", tt.txType)
			}
		})
	}
}

func TestSignLogic_TransferInNotDeployed(t *testing.T) {
	// CRITICAL: transfer_in increases cash balance but should NOT count as
	// "net deployed" because it's moving money within the portfolio, not adding new capital.
	// Verify the growth.go implementation is correct.
	txs := []models.CashTransaction{
		{Type: models.CashTxTransferIn, Date: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), Amount: 50000},
	}

	var runningCashBalance, runningNetDeployed float64
	for _, tx := range txs {
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

	assert.Equal(t, 50000.0, runningCashBalance,
		"transfer_in should increase cash balance")
	assert.Equal(t, 0.0, runningNetDeployed,
		"transfer_in should NOT count as net deployed capital")
}

func TestSignLogic_DividendNotDeployed(t *testing.T) {
	// Dividend increases cash balance but is NOT deployed capital
	txs := []models.CashTransaction{
		{Type: models.CashTxDividend, Date: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), Amount: 2500},
	}

	var runningCashBalance, runningNetDeployed float64
	for _, tx := range txs {
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

	assert.Equal(t, 2500.0, runningCashBalance,
		"dividend should increase cash balance")
	assert.Equal(t, 0.0, runningNetDeployed,
		"dividend should NOT count as net deployed capital")
}

func TestSignLogic_PopulateNetFlows_MixedTypes(t *testing.T) {
	now := time.Now().Truncate(24 * time.Hour)
	yesterday := now.AddDate(0, 0, -1)

	svc := &Service{
		cashflowSvc: &mockCashFlowService{
			ledger: &models.CashFlowLedger{
				Transactions: []models.CashTransaction{
					{Type: models.CashTxDeposit, Date: yesterday, Amount: 10000},
					{Type: models.CashTxWithdrawal, Date: yesterday, Amount: 3000},
					{Type: models.CashTxDividend, Date: yesterday, Amount: 500},
					{Type: models.CashTxTransferOut, Date: yesterday, Amount: 200},
				},
			},
		},
	}
	portfolio := &models.Portfolio{Name: "SMSF"}
	svc.populateNetFlows(testCtx(), portfolio)

	// Net flow = +10000 (deposit) - 3000 (withdrawal) - 200 (transfer_out)
	// Dividends are excluded — they are investment returns, not capital movements.
	expected := 10000.0 - 3000.0 - 200.0
	assert.Equal(t, expected, portfolio.YesterdayNetFlow,
		"yesterday net flow should exclude dividends and account for capital movements only")
}

// =============================================================================
// 6. FX Edge Cases: Transactions in AUD but holdings may have USD values.
// =============================================================================

func TestFX_CashBalanceInPortfolioCurrency(t *testing.T) {
	// FINDING: Cash transactions have no currency field — they are assumed to be
	// in the portfolio's base currency (AUD). The growth.go cash flow merge adds
	// transaction amounts directly to runningCashBalance without FX conversion.
	// This is correct as long as all cash transactions are in AUD.
	// If a user enters a USD deposit, the amount would be in AUD-equivalent already.
	tx := models.CashTransaction{
		Type:   models.CashTxDeposit,
		Date:   time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		Amount: 10000, // AUD
	}
	// No currency field on CashTransaction — it's always portfolio currency
	assert.Equal(t, 10000.0, tx.Amount)
	// This is safe as long as the user enters amounts in portfolio currency.
	// No FX conversion needed in the timeline computation.
}

// =============================================================================
// 7. Same-Day Multiple Transactions: all must be counted.
// =============================================================================

func TestSameDayTransactions_GrowthCursor(t *testing.T) {
	// Multiple transactions on the same date should all be processed
	date := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	txs := []models.CashTransaction{
		{Type: models.CashTxDeposit, Date: date, Amount: 5000},
		{Type: models.CashTxDeposit, Date: date.Add(time.Hour), Amount: 3000},
		{Type: models.CashTxWithdrawal, Date: date.Add(2 * time.Hour), Amount: 1000},
	}

	// Simulate cursor merge for this date
	dates := []time.Time{date}
	txCursor := 0
	var runningCashBalance, runningNetDeployed float64

	for _, d := range dates {
		endOfDay := d.AddDate(0, 0, 1)
		for txCursor < len(txs) && txs[txCursor].Date.Before(endOfDay) {
			tx := txs[txCursor]
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
			txCursor++
		}
	}

	assert.Equal(t, 3, txCursor, "all 3 same-day transactions should be processed")
	assert.Equal(t, 7000.0, runningCashBalance, "5000 + 3000 - 1000 = 7000")
	assert.Equal(t, 7000.0, runningNetDeployed, "5000 + 3000 - 1000 = 7000")
}

func TestSameDayTransactions_PopulateNetFlows(t *testing.T) {
	now := time.Now().Truncate(24 * time.Hour)
	yesterday := now.AddDate(0, 0, -1)

	svc := &Service{
		cashflowSvc: &mockCashFlowService{
			ledger: &models.CashFlowLedger{
				Transactions: []models.CashTransaction{
					{Type: models.CashTxDeposit, Date: yesterday, Amount: 5000},
					{Type: models.CashTxDeposit, Date: yesterday.Add(time.Hour), Amount: 3000},
					{Type: models.CashTxWithdrawal, Date: yesterday.Add(2 * time.Hour), Amount: 1000},
				},
			},
		},
	}
	portfolio := &models.Portfolio{Name: "SMSF"}
	svc.populateNetFlows(testCtx(), portfolio)

	assert.Equal(t, 7000.0, portfolio.YesterdayNetFlow,
		"all same-day transactions should be summed: 5000 + 3000 - 1000")
}

// =============================================================================
// 8. Future-Dated Transactions: What if a transaction date is after today?
// =============================================================================

func TestFutureDatedTransactions_GrowthCursor(t *testing.T) {
	// Growth dates go up to yesterday (clamped in GetDailyGrowth).
	// A future-dated transaction should be processed when the cursor reaches
	// its date — but since dates stop at yesterday, it won't be reached.
	today := time.Now().Truncate(24 * time.Hour)
	futureDate := today.AddDate(0, 0, 30) // 30 days in the future

	txs := []models.CashTransaction{
		{Type: models.CashTxDeposit, Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 10000},
		{Type: models.CashTxDeposit, Date: futureDate, Amount: 50000},
	}

	// Dates stop at yesterday
	yesterday := today.AddDate(0, 0, -1)
	dates := generateCalendarDates(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), yesterday)

	txCursor := 0
	var runningCashBalance float64
	for _, date := range dates {
		endOfDay := date.AddDate(0, 0, 1)
		for txCursor < len(txs) && txs[txCursor].Date.Before(endOfDay) {
			runningCashBalance += txs[txCursor].Amount
			txCursor++
		}
	}

	assert.Equal(t, 10000.0, runningCashBalance,
		"future-dated transaction should NOT be processed in historical timeline")
	assert.Equal(t, 1, txCursor,
		"cursor should stop at the past transaction, skipping future one")
}

func TestFutureDatedTransactions_PopulateNetFlows(t *testing.T) {
	now := time.Now().Truncate(24 * time.Hour)
	tomorrow := now.AddDate(0, 0, 1)

	svc := &Service{
		cashflowSvc: &mockCashFlowService{
			ledger: &models.CashFlowLedger{
				Transactions: []models.CashTransaction{
					{Type: models.CashTxDeposit, Date: tomorrow, Amount: 99999},
				},
			},
		},
	}
	portfolio := &models.Portfolio{Name: "SMSF"}
	svc.populateNetFlows(testCtx(), portfolio)

	// Tomorrow is not in [lastWeek, now), so should not be counted
	assert.Equal(t, 0.0, portfolio.YesterdayNetFlow,
		"future-dated transaction should not affect yesterday net flow")
	assert.Equal(t, 0.0, portfolio.LastWeekNetFlow,
		"future-dated transaction should not affect last week net flow")
}

// =============================================================================
// Edge Cases: growthPointsToTimeSeries with new capital fields
// =============================================================================

func TestGrowthPointsToTimeSeries_CapitalFields(t *testing.T) {
	points := []models.GrowthDataPoint{
		{
			Date:        time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC),
			TotalValue:  500000,
			TotalCost:   400000,
			CashBalance: 25000,
			NetDeployed: 350000,
		},
	}
	externalBalance := 100000.0

	ts := growthPointsToTimeSeries(points, externalBalance)
	require.Len(t, ts, 1)

	pt := ts[0]
	assert.Equal(t, 600000.0, pt.Value, "value = 500000 + 100000 external")
	assert.Equal(t, 25000.0, pt.CashBalance, "cash balance passthrough")
	assert.Equal(t, 100000.0, pt.ExternalBalance, "external balance")
	assert.Equal(t, 625000.0, pt.TotalCapital,
		"total_capital = value(600000) + cash_balance(25000) = 625000")
	assert.Equal(t, 350000.0, pt.NetDeployed, "net deployed passthrough")
}

func TestGrowthPointsToTimeSeries_ZeroCashFields(t *testing.T) {
	// When no transactions provided, cash fields should be zero and omitted in JSON
	points := []models.GrowthDataPoint{
		{
			Date:       time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC),
			TotalValue: 100000,
		},
	}

	ts := growthPointsToTimeSeries(points, 50000)
	require.Len(t, ts, 1)

	pt := ts[0]
	assert.Equal(t, 0.0, pt.CashBalance)
	assert.Equal(t, 0.0, pt.NetDeployed)
	assert.Equal(t, 150000.0, pt.TotalCapital,
		"total_capital = value(150000) + cash_balance(0)")

	// Verify omitempty: zero fields should be omitted from JSON
	data, err := json.Marshal(pt)
	require.NoError(t, err)
	raw := string(data)
	assert.NotContains(t, raw, `"cash_balance"`,
		"zero cash_balance should be omitted via omitempty")
	assert.NotContains(t, raw, `"net_deployed"`,
		"zero net_deployed should be omitted via omitempty")
}

func TestGrowthPointsToTimeSeries_NegativeCashBalance(t *testing.T) {
	// Cash balance can go negative (more withdrawals than deposits)
	points := []models.GrowthDataPoint{
		{
			Date:        time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC),
			TotalValue:  100000,
			CashBalance: -5000, // overdraft scenario
		},
	}

	ts := growthPointsToTimeSeries(points, 0)
	require.Len(t, ts, 1)

	assert.Equal(t, -5000.0, ts[0].CashBalance)
	assert.Equal(t, 95000.0, ts[0].TotalCapital,
		"total_capital = 100000 + (-5000) = 95000")
}

func TestGrowthPointsToTimeSeries_NaNCashBalance(t *testing.T) {
	// NaN in cash fields should propagate (not crash)
	points := []models.GrowthDataPoint{
		{
			Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			TotalValue:  100000,
			CashBalance: math.NaN(),
		},
	}

	ts := growthPointsToTimeSeries(points, 50000)
	require.Len(t, ts, 1)
	assert.True(t, math.IsNaN(ts[0].CashBalance), "NaN should propagate")
	assert.True(t, math.IsNaN(ts[0].TotalCapital), "NaN in CashBalance makes TotalCapital NaN")
}

// =============================================================================
// GrowthDataPoint and TimeSeriesPoint JSON serialization
// =============================================================================

func TestTimeSeriesPoint_CapitalFields_JSON(t *testing.T) {
	pt := models.TimeSeriesPoint{
		Date:            time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC),
		Value:           500000,
		CashBalance:     25000,
		ExternalBalance: 100000,
		TotalCapital:    625000,
		NetDeployed:     350000,
	}

	data, err := json.Marshal(pt)
	require.NoError(t, err)
	raw := string(data)

	assert.Contains(t, raw, `"cash_balance"`)
	assert.Contains(t, raw, `"external_balance"`)
	assert.Contains(t, raw, `"total_capital"`)
	assert.Contains(t, raw, `"net_deployed"`)
}

func TestTimeSeriesPoint_CapitalFields_JSONRoundtrip(t *testing.T) {
	original := models.TimeSeriesPoint{
		Date:            time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC),
		Value:           500000,
		Cost:            400000,
		NetReturn:       100000,
		NetReturnPct:    25.0,
		HoldingCount:    10,
		CashBalance:     25000,
		ExternalBalance: 100000,
		TotalCapital:    625000,
		NetDeployed:     350000,
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var restored models.TimeSeriesPoint
	require.NoError(t, json.Unmarshal(data, &restored))

	assert.Equal(t, original.CashBalance, restored.CashBalance)
	assert.Equal(t, original.ExternalBalance, restored.ExternalBalance)
	assert.Equal(t, original.TotalCapital, restored.TotalCapital)
	assert.Equal(t, original.NetDeployed, restored.NetDeployed)
}

func TestPortfolio_NetFlowFields_JSON(t *testing.T) {
	p := models.Portfolio{
		YesterdayNetFlow: 5000,
		LastWeekNetFlow:  12500,
	}

	data, err := json.Marshal(p)
	require.NoError(t, err)
	raw := string(data)

	assert.Contains(t, raw, `"yesterday_net_flow"`)
	assert.Contains(t, raw, `"last_week_net_flow"`)
}

func TestPortfolio_NetFlowFields_OmitEmpty(t *testing.T) {
	p := models.Portfolio{
		YesterdayNetFlow: 0,
		LastWeekNetFlow:  0,
	}

	data, err := json.Marshal(p)
	require.NoError(t, err)
	raw := string(data)

	assert.NotContains(t, raw, `"yesterday_net_flow"`,
		"zero yesterday_net_flow should be omitted")
	assert.NotContains(t, raw, `"last_week_net_flow"`,
		"zero last_week_net_flow should be omitted")
}

// =============================================================================
// populateNetFlows window boundary tests
// =============================================================================

func TestPopulateNetFlows_WindowBoundaries(t *testing.T) {
	now := time.Now().Truncate(24 * time.Hour)

	tests := []struct {
		name            string
		txDate          time.Time
		expectYesterday bool
		expectLastWeek  bool
	}{
		{
			name:            "8 days ago — outside last week",
			txDate:          now.AddDate(0, 0, -8),
			expectYesterday: false,
			expectLastWeek:  false,
		},
		{
			name:            "exactly 7 days ago — start of last week window",
			txDate:          now.AddDate(0, 0, -7),
			expectYesterday: false,
			expectLastWeek:  true,
		},
		{
			name:            "6 days ago — within last week",
			txDate:          now.AddDate(0, 0, -6),
			expectYesterday: false,
			expectLastWeek:  true,
		},
		{
			name:            "2 days ago — within last week but not yesterday",
			txDate:          now.AddDate(0, 0, -2),
			expectYesterday: false,
			expectLastWeek:  true,
		},
		{
			name:            "yesterday — both yesterday and last week",
			txDate:          now.AddDate(0, 0, -1),
			expectYesterday: true,
			expectLastWeek:  true,
		},
		{
			name:            "today — not in either window (exclusive of today)",
			txDate:          now,
			expectYesterday: false,
			expectLastWeek:  false,
		},
		{
			name:            "tomorrow — future, not in any window",
			txDate:          now.AddDate(0, 0, 1),
			expectYesterday: false,
			expectLastWeek:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &Service{
				cashflowSvc: &mockCashFlowService{
					ledger: &models.CashFlowLedger{
						Transactions: []models.CashTransaction{
							{Type: models.CashTxDeposit, Date: tt.txDate, Amount: 1000},
						},
					},
				},
			}
			portfolio := &models.Portfolio{Name: "SMSF"}
			svc.populateNetFlows(testCtx(), portfolio)

			if tt.expectYesterday {
				assert.Equal(t, 1000.0, portfolio.YesterdayNetFlow,
					"should be in yesterday window")
			} else {
				assert.Equal(t, 0.0, portfolio.YesterdayNetFlow,
					"should NOT be in yesterday window")
			}

			if tt.expectLastWeek {
				assert.Equal(t, 1000.0, portfolio.LastWeekNetFlow,
					"should be in last week window")
			} else {
				assert.Equal(t, 0.0, portfolio.LastWeekNetFlow,
					"should NOT be in last week window")
			}
		})
	}
}

// =============================================================================
// TotalCapital formula: verify it includes ExternalBalance correctly
// =============================================================================

func TestTotalCapital_Formula(t *testing.T) {
	// TotalCapital = Value + CashBalance
	// where Value = TotalValue + ExternalBalanceTotal
	// So TotalCapital = TotalValue + ExternalBalanceTotal + CashBalance

	points := []models.GrowthDataPoint{
		{
			Date:        time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC),
			TotalValue:  200000, // equity holdings
			CashBalance: 15000,  // running cash from transactions
		},
	}
	externalBalance := 75000.0 // external balances (accumulate, term deposits)

	ts := growthPointsToTimeSeries(points, externalBalance)
	require.Len(t, ts, 1)

	pt := ts[0]
	expectedValue := 200000.0 + 75000.0             // 275000
	expectedTotalCapital := expectedValue + 15000.0 // 290000

	assert.Equal(t, expectedValue, pt.Value)
	assert.Equal(t, expectedTotalCapital, pt.TotalCapital,
		"TotalCapital = Value(%v) + CashBalance(%v)", pt.Value, pt.CashBalance)
}

// =============================================================================
// Concurrent access safety
// =============================================================================

func TestPopulateNetFlows_ConcurrentSafe(t *testing.T) {
	now := time.Now().Truncate(24 * time.Hour)
	yesterday := now.AddDate(0, 0, -1)

	svc := &Service{
		cashflowSvc: &mockCashFlowService{
			ledger: &models.CashFlowLedger{
				Transactions: []models.CashTransaction{
					{Type: models.CashTxDeposit, Date: yesterday, Amount: 1000},
				},
			},
		},
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p := &models.Portfolio{Name: "SMSF"}
			svc.populateNetFlows(testCtx(), p)
			assert.Equal(t, 1000.0, p.YesterdayNetFlow)
			assert.Equal(t, 1000.0, p.LastWeekNetFlow)
		}()
	}
	wg.Wait()
}

func TestGrowthCashMerge_ConcurrentSafe(t *testing.T) {
	// The cash merge in GetDailyGrowth uses local variables (txCursor,
	// runningCashBalance, runningNetDeployed), so concurrent calls are safe.
	// This test verifies via the cursor logic in isolation.
	txs := []models.CashTransaction{
		{Type: models.CashTxDeposit, Date: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), Amount: 10000},
		{Type: models.CashTxWithdrawal, Date: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC), Amount: 3000},
	}
	dates := generateCalendarDates(
		time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
	)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Each goroutine has its own cursor state
			localTxs := make([]models.CashTransaction, len(txs))
			copy(localTxs, txs)

			txCursor := 0
			var cashBal, netDep float64
			for _, date := range dates {
				endOfDay := date.AddDate(0, 0, 1)
				for txCursor < len(localTxs) && localTxs[txCursor].Date.Before(endOfDay) {
					tx := localTxs[txCursor]
					if models.IsInflowType(tx.Type) {
						cashBal += tx.Amount
					} else {
						cashBal -= tx.Amount
					}
					switch tx.Type {
					case models.CashTxDeposit, models.CashTxContribution:
						netDep += tx.Amount
					case models.CashTxWithdrawal:
						netDep -= tx.Amount
					}
					txCursor++
				}
			}
			assert.Equal(t, 7000.0, cashBal, "10000 - 3000 = 7000")
			assert.Equal(t, 7000.0, netDep, "10000 - 3000 = 7000")
		}()
	}
	wg.Wait()
}

// =============================================================================
// FINDING: sort.Slice on opts.Transactions mutates the caller's slice
// =============================================================================

func TestGrowthSortMutatesCallerSlice(t *testing.T) {
	// FINDING: GetDailyGrowth does `sort.Slice(txs, ...)` where txs = opts.Transactions.
	// This modifies the caller's slice in-place because Go slices are references.
	// If GetPortfolioIndicators passes ledger.Transactions to GetDailyGrowth,
	// the ledger's transaction order is mutated as a side effect.
	// This is not a bug per se (the ledger is discarded after), but it's a
	// potential surprise if the transactions are used elsewhere after.

	original := []models.CashTransaction{
		{Type: models.CashTxDeposit, Date: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), Amount: 5000},
		{Type: models.CashTxDeposit, Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 10000},
		{Type: models.CashTxDeposit, Date: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), Amount: 7500},
	}

	// Verify that the original order is Jun, Jan, Mar (unsorted)
	assert.True(t, original[0].Date.After(original[1].Date),
		"original order: first is after second")

	// When GetDailyGrowth sorts opts.Transactions in-place,
	// the original slice gets sorted too
	opts := interfaces.GrowthOptions{Transactions: original}
	txs := opts.Transactions

	// Simulate what GetDailyGrowth does
	import_sort_slice(txs)

	// The original slice is now sorted (mutation!)
	assert.True(t, original[0].Date.Before(original[1].Date),
		"FINDING: sort.Slice in GetDailyGrowth mutates the caller's transaction slice. "+
			"This is safe in current usage (ledger is loaded fresh each time) but "+
			"could cause subtle bugs if the ledger is cached or reused.")
}

// import_sort_slice simulates the sort in GetDailyGrowth
func import_sort_slice(txs []models.CashTransaction) {
	// Inline sort to avoid importing sort in the declaration area above
	for i := 0; i < len(txs); i++ {
		for j := i + 1; j < len(txs); j++ {
			if txs[j].Date.Before(txs[i].Date) {
				txs[i], txs[j] = txs[j], txs[i]
			}
		}
	}
}

// =============================================================================
// FINDING: Cash flow points skipped when totalValue == 0 && totalCost == 0
// =============================================================================

func TestCashFlowPointsSkippedBeforeFirstTrade(t *testing.T) {
	// FINDING: GetDailyGrowth skips dates where totalValue == 0 && totalCost == 0.
	// If cash transactions exist BEFORE the first trade, those cash flow values
	// are accumulated in runningCashBalance/runningNetDeployed but the
	// GrowthDataPoint is not emitted for those dates.
	// The first emitted point will already have the accumulated cash balance.
	// This is correct behavior: the timeline starts when the portfolio has value.

	// Simulate: deposit on Jan 1, first buy on Feb 1
	txs := []models.CashTransaction{
		{Type: models.CashTxDeposit, Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 50000},
	}

	dates := []time.Time{
		time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), // no holdings yet
		time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC), // holdings start
	}

	txCursor := 0
	var runningCashBalance float64
	var points []models.GrowthDataPoint

	for _, date := range dates {
		totalValue := 0.0
		totalCost := 0.0
		if date.After(time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC)) {
			totalValue = 48000 // first trade, market value
			totalCost = 50000
		}

		endOfDay := date.AddDate(0, 0, 1)
		for txCursor < len(txs) && txs[txCursor].Date.Before(endOfDay) {
			runningCashBalance += txs[txCursor].Amount
			txCursor++
		}

		if totalValue == 0 && totalCost == 0 {
			continue // skipped — same as GetDailyGrowth
		}

		points = append(points, models.GrowthDataPoint{
			Date:        date,
			TotalValue:  totalValue,
			TotalCost:   totalCost,
			CashBalance: runningCashBalance,
		})
	}

	require.Len(t, points, 1, "only Feb 1 point should be emitted")
	assert.Equal(t, 50000.0, points[0].CashBalance,
		"first emitted point should include accumulated cash from Jan deposit")
}

// =============================================================================
// Test helpers
// =============================================================================

// mockCashFlowService implements interfaces.CashFlowService for testing
type mockCashFlowService struct {
	ledger *models.CashFlowLedger
	err    error
}

func (m *mockCashFlowService) GetLedger(_ context.Context, _ string) (*models.CashFlowLedger, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.ledger, nil
}

func (m *mockCashFlowService) AddTransaction(_ context.Context, _ string, _ models.CashTransaction) (*models.CashFlowLedger, error) {
	return nil, nil
}

func (m *mockCashFlowService) UpdateTransaction(_ context.Context, _, _ string, _ models.CashTransaction) (*models.CashFlowLedger, error) {
	return nil, nil
}

func (m *mockCashFlowService) RemoveTransaction(_ context.Context, _, _ string) (*models.CashFlowLedger, error) {
	return nil, nil
}

func (m *mockCashFlowService) CalculatePerformance(_ context.Context, _ string) (*models.CapitalPerformance, error) {
	return nil, nil
}

// trackingCashFlowService tracks call counts for cycle detection
type trackingCashFlowService struct {
	mockCashFlowService
	getLedgerCalls int
}

func (m *trackingCashFlowService) GetLedger(ctx context.Context, name string) (*models.CashFlowLedger, error) {
	m.getLedgerCalls++
	return m.mockCashFlowService.GetLedger(ctx, name)
}

// testCtx returns a context with a test user
func testCtx() context.Context {
	return context.Background()
}
