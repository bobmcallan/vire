package server

import (
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// ============================================================================
// Stress tests for parseStockDataInclude — the fixed include param parsing
// that handles both array-style (?include=x&include=y) and comma-separated
// (?include=x,y) formats, plus mixed combinations.
// ============================================================================

// ============================================================================
// 1. Core include param parsing — array-style (?include=x&include=y)
// ============================================================================

func TestIncludeStress_ArrayStyle_SingleValue(t *testing.T) {
	inc := parseStockDataInclude([]string{"price"})

	assert.True(t, inc.Price, "price should be true")
	assert.False(t, inc.Fundamentals, "fundamentals should be false")
	assert.False(t, inc.Signals, "signals should be false")
	assert.False(t, inc.News, "news should be false")
}

func TestIncludeStress_ArrayStyle_MultipleValues(t *testing.T) {
	inc := parseStockDataInclude([]string{"price", "signals"})

	assert.True(t, inc.Price, "price should be true")
	assert.False(t, inc.Fundamentals, "fundamentals should be false")
	assert.True(t, inc.Signals, "signals should be true")
	assert.False(t, inc.News, "news should be false")
}

func TestIncludeStress_ArrayStyle_AllValues(t *testing.T) {
	inc := parseStockDataInclude([]string{"price", "fundamentals", "signals", "news"})

	assert.True(t, inc.Price)
	assert.True(t, inc.Fundamentals)
	assert.True(t, inc.Signals)
	assert.True(t, inc.News)
}

// ============================================================================
// 2. Comma-separated style (?include=price,signals)
// ============================================================================

func TestIncludeStress_CommaSeparated_TwoValues(t *testing.T) {
	inc := parseStockDataInclude([]string{"price,signals"})

	assert.True(t, inc.Price)
	assert.False(t, inc.Fundamentals)
	assert.True(t, inc.Signals)
	assert.False(t, inc.News)
}

func TestIncludeStress_CommaSeparated_AllValues(t *testing.T) {
	inc := parseStockDataInclude([]string{"price,fundamentals,signals,news"})

	assert.True(t, inc.Price)
	assert.True(t, inc.Fundamentals)
	assert.True(t, inc.Signals)
	assert.True(t, inc.News)
}

// ============================================================================
// 3. Mixed format (?include=price,signals&include=news)
// ============================================================================

func TestIncludeStress_MixedFormat(t *testing.T) {
	inc := parseStockDataInclude([]string{"price,signals", "news"})

	assert.True(t, inc.Price, "price from comma-separated should be true")
	assert.False(t, inc.Fundamentals, "fundamentals not mentioned should be false")
	assert.True(t, inc.Signals, "signals from comma-separated should be true")
	assert.True(t, inc.News, "news from array-style should be true")
}

func TestIncludeStress_MixedFormat_AllCombinations(t *testing.T) {
	inc := parseStockDataInclude([]string{"price,fundamentals", "signals,news"})

	assert.True(t, inc.Price)
	assert.True(t, inc.Fundamentals)
	assert.True(t, inc.Signals)
	assert.True(t, inc.News)
}

// ============================================================================
// 4. Empty and invalid include values
// ============================================================================

func TestIncludeStress_EmptyIncludeValue(t *testing.T) {
	// Equivalent to ?include=&include=price — empty first value
	inc := parseStockDataInclude([]string{"", "price"})

	assert.True(t, inc.Price, "price should still be parsed")
	assert.False(t, inc.Fundamentals, "empty value should not enable fundamentals")
	assert.False(t, inc.Signals)
	assert.False(t, inc.News)
}

func TestIncludeStress_AllEmptyIncludeValues(t *testing.T) {
	// Equivalent to ?include=&include= — all empty
	inc := parseStockDataInclude([]string{"", ""})

	// len(params) > 0 so defaults are cleared, but nothing is set
	assert.False(t, inc.Price, "all empty should result in no fields enabled")
	assert.False(t, inc.Fundamentals)
	assert.False(t, inc.Signals)
	assert.False(t, inc.News)
}

func TestIncludeStress_UnknownValues(t *testing.T) {
	inc := parseStockDataInclude([]string{"price,INVALID,signals"})

	assert.True(t, inc.Price, "valid values around INVALID should parse")
	assert.True(t, inc.Signals, "valid values around INVALID should parse")
	assert.False(t, inc.Fundamentals)
	assert.False(t, inc.News)
}

func TestIncludeStress_OnlyUnknownValues(t *testing.T) {
	inc := parseStockDataInclude([]string{"INVALID", "bogus"})

	// All unknown — defaults cleared, nothing enabled
	assert.False(t, inc.Price)
	assert.False(t, inc.Fundamentals)
	assert.False(t, inc.Signals)
	assert.False(t, inc.News)
}

func TestIncludeStress_CaseSensitivity(t *testing.T) {
	// Include values should be case-sensitive (lowercase only)
	inc := parseStockDataInclude([]string{"Price", "SIGNALS", "News"})

	assert.False(t, inc.Price, "Price (capital P) should not match")
	assert.False(t, inc.Signals, "SIGNALS should not match")
	assert.False(t, inc.News, "News should not match")
}

// ============================================================================
// 5. Duplicate values
// ============================================================================

func TestIncludeStress_DuplicateValues_ArrayStyle(t *testing.T) {
	inc := parseStockDataInclude([]string{"price", "price"})

	assert.True(t, inc.Price, "duplicate price should still be true")
	assert.False(t, inc.Fundamentals)
}

func TestIncludeStress_DuplicateValues_CommaSeparated(t *testing.T) {
	inc := parseStockDataInclude([]string{"price,price,price"})

	assert.True(t, inc.Price, "triple price should still be true")
	assert.False(t, inc.Fundamentals)
}

func TestIncludeStress_DuplicateValues_MixedFormats(t *testing.T) {
	inc := parseStockDataInclude([]string{"price,signals", "price", "signals,news"})

	assert.True(t, inc.Price)
	assert.True(t, inc.Signals)
	assert.True(t, inc.News)
	assert.False(t, inc.Fundamentals)
}

// ============================================================================
// 6. No include param — defaults
// ============================================================================

func TestIncludeStress_NilSlice(t *testing.T) {
	inc := parseStockDataInclude(nil)

	assert.True(t, inc.Price, "default: all true")
	assert.True(t, inc.Fundamentals, "default: all true")
	assert.True(t, inc.Signals, "default: all true")
	assert.True(t, inc.News, "default: all true")
}

func TestIncludeStress_EmptySlice(t *testing.T) {
	inc := parseStockDataInclude([]string{})

	assert.True(t, inc.Price, "default: all true")
	assert.True(t, inc.Fundamentals, "default: all true")
	assert.True(t, inc.Signals, "default: all true")
	assert.True(t, inc.News, "default: all true")
}

// ============================================================================
// 7. Whitespace handling
// ============================================================================

func TestIncludeStress_WhitespaceInValues(t *testing.T) {
	// Spaces around comma-separated values
	inc := parseStockDataInclude([]string{"price , signals"})

	assert.True(t, inc.Price, "trimmed 'price ' should match")
	assert.True(t, inc.Signals, "trimmed ' signals' should match")
}

func TestIncludeStress_WhitespaceOnlyValue(t *testing.T) {
	inc := parseStockDataInclude([]string{"   "})

	// Whitespace-only value should not match any field
	assert.False(t, inc.Price)
	assert.False(t, inc.Fundamentals)
	assert.False(t, inc.Signals)
	assert.False(t, inc.News)
}

// ============================================================================
// 8. Injection attempts via include param
// ============================================================================

func TestIncludeStress_SQLInjection(t *testing.T) {
	payloads := []string{
		"price'; DROP TABLE market_data; --",
		"price%27%3B+DROP+TABLE+market_data%3B+--",
		"price,fundamentals' OR '1'='1",
	}

	for _, payload := range payloads {
		t.Run(payload[:min(len(payload), 30)], func(t *testing.T) {
			inc := parseStockDataInclude([]string{payload})
			// Should not panic, should only set recognized fields
			// Invalid values are silently ignored by the switch statement
			_ = inc
		})
	}
}

func TestIncludeStress_VeryLongIncludeParam(t *testing.T) {
	// 100K character include value
	longValue := strings.Repeat("price,", 20000)
	inc := parseStockDataInclude([]string{longValue})

	assert.True(t, inc.Price, "should still parse price from repeated values")
}

func TestIncludeStress_ManyIncludeParams(t *testing.T) {
	// 1000 include= params
	params := make([]string, 1000)
	for i := range params {
		params[i] = "price"
	}
	inc := parseStockDataInclude(params)

	assert.True(t, inc.Price, "should handle many include params")
	assert.False(t, inc.Fundamentals)
}

// ============================================================================
// 9. BUG REGRESSION: old code used Query().Get() which only reads first value
// ============================================================================

func TestIncludeStress_BugRegression_GetVsIndex(t *testing.T) {
	// This is the exact scenario that was broken before the fix.
	// Query().Get("include") returns only the first value "price",
	// missing "signals" entirely.
	rawQuery := "include=price&include=signals"
	q, _ := url.ParseQuery(rawQuery)

	// Old behavior (broken): only reads first value
	oldInclude := q.Get("include")
	assert.Equal(t, "price", oldInclude,
		"Get() only returns first value — this confirms the bug")

	// New behavior (fixed): reads all values
	allIncludes := q["include"]
	assert.Len(t, allIncludes, 2,
		"query[key] returns all values — this is the fix")
	assert.Equal(t, "price", allIncludes[0])
	assert.Equal(t, "signals", allIncludes[1])

	// Verify parseStockDataInclude handles both
	inc := parseStockDataInclude(allIncludes)
	assert.True(t, inc.Price)
	assert.True(t, inc.Signals)
}

// ============================================================================
// 10. URL query integration test — verifies full round-trip parsing
// ============================================================================

func TestIncludeStress_URLQueryIntegration(t *testing.T) {
	cases := []struct {
		name          string
		query         string
		expectPrice   bool
		expectFundies bool
		expectSignals bool
		expectNews    bool
	}{
		{"array_style", "include=price&include=signals", true, false, true, false},
		{"comma_style", "include=price,signals", true, false, true, false},
		{"mixed_style", "include=price,fundamentals&include=news", true, true, false, true},
		{"no_include", "other=foo", true, true, true, true},
		{"empty_include", "include=", false, false, false, false},
		{"single_full", "include=price,fundamentals,signals,news", true, true, true, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q, _ := url.ParseQuery(tc.query)
			inc := parseStockDataInclude(q["include"])
			assert.Equal(t, tc.expectPrice, inc.Price, "price")
			assert.Equal(t, tc.expectFundies, inc.Fundamentals, "fundamentals")
			assert.Equal(t, tc.expectSignals, inc.Signals, "signals")
			assert.Equal(t, tc.expectNews, inc.News, "news")
		})
	}
}
