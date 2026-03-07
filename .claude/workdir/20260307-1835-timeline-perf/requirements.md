# Requirements: Timeline Data Fix + Dashboard Performance

## Context

Two issues reported:
1. **Trading portfolio "flat"**: WES.AU has a bad EOD bar (Mar 6: close=41.71 instead of ~75.82). EODHD returned wrong data. This poisons `yesterday_close_price`, `GetStockData` price percentages, and candle data.
2. **Dashboard load optimization**: N+1 signal queries and duplicate market data lookups.

## Scope

**In scope:**
- A: EOD bar-to-bar divergence guard (prevent future bad data)
- B: Fix GetStockData pct calculations after live quote override
- C: Batch signal queries in populateFromMarketData
- D: Batch market data lookups in SyncPortfolio TWRR/country loop

**Out of scope:**
- Portal chart rendering
- Timeline snapshot date resolution (weekend vs trading day) — separate ticket
- Manual data cleanup for WES.AU — will be handled by force re-collect after fix

---

## Fix A: EOD Bar-to-Bar Divergence Guard

**File:** `internal/services/market/service.go`

**Function:** `mergeEODBars()` (line 468)

**What:** After merging and sorting bars (descending), scan for any bar whose close diverges >40% from its neighbor. Remove divergent bars and log a warning.

**Code template:**
```go
// mergeEODBars — after existing merge + sort logic, add:

// Remove bars with >40% close-to-close divergence (bad EODHD data guard)
merged = filterDivergentBars(merged)
```

**New function** (add after `mergeEODBars`):
```go
// filterDivergentBars removes bars whose close diverges >40% from adjacent bars.
// This guards against bad data from EODHD (e.g., wrong ticker mapping for a day).
// Bars are expected to be sorted descending (most recent first).
func filterDivergentBars(bars []models.EODBar) []models.EODBar {
    if len(bars) < 3 {
        return bars // not enough context to judge
    }
    // Check each bar (except first and last) against both neighbors.
    // For first bar, check against second bar only.
    filtered := make([]models.EODBar, 0, len(bars))
    for i, bar := range bars {
        if bar.Close <= 0 {
            continue // skip zero-price bars
        }
        if i == 0 {
            // First bar: check against next bar
            if bars[1].Close > 0 {
                ratio := bar.Close / bars[1].Close
                if ratio < 0.6 || ratio > 1.667 {
                    // >40% divergence from next bar — likely bad data
                    continue // skip this bar
                }
            }
        } else {
            // Check against previous (already-accepted) bar
            prev := filtered[len(filtered)-1]
            if prev.Close > 0 {
                ratio := bar.Close / prev.Close
                if ratio < 0.6 || ratio > 1.667 {
                    continue // skip this bar
                }
            }
        }
        filtered = append(filtered, bar)
    }
    return filtered
}
```

**IMPORTANT:** The divergence check at the first bar position (i==0, the MOST RECENT bar) is the most critical case. This is the bar used for current price, yesterday_close_price, etc. If EOD[0] is bad, everything downstream breaks.

**However:** The ratio check needs to be careful — legitimate stock moves of >40% do exist (penny stocks, earnings, etc). Use a logger to warn when bars are filtered so we can monitor false positives:
```go
// Inside the continue paths, add logging. Accept a logger parameter or use package-level.
// Actually, mergeEODBars is a pure function. Instead, return (filtered, removed count)
// and let the caller log.
```

**Revised approach:** Keep `mergeEODBars` pure. Add the divergence filter as a separate step called by each code path that stores EOD bars. The callers (CollectMarketData, CollectEOD, collectCoreTicker) already have access to the logger.

**Pattern:** Add a method `(s *Service) filterBadEODBars(bars []models.EODBar, ticker string) []models.EODBar` on the Service struct so it can log:
```go
func (s *Service) filterBadEODBars(bars []models.EODBar, ticker string) []models.EODBar {
    if len(bars) < 3 {
        return bars
    }
    filtered := make([]models.EODBar, 0, len(bars))
    for i, bar := range bars {
        if bar.Close <= 0 {
            continue
        }
        if i == 0 {
            if len(bars) > 1 && bars[1].Close > 0 {
                ratio := bar.Close / bars[1].Close
                if ratio < 0.6 || ratio > 1.667 {
                    s.logger.Warn().Str("ticker", ticker).
                        Str("date", bar.Date.Format("2006-01-02")).
                        Float64("close", bar.Close).
                        Float64("neighbor_close", bars[1].Close).
                        Msg("Filtered divergent EOD bar (>40% from neighbor)")
                    continue
                }
            }
        } else if len(filtered) > 0 {
            prev := filtered[len(filtered)-1]
            if prev.Close > 0 {
                ratio := bar.Close / prev.Close
                if ratio < 0.6 || ratio > 1.667 {
                    s.logger.Warn().Str("ticker", ticker).
                        Str("date", bar.Date.Format("2006-01-02")).
                        Float64("close", bar.Close).
                        Float64("neighbor_close", prev.Close).
                        Msg("Filtered divergent EOD bar (>40% from neighbor)")
                    continue
                }
            }
        }
        filtered = append(filtered, bar)
    }
    return filtered
}
```

**Call sites:** After every `mergeEODBars` call AND after every direct `marketData.EOD = eodResp.Data` assignment. Search for these patterns in:
- `service.go` CollectMarketData (~line 107, 118, 132)
- `service.go` collectCoreTicker (~line 352, 366, 377, 391)
- `collect.go` CollectEOD (similar pattern)

For each location where `marketData.EOD` is set with new data, add:
```go
marketData.EOD = s.filterBadEODBars(marketData.EOD, ticker)
```

**Test cases:**
- `TestFilterBadEODBars_RemovesDivergentBar`: [100, 50, 98, 97] → removes 50
- `TestFilterBadEODBars_RemovesDivergentFirstBar`: [50, 98, 97, 96] → removes 50
- `TestFilterBadEODBars_KeepsLegitimateMove`: [100, 80, 78] → keeps all (20% is OK)
- `TestFilterBadEODBars_TooFewBars`: [100, 50] → returns as-is
- `TestFilterBadEODBars_EmptyBars`: [] → returns empty

---

## Fix B: Recalculate Pct After Live Quote Override

**File:** `internal/services/market/service.go`

**Function:** `GetStockData()` (lines 563-580)

**What:** After the live quote overrides `Price.Current`, recalculate `YesterdayPct` and `LastWeekPct` using the new price.

**Current code** (lines 563-580):
```go
if s.eodhd != nil {
    if quote, err := s.eodhd.GetRealTimeQuote(ctx, ticker); err == nil && quote.Close > 0 {
        stockData.Price.Current = quote.Close
        // ... other overrides ...
        if prevClose > 0 {
            stockData.Price.Change = quote.Close - prevClose
            stockData.Price.ChangePct = ((quote.Close - prevClose) / prevClose) * 100
        }
        // MISSING: recalculate YesterdayPct and LastWeekPct
    }
}
```

**Add after the existing recalculation block (after line 575), still inside the `quote.Close > 0` block:**
```go
// Recalculate historical change percentages with live price
if stockData.Price.YesterdayClose > 0 {
    stockData.Price.YesterdayPct = ((quote.Close - stockData.Price.YesterdayClose) / stockData.Price.YesterdayClose) * 100
}
if stockData.Price.LastWeekClose > 0 {
    stockData.Price.LastWeekPct = ((quote.Close - stockData.Price.LastWeekClose) / stockData.Price.LastWeekClose) * 100
}
```

**Test cases:**
- `TestGetStockData_LiveQuoteRecalculatesPcts`: Mock GetRealTimeQuote to return different price than EOD[0]. Verify YesterdayPct and LastWeekPct use live price, not EOD[0].

---

## Fix C: Batch Signal Queries

**File:** `internal/services/portfolio/service.go`

**Function:** `populateFromMarketData()` (lines 1060-1065)

**Current code** (N+1 pattern):
```go
for i := range portfolio.Holdings {
    // ... per-holding market data processing ...
    if ss := s.storage.SignalStorage(); ss != nil {
        if sigs, err := ss.GetSignals(ctx, ticker); err == nil && sigs.TrendMomentum.Level != "" {
            h.TrendLabel = trendMomentumLabel(sigs.TrendMomentum.Level)
            h.TrendScore = sigs.TrendMomentum.Score
        }
    }
}
```

**Replace with batch approach.** Before the holdings loop, fetch all signals at once:
```go
// Batch-fetch signals for all open holdings
var signalsByTicker map[string]*models.TickerSignals
if ss := s.storage.SignalStorage(); ss != nil {
    if allSignals, err := ss.GetSignalsBatch(ctx, tickers); err == nil {
        signalsByTicker = make(map[string]*models.TickerSignals, len(allSignals))
        for _, sig := range allSignals {
            signalsByTicker[sig.Ticker] = sig
        }
    }
}
```

Then inside the loop, replace the GetSignals call with map lookup:
```go
if sigs := signalsByTicker[ticker]; sigs != nil && sigs.TrendMomentum.Level != "" {
    h.TrendLabel = trendMomentumLabel(sigs.TrendMomentum.Level)
    h.TrendScore = sigs.TrendMomentum.Score
}
```

**Test:** Existing tests should pass. Add test to verify signals are populated from batch.

---

## Fix D: Batch Market Data in SyncPortfolio

**File:** `internal/services/portfolio/service.go`

**Function:** SyncPortfolio TWRR/country loop (lines 479-501)

**Current code** (per-holding GetMarketData):
```go
for i := range holdings {
    ticker := holdings[i].EODHDTicker()
    md, err := s.storage.MarketDataStorage().GetMarketData(ctx, ticker)
    if err == nil && md != nil {
        if md.Fundamentals != nil && md.Fundamentals.CountryISO != "" {
            holdings[i].Country = md.Fundamentals.CountryISO
        }
    }
    // ... TWRR calculation using md.EOD ...
}
```

**Replace with batch approach.** Before the loop:
```go
// Batch-fetch market data for TWRR and country population
openTickers := make([]string, 0, len(holdings))
for _, h := range holdings {
    if h.Units > 0 || len(h.Trades) > 0 {
        openTickers = append(openTickers, h.EODHDTicker())
    }
}
allMD, _ := s.storage.MarketDataStorage().GetMarketDataBatch(ctx, openTickers)
mdByTicker := make(map[string]*models.MarketData, len(allMD))
for _, md := range allMD {
    mdByTicker[md.Ticker] = md
}
```

Then in the loop:
```go
for i := range holdings {
    ticker := holdings[i].EODHDTicker()
    md := mdByTicker[ticker]
    if md != nil {
        if md.Fundamentals != nil && md.Fundamentals.CountryISO != "" {
            holdings[i].Country = md.Fundamentals.CountryISO
        }
    }
    // ... TWRR uses md (may be nil) ...
}
```

**Test:** Existing tests should pass since behavior is unchanged.

---

## Files Changed Summary

| File | Changes |
|------|---------|
| `internal/services/market/service.go` | Add `filterBadEODBars()`, call after EOD merges/assignments, fix GetStockData pct recalc |
| `internal/services/market/service_test.go` | Tests for filterBadEODBars, live quote pct recalc |
| `internal/services/market/collect.go` | Call `filterBadEODBars()` after EOD assignments |
| `internal/services/portfolio/service.go` | Batch signals, batch market data in sync loop |

---

## Verification

1. `go build ./cmd/vire-server/`
2. `go vet ./...`
3. `go test ./internal/services/market/... -timeout 120s`
4. `go test ./internal/services/portfolio/... -timeout 120s`
5. `go test ./internal/... -timeout 180s`
