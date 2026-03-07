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
	SourceType               SourceType          `json:"source_type,omitempty"` // navexa (default), manual, snapshot, hybrid
	Holdings                 []Holding           `json:"holdings"`
	EquityHoldingsValue      float64             `json:"equity_holdings_value"` // equity holdings only
	AssetSetsValue           float64             `json:"asset_sets_value"`      // non-equity asset sets (property, crypto, etc.)
	PortfolioValue           float64             `json:"portfolio_value"`       // holdings + available cash + asset sets
	EquityHoldingsCost       float64             `json:"equity_holdings_cost"`
	EquityHoldingsReturn     float64             `json:"equity_holdings_return"`
	EquityHoldingsReturnPct  float64             `json:"equity_holdings_return_pct"`
	Currency                 string              `json:"currency"`
	FXRate                   float64             `json:"fx_rate,omitempty"` // AUDUSD rate used for currency conversion at sync time
	EquityHoldingsRealized   float64             `json:"equity_holdings_realized"`
	EquityHoldingsUnrealized float64             `json:"equity_holdings_unrealized"`
	IncomeDividendsForecast  float64             `json:"income_dividends_forecast"`    // forecasted dividends (Navexa total minus holdings with confirmed ledger payments)
	IncomeDividendsReceived  float64             `json:"income_dividends_received"`    // confirmed dividends from cash flow ledger
	CalculationMethod        string              `json:"calculation_method,omitempty"` // documents return % methodology (e.g. "average_cost")
	DataVersion              string              `json:"data_version,omitempty"`       // schema version at save time — mismatch triggers re-sync
	CapitalGross             float64             `json:"capital_gross"`
	CapitalAvailable         float64             `json:"capital_available"`                 // capital_gross - equity_holdings_cost (uninvested cash)
	PortfolioReturn          float64             `json:"portfolio_return,omitempty"`        // portfolio_value - capital_contributions_net
	PortfolioReturnPct       float64             `json:"portfolio_return_pct,omitempty"`    // portfolio_return / capital_contributions_net × 100
	CapitalPerformance       *CapitalPerformance `json:"capital_performance,omitempty"`     // computed on response, not persisted
	IncomeDividendsNavexa    float64             `json:"income_dividends_navexa,omitempty"` // Navexa-calculated dividend return
	TradeHash                string              `json:"trade_hash,omitempty"`              // hash of trade+cash data; change triggers timeline invalidation
	LastSynced               time.Time           `json:"last_synced"`
	CreatedAt                time.Time           `json:"created_at"`
	UpdatedAt                time.Time           `json:"updated_at"`

	// Aggregate historical values — computed on response, not persisted
	PortfolioYesterdayValue     float64 `json:"portfolio_yesterday_value,omitempty"`      // Total value at yesterday's close
	PortfolioYesterdayChangePct float64 `json:"portfolio_yesterday_change_pct,omitempty"` // % change from yesterday
	PortfolioLastWeekValue      float64 `json:"portfolio_last_week_value,omitempty"`      // Total value at last week's close
	PortfolioLastWeekChangePct  float64 `json:"portfolio_last_week_change_pct,omitempty"` // % change from last week

	// Net cash flow fields — computed on response, not persisted
	NetCashYesterdayFlow float64 `json:"net_cash_yesterday_flow,omitempty"` // Net cash flow yesterday (deposits - withdrawals)
	NetCashLastWeekFlow  float64 `json:"net_cash_last_week_flow,omitempty"` // Net cash flow last 7 days

	// Change tracking — computed on response, not persisted
	Changes            *PortfolioChanges `json:"changes,omitempty"`
	Breadth            *PortfolioBreadth `json:"breadth,omitempty"`
	TimelineRebuilding bool              `json:"timeline_rebuilding,omitempty"` // true when a full timeline rebuild is in progress
}

// MetricChange tracks raw and percentage change for a single metric.
type MetricChange struct {
	Current     float64 `json:"current"`              // Current value
	Previous    float64 `json:"previous"`             // Value at reference date
	RawChange   float64 `json:"raw_change"`           // Current - Previous
	PctChange   float64 `json:"pct_change,omitempty"` // % change ((current - previous) / previous * 100)
	HasPrevious bool    `json:"has_previous"`         // True if historical data available
}

// PeriodChanges groups metric changes for a single time period.
type PeriodChanges struct {
	PortfolioValue      MetricChange `json:"portfolio_value"`       // Total portfolio (equity + cash)
	EquityHoldingsValue MetricChange `json:"equity_holdings_value"` // Market value of equity holdings
	CapitalGross        MetricChange `json:"capital_gross"`         // Cash balance
	IncomeDividends     MetricChange `json:"income_dividends"`      // Cumulative dividends received
}

// PortfolioChanges contains change tracking across multiple time periods.
// Computed on response from timeline snapshots — not persisted.
type PortfolioChanges struct {
	Yesterday PeriodChanges `json:"yesterday"` // Changes since yesterday close
	Week      PeriodChanges `json:"week"`      // Changes since 7 days ago
	Month     PeriodChanges `json:"month"`     // Changes since 30 days ago
}

// PortfolioBreadth aggregates holding trend signals into a portfolio-level breadth summary.
// Computed on response from holdings that have trend data — not persisted.
type PortfolioBreadth struct {
	// Counts by trend direction
	RisingCount  int `json:"rising_count"`
	FlatCount    int `json:"flat_count"`
	FallingCount int `json:"falling_count"`

	// Dollar-weighted proportions (0.0 to 1.0, sum to 1.0)
	RisingWeight  float64 `json:"rising_weight"`
	FlatWeight    float64 `json:"flat_weight"`
	FallingWeight float64 `json:"falling_weight"`

	// Dollar amounts by direction
	RisingValue  float64 `json:"rising_value"`
	FlatValue    float64 `json:"flat_value"`
	FallingValue float64 `json:"falling_value"`

	// Portfolio-level trend
	TrendLabel string  `json:"trend_label"` // "Strong Uptrend", "Uptrend", "Mixed", "Downtrend", "Strong Downtrend"
	TrendScore float64 `json:"trend_score"` // Dollar-weighted average of holding trend scores (-1.0 to +1.0)

	// Today's aggregate change
	TodayChange    float64 `json:"today_change"`     // Sum of (yesterday_price_change_pct / 100 * market_value) across holdings
	TodayChangePct float64 `json:"today_change_pct"` // Weighted % change
}

// Holding represents a portfolio position
type Holding struct {
	Ticker                     string         `json:"ticker"`
	Exchange                   string         `json:"exchange"`
	Name                       string         `json:"name"`
	SourceType                 SourceType     `json:"source_type,omitempty"` // navexa, manual, snapshot, csv
	SourceRef                  string         `json:"source_ref,omitempty"`  // free-form provenance tag
	Status                     string         `json:"status"`                // "open" or "closed"
	Units                      float64        `json:"units"`
	AvgCost                    float64        `json:"holding_cost_avg"`
	CurrentPrice               float64        `json:"current_price"`
	MarketValue                float64        `json:"holding_value_market"`
	ReturnNet                  float64        `json:"holding_return_net"`
	ReturnNetPct               float64        `json:"holding_return_net_pct"` // Simple net return percentage (ReturnNet / GrossInvested * 100)
	WeightPct                  float64        `json:"holding_weight_pct"`     // Portfolio weight percentage
	CostBasis                  float64        `json:"cost_basis"`             // Remaining cost basis (average cost * remaining units)
	GrossInvested              float64        `json:"gross_invested"`         // Sum of all buy costs + fees (total capital deployed)
	GrossProceeds              float64        `json:"gross_proceeds"`         // Sum of all sell proceeds (units × price − fees)
	RealizedReturn             float64        `json:"realized_return"`        // P&L from sold portions
	UnrealizedReturn           float64        `json:"unrealized_return"`      // P&L on remaining position
	DividendReturn             float64        `json:"dividend_return"`
	AnnualizedCapitalReturnPct float64        `json:"annualized_capital_return_pct"` // XIRR annualised return (capital gains only, excl. dividends)
	AnnualizedTotalReturnPct   float64        `json:"annualized_total_return_pct"`   // XIRR annualised return (including dividends)
	TimeWeightedReturnPct      float64        `json:"time_weighted_return_pct"`      // Time-weighted return (computed locally)
	Currency                   string         `json:"currency"`                      // Holding currency (AUD, USD) — converted to portfolio base currency after FX
	OriginalCurrency           string         `json:"original_currency,omitempty"`   // Native currency before FX conversion (set only when converted)
	Country                    string         `json:"country,omitempty"`             // Domicile country ISO code (e.g. "AU", "US")
	Trades                     []*NavexaTrade `json:"trades,omitempty"`
	LastUpdated                time.Time      `json:"last_updated"`

	// Derived breakeven field — populated for open positions only (units > 0).
	// Nil for closed positions.
	TrueBreakevenPrice *float64 `json:"true_breakeven_price"`

	// Historical values — computed on response, not persisted
	YesterdayClosePrice     float64 `json:"yesterday_close_price,omitempty"`       // Previous trading day close (AUD)
	YesterdayPriceChangePct float64 `json:"yesterday_price_change_pct,omitempty"`  // % change from yesterday to today
	LastWeekClosePrice      float64 `json:"last_week_close_price,omitempty"`       // Last Friday close (AUD)
	LastWeekPriceChangePct  float64 `json:"last_week_price_change_pct,omitempty"`  // % change from last week to today
	LastMonthClosePrice     float64 `json:"last_month_close_price,omitempty"`      // ~22 trading days ago close (AUD)
	LastMonthPriceChangePct float64 `json:"last_month_price_change_pct,omitempty"` // % change from last month to today
	TrendLabel              string  `json:"trend_label,omitempty"`                 // "Strong Uptrend", "Uptrend", "Consolidating", "Downtrend", "Strong Downtrend"
	TrendScore              float64 `json:"trend_score,omitempty"`                 // -1.0 to +1.0 from signal engine
}

// EODHDTicker returns the full EODHD-format ticker (e.g. "BHP.AU", "CBOE.US").
// Maps Navexa exchange names to EODHD codes and falls back to ".AU" if empty.
func (h Holding) EODHDTicker() string {
	return h.Ticker + "." + EodhExchange(h.Exchange)
}

// PortfolioReview contains the analysis results for a portfolio
type PortfolioReview struct {
	PortfolioName           string               `json:"portfolio_name"`
	ReviewDate              time.Time            `json:"review_date"`
	PortfolioValue          float64              `json:"portfolio_value"`
	EquityHoldingsCost      float64              `json:"equity_holdings_cost"`
	EquityHoldingsReturn    float64              `json:"equity_holdings_return"`
	EquityHoldingsReturnPct float64              `json:"equity_holdings_return_pct"`
	PortfolioDayChange      float64              `json:"portfolio_day_change"`
	PortfolioDayChangePct   float64              `json:"portfolio_day_change_pct"`
	FXRate                  float64              `json:"fx_rate,omitempty"` // AUDUSD rate used for currency conversion
	HoldingReviews          []HoldingReview      `json:"holding_reviews"`
	Alerts                  []Alert              `json:"alerts"`
	Summary                 string               `json:"summary"` // AI-generated summary
	Recommendations         []string             `json:"recommendations"`
	PortfolioBalance        *PortfolioBalance    `json:"portfolio_balance,omitempty"`
	PortfolioIndicators     *PortfolioIndicators `json:"portfolio_indicators,omitempty"`
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
	HoldingNote      *HoldingNote      `json:"holding_note,omitempty"`      // Analyst context note
	SignalConfidence SignalConfidence  `json:"signal_confidence,omitempty"` // high/medium/low based on asset type
	NoteStale        bool              `json:"note_stale,omitempty"`        // True if note needs review
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
	PortfolioName           string
	AsOfDate                time.Time
	PriceDate               time.Time // actual trading day used for prices (may differ on weekends/holidays)
	Holdings                []SnapshotHolding
	EquityHoldingsValue     float64
	EquityHoldingsCost      float64
	EquityHoldingsReturn    float64
	EquityHoldingsReturnPct float64
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
	Date                    time.Time
	EquityHoldingsValue     float64
	EquityHoldingsCost      float64
	EquityHoldingsReturn    float64
	EquityHoldingsReturnPct float64
	HoldingCount            int
	CapitalGross            float64 // Sum of all cash transactions (no trade settlements)
	CapitalAvailable        float64 // Gross cash - equity purchases + sell proceeds (uninvested cash)
	AssetSetsValue          float64 // Non-equity asset set values (property, crypto, etc.)
	PortfolioValue          float64 // EquityHoldingsValue + CapitalAvailable + AssetSetsValue
	CapitalContributionsNet float64 // Cumulative contributions to date
}

// StockTimelinePoint represents a single point in a per-stock value time series.
// Computed on demand from trade replay and EOD prices — not stored.
type StockTimelinePoint struct {
	Date         time.Time `json:"date"`
	Units        float64   `json:"units"`
	CostBasis    float64   `json:"cost_basis"`
	ClosePrice   float64   `json:"close_price"`
	MarketValue  float64   `json:"market_value"`
	NetReturn    float64   `json:"net_return"`
	NetReturnPct float64   `json:"net_return_pct"`
}

// TimeSeriesPoint represents a single point in the daily portfolio value time series.
type TimeSeriesPoint struct {
	Date                    time.Time `json:"date"`
	EquityHoldingsValue     float64   `json:"equity_holdings_value"`
	EquityHoldingsCost      float64   `json:"equity_holdings_cost"`
	EquityHoldingsReturn    float64   `json:"equity_holdings_return"`
	EquityHoldingsReturnPct float64   `json:"equity_holdings_return_pct"`
	HoldingCount            int       `json:"holding_count"`
	CapitalGross            float64   `json:"capital_gross,omitempty"`
	CapitalAvailable        float64   `json:"capital_available,omitempty"`
	AssetSetsValue          float64   `json:"asset_sets_value,omitempty"`
	PortfolioValue          float64   `json:"portfolio_value,omitempty"`
	CapitalContributionsNet float64   `json:"capital_contributions_net,omitempty"`
}

// PortfolioIndicators contains technical indicators computed on the daily portfolio value time series.
type PortfolioIndicators struct {
	PortfolioName  string    `json:"portfolio_name"`
	ComputeDate    time.Time `json:"compute_date"`
	PortfolioValue float64   `json:"portfolio_value"`
	DataPoints     int       `json:"data_points"`

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

// TimelineSnapshot represents a persisted daily portfolio value snapshot.
// One row per portfolio per day, stored in the portfolio_timeline table.
// Today's row is overwritten on each SyncPortfolio call as intraday prices update.
type TimelineSnapshot struct {
	UserID        string    `json:"user_id"`
	PortfolioName string    `json:"portfolio_name"`
	Date          time.Time `json:"date"`

	// Equity
	EquityHoldingsValue     float64 `json:"equity_holdings_value"`
	EquityHoldingsCost      float64 `json:"equity_holdings_cost"`
	EquityHoldingsReturn    float64 `json:"equity_holdings_return"`
	EquityHoldingsReturnPct float64 `json:"equity_holdings_return_pct"`
	HoldingCount            int     `json:"holding_count"`

	// Asset sets
	AssetSetsValue float64 `json:"asset_sets_value,omitempty"` // non-equity asset set values

	// Cash
	CapitalGross            float64 `json:"capital_gross"`
	CapitalAvailable        float64 `json:"capital_available"`
	PortfolioValue          float64 `json:"portfolio_value"`
	CapitalContributionsNet float64 `json:"capital_contributions_net"`

	// Cumulative dividend tracking
	IncomeDividendsCumulative float64 `json:"income_dividends_cumulative,omitempty"` // Total dividends received up to this date

	// Metadata
	FXRate      float64   `json:"fx_rate,omitempty"`
	DataVersion string    `json:"data_version"`
	ComputedAt  time.Time `json:"computed_at"`
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
