package data

import (
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFilingSummaryBatchPersistence verifies that MarketData with filing summaries
// can be saved incrementally (after each batch) and that intermediate saves
// preserve existing data while adding new summaries.
func TestFilingSummaryBatchPersistence(t *testing.T) {
	mgr := testManager(t)
	store := mgr.MarketDataStorage()
	ctx := testContext()

	now := time.Now().Truncate(time.Second)

	// Create initial MarketData with EOD, filings, but no summaries
	md := &models.MarketData{
		Ticker:   "BATCH.AU",
		Exchange: "AU",
		Name:     "Batch Test Ltd",
		EOD: []models.EODBar{
			{Date: now, Close: 10.0, Volume: 100000},
			{Date: now.AddDate(0, 0, -1), Close: 9.50, Volume: 90000},
		},
		Filings: []models.CompanyFiling{
			{Date: now, Headline: "Annual Report", DocumentKey: "doc1"},
			{Date: now.AddDate(0, -1, 0), Headline: "Quarterly Report", DocumentKey: "doc2"},
			{Date: now.AddDate(0, -3, 0), Headline: "Half Year Report", DocumentKey: "doc3"},
		},
		LastUpdated:      now,
		FilingsUpdatedAt: now,
	}

	require.NoError(t, store.SaveMarketData(ctx, md))

	t.Run("initial state has no summaries", func(t *testing.T) {
		got, err := store.GetMarketData(ctx, "BATCH.AU")
		require.NoError(t, err)
		assert.Nil(t, got.FilingSummaries)
		assert.Len(t, got.EOD, 2, "EOD bars should be preserved")
		assert.Len(t, got.Filings, 3, "filings should be preserved")
	})

	t.Run("first batch save persists summaries without losing EOD", func(t *testing.T) {
		// Simulate first batch: add 2 summaries
		md.FilingSummaries = []models.FilingSummary{
			{Date: now, Headline: "Annual Report", Type: "financial_results", Revenue: "$100M"},
			{Date: now.AddDate(0, -1, 0), Headline: "Quarterly Report", Type: "financial_results", Revenue: "$25M"},
		}
		md.FilingSummariesUpdatedAt = now
		require.NoError(t, store.SaveMarketData(ctx, md))

		got, err := store.GetMarketData(ctx, "BATCH.AU")
		require.NoError(t, err)
		assert.Len(t, got.FilingSummaries, 2, "should have 2 summaries after first batch")
		assert.Len(t, got.EOD, 2, "EOD bars must be preserved across batch save")
		assert.Len(t, got.Filings, 3, "filings must be preserved across batch save")
		assert.Equal(t, "$100M", got.FilingSummaries[0].Revenue)
	})

	t.Run("second batch save appends summaries", func(t *testing.T) {
		// Simulate second batch: add 1 more summary
		md.FilingSummaries = append(md.FilingSummaries, models.FilingSummary{
			Date: now.AddDate(0, -3, 0), Headline: "Half Year Report", Type: "financial_results", Revenue: "$50M",
		})
		require.NoError(t, store.SaveMarketData(ctx, md))

		got, err := store.GetMarketData(ctx, "BATCH.AU")
		require.NoError(t, err)
		assert.Len(t, got.FilingSummaries, 3, "should have 3 summaries after second batch")
		assert.Len(t, got.EOD, 2, "EOD bars must still be preserved")
	})
}

// TestFilingSummaryPersistenceWithNilFields verifies that saving MarketData
// with some fields nil'd (as happens during memory optimization) does not
// corrupt previously saved data when those fields are restored before saving.
func TestFilingSummaryPersistenceWithNilFields(t *testing.T) {
	mgr := testManager(t)
	store := mgr.MarketDataStorage()
	ctx := testContext()

	now := time.Now().Truncate(time.Second)

	// Create full MarketData with all fields populated
	md := &models.MarketData{
		Ticker:   "NILTEST.AU",
		Exchange: "AU",
		Name:     "NilTest Ltd",
		EOD: []models.EODBar{
			{Date: now, Close: 42.50, Volume: 3000000},
		},
		News: []*models.NewsItem{
			{PublishedAt: now, Title: "Test news", Source: "Test"},
		},
		CompanyTimeline: &models.CompanyTimeline{
			BusinessModel: "Engineering services",
			GeneratedAt:   now,
		},
		Filings: []models.CompanyFiling{
			{Date: now, Headline: "Annual Report", DocumentKey: "doc1"},
		},
		LastUpdated:      now,
		FilingsUpdatedAt: now,
	}

	require.NoError(t, store.SaveMarketData(ctx, md))

	// Verify all data is stored
	initial, err := store.GetMarketData(ctx, "NILTEST.AU")
	require.NoError(t, err)
	assert.Len(t, initial.EOD, 1)
	assert.Len(t, initial.News, 1)
	assert.NotNil(t, initial.CompanyTimeline)

	t.Run("save with nil EOD and News does overwrite those fields", func(t *testing.T) {
		// This simulates what would happen if save is called without restoring fields
		md.EOD = nil
		md.News = nil
		md.CompanyTimeline = nil
		md.FilingSummaries = []models.FilingSummary{
			{Date: now, Headline: "Annual Report", Type: "financial_results"},
		}
		require.NoError(t, store.SaveMarketData(ctx, md))

		got, err := store.GetMarketData(ctx, "NILTEST.AU")
		require.NoError(t, err)
		assert.Len(t, got.FilingSummaries, 1, "summaries should be saved")
		// When fields are nil'd and saved, they should be empty/nil in storage
		assert.Empty(t, got.EOD, "EOD should be empty when saved as nil")
		assert.Empty(t, got.News, "News should be empty when saved as nil")
		assert.Nil(t, got.CompanyTimeline, "Timeline should be nil when saved as nil")
	})
}

// TestMarketDataBatchRetrievalWithSummaries verifies that batch retrieval of
// market data correctly returns filing summaries for multiple tickers.
func TestMarketDataBatchRetrievalWithSummaries(t *testing.T) {
	mgr := testManager(t)
	store := mgr.MarketDataStorage()
	ctx := testContext()

	now := time.Now().Truncate(time.Second)

	tickers := []string{"SUM_A.AU", "SUM_B.AU", "SUM_C.AU"}
	for i, ticker := range tickers {
		md := &models.MarketData{
			Ticker:      ticker,
			Exchange:    "AU",
			LastUpdated: now,
			FilingSummaries: []models.FilingSummary{
				{
					Date:     now,
					Headline: "Report " + ticker,
					Revenue:  "$" + string(rune('1'+i)) + "00M",
				},
			},
			FilingSummariesUpdatedAt: now,
		}
		require.NoError(t, store.SaveMarketData(ctx, md))
	}

	results, err := store.GetMarketDataBatch(ctx, tickers)
	require.NoError(t, err)
	assert.Len(t, results, 3)

	for _, md := range results {
		assert.Len(t, md.FilingSummaries, 1,
			"ticker %s should have 1 filing summary", md.Ticker)
		assert.NotEmpty(t, md.FilingSummaries[0].Revenue,
			"ticker %s should have revenue in summary", md.Ticker)
	}
}
