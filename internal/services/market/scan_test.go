package market

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- helpers ---

// makeEODBars generates n EOD bars starting from today going backwards (descending order).
func makeEODBars(n int, baseClose float64) []models.EODBar {
	bars := make([]models.EODBar, n)
	now := time.Now()
	for i := 0; i < n; i++ {
		price := baseClose + float64(i)*0.10 // slight variation
		bars[i] = models.EODBar{
			Date:   now.AddDate(0, 0, -i),
			Open:   price - 0.50,
			High:   price + 1.00,
			Low:    price - 1.00,
			Close:  price,
			Volume: int64(1000000 + i*10000),
		}
	}
	return bars
}

// scanTestStorage implements interfaces.StorageManager with signal storage that returns data.
type scanTestStorage struct {
	market  *mockMarketDataStorage
	signals *scanTestSignalStorage
	index   *bulkTestStockIndex
}

func (m *scanTestStorage) MarketDataStorage() interfaces.MarketDataStorage { return m.market }
func (m *scanTestStorage) SignalStorage() interfaces.SignalStorage         { return m.signals }
func (m *scanTestStorage) InternalStore() interfaces.InternalStore         { return nil }
func (m *scanTestStorage) UserDataStore() interfaces.UserDataStore         { return nil }
func (m *scanTestStorage) StockIndexStore() interfaces.StockIndexStore     { return m.index }
func (m *scanTestStorage) JobQueueStore() interfaces.JobQueueStore         { return nil }
func (m *scanTestStorage) FileStore() interfaces.FileStore {
	return &mockFileStore{files: make(map[string][]byte)}
}
func (m *scanTestStorage) FeedbackStore() interfaces.FeedbackStore        { return nil }
func (m *scanTestStorage) OAuthStore() interfaces.OAuthStore              { return nil }
func (m *scanTestStorage) DataPath() string                               { return "" }
func (m *scanTestStorage) WriteRaw(subdir, key string, data []byte) error { return nil }
func (m *scanTestStorage) PurgeDerivedData(_ context.Context) (map[string]int, error) {
	return nil, nil
}
func (m *scanTestStorage) PurgeReports(_ context.Context) (int, error) { return 0, nil }
func (m *scanTestStorage) Close() error                                { return nil }

// makeScanTestData creates mock storage with market data and signals for multiple tickers.
func makeScanTestData(tickers []string, marketDataMap map[string]*models.MarketData, signalsMap map[string]*models.TickerSignals) *scanTestStorage {
	mdStorage := &mockMarketDataStorage{data: make(map[string]*models.MarketData)}
	for k, v := range marketDataMap {
		mdStorage.data[k] = v
	}

	sigStorage := &scanTestSignalStorage{data: make(map[string]*models.TickerSignals)}
	for k, v := range signalsMap {
		sigStorage.data[k] = v
	}

	entries := make([]*models.StockIndexEntry, len(tickers))
	for i, t := range tickers {
		code := t
		exchange := ""
		for j := len(t) - 1; j >= 0; j-- {
			if t[j] == '.' {
				code = t[:j]
				exchange = t[j+1:]
				break
			}
		}
		entries[i] = &models.StockIndexEntry{
			Ticker:   t,
			Code:     code,
			Exchange: exchange,
		}
	}

	return &scanTestStorage{
		market:  mdStorage,
		signals: sigStorage,
		index:   newBulkTestStockIndex(entries...),
	}
}

// scanTestSignalStorage is a mock that returns stored signals.
type scanTestSignalStorage struct {
	data map[string]*models.TickerSignals
}

func (m *scanTestSignalStorage) GetSignals(_ context.Context, ticker string) (*models.TickerSignals, error) {
	if sig, ok := m.data[ticker]; ok {
		return sig, nil
	}
	return nil, nil
}
func (m *scanTestSignalStorage) SaveSignals(_ context.Context, sig *models.TickerSignals) error {
	m.data[sig.Ticker] = sig
	return nil
}
func (m *scanTestSignalStorage) GetSignalsBatch(_ context.Context, tickers []string) ([]*models.TickerSignals, error) {
	var result []*models.TickerSignals
	for _, t := range tickers {
		if sig, ok := m.data[t]; ok {
			result = append(result, sig)
		}
	}
	return result, nil
}

// newTestScanner creates a Scanner with mock storage for testing.
func newTestScanner(storage interfaces.StorageManager) *Scanner {
	logger := common.NewLogger("error")
	return NewScanner(storage, logger)
}

// --- Filter operator tests ---

func TestScan_FilterOperators(t *testing.T) {
	now := time.Now()
	tickers := []string{"BHP.AU", "RIO.AU", "WOW.AU"}

	mdMap := map[string]*models.MarketData{
		"BHP.AU": {
			Ticker: "BHP.AU", Exchange: "AU", Name: "BHP Group",
			EOD:          makeEODBars(300, 42.0),
			Fundamentals: &models.Fundamentals{PE: 12.5, Sector: "Materials", MarketCap: 150_000_000_000, EPS: 3.36, Beta: 1.1},
			LastUpdated:  now,
		},
		"RIO.AU": {
			Ticker: "RIO.AU", Exchange: "AU", Name: "Rio Tinto",
			EOD:          makeEODBars(300, 110.0),
			Fundamentals: &models.Fundamentals{PE: 8.0, Sector: "Materials", MarketCap: 120_000_000_000, EPS: 13.75, Beta: 0.9},
			LastUpdated:  now,
		},
		"WOW.AU": {
			Ticker: "WOW.AU", Exchange: "AU", Name: "Woolworths",
			EOD:          makeEODBars(300, 35.0),
			Fundamentals: &models.Fundamentals{PE: 25.0, Sector: "Consumer Staples", MarketCap: 40_000_000_000, EPS: 1.40, Beta: 0.5},
			LastUpdated:  now,
		},
	}

	sigMap := map[string]*models.TickerSignals{
		"BHP.AU": {Ticker: "BHP.AU", Price: models.PriceSignals{Current: 42.0, SMA20: 41.0, SMA50: 40.0, SMA200: 38.0}, Technical: models.TechnicalSignals{RSI: 55.0}},
		"RIO.AU": {Ticker: "RIO.AU", Price: models.PriceSignals{Current: 110.0, SMA20: 108.0, SMA50: 105.0, SMA200: 100.0}, Technical: models.TechnicalSignals{RSI: 62.0}},
		"WOW.AU": {Ticker: "WOW.AU", Price: models.PriceSignals{Current: 35.0, SMA20: 34.5, SMA50: 34.0, SMA200: 33.0}, Technical: models.TechnicalSignals{RSI: 48.0}},
	}

	storage := makeScanTestData(tickers, mdMap, sigMap)
	scanner := newTestScanner(storage)

	tests := []struct {
		name          string
		filters       []models.ScanFilter
		fields        []string
		expectedCount int
		checkTickers  []string // tickers expected in results
	}{
		{
			name:          "eq operator on sector",
			filters:       []models.ScanFilter{{Field: "sector", Op: "==", Value: "Materials"}},
			fields:        []string{"ticker", "sector"},
			expectedCount: 2,
			checkTickers:  []string{"BHP.AU", "RIO.AU"},
		},
		{
			name:          "ne operator on sector",
			filters:       []models.ScanFilter{{Field: "sector", Op: "!=", Value: "Materials"}},
			fields:        []string{"ticker", "sector"},
			expectedCount: 1,
			checkTickers:  []string{"WOW.AU"},
		},
		{
			name:          "lt operator on pe_ratio",
			filters:       []models.ScanFilter{{Field: "pe_ratio", Op: "<", Value: 10.0}},
			fields:        []string{"ticker", "pe_ratio"},
			expectedCount: 1,
			checkTickers:  []string{"RIO.AU"},
		},
		{
			name:          "le operator on pe_ratio",
			filters:       []models.ScanFilter{{Field: "pe_ratio", Op: "<=", Value: 12.5}},
			fields:        []string{"ticker", "pe_ratio"},
			expectedCount: 2,
			checkTickers:  []string{"BHP.AU", "RIO.AU"},
		},
		{
			name:          "gt operator on pe_ratio",
			filters:       []models.ScanFilter{{Field: "pe_ratio", Op: ">", Value: 20.0}},
			fields:        []string{"ticker", "pe_ratio"},
			expectedCount: 1,
			checkTickers:  []string{"WOW.AU"},
		},
		{
			name:          "ge operator on pe_ratio",
			filters:       []models.ScanFilter{{Field: "pe_ratio", Op: ">=", Value: 12.5}},
			fields:        []string{"ticker", "pe_ratio"},
			expectedCount: 2,
			checkTickers:  []string{"BHP.AU", "WOW.AU"},
		},
		{
			name: "between operator on pe_ratio",
			filters: []models.ScanFilter{{Field: "pe_ratio", Op: "between", Value: []interface{}{
				float64(7), float64(13),
			}}},
			fields:        []string{"ticker", "pe_ratio"},
			expectedCount: 2,
			checkTickers:  []string{"BHP.AU", "RIO.AU"},
		},
		{
			name:          "in operator on sector",
			filters:       []models.ScanFilter{{Field: "sector", Op: "in", Value: []interface{}{"Materials", "Technology"}}},
			fields:        []string{"ticker", "sector"},
			expectedCount: 2,
			checkTickers:  []string{"BHP.AU", "RIO.AU"},
		},
		{
			name:          "not_in operator on sector",
			filters:       []models.ScanFilter{{Field: "sector", Op: "not_in", Value: []interface{}{"Materials"}}},
			fields:        []string{"ticker", "sector"},
			expectedCount: 1,
			checkTickers:  []string{"WOW.AU"},
		},
		{
			name:          "AND composition - multiple filters",
			filters:       []models.ScanFilter{{Field: "sector", Op: "==", Value: "Materials"}, {Field: "pe_ratio", Op: "<", Value: 10.0}},
			fields:        []string{"ticker", "pe_ratio", "sector"},
			expectedCount: 1,
			checkTickers:  []string{"RIO.AU"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := models.ScanQuery{
				Exchange: "AU",
				Filters:  tt.filters,
				Fields:   tt.fields,
				Limit:    50,
			}

			resp, err := scanner.Scan(context.Background(), query)
			require.NoError(t, err)
			require.NotNil(t, resp)
			assert.Equal(t, tt.expectedCount, len(resp.Results), "result count mismatch")

			// Verify expected tickers are present
			resultTickers := make([]string, 0, len(resp.Results))
			for _, r := range resp.Results {
				if ticker, ok := r["ticker"]; ok {
					resultTickers = append(resultTickers, ticker.(string))
				}
			}
			for _, expected := range tt.checkTickers {
				assert.Contains(t, resultTickers, expected, "expected ticker %s in results", expected)
			}
		})
	}
}

func TestScan_FilterNullOperators(t *testing.T) {
	now := time.Now()
	tickers := []string{"BHP.AU", "NULL.AU"}

	mdMap := map[string]*models.MarketData{
		"BHP.AU": {
			Ticker: "BHP.AU", Exchange: "AU", Name: "BHP Group",
			EOD:          makeEODBars(10, 42.0),
			Fundamentals: &models.Fundamentals{PE: 12.5, Sector: "Materials"},
			LastUpdated:  now,
		},
		"NULL.AU": {
			Ticker: "NULL.AU", Exchange: "AU", Name: "No Fundamentals",
			EOD:         makeEODBars(10, 5.0),
			LastUpdated: now,
			// Fundamentals is nil -- fields like pe_ratio should be null
		},
	}

	storage := makeScanTestData(tickers, mdMap, nil)
	scanner := newTestScanner(storage)

	t.Run("is_null finds tickers with nil fields", func(t *testing.T) {
		query := models.ScanQuery{
			Exchange: "AU",
			Filters:  []models.ScanFilter{{Field: "pe_ratio", Op: "is_null"}},
			Fields:   []string{"ticker"},
			Limit:    50,
		}
		resp, err := scanner.Scan(context.Background(), query)
		require.NoError(t, err)
		assert.Equal(t, 1, len(resp.Results))
		assert.Equal(t, "NULL.AU", resp.Results[0]["ticker"])
	})

	t.Run("not_null excludes tickers with nil fields", func(t *testing.T) {
		query := models.ScanQuery{
			Exchange: "AU",
			Filters:  []models.ScanFilter{{Field: "pe_ratio", Op: "not_null"}},
			Fields:   []string{"ticker"},
			Limit:    50,
		}
		resp, err := scanner.Scan(context.Background(), query)
		require.NoError(t, err)
		assert.Equal(t, 1, len(resp.Results))
		assert.Equal(t, "BHP.AU", resp.Results[0]["ticker"])
	})

	t.Run("comparison operators return false for nil field values", func(t *testing.T) {
		query := models.ScanQuery{
			Exchange: "AU",
			Filters:  []models.ScanFilter{{Field: "pe_ratio", Op: ">", Value: 0.0}},
			Fields:   []string{"ticker"},
			Limit:    50,
		}
		resp, err := scanner.Scan(context.Background(), query)
		require.NoError(t, err)
		// Only BHP.AU should match — NULL.AU has nil pe_ratio
		assert.Equal(t, 1, len(resp.Results))
		assert.Equal(t, "BHP.AU", resp.Results[0]["ticker"])
	})
}

func TestScan_ORGroupFilters(t *testing.T) {
	now := time.Now()
	tickers := []string{"BHP.AU", "RIO.AU", "WOW.AU"}

	mdMap := map[string]*models.MarketData{
		"BHP.AU": {
			Ticker: "BHP.AU", Exchange: "AU", Name: "BHP Group",
			EOD:          makeEODBars(10, 42.0),
			Fundamentals: &models.Fundamentals{PE: 12.5, Sector: "Materials"},
			LastUpdated:  now,
		},
		"RIO.AU": {
			Ticker: "RIO.AU", Exchange: "AU", Name: "Rio Tinto",
			EOD:          makeEODBars(10, 110.0),
			Fundamentals: &models.Fundamentals{PE: 8.0, Sector: "Materials"},
			LastUpdated:  now,
		},
		"WOW.AU": {
			Ticker: "WOW.AU", Exchange: "AU", Name: "Woolworths",
			EOD:          makeEODBars(10, 35.0),
			Fundamentals: &models.Fundamentals{PE: 25.0, Sector: "Consumer Staples"},
			LastUpdated:  now,
		},
	}

	storage := makeScanTestData(tickers, mdMap, nil)
	scanner := newTestScanner(storage)

	t.Run("OR group matches any sub-filter", func(t *testing.T) {
		query := models.ScanQuery{
			Exchange: "AU",
			Filters: []models.ScanFilter{
				{
					Or: []models.ScanFilter{
						{Field: "sector", Op: "==", Value: "Consumer Staples"},
						{Field: "pe_ratio", Op: "<", Value: 10.0},
					},
				},
			},
			Fields: []string{"ticker", "sector", "pe_ratio"},
			Limit:  50,
		}
		resp, err := scanner.Scan(context.Background(), query)
		require.NoError(t, err)
		// RIO (PE=8) and WOW (Consumer Staples) should match
		assert.Equal(t, 2, len(resp.Results))
	})

	t.Run("OR group ANDed with other filters", func(t *testing.T) {
		query := models.ScanQuery{
			Exchange: "AU",
			Filters: []models.ScanFilter{
				{Field: "sector", Op: "==", Value: "Materials"}, // AND: must be Materials
				{
					Or: []models.ScanFilter{
						{Field: "pe_ratio", Op: "<", Value: 10.0},
						{Field: "pe_ratio", Op: ">", Value: 20.0},
					},
				},
			},
			Fields: []string{"ticker", "sector", "pe_ratio"},
			Limit:  50,
		}
		resp, err := scanner.Scan(context.Background(), query)
		require.NoError(t, err)
		// Only RIO.AU: Materials AND (PE<10 OR PE>20), PE=8 passes
		assert.Equal(t, 1, len(resp.Results))
		assert.Equal(t, "RIO.AU", resp.Results[0]["ticker"])
	})
}

// --- Sorting tests ---

func TestScan_Sorting(t *testing.T) {
	now := time.Now()
	tickers := []string{"BHP.AU", "RIO.AU", "WOW.AU"}

	mdMap := map[string]*models.MarketData{
		"BHP.AU": {
			Ticker: "BHP.AU", Exchange: "AU", Name: "BHP Group",
			EOD:          makeEODBars(10, 42.0),
			Fundamentals: &models.Fundamentals{PE: 12.5, Sector: "Materials", MarketCap: 150_000_000_000},
			LastUpdated:  now,
		},
		"RIO.AU": {
			Ticker: "RIO.AU", Exchange: "AU", Name: "Rio Tinto",
			EOD:          makeEODBars(10, 110.0),
			Fundamentals: &models.Fundamentals{PE: 8.0, Sector: "Materials", MarketCap: 120_000_000_000},
			LastUpdated:  now,
		},
		"WOW.AU": {
			Ticker: "WOW.AU", Exchange: "AU", Name: "Woolworths",
			EOD:          makeEODBars(10, 35.0),
			Fundamentals: &models.Fundamentals{PE: 25.0, Sector: "Consumer Staples", MarketCap: 40_000_000_000},
			LastUpdated:  now,
		},
	}

	storage := makeScanTestData(tickers, mdMap, nil)
	scanner := newTestScanner(storage)

	t.Run("sort ascending by pe_ratio", func(t *testing.T) {
		sortJSON, _ := json.Marshal(models.ScanSort{Field: "pe_ratio", Order: "asc"})
		query := models.ScanQuery{
			Exchange: "AU",
			Fields:   []string{"ticker", "pe_ratio"},
			Sort:     json.RawMessage(sortJSON),
			Limit:    50,
		}
		resp, err := scanner.Scan(context.Background(), query)
		require.NoError(t, err)
		require.Equal(t, 3, len(resp.Results))
		// RIO (8) < BHP (12.5) < WOW (25)
		assert.Equal(t, "RIO.AU", resp.Results[0]["ticker"])
		assert.Equal(t, "WOW.AU", resp.Results[2]["ticker"])
	})

	t.Run("sort descending by market_cap", func(t *testing.T) {
		sortJSON, _ := json.Marshal(models.ScanSort{Field: "market_cap", Order: "desc"})
		query := models.ScanQuery{
			Exchange: "AU",
			Fields:   []string{"ticker", "market_cap"},
			Sort:     json.RawMessage(sortJSON),
			Limit:    50,
		}
		resp, err := scanner.Scan(context.Background(), query)
		require.NoError(t, err)
		require.Equal(t, 3, len(resp.Results))
		// BHP (150B) > RIO (120B) > WOW (40B)
		assert.Equal(t, "BHP.AU", resp.Results[0]["ticker"])
		assert.Equal(t, "WOW.AU", resp.Results[2]["ticker"])
	})

	t.Run("multi-field sort", func(t *testing.T) {
		sortJSON, _ := json.Marshal([]models.ScanSort{
			{Field: "market_cap", Order: "desc"},
			{Field: "pe_ratio", Order: "asc"},
		})
		query := models.ScanQuery{
			Exchange: "AU",
			Fields:   []string{"ticker", "market_cap", "pe_ratio"},
			Sort:     json.RawMessage(sortJSON),
			Limit:    50,
		}
		resp, err := scanner.Scan(context.Background(), query)
		require.NoError(t, err)
		require.Equal(t, 3, len(resp.Results))
		// Market cap desc: BHP (150B) > RIO (120B) > WOW (40B)
		assert.Equal(t, "BHP.AU", resp.Results[0]["ticker"])
		assert.Equal(t, "RIO.AU", resp.Results[1]["ticker"])
		assert.Equal(t, "WOW.AU", resp.Results[2]["ticker"])
	})
}

// --- Limit tests ---

func TestScan_LimitEnforcement(t *testing.T) {
	now := time.Now()
	tickers := []string{"A.AU", "B.AU", "C.AU", "D.AU", "E.AU"}

	mdMap := make(map[string]*models.MarketData)
	for _, tk := range tickers {
		mdMap[tk] = &models.MarketData{
			Ticker:       tk,
			Exchange:     "AU",
			Name:         tk,
			EOD:          makeEODBars(10, 10.0),
			Fundamentals: &models.Fundamentals{PE: 10.0, Sector: "Test"},
			LastUpdated:  now,
		}
	}

	storage := makeScanTestData(tickers, mdMap, nil)
	scanner := newTestScanner(storage)

	t.Run("limit restricts results", func(t *testing.T) {
		query := models.ScanQuery{
			Exchange: "AU",
			Fields:   []string{"ticker"},
			Limit:    3,
		}
		resp, err := scanner.Scan(context.Background(), query)
		require.NoError(t, err)
		assert.Equal(t, 3, len(resp.Results))
		assert.Equal(t, 5, resp.Meta.TotalMatched)
	})

	t.Run("default limit applied when zero", func(t *testing.T) {
		query := models.ScanQuery{
			Exchange: "AU",
			Fields:   []string{"ticker"},
			Limit:    0, // should default to 20
		}
		resp, err := scanner.Scan(context.Background(), query)
		require.NoError(t, err)
		assert.Equal(t, 5, len(resp.Results)) // fewer than default limit
	})

	t.Run("limit capped at maximum", func(t *testing.T) {
		query := models.ScanQuery{
			Exchange: "AU",
			Fields:   []string{"ticker"},
			Limit:    100, // exceeds max of 50
		}
		resp, err := scanner.Scan(context.Background(), query)
		require.NoError(t, err)
		assert.Equal(t, 5, len(resp.Results))
		assert.True(t, resp.Meta.Returned <= 50)
	})
}

// --- Metadata tests ---

func TestScan_ResponseMetadata(t *testing.T) {
	now := time.Now()
	tickers := []string{"BHP.AU"}

	mdMap := map[string]*models.MarketData{
		"BHP.AU": {
			Ticker:       "BHP.AU",
			Exchange:     "AU",
			Name:         "BHP Group",
			EOD:          makeEODBars(10, 42.0),
			Fundamentals: &models.Fundamentals{PE: 12.5, Sector: "Materials"},
			LastUpdated:  now,
		},
	}

	storage := makeScanTestData(tickers, mdMap, nil)
	scanner := newTestScanner(storage)

	query := models.ScanQuery{
		Exchange: "AU",
		Fields:   []string{"ticker", "pe_ratio"},
		Limit:    20,
	}

	resp, err := scanner.Scan(context.Background(), query)
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, "AU", resp.Meta.Exchange)
	assert.Equal(t, 1, resp.Meta.TotalMatched)
	assert.Equal(t, 1, resp.Meta.Returned)
	assert.False(t, resp.Meta.ExecutedAt.IsZero())
	assert.True(t, resp.Meta.QueryTimeMS >= 0)
}

// --- Field extraction tests ---

func TestScan_FieldExtraction(t *testing.T) {
	now := time.Now()
	tickers := []string{"BHP.AU"}

	mdMap := map[string]*models.MarketData{
		"BHP.AU": {
			Ticker:   "BHP.AU",
			Exchange: "AU",
			Name:     "BHP Group",
			EOD:      makeEODBars(300, 42.0),
			Fundamentals: &models.Fundamentals{
				PE:            12.5,
				PB:            2.1,
				EPS:           3.36,
				Sector:        "Materials",
				Industry:      "Mining",
				MarketCap:     150_000_000_000,
				DividendYield: 0.035,
				Beta:          1.1,
				CountryISO:    "AU",
			},
			LastUpdated: now,
		},
	}

	sigMap := map[string]*models.TickerSignals{
		"BHP.AU": {
			Ticker: "BHP.AU",
			Price: models.PriceSignals{
				Current: 42.0, SMA20: 41.0, SMA50: 40.0, SMA200: 38.0,
				DistanceToSMA20: 2.44, DistanceToSMA50: 5.0, DistanceToSMA200: 10.53,
			},
			Technical: models.TechnicalSignals{
				RSI: 55.0, MACD: 0.5, MACDSignal: 0.3, MACDHistogram: 0.2,
				ATR: 1.2, ATRPct: 2.86,
			},
			Trend: models.TrendBullish,
		},
	}

	storage := makeScanTestData(tickers, mdMap, sigMap)
	scanner := newTestScanner(storage)

	query := models.ScanQuery{
		Exchange: "AU",
		Fields: []string{
			"ticker", "name", "exchange", "sector", "industry", "country",
			"market_cap", "pe_ratio", "pb_ratio", "eps_ttm",
			"dividend_yield_pct", "beta",
			"price", "sma_20", "sma_50", "sma_200",
			"price_vs_sma_20_pct", "price_vs_sma_50_pct", "price_vs_sma_200_pct",
			"rsi_14", "macd", "macd_signal", "macd_histogram",
			"atr", "atr_pct",
		},
		Limit: 50,
	}

	resp, err := scanner.Scan(context.Background(), query)
	require.NoError(t, err)
	require.Equal(t, 1, len(resp.Results))

	r := resp.Results[0]

	// Identity fields
	assert.Equal(t, "BHP.AU", r["ticker"])
	assert.Equal(t, "BHP Group", r["name"])
	assert.Equal(t, "AU", r["exchange"])
	assert.Equal(t, "Materials", r["sector"])
	assert.Equal(t, "Mining", r["industry"])
	assert.Equal(t, "AU", r["country"])

	// Fundamental fields
	assert.InDelta(t, 150_000_000_000.0, r["market_cap"], 1.0)
	assert.InDelta(t, 12.5, r["pe_ratio"], 0.01)
	assert.InDelta(t, 2.1, r["pb_ratio"], 0.01)
	assert.InDelta(t, 3.36, r["eps_ttm"], 0.01)
	assert.InDelta(t, 3.5, r["dividend_yield_pct"], 0.1)
	assert.InDelta(t, 1.1, r["beta"], 0.01)

	// Price fields from signals
	assert.InDelta(t, 42.0, r["price"], 0.1)
	assert.InDelta(t, 41.0, r["sma_20"], 0.1)
	assert.InDelta(t, 40.0, r["sma_50"], 0.1)
	assert.InDelta(t, 38.0, r["sma_200"], 0.1)

	// Technical fields
	assert.InDelta(t, 55.0, r["rsi_14"], 0.1)
	assert.InDelta(t, 0.5, r["macd"], 0.01)
}

// --- Computed fields tests ---

func TestScan_ComputedFieldsFromEOD(t *testing.T) {
	now := time.Now()

	// Create bars with predictable prices for return calculations
	bars := make([]models.EODBar, 260) // ~1 year of trading days
	for i := 0; i < 260; i++ {
		// Price increases over time (most recent = highest)
		price := 100.0 + float64(260-i)*0.10
		bars[i] = models.EODBar{
			Date:   now.AddDate(0, 0, -i),
			Open:   price - 0.50,
			High:   price + 1.00,
			Low:    price - 1.00,
			Close:  price,
			Volume: int64(1000000 + i*1000),
		}
	}

	tickers := []string{"TEST.AU"}
	mdMap := map[string]*models.MarketData{
		"TEST.AU": {
			Ticker:       "TEST.AU",
			Exchange:     "AU",
			Name:         "Test Stock",
			EOD:          bars,
			Fundamentals: &models.Fundamentals{Sector: "Test"},
			LastUpdated:  now,
		},
	}

	storage := makeScanTestData(tickers, mdMap, nil)
	scanner := newTestScanner(storage)

	query := models.ScanQuery{
		Exchange: "AU",
		Fields: []string{
			"ticker", "price", "volume",
			"52_week_high", "52_week_low",
			"avg_volume_30d", "avg_volume_90d",
		},
		Limit: 50,
	}

	resp, err := scanner.Scan(context.Background(), query)
	require.NoError(t, err)
	require.Equal(t, 1, len(resp.Results))

	r := resp.Results[0]

	// Price should be the most recent bar close
	currentPrice := bars[0].Close
	assert.InDelta(t, currentPrice, r["price"], 0.01)

	// 52-week high/low should be computed from last 252 bars
	if high, ok := r["52_week_high"]; ok {
		assert.Greater(t, high.(float64), 0.0)
	}
	if low, ok := r["52_week_low"]; ok {
		assert.Greater(t, low.(float64), 0.0)
	}

	// Volume averages (returned as int64)
	if avgVol30, ok := r["avg_volume_30d"]; ok && avgVol30 != nil {
		switch v := avgVol30.(type) {
		case int64:
			assert.Greater(t, v, int64(0))
		case float64:
			assert.Greater(t, v, 0.0)
		default:
			t.Errorf("unexpected type for avg_volume_30d: %T", avgVol30)
		}
	}
}

// --- Exchange filter tests ---

func TestScan_ExchangeFilter(t *testing.T) {
	now := time.Now()
	tickers := []string{"BHP.AU", "AAPL.US"}

	mdMap := map[string]*models.MarketData{
		"BHP.AU": {
			Ticker: "BHP.AU", Exchange: "AU", Name: "BHP",
			EOD: makeEODBars(10, 42.0), Fundamentals: &models.Fundamentals{Sector: "Materials"},
			LastUpdated: now,
		},
		"AAPL.US": {
			Ticker: "AAPL.US", Exchange: "US", Name: "Apple",
			EOD: makeEODBars(10, 180.0), Fundamentals: &models.Fundamentals{Sector: "Technology"},
			LastUpdated: now,
		},
	}

	storage := makeScanTestData(tickers, mdMap, nil)
	scanner := newTestScanner(storage)

	t.Run("AU exchange only returns AU tickers", func(t *testing.T) {
		query := models.ScanQuery{
			Exchange: "AU",
			Fields:   []string{"ticker", "exchange"},
			Limit:    50,
		}
		resp, err := scanner.Scan(context.Background(), query)
		require.NoError(t, err)
		assert.Equal(t, 1, len(resp.Results))
		assert.Equal(t, "BHP.AU", resp.Results[0]["ticker"])
	})

	t.Run("US exchange only returns US tickers", func(t *testing.T) {
		query := models.ScanQuery{
			Exchange: "US",
			Fields:   []string{"ticker", "exchange"},
			Limit:    50,
		}
		resp, err := scanner.Scan(context.Background(), query)
		require.NoError(t, err)
		assert.Equal(t, 1, len(resp.Results))
		assert.Equal(t, "AAPL.US", resp.Results[0]["ticker"])
	})
}

// --- Edge cases ---

func TestScan_EdgeCases(t *testing.T) {
	now := time.Now()

	t.Run("empty exchange returns error", func(t *testing.T) {
		tickers := []string{"BHP.AU"}
		mdMap := map[string]*models.MarketData{
			"BHP.AU": {
				Ticker: "BHP.AU", Exchange: "AU", EOD: makeEODBars(10, 42.0),
				Fundamentals: &models.Fundamentals{Sector: "Materials"},
				LastUpdated:  now,
			},
		}
		storage := makeScanTestData(tickers, mdMap, nil)
		scanner := newTestScanner(storage)

		query := models.ScanQuery{
			Exchange: "",
			Fields:   []string{"ticker"},
			Limit:    20,
		}
		_, err := scanner.Scan(context.Background(), query)
		assert.Error(t, err)
	})

	t.Run("empty fields returns error", func(t *testing.T) {
		tickers := []string{"BHP.AU"}
		mdMap := map[string]*models.MarketData{
			"BHP.AU": {
				Ticker: "BHP.AU", Exchange: "AU", EOD: makeEODBars(10, 42.0),
				Fundamentals: &models.Fundamentals{Sector: "Materials"},
				LastUpdated:  now,
			},
		}
		storage := makeScanTestData(tickers, mdMap, nil)
		scanner := newTestScanner(storage)

		query := models.ScanQuery{
			Exchange: "AU",
			Fields:   []string{},
			Limit:    20,
		}
		_, err := scanner.Scan(context.Background(), query)
		assert.Error(t, err)
	})

	t.Run("no matching tickers returns empty results", func(t *testing.T) {
		tickers := []string{"BHP.AU"}
		mdMap := map[string]*models.MarketData{
			"BHP.AU": {
				Ticker: "BHP.AU", Exchange: "AU", EOD: makeEODBars(10, 42.0),
				Fundamentals: &models.Fundamentals{Sector: "Materials"},
				LastUpdated:  now,
			},
		}
		storage := makeScanTestData(tickers, mdMap, nil)
		scanner := newTestScanner(storage)

		query := models.ScanQuery{
			Exchange: "US", // No US tickers in data
			Fields:   []string{"ticker"},
			Limit:    20,
		}
		resp, err := scanner.Scan(context.Background(), query)
		require.NoError(t, err)
		assert.Equal(t, 0, len(resp.Results))
		assert.Equal(t, 0, resp.Meta.TotalMatched)
	})

	t.Run("all filtered out returns empty results", func(t *testing.T) {
		tickers := []string{"BHP.AU"}
		mdMap := map[string]*models.MarketData{
			"BHP.AU": {
				Ticker: "BHP.AU", Exchange: "AU", EOD: makeEODBars(10, 42.0),
				Fundamentals: &models.Fundamentals{PE: 12.5, Sector: "Materials"},
				LastUpdated:  now,
			},
		}
		storage := makeScanTestData(tickers, mdMap, nil)
		scanner := newTestScanner(storage)

		query := models.ScanQuery{
			Exchange: "AU",
			Filters:  []models.ScanFilter{{Field: "pe_ratio", Op: ">", Value: 100.0}},
			Fields:   []string{"ticker"},
			Limit:    20,
		}
		resp, err := scanner.Scan(context.Background(), query)
		require.NoError(t, err)
		assert.Equal(t, 0, len(resp.Results))
	})
}

// --- Fields introspection tests ---

func TestScan_FieldsIntrospection(t *testing.T) {
	storage := makeScanTestData(nil, nil, nil)
	scanner := newTestScanner(storage)

	fieldsResp := scanner.Fields()
	require.NotNil(t, fieldsResp)

	t.Run("has groups", func(t *testing.T) {
		assert.Greater(t, len(fieldsResp.Groups), 0, "should have at least one field group")
	})

	t.Run("has exchanges", func(t *testing.T) {
		assert.Contains(t, fieldsResp.Exchanges, "AU")
		assert.Contains(t, fieldsResp.Exchanges, "US")
	})

	t.Run("has max_limit", func(t *testing.T) {
		assert.Greater(t, fieldsResp.MaxLimit, 0)
	})

	t.Run("fields have required metadata", func(t *testing.T) {
		totalFields := 0
		for _, group := range fieldsResp.Groups {
			assert.NotEmpty(t, group.Name, "group name should not be empty")
			for _, field := range group.Fields {
				assert.NotEmpty(t, field.Field, "field name should not be empty")
				assert.NotEmpty(t, field.Type, "field type should not be empty for %s", field.Field)
				assert.NotEmpty(t, field.Description, "field description should not be empty for %s", field.Field)
				totalFields++
			}
		}
		// The spec defines ~70 fields
		assert.Greater(t, totalFields, 30, "should have at least 30 fields defined")
	})

	t.Run("key fields are present", func(t *testing.T) {
		fieldNames := make(map[string]bool)
		for _, group := range fieldsResp.Groups {
			for _, field := range group.Fields {
				fieldNames[field.Field] = true
			}
		}

		expectedFields := []string{
			"ticker", "name", "exchange", "sector", "market_cap",
			"price", "pe_ratio", "rsi_14", "volume", "sma_200",
		}
		for _, name := range expectedFields {
			assert.True(t, fieldNames[name], "field %q should be present", name)
		}
	})
}

// --- Only requested fields returned ---

func TestScan_OnlyRequestedFieldsReturned(t *testing.T) {
	now := time.Now()
	tickers := []string{"BHP.AU"}

	mdMap := map[string]*models.MarketData{
		"BHP.AU": {
			Ticker: "BHP.AU", Exchange: "AU", Name: "BHP Group",
			EOD:          makeEODBars(10, 42.0),
			Fundamentals: &models.Fundamentals{PE: 12.5, Sector: "Materials", MarketCap: 150_000_000_000},
			LastUpdated:  now,
		},
	}

	storage := makeScanTestData(tickers, mdMap, nil)
	scanner := newTestScanner(storage)

	query := models.ScanQuery{
		Exchange: "AU",
		Fields:   []string{"ticker", "pe_ratio"},
		Limit:    50,
	}

	resp, err := scanner.Scan(context.Background(), query)
	require.NoError(t, err)
	require.Equal(t, 1, len(resp.Results))

	r := resp.Results[0]
	assert.Equal(t, 2, len(r), "should only have 2 fields: ticker and pe_ratio")
	assert.Contains(t, r, "ticker")
	assert.Contains(t, r, "pe_ratio")
	assert.NotContains(t, r, "sector")
	assert.NotContains(t, r, "market_cap")
}

// --- Boolean field tests ---

func TestScan_BooleanFields(t *testing.T) {
	now := time.Now()
	tickers := []string{"BHP.AU", "LOSS.AU"}

	mdMap := map[string]*models.MarketData{
		"BHP.AU": {
			Ticker: "BHP.AU", Exchange: "AU", Name: "BHP Group",
			EOD:          makeEODBars(300, 42.0),
			Fundamentals: &models.Fundamentals{PE: 12.5, EPS: 3.36, Sector: "Materials"},
			LastUpdated:  now,
		},
		"LOSS.AU": {
			Ticker: "LOSS.AU", Exchange: "AU", Name: "Loss Corp",
			EOD:          makeEODBars(300, 5.0),
			Fundamentals: &models.Fundamentals{PE: -5.0, EPS: -1.0, Sector: "Tech"},
			LastUpdated:  now,
		},
	}

	sigMap := map[string]*models.TickerSignals{
		"BHP.AU": {
			Ticker: "BHP.AU",
			Price:  models.PriceSignals{Current: 42.0, SMA20: 41.0, SMA50: 40.0, SMA200: 38.0},
		},
		"LOSS.AU": {
			Ticker: "LOSS.AU",
			Price:  models.PriceSignals{Current: 5.0, SMA20: 6.0, SMA50: 7.0, SMA200: 8.0},
		},
	}

	storage := makeScanTestData(tickers, mdMap, sigMap)
	scanner := newTestScanner(storage)

	t.Run("earnings_positive filter", func(t *testing.T) {
		query := models.ScanQuery{
			Exchange: "AU",
			Filters:  []models.ScanFilter{{Field: "earnings_positive", Op: "==", Value: true}},
			Fields:   []string{"ticker", "earnings_positive"},
			Limit:    50,
		}
		resp, err := scanner.Scan(context.Background(), query)
		require.NoError(t, err)
		assert.Equal(t, 1, len(resp.Results))
		assert.Equal(t, "BHP.AU", resp.Results[0]["ticker"])
	})

	t.Run("sma_50_above_sma_200 filter", func(t *testing.T) {
		query := models.ScanQuery{
			Exchange: "AU",
			Fields:   []string{"ticker", "sma_50_above_sma_200"},
			Limit:    50,
		}
		resp, err := scanner.Scan(context.Background(), query)
		require.NoError(t, err)
		// Both tickers should be present, verify field extraction
		assert.Greater(t, len(resp.Results), 0)
	})
}
