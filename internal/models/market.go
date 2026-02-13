// Package models defines data structures for Vire
package models

import (
	"time"
)

// RealTimeQuote holds a live OHLCV snapshot from a real-time price source
type RealTimeQuote struct {
	Code          string    `json:"code"`
	Open          float64   `json:"open"`
	High          float64   `json:"high"`
	Low           float64   `json:"low"`
	Close         float64   `json:"close"`          // current/last price
	PreviousClose float64   `json:"previous_close"` // previous day's close
	Change        float64   `json:"change"`         // absolute change from previous close
	ChangePct     float64   `json:"change_p"`       // percentage change from previous close
	Volume        int64     `json:"volume"`
	Timestamp     time.Time `json:"timestamp"`
	Source        string    `json:"source,omitempty"` // "eodhd" or "asx"
}

// MarketData holds all market data for a ticker
type MarketData struct {
	Ticker           string            `json:"ticker"`
	Exchange         string            `json:"exchange"`
	Name             string            `json:"name"`
	EOD              []EODBar          `json:"eod"`
	Fundamentals     *Fundamentals     `json:"fundamentals,omitempty"`
	News             []*NewsItem       `json:"news,omitempty"`
	LastUpdated      time.Time         `json:"last_updated"`
	NewsIntelligence *NewsIntelligence `json:"news_intelligence,omitempty"`
	// Filings data
	Filings             []CompanyFiling      `json:"filings,omitempty"`
	FilingsIntelligence *FilingsIntelligence `json:"filings_intelligence,omitempty"`
	// Per-component freshness timestamps
	EODUpdatedAt          time.Time `json:"eod_updated_at"`
	FundamentalsUpdatedAt time.Time `json:"fundamentals_updated_at"`
	NewsUpdatedAt         time.Time `json:"news_updated_at"`
	NewsIntelUpdatedAt    time.Time `json:"news_intel_updated_at"`
	FilingsUpdatedAt      time.Time `json:"filings_updated_at"`
	FilingsIntelUpdatedAt time.Time `json:"filings_intel_updated_at"`
}

// EODBar represents a single day's price data
type EODBar struct {
	Date     time.Time `json:"date"`
	Open     float64   `json:"open"`
	High     float64   `json:"high"`
	Low      float64   `json:"low"`
	Close    float64   `json:"close"`
	AdjClose float64   `json:"adjusted_close"`
	Volume   int64     `json:"volume"`
}

// Fundamentals contains fundamental data for a stock or ETF
type Fundamentals struct {
	Ticker            string    `json:"ticker"`
	MarketCap         float64   `json:"market_cap"`
	PE                float64   `json:"pe_ratio"`
	PB                float64   `json:"pb_ratio"`
	EPS               float64   `json:"eps"`
	DividendYield     float64   `json:"dividend_yield"`
	Beta              float64   `json:"beta"`
	SharesOutstanding int64     `json:"shares_outstanding"`
	SharesFloat       int64     `json:"shares_float"`
	Sector            string    `json:"sector"`
	Industry          string    `json:"industry"`
	CountryISO        string    `json:"country_iso,omitempty"` // Domicile country ISO 2-letter code derived from ISIN (e.g., "US", "AU", "CN")
	ISIN              string    `json:"isin,omitempty"`        // Full ISIN; prefix = domicile country
	Description       string    `json:"description,omitempty"`
	LastUpdated       time.Time `json:"last_updated"`
	// ETF-specific fields
	IsETF            bool            `json:"is_etf"`
	ExpenseRatio     float64         `json:"expense_ratio,omitempty"`
	ManagementStyle  string          `json:"management_style,omitempty"` // Passive, Active
	AnnualisedReturn float64         `json:"annualised_return,omitempty"`
	TopHoldings      []ETFHolding    `json:"top_holdings,omitempty"`
	SectorWeights    []SectorWeight  `json:"sector_weights,omitempty"`
	CountryWeights   []CountryWeight `json:"country_weights,omitempty"`
	WebURL           string          `json:"web_url,omitempty"`
	EnrichedAt       time.Time       `json:"enriched_at,omitempty"`
}

// ETFHolding represents a holding within an ETF
type ETFHolding struct {
	Name   string  `json:"name"`
	Ticker string  `json:"ticker,omitempty"`
	Weight float64 `json:"weight"` // Percentage weight
}

// SectorWeight represents sector allocation in an ETF
type SectorWeight struct {
	Sector string  `json:"sector"`
	Weight float64 `json:"weight"`
}

// CountryWeight represents country allocation in an ETF
type CountryWeight struct {
	Country string  `json:"country"`
	Weight  float64 `json:"weight"`
}

// NewsItem represents a news article
type NewsItem struct {
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	Source      string    `json:"source"`
	PublishedAt time.Time `json:"published_at"`
	Sentiment   string    `json:"sentiment,omitempty"` // positive, negative, neutral
	Summary     string    `json:"summary,omitempty"`
}

// StockData combines all data for a stock
type StockData struct {
	Ticker              string               `json:"ticker"`
	Exchange            string               `json:"exchange"`
	Name                string               `json:"name"`
	Price               *PriceData           `json:"price,omitempty"`
	Fundamentals        *Fundamentals        `json:"fundamentals,omitempty"`
	Signals             *TickerSignals       `json:"signals,omitempty"`
	News                []*NewsItem          `json:"news,omitempty"`
	NewsIntelligence    *NewsIntelligence    `json:"news_intelligence,omitempty"`
	Filings             []CompanyFiling      `json:"filings,omitempty"`
	FilingsIntelligence *FilingsIntelligence `json:"filings_intelligence,omitempty"`
}

// PriceData contains current price information
type PriceData struct {
	Current       float64   `json:"current"`
	Open          float64   `json:"open"`
	High          float64   `json:"high"`
	Low           float64   `json:"low"`
	PreviousClose float64   `json:"previous_close"`
	Change        float64   `json:"change"`
	ChangePct     float64   `json:"change_pct"`
	Volume        int64     `json:"volume"`
	AvgVolume     int64     `json:"avg_volume"`
	High52Week    float64   `json:"high_52_week"`
	Low52Week     float64   `json:"low_52_week"`
	LastUpdated   time.Time `json:"last_updated"`
}

// NewsIntelligence contains AI-analyzed news summary for a ticker
type NewsIntelligence struct {
	Summary          string            `json:"summary"`
	OverallSentiment string            `json:"overall_sentiment"` // bullish, bearish, neutral, mixed
	KeyThemes        []string          `json:"key_themes"`
	ImpactWeek       string            `json:"impact_week"`
	ImpactMonth      string            `json:"impact_month"`
	ImpactYear       string            `json:"impact_year"`
	Articles         []AnalyzedArticle `json:"articles"`
	GeneratedAt      time.Time         `json:"generated_at"`
}

// AnalyzedArticle represents an AI-assessed news article
type AnalyzedArticle struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Source      string `json:"source"`
	Credibility string `json:"credibility"` // credible, fluff, promotional, speculative
	Relevance   string `json:"relevance"`   // high, medium, low
	Summary     string `json:"summary"`
}

// SnipeBuy represents a potential turnaround buy candidate
type SnipeBuy struct {
	Ticker      string         `json:"ticker"`
	Exchange    string         `json:"exchange"`
	Name        string         `json:"name"`
	Score       float64        `json:"score"` // 0.0 - 1.0
	Price       float64        `json:"price"`
	TargetPrice float64        `json:"target_price"`
	UpsidePct   float64        `json:"upside_pct"`
	Signals     *TickerSignals `json:"signals,omitempty"`
	Reasons     []string       `json:"reasons"`
	RiskFactors []string       `json:"risk_factors"`
	Sector      string         `json:"sector"`
	Analysis    string         `json:"analysis,omitempty"` // AI analysis
}

// ScreenCandidate represents a stock passing the quality-value screen
type ScreenCandidate struct {
	Ticker           string         `json:"ticker"`
	Exchange         string         `json:"exchange"`
	Name             string         `json:"name"`
	Score            float64        `json:"score"` // 0.0 - 1.0
	Price            float64        `json:"price"`
	PE               float64        `json:"pe_ratio"`
	EPS              float64        `json:"eps"`
	DividendYield    float64        `json:"dividend_yield"`
	MarketCap        float64        `json:"market_cap"`
	Sector           string         `json:"sector"`
	Industry         string         `json:"industry"`
	QuarterlyReturns []float64      `json:"quarterly_returns"` // last 3 quarters annualised %
	AvgQtrReturn     float64        `json:"avg_quarterly_return"`
	Signals          *TickerSignals `json:"signals,omitempty"`
	NewsSentiment    string         `json:"news_sentiment"`   // bullish, bearish, neutral, mixed
	NewsCredibility  string         `json:"news_credibility"` // high, mixed, low
	Strengths        []string       `json:"strengths"`
	Concerns         []string       `json:"concerns"`
	Analysis         string         `json:"analysis,omitempty"` // AI analysis
}

// Symbol represents an exchange symbol
type Symbol struct {
	Code     string `json:"Code"`
	Name     string `json:"Name"`
	Country  string `json:"Country"`
	Exchange string `json:"Exchange"`
	Currency string `json:"Currency"`
	Type     string `json:"Type"`
}

// EODResponse represents the EODHD API response
type EODResponse struct {
	Data []EODBar `json:"data"`
}

// BulkEODBar represents a single day's price data from the bulk API.
// The bulk API returns data with a "code" field identifying the ticker.
type BulkEODBar struct {
	Code     string    `json:"code"`
	Date     time.Time `json:"date"`
	Open     float64   `json:"open"`
	High     float64   `json:"high"`
	Low      float64   `json:"low"`
	Close    float64   `json:"close"`
	AdjClose float64   `json:"adjusted_close"`
	Volume   int64     `json:"volume"`
}

// TechnicalResponse represents EODHD technical indicators response
type TechnicalResponse struct {
	Data map[string]interface{} `json:"data"`
}

// CompanyFiling represents a single ASX announcement/filing
type CompanyFiling struct {
	Date           time.Time `json:"date"`
	Headline       string    `json:"headline"`
	Type           string    `json:"type"` // "Annual Report", "Quarterly Report", "Dividend", etc.
	PDFURL         string    `json:"pdf_url,omitempty"`
	DocumentKey    string    `json:"document_key,omitempty"`
	PriceSensitive bool      `json:"price_sensitive"`
	Relevance      string    `json:"relevance"`          // HIGH, MEDIUM, LOW, NOISE
	PDFPath        string    `json:"pdf_path,omitempty"` // Local filesystem path
}

// FilingsIntelligence contains AI-analyzed company filings summary
type FilingsIntelligence struct {
	Summary           string              `json:"summary"`
	FinancialHealth   string              `json:"financial_health"` // strong, stable, concerning, weak
	GrowthOutlook     string              `json:"growth_outlook"`   // positive, neutral, negative
	CanSupport10PctPA bool                `json:"can_support_10pct_pa"`
	GrowthRationale   string              `json:"growth_rationale"`
	KeyMetrics        []FilingMetric      `json:"key_metrics,omitempty"`
	YearOverYear      []YearOverYearEntry `json:"year_over_year,omitempty"`
	StrategyNotes     string              `json:"strategy_notes,omitempty"`
	RiskFactors       []string            `json:"risk_factors,omitempty"`
	PositiveFactors   []string            `json:"positive_factors,omitempty"`
	FilingsAnalyzed   int                 `json:"filings_analyzed"`
	GeneratedAt       time.Time           `json:"generated_at"`
}

// FilingMetric represents a key financial metric extracted from filings
type FilingMetric struct {
	Name   string `json:"name"`   // e.g. "Revenue", "Net Income", "EBITDA"
	Value  string `json:"value"`  // Formatted string, e.g. "$1.2B"
	Period string `json:"period"` // e.g. "FY2025", "H1 2025"
	Trend  string `json:"trend"`  // up, down, flat
}

// YearOverYearEntry tracks year-on-year changes
type YearOverYearEntry struct {
	Period     string `json:"period"`  // e.g. "FY2025 vs FY2024"
	Revenue    string `json:"revenue"` // e.g. "+12.3%"
	Profit     string `json:"profit"`  // e.g. "-5.1%"
	Outlook    string `json:"outlook"` // improved, stable, deteriorated
	KeyChanges string `json:"key_changes"`
}

// ScreenerFilter represents a single filter for the EODHD Screener API.
// Each filter is a 3-element array: [field, operator, value].
type ScreenerFilter struct {
	Field    string      `json:"field"`
	Operator string      `json:"operator"`
	Value    interface{} `json:"value"`
}

// ScreenerOptions configures an EODHD Screener API call
type ScreenerOptions struct {
	Filters []ScreenerFilter `json:"filters"`
	Signals []string         `json:"signals,omitempty"` // e.g. "50d_new_hi", "bookvalue_pos"
	Sort    string           `json:"sort,omitempty"`    // e.g. "market_capitalization.desc"
	Limit   int              `json:"limit,omitempty"`   // 1-100
	Offset  int              `json:"offset,omitempty"`  // 0-999
}

// ScreenerResult represents a single result from the EODHD Screener API
type ScreenerResult struct {
	Code           string  `json:"code"`
	Name           string  `json:"name"`
	Exchange       string  `json:"exchange"`
	Sector         string  `json:"sector"`
	Industry       string  `json:"industry"`
	MarketCap      float64 `json:"market_capitalization"`
	EarningsShare  float64 `json:"earnings_share"`
	DividendYield  float64 `json:"dividend_yield"`
	AdjustedClose  float64 `json:"adjusted_close"`
	CurrencySymbol string  `json:"currency_symbol"`
	Refund1dPct    float64 `json:"refund_1d_p"`
	Refund5dPct    float64 `json:"refund_5d_p"`
	AvgVol200d     float64 `json:"avgvol_200d"`
}

// FunnelResult holds the output of a multi-stage funnel screen
type FunnelResult struct {
	Candidates []*ScreenCandidate `json:"candidates"`
	Stages     []FunnelStage      `json:"stages"`
	Exchange   string             `json:"exchange"`
	Sector     string             `json:"sector,omitempty"`
	Duration   time.Duration      `json:"duration"`
}

// FunnelStage records what happened at each funnel stage
type FunnelStage struct {
	Name        string        `json:"name"`
	InputCount  int           `json:"input_count"`
	OutputCount int           `json:"output_count"`
	Duration    time.Duration `json:"duration"`
	Filters     string        `json:"filters,omitempty"` // human-readable description
}

// SearchRecord stores a screen/snipe/funnel search result for history
type SearchRecord struct {
	ID           string    `json:"id"`
	Type         string    `json:"type"` // "screen", "snipe", "funnel"
	Exchange     string    `json:"exchange"`
	Filters      string    `json:"filters"` // JSON of filters applied
	ResultCount  int       `json:"result_count"`
	Results      string    `json:"results"`          // JSON of results ([]ScreenCandidate or []SnipeBuy)
	Stages       string    `json:"stages,omitempty"` // JSON of funnel stages (for funnel type)
	StrategyName string    `json:"strategy_name,omitempty"`
	StrategyVer  int       `json:"strategy_version,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}
