# Requirements: Asset Categories for SMSF Portfolios

## Scope

**In scope:**
- New `AssetCategory` type to classify asset groups (equity, property, crypto, fixed_income, collectibles, other)
- New `AssetSet` model — a named, manually-valued collection of assets within a category
- New `AssetItem` model — individual assets within a set (with value, cost basis, acquisition date)
- New `AssetSetService` with full CRUD (get collection, add/update/remove sets, add/update/remove items)
- New API endpoints under `/api/portfolios/{name}/asset-sets`
- Portfolio value integration: `PortfolioValue` includes asset set total values
- Timeline snapshot integration: asset set values persisted in snapshots
- Multiple asset sets of the same category per portfolio (e.g., two property sets)
- Default category is `equity` (existing Holdings remain untouched — they ARE the equity category)

**Out of scope:**
- Migrating existing Holdings into AssetSets (Holdings remain first-class)
- Migrating existing CashFlowLedger into AssetSets (cash remains first-class)
- Automated valuations or market data for non-equity assets
- SMSF compliance rules enforcement
- Portal UI changes

## Architecture

The design adds a new parallel concept alongside Holdings and Cash — **AssetSets** — for manually-valued non-equity assets. This preserves the existing equity + cash architecture completely.

```
Portfolio
├── Holdings[]         (equity positions — existing, untouched)
├── CashFlowLedger     (cash accounts — existing, untouched)
├── AssetSets[]        (NEW: property, crypto, etc.)
└── Computed values
    ├── EquityHoldingsValue  (sum of equity holdings)
    ├── AssetSetsValue       (NEW: sum of all asset set values)
    ├── CapitalGross         (cash balance)
    └── PortfolioValue       (equity + cash + asset sets)
```

Storage: UserDataStore with `subject="asset_sets"`, key=portfolio name. One document per portfolio containing all asset sets.

## Files to Change

### 1. NEW: `internal/models/asset_set.go`

```go
package models

import "time"

// AssetCategory classifies the type of asset set.
type AssetCategory string

const (
	AssetCategoryEquity      AssetCategory = "equity"       // Stocks, ETFs (default — represented by Holdings[], not AssetSets)
	AssetCategoryProperty    AssetCategory = "property"     // Real estate
	AssetCategoryFixedIncome AssetCategory = "fixed_income" // Bonds, term deposits
	AssetCategoryCrypto      AssetCategory = "crypto"       // Cryptocurrency
	AssetCategoryCollectible AssetCategory = "collectible"  // Art, wine, cars, etc.
	AssetCategoryOther       AssetCategory = "other"        // Catch-all
)

// validAssetCategories lists all accepted categories for manual asset sets.
// Equity is excluded — equity holdings use the existing Holdings[] system.
var validAssetCategories = map[AssetCategory]bool{
	AssetCategoryProperty:    true,
	AssetCategoryFixedIncome: true,
	AssetCategoryCrypto:      true,
	AssetCategoryCollectible: true,
	AssetCategoryOther:       true,
}

// ValidAssetCategory returns true if c is a valid category for manual asset sets.
func ValidAssetCategory(c AssetCategory) bool {
	return validAssetCategories[c]
}

// AssetItem represents a single asset within an asset set.
type AssetItem struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`                    // e.g., "123 Main St", "Bitcoin"
	Value       float64   `json:"value"`                   // Current estimated market value
	CostBasis   float64   `json:"cost_basis"`              // Original purchase price / cost
	AcquiredAt  time.Time `json:"acquired_at,omitempty"`   // When acquired
	Description string    `json:"description,omitempty"`
	Notes       string    `json:"notes,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// AssetSet is a named collection of assets within a category.
// A portfolio can have multiple sets of the same category
// (e.g., "Sydney Property", "Melbourne Property").
type AssetSet struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`              // e.g., "Investment Properties", "Crypto Holdings"
	Category  AssetCategory `json:"category"`           // property, crypto, etc.
	Items     []AssetItem   `json:"items"`
	Currency  string        `json:"currency,omitempty"` // ISO 4217 (default AUD)
	Notes     string        `json:"notes,omitempty"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

// TotalValue returns the sum of all item values in this set.
func (s *AssetSet) TotalValue() float64 {
	var total float64
	for _, item := range s.Items {
		total += item.Value
	}
	return total
}

// TotalCostBasis returns the sum of all item cost bases in this set.
func (s *AssetSet) TotalCostBasis() float64 {
	var total float64
	for _, item := range s.Items {
		total += item.CostBasis
	}
	return total
}

// PortfolioAssetSets is the versioned collection of asset sets per portfolio.
// Stored as a single document in UserDataStore (subject="asset_sets").
type PortfolioAssetSets struct {
	PortfolioName string     `json:"portfolio_name"`
	Version       int        `json:"version"`
	Sets          []AssetSet `json:"sets"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// TotalValue returns the aggregate value across all asset sets.
func (p *PortfolioAssetSets) TotalValue() float64 {
	var total float64
	for _, s := range p.Sets {
		total += s.TotalValue()
	}
	return total
}

// TotalCostBasis returns the aggregate cost basis across all asset sets.
func (p *PortfolioAssetSets) TotalCostBasis() float64 {
	var total float64
	for _, s := range p.Sets {
		total += s.TotalCostBasis()
	}
	return total
}

// FindSetByID returns the set and index for a given ID, or nil/-1 if not found.
func (p *PortfolioAssetSets) FindSetByID(id string) (*AssetSet, int) {
	for i := range p.Sets {
		if p.Sets[i].ID == id {
			return &p.Sets[i], i
		}
	}
	return nil, -1
}
```

### 2. NEW: `internal/services/assetset/service.go`

Follow the pattern of `internal/services/holdingnotes/service.go` (same storage pattern: UserDataStore, subject="asset_sets", key=portfolio name).

```go
package assetset

// Service implements AssetSetService
type Service struct {
	storage interfaces.StorageManager
	portfolioSvc interfaces.PortfolioService // for timeline invalidation
	logger  *common.Logger
}
```

**Methods (follow holdingnotes pattern exactly):**
- `GetAssetSets(ctx, portfolioName) → (*models.PortfolioAssetSets, error)` — Load from UserDataStore; return empty collection if not found
- `SaveAssetSets(ctx, *models.PortfolioAssetSets) → error` — Increment version, set UpdatedAt, save
- `AddAssetSet(ctx, portfolioName, *models.AssetSet) → (*models.PortfolioAssetSets, error)` — Generate ID (use `common.GenerateID()`), validate category, append, save. Invalidate timeline.
- `UpdateAssetSet(ctx, portfolioName, setID, *models.AssetSet) → (*models.PortfolioAssetSets, error)` — Find by ID, merge non-zero fields, save. Invalidate timeline.
- `RemoveAssetSet(ctx, portfolioName, setID) → (*models.PortfolioAssetSets, error)` — Find by ID, remove, save. Invalidate timeline.
- `AddItem(ctx, portfolioName, setID, *models.AssetItem) → (*models.PortfolioAssetSets, error)` — Find set, generate item ID, append item, save. Invalidate timeline.
- `UpdateItem(ctx, portfolioName, setID, itemID, *models.AssetItem) → (*models.PortfolioAssetSets, error)` — Find set+item, merge, save. Invalidate timeline.
- `RemoveItem(ctx, portfolioName, setID, itemID) → (*models.PortfolioAssetSets, error)` — Find set+item, remove, save. Invalidate timeline.
- `SetPortfolioService(svc interfaces.PortfolioService)` — Setter injection to break cycle

**Storage key:** `subject="asset_sets"`, `key=portfolioName`

**Timeline invalidation:** After any mutation (add/update/remove set or item), call `s.portfolioSvc.InvalidateAndRebuildTimeline(ctx, portfolioName)` to update portfolio value snapshots.

**ID generation:** Use `common.GenerateID()` — look for this function in `internal/common/`. If it doesn't exist, use `fmt.Sprintf("as_%d", time.Now().UnixNano())` for sets and `fmt.Sprintf("ai_%d", time.Now().UnixNano())` for items.

### 3. MODIFY: `internal/interfaces/services.go`

Add new interface after `HoldingNoteService`:

```go
// AssetSetService manages non-equity asset sets (property, crypto, etc.)
type AssetSetService interface {
	// GetAssetSets retrieves all asset sets for a portfolio
	GetAssetSets(ctx context.Context, portfolioName string) (*models.PortfolioAssetSets, error)

	// SaveAssetSets saves asset sets with version increment
	SaveAssetSets(ctx context.Context, sets *models.PortfolioAssetSets) error

	// AddAssetSet adds a new asset set to the portfolio
	AddAssetSet(ctx context.Context, portfolioName string, set *models.AssetSet) (*models.PortfolioAssetSets, error)

	// UpdateAssetSet updates an existing asset set by ID (merge semantics)
	UpdateAssetSet(ctx context.Context, portfolioName string, setID string, update *models.AssetSet) (*models.PortfolioAssetSets, error)

	// RemoveAssetSet removes an asset set by ID
	RemoveAssetSet(ctx context.Context, portfolioName string, setID string) (*models.PortfolioAssetSets, error)

	// AddItem adds an item to an asset set
	AddItem(ctx context.Context, portfolioName string, setID string, item *models.AssetItem) (*models.PortfolioAssetSets, error)

	// UpdateItem updates an item within an asset set (merge semantics)
	UpdateItem(ctx context.Context, portfolioName string, setID string, itemID string, update *models.AssetItem) (*models.PortfolioAssetSets, error)

	// RemoveItem removes an item from an asset set
	RemoveItem(ctx context.Context, portfolioName string, setID string, itemID string) (*models.PortfolioAssetSets, error)

	// SetPortfolioService injects portfolio service for timeline invalidation
	SetPortfolioService(svc PortfolioService)
}
```

### 4. MODIFY: `internal/models/portfolio.go`

Add `AssetSetsValue` field to Portfolio struct after `EquityHoldingsValue`:

```go
// In Portfolio struct, add after EquityHoldingsValue line:
AssetSetsValue float64 `json:"asset_sets_value"` // non-equity asset sets (property, crypto, etc.)
```

Update `PortfolioValue` comment to reflect it now includes asset sets:
```go
PortfolioValue float64 `json:"portfolio_value"` // holdings + available cash + asset sets
```

Add `AssetSetsValue` to `TimelineSnapshot` struct:
```go
// In TimelineSnapshot struct, after HoldingCount:
AssetSetsValue float64 `json:"asset_sets_value,omitempty"` // non-equity asset set values
```

Add `AssetSetsValue` to `GrowthDataPoint`:
```go
AssetSetsValue float64 `json:"asset_sets_value,omitempty"`
```

Add `AssetSetsValue` to `TimeSeriesPoint`:
```go
AssetSetsValue float64 `json:"asset_sets_value,omitempty"`
```

### 5. MODIFY: `internal/services/portfolio/service.go`

**Add assetSetSvc field to Service struct:**
```go
assetSetSvc interfaces.AssetSetService
```

**Add setter:**
```go
func (s *Service) SetAssetSetService(svc interfaces.AssetSetService) {
	s.assetSetSvc = svc
}
```

**Modify portfolio value computation:**
In `GetPortfolio()` and `SyncPortfolio()`, after computing EquityHoldingsValue and CapitalAvailable, load asset sets and add their value:

```go
// After existing portfolio value computation:
if s.assetSetSvc != nil {
	assetSets, err := s.assetSetSvc.GetAssetSets(ctx, name)
	if err == nil && assetSets != nil {
		portfolio.AssetSetsValue = assetSets.TotalValue()
		portfolio.PortfolioValue += portfolio.AssetSetsValue
	}
}
```

Find the exact lines where `PortfolioValue` is set (search for `PortfolioValue =` in service.go) and add the asset sets value there. There will be multiple locations — handle all of them.

**Modify `writeTodaySnapshot()`:** Include `AssetSetsValue` in the `TimelineSnapshot`.

**Modify `GetDailyGrowth()`:** Include `AssetSetsValue` in growth data points. For historical snapshots, read from the persisted `asset_sets_value`. For today, read live from the service.

### 6. MODIFY: `internal/server/handlers.go`

Add new handler methods. Follow the pattern of `handleHoldingNotes` (search for it in handlers.go).

**Add to Server struct (or app dependencies):**
The server already has `s.app` with services. Add `AssetSetService` to the app struct.

**New handler: `handleAssetSets`**

Route: `/api/portfolios/{name}/asset-sets` and `/api/portfolios/{name}/asset-sets/{setID}` and `.../items`

```
GET  /api/portfolios/{name}/asset-sets              → list all asset sets
POST /api/portfolios/{name}/asset-sets               → add a new asset set
PUT  /api/portfolios/{name}/asset-sets/{setID}       → update an asset set
DELETE /api/portfolios/{name}/asset-sets/{setID}     → remove an asset set
POST /api/portfolios/{name}/asset-sets/{setID}/items → add item to set
PUT  /api/portfolios/{name}/asset-sets/{setID}/items/{itemID} → update item
DELETE /api/portfolios/{name}/asset-sets/{setID}/items/{itemID} → remove item
```

**Handler logic (follow handleHoldingNotes pattern):**

For GET: Call `s.app.AssetSetService.GetAssetSets(ctx, name)` → WriteJSON

For POST (add set):
- Decode request body as `models.AssetSet`
- Validate: `Name` required, `Category` must pass `ValidAssetCategory()`
- Call `s.app.AssetSetService.AddAssetSet(ctx, name, &set)`
- WriteJSON 201

For PUT (update set):
- Extract setID from URL path
- Decode body, call `UpdateAssetSet`
- WriteJSON 200

For DELETE (remove set):
- Extract setID from URL path
- Call `RemoveAssetSet`
- WriteJSON 200

For item endpoints: same pattern with `AddItem/UpdateItem/RemoveItem`

**Validation rules:**
- `AssetSet.Name` required (non-empty after TrimSpace)
- `AssetSet.Category` must be valid (not equity — equity is Holdings[])
- `AssetItem.Name` required
- `AssetItem.Value` must be >= 0
- `AssetItem.CostBasis` must be >= 0

### 7. MODIFY: `internal/server/routes.go`

Add routing for asset set endpoints in `routePortfolios()`. Follow the existing sub-resource routing pattern (look at how `/cash-transactions`, `/notes`, `/stock/{ticker}` are routed).

After the existing portfolio sub-routes, add:
```go
case "asset-sets":
	s.handleAssetSets(w, r, name, subParts[1:])
```

Where `subParts[1:]` passes remaining path segments for set ID / item routing.

### 8. MODIFY: App wiring

Find the app struct (likely `internal/server/app.go` or `internal/server/server.go`) and wire the new service:

```go
// In app struct:
AssetSetService interfaces.AssetSetService

// In initialization:
assetSetSvc := assetset.NewService(storage, logger)
app.AssetSetService = assetSetSvc

// After portfolio service is created:
assetSetSvc.SetPortfolioService(portfolioSvc)
portfolioSvc.SetAssetSetService(assetSetSvc) // new setter
```

### 9. MODIFY: `internal/interfaces/services.go`

Add `SetAssetSetService` to `PortfolioService` interface:
```go
// SetAssetSetService injects the asset set service
SetAssetSetService(svc AssetSetService)
```

## Test Cases

### Unit Tests: `internal/services/assetset/service_test.go`

1. **TestGetAssetSets_Empty** — Returns empty collection for portfolio with no sets
2. **TestAddAssetSet** — Adds a set, verifies it appears in collection with generated ID
3. **TestAddAssetSet_InvalidCategory** — Rejects equity category
4. **TestAddAssetSet_EmptyName** — Rejects empty name
5. **TestAddMultipleSetsForSameCategory** — Two property sets in one portfolio
6. **TestUpdateAssetSet** — Updates name/notes, verifies version increment
7. **TestRemoveAssetSet** — Removes set, verifies it's gone
8. **TestAddItem** — Adds item to set, verifies it appears
9. **TestUpdateItem** — Updates item value/cost, verifies
10. **TestRemoveItem** — Removes item from set
11. **TestTotalValue** — Verifies aggregate value across sets
12. **TestTotalCostBasis** — Verifies aggregate cost across sets

### Unit Tests: `internal/models/asset_set_test.go`

1. **TestValidAssetCategory** — Valid and invalid categories
2. **TestAssetSetTotalValue** — Sum of item values
3. **TestAssetSetTotalCostBasis** — Sum of item costs
4. **TestPortfolioAssetSetsTotalValue** — Cross-set aggregate
5. **TestFindSetByID** — Found and not-found cases

## Integration Points

1. **Portfolio value computation** in `portfolio/service.go`: After computing `EquityHoldingsValue + CapitalAvailable`, add `AssetSetsValue`
2. **Timeline snapshots**: `writeTodaySnapshot()` includes `AssetSetsValue`
3. **Route dispatcher**: `routePortfolios()` dispatches `asset-sets` sub-path
4. **App wiring**: Service constructed and injected via setter to break circular dep
5. **PurgeDerivedData**: Asset sets are user data, NOT derived data — do NOT purge them

## Naming Convention Compliance

Per vire-naming skill:
- `asset_sets_value` — portfolio-level aggregate (domain: asset, concept: sets, qualifier: value)
- `asset_set` / `asset_item` — model names
- `asset_category` — classification field
- `cost_basis` — existing convention, reused
- JSON tags all `snake_case`
