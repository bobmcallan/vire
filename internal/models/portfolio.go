// Package models defines data structures for Vire
package models

import (
	"strings"
	"time"
)

// eodhExchange maps Navexa exchange names (e.g. "ASX", "NYSE") to EODHD
// exchange codes (e.g. "AU", "US"). Returns "AU" for empty/unknown exchanges.
func eodhExchange(exchange string) string {
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
	ID                       string    `json:"id"`
	Name                     string    `json:"name"`
	NavexaID                 string    `json:"navexa_id,omitempty"`
	Holdings                 []Holding `json:"holdings"`
	TotalValue               float64   `json:"total_value"`
	TotalCost                float64   `json:"total_cost"`
	TotalNetReturn           float64   `json:"total_net_return"`
	TotalNetReturnPct        float64   `json:"total_net_return_pct"`
	Currency                 string    `json:"currency"`
	FXRate                   float64   `json:"fx_rate,omitempty"` // AUDUSD rate used for currency conversion at sync time
	TotalRealizedNetReturn   float64   `json:"total_realized_net_return"`
	TotalUnrealizedNetReturn float64   `json:"total_unrealized_net_return"`
	CalculationMethod        string    `json:"calculation_method,omitempty"` // documents return % methodology (e.g. "average_cost")
	LastSynced               time.Time `json:"last_synced"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
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
	NetReturn           float64        `json:"net_return"`
	NetReturnPct        float64        `json:"net_return_pct"`        // Simple net return percentage (NetReturn / TotalInvested * 100)
	Weight              float64        `json:"weight"`                // Portfolio weight percentage
	TotalCost           float64        `json:"total_cost"`            // Remaining cost basis (average cost * remaining units)
	TotalInvested       float64        `json:"total_invested"`        // Sum of all buy costs + fees (total capital deployed)
	RealizedNetReturn   float64        `json:"realized_net_return"`   // P&L from sold portions
	UnrealizedNetReturn float64        `json:"unrealized_net_return"` // P&L on remaining position
	DividendReturn      float64        `json:"dividend_return"`
	CapitalGainPct      float64        `json:"capital_gain_pct"`    // XIRR annualised return (capital gains only, excl. dividends)
	NetReturnPctIRR     float64        `json:"net_return_pct_irr"`  // XIRR annualised return (including dividends)
	NetReturnPctTWRR    float64        `json:"net_return_pct_twrr"` // Time-weighted return (computed locally)
	Currency            string         `json:"currency"`            // Holding currency (AUD, USD)
	Country             string         `json:"country,omitempty"`   // Domicile country ISO code (e.g. "AU", "US")
	Trades              []*NavexaTrade `json:"trades,omitempty"`
	LastUpdated         time.Time      `json:"last_updated"`

	// Derived breakeven field — populated for open positions only (units > 0).
	// Nil for closed positions.
	TrueBreakevenPrice *float64 `json:"true_breakeven_price"`
}

// EODHDTicker returns the full EODHD-format ticker (e.g. "BHP.AU", "CBOE.US").
// Maps Navexa exchange names to EODHD codes and falls back to ".AU" if empty.
func (h Holding) EODHDTicker() string {
	return h.Ticker + "." + eodhExchange(h.Exchange)
}

// PortfolioReview contains the analysis results for a portfolio
type PortfolioReview struct {
	PortfolioName     string            `json:"portfolio_name"`
	ReviewDate        time.Time         `json:"review_date"`
	TotalValue        float64           `json:"total_value"`
	TotalCost         float64           `json:"total_cost"`
	TotalNetReturn    float64           `json:"total_net_return"`
	TotalNetReturnPct float64           `json:"total_net_return_pct"`
	DayChange         float64           `json:"day_change"`
	DayChangePct      float64           `json:"day_change_pct"`
	FXRate            float64           `json:"fx_rate,omitempty"` // AUDUSD rate used for currency conversion
	HoldingReviews    []HoldingReview   `json:"holding_reviews"`
	Alerts            []Alert           `json:"alerts"`
	Summary           string            `json:"summary"` // AI-generated summary
	Recommendations   []string          `json:"recommendations"`
	PortfolioBalance  *PortfolioBalance `json:"portfolio_balance,omitempty"`
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
	Holding             Holding              `json:"holding"`
	Signals             *TickerSignals       `json:"signals,omitempty"`
	Fundamentals        *Fundamentals        `json:"fundamentals,omitempty"`
	OvernightMove       float64              `json:"overnight_move"`
	OvernightPct        float64              `json:"overnight_pct"`
	NewsImpact          string               `json:"news_impact,omitempty"`
	NewsIntelligence    *NewsIntelligence    `json:"news_intelligence,omitempty"`
	FilingsIntelligence *FilingsIntelligence `json:"filings_intelligence,omitempty"` // deprecated
	FilingSummaries     []FilingSummary      `json:"filing_summaries,omitempty"`
	Timeline            *CompanyTimeline     `json:"timeline,omitempty"`
	ActionRequired      string               `json:"action_required"` // BUY, SELL, HOLD, WATCH
	ActionReason        string               `json:"action_reason"`
	Compliance          *ComplianceResult    `json:"compliance,omitempty"`
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
	TotalValue        float64
	TotalCost         float64
	TotalNetReturn    float64
	TotalNetReturnPct float64
}

// SnapshotHolding represents a single position within a historical portfolio snapshot.
type SnapshotHolding struct {
	Ticker, Name              string
	Units, AvgCost, TotalCost float64
	ClosePrice, MarketValue   float64
	NetReturn, NetReturnPct   float64
	Weight                    float64
}

// GrowthDataPoint represents a single point in the portfolio growth time series.
// Computed on demand from monthly snapshots — not stored.
type GrowthDataPoint struct {
	Date         time.Time
	TotalValue   float64
	TotalCost    float64
	NetReturn    float64
	NetReturnPct float64
	HoldingCount int
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
