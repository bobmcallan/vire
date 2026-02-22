package data

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPortfolioDataVersionRoundtrip verifies that a portfolio with the DataVersion
// field set survives storage roundtrip through SurrealDB. The DataVersion field is
// used for cache invalidation: when the schema changes, stale cached portfolios
// with old DataVersion values should be detected and trigger a re-sync.
func TestPortfolioDataVersionRoundtrip(t *testing.T) {
	mgr := testManager(t)
	store := mgr.UserDataStore()
	ctx := testContext()

	breakeven := 48.50
	tests := []struct {
		name      string
		portfolio models.Portfolio
	}{
		{
			name: "current_version",
			portfolio: models.Portfolio{
				Name:              "dv_current",
				TotalValue:        10000.00,
				TotalCost:         8000.00,
				TotalNetReturn:    2000.00,
				Currency:          "AUD",
				DataVersion:       common.SchemaVersion,
				CalculationMethod: "average_cost",
				Holdings: []models.Holding{
					{
						Ticker:       "BHP",
						Exchange:     "AU",
						Name:         "BHP Group",
						Units:        100,
						AvgCost:      80.00,
						CurrentPrice: 100.00,
						MarketValue:  10000.00,
						NetReturn:    2000.00,
						TotalCost:    8000.00,
						Currency:     "AUD",
					},
				},
				LastSynced: time.Now().Truncate(time.Second),
			},
		},
		{
			name: "old_version",
			portfolio: models.Portfolio{
				Name:           "dv_old",
				TotalValue:     5000.00,
				TotalCost:      4000.00,
				TotalNetReturn: 1000.00,
				Currency:       "AUD",
				DataVersion:    "5", // deliberately old version
				Holdings: []models.Holding{
					{
						Ticker:       "CBA",
						Exchange:     "AU",
						Name:         "Commonwealth Bank",
						Units:        50,
						AvgCost:      80.00,
						CurrentPrice: 100.00,
						MarketValue:  5000.00,
						NetReturn:    1000.00,
						TotalCost:    4000.00,
						Currency:     "AUD",
					},
				},
				LastSynced: time.Now().Truncate(time.Second),
			},
		},
		{
			name: "empty_version",
			portfolio: models.Portfolio{
				Name:           "dv_empty",
				TotalValue:     3000.00,
				TotalCost:      2500.00,
				TotalNetReturn: 500.00,
				Currency:       "AUD",
				DataVersion:    "", // no version (legacy data)
				Holdings: []models.Holding{
					{
						Ticker:       "WES",
						Exchange:     "AU",
						Name:         "Wesfarmers",
						Units:        25,
						AvgCost:      100.00,
						CurrentPrice: 120.00,
						MarketValue:  3000.00,
						NetReturn:    500.00,
						TotalCost:    2500.00,
						Currency:     "AUD",
					},
				},
				LastSynced: time.Now().Truncate(time.Second),
			},
		},
		{
			name: "with_fx_and_original_currency",
			portfolio: models.Portfolio{
				Name:           "dv_fx",
				TotalValue:     15000.00,
				TotalCost:      12000.00,
				TotalNetReturn: 3000.00,
				Currency:       "AUD",
				FXRate:         0.6500,
				DataVersion:    common.SchemaVersion,
				Holdings: []models.Holding{
					{
						Ticker:             "BHP",
						Exchange:           "AU",
						Name:               "BHP Group",
						Units:              100,
						AvgCost:            80.00,
						CurrentPrice:       100.00,
						MarketValue:        10000.00,
						NetReturn:          2000.00,
						TotalCost:          8000.00,
						Currency:           "AUD",
						TrueBreakevenPrice: &breakeven,
					},
					{
						Ticker:           "CBOE",
						Exchange:         "US",
						Name:             "Cboe Global Markets",
						Units:            20,
						AvgCost:          250.00,
						CurrentPrice:     250.00,
						MarketValue:      5000.00,
						NetReturn:        1000.00,
						TotalCost:        4000.00,
						Currency:         "AUD", // converted to AUD
						OriginalCurrency: "USD", // was originally USD
					},
				},
				LastSynced: time.Now().Truncate(time.Second),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.portfolio)
			require.NoError(t, err, "marshal portfolio")

			record := &models.UserRecord{
				UserID:   "dv_test_user",
				Subject:  "portfolio",
				Key:      tt.portfolio.Name,
				Value:    string(data),
				Version:  1,
				DateTime: time.Now().Truncate(time.Second),
			}
			require.NoError(t, store.Put(ctx, record), "store portfolio")

			// Retrieve
			got, err := store.Get(ctx, "dv_test_user", "portfolio", tt.portfolio.Name)
			require.NoError(t, err, "get portfolio")

			var restored models.Portfolio
			require.NoError(t, json.Unmarshal([]byte(got.Value), &restored), "unmarshal portfolio")

			// Verify DataVersion survived roundtrip
			assert.Equal(t, tt.portfolio.DataVersion, restored.DataVersion,
				"DataVersion should survive storage roundtrip")

			// Verify other portfolio fields
			assert.Equal(t, tt.portfolio.Name, restored.Name)
			assert.InDelta(t, tt.portfolio.TotalValue, restored.TotalValue, 0.01)
			assert.InDelta(t, tt.portfolio.FXRate, restored.FXRate, 0.0001)

			// Verify holdings
			require.Len(t, restored.Holdings, len(tt.portfolio.Holdings))
			for i, expected := range tt.portfolio.Holdings {
				actual := restored.Holdings[i]
				assert.Equal(t, expected.Currency, actual.Currency,
					"holding[%d] Currency", i)
				assert.Equal(t, expected.OriginalCurrency, actual.OriginalCurrency,
					"holding[%d] OriginalCurrency", i)
			}
		})
	}
}

// TestPortfolioDataVersionMismatchDetection verifies that code can detect a
// DataVersion mismatch by comparing the stored version against the current
// SchemaVersion. This is the foundation of cache invalidation.
func TestPortfolioDataVersionMismatchDetection(t *testing.T) {
	mgr := testManager(t)
	store := mgr.UserDataStore()
	ctx := testContext()

	tests := []struct {
		name        string
		dataVersion string
		expectStale bool
	}{
		{
			name:        "current_version_is_fresh",
			dataVersion: common.SchemaVersion,
			expectStale: false,
		},
		{
			name:        "old_version_is_stale",
			dataVersion: "5",
			expectStale: true,
		},
		{
			name:        "empty_version_is_stale",
			dataVersion: "",
			expectStale: true,
		},
		{
			name:        "future_version_is_stale",
			dataVersion: "999",
			expectStale: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			portfolio := models.Portfolio{
				Name:           "mismatch_" + tt.name,
				TotalValue:     1000.00,
				TotalCost:      800.00,
				TotalNetReturn: 200.00,
				DataVersion:    tt.dataVersion,
				Holdings: []models.Holding{
					{
						Ticker:       "TST",
						Exchange:     "AU",
						Units:        10,
						CurrentPrice: 100.00,
						MarketValue:  1000.00,
						TotalCost:    800.00,
						Currency:     "AUD",
					},
				},
				LastSynced: time.Now().Truncate(time.Second),
			}

			data, err := json.Marshal(portfolio)
			require.NoError(t, err)

			require.NoError(t, store.Put(ctx, &models.UserRecord{
				UserID:   "mismatch_user",
				Subject:  "portfolio",
				Key:      portfolio.Name,
				Value:    string(data),
				Version:  1,
				DateTime: time.Now().Truncate(time.Second),
			}))

			// Retrieve and check version
			got, err := store.Get(ctx, "mismatch_user", "portfolio", portfolio.Name)
			require.NoError(t, err)

			var restored models.Portfolio
			require.NoError(t, json.Unmarshal([]byte(got.Value), &restored))

			isStale := restored.DataVersion != common.SchemaVersion
			assert.Equal(t, tt.expectStale, isStale,
				"DataVersion=%q vs SchemaVersion=%q: stale=%v expected=%v",
				restored.DataVersion, common.SchemaVersion, isStale, tt.expectStale)
		})
	}
}

// TestPortfolioOriginalCurrencyRoundtrip verifies that the OriginalCurrency field
// on holdings survives storage roundtrip and is correctly absent for non-converted holdings.
func TestPortfolioOriginalCurrencyRoundtrip(t *testing.T) {
	mgr := testManager(t)
	store := mgr.UserDataStore()
	ctx := testContext()

	portfolio := models.Portfolio{
		Name:           "oc_roundtrip",
		TotalValue:     20000.00,
		TotalCost:      16000.00,
		TotalNetReturn: 4000.00,
		Currency:       "AUD",
		FXRate:         0.6500,
		DataVersion:    common.SchemaVersion,
		Holdings: []models.Holding{
			{
				Ticker:       "BHP",
				Exchange:     "AU",
				Name:         "BHP Group",
				Units:        100,
				AvgCost:      80.00,
				CurrentPrice: 100.00,
				MarketValue:  10000.00,
				NetReturn:    2000.00,
				TotalCost:    8000.00,
				Currency:     "AUD",
				// No OriginalCurrency -- native AUD
			},
			{
				Ticker:           "CBOE",
				Exchange:         "US",
				Name:             "Cboe Global Markets",
				Units:            20,
				AvgCost:          250.00,
				CurrentPrice:     250.00,
				MarketValue:      5000.00,
				NetReturn:        1000.00,
				TotalCost:        4000.00,
				Currency:         "AUD",
				OriginalCurrency: "USD",
			},
			{
				Ticker:           "MSFT",
				Exchange:         "US",
				Name:             "Microsoft",
				Units:            10,
				AvgCost:          500.00,
				CurrentPrice:     500.00,
				MarketValue:      5000.00,
				NetReturn:        1000.00,
				TotalCost:        4000.00,
				Currency:         "AUD",
				OriginalCurrency: "USD",
			},
		},
		LastSynced: time.Now().Truncate(time.Second),
	}

	data, err := json.Marshal(portfolio)
	require.NoError(t, err)

	require.NoError(t, store.Put(ctx, &models.UserRecord{
		UserID:   "oc_user",
		Subject:  "portfolio",
		Key:      portfolio.Name,
		Value:    string(data),
		Version:  1,
		DateTime: time.Now().Truncate(time.Second),
	}))

	got, err := store.Get(ctx, "oc_user", "portfolio", "oc_roundtrip")
	require.NoError(t, err)

	var restored models.Portfolio
	require.NoError(t, json.Unmarshal([]byte(got.Value), &restored))

	require.Len(t, restored.Holdings, 3)

	t.Run("aud_holding_no_original_currency", func(t *testing.T) {
		h := restored.Holdings[0]
		assert.Equal(t, "BHP", h.Ticker)
		assert.Equal(t, "AUD", h.Currency)
		assert.Empty(t, h.OriginalCurrency,
			"AUD holding should not have OriginalCurrency set")
	})

	t.Run("usd_holding_has_original_currency", func(t *testing.T) {
		h := restored.Holdings[1]
		assert.Equal(t, "CBOE", h.Ticker)
		assert.Equal(t, "AUD", h.Currency)
		assert.Equal(t, "USD", h.OriginalCurrency,
			"converted USD holding should have OriginalCurrency=USD")
	})

	t.Run("second_usd_holding_has_original_currency", func(t *testing.T) {
		h := restored.Holdings[2]
		assert.Equal(t, "MSFT", h.Ticker)
		assert.Equal(t, "AUD", h.Currency)
		assert.Equal(t, "USD", h.OriginalCurrency)
	})

	// Verify JSON omitempty: OriginalCurrency should be absent for AUD holdings
	t.Run("json_omitempty_original_currency", func(t *testing.T) {
		var raw map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &raw))

		holdings, ok := raw["holdings"].([]interface{})
		require.True(t, ok)
		require.Len(t, holdings, 3)

		// BHP (AUD): original_currency should be absent
		bhpRaw := holdings[0].(map[string]interface{})
		_, hasOC := bhpRaw["original_currency"]
		assert.False(t, hasOC,
			"BHP (AUD) should not have original_currency in JSON (omitempty)")

		// CBOE (USD converted): original_currency should be present
		cboeRaw := holdings[1].(map[string]interface{})
		assert.Equal(t, "USD", cboeRaw["original_currency"],
			"CBOE should have original_currency=USD in JSON")
	})
}
