package server

import (
	"testing"

	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/stretchr/testify/assert"
)

func TestParseStockDataInclude_NoParams(t *testing.T) {
	result := parseStockDataInclude(nil)
	assert.True(t, result.Price)
	assert.True(t, result.Fundamentals)
	assert.True(t, result.Signals)
	assert.True(t, result.News)
}

func TestParseStockDataInclude_EmptySlice(t *testing.T) {
	result := parseStockDataInclude([]string{})
	assert.True(t, result.Price)
	assert.True(t, result.Fundamentals)
	assert.True(t, result.Signals)
	assert.True(t, result.News)
}

func TestParseStockDataInclude_CommaSeparated(t *testing.T) {
	result := parseStockDataInclude([]string{"price,signals"})
	assert.Equal(t, interfaces.StockDataInclude{
		Price: true, Signals: true,
	}, result)
}

func TestParseStockDataInclude_RepeatedKeys(t *testing.T) {
	// This is the bug fix: ?include=price&include=signals
	result := parseStockDataInclude([]string{"price", "signals"})
	assert.Equal(t, interfaces.StockDataInclude{
		Price: true, Signals: true,
	}, result)
}

func TestParseStockDataInclude_MixedFormats(t *testing.T) {
	// ?include=price,fundamentals&include=news
	result := parseStockDataInclude([]string{"price,fundamentals", "news"})
	assert.Equal(t, interfaces.StockDataInclude{
		Price: true, Fundamentals: true, News: true,
	}, result)
}

func TestParseStockDataInclude_SingleValue(t *testing.T) {
	result := parseStockDataInclude([]string{"price"})
	assert.Equal(t, interfaces.StockDataInclude{
		Price: true,
	}, result)
}

func TestParseStockDataInclude_AllValues(t *testing.T) {
	result := parseStockDataInclude([]string{"price,fundamentals,signals,news"})
	assert.Equal(t, interfaces.StockDataInclude{
		Price: true, Fundamentals: true, Signals: true, News: true,
	}, result)
}

func TestParseStockDataInclude_UnknownValuesIgnored(t *testing.T) {
	result := parseStockDataInclude([]string{"price,unknown,signals"})
	assert.Equal(t, interfaces.StockDataInclude{
		Price: true, Signals: true,
	}, result)
}

func TestParseStockDataInclude_WhitespaceHandled(t *testing.T) {
	result := parseStockDataInclude([]string{" price , signals "})
	assert.Equal(t, interfaces.StockDataInclude{
		Price: true, Signals: true,
	}, result)
}
