package data

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGainLossStorageRoundtrip verifies that portfolio data with GainLoss values
// (including trades, realised components, and negative GainLoss) survives storage
// roundtrip through SurrealDB without data loss or precision issues.
func TestGainLossStorageRoundtrip(t *testing.T) {
	mgr := testManager(t)
	store := mgr.UserDataStore()
	ctx := testContext()

	tests := []struct {
		name      string
		portfolio models.Portfolio
	}{
		{
			name: "partial_sell_with_realised_loss",
			portfolio: models.Portfolio{
				Name:       "SMSF",
				TotalValue: 23394.57,
				TotalCost:  19820.84,
				TotalGain:  1163.40,
				Holdings: []models.Holding{
					{
						Ticker:       "SKS",
						Exchange:     "AU",
						Name:         "SKS Technologies",
						Units:        4967,
						AvgCost:      3.99,
						CurrentPrice: 4.71,
						MarketValue:  23394.57,
						GainLoss:     1163.40,
						TotalCost:    19820.84,
						Currency:     "AUD",
						Trades: []*models.NavexaTrade{
							{ID: "t1", Type: "buy", Units: 4925, Price: 4.0248, Fees: 3.00},
							{ID: "t2", Type: "sell", Units: 1333, Price: 3.7627, Fees: 3.00},
							{ID: "t3", Type: "sell", Units: 819, Price: 3.680, Fees: 3.00},
							{ID: "t4", Type: "sell", Units: 2773, Price: 3.4508, Fees: 3.00},
							{ID: "t5", Type: "buy", Units: 2511, Price: 3.980, Fees: 3.00},
							{ID: "t6", Type: "buy", Units: 2456, Price: 4.070, Fees: 3.00},
						},
					},
				},
				LastSynced: time.Now().Truncate(time.Second),
			},
		},
		{
			name: "negative_gainloss_with_eodhd_price_update",
			portfolio: models.Portfolio{
				Name:       "test_negative",
				TotalValue: 550.00,
				TotalCost:  500.00,
				TotalGain:  -50.00,
				Holdings: []models.Holding{
					{
						Ticker:           "TST",
						Exchange:         "AU",
						Name:             "Test Corp",
						Units:            50,
						AvgCost:          10.00,
						CurrentPrice:     11.00,
						MarketValue:      550.00,
						GainLoss:         -50.00, // negative: realised loss exceeds unrealised gain
						TotalCost:        500.00,
						DividendReturn:   25.00,
						TotalReturnValue: -25.00,
						Currency:         "AUD",
						Trades: []*models.NavexaTrade{
							{ID: "t1", Type: "buy", Units: 100, Price: 10.00},
							{ID: "t2", Type: "sell", Units: 50, Price: 8.00},
						},
					},
				},
				LastSynced: time.Now().Truncate(time.Second),
			},
		},
		{
			name: "pure_buy_and_hold",
			portfolio: models.Portfolio{
				Name:       "test_buyhold",
				TotalValue: 9000.00,
				TotalCost:  7765.00,
				TotalGain:  1235.00,
				Holdings: []models.Holding{
					{
						Ticker:       "BHP",
						Exchange:     "AU",
						Name:         "BHP Group",
						Units:        150,
						AvgCost:      51.77,
						CurrentPrice: 60.00,
						MarketValue:  9000.00,
						GainLoss:     1235.00,
						TotalCost:    7765.00,
						Currency:     "AUD",
						Trades: []*models.NavexaTrade{
							{ID: "t1", Type: "buy", Units: 100, Price: 50.00, Fees: 10.00},
							{ID: "t2", Type: "buy", Units: 50, Price: 55.00, Fees: 5.00},
						},
					},
				},
				LastSynced: time.Now().Truncate(time.Second),
			},
		},
		{
			name: "closed_position",
			portfolio: models.Portfolio{
				Name:       "test_closed",
				TotalValue: 0,
				TotalCost:  1000.00,
				TotalGain:  480.00,
				Holdings: []models.Holding{
					{
						Ticker:       "XYZ",
						Exchange:     "AU",
						Name:         "XYZ Corp",
						Units:        0,
						CurrentPrice: 15.00,
						MarketValue:  0,
						GainLoss:     480.00,
						TotalCost:    1000.00,
						Currency:     "AUD",
						Trades: []*models.NavexaTrade{
							{ID: "t1", Type: "buy", Units: 100, Price: 10.00, Fees: 10.00},
							{ID: "t2", Type: "sell", Units: 100, Price: 15.00, Fees: 10.00},
						},
					},
				},
				LastSynced: time.Now().Truncate(time.Second),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Store as JSON in UserDataStore (same as production path)
			data, err := json.Marshal(tt.portfolio)
			require.NoError(t, err, "marshal portfolio")

			record := &models.UserRecord{
				UserID:   "gl_test_user",
				Subject:  "portfolio",
				Key:      tt.portfolio.Name,
				Value:    string(data),
				Version:  1,
				DateTime: time.Now().Truncate(time.Second),
			}
			require.NoError(t, store.Put(ctx, record), "store portfolio")

			// Retrieve
			got, err := store.Get(ctx, "gl_test_user", "portfolio", tt.portfolio.Name)
			require.NoError(t, err, "get portfolio")

			var restored models.Portfolio
			require.NoError(t, json.Unmarshal([]byte(got.Value), &restored), "unmarshal portfolio")

			// Verify portfolio-level fields
			assert.Equal(t, tt.portfolio.Name, restored.Name)
			assert.InDelta(t, tt.portfolio.TotalValue, restored.TotalValue, 0.01)
			assert.InDelta(t, tt.portfolio.TotalCost, restored.TotalCost, 0.01)
			assert.InDelta(t, tt.portfolio.TotalGain, restored.TotalGain, 0.01)

			// Verify holdings
			require.Len(t, restored.Holdings, len(tt.portfolio.Holdings))

			for i, expected := range tt.portfolio.Holdings {
				actual := restored.Holdings[i]

				assert.Equal(t, expected.Ticker, actual.Ticker, "holding[%d] ticker", i)
				assert.InDelta(t, expected.Units, actual.Units, 0.01, "holding[%d] units", i)
				assert.InDelta(t, expected.GainLoss, actual.GainLoss, 0.01, "holding[%d] GainLoss", i)
				assert.InDelta(t, expected.TotalCost, actual.TotalCost, 0.01, "holding[%d] TotalCost", i)
				assert.InDelta(t, expected.MarketValue, actual.MarketValue, 0.01, "holding[%d] MarketValue", i)
				assert.InDelta(t, expected.CurrentPrice, actual.CurrentPrice, 0.01, "holding[%d] CurrentPrice", i)
				assert.InDelta(t, expected.DividendReturn, actual.DividendReturn, 0.01, "holding[%d] DividendReturn", i)
				assert.InDelta(t, expected.TotalReturnValue, actual.TotalReturnValue, 0.01, "holding[%d] TotalReturnValue", i)

				// Verify trades survived roundtrip
				require.Len(t, actual.Trades, len(expected.Trades), "holding[%d] trade count", i)
				for j, expTrade := range expected.Trades {
					actTrade := actual.Trades[j]
					assert.Equal(t, expTrade.ID, actTrade.ID, "trade[%d][%d] ID", i, j)
					assert.Equal(t, expTrade.Type, actTrade.Type, "trade[%d][%d] Type", i, j)
					assert.InDelta(t, expTrade.Units, actTrade.Units, 0.01, "trade[%d][%d] Units", i, j)
					assert.InDelta(t, expTrade.Price, actTrade.Price, 0.0001, "trade[%d][%d] Price", i, j)
					assert.InDelta(t, expTrade.Fees, actTrade.Fees, 0.01, "trade[%d][%d] Fees", i, j)
				}
			}
		})
	}
}

// TestGainLossMultiHoldingSameTickerStorage verifies that a portfolio with
// multiple holdings for the same ticker (merged trades via append fix)
// stores and retrieves correctly.
func TestGainLossMultiHoldingSameTickerStorage(t *testing.T) {
	mgr := testManager(t)
	store := mgr.UserDataStore()
	ctx := testContext()

	// Portfolio where trades from multiple Navexa holdings (same ticker) are merged
	portfolio := models.Portfolio{
		Name:       "multi_holding",
		TotalValue: 2200.00,
		TotalCost:  2200.00,
		TotalGain:  200.00,
		Holdings: []models.Holding{
			{
				Ticker:       "BHP",
				Exchange:     "AU",
				Name:         "BHP Group",
				Units:        200,
				CurrentPrice: 11.00,
				MarketValue:  2200.00,
				GainLoss:     200.00,
				TotalCost:    2200.00,
				Currency:     "AUD",
				// Merged trades from two Navexa holdings (closed + open)
				Trades: []*models.NavexaTrade{
					{ID: "t1", Type: "buy", Units: 100, Price: 10.00},
					{ID: "t2", Type: "sell", Units: 100, Price: 12.00},
					{ID: "t3", Type: "buy", Units: 200, Price: 11.00},
				},
			},
		},
		LastSynced: time.Now().Truncate(time.Second),
	}

	data, err := json.Marshal(portfolio)
	require.NoError(t, err)

	require.NoError(t, store.Put(ctx, &models.UserRecord{
		UserID:   "mh_test_user",
		Subject:  "portfolio",
		Key:      portfolio.Name,
		Value:    string(data),
		Version:  1,
		DateTime: time.Now().Truncate(time.Second),
	}))

	got, err := store.Get(ctx, "mh_test_user", "portfolio", "multi_holding")
	require.NoError(t, err)

	var restored models.Portfolio
	require.NoError(t, json.Unmarshal([]byte(got.Value), &restored))

	require.Len(t, restored.Holdings, 1)
	holding := restored.Holdings[0]

	// All 3 trades should be present (the append fix ensures none are lost)
	require.Len(t, holding.Trades, 3, "all merged trades must survive storage")
	assert.Equal(t, "t1", holding.Trades[0].ID)
	assert.Equal(t, "t2", holding.Trades[1].ID)
	assert.Equal(t, "t3", holding.Trades[2].ID)
}

// TestGainLossPrecision verifies that GainLoss values with many decimal places
// are stored and retrieved without floating-point precision loss.
func TestGainLossPrecision(t *testing.T) {
	mgr := testManager(t)
	store := mgr.UserDataStore()
	ctx := testContext()

	// Use values that are prone to floating-point precision issues
	portfolio := models.Portfolio{
		Name:       "precision_test",
		TotalValue: 23394.57,
		TotalCost:  39820.84,
		TotalGain:  1163.40,
		Holdings: []models.Holding{
			{
				Ticker:       "SKS",
				Exchange:     "AU",
				Units:        4967,
				AvgCost:      3.9906, // computed average
				CurrentPrice: 4.71,
				MarketValue:  23394.57,
				GainLoss:     1163.40,
				TotalCost:    19820.84,
				Currency:     "AUD",
			},
		},
		LastSynced: time.Now().Truncate(time.Second),
	}

	data, err := json.Marshal(portfolio)
	require.NoError(t, err)

	require.NoError(t, store.Put(ctx, &models.UserRecord{
		UserID:   "prec_user",
		Subject:  "portfolio",
		Key:      portfolio.Name,
		Value:    string(data),
		Version:  1,
		DateTime: time.Now().Truncate(time.Second),
	}))

	got, err := store.Get(ctx, "prec_user", "portfolio", "precision_test")
	require.NoError(t, err)

	var restored models.Portfolio
	require.NoError(t, json.Unmarshal([]byte(got.Value), &restored))

	require.Len(t, restored.Holdings, 1)
	h := restored.Holdings[0]

	// Verify sub-cent precision is preserved
	assert.InDelta(t, 4967.0, h.Units, 0.001)
	assert.InDelta(t, 3.9906, h.AvgCost, 0.0001)
	assert.InDelta(t, 4.71, h.CurrentPrice, 0.001)
	assert.InDelta(t, 23394.57, h.MarketValue, 0.01)
	assert.InDelta(t, 1163.40, h.GainLoss, 0.01)

	// Verify sign is preserved (not absolute-valued)
	assert.False(t, math.Signbit(h.GainLoss), "positive GainLoss should stay positive")
}
