# Requirements: Source-Typed Portfolios & Trade Management

**Feature Doc:** `docs/features/26-03-02-portfolio-refactor-snaphots.md`
**Work Dir:** `.claude/workdir/20260304-portfolio-source-types/`
**Schema Version:** 13 → 14

---

## Scope

### IN SCOPE (Phase 1)
1. Model: Trade, SnapshotPosition, TradeBook structs; SourceType on Portfolio/Holding
2. Trade service: CRUD + position derivation from trades
3. Portfolio service: CreatePortfolio, GetPortfolio routing by source_type, manual/snapshot assembly
4. MCP endpoints: `portfolio_create`, `trade_add`, `trade_list`, `trade_remove`, `trade_update`, `portfolio_snapshot`
5. Route + catalog registration
6. Schema version bump 13 → 14
7. Unit tests for trade service + position derivation

### OUT OF SCOPE
- CSV import (`import_trades_csv`)
- Portfolio reconciliation (`reconcile_portfolio`)
- `set_portfolio_source` endpoint
- Hybrid portfolio assembly (Navexa + local merge)
- Advanced enrichment (dividend tracking, DRP for manual holdings)

---

## File Changes

### New Files

#### 1. `internal/models/trade.go` — Trade + TradeBook models

Follow `internal/models/cashflow.go` as the template.

```go
package models

import "time"

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
	ID            string      `json:"id"`                       // Auto-generated "tr_" + 8 hex chars
	PortfolioName string      `json:"portfolio_name"`
	Ticker        string      `json:"ticker"`                   // e.g. "BHP.AU"
	Action        TradeAction `json:"action"`                   // "buy" or "sell"
	Units         float64     `json:"units"`
	Price         float64     `json:"price"`                    // per unit, excluding fees
	Fees          float64     `json:"fees"`                     // brokerage / commission
	Date          time.Time   `json:"date"`                     // trade date
	SettleDate    string      `json:"settle_date,omitempty"`    // settlement date (optional)
	SourceType    SourceType  `json:"source_type,omitempty"`    // "manual", "snapshot", "csv"
	SourceRef     string      `json:"source_ref,omitempty"`     // free-form provenance tag
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
	Ticker       string     `json:"ticker"`
	Name         string     `json:"name,omitempty"`
	Units        float64    `json:"units"`
	AvgCost      float64    `json:"avg_cost"`                 // average cost per unit
	CurrentPrice float64    `json:"current_price,omitempty"`  // price at time of snapshot
	MarketValue  float64    `json:"market_value,omitempty"`   // can derive from units × current_price
	FeesTotal    float64    `json:"fees_total,omitempty"`     // cumulative brokerage
	SourceRef    string     `json:"source_ref,omitempty"`
	Notes        string     `json:"notes,omitempty"`
	SnapshotDate string     `json:"snapshot_date,omitempty"`  // date of the snapshot
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
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
```

**Note:** The `sort` import will be needed. Add `"sort"` to the imports.

---

#### 2. `internal/services/trade/service.go` — TradeService

Follow `internal/services/cashflow/service.go` as the template. Same patterns: UserDataStore KV, ID generation, version increment, full-document save.

**Storage:** Subject `"trades"`, key = portfolio name, value = JSON-serialized TradeBook.

```go
package trade

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

type Service struct {
	storage interfaces.StorageManager
	logger  *common.Logger
}

func NewService(storage interfaces.StorageManager, logger *common.Logger) *Service {
	return &Service{storage: storage, logger: logger}
}
```

**ID generation** (follow cashflow/service.go:45-52):
```go
func generateTradeID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "tr_00000000"
	}
	return "tr_" + hex.EncodeToString(b)
}
```

**Methods to implement:**

##### `GetTradeBook(ctx, portfolioName) → (*TradeBook, error)`
- Read from UserDataStore subject="trades", key=portfolioName
- If not found, return empty TradeBook (follow cashflow pattern: `getLedger()`)
- Unmarshal JSON into TradeBook

##### `AddTrade(ctx, portfolioName, trade) → (*Trade, *DerivedHolding, error)`
- Validate trade: ticker non-empty, units > 0, price ≥ 0, action is "buy" or "sell", date non-zero
- For sells: validate units ≤ current position (compute from existing trades)
- Generate ID with `generateTradeID()`
- Set CreatedAt, UpdatedAt = time.Now()
- Set trade.PortfolioName = portfolioName
- Append to TradeBook.Trades
- Sort trades by date
- Increment version, save
- Derive holding for that ticker and return it

##### `RemoveTrade(ctx, portfolioName, tradeID) → (*TradeBook, error)`
- Find trade by ID in TradeBook.Trades
- Remove it
- Save TradeBook
- Return updated TradeBook

##### `UpdateTrade(ctx, portfolioName, tradeID, update) → (*Trade, error)`
- Find trade by ID
- Merge semantics: only update non-zero fields from the update struct
- Set UpdatedAt = time.Now()
- Re-sort trades by date
- Save
- Return updated trade

##### `ListTrades(ctx, portfolioName, filters) → ([]Trade, int, error)`
- Load TradeBook
- Apply filters: ticker, action, date_from, date_to, source_type
- Apply pagination: offset, limit (default 50, max 200)
- Return filtered trades and total count

##### `SnapshotPositions(ctx, portfolioName, positions, mode, sourceRef, snapshotDate) → (*TradeBook, error)`
- If mode == "replace": clear existing SnapshotPositions, replace with new
- If mode == "merge": update matching tickers, add new ones, leave unmatched
- Set CreatedAt/UpdatedAt on each position
- Save TradeBook

##### `DeriveHolding(ctx, trades []Trade, currentPrice float64) → DerivedHolding`
**This is the position derivation from the feature doc (Section "Position Derivation"):**
```go
type DerivedHolding struct {
	Ticker          string  `json:"ticker"`
	Units           float64 `json:"units"`
	AvgCost         float64 `json:"avg_cost"`
	CostBasis       float64 `json:"cost_basis"`
	RealizedReturn  float64 `json:"realized_return"`
	UnrealizedReturn float64 `json:"unrealized_return"`
	MarketValue     float64 `json:"market_value"`
	GrossInvested   float64 `json:"gross_invested"`
	GrossProceeds   float64 `json:"gross_proceeds"`
	TradeCount      int     `json:"trade_count"`
}
```

Algorithm (from feature doc):
```
running_units = 0
running_cost  = 0
realized_pnl  = 0
gross_invested = 0
gross_proceeds = 0

For each trade ordered by date:
  If buy:
    cost := (trade.units × trade.price) + trade.fees
    running_cost  += cost
    running_units += trade.units
    gross_invested += cost

  If sell:
    avg_cost_at_sell = running_cost / running_units
    cost_of_sold     = avg_cost_at_sell × trade.units
    proceeds         = (trade.units × trade.price) - trade.fees
    realized_pnl    += proceeds - cost_of_sold
    running_cost    -= cost_of_sold
    running_units   -= trade.units
    gross_proceeds  += proceeds

holding.units       = running_units
holding.avg_cost    = running_cost / running_units  (if units > 0, else 0)
holding.cost_basis  = running_cost
holding.realized    = realized_pnl
holding.unrealized  = (currentPrice × running_units) - running_cost
holding.market_value = currentPrice × running_units
holding.gross_invested = gross_invested
holding.gross_proceeds = gross_proceeds
```

##### `DeriveAllHoldings(ctx, tradeBook) → []DerivedHolding`
- Group trades by ticker
- Call DeriveHolding for each ticker (pass currentPrice=0 — price enrichment is caller's job)
- Return list of derived holdings

##### `saveTradeBook(ctx, tradeBook) error`
- Follow cashflow saveLedger pattern:
  - Increment version
  - Set UpdatedAt = time.Now()
  - Marshal to JSON
  - UserDataStore.Put(ctx, &models.UserRecord{UserID, Subject: "trades", Key: portfolioName, Value: jsonStr})

**Trade filter struct:**
```go
type TradeFilter struct {
	Ticker     string
	Action     models.TradeAction
	DateFrom   time.Time
	DateTo     time.Time
	SourceType models.SourceType
	Limit      int  // default 50, max 200
	Offset     int
}
```

---

#### 3. `internal/services/trade/service_test.go` — Unit tests

Test cases (follow `internal/services/cashflow/service_test.go` patterns):

1. **TestAddTrade_Buy** — add a buy trade, verify ID generated, trade stored, holding derived
2. **TestAddTrade_Sell** — add buy then sell, verify realized P&L
3. **TestAddTrade_SellValidation** — sell more than held → error
4. **TestAddTrade_ValidationErrors** — empty ticker, zero units, invalid action → errors
5. **TestRemoveTrade** — add trade, remove it, verify gone
6. **TestRemoveTrade_NotFound** — remove nonexistent ID → error
7. **TestUpdateTrade** — add trade, update price, verify merge semantics
8. **TestUpdateTrade_NotFound** — update nonexistent ID → error
9. **TestListTrades** — add multiple trades, list with/without filters
10. **TestListTrades_Pagination** — verify offset/limit
11. **TestDeriveHolding_SingleBuy** — one buy → units, avg_cost, cost_basis correct
12. **TestDeriveHolding_MultipleBuys** — weighted average cost calculation
13. **TestDeriveHolding_BuyThenSell** — partial sell → realized P&L, remaining position
14. **TestDeriveHolding_FullSell** — sell all units → zero position, all realized
15. **TestDeriveHolding_MultipleBuySell** — complex sequence with multiple buys/sells
16. **TestSnapshotPositions_Replace** — replace mode clears and inserts
17. **TestSnapshotPositions_Merge** — merge mode updates matching, adds new
18. **TestGenerateTradeID** — verify "tr_" prefix, 8 hex chars, uniqueness

**Mock storage:** Create a `mockStorageManager` with in-memory map for UserDataStore, following the test pattern in `internal/services/cashflow/service_test.go`.

---

### Modified Files

#### 4. `internal/models/portfolio.go` — Add SourceType fields

**Portfolio struct (line 43-83):** Add after `NavexaID` field (line 46):
```go
SourceType  SourceType `json:"source_type,omitempty"`   // navexa (default), manual, snapshot, hybrid
```

**Holding struct (line 111-147):** Add after `Name` field (line 114):
```go
SourceType SourceType `json:"source_type,omitempty"`  // navexa, manual, snapshot, csv
SourceRef  string     `json:"source_ref,omitempty"`   // free-form provenance tag
```

That's it for portfolio.go. The SourceType type definition goes in trade.go (new file).

---

#### 5. `internal/interfaces/services.go` — Add TradeService interface

Add after CashFlowService (after line 282):

```go
// TradeService manages trades and position derivation for manual/snapshot portfolios
type TradeService interface {
	// GetTradeBook retrieves the trade book for a portfolio
	GetTradeBook(ctx context.Context, portfolioName string) (*models.TradeBook, error)

	// AddTrade records a buy or sell trade and returns the trade + derived holding
	AddTrade(ctx context.Context, portfolioName string, trade models.Trade) (*models.Trade, *models.DerivedHolding, error)

	// RemoveTrade deletes a trade by ID and returns the updated trade book
	RemoveTrade(ctx context.Context, portfolioName string, tradeID string) (*models.TradeBook, error)

	// UpdateTrade updates a trade by ID (merge semantics)
	UpdateTrade(ctx context.Context, portfolioName string, tradeID string, update models.Trade) (*models.Trade, error)

	// ListTrades returns trades matching the filter criteria
	ListTrades(ctx context.Context, portfolioName string, filter TradeFilter) ([]models.Trade, int, error)

	// SnapshotPositions bulk-imports positions for snapshot-type portfolios
	SnapshotPositions(ctx context.Context, portfolioName string, positions []models.SnapshotPosition, mode string, sourceRef string, snapshotDate string) (*models.TradeBook, error)

	// DeriveHoldings computes holdings from the trade book
	DeriveHoldings(ctx context.Context, portfolioName string) ([]models.DerivedHolding, error)
}

// TradeFilter configures trade list queries
type TradeFilter struct {
	Ticker     string
	Action     models.TradeAction
	DateFrom   time.Time
	DateTo     time.Time
	SourceType models.SourceType
	Limit      int
	Offset     int
}
```

---

#### 6. `internal/services/portfolio/service.go` — CreatePortfolio + GetPortfolio routing

##### Add `tradeService` field to Service struct (line ~25):
```go
type Service struct {
	storage        interfaces.StorageManager
	navexa         interfaces.NavexaClient
	eodhd          interfaces.EODHDClient
	cashflowSvc    interfaces.CashFlowService
	tradeService   interfaces.TradeService      // ← NEW
	signalComputer *signals.Computer
	// ... existing fields
}
```

##### Add setter (follow `SetCashFlowService` pattern):
```go
func (s *Service) SetTradeService(svc interfaces.TradeService) {
	s.tradeService = svc
}
```

##### Add `CreatePortfolio` method:
```go
func (s *Service) CreatePortfolio(ctx context.Context, name string, sourceType models.SourceType, currency string) (*models.Portfolio, error) {
	// Validate source type
	if !models.ValidPortfolioSourceTypes[sourceType] {
		return nil, fmt.Errorf("invalid source_type: %q (valid: manual, snapshot, hybrid)", sourceType)
	}
	// Validate name
	if name == "" {
		return nil, fmt.Errorf("portfolio name is required")
	}
	if len(name) > 100 {
		return nil, fmt.Errorf("portfolio name too long (max 100 chars)")
	}
	// Check if already exists
	existing, _ := s.getPortfolioRecord(ctx, name)
	if existing != nil {
		return nil, fmt.Errorf("portfolio %q already exists", name)
	}
	// Default currency
	if currency == "" {
		currency = "AUD"
	}
	now := time.Now()
	portfolio := &models.Portfolio{
		Name:        name,
		SourceType:  sourceType,
		Currency:    currency,
		DataVersion: common.SchemaVersion,
		LastSynced:  now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	// Save to UserDataStore
	if err := s.savePortfolioRecord(ctx, portfolio); err != nil {
		return nil, err
	}
	return portfolio, nil
}
```

##### Modify `GetPortfolio` (line 547-568):

```go
func (s *Service) GetPortfolio(ctx context.Context, name string) (*models.Portfolio, error) {
	portfolio, err := s.getPortfolioRecord(ctx, name)
	if err != nil {
		// For Navexa portfolios: auto-sync on first access
		if synced, syncErr := s.SyncPortfolio(ctx, name, false); syncErr == nil {
			return synced, nil
		}
		return nil, err
	}

	// Route by source type
	switch portfolio.SourceType {
	case models.SourceManual:
		return s.assembleManualPortfolio(ctx, portfolio)
	case models.SourceSnapshot:
		return s.assembleSnapshotPortfolio(ctx, portfolio)
	case models.SourceNavexa, "":
		// Existing Navexa behaviour
		if !common.IsFresh(portfolio.LastSynced, common.FreshnessPortfolio) {
			if synced, syncErr := s.SyncPortfolio(ctx, name, false); syncErr == nil {
				return synced, nil
			}
		}
		s.populateHistoricalValues(ctx, portfolio)
		return portfolio, nil
	default:
		// Unknown source type, return as-is
		s.populateHistoricalValues(ctx, portfolio)
		return portfolio, nil
	}
}
```

##### Add `assembleManualPortfolio`:
```go
func (s *Service) assembleManualPortfolio(ctx context.Context, portfolio *models.Portfolio) (*models.Portfolio, error) {
	if s.tradeService == nil {
		return portfolio, nil
	}
	derived, err := s.tradeService.DeriveHoldings(ctx, portfolio.Name)
	if err != nil {
		return nil, fmt.Errorf("deriving holdings from trades: %w", err)
	}

	// Convert DerivedHoldings to Holdings
	holdings := make([]models.Holding, 0, len(derived))
	var totalEquityValue, totalCost, totalRealized, totalUnrealized, totalGrossInvested float64
	for _, dh := range derived {
		// Only include open positions (units > 0)
		if dh.Units <= 0 {
			continue
		}
		h := models.Holding{
			Ticker:         dh.Ticker,
			Exchange:       models.EodhExchange(tickerExchange(dh.Ticker)),
			Name:           dh.Ticker, // will be enriched with market data if available
			Status:         "open",
			Units:          dh.Units,
			AvgCost:        dh.AvgCost,
			CurrentPrice:   dh.AvgCost, // default to avg cost; enrich with market data below
			MarketValue:    dh.MarketValue,
			CostBasis:      dh.CostBasis,
			GrossInvested:  dh.GrossInvested,
			GrossProceeds:  dh.GrossProceeds,
			RealizedReturn: dh.RealizedReturn,
			UnrealizedReturn: dh.UnrealizedReturn,
			SourceType:     models.SourceManual,
			Currency:       portfolio.Currency,
		}

		// Try to enrich with current market price
		if s.eodhd != nil {
			if md, err := s.storage.MarketDataStorage().GetMarketData(ctx, dh.Ticker); err == nil && md != nil && len(md.EOD) > 0 {
				latestPrice := md.EOD[len(md.EOD)-1].Close
				if latestPrice > 0 {
					h.CurrentPrice = latestPrice
					h.MarketValue = latestPrice * dh.Units
					h.UnrealizedReturn = h.MarketValue - dh.CostBasis
				}
			}
		}

		h.NetReturn = h.RealizedReturn + h.UnrealizedReturn
		if h.GrossInvested > 0 {
			h.NetReturnPct = (h.NetReturn / h.GrossInvested) * 100
		}

		totalEquityValue += h.MarketValue
		totalCost += h.CostBasis
		totalRealized += h.RealizedReturn
		totalUnrealized += h.UnrealizedReturn
		totalGrossInvested += h.GrossInvested

		holdings = append(holdings, h)
	}

	// Compute portfolio weights
	for i := range holdings {
		if totalEquityValue > 0 {
			holdings[i].PortfolioWeightPct = (holdings[i].MarketValue / totalEquityValue) * 100
		}
	}

	portfolio.Holdings = holdings
	portfolio.EquityValue = totalEquityValue
	portfolio.NetEquityCost = totalCost
	portfolio.NetEquityReturn = totalRealized + totalUnrealized
	if totalGrossInvested > 0 {
		portfolio.NetEquityReturnPct = (portfolio.NetEquityReturn / totalGrossInvested) * 100
	}
	portfolio.RealizedEquityReturn = totalRealized
	portfolio.UnrealizedEquityReturn = totalUnrealized
	portfolio.PortfolioValue = totalEquityValue + portfolio.GrossCashBalance
	portfolio.CalculationMethod = "average_cost"

	// Populate historical values (market data for price changes)
	s.populateHistoricalValues(ctx, portfolio)

	return portfolio, nil
}
```

**Helper function:**
```go
// tickerExchange extracts the exchange suffix from a ticker (e.g. "BHP.AU" → "AU")
func tickerExchange(ticker string) string {
	parts := strings.SplitN(ticker, ".", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return "AU"
}
```

##### Add `assembleSnapshotPortfolio`:
```go
func (s *Service) assembleSnapshotPortfolio(ctx context.Context, portfolio *models.Portfolio) (*models.Portfolio, error) {
	if s.tradeService == nil {
		return portfolio, nil
	}
	tb, err := s.tradeService.GetTradeBook(ctx, portfolio.Name)
	if err != nil {
		return nil, err
	}

	holdings := make([]models.Holding, 0, len(tb.SnapshotPositions))
	var totalEquityValue, totalCost float64
	for _, sp := range tb.SnapshotPositions {
		costBasis := sp.AvgCost * sp.Units
		marketValue := sp.MarketValue
		if marketValue == 0 && sp.CurrentPrice > 0 {
			marketValue = sp.CurrentPrice * sp.Units
		}
		if marketValue == 0 {
			marketValue = costBasis // fallback
		}

		h := models.Holding{
			Ticker:       sp.Ticker,
			Exchange:     models.EodhExchange(tickerExchange(sp.Ticker)),
			Name:         sp.Name,
			Status:       "open",
			Units:        sp.Units,
			AvgCost:      sp.AvgCost,
			CurrentPrice: sp.CurrentPrice,
			MarketValue:  marketValue,
			CostBasis:    costBasis,
			GrossInvested: costBasis + sp.FeesTotal,
			UnrealizedReturn: marketValue - costBasis,
			SourceType:   models.SourceSnapshot,
			SourceRef:    sp.SourceRef,
			Currency:     portfolio.Currency,
		}
		h.NetReturn = h.UnrealizedReturn
		if h.GrossInvested > 0 {
			h.NetReturnPct = (h.NetReturn / h.GrossInvested) * 100
		}

		totalEquityValue += h.MarketValue
		totalCost += h.CostBasis
		holdings = append(holdings, h)
	}

	// Compute weights
	for i := range holdings {
		if totalEquityValue > 0 {
			holdings[i].PortfolioWeightPct = (holdings[i].MarketValue / totalEquityValue) * 100
		}
	}

	portfolio.Holdings = holdings
	portfolio.EquityValue = totalEquityValue
	portfolio.NetEquityCost = totalCost
	portfolio.NetEquityReturn = totalEquityValue - totalCost
	if totalCost > 0 {
		portfolio.NetEquityReturnPct = ((totalEquityValue - totalCost) / totalCost) * 100
	}
	portfolio.UnrealizedEquityReturn = totalEquityValue - totalCost
	portfolio.PortfolioValue = totalEquityValue + portfolio.GrossCashBalance
	portfolio.CalculationMethod = "snapshot"

	s.populateHistoricalValues(ctx, portfolio)
	return portfolio, nil
}
```

##### Update `ListPortfolios` to return source types:

The current `ListPortfolios` returns `[]string`. We need to keep backward compat but also expose source_type. The interface change is small — we can have the handler read the full portfolio data when building the list response. Or better: add a new method `ListPortfolioSummaries` that returns richer data.

**Actually** — the current `handlePortfolioList` handler already reads portfolios. Let's just keep `ListPortfolios` returning `[]string` and have the handler enrich with source_type when building the response. This avoids interface changes for now.

##### Add `CreatePortfolio` to the PortfolioService interface:

In `internal/interfaces/services.go`, add to PortfolioService (after line 28):
```go
// CreatePortfolio creates a new manually-managed portfolio
CreatePortfolio(ctx context.Context, name string, sourceType models.SourceType, currency string) (*models.Portfolio, error)
```

##### Add `savePortfolioRecord` helper:

Follow the `getPortfolioRecord` pattern. Read the existing code at `service.go` near line ~940+ to find how it saves portfolios. The existing code likely saves via `UserDataStore.Put`. Add:
```go
func (s *Service) savePortfolioRecord(ctx context.Context, portfolio *models.Portfolio) error {
	userID := common.ResolveUserID(ctx)
	data, err := json.Marshal(portfolio)
	if err != nil {
		return fmt.Errorf("marshalling portfolio: %w", err)
	}
	return s.storage.UserDataStore().Put(ctx, &models.UserRecord{
		UserID:  userID,
		Subject: "portfolio",
		Key:     portfolio.Name,
		Value:   string(data),
	})
}
```

**Important:** Check if `savePortfolioRecord` or equivalent already exists. The existing SyncPortfolio likely calls something similar after computing the portfolio. Find it and reuse if possible.

---

#### 7. `internal/server/handlers.go` — New handler functions

Add new handlers after the cash flow handlers section (~line 2220+).

##### `handlePortfolioCreate` — POST /api/portfolios

```go
func (s *Server) handlePortfolioCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	var req struct {
		Name       string           `json:"name"`
		SourceType models.SourceType `json:"source_type"`
		Currency   string           `json:"currency"`
	}
	if !DecodeJSON(w, r, &req) {
		return
	}
	ctx := s.authenticatedContext(r)
	portfolio, err := s.app.PortfolioService.CreatePortfolio(ctx, req.Name, req.SourceType, req.Currency)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "required") {
			WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error creating portfolio: %v", err))
		return
	}
	WriteJSON(w, http.StatusCreated, portfolio)
}
```

##### `handleTrades` — GET/POST /api/portfolios/{name}/trades

```go
func (s *Server) handleTrades(w http.ResponseWriter, r *http.Request, portfolioName string) {
	ctx := s.authenticatedContext(r)
	switch r.Method {
	case http.MethodGet:
		// ListTrades with filters from query params
		filter := interfaces.TradeFilter{
			Ticker:  r.URL.Query().Get("ticker"),
			Limit:   50,
		}
		if action := r.URL.Query().Get("action"); action != "" {
			filter.Action = models.TradeAction(action)
		}
		if dateFrom := r.URL.Query().Get("date_from"); dateFrom != "" {
			if t, err := time.Parse("2006-01-02", dateFrom); err == nil {
				filter.DateFrom = t
			}
		}
		if dateTo := r.URL.Query().Get("date_to"); dateTo != "" {
			if t, err := time.Parse("2006-01-02", dateTo); err == nil {
				filter.DateTo = t
			}
		}
		if src := r.URL.Query().Get("source_type"); src != "" {
			filter.SourceType = models.SourceType(src)
		}
		if limit := r.URL.Query().Get("limit"); limit != "" {
			if n, err := strconv.Atoi(limit); err == nil && n > 0 {
				filter.Limit = n
			}
		}
		if offset := r.URL.Query().Get("offset"); offset != "" {
			if n, err := strconv.Atoi(offset); err == nil && n >= 0 {
				filter.Offset = n
			}
		}
		trades, total, err := s.app.TradeService.ListTrades(ctx, portfolioName, filter)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error listing trades: %v", err))
			return
		}
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"trades": trades,
			"total":  total,
		})

	case http.MethodPost:
		// AddTrade
		var req struct {
			Ticker     string  `json:"ticker"`
			Action     string  `json:"action"`
			Units      float64 `json:"units"`
			Price      float64 `json:"price"`
			Fees       float64 `json:"fees"`
			Date       string  `json:"date"`
			SettleDate string  `json:"settle_date"`
			SourceType string  `json:"source_type"`
			SourceRef  string  `json:"source_ref"`
			Notes      string  `json:"notes"`
		}
		if !DecodeJSON(w, r, &req) {
			return
		}
		date, err := time.Parse("2006-01-02", req.Date)
		if err != nil {
			WriteError(w, http.StatusBadRequest, fmt.Sprintf("invalid date format: %v (expected YYYY-MM-DD)", err))
			return
		}
		trade := models.Trade{
			Ticker:     req.Ticker,
			Action:     models.TradeAction(req.Action),
			Units:      req.Units,
			Price:      req.Price,
			Fees:       req.Fees,
			Date:       date,
			SettleDate: req.SettleDate,
			SourceType: models.SourceType(req.SourceType),
			SourceRef:  req.SourceRef,
			Notes:      req.Notes,
		}
		created, holding, err := s.app.TradeService.AddTrade(ctx, portfolioName, trade)
		if err != nil {
			if strings.Contains(err.Error(), "insufficient") || strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "required") {
				WriteError(w, http.StatusBadRequest, err.Error())
				return
			}
			WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error adding trade: %v", err))
			return
		}
		WriteJSON(w, http.StatusCreated, map[string]interface{}{
			"trade":   created,
			"holding": holding,
		})

	default:
		WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}
```

##### `handleTradeItem` — PUT/DELETE /api/portfolios/{name}/trades/{id}

```go
func (s *Server) handleTradeItem(w http.ResponseWriter, r *http.Request, portfolioName, tradeID string) {
	ctx := s.authenticatedContext(r)
	switch r.Method {
	case http.MethodPut:
		var req models.Trade
		if !DecodeJSON(w, r, &req) {
			return
		}
		updated, err := s.app.TradeService.UpdateTrade(ctx, portfolioName, tradeID, req)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				WriteError(w, http.StatusNotFound, err.Error())
				return
			}
			WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error updating trade: %v", err))
			return
		}
		WriteJSON(w, http.StatusOK, updated)

	case http.MethodDelete:
		tb, err := s.app.TradeService.RemoveTrade(ctx, portfolioName, tradeID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				WriteError(w, http.StatusNotFound, err.Error())
				return
			}
			WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error removing trade: %v", err))
			return
		}
		WriteJSON(w, http.StatusOK, tb)

	default:
		WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}
```

##### `handlePortfolioSnapshotImport` — POST /api/portfolios/{name}/snapshot

**Note:** The route "snapshot" is already used for `handlePortfolioSnapshot` (historical portfolio snapshot). Use "import" or "snapshot-import" to avoid collision. Check the existing handler:
- `case "snapshot":` at routes.go:222 → `handlePortfolioSnapshot` — this is for GET (point-in-time historical snapshot)
- Our new endpoint is POST (bulk import positions). So we can use the same route but dispatch by HTTP method.

Actually, looking at routes.go line 222: `s.handlePortfolioSnapshot(w, r, name)` — this handles GET for historical snapshots. We need POST for imports. So:
- **GET /api/portfolios/{name}/snapshot** → existing `handlePortfolioSnapshot` (historical)
- **POST /api/portfolios/{name}/snapshot** → new `handlePortfolioSnapshotImport` (bulk import)

Modify `handlePortfolioSnapshot` to check method and dispatch:
```go
// In the existing handlePortfolioSnapshot, add at the top:
if r.Method == http.MethodPost {
    s.handlePortfolioSnapshotImport(w, r, name)
    return
}
// ... existing GET logic
```

The import handler:
```go
func (s *Server) handlePortfolioSnapshotImport(w http.ResponseWriter, r *http.Request, portfolioName string) {
	var req struct {
		Positions    []models.SnapshotPosition `json:"positions"`
		Mode         string                    `json:"mode"`          // "replace" (default) or "merge"
		SourceRef    string                    `json:"source_ref"`
		SnapshotDate string                    `json:"snapshot_date"` // YYYY-MM-DD, default today
	}
	if !DecodeJSON(w, r, &req) {
		return
	}
	if len(req.Positions) == 0 {
		WriteError(w, http.StatusBadRequest, "positions array is required and cannot be empty")
		return
	}
	if req.Mode == "" {
		req.Mode = "replace"
	}
	if req.Mode != "replace" && req.Mode != "merge" {
		WriteError(w, http.StatusBadRequest, fmt.Sprintf("invalid mode: %q (valid: replace, merge)", req.Mode))
		return
	}

	ctx := s.authenticatedContext(r)

	// Auto-create portfolio if it doesn't exist
	if _, err := s.app.PortfolioService.GetPortfolio(ctx, portfolioName); err != nil {
		if _, createErr := s.app.PortfolioService.CreatePortfolio(ctx, portfolioName, models.SourceSnapshot, "AUD"); createErr != nil {
			// Ignore if already exists
			if !strings.Contains(createErr.Error(), "already exists") {
				WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error creating portfolio: %v", createErr))
				return
			}
		}
	}

	tb, err := s.app.TradeService.SnapshotPositions(ctx, portfolioName, req.Positions, req.Mode, req.SourceRef, req.SnapshotDate)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error importing snapshot: %v", err))
		return
	}

	WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"portfolio_name": portfolioName,
		"positions":      len(tb.SnapshotPositions),
		"mode":           req.Mode,
	})
}
```

---

#### 8. `internal/server/routes.go` — Route registration

In `routePortfolios` (line 196-277):

**Add trade routes** — add to the `default:` block (line 245+):
```go
} else if subpath == "trades" {
	s.handleTrades(w, r, name)
} else if strings.HasPrefix(subpath, "trades/") {
	tradeID := strings.TrimPrefix(subpath, "trades/")
	s.handleTradeItem(w, r, name, tradeID)
```

**Add portfolio creation** — in `routePortfolios`, handle empty path POST:
```go
// At line 199-201, modify:
if path == "" {
	if r.Method == http.MethodPost {
		s.handlePortfolioCreate(w, r)
		return
	}
	s.handlePortfolioList(w, r)
	return
}
```

**Snapshot import** — modify existing `case "snapshot":` to also handle POST:
The existing handler already handles this route. Modify the existing `handlePortfolioSnapshot` function to dispatch POST to the new import handler as described above.

---

#### 9. `internal/server/catalog.go` — MCP tool definitions

Add after the cash flow tools section. Follow the naming convention: `{category}_{action}_{sub}`.

```go
// --- Trades ---
{
	Name:        "portfolio_create",
	Description: "Create a new portfolio that is not sourced from Navexa. Use for manual tracking, screenshot imports, or hybrid portfolios.",
	Method:      "POST",
	Path:        "/api/portfolios",
	Params: []models.ParamDefinition{
		{Name: "name", Type: "string", Description: "Portfolio name", Required: true, In: "body"},
		{Name: "source_type", Type: "string", Description: "Source type: manual, snapshot, or hybrid", Required: true, In: "body"},
		{Name: "currency", Type: "string", Description: "Base currency (default: AUD)", In: "body"},
	},
},
{
	Name:        "trade_add",
	Description: "Record a single buy or sell trade. Creates the holding if the ticker doesn't exist. Returns the trade and updated holding summary.",
	Method:      "POST",
	Path:        "/api/portfolios/{portfolio_name}/trades",
	Params: []models.ParamDefinition{
		portfolioParam,
		{Name: "ticker", Type: "string", Description: "Stock ticker (e.g. 'BHP.AU')", Required: true, In: "body"},
		{Name: "action", Type: "string", Description: "Trade action: buy or sell", Required: true, In: "body"},
		{Name: "units", Type: "number", Description: "Number of shares/units", Required: true, In: "body"},
		{Name: "price", Type: "number", Description: "Price per unit (excluding fees)", Required: true, In: "body"},
		{Name: "date", Type: "string", Description: "Trade date (YYYY-MM-DD)", Required: true, In: "body"},
		{Name: "fees", Type: "number", Description: "Brokerage/commission (default: 0)", In: "body"},
		{Name: "settle_date", Type: "string", Description: "Settlement date (optional)", In: "body"},
		{Name: "source_type", Type: "string", Description: "Source: manual (default), snapshot, csv", In: "body"},
		{Name: "source_ref", Type: "string", Description: "Provenance tag (e.g. 'commsec:2026-03-02')", In: "body"},
		{Name: "notes", Type: "string", Description: "Free-form notes", In: "body"},
	},
},
{
	Name:        "trade_list",
	Description: "List trades for a portfolio with optional filters. Returns trades array and total count for pagination.",
	Method:      "GET",
	Path:        "/api/portfolios/{portfolio_name}/trades",
	Params: []models.ParamDefinition{
		portfolioParam,
		{Name: "ticker", Type: "string", Description: "Filter by ticker", In: "query"},
		{Name: "action", Type: "string", Description: "Filter by action: buy or sell", In: "query"},
		{Name: "date_from", Type: "string", Description: "Filter by start date (YYYY-MM-DD)", In: "query"},
		{Name: "date_to", Type: "string", Description: "Filter by end date (YYYY-MM-DD)", In: "query"},
		{Name: "source_type", Type: "string", Description: "Filter by source type", In: "query"},
		{Name: "limit", Type: "number", Description: "Max results (default: 50, max: 200)", In: "query"},
		{Name: "offset", Type: "number", Description: "Pagination offset", In: "query"},
	},
},
{
	Name:        "trade_update",
	Description: "Update a trade by ID. Merge semantics — only provided fields are changed. Recalculates the holding position.",
	Method:      "PUT",
	Path:        "/api/portfolios/{portfolio_name}/trades/{id}",
	Params: []models.ParamDefinition{
		portfolioParam,
		{Name: "id", Type: "string", Description: "Trade ID (e.g. 'tr_1a2b3c4d')", Required: true, In: "path"},
		{Name: "units", Type: "number", Description: "Updated units", In: "body"},
		{Name: "price", Type: "number", Description: "Updated price per unit", In: "body"},
		{Name: "fees", Type: "number", Description: "Updated fees", In: "body"},
		{Name: "date", Type: "string", Description: "Updated trade date (YYYY-MM-DD)", In: "body"},
		{Name: "notes", Type: "string", Description: "Updated notes", In: "body"},
	},
},
{
	Name:        "trade_remove",
	Description: "Delete a trade by ID. Recalculates the holding position. Removes the holding if no trades remain.",
	Method:      "DELETE",
	Path:        "/api/portfolios/{portfolio_name}/trades/{id}",
	Params: []models.ParamDefinition{
		portfolioParam,
		{Name: "id", Type: "string", Description: "Trade ID (e.g. 'tr_1a2b3c4d')", Required: true, In: "body"},
	},
},
{
	Name:        "portfolio_snapshot",
	Description: "Bulk-import positions from a screenshot or external source. For snapshot-type portfolios where positions are the source of truth. Auto-creates portfolio if it doesn't exist.",
	Method:      "POST",
	Path:        "/api/portfolios/{portfolio_name}/snapshot",
	Params: []models.ParamDefinition{
		portfolioParam,
		{Name: "positions", Type: "array", Description: "Array of positions: {ticker (required), name, units (required), avg_cost (required), current_price, market_value, fees_total, notes}", Required: true, In: "body"},
		{Name: "mode", Type: "string", Description: "Import mode: replace (default) or merge", In: "body"},
		{Name: "source_ref", Type: "string", Description: "Provenance tag (e.g. 'commsec:2026-03-02')", In: "body"},
		{Name: "snapshot_date", Type: "string", Description: "Date of the snapshot (default: today)", In: "body"},
	},
},
```

---

#### 10. `internal/app/app.go` — Wire TradeService

Add `TradeService` field to App struct (after line 46):
```go
TradeService     interfaces.TradeService
```

In `NewApp()` (after cashflowService creation, ~line 194):
```go
tradeService := trade.NewService(storageManager, logger)
portfolioService.SetTradeService(tradeService) // inject trade service
```

Add to App struct initialization (~line 240):
```go
TradeService:     tradeService,
```

Add import:
```go
"github.com/bobmcallan/vire/internal/services/trade"
```

---

#### 11. `internal/common/version.go` — Schema version bump

Change line 15:
```go
const SchemaVersion = "14"  // was "13" — source-typed portfolios
```

---

#### 12. All mock PortfolioService implementations — Add CreatePortfolio stub

Search for `ListPortfolios` in test files to find all mock implementations. Each needs a new `CreatePortfolio` method:
```go
func (m *mockPortfolioService) CreatePortfolio(_ context.Context, _ string, _ models.SourceType, _ string) (*models.Portfolio, error) {
	return nil, fmt.Errorf("not implemented")
}
```

Files to update (from Grep results):
- `internal/server/handlers_portfolio_test.go`
- `internal/services/cashflow/service_test.go`
- `internal/services/report/devils_advocate_test.go`

---

## Integration Points

1. **Trade service → Portfolio service:** Portfolio.GetPortfolio calls TradeService.DeriveHoldings for manual portfolios
2. **Trade service → UserDataStore:** Trades stored as subject="trades", key=portfolio_name
3. **Portfolio service → Market data:** assembleManualPortfolio enriches holdings with current prices from MarketDataStorage
4. **App bootstrap:** TradeService created and injected into PortfolioService via SetTradeService

---

## Testing Strategy

### Unit Tests (implementer)
- `internal/services/trade/service_test.go` — 18 test cases (listed above)
- Position derivation correctness is critical — test edge cases

### Integration Tests (test-creator)
- `tests/data/trade_test.go` or `tests/api/trade_test.go`:
  1. Create manual portfolio → verify source_type in response
  2. Add trade → verify holding derived
  3. Add multiple trades → verify position aggregation
  4. Sell trade → verify realized P&L
  5. Remove trade → verify position recalculated
  6. List trades with filters
  7. Snapshot import (replace mode)
  8. Snapshot import (merge mode)
  9. Get portfolio for manual portfolio → verify holdings from trades
  10. Get portfolio for snapshot portfolio → verify holdings from positions

---

## Risks & Mitigations

1. **Route collision:** "snapshot" path is already used for GET (historical). Using method dispatch (GET=historical, POST=import) avoids collision.
2. **Schema version bump:** Purges all cached portfolios and market data on restart. Necessary because Portfolio struct changed.
3. **Market price enrichment:** For manual portfolios, current prices come from cached market data. If no market data is cached, holdings show avg_cost as current price. This is acceptable — user can trigger market data collection.
4. **No Navexa for manual portfolios:** SyncPortfolio is skipped entirely. This is correct — manual portfolios have no Navexa connection.
