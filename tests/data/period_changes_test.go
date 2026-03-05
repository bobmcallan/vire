package data

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPeriodChanges_EquityValuePopulated verifies PeriodChanges contains
// EquityValue (market value change) and that values are properly populated.
func TestPeriodChanges_EquityValuePopulated(t *testing.T) {
	store := testManager(t).UserDataStore()
	ctx := testContext()

	portfolio := models.Portfolio{
		Name:                 "SMSF-Test",
		EquityValue:          100000.0,
		PortfolioValue:       110000.0,
		GrossCashBalance:     10000.0,
		LedgerDividendReturn: 600.0,
		Changes: &models.PortfolioChanges{
			Yesterday: models.PeriodChanges{
				EquityValue: models.MetricChange{
					Current:     100000.0,
					Previous:    95000.0,
					HasPrevious: true,
					RawChange:   5000.0,
					PctChange:   5.26,
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

	assert.NotNil(t, retrieved.Changes, "Changes should be populated")
	assert.True(t, retrieved.Changes.Yesterday.EquityValue.HasPrevious, "EquityValue.HasPrevious should be true")
	assert.Equal(t, 100000.0, retrieved.Changes.Yesterday.EquityValue.Current)
	assert.Equal(t, 95000.0, retrieved.Changes.Yesterday.EquityValue.Previous)
	assert.Equal(t, 5000.0, retrieved.Changes.Yesterday.EquityValue.RawChange)
	assert.InDelta(t, 5.26, retrieved.Changes.Yesterday.EquityValue.PctChange, 0.01)
}

// TestPeriodChanges_EquityValueInJSON verifies the JSON field name is "equity_value"
// and that "net_equity_return" is NOT present in PeriodChanges.
func TestPeriodChanges_EquityValueInJSON(t *testing.T) {
	portfolio := models.Portfolio{
		Name:                 "EV-JSON-Test",
		EquityValue:          100000.0,
		PortfolioValue:       110000.0,
		GrossCashBalance:     10000.0,
		LedgerDividendReturn: 600.0,
		Changes: &models.PortfolioChanges{
			Yesterday: models.PeriodChanges{
				EquityValue: models.MetricChange{
					Current:     100000.0,
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

	var jsonData map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &jsonData))

	changesData := jsonData["changes"].(map[string]interface{})
	yesterdayData := changesData["yesterday"].(map[string]interface{})

	// Verify EquityValue IS present
	assert.Contains(t, yesterdayData, "equity_value", "equity_value should be in PeriodChanges")
	// Verify NetEquityReturn is NOT present
	assert.NotContains(t, yesterdayData, "net_equity_return", "net_equity_return should NOT be in PeriodChanges")
	assert.NotContains(t, yesterdayData, "net_equity_return_pct", "net_equity_return_pct should NOT be in PeriodChanges")
}

// TestPeriodChanges_NoTimelineData verifies HasPrevious is false when no previous
// data exists (timeline snapshots weren't collected).
func TestPeriodChanges_NoTimelineData(t *testing.T) {
	store := testManager(t).UserDataStore()
	ctx := testContext()

	portfolio := models.Portfolio{
		Name:                 "No-Timeline",
		EquityValue:          100000.0,
		PortfolioValue:       110000.0,
		GrossCashBalance:     10000.0,
		LedgerDividendReturn: 600.0,
		Changes: &models.PortfolioChanges{
			Yesterday: models.PeriodChanges{
				EquityValue: models.MetricChange{
					Current:     100000.0,
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

	assert.False(t, retrieved.Changes.Yesterday.EquityValue.HasPrevious)
	assert.False(t, retrieved.Changes.Yesterday.PortfolioValue.HasPrevious)
	assert.False(t, retrieved.Changes.Yesterday.GrossCash.HasPrevious)
	assert.False(t, retrieved.Changes.Yesterday.Dividend.HasPrevious)

	// But current values should still be set
	assert.Equal(t, 100000.0, retrieved.Changes.Yesterday.EquityValue.Current)
	assert.Equal(t, 110000.0, retrieved.Changes.Yesterday.PortfolioValue.Current)
}

// TestPeriodChanges_ThreePeriodsPopulated verifies Yesterday, Week, and Month
// period changes are all computed and stored independently.
func TestPeriodChanges_ThreePeriodsPopulated(t *testing.T) {
	store := testManager(t).UserDataStore()
	ctx := testContext()

	portfolio := models.Portfolio{
		Name:                 "Three-Periods",
		EquityValue:          110000.0,
		PortfolioValue:       120000.0,
		GrossCashBalance:     10000.0,
		LedgerDividendReturn: 600.0,
		Changes: &models.PortfolioChanges{
			Yesterday: models.PeriodChanges{
				EquityValue: models.MetricChange{
					Current:     110000.0,
					Previous:    108000.0,
					HasPrevious: true,
					RawChange:   2000.0,
					PctChange:   1.85,
				},
			},
			Week: models.PeriodChanges{
				EquityValue: models.MetricChange{
					Current:     110000.0,
					Previous:    105000.0,
					HasPrevious: true,
					RawChange:   5000.0,
					PctChange:   4.76,
				},
			},
			Month: models.PeriodChanges{
				EquityValue: models.MetricChange{
					Current:     110000.0,
					Previous:    100000.0,
					HasPrevious: true,
					RawChange:   10000.0,
					PctChange:   10.0,
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
	assert.True(t, retrieved.Changes.Yesterday.EquityValue.HasPrevious)
	assert.Equal(t, 110000.0, retrieved.Changes.Yesterday.EquityValue.Current)
	assert.Equal(t, 108000.0, retrieved.Changes.Yesterday.EquityValue.Previous)
	assert.Equal(t, 2000.0, retrieved.Changes.Yesterday.EquityValue.RawChange)

	// Verify Week period
	assert.True(t, retrieved.Changes.Week.EquityValue.HasPrevious)
	assert.Equal(t, 110000.0, retrieved.Changes.Week.EquityValue.Current)
	assert.Equal(t, 105000.0, retrieved.Changes.Week.EquityValue.Previous)
	assert.Equal(t, 5000.0, retrieved.Changes.Week.EquityValue.RawChange)

	// Verify Month period
	assert.True(t, retrieved.Changes.Month.EquityValue.HasPrevious)
	assert.Equal(t, 110000.0, retrieved.Changes.Month.EquityValue.Current)
	assert.Equal(t, 100000.0, retrieved.Changes.Month.EquityValue.Previous)
	assert.Equal(t, 10000.0, retrieved.Changes.Month.EquityValue.RawChange)
}
