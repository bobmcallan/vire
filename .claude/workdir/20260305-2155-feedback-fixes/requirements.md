# Requirements: Feedback Batch Fix (7 Items)

## Scope

Fix 7 feedback items: 3 calculation errors (high), 1 trade list bug (high), 2 missing data issues (medium/high), 1 glossary correction (low).

### In Scope
- Fix `simple_capital_return_pct` to use `PortfolioValue` instead of `EquityValue`
- Fix `net_capital_return` returning $0.00
- Fix glossary Growth Metrics: duplicate terms, incorrect formulas, wrong values
- Fix D/W/M glossary calculations mixing equity_value with portfolio_value
- Fix `trade_list` returning empty for Navexa-sourced portfolios
- Add advisory when market data price is unavailable for a ticker
- Update glossary definitions per feedback

### Out of Scope
- EODHD data provider gaps (we can only add advisory, not fix external API)
- Changing how Navexa trade data is stored
- Refactoring CapitalPerformance into a service method

---

## Fix 1: simple_capital_return_pct Uses EquityValue (fb_51aee182)

**Problem**: `CalculatePerformance()` uses `portfolio.EquityValue` (equity only, ~$265k) but `net_capital_deployed` includes cash contributions (~$477k). The formula `(265k - 477k) / 477k = -44%` is wildly wrong. Should use `portfolio.PortfolioValue` (~$471k) giving `(471k - 477k) / 477k = -1.27%`.

**Files to change**:

### `internal/services/cashflow/service.go` (line 504)
```go
// BEFORE (line 504):
currentValue := portfolio.EquityValue

// AFTER:
currentValue := portfolio.PortfolioValue
```

This single line fixes the `SimpleCapitalReturnPct` calculation. The variable `currentValue` feeds into:
- Line 513: `simpleReturnPct = (currentValue - netCapital) / netCapital * 100`
- Line 523: `EquityValue: currentValue` in the CapitalPerformance struct

### `internal/models/cashflow.go` — Rename field
Find the `CapitalPerformance` struct. Rename the `EquityValue` field to `CurrentValue` since it now represents portfolio value (equity + cash), not just equity:
```go
// BEFORE:
EquityValue float64 `json:"equity_value"`

// AFTER:
CurrentValue float64 `json:"current_value"`
```

Then update all references to `EquityValue` on `CapitalPerformance` throughout the codebase:
- `cashflow/service.go:523` — `EquityValue: currentValue` → `CurrentValue: currentValue`
- `cashflow/service.go` in `deriveFromTrades` — same field assignment (search for `EquityValue:` in that function)
- `server/glossary.go:307` — `fmtMoney(cp.EquityValue)` → `fmtMoney(cp.CurrentValue)`
- All test files referencing `CapitalPerformance.EquityValue`

**Test cases** (add to existing `internal/services/cashflow/service_test.go`):
1. `TestCalculatePerformance_UsesPortfolioValue` — verify SimpleCapitalReturnPct uses portfolio.PortfolioValue, not EquityValue
2. `TestCalculatePerformance_SimpleReturnWithCash` — portfolio with equity $265k, cash $206k, deployed $477k → return should be ~-1.27%, not -44%

---

## Fix 2: net_capital_return Returns $0.00 (fb_cedb56ee)

**Problem**: In `handlers.go:136-143`, `portfolio.NetCapitalReturn` is computed inside a guard:
```go
if perf, err := ...; err == nil && perf != nil && perf.TransactionCount > 0 {
    portfolio.CapitalPerformance = perf
    if perf.NetCapitalDeployed > 0 {
        portfolio.NetCapitalReturn = portfolio.PortfolioValue - perf.NetCapitalDeployed
        portfolio.NetCapitalReturnPct = (portfolio.NetCapitalReturn / perf.NetCapitalDeployed) * 100
    }
}
```

The guard `perf.TransactionCount > 0` may fail if `CalculatePerformance` falls through to `deriveFromTrades` (which may set TransactionCount differently), or if the ledger is empty for this portfolio.

**Investigation needed**: Check what `deriveFromTrades` sets for `TransactionCount`. If it counts buy/sell trades from `portfolio.Holdings[].Trades`, it should be > 0 for a Navexa portfolio with trades.

**Files to change**:

### `internal/server/handlers.go` (lines 136-143)
The guard `perf.NetCapitalDeployed > 0` is too restrictive — it excludes negative deployed capital (more withdrawn than deposited). Change to:
```go
// BEFORE (line 140):
if perf.NetCapitalDeployed > 0 {

// AFTER:
if perf.NetCapitalDeployed != 0 {
```

This ensures net_capital_return is computed whenever there IS deployed capital, even if net is negative.

Also check `deriveFromTrades` in `cashflow/service.go` — if the Navexa portfolio has holdings with NavexaTrade arrays, verify that `deriveFromTrades` correctly iterates them. The function at line 534 calls `s.portfolioService.GetPortfolio()` and then looks at `portfolio.Holdings`. For Navexa portfolios, each Holding has `Trades []*NavexaTrade`. The `deriveFromTrades` function needs to handle NavexaTrade (not just Trade) format.

### `internal/services/cashflow/service.go` — Check `deriveFromTrades`
Read the full `deriveFromTrades` function. If it iterates `holding.Trades` (which are `*NavexaTrade`), verify it correctly sums buy/sell values. If it looks at a different trade structure, this is why it returns nil for Navexa portfolios.

**Likely fix**: In `deriveFromTrades`, ensure it handles Navexa holdings that have `Trades []*NavexaTrade` populated. The NavexaTrade struct has `Type` (buy/sell), `Units`, `Price`, `Fees`, `Value` fields (models/navexa.go:53-65).

**Test cases**:
1. `TestNetCapitalReturn_NonZeroWhenDeployed` — verify handler sets NetCapitalReturn when NetCapitalDeployed > 0
2. `TestNetCapitalReturn_NegativeDeployed` — verify handler sets NetCapitalReturn when NetCapitalDeployed < 0

---

## Fix 3: D/W/M Glossary Mixing Fields (fb_1bf1e2f0)

**Problem**: In `glossary.go:366-367`:
```go
yesterdayChange := p.EquityValue - p.PortfolioYesterdayValue
lastWeekChange := p.EquityValue - p.PortfolioLastWeekValue
```
`EquityValue` is equity-only (~$265k), `PortfolioYesterdayValue` is portfolio_value (~$478k). The subtraction produces nonsensical -$213k.

**File to change**: `internal/server/glossary.go`

### Lines 366-367 — Fix calculation
```go
// BEFORE:
yesterdayChange := p.EquityValue - p.PortfolioYesterdayValue
lastWeekChange := p.EquityValue - p.PortfolioLastWeekValue

// AFTER:
yesterdayChange := p.PortfolioValue - p.PortfolioYesterdayValue
lastWeekChange := p.PortfolioValue - p.PortfolioLastWeekValue
```

### Lines 383-384, 392-393 — Fix examples to match
```go
// Line 384 BEFORE:
Example: fmt.Sprintf("%s - %s = %s (%.2f%%)", fmtMoney(p.EquityValue), ...

// AFTER:
Example: fmt.Sprintf("%s - %s = %s (%.2f%%)", fmtMoney(p.PortfolioValue), ...
```

Same pattern for last_week_change example.

---

## Fix 4: Glossary Corrections (fb_dda6e7e0)

**Problem**: Multiple glossary issues:
1. Duplicate `gross_cash_balance` — in Valuation (line 137) AND Growth (line 395)
2. Duplicate `net_capital_deployed` — in Capital (line 294) AND Growth (line 402)
3. Growth `gross_cash_balance` hardcoded to `0.0` (line 369) instead of `p.GrossCashBalance`
4. `net_equity_return` definition says "Unrealised gain or loss" — incorrect, it blends realised+unrealised
5. `simple_capital_return_pct` glossary formula uses `equity_value` — must match code fix

**File to change**: `internal/server/glossary.go`

### Remove duplicates from Growth Metrics (lines 394-408)
Remove the `gross_cash_balance` and `net_capital_deployed` entries from `buildGrowthCategory()`. These are already defined in Valuation and Capital categories respectively. The Growth category should ONLY contain `yesterday_change` and `last_week_change`.

```go
// BEFORE (buildGrowthCategory return):
Terms: []models.GlossaryTerm{
    {yesterday_change...},
    {last_week_change...},
    {gross_cash_balance...},     // REMOVE
    {net_capital_deployed...},   // REMOVE
},

// AFTER:
Terms: []models.GlossaryTerm{
    {yesterday_change...},
    {last_week_change...},
},
```

Also remove the unused `grossCashBalance` and `netDeployed` local variables (lines 369-373).

### Fix net_equity_return definition (line 107)
```go
// BEFORE:
Definition: "Unrealised gain or loss across the portfolio.",

// AFTER:
Definition: "Net return on all capital deployed into equities, including realised gains/losses from closed positions.",
```

### Fix simple_capital_return_pct formula (line 305)
```go
// BEFORE:
Formula: "(equity_value - net_capital_deployed) / net_capital_deployed * 100",

// AFTER:
Formula: "(portfolio_value - net_capital_deployed) / net_capital_deployed * 100",
```

### Fix simple_capital_return_pct example (line 307)
Update `fmtMoney(cp.EquityValue)` → `fmtMoney(cp.CurrentValue)` (after Fix 1 renames the field).

---

## Fix 5: trade_list Empty for Navexa Portfolios (fb_9e063cf1)

**Problem**: `TradeService.ListTrades()` always queries `UserDataStore`, which stores trades for manual/snapshot portfolios only. Navexa portfolios have trades in the Navexa API — each Holding has `Trades []*NavexaTrade`.

**Architecture choice**: Fix in the handler layer (not TradeService) to avoid adding PortfolioService dependency to TradeService. The handler checks portfolio source type and extracts trades from holdings when Navexa-sourced.

**File to change**: `internal/server/handlers_trade.go`

### In `handleTrades` GET branch (after line 76)
Add logic before the TradeService call to check portfolio source type:

```go
// Check if portfolio is Navexa-sourced — trades come from holdings, not UserDataStore
portfolio, pErr := s.app.PortfolioService.GetPortfolio(ctx, portfolioName)
if pErr == nil && (portfolio.SourceType == models.SourceNavexa || portfolio.SourceType == "") {
    // Extract trades from Navexa holdings
    trades := extractNavexaTrades(portfolio, filter)
    total := len(trades)
    // Apply pagination
    limit := filter.Limit
    if limit <= 0 { limit = 50 }
    if limit > 200 { limit = 200 }
    offset := filter.Offset
    if offset < 0 { offset = 0 }
    if offset >= total {
        WriteJSON(w, http.StatusOK, map[string]interface{}{"trades": []models.Trade{}, "total": total})
        return
    }
    end := offset + limit
    if end > total { end = total }
    WriteJSON(w, http.StatusOK, map[string]interface{}{"trades": trades[offset:end], "total": total})
    return
}

// Fallback: manual/snapshot portfolios use TradeService
trades, total, err := s.app.TradeService.ListTrades(ctx, portfolioName, filter)
```

### Add `extractNavexaTrades` helper function (same file)
```go
// extractNavexaTrades converts Navexa holdings' trade arrays to Trade format with filtering.
func extractNavexaTrades(p *models.Portfolio, filter interfaces.TradeFilter) []models.Trade {
    var trades []models.Trade
    for _, h := range p.Holdings {
        for _, nt := range h.Trades {
            t := models.Trade{
                Ticker:     h.Ticker,
                Action:     models.TradeAction(nt.Type), // "buy"/"sell"
                Units:      nt.Units,
                Price:      nt.Price,
                Fees:       nt.Fees,
                SourceType: models.SourceNavexa,
            }
            // Parse date
            if d, err := time.Parse("2006-01-02", nt.Date); err == nil {
                t.Date = d
            }
            // Apply filters
            if filter.Ticker != "" && t.Ticker != filter.Ticker { continue }
            if filter.Action != "" && t.Action != filter.Action { continue }
            if !filter.DateFrom.IsZero() && t.Date.Before(filter.DateFrom) { continue }
            if !filter.DateTo.IsZero() && t.Date.After(filter.DateTo) { continue }
            trades = append(trades, t)
        }
    }
    // Sort by date descending (most recent first)
    sort.Slice(trades, func(i, j int) bool { return trades[i].Date.After(trades[j].Date) })
    return trades
}
```

**Note**: `h.Trades` is `[]*NavexaTrade` on the Holding struct (models/portfolio.go:140). The handler strips trades for portfolio GET (handlers.go:132-134), but here we're in the trades handler, not portfolio handler.

**Test cases**:
1. `TestTradeList_NavexaPortfolio_ReturnsTrades` — Navexa portfolio with holdings containing trades → trades returned
2. `TestTradeList_NavexaPortfolio_FilterByAction` — filter by "sell" returns only sell trades
3. `TestTradeList_NavexaPortfolio_FilterByDate` — date range filter works
4. `TestTradeList_ManualPortfolio_UsesTradeService` — manual portfolio still uses TradeService

---

## Fix 6: Missing Price/Signals Advisory (fb_6cbc1eea, fb_e343c626)

**Problem**: When EODHD has no EOD data for a ticker (e.g. DFND.AU, SXE.AU, SRG.AU), the response silently omits `price` and `signals`. The consumer doesn't know if it's a bug or a data gap.

**File to change**: `internal/services/market/service.go`

### In `GetStockData` (after line 560, after the price block)
Add an advisory when price data is unavailable:

```go
// After the price block (line 560):
if stockData.Price == nil && include.Price {
    stockData.Advisory = append(stockData.Advisory, "Price data unavailable from EODHD for this ticker. This may be a data provider gap for ETFs or recently listed securities.")
}
```

### `internal/models/market.go` — Add Advisory field to StockData
Find the `StockData` struct and add:
```go
Advisory []string `json:"advisory,omitempty"`
```

**Test cases**:
1. `TestGetStockData_NoPriceAdvisory` — when EOD is empty and include.Price=true, Advisory contains message
2. `TestGetStockData_HasPriceNoAdvisory` — when EOD has data, Advisory is nil

---

## Fix Summary

| # | Feedback | File(s) | Change |
|---|----------|---------|--------|
| 1 | fb_51aee182 | cashflow/service.go:504, models/cashflow.go | EquityValue → PortfolioValue, rename struct field |
| 2 | fb_cedb56ee | handlers.go:140, cashflow/service.go (deriveFromTrades) | Change guard `> 0` to `!= 0`, fix deriveFromTrades for Navexa |
| 3 | fb_1bf1e2f0 | glossary.go:366-367,384,393 | EquityValue → PortfolioValue in growth calcs |
| 4 | fb_dda6e7e0 | glossary.go (multiple) | Remove duplicates, fix definitions and formulas |
| 5 | fb_9e063cf1 | handlers_trade.go | Add Navexa trade extraction in GET handler |
| 6 | fb_6cbc1eea, fb_e343c626 | market/service.go, models/market.go | Add Advisory field when price unavailable |

## Test Plan

### Unit tests (implementer)
- `cashflow/service_test.go` — SimpleCapitalReturnPct uses PortfolioValue
- `handlers.go` — NetCapitalReturn computed when NetCapitalDeployed != 0

### Integration tests (test-creator)
- `tests/data/capital_return_fixes_test.go` — end-to-end capital return calculations
- `tests/api/trade_list_navexa_test.go` — Navexa trade extraction
