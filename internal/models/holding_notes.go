package models

import (
	"strings"
	"time"
)

// AssetType categorizes the type of investment instrument
type AssetType string

const (
	AssetTypeETF      AssetType = "ETF"
	AssetTypeASXStock AssetType = "ASX_stock"
	AssetTypeUSEquity AssetType = "US_equity"
)

// ValidAssetType returns true if the asset type is one of the known types
func ValidAssetType(t AssetType) bool {
	switch t {
	case AssetTypeETF, AssetTypeASXStock, AssetTypeUSEquity:
		return true
	}
	return false
}

// LiquidityProfile describes expected trading liquidity
type LiquidityProfile string

const (
	LiquidityHigh   LiquidityProfile = "high"
	LiquidityMedium LiquidityProfile = "medium"
	LiquidityLow    LiquidityProfile = "low"
)

// SignalConfidence classifies how much weight technical signals should carry
type SignalConfidence string

const (
	SignalConfidenceHigh   SignalConfidence = "high"
	SignalConfidenceMedium SignalConfidence = "medium"
	SignalConfidenceLow    SignalConfidence = "low"
)

// HoldingNote stores analyst-style context for a single holding.
// Notes add qualitative intelligence that helps interpret technical signals.
type HoldingNote struct {
	Ticker           string           `json:"ticker"`
	Name             string           `json:"name,omitempty"`              // Company name (for display)
	AssetType        AssetType        `json:"asset_type,omitempty"`        // ETF, ASX_stock, US_equity
	LiquidityProfile LiquidityProfile `json:"liquidity_profile,omitempty"` // high, medium, low
	Thesis           string           `json:"thesis,omitempty"`            // Investment thesis
	KnownBehaviours  string           `json:"known_behaviours,omitempty"`  // Known behaviours / patterns
	SignalOverrides  string           `json:"signal_overrides,omitempty"`  // Context for signal interpretation
	Notes            string           `json:"notes,omitempty"`             // Free-form notes
	StaleDays        int              `json:"stale_days,omitempty"`        // Days until stale (default 90)
	CreatedAt        time.Time        `json:"created_at"`
	ReviewedAt       time.Time        `json:"reviewed_at"` // When note was last reviewed/updated
	UpdatedAt        time.Time        `json:"updated_at"`
}

// IsStale returns true if the note has not been reviewed within its staleness TTL
func (n *HoldingNote) IsStale() bool {
	ttl := n.StaleDays
	if ttl <= 0 {
		ttl = 90 // default 90 days
	}
	return time.Since(n.ReviewedAt) > time.Duration(ttl)*24*time.Hour
}

// DeriveSignalConfidence returns the signal confidence level based on asset type
// and liquidity profile.
//
// Rules:
//   - ETF → low (technical signals are noise on ETFs, use stop-loss instead)
//   - ETF + low liquidity → low
//   - ASX_stock + high/medium liquidity → high
//   - ASX_stock + low liquidity → medium
//   - US_equity → high (deep markets, signals reliable)
//   - Unknown/empty → medium (no context available)
func (n *HoldingNote) DeriveSignalConfidence() SignalConfidence {
	if n == nil {
		return SignalConfidenceMedium
	}
	switch n.AssetType {
	case AssetTypeETF:
		return SignalConfidenceLow
	case AssetTypeASXStock:
		if n.LiquidityProfile == LiquidityLow {
			return SignalConfidenceMedium
		}
		return SignalConfidenceHigh
	case AssetTypeUSEquity:
		return SignalConfidenceHigh
	default:
		return SignalConfidenceMedium
	}
}

// PortfolioHoldingNotes is a versioned collection of holding notes per portfolio.
// Stored as a single document in UserDataStore (subject="holding_notes").
type PortfolioHoldingNotes struct {
	PortfolioName string        `json:"portfolio_name"`
	Version       int           `json:"version"`
	Items         []HoldingNote `json:"items"`
	Notes         string        `json:"notes,omitempty"` // Portfolio-level notes
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
}

// FindByTicker returns the note and index for a given ticker, or nil/-1 if not found
func (h *PortfolioHoldingNotes) FindByTicker(ticker string) (*HoldingNote, int) {
	for i, item := range h.Items {
		if strings.EqualFold(item.Ticker, ticker) {
			return &h.Items[i], i
		}
	}
	return nil, -1
}

// NoteMap returns a map of ticker → *HoldingNote for O(1) lookups during review.
func (h *PortfolioHoldingNotes) NoteMap() map[string]*HoldingNote {
	m := make(map[string]*HoldingNote, len(h.Items))
	for i := range h.Items {
		m[strings.ToUpper(h.Items[i].Ticker)] = &h.Items[i]
	}
	return m
}
