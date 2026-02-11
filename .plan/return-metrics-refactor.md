# Return Metrics Refactor: Investigation & Approach

## Current State

### Data Flow
1. **Navexa Performance API** (`GetEnrichedHoldings`) provides IRR p.a. values:
   - `CapitalGainPctPA` (annualised capital gain %)
   - `TotalReturnPctPA` (annualised total return %)
2. **SyncPortfolio** (`service.go`) calculates simple % from trade history:
   - `GainLossPct` = gain / totalInvested * 100
   - `CapitalGainPct` = same as GainLossPct
   - `TotalReturnPct` = (gainLoss + dividends) / totalInvested * 100
3. **Formatters** display both side by side:
   - Column headers: "Capital Gain % (simple)" and "Capital Gain % (IRR p.a.)"
   - Same for Total Return %

### Fields in Holding struct (portfolio.go)
- `GainLossPct` - simple %
- `GainLossPctPA` - IRR p.a.
- `CapitalGainPct` - simple %
- `CapitalGainPctPA` - IRR p.a.
- `TotalReturnPct` - simple %
- `TotalReturnPctPA` - IRR p.a.

### Fields in NavexaHolding struct (navexa.go)
Same six fields mirrored.

## Proposed Changes

### Phase 1: Model Changes (internal/models/)

**portfolio.go** - Holding struct:
- Remove: `GainLossPct`, `CapitalGainPct`, `TotalReturnPct` (simple % fields)
- Rename: `GainLossPctPA` -> `GainLossPct` (IRR becomes the default)
- Rename: `CapitalGainPctPA` -> `CapitalGainPct` (IRR becomes the default)
- Rename: `TotalReturnPctPA` -> `TotalReturnPct` (IRR becomes the default)
- Add: `TotalReturnTWRR float64` (TWRR as secondary metric)

**navexa.go** - NavexaHolding struct:
- Same removals and renames to match.

### Phase 2: TWRR Calculator (new file: internal/services/portfolio/twrr.go)

**Algorithm**: Time-Weighted Rate of Return using Modified Dietz sub-periods.

```
TWRR = (Product of sub-period returns) - 1

For each sub-period between cash flows (buy/sell dates):
  Sub-period return = (End Value - Begin Value - Net Cash Flow) / (Begin Value + weighted cash flows)

Where weighted cash flow uses day-weight: (days remaining in period / total days in period)
```

**Implementation**:
```go
// CalculateTWRR computes TWRR for a holding using trade dates and EOD price history.
// Returns annualised TWRR as a percentage.
func CalculateTWRR(trades []*models.NavexaTrade, eodBars []models.EODBar, currentPrice float64) float64
```

**Data requirements** (all already available):
- Trade history: stored on Holding.Trades (from Navexa trades API)
- EOD bars: stored in MarketData.EOD (from EODHD, sorted descending)
- Current price: Holding.CurrentPrice

**Sub-period boundaries**:
1. Each buy/sell trade date creates a sub-period boundary
2. First sub-period starts at first trade date
3. Last sub-period ends at today (using current price)
4. For each boundary, find the closing price from EOD data using `findClosingPriceAsOf()`

**Edge cases**:
- Single trade (no sub-periods yet): use simple return
- No EOD data for a date: skip that sub-period
- All sells (closed position): use final sell date as end, no current price needed
- Negative sub-period denominator: skip (invalid period)

### Phase 3: Service Changes (internal/services/portfolio/service.go)

**SyncPortfolio**:
1. Remove all simple % calculations:
   - Remove `h.GainLossPct = (gainLoss / totalInvested) * 100`
   - Remove `h.CapitalGainPct = h.GainLossPct`
   - Remove `h.TotalReturnPct = (h.TotalReturnValue / totalInvested) * 100`
   - Remove same from EODHD price refresh block
2. IRR values from Navexa are already set in GetEnrichedHoldings - just map to renamed fields
3. After all holdings are built, compute TWRR for each:
   - Look up MarketData for the holding
   - Call `CalculateTWRR(holding.Trades, marketData.EOD, holding.CurrentPrice)`
   - Set `holding.TotalReturnTWRR`

**Holding conversion** (NavexaHolding -> Holding mapping):
- `GainLossPct: h.GainLossPctPA` (was: h.GainLossPct which was simple %)
- `CapitalGainPct: h.CapitalGainPctPA` (was: h.CapitalGainPct which was simple %)
- `TotalReturnPct: h.TotalReturnPctPA` (was: h.TotalReturnPct which was simple %)
- Add: `TotalReturnTWRR: <computed>`

### Phase 4: Navexa Client Changes (internal/clients/navexa/client.go)

**GetEnrichedHoldings**:
- Remove simple % fields from NavexaHolding mapping
- Map IRR values to the renamed fields directly

### Phase 5: Formatter Changes

**cmd/vire-mcp/formatters.go**:
- Remove "(simple)" columns from all tables
- Remove "(IRR p.a.)" suffix - IRR becomes just "Capital Gain %" and "Total Return %"
- Add "TWRR %" column after Total Return %
- Update all table headers and row formatting
- Update `formatPortfolioHoldings` similarly (remove simple, add TWRR)

**internal/services/report/formatter.go**:
- Same changes: remove simple columns, rename IRR columns, add TWRR column
- Update `formatReportSummary`, `formatStockReport`, `formatETFReport`
- Update `formatTradeHistory` totals (uses CapitalGainPct, TotalReturnPct)

### Phase 6: Snapshot/Growth (no changes needed)
- `SnapshotHolding.GainLossPct` is an independent simple calculation (market value - cost / cost) used for historical snapshots where IRR/TWRR doesn't apply. **Leave as-is**.
- `GrowthDataPoint.GainLossPct` is same type of point-in-time calculation. **Leave as-is**.

## Files Changed (complete list)

1. `internal/models/portfolio.go` - Holding struct field changes
2. `internal/models/navexa.go` - NavexaHolding struct field changes
3. `internal/services/portfolio/twrr.go` - **NEW** TWRR calculator
4. `internal/services/portfolio/twrr_test.go` - **NEW** TWRR tests
5. `internal/services/portfolio/service.go` - Remove simple %, add TWRR computation
6. `internal/clients/navexa/client.go` - Field mapping updates
7. `cmd/vire-mcp/formatters.go` - Table header/row updates
8. `internal/services/report/formatter.go` - Table header/row updates
9. `internal/services/portfolio/service_test.go` - Update test expectations
10. `internal/storage/file_test.go` - If any field references need updating
11. `internal/services/strategy/rules.go` - If any field references need updating
12. `internal/services/strategy/rules_test.go` - If any field references need updating
13. `internal/services/portfolio/growth_test.go` - If any field references need updating

## TWRR Implementation Detail

```go
func CalculateTWRR(trades []*models.NavexaTrade, eodBars []models.EODBar, currentPrice float64, now time.Time) float64 {
    // 1. Sort trades by date ascending
    // 2. Build cash flow events: date + net cash flow (buy = negative outflow, sell = positive inflow)
    // 3. For each pair of consecutive cash flow dates, compute sub-period return:
    //    - beginValue = units held at start * closing price on start date
    //    - endValue = units held at end * closing price on end date (before the trade on that date)
    //    - cashFlow = net cash deployed/received on the end date
    //    - subReturn = (endValue - cashFlow) / beginValue
    // 4. Last sub-period ends at now using currentPrice
    // 5. TWRR = product(subReturns) - 1
    // 6. Annualise: (1 + TWRR)^(365/days) - 1
    // 7. Return as percentage (multiply by 100)
}
```

Key: Sub-period returns are computed as the portfolio return *excluding* the effect of cash flows, which is exactly what TWRR measures - the investment performance independent of the timing/amount of contributions.

## Risk Assessment

- **Breaking change to stored data**: The JSON field names on Holding change. Need to `rebuild_data` after deploy.
- **Snapshot/Growth unchanged**: These use independent `GainLossPct` on different structs.
- **Strategy rules**: Need to verify `rules.go` doesn't reference removed fields.
- **TWRR accuracy**: Sub-period returns depend on EOD data availability. Missing bars will reduce accuracy but won't crash.
