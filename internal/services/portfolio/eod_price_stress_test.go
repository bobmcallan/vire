package portfolio

import (
	"math"
	"testing"

	"github.com/bobmcallan/vire/internal/models"
)

// --- Stress tests for eodClosePrice ---
// Focus: hostile data from EODHD API, NaN/Inf, extreme values

func TestEodClosePrice_NaN_AdjClose(t *testing.T) {
	bar := models.EODBar{Close: 100.0, AdjClose: math.NaN()}
	got := eodClosePrice(bar)
	// NaN > 0 is false, so falls through to return Close
	if got != 100.0 {
		t.Errorf("eodClosePrice with NaN AdjClose = %v, want 100.0", got)
	}
}

func TestEodClosePrice_NaN_Close(t *testing.T) {
	bar := models.EODBar{Close: math.NaN(), AdjClose: 50.0}
	got := eodClosePrice(bar)
	// AdjClose > 0 is true, Close > 0 is false (NaN > 0 is false),
	// so ratio check is skipped. Returns AdjClose.
	if got != 50.0 {
		t.Errorf("eodClosePrice with NaN Close = %v, want 50.0 (AdjClose)", got)
	}
}

func TestEodClosePrice_NaN_Both(t *testing.T) {
	bar := models.EODBar{Close: math.NaN(), AdjClose: math.NaN()}
	got := eodClosePrice(bar)
	// NaN > 0 is false for AdjClose, falls through to return Close (which is NaN).
	// This propagates NaN to the portfolio. No guard.
	if !math.IsNaN(got) {
		t.Logf("eodClosePrice with both NaN = %v (expected NaN)", got)
	} else {
		t.Log("WARNING: eodClosePrice returns NaN when both Close and AdjClose are NaN. " +
			"This NaN will propagate to portfolio valuation and corrupt holding.MarketValue.")
	}
}

func TestEodClosePrice_Inf_AdjClose(t *testing.T) {
	bar := models.EODBar{Close: 100.0, AdjClose: math.Inf(1)}
	got := eodClosePrice(bar)
	// IsInf check catches +Inf, falls back to Close
	if got != 100.0 {
		t.Errorf("eodClosePrice with +Inf AdjClose = %v, want 100.0", got)
	}
}

func TestEodClosePrice_NegInf_AdjClose(t *testing.T) {
	bar := models.EODBar{Close: 100.0, AdjClose: math.Inf(-1)}
	got := eodClosePrice(bar)
	// -Inf > 0 is false, falls through to Close
	if got != 100.0 {
		t.Errorf("eodClosePrice with -Inf AdjClose = %v, want 100.0", got)
	}
}

func TestEodClosePrice_Inf_Close(t *testing.T) {
	bar := models.EODBar{Close: math.Inf(1), AdjClose: 50.0}
	got := eodClosePrice(bar)
	// AdjClose > 0 is true, Close > 0 is true (Inf > 0),
	// ratio = 50.0 / Inf = 0, which is < 0.5, so returns Close = +Inf.
	if math.IsInf(got, 1) {
		t.Log("WARNING: eodClosePrice returns +Inf when Close is +Inf. " +
			"The ratio check (AdjClose/Close = 0 < 0.5) falls back to Close, " +
			"which is itself +Inf. No guard on Close being finite.")
	}
}

func TestEodClosePrice_NegativeClose(t *testing.T) {
	bar := models.EODBar{Close: -50.0, AdjClose: 10.0}
	got := eodClosePrice(bar)
	// AdjClose > 0 is true, Close > 0 is false (-50 > 0 is false),
	// so ratio check is skipped. Returns AdjClose.
	if got != 10.0 {
		t.Errorf("eodClosePrice with negative Close = %v, want 10.0 (AdjClose)", got)
	}
}

func TestEodClosePrice_NegativeClose_NegativeAdjClose(t *testing.T) {
	bar := models.EODBar{Close: -50.0, AdjClose: -10.0}
	got := eodClosePrice(bar)
	// AdjClose > 0 is false, falls through to return Close = -50.
	// This propagates a negative price to the portfolio.
	if got != -50.0 {
		t.Errorf("eodClosePrice with both negative = %v, want -50.0", got)
	}
	if got < 0 {
		t.Log("WARNING: eodClosePrice returns negative price when both Close and AdjClose " +
			"are negative. This would produce negative MarketValue in holdings.")
	}
}

func TestEodClosePrice_MaxFloat64_AdjClose(t *testing.T) {
	bar := models.EODBar{Close: 100.0, AdjClose: math.MaxFloat64}
	got := eodClosePrice(bar)
	// MaxFloat64 > 0 is true, not Inf, not NaN.
	// ratio = MaxFloat64 / 100.0 = very large > 2.0, so returns Close.
	if got != 100.0 {
		t.Errorf("eodClosePrice with MaxFloat64 AdjClose = %v, want 100.0", got)
	}
}

func TestEodClosePrice_SmallestPositive_AdjClose(t *testing.T) {
	bar := models.EODBar{Close: 100.0, AdjClose: math.SmallestNonzeroFloat64}
	got := eodClosePrice(bar)
	// SmallestNonzeroFloat64 > 0 is true, not Inf, not NaN.
	// ratio = tiny / 100.0 ~ 0, which is < 0.5, so returns Close.
	if got != 100.0 {
		t.Errorf("eodClosePrice with SmallestNonzeroFloat64 AdjClose = %v, want 100.0", got)
	}
}

func TestEodClosePrice_RatioExactly0_5_Boundary(t *testing.T) {
	// Ratio = 0.5 exactly: bar.Close = 100, bar.AdjClose = 50
	// ratio = 50/100 = 0.5; check is `ratio < 0.5`, so 0.5 < 0.5 is false → returns AdjClose
	bar := models.EODBar{Close: 100.0, AdjClose: 50.0}
	got := eodClosePrice(bar)
	if got != 50.0 {
		t.Errorf("eodClosePrice at ratio=0.5 = %v, want 50.0 (boundary inclusive)", got)
	}
}

func TestEodClosePrice_RatioExactly2_0_Boundary(t *testing.T) {
	// Ratio = 2.0 exactly: bar.Close = 100, bar.AdjClose = 200
	// ratio = 200/100 = 2.0; check is `ratio > 2.0`, so 2.0 > 2.0 is false → returns AdjClose
	bar := models.EODBar{Close: 100.0, AdjClose: 200.0}
	got := eodClosePrice(bar)
	if got != 200.0 {
		t.Errorf("eodClosePrice at ratio=2.0 = %v, want 200.0 (boundary inclusive)", got)
	}
}

func TestEodClosePrice_Close_Subnormal(t *testing.T) {
	// Very small positive Close that could cause ratio overflow
	bar := models.EODBar{Close: math.SmallestNonzeroFloat64, AdjClose: 100.0}
	got := eodClosePrice(bar)
	// ratio = 100.0 / SmallestNonzeroFloat64 = +Inf (overflow)
	// +Inf > 2.0 is true, so returns Close = SmallestNonzeroFloat64
	if got != math.SmallestNonzeroFloat64 {
		t.Errorf("eodClosePrice with subnormal Close = %v, want SmallestNonzeroFloat64", got)
	}
}

func TestEodClosePrice_Division_ProducesInf(t *testing.T) {
	// AdjClose / Close can produce +Inf when Close is very small
	bar := models.EODBar{Close: 1e-308, AdjClose: 1e+308}
	got := eodClosePrice(bar)
	// ratio = 1e308 / 1e-308 = +Inf, which is > 2.0, so returns Close
	if got != 1e-308 {
		t.Errorf("eodClosePrice with ratio overflow = %v, want 1e-308", got)
	}
}
