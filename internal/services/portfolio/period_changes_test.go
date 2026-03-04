package portfolio

import (
	"context"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
)

// Unit tests for buildSignedMetricChange and computePeriodChanges (Net Return D/W/M).

func TestBuildSignedMetricChange_PositiveValues(t *testing.T) {
	// Standard case: both current and previous positive
	mc := buildSignedMetricChange(1500.0, 1000.0, true)
	assert.Equal(t, 1500.0, mc.Current)
	assert.Equal(t, 1000.0, mc.Previous)
	assert.True(t, mc.HasPrevious)
	assert.Equal(t, 500.0, mc.RawChange)
	assert.InDelta(t, 50.0, mc.PctChange, 0.001)
}

func TestBuildSignedMetricChange_NegativeValues(t *testing.T) {
	// Both values negative (portfolio in loss)
	// PctChange uses Abs denominator to handle sign correctly
	mc := buildSignedMetricChange(-1500.0, -1000.0, true)
	assert.Equal(t, -1500.0, mc.Current)
	assert.Equal(t, -1000.0, mc.Previous)
	assert.True(t, mc.HasPrevious)
	assert.Equal(t, -500.0, mc.RawChange)
	// (-1500 - (-1000)) / Abs(-1000) * 100 = -500/1000*100 = -50%
	assert.InDelta(t, -50.0, mc.PctChange, 0.001)
}

func TestBuildSignedMetricChange_CrossZero(t *testing.T) {
	// Previous positive, current negative (went from gain to loss)
	mc := buildSignedMetricChange(-200.0, 500.0, true)
	assert.Equal(t, -200.0, mc.Current)
	assert.Equal(t, 500.0, mc.Previous)
	assert.True(t, mc.HasPrevious)
	assert.Equal(t, -700.0, mc.RawChange)
	// (-200 - 500) / Abs(500) * 100 = -700/500*100 = -140%
	assert.InDelta(t, -140.0, mc.PctChange, 0.001)
}

func TestBuildSignedMetricChange_ZeroPrevious(t *testing.T) {
	// Previous is zero — PctChange should be 0, no division by zero
	mc := buildSignedMetricChange(1000.0, 0.0, true)
	assert.Equal(t, 1000.0, mc.Current)
	assert.Equal(t, 0.0, mc.Previous)
	assert.True(t, mc.HasPrevious)
	assert.Equal(t, 1000.0, mc.RawChange)
	assert.Equal(t, 0.0, mc.PctChange, "PctChange must be 0 when previous is 0 (no divide-by-zero)")
}

func TestBuildSignedMetricChange_NoPrevious(t *testing.T) {
	// hasPrevious=false — HasPrevious false, RawChange still computed
	mc := buildSignedMetricChange(1000.0, 0.0, false)
	assert.Equal(t, 1000.0, mc.Current)
	assert.False(t, mc.HasPrevious)
	assert.Equal(t, 1000.0, mc.RawChange)
	assert.Equal(t, 0.0, mc.PctChange)
}

func TestComputePeriodChanges_HasNetEquityReturn(t *testing.T) {
	// Verify PeriodChanges contains NetEquityReturn and NetEquityReturnPct (not EquityValue)
	now := time.Now().Truncate(24 * time.Hour)
	yesterday := now.AddDate(0, 0, -1)

	tl := &stressMockTimelineStore{
		snapshots: []models.TimelineSnapshot{
			{
				UserID:             "user1",
				PortfolioName:      "SMSF",
				Date:               yesterday,
				NetEquityReturn:    5000.0,
				NetEquityReturnPct: 5.26,
				PortfolioValue:     105000.0,
				GrossCashBalance:   10000.0,
			},
		},
	}

	svc := &Service{
		storage: &stressMockStorageManager{timelineStore: tl},
	}
	portfolio := &models.Portfolio{
		Name:                 "SMSF",
		NetEquityReturn:      7000.0,
		NetEquityReturnPct:   7.0,
		PortfolioValue:       110000.0,
		GrossCashBalance:     10000.0,
		LedgerDividendReturn: 600.0,
	}

	pc := svc.computePeriodChanges(context.Background(), "user1", portfolio, tl, yesterday)

	// NetEquityReturn should be populated
	assert.True(t, pc.NetEquityReturn.HasPrevious, "NetEquityReturn.HasPrevious should be true when snapshot exists")
	assert.Equal(t, 7000.0, pc.NetEquityReturn.Current)
	assert.Equal(t, 5000.0, pc.NetEquityReturn.Previous)
	assert.Equal(t, 2000.0, pc.NetEquityReturn.RawChange)

	// NetEquityReturnPct should be populated
	assert.True(t, pc.NetEquityReturnPct.HasPrevious, "NetEquityReturnPct.HasPrevious should be true when snapshot exists")
	assert.Equal(t, 7.0, pc.NetEquityReturnPct.Current)
	assert.Equal(t, 5.26, pc.NetEquityReturnPct.Previous)
}

func TestComputePeriodChanges_NoSnapshot(t *testing.T) {
	// When no timeline data, all HasPrevious = false
	now := time.Now().Truncate(24 * time.Hour)
	yesterday := now.AddDate(0, 0, -1)

	tl := &stressMockTimelineStore{
		snapshots: []models.TimelineSnapshot{}, // empty
	}

	svc := &Service{
		storage:     &stressMockStorageManager{timelineStore: tl},
		cashflowSvc: nil,
	}
	portfolio := &models.Portfolio{
		Name:                 "SMSF",
		NetEquityReturn:      5000.0,
		NetEquityReturnPct:   5.0,
		PortfolioValue:       110000.0,
		GrossCashBalance:     10000.0,
		LedgerDividendReturn: 600.0,
	}

	pc := svc.computePeriodChanges(context.Background(), "user1", portfolio, tl, yesterday)

	assert.False(t, pc.NetEquityReturn.HasPrevious, "HasPrevious should be false when no snapshot")
	assert.False(t, pc.NetEquityReturnPct.HasPrevious, "HasPrevious should be false when no snapshot")
	assert.False(t, pc.PortfolioValue.HasPrevious)
	assert.False(t, pc.GrossCash.HasPrevious)
	assert.False(t, pc.Dividend.HasPrevious)

	// Current values should still be set
	assert.Equal(t, 5000.0, pc.NetEquityReturn.Current)
	assert.Equal(t, 5.0, pc.NetEquityReturnPct.Current)
	assert.Equal(t, 110000.0, pc.PortfolioValue.Current)
}
