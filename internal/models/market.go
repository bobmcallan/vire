// Package models defines data structures for Vire
package models

import (
	"encoding/gob"
	"time"
)

func init() {
	// Register types for gob encoding
	gob.Register(MarketData{})
	gob.Register(EODBar{})
	gob.Register(Fundamentals{})
	gob.Register(NewsItem{})
	gob.Register(StockData{})
	gob.Register(SnipeBuy{})
	gob.Register(Symbol{})
}

// MarketData holds all market data for a ticker
type MarketData struct {
	Ticker       string        `json:"ticker" badgerhold:"key"`
	Exchange     string        `json:"exchange" badgerhold:"index"`
	Name         string        `json:"name"`
	EOD          []EODBar      `json:"eod"`
	Fundamentals *Fundamentals `json:"fundamentals,omitempty"`
	News         []*NewsItem   `json:"news,omitempty"`
	LastUpdated  time.Time     `json:"last_updated" badgerhold:"index"`
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

// Fundamentals contains fundamental data for a stock
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
	Description       string    `json:"description,omitempty"`
	LastUpdated       time.Time `json:"last_updated"`
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
	Ticker       string         `json:"ticker"`
	Exchange     string         `json:"exchange"`
	Name         string         `json:"name"`
	Price        *PriceData     `json:"price,omitempty"`
	Fundamentals *Fundamentals  `json:"fundamentals,omitempty"`
	Signals      *TickerSignals `json:"signals,omitempty"`
	News         []*NewsItem    `json:"news,omitempty"`
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

// TechnicalResponse represents EODHD technical indicators response
type TechnicalResponse struct {
	Data map[string]interface{} `json:"data"`
}
