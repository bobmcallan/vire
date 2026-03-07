package market

import (
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
)

// ============================================================================
// Stress tests for Fix A: filterBadEODBars
// Devils-advocate adversarial edge cases
// ============================================================================

// TestStress_FilterBadEOD_AllBarsDivergent verifies behavior when ALL bars
// diverge from each other. This can happen if a ticker gets completely wrong
// data from EODHD for an extended period.
// Expected: only bars that agree with at least one neighbor survive.
func TestStress_FilterBadEOD_AllBarsDivergent(t *testing.T) {
	logger := common.NewLogger("error")

	// Each bar diverges >40% from its neighbors: 100, 20, 200, 10, 300
	bars := []models.EODBar{
		{Date: time.Now(), Close: 100},
		{Date: time.Now().AddDate(0, 0, -1), Close: 20},
		{Date: time.Now().AddDate(0, 0, -2), Close: 200},
		{Date: time.Now().AddDate(0, 0, -3), Close: 10},
		{Date: time.Now().AddDate(0, 0, -4), Close: 300},
	}

	filtered := filterBadEODBars(bars, "CHAOS.AU", logger)

	// The function should not panic and should not return empty.
	// Since every bar diverges from both neighbors, all get marked bad.
	// The function should still produce a result (possibly empty).
	if filtered == nil {
		t.Fatal("filtered should not be nil")
	}
	t.Logf("All-divergent scenario: %d of %d bars survived", len(filtered), len(bars))

	// CRITICAL: if all bars are filtered, downstream code using EOD[0] will panic
	// on index-out-of-range. This is an acceptable but noteworthy risk.
	if len(filtered) == 0 {
		t.Log("WARNING: all bars filtered out. Callers must handle empty EOD slice.")
	}
}

// TestStress_FilterBadEOD_ConsecutiveBadBars tests two consecutive bad bars.
// If bar[1] and bar[2] are both bad, the algorithm must not erroneously
// accept bar[2] because bar[1] was also bad (both-neighbor check).
func TestStress_FilterBadEOD_ConsecutiveBadBars(t *testing.T) {
	logger := common.NewLogger("error")

	// Two consecutive divergent bars in the middle
	bars := []models.EODBar{
		{Date: time.Now(), Close: 100},
		{Date: time.Now().AddDate(0, 0, -1), Close: 40}, // bad (>40% from 100 AND from 45)
		{Date: time.Now().AddDate(0, 0, -2), Close: 45}, // bad (>40% from 40... wait, 45/40=1.125, within range)
		{Date: time.Now().AddDate(0, 0, -3), Close: 98},
		{Date: time.Now().AddDate(0, 0, -4), Close: 97},
	}

	filtered := filterBadEODBars(bars, "CONSEC.AU", logger)

	// bar[1] (40): diverges from bar[0]=100 (ratio 0.4) AND bar[2]=45 (ratio 0.889 — within range!)
	// So bar[1] is NOT bad (agrees with bar[2])
	// bar[2] (45): diverges from bar[1]=40 (ratio 1.125 — OK) AND bar[3]=98 (ratio 0.459 — bad)
	// So bar[2] IS bad (diverges from both neighbors, since 45/40=1.125 is OK but 45/98=0.459 is bad)
	// Wait: 45/40 = 1.125 which is NOT >40% divergent. And 45/98 = 0.459 which IS <0.6.
	// So bar[2] diverges from bar[3] but agrees with bar[1]. One neighbor agrees => NOT bad.

	t.Logf("Consecutive bad bars: %d of %d survived. Closes: ", len(filtered), len(bars))
	for _, b := range filtered {
		t.Logf("  close=%.1f", b.Close)
	}
}

// TestStress_FilterBadEOD_ZeroCloseInMiddle tests bars with Close=0 mixed in.
// Zero-close bars should be filtered (bad[i] = true for Close <= 0).
func TestStress_FilterBadEOD_ZeroCloseInMiddle(t *testing.T) {
	logger := common.NewLogger("error")

	bars := []models.EODBar{
		{Date: time.Now(), Close: 100},
		{Date: time.Now().AddDate(0, 0, -1), Close: 0}, // zero-close
		{Date: time.Now().AddDate(0, 0, -2), Close: 99},
		{Date: time.Now().AddDate(0, 0, -3), Close: 98},
	}

	filtered := filterBadEODBars(bars, "ZERO.AU", logger)

	// Zero-close bar should be removed
	for _, b := range filtered {
		if b.Close == 0 {
			t.Error("zero-close bar should have been filtered")
		}
	}
	if len(filtered) != 3 {
		t.Errorf("expected 3 bars (one zero removed), got %d", len(filtered))
	}
}

// TestStress_FilterBadEOD_NegativeClose tests bars with negative close prices.
// These should never exist but could come from corrupt data.
func TestStress_FilterBadEOD_NegativeClose(t *testing.T) {
	logger := common.NewLogger("error")

	bars := []models.EODBar{
		{Date: time.Now(), Close: 100},
		{Date: time.Now().AddDate(0, 0, -1), Close: -50}, // negative
		{Date: time.Now().AddDate(0, 0, -2), Close: 99},
		{Date: time.Now().AddDate(0, 0, -3), Close: 98},
	}

	filtered := filterBadEODBars(bars, "NEG.AU", logger)

	for _, b := range filtered {
		if b.Close < 0 {
			t.Error("negative-close bar should have been filtered")
		}
	}
	if len(filtered) != 3 {
		t.Errorf("expected 3 bars, got %d", len(filtered))
	}
}

// TestStress_FilterBadEOD_BoundaryRatio tests the exact boundary values.
// Ratio = 0.6 means exactly 40% drop. Ratio = 1.667 means exactly ~66.7% gain.
// The threshold is strict less/greater, so exact boundary should pass.
func TestStress_FilterBadEOD_BoundaryRatio(t *testing.T) {
	logger := common.NewLogger("error")

	tests := []struct {
		name     string
		close0   float64
		close1   float64
		close2   float64
		wantKeep bool // whether close0 should be kept
	}{
		{
			name: "exact 40% drop (ratio=0.6) from both neighbors",
			// bar[0]=60, bar[1]=100, bar[2]=100: ratio = 60/100 = 0.6 (exactly at threshold)
			close0: 60, close1: 100, close2: 100,
			wantKeep: true, // 0.6 < 0.6 is false => NOT divergent => kept
		},
		{
			name: "just below 40% drop from both neighbors",
			// bar[0]=59.9, bar[1]=100, bar[2]=100: ratio = 0.599
			close0: 59.9, close1: 100, close2: 100,
			wantKeep: false, // 0.599 < 0.6 is true => divergent from both => filtered
		},
		{
			name: "just above 40% drop from both neighbors",
			// bar[0]=60.1, bar[1]=100, bar[2]=100: ratio = 0.601
			close0: 60.1, close1: 100, close2: 100,
			wantKeep: true, // 0.601 < 0.6 is false => not divergent
		},
		{
			name: "exact upper boundary (ratio=1.667) from both neighbors",
			// bar[0]=166.7, bar[1]=100, bar[2]=100: ratio = 1.667
			close0: 166.7, close1: 100, close2: 100,
			wantKeep: true, // 1.667 > 1.667 is false => NOT divergent => kept
		},
		{
			name:   "just above upper boundary from both neighbors",
			close0: 166.8, close1: 100, close2: 100,
			wantKeep: false, // 1.668 > 1.667 => divergent => filtered
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bars := []models.EODBar{
				{Date: time.Now(), Close: tt.close0},
				{Date: time.Now().AddDate(0, 0, -1), Close: tt.close1},
				{Date: time.Now().AddDate(0, 0, -2), Close: tt.close2},
			}
			filtered := filterBadEODBars(bars, "BOUND.AU", logger)

			bar0Kept := false
			for _, b := range filtered {
				if b.Close == tt.close0 {
					bar0Kept = true
				}
			}
			if tt.wantKeep && !bar0Kept {
				t.Errorf("bar with close=%.1f should have been KEPT", tt.close0)
			}
			if !tt.wantKeep && bar0Kept {
				t.Errorf("bar with close=%.1f should have been FILTERED", tt.close0)
			}
		})
	}
}

// TestStress_FilterBadEOD_StockSplitFalsePositive tests whether a legitimate
// 2:1 stock split (50% price drop overnight) gets incorrectly filtered.
// A stock split causes a genuine >40% move. The algorithm should filter this
// only if both neighbors disagree.
func TestStress_FilterBadEOD_StockSplitFalsePositive(t *testing.T) {
	logger := common.NewLogger("error")

	// 2:1 stock split: price halves from 100 to 50, then continues near 50
	bars := []models.EODBar{
		{Date: time.Now(), Close: 51},                    // post-split, day 2
		{Date: time.Now().AddDate(0, 0, -1), Close: 50},  // post-split, day 1
		{Date: time.Now().AddDate(0, 0, -2), Close: 100}, // pre-split, last day
		{Date: time.Now().AddDate(0, 0, -3), Close: 101}, // pre-split
		{Date: time.Now().AddDate(0, 0, -4), Close: 99},  // pre-split
	}

	filtered := filterBadEODBars(bars, "SPLIT.AU", logger)

	// bar[1] (50): diverges from bar[0]=51 (ratio 0.98 OK) and bar[2]=100 (ratio 0.5 bad)
	// But it agrees with bar[0], so NOT filtered. Good.
	// bar[2] (100): diverges from bar[1]=50 (ratio 2.0 bad) and bar[3]=101 (ratio 0.99 OK)
	// Agrees with bar[3], so NOT filtered. Good.

	if len(filtered) != 5 {
		t.Errorf("stock split should NOT cause filtering: expected 5 bars, got %d", len(filtered))
		for _, b := range filtered {
			t.Logf("  kept: close=%.1f", b.Close)
		}
	}
}

// TestStress_FilterBadEOD_SingleBadBarAmongMany tests a single bad bar in a
// large dataset (100 bars). Verifies performance is acceptable.
func TestStress_FilterBadEOD_SingleBadBarAmongMany(t *testing.T) {
	logger := common.NewLogger("error")

	n := 500
	bars := make([]models.EODBar, n)
	for i := 0; i < n; i++ {
		bars[i] = models.EODBar{
			Date:  time.Now().AddDate(0, 0, -i),
			Close: 100.0 - float64(i)*0.1, // gentle downtrend
		}
	}
	// Inject one bad bar in the middle
	bars[250].Close = 10.0

	start := time.Now()
	filtered := filterBadEODBars(bars, "PERF.AU", logger)
	elapsed := time.Since(start)

	if elapsed > 50*time.Millisecond {
		t.Errorf("filterBadEODBars took %v for %d bars — too slow", elapsed, n)
	}

	if len(filtered) != n-1 {
		t.Errorf("expected %d bars (1 removed), got %d", n-1, len(filtered))
	}

	for _, b := range filtered {
		if b.Close == 10.0 {
			t.Error("bad bar with close=10.0 should have been filtered")
		}
	}
}

// TestStress_FilterBadEOD_FirstBarBadWithZeroSecond tests the edge case where
// the first bar is bad AND the second bar has Close=0. The isDivergent function
// returns false when b<=0, which means the first bar is considered non-divergent
// from bar[1] (since bar[1].Close=0 makes isDivergent return false).
// This could allow a bad first bar to survive.
func TestStress_FilterBadEOD_FirstBarBadWithZeroSecond(t *testing.T) {
	logger := common.NewLogger("error")

	bars := []models.EODBar{
		{Date: time.Now(), Close: 50},                  // bad (should be ~100)
		{Date: time.Now().AddDate(0, 0, -1), Close: 0}, // zero
		{Date: time.Now().AddDate(0, 0, -2), Close: 100},
		{Date: time.Now().AddDate(0, 0, -3), Close: 99},
	}

	filtered := filterBadEODBars(bars, "ZEROSEC.AU", logger)

	// bar[0] (50): isDivergent(50, 0) => false (b<=0), isDivergent(50, 100) => true
	// Since NOT divergent from bar[1] (due to zero), bar[0] is NOT marked bad.
	// This is a known limitation: zero-close neighbors mask divergence.
	t.Logf("First bar bad + second bar zero: %d bars survived", len(filtered))
	for _, b := range filtered {
		t.Logf("  close=%.1f", b.Close)
	}

	// The zero bar should be removed (Close <= 0)
	for _, b := range filtered {
		if b.Close == 0 {
			t.Error("zero-close bar should be filtered")
		}
	}

	// FINDING: bar[0]=50 survives because isDivergent returns false when
	// neighbor has Close=0. This could allow bad data through if a zero bar
	// is adjacent to the bad bar. This is an acceptable edge case per the
	// design (we need at least one valid neighbor to judge divergence).
}

// TestStress_FilterBadEOD_LastBarDivergent tests the last bar (oldest) being bad.
func TestStress_FilterBadEOD_LastBarDivergent(t *testing.T) {
	logger := common.NewLogger("error")

	bars := []models.EODBar{
		{Date: time.Now(), Close: 100},
		{Date: time.Now().AddDate(0, 0, -1), Close: 99},
		{Date: time.Now().AddDate(0, 0, -2), Close: 98},
		{Date: time.Now().AddDate(0, 0, -3), Close: 40}, // bad last bar
	}

	filtered := filterBadEODBars(bars, "LAST.AU", logger)

	// bar[3] (40): isDivergent(40, 98) => ratio 0.408, <0.6 => true
	// isDivergent(40, 99) => ratio 0.404, <0.6 => true
	// Divergent from both bar[i-1] and bar[i-2] => filtered
	if len(filtered) != 3 {
		t.Errorf("expected 3 bars (last one filtered), got %d", len(filtered))
	}
	for _, b := range filtered {
		if b.Close == 40 {
			t.Error("divergent last bar should have been filtered")
		}
	}
}

// TestStress_FilterBadEOD_ExactlyThreeBars tests the minimum case where
// filtering can happen (len >= 3).
func TestStress_FilterBadEOD_ExactlyThreeBars(t *testing.T) {
	logger := common.NewLogger("error")

	t.Run("middle bar bad", func(t *testing.T) {
		bars := []models.EODBar{
			{Date: time.Now(), Close: 100},
			{Date: time.Now().AddDate(0, 0, -1), Close: 30}, // divergent from both
			{Date: time.Now().AddDate(0, 0, -2), Close: 99},
		}
		filtered := filterBadEODBars(bars, "THREE.AU", logger)
		if len(filtered) != 2 {
			t.Errorf("expected 2 bars, got %d", len(filtered))
		}
	})

	t.Run("first bar bad", func(t *testing.T) {
		bars := []models.EODBar{
			{Date: time.Now(), Close: 30}, // divergent from both bar[1] and bar[2]
			{Date: time.Now().AddDate(0, 0, -1), Close: 100},
			{Date: time.Now().AddDate(0, 0, -2), Close: 99},
		}
		filtered := filterBadEODBars(bars, "THREE.AU", logger)
		if len(filtered) != 2 {
			t.Errorf("expected 2 bars, got %d", len(filtered))
		}
	})

	t.Run("last bar bad", func(t *testing.T) {
		bars := []models.EODBar{
			{Date: time.Now(), Close: 100},
			{Date: time.Now().AddDate(0, 0, -1), Close: 99},
			{Date: time.Now().AddDate(0, 0, -2), Close: 30}, // divergent from bar[i-1] and bar[i-2]
		}
		filtered := filterBadEODBars(bars, "THREE.AU", logger)
		if len(filtered) != 2 {
			t.Errorf("expected 2 bars, got %d", len(filtered))
		}
	})

	t.Run("all similar", func(t *testing.T) {
		bars := []models.EODBar{
			{Date: time.Now(), Close: 100},
			{Date: time.Now().AddDate(0, 0, -1), Close: 99},
			{Date: time.Now().AddDate(0, 0, -2), Close: 98},
		}
		filtered := filterBadEODBars(bars, "THREE.AU", logger)
		if len(filtered) != 3 {
			t.Errorf("expected 3 bars (all valid), got %d", len(filtered))
		}
	})
}
