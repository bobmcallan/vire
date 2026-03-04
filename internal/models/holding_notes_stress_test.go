package models

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// =============================================================================
// Holding Notes — adversarial stress tests
// =============================================================================

// --- ValidAssetType ---

func TestValidAssetType_KnownTypes(t *testing.T) {
	assert.True(t, ValidAssetType(AssetTypeETF))
	assert.True(t, ValidAssetType(AssetTypeASXStock))
	assert.True(t, ValidAssetType(AssetTypeUSEquity))
}

func TestValidAssetType_EmptyString(t *testing.T) {
	assert.False(t, ValidAssetType(""))
}

func TestValidAssetType_CaseSensitive(t *testing.T) {
	// "etf" is not "ETF" — should be rejected
	assert.False(t, ValidAssetType("etf"))
	assert.False(t, ValidAssetType("Etf"))
	assert.False(t, ValidAssetType("asx_stock"))
	assert.False(t, ValidAssetType("us_equity"))
}

func TestValidAssetType_Injection(t *testing.T) {
	assert.False(t, ValidAssetType("ETF'; DROP TABLE notes;--"))
	assert.False(t, ValidAssetType("<script>alert('xss')</script>"))
	assert.False(t, ValidAssetType("ETF\x00NULL"))
}

func TestValidAssetType_UnknownType(t *testing.T) {
	assert.False(t, ValidAssetType("CRYPTO"))
	assert.False(t, ValidAssetType("BOND"))
	assert.False(t, ValidAssetType("COMMODITY"))
}

// --- IsStale ---

func TestIsStale_DefaultTTL_Fresh(t *testing.T) {
	note := &HoldingNote{
		ReviewedAt: time.Now().Add(-24 * time.Hour), // 1 day ago
	}
	assert.False(t, note.IsStale(), "1-day-old note with 90-day TTL should not be stale")
}

func TestIsStale_DefaultTTL_Stale(t *testing.T) {
	note := &HoldingNote{
		ReviewedAt: time.Now().Add(-91 * 24 * time.Hour), // 91 days ago
	}
	assert.True(t, note.IsStale(), "91-day-old note with 90-day TTL should be stale")
}

func TestIsStale_DefaultTTL_Boundary(t *testing.T) {
	note := &HoldingNote{
		ReviewedAt: time.Now().Add(-90 * 24 * time.Hour), // exactly 90 days ago
	}
	// time.Since adds sub-second drift, so 90 days + epsilon > 90 days → stale
	assert.True(t, note.IsStale(), "exactly 90 days should be stale (due to sub-second drift)")
}

func TestIsStale_CustomTTL(t *testing.T) {
	note := &HoldingNote{
		StaleDays:  30,
		ReviewedAt: time.Now().Add(-31 * 24 * time.Hour),
	}
	assert.True(t, note.IsStale(), "31-day-old note with 30-day TTL should be stale")
}

func TestIsStale_CustomTTL_Fresh(t *testing.T) {
	note := &HoldingNote{
		StaleDays:  30,
		ReviewedAt: time.Now().Add(-15 * 24 * time.Hour),
	}
	assert.False(t, note.IsStale(), "15-day-old note with 30-day TTL should not be stale")
}

func TestIsStale_ZeroStaleDays(t *testing.T) {
	// StaleDays=0 should default to 90
	note := &HoldingNote{
		StaleDays:  0,
		ReviewedAt: time.Now().Add(-89 * 24 * time.Hour),
	}
	assert.False(t, note.IsStale(), "StaleDays=0 should default to 90, so 89 days is fresh")
}

func TestIsStale_NegativeStaleDays(t *testing.T) {
	// StaleDays < 0 should default to 90 (ttl <= 0 check)
	note := &HoldingNote{
		StaleDays:  -5,
		ReviewedAt: time.Now().Add(-89 * 24 * time.Hour),
	}
	assert.False(t, note.IsStale(), "negative StaleDays should default to 90")
}

func TestIsStale_ZeroReviewedAt(t *testing.T) {
	// Zero ReviewedAt is time.Time{} which is year 0001
	// time.Since(zero) is huge → always stale
	note := &HoldingNote{}
	assert.True(t, note.IsStale(), "zero ReviewedAt should always be stale")
}

func TestIsStale_FutureReviewedAt(t *testing.T) {
	// ReviewedAt in the future (e.g., clock skew)
	note := &HoldingNote{
		ReviewedAt: time.Now().Add(24 * time.Hour), // tomorrow
	}
	assert.False(t, note.IsStale(), "future ReviewedAt should not be stale")
}

func TestIsStale_VeryLargeStaleDays(t *testing.T) {
	note := &HoldingNote{
		StaleDays:  36500, // 100 years
		ReviewedAt: time.Now().Add(-365 * 24 * time.Hour),
	}
	assert.False(t, note.IsStale(), "1-year-old note with 100-year TTL should not be stale")
}

func TestIsStale_StaleDaysOne(t *testing.T) {
	note := &HoldingNote{
		StaleDays:  1,
		ReviewedAt: time.Now().Add(-25 * time.Hour), // 25 hours ago
	}
	assert.True(t, note.IsStale(), "25-hour-old note with 1-day TTL should be stale")
}

// --- DeriveSignalConfidence ---

func TestDeriveSignalConfidence_NilReceiver(t *testing.T) {
	var note *HoldingNote
	assert.Equal(t, SignalConfidenceMedium, note.DeriveSignalConfidence(), "nil receiver should return medium")
}

func TestDeriveSignalConfidence_ETF(t *testing.T) {
	note := &HoldingNote{AssetType: AssetTypeETF}
	assert.Equal(t, SignalConfidenceLow, note.DeriveSignalConfidence())
}

func TestDeriveSignalConfidence_ETF_LowLiquidity(t *testing.T) {
	// ETF + low liquidity → still low (ETF dominates)
	note := &HoldingNote{AssetType: AssetTypeETF, LiquidityProfile: LiquidityLow}
	assert.Equal(t, SignalConfidenceLow, note.DeriveSignalConfidence())
}

func TestDeriveSignalConfidence_ETF_HighLiquidity(t *testing.T) {
	// ETF + high liquidity → still low (ETF dominates)
	note := &HoldingNote{AssetType: AssetTypeETF, LiquidityProfile: LiquidityHigh}
	assert.Equal(t, SignalConfidenceLow, note.DeriveSignalConfidence())
}

func TestDeriveSignalConfidence_ASXStock_HighLiquidity(t *testing.T) {
	note := &HoldingNote{AssetType: AssetTypeASXStock, LiquidityProfile: LiquidityHigh}
	assert.Equal(t, SignalConfidenceHigh, note.DeriveSignalConfidence())
}

func TestDeriveSignalConfidence_ASXStock_MediumLiquidity(t *testing.T) {
	note := &HoldingNote{AssetType: AssetTypeASXStock, LiquidityProfile: LiquidityMedium}
	assert.Equal(t, SignalConfidenceHigh, note.DeriveSignalConfidence())
}

func TestDeriveSignalConfidence_ASXStock_LowLiquidity(t *testing.T) {
	note := &HoldingNote{AssetType: AssetTypeASXStock, LiquidityProfile: LiquidityLow}
	assert.Equal(t, SignalConfidenceMedium, note.DeriveSignalConfidence())
}

func TestDeriveSignalConfidence_ASXStock_NoLiquidity(t *testing.T) {
	// Empty liquidity profile — defaults to high (doesn't match LiquidityLow)
	note := &HoldingNote{AssetType: AssetTypeASXStock}
	assert.Equal(t, SignalConfidenceHigh, note.DeriveSignalConfidence())
}

func TestDeriveSignalConfidence_USEquity(t *testing.T) {
	note := &HoldingNote{AssetType: AssetTypeUSEquity}
	assert.Equal(t, SignalConfidenceHigh, note.DeriveSignalConfidence())
}

func TestDeriveSignalConfidence_USEquity_LowLiquidity(t *testing.T) {
	// US equity + low liquidity → still high (liquidity profile ignored for US)
	note := &HoldingNote{AssetType: AssetTypeUSEquity, LiquidityProfile: LiquidityLow}
	assert.Equal(t, SignalConfidenceHigh, note.DeriveSignalConfidence())
}

func TestDeriveSignalConfidence_EmptyAssetType(t *testing.T) {
	note := &HoldingNote{}
	assert.Equal(t, SignalConfidenceMedium, note.DeriveSignalConfidence())
}

func TestDeriveSignalConfidence_UnknownAssetType(t *testing.T) {
	note := &HoldingNote{AssetType: "CRYPTO"}
	assert.Equal(t, SignalConfidenceMedium, note.DeriveSignalConfidence())
}

func TestDeriveSignalConfidence_InvalidLiquidity(t *testing.T) {
	// ASX_stock + invalid liquidity → defaults to high (invalid != "low")
	note := &HoldingNote{AssetType: AssetTypeASXStock, LiquidityProfile: "INVALID"}
	assert.Equal(t, SignalConfidenceHigh, note.DeriveSignalConfidence())
}

// --- FindByTicker ---

func TestFindByTicker_CaseInsensitive(t *testing.T) {
	notes := &PortfolioHoldingNotes{
		Items: []HoldingNote{{Ticker: "BHP.AU"}},
	}
	note, idx := notes.FindByTicker("bhp.au")
	assert.Equal(t, 0, idx)
	assert.Equal(t, "BHP.AU", note.Ticker)
}

func TestFindByTicker_Empty(t *testing.T) {
	notes := &PortfolioHoldingNotes{Items: []HoldingNote{}}
	note, idx := notes.FindByTicker("BHP.AU")
	assert.Nil(t, note)
	assert.Equal(t, -1, idx)
}

func TestFindByTicker_NilItems(t *testing.T) {
	notes := &PortfolioHoldingNotes{}
	note, idx := notes.FindByTicker("BHP.AU")
	assert.Nil(t, note)
	assert.Equal(t, -1, idx)
}

func TestFindByTicker_MultipleMatches(t *testing.T) {
	// If Items somehow has duplicates (same ticker, different case), FindByTicker returns first match
	notes := &PortfolioHoldingNotes{
		Items: []HoldingNote{
			{Ticker: "BHP.AU", Thesis: "first"},
			{Ticker: "bhp.au", Thesis: "second"},
		},
	}
	note, idx := notes.FindByTicker("BHP.AU")
	assert.Equal(t, 0, idx, "should return first match")
	assert.Equal(t, "first", note.Thesis)
}

func TestFindByTicker_EmptyTicker(t *testing.T) {
	notes := &PortfolioHoldingNotes{
		Items: []HoldingNote{{Ticker: "BHP.AU"}},
	}
	note, idx := notes.FindByTicker("")
	assert.Nil(t, note)
	assert.Equal(t, -1, idx)
}

func TestFindByTicker_EmptyTickerInItems(t *testing.T) {
	// An item with empty ticker should be findable by empty string
	notes := &PortfolioHoldingNotes{
		Items: []HoldingNote{{Ticker: "", Thesis: "ghost"}},
	}
	note, idx := notes.FindByTicker("")
	assert.Equal(t, 0, idx)
	assert.Equal(t, "ghost", note.Thesis)
}

func TestFindByTicker_SpecialCharacters(t *testing.T) {
	notes := &PortfolioHoldingNotes{
		Items: []HoldingNote{{Ticker: "BRK-B.US"}},
	}
	note, idx := notes.FindByTicker("brk-b.us")
	assert.Equal(t, 0, idx)
	assert.NotNil(t, note)
}

func TestFindByTicker_ReturnsMutablePointer(t *testing.T) {
	// FindByTicker returns &h.Items[i], so mutations affect the original slice
	notes := &PortfolioHoldingNotes{
		Items: []HoldingNote{{Ticker: "BHP.AU", Thesis: "original"}},
	}
	note, _ := notes.FindByTicker("BHP.AU")
	note.Thesis = "modified"
	assert.Equal(t, "modified", notes.Items[0].Thesis, "pointer should mutate original")
}

// --- NoteMap ---

func TestNoteMap_UppercaseKeys(t *testing.T) {
	notes := &PortfolioHoldingNotes{
		Items: []HoldingNote{
			{Ticker: "bhp.au", Thesis: "lower"},
			{Ticker: "CBA.AU", Thesis: "upper"},
		},
	}
	m := notes.NoteMap()
	assert.Contains(t, m, "BHP.AU")
	assert.Contains(t, m, "CBA.AU")
	assert.NotContains(t, m, "bhp.au")
}

func TestNoteMap_DuplicateTickers_LastWins(t *testing.T) {
	// If items have duplicate tickers (different case), NoteMap uppercases them.
	// Last item with same uppercase key overwrites earlier ones.
	notes := &PortfolioHoldingNotes{
		Items: []HoldingNote{
			{Ticker: "BHP.AU", Thesis: "first"},
			{Ticker: "bhp.au", Thesis: "second"},
		},
	}
	m := notes.NoteMap()
	assert.Equal(t, "second", m["BHP.AU"].Thesis, "last item with same uppercase key should win")
}

func TestNoteMap_EmptyItems(t *testing.T) {
	notes := &PortfolioHoldingNotes{Items: []HoldingNote{}}
	m := notes.NoteMap()
	assert.NotNil(t, m)
	assert.Len(t, m, 0)
}

func TestNoteMap_NilItems(t *testing.T) {
	notes := &PortfolioHoldingNotes{}
	m := notes.NoteMap()
	assert.NotNil(t, m)
	assert.Len(t, m, 0)
}

func TestNoteMap_ReturnsMutablePointers(t *testing.T) {
	// NoteMap returns pointers into the Items slice — mutations propagate
	notes := &PortfolioHoldingNotes{
		Items: []HoldingNote{{Ticker: "BHP.AU", Thesis: "original"}},
	}
	m := notes.NoteMap()
	m["BHP.AU"].Thesis = "mutated"
	assert.Equal(t, "mutated", notes.Items[0].Thesis, "NoteMap pointers should reference original items")
}

func TestNoteMap_EmptyTicker(t *testing.T) {
	// Item with empty ticker → key is "" (uppercased empty string)
	notes := &PortfolioHoldingNotes{
		Items: []HoldingNote{{Ticker: "", Thesis: "ghost"}},
	}
	m := notes.NoteMap()
	assert.Contains(t, m, "")
	assert.Equal(t, "ghost", m[""].Thesis)
}

func TestNoteMap_LargeCollection(t *testing.T) {
	// 1000 items — should handle without issue
	items := make([]HoldingNote, 1000)
	for i := range items {
		items[i] = HoldingNote{Ticker: "TICK" + strings.Repeat("X", i%50) + ".AU"}
	}
	notes := &PortfolioHoldingNotes{Items: items}
	m := notes.NoteMap()
	assert.NotNil(t, m)
	assert.Greater(t, len(m), 0)
}

// --- XSS in thesis/notes fields (model layer does not sanitize — documenting) ---

func TestHoldingNote_XSSInThesis(t *testing.T) {
	// Model layer stores raw strings. XSS protection must be at the handler or frontend layer.
	note := &HoldingNote{
		Ticker: "BHP.AU",
		Thesis: "<script>alert('xss')</script>",
	}
	assert.Contains(t, note.Thesis, "<script>",
		"model layer stores raw HTML — XSS protection must happen at handler/frontend level")
}

func TestHoldingNote_LargeThesis(t *testing.T) {
	// 1MB thesis string — no panic
	note := &HoldingNote{
		Ticker: "BHP.AU",
		Thesis: strings.Repeat("A", 1024*1024),
	}
	assert.Len(t, note.Thesis, 1024*1024)
}

func TestHoldingNote_NullBytesInFields(t *testing.T) {
	note := &HoldingNote{
		Ticker: "BHP\x00.AU",
		Thesis: "Normal thesis with \x00 null byte",
	}
	// Should not panic — database layer may handle null bytes differently
	assert.Contains(t, note.Ticker, "\x00")
}
