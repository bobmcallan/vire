// Package models defines data structures for Vire
package models

import (
	"strings"
	"time"
)

// EodhExchange maps Navexa exchange names (e.g. "ASX", "NYSE") to EODHD
// exchange codes (e.g. "AU", "US"). Returns "AU" for empty/unknown exchanges.
func EodhExchange(exchange string) string {
	switch strings.ToUpper(exchange) {
	case "ASX", "AU":
		return "AU"
	case "NYSE", "NASDAQ", "US", "BATS", "AMEX", "ARCA":
		return "US"
	case "LSE", "LON":
		return "LSE"
	case "":
		return "AU"
	default:
		return exchange
	}
}

// ComplianceStatus indicates whether a holding complies with the portfolio strategy
type ComplianceStatus string

const (
	ComplianceStatusCompliant    ComplianceStatus = "compliant"
	ComplianceStatusNonCompliant ComplianceStatus = "non_compliant"
	ComplianceStatusUnknown      ComplianceStatus = "unknown"
)

// ComplianceResult captures per-holding compliance with the portfolio strategy
type ComplianceResult struct {
	Status   ComplianceStatus `json:"status"`
	Reasons  []string         `json:"reasons,omitempty"`
	RuleHits []string         `json:"rule_hits,omitempty"` // which rule names triggered
}

// Portfolio represents a stock portfolio
type Portfolio struct {
	ID                       string              `json:"id"`
	Name                     string              `json:"name"`
	NavexaID                 string              `json:"navexa_id,omitempty"`
	Holdings                 []Holding           `json:"holdings"`
	EquityValue              float64             `json:"equity_value"`              // equity holdings only
	PortfolioValue           float64             `json:"portfolio_value"`           // holdings + available cash
	NetEquityCost            float64             `json:"net_equity_cost"`
	NetEquityReturn          float64             `json:"net_equity_return"`
	NetEquityReturnPct       float64             `json:"net_equity_return_pct"`
	Currency                 string              `json:"currency"`
	FXRate                   float64             `json:"fx_rate,omitempty"` // AUDUSD rate used for currency conversion at sync time
	RealizedEquityReturn     float64             `json:"realized_equity_return"`
	UnrealizedEquityReturn   float64             `json:"unrealized_equity_return"`
	CalculationMethod        string              `json:"calculation_method,omitempty"` // documents return % methodology (e.g. "average_cost")
	DataVersion              string              `json:"data_version,omitempty"`       // schema version at save time — mismatch triggers re-sync
	GrossCashBalance         float64             `json:"gross_cash_balance"`
	NetCashBalance           float64             `json:"net_cash_balance"`              // gross_cash_balance - net_equity_cost (uninvested cash)
	NetCapitalReturn         float64             `json:"net_capital_return,omitempty"`  // portfolio_value - net_capital_deployed
	NetCapitalReturnPct      float64             `json:"net_capital_return_pct,omitempty"` // net_capital_return / net_capital_deployed × 100
	CapitalPerformance       *CapitalPerformance `json:"capital_performance,omitempty"`    // computed on response, not persisted
	LastSynced               time.Time           `json:"last_synced"`
	CreatedAt                time.Time           `json:"created_at"`
	UpdatedAt                time.Time           `json:"updated_at"`

	// Aggregate historical values — computed on response, not persisted
	PortfolioYesterdayValue      float64 `json:"portfolio_yesterday_value,omitempty"`      // Total value at yesterday's close
	PortfolioYesterdayChangePct  float64 `json:"portfolio_yesterday_change_pct,omitempty"` // % change from yesterday
	PortfolioLastWeekValue       float64 `json:"portfolio_last_week_value,omitempty"`      // Total value at last week's close
	PortfolioLastWeekChangePct   float64 `json:"portfolio_last_week_change_pct,omitempty"` // % change from last week

	// Net cash flow fields — computed on response, not persisted
	NetCashYesterdayFlow float64 `json:"net_cash_yesterday_flow,omitempty"` // Net cash flow yesterday (deposits - withdrawals)
	NetCashLastWeekFlow  float64 `json:"net_cash_last_week_flow,omitempty"` // Net cash flow last 7 days
}

// Holding represents a portfolio position
type Holding struct {
	Ticker              string         `json:"ticker"`
	Exchange            string         `json:"exchange"`
	Name                string         `json:"name"`
	Units               float64        `json:"units"`
	AvgCost             float64        `json:"avg_cost"`
	CurrentPrice        float64        `json:"current_price"`
	MarketValue         float64        `json:"market_value"`
	NetReturn                   float64        `json:"net_return"`
	NetReturnPct                float64        `json:"net_return_pct"`        // Simple net return percentage (NetReturn / GrossInvested * 100)
	PortfolioWeightPct          float64        `json:"portfolio_weight_pct"`  // Portfolio weight percentage
	CostBasis                   float64        `json:"cost_basis"`            // Remaining cost basis (average cost * remaining units)
	GrossInvested               float64        `json:"gross_invested"`        // Sum of all buy costs + fees (total capital deployed)
	GrossProceeds               float64        `json:"gross_proceeds"`        // Sum of all sell proceeds (units × price − fees)
	RealizedReturn              float64        `json:"realized_return"`       // P&L from sold portions
	UnrealizedReturn            float64        `json:"unrealized_return"`     // P&L on remaining position
	DividendReturn              float64        `json:"dividend_return"`
	AnnualizedCapitalReturnPct  float64        `json:"annualized_capital_return_pct"` // XIRR annualised return (capital gains only, excl. dividends)
	AnnualizedTotalReturnPct    float64        `json:"annualized_total_return_pct"`   // XIRR annualised return (including dividends)
	TimeWeightedReturnPct       float64        `json:"time_weighted_return_pct"`      // Time-weighted return (computed locally)
	Currency            string         `json:"currency"`                    // Holding currency (AUD, USD) — converted to portfolio base currency after FX
	OriginalCurrency    string         `json:"original_currency,omitempty"` // Native currency before FX conversion (set only when converted)
	Country             string         `json:"country,omitempty"`           // Domicile country ISO code (e.g. "AU", "US")
	Trades              []*NavexaTrade `json:"trades,omitempty"`
	LastUpdated         time.Time      `json:"last_updated"`

	// Derived breakeven field — populated for open positions only (units > 0).
	// Nil for closed positions.
	TrueBreakevenPrice *float64 `json:"true_breakeven_price"`

	// Historical values — computed on response, not persisted
	YesterdayClosePrice    float64 `json:"yesterday_close_price,omitempty"`    // Previous trading day close (AUD)
	YesterdayPriceChangePct float64 `json:"yesterday_price_change_pct,omitempty"` // % change from yesterday to today
	LastWeekClosePrice     float64 `json:"last_week_close_price,omitempty"`    // Last Friday close (AUD)
	LastWeekPriceChangePct  float64 `json:"last_week_price_change_pct,omitempty"` // % change from last week to today
}

// EODHDTicker returns the full EODHD-format ticker (e.g. "BHP.AU", "CBOE.US").
// Maps Navexa exchange names to EODHD codes and falls back to ".AU" if empty.
func (h Holding) EODHDTicker() string {
	return h.Ticker + "." + EodhExchange(h.Exchange)
}

// PortfolioReview contains the analysis results for a portfolio
type PortfolioReview struct {
	PortfolioName       string               `json:"portfolio_name"`
	ReviewDate          time.Time            `json:"review_date"`
	PortfolioValue      float64              `json:"portfolio_value"`
	NetEquityCost       float64              `json:"net_equity_cost"`
	NetEquityReturn     float64              `json:"net_equity_return"`
	NetEquityReturnPct  float64              `json:"net_equity_return_pct"`
	PortfolioDayChange  float64              `json:"portfolio_day_change"`
	PortfolioDayChangePct float64            `json:"portfolio_day_change_pct"`
	FXRate              float64              `json:"fx_rate,omitempty"` // AUDUSD rate used for currency conversion
	HoldingReviews      []HoldingReview      `json:"holding_reviews"`
	Alerts              []Alert              `json:"alerts"`
	Summary             string               `json:"summary"` // AI-generated summary
	Recommendations     []string             `json:"recommendations"`
	PortfolioBalance    *PortfolioBalance    `json:"portfolio_balance,omitempty"`
	PortfolioIndicators *PortfolioIndicators `json:"portfolio_indicators,omitempty"`
}

// PortfolioBalance contains sector/industry allocation analysis
type PortfolioBalance struct {
	SectorAllocations   []SectorAllocation `json:"sector_allocations"`
	DefensiveWeight     float64            `json:"defensive_weight"`     // % in defensive sectors
	GrowthWeight        float64            `json:"growth_weight"`        // % in growth sectors
	IncomeWeight        float64            `json:"income_weight"`        // % in high-dividend stocks
	ConcentrationRisk   string             `json:"concentration_risk"`   // low, medium, high
	DiversificationNote string             `json:"diversification_note"` // Analysis note
}

// SectorAllocation represents allocation to a sector
type SectorAllocation struct {
	Sector   string   `json:"sector"`
	Weight   float64  `json:"weight"`
	Holdings []string `json:"holdings"`
}

// HoldingReview contains the analysis for a single holding
type HoldingReview struct {
	Holding          Holding           `json:"holding"`
	Signals          *TickerSignals    `json:"signals,omitempty"`
	Fundamentals     *Fundamentals     `json:"fundamentals,omitempty"`
	OvernightMove    float64           `json:"overnight_move"`
	OvernightPct     float64           `json:"overnight_pct"`
	NewsImpact       string            `json:"news_impact,omitempty"`
	NewsIntelligence *NewsIntelligence `json:"news_intelligence,omitempty"`
	FilingSummaries  []FilingSummary   `json:"filing_summaries,omitempty"`
	Timeline         *CompanyTimeline  `json:"timeline,omitempty"`
	ActionRequired   string            `json:"action_required"` // BUY, SELL, HOLD, WATCH
	ActionReason     string            `json:"action_reason"`
	Compliance       *ComplianceResult `json:"compliance,omitempty"`
}

// Alert represents a portfolio alert
type Alert struct {
	Type     AlertType `json:"type"`
	Severity string    `json:"severity"` // high, medium, low
	Ticker   string    `json:"ticker,omitempty"`
	Message  string    `json:"message"`
	Signal   string    `json:"signal,omitempty"`
}

// PortfolioSnapshot represents the reconstructed state of a portfolio at a historical date.
// Computed on demand from trade history and EOD prices — not stored.
type PortfolioSnapshot struct {
	PortfolioName     string
	AsOfDate          time.Time
	PriceDate         time.Time // actual trading day used for prices (may differ on weekends/holidays)
	Holdings          []SnapshotHolding
	EquityValue        float64
	NetEquityCost      float64
	NetEquityReturn    float64
	NetEquityReturnPct float64
}

// SnapshotHolding represents a single position within a historical portfolio snapshot.
type SnapshotHolding struct {
	Ticker, Name              string
	Units, AvgCost, CostBasis float64
	ClosePrice, MarketValue   float64
	NetReturn, NetReturnPct   float64
	Weight                    float64
}

// GrowthDataPoint represents a single point in the portfolio growth time series.
// Computed on demand from monthly snapshots — not stored.
type GrowthDataPoint struct {
	Date                time.Time
	EquityValue         float64
	NetEquityCost       float64
	NetEquityReturn     float64
	NetEquityReturnPct  float64
	HoldingCount        int
	GrossCashBalance    float64 // Running cash balance as of this date
	PortfolioValue      float64 // EquityValue + GrossCashBalance
	NetCapitalDeployed  float64 // Cumulative deposits - withdrawals to date
}

// TimeSeriesPoint represents a single point in the daily portfolio value time series.
type TimeSeriesPoint struct {
	Date               time.Time `json:"date"`
	EquityValue        float64   `json:"equity_value"`
	NetEquityCost      float64   `json:"net_equity_cost"`
	NetEquityReturn    float64   `json:"net_equity_return"`
	NetEquityReturnPct float64   `json:"net_equity_return_pct"`
	HoldingCount       int       `json:"holding_count"`
	GrossCashBalance   float64   `json:"gross_cash_balance,omitempty"`
	NetCashBalance     float64   `json:"net_cash_balance,omitempty"`
	PortfolioValue     float64   `json:"portfolio_value,omitempty"`
	NetCapitalDeployed float64   `json:"net_capital_deployed,omitempty"`
}

// PortfolioIndicators contains technical indicators computed on the daily portfolio value time series.
type PortfolioIndicators struct {
	PortfolioName string    `json:"portfolio_name"`
	ComputeDate   time.Time `json:"compute_date"`
	PortfolioValue float64   `json:"portfolio_value"`
	DataPoints    int       `json:"data_points"`

	// Moving Averages
	EMA20       float64 `json:"ema_20"`
	EMA50       float64 `json:"ema_50"`
	EMA200      float64 `json:"ema_200"`
	AboveEMA20  bool    `json:"above_ema_20"`
	AboveEMA50  bool    `json:"above_ema_50"`
	AboveEMA200 bool    `json:"above_ema_200"`

	// RSI
	RSI       float64 `json:"rsi"`
	RSISignal string  `json:"rsi_signal"`

	// Crossovers
	EMA50CrossEMA200 string `json:"ema_50_cross_200"`

	// Trend
	Trend            TrendType `json:"trend"`
	TrendDescription string    `json:"trend_description"`

}

// AlertType categorizes alerts
type AlertType string

const (
	AlertTypeSignal   AlertType = "signal"
	AlertTypePrice    AlertType = "price"
	AlertTypeNews     AlertType = "news"
	AlertTypeVolume   AlertType = "volume"
	AlertTypeRisk     AlertType = "risk"
	AlertTypeStrategy AlertType = "strategy"
)
