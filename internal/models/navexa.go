// Package models defines data structures for Vire
package models

import (
	"time"
)

// NavexaPortfolio represents a Navexa portfolio response
type NavexaPortfolio struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Currency     string    `json:"currency"`
	TotalValue   float64   `json:"total_value"`
	TotalCost    float64   `json:"total_cost"`
	TotalGain    float64   `json:"total_gain"`
	TotalGainPct float64   `json:"total_gain_pct"`
	DateCreated  string    `json:"date_created"` // Raw date string from API (e.g. "2020-01-15") for performance endpoint
	CreatedAt    time.Time `json:"created_at"`
}

// NavexaHolding represents a Navexa holding response
type NavexaHolding struct {
	ID                 string    `json:"id"`
	PortfolioID        string    `json:"portfolio_id"`
	Ticker             string    `json:"ticker"`
	Exchange           string    `json:"exchange"`
	Name               string    `json:"name"`
	Units              float64   `json:"units"`
	AvgCost            float64   `json:"avg_cost"`
	TotalCost          float64   `json:"total_cost"`
	CurrentPrice       float64   `json:"current_price"`
	MarketValue        float64   `json:"market_value"`
	GainLoss           float64   `json:"gain_loss"`
	GainLossPct        float64   `json:"gain_loss_pct"` // IRR p.a. from Navexa
	DividendYield      float64   `json:"dividend_yield"`
	DividendReturn     float64   `json:"dividend_return"`
	CapitalGainPct     float64   `json:"capital_gain_pct"` // IRR p.a. from Navexa
	TotalReturnValue   float64   `json:"total_return_value"`
	TotalReturnPct     float64   `json:"total_return_pct"`      // IRR p.a. from Navexa
	TotalReturnPctTWRR float64   `json:"total_return_pct_twrr"` // Time-weighted return (computed locally)
	Currency           string    `json:"currency"`              // Holding currency code (e.g. "AUD", "USD")
	LastUpdated        time.Time `json:"last_updated"`
}

// EODHDTicker returns the full EODHD-format ticker (e.g. "BHP.AU", "CBOE.US").
// Maps Navexa exchange names to EODHD codes and falls back to ".AU" if empty.
func (h NavexaHolding) EODHDTicker() string {
	return h.Ticker + "." + eodhExchange(h.Exchange)
}

// NavexaTrade represents a single trade from the Navexa trades endpoint
type NavexaTrade struct {
	ID          string  `json:"id"`
	HoldingID   string  `json:"holding_id"`
	PortfolioID string  `json:"portfolio_id"`
	Symbol      string  `json:"symbol"`
	Type        string  `json:"type"` // buy, sell, split, etc.
	Date        string  `json:"date"`
	Units       float64 `json:"units"`
	Price       float64 `json:"price"`
	Fees        float64 `json:"fees"`
	Value       float64 `json:"value"`
	Currency    string  `json:"currency"`
}

// NavexaPerformance represents portfolio performance metrics
type NavexaPerformance struct {
	PortfolioID      string             `json:"portfolio_id"`
	TotalValue       float64            `json:"total_value"`
	TotalCost        float64            `json:"total_cost"`
	TotalReturn      float64            `json:"total_return"`
	TotalReturnPct   float64            `json:"total_return_pct"`
	AnnualisedReturn float64            `json:"annualised_return"`
	Volatility       float64            `json:"volatility"`
	SharpeRatio      float64            `json:"sharpe_ratio"`
	MaxDrawdown      float64            `json:"max_drawdown"`
	PeriodReturns    map[string]float64 `json:"period_returns"` // 1d, 1w, 1m, 3m, 6m, 1y, ytd
	AsOfDate         time.Time          `json:"as_of_date"`
}
