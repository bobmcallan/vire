// Package models defines data structures for Vire
package models

import (
	"time"
)

// TickerSignals contains all computed signals for a ticker
type TickerSignals struct {
	Ticker           string    `json:"ticker"`
	ComputeTimestamp time.Time `json:"compute_timestamp"`
	Error            string    `json:"error,omitempty"`

	// Core price data
	Price PriceSignals `json:"price"`

	// Technical signals
	Technical TechnicalSignals `json:"technical"`

	// Advanced signals
	PBAS          PBASSignal    `json:"pbas"`
	VLI           VLISignal     `json:"vli"`
	Regime        RegimeSignal  `json:"regime"`
	RS            RSSignal      `json:"relative_strength"`
	TrendMomentum TrendMomentum `json:"trend_momentum"`

	// Trend classification
	Trend            TrendType `json:"trend"`
	TrendDescription string    `json:"trend_description"`

	// Risk tracking
	RiskFlags       []string `json:"risk_flags"`
	RiskDescription string   `json:"risk_description"`
}

// PriceSignals contains price-based signal data
type PriceSignals struct {
	Current          float64 `json:"current"`
	Change           float64 `json:"change"`
	ChangePct        float64 `json:"change_pct"`
	SMA20            float64 `json:"sma_20"`
	SMA50            float64 `json:"sma_50"`
	SMA200           float64 `json:"sma_200"`
	DistanceToSMA20  float64 `json:"distance_to_sma_20"`
	DistanceToSMA50  float64 `json:"distance_to_sma_50"`
	DistanceToSMA200 float64 `json:"distance_to_sma_200"`
}

// TechnicalSignals contains technical indicator values
type TechnicalSignals struct {
	RSI           float64 `json:"rsi"`
	RSISignal     string  `json:"rsi_signal"` // oversold, neutral, overbought
	MACD          float64 `json:"macd"`
	MACDSignal    float64 `json:"macd_signal"`
	MACDHistogram float64 `json:"macd_histogram"`
	MACDCrossover string  `json:"macd_crossover"` // bullish, bearish, none
	VolumeRatio   float64 `json:"volume_ratio"`   // Current vs average
	VolumeSignal  string  `json:"volume_signal"`  // spike, normal, low
	ATR           float64 `json:"atr"`
	ATRPct        float64 `json:"atr_pct"` // ATR as % of price

	// SMA crossover signals
	SMA20CrossSMA50  string `json:"sma_20_cross_50"`  // golden_cross, death_cross, none
	SMA50CrossSMA200 string `json:"sma_50_cross_200"` // golden_cross, death_cross, none
	PriceCrossSMA200 string `json:"price_cross_200"`  // above, below, crossing_up, crossing_down

	// Support/Resistance
	NearSupport     bool    `json:"near_support"`
	NearResistance  bool    `json:"near_resistance"`
	SupportLevel    float64 `json:"support_level"`
	ResistanceLevel float64 `json:"resistance_level"`
}

// PBASSignal represents Price-Book-Accumulation Score
type PBASSignal struct {
	Score            float64 `json:"score"` // 0.0 - 1.0
	BusinessMomentum float64 `json:"business_momentum"`
	PriceMomentum    float64 `json:"price_momentum"`
	Divergence       float64 `json:"divergence"`
	Interpretation   string  `json:"interpretation"` // underpriced, neutral, overpriced
	Description      string  `json:"description"`
	Comment          string  `json:"comment"`
}

// VLISignal represents Volume-Liquidity Index
type VLISignal struct {
	Score          float64 `json:"score"` // 0.0 - 1.0
	VolumeStrength float64 `json:"volume_strength"`
	LiquidityScore float64 `json:"liquidity_score"`
	Interpretation string  `json:"interpretation"` // accumulating, distributing, neutral
	Description    string  `json:"description"`
}

// RegimeSignal represents market regime classification
type RegimeSignal struct {
	Current      RegimeType `json:"current"`
	Previous     RegimeType `json:"previous"`
	DaysInRegime int        `json:"days_in_regime"`
	Confidence   float64    `json:"confidence"` // 0.0 - 1.0
	Description  string     `json:"description"`
}

// RegimeType categorizes market regimes
type RegimeType string

const (
	RegimeBreakout     RegimeType = "breakout"
	RegimeTrendUp      RegimeType = "trend_up"
	RegimeTrendDown    RegimeType = "trend_down"
	RegimeAccumulation RegimeType = "accumulation"
	RegimeDistribution RegimeType = "distribution"
	RegimeRange        RegimeType = "range"
	RegimeDecay        RegimeType = "decay"
	RegimeUndefined    RegimeType = "undefined"
)

// RSSignal represents Relative Strength vs market
type RSSignal struct {
	Score          float64 `json:"score"` // > 1.0 = outperforming
	VsMarket       float64 `json:"vs_market"`
	VsSector       float64 `json:"vs_sector"`
	Rank           int     `json:"rank"` // Rank within sector
	TotalInSector  int     `json:"total_in_sector"`
	Interpretation string  `json:"interpretation"` // leader, average, laggard
}

// TrendMomentumLevel classifies short-term trend momentum on a 5-point scale
type TrendMomentumLevel string

const (
	TrendMomentumStrongUp   TrendMomentumLevel = "TREND_STRONG_UP"
	TrendMomentumUp         TrendMomentumLevel = "TREND_UP"
	TrendMomentumFlat       TrendMomentumLevel = "TREND_FLAT"
	TrendMomentumDown       TrendMomentumLevel = "TREND_DOWN"
	TrendMomentumStrongDown TrendMomentumLevel = "TREND_STRONG_DOWN"
)

// TrendMomentum tracks multi-timeframe price trajectory and acceleration.
// Unlike Trend (SMA-based, long-term), this captures short-term momentum
// across 3/5/10-day windows to provide early warning of deterioration.
type TrendMomentum struct {
	Level          TrendMomentumLevel `json:"level"`            // 5-point classification
	Score          float64            `json:"score"`            // -1.0 (strong down) to +1.0 (strong up)
	PriceChange3D  float64            `json:"price_change_3d"`  // 3-day price change %
	PriceChange5D  float64            `json:"price_change_5d"`  // 5-day price change %
	PriceChange10D float64            `json:"price_change_10d"` // 10-day price change %
	Acceleration   float64            `json:"acceleration"`     // Rate of change of price changes (positive = accelerating)
	VolumeConfirm  bool               `json:"volume_confirm"`   // True if volume supports the price direction
	NearSupport    bool               `json:"near_support"`     // True if within 3% of support level
	Description    string             `json:"description"`      // Human-readable narrative
}

// TrendType classifies overall trend
type TrendType string

const (
	TrendBullish TrendType = "bullish"
	TrendBearish TrendType = "bearish"
	TrendNeutral TrendType = "neutral"
)

// Signal type constants for filtering
const (
	SignalTypeSMA           = "sma"
	SignalTypeRSI           = "rsi"
	SignalTypeVolume        = "volume"
	SignalTypePBAS          = "pbas"
	SignalTypeVLI           = "vli"
	SignalTypeRegime        = "regime"
	SignalTypeRS            = "relative_strength"
	SignalTypeTrend         = "trend"
	SignalTypeSupport       = "support_resistance"
	SignalTypeMACD          = "macd"
	SignalTypeTrendMomentum = "trend_momentum"
)

// AllSignalTypes returns all available signal types
func AllSignalTypes() []string {
	return []string{
		SignalTypeSMA,
		SignalTypeRSI,
		SignalTypeVolume,
		SignalTypePBAS,
		SignalTypeVLI,
		SignalTypeRegime,
		SignalTypeRS,
		SignalTypeTrend,
		SignalTypeSupport,
		SignalTypeMACD,
		SignalTypeTrendMomentum,
	}
}
