// Package models defines data structures for Vire
package models

import (
	"encoding/gob"
	"time"
)

func init() {
	// Register types for gob encoding (required for BadgerHold)
	gob.Register(Portfolio{})
	gob.Register(Holding{})
	gob.Register(PortfolioReview{})
	gob.Register(HoldingReview{})
}

// Portfolio represents a stock portfolio
type Portfolio struct {
	ID         string    `json:"id" badgerhold:"key"`
	Name       string    `json:"name" badgerhold:"index"`
	NavexaID   string    `json:"navexa_id,omitempty"`
	Holdings   []Holding `json:"holdings"`
	TotalValue float64   `json:"total_value"`
	Currency   string    `json:"currency"`
	LastSynced time.Time `json:"last_synced"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Holding represents a portfolio position
type Holding struct {
	Ticker       string    `json:"ticker"`
	Exchange     string    `json:"exchange"`
	Name         string    `json:"name"`
	Units        float64   `json:"units"`
	AvgCost      float64   `json:"avg_cost"`
	CurrentPrice float64   `json:"current_price"`
	MarketValue  float64   `json:"market_value"`
	GainLoss     float64   `json:"gain_loss"`
	GainLossPct  float64   `json:"gain_loss_pct"`
	Weight       float64   `json:"weight"` // Portfolio weight percentage
	LastUpdated  time.Time `json:"last_updated"`
}

// PortfolioReview contains the analysis results for a portfolio
type PortfolioReview struct {
	PortfolioName   string          `json:"portfolio_name"`
	ReviewDate      time.Time       `json:"review_date"`
	TotalValue      float64         `json:"total_value"`
	DayChange       float64         `json:"day_change"`
	DayChangePct    float64         `json:"day_change_pct"`
	HoldingReviews  []HoldingReview `json:"holding_reviews"`
	Alerts          []Alert         `json:"alerts"`
	Summary         string          `json:"summary"` // AI-generated summary
	Recommendations []string        `json:"recommendations"`
}

// HoldingReview contains the analysis for a single holding
type HoldingReview struct {
	Holding        Holding        `json:"holding"`
	Signals        *TickerSignals `json:"signals,omitempty"`
	OvernightMove  float64        `json:"overnight_move"`
	OvernightPct   float64        `json:"overnight_pct"`
	NewsImpact     string         `json:"news_impact,omitempty"`
	ActionRequired string         `json:"action_required"` // BUY, SELL, HOLD, WATCH
	ActionReason   string         `json:"action_reason"`
}

// Alert represents a portfolio alert
type Alert struct {
	Type     AlertType `json:"type"`
	Severity string    `json:"severity"` // high, medium, low
	Ticker   string    `json:"ticker,omitempty"`
	Message  string    `json:"message"`
	Signal   string    `json:"signal,omitempty"`
}

// AlertType categorizes alerts
type AlertType string

const (
	AlertTypeSignal AlertType = "signal"
	AlertTypePrice  AlertType = "price"
	AlertTypeNews   AlertType = "news"
	AlertTypeVolume AlertType = "volume"
	AlertTypeRisk   AlertType = "risk"
)
