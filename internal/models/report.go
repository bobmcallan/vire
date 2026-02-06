// Package models defines data structures for Vire
package models

import (
	"encoding/gob"
	"time"
)

func init() {
	gob.Register(PortfolioReport{})
	gob.Register(TickerReport{})
}

// PortfolioReport is a stored report for a portfolio
type PortfolioReport struct {
	Portfolio       string         `json:"portfolio" badgerhold:"key"`
	GeneratedAt     time.Time      `json:"generated_at" badgerhold:"index"`
	SummaryMarkdown string         `json:"summary_markdown"`
	TickerReports   []TickerReport `json:"ticker_reports"`
	Tickers         []string       `json:"tickers"`
}

// TickerReport is a stored report for a single ticker within a portfolio
type TickerReport struct {
	Ticker   string `json:"ticker"`
	Name     string `json:"name"`
	IsETF    bool   `json:"is_etf"`
	Markdown string `json:"markdown"`
}
