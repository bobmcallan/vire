package server

import (
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Devils-advocate stress tests for extractNavexaTrades (Fix 5).

func TestExtractNavexaTrades_EmptyHoldings(t *testing.T) {
	p := &models.Portfolio{Holdings: nil}
	trades := extractNavexaTrades(p, interfaces.TradeFilter{})
	assert.Empty(t, trades)
}

func TestExtractNavexaTrades_NilTradesOnHolding(t *testing.T) {
	p := &models.Portfolio{
		Holdings: []models.Holding{
			{Ticker: "BHP.AU", Trades: nil},
		},
	}
	trades := extractNavexaTrades(p, interfaces.TradeFilter{})
	assert.Empty(t, trades)
}

func TestExtractNavexaTrades_MalformedDate(t *testing.T) {
	p := &models.Portfolio{
		Holdings: []models.Holding{
			{
				Ticker: "BHP.AU",
				Trades: []*models.NavexaTrade{
					{Type: "buy", Date: "not-a-date", Units: 100, Price: 50},
					{Type: "buy", Date: "", Units: 50, Price: 40},
					{Type: "buy", Date: "2024-01-15", Units: 200, Price: 45},
				},
			},
		},
	}
	trades := extractNavexaTrades(p, interfaces.TradeFilter{})

	// All 3 trades should be extracted. Malformed dates have zero-value time.
	require.Len(t, trades, 3)

	// The valid-date trade should have a non-zero date
	var validFound bool
	for _, tr := range trades {
		if tr.Units == 200 {
			assert.False(t, tr.Date.IsZero(), "valid date should parse")
			validFound = true
		}
	}
	assert.True(t, validFound)
}

func TestExtractNavexaTrades_SplitAndDividendTypes(t *testing.T) {
	// NavexaTrade.Type can be "split", "dividend", "corporate action", etc.
	// extractNavexaTrades passes Type directly as TradeAction without filtering.
	// This means the caller gets trades with Action="split" which isn't buy/sell.
	p := &models.Portfolio{
		Holdings: []models.Holding{
			{
				Ticker: "BHP.AU",
				Trades: []*models.NavexaTrade{
					{Type: "buy", Date: "2024-01-15", Units: 100, Price: 50},
					{Type: "split", Date: "2024-03-15", Units: 200, Price: 0},
					{Type: "dividend", Date: "2024-06-15", Units: 0, Price: 5},
					{Type: "sell", Date: "2024-09-15", Units: 50, Price: 55},
				},
			},
		},
	}
	trades := extractNavexaTrades(p, interfaces.TradeFilter{})

	// FINDING: All 4 trades are returned, including split and dividend.
	// The MCP trade_list consumer may not expect these types.
	require.Len(t, trades, 4, "all trade types are returned including split/dividend")

	// Filter by action="buy" should exclude split and dividend
	filtered := extractNavexaTrades(p, interfaces.TradeFilter{Action: "buy"})
	require.Len(t, filtered, 1)
	assert.Equal(t, models.TradeAction("buy"), filtered[0].Action)
}

func TestExtractNavexaTrades_TickerFilterCaseSensitive(t *testing.T) {
	// Verify ticker filter is case-sensitive (holdings use uppercase).
	p := &models.Portfolio{
		Holdings: []models.Holding{
			{Ticker: "BHP.AU", Trades: []*models.NavexaTrade{
				{Type: "buy", Date: "2024-01-15", Units: 100, Price: 50},
			}},
		},
	}

	// Exact match
	trades := extractNavexaTrades(p, interfaces.TradeFilter{Ticker: "BHP.AU"})
	assert.Len(t, trades, 1)

	// Case mismatch
	trades = extractNavexaTrades(p, interfaces.TradeFilter{Ticker: "bhp.au"})
	assert.Len(t, trades, 0, "FINDING: ticker filter is case-sensitive — 'bhp.au' doesn't match 'BHP.AU'")
}

func TestExtractNavexaTrades_DateFilter(t *testing.T) {
	p := &models.Portfolio{
		Holdings: []models.Holding{
			{
				Ticker: "BHP.AU",
				Trades: []*models.NavexaTrade{
					{Type: "buy", Date: "2024-01-15", Units: 100, Price: 50},
					{Type: "buy", Date: "2024-06-15", Units: 50, Price: 55},
					{Type: "sell", Date: "2024-12-15", Units: 30, Price: 60},
				},
			},
		},
	}

	// DateFrom filter
	from := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	trades := extractNavexaTrades(p, interfaces.TradeFilter{DateFrom: from})
	assert.Len(t, trades, 2, "should include June and December trades")

	// DateTo filter
	to := time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)
	trades = extractNavexaTrades(p, interfaces.TradeFilter{DateTo: to})
	assert.Len(t, trades, 2, "should include January and June trades")

	// Both filters
	trades = extractNavexaTrades(p, interfaces.TradeFilter{DateFrom: from, DateTo: to})
	assert.Len(t, trades, 1, "should include only June trade")
}

func TestExtractNavexaTrades_SortDescending(t *testing.T) {
	p := &models.Portfolio{
		Holdings: []models.Holding{
			{Ticker: "A.AU", Trades: []*models.NavexaTrade{
				{Type: "buy", Date: "2024-01-15", Units: 10, Price: 10},
			}},
			{Ticker: "B.AU", Trades: []*models.NavexaTrade{
				{Type: "buy", Date: "2024-12-15", Units: 20, Price: 20},
			}},
			{Ticker: "C.AU", Trades: []*models.NavexaTrade{
				{Type: "buy", Date: "2024-06-15", Units: 30, Price: 30},
			}},
		},
	}

	trades := extractNavexaTrades(p, interfaces.TradeFilter{})
	require.Len(t, trades, 3)

	// Most recent first
	assert.Equal(t, "B.AU", trades[0].Ticker)
	assert.Equal(t, "C.AU", trades[1].Ticker)
	assert.Equal(t, "A.AU", trades[2].Ticker)
}

func TestExtractNavexaTrades_Pagination_OffsetBeyondTotal(t *testing.T) {
	// This tests the handler-level pagination, not extractNavexaTrades directly.
	// When offset >= total, the handler returns empty trades.
	p := &models.Portfolio{
		Holdings: []models.Holding{
			{Ticker: "BHP.AU", Trades: []*models.NavexaTrade{
				{Type: "buy", Date: "2024-01-15", Units: 100, Price: 50},
			}},
		},
	}
	trades := extractNavexaTrades(p, interfaces.TradeFilter{})
	require.Len(t, trades, 1)

	// Handler pagination: offset=10, total=1 → empty result
	// (We can't test handler pagination here directly, but verify extractNavexaTrades
	// returns the correct total for the handler to use.)
}

func TestExtractNavexaTrades_LargeHoldingCount(t *testing.T) {
	// 100 holdings with 10 trades each = 1000 trades.
	// extractNavexaTrades should handle this without issue.
	var holdings []models.Holding
	for i := 0; i < 100; i++ {
		var trades []*models.NavexaTrade
		for j := 0; j < 10; j++ {
			trades = append(trades, &models.NavexaTrade{
				Type:  "buy",
				Date:  "2024-01-15",
				Units: float64(i*10 + j),
				Price: 50,
			})
		}
		holdings = append(holdings, models.Holding{
			Ticker: "TICK" + string(rune('A'+i%26)) + ".AU",
			Trades: trades,
		})
	}
	p := &models.Portfolio{Holdings: holdings}

	trades := extractNavexaTrades(p, interfaces.TradeFilter{})
	assert.Len(t, trades, 1000)
}

func TestExtractNavexaTrades_SourceTypeAlwaysNavexa(t *testing.T) {
	p := &models.Portfolio{
		Holdings: []models.Holding{
			{Ticker: "BHP.AU", Trades: []*models.NavexaTrade{
				{Type: "buy", Date: "2024-01-15", Units: 100, Price: 50},
			}},
		},
	}
	trades := extractNavexaTrades(p, interfaces.TradeFilter{})
	require.Len(t, trades, 1)
	assert.Equal(t, models.SourceNavexa, trades[0].SourceType)
}

func TestExtractNavexaTrades_ZeroUnitsAndPrice(t *testing.T) {
	// NavexaTrade with zero units and zero price (e.g., corporate action placeholder)
	p := &models.Portfolio{
		Holdings: []models.Holding{
			{Ticker: "BHP.AU", Trades: []*models.NavexaTrade{
				{Type: "buy", Date: "2024-01-15", Units: 0, Price: 0, Fees: 0},
			}},
		},
	}
	trades := extractNavexaTrades(p, interfaces.TradeFilter{})
	require.Len(t, trades, 1)
	assert.Equal(t, 0.0, trades[0].Units)
	assert.Equal(t, 0.0, trades[0].Price)
}
