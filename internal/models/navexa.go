// Package models defines data structures for Vire
package models

import (
	"encoding/gob"
	"time"
)

func init() {
	// Register types for gob encoding
	gob.Register(NavexaPortfolio{})
	gob.Register(NavexaHolding{})
	gob.Register(NavexaPerformance{})
}

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
	ID            string    `json:"id"`
	PortfolioID   string    `json:"portfolio_id"`
	Ticker        string    `json:"ticker"`
	Exchange      string    `json:"exchange"`
	Name          string    `json:"name"`
	Units         float64   `json:"units"`
	AvgCost       float64   `json:"avg_cost"`
	TotalCost     float64   `json:"total_cost"`
	CurrentPrice  float64   `json:"current_price"`
	MarketValue   float64   `json:"market_value"`
	GainLoss      float64   `json:"gain_loss"`
	GainLossPct   float64   `json:"gain_loss_pct"`
	DividendYield float64   `json:"dividend_yield"`
	LastUpdated   time.Time `json:"last_updated"`
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
