package portfolio

import (
	"context"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
)

// Unit tests for buildMetricChange, buildSignedMetricChange, and computePeriodChanges.

func TestBuildSignedMetricChange_PositiveValues(t *testing.T) {
	mc := buildSignedMetricChange(1500.0, 1000.0, true)
	assert.Equal(t, 1500.0, mc.Current)
	assert.Equal(t, 1000.0, mc.Previous)
	assert.True(t, mc.HasPrevious)
	assert.Equal(t, 500.0, mc.RawChange)
	assert.InDelta(t, 50.0, mc.PctChange, 0.001)
}

func TestBuildSignedMetricChange_NegativeValues(t *testing.T) {
	mc := buildSignedMetricChange(-1500.0, -1000.0, true)
	assert.Equal(t, -1500.0, mc.Current)
	assert.Equal(t, -1000.0, mc.Previous)
	assert.True(t, mc.HasPrevious)
	assert.Equal(t, -500.0, mc.RawChange)
	assert.InDelta(t, -50.0, mc.PctChange, 0.001)
}

func TestBuildSignedMetricChange_CrossZero(t *testing.T) {
	mc := buildSignedMetricChange(-200.0, 500.0, true)
	assert.Equal(t, -200.0, mc.Current)
	assert.Equal(t, 500.0, mc.Previous)
	assert.True(t, mc.HasPrevious)
	assert.Equal(t, -700.0, mc.RawChange)
	assert.InDelta(t, -140.0, mc.PctChange, 0.001)
}

func TestBuildSignedMetricChange_ZeroPrevious(t *testing.T) {
	mc := buildSignedMetricChange(1000.0, 0.0, true)
	assert.Equal(t, 1000.0, mc.Current)
	assert.Equal(t, 0.0, mc.Previous)
	assert.True(t, mc.HasPrevious)
	assert.Equal(t, 1000.0, mc.RawChange)
	assert.Equal(t, 0.0, mc.PctChange, "PctChange must be 0 when previous is 0 (no divide-by-zero)")
}

func TestBuildSignedMetricChange_NoPrevious(t *testing.T) {
	mc := buildSignedMetricChange(1000.0, 0.0, false)
	assert.Equal(t, 1000.0, mc.Current)
	assert.False(t, mc.HasPrevious)
	assert.Equal(t, 1000.0, mc.RawChange)
	assert.Equal(t, 0.0, mc.PctChange)
}

func TestComputePeriodChanges_HasEquityValue(t *testing.T) {
	// Verify PeriodChanges contains EquityValue (market value change, not P&L change)
	now := time.Now().Truncate(24 * time.Hour)
	yesterday := now.AddDate(0, 0, -1)

	tl := &stressMockTimelineStore{
		snapshots: []models.TimelineSnapshot{
			{
				UserID:           "user1",
				PortfolioName:    "SMSF",
				Date:             yesterday,
				EquityValue:      95000.0,
				PortfolioValue:   105000.0,
				GrossCashBalance: 10000.0,
			},
		},
	}

	svc := &Service{
		storage: &stressMockStorageManager{timelineStore: tl},
	}
	portfolio := &models.Portfolio{
		Name:                 "SMSF",
		EquityValue:          100000.0,
		PortfolioValue:       110000.0,
		GrossCashBalance:     10000.0,
		LedgerDividendReturn: 600.0,
	}

	pc := svc.computePeriodChanges(context.Background(), "user1", portfolio, tl, yesterday)

	// EquityValue should show market value change
	assert.True(t, pc.EquityValue.HasPrevious, "EquityValue.HasPrevious should be true when snapshot exists")
	assert.Equal(t, 100000.0, pc.EquityValue.Current)
	assert.Equal(t, 95000.0, pc.EquityValue.Previous)
	assert.Equal(t, 5000.0, pc.EquityValue.RawChange)
	assert.InDelta(t, 5.26, pc.EquityValue.PctChange, 0.01)

	// PortfolioValue should also be populated
	assert.True(t, pc.PortfolioValue.HasPrevious)
	assert.Equal(t, 5000.0, pc.PortfolioValue.RawChange)
}

func TestComputePeriodChanges_NoSnapshot(t *testing.T) {
	now := time.Now().Truncate(24 * time.Hour)
	yesterday := now.AddDate(0, 0, -1)

	tl := &stressMockTimelineStore{
		snapshots: []models.TimelineSnapshot{},
	}

	svc := &Service{
		storage:     &stressMockStorageManager{timelineStore: tl},
		cashflowSvc: nil,
	}
	portfolio := &models.Portfolio{
		Name:                 "SMSF",
		EquityValue:          100000.0,
		PortfolioValue:       110000.0,
		GrossCashBalance:     10000.0,
		LedgerDividendReturn: 600.0,
	}

	pc := svc.computePeriodChanges(context.Background(), "user1", portfolio, tl, yesterday)

	assert.False(t, pc.EquityValue.HasPrevious, "HasPrevious should be false when no snapshot")
	assert.False(t, pc.PortfolioValue.HasPrevious)
	assert.False(t, pc.GrossCash.HasPrevious)
	assert.False(t, pc.Dividend.HasPrevious)

	// Current values should still be set
	assert.Equal(t, 100000.0, pc.EquityValue.Current)
	assert.Equal(t, 110000.0, pc.PortfolioValue.Current)
}
