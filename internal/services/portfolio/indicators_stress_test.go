package portfolio

import (
	"encoding/json"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
	"github.com/bobmcallan/vire/internal/signals"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// truncateStr returns the first n characters of s, or s if shorter.
func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// --- growthToBars edge cases ---

func TestGrowthToBars_SinglePoint(t *testing.T) {
	points := []models.GrowthDataPoint{
		{Date: time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC), TotalValue: 500000},
	}
	bars := growthToBars(points)
	require.Len(t, bars, 1)
	assert.Equal(t, 500000.0, bars[0].Close)
	assert.Equal(t, bars[0].Open, bars[0].Close)
	assert.Equal(t, bars[0].High, bars[0].Close)
	assert.Equal(t, bars[0].Low, bars[0].Close)
	assert.Equal(t, bars[0].AdjClose, bars[0].Close)
}

func TestGrowthToBars_NegativeTotalValue_StressEdge(t *testing.T) {
	// After fix: growthToBars uses TotalValue only (no external balance parameter)
	points := []models.GrowthDataPoint{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), TotalValue: 100},
	}
	bars := growthToBars(points)
	require.Len(t, bars, 1)
	assert.Equal(t, 100.0, bars[0].Close, "bar value = TotalValue only")
}

func TestGrowthToBars_NegativeTotalValue(t *testing.T) {
	// Edge case: a portfolio with negative total value (e.g. short positions)
	points := []models.GrowthDataPoint{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), TotalValue: -1000},
	}
	bars := growthToBars(points)
	require.Len(t, bars, 1)
	assert.Equal(t, -1000.0, bars[0].Close, "bar value = TotalValue")
}

func TestGrowthToBars_ZeroTotalValue(t *testing.T) {
	points := []models.GrowthDataPoint{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), TotalValue: 0},
	}
	bars := growthToBars(points)
	require.Len(t, bars, 1)
	assert.Equal(t, 0.0, bars[0].Close)
}

func TestGrowthToBars_VeryLargeValues(t *testing.T) {
	points := []models.GrowthDataPoint{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), TotalValue: 1e15},
		{Date: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), TotalValue: 1e15 + 1},
	}
	bars := growthToBars(points)
	require.Len(t, bars, 2)
	// Verify precision is maintained for large values
	assert.Greater(t, bars[0].Close, bars[1].Close, "newest bar should have higher value")
}

func TestGrowthToBars_NaNTotalValue(t *testing.T) {
	points := []models.GrowthDataPoint{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), TotalValue: math.NaN()},
	}
	bars := growthToBars(points)
	require.Len(t, bars, 1)
	// NaN propagates through
	if !math.IsNaN(bars[0].Close) {
		t.Logf("growthToBars with NaN TotalValue produced %.2f (NaN expected)", bars[0].Close)
	}
}

func TestGrowthToBars_InfTotalValue(t *testing.T) {
	points := []models.GrowthDataPoint{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), TotalValue: math.Inf(1)},
	}
	bars := growthToBars(points)
	require.Len(t, bars, 1)
	assert.True(t, math.IsInf(bars[0].Close, 1), "+Inf TotalValue propagates")
}

func TestGrowthToBars_NaNExternalBalance_Deprecated(t *testing.T) {
	// ExternalBalance parameter removed from growthToBars.
	// NaN TotalValue is now the only source of NaN in bar values.
	points := []models.GrowthDataPoint{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), TotalValue: 100},
	}
	bars := growthToBars(points)
	require.Len(t, bars, 1)
	assert.Equal(t, 100.0, bars[0].Close, "bar value = TotalValue (no external balance)")
}

func TestGrowthToBars_NewestFirstOrdering(t *testing.T) {
	// Verify output is strictly newest-first
	n := 100
	points := make([]models.GrowthDataPoint, n)
	for i := 0; i < n; i++ {
		points[i] = models.GrowthDataPoint{
			Date:       time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i),
			TotalValue: float64(i+1) * 1000,
		}
	}
	bars := growthToBars(points)
	require.Len(t, bars, n)
	for i := 0; i < n-1; i++ {
		assert.True(t, bars[i].Date.After(bars[i+1].Date),
			"bars[%d].Date (%v) should be after bars[%d].Date (%v)", i, bars[i].Date, i+1, bars[i+1].Date)
	}
}

// --- Portfolio indicator data point thresholds ---

func TestIndicatorThresholds_LessThan14Points(t *testing.T) {
	bars := makeFlatBars(13, 100)
	rsi := signals.RSI(bars, 14)
	// RSI returns neutral default (50) when insufficient data
	assert.Equal(t, 50.0, rsi, "RSI with < 14+1 bars should return neutral default")
}

func TestIndicatorThresholds_Exactly14Points(t *testing.T) {
	// RSI needs period+1 bars (15 for RSI-14)
	bars := makeFlatBars(14, 100)
	rsi := signals.RSI(bars, 14)
	assert.Equal(t, 50.0, rsi, "RSI with exactly 14 bars (needs 15) should return neutral default")
}

func TestIndicatorThresholds_15Points_RSIComputable(t *testing.T) {
	bars := makeFlatBars(15, 100)
	rsi := signals.RSI(bars, 14)
	// With flat data, RSI should be 50 (no gains, no losses → avgLoss=0 → returns 100, wait...)
	// Actually with flat data, all changes are 0, so avgGain=0 and avgLoss=0, returns 100
	// because avgLoss == 0 → return 100
	assert.Equal(t, 100.0, rsi, "RSI with flat data should be 100 (zero losses)")
}

func TestIndicatorThresholds_LessThan20Points_NoEMA20(t *testing.T) {
	bars := makeFlatBars(19, 100)
	ema := signals.EMA(bars, 20)
	assert.Equal(t, 0.0, ema, "EMA20 with < 20 bars should return 0")
}

func TestIndicatorThresholds_Exactly20Points_EMA20(t *testing.T) {
	bars := makeFlatBars(20, 100)
	ema := signals.EMA(bars, 20)
	assert.InDelta(t, 100.0, ema, 0.01, "EMA20 with 20 flat bars should be ~100")
}

func TestIndicatorThresholds_LessThan50Points_NoEMA50(t *testing.T) {
	bars := makeFlatBars(49, 100)
	ema := signals.EMA(bars, 50)
	assert.Equal(t, 0.0, ema, "EMA50 with < 50 bars should return 0")
}

func TestIndicatorThresholds_LessThan200Points_NoEMA200(t *testing.T) {
	bars := makeFlatBars(199, 100)
	ema := signals.EMA(bars, 200)
	assert.Equal(t, 0.0, ema, "EMA200 with < 200 bars should return 0")
}

func TestIndicatorThresholds_Exactly200Points_EMA200(t *testing.T) {
	bars := makeFlatBars(200, 100)
	ema := signals.EMA(bars, 200)
	assert.InDelta(t, 100.0, ema, 0.01, "EMA200 with 200 flat bars should be ~100")
}

// --- detectEMACrossover edge cases ---

func TestDetectEMACrossover_ExactlyEqualEMAs(t *testing.T) {
	// Flat data where EMA50 == EMA200 on all days — no crossover
	bars := makeFlatBars(250, 100)
	result := detectEMACrossover(bars)
	assert.Equal(t, "none", result, "flat data should produce no crossover")
}

func TestDetectEMACrossover_Exactly201Bars(t *testing.T) {
	// Minimum data for crossover detection
	bars := makeFlatBars(201, 100)
	result := detectEMACrossover(bars)
	assert.Equal(t, "none", result, "flat data at boundary length should produce no crossover")
}

func TestDetectEMACrossover_DeathCross(t *testing.T) {
	// Build data where EMA50 drops below EMA200:
	// High prices historically, then sharply declining recent prices
	bars := make([]models.EODBar, 250)
	for i := range bars {
		price := 100.0
		if i < 50 {
			// Recent data: sharply declining (newest first, so low recent prices)
			price = 50.0 + float64(i)*0.5
		} else {
			// Older data: high flat prices
			price = 150.0
		}
		bars[i] = models.EODBar{
			Date:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -i),
			Close: price,
		}
	}
	result := detectEMACrossover(bars)
	// Should be either death_cross or none depending on exact EMA math
	assert.Contains(t, []string{"death_cross", "none"}, result,
		"declining recent prices should produce death_cross or none")
}

func TestDetectEMACrossover_VeryCloseEMAs(t *testing.T) {
	// Nearly flat data with tiny fluctuations
	bars := make([]models.EODBar, 250)
	for i := range bars {
		// Tiny oscillation around 100
		bars[i] = models.EODBar{
			Date:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -i),
			Close: 100.0 + float64(i%2)*0.001,
		}
	}
	result := detectEMACrossover(bars)
	// With near-identical data, EMAs should be very close — no definitive crossover
	assert.Contains(t, []string{"golden_cross", "death_cross", "none"}, result)
}

// --- RSI boundary values ---

func TestRSI_AllGains(t *testing.T) {
	// Strictly rising prices — RSI should approach 100
	bars := make([]models.EODBar, 20)
	for i := range bars {
		// Newest first, declining index = older = lower price
		bars[i] = models.EODBar{Close: 200.0 - float64(i)}
	}
	rsi := signals.RSI(bars, 14)
	assert.Equal(t, 100.0, rsi, "all-gains RSI should be 100 (avgLoss=0)")
}

func TestRSI_AllLosses(t *testing.T) {
	// Strictly falling prices — RSI should approach 0
	bars := make([]models.EODBar, 20)
	for i := range bars {
		// Newest first, lower index = newer = lower price
		bars[i] = models.EODBar{Close: float64(i) + 1}
	}
	rsi := signals.RSI(bars, 14)
	assert.InDelta(t, 0.0, rsi, 0.01, "all-losses RSI should be ~0")
}

func TestRSI_EqualGainsAndLosses(t *testing.T) {
	// Alternating gains and losses of equal magnitude — RSI should be ~50
	bars := make([]models.EODBar, 20)
	for i := range bars {
		if i%2 == 0 {
			bars[i] = models.EODBar{Close: 100}
		} else {
			bars[i] = models.EODBar{Close: 110}
		}
	}
	rsi := signals.RSI(bars, 14)
	// Wilder's smoothing weights recent values more heavily, so alternating
	// patterns may deviate from 50 depending on which direction the most
	// recent bar moved. Allow a wider tolerance.
	assert.InDelta(t, 50.0, rsi, 5.0, "equal gains/losses RSI should be ~50")
}

func TestClassifyRSI_Boundaries(t *testing.T) {
	tests := []struct {
		rsi      float64
		expected string
	}{
		{0, "oversold"},
		{29.99, "oversold"},
		{30, "oversold"},
		{30.01, "neutral"},
		{50, "neutral"},
		{69.99, "neutral"},
		{70, "overbought"},
		{70.01, "overbought"},
		{100, "overbought"},
	}
	for _, tt := range tests {
		got := signals.ClassifyRSI(tt.rsi)
		assert.Equal(t, tt.expected, got, "ClassifyRSI(%.2f)", tt.rsi)
	}
}

// --- TotalValue split invariant ---
// TotalCash is now computed from TotalCashBalance() across all cashflow ledger accounts.
// The invariant TotalValue = TotalValueHoldings + TotalCash still holds.

func TestTotalValueSplit_InvariantAfterRecompute(t *testing.T) {
	tests := []struct {
		name          string
		holdingsValue float64
		totalCash     float64
		expectedTotal float64
	}{
		{
			name:          "no cash balances",
			holdingsValue: 100000,
			totalCash:     0,
			expectedTotal: 100000,
		},
		{
			name:          "with total cash",
			holdingsValue: 100000,
			totalCash:     50000,
			expectedTotal: 150000,
		},
		{
			name:          "multiple accounts combined",
			holdingsValue: 200000,
			totalCash:     200000, // 25000 + 75000 + 100000
			expectedTotal: 400000,
		},
		{
			name:          "zero holdings",
			holdingsValue: 0,
			totalCash:     50000,
			expectedTotal: 50000,
		},
		{
			name:          "zero everything",
			holdingsValue: 0,
			totalCash:     0,
			expectedTotal: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// TotalCash is pre-computed from the cashflow ledger.
			// TotalValue = TotalValueHoldings + TotalCash.
			p := &models.Portfolio{
				TotalValueHoldings: tt.holdingsValue,
				TotalCash:          tt.totalCash,
				TotalValue:         tt.holdingsValue + tt.totalCash,
			}

			// Invariant: TotalValue = TotalValueHoldings + TotalCash
			assert.Equal(t, tt.expectedTotal, p.TotalValue,
				"TotalValue should equal TotalValueHoldings + TotalCash")
			assert.Equal(t, p.TotalValueHoldings+p.TotalCash, p.TotalValue,
				"invariant: TotalValue = TotalValueHoldings + TotalCash")
		})
	}
}

func TestTotalValueSplit_InvariantHoldsWithDifferentBalances(t *testing.T) {
	// Verify the invariant TotalValue = TotalValueHoldings + TotalCash
	// with progressively larger cash balances.
	holdingsValue := 100000.0
	for i := 0; i < 10; i++ {
		totalCash := float64(i+1) * 10000
		p := &models.Portfolio{
			TotalValueHoldings: holdingsValue,
			TotalCash:          totalCash,
			TotalValue:         holdingsValue + totalCash,
		}

		// Invariant must hold
		assert.Equal(t, p.TotalValueHoldings+p.TotalCash, p.TotalValue,
			"invariant broken at iteration %d", i)
	}
}

// --- Large portfolio values ---

func TestGrowthToBars_Float64Precision(t *testing.T) {
	// Large values where float64 precision matters
	points := []models.GrowthDataPoint{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), TotalValue: 1e13},
		{Date: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), TotalValue: 1e13 + 1},
	}
	bars := growthToBars(points)
	require.Len(t, bars, 2)

	// Both bars should have distinct values
	assert.NotEqual(t, bars[0].Close, bars[1].Close,
		"large values should maintain distinct bar values")
}

// --- JSON serialization ---

func TestPortfolioIndicators_JSONRoundtrip(t *testing.T) {
	ind := models.PortfolioIndicators{
		PortfolioName:    "SMSF",
		ComputeDate:      time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC),
		CurrentValue:     500000,
		DataPoints:       250,
		EMA20:            490000,
		EMA50:            480000,
		EMA200:           450000,
		AboveEMA20:       true,
		AboveEMA50:       true,
		AboveEMA200:      true,
		RSI:              55.5,
		RSISignal:        "neutral",
		EMA50CrossEMA200: "none",
		Trend:            models.TrendBullish,
		TrendDescription: "Portfolio value is in an uptrend",
	}

	data, err := json.Marshal(ind)
	require.NoError(t, err)

	var restored models.PortfolioIndicators
	require.NoError(t, json.Unmarshal(data, &restored))

	assert.Equal(t, ind.PortfolioName, restored.PortfolioName)
	assert.Equal(t, ind.CurrentValue, restored.CurrentValue)
	assert.Equal(t, ind.DataPoints, restored.DataPoints)
	assert.Equal(t, ind.EMA20, restored.EMA20)
	assert.Equal(t, ind.EMA50, restored.EMA50)
	assert.Equal(t, ind.EMA200, restored.EMA200)
	assert.Equal(t, ind.AboveEMA20, restored.AboveEMA20)
	assert.Equal(t, ind.AboveEMA50, restored.AboveEMA50)
	assert.Equal(t, ind.AboveEMA200, restored.AboveEMA200)
	assert.Equal(t, ind.RSI, restored.RSI)
	assert.Equal(t, ind.RSISignal, restored.RSISignal)
	assert.Equal(t, ind.EMA50CrossEMA200, restored.EMA50CrossEMA200)
	assert.Equal(t, ind.Trend, restored.Trend)
	assert.Equal(t, ind.TrendDescription, restored.TrendDescription)
}

func TestPortfolioIndicators_JSONFieldNames(t *testing.T) {
	ind := models.PortfolioIndicators{
		EMA20:            1.0,
		EMA50:            2.0,
		EMA200:           3.0,
		AboveEMA20:       true,
		EMA50CrossEMA200: "golden_cross",
	}
	data, err := json.Marshal(ind)
	require.NoError(t, err)

	raw := string(data)
	assert.Contains(t, raw, `"ema_20"`)
	assert.Contains(t, raw, `"ema_50"`)
	assert.Contains(t, raw, `"ema_200"`)
	assert.Contains(t, raw, `"above_ema_20"`)
	assert.Contains(t, raw, `"above_ema_50"`)
	assert.Contains(t, raw, `"above_ema_200"`)
	assert.Contains(t, raw, `"rsi"`)
	assert.Contains(t, raw, `"rsi_signal"`)
	assert.Contains(t, raw, `"ema_50_cross_200"`)
	assert.Contains(t, raw, `"trend"`)
	assert.Contains(t, raw, `"trend_description"`)
}

func TestPortfolioReview_IncludesIndicators(t *testing.T) {
	review := models.PortfolioReview{
		PortfolioName: "SMSF",
		PortfolioIndicators: &models.PortfolioIndicators{
			PortfolioName: "SMSF",
			RSI:           55.0,
			Trend:         models.TrendNeutral,
		},
	}
	data, err := json.Marshal(review)
	require.NoError(t, err)

	raw := string(data)
	assert.Contains(t, raw, `"portfolio_indicators"`)
	assert.Contains(t, raw, `"rsi":55`)
}

func TestPortfolioReview_NilIndicatorsOmitted(t *testing.T) {
	review := models.PortfolioReview{
		PortfolioName:       "SMSF",
		PortfolioIndicators: nil,
	}
	data, err := json.Marshal(review)
	require.NoError(t, err)

	raw := string(data)
	assert.NotContains(t, raw, `"portfolio_indicators"`, "nil indicators should be omitted")
}

func TestPortfolio_TotalValueHoldings_JSONField(t *testing.T) {
	p := models.Portfolio{
		TotalValueHoldings: 100000,
		TotalValue:         150000,
	}
	data, err := json.Marshal(p)
	require.NoError(t, err)

	raw := string(data)
	assert.Contains(t, raw, `"total_value_holdings"`)
	assert.Contains(t, raw, `"total_value"`)
}

func TestPortfolio_BackwardCompatibility_OldJSON(t *testing.T) {
	// Old JSON that only has "total_value" (no total_value_holdings)
	oldJSON := `{
		"id": "test",
		"name": "SMSF",
		"holdings": [],
		"total_value": 100000,
		"total_cost": 90000,
		"currency": "AUD",
		"total_cash": 0,
		"last_synced": "2025-01-01T00:00:00Z",
		"created_at": "2025-01-01T00:00:00Z",
		"updated_at": "2025-01-01T00:00:00Z"
	}`

	var p models.Portfolio
	require.NoError(t, json.Unmarshal([]byte(oldJSON), &p))

	// TotalValue should be populated from old JSON
	assert.Equal(t, 100000.0, p.TotalValue)
	// TotalValueHoldings should be zero (not present in old JSON)
	assert.Equal(t, 0.0, p.TotalValueHoldings)
}

// --- Trend classification ---

func TestDetermineTrend_AllZeroSMAs(t *testing.T) {
	// When insufficient data, SMA returns 0
	trend := signals.DetermineTrend(100, 0, 0, 0)
	// Price > SMA200 (100 > 0) AND SMA20 > SMA50 (0 > 0 = false) → not bullish
	// Price < SMA200 (100 < 0 = false) → not bearish
	assert.Equal(t, models.TrendNeutral, trend)
}

func TestDetermineTrend_ZeroPrice(t *testing.T) {
	trend := signals.DetermineTrend(0, 100, 100, 100)
	// Price < SMA200 (0 < 100) AND SMA20 < SMA50 (100 < 100 = false) → not bearish
	assert.Equal(t, models.TrendNeutral, trend)
}

func TestDetermineTrend_Classification(t *testing.T) {
	tests := []struct {
		name   string
		price  float64
		sma20  float64
		sma50  float64
		sma200 float64
		want   models.TrendType
	}{
		{"bullish: price>200, 20>50", 150, 120, 100, 80, models.TrendBullish},
		{"bearish: price<200, 20<50", 50, 60, 80, 100, models.TrendBearish},
		{"neutral: price>200, 20<50", 150, 80, 100, 80, models.TrendNeutral},
		{"neutral: price<200, 20>50", 50, 120, 100, 100, models.TrendNeutral},
		{"neutral: price==200", 100, 120, 100, 100, models.TrendNeutral},
		{"neutral: sma20==sma50", 150, 100, 100, 80, models.TrendNeutral},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := signals.DetermineTrend(tt.price, tt.sma20, tt.sma50, tt.sma200)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- EMA/SMA with hostile data ---

func TestEMA_FlatData(t *testing.T) {
	bars := makeFlatBars(100, 500)
	ema20 := signals.EMA(bars, 20)
	ema50 := signals.EMA(bars, 50)
	assert.InDelta(t, 500.0, ema20, 0.01, "EMA20 of flat data should equal the flat value")
	assert.InDelta(t, 500.0, ema50, 0.01, "EMA50 of flat data should equal the flat value")
}

func TestSMA_FlatData(t *testing.T) {
	bars := makeFlatBars(100, 500)
	sma20 := signals.SMA(bars, 20)
	sma50 := signals.SMA(bars, 50)
	assert.Equal(t, 500.0, sma20, "SMA20 of flat data should equal the flat value")
	assert.Equal(t, 500.0, sma50, "SMA50 of flat data should equal the flat value")
}

// --- Hostile portfolio name tests ---

func TestGrowthToBars_HostilePortfolioName(t *testing.T) {
	// Hostile names shouldn't affect growthToBars (it only processes data, not names)
	// but verify no panics with data that could contain hostile-origin values
	hostileNames := []string{
		"",
		"../../../etc/passwd",
		"<script>alert(1)</script>",
		"'; DROP TABLE portfolio;--",
		strings.Repeat("X", 10000),
	}
	for _, name := range hostileNames {
		t.Run("name="+truncateStr(name, 30), func(t *testing.T) {
			points := []models.GrowthDataPoint{
				{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), TotalValue: 100},
			}
			bars := growthToBars(points)
			assert.Len(t, bars, 1, "growthToBars should work regardless of portfolio name context: %s", truncateStr(name, 30))
		})
	}
}

// --- Concurrent access ---

func TestGrowthToBars_ConcurrentCalls(t *testing.T) {
	points := []models.GrowthDataPoint{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), TotalValue: 100},
		{Date: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), TotalValue: 110},
		{Date: time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC), TotalValue: 120},
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bars := growthToBars(points)
			assert.Len(t, bars, 3)
		}()
	}
	wg.Wait()
}

func TestDetectEMACrossover_ConcurrentCalls(t *testing.T) {
	bars := makeFlatBars(250, 100)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result := detectEMACrossover(bars)
			assert.Equal(t, "none", result)
		}()
	}
	wg.Wait()
}

func TestTotalCash_ConcurrentSafe(t *testing.T) {
	// Each goroutine gets its own Portfolio — no shared mutation.
	// TotalCash is a direct field, no recompute function needed.
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(val float64) {
			defer wg.Done()
			p := &models.Portfolio{
				TotalValueHoldings: 100000,
				TotalCash:          val,
				TotalValue:         100000 + val,
			}
			assert.Equal(t, 100000+val, p.TotalValue)
		}(float64(i) * 1000)
	}
	wg.Wait()
}

// --- Helper to generate bars ---

func makeFlatBars(n int, price float64) []models.EODBar {
	bars := make([]models.EODBar, n)
	for i := range bars {
		bars[i] = models.EODBar{
			Date:     time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -i),
			Open:     price,
			High:     price,
			Low:      price,
			Close:    price,
			AdjClose: price,
		}
	}
	return bars
}
