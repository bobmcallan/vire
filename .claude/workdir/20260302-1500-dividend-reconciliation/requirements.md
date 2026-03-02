# Requirements: Portfolio Dividend Return Field (fb_6d01379b)

## Problem

The portal displays a "Dividends" figure ($1,906.42) that doesn't reconcile with any authoritative source:
- Cash ledger dividends: $971.47
- Holding-level `dividend_return` sum: $1,263.73
- Navexa: dividends $1,855.19 + currency gains $197.09 = $2,052.28

**Root cause:** The Portfolio struct has NO portfolio-level dividend field. The `totalDividends` variable is computed in `SyncPortfolio` (service.go:400-404) but is only added to `NetEquityReturn` (line 411) and discarded — never exposed as a separate field. The portal is forced to compute its own sum, and the result doesn't match because:
1. The portal may include/exclude different holdings (e.g. closed positions)
2. FX conversion timing differences
3. Possible mixing with other return components

## Scope

### In scope
1. Add `TotalDividendReturn` field to Portfolio struct — server-computed total
2. Populate from existing `totalDividends` computation in SyncPortfolio
3. Update MCP tool description for `get_portfolio`
4. Unit tests

### Out of scope
- Currency gain tracking (separate feature: `docs/features/20260223-fx-currency-gain-calculation.md`)
- Portal changes (separate repo)
- Navexa reconciliation (the server exposes what it computes; portal uses it)

## Files to Change

### 1. `internal/models/portfolio.go` — Add field to Portfolio struct

Add after `UnrealizedEquityReturn` (line 56):

```go
TotalDividendReturn    float64             `json:"total_dividend_return"`
```

This follows the naming convention: `Total` prefix for aggregated values, `DividendReturn` matching the holding-level field name.

### 2. `internal/services/portfolio/service.go` — Populate field in SyncPortfolio

At line ~443 (portfolio construction), add the field:

```go
portfolio := &models.Portfolio{
    // ... existing fields ...
    RealizedEquityReturn:   totalRealizedNetReturn,
    UnrealizedEquityReturn: totalUnrealizedNetReturn,
    TotalDividendReturn:    totalDividends,           // ← ADD THIS
    CalculationMethod:      "average_cost",
    // ...
}
```

The `totalDividends` variable is already computed at lines 400-404:
```go
var totalValue, totalCost, totalGain, totalDividends float64
for _, h := range holdings {
    totalDividends += h.DividendReturn  // line 404
}
```

### 3. `internal/server/catalog.go` — Update MCP tool description

Update the `get_portfolio` description (line 254) to mention the new field. Add after the "Key value fields" section:

> Includes total_dividend_return (sum of holding-level dividend_return, already FX-converted to AUD).

### 4. `internal/services/portfolio/service_test.go` — Unit test

Add a test that verifies `TotalDividendReturn` is correctly computed from holding dividends.

Test case: Create mock holdings with known `DividendReturn` values, verify the portfolio's `TotalDividendReturn` equals their sum.

```go
func TestSyncPortfolio_TotalDividendReturn(t *testing.T) {
    // Setup: mock Navexa client returning holdings with dividends
    // Holding A: DividendReturn = 100.50
    // Holding B: DividendReturn = 250.25
    // Holding C: DividendReturn = 0.00 (no dividends)
    // Expected TotalDividendReturn = 350.75
    // Also verify TotalDividendReturn is included in NetEquityReturn
}
```

Follow the existing test patterns in `service_test.go` — use `approxEqual()` helper, direct function calls where possible.

**Note:** SyncPortfolio requires a full Navexa mock. If this is too complex, write the test against the aggregation logic directly. The key assertion is:
- `portfolio.TotalDividendReturn == sum(h.DividendReturn for h in holdings)`
- `portfolio.NetEquityReturn includes totalDividends`

## Integration Points

- **Line 400-411 in service.go**: `totalDividends` is already computed — just needs to be stored
- **Line 443-461 in service.go**: Portfolio construction — add the field
- **handlePortfolioGet in handlers.go**: No changes needed — it serializes the full Portfolio struct
- **getPortfolioRecord / savePortfolioRecord**: No changes needed — Portfolio struct is stored/retrieved as-is via JSON marshaling

## Test Cases

| Test Name | What it Verifies |
|-----------|-----------------|
| `TestSyncPortfolio_TotalDividendReturn` | Sum of holding dividends populates portfolio field |
| `TestSyncPortfolio_TotalDividendReturn_NoDiv` | Zero dividends → field is 0 |
| `TestSyncPortfolio_TotalDividendReturn_InNetReturn` | TotalDividendReturn is included in NetEquityReturn |
