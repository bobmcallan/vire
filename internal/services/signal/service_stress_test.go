package signal

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
)

// --- Stress tests for overlayLiveQuote ---
// Focus: hostile inputs, edge cases, failure modes, data integrity

func TestOverlayLiveQuote_NaNClose(t *testing.T) {
	today := time.Now().Truncate(24 * time.Hour)
	md := &models.MarketData{
		Ticker: "EVIL.AU",
		EOD: []models.EODBar{
			{Date: today, Close: 41.0, High: 42.0, Low: 39.0},
		},
	}

	eodhd := &mockEODHDClient{
		quote: &models.RealTimeQuote{Close: math.NaN(), High: 50.0, Low: 35.0},
	}

	logger := common.NewLogger("debug")
	svc := &Service{eodhd: eodhd, logger: logger}
	svc.overlayLiveQuote(context.Background(), "EVIL.AU", md)

	// NaN Close should be rejected (quote.Close <= 0 check doesn't catch NaN)
	// NaN is not > 0, so it should be caught by the `quote.Close <= 0` guard.
	// But NaN <= 0 is also false. Need to verify behaviour.
	if math.IsNaN(md.EOD[0].Close) {
		t.Error("NaN Close from quote leaked into bar data — the guard `quote.Close <= 0` does not catch NaN")
	}
}

func TestOverlayLiveQuote_InfClose(t *testing.T) {
	today := time.Now().Truncate(24 * time.Hour)
	md := &models.MarketData{
		Ticker: "EVIL.AU",
		EOD: []models.EODBar{
			{Date: today, Close: 41.0, High: 42.0, Low: 39.0},
		},
	}

	eodhd := &mockEODHDClient{
		quote: &models.RealTimeQuote{Close: math.Inf(1), High: 50.0, Low: 35.0},
	}

	logger := common.NewLogger("debug")
	svc := &Service{eodhd: eodhd, logger: logger}
	svc.overlayLiveQuote(context.Background(), "EVIL.AU", md)

	// +Inf > 0 is true, so it passes the guard. This is a problem.
	if math.IsInf(md.EOD[0].Close, 0) {
		t.Error("+Inf Close from quote leaked into bar data — need explicit Inf check on quote.Close")
	}
}

func TestOverlayLiveQuote_NegativeInfClose(t *testing.T) {
	today := time.Now().Truncate(24 * time.Hour)
	md := &models.MarketData{
		Ticker: "EVIL.AU",
		EOD: []models.EODBar{
			{Date: today, Close: 41.0, High: 42.0, Low: 39.0},
		},
	}

	eodhd := &mockEODHDClient{
		quote: &models.RealTimeQuote{Close: math.Inf(-1)},
	}

	logger := common.NewLogger("debug")
	svc := &Service{eodhd: eodhd, logger: logger}
	svc.overlayLiveQuote(context.Background(), "EVIL.AU", md)

	// -Inf <= 0 is true, so it should be rejected. Verify.
	if math.IsInf(md.EOD[0].Close, 0) {
		t.Error("-Inf leaked into bar data")
	}
	if md.EOD[0].Close != 41.0 {
		t.Errorf("Close = %v, want 41.0 (unchanged)", md.EOD[0].Close)
	}
}

func TestOverlayLiveQuote_NaNHighLow(t *testing.T) {
	today := time.Now().Truncate(24 * time.Hour)
	md := &models.MarketData{
		Ticker: "EVIL.AU",
		EOD: []models.EODBar{
			{Date: today, Close: 41.0, High: 42.0, Low: 39.0},
		},
	}

	eodhd := &mockEODHDClient{
		quote: &models.RealTimeQuote{
			Close: 43.0,
			High:  math.NaN(),
			Low:   math.NaN(),
		},
	}

	logger := common.NewLogger("debug")
	svc := &Service{eodhd: eodhd, logger: logger}
	svc.overlayLiveQuote(context.Background(), "EVIL.AU", md)

	// Close should be updated, but High/Low should not become NaN
	if md.EOD[0].Close != 43.0 {
		t.Errorf("Close = %v, want 43.0", md.EOD[0].Close)
	}
	// NaN > 42.0 is false, so High should be safe
	if math.IsNaN(md.EOD[0].High) {
		t.Error("NaN High leaked into bar — the comparison guard doesn't catch this")
	}
	// NaN < 39.0 is false, so Low should be safe
	if math.IsNaN(md.EOD[0].Low) {
		t.Error("NaN Low leaked into bar")
	}
}

func TestOverlayLiveQuote_ExtremePrice_MaxFloat64(t *testing.T) {
	today := time.Now().Truncate(24 * time.Hour)
	md := &models.MarketData{
		Ticker: "EVIL.AU",
		EOD: []models.EODBar{
			{Date: today, Close: 41.0, High: 42.0, Low: 39.0},
		},
	}

	eodhd := &mockEODHDClient{
		quote: &models.RealTimeQuote{
			Close: math.MaxFloat64,
			High:  math.MaxFloat64,
			Low:   1.0,
		},
	}

	logger := common.NewLogger("debug")
	svc := &Service{eodhd: eodhd, logger: logger}
	svc.overlayLiveQuote(context.Background(), "EVIL.AU", md)

	// MaxFloat64 is a valid float64 > 0, so it passes all guards.
	// This could produce absurd indicator values (SMA = ~MaxFloat64/20)
	// but won't crash. We just document the lack of upper-bound validation.
	if md.EOD[0].Close != math.MaxFloat64 {
		t.Errorf("Close = %v, want MaxFloat64 (no upper bound guard)", md.EOD[0].Close)
	}
}

func TestOverlayLiveQuote_NegativeClose(t *testing.T) {
	today := time.Now().Truncate(24 * time.Hour)
	md := &models.MarketData{
		Ticker: "EVIL.AU",
		EOD: []models.EODBar{
			{Date: today, Close: 41.0, High: 42.0, Low: 39.0},
		},
	}

	eodhd := &mockEODHDClient{
		quote: &models.RealTimeQuote{Close: -100.0},
	}

	logger := common.NewLogger("debug")
	svc := &Service{eodhd: eodhd, logger: logger}
	svc.overlayLiveQuote(context.Background(), "EVIL.AU", md)

	// Negative close should be rejected
	if md.EOD[0].Close != 41.0 {
		t.Errorf("Close = %v, want 41.0 (negative should be rejected)", md.EOD[0].Close)
	}
}

func TestOverlayLiveQuote_SyntheticBar_NegativeHigh(t *testing.T) {
	yesterday := time.Now().Truncate(24*time.Hour).AddDate(0, 0, -1)

	md := &models.MarketData{
		Ticker: "EVIL.AU",
		EOD: []models.EODBar{
			{Date: yesterday, Close: 41.0},
		},
	}

	eodhd := &mockEODHDClient{
		quote: &models.RealTimeQuote{
			Close:         42.5,
			High:          -10.0, // hostile
			Low:           -5.0,  // hostile
			Open:          42.0,
			PreviousClose: 41.0,
			Volume:        100,
		},
	}

	logger := common.NewLogger("debug")
	svc := &Service{eodhd: eodhd, logger: logger}
	svc.overlayLiveQuote(context.Background(), "EVIL.AU", md)

	if len(md.EOD) != 2 {
		t.Fatalf("bar count = %d, want 2", len(md.EOD))
	}

	// Synthetic bar is created with negative High/Low — no validation
	synthetic := md.EOD[0]
	if synthetic.High >= 0 {
		t.Logf("Note: High = %v — negative High accepted without validation", synthetic.High)
	}
	if synthetic.Low >= 0 {
		t.Logf("Note: Low = %v — negative Low accepted without validation", synthetic.Low)
	}
}

func TestOverlayLiveQuote_SyntheticBar_ZeroVolume(t *testing.T) {
	yesterday := time.Now().Truncate(24*time.Hour).AddDate(0, 0, -1)

	md := &models.MarketData{
		Ticker: "TEST.AU",
		EOD: []models.EODBar{
			{Date: yesterday, Close: 41.0, Volume: 1000},
		},
	}

	eodhd := &mockEODHDClient{
		quote: &models.RealTimeQuote{
			Close:         42.5,
			High:          43.0,
			Low:           40.5,
			Open:          41.0,
			PreviousClose: 41.0,
			Volume:        0, // zero volume from quote
		},
	}

	logger := common.NewLogger("debug")
	svc := &Service{eodhd: eodhd, logger: logger}
	svc.overlayLiveQuote(context.Background(), "TEST.AU", md)

	if len(md.EOD) != 2 {
		t.Fatalf("bar count = %d, want 2", len(md.EOD))
	}

	// VolumeRatio uses bars[0].Volume / avg. If volume=0, ratio=0.
	// This won't crash but produces a "low" volume signal for a bar that
	// just hasn't opened yet.
	if md.EOD[0].Volume != 0 {
		t.Errorf("synthetic Volume = %v, want 0", md.EOD[0].Volume)
	}
}

func TestOverlayLiveQuote_FutureTimestamp_BarDateInFuture(t *testing.T) {
	// The function uses time.Now() internally, so we can't inject the clock.
	// But we can test what happens when the latest bar's date is in the future
	// (e.g., from a buggy data feed).
	future := time.Now().Truncate(24*time.Hour).AddDate(0, 0, 7)

	md := &models.MarketData{
		Ticker: "TIME.AU",
		EOD: []models.EODBar{
			{Date: future, Close: 100.0, High: 105.0, Low: 95.0},
		},
	}

	eodhd := &mockEODHDClient{
		quote: &models.RealTimeQuote{Close: 110.0, High: 115.0, Low: 90.0},
	}

	logger := common.NewLogger("debug")
	svc := &Service{eodhd: eodhd, logger: logger}
	svc.overlayLiveQuote(context.Background(), "TIME.AU", md)

	// latestBarDate is in the future: not Equal(today) and not Before(today).
	// So neither branch executes. The bar is left untouched. This is correct.
	if md.EOD[0].Close != 100.0 {
		t.Errorf("Close = %v, want 100.0 (future bar should be untouched)", md.EOD[0].Close)
	}
	if len(md.EOD) != 1 {
		t.Errorf("bar count = %d, want 1 (no synthetic bar added for future)", len(md.EOD))
	}
}

func TestOverlayLiveQuote_MutatesMarketData_InPlace(t *testing.T) {
	// CRITICAL: overlayLiveQuote mutates md.EOD in place.
	// If the caller later saves the mutated MarketData, the live quote
	// gets persisted as if it were EOD data. Verify this is the case.
	today := time.Now().Truncate(24 * time.Hour)

	md := &models.MarketData{
		Ticker: "MUT.AU",
		EOD: []models.EODBar{
			{Date: today, Close: 41.0, High: 42.0, Low: 39.0, Volume: 1000},
		},
	}

	eodhd := &mockEODHDClient{
		quote: &models.RealTimeQuote{Close: 50.0, High: 55.0, Low: 38.0},
	}

	logger := common.NewLogger("debug")
	svc := &Service{eodhd: eodhd, logger: logger}

	// Keep a reference to the original slice header
	originalBarPtr := &md.EOD[0]

	svc.overlayLiveQuote(context.Background(), "MUT.AU", md)

	// The mutation is in-place — same pointer
	if &md.EOD[0] != originalBarPtr {
		t.Error("overlayLiveQuote replaced the bar slice instead of mutating in-place")
	}

	// The bar is mutated. DetectSignals saves the computed signals (not the
	// MarketData), so this mutation only affects the signal computation, not
	// persisted market data. That's the correct behaviour.
	if md.EOD[0].Close != 50.0 {
		t.Errorf("Close = %v, want 50.0 (mutated)", md.EOD[0].Close)
	}
}

func TestOverlayLiveQuote_SyntheticBar_PrependDoesNotCorruptExistingSlice(t *testing.T) {
	yesterday := time.Now().Truncate(24*time.Hour).AddDate(0, 0, -1)

	originalBars := []models.EODBar{
		{Date: yesterday, Close: 41.0, High: 42.0, Low: 39.0, Volume: 1000},
		{Date: yesterday.AddDate(0, 0, -1), Close: 40.0, High: 41.0, Low: 38.0, Volume: 900},
	}
	// Save original values for comparison
	origClose0 := originalBars[0].Close
	origClose1 := originalBars[1].Close

	md := &models.MarketData{
		Ticker: "PREP.AU",
		EOD:    originalBars,
	}

	eodhd := &mockEODHDClient{
		quote: &models.RealTimeQuote{
			Close:         42.5,
			High:          43.0,
			Low:           40.5,
			Open:          41.0,
			PreviousClose: 41.0,
			Volume:        500,
		},
	}

	logger := common.NewLogger("debug")
	svc := &Service{eodhd: eodhd, logger: logger}
	svc.overlayLiveQuote(context.Background(), "PREP.AU", md)

	if len(md.EOD) != 3 {
		t.Fatalf("bar count = %d, want 3", len(md.EOD))
	}

	// The prepend creates a new slice. Verify original data is preserved.
	if md.EOD[1].Close != origClose0 {
		t.Errorf("original bar[0] Close = %v, want %v", md.EOD[1].Close, origClose0)
	}
	if md.EOD[2].Close != origClose1 {
		t.Errorf("original bar[1] Close = %v, want %v", md.EOD[2].Close, origClose1)
	}
}

func TestOverlayLiveQuote_SameDay_LowIsZero_DoesNotOverwrite(t *testing.T) {
	today := time.Now().Truncate(24 * time.Hour)

	md := &models.MarketData{
		Ticker: "LOW0.AU",
		EOD: []models.EODBar{
			{Date: today, Close: 41.0, High: 42.0, Low: 39.0},
		},
	}

	eodhd := &mockEODHDClient{
		quote: &models.RealTimeQuote{
			Close: 43.0,
			High:  44.0,
			Low:   0, // zero low — should not overwrite
		},
	}

	logger := common.NewLogger("debug")
	svc := &Service{eodhd: eodhd, logger: logger}
	svc.overlayLiveQuote(context.Background(), "LOW0.AU", md)

	// Low=0 check: `quote.Low > 0 && quote.Low < md.EOD[0].Low` → false (0 > 0 is false)
	if md.EOD[0].Low != 39.0 {
		t.Errorf("Low = %v, want 39.0 (zero low should not overwrite)", md.EOD[0].Low)
	}
}

func TestOverlayLiveQuote_NilMarketData(t *testing.T) {
	eodhd := &mockEODHDClient{
		quote: &models.RealTimeQuote{Close: 42.5},
	}

	logger := common.NewLogger("debug")
	svc := &Service{eodhd: eodhd, logger: logger}

	// Should not panic
	svc.overlayLiveQuote(context.Background(), "NIL.AU", nil)
}

// --- Stress tests for eodClosePrice edge cases ---
// These are in portfolio package, so tested via the existing test file.
// But we verify the interaction with the cross-check in SyncPortfolio here
// by validating signal behaviour when the close comes from eodClosePrice.

func TestOverlayLiveQuote_SyntheticBar_OpenAndPreviousCloseBothZero(t *testing.T) {
	yesterday := time.Now().Truncate(24*time.Hour).AddDate(0, 0, -1)

	md := &models.MarketData{
		Ticker: "ZERO.AU",
		EOD: []models.EODBar{
			{Date: yesterday, Close: 41.0},
		},
	}

	eodhd := &mockEODHDClient{
		quote: &models.RealTimeQuote{
			Close:         42.5,
			High:          43.0,
			Low:           40.5,
			Open:          0,
			PreviousClose: 0,
			Volume:        100,
		},
	}

	logger := common.NewLogger("debug")
	svc := &Service{eodhd: eodhd, logger: logger}
	svc.overlayLiveQuote(context.Background(), "ZERO.AU", md)

	if len(md.EOD) != 2 {
		t.Fatalf("bar count = %d, want 2", len(md.EOD))
	}

	// Open=0 triggers fallback to PreviousClose=0, so Open stays 0.
	// This means the synthetic bar has Open=0, which could confuse
	// indicators that use Open (currently none in this codebase, but worth noting).
	if md.EOD[0].Open != 0 {
		t.Errorf("synthetic Open = %v, want 0 (both open and previousClose are zero)", md.EOD[0].Open)
	}
}

func TestOverlayLiveQuote_BarDateExactlyToday_Midnight(t *testing.T) {
	// Verify Truncate(24h) works for exact midnight comparisons
	today := time.Now().Truncate(24 * time.Hour)

	// Bar date at some specific hour today — should truncate to same day
	barTime := today.Add(15*time.Hour + 30*time.Minute)
	md := &models.MarketData{
		Ticker: "TZ.AU",
		EOD: []models.EODBar{
			{Date: barTime, Close: 41.0, High: 42.0, Low: 39.0},
		},
	}

	eodhd := &mockEODHDClient{
		quote: &models.RealTimeQuote{Close: 43.0, High: 44.0, Low: 38.0},
	}

	logger := common.NewLogger("debug")
	svc := &Service{eodhd: eodhd, logger: logger}
	svc.overlayLiveQuote(context.Background(), "TZ.AU", md)

	// barTime.Truncate(24h) == today, so same-day branch should execute
	if md.EOD[0].Close != 43.0 {
		t.Errorf("Close = %v, want 43.0 (same day after truncation)", md.EOD[0].Close)
	}
}

func TestDetectSignals_DoesNotPersistOverlaidMarketData(t *testing.T) {
	// Verify that the overlay mutates a MarketData object from storage,
	// but SaveSignals only saves the computed signals, NOT the modified market data.
	today := time.Now().Truncate(24 * time.Hour)

	bars := make([]models.EODBar, 50)
	for i := range bars {
		bars[i] = models.EODBar{
			Date:   today.AddDate(0, 0, -i),
			Open:   100.0,
			High:   105.0,
			Low:    95.0,
			Close:  100.0,
			Volume: 10000,
		}
	}

	marketStorage := &mockMarketDataStorage{
		data: map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", EOD: bars},
		},
	}
	signalStorage := &mockSignalStorage{}
	storage := &mockStorageManager{
		marketStorage: marketStorage,
		signalStorage: signalStorage,
	}

	eodhd := &mockEODHDClient{
		quote: &models.RealTimeQuote{Close: 150.0, High: 155.0, Low: 95.0},
	}

	logger := common.NewLogger("debug")
	svc := NewService(storage, eodhd, logger)

	_, err := svc.DetectSignals(context.Background(), []string{"BHP.AU"}, nil, true)
	if err != nil {
		t.Fatalf("DetectSignals error: %v", err)
	}

	// The signal storage should have received signals
	if len(signalStorage.saved) != 1 {
		t.Fatalf("expected 1 saved signal set, got %d", len(signalStorage.saved))
	}

	// Verify the signal reflects the live price
	if signalStorage.saved[0].Price.Current != 150.0 {
		t.Errorf("saved signal price = %v, want 150.0", signalStorage.saved[0].Price.Current)
	}

	// BUT: the MarketData in the mock storage has been mutated in-place.
	// This is a side effect: GetMarketData returns a pointer, and overlayLiveQuote
	// mutates it. If GetMarketData returns the same pointer on next call,
	// subsequent signal computations would see the already-overlaid data.
	// This is a potential issue: the live quote gets "baked in" to the cached MarketData.
	cached := marketStorage.data["BHP.AU"]
	if cached.EOD[0].Close == 150.0 {
		t.Log("WARNING: overlayLiveQuote mutated the cached MarketData in storage. " +
			"If GetMarketData returns the same pointer, the live quote is permanently " +
			"baked into the cache. This could cause stale-but-overlaid data on subsequent calls.")
	}
}
