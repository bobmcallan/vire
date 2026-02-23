package market

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// 1. Input validation: extremely long field names
// ============================================================================

func TestStress_Scan_LongFieldName(t *testing.T) {
	storage := makeScanTestData(
		[]string{"BHP.AU"},
		map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", Exchange: "AU", EOD: makeEODBars(10, 42.0), Fundamentals: &models.Fundamentals{Sector: "Materials"}},
		},
		nil,
	)
	scanner := newTestScanner(storage)

	longField := strings.Repeat("a", 10000)
	query := models.ScanQuery{
		Exchange: "AU",
		Fields:   []string{longField},
		Limit:    20,
	}
	_, err := scanner.Scan(context.Background(), query)
	assert.Error(t, err, "long field name should be rejected as unknown")
}

// ============================================================================
// 2. Input validation: extremely long exchange string
// ============================================================================

func TestStress_Scan_LongExchange(t *testing.T) {
	storage := makeScanTestData([]string{"BHP.AU"}, map[string]*models.MarketData{
		"BHP.AU": {Ticker: "BHP.AU", Exchange: "AU", EOD: makeEODBars(10, 42.0)},
	}, nil)
	scanner := newTestScanner(storage)

	query := models.ScanQuery{
		Exchange: strings.Repeat("X", 10000),
		Fields:   []string{"ticker"},
		Limit:    20,
	}
	// Should not panic, just return empty results
	resp, err := scanner.Scan(context.Background(), query)
	require.NoError(t, err)
	assert.Equal(t, 0, len(resp.Results))
}

// ============================================================================
// 3. Input validation: negative limit
// ============================================================================

func TestStress_Scan_NegativeLimit(t *testing.T) {
	storage := makeScanTestData([]string{"BHP.AU"}, map[string]*models.MarketData{
		"BHP.AU": {Ticker: "BHP.AU", Exchange: "AU", EOD: makeEODBars(10, 42.0), Fundamentals: &models.Fundamentals{Sector: "Materials"}},
	}, nil)
	scanner := newTestScanner(storage)

	query := models.ScanQuery{
		Exchange: "AU",
		Fields:   []string{"ticker"},
		Limit:    -5,
	}
	resp, err := scanner.Scan(context.Background(), query)
	require.NoError(t, err)
	// Negative limit should be clamped to default (20)
	assert.LessOrEqual(t, resp.Meta.Returned, 20)
}

// ============================================================================
// 4. Input validation: deeply nested OR groups
// ============================================================================

func TestStress_Scan_DeeplyNestedORGroups(t *testing.T) {
	storage := makeScanTestData([]string{"BHP.AU"}, map[string]*models.MarketData{
		"BHP.AU": {Ticker: "BHP.AU", Exchange: "AU", EOD: makeEODBars(10, 42.0), Fundamentals: &models.Fundamentals{PE: 12.5, Sector: "Materials", MarketCap: 150e9}},
	}, nil)
	scanner := newTestScanner(storage)

	t.Run("nesting within depth limit succeeds", func(t *testing.T) {
		// Build 5-deep nested OR (within maxFilterDepth=10)
		leaf := models.ScanFilter{Field: "pe_ratio", Op: ">", Value: 0.0}
		current := leaf
		for i := 0; i < 5; i++ {
			current = models.ScanFilter{Or: []models.ScanFilter{current}}
		}

		query := models.ScanQuery{
			Exchange: "AU",
			Filters:  []models.ScanFilter{current},
			Fields:   []string{"ticker"},
			Limit:    20,
		}
		resp, err := scanner.Scan(context.Background(), query)
		require.NoError(t, err)
		assert.Equal(t, 1, len(resp.Results))
	})

	t.Run("nesting beyond depth limit rejected", func(t *testing.T) {
		// Build 100-deep nested OR (exceeds maxFilterDepth=10)
		leaf := models.ScanFilter{Field: "pe_ratio", Op: ">", Value: 0.0}
		current := leaf
		for i := 0; i < 100; i++ {
			current = models.ScanFilter{Or: []models.ScanFilter{current}}
		}

		query := models.ScanQuery{
			Exchange: "AU",
			Filters:  []models.ScanFilter{current},
			Fields:   []string{"ticker"},
			Limit:    20,
		}
		_, err := scanner.Scan(context.Background(), query)
		assert.Error(t, err, "deeply nested OR groups should be rejected by depth limit")
		assert.Contains(t, err.Error(), "depth")
	})
}

// ============================================================================
// 5. Input validation: filters referencing non-existent fields
// ============================================================================

func TestStress_Scan_NonExistentFilterField(t *testing.T) {
	storage := makeScanTestData([]string{"BHP.AU"}, map[string]*models.MarketData{
		"BHP.AU": {Ticker: "BHP.AU", Exchange: "AU", EOD: makeEODBars(10, 42.0)},
	}, nil)
	scanner := newTestScanner(storage)

	query := models.ScanQuery{
		Exchange: "AU",
		Filters:  []models.ScanFilter{{Field: "nonexistent_field", Op: "==", Value: "test"}},
		Fields:   []string{"ticker"},
		Limit:    20,
	}
	_, err := scanner.Scan(context.Background(), query)
	assert.Error(t, err, "non-existent filter field should be rejected")
}

// ============================================================================
// 6. Input validation: invalid operator for field type
// ============================================================================

func TestStress_Scan_InvalidOperatorForField(t *testing.T) {
	storage := makeScanTestData([]string{"BHP.AU"}, map[string]*models.MarketData{
		"BHP.AU": {Ticker: "BHP.AU", Exchange: "AU", EOD: makeEODBars(10, 42.0), Fundamentals: &models.Fundamentals{Sector: "Materials"}},
	}, nil)
	scanner := newTestScanner(storage)

	// "sector" is a string field — "<" is not in its operators
	query := models.ScanQuery{
		Exchange: "AU",
		Filters:  []models.ScanFilter{{Field: "sector", Op: "<", Value: "Materials"}},
		Fields:   []string{"ticker"},
		Limit:    20,
	}
	_, err := scanner.Scan(context.Background(), query)
	assert.Error(t, err, "invalid operator for field type should be rejected")
}

// ============================================================================
// 7. Input validation: type confusion — string value for numeric filter
// ============================================================================

func TestStress_Scan_TypeConfusionStringForNumeric(t *testing.T) {
	storage := makeScanTestData([]string{"BHP.AU"}, map[string]*models.MarketData{
		"BHP.AU": {Ticker: "BHP.AU", Exchange: "AU", EOD: makeEODBars(10, 42.0), Fundamentals: &models.Fundamentals{PE: 12.5, Sector: "Materials", MarketCap: 150e9}},
	}, nil)
	scanner := newTestScanner(storage)

	// Pass a string where a number is expected
	query := models.ScanQuery{
		Exchange: "AU",
		Filters:  []models.ScanFilter{{Field: "pe_ratio", Op: ">", Value: "not_a_number"}},
		Fields:   []string{"ticker"},
		Limit:    20,
	}
	// Should not panic — will just fail the comparison
	resp, err := scanner.Scan(context.Background(), query)
	require.NoError(t, err)
	// Type coercion fails: "not_a_number" can't be converted to float64, comparison returns false
	assert.Equal(t, 0, len(resp.Results))
}

// ============================================================================
// 8. Null/missing data: ticker with no MarketData
// ============================================================================

func TestStress_Scan_TickerWithNoMarketData(t *testing.T) {
	// Stock index has the ticker but no market data stored
	storage := makeScanTestData([]string{"GHOST.AU"}, nil, nil)
	scanner := newTestScanner(storage)

	query := models.ScanQuery{
		Exchange: "AU",
		Fields:   []string{"ticker"},
		Limit:    20,
	}
	resp, err := scanner.Scan(context.Background(), query)
	require.NoError(t, err)
	assert.Equal(t, 0, len(resp.Results))
}

// ============================================================================
// 9. Null/missing data: ticker with no Fundamentals
// ============================================================================

func TestStress_Scan_NoFundamentals(t *testing.T) {
	storage := makeScanTestData(
		[]string{"NOFUND.AU"},
		map[string]*models.MarketData{
			"NOFUND.AU": {Ticker: "NOFUND.AU", Exchange: "AU", EOD: makeEODBars(10, 5.0)},
		},
		nil,
	)
	scanner := newTestScanner(storage)

	// Request fundamental fields — should return nil, not panic
	query := models.ScanQuery{
		Exchange: "AU",
		Fields:   []string{"ticker", "pe_ratio", "sector", "market_cap", "eps_ttm", "beta"},
		Limit:    20,
	}
	resp, err := scanner.Scan(context.Background(), query)
	require.NoError(t, err)
	require.Equal(t, 1, len(resp.Results))
	assert.Equal(t, "NOFUND.AU", resp.Results[0]["ticker"])
	assert.Nil(t, resp.Results[0]["pe_ratio"])
	assert.Nil(t, resp.Results[0]["sector"])
}

// ============================================================================
// 10. Null/missing data: ticker with no EOD bars
// ============================================================================

func TestStress_Scan_NoEODBars(t *testing.T) {
	storage := makeScanTestData(
		[]string{"NOEOD.AU"},
		map[string]*models.MarketData{
			"NOEOD.AU": {Ticker: "NOEOD.AU", Exchange: "AU", EOD: nil, Fundamentals: &models.Fundamentals{PE: 10.0, Sector: "Test"}},
		},
		nil,
	)
	scanner := newTestScanner(storage)

	query := models.ScanQuery{
		Exchange: "AU",
		Fields:   []string{"ticker", "price", "52_week_high", "52_week_low", "volume", "avg_volume_30d"},
		Limit:    20,
	}
	resp, err := scanner.Scan(context.Background(), query)
	require.NoError(t, err)
	require.Equal(t, 1, len(resp.Results))
	assert.Nil(t, resp.Results[0]["price"])
	assert.Nil(t, resp.Results[0]["52_week_high"])
}

// ============================================================================
// 11. Null/missing data: ticker with no TickerSignals
// ============================================================================

func TestStress_Scan_NoSignals(t *testing.T) {
	storage := makeScanTestData(
		[]string{"NOSIG.AU"},
		map[string]*models.MarketData{
			"NOSIG.AU": {Ticker: "NOSIG.AU", Exchange: "AU", EOD: makeEODBars(10, 20.0), Fundamentals: &models.Fundamentals{Sector: "Test"}},
		},
		nil, // no signals
	)
	scanner := newTestScanner(storage)

	query := models.ScanQuery{
		Exchange: "AU",
		Fields:   []string{"ticker", "price", "sma_20", "sma_50", "sma_200", "rsi_14", "macd"},
		Limit:    20,
	}
	resp, err := scanner.Scan(context.Background(), query)
	require.NoError(t, err)
	require.Equal(t, 1, len(resp.Results))
	// Price should fall back to EOD
	assert.NotNil(t, resp.Results[0]["price"])
	// Signal-dependent fields should be nil
	assert.Nil(t, resp.Results[0]["sma_20"])
	assert.Nil(t, resp.Results[0]["rsi_14"])
}

// ============================================================================
// 12. Edge case: between filter with min > max
// ============================================================================

func TestStress_Scan_BetweenMinGreaterThanMax(t *testing.T) {
	storage := makeScanTestData(
		[]string{"BHP.AU"},
		map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", Exchange: "AU", EOD: makeEODBars(10, 42.0), Fundamentals: &models.Fundamentals{PE: 12.5, Sector: "Materials", MarketCap: 150e9}},
		},
		nil,
	)
	scanner := newTestScanner(storage)

	query := models.ScanQuery{
		Exchange: "AU",
		Filters:  []models.ScanFilter{{Field: "pe_ratio", Op: "between", Value: []interface{}{float64(20), float64(5)}}},
		Fields:   []string{"ticker"},
		Limit:    20,
	}
	resp, err := scanner.Scan(context.Background(), query)
	require.NoError(t, err)
	// min > max: no values can be >= 20 AND <= 5
	assert.Equal(t, 0, len(resp.Results))
}

// ============================================================================
// 13. Edge case: in filter with empty list
// ============================================================================

func TestStress_Scan_InFilterEmptyList(t *testing.T) {
	storage := makeScanTestData(
		[]string{"BHP.AU"},
		map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", Exchange: "AU", EOD: makeEODBars(10, 42.0), Fundamentals: &models.Fundamentals{Sector: "Materials"}},
		},
		nil,
	)
	scanner := newTestScanner(storage)

	query := models.ScanQuery{
		Exchange: "AU",
		Filters:  []models.ScanFilter{{Field: "sector", Op: "in", Value: []interface{}{}}},
		Fields:   []string{"ticker"},
		Limit:    20,
	}
	resp, err := scanner.Scan(context.Background(), query)
	require.NoError(t, err)
	// Empty "in" list: nothing matches
	assert.Equal(t, 0, len(resp.Results))
}

// ============================================================================
// 14. Edge case: sort by field that is null for many results
// ============================================================================

func TestStress_Scan_SortByNullField(t *testing.T) {
	now := time.Now()
	tickers := []string{"A.AU", "B.AU", "C.AU"}
	mdMap := map[string]*models.MarketData{
		"A.AU": {Ticker: "A.AU", Exchange: "AU", EOD: makeEODBars(10, 10.0), Fundamentals: &models.Fundamentals{PE: 5.0, Sector: "Test"}, LastUpdated: now},
		"B.AU": {Ticker: "B.AU", Exchange: "AU", EOD: makeEODBars(10, 20.0), LastUpdated: now}, // no fundamentals
		"C.AU": {Ticker: "C.AU", Exchange: "AU", EOD: makeEODBars(10, 30.0), Fundamentals: &models.Fundamentals{PE: 15.0, Sector: "Test"}, LastUpdated: now},
	}

	storage := makeScanTestData(tickers, mdMap, nil)
	scanner := newTestScanner(storage)

	sortJSON, _ := json.Marshal(models.ScanSort{Field: "pe_ratio", Order: "asc"})
	query := models.ScanQuery{
		Exchange: "AU",
		Fields:   []string{"ticker", "pe_ratio"},
		Sort:     json.RawMessage(sortJSON),
		Limit:    50,
	}
	resp, err := scanner.Scan(context.Background(), query)
	require.NoError(t, err)
	assert.Equal(t, 3, len(resp.Results))
	// Null values should sort to the end
	assert.Nil(t, resp.Results[2]["pe_ratio"])
}

// ============================================================================
// 15. Edge case: computed fields with fewer bars than lookback
// ============================================================================

func TestStress_Scan_FewEODBarsForComputedFields(t *testing.T) {
	// Only 5 bars — not enough for 52-week, 30d, etc.
	storage := makeScanTestData(
		[]string{"NEW.AU"},
		map[string]*models.MarketData{
			"NEW.AU": {Ticker: "NEW.AU", Exchange: "AU", EOD: makeEODBars(5, 10.0), Fundamentals: &models.Fundamentals{Sector: "Test"}},
		},
		nil,
	)
	scanner := newTestScanner(storage)

	query := models.ScanQuery{
		Exchange: "AU",
		Fields:   []string{"ticker", "52_week_return_pct", "30d_return_pct", "avg_volume_30d", "avg_volume_90d"},
		Limit:    20,
	}
	resp, err := scanner.Scan(context.Background(), query)
	require.NoError(t, err)
	require.Equal(t, 1, len(resp.Results))
	// These should be nil when not enough bars
	assert.Nil(t, resp.Results[0]["52_week_return_pct"])
	assert.Nil(t, resp.Results[0]["30d_return_pct"])
	assert.Nil(t, resp.Results[0]["avg_volume_30d"])
	assert.Nil(t, resp.Results[0]["avg_volume_90d"])
}

// ============================================================================
// 16. Resource: requesting all fields
// ============================================================================

func TestStress_Scan_AllFields(t *testing.T) {
	now := time.Now()
	storage := makeScanTestData(
		[]string{"BHP.AU"},
		map[string]*models.MarketData{
			"BHP.AU": {
				Ticker: "BHP.AU", Exchange: "AU", Name: "BHP Group",
				EOD: makeEODBars(300, 42.0),
				Fundamentals: &models.Fundamentals{
					PE: 12.5, PB: 2.1, EPS: 3.36, Sector: "Materials",
					Industry: "Mining", MarketCap: 150e9, DividendYield: 0.035, Beta: 1.1,
				},
				LastUpdated: now,
			},
		},
		map[string]*models.TickerSignals{
			"BHP.AU": {
				Ticker: "BHP.AU",
				Price:  models.PriceSignals{Current: 42.0, SMA20: 41.0, SMA50: 40.0, SMA200: 38.0},
				Technical: models.TechnicalSignals{
					RSI: 55.0, MACD: 0.5, MACDSignal: 0.3, MACDHistogram: 0.2,
					ATR: 1.2, ATRPct: 2.86, VolumeRatio: 1.5,
				},
			},
		},
	)
	scanner := newTestScanner(storage)

	// Get all field names from registry
	fieldsResp := scanner.Fields()
	var allFields []string
	for _, group := range fieldsResp.Groups {
		for _, f := range group.Fields {
			allFields = append(allFields, f.Field)
		}
	}

	query := models.ScanQuery{
		Exchange: "AU",
		Fields:   allFields,
		Limit:    50,
	}
	resp, err := scanner.Scan(context.Background(), query)
	require.NoError(t, err)
	require.Equal(t, 1, len(resp.Results))

	// Result should have all requested fields
	assert.Equal(t, len(allFields), len(resp.Results[0]))
}

// ============================================================================
// 17. Resource: large number of tickers
// ============================================================================

func TestStress_Scan_ManyTickers(t *testing.T) {
	now := time.Now()
	n := 500
	tickers := make([]string, n)
	mdMap := make(map[string]*models.MarketData, n)

	for i := 0; i < n; i++ {
		ticker := fmt.Sprintf("T%04d.AU", i)
		tickers[i] = ticker
		mdMap[ticker] = &models.MarketData{
			Ticker:       ticker,
			Exchange:     "AU",
			Name:         fmt.Sprintf("Stock %d", i),
			EOD:          makeEODBars(10, float64(10+i)),
			Fundamentals: &models.Fundamentals{PE: float64(5 + i%30), Sector: "Test", MarketCap: float64(1e9 + float64(i)*1e6)},
			LastUpdated:  now,
		}
	}

	storage := makeScanTestData(tickers, mdMap, nil)
	scanner := newTestScanner(storage)

	sortJSON, _ := json.Marshal(models.ScanSort{Field: "pe_ratio", Order: "asc"})
	query := models.ScanQuery{
		Exchange: "AU",
		Filters:  []models.ScanFilter{{Field: "pe_ratio", Op: "<=", Value: 20.0}},
		Fields:   []string{"ticker", "pe_ratio", "market_cap", "price"},
		Sort:     json.RawMessage(sortJSON),
		Limit:    20,
	}

	start := time.Now()
	resp, err := scanner.Scan(context.Background(), query)
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Equal(t, 20, len(resp.Results), "should return exactly limit results")
	assert.Greater(t, resp.Meta.TotalMatched, 0)
	assert.Less(t, elapsed, 5*time.Second, "scan should complete in under 5 seconds")
}

// ============================================================================
// 18. exchange: "ALL" returns tickers from all exchanges
// ============================================================================

func TestStress_Scan_AllExchanges(t *testing.T) {
	now := time.Now()
	tickers := []string{"BHP.AU", "AAPL.US"}
	mdMap := map[string]*models.MarketData{
		"BHP.AU":  {Ticker: "BHP.AU", Exchange: "AU", EOD: makeEODBars(10, 42.0), Fundamentals: &models.Fundamentals{Sector: "Materials"}, LastUpdated: now},
		"AAPL.US": {Ticker: "AAPL.US", Exchange: "US", EOD: makeEODBars(10, 180.0), Fundamentals: &models.Fundamentals{Sector: "Technology"}, LastUpdated: now},
	}

	storage := makeScanTestData(tickers, mdMap, nil)
	scanner := newTestScanner(storage)

	query := models.ScanQuery{
		Exchange: "ALL",
		Fields:   []string{"ticker", "exchange"},
		Limit:    50,
	}
	resp, err := scanner.Scan(context.Background(), query)
	require.NoError(t, err)
	assert.Equal(t, 2, len(resp.Results))
}

// ============================================================================
// 19. Context cancellation during scan
// ============================================================================

func TestStress_Scan_ContextCancel(t *testing.T) {
	now := time.Now()
	n := 100
	tickers := make([]string, n)
	mdMap := make(map[string]*models.MarketData, n)
	for i := 0; i < n; i++ {
		ticker := fmt.Sprintf("T%04d.AU", i)
		tickers[i] = ticker
		mdMap[ticker] = &models.MarketData{
			Ticker: ticker, Exchange: "AU", EOD: makeEODBars(10, float64(10+i)),
			Fundamentals: &models.Fundamentals{Sector: "Test"}, LastUpdated: now,
		}
	}

	storage := makeScanTestData(tickers, mdMap, nil)
	scanner := newTestScanner(storage)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	query := models.ScanQuery{
		Exchange: "AU",
		Fields:   []string{"ticker"},
		Limit:    20,
	}

	// Should complete (may return error or empty results) without hanging
	done := make(chan struct{})
	go func() {
		scanner.Scan(ctx, query)
		close(done)
	}()

	select {
	case <-done:
		// good — completed
	case <-time.After(5 * time.Second):
		t.Fatal("DEADLOCK: scan did not return after context cancellation")
	}
}

// ============================================================================
// 20. Sort validation: unsortable field rejected
// ============================================================================

func TestStress_Scan_SortUnsortableField(t *testing.T) {
	storage := makeScanTestData(
		[]string{"BHP.AU"},
		map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", Exchange: "AU", EOD: makeEODBars(10, 42.0), Fundamentals: &models.Fundamentals{Sector: "Materials"}},
		},
		nil,
	)
	scanner := newTestScanner(storage)

	sortJSON, _ := json.Marshal(models.ScanSort{Field: "ticker", Order: "asc"})
	query := models.ScanQuery{
		Exchange: "AU",
		Fields:   []string{"ticker"},
		Sort:     json.RawMessage(sortJSON),
		Limit:    20,
	}
	_, err := scanner.Scan(context.Background(), query)
	assert.Error(t, err, "unsortable field should be rejected")
}

// ============================================================================
// 21. Sort validation: invalid sort order
// ============================================================================

func TestStress_Scan_InvalidSortOrder(t *testing.T) {
	storage := makeScanTestData(
		[]string{"BHP.AU"},
		map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", Exchange: "AU", EOD: makeEODBars(10, 42.0), Fundamentals: &models.Fundamentals{PE: 12.5, Sector: "Materials", MarketCap: 150e9}},
		},
		nil,
	)
	scanner := newTestScanner(storage)

	sortJSON, _ := json.Marshal(models.ScanSort{Field: "pe_ratio", Order: "sideways"})
	query := models.ScanQuery{
		Exchange: "AU",
		Fields:   []string{"ticker"},
		Sort:     json.RawMessage(sortJSON),
		Limit:    20,
	}
	_, err := scanner.Scan(context.Background(), query)
	assert.Error(t, err, "invalid sort order should be rejected")
}

// ============================================================================
// 22. Non-filterable field in filter
// ============================================================================

func TestStress_Scan_NonFilterableField(t *testing.T) {
	storage := makeScanTestData(
		[]string{"BHP.AU"},
		map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", Exchange: "AU", EOD: makeEODBars(10, 42.0), Fundamentals: &models.Fundamentals{Sector: "Materials"}},
		},
		nil,
	)
	scanner := newTestScanner(storage)

	// "name" is not filterable in the registry
	query := models.ScanQuery{
		Exchange: "AU",
		Filters:  []models.ScanFilter{{Field: "name", Op: "==", Value: "BHP"}},
		Fields:   []string{"ticker"},
		Limit:    20,
	}
	_, err := scanner.Scan(context.Background(), query)
	assert.Error(t, err, "non-filterable field should be rejected in filter")
}

// ============================================================================
// 23. Boolean filter with non-bool value
// ============================================================================

func TestStress_Scan_BoolFieldWithStringValue(t *testing.T) {
	storage := makeScanTestData(
		[]string{"BHP.AU"},
		map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", Exchange: "AU", EOD: makeEODBars(10, 42.0), Fundamentals: &models.Fundamentals{EPS: 3.36, Sector: "Materials", MarketCap: 150e9}},
		},
		nil,
	)
	scanner := newTestScanner(storage)

	// earnings_positive is a bool field; pass a string
	query := models.ScanQuery{
		Exchange: "AU",
		Filters:  []models.ScanFilter{{Field: "earnings_positive", Op: "==", Value: "yes"}},
		Fields:   []string{"ticker"},
		Limit:    20,
	}
	// Should not panic; bool comparison should handle gracefully
	resp, err := scanner.Scan(context.Background(), query)
	require.NoError(t, err)
	// "yes" != bool, so comparison fails, no results
	assert.Equal(t, 0, len(resp.Results))
}

// ============================================================================
// 24. between filter with non-array value
// ============================================================================

func TestStress_Scan_BetweenNonArrayValue(t *testing.T) {
	storage := makeScanTestData(
		[]string{"BHP.AU"},
		map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", Exchange: "AU", EOD: makeEODBars(10, 42.0), Fundamentals: &models.Fundamentals{PE: 12.5, Sector: "Materials", MarketCap: 150e9}},
		},
		nil,
	)
	scanner := newTestScanner(storage)

	query := models.ScanQuery{
		Exchange: "AU",
		Filters:  []models.ScanFilter{{Field: "pe_ratio", Op: "between", Value: float64(10)}},
		Fields:   []string{"ticker"},
		Limit:    20,
	}
	// Should not panic; between with scalar should just fail the filter
	resp, err := scanner.Scan(context.Background(), query)
	require.NoError(t, err)
	assert.Equal(t, 0, len(resp.Results))
}

// ============================================================================
// 25. in filter with non-array value
// ============================================================================

func TestStress_Scan_InFilterNonArrayValue(t *testing.T) {
	storage := makeScanTestData(
		[]string{"BHP.AU"},
		map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", Exchange: "AU", EOD: makeEODBars(10, 42.0), Fundamentals: &models.Fundamentals{Sector: "Materials"}},
		},
		nil,
	)
	scanner := newTestScanner(storage)

	query := models.ScanQuery{
		Exchange: "AU",
		Filters:  []models.ScanFilter{{Field: "sector", Op: "in", Value: "Materials"}},
		Fields:   []string{"ticker"},
		Limit:    20,
	}
	// Should not panic; "in" with non-array should just fail
	resp, err := scanner.Scan(context.Background(), query)
	require.NoError(t, err)
	assert.Equal(t, 0, len(resp.Results))
}

// ============================================================================
// 26. Empty OR group
// ============================================================================

func TestStress_Scan_EmptyORGroup(t *testing.T) {
	storage := makeScanTestData(
		[]string{"BHP.AU"},
		map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", Exchange: "AU", EOD: makeEODBars(10, 42.0), Fundamentals: &models.Fundamentals{Sector: "Materials"}},
		},
		nil,
	)
	scanner := newTestScanner(storage)

	// Empty OR group with Field set triggers validation:
	// The Or slice is empty but the filter has no Field, which is caught by validateFilter
	query := models.ScanQuery{
		Exchange: "AU",
		Filters:  []models.ScanFilter{{Or: []models.ScanFilter{}}},
		Fields:   []string{"ticker"},
		Limit:    20,
	}
	// Validation catches: empty Or list + no Field => "filter field is required"
	_, err := scanner.Scan(context.Background(), query)
	assert.Error(t, err, "empty OR group should be rejected by validation")
}

// ============================================================================
// 27. Injection-style values in filter
// ============================================================================

func TestStress_Scan_InjectionAttempts(t *testing.T) {
	storage := makeScanTestData(
		[]string{"BHP.AU"},
		map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", Exchange: "AU", EOD: makeEODBars(10, 42.0), Fundamentals: &models.Fundamentals{Sector: "Materials"}},
		},
		nil,
	)
	scanner := newTestScanner(storage)

	injections := []string{
		"'; DROP TABLE stocks; --",
		`{"$gt": 0}`,
		"<script>alert(1)</script>",
		"../../../etc/passwd",
	}

	for _, injection := range injections {
		t.Run(injection[:min(len(injection), 30)], func(t *testing.T) {
			query := models.ScanQuery{
				Exchange: "AU",
				Filters:  []models.ScanFilter{{Field: "sector", Op: "==", Value: injection}},
				Fields:   []string{"ticker"},
				Limit:    20,
			}
			// Should not panic or cause issues
			resp, err := scanner.Scan(context.Background(), query)
			require.NoError(t, err)
			assert.Equal(t, 0, len(resp.Results))
		})
	}
}

// ============================================================================
// 28. Invalid sort JSON
// ============================================================================

func TestStress_Scan_InvalidSortJSON(t *testing.T) {
	storage := makeScanTestData(
		[]string{"BHP.AU"},
		map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", Exchange: "AU", EOD: makeEODBars(10, 42.0), Fundamentals: &models.Fundamentals{Sector: "Materials"}},
		},
		nil,
	)
	scanner := newTestScanner(storage)

	query := models.ScanQuery{
		Exchange: "AU",
		Fields:   []string{"ticker"},
		Sort:     json.RawMessage(`{invalid json}`),
		Limit:    20,
	}
	_, err := scanner.Scan(context.Background(), query)
	assert.Error(t, err, "invalid sort JSON should be rejected")
}

// ============================================================================
// 29. Deep OR nesting (depth > maxFilterDepth) must be rejected
// ============================================================================

func TestStress_Scan_ORNestingBeyondDepthLimit(t *testing.T) {
	storage := makeScanTestData(
		[]string{"BHP.AU"},
		map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", Exchange: "AU", EOD: makeEODBars(10, 42.0), Fundamentals: &models.Fundamentals{PE: 12.5, Sector: "Materials"}},
		},
		nil,
	)
	scanner := newTestScanner(storage)

	// Build 200-deep nested OR chain — should be rejected by depth limit
	leaf := models.ScanFilter{Field: "pe_ratio", Op: ">", Value: 0.0}
	current := leaf
	for i := 0; i < 200; i++ {
		current = models.ScanFilter{Or: []models.ScanFilter{current}}
	}

	query := models.ScanQuery{
		Exchange: "AU",
		Filters:  []models.ScanFilter{current},
		Fields:   []string{"ticker"},
		Limit:    20,
	}
	_, err := scanner.Scan(context.Background(), query)
	assert.Error(t, err, "deeply nested OR groups beyond depth limit should be rejected")
	assert.Contains(t, err.Error(), "depth")
}

// ============================================================================
// 30. Many filters (performance/resource check)
// ============================================================================

func TestStress_Scan_ManyFilters(t *testing.T) {
	storage := makeScanTestData(
		[]string{"BHP.AU"},
		map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", Exchange: "AU", EOD: makeEODBars(10, 42.0), Fundamentals: &models.Fundamentals{PE: 12.5, Sector: "Materials", MarketCap: 150e9}},
		},
		nil,
	)
	scanner := newTestScanner(storage)

	// 500 identical filters (all should pass)
	filters := make([]models.ScanFilter, 500)
	for i := range filters {
		filters[i] = models.ScanFilter{Field: "pe_ratio", Op: ">", Value: 0.0}
	}

	query := models.ScanQuery{
		Exchange: "AU",
		Filters:  filters,
		Fields:   []string{"ticker"},
		Limit:    20,
	}

	// Should complete without issue
	resp, err := scanner.Scan(context.Background(), query)
	require.NoError(t, err)
	assert.Equal(t, 1, len(resp.Results))
}

// ============================================================================
// 31. Many duplicate fields (no crash, no memory blowup)
// ============================================================================

func TestStress_Scan_ManyDuplicateFields(t *testing.T) {
	storage := makeScanTestData(
		[]string{"BHP.AU"},
		map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", Exchange: "AU", EOD: makeEODBars(10, 42.0), Fundamentals: &models.Fundamentals{PE: 12.5, Sector: "Materials"}},
		},
		nil,
	)
	scanner := newTestScanner(storage)

	// 1000 duplicate "ticker" fields
	fields := make([]string, 1000)
	for i := range fields {
		fields[i] = "ticker"
	}

	query := models.ScanQuery{
		Exchange: "AU",
		Fields:   fields,
		Limit:    20,
	}

	resp, err := scanner.Scan(context.Background(), query)
	require.NoError(t, err)
	require.Equal(t, 1, len(resp.Results))
	// Since ScanResult is a map, duplicates just overwrite: result has 1 key
	assert.Equal(t, 1, len(resp.Results[0]))
}

// ============================================================================
// 32. NaN in EOD bars — division-by-zero and Inf scenarios
// ============================================================================

func TestStress_Scan_NaNInfEODFields(t *testing.T) {
	now := time.Now()
	// Create bars where computeReturn would divide by zero (bar[N].Close = 0)
	bars := make([]models.EODBar, 30)
	for i := range bars {
		bars[i] = models.EODBar{
			Date:   now.AddDate(0, 0, -i),
			Open:   10.0,
			High:   11.0,
			Low:    9.0,
			Close:  10.0,
			Volume: 100000,
		}
	}
	bars[5].Close = 0  // division by zero for computeReturn with period=5
	bars[21].Close = 0 // division by zero for 30d return

	storage := makeScanTestData(
		[]string{"BAD.AU"},
		map[string]*models.MarketData{
			"BAD.AU": {
				Ticker: "BAD.AU", Exchange: "AU", Name: "Bad Data",
				EOD:          bars,
				Fundamentals: &models.Fundamentals{Sector: "Test"},
				LastUpdated:  now,
			},
		},
		nil,
	)
	scanner := newTestScanner(storage)

	query := models.ScanQuery{
		Exchange: "AU",
		Fields: []string{
			"ticker", "7d_return_pct", "30d_return_pct",
			"weighted_alpha", "relative_volume",
		},
		Limit: 10,
	}

	// Should not panic
	resp, err := scanner.Scan(context.Background(), query)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
}

// ============================================================================
// 33. Flat-price bars (all OHLC identical) — division by zero guards
// ============================================================================

func TestStress_Scan_FlatPriceBars(t *testing.T) {
	now := time.Now()
	bars := make([]models.EODBar, 50)
	for i := range bars {
		bars[i] = models.EODBar{
			Date:   now.AddDate(0, 0, -i),
			Open:   10.0,
			High:   10.0,
			Low:    10.0,
			Close:  10.0,
			Volume: 100000,
		}
	}

	storage := makeScanTestData(
		[]string{"FLAT.AU"},
		map[string]*models.MarketData{
			"FLAT.AU": {
				Ticker: "FLAT.AU", Exchange: "AU", Name: "Flat Price",
				EOD:          bars,
				Fundamentals: &models.Fundamentals{Sector: "Test"},
				LastUpdated:  now,
			},
		},
		nil,
	)
	scanner := newTestScanner(storage)

	query := models.ScanQuery{
		Exchange: "AU",
		Fields: []string{
			"ticker", "williams_r", "stoch_k", "stoch_d",
			"bollinger_pct_b", "cci", "adx",
		},
		Limit: 10,
	}

	resp, err := scanner.Scan(context.Background(), query)
	require.NoError(t, err)
	require.Equal(t, 1, len(resp.Results))

	// williams_r: highest==lowest → should return nil (guarded)
	assert.Nil(t, resp.Results[0]["williams_r"])
	// bollinger_pct_b: upper==lower → should return nil (guarded)
	assert.Nil(t, resp.Results[0]["bollinger_pct_b"])
	// CCI: mean deviation==0 → should return nil (guarded)
	assert.Nil(t, resp.Results[0]["cci"])
}

// ============================================================================
// 34. Concurrent scan safety — no data races
// ============================================================================

func TestStress_Scan_ConcurrentSafety(t *testing.T) {
	now := time.Now()
	tickers := []string{"BHP.AU", "RIO.AU", "WOW.AU"}
	mdMap := map[string]*models.MarketData{
		"BHP.AU": {Ticker: "BHP.AU", Exchange: "AU", EOD: makeEODBars(50, 42.0), Fundamentals: &models.Fundamentals{PE: 12.5, Sector: "Materials"}, LastUpdated: now},
		"RIO.AU": {Ticker: "RIO.AU", Exchange: "AU", EOD: makeEODBars(50, 110.0), Fundamentals: &models.Fundamentals{PE: 8.0, Sector: "Materials"}, LastUpdated: now},
		"WOW.AU": {Ticker: "WOW.AU", Exchange: "AU", EOD: makeEODBars(50, 35.0), Fundamentals: &models.Fundamentals{PE: 25.0, Sector: "Staples"}, LastUpdated: now},
	}
	storage := makeScanTestData(tickers, mdMap, nil)
	scanner := newTestScanner(storage)

	const n = 50
	done := make(chan struct{}, n)
	for i := 0; i < n; i++ {
		go func(id int) {
			defer func() { done <- struct{}{} }()
			query := models.ScanQuery{
				Exchange: "AU",
				Fields:   []string{"ticker", "pe_ratio", "sector"},
				Filters:  []models.ScanFilter{{Field: "pe_ratio", Op: ">", Value: float64(id % 30)}},
				Limit:    10,
			}
			resp, err := scanner.Scan(context.Background(), query)
			assert.NoError(t, err)
			assert.NotNil(t, resp)
		}(i)
	}
	for i := 0; i < n; i++ {
		<-done
	}
}

// ============================================================================
// 35. Zero-length EOD bars slice vs nil EOD bars
// ============================================================================

func TestStress_Scan_EmptyVsNilEOD(t *testing.T) {
	now := time.Now()
	storage := makeScanTestData(
		[]string{"EMPTY.AU", "NIL.AU"},
		map[string]*models.MarketData{
			"EMPTY.AU": {Ticker: "EMPTY.AU", Exchange: "AU", EOD: []models.EODBar{}, Fundamentals: &models.Fundamentals{Sector: "Test"}, LastUpdated: now},
			"NIL.AU":   {Ticker: "NIL.AU", Exchange: "AU", EOD: nil, Fundamentals: &models.Fundamentals{Sector: "Test"}, LastUpdated: now},
		},
		nil,
	)
	scanner := newTestScanner(storage)

	query := models.ScanQuery{
		Exchange: "AU",
		Fields:   []string{"ticker", "price", "52_week_high", "volume"},
		Limit:    50,
	}

	resp, err := scanner.Scan(context.Background(), query)
	require.NoError(t, err)
	require.Equal(t, 2, len(resp.Results))

	for _, r := range resp.Results {
		assert.Nil(t, r["price"], "price should be nil for %s", r["ticker"])
		assert.Nil(t, r["volume"], "volume should be nil for %s", r["ticker"])
	}
}

// ============================================================================
// 36. ParseScanSort edge cases
// ============================================================================

func TestStress_Scan_ParseScanSortEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		sort    json.RawMessage
		wantErr bool
		wantLen int
	}{
		{"nil sort", nil, false, 0},
		{"empty bytes", json.RawMessage{}, false, 0},
		{"null JSON", json.RawMessage(`null`), false, 0},
		{"number", json.RawMessage(`42`), true, 0},
		{"bare string", json.RawMessage(`"pe_ratio"`), true, 0},
		{"boolean", json.RawMessage(`true`), true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := models.ScanQuery{Sort: tt.sort}
			sorts, err := query.ParseScanSort()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantLen, len(sorts))
			}
		})
	}
}
