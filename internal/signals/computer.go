// Package signals provides signal computation
package signals

import (
	"time"

	"github.com/bobmccarthy/vire/internal/models"
)

// Computer computes all signals for a ticker
type Computer struct{}

// NewComputer creates a new signal computer
func NewComputer() *Computer {
	return &Computer{}
}

// Compute calculates all signals from market data
func (c *Computer) Compute(marketData *models.MarketData) *models.TickerSignals {
	if marketData == nil || len(marketData.EOD) == 0 {
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
