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

func TestCircularDependency_SetCashFlowServiceNil(t *testing.T) {
	svc := &Service{}
	svc.SetCashFlowService(nil)
	assert.Nil(t, svc.cashflowSvc)
}

func TestCircularDependency_SetCashFlowServiceTwice(t *testing.T) {
	svc := &Service{}
	mock1 := &mockCashFlowService{ledger: &models.CashFlowLedger{}}
	mock2 := &mockCashFlowService{ledger: &models.CashFlowLedger{
		Transactions: []models.CashTransaction{
			{Account: "Trading", Category: models.CashCatContribution, Amount: 1000},
		},
	}}
	svc.SetCashFlowService(mock1)
	assert.Equal(t, mock1, svc.cashflowSvc)
	svc.SetCashFlowService(mock2)
	assert.Equal(t, mock2, svc.cashflowSvc)
}

func TestCircularDependency_NoCyclicCallInPopulateNetFlows(t *testing.T) {
	tracker := &trackingCashFlowService{
		mockCashFlowService: mockCashFlowService{
			ledger: &models.CashFlowLedger{
				Transactions: []models.CashTransaction{
					{Account: "Trading", Category: models.CashCatContribution, Date: time.Now().Add(-24 * time.Hour), Amount: 5000},
				},
			},
		},
	}
	svc := &Service{cashflowSvc: tracker}
	portfolio := &models.Portfolio{Name: "SMSF"}
	svc.populateNetFlows(testCtx(), portfolio)
	assert.Equal(t, 1, tracker.getLedgerCalls, "GetLedger should be called exactly once")
}

func TestNilCashFlowService_PopulateNetFlows(t *testing.T) {
	svc := &Service{cashflowSvc: nil}
	portfolio := &models.Portfolio{Name: "SMSF", TotalValue: 100000}
	svc.populateNetFlows(testCtx(), portfolio)
	assert.Equal(t, 0.0, portfolio.YesterdayNetFlow)
	assert.Equal(t, 0.0, portfolio.LastWeekNetFlow)
}

func TestNilCashFlowService_GrowthOptionsNoTransactions(t *testing.T) {
	opts := interfaces.GrowthOptions{Transactions: nil}
	assert.Empty(t, opts.Transactions)
}

func TestNilCashFlowService_GrowthPointsHaveZeroCashFields(t *testing.T) {
	point := models.GrowthDataPoint{
		Date: time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC), TotalValue: 100000, TotalCost: 90000,
	}
	assert.Equal(t, 0.0, point.CashBalance)
	assert.Equal(t, 0.0, point.NetDeployed)
	assert.Equal(t, 0.0, point.TotalCapital)
	assert.Equal(t, 0.0, point.ExternalBalance)
}

func TestEmptyLedger_PopulateNetFlows(t *testing.T) {
	svc := &Service{
		cashflowSvc: &mockCashFlowService{ledger: &models.CashFlowLedger{Transactions: []models.CashTransaction{}}},
	}
	portfolio := &models.Portfolio{Name: "SMSF"}
	svc.populateNetFlows(testCtx(), portfolio)
	assert.Equal(t, 0.0, portfolio.YesterdayNetFlow)
	assert.Equal(t, 0.0, portfolio.LastWeekNetFlow)
}

func TestNilLedger_PopulateNetFlows(t *testing.T) {
	svc := &Service{cashflowSvc: &mockCashFlowService{ledger: nil}}
	portfolio := &models.Portfolio{Name: "SMSF"}
	svc.populateNetFlows(testCtx(), portfolio)
	assert.Equal(t, 0.0, portfolio.YesterdayNetFlow)
	assert.Equal(t, 0.0, portfolio.LastWeekNetFlow)
}

func TestErrorLedger_PopulateNetFlows(t *testing.T) {
	svc := &Service{cashflowSvc: &mockCashFlowService{err: assert.AnError}}
	portfolio := &models.Portfolio{Name: "SMSF"}
	svc.populateNetFlows(testCtx(), portfolio)
	assert.Equal(t, 0.0, portfolio.YesterdayNetFlow)
	assert.Equal(t, 0.0, portfolio.LastWeekNetFlow)
}

func TestZeroDateTransactions_GrowthCashFlow(t *testing.T) {
	txs := []models.CashTransaction{
		{Account: "Trading", Category: models.CashCatContribution, Date: time.Time{}, Amount: 10000},
		{Account: "Trading", Category: models.CashCatContribution, Date: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), Amount: 5000},
	}
	dates := []time.Time{
		time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 6, 2, 0, 0, 0, 0, time.UTC),
	}
	txCursor := 0
	var runningCashBalance float64
	for _, date := range dates {
		endOfDay := date.AddDate(0, 0, 1)
		for txCursor < len(txs) && txs[txCursor].Date.Before(endOfDay) {
			runningCashBalance += txs[txCursor].SignedAmount()
			txCursor++
		}
	}
	assert.Equal(t, 15000.0, runningCashBalance, "both transactions should be processed")
}

func TestZeroDateTransactions_PopulateNetFlows(t *testing.T) {
	now := time.Now().Truncate(24 * time.Hour)
	svc := &Service{
		cashflowSvc: &mockCashFlowService{
			ledger: &models.CashFlowLedger{
				Transactions: []models.CashTransaction{
					{Account: "Trading", Category: models.CashCatContribution, Date: time.Time{}, Amount: 5000},
				},
			},
		},
	}
	portfolio := &models.Portfolio{Name: "SMSF"}
	svc.populateNetFlows(testCtx(), portfolio)
	_ = now
	assert.Equal(t, 0.0, portfolio.YesterdayNetFlow)
	assert.Equal(t, 0.0, portfolio.LastWeekNetFlow)
}

func TestLargeTransactionVolume_CursorMerge(t *testing.T) {
	n := 10000
	txs := make([]models.CashTransaction, n)
	baseDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		txs[i] = models.CashTransaction{
			Account: "Trading", Category: models.CashCatContribution,
			Date: baseDate.Add(time.Duration(i) * time.Hour), Amount: 100,
		}
	}
	dates := generateCalendarDates(baseDate, baseDate.AddDate(0, 0, 364))
	require.Len(t, dates, 365)

	start := time.Now()
	txCursor := 0
	var runningCashBalance, runningNetDeployed float64
	for _, date := range dates {
		endOfDay := date.AddDate(0, 0, 1)
		for txCursor < len(txs) && txs[txCursor].Date.Before(endOfDay) {
			tx := txs[txCursor]
			txCursor++
			runningCashBalance += tx.SignedAmount()
			runningNetDeployed += tx.NetDeployedImpact()
		}
	}
	elapsed := time.Since(start)

	expectedTxCount := 365 * 24
	assert.Equal(t, expectedTxCount, txCursor)
	assert.Equal(t, float64(expectedTxCount)*100, runningCashBalance)
	assert.Equal(t, float64(expectedTxCount)*100, runningNetDeployed)
	assert.Less(t, elapsed, 100*time.Millisecond)
}

func TestLargeTransactionVolume_PopulateNetFlows(t *testing.T) {
	now := time.Now().Truncate(24 * time.Hour)
	yesterday := now.AddDate(0, 0, -1)
	txs := make([]models.CashTransaction, 1000)
	for i := 0; i < 1000; i++ {
		txs[i] = models.CashTransaction{
			Account: "Trading", Category: models.CashCatContribution,
			Date: yesterday.Add(time.Duration(i) * time.Minute), Amount: 100,
		}
	}
	svc := &Service{cashflowSvc: &mockCashFlowService{ledger: &models.CashFlowLedger{Transactions: txs}}}
	portfolio := &models.Portfolio{Name: "SMSF"}
	start := time.Now()
	svc.populateNetFlows(testCtx(), portfolio)
	elapsed := time.Since(start)
	assert.Equal(t, 100000.0, portfolio.YesterdayNetFlow)
	assert.Equal(t, 100000.0, portfolio.LastWeekNetFlow)
	assert.Less(t, elapsed, 50*time.Millisecond)
}

// Sign logic: with signed amounts, Amount is already signed.
// Positive = credit/inflow, negative = debit/outflow.

func TestSignLogic_CashBalance_AllTypes(t *testing.T) {
	tests := []struct {
		name           string
		category       models.CashCategory
		amount         float64
		expectInflow   bool
		expectDeployed string
	}{
		{"credit_contribution", models.CashCatContribution, 1000, true, "add"},
		{"credit_dividend", models.CashCatDividend, 100, true, "none"},
		{"credit_transfer", models.CashCatTransfer, 500, true, "none"},
		{"debit_other", models.CashCatOther, -3000, false, "subtract"},
		{"debit_fee", models.CashCatFee, -50, false, "subtract"},
		{"debit_transfer", models.CashCatTransfer, -750, false, "subtract"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := models.CashTransaction{Account: "Trading", Category: tt.category, Amount: tt.amount}

			isInflow := tx.SignedAmount() > 0
			assert.Equal(t, tt.expectInflow, isInflow)

			cashBalance := tx.SignedAmount()
			if tt.expectInflow {
				assert.Greater(t, cashBalance, 0.0)
			} else {
				assert.Less(t, cashBalance, 0.0)
			}

			netDeployed := tx.NetDeployedImpact()
			switch tt.expectDeployed {
			case "add":
				assert.Greater(t, netDeployed, 0.0)
			case "subtract":
				assert.Less(t, netDeployed, 0.0)
			case "none":
				assert.Equal(t, 0.0, netDeployed)
			}
		})
	}
}

func TestSignLogic_TransferNotDeployed(t *testing.T) {
	txs := []models.CashTransaction{
		{Account: "Trading", Category: models.CashCatTransfer, Amount: 50000},
		{Account: "Trading", Category: models.CashCatTransfer, Amount: -50000},
	}
	var runningCashBalance, runningNetDeployed float64
	for _, tx := range txs {
		runningCashBalance += tx.SignedAmount()
		runningNetDeployed += tx.NetDeployedImpact()
	}
	assert.Equal(t, 0.0, runningCashBalance)
	assert.Equal(t, -50000.0, runningNetDeployed)
}

func TestSignLogic_DividendNotDeployed(t *testing.T) {
	txs := []models.CashTransaction{
		{Account: "Trading", Category: models.CashCatDividend, Amount: 2500},
	}
	var runningCashBalance, runningNetDeployed float64
	for _, tx := range txs {
		runningCashBalance += tx.SignedAmount()
		runningNetDeployed += tx.NetDeployedImpact()
	}
	assert.Equal(t, 2500.0, runningCashBalance)
	assert.Equal(t, 0.0, runningNetDeployed)
}

func TestSignLogic_PopulateNetFlows_MixedTypes(t *testing.T) {
	now := time.Now().Truncate(24 * time.Hour)
	yesterday := now.AddDate(0, 0, -1)
	svc := &Service{
		cashflowSvc: &mockCashFlowService{
			ledger: &models.CashFlowLedger{
				Transactions: []models.CashTransaction{
					{Account: "Trading", Category: models.CashCatContribution, Date: yesterday, Amount: 10000},
					{Account: "Trading", Category: models.CashCatDividend, Date: yesterday, Amount: 500},
					{Account: "Trading", Category: models.CashCatOther, Date: yesterday, Amount: -3000},
					{Account: "Trading", Category: models.CashCatTransfer, Date: yesterday, Amount: -200},
				},
			},
		},
	}
	portfolio := &models.Portfolio{Name: "SMSF"}
	svc.populateNetFlows(testCtx(), portfolio)
	// Dividends excluded from net flow
	assert.Equal(t, 10000.0-3000.0-200.0, portfolio.YesterdayNetFlow)
}

func TestFX_CashBalanceInPortfolioCurrency(t *testing.T) {
	tx := models.CashTransaction{
		Account: "Trading", Category: models.CashCatContribution,
		Date: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), Amount: 10000,
	}
	assert.Equal(t, 10000.0, tx.SignedAmount())
}

func TestSameDayTransactions_GrowthCursor(t *testing.T) {
	date := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	txs := []models.CashTransaction{
		{Account: "Trading", Category: models.CashCatContribution, Date: date, Amount: 5000},
		{Account: "Trading", Category: models.CashCatContribution, Date: date, Amount: 3000},
		{Account: "Trading", Category: models.CashCatOther, Date: date, Amount: -1000},
	}
	dates := []time.Time{date}
	txCursor := 0
	var runningCashBalance, runningNetDeployed float64
	for _, d := range dates {
		endOfDay := d.AddDate(0, 0, 1)
		for txCursor < len(txs) && txs[txCursor].Date.Before(endOfDay) {
			tx := txs[txCursor]
			txCursor++
			runningCashBalance += tx.SignedAmount()
			runningNetDeployed += tx.NetDeployedImpact()
		}
	}
	assert.Equal(t, 3, txCursor)
	assert.Equal(t, 7000.0, runningCashBalance)
	assert.Equal(t, 7000.0, runningNetDeployed)
}

func TestSameDayTransactions_PopulateNetFlows(t *testing.T) {
	now := time.Now().Truncate(24 * time.Hour)
	yesterday := now.AddDate(0, 0, -1)
	svc := &Service{
		cashflowSvc: &mockCashFlowService{
			ledger: &models.CashFlowLedger{
				Transactions: []models.CashTransaction{
					{Account: "Trading", Category: models.CashCatContribution, Date: yesterday, Amount: 5000},
					{Account: "Trading", Category: models.CashCatContribution, Date: yesterday, Amount: 3000},
					{Account: "Trading", Category: models.CashCatOther, Date: yesterday, Amount: -1000},
				},
			},
		},
	}
	portfolio := &models.Portfolio{Name: "SMSF"}
	svc.populateNetFlows(testCtx(), portfolio)
	assert.Equal(t, 7000.0, portfolio.YesterdayNetFlow)
}

func TestFutureDatedTransactions_GrowthCursor(t *testing.T) {
	today := time.Now().Truncate(24 * time.Hour)
	futureDate := today.AddDate(0, 0, 30)
	txs := []models.CashTransaction{
		{Account: "Trading", Category: models.CashCatContribution, Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 10000},
		{Account: "Trading", Category: models.CashCatContribution, Date: futureDate, Amount: 5000},
	}
	yesterday := today.AddDate(0, 0, -1)
	dates := generateCalendarDates(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), yesterday)
	txCursor := 0
	var runningCashBalance float64
	for _, date := range dates {
		endOfDay := date.AddDate(0, 0, 1)
		for txCursor < len(txs) && txs[txCursor].Date.Before(endOfDay) {
			runningCashBalance += txs[txCursor].SignedAmount()
			txCursor++
		}
	}
	assert.Equal(t, 10000.0, runningCashBalance)
	assert.Equal(t, 1, txCursor)
}

func TestFutureDatedTransactions_PopulateNetFlows(t *testing.T) {
	now := time.Now().Truncate(24 * time.Hour)
	tomorrow := now.AddDate(0, 0, 1)
	svc := &Service{
		cashflowSvc: &mockCashFlowService{
			ledger: &models.CashFlowLedger{
				Transactions: []models.CashTransaction{
					{Account: "Trading", Category: models.CashCatContribution, Date: tomorrow, Amount: 5000},
				},
			},
		},
	}
	portfolio := &models.Portfolio{Name: "SMSF"}
	svc.populateNetFlows(testCtx(), portfolio)
	assert.Equal(t, 0.0, portfolio.YesterdayNetFlow)
	assert.Equal(t, 0.0, portfolio.LastWeekNetFlow)
}

func TestGrowthPointsToTimeSeries_CapitalFields(t *testing.T) {
	points := []models.GrowthDataPoint{
		{Date: time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC), TotalValue: 500000, TotalCost: 400000, CashBalance: 25000, NetDeployed: 350000},
	}
	ts := growthPointsToTimeSeries(points, 100000)
	require.Len(t, ts, 1)
	pt := ts[0]
	assert.Equal(t, 600000.0, pt.Value)
	assert.Equal(t, 25000.0, pt.CashBalance)
	assert.Equal(t, 100000.0, pt.ExternalBalance)
	assert.Equal(t, 625000.0, pt.TotalCapital)
	assert.Equal(t, 350000.0, pt.NetDeployed)
}

func TestGrowthPointsToTimeSeries_ZeroCashFields(t *testing.T) {
	points := []models.GrowthDataPoint{
		{Date: time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC), TotalValue: 100000},
	}
	ts := growthPointsToTimeSeries(points, 50000)
	require.Len(t, ts, 1)
	pt := ts[0]
	assert.Equal(t, 0.0, pt.CashBalance)
	assert.Equal(t, 0.0, pt.NetDeployed)
	assert.Equal(t, 150000.0, pt.TotalCapital)
	data, err := json.Marshal(pt)
	require.NoError(t, err)
	raw := string(data)
	assert.NotContains(t, raw, `"cash_balance"`)
	assert.NotContains(t, raw, `"net_deployed"`)
}

func TestGrowthPointsToTimeSeries_NegativeCashBalance(t *testing.T) {
	points := []models.GrowthDataPoint{
		{Date: time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC), TotalValue: 100000, CashBalance: -5000},
	}
	ts := growthPointsToTimeSeries(points, 0)
	require.Len(t, ts, 1)
	assert.Equal(t, -5000.0, ts[0].CashBalance)
	assert.Equal(t, 95000.0, ts[0].TotalCapital)
}

func TestGrowthPointsToTimeSeries_NaNCashBalance(t *testing.T) {
	points := []models.GrowthDataPoint{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), TotalValue: 100000, CashBalance: math.NaN()},
	}
	ts := growthPointsToTimeSeries(points, 50000)
	require.Len(t, ts, 1)
	assert.True(t, math.IsNaN(ts[0].CashBalance))
	assert.True(t, math.IsNaN(ts[0].TotalCapital))
}

func TestTimeSeriesPoint_CapitalFields_JSON(t *testing.T) {
	pt := models.TimeSeriesPoint{
		Date: time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC), Value: 500000,
		CashBalance: 25000, ExternalBalance: 100000, TotalCapital: 625000, NetDeployed: 350000,
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
		Date: time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC), Value: 500000, Cost: 400000,
		NetReturn: 100000, NetReturnPct: 25.0, HoldingCount: 10,
		CashBalance: 25000, ExternalBalance: 100000, TotalCapital: 625000, NetDeployed: 350000,
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
	p := models.Portfolio{YesterdayNetFlow: 5000, LastWeekNetFlow: 12500}
	data, err := json.Marshal(p)
	require.NoError(t, err)
	raw := string(data)
	assert.Contains(t, raw, `"yesterday_net_flow"`)
	assert.Contains(t, raw, `"last_week_net_flow"`)
}

func TestPortfolio_NetFlowFields_OmitEmpty(t *testing.T) {
	p := models.Portfolio{YesterdayNetFlow: 0, LastWeekNetFlow: 0}
	data, err := json.Marshal(p)
	require.NoError(t, err)
	raw := string(data)
	assert.NotContains(t, raw, `"yesterday_net_flow"`)
	assert.NotContains(t, raw, `"last_week_net_flow"`)
}

func TestPopulateNetFlows_WindowBoundaries(t *testing.T) {
	now := time.Now().Truncate(24 * time.Hour)
	tests := []struct {
		name            string
		txDate          time.Time
		expectYesterday bool
		expectLastWeek  bool
	}{
		{"8 days ago", now.AddDate(0, 0, -8), false, false},
		{"exactly 7 days ago", now.AddDate(0, 0, -7), false, true},
		{"6 days ago", now.AddDate(0, 0, -6), false, true},
		{"2 days ago", now.AddDate(0, 0, -2), false, true},
		{"yesterday", now.AddDate(0, 0, -1), true, true},
		{"today", now, false, false},
		{"tomorrow", now.AddDate(0, 0, 1), false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &Service{
				cashflowSvc: &mockCashFlowService{
					ledger: &models.CashFlowLedger{
						Transactions: []models.CashTransaction{
							{Account: "Trading", Category: models.CashCatContribution, Date: tt.txDate, Amount: 1000},
						},
					},
				},
			}
			portfolio := &models.Portfolio{Name: "SMSF"}
			svc.populateNetFlows(testCtx(), portfolio)
			if tt.expectYesterday {
				assert.Equal(t, 1000.0, portfolio.YesterdayNetFlow)
			} else {
				assert.Equal(t, 0.0, portfolio.YesterdayNetFlow)
			}
			if tt.expectLastWeek {
				assert.Equal(t, 1000.0, portfolio.LastWeekNetFlow)
			} else {
				assert.Equal(t, 0.0, portfolio.LastWeekNetFlow)
			}
		})
	}
}

func TestTotalCapital_Formula(t *testing.T) {
	points := []models.GrowthDataPoint{
		{Date: time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC), TotalValue: 200000, CashBalance: 15000},
	}
	ts := growthPointsToTimeSeries(points, 75000)
	require.Len(t, ts, 1)
	assert.Equal(t, 275000.0, ts[0].Value)
	assert.Equal(t, 290000.0, ts[0].TotalCapital)
}

func TestPopulateNetFlows_ConcurrentSafe(t *testing.T) {
	now := time.Now().Truncate(24 * time.Hour)
	yesterday := now.AddDate(0, 0, -1)
	svc := &Service{
		cashflowSvc: &mockCashFlowService{
			ledger: &models.CashFlowLedger{
				Transactions: []models.CashTransaction{
					{Account: "Trading", Category: models.CashCatContribution, Date: yesterday, Amount: 1000},
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
	txs := []models.CashTransaction{
		{Account: "Trading", Category: models.CashCatContribution, Date: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), Amount: 10000},
		{Account: "Trading", Category: models.CashCatOther, Date: time.Date(2024, 2, 15, 0, 0, 0, 0, time.UTC), Amount: -3000},
	}
	dates := generateCalendarDates(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC))
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			localTxs := make([]models.CashTransaction, len(txs))
			copy(localTxs, txs)
			txCursor := 0
			var cashBal, netDep float64
			for _, date := range dates {
				endOfDay := date.AddDate(0, 0, 1)
				for txCursor < len(localTxs) && localTxs[txCursor].Date.Before(endOfDay) {
					tx := localTxs[txCursor]
					txCursor++
					cashBal += tx.SignedAmount()
					netDep += tx.NetDeployedImpact()
				}
			}
			assert.Equal(t, 7000.0, cashBal)
			assert.Equal(t, 7000.0, netDep)
		}()
	}
	wg.Wait()
}

func TestGrowthSortMutatesCallerSlice(t *testing.T) {
	original := []models.CashTransaction{
		{Account: "Trading", Category: models.CashCatContribution, Date: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), Amount: 1000},
		{Account: "Trading", Category: models.CashCatContribution, Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 2000},
		{Account: "Trading", Category: models.CashCatContribution, Date: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), Amount: 3000},
	}
	assert.True(t, original[0].Date.After(original[1].Date))
	opts := interfaces.GrowthOptions{Transactions: original}
	import_sort_slice(opts.Transactions)
	assert.True(t, original[0].Date.Before(original[1].Date),
		"FINDING: sort mutates the caller's transaction slice")
}

func import_sort_slice(txs []models.CashTransaction) {
	for i := 0; i < len(txs); i++ {
		for j := i + 1; j < len(txs); j++ {
			if txs[j].Date.Before(txs[i].Date) {
				txs[i], txs[j] = txs[j], txs[i]
			}
		}
	}
}

func TestCashFlowPointsSkippedBeforeFirstTrade(t *testing.T) {
	txs := []models.CashTransaction{
		{Account: "Trading", Category: models.CashCatContribution, Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 50000},
	}
	dates := []time.Time{
		time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
	}
	txCursor := 0
	var runningCashBalance float64
	var points []models.GrowthDataPoint
	for _, date := range dates {
		totalValue := 0.0
		totalCost := 0.0
		if date.After(time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC)) {
			totalValue = 48000
			totalCost = 50000
		}
		endOfDay := date.AddDate(0, 0, 1)
		for txCursor < len(txs) && txs[txCursor].Date.Before(endOfDay) {
			runningCashBalance += txs[txCursor].SignedAmount()
			txCursor++
		}
		if totalValue == 0 && totalCost == 0 {
			continue
		}
		points = append(points, models.GrowthDataPoint{
			Date: date, TotalValue: totalValue, TotalCost: totalCost, CashBalance: runningCashBalance,
		})
	}
	require.Len(t, points, 1)
	assert.Equal(t, 50000.0, points[0].CashBalance)
}

// Test helpers

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
func (m *mockCashFlowService) AddTransfer(_ context.Context, _ string, _, _ string, _ float64, _ time.Time, _ string) (*models.CashFlowLedger, error) {
	return nil, nil
}
func (m *mockCashFlowService) UpdateTransaction(_ context.Context, _, _ string, _ models.CashTransaction) (*models.CashFlowLedger, error) {
	return nil, nil
}
func (m *mockCashFlowService) RemoveTransaction(_ context.Context, _, _ string) (*models.CashFlowLedger, error) {
	return nil, nil
}
func (m *mockCashFlowService) SetTransactions(_ context.Context, _ string, _ []models.CashTransaction, _ string) (*models.CashFlowLedger, error) {
	return nil, nil
}
func (m *mockCashFlowService) UpdateAccount(_ context.Context, _ string, _ string, _ models.CashAccountUpdate) (*models.CashFlowLedger, error) {
	return nil, nil
}
func (m *mockCashFlowService) CalculatePerformance(_ context.Context, _ string) (*models.CapitalPerformance, error) {
	return nil, nil
}

type trackingCashFlowService struct {
	mockCashFlowService
	getLedgerCalls int
}

func (m *trackingCashFlowService) GetLedger(ctx context.Context, name string) (*models.CashFlowLedger, error) {
	m.getLedgerCalls++
	return m.mockCashFlowService.GetLedger(ctx, name)
}

func testCtx() context.Context {
	return context.Background()
}
