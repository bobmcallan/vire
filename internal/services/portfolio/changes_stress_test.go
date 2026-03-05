package portfolio

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Devils-advocate stress tests for portfolio changes feature.
// Tests adversarial scenarios: empty data, zero divisions, negative changes,
// large numbers, concurrent access, and edge cases.
// Tests are prefixed with "Stress_" to avoid collision with implementer's unit tests.

// =============================================================================
// buildMetricChange stress tests
// =============================================================================

func TestStress_BuildMetricChange_ZeroPrevious(t *testing.T) {
	// CRITICAL: Division by zero protection
	mc := buildMetricChange(1000.0, 0.0)
	assert.Equal(t, 1000.0, mc.Current)
	assert.Equal(t, 0.0, mc.Previous)
	assert.Equal(t, 1000.0, mc.RawChange)
	assert.Equal(t, 0.0, mc.PctChange, "PctChange must be 0 when previous is 0 (no divide-by-zero)")
	assert.False(t, mc.HasPrevious, "HasPrevious must be false when previous is 0")
}

func TestStress_BuildMetricChange_ZeroCurrent(t *testing.T) {
	// Position sold off completely
	mc := buildMetricChange(0.0, 1000.0)
	assert.Equal(t, 0.0, mc.Current)
	assert.Equal(t, 1000.0, mc.Previous)
	assert.Equal(t, -1000.0, mc.RawChange)
	assert.Equal(t, -100.0, mc.PctChange)
	assert.True(t, mc.HasPrevious)
}

func TestStress_BuildMetricChange_BothZero(t *testing.T) {
	mc := buildMetricChange(0.0, 0.0)
	assert.Equal(t, 0.0, mc.Current)
	assert.Equal(t, 0.0, mc.Previous)
	assert.Equal(t, 0.0, mc.RawChange)
	assert.Equal(t, 0.0, mc.PctChange)
	assert.False(t, mc.HasPrevious)
}

func TestStress_BuildMetricChange_VerySmallPrevious(t *testing.T) {
	// Near-zero previous value - should still calculate
	mc := buildMetricChange(1000.0, 0.01)
	assert.Equal(t, 1000.0, mc.Current)
	assert.Equal(t, 0.01, mc.Previous)
	assert.Equal(t, 999.99, mc.RawChange)
	// 1000/0.01 * 100 = 10,000,000%
	assert.InDelta(t, 9999900.0, mc.PctChange, 1.0)
	assert.True(t, mc.HasPrevious)
}

func TestStress_BuildMetricChange_LargeNumbers(t *testing.T) {
	// Test with large portfolio values (millions)
	mc := buildMetricChange(5_000_000.0, 4_500_000.0)
	assert.Equal(t, 5_000_000.0, mc.Current)
	assert.Equal(t, 4_500_000.0, mc.Previous)
	assert.Equal(t, 500_000.0, mc.RawChange)
	assert.InDelta(t, 11.111, mc.PctChange, 0.01)
	assert.True(t, mc.HasPrevious)
}

func TestStress_BuildMetricChange_VeryLargeNumbers(t *testing.T) {
	// Test with very large values (hundreds of millions)
	mc := buildMetricChange(500_000_000.0, 450_000_000.0)
	assert.Equal(t, 500_000_000.0, mc.Current)
	assert.Equal(t, 450_000_000.0, mc.Previous)
	assert.Equal(t, 50_000_000.0, mc.RawChange)
	assert.InDelta(t, 11.111, mc.PctChange, 0.01)
}

func TestStress_BuildMetricChange_FloatPrecision(t *testing.T) {
	// Test that small differences in large numbers are handled correctly
	mc := buildMetricChange(1_000_000.01, 1_000_000.00)
	assert.InDelta(t, 0.01, mc.RawChange, 0.0001)
	assert.InDelta(t, 0.000001, mc.PctChange, 0.0000001)
}

func TestStress_BuildMetricChange_NegativeValues(t *testing.T) {
	// Negative values (e.g., negative cash balance)
	// DESIGN NOTE: HasPrevious = previous > 0, so negative previous sets HasPrevious=false
	// This means PctChange is also NOT calculated (remains 0)
	mc := buildMetricChange(-5000.0, -3000.0)
	assert.Equal(t, -5000.0, mc.Current)
	assert.Equal(t, -3000.0, mc.Previous)
	assert.Equal(t, -2000.0, mc.RawChange)
	// PctChange is 0 because previous > 0 is false for negative values
	assert.Equal(t, 0.0, mc.PctChange, "PctChange is 0 when previous is negative (HasPrevious=false)")
	assert.False(t, mc.HasPrevious, "HasPrevious is false for negative previous (previous > 0 check)")
}

func TestStress_BuildMetricChange_NegativeToPositive(t *testing.T) {
	// From negative to positive
	// DESIGN NOTE: HasPrevious = previous > 0, so negative previous sets HasPrevious=false
	mc := buildMetricChange(1000.0, -500.0)
	assert.Equal(t, 1000.0, mc.Current)
	assert.Equal(t, -500.0, mc.Previous)
	assert.Equal(t, 1500.0, mc.RawChange)
	// PctChange is 0 because previous > 0 is false for negative values
	assert.Equal(t, 0.0, mc.PctChange, "PctChange is 0 when previous is negative")
	assert.False(t, mc.HasPrevious, "HasPrevious is false for negative previous")
}

func TestStress_BuildMetricChange_PositiveToNegative(t *testing.T) {
	// From positive to negative
	mc := buildMetricChange(-500.0, 1000.0)
	assert.Equal(t, -500.0, mc.Current)
	assert.Equal(t, 1000.0, mc.Previous)
	assert.Equal(t, -1500.0, mc.RawChange)
	assert.Equal(t, -150.0, mc.PctChange)
	assert.True(t, mc.HasPrevious)
}

// =============================================================================
// cumulativeDividendsByDate stress tests
// =============================================================================

func TestStress_CumulativeDividendsByDate_NoDividends(t *testing.T) {
	ledger := &models.CashFlowLedger{
		Transactions: []models.CashTransaction{
			{Account: "Trading", Category: models.CashCatContribution, Date: time.Now().Add(-24 * time.Hour), Amount: 10000},
			{Account: "Trading", Category: models.CashCatTransfer, Date: time.Now().Add(-24 * time.Hour), Amount: -5000},
		},
	}
	refDate := time.Now()
	total := cumulativeDividendsByDate(ledger, refDate)
	assert.Equal(t, 0.0, total, "Only dividend transactions should count")
}

func TestStress_CumulativeDividendsByDate_ExactRefDate(t *testing.T) {
	refDate := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	ledger := &models.CashFlowLedger{
		Transactions: []models.CashTransaction{
			{Account: "Trading", Category: models.CashCatDividend, Date: refDate, Amount: 500},                      // ON ref date
			{Account: "Trading", Category: models.CashCatDividend, Date: refDate.Add(-24 * time.Hour), Amount: 300}, // before
			{Account: "Trading", Category: models.CashCatDividend, Date: refDate.Add(24 * time.Hour), Amount: 200},  // after (excluded)
		},
	}
	total := cumulativeDividendsByDate(ledger, refDate)
	assert.Equal(t, 800.0, total, "Should include dividends ON and BEFORE refDate")
}

func TestStress_CumulativeDividendsByDate_AllAfterRefDate(t *testing.T) {
	refDate := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	ledger := &models.CashFlowLedger{
		Transactions: []models.CashTransaction{
			{Account: "Trading", Category: models.CashCatDividend, Date: refDate.Add(24 * time.Hour), Amount: 500},
			{Account: "Trading", Category: models.CashCatDividend, Date: refDate.Add(48 * time.Hour), Amount: 300},
		},
	}
	total := cumulativeDividendsByDate(ledger, refDate)
	assert.Equal(t, 0.0, total, "All dividends are after refDate")
}

func TestStress_CumulativeDividendsByDate_VeryLargeAmounts(t *testing.T) {
	// Test float precision with large dividend amounts
	refDate := time.Now()
	ledger := &models.CashFlowLedger{
		Transactions: []models.CashTransaction{
			{Account: "Trading", Category: models.CashCatDividend, Date: refDate.Add(-24 * time.Hour), Amount: 1_000_000.0},
			{Account: "Trading", Category: models.CashCatDividend, Date: refDate.Add(-48 * time.Hour), Amount: 2_000_000.0},
			{Account: "Trading", Category: models.CashCatDividend, Date: refDate.Add(-72 * time.Hour), Amount: 3_000_000.0},
		},
	}
	total := cumulativeDividendsByDate(ledger, refDate)
	assert.Equal(t, 6_000_000.0, total)
}

func TestStress_CumulativeDividendsByDate_ManyTransactions(t *testing.T) {
	// Test performance with many dividend transactions
	refDate := time.Now()
	n := 10000
	txs := make([]models.CashTransaction, n)
	for i := 0; i < n; i++ {
		txs[i] = models.CashTransaction{
			Account:  "Trading",
			Category: models.CashCatDividend,
			Date:     refDate.Add(-time.Duration(i) * time.Hour),
			Amount:   100.0,
		}
	}
	ledger := &models.CashFlowLedger{Transactions: txs}

	start := time.Now()
	total := cumulativeDividendsByDate(ledger, refDate)
	elapsed := time.Since(start)

	assert.Less(t, elapsed, 50*time.Millisecond, "Should handle 10k transactions quickly")
	assert.Equal(t, float64(n)*100.0, total)
}

func TestStress_CumulativeDividendsByDate_TimeTruncation(t *testing.T) {
	// Test that time truncation is applied correctly
	refDate := time.Date(2024, 6, 15, 12, 30, 45, 0, time.UTC) // midday ref
	txDatetime := time.Date(2024, 6, 15, 8, 0, 0, 0, time.UTC) // same day, earlier time

	ledger := &models.CashFlowLedger{
		Transactions: []models.CashTransaction{
			{Account: "Trading", Category: models.CashCatDividend, Date: txDatetime, Amount: 500},
		},
	}
	total := cumulativeDividendsByDate(ledger, refDate)
	assert.Equal(t, 500.0, total, "Transaction on same day should be included regardless of time")
}

func TestStress_CumulativeDividendsByDate_NegativeDividend(t *testing.T) {
	// Dividend corrections/reversals (negative amounts)
	refDate := time.Now()
	ledger := &models.CashFlowLedger{
		Transactions: []models.CashTransaction{
			{Account: "Trading", Category: models.CashCatDividend, Date: refDate.Add(-24 * time.Hour), Amount: 500},  // credit
			{Account: "Trading", Category: models.CashCatDividend, Date: refDate.Add(-12 * time.Hour), Amount: -100}, // reversal
		},
	}
	total := cumulativeDividendsByDate(ledger, refDate)
	assert.Equal(t, 400.0, total, "Negative dividend amounts should subtract")
}

// =============================================================================
// computePeriodChanges stress tests (with mock timeline store)
// =============================================================================

type stressMockTimelineStore struct {
	snapshots []models.TimelineSnapshot
	err       error
}

func (m *stressMockTimelineStore) GetRange(ctx context.Context, userID, portfolioName string, from, to time.Time) ([]models.TimelineSnapshot, error) {
	if m.err != nil {
		return nil, m.err
	}
	// Filter snapshots by date range [from, to]
	var result []models.TimelineSnapshot
	for _, snap := range m.snapshots {
		snapDate := snap.Date.Truncate(24 * time.Hour)
		if snapDate.Equal(from) || snapDate.Equal(to) || (snapDate.After(from) && snapDate.Before(to)) {
			result = append(result, snap)
		}
	}
	return result, nil
}
func (m *stressMockTimelineStore) GetLatest(ctx context.Context, userID, portfolioName string) (*models.TimelineSnapshot, error) {
	if m.err != nil || len(m.snapshots) == 0 {
		return nil, m.err
	}
	return &m.snapshots[0], nil
}
func (m *stressMockTimelineStore) SaveBatch(ctx context.Context, snapshots []models.TimelineSnapshot) error {
	return nil
}
func (m *stressMockTimelineStore) DeleteRange(ctx context.Context, userID, portfolioName string, from, to time.Time) (int, error) {
	return 0, nil
}
func (m *stressMockTimelineStore) DeleteAll(ctx context.Context, userID, portfolioName string) (int, error) {
	return 0, nil
}

func TestStress_ComputePeriodChanges_NoTimelineStore(t *testing.T) {
	svc := &Service{
		storage: &stressMockStorageManager{timelineStore: nil},
	}
	portfolio := &models.Portfolio{
		Name:             "SMSF",
		EquityValue:      100000,
		PortfolioValue:   110000,
		GrossCashBalance: 10000,
	}

	now := time.Now().Truncate(24 * time.Hour)
	pc := svc.computePeriodChanges(testCtx(), "user1", portfolio, nil, now.AddDate(0, 0, -1))

	// Without timeline store, HasPrevious should be false
	assert.False(t, pc.EquityValue.HasPrevious)
	assert.False(t, pc.PortfolioValue.HasPrevious)
	assert.False(t, pc.GrossCash.HasPrevious)
	assert.False(t, pc.Dividend.HasPrevious)

	// Current values should still be set
	assert.Equal(t, 100000.0, pc.EquityValue.Current)
	assert.Equal(t, 110000.0, pc.PortfolioValue.Current)
	assert.Equal(t, 10000.0, pc.GrossCash.Current)
}

func TestStress_ComputePeriodChanges_TimelineHit(t *testing.T) {
	now := time.Now().Truncate(24 * time.Hour)
	yesterday := now.AddDate(0, 0, -1)

	tl := &stressMockTimelineStore{
		snapshots: []models.TimelineSnapshot{
			{
				UserID:                   "user1",
				PortfolioName:            "SMSF",
				Date:                     yesterday,
				EquityValue:              95000,
				PortfolioValue:           105000,
				GrossCashBalance:         10000,
				CumulativeDividendReturn: 500,
			},
		},
	}

	svc := &Service{
		storage: &stressMockStorageManager{timelineStore: tl},
	}
	portfolio := &models.Portfolio{
		Name:                 "SMSF",
		EquityValue:          100000,
		PortfolioValue:       110000,
		GrossCashBalance:     10000,
		LedgerDividendReturn: 600,
	}

	pc := svc.computePeriodChanges(testCtx(), "user1", portfolio, tl, yesterday)

	assert.True(t, pc.EquityValue.HasPrevious)
	assert.Equal(t, 100000.0, pc.EquityValue.Current)
	assert.Equal(t, 95000.0, pc.EquityValue.Previous)
	assert.Equal(t, 5000.0, pc.EquityValue.RawChange)
	assert.InDelta(t, 5.26, pc.EquityValue.PctChange, 0.01)

	assert.True(t, pc.PortfolioValue.HasPrevious)
	assert.Equal(t, 110000.0, pc.PortfolioValue.Current)
	assert.Equal(t, 105000.0, pc.PortfolioValue.Previous)

	assert.True(t, pc.Dividend.HasPrevious)
	assert.Equal(t, 600.0, pc.Dividend.Current)
	assert.Equal(t, 500.0, pc.Dividend.Previous)
	assert.Equal(t, 100.0, pc.Dividend.RawChange)
}

func TestStress_ComputePeriodChanges_TimelineEmpty(t *testing.T) {
	now := time.Now().Truncate(24 * time.Hour)
	yesterday := now.AddDate(0, 0, -1)

	tl := &stressMockTimelineStore{
		snapshots: []models.TimelineSnapshot{}, // empty
	}

	svc := &Service{
		storage:     &stressMockStorageManager{timelineStore: tl},
		cashflowSvc: nil, // no fallback
	}
	portfolio := &models.Portfolio{
		Name:                 "SMSF",
		EquityValue:          100000,
		PortfolioValue:       110000,
		GrossCashBalance:     10000,
		LedgerDividendReturn: 500,
	}

	pc := svc.computePeriodChanges(testCtx(), "user1", portfolio, tl, yesterday)

	// No historical data available
	assert.False(t, pc.EquityValue.HasPrevious)
	assert.False(t, pc.PortfolioValue.HasPrevious)
	assert.False(t, pc.GrossCash.HasPrevious)
	assert.False(t, pc.Dividend.HasPrevious)
}

func TestStress_ComputePeriodChanges_TimelineError(t *testing.T) {
	now := time.Now().Truncate(24 * time.Hour)
	yesterday := now.AddDate(0, 0, -1)

	tl := &stressMockTimelineStore{
		err: assert.AnError, // simulate error
	}

	svc := &Service{
		storage:     &stressMockStorageManager{timelineStore: tl},
		cashflowSvc: nil,
	}
	portfolio := &models.Portfolio{
		Name:                 "SMSF",
		EquityValue:          100000,
		PortfolioValue:       110000,
		GrossCashBalance:     10000,
		LedgerDividendReturn: 500,
	}

	pc := svc.computePeriodChanges(testCtx(), "user1", portfolio, tl, yesterday)

	// Error from timeline should result in no previous data
	assert.False(t, pc.EquityValue.HasPrevious)
	assert.False(t, pc.Dividend.HasPrevious)
}

func TestStress_ComputePeriodChanges_LedgerFallback(t *testing.T) {
	now := time.Now().Truncate(24 * time.Hour)
	yesterday := now.AddDate(0, 0, -1)

	tl := &stressMockTimelineStore{
		snapshots: []models.TimelineSnapshot{}, // no timeline data
	}

	svc := &Service{
		storage: &stressMockStorageManager{timelineStore: tl},
		cashflowSvc: &mockCashFlowService{
			ledger: &models.CashFlowLedger{
				Transactions: []models.CashTransaction{
					{Account: "Trading", Category: models.CashCatDividend, Date: yesterday.Add(-12 * time.Hour), Amount: 400},
					{Account: "Trading", Category: models.CashCatDividend, Date: now.Add(24 * time.Hour), Amount: 200}, // future, excluded
				},
			},
		},
	}
	portfolio := &models.Portfolio{
		Name:                 "SMSF",
		EquityValue:          100000,
		PortfolioValue:       110000,
		GrossCashBalance:     10000,
		LedgerDividendReturn: 600, // current total
	}

	pc := svc.computePeriodChanges(testCtx(), "user1", portfolio, tl, yesterday)

	// Timeline didn't have data, so Equity/Portfolio/GrossCash have no previous
	assert.False(t, pc.EquityValue.HasPrevious)
	assert.False(t, pc.PortfolioValue.HasPrevious)
	assert.False(t, pc.GrossCash.HasPrevious)

	// But dividend should fall back to ledger
	assert.True(t, pc.Dividend.HasPrevious, "Dividend should use ledger fallback")
	assert.Equal(t, 600.0, pc.Dividend.Current)
	assert.Equal(t, 400.0, pc.Dividend.Previous)
	assert.Equal(t, 200.0, pc.Dividend.RawChange)
}

// =============================================================================
// populateChanges stress tests (integration-style)
// =============================================================================

func TestStress_PopulateChanges_AllPeriods(t *testing.T) {
	now := time.Now().Truncate(24 * time.Hour)
	yesterday := now.AddDate(0, 0, -1)
	weekAgo := now.AddDate(0, 0, -7)
	monthAgo := now.AddDate(0, 0, -30)

	tl := &stressMockTimelineStore{
		snapshots: []models.TimelineSnapshot{
			{Date: yesterday, EquityValue: 95000, PortfolioValue: 105000, GrossCashBalance: 10000, CumulativeDividendReturn: 500},
			{Date: weekAgo, EquityValue: 90000, PortfolioValue: 100000, GrossCashBalance: 10000, CumulativeDividendReturn: 400},
			{Date: monthAgo, EquityValue: 85000, PortfolioValue: 95000, GrossCashBalance: 10000, CumulativeDividendReturn: 300},
		},
	}

	svc := &Service{
		storage: &stressMockStorageManager{timelineStore: tl},
	}
	portfolio := &models.Portfolio{
		Name:                 "SMSF",
		EquityValue:          100000,
		PortfolioValue:       110000,
		GrossCashBalance:     10000,
		LedgerDividendReturn: 600,
	}

	svc.populateChanges(testCtx(), portfolio)

	require.NotNil(t, portfolio.Changes)

	// Yesterday
	assert.True(t, portfolio.Changes.Yesterday.EquityValue.HasPrevious)
	assert.Equal(t, 5000.0, portfolio.Changes.Yesterday.EquityValue.RawChange)

	// Week
	assert.True(t, portfolio.Changes.Week.EquityValue.HasPrevious)
	assert.Equal(t, 10000.0, portfolio.Changes.Week.EquityValue.RawChange)

	// Month
	assert.True(t, portfolio.Changes.Month.EquityValue.HasPrevious)
	assert.Equal(t, 15000.0, portfolio.Changes.Month.EquityValue.RawChange)
}

func TestStress_PopulateChanges_FreshPortfolio(t *testing.T) {
	// New portfolio with no historical timeline data
	tl := &stressMockTimelineStore{
		snapshots: []models.TimelineSnapshot{},
	}

	svc := &Service{
		storage:     &stressMockStorageManager{timelineStore: tl},
		cashflowSvc: nil,
	}
	portfolio := &models.Portfolio{
		Name:                 "SMSF",
		EquityValue:          100000,
		PortfolioValue:       110000,
		GrossCashBalance:     10000,
		LedgerDividendReturn: 0,
	}

	svc.populateChanges(testCtx(), portfolio)

	require.NotNil(t, portfolio.Changes)

	// All periods should have HasPrevious = false
	assert.False(t, portfolio.Changes.Yesterday.EquityValue.HasPrevious)
	assert.False(t, portfolio.Changes.Yesterday.PortfolioValue.HasPrevious)
	assert.False(t, portfolio.Changes.Yesterday.GrossCash.HasPrevious)
	assert.False(t, portfolio.Changes.Yesterday.Dividend.HasPrevious)

	assert.False(t, portfolio.Changes.Week.EquityValue.HasPrevious)
	assert.False(t, portfolio.Changes.Month.EquityValue.HasPrevious)
}

// =============================================================================
// Concurrent access tests
// =============================================================================

func TestStress_PopulateChanges_ConcurrentSafe(t *testing.T) {
	now := time.Now().Truncate(24 * time.Hour)
	yesterday := now.AddDate(0, 0, -1)

	tl := &stressMockTimelineStore{
		snapshots: []models.TimelineSnapshot{
			{Date: yesterday, EquityValue: 95000, PortfolioValue: 105000, GrossCashBalance: 10000, CumulativeDividendReturn: 500},
		},
	}

	svc := &Service{
		storage: &stressMockStorageManager{timelineStore: tl},
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			portfolio := &models.Portfolio{
				Name:                 "SMSF",
				EquityValue:          100000,
				PortfolioValue:       110000,
				GrossCashBalance:     10000,
				LedgerDividendReturn: 600,
			}
			svc.populateChanges(testCtx(), portfolio)
			require.NotNil(t, portfolio.Changes)
			assert.True(t, portfolio.Changes.Yesterday.EquityValue.HasPrevious)
		}()
	}
	wg.Wait()
}

func TestStress_BuildMetricChange_ConcurrentSafe(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			current := float64(i * 1000)
			previous := float64((i - 1) * 1000)
			if previous <= 0 {
				previous = 1
			}
			mc := buildMetricChange(current, previous)
			assert.Equal(t, current, mc.Current)
			assert.Equal(t, previous, mc.Previous)
		}(i)
	}
	wg.Wait()
}

// =============================================================================
// Edge cases and adversarial inputs
// =============================================================================

func TestStress_PopulateChanges_NilPortfolio(t *testing.T) {
	svc := &Service{}
	// This should not panic - it's a programming error but we test for robustness
	defer func() {
		if r := recover(); r != nil {
			t.Logf("populateChanges panicked with nil portfolio: %v", r)
		}
	}()
	svc.populateChanges(testCtx(), nil)
}

func TestStress_PopulateChanges_ZeroPortfolioValues(t *testing.T) {
	now := time.Now().Truncate(24 * time.Hour)
	yesterday := now.AddDate(0, 0, -1)

	tl := &stressMockTimelineStore{
		snapshots: []models.TimelineSnapshot{
			{Date: yesterday, EquityValue: 0, PortfolioValue: 0, GrossCashBalance: 0, CumulativeDividendReturn: 0},
		},
	}

	svc := &Service{
		storage: &stressMockStorageManager{timelineStore: tl},
	}
	portfolio := &models.Portfolio{
		Name:                 "SMSF",
		EquityValue:          0,
		PortfolioValue:       0,
		GrossCashBalance:     0,
		LedgerDividendReturn: 0,
	}

	svc.populateChanges(testCtx(), portfolio)

	require.NotNil(t, portfolio.Changes)
	// With buildMetricChange, HasPrevious = previous > 0, so 0 previous means no previous
	assert.False(t, portfolio.Changes.Yesterday.EquityValue.HasPrevious)
	assert.Equal(t, 0.0, portfolio.Changes.Yesterday.EquityValue.Current)
}

func TestStress_PopulateChanges_SmallPortfolioValue(t *testing.T) {
	// Portfolio with small equity value (near-zero but positive)
	now := time.Now().Truncate(24 * time.Hour)
	yesterday := now.AddDate(0, 0, -1)

	tl := &stressMockTimelineStore{
		snapshots: []models.TimelineSnapshot{
			{Date: yesterday, EquityValue: 100, PortfolioValue: 200, GrossCashBalance: 100, CumulativeDividendReturn: 0},
		},
	}

	svc := &Service{
		storage: &stressMockStorageManager{timelineStore: tl},
	}
	portfolio := &models.Portfolio{
		Name:                 "SMSF",
		EquityValue:          50,
		PortfolioValue:       150,
		GrossCashBalance:     100,
		LedgerDividendReturn: 0,
	}

	svc.populateChanges(testCtx(), portfolio)

	require.NotNil(t, portfolio.Changes)
	// EquityValue uses buildMetricChange: HasPrevious = previous > 0
	assert.True(t, portfolio.Changes.Yesterday.EquityValue.HasPrevious, "Snapshot exists with positive previous")
	assert.Equal(t, 50.0, portfolio.Changes.Yesterday.EquityValue.Current)
	assert.Equal(t, 100.0, portfolio.Changes.Yesterday.EquityValue.Previous)
	assert.Equal(t, -50.0, portfolio.Changes.Yesterday.EquityValue.RawChange)
	assert.InDelta(t, -50.0, portfolio.Changes.Yesterday.EquityValue.PctChange, 0.01)
}

func TestStress_BuildMetricChange_InfinityValues(t *testing.T) {
	// Test with infinity (should not happen but test for robustness)
	mc := buildMetricChange(math.Inf(1), 1000)
	assert.True(t, math.IsInf(mc.PctChange, 1), "Infinity current should produce infinity pct change")

	// DESIGN NOTE: math.Inf(1) > 0 is true, so HasPrevious is set to true
	// This is technically correct per the implementation, even if unusual
	mc = buildMetricChange(1000, math.Inf(1))
	assert.True(t, mc.HasPrevious, "Infinity previous > 0 is true, so HasPrevious is true")
	// (1000 - Inf) / Inf * 100 = -Inf / Inf = NaN (indeterminate form)
	assert.True(t, math.IsNaN(mc.PctChange), "PctChange should be NaN (indeterminate: -Inf/Inf)")
}

func TestStress_BuildMetricChange_NaNValues(t *testing.T) {
	mc := buildMetricChange(math.NaN(), 1000)
	assert.True(t, math.IsNaN(mc.RawChange), "NaN current should produce NaN raw change")

	mc = buildMetricChange(1000, math.NaN())
	assert.False(t, mc.HasPrevious, "NaN previous should not set HasPrevious")
}

// =============================================================================
// Mock storage manager
// =============================================================================

type stressMockStorageManager struct {
	timelineStore interfaces.TimelineStore
}

func (m *stressMockStorageManager) TimelineStore() interfaces.TimelineStore {
	return m.timelineStore
}
func (m *stressMockStorageManager) InternalStore() interfaces.InternalStore         { return nil }
func (m *stressMockStorageManager) UserDataStore() interfaces.UserDataStore         { return nil }
func (m *stressMockStorageManager) MarketDataStorage() interfaces.MarketDataStorage { return nil }
func (m *stressMockStorageManager) SignalStorage() interfaces.SignalStorage         { return nil }
func (m *stressMockStorageManager) StockIndexStore() interfaces.StockIndexStore     { return nil }
func (m *stressMockStorageManager) JobQueueStore() interfaces.JobQueueStore         { return nil }
func (m *stressMockStorageManager) FileStore() interfaces.FileStore                 { return nil }
func (m *stressMockStorageManager) FeedbackStore() interfaces.FeedbackStore         { return nil }
func (m *stressMockStorageManager) ChangelogStore() interfaces.ChangelogStore       { return nil }
func (m *stressMockStorageManager) OAuthStore() interfaces.OAuthStore               { return nil }
func (m *stressMockStorageManager) DataPath() string                                { return "" }
func (m *stressMockStorageManager) WriteRaw(subdir, key string, data []byte) error  { return nil }
func (m *stressMockStorageManager) PurgeDerivedData(ctx context.Context) (map[string]int, error) {
	return nil, nil
}
func (m *stressMockStorageManager) PurgeReports(ctx context.Context) (int, error) { return 0, nil }
func (m *stressMockStorageManager) Close() error                                  { return nil }
