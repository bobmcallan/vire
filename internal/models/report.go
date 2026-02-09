// Package models defines data structures for Vire
package models

import (
	"time"
)

// PortfolioReport is a stored report for a portfolio
type PortfolioReport struct {
	Portfolio       string         `json:"portfolio"`
	GeneratedAt     time.Time      `json:"generated_at"`
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
