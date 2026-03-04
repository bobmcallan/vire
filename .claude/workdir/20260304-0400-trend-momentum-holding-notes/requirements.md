# Requirements: Trend Momentum Signal & Holding Intelligence Layer

**Feedback**: fb_28c6e0bd (trend momentum), fb_61fba80b (holding intelligence)
**Work dir**: `.claude/workdir/20260304-0400-trend-momentum-holding-notes/`

---

## Scope

### IN SCOPE
1. **Trend Momentum Signal** — new composite signal in the signal pipeline
2. **Holding Notes** — per-portfolio per-ticker analyst notes with CRUD service
3. **Signal Confidence** — asset-type-aware confidence on signal output
4. **Compliance integration** — both features integrated into ReviewPortfolio/ReviewWatchlist

### OUT OF SCOPE
- Automated weekly review job (future work, builds on notes infrastructure)
- Frontend/portal changes
- Relative strength vs index improvements (RS signal already exists)

---

## Part A: Trend Momentum Signal (fb_28c6e0bd)

### A1. Model — `internal/models/signals.go`

Add new types after the existing `RSSignal` struct (before `TrendType`):

```go
// TrendMomentumLevel classifies short-term trend momentum on a 5-point scale
type TrendMomentumLevel string

const (
	TrendMomentumStrongUp TrendMomentumLevel = "TREND_STRONG_UP"
	TrendMomentumUp       TrendMomentumLevel = "TREND_UP"
	TrendMomentumFlat     TrendMomentumLevel = "TREND_FLAT"
	TrendMomentumDown     TrendMomentumLevel = "TREND_DOWN"
	TrendMomentumStrongDown TrendMomentumLevel = "TREND_STRONG_DOWN"
)

// TrendMomentum tracks multi-timeframe price trajectory and acceleration.
// Unlike Trend (SMA-based, long-term), this captures short-term momentum
// across 3/5/10-day windows to provide early warning of deterioration.
type TrendMomentum struct {
	Level           TrendMomentumLevel `json:"level"`            // 5-point classification
	Score           float64            `json:"score"`            // -1.0 (strong down) to +1.0 (strong up)
	PriceChange3D   float64            `json:"price_change_3d"`  // 3-day price change %
	PriceChange5D   float64            `json:"price_change_5d"`  // 5-day price change %
	PriceChange10D  float64            `json:"price_change_10d"` // 10-day price change %
	Acceleration    float64            `json:"acceleration"`     // Rate of change of price changes (positive = accelerating)
	VolumeConfirm   bool               `json:"volume_confirm"`   // True if volume supports the price direction
	NearSupport     bool               `json:"near_support"`     // True if within 3% of support level
	Description     string             `json:"description"`      // Human-readable narrative
}
```

Add the field to `TickerSignals` struct — add after the `RS` field:

```go
	TrendMomentum TrendMomentum `json:"trend_momentum"`
```

Add new signal type constant:

```go
	SignalTypeTrendMomentum = "trend_momentum"
```

Add it to `AllSignalTypes()` return slice.

### A2. Computation — `internal/signals/computer.go`

Add call in `Compute()` method, after `c.computeRelativeStrength(signals, marketData)`:

```go
	c.computeTrendMomentum(signals, marketData)
```

New method on Computer:

```go
// computeTrendMomentum calculates multi-timeframe trend momentum.
// Requires >= 11 bars for full 10-day analysis. Falls back gracefully.
func (c *Computer) computeTrendMomentum(signals *models.TickerSignals, data *models.MarketData) {
	bars := data.EOD
	if len(bars) < 4 { // Need at least 3-day change
		signals.TrendMomentum = models.TrendMomentum{
			Level:       models.TrendMomentumFlat,
			Description: "Insufficient data for trend momentum",
		}
		return
	}

	currentPrice := bars[0].Close

	// Multi-timeframe price changes (%)
	change3d := priceChangePct(currentPrice, bars, 3)
	change5d := priceChangePct(currentPrice, bars, 5)
	change10d := priceChangePct(currentPrice, bars, 10)

	// Acceleration: compare recent momentum (3d) vs longer (10d normalized to 3d rate).
	// Positive = momentum is increasing, negative = decelerating.
	acceleration := 0.0
	if len(bars) >= 11 {
		rate10dPer3d := change10d * 3.0 / 10.0 // normalize 10d rate to per-3-day
		acceleration = change3d - rate10dPer3d
	}

	// Volume confirmation: average volume for last 3 days vs 20-day average.
	// Volume should support the price direction.
	volumeConfirm := false
	if len(bars) >= 20 {
		recentAvgVol := avgVolume(bars[:3])
		longerAvgVol := avgVolume(bars[:20])
		if longerAvgVol > 0 {
			volRatio := recentAvgVol / longerAvgVol
			// Confirm if: moving up with above-avg volume, or moving down with above-avg volume
			if (change3d > 0 && volRatio > 1.2) || (change3d < -1 && volRatio > 1.2) {
				volumeConfirm = true
			}
		}
	}

	// Support proximity (reuse existing support level from technical signals)
	nearSupport := false
	if signals.Technical.SupportLevel > 0 && currentPrice > 0 {
		distPct := (currentPrice - signals.Technical.SupportLevel) / signals.Technical.SupportLevel * 100
		nearSupport = distPct >= 0 && distPct <= 3.0
	}

	// Composite score: weighted combination of timeframe changes + acceleration
	// Weights: 3d=0.45, 5d=0.30, 10d=0.15, acceleration=0.10
	// Normalize each component to roughly -1 to +1 range (cap at ±20% moves)
	norm3d := clampFloat(change3d/10.0, -1, 1)
	norm5d := clampFloat(change5d/15.0, -1, 1)
	norm10d := clampFloat(change10d/20.0, -1, 1)
	normAccel := clampFloat(acceleration/5.0, -1, 1)

	score := norm3d*0.45 + norm5d*0.30 + norm10d*0.15 + normAccel*0.10

	// Classify into 5 levels
	level := classifyTrendMomentum(score, volumeConfirm)

	// Generate narrative
	desc := describeTrendMomentum(level, change3d, change5d, change10d, acceleration, volumeConfirm, nearSupport)

	signals.TrendMomentum = models.TrendMomentum{
		Level:          level,
		Score:          score,
		PriceChange3D:  change3d,
		PriceChange5D:  change5d,
		PriceChange10D: change10d,
		Acceleration:   acceleration,
		VolumeConfirm:  volumeConfirm,
		NearSupport:    nearSupport,
		Description:    desc,
	}
}
```

Helper functions (in computer.go or indicators.go — implementer's choice, keep it clean):

```go
// priceChangePct returns the % change from `period` bars ago to current price.
// If bars are shorter than period, uses the oldest available bar.
func priceChangePct(currentPrice float64, bars []models.EODBar, period int) float64 {
	idx := period
	if idx >= len(bars) {
		idx = len(bars) - 1
	}
	if bars[idx].Close == 0 {
		return 0
	}
	return (currentPrice - bars[idx].Close) / bars[idx].Close * 100
}

// avgVolume returns average volume for the given slice of bars
func avgVolume(bars []models.EODBar) float64 {
	if len(bars) == 0 {
		return 0
	}
	var sum float64
	for _, b := range bars {
		sum += float64(b.Volume)
	}
	return sum / float64(len(bars))
}

// clampFloat clamps v to [min, max]
func clampFloat(v, min, max float64) float64 {
	if v < min { return min }
	if v > max { return max }
	return v
}

// classifyTrendMomentum maps composite score to 5-level enum.
// Volume confirmation strengthens the signal (widens the classification band).
func classifyTrendMomentum(score float64, volumeConfirm bool) models.TrendMomentumLevel {
	strongThreshold := 0.5
	weakThreshold := 0.15
	if volumeConfirm {
		strongThreshold = 0.4  // easier to qualify when volume confirms
		weakThreshold = 0.10
	}

	switch {
	case score >= strongThreshold:
		return models.TrendMomentumStrongUp
	case score >= weakThreshold:
		return models.TrendMomentumUp
	case score <= -strongThreshold:
		return models.TrendMomentumStrongDown
	case score <= -weakThreshold:
		return models.TrendMomentumDown
	default:
		return models.TrendMomentumFlat
	}
}

// describeTrendMomentum generates a human-readable narrative for the trend momentum.
func describeTrendMomentum(level models.TrendMomentumLevel, c3, c5, c10, accel float64, volConfirm, nearSupport bool) string {
	// Build narrative based on level and components.
	// Examples:
	//   "Strong downtrend: -8.2% over 3 days, -12.1% over 10 days, accelerating losses with volume confirmation"
	//   "Mild uptrend: +2.1% over 3 days, decelerating — approaching resistance"
	//   "Flat: minimal movement across all timeframes"
	// Implementer: generate sensible prose from the components. Keep it 1-2 sentences.
	// Use the component values to explain WHY the level was assigned.
	// Mention volume confirmation and support proximity when relevant.
	// This string appears in compliance review output.
}
```

### A3. Integration into compliance — `internal/services/portfolio/service.go`

**In `determineAction()`** — add trend momentum check BEFORE the existing exit triggers (after strategy rules, around line 1675):

```go
	// Trend momentum — early warning for deteriorating positions
	if signals.TrendMomentum.Level == models.TrendMomentumStrongDown {
		return "EXIT TRIGGER", fmt.Sprintf("Strong downtrend: %.1f%% over 3d, %.1f%% over 10d",
			signals.TrendMomentum.PriceChange3D, signals.TrendMomentum.PriceChange10D)
	}
	if signals.TrendMomentum.Level == models.TrendMomentumDown {
		return "WATCH", fmt.Sprintf("Deteriorating trend: %.1f%% over 3d, %.1f%% over 10d",
			signals.TrendMomentum.PriceChange3D, signals.TrendMomentum.PriceChange10D)
	}
```

**In `generateAlerts()`** — add trend momentum alert (after volume alerts, around line 1771):

```go
	// Trend momentum alerts
	if signals.TrendMomentum.Level == models.TrendMomentumStrongDown {
		alerts = append(alerts, models.Alert{
			Type:     models.AlertTypeSignal,
			Severity: "high",
			Ticker:   holding.Ticker,
			Message:  fmt.Sprintf("%s strong downtrend: %s", holding.Ticker, signals.TrendMomentum.Description),
			Signal:   "trend_momentum_strong_down",
		})
	} else if signals.TrendMomentum.Level == models.TrendMomentumDown {
		alerts = append(alerts, models.Alert{
			Type:     models.AlertTypeSignal,
			Severity: "medium",
			Ticker:   holding.Ticker,
			Message:  fmt.Sprintf("%s deteriorating: %s", holding.Ticker, signals.TrendMomentum.Description),
			Signal:   "trend_momentum_down",
		})
	}
```

### A4. Unit tests — `internal/signals/computer_test.go`

Write tests for `computeTrendMomentum`. Create helper to build mock bars.

| Test | Scenario |
|------|----------|
| TestTrendMomentum_StrongDown | 10 bars with steadily declining prices → TREND_STRONG_DOWN |
| TestTrendMomentum_StrongUp | 10 bars with steadily rising prices → TREND_STRONG_UP |
| TestTrendMomentum_Flat | 10 bars with minimal movement → TREND_FLAT |
| TestTrendMomentum_Acceleration | Prices declining faster recently → negative acceleration |
| TestTrendMomentum_VolumeConfirmation | Above-avg volume on down move → volumeConfirm=true |
| TestTrendMomentum_InsufficientData | <4 bars → safe default (FLAT) |
| TestTrendMomentum_NearSupport | Price within 3% of support → nearSupport=true |

---

## Part B: Holding Intelligence Layer (fb_61fba80b)

### B1. Model — `internal/models/holding_notes.go` (NEW FILE)

```go
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
```

### B2. Interface — `internal/interfaces/services.go`

Add after the WatchlistService interface:

```go
// HoldingNoteService manages per-holding analyst notes
type HoldingNoteService interface {
	// GetNotes retrieves all holding notes for a portfolio
	GetNotes(ctx context.Context, portfolioName string) (*models.PortfolioHoldingNotes, error)

	// SaveNotes saves notes with version increment
	SaveNotes(ctx context.Context, notes *models.PortfolioHoldingNotes) error

	// AddOrUpdateNote adds a new note or updates an existing one (upsert keyed on ticker)
	AddOrUpdateNote(ctx context.Context, portfolioName string, note *models.HoldingNote) (*models.PortfolioHoldingNotes, error)

	// UpdateNote updates an existing note by ticker (merge semantics)
	UpdateNote(ctx context.Context, portfolioName, ticker string, update *models.HoldingNote) (*models.PortfolioHoldingNotes, error)

	// RemoveNote removes a note by ticker
	RemoveNote(ctx context.Context, portfolioName, ticker string) (*models.PortfolioHoldingNotes, error)
}
```

### B3. Service — `internal/services/holdingnotes/service.go` (NEW FILE)

Follow the exact pattern of `internal/services/watchlist/service.go`:

```go
package holdingnotes

// Compile-time interface check
var _ interfaces.HoldingNoteService = (*Service)(nil)

type Service struct {
	storage interfaces.StorageManager
	logger  *common.Logger
}

func NewService(storage interfaces.StorageManager, logger *common.Logger) *Service

// Subject: "holding_notes", Key: portfolioName
// Methods: GetNotes, SaveNotes, AddOrUpdateNote, UpdateNote, RemoveNote
// Follow exact watchlist/service.go patterns:
// - GetNotes: UserDataStore.Get → unmarshal
// - SaveNotes: marshal → UserDataStore.Put (increment Version, set UpdatedAt)
// - AddOrUpdateNote: get-or-create → upsert by ticker → save (set ReviewedAt=now on new items)
// - UpdateNote: get → find ticker → merge non-zero fields → save (update ReviewedAt if content changed)
// - RemoveNote: get → find ticker → splice out → save
```

### B4. App wiring — `internal/app/app.go`

Add field to App struct:
```go
	HoldingNoteService interfaces.HoldingNoteService
```

In `NewApp()`, create the service (after watchlistService):
```go
	holdingNoteService := holdingnotes.NewService(storageManager, logger)
```

Add to App construction:
```go
	HoldingNoteService: holdingNoteService,
```

### B5. Handlers — `internal/server/handlers.go`

Add handler functions following the watchlist pattern. Use the same patterns from `handleWatchlistItemAdd`, `handleWatchlistItem`, `handlePortfolioWatchlist`.

```go
// handleHoldingNotes handles GET (list) and PUT (replace all) for holding notes
func (s *Server) handleHoldingNotes(w http.ResponseWriter, r *http.Request, name string) {
	switch r.Method {
	case http.MethodGet:
		notes, err := s.app.HoldingNoteService.GetNotes(r.Context(), name)
		if err != nil {
			// Return empty collection if none exist
			WriteJSON(w, http.StatusOK, &models.PortfolioHoldingNotes{
				PortfolioName: name,
				Items:         []models.HoldingNote{},
			})
			return
		}
		WriteJSON(w, http.StatusOK, notes)

	case http.MethodPut:
		var notes models.PortfolioHoldingNotes
		if !DecodeJSON(w, r, &notes) {
			return
		}
		notes.PortfolioName = name
		if err := s.app.HoldingNoteService.SaveNotes(r.Context(), &notes); err != nil {
			WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error saving notes: %v", err))
			return
		}
		WriteJSON(w, http.StatusOK, &notes)

	default:
		RequireMethod(w, r, http.MethodGet, http.MethodPut)
	}
}

// handleHoldingNoteAdd handles POST to add/upsert a holding note
func (s *Server) handleHoldingNoteAdd(w http.ResponseWriter, r *http.Request, name string) {
	if !RequireMethod(w, r, http.MethodPost) {
		return
	}
	var note models.HoldingNote
	if !DecodeJSON(w, r, &note) {
		return
	}
	if note.Ticker == "" {
		WriteError(w, http.StatusBadRequest, "ticker is required")
		return
	}
	ticker, errMsg := validateTicker(note.Ticker)
	if errMsg != "" {
		WriteError(w, http.StatusBadRequest, errMsg)
		return
	}
	note.Ticker = ticker

	if note.AssetType != "" && !models.ValidAssetType(note.AssetType) {
		WriteError(w, http.StatusBadRequest, fmt.Sprintf("invalid asset_type '%s'", note.AssetType))
		return
	}

	notes, err := s.app.HoldingNoteService.AddOrUpdateNote(r.Context(), name, &note)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error adding note: %v", err))
		return
	}
	WriteJSON(w, http.StatusCreated, notes)
}

// handleHoldingNoteItem handles PATCH (update) and DELETE (remove) for a specific note
func (s *Server) handleHoldingNoteItem(w http.ResponseWriter, r *http.Request, name, ticker string) {
	ctx := r.Context()

	ticker, errMsg := validateTicker(ticker)
	if errMsg != "" {
		WriteError(w, http.StatusBadRequest, errMsg)
		return
	}

	switch r.Method {
	case http.MethodPatch:
		var update models.HoldingNote
		if !DecodeJSON(w, r, &update) {
			return
		}
		if update.AssetType != "" && !models.ValidAssetType(update.AssetType) {
			WriteError(w, http.StatusBadRequest, fmt.Sprintf("invalid asset_type '%s'", update.AssetType))
			return
		}
		notes, err := s.app.HoldingNoteService.UpdateNote(ctx, name, ticker, &update)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error updating note: %v", err))
			return
		}
		WriteJSON(w, http.StatusOK, notes)

	case http.MethodDelete:
		notes, err := s.app.HoldingNoteService.RemoveNote(ctx, name, ticker)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error removing note: %v", err))
			return
		}
		WriteJSON(w, http.StatusOK, notes)

	default:
		RequireMethod(w, r, http.MethodPatch, http.MethodDelete)
	}
}
```

### B6. Routes — `internal/server/routes.go`

Add new route dispatcher (after `routeWatchlist`):

```go
// routeHoldingNotes dispatches /api/portfolios/{name}/notes/* sub-routes.
func (s *Server) routeHoldingNotes(w http.ResponseWriter, r *http.Request, portfolioName, subpath string) {
	switch {
	case subpath == "items":
		s.handleHoldingNoteAdd(w, r, portfolioName)
	case strings.HasPrefix(subpath, "items/"):
		ticker := strings.TrimPrefix(subpath, "items/")
		s.handleHoldingNoteItem(w, r, portfolioName, ticker)
	default:
		WriteError(w, http.StatusNotFound, "Not found")
	}
}
```

In `routePortfolios()`, add two new cases:

1. In the `switch subpath` block (around line 250), add:
```go
	case "notes":
		s.handleHoldingNotes(w, r, name)
```

2. In the `else if` chain for nested paths (around line 280), add:
```go
	} else if strings.HasPrefix(subpath, "notes/") {
		s.routeHoldingNotes(w, r, name, strings.TrimPrefix(subpath, "notes/"))
```

### B7. MCP Tool Catalog — `internal/server/catalog.go`

Add 5 new tool definitions. Follow the exact pattern of the watchlist tools. Group them together after the watchlist tools section.

| Tool | Method | Path | Description |
|------|--------|------|-------------|
| `holding_note_get` | GET | `/api/portfolios/{portfolio_name}/notes` | Get holding notes for a portfolio |
| `holding_note_set` | PUT | `/api/portfolios/{portfolio_name}/notes` | Replace all holding notes |
| `holding_note_add` | POST | `/api/portfolios/{portfolio_name}/notes/items` | Add or update a holding note |
| `holding_note_update` | PATCH | `/api/portfolios/{portfolio_name}/notes/items/{ticker}` | Update a holding note (merge) |
| `holding_note_remove` | DELETE | `/api/portfolios/{portfolio_name}/notes/items/{ticker}` | Remove a holding note |

Parameters for `holding_note_add`:
- `portfolio_name` (string, optional): Portfolio name
- `ticker` (string, required): Ticker symbol e.g. 'BHP.AU'
- `asset_type` (string, optional): ETF, ASX_stock, US_equity
- `liquidity_profile` (string, optional): high, medium, low
- `thesis` (string, optional): Investment thesis
- `known_behaviours` (string, optional): Known patterns/behaviours
- `signal_overrides` (string, optional): Context for interpreting signals
- `notes` (string, optional): Free-form notes
- `stale_days` (number, optional): Days until stale (default 90)

Parameters for `holding_note_set`:
- `portfolio_name` (string, optional)
- `items` (array, required): Array of holding notes (same fields as add)
- `notes` (string, optional): Portfolio-level notes

### B8. Compliance integration — `internal/services/portfolio/service.go`

**Add `HoldingNote` and `SignalConfidence` fields to `HoldingReview`** in `internal/models/portfolio.go`:

```go
type HoldingReview struct {
	// ... existing fields ...
	Compliance       *ComplianceResult `json:"compliance,omitempty"`
	HoldingNote      *HoldingNote      `json:"holding_note,omitempty"`       // Analyst context note
	SignalConfidence SignalConfidence   `json:"signal_confidence,omitempty"`  // high/medium/low based on asset type
	NoteStale        bool              `json:"note_stale,omitempty"`         // True if note needs review
}
```

**Add `HoldingNote` and `SignalConfidence` fields to `WatchlistItemReview`** in `internal/models/watchlist.go`:

```go
type WatchlistItemReview struct {
	// ... existing fields ...
	Compliance       *ComplianceResult `json:"compliance,omitempty"`
	HoldingNote      *HoldingNote      `json:"holding_note,omitempty"`
	SignalConfidence SignalConfidence   `json:"signal_confidence,omitempty"`
	NoteStale        bool              `json:"note_stale,omitempty"`
}
```

**In `ReviewPortfolio()`** — load notes alongside strategy (Phase 1, around line 1286):

```go
	// Load holding notes (nil if none exist)
	var noteMap map[string]*models.HoldingNote
	if s.holdingNoteService != nil {
		if hn, err := s.holdingNoteService.GetNotes(ctx, name); err == nil && hn != nil {
			noteMap = hn.NoteMap()
		}
	}
```

In the holdings loop (Phase 3), after constructing `holdingReview` and before compliance check:

```go
		// Attach holding note and derive signal confidence
		if note, ok := noteMap[strings.ToUpper(holding.Ticker)]; ok {
			holdingReview.HoldingNote = note
			holdingReview.SignalConfidence = note.DeriveSignalConfidence()
			holdingReview.NoteStale = note.IsStale()
		} else {
			holdingReview.SignalConfidence = models.SignalConfidenceMedium
		}
```

Add stale note alert in the alerts section (after the holding loop or within it):

```go
		// Stale note alert
		if holdingReview.NoteStale {
			alerts = append(alerts, models.Alert{
				Type:     models.AlertTypeSignal,
				Severity: "low",
				Ticker:   holding.Ticker,
				Message:  fmt.Sprintf("%s holding note is stale (last reviewed %s)", holding.Ticker, holdingReview.HoldingNote.ReviewedAt.Format("2006-01-02")),
				Signal:   "note_stale",
			})
		}
```

**In `ReviewWatchlist()`** — same pattern: load noteMap, attach to each WatchlistItemReview.

**Dependency injection**: Add `holdingNoteService` to portfolio Service struct and a setter:

In `internal/services/portfolio/service.go`, add field to Service struct:
```go
	holdingNoteService interfaces.HoldingNoteService
```

Add setter method:
```go
// SetHoldingNoteService injects the holding note service (setter injection to avoid circular deps)
func (s *Service) SetHoldingNoteService(svc interfaces.HoldingNoteService) {
	s.holdingNoteService = svc
}
```

In `internal/interfaces/services.go`, add to PortfolioService interface:
```go
	// SetHoldingNoteService injects the holding note service
	SetHoldingNoteService(svc HoldingNoteService)
```

In `internal/app/app.go`, wire it up after creating holdingNoteService:
```go
	portfolioService.SetHoldingNoteService(holdingNoteService)
```

### B9. App test — `internal/app/app_test.go`

Add nil check:
```go
	if a.HoldingNoteService == nil {
		t.Error("HoldingNoteService is nil")
	}
```

### B10. Mock updates

In every file that mocks `PortfolioService`, add the stub:

```go
func (m *mockPortfolioService) SetHoldingNoteService(_ interfaces.HoldingNoteService) {}
```

Files that need this (same files that got `IsTimelineRebuilding` mock last time):
- `internal/server/handlers_portfolio_test.go`
- `internal/services/report/devils_advocate_test.go`
- `internal/services/cashflow/service_test.go`

---

## File Change Summary

| File | Status | Change |
|------|--------|--------|
| `internal/models/signals.go` | MODIFY | Add TrendMomentum struct, TrendMomentumLevel enum, SignalTypeTrendMomentum, field on TickerSignals |
| `internal/models/holding_notes.go` | NEW | HoldingNote, AssetType, LiquidityProfile, SignalConfidence, PortfolioHoldingNotes |
| `internal/models/portfolio.go` | MODIFY | Add HoldingNote/SignalConfidence/NoteStale to HoldingReview |
| `internal/models/watchlist.go` | MODIFY | Add HoldingNote/SignalConfidence/NoteStale to WatchlistItemReview |
| `internal/interfaces/services.go` | MODIFY | Add HoldingNoteService interface, SetHoldingNoteService to PortfolioService |
| `internal/signals/computer.go` | MODIFY | Add computeTrendMomentum() + helpers |
| `internal/signals/computer_test.go` | NEW | Unit tests for trend momentum computation |
| `internal/services/holdingnotes/service.go` | NEW | HoldingNoteService implementation |
| `internal/services/portfolio/service.go` | MODIFY | Add holdingNoteService field + setter; integrate notes into ReviewPortfolio/ReviewWatchlist; add trend momentum to determineAction/generateAlerts |
| `internal/server/handlers.go` | MODIFY | Add handleHoldingNotes, handleHoldingNoteAdd, handleHoldingNoteItem |
| `internal/server/routes.go` | MODIFY | Add "notes" case and notes/ routing in routePortfolios |
| `internal/server/catalog.go` | MODIFY | Add 5 holding_note MCP tool definitions |
| `internal/app/app.go` | MODIFY | Add HoldingNoteService field + wiring |
| `internal/app/app_test.go` | MODIFY | Add nil check for HoldingNoteService |
| `internal/server/handlers_portfolio_test.go` | MODIFY | Add SetHoldingNoteService mock stub |
| `internal/services/report/devils_advocate_test.go` | MODIFY | Add SetHoldingNoteService mock stub |
| `internal/services/cashflow/service_test.go` | MODIFY | Add SetHoldingNoteService mock stub |

---

## Test Plan

### Unit Tests (implementer writes)
- `internal/signals/computer_test.go` — 7 trend momentum tests (see A4)
- `internal/services/holdingnotes/` — service tests are optional (simple CRUD, covered by integration)
- Existing portfolio/service_test.go may need mock updates only

### Integration Tests (test-creator writes)
- `tests/data/holding_notes_test.go` — CRUD for holding notes via data layer
- `tests/api/holding_notes_test.go` — API endpoint tests for all 5 MCP tools

---

## Verify Checklist
- [ ] `go build ./cmd/vire-server/`
- [ ] `go vet ./...`
- [ ] `go test ./internal/signals/...`
- [ ] `go test ./internal/services/holdingnotes/...`
- [ ] `go test ./internal/services/portfolio/...`
- [ ] `go test ./internal/server/...`
- [ ] `go test ./internal/app/...`
- [ ] All existing tests still pass
