package market

import (
	"math"
	"time"

	"github.com/bobmcallan/vire/internal/models"
)

// fieldExtractor extracts a field value from market data and signals.
// Returns nil when the field is unavailable for a ticker.
type fieldExtractor func(md *models.MarketData, sig *models.TickerSignals) interface{}

// scanFieldEntry is the internal registry entry for a scan field.
type scanFieldEntry struct {
	def       models.ScanFieldDef
	group     string
	extractor fieldExtractor
}

// ScanFieldRegistry holds all registered scan fields.
type ScanFieldRegistry struct {
	fields map[string]*scanFieldEntry
	groups []models.ScanFieldGroup
}

// NewScanFieldRegistry creates and populates the field registry.
func NewScanFieldRegistry() *ScanFieldRegistry {
	r := &ScanFieldRegistry{
		fields: make(map[string]*scanFieldEntry),
	}
	r.registerAll()
	return r
}

// Get returns the registry entry for a field, or nil if not found.
func (r *ScanFieldRegistry) Get(field string) *scanFieldEntry {
	return r.fields[field]
}

// FieldsResponse builds the ScanFieldsResponse for the introspection endpoint.
func (r *ScanFieldRegistry) FieldsResponse() *models.ScanFieldsResponse {
	return &models.ScanFieldsResponse{
		Version:     "1.0.0",
		Groups:      r.groups,
		Exchanges:   []string{"AU", "US", "ALL"},
		MaxLimit:    50,
		GeneratedAt: time.Now().UTC(),
	}
}

// register adds a field to the registry.
func (r *ScanFieldRegistry) register(group string, def models.ScanFieldDef, extractor fieldExtractor) {
	r.fields[def.Field] = &scanFieldEntry{
		def:       def,
		group:     group,
		extractor: extractor,
	}
}

// --- Operator sets ---

var (
	opsString  = []string{"==", "!=", "in", "not_in", "is_null", "not_null"}
	opsNumeric = []string{"==", "!=", "<", "<=", ">", ">=", "between", "is_null", "not_null"}
	opsBool    = []string{"==", "!="}
)

// registerAll registers all ~70+ fields across 7 groups.
func (r *ScanFieldRegistry) registerAll() {
	r.registerIdentity()
	r.registerPriceMomentum()
	r.registerMovingAverages()
	r.registerOscillators()
	r.registerVolume()
	r.registerFundamentals()
	r.registerAnalyst()

	// Build groups slice in display order
	groupOrder := []string{
		"Identity",
		"Price & Momentum",
		"Moving Averages",
		"Oscillators & Indicators",
		"Volume & Liquidity",
		"Fundamentals",
		"Analyst Sentiment",
	}
	groupFields := make(map[string][]models.ScanFieldDef)
	for _, entry := range r.fields {
		groupFields[entry.group] = append(groupFields[entry.group], entry.def)
	}
	for _, name := range groupOrder {
		if fields, ok := groupFields[name]; ok {
			r.groups = append(r.groups, models.ScanFieldGroup{
				Name:   name,
				Fields: fields,
			})
		}
	}
}

// --- Identity fields ---

func (r *ScanFieldRegistry) registerIdentity() {
	g := "Identity"
	r.register(g, models.ScanFieldDef{
		Field: "ticker", Type: "string", Description: "Exchange ticker symbol",
		Filterable: true, Sortable: false, Operators: []string{"==", "in", "not_in"},
		Example: "BHP.AU",
	}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
		return md.Ticker
	})
	r.register(g, models.ScanFieldDef{
		Field: "name", Type: "string", Description: "Company name",
		Filterable: false, Sortable: false, Operators: opsString,
	}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
		return md.Name
	})
	r.register(g, models.ScanFieldDef{
		Field: "exchange", Type: "string", Description: "Exchange code (AU, US)",
		Filterable: true, Sortable: false, Operators: []string{"==", "in"},
		Example: "AU",
	}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
		return md.Exchange
	})
	r.register(g, models.ScanFieldDef{
		Field: "sector", Type: "string", Description: "GICS sector classification",
		Filterable: true, Sortable: false, Operators: []string{"==", "!=", "in", "not_in", "is_null", "not_null"},
		Example: "Industrials",
	}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
		if md.Fundamentals == nil || md.Fundamentals.Sector == "" {
			return nil
		}
		return md.Fundamentals.Sector
	})
	r.register(g, models.ScanFieldDef{
		Field: "industry", Type: "string", Description: "GICS industry classification",
		Filterable: true, Sortable: false, Operators: []string{"==", "!=", "in", "not_in", "is_null", "not_null"},
		Nullable: true,
	}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
		if md.Fundamentals == nil || md.Fundamentals.Industry == "" {
			return nil
		}
		return md.Fundamentals.Industry
	})
	r.register(g, models.ScanFieldDef{
		Field: "country", Type: "string", Description: "Country of domicile (ISO 2-letter)",
		Filterable: true, Sortable: false, Operators: []string{"==", "!=", "in", "not_in", "is_null", "not_null"},
		Nullable: true, Example: "AU",
	}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
		if md.Fundamentals == nil || md.Fundamentals.CountryISO == "" {
			return nil
		}
		return md.Fundamentals.CountryISO
	})
	r.register(g, models.ScanFieldDef{
		Field: "market_cap", Type: "float", Description: "Market capitalisation",
		Filterable: true, Sortable: true, Operators: opsNumeric,
		Nullable: true, Example: 5000000000.0,
	}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
		if md.Fundamentals == nil || md.Fundamentals.MarketCap == 0 {
			return nil
		}
		return md.Fundamentals.MarketCap
	})
	r.register(g, models.ScanFieldDef{
		Field: "currency", Type: "string", Description: "Native currency",
		Filterable: true, Sortable: false, Operators: []string{"==", "in"},
		Nullable: true, Example: "AUD",
	}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
		// Currency is not stored on MarketData directly; derive from exchange
		switch md.Exchange {
		case "AU":
			return "AUD"
		case "US":
			return "USD"
		default:
			return nil
		}
	})
}

// --- Price & Momentum fields ---

func (r *ScanFieldRegistry) registerPriceMomentum() {
	g := "Price & Momentum"
	r.register(g, models.ScanFieldDef{
		Field: "price", Type: "float", Description: "Last close price",
		Filterable: true, Sortable: true, Operators: opsNumeric,
		Nullable: true,
	}, func(md *models.MarketData, sig *models.TickerSignals) interface{} {
		if sig != nil && sig.Price.Current > 0 {
			return sig.Price.Current
		}
		if len(md.EOD) > 0 {
			return md.EOD[0].Close
		}
		return nil
	})
	r.register(g, models.ScanFieldDef{
		Field: "price_open", Type: "float", Description: "Last session open",
		Filterable: false, Sortable: false, Operators: opsNumeric,
		Nullable: true,
	}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
		if len(md.EOD) > 0 {
			return md.EOD[0].Open
		}
		return nil
	})
	r.register(g, models.ScanFieldDef{
		Field: "price_high", Type: "float", Description: "Last session high",
		Filterable: false, Sortable: false, Operators: opsNumeric,
		Nullable: true,
	}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
		if len(md.EOD) > 0 {
			return md.EOD[0].High
		}
		return nil
	})
	r.register(g, models.ScanFieldDef{
		Field: "price_low", Type: "float", Description: "Last session low",
		Filterable: false, Sortable: false, Operators: opsNumeric,
		Nullable: true,
	}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
		if len(md.EOD) > 0 {
			return md.EOD[0].Low
		}
		return nil
	})
	r.register(g, models.ScanFieldDef{
		Field: "52_week_high", Type: "float", Description: "52-week high",
		Filterable: true, Sortable: true, Operators: opsNumeric,
		Nullable: true,
	}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
		if len(md.EOD) == 0 {
			return nil
		}
		return high52Week(md.EOD)
	})
	r.register(g, models.ScanFieldDef{
		Field: "52_week_low", Type: "float", Description: "52-week low",
		Filterable: true, Sortable: true, Operators: opsNumeric,
		Nullable: true,
	}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
		if len(md.EOD) == 0 {
			return nil
		}
		return low52Week(md.EOD)
	})
	r.register(g, models.ScanFieldDef{
		Field: "52_week_return_pct", Type: "float", Description: "Price return over 52 weeks (%)",
		Filterable: true, Sortable: true, Operators: opsNumeric,
		Nullable: true, Unit: "percent",
	}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
		return computeReturn(md.EOD, 252)
	})
	r.register(g, models.ScanFieldDef{
		Field: "30d_return_pct", Type: "float", Description: "Price return over 30 days (%)",
		Filterable: true, Sortable: true, Operators: opsNumeric,
		Nullable: true, Unit: "percent",
	}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
		return computeReturn(md.EOD, 21)
	})
	r.register(g, models.ScanFieldDef{
		Field: "7d_return_pct", Type: "float", Description: "Price return over 7 days (%)",
		Filterable: true, Sortable: true, Operators: opsNumeric,
		Nullable: true, Unit: "percent",
	}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
		return computeReturn(md.EOD, 5)
	})
	r.register(g, models.ScanFieldDef{
		Field: "ytd_return_pct", Type: "float", Description: "Year-to-date return (%)",
		Filterable: true, Sortable: true, Operators: opsNumeric,
		Nullable: true, Unit: "percent",
	}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
		return computeYTDReturn(md.EOD)
	})
	r.register(g, models.ScanFieldDef{
		Field: "new_highs_30d", Type: "int", Description: "Number of new highs in last 30 trading days",
		Filterable: true, Sortable: true, Operators: opsNumeric,
		Nullable: true,
	}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
		return countNewHighs(md.EOD, 21)
	})
	r.register(g, models.ScanFieldDef{
		Field: "weighted_alpha", Type: "float", Description: "Weighted price momentum (recent days weighted higher)",
		Filterable: true, Sortable: true, Operators: opsNumeric,
		Nullable: true,
	}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
		return computeWeightedAlpha(md.EOD)
	})
	r.register(g, models.ScanFieldDef{
		Field: "distance_to_52w_high_pct", Type: "float", Description: "% below 52-week high",
		Filterable: true, Sortable: true, Operators: opsNumeric,
		Nullable: true, Unit: "percent",
	}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
		if len(md.EOD) == 0 {
			return nil
		}
		h := high52Week(md.EOD)
		price := md.EOD[0].Close
		if h == 0 || price == 0 {
			return nil
		}
		return ((price - h) / h) * 100
	})
	r.register(g, models.ScanFieldDef{
		Field: "distance_to_52w_low_pct", Type: "float", Description: "% above 52-week low",
		Filterable: true, Sortable: true, Operators: opsNumeric,
		Nullable: true, Unit: "percent",
	}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
		if len(md.EOD) == 0 {
			return nil
		}
		l := low52Week(md.EOD)
		price := md.EOD[0].Close
		if l == 0 || price == 0 {
			return nil
		}
		return ((price - l) / l) * 100
	})
}

// --- Moving Average fields ---

func (r *ScanFieldRegistry) registerMovingAverages() {
	g := "Moving Averages"
	r.register(g, models.ScanFieldDef{
		Field: "sma_20", Type: "float", Description: "20-day simple moving average",
		Filterable: true, Sortable: false, Operators: opsNumeric,
		Nullable: true,
	}, func(_ *models.MarketData, sig *models.TickerSignals) interface{} {
		if sig == nil || sig.Price.SMA20 == 0 {
			return nil
		}
		return sig.Price.SMA20
	})
	r.register(g, models.ScanFieldDef{
		Field: "sma_50", Type: "float", Description: "50-day simple moving average",
		Filterable: true, Sortable: false, Operators: opsNumeric,
		Nullable: true,
	}, func(_ *models.MarketData, sig *models.TickerSignals) interface{} {
		if sig == nil || sig.Price.SMA50 == 0 {
			return nil
		}
		return sig.Price.SMA50
	})
	r.register(g, models.ScanFieldDef{
		Field: "sma_200", Type: "float", Description: "200-day simple moving average",
		Filterable: true, Sortable: false, Operators: opsNumeric,
		Nullable: true,
	}, func(_ *models.MarketData, sig *models.TickerSignals) interface{} {
		if sig == nil || sig.Price.SMA200 == 0 {
			return nil
		}
		return sig.Price.SMA200
	})
	r.register(g, models.ScanFieldDef{
		Field: "ema_20", Type: "float", Description: "20-day exponential moving average",
		Filterable: true, Sortable: false, Operators: opsNumeric,
		Nullable: true,
	}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
		return computeEMA(md.EOD, 20)
	})
	r.register(g, models.ScanFieldDef{
		Field: "ema_50", Type: "float", Description: "50-day exponential moving average",
		Filterable: true, Sortable: false, Operators: opsNumeric,
		Nullable: true,
	}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
		return computeEMA(md.EOD, 50)
	})
	r.register(g, models.ScanFieldDef{
		Field: "price_vs_sma_20_pct", Type: "float", Description: "% above/below SMA20",
		Filterable: true, Sortable: true, Operators: opsNumeric,
		Nullable: true, Unit: "percent",
	}, func(_ *models.MarketData, sig *models.TickerSignals) interface{} {
		if sig == nil || sig.Price.SMA20 == 0 {
			return nil
		}
		return sig.Price.DistanceToSMA20
	})
	r.register(g, models.ScanFieldDef{
		Field: "price_vs_sma_50_pct", Type: "float", Description: "% above/below SMA50",
		Filterable: true, Sortable: true, Operators: opsNumeric,
		Nullable: true, Unit: "percent",
	}, func(_ *models.MarketData, sig *models.TickerSignals) interface{} {
		if sig == nil || sig.Price.SMA50 == 0 {
			return nil
		}
		return sig.Price.DistanceToSMA50
	})
	r.register(g, models.ScanFieldDef{
		Field: "price_vs_sma_200_pct", Type: "float", Description: "% above/below SMA200",
		Filterable: true, Sortable: true, Operators: opsNumeric,
		Nullable: true, Unit: "percent",
	}, func(_ *models.MarketData, sig *models.TickerSignals) interface{} {
		if sig == nil || sig.Price.SMA200 == 0 {
			return nil
		}
		return sig.Price.DistanceToSMA200
	})
	r.register(g, models.ScanFieldDef{
		Field: "sma_20_above_sma_50", Type: "bool", Description: "SMA20 above SMA50 (short-term golden cross)",
		Filterable: true, Sortable: false, Operators: opsBool,
		Nullable: true,
	}, func(_ *models.MarketData, sig *models.TickerSignals) interface{} {
		if sig == nil || sig.Price.SMA20 == 0 || sig.Price.SMA50 == 0 {
			return nil
		}
		return sig.Price.SMA20 > sig.Price.SMA50
	})
	r.register(g, models.ScanFieldDef{
		Field: "sma_50_above_sma_200", Type: "bool", Description: "SMA50 above SMA200 (long-term golden cross)",
		Filterable: true, Sortable: false, Operators: opsBool,
		Nullable: true,
	}, func(_ *models.MarketData, sig *models.TickerSignals) interface{} {
		if sig == nil || sig.Price.SMA50 == 0 || sig.Price.SMA200 == 0 {
			return nil
		}
		return sig.Price.SMA50 > sig.Price.SMA200
	})
}

// --- Oscillators & Indicators ---

func (r *ScanFieldRegistry) registerOscillators() {
	g := "Oscillators & Indicators"
	r.register(g, models.ScanFieldDef{
		Field: "rsi_14", Type: "float", Description: "RSI 14-period",
		Filterable: true, Sortable: true, Operators: opsNumeric,
		Nullable: true, Example: 55.0,
	}, func(_ *models.MarketData, sig *models.TickerSignals) interface{} {
		if sig == nil || sig.Technical.RSI == 0 {
			return nil
		}
		return sig.Technical.RSI
	})
	r.register(g, models.ScanFieldDef{
		Field: "rsi_signal", Type: "string", Description: "RSI classification: overbought / neutral / oversold",
		Filterable: true, Sortable: false, Operators: []string{"==", "!=", "in"},
		Nullable: true,
		Enum:     []string{"overbought", "neutral", "oversold"},
	}, func(_ *models.MarketData, sig *models.TickerSignals) interface{} {
		if sig == nil || sig.Technical.RSISignal == "" {
			return nil
		}
		return sig.Technical.RSISignal
	})
	r.register(g, models.ScanFieldDef{
		Field: "macd", Type: "float", Description: "MACD line",
		Filterable: true, Sortable: true, Operators: opsNumeric,
		Nullable: true,
	}, func(_ *models.MarketData, sig *models.TickerSignals) interface{} {
		if sig == nil {
			return nil
		}
		return sig.Technical.MACD
	})
	r.register(g, models.ScanFieldDef{
		Field: "macd_signal", Type: "float", Description: "MACD signal line",
		Filterable: true, Sortable: false, Operators: opsNumeric,
		Nullable: true,
	}, func(_ *models.MarketData, sig *models.TickerSignals) interface{} {
		if sig == nil {
			return nil
		}
		return sig.Technical.MACDSignal
	})
	r.register(g, models.ScanFieldDef{
		Field: "macd_histogram", Type: "float", Description: "MACD histogram",
		Filterable: true, Sortable: true, Operators: opsNumeric,
		Nullable: true,
	}, func(_ *models.MarketData, sig *models.TickerSignals) interface{} {
		if sig == nil {
			return nil
		}
		return sig.Technical.MACDHistogram
	})
	r.register(g, models.ScanFieldDef{
		Field: "macd_crossover", Type: "string", Description: "MACD crossover: bullish / bearish / none",
		Filterable: true, Sortable: false, Operators: []string{"==", "!=", "in"},
		Nullable: true,
		Enum:     []string{"bullish", "bearish", "none"},
	}, func(_ *models.MarketData, sig *models.TickerSignals) interface{} {
		if sig == nil || sig.Technical.MACDCrossover == "" {
			return nil
		}
		return sig.Technical.MACDCrossover
	})
	r.register(g, models.ScanFieldDef{
		Field: "atr", Type: "float", Description: "Average True Range",
		Filterable: true, Sortable: true, Operators: opsNumeric,
		Nullable: true,
	}, func(_ *models.MarketData, sig *models.TickerSignals) interface{} {
		if sig == nil || sig.Technical.ATR == 0 {
			return nil
		}
		return sig.Technical.ATR
	})
	r.register(g, models.ScanFieldDef{
		Field: "atr_pct", Type: "float", Description: "ATR as % of price (volatility proxy)",
		Filterable: true, Sortable: true, Operators: opsNumeric,
		Nullable: true, Unit: "percent",
	}, func(_ *models.MarketData, sig *models.TickerSignals) interface{} {
		if sig == nil || sig.Technical.ATRPct == 0 {
			return nil
		}
		return sig.Technical.ATRPct
	})
	r.register(g, models.ScanFieldDef{
		Field: "bollinger_upper", Type: "float", Description: "Bollinger upper band (SMA20 + 2*stddev)",
		Filterable: true, Sortable: false, Operators: opsNumeric,
		Nullable: true,
	}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
		upper, _ := computeBollingerBands(md.EOD, 20, 2)
		return upper
	})
	r.register(g, models.ScanFieldDef{
		Field: "bollinger_lower", Type: "float", Description: "Bollinger lower band (SMA20 - 2*stddev)",
		Filterable: true, Sortable: false, Operators: opsNumeric,
		Nullable: true,
	}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
		_, lower := computeBollingerBands(md.EOD, 20, 2)
		return lower
	})
	r.register(g, models.ScanFieldDef{
		Field: "bollinger_pct_b", Type: "float", Description: "Position within Bollinger bands (0=lower, 1=upper)",
		Filterable: true, Sortable: true, Operators: opsNumeric,
		Nullable: true,
	}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
		return computeBollingerPctB(md.EOD, 20, 2)
	})
	r.register(g, models.ScanFieldDef{
		Field: "stoch_k", Type: "float", Description: "Stochastic %K (14-period)",
		Filterable: true, Sortable: true, Operators: opsNumeric,
		Nullable: true,
	}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
		k, _ := computeStochastic(md.EOD, 14, 3)
		return k
	})
	r.register(g, models.ScanFieldDef{
		Field: "stoch_d", Type: "float", Description: "Stochastic %D (3-period SMA of %K)",
		Filterable: true, Sortable: true, Operators: opsNumeric,
		Nullable: true,
	}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
		_, d := computeStochastic(md.EOD, 14, 3)
		return d
	})
	r.register(g, models.ScanFieldDef{
		Field: "adx", Type: "float", Description: "Average Directional Index (14-period, trend strength)",
		Filterable: true, Sortable: true, Operators: opsNumeric,
		Nullable: true,
	}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
		return computeADX(md.EOD, 14)
	})
	r.register(g, models.ScanFieldDef{
		Field: "cci", Type: "float", Description: "Commodity Channel Index (20-period)",
		Filterable: true, Sortable: true, Operators: opsNumeric,
		Nullable: true,
	}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
		return computeCCI(md.EOD, 20)
	})
	r.register(g, models.ScanFieldDef{
		Field: "williams_r", Type: "float", Description: "Williams %R (14-period)",
		Filterable: true, Sortable: true, Operators: opsNumeric,
		Nullable: true,
	}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
		return computeWilliamsR(md.EOD, 14)
	})
	r.register(g, models.ScanFieldDef{
		Field: "near_support", Type: "bool", Description: "Price within 2% of support level",
		Filterable: true, Sortable: false, Operators: opsBool,
		Nullable: true,
	}, func(_ *models.MarketData, sig *models.TickerSignals) interface{} {
		if sig == nil {
			return nil
		}
		return sig.Technical.NearSupport
	})
	r.register(g, models.ScanFieldDef{
		Field: "near_resistance", Type: "bool", Description: "Price within 2% of resistance level",
		Filterable: true, Sortable: false, Operators: opsBool,
		Nullable: true,
	}, func(_ *models.MarketData, sig *models.TickerSignals) interface{} {
		if sig == nil {
			return nil
		}
		return sig.Technical.NearResistance
	})
	r.register(g, models.ScanFieldDef{
		Field: "support_level", Type: "float", Description: "Nearest support price",
		Filterable: true, Sortable: false, Operators: opsNumeric,
		Nullable: true,
	}, func(_ *models.MarketData, sig *models.TickerSignals) interface{} {
		if sig == nil || sig.Technical.SupportLevel == 0 {
			return nil
		}
		return sig.Technical.SupportLevel
	})
	r.register(g, models.ScanFieldDef{
		Field: "resistance_level", Type: "float", Description: "Nearest resistance price",
		Filterable: true, Sortable: false, Operators: opsNumeric,
		Nullable: true,
	}, func(_ *models.MarketData, sig *models.TickerSignals) interface{} {
		if sig == nil || sig.Technical.ResistanceLevel == 0 {
			return nil
		}
		return sig.Technical.ResistanceLevel
	})
}

// --- Volume & Liquidity ---

func (r *ScanFieldRegistry) registerVolume() {
	g := "Volume & Liquidity"
	r.register(g, models.ScanFieldDef{
		Field: "volume", Type: "int", Description: "Last session volume",
		Filterable: true, Sortable: true, Operators: opsNumeric,
		Nullable: true,
	}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
		if len(md.EOD) == 0 {
			return nil
		}
		return md.EOD[0].Volume
	})
	r.register(g, models.ScanFieldDef{
		Field: "avg_volume_30d", Type: "int", Description: "30-day average volume",
		Filterable: true, Sortable: true, Operators: opsNumeric,
		Nullable: true,
	}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
		return computeAvgVolume(md.EOD, 30)
	})
	r.register(g, models.ScanFieldDef{
		Field: "avg_volume_90d", Type: "int", Description: "90-day average volume",
		Filterable: true, Sortable: true, Operators: opsNumeric,
		Nullable: true,
	}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
		return computeAvgVolume(md.EOD, 90)
	})
	r.register(g, models.ScanFieldDef{
		Field: "volume_ratio", Type: "float", Description: "Volume / avg_volume_30d",
		Filterable: true, Sortable: true, Operators: opsNumeric,
		Nullable: true,
	}, func(_ *models.MarketData, sig *models.TickerSignals) interface{} {
		if sig == nil || sig.Technical.VolumeRatio == 0 {
			return nil
		}
		return sig.Technical.VolumeRatio
	})
	r.register(g, models.ScanFieldDef{
		Field: "volume_trend", Type: "string", Description: "Volume trend: accumulating / distributing / neutral",
		Filterable: true, Sortable: false, Operators: []string{"==", "!=", "in"},
		Nullable: true,
		Enum:     []string{"accumulating", "distributing", "neutral"},
	}, func(_ *models.MarketData, sig *models.TickerSignals) interface{} {
		if sig == nil || sig.VLI.Interpretation == "" {
			return nil
		}
		return sig.VLI.Interpretation
	})
	r.register(g, models.ScanFieldDef{
		Field: "relative_volume", Type: "float", Description: "Volume vs 90-day average",
		Filterable: true, Sortable: true, Operators: opsNumeric,
		Nullable: true,
	}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
		if len(md.EOD) == 0 {
			return nil
		}
		avg90 := computeAvgVolume(md.EOD, 90)
		if avg90 == nil {
			return nil
		}
		avgVal := avg90.(int64)
		if avgVal == 0 {
			return nil
		}
		return float64(md.EOD[0].Volume) / float64(avgVal)
	})
}

// --- Fundamentals ---

func (r *ScanFieldRegistry) registerFundamentals() {
	g := "Fundamentals"

	numField := func(field, desc string, extract func(f *models.Fundamentals) float64) {
		r.register(g, models.ScanFieldDef{
			Field: field, Type: "float", Description: desc,
			Filterable: true, Sortable: true, Operators: opsNumeric,
			Nullable: true,
		}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
			if md.Fundamentals == nil {
				return nil
			}
			v := extract(md.Fundamentals)
			if v == 0 {
				return nil
			}
			return v
		})
	}

	numField("pe_ratio", "Trailing price-to-earnings ratio", func(f *models.Fundamentals) float64 { return f.PE })
	numField("pe_forward", "Forward PE (next 12 months)", func(f *models.Fundamentals) float64 { return f.ForwardPE })
	numField("pb_ratio", "Price to book ratio", func(f *models.Fundamentals) float64 { return f.PB })
	numField("ps_ratio", "Price to sales ratio", func(f *models.Fundamentals) float64 {
		if f.RevenueTTM > 0 && f.MarketCap > 0 {
			return f.MarketCap / f.RevenueTTM
		}
		return 0
	})
	numField("ev_ebitda", "EV/EBITDA ratio", func(f *models.Fundamentals) float64 {
		if f.EBITDA > 0 && f.MarketCap > 0 {
			return f.MarketCap / f.EBITDA
		}
		return 0
	})
	numField("peg_ratio", "PEG ratio", func(f *models.Fundamentals) float64 { return f.PEGRatio })

	// Boolean: positive earnings
	r.register(g, models.ScanFieldDef{
		Field: "earnings_positive", Type: "bool", Description: "Positive trailing earnings",
		Filterable: true, Sortable: false, Operators: opsBool,
		Nullable: true,
	}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
		if md.Fundamentals == nil {
			return nil
		}
		return md.Fundamentals.EPS > 0
	})

	numField("eps_ttm", "Earnings per share (trailing 12 months)", func(f *models.Fundamentals) float64 { return f.EPS })
	numField("eps_next_yr", "Estimated EPS next year", func(f *models.Fundamentals) float64 { return f.EPSEstimateNext })
	numField("earnings_growth_next_yr_pct", "Estimated earnings growth next year (%)", func(f *models.Fundamentals) float64 { return f.EarningsGrowthYOY })
	numField("revenue_ttm", "Revenue trailing 12 months", func(f *models.Fundamentals) float64 { return f.RevenueTTM })
	numField("revenue_growth_1yr_pct", "Revenue growth YoY (%)", func(f *models.Fundamentals) float64 { return f.RevGrowthYOY })

	// revenue_growth_3yr_pct — compute from historical financials
	r.register(g, models.ScanFieldDef{
		Field: "revenue_growth_3yr_pct", Type: "float", Description: "Revenue CAGR over 3 years (%)",
		Filterable: true, Sortable: true, Operators: opsNumeric,
		Nullable: true, Unit: "percent",
	}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
		if md.Fundamentals == nil || len(md.Fundamentals.HistoricalFinancials) < 3 {
			return nil
		}
		hf := md.Fundamentals.HistoricalFinancials
		recent := hf[0].Revenue
		old := hf[2].Revenue
		if old <= 0 || recent <= 0 {
			return nil
		}
		cagr := (math.Pow(recent/old, 1.0/3.0) - 1) * 100
		return cagr
	})

	numField("gross_margin_pct", "Gross margin (%)", func(f *models.Fundamentals) float64 {
		if f.RevenueTTM > 0 && f.GrossProfitTTM > 0 {
			return (f.GrossProfitTTM / f.RevenueTTM) * 100
		}
		return 0
	})
	numField("operating_margin_pct", "Operating margin (%)", func(f *models.Fundamentals) float64 {
		return f.OperatingMarginTTM * 100
	})
	numField("net_margin_pct", "Net margin (%)", func(f *models.Fundamentals) float64 {
		return f.ProfitMargin * 100
	})
	numField("roe", "Return on equity", func(f *models.Fundamentals) float64 { return f.ReturnOnEquityTTM })
	numField("roa", "Return on assets", func(f *models.Fundamentals) float64 { return f.ReturnOnAssetsTTM })
	numField("beta", "Beta vs market index", func(f *models.Fundamentals) float64 { return f.Beta })

	numField("dividend_yield_pct", "Trailing dividend yield (%)", func(f *models.Fundamentals) float64 {
		return f.DividendYield * 100
	})

	// debt_to_equity, current_ratio, free_cash_flow, fcf_yield_pct, payout_ratio, buyback_yield_pct,
	// dividend_growth_3yr_pct, short_interest_pct, days_to_cover — not available in current Fundamentals struct.
	// Register them but return nil (nullable).
	for _, f := range []struct {
		field string
		desc  string
	}{
		{"debt_to_equity", "Debt-to-equity ratio"},
		{"current_ratio", "Current ratio"},
		{"free_cash_flow", "Free cash flow (TTM)"},
		{"fcf_yield_pct", "FCF yield (%)"},
		{"dividend_growth_3yr_pct", "Dividend CAGR 3 years (%)"},
		{"payout_ratio", "Dividend payout ratio"},
		{"buyback_yield_pct", "Share buyback yield (%)"},
		{"short_interest_pct", "Short interest % of float"},
		{"days_to_cover", "Short interest days to cover"},
	} {
		field := f.field
		desc := f.desc
		r.register(g, models.ScanFieldDef{
			Field: field, Type: "float", Description: desc,
			Filterable: true, Sortable: true, Operators: opsNumeric,
			Nullable: true,
		}, func(_ *models.MarketData, _ *models.TickerSignals) interface{} {
			return nil // not yet available in data model
		})
	}
}

// --- Analyst Sentiment ---

func (r *ScanFieldRegistry) registerAnalyst() {
	g := "Analyst Sentiment"

	intField := func(field, desc string, extract func(ar *models.AnalystRatings) int) {
		r.register(g, models.ScanFieldDef{
			Field: field, Type: "int", Description: desc,
			Filterable: true, Sortable: true, Operators: opsNumeric,
			Nullable: true,
		}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
			if md.Fundamentals == nil || md.Fundamentals.AnalystRatings == nil {
				return nil
			}
			return extract(md.Fundamentals.AnalystRatings)
		})
	}

	intField("analyst_strong_buy", "Count of Strong Buy ratings", func(ar *models.AnalystRatings) int { return ar.StrongBuy })
	intField("analyst_buy", "Count of Buy ratings", func(ar *models.AnalystRatings) int { return ar.Buy })
	intField("analyst_hold", "Count of Hold ratings", func(ar *models.AnalystRatings) int { return ar.Hold })
	intField("analyst_sell", "Count of Sell ratings", func(ar *models.AnalystRatings) int { return ar.Sell })

	r.register(g, models.ScanFieldDef{
		Field: "analyst_consensus", Type: "string", Description: "Consensus rating: strong_buy / buy / hold / sell",
		Filterable: true, Sortable: false, Operators: []string{"==", "!=", "in"},
		Nullable: true,
		Enum:     []string{"strong_buy", "buy", "hold", "sell"},
	}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
		if md.Fundamentals == nil || md.Fundamentals.AnalystRatings == nil {
			return nil
		}
		return deriveConsensus(md.Fundamentals.AnalystRatings)
	})

	r.register(g, models.ScanFieldDef{
		Field: "analyst_target_price", Type: "float", Description: "Mean analyst price target",
		Filterable: true, Sortable: true, Operators: opsNumeric,
		Nullable: true,
	}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
		if md.Fundamentals == nil || md.Fundamentals.AnalystRatings == nil || md.Fundamentals.AnalystRatings.TargetPrice == 0 {
			return nil
		}
		return md.Fundamentals.AnalystRatings.TargetPrice
	})

	r.register(g, models.ScanFieldDef{
		Field: "analyst_target_upside_pct", Type: "float", Description: "% upside to mean analyst target",
		Filterable: true, Sortable: true, Operators: opsNumeric,
		Nullable: true, Unit: "percent",
	}, func(md *models.MarketData, sig *models.TickerSignals) interface{} {
		if md.Fundamentals == nil || md.Fundamentals.AnalystRatings == nil || md.Fundamentals.AnalystRatings.TargetPrice == 0 {
			return nil
		}
		price := 0.0
		if sig != nil && sig.Price.Current > 0 {
			price = sig.Price.Current
		} else if len(md.EOD) > 0 {
			price = md.EOD[0].Close
		}
		if price == 0 {
			return nil
		}
		return ((md.Fundamentals.AnalystRatings.TargetPrice - price) / price) * 100
	})

	r.register(g, models.ScanFieldDef{
		Field: "analyst_count", Type: "int", Description: "Total analysts covering",
		Filterable: true, Sortable: true, Operators: opsNumeric,
		Nullable: true,
	}, func(md *models.MarketData, _ *models.TickerSignals) interface{} {
		if md.Fundamentals == nil || md.Fundamentals.AnalystRatings == nil {
			return nil
		}
		ar := md.Fundamentals.AnalystRatings
		total := ar.StrongBuy + ar.Buy + ar.Hold + ar.Sell + ar.StrongSell
		if total == 0 {
			return nil
		}
		return total
	})
}

// --- Helper functions for computed fields ---

func high52Week(bars []models.EODBar) float64 {
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

func low52Week(bars []models.EODBar) float64 {
	period := 252
	if len(bars) < period {
		period = len(bars)
	}
	if period == 0 {
		return 0
	}
	low := bars[0].Low
	for i := 1; i < period; i++ {
		if bars[i].Low < low && bars[i].Low > 0 {
			low = bars[i].Low
		}
	}
	return low
}

func computeReturn(bars []models.EODBar, periods int) interface{} {
	if len(bars) <= periods {
		return nil
	}
	current := bars[0].Close
	old := bars[periods].Close
	if old <= 0 {
		return nil
	}
	return ((current - old) / old) * 100
}

func computeYTDReturn(bars []models.EODBar) interface{} {
	if len(bars) < 2 {
		return nil
	}
	current := bars[0].Close
	currentYear := bars[0].Date.Year()

	// Find the first bar of the current year (or closest to Jan 1)
	var yearStartClose float64
	for i := len(bars) - 1; i >= 0; i-- {
		if bars[i].Date.Year() == currentYear {
			yearStartClose = bars[i].Close
			break
		}
	}
	if yearStartClose <= 0 {
		// Fallback: use last bar of previous year
		for i := 0; i < len(bars); i++ {
			if bars[i].Date.Year() < currentYear {
				yearStartClose = bars[i].Close
				break
			}
		}
	}
	if yearStartClose <= 0 {
		return nil
	}
	return ((current - yearStartClose) / yearStartClose) * 100
}

func countNewHighs(bars []models.EODBar, period int) interface{} {
	if len(bars) < period+1 {
		return nil
	}
	count := 0
	for i := 0; i < period; i++ {
		// Check if this bar's high is higher than the previous bar's high
		if bars[i].High > bars[i+1].High {
			count++
		}
	}
	return count
}

func computeWeightedAlpha(bars []models.EODBar) interface{} {
	period := 252
	if len(bars) <= period {
		return nil
	}
	// Weighted alpha: sum of daily returns weighted linearly (recent days weighted more)
	totalWeight := 0.0
	weightedReturn := 0.0
	for i := 0; i < period; i++ {
		if bars[i+1].Close <= 0 {
			continue
		}
		dailyReturn := (bars[i].Close - bars[i+1].Close) / bars[i+1].Close
		weight := float64(period - i) // most recent = highest weight
		weightedReturn += dailyReturn * weight
		totalWeight += weight
	}
	if totalWeight == 0 {
		return nil
	}
	// Scale to approximate annual percentage
	return (weightedReturn / totalWeight) * 100 * float64(period)
}

func computeEMA(bars []models.EODBar, period int) interface{} {
	if len(bars) < period {
		return nil
	}
	// EMA calculation. Bars are descending, so we need to iterate in reverse.
	// Take the most recent `period` bars, reversed.
	startIdx := period - 1
	if startIdx >= len(bars) {
		return nil
	}

	multiplier := 2.0 / float64(period+1)

	// Seed with SMA of first `period` bars (oldest first)
	sum := 0.0
	for i := startIdx; i >= 0; i-- {
		sum += bars[i].Close
	}
	// We don't have enough history for a proper seed — use a wider window
	// Use all available bars up to 2x period for seeding
	barsToUse := len(bars)
	if barsToUse > period*3 {
		barsToUse = period * 3
	}

	// Compute SMA seed from the oldest `period` bars
	smaSum := 0.0
	seedStart := barsToUse - 1
	seedEnd := barsToUse - period
	if seedEnd < 0 {
		seedEnd = 0
	}
	count := 0
	for i := seedStart; i >= seedEnd; i-- {
		smaSum += bars[i].Close
		count++
	}
	if count == 0 {
		return nil
	}
	ema := smaSum / float64(count)

	// Apply EMA from oldest to most recent
	for i := seedEnd - 1; i >= 0; i-- {
		ema = (bars[i].Close-ema)*multiplier + ema
	}
	return ema
}

func computeBollingerBands(bars []models.EODBar, period int, mult float64) (upper interface{}, lower interface{}) {
	if len(bars) < period {
		return nil, nil
	}
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += bars[i].Close
	}
	sma := sum / float64(period)

	variance := 0.0
	for i := 0; i < period; i++ {
		diff := bars[i].Close - sma
		variance += diff * diff
	}
	stddev := math.Sqrt(variance / float64(period))

	return sma + mult*stddev, sma - mult*stddev
}

func computeBollingerPctB(bars []models.EODBar, period int, mult float64) interface{} {
	upper, lower := computeBollingerBands(bars, period, mult)
	if upper == nil || lower == nil {
		return nil
	}
	u := upper.(float64)
	l := lower.(float64)
	if u == l {
		return nil
	}
	if len(bars) == 0 {
		return nil
	}
	return (bars[0].Close - l) / (u - l)
}

func computeStochastic(bars []models.EODBar, kPeriod, dPeriod int) (k interface{}, d interface{}) {
	if len(bars) < kPeriod+dPeriod {
		return nil, nil
	}

	// Compute %K values for dPeriod bars to get %D
	kValues := make([]float64, 0, dPeriod)
	for offset := 0; offset < dPeriod; offset++ {
		highest := 0.0
		lowest := math.MaxFloat64
		for i := offset; i < offset+kPeriod; i++ {
			if bars[i].High > highest {
				highest = bars[i].High
			}
			if bars[i].Low < lowest {
				lowest = bars[i].Low
			}
		}
		if highest == lowest {
			kValues = append(kValues, 50)
		} else {
			kValues = append(kValues, ((bars[offset].Close-lowest)/(highest-lowest))*100)
		}
	}

	// Current %K
	kVal := kValues[0]

	// %D = SMA of %K values
	dSum := 0.0
	for _, v := range kValues {
		dSum += v
	}
	dVal := dSum / float64(len(kValues))

	return kVal, dVal
}

func computeADX(bars []models.EODBar, period int) interface{} {
	// Need at least 2*period+1 bars for a stable ADX
	needed := 2*period + 1
	if len(bars) < needed {
		return nil
	}

	// Bars are descending. We need chronological order for ADX calculation.
	// Work with the most recent `needed` bars, reversed.
	n := needed
	chronoBars := make([]models.EODBar, n)
	for i := 0; i < n; i++ {
		chronoBars[i] = bars[n-1-i]
	}

	// Compute TR, +DM, -DM
	trs := make([]float64, n-1)
	plusDMs := make([]float64, n-1)
	minusDMs := make([]float64, n-1)

	for i := 1; i < n; i++ {
		high := chronoBars[i].High
		low := chronoBars[i].Low
		prevClose := chronoBars[i-1].Close
		prevHigh := chronoBars[i-1].High
		prevLow := chronoBars[i-1].Low

		tr := math.Max(high-low, math.Max(math.Abs(high-prevClose), math.Abs(low-prevClose)))
		trs[i-1] = tr

		upMove := high - prevHigh
		downMove := prevLow - low

		if upMove > downMove && upMove > 0 {
			plusDMs[i-1] = upMove
		}
		if downMove > upMove && downMove > 0 {
			minusDMs[i-1] = downMove
		}
	}

	// Smoothed averages (Wilder's smoothing)
	atr := 0.0
	plusDM := 0.0
	minusDM := 0.0
	for i := 0; i < period; i++ {
		atr += trs[i]
		plusDM += plusDMs[i]
		minusDM += minusDMs[i]
	}

	dxValues := make([]float64, 0, period)
	for i := period; i < len(trs); i++ {
		atr = atr - atr/float64(period) + trs[i]
		plusDM = plusDM - plusDM/float64(period) + plusDMs[i]
		minusDM = minusDM - minusDM/float64(period) + minusDMs[i]

		if atr == 0 {
			continue
		}
		plusDI := (plusDM / atr) * 100
		minusDI := (minusDM / atr) * 100
		diSum := plusDI + minusDI
		if diSum == 0 {
			dxValues = append(dxValues, 0)
		} else {
			dxValues = append(dxValues, math.Abs(plusDI-minusDI)/diSum*100)
		}
	}

	if len(dxValues) == 0 {
		return nil
	}

	// ADX = smoothed average of DX
	adx := 0.0
	for _, dx := range dxValues {
		adx += dx
	}
	return adx / float64(len(dxValues))
}

func computeCCI(bars []models.EODBar, period int) interface{} {
	if len(bars) < period {
		return nil
	}
	// Typical prices for the period
	tps := make([]float64, period)
	sum := 0.0
	for i := 0; i < period; i++ {
		tp := (bars[i].High + bars[i].Low + bars[i].Close) / 3
		tps[i] = tp
		sum += tp
	}
	mean := sum / float64(period)

	// Mean deviation
	devSum := 0.0
	for _, tp := range tps {
		devSum += math.Abs(tp - mean)
	}
	meanDev := devSum / float64(period)

	if meanDev == 0 {
		return nil
	}
	return (tps[0] - mean) / (0.015 * meanDev)
}

func computeWilliamsR(bars []models.EODBar, period int) interface{} {
	if len(bars) < period {
		return nil
	}
	highest := 0.0
	lowest := math.MaxFloat64
	for i := 0; i < period; i++ {
		if bars[i].High > highest {
			highest = bars[i].High
		}
		if bars[i].Low < lowest {
			lowest = bars[i].Low
		}
	}
	if highest == lowest {
		return nil
	}
	return ((highest - bars[0].Close) / (highest - lowest)) * -100
}

func computeAvgVolume(bars []models.EODBar, period int) interface{} {
	if len(bars) < period {
		return nil
	}
	var sum int64
	for i := 0; i < period; i++ {
		sum += bars[i].Volume
	}
	return sum / int64(period)
}

func deriveConsensus(ar *models.AnalystRatings) string {
	total := ar.StrongBuy + ar.Buy + ar.Hold + ar.Sell + ar.StrongSell
	if total == 0 {
		return "hold"
	}
	// Weighted score: strong_buy=5, buy=4, hold=3, sell=2, strong_sell=1
	score := float64(ar.StrongBuy*5+ar.Buy*4+ar.Hold*3+ar.Sell*2+ar.StrongSell) / float64(total)
	switch {
	case score >= 4.5:
		return "strong_buy"
	case score >= 3.5:
		return "buy"
	case score >= 2.5:
		return "hold"
	default:
		return "sell"
	}
}
