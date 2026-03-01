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

// TestGainLossStorageRoundtrip verifies that portfolio data with NetReturn values
// (including trades, realised components, and negative NetReturn) survives storage
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
				Name:           "SMSF",
				EquityValue:     23394.57,
				NetEquityCost:  19820.84,
				NetEquityReturn: 1163.40,
				Holdings: []models.Holding{
					{
						Ticker:       "SKS",
						Exchange:     "AU",
						Name:         "SKS Technologies",
						Units:        4967,
						AvgCost:      3.99,
						CurrentPrice: 4.71,
						MarketValue:  23394.57,
						NetReturn:    1163.40,
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
			name: "negative_netreturn",
			portfolio: models.Portfolio{
				Name:           "test_negative",
				EquityValue:     550.00,
				NetEquityCost:      500.00,
				NetEquityReturn: -50.00,
				Holdings: []models.Holding{
					{
						Ticker:         "TST",
						Exchange:       "AU",
						Name:           "Test Corp",
						Units:          50,
						AvgCost:        10.00,
						CurrentPrice:   11.00,
						MarketValue:    550.00,
						NetReturn:      -50.00, // negative: realised loss exceeds unrealised gain
						DividendReturn: 25.00,
						Currency:       "AUD",
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
				Name:           "test_buyhold",
				EquityValue:     9000.00,
				NetEquityCost:      7765.00,
				NetEquityReturn: 1235.00,
				Holdings: []models.Holding{
					{
						Ticker:       "BHP",
						Exchange:     "AU",
						Name:         "BHP Group",
						Units:        150,
						AvgCost:      51.77,
						CurrentPrice: 60.00,
						MarketValue:  9000.00,
						NetReturn:    1235.00,
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
				Name:           "test_closed",
				EquityValue:     0,
				NetEquityCost:      1000.00,
				NetEquityReturn: 480.00,
				Holdings: []models.Holding{
					{
						Ticker:       "XYZ",
						Exchange:     "AU",
						Name:         "XYZ Corp",
						Units:        0,
						CurrentPrice: 15.00,
						MarketValue:  0,
						NetReturn:    480.00,
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
		{
			name: "with_realized_unrealized_breakdown",
			portfolio: models.Portfolio{
				Name:                     "test_breakdown",
				EquityValue:               15000.00,
				NetEquityCost:                10000.00,
				NetEquityReturn:        5000.00,
				RealizedEquityReturn:  2000.00,
				UnrealizedEquityReturn: 3000.00,
				Holdings: []models.Holding{
					{
						Ticker:              "ABC",
						Exchange:            "AU",
						Name:                "ABC Corp",
						Units:               100,
						AvgCost:             100.00,
						CurrentPrice:        150.00,
						MarketValue:         15000.00,
						NetReturn:           5000.00,
						NetReturnPct:        50.00,
						TotalInvested:       10000.00,
						RealizedNetReturn:   2000.00,
						UnrealizedNetReturn: 3000.00,
						DividendReturn:      500.00,
						AnnualizedTotalReturnPct:     12.5,
						NetReturnPctTWRR:    11.2,
						Currency:            "AUD",
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
			assert.InDelta(t, tt.portfolio.EquityValue, restored.EquityValue, 0.01)
			assert.InDelta(t, tt.portfolio.NetEquityCost, restored.NetEquityCost, 0.01)
			assert.InDelta(t, tt.portfolio.TotalNetReturn, restored.TotalNetReturn, 0.01)
			assert.InDelta(t, tt.portfolio.TotalRealizedNetReturn, restored.TotalRealizedNetReturn, 0.01)
			assert.InDelta(t, tt.portfolio.TotalUnrealizedNetReturn, restored.TotalUnrealizedNetReturn, 0.01)

			// Verify holdings
			require.Len(t, restored.Holdings, len(tt.portfolio.Holdings))

			for i, expected := range tt.portfolio.Holdings {
				actual := restored.Holdings[i]

				assert.Equal(t, expected.Ticker, actual.Ticker, "holding[%d] ticker", i)
				assert.InDelta(t, expected.Units, actual.Units, 0.01, "holding[%d] units", i)
				assert.InDelta(t, expected.NetReturn, actual.NetReturn, 0.01, "holding[%d] NetReturn", i)
				assert.InDelta(t, expected.NetReturnPct, actual.NetReturnPct, 0.01, "holding[%d] NetReturnPct", i)
				assert.InDelta(t, expected.NetEquityCost, actual.NetEquityCost, 0.01, "holding[%d] TotalCost", i)
				assert.InDelta(t, expected.MarketValue, actual.MarketValue, 0.01, "holding[%d] MarketValue", i)
				assert.InDelta(t, expected.CurrentPrice, actual.CurrentPrice, 0.01, "holding[%d] CurrentPrice", i)
				assert.InDelta(t, expected.DividendReturn, actual.DividendReturn, 0.01, "holding[%d] DividendReturn", i)
				assert.InDelta(t, expected.RealizedNetReturn, actual.RealizedNetReturn, 0.01, "holding[%d] RealizedNetReturn", i)
				assert.InDelta(t, expected.UnrealizedNetReturn, actual.UnrealizedNetReturn, 0.01, "holding[%d] UnrealizedNetReturn", i)
				assert.InDelta(t, expected.NetReturnPctIRR, actual.NetReturnPctIRR, 0.01, "holding[%d] NetReturnPctIRR", i)
				assert.InDelta(t, expected.NetReturnPctTWRR, actual.NetReturnPctTWRR, 0.01, "holding[%d] NetReturnPctTWRR", i)

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
		Name:           "multi_holding",
		EquityValue:     2200.00,
		NetEquityCost:      2200.00,
		NetEquityReturn: 200.00,
		Holdings: []models.Holding{
			{
				Ticker:       "BHP",
				Exchange:     "AU",
				Name:         "BHP Group",
				Units:        200,
				CurrentPrice: 11.00,
				MarketValue:  2200.00,
				NetReturn:    200.00,
				NetEquityCost:    2200.00,
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

// TestGainLossPrecision verifies that NetReturn values with many decimal places
// are stored and retrieved without floating-point precision loss.
func TestGainLossPrecision(t *testing.T) {
	mgr := testManager(t)
	store := mgr.UserDataStore()
	ctx := testContext()

	// Use values that are prone to floating-point precision issues
	portfolio := models.Portfolio{
		Name:           "precision_test",
		EquityValue:     23394.57,
		NetEquityCost:      39820.84,
		NetEquityReturn: 1163.40,
		Holdings: []models.Holding{
			{
				Ticker:       "SKS",
				Exchange:     "AU",
				Units:        4967,
				AvgCost:      3.9906, // computed average
				CurrentPrice: 4.71,
				MarketValue:  23394.57,
				NetReturn:    1163.40,
				NetEquityCost:    19820.84,
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
	assert.InDelta(t, 1163.40, h.NetReturn, 0.01)

	// Verify sign is preserved (not absolute-valued)
	assert.False(t, math.Signbit(h.NetReturn), "positive NetReturn should stay positive")
}

// TestGainLossNewFieldsRoundtrip verifies that the new portfolio-level fields
// (TotalRealizedNetReturn, TotalUnrealizedNetReturn) and holding-level fields
// (RealizedNetReturn, UnrealizedNetReturn, NetReturnPctIRR, NetReturnPctTWRR)
// survive storage roundtrip correctly.
func TestGainLossNewFieldsRoundtrip(t *testing.T) {
	mgr := testManager(t)
	store := mgr.UserDataStore()
	ctx := testContext()

	breakeven := 95.50
	portfolio := models.Portfolio{
		Name:                     "new_fields_test",
		EquityValue:               50000.00,
		NetEquityCost:                40000.00,
		NetEquityReturn:           10000.00,
		TotalNetReturnPct:        25.00,
		TotalRealizedNetReturn:   3000.00,
		TotalUnrealizedNetReturn: 7000.00,
		Currency:                 "AUD",
		FXRate:                   0.65,
		Holdings: []models.Holding{
			{
				Ticker:              "CBA",
				Exchange:            "AU",
				Name:                "Commonwealth Bank",
				Units:               100,
				AvgCost:             100.00,
				CurrentPrice:        120.00,
				MarketValue:         12000.00,
				NetReturn:           2000.00,
				NetReturnPct:        20.00,
				NetEquityCost:           10000.00,
				TotalInvested:       10000.00,
				RealizedNetReturn:   500.00,
				UnrealizedNetReturn: 1500.00,
				DividendReturn:      200.00,
				CapitalGainPct:      18.00,
				AnnualizedTotalReturnPct:     15.50,
				NetReturnPctTWRR:    14.20,
				Currency:            "AUD",
				TrueBreakevenPrice:  &breakeven,
			},
		},
		LastSynced: time.Now().Truncate(time.Second),
	}

	data, err := json.Marshal(portfolio)
	require.NoError(t, err)

	require.NoError(t, store.Put(ctx, &models.UserRecord{
		UserID:   "nf_user",
		Subject:  "portfolio",
		Key:      portfolio.Name,
		Value:    string(data),
		Version:  1,
		DateTime: time.Now().Truncate(time.Second),
	}))

	got, err := store.Get(ctx, "nf_user", "portfolio", "new_fields_test")
	require.NoError(t, err)

	var restored models.Portfolio
	require.NoError(t, json.Unmarshal([]byte(got.Value), &restored))

	// Portfolio-level new fields
	assert.InDelta(t, 10000.00, restored.TotalNetReturn, 0.01)
	assert.InDelta(t, 25.00, restored.TotalNetReturnPct, 0.01)
	assert.InDelta(t, 3000.00, restored.TotalRealizedNetReturn, 0.01)
	assert.InDelta(t, 7000.00, restored.TotalUnrealizedNetReturn, 0.01)

	require.Len(t, restored.Holdings, 1)
	h := restored.Holdings[0]

	// Holding-level new fields
	assert.InDelta(t, 2000.00, h.NetReturn, 0.01)
	assert.InDelta(t, 20.00, h.NetReturnPct, 0.01)
	assert.InDelta(t, 500.00, h.RealizedNetReturn, 0.01)
	assert.InDelta(t, 1500.00, h.UnrealizedNetReturn, 0.01)
	assert.InDelta(t, 15.50, h.NetReturnPctIRR, 0.01)
	assert.InDelta(t, 14.20, h.NetReturnPctTWRR, 0.01)
	assert.NotNil(t, h.TrueBreakevenPrice)
	assert.InDelta(t, 95.50, *h.TrueBreakevenPrice, 0.01)

	// Verify JSON field names in serialized output
	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &raw))

	assert.Contains(t, raw, "total_net_return")
	assert.Contains(t, raw, "total_net_return_pct")
	assert.Contains(t, raw, "total_realized_net_return")
	assert.Contains(t, raw, "total_unrealized_net_return")
	assert.NotContains(t, raw, "total_gain")
	assert.NotContains(t, raw, "total_gain_pct")

	// Check holding-level JSON field names
	holdings, ok := raw["holdings"].([]interface{})
	require.True(t, ok)
	require.Len(t, holdings, 1)
	hRaw, ok := holdings[0].(map[string]interface{})
	require.True(t, ok)

	assert.Contains(t, hRaw, "net_return")
	assert.Contains(t, hRaw, "net_return_pct")
	assert.Contains(t, hRaw, "realized_net_return")
	assert.Contains(t, hRaw, "unrealized_net_return")
	assert.Contains(t, hRaw, "net_return_pct_irr")
	assert.Contains(t, hRaw, "net_return_pct_twrr")
	assert.Contains(t, hRaw, "true_breakeven_price")
	assert.NotContains(t, hRaw, "gain_loss")
	assert.NotContains(t, hRaw, "gain_loss_pct")
	assert.NotContains(t, hRaw, "total_return_value")
	assert.NotContains(t, hRaw, "total_return_pct")
	assert.NotContains(t, hRaw, "total_return_pct_irr")
	assert.NotContains(t, hRaw, "total_return_pct_twrr")
	assert.NotContains(t, hRaw, "net_pnl_if_sold_today")
	assert.NotContains(t, hRaw, "price_target_15pct")
	assert.NotContains(t, hRaw, "stop_loss_5pct")
	assert.NotContains(t, hRaw, "stop_loss_10pct")
	assert.NotContains(t, hRaw, "stop_loss_15pct")
}
