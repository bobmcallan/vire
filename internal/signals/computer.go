// Package signals provides signal computation
package signals

import (
	"fmt"
	"time"

	"github.com/bobmcallan/vire/internal/models"
)

// Computer computes all signals for a ticker
type Computer struct{}

// NewComputer creates a new signal computer
func NewComputer() *Computer {
	return &Computer{}
}

// Compute calculates all signals from market data
func (c *Computer) Compute(marketData *models.MarketData) *models.TickerSignals {
	if marketData == nil {
		return &models.TickerSignals{
			ComputeTimestamp: time.Now(),
		}
	}
	if len(marketData.EOD) == 0 {
		return &models.TickerSignals{
			Ticker:           marketData.Ticker,
			ComputeTimestamp: time.Now(),
		}
	}

	bars := marketData.EOD
	currentPrice := bars[0].Close

	// Calculate SMAs
	sma20 := SMA(bars, 20)
	sma50 := SMA(bars, 50)
	sma200 := SMA(bars, 200)

	// Calculate price change
	var change, changePct float64
	if len(bars) > 1 {
		change = currentPrice - bars[1].Close
		changePct = (change / bars[1].Close) * 100
	}

	// Calculate technical indicators
	rsi := RSI(bars, 14)
	macdLine, macdSignal, macdHist := MACD(bars, 12, 26, 9)
	atr := ATR(bars, 14)
	volRatio := VolumeRatio(bars, 20)

	// Detect crossovers
	sma20Cross50 := DetectCrossover(bars, 20, 50)
	sma50Cross200 := DetectCrossover(bars, 50, 200)

	// Price vs SMA200 crossover
	var priceCross200 string
	if len(bars) > 1 {
		if bars[1].Close <= sma200 && currentPrice > sma200 {
			priceCross200 = "crossing_up"
		} else if bars[1].Close >= sma200 && currentPrice < sma200 {
			priceCross200 = "crossing_down"
		} else if currentPrice > sma200 {
			priceCross200 = "above"
		} else {
			priceCross200 = "below"
		}
	}

	// Support/Resistance
	support, resistance := DetectSupportResistance(bars, 60)
	nearSupport := currentPrice <= support*1.02
	nearResistance := currentPrice >= resistance*0.98

	// MACD crossover detection
	var macdCrossover string
	if macdHist > 0 && macdLine > 0 {
		macdCrossover = "bullish"
	} else if macdHist < 0 && macdLine < 0 {
		macdCrossover = "bearish"
	} else {
		macdCrossover = "none"
	}

	// Determine trend
	trend := DetermineTrend(currentPrice, sma20, sma50, sma200)
	trendDesc := TrendDescription(trend, sma20Cross50, sma50Cross200, priceCross200)

	signals := &models.TickerSignals{
		Ticker:           marketData.Ticker,
		ComputeTimestamp: time.Now(),

		Price: models.PriceSignals{
			Current:          currentPrice,
			Change:           change,
			ChangePct:        changePct,
			SMA20:            sma20,
			SMA50:            sma50,
			SMA200:           sma200,
			DistanceToSMA20:  DistanceToSMA(currentPrice, sma20),
			DistanceToSMA50:  DistanceToSMA(currentPrice, sma50),
			DistanceToSMA200: DistanceToSMA(currentPrice, sma200),
		},

		Technical: models.TechnicalSignals{
			RSI:              rsi,
			RSISignal:        ClassifyRSI(rsi),
			MACD:             macdLine,
			MACDSignal:       macdSignal,
			MACDHistogram:    macdHist,
			MACDCrossover:    macdCrossover,
			VolumeRatio:      volRatio,
			VolumeSignal:     ClassifyVolume(volRatio),
			ATR:              atr,
			ATRPct:           (atr / currentPrice) * 100,
			SMA20CrossSMA50:  sma20Cross50,
			SMA50CrossSMA200: sma50Cross200,
			PriceCrossSMA200: priceCross200,
			NearSupport:      nearSupport,
			NearResistance:   nearResistance,
			SupportLevel:     support,
			ResistanceLevel:  resistance,
		},

		Trend:            trend,
		TrendDescription: trendDesc,
	}

	// Calculate advanced signals
	c.computePBAS(signals, marketData)
	c.computeVLI(signals, marketData)
	c.computeRegime(signals, marketData)
	c.computeRelativeStrength(signals, marketData)
	c.computeTrendMomentum(signals, marketData)
	c.detectRiskFlags(signals, marketData)

	return signals
}

// computePBAS calculates Price-Book-Accumulation Score
func (c *Computer) computePBAS(signals *models.TickerSignals, data *models.MarketData) {
	if data.Fundamentals == nil {
		signals.PBAS = models.PBASSignal{
			Score:          0.5,
			Interpretation: "neutral",
			Description:    "Insufficient fundamental data",
		}
		return
	}

	// Calculate components
	// Business momentum: based on P/E trend
	pe := data.Fundamentals.PE
	var businessMomentum float64
	if pe > 0 && pe < 15 {
		businessMomentum = 0.7
	} else if pe >= 15 && pe < 25 {
		businessMomentum = 0.5
	} else {
		businessMomentum = 0.3
	}

	// Price momentum: based on trend
	var priceMomentum float64
	switch signals.Trend {
	case models.TrendBullish:
		priceMomentum = 0.7
	case models.TrendBearish:
		priceMomentum = 0.3
	default:
		priceMomentum = 0.5
	}

	// Divergence: difference between business and price momentum
	divergence := businessMomentum - priceMomentum

	// Overall score
	score := (businessMomentum + priceMomentum) / 2

	var interpretation string
	if divergence > 0.2 {
		interpretation = "underpriced"
	} else if divergence < -0.2 {
		interpretation = "overpriced"
	} else {
		interpretation = "neutral"
	}

	signals.PBAS = models.PBASSignal{
		Score:            score,
		BusinessMomentum: businessMomentum,
		PriceMomentum:    priceMomentum,
		Divergence:       divergence,
		Interpretation:   interpretation,
		Description:      "Price-Book-Accumulation Score measures value vs momentum",
	}
}

// computeVLI calculates Volume-Liquidity Index
func (c *Computer) computeVLI(signals *models.TickerSignals, data *models.MarketData) {
	if len(data.EOD) < 20 {
		signals.VLI = models.VLISignal{
			Score:          0.5,
			Interpretation: "neutral",
			Description:    "Insufficient data for VLI calculation",
		}
		return
	}

	// Volume strength: based on recent volume trend
	volRatio := signals.Technical.VolumeRatio
	var volumeStrength float64
	if volRatio > 1.5 {
		volumeStrength = 0.8
	} else if volRatio > 1.0 {
		volumeStrength = 0.6
	} else if volRatio > 0.5 {
		volumeStrength = 0.4
	} else {
		volumeStrength = 0.2
	}

	// Liquidity score: based on average volume
	avgVol := AverageVolume(data.EOD, 20)
	var liquidityScore float64
	if avgVol > 1000000 {
		liquidityScore = 0.9
	} else if avgVol > 100000 {
		liquidityScore = 0.6
	} else {
		liquidityScore = 0.3
	}

	score := (volumeStrength + liquidityScore) / 2

	var interpretation string
	if volumeStrength > 0.6 && signals.Trend == models.TrendBullish {
		interpretation = "accumulating"
	} else if volumeStrength > 0.6 && signals.Trend == models.TrendBearish {
		interpretation = "distributing"
	} else {
		interpretation = "neutral"
	}

	signals.VLI = models.VLISignal{
		Score:          score,
		VolumeStrength: volumeStrength,
		LiquidityScore: liquidityScore,
		Interpretation: interpretation,
		Description:    "Volume-Liquidity Index measures institutional activity",
	}
}

// computeRegime classifies market regime
func (c *Computer) computeRegime(signals *models.TickerSignals, data *models.MarketData) {
	var regime models.RegimeType
	var confidence float64

	// Simple regime detection based on trend and volatility
	atrPct := signals.Technical.ATRPct

	switch signals.Trend {
	case models.TrendBullish:
		if atrPct > 3 {
			regime = models.RegimeBreakout
			confidence = 0.7
		} else {
			regime = models.RegimeTrendUp
			confidence = 0.8
		}
	case models.TrendBearish:
		if atrPct > 3 {
			regime = models.RegimeDistribution
			confidence = 0.7
		} else {
			regime = models.RegimeTrendDown
			confidence = 0.8
		}
	default:
		if signals.VLI.Interpretation == "accumulating" {
			regime = models.RegimeAccumulation
			confidence = 0.6
		} else if signals.VLI.Interpretation == "distributing" {
			regime = models.RegimeDistribution
			confidence = 0.6
		} else {
			regime = models.RegimeRange
			confidence = 0.5
		}
	}

	signals.Regime = models.RegimeSignal{
		Current:      regime,
		Previous:     models.RegimeUndefined, // Would need historical data
		DaysInRegime: 0,                      // Would need tracking
		Confidence:   confidence,
		Description:  describeRegime(regime),
	}
}

func describeRegime(regime models.RegimeType) string {
	switch regime {
	case models.RegimeBreakout:
		return "Breakout: Strong momentum with high volatility"
	case models.RegimeTrendUp:
		return "Uptrend: Sustained bullish momentum"
	case models.RegimeTrendDown:
		return "Downtrend: Sustained bearish momentum"
	case models.RegimeAccumulation:
		return "Accumulation: Base building with volume support"
	case models.RegimeDistribution:
		return "Distribution: Selling pressure with volume"
	case models.RegimeRange:
		return "Range: Consolidating within support/resistance"
	case models.RegimeDecay:
		return "Decay: Gradual decline with low volume"
	default:
		return "Undefined: Insufficient data"
	}
}

// computeRelativeStrength calculates relative strength vs market
func (c *Computer) computeRelativeStrength(signals *models.TickerSignals, data *models.MarketData) {
	// Simplified RS calculation
	// Would normally compare to index like ASX200

	score := 1.0 // Neutral

	// Adjust based on signals
	if signals.Trend == models.TrendBullish {
		score = 1.2
	} else if signals.Trend == models.TrendBearish {
		score = 0.8
	}

	var interpretation string
	if score > 1.1 {
		interpretation = "leader"
	} else if score < 0.9 {
		interpretation = "laggard"
	} else {
		interpretation = "average"
	}

	signals.RS = models.RSSignal{
		Score:          score,
		VsMarket:       score,
		VsSector:       score,
		Rank:           0,
		TotalInSector:  0,
		Interpretation: interpretation,
	}
}

// computeTrendMomentum calculates multi-timeframe trend momentum.
// Requires >= 11 bars for full 10-day analysis. Falls back gracefully.
func (c *Computer) computeTrendMomentum(signals *models.TickerSignals, data *models.MarketData) {
	bars := data.EOD
	if len(bars) < 4 { // Need at least 3-day change
		signals.TrendMomentum = models.TrendMomentum{
			Level:       models.TrendMomentumFlat,
			Description: "Insufficient data for trend momentum",
		}
		return
	}

	currentPrice := bars[0].Close

	// Multi-timeframe price changes (%)
	change3d := priceChangePct(currentPrice, bars, 3)
	change5d := priceChangePct(currentPrice, bars, 5)
	change10d := priceChangePct(currentPrice, bars, 10)

	// Acceleration: compare recent momentum (3d) vs longer (10d normalized to 3d rate).
	// Positive = momentum is increasing, negative = decelerating.
	acceleration := 0.0
	if len(bars) >= 11 {
		rate10dPer3d := change10d * 3.0 / 10.0 // normalize 10d rate to per-3-day
		acceleration = change3d - rate10dPer3d
	}

	// Volume confirmation: average volume for last 3 days vs 20-day average.
	// Volume should support the price direction.
	volumeConfirm := false
	if len(bars) >= 20 {
		recentAvgVol := avgVolume(bars[:3])
		longerAvgVol := avgVolume(bars[:20])
		if longerAvgVol > 0 {
			volRatio := recentAvgVol / longerAvgVol
			// Confirm if: moving up with above-avg volume, or moving down with above-avg volume
			if (change3d > 0 && volRatio > 1.2) || (change3d < -1 && volRatio > 1.2) {
				volumeConfirm = true
			}
		}
	}

	// Support proximity (reuse existing support level from technical signals)
	nearSupport := false
	if signals.Technical.SupportLevel > 0 && currentPrice > 0 {
		distPct := (currentPrice - signals.Technical.SupportLevel) / signals.Technical.SupportLevel * 100
		nearSupport = distPct >= 0 && distPct <= 3.0
	}

	// Composite score: weighted combination of timeframe changes + acceleration
	// Weights: 3d=0.45, 5d=0.30, 10d=0.15, acceleration=0.10
	// Normalize each component to roughly -1 to +1 range (cap at ±20% moves)
	norm3d := clampFloat(change3d/10.0, -1, 1)
	norm5d := clampFloat(change5d/15.0, -1, 1)
	norm10d := clampFloat(change10d/20.0, -1, 1)
	normAccel := clampFloat(acceleration/5.0, -1, 1)

	score := norm3d*0.45 + norm5d*0.30 + norm10d*0.15 + normAccel*0.10

	// Classify into 5 levels
	level := classifyTrendMomentum(score, volumeConfirm)

	// Generate narrative
	desc := describeTrendMomentum(level, change3d, change5d, change10d, acceleration, volumeConfirm, nearSupport)

	signals.TrendMomentum = models.TrendMomentum{
		Level:          level,
		Score:          score,
		PriceChange3D:  change3d,
		PriceChange5D:  change5d,
		PriceChange10D: change10d,
		Acceleration:   acceleration,
		VolumeConfirm:  volumeConfirm,
		NearSupport:    nearSupport,
		Description:    desc,
	}
}

// detectRiskFlags identifies potential risk factors
func (c *Computer) detectRiskFlags(signals *models.TickerSignals, data *models.MarketData) {
	var flags []string

	// High volatility
	if signals.Technical.ATRPct > 5 {
		flags = append(flags, "high_volatility")
	}

	// Extreme RSI
	if signals.Technical.RSI > 80 {
		flags = append(flags, "extremely_overbought")
	} else if signals.Technical.RSI < 20 {
		flags = append(flags, "extremely_oversold")
	}

	// Low liquidity
	if signals.VLI.LiquidityScore < 0.4 {
		flags = append(flags, "low_liquidity")
	}

	// Large gap from moving averages
	if signals.Price.DistanceToSMA200 > 30 || signals.Price.DistanceToSMA200 < -30 {
		flags = append(flags, "extended_from_mean")
	}

	// Death cross
	if signals.Technical.SMA20CrossSMA50 == "death_cross" {
		flags = append(flags, "recent_death_cross")
	}

	// Negative PE or very high PE
	if data.Fundamentals != nil {
		if data.Fundamentals.PE < 0 {
			flags = append(flags, "negative_earnings")
		} else if data.Fundamentals.PE > 50 {
			flags = append(flags, "high_valuation")
		}
	}

	signals.RiskFlags = flags
	if len(flags) > 0 {
		signals.RiskDescription = formatRiskFlags(flags)
	}
}

func formatRiskFlags(flags []string) string {
	if len(flags) == 0 {
		return "No significant risk factors detected"
	}
	return "Risk factors: " + joinStrings(flags, ", ")
}

func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}

// priceChangePct returns the % change from `period` bars ago to current price.
// If bars are shorter than period, uses the oldest available bar.
func priceChangePct(currentPrice float64, bars []models.EODBar, period int) float64 {
	idx := period
	if idx >= len(bars) {
		idx = len(bars) - 1
	}
	if bars[idx].Close == 0 {
		return 0
	}
	return (currentPrice - bars[idx].Close) / bars[idx].Close * 100
}

// avgVolume returns average volume for the given slice of bars
func avgVolume(bars []models.EODBar) float64 {
	if len(bars) == 0 {
		return 0
	}
	var sum float64
	for _, b := range bars {
		sum += float64(b.Volume)
	}
	return sum / float64(len(bars))
}

// clampFloat clamps v to [min, max]
func clampFloat(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// classifyTrendMomentum maps composite score to 5-level enum.
// Volume confirmation strengthens the signal (widens the classification band).
func classifyTrendMomentum(score float64, volumeConfirm bool) models.TrendMomentumLevel {
	strongThreshold := 0.5
	weakThreshold := 0.15
	if volumeConfirm {
		strongThreshold = 0.4 // easier to qualify when volume confirms
		weakThreshold = 0.10
	}

	switch {
	case score >= strongThreshold:
		return models.TrendMomentumStrongUp
	case score >= weakThreshold:
		return models.TrendMomentumUp
	case score <= -strongThreshold:
		return models.TrendMomentumStrongDown
	case score <= -weakThreshold:
		return models.TrendMomentumDown
	default:
		return models.TrendMomentumFlat
	}
}

// describeTrendMomentum generates a human-readable narrative for the trend momentum.
func describeTrendMomentum(level models.TrendMomentumLevel, c3, c5, c10, accel float64, volConfirm, nearSupport bool) string {
	var narrative string
	switch level {
	case models.TrendMomentumStrongUp:
		narrative = fmt.Sprintf("Strong uptrend: +%.1f%% over 3d, +%.1f%% over 10d", c3, c10)
		if accel > 0 {
			narrative += ", accelerating gains"
		} else {
			narrative += ", decelerating gains"
		}
		if volConfirm {
			narrative += " with volume confirmation"
		}
	case models.TrendMomentumUp:
		narrative = fmt.Sprintf("Mild uptrend: +%.1f%% over 3d, +%.1f%% over 10d", c3, c10)
		if accel < 0 {
			narrative += ", momentum fading"
		}
	case models.TrendMomentumFlat:
		narrative = "Flat: minimal movement across all timeframes"
	case models.TrendMomentumDown:
		narrative = fmt.Sprintf("Deteriorating trend: %.1f%% over 3d, %.1f%% over 10d", c3, c10)
		if accel < 0 {
			narrative += ", accelerating losses"
		}
		if nearSupport {
			narrative += ", approaching support"
		}
	case models.TrendMomentumStrongDown:
		narrative = fmt.Sprintf("Strong downtrend: %.1f%% over 3d, %.1f%% over 10d", c3, c10)
		if accel < 0 {
			narrative += ", accelerating losses"
		} else {
			narrative += ", decelerating losses"
		}
		if volConfirm {
			narrative += " with volume confirmation"
		}
	}
	return narrative
}
