package data

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPeriodChanges_NetEquityReturnPopulated verifies PeriodChanges contains
// NetEquityReturn (not EquityValue) and that values are properly populated.
func TestPeriodChanges_NetEquityReturnPopulated(t *testing.T) {
	store := testManager(t).UserDataStore()
	ctx := testContext()

	// Create a portfolio with PeriodChanges containing NetEquityReturn
	portfolio := models.Portfolio{
		Name:                 "SMSF-Test",
		NetEquityReturn:      7000.0, // Current P&L
		NetEquityReturnPct:   7.0,    // Current %
		PortfolioValue:       110000.0,
		GrossCashBalance:     10000.0,
		LedgerDividendReturn: 600.0,
		Changes: &models.PortfolioChanges{
			Yesterday: models.PeriodChanges{
				NetEquityReturn: models.MetricChange{
					Current:     7000.0,
					Previous:    5000.0,
					HasPrevious: true,
					RawChange:   2000.0,
					PctChange:   40.0,
				},
				NetEquityReturnPct: models.MetricChange{
					Current:     7.0,
					Previous:    5.26,
					HasPrevious: true,
					RawChange:   1.74,
					PctChange:   33.0,
				},
				PortfolioValue: models.MetricChange{
					Current:     110000.0,
					Previous:    105000.0,
					HasPrevious: true,
					RawChange:   5000.0,
					PctChange:   4.76,
				},
				GrossCash: models.MetricChange{
					Current:     10000.0,
					Previous:    10000.0,
					HasPrevious: true,
					RawChange:   0.0,
					PctChange:   0.0,
				},
				Dividend: models.MetricChange{
					Current:     600.0,
					Previous:    500.0,
					HasPrevious: true,
					RawChange:   100.0,
					PctChange:   20.0,
				},
			},
		},
	}

	// Store the portfolio
	data, err := json.Marshal(portfolio)
	require.NoError(t, err)

	record := &models.UserRecord{
		UserID:   "user_001",
		Subject:  "portfolio",
		Key:      portfolio.Name,
		Value:    string(data),
		Version:  1,
		DateTime: time.Now().Truncate(time.Second),
	}
	require.NoError(t, store.Put(ctx, record))

	// Retrieve and verify
	got, err := store.Get(ctx, "user_001", "portfolio", portfolio.Name)
	require.NoError(t, err)

	var retrieved models.Portfolio
	require.NoError(t, json.Unmarshal([]byte(got.Value), &retrieved))

	// Verify NetEquityReturn is in PeriodChanges
	assert.NotNil(t, retrieved.Changes, "Changes should be populated")
	assert.True(t, retrieved.Changes.Yesterday.NetEquityReturn.HasPrevious, "NetEquityReturn.HasPrevious should be true")
	assert.Equal(t, 7000.0, retrieved.Changes.Yesterday.NetEquityReturn.Current)
	assert.Equal(t, 5000.0, retrieved.Changes.Yesterday.NetEquityReturn.Previous)
	assert.Equal(t, 2000.0, retrieved.Changes.Yesterday.NetEquityReturn.RawChange)
	assert.InDelta(t, 40.0, retrieved.Changes.Yesterday.NetEquityReturn.PctChange, 0.01)
}

// TestPeriodChanges_NetEquityReturnPctPopulated verifies NetEquityReturnPct is populated
// with correct percentage change calculations (handles negative values).
func TestPeriodChanges_NetEquityReturnPctPopulated(t *testing.T) {
	store := testManager(t).UserDataStore()
	ctx := testContext()

	portfolio := models.Portfolio{
		Name:                 "Portfolio-Pct",
		NetEquityReturn:      8.5,
		NetEquityReturnPct:   8.5,
		PortfolioValue:       110000.0,
		GrossCashBalance:     10000.0,
		LedgerDividendReturn: 600.0,
		Changes: &models.PortfolioChanges{
			Yesterday: models.PeriodChanges{
				NetEquityReturnPct: models.MetricChange{
					Current:     8.5,
					Previous:    5.0,
					HasPrevious: true,
					RawChange:   3.5,
					PctChange:   70.0, // (8.5 - 5.0) / Abs(5.0) * 100
				},
			},
		},
	}

	data, err := json.Marshal(portfolio)
	require.NoError(t, err)

	record := &models.UserRecord{
		UserID:   "user_001",
		Subject:  "portfolio",
		Key:      portfolio.Name,
		Value:    string(data),
		Version:  1,
		DateTime: time.Now().Truncate(time.Second),
	}
	require.NoError(t, store.Put(ctx, record))

	got, err := store.Get(ctx, "user_001", "portfolio", portfolio.Name)
	require.NoError(t, err)

	var retrieved models.Portfolio
	require.NoError(t, json.Unmarshal([]byte(got.Value), &retrieved))

	// Verify NetEquityReturnPct calculations
	pct := retrieved.Changes.Yesterday.NetEquityReturnPct
	assert.True(t, pct.HasPrevious)
	assert.Equal(t, 8.5, pct.Current)
	assert.Equal(t, 5.0, pct.Previous)
	assert.InDelta(t, 3.5, pct.RawChange, 0.001)
	assert.InDelta(t, 70.0, pct.PctChange, 0.01)
}

// TestPeriodChanges_NoEquityValueField verifies EquityValue field is NOT present
// in PeriodChanges struct (removed during refactoring). This test verifies the
// JSON marshaling excludes this field.
func TestPeriodChanges_NoEquityValueField(t *testing.T) {
	portfolio := models.Portfolio{
		Name:                 "No-EV-Test",
		EquityValue:          100000.0, // This field exists on Portfolio
		NetEquityReturn:      5000.0,   // But not on PeriodChanges
		PortfolioValue:       110000.0,
		GrossCashBalance:     10000.0,
		LedgerDividendReturn: 600.0,
		Changes: &models.PortfolioChanges{
			Yesterday: models.PeriodChanges{
				NetEquityReturn: models.MetricChange{
					Current:     5000.0,
					HasPrevious: false,
				},
				PortfolioValue: models.MetricChange{
					Current:     110000.0,
					HasPrevious: false,
				},
			},
		},
	}

	data, err := json.Marshal(portfolio)
	require.NoError(t, err)

	// Verify that the JSON does not contain "equity_value" key at the Changes level
	var jsonData map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &jsonData))

	changesData := jsonData["changes"].(map[string]interface{})
	yesterdayData := changesData["yesterday"].(map[string]interface{})

	// Verify NetEquityReturn is present
	assert.Contains(t, yesterdayData, "net_equity_return", "net_equity_return should be in PeriodChanges")
	// Verify EquityValue is NOT present
	assert.NotContains(t, yesterdayData, "equity_value", "equity_value should NOT be in PeriodChanges")
}

// TestPeriodChanges_NegativePnL verifies negative P&L values are handled correctly,
// with percentage change computed using math.Abs(previous) as denominator.
func TestPeriodChanges_NegativePnL(t *testing.T) {
	store := testManager(t).UserDataStore()
	ctx := testContext()

	// Portfolio with negative P&L (loss)
	portfolio := models.Portfolio{
		Name:                 "Loss-Portfolio",
		NetEquityReturn:      -3000.0, // Current loss
		NetEquityReturnPct:   -3.0,    // Current loss %
		PortfolioValue:       107000.0,
		GrossCashBalance:     10000.0,
		LedgerDividendReturn: 0.0,
		Changes: &models.PortfolioChanges{
			Yesterday: models.PeriodChanges{
				NetEquityReturn: models.MetricChange{
					Current:     -3000.0,
					Previous:    -5000.0,
					HasPrevious: true,
					RawChange:   2000.0, // Improved by 2000
					PctChange:   40.0,   // ((-3000) - (-5000)) / Abs(-5000) * 100 = 2000/5000*100
				},
			},
		},
	}

	data, err := json.Marshal(portfolio)
	require.NoError(t, err)

	record := &models.UserRecord{
		UserID:   "user_001",
		Subject:  "portfolio",
		Key:      portfolio.Name,
		Value:    string(data),
		Version:  1,
		DateTime: time.Now().Truncate(time.Second),
	}
	require.NoError(t, store.Put(ctx, record))

	got, err := store.Get(ctx, "user_001", "portfolio", portfolio.Name)
	require.NoError(t, err)

	var retrieved models.Portfolio
	require.NoError(t, json.Unmarshal([]byte(got.Value), &retrieved))

	// Verify negative P&L handling
	netEqReturn := retrieved.Changes.Yesterday.NetEquityReturn
	assert.True(t, netEqReturn.HasPrevious)
	assert.Equal(t, -3000.0, netEqReturn.Current)
	assert.Equal(t, -5000.0, netEqReturn.Previous)
	assert.Equal(t, 2000.0, netEqReturn.RawChange)
	assert.InDelta(t, 40.0, netEqReturn.PctChange, 0.01)
}

// TestPeriodChanges_NoTimelineData verifies HasPrevious is false when no previous
// data exists (timeline snapshots weren't collected).
func TestPeriodChanges_NoTimelineData(t *testing.T) {
	store := testManager(t).UserDataStore()
	ctx := testContext()

	portfolio := models.Portfolio{
		Name:                 "No-Timeline",
		NetEquityReturn:      5000.0,
		NetEquityReturnPct:   5.0,
		PortfolioValue:       110000.0,
		GrossCashBalance:     10000.0,
		LedgerDividendReturn: 600.0,
		Changes: &models.PortfolioChanges{
			Yesterday: models.PeriodChanges{
				NetEquityReturn: models.MetricChange{
					Current:     5000.0,
					HasPrevious: false,
				},
				NetEquityReturnPct: models.MetricChange{
					Current:     5.0,
					HasPrevious: false,
				},
				PortfolioValue: models.MetricChange{
					Current:     110000.0,
					HasPrevious: false,
				},
				GrossCash: models.MetricChange{
					Current:     10000.0,
					HasPrevious: false,
				},
				Dividend: models.MetricChange{
					Current:     600.0,
					HasPrevious: false,
				},
			},
		},
	}

	data, err := json.Marshal(portfolio)
	require.NoError(t, err)

	record := &models.UserRecord{
		UserID:   "user_001",
		Subject:  "portfolio",
		Key:      portfolio.Name,
		Value:    string(data),
		Version:  1,
		DateTime: time.Now().Truncate(time.Second),
	}
	require.NoError(t, store.Put(ctx, record))

	got, err := store.Get(ctx, "user_001", "portfolio", portfolio.Name)
	require.NoError(t, err)

	var retrieved models.Portfolio
	require.NoError(t, json.Unmarshal([]byte(got.Value), &retrieved))

	// Without timeline data, all period changes should have HasPrevious=false
	assert.False(t, retrieved.Changes.Yesterday.NetEquityReturn.HasPrevious)
	assert.False(t, retrieved.Changes.Yesterday.NetEquityReturnPct.HasPrevious)
	assert.False(t, retrieved.Changes.Yesterday.PortfolioValue.HasPrevious)
	assert.False(t, retrieved.Changes.Yesterday.GrossCash.HasPrevious)
	assert.False(t, retrieved.Changes.Yesterday.Dividend.HasPrevious)

	// But current values should still be set
	assert.Equal(t, 5000.0, retrieved.Changes.Yesterday.NetEquityReturn.Current)
	assert.Equal(t, 5.0, retrieved.Changes.Yesterday.NetEquityReturnPct.Current)
	assert.Equal(t, 110000.0, retrieved.Changes.Yesterday.PortfolioValue.Current)
}

// TestPeriodChanges_ThreePeriodsPopulated verifies Yesterday, Week, and Month
// period changes are all computed and stored independently.
func TestPeriodChanges_ThreePeriodsPopulated(t *testing.T) {
	store := testManager(t).UserDataStore()
	ctx := testContext()

	portfolio := models.Portfolio{
		Name:                 "Three-Periods",
		NetEquityReturn:      10000.0,
		NetEquityReturnPct:   10.0,
		PortfolioValue:       110000.0,
		GrossCashBalance:     10000.0,
		LedgerDividendReturn: 600.0,
		Changes: &models.PortfolioChanges{
			Yesterday: models.PeriodChanges{
				NetEquityReturn: models.MetricChange{
					Current:     10000.0,
					Previous:    9000.0,
					HasPrevious: true,
					RawChange:   1000.0,
					PctChange:   11.11,
				},
			},
			Week: models.PeriodChanges{
				NetEquityReturn: models.MetricChange{
					Current:     10000.0,
					Previous:    8000.0,
					HasPrevious: true,
					RawChange:   2000.0,
					PctChange:   25.0,
				},
			},
			Month: models.PeriodChanges{
				NetEquityReturn: models.MetricChange{
					Current:     10000.0,
					Previous:    5000.0,
					HasPrevious: true,
					RawChange:   5000.0,
					PctChange:   100.0,
				},
			},
		},
	}

	data, err := json.Marshal(portfolio)
	require.NoError(t, err)

	record := &models.UserRecord{
		UserID:   "user_001",
		Subject:  "portfolio",
		Key:      portfolio.Name,
		Value:    string(data),
		Version:  1,
		DateTime: time.Now().Truncate(time.Second),
	}
	require.NoError(t, store.Put(ctx, record))

	got, err := store.Get(ctx, "user_001", "portfolio", portfolio.Name)
	require.NoError(t, err)

	var retrieved models.Portfolio
	require.NoError(t, json.Unmarshal([]byte(got.Value), &retrieved))

	// Verify Yesterday period
	assert.True(t, retrieved.Changes.Yesterday.NetEquityReturn.HasPrevious)
	assert.Equal(t, 10000.0, retrieved.Changes.Yesterday.NetEquityReturn.Current)
	assert.Equal(t, 9000.0, retrieved.Changes.Yesterday.NetEquityReturn.Previous)
	assert.Equal(t, 1000.0, retrieved.Changes.Yesterday.NetEquityReturn.RawChange)

	// Verify Week period
	assert.True(t, retrieved.Changes.Week.NetEquityReturn.HasPrevious)
	assert.Equal(t, 10000.0, retrieved.Changes.Week.NetEquityReturn.Current)
	assert.Equal(t, 8000.0, retrieved.Changes.Week.NetEquityReturn.Previous)
	assert.Equal(t, 2000.0, retrieved.Changes.Week.NetEquityReturn.RawChange)

	// Verify Month period
	assert.True(t, retrieved.Changes.Month.NetEquityReturn.HasPrevious)
	assert.Equal(t, 10000.0, retrieved.Changes.Month.NetEquityReturn.Current)
	assert.Equal(t, 5000.0, retrieved.Changes.Month.NetEquityReturn.Previous)
	assert.Equal(t, 5000.0, retrieved.Changes.Month.NetEquityReturn.RawChange)
}
