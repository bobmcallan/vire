package models

import (
	"sort"
	"time"
)

// SourceType identifies the data origin for a portfolio or holding.
type SourceType string

const (
	SourceNavexa   SourceType = "navexa"
	SourceManual   SourceType = "manual"
	SourceSnapshot SourceType = "snapshot"
	SourceCSV      SourceType = "csv"
	SourceHybrid   SourceType = "hybrid"
)

// ValidPortfolioSourceTypes are the valid source types for creating portfolios.
var ValidPortfolioSourceTypes = map[SourceType]bool{
	SourceManual:   true,
	SourceSnapshot: true,
	SourceHybrid:   true,
}

// TradeAction represents a buy or sell action.
type TradeAction string

const (
	TradeActionBuy  TradeAction = "buy"
	TradeActionSell TradeAction = "sell"
)

// Trade represents a single buy or sell transaction.
type Trade struct {
	ID            string      `json:"id"` // Auto-generated "tr_" + 8 hex chars
	PortfolioName string      `json:"portfolio_name"`
	Ticker        string      `json:"ticker"` // e.g. "BHP.AU"
	Action        TradeAction `json:"action"` // "buy" or "sell"
	Units         float64     `json:"units"`
	Price         float64     `json:"price"`                 // per unit, excluding fees
	Fees          float64     `json:"fees"`                  // brokerage / commission
	Date          time.Time   `json:"date"`                  // trade date
	SettleDate    string      `json:"settle_date,omitempty"` // settlement date (optional)
	SourceType    SourceType  `json:"source_type,omitempty"` // "manual", "snapshot", "csv"
	SourceRef     string      `json:"source_ref,omitempty"`  // free-form provenance tag
	Notes         string      `json:"notes,omitempty"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
}

// Consideration returns the total cost/proceeds for this trade (units × price ± fees).
// Buy: units × price + fees (cost). Sell: units × price - fees (proceeds).
func (t Trade) Consideration() float64 {
	base := t.Units * t.Price
	if t.Action == TradeActionBuy {
		return base + t.Fees
	}
	return base - t.Fees
}

// SnapshotPosition is a point-in-time position from a screenshot/bulk import.
// For snapshot portfolios, these ARE the holdings — no trade derivation.
type SnapshotPosition struct {
	Ticker       string    `json:"ticker"`
	Name         string    `json:"name,omitempty"`
	Units        float64   `json:"units"`
	AvgCost      float64   `json:"avg_cost"`                // average cost per unit
	CurrentPrice float64   `json:"current_price,omitempty"` // price at time of snapshot
	MarketValue  float64   `json:"market_value,omitempty"`  // can derive from units × current_price
	FeesTotal    float64   `json:"fees_total,omitempty"`    // cumulative brokerage
	SourceRef    string    `json:"source_ref,omitempty"`
	Notes        string    `json:"notes,omitempty"`
	SnapshotDate string    `json:"snapshot_date,omitempty"` // date of the snapshot
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// TradeBook stores all trades and snapshot positions for a portfolio.
// Analogous to CashFlowLedger — the complete local record.
type TradeBook struct {
	PortfolioName     string             `json:"portfolio_name"`
	Version           int                `json:"version"`
	Trades            []Trade            `json:"trades"`
	SnapshotPositions []SnapshotPosition `json:"snapshot_positions,omitempty"`
	Notes             string             `json:"notes,omitempty"`
	CreatedAt         time.Time          `json:"created_at"`
	UpdatedAt         time.Time          `json:"updated_at"`
}

// TradesForTicker returns trades filtered by ticker, sorted by date ascending.
func (tb *TradeBook) TradesForTicker(ticker string) []Trade {
	var result []Trade
	for _, t := range tb.Trades {
		if t.Ticker == ticker {
			result = append(result, t)
		}
	}
	// Sort by date ascending (trades should already be sorted, but ensure)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Date.Before(result[j].Date)
	})
	return result
}

// UniqueTickers returns the set of tickers that have trades.
func (tb *TradeBook) UniqueTickers() []string {
	seen := make(map[string]bool)
	var tickers []string
	for _, t := range tb.Trades {
		if !seen[t.Ticker] {
			seen[t.Ticker] = true
			tickers = append(tickers, t.Ticker)
		}
	}
	sort.Strings(tickers)
	return tickers
}

// DerivedHolding is the computed position for a ticker derived from trade history.
type DerivedHolding struct {
	Ticker           string  `json:"ticker"`
	Units            float64 `json:"units"`
	AvgCost          float64 `json:"avg_cost"`
	CostBasis        float64 `json:"cost_basis"`
	RealizedReturn   float64 `json:"realized_return"`
	UnrealizedReturn float64 `json:"unrealized_return"`
	MarketValue      float64 `json:"market_value"`
	GrossInvested    float64 `json:"gross_invested"`
	GrossProceeds    float64 `json:"gross_proceeds"`
	TradeCount       int     `json:"trade_count"`
}
