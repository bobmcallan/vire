package market

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

// ============================================================================
// Stress tests for Fix B: YesterdayPct/LastWeekPct recalculation after live quote
// ============================================================================

// TestStress_LiveQuote_ZeroYesterdayClose verifies no division by zero when
// YesterdayClose is 0 and a live quote overrides the price.
func TestStress_LiveQuote_ZeroYesterdayClose(t *testing.T) {
	today := time.Now()

	bars := make([]models.EODBar, 7)
	for i := range bars {
		bars[i] = models.EODBar{
			Date: today.AddDate(0, 0, -i),
			Open: 99, High: 101, Low: 98, Close: 100,
		}
	}
	// Yesterday (EOD[1]) has zero close
	bars[1].Close = 0

	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"ZEROYD.AU": {
					Ticker: "ZEROYD.AU", Exchange: "AU",
					EOD: bars, EODUpdatedAt: today,
					DataVersion: common.SchemaVersion, LastUpdated: today,
				},
			},
		},
		signals: &mockSignalStorage{},
	}

	eodhd := &mockEODHDClient{
		realTimeQuoteFn: func(_ context.Context, _ string) (*models.RealTimeQuote, error) {
			return &models.RealTimeQuote{Close: 110, Open: 109, High: 111, Low: 108, Volume: 1000, Timestamp: today}, nil
		},
	}

	svc := NewService(storage, eodhd, nil, common.NewLogger("error"))

	data, err := svc.GetStockData(context.Background(), "ZEROYD.AU", interfaces.StockDataInclude{Price: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// YesterdayClose=0 means YesterdayPct should remain 0 (guard: if > 0)
	if math.IsNaN(data.Price.YesterdayPct) || math.IsInf(data.Price.YesterdayPct, 0) {
		t.Errorf("YesterdayPct should not be NaN/Inf when YesterdayClose=0, got %v", data.Price.YesterdayPct)
	}
}

// TestStress_LiveQuote_ZeroLastWeekClose verifies no division by zero when
// LastWeekClose is 0 and a live quote overrides the price.
func TestStress_LiveQuote_ZeroLastWeekClose(t *testing.T) {
	today := time.Now()

	bars := make([]models.EODBar, 7)
	for i := range bars {
		bars[i] = models.EODBar{
			Date: today.AddDate(0, 0, -i),
			Open: 99, High: 101, Low: 98, Close: 100,
		}
	}
	// Last week (EOD[5]) has zero close
	bars[5].Close = 0

	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"ZEROLW.AU": {
					Ticker: "ZEROLW.AU", Exchange: "AU",
					EOD: bars, EODUpdatedAt: today,
					DataVersion: common.SchemaVersion, LastUpdated: today,
				},
			},
		},
		signals: &mockSignalStorage{},
	}

	eodhd := &mockEODHDClient{
		realTimeQuoteFn: func(_ context.Context, _ string) (*models.RealTimeQuote, error) {
			return &models.RealTimeQuote{Close: 110, Open: 109, High: 111, Low: 108, Volume: 1000, Timestamp: today}, nil
		},
	}

	svc := NewService(storage, eodhd, nil, common.NewLogger("error"))

	data, err := svc.GetStockData(context.Background(), "ZEROLW.AU", interfaces.StockDataInclude{Price: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if math.IsNaN(data.Price.LastWeekPct) || math.IsInf(data.Price.LastWeekPct, 0) {
		t.Errorf("LastWeekPct should not be NaN/Inf when LastWeekClose=0, got %v", data.Price.LastWeekPct)
	}
}

// TestStress_LiveQuote_StaleWrongData verifies behavior when the live quote
// returns a completely wrong price (e.g., 0, negative, or astronomically high).
func TestStress_LiveQuote_StaleWrongData(t *testing.T) {
	today := time.Now()

	bars := make([]models.EODBar, 7)
	for i := range bars {
		bars[i] = models.EODBar{
			Date: today.AddDate(0, 0, -i),
			Open: 99, High: 101, Low: 98, Close: 100,
		}
	}

	tests := []struct {
		name      string
		liveClose float64
		expectUse bool // whether the live quote should be used
	}{
		{"zero close", 0, false},           // quote.Close > 0 guard prevents use
		{"negative close", -50, false},     // quote.Close > 0 guard prevents use
		{"very high close", 1000000, true}, // accepted — no upper bound check
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage := &mockStorageManager{
				market: &mockMarketDataStorage{
					data: map[string]*models.MarketData{
						"STALE.AU": {
							Ticker: "STALE.AU", Exchange: "AU",
							EOD: bars, EODUpdatedAt: today,
							DataVersion: common.SchemaVersion, LastUpdated: today,
						},
					},
				},
				signals: &mockSignalStorage{},
			}

			eodhd := &mockEODHDClient{
				realTimeQuoteFn: func(_ context.Context, _ string) (*models.RealTimeQuote, error) {
					return &models.RealTimeQuote{
						Close: tt.liveClose, Open: tt.liveClose - 1,
						High: tt.liveClose + 1, Low: tt.liveClose - 2,
						Volume: 1000, Timestamp: today,
					}, nil
				},
			}

			svc := NewService(storage, eodhd, nil, common.NewLogger("error"))

			data, err := svc.GetStockData(context.Background(), "STALE.AU", interfaces.StockDataInclude{Price: true})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.expectUse {
				if data.Price.Current != tt.liveClose {
					t.Errorf("expected live price %.1f to be used, got %.1f", tt.liveClose, data.Price.Current)
				}
			} else {
				if data.Price.Current == tt.liveClose {
					t.Errorf("live price %.1f should NOT have been used (guard: Close > 0)", tt.liveClose)
				}
			}

			// Verify no NaN/Inf in any pct fields
			if math.IsNaN(data.Price.YesterdayPct) || math.IsInf(data.Price.YesterdayPct, 0) {
				t.Error("YesterdayPct is NaN/Inf")
			}
			if math.IsNaN(data.Price.LastWeekPct) || math.IsInf(data.Price.LastWeekPct, 0) {
				t.Error("LastWeekPct is NaN/Inf")
			}
			if math.IsNaN(data.Price.ChangePct) || math.IsInf(data.Price.ChangePct, 0) {
				t.Error("ChangePct is NaN/Inf")
			}
		})
	}
}

// TestStress_LiveQuote_FewerThan6Bars verifies that when there are fewer than
// 6 EOD bars, the LastWeekPct calculation doesn't panic on index out of range.
func TestStress_LiveQuote_FewerThan6Bars(t *testing.T) {
	today := time.Now()

	tests := []struct {
		name    string
		numBars int
	}{
		{"1 bar", 1},
		{"2 bars", 2},
		{"5 bars", 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bars := make([]models.EODBar, tt.numBars)
			for i := range bars {
				bars[i] = models.EODBar{
					Date: today.AddDate(0, 0, -i),
					Open: 99, High: 101, Low: 98, Close: 100,
				}
			}

			storage := &mockStorageManager{
				market: &mockMarketDataStorage{
					data: map[string]*models.MarketData{
						"FEW.AU": {
							Ticker: "FEW.AU", Exchange: "AU",
							EOD: bars, EODUpdatedAt: today,
							DataVersion: common.SchemaVersion, LastUpdated: today,
						},
					},
				},
				signals: &mockSignalStorage{},
			}

			eodhd := &mockEODHDClient{
				realTimeQuoteFn: func(_ context.Context, _ string) (*models.RealTimeQuote, error) {
					return &models.RealTimeQuote{Close: 110, Open: 109, High: 111, Low: 108, Volume: 1000, Timestamp: today}, nil
				},
			}

			svc := NewService(storage, eodhd, nil, common.NewLogger("error"))

			// This should not panic
			data, err := svc.GetStockData(context.Background(), "FEW.AU", interfaces.StockDataInclude{Price: true})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if data.Price == nil {
				t.Fatal("price data should not be nil")
			}

			// LastWeekPct should be 0 when we have fewer than 6 bars (no EOD[5])
			if tt.numBars < 6 && data.Price.LastWeekPct != 0 {
				// LastWeekPct could still be set if there is no EOD[5]
				// The guard is len(marketData.EOD) > 5
				t.Logf("LastWeekPct=%.2f with only %d bars (OK if 0)", data.Price.LastWeekPct, tt.numBars)
			}
		})
	}
}
