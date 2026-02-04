// Package signals provides technical indicator calculations
package signals

import (
	"math"
	"sort"

	"github.com/bobmccarthy/vire/internal/models"
)

// SMA calculates Simple Moving Average for the given period
func SMA(bars []models.EODBar, period int) float64 {
	if len(bars) < period {
		return 0
	}

	sum := 0.0
	for i := 0; i < period; i++ {
		sum += bars[i].Close
	}
	return sum / float64(period)
}

// EMA calculates Exponential Moving Average for the given period
func EMA(bars []models.EODBar, period int) float64 {
	if len(bars) < period {
		return 0
	}

	multiplier := 2.0 / float64(period+1)
	ema := SMA(bars[len(bars)-period:], period) // Start with SMA

	// Calculate EMA from oldest to newest within the period
	for i := period - 1; i >= 0; i-- {
		ema = (bars[i].Close-ema)*multiplier + ema
	}

	return ema
}

// RSI calculates Relative Strength Index
func RSI(bars []models.EODBar, period int) float64 {
	if len(bars) < period+1 {
		return 50 // Neutral default
	}

	var gains, losses float64
	for i := 0; i < period; i++ {
		change := bars[i].Close - bars[i+1].Close
		if change > 0 {
			gains += change
		} else {
			losses -= change
		}
	}

	avgGain := gains / float64(period)
	avgLoss := losses / float64(period)

	if avgLoss == 0 {
		return 100
	}

	rs := avgGain / avgLoss
	return 100 - (100 / (1 + rs))
}

// MACD calculates Moving Average Convergence Divergence
// Returns MACD line, Signal line, and Histogram
func MACD(bars []models.EODBar, fastPeriod, slowPeriod, signalPeriod int) (float64, float64, float64) {
	if len(bars) < slowPeriod {
		return 0, 0, 0
	}

	fastEMA := EMA(bars, fastPeriod)
	slowEMA := EMA(bars, slowPeriod)
	macdLine := fastEMA - slowEMA

	// Calculate signal line (EMA of MACD line)
	// For simplicity, we'll use a smoothed approximation
	signalLine := macdLine * 0.9 // Simplified signal

	histogram := macdLine - signalLine

	return macdLine, signalLine, histogram
}

// ATR calculates Average True Range
func ATR(bars []models.EODBar, period int) float64 {
	if len(bars) < period+1 {
		return 0
	}

	trSum := 0.0
	for i := 0; i < period; i++ {
		high := bars[i].High
		low := bars[i].Low
		prevClose := bars[i+1].Close

		tr1 := high - low
		tr2 := math.Abs(high - prevClose)
		tr3 := math.Abs(low - prevClose)

		trSum += math.Max(tr1, math.Max(tr2, tr3))
	}

	return trSum / float64(period)
}

// AverageVolume calculates average volume over a period
func AverageVolume(bars []models.EODBar, period int) int64 {
	if len(bars) < period {
		return 0
	}

	var sum int64
	for i := 0; i < period; i++ {
		sum += bars[i].Volume
	}
	return sum / int64(period)
}

// VolumeRatio calculates current volume as ratio of average
func VolumeRatio(bars []models.EODBar, period int) float64 {
	if len(bars) == 0 {
		return 1.0
	}

	avg := AverageVolume(bars, period)
	if avg == 0 {
		return 1.0
	}

	return float64(bars[0].Volume) / float64(avg)
}

// High52Week returns the highest close in the last 252 trading days
func High52Week(bars []models.EODBar) float64 {
	period := 252
	if len(bars) < period {
		period = len(bars)
	}

	high := 0.0
	for i := 0; i < period; i++ {
		if bars[i].High > high {
			high = bars[i].High
		}
	}
	return high
}

// Low52Week returns the lowest close in the last 252 trading days
func Low52Week(bars []models.EODBar) float64 {
	period := 252
	if len(bars) < period {
		period = len(bars)
	}

	low := math.MaxFloat64
	for i := 0; i < period; i++ {
		if bars[i].Low < low {
			low = bars[i].Low
		}
	}
	if low == math.MaxFloat64 {
		return 0
	}
	return low
}

// DetectSupportResistance finds support and resistance levels
func DetectSupportResistance(bars []models.EODBar, lookback int) (support, resistance float64) {
	if len(bars) < lookback {
		lookback = len(bars)
	}

	// Collect highs and lows
	highs := make([]float64, lookback)
	lows := make([]float64, lookback)
	for i := 0; i < lookback; i++ {
		highs[i] = bars[i].High
		lows[i] = bars[i].Low
	}

	// Sort to find clusters
	sort.Float64s(highs)
	sort.Float64s(lows)

	// Resistance is upper quartile of highs
	resistance = highs[int(float64(len(highs))*0.75)]

	// Support is lower quartile of lows
	support = lows[int(float64(len(lows))*0.25)]

	return support, resistance
}

// DetectCrossover detects SMA crossovers
// Returns "golden_cross", "death_cross", or "none"
func DetectCrossover(bars []models.EODBar, shortPeriod, longPeriod int) string {
	if len(bars) < longPeriod+1 {
		return "none"
	}

	// Current values
	shortSMA := SMA(bars, shortPeriod)
	longSMA := SMA(bars, longPeriod)

	// Previous values (shift by 1)
	prevShortSMA := SMA(bars[1:], shortPeriod)
	prevLongSMA := SMA(bars[1:], longPeriod)

	// Golden cross: short crosses above long
	if prevShortSMA <= prevLongSMA && shortSMA > longSMA {
		return "golden_cross"
	}

	// Death cross: short crosses below long
	if prevShortSMA >= prevLongSMA && shortSMA < longSMA {
		return "death_cross"
	}

	return "none"
}

// ClassifyRSI classifies RSI value
func ClassifyRSI(rsi float64) string {
	if rsi >= 70 {
		return "overbought"
	}
	if rsi <= 30 {
		return "oversold"
	}
	return "neutral"
}

// ClassifyVolume classifies volume based on ratio
func ClassifyVolume(ratio float64) string {
	if ratio >= 2.0 {
		return "spike"
	}
	if ratio <= 0.5 {
		return "low"
	}
	return "normal"
}

// DistanceToSMA calculates percentage distance from current price to SMA
func DistanceToSMA(currentPrice, sma float64) float64 {
	if sma == 0 {
		return 0
	}
	return ((currentPrice - sma) / sma) * 100
}

// DetermineTrend classifies the overall trend
func DetermineTrend(currentPrice, sma20, sma50, sma200 float64) models.TrendType {
	// BULLISH: Price > SMA200 AND SMA20 > SMA50
	if currentPrice > sma200 && sma20 > sma50 {
		return models.TrendBullish
	}

	// BEARISH: Price < SMA200 AND SMA20 < SMA50
	if currentPrice < sma200 && sma20 < sma50 {
		return models.TrendBearish
	}

	return models.TrendNeutral
}

// TrendDescription returns a human-readable trend description
func TrendDescription(trend models.TrendType, sma20Cross50, sma50Cross200, priceCross200 string) string {
	switch trend {
	case models.TrendBullish:
		desc := "Bullish trend: Price above 200-day SMA with positive momentum"
		if sma20Cross50 == "golden_cross" {
			desc += " (recent golden cross)"
		}
		return desc
	case models.TrendBearish:
		desc := "Bearish trend: Price below 200-day SMA with negative momentum"
		if sma20Cross50 == "death_cross" {
			desc += " (recent death cross)"
		}
		return desc
	default:
		return "Neutral trend: Mixed signals, no clear direction"
	}
}
