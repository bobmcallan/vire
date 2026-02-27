# Requirements: Fix 4 Feedback Items

## Feedback Items
- fb_9f137670: ACDC.AU bad price ($5.02 vs ~$153)
- fb_fb1b044f: ACDC.AU stale yesterday_close ($5.00) causing +2941% daily change
- fb_5581bfa1: compute_indicators empty response + nonsensical EMA/RSI
- fb_c6fd4a3e: Compliance exchange→country mapping

---

## Bug 1: EODHD Ticker Resolution (fb_9f137670 + fb_fb1b044f)

**Problem:** ACDC.AU resolves to US-listed ACDC (~$5) instead of ASX ACDC (~$153). The EODHD real-time endpoint may strip `.AU` suffix and default to US markets. Also, `GetBulkEOD()` strips suffixes but other methods don't — inconsistent behavior.

**Root cause:** No response validation after EODHD API calls. The API may return data for a different ticker than requested.

**Fix (3 parts):**

### 1a. Add response validation to EODHD client
**File:** `internal/clients/eodhd/client.go`

After `GetRealTimeQuote()` returns, validate that the returned ticker code matches the requested ticker. If mismatch, log a warning and return an error.

```go
// After getting response, validate ticker matches
if resp.Code != "" && !tickerMatches(requested, resp.Code) {
    return nil, fmt.Errorf("ticker mismatch: requested %q but EODHD returned %q", requested, resp.Code)
}
```

### 1b. Add ticker validation helper
**File:** `internal/clients/eodhd/client.go`

```go
// tickerMatches checks if EODHD response code matches the requested ticker.
// Handles suffix stripping: "ACDC.AU" matches "ACDC" but "ACDC.AU" should not match "ACDC.US"
func tickerMatches(requested, returned string) bool {
    // Strip exchange suffixes for comparison
    reqBase := strings.Split(requested, ".")[0]
    retBase := strings.Split(returned, ".")[0]
    if reqBase != retBase {
        return false
    }
    // If returned has an exchange suffix, verify it matches
    reqParts := strings.Split(requested, ".")
    retParts := strings.Split(returned, ".")
    if len(reqParts) > 1 && len(retParts) > 1 {
        return strings.EqualFold(reqParts[1], retParts[1])
    }
    return true
}
```

### 1c. Add exchange validation to real-time quote
**File:** `internal/clients/eodhd/client.go` — `GetRealTimeQuote()` (line ~256)

After the API call succeeds, check that the returned exchange matches what was requested. EODHD real-time response includes an `exchange` field — validate it.

**Tests:**
- Unit test for `tickerMatches()` — various suffix combinations
- Unit test for `GetRealTimeQuote()` with mismatched response (mock)

---

## Bug 2: compute_indicators Empty Response + Bad Values (fb_5581bfa1)

**Problem:** Three sub-bugs:

### 2a. Silent failure — empty response with 200 OK
**File:** `internal/services/signal/service.go` — `DetectSignals()` (lines 38-89)

When market data is unavailable, tickers are silently skipped via `continue`. Returns empty array with no error info.

**Fix:** Track failed tickers and include them in response. Return partial results with error details.

```go
type TickerSignals struct {
    // existing fields...
    Error string `json:"error,omitempty"` // Add error field
}
```

When market data lookup fails, add a `TickerSignals` entry with the error message instead of silently skipping.

### 2b. EMA backward loop
**File:** `internal/signals/indicators.go` — `EMA()` (lines 24-39)

Loop iterates backward (`period-1` → `0`), applying multiplier in wrong direction. Should iterate forward through the data after the initial SMA window.

**Fix:** Correct the loop to iterate from oldest to newest:
```go
func EMA(bars []models.EODBar, period int) float64 {
    if len(bars) < period {
        return 0
    }
    // bars[0] is newest. SMA seed from oldest 'period' bars.
    ema := SMA(bars[len(bars)-period:], period)
    // Iterate from end of SMA window toward newest
    for i := len(bars) - period - 1; i >= 0; i-- {
        ema = (bars[i].Close-ema)*multiplier + ema
    }
    return ema
}
```

### 2c. RSI without Wilder's smoothing
**File:** `internal/signals/indicators.go` — `RSI()` (lines 41-66)

Uses simple average of gains/losses. For portfolio data (monotonically growing), this always returns ~100.

**Fix:** Implement Wilder's smoothing (exponential moving average of gains and losses):
```go
// Initial averages from first 'period' changes
avgGain := gains / float64(period)
avgLoss := losses / float64(period)
// Wilder's smoothing for remaining bars
for i := period; i < len(bars)-1; i++ {
    change := bars[i].Close - bars[i+1].Close
    if change > 0 {
        avgGain = (avgGain*float64(period-1) + change) / float64(period)
        avgLoss = (avgLoss * float64(period-1)) / float64(period)
    } else {
        avgGain = (avgGain * float64(period-1)) / float64(period)
        avgLoss = (avgLoss*float64(period-1) - change) / float64(period)
    }
}
```

**Tests:**
- Unit test for EMA with known values (verify correct smoothing direction)
- Unit test for RSI with monotonic growth (should NOT return 100)
- Unit test for RSI with mixed gains/losses
- Unit test for DetectSignals with missing market data (should return error entries)

---

## Bug 3: Compliance Exchange→Country Mapping (fb_c6fd4a3e)

**Problem:** Compliance engine compares `holding.Exchange` ("ASX", "NYSE") against `investment_universe` (["AU", "US"]). The `eodhExchange()` mapping function exists in `models/portfolio.go` but is unexported and unused in compliance.

**File:** `internal/services/strategy/compliance.go` (lines 98-111)

**Fix:**
1. Export `eodhExchange()` → `EodhExchange()` in `internal/models/portfolio.go`
2. Use it in compliance check:
```go
exchangeCode := models.EodhExchange(holding.Exchange)
for _, u := range strategy.InvestmentUniverse {
    if strings.EqualFold(exchangeCode, u) {
        found = true
        break
    }
}
```

**Tests:**
- Fix existing test `TestCheckCompliance_InvestmentUniverseMismatch` — use "NYSE" not "US"
- Add test: ASX holding with universe ["AU"] → compliant
- Add test: NYSE holding with universe ["AU", "US"] → compliant
- Add test: LSE holding with universe ["AU", "US"] → non-compliant

---

## Files Expected to Change

| File | Change |
|------|--------|
| `internal/clients/eodhd/client.go` | Add ticker validation after API responses |
| `internal/clients/eodhd/client_test.go` | Tests for ticker validation |
| `internal/signals/indicators.go` | Fix EMA loop direction, RSI Wilder's smoothing |
| `internal/signals/indicators_test.go` | Tests for corrected EMA/RSI |
| `internal/services/signal/service.go` | Return error entries instead of silent skip |
| `internal/models/portfolio.go` | Export `eodhExchange()` → `EodhExchange()` |
| `internal/services/strategy/compliance.go` | Use `EodhExchange()` in universe check |
| `internal/services/strategy/compliance_test.go` | Fix and add compliance tests |
