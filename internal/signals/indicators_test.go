package signals

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/bobmcallan/vire/internal/models"
)

func TestSMA(t *testing.T) {
	tests := []struct {
		name     string
		bars     []models.EODBar
		period   int
		expected float64
	}{
		{
			name:     "simple 3-day SMA",
			bars:     generateBars([]float64{10, 20, 30}),
			period:   3,
			expected: 20.0,
		},
		{
			name:     "5-day SMA",
			bars:     generateBars([]float64{10, 20, 30, 40, 50}),
			period:   5,
			expected: 30.0,
		},
		{
			name:     "insufficient data",
			bars:     generateBars([]float64{10, 20}),
			period:   5,
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SMA(tt.bars, tt.period)
			assert.InDelta(t, tt.expected, result, 0.01)
		})
	}
}

func TestRSI(t *testing.T) {
	tests := []struct {
		name   string
		bars   []models.EODBar
		period int
		minRSI float64
		maxRSI float64
	}{
		{
			name:   "uptrend should have high RSI",
			bars:   generateTrendBars(50, 1.0, 20), // Strong uptrend
			period: 14,
			minRSI: 60,
			maxRSI: 100,
		},
		{
			name:   "downtrend should have low RSI",
			bars:   generateTrendBars(50, -1.0, 20), // Strong downtrend
			period: 14,
			minRSI: 0,
			maxRSI: 40,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RSI(tt.bars, tt.period)
			assert.GreaterOrEqual(t, result, tt.minRSI)
			assert.LessOrEqual(t, result, tt.maxRSI)
		})
	}
}

func TestClassifyRSI(t *testing.T) {
	tests := []struct {
		rsi      float64
		expected string
	}{
		{75, "overbought"},
		{70, "overbought"},
		{50, "neutral"},
		{30, "oversold"},
		{25, "oversold"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := ClassifyRSI(tt.rsi)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDetectCrossover(t *testing.T) {
	tests := []struct {
		name     string
		bars     []models.EODBar
		short    int
		long     int
		expected string
	}{
		{
			name:     "no crossover in flat market",
			bars:     generateBars([]float64{50, 50, 50, 50, 50, 50, 50, 50, 50, 50}),
			short:    3,
			long:     5,
			expected: "none",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectCrossover(tt.bars, tt.short, tt.long)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDetermineTrend(t *testing.T) {
	tests := []struct {
		name     string
		price    float64
		sma20    float64
		sma50    float64
		sma200   float64
		expected models.TrendType
	}{
		{
			name:     "bullish - price above all SMAs and SMA20 > SMA50",
			price:    110,
			sma20:    105,
			sma50:    100,
			sma200:   90,
			expected: models.TrendBullish,
		},
		{
			name:     "bearish - price below SMA200 and SMA20 < SMA50",
			price:    80,
			sma20:    85,
			sma50:    90,
			sma200:   100,
			expected: models.TrendBearish,
		},
		{
			name:     "neutral - mixed signals",
			price:    95,
			sma20:    90,
			sma50:    100,
			sma200:   90,
			expected: models.TrendNeutral,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetermineTrend(tt.price, tt.sma20, tt.sma50, tt.sma200)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestVolumeRatio(t *testing.T) {
	bars := make([]models.EODBar, 25)
	for i := 0; i < 25; i++ {
		bars[i] = models.EODBar{
			Close:  50,
			Volume: 1000000,
		}
	}
	// Make current volume 2x average
	bars[0].Volume = 2000000

	ratio := VolumeRatio(bars, 20)
	assert.InDelta(t, 2.0, ratio, 0.1)
}

func TestClassifyVolume(t *testing.T) {
	tests := []struct {
		ratio    float64
		expected string
	}{
		{2.5, "spike"},
		{2.0, "spike"},
		{1.0, "normal"},
		{0.5, "low"},
		{0.3, "low"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := ClassifyVolume(tt.ratio)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDistanceToSMA(t *testing.T) {
	tests := []struct {
		price    float64
		sma      float64
		expected float64
	}{
		{110, 100, 10.0},
		{90, 100, -10.0},
		{100, 100, 0.0},
		{50, 0, 0.0}, // Handle zero SMA
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := DistanceToSMA(tt.price, tt.sma)
			assert.InDelta(t, tt.expected, result, 0.01)
		})
	}
}

// Helper functions

func generateBars(closes []float64) []models.EODBar {
	bars := make([]models.EODBar, len(closes))
	for i, close := range closes {
		bars[i] = models.EODBar{
			Date:     time.Now().AddDate(0, 0, -i),
			Open:     close - 0.5,
			High:     close + 0.5,
			Low:      close - 0.5,
			Close:    close,
			AdjClose: close,
			Volume:   1000000,
		}
	}
	return bars
}

func generateTrendBars(startPrice, dailyChange float64, days int) []models.EODBar {
	bars := make([]models.EODBar, days)
	price := startPrice
	for i := 0; i < days; i++ {
		bars[i] = models.EODBar{
			Date:     time.Now().AddDate(0, 0, -i),
			Close:    price,
			AdjClose: price,
			Volume:   1000000,
		}
		price -= dailyChange // Going back in time
	}
	return bars
}
