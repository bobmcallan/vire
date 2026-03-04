# Devils Advocate Analysis: Source-Typed Portfolios & Trade Management

**Analyst:** devils-advocate
**Date:** 2026-03-04
**Status:** COMPLETE — implementation reviewed, 34 stress tests executed
**Stress Test File:** `internal/services/trade/service_stress_test.go`
**Results:** 24 PASS, 10 FAIL (confirmed vulnerabilities)

---

## 1. Input Validation Gaps

### 1.1 Trade Validation — Missing Controls (MEDIUM)

The requirements specify validation: "ticker non-empty, units > 0, price >= 0, action is buy/sell, date non-zero"

**Missing from requirements (compare with cashflow `validateCashTransaction`):**

| Check | CashFlow Has It | Trade Spec Has It | Risk |
|-------|----------------|-------------------|------|
| String length limits (ticker, notes, source_ref) | Yes (account 100, desc 500, notes 1000) | NO | Unbounded strings → storage bloat |
| `math.IsInf` / `math.IsNaN` on numeric fields | Yes (amount) | NO | NaN/Inf in units/price → poison calculations |
| Max value cap (1e15) | Yes (amount) | NO | Overflow in Consideration() → corrupted P&L |
| Future date rejection | Yes (date) | NO | Trades dated in 2099 → confuse position timeline |
| TrimSpace on string fields | Yes (account, description) | NOT SPECIFIED | Leading/trailing whitespace → duplicate tickers "BHP.AU" vs " BHP.AU" |

**Attack vectors:**
- `units: NaN` → `DeriveHolding` produces NaN avg_cost, NaN cost_basis, propagates to portfolio
- `units: Infinity` → `running_units += Inf` → all subsequent math corrupted
- `price: -1` → negative consideration → realized P&L inverted
- `ticker: ""` (empty after trim) vs `ticker: "   "` (whitespace only) — do both get caught?
- `ticker: "BHP.AU"` vs `ticker: "bhp.au"` — case sensitivity creates duplicate positions
- `fees: -100` → negative fees = free money in consideration calculation

### 1.2 Price >= 0 Allows Zero Price (LOW)

Spec says `price >= 0`. A buy at price=0 is suspicious but technically valid (bonus shares, stock grants). However:
- Sell at price=0: `proceeds = 0 * units - fees = -fees` → negative proceeds. Acceptable?
- Buy at price=0, then sell: `avg_cost = 0`, realized P&L = proceeds - 0 = proceeds. This is correct for bonus shares.
- **Verdict:** price=0 is OK but should probably log a warning.

### 1.3 Fees Validation Missing (MEDIUM)

No validation on `fees` field at all in the spec:
- `fees: -50` → Buy consideration = `units * price + (-50)` = discount. Sell = `units * price - (-50)` = bonus proceeds.
- `fees: Infinity` → corrupts consideration
- `fees: NaN` → propagates through all calculations

**Recommendation:** Validate `fees >= 0`, `!IsInf`, `!IsNaN`, `fees < 1e10`.

---

## 2. Sell Validation & Position Integrity

### 2.1 Sell More Than Held (CRITICAL — spec addresses this)

Spec says: "For sells: validate units <= current position (compute from existing trades)"

**Attack sequence to probe:**
1. Buy 100 BHP @ $50
2. Sell 100 BHP @ $55 (full sell — valid)
3. Sell 1 BHP @ $55 → MUST fail (0 units held)

**Edge case:** What about the float comparison? `running_units` after full sell might be `0.0000000000001` due to float precision. If the check is `sell_units > running_units`, a sell of 1 unit might pass because `1 > 0.0000000000001` is true. But position derivation would produce negative units.

**Recommendation:** Use epsilon comparison: `if sell_units > running_units + 1e-9`

### 2.2 Sell Before Any Buy (MEDIUM)

Can you add a sell as the first trade for a ticker? The spec computes current position from existing trades. If there are no buys, position = 0, sell of any amount should fail.

### 2.3 Partial Sell Creates Fractional Position (LOW)

Buy 3 units, sell 2 units → 1 unit remaining. This is fine. But:
- Buy 100 units @ $10, sell 33.333... units → remaining = 66.666... units
- Float precision: `100 - 33.333333333333336 = 66.66666666666666` — is this displayed cleanly?

---

## 3. Position Derivation (DeriveHolding) — Critical Path

### 3.1 Division by Zero (CRITICAL)

```
avg_cost_at_sell = running_cost / running_units
```

If `running_units == 0` when a sell occurs → division by zero → panic or Inf.

**When can this happen?**
- Bug in sell validation (2.1 above)
- Trade update changes units after sell validation passed
- Trade removal leaves orphaned sells

**Also at the end:**
```
holding.avg_cost = running_cost / running_units  (if units > 0, else 0)
```

The spec handles the final case (`if units > 0, else 0`). But `avg_cost_at_sell` mid-loop has NO guard in the spec.

### 3.2 Float Precision Accumulation (MEDIUM)

Large number of buy/sell cycles will accumulate floating-point errors:
```
Buy 1000 @ $1.11 → cost = 1110
Sell 500 → avg_cost_at_sell = 1110/1000 = 1.11, cost_of_sold = 555
remaining_cost = 555, remaining_units = 500
Sell 500 → avg_cost_at_sell = 555/500 = 1.11, cost_of_sold = 555
remaining_cost = 0, remaining_units = 0
```

But with floats:
```
Buy 1000 @ $1.11 → cost = 1110.0000000000002 (float imprecision)
Sell 333 → avg_cost = 1.1100000000000003, cost_of_sold = 369.63000000000011
... errors compound over 50+ cycles
```

### 3.3 Very Large Numbers (LOW)

Without a cap on units/price:
- Buy 1e18 units @ $1e18 → consideration = 1e36 → exceeds float64 safe integer range (2^53)
- Operations on such values lose precision silently

### 3.4 Negative Running Cost After Float Drift (MEDIUM)

After many sell cycles, `running_cost` could drift to a small negative number (e.g., -0.0000001) due to float imprecision. This would make `avg_cost` negative, which cascades:
- `cost_of_sold` becomes negative
- `realized_pnl` inflated
- `unrealized_return` miscalculated

---

## 4. Snapshot Import Vulnerabilities

### 4.1 Duplicate Tickers in Positions Array (MEDIUM)

What happens when the positions array contains the same ticker twice?
```json
{"positions": [
  {"ticker": "BHP.AU", "units": 100, "avg_cost": 45},
  {"ticker": "BHP.AU", "units": 200, "avg_cost": 50}
]}
```

In `assembleSnapshotPortfolio`, both would create separate holdings → portfolio has two BHP.AU entries with different weights. The UI would show duplicates.

**Recommendation:** Deduplicate by ticker (last wins, or merge units).

### 4.2 Zero/Negative Values in Snapshot (MEDIUM)

- `units: 0` → holding with 0 units, 0 market value, 0 weight. Creates noise.
- `units: -5` → negative position. Not validated anywhere in the spec.
- `avg_cost: -10` → negative cost basis → nonsensical returns.
- `avg_cost: 0` → cost_basis = 0, market_value = current_price * units. Return = infinity if cost = 0.

### 4.3 Empty Ticker in Snapshot Position (LOW)

No ticker validation on SnapshotPosition. Empty ticker → holding with empty ticker string.

### 4.4 Merge Mode: Ticker Matching Case Sensitivity (LOW)

In merge mode, matching is presumably by ticker string equality. "BHP.AU" != "bhp.au" → creates duplicate instead of merging.

---

## 5. Portfolio Creation Vulnerabilities

### 5.1 Source Type Injection (LOW — spec handles this)

Spec validates against `ValidPortfolioSourceTypes`. Passing `source_type: "navexa"` fails because it's not in the valid set. Good.

### 5.2 Name Validation (LOW — spec handles this)

Spec has `name != ""` and `len(name) <= 100`. Good.

**Not checked:**
- Name with only whitespace: `"   "` — passes length check but is effectively empty
- Name with special characters: `"../../etc/passwd"` — not a path traversal risk since it's a KV key, but ugly
- Name with newlines/control characters: `"Portfolio\nInjection"` — could break log output

### 5.3 Overwrite Prevention (LOW — spec handles this)

Spec checks `existing != nil` and returns error. Good. But:
- Race condition: two concurrent CreatePortfolio with the same name. Both check "not exists", both save → last write wins, first portfolio's data lost. No mutex or CAS operation.

---

## 6. Race Conditions (MEDIUM-HIGH)

### 6.1 Concurrent Trade Adds — Full-Document Save Pattern

The trade service uses the same full-document save pattern as cashflow:
1. Read TradeBook
2. Append trade
3. Save TradeBook

Two concurrent AddTrade calls:
1. Goroutine A reads TradeBook (version 5, 10 trades)
2. Goroutine B reads TradeBook (version 5, 10 trades)
3. Goroutine A appends trade, saves (version 6, 11 trades)
4. Goroutine B appends trade, saves (version 6, 11 trades) — **OVERWRITES A's trade**

The cashflow service has the same vulnerability (known pattern in the codebase).

**Impact:** Lost trades in concurrent usage. Low probability in single-user MCP context but still a correctness issue.

**Recommendation:** Optimistic concurrency via version check: `if stored.Version != expected.Version { return ErrConflict }`.

### 6.2 Concurrent Sell Validation Race

More dangerous variant:
1. Position = 100 units
2. Goroutine A: sell 80 units → validates (80 <= 100) ✓
3. Goroutine B: sell 80 units → validates (80 <= 100) ✓ (read same snapshot)
4. Both save → total sold = 160, but only 100 held → negative position

---

## 7. Data Integrity & Recovery

### 7.1 Corrupted TradeBook JSON (LOW)

If the stored JSON is corrupted (truncated write, disk error), `GetTradeBook` will fail to unmarshal. The cashflow pattern returns an empty ledger on "not found" — but what about corrupted data? The unmarshal error should be returned, not silently swallowed as "empty".

### 7.2 Orphaned TradeBook After Portfolio Delete (LOW)

If a portfolio is deleted, the TradeBook (stored separately under subject="trades") remains. No cascade delete. Not a security issue but wastes storage.

### 7.3 TradeBook Without Portfolio (LOW)

Can you add trades to a portfolio that doesn't exist? The spec doesn't show a check for portfolio existence in `AddTrade`. The trade would be stored but `GetPortfolio` would fail to route to `assembleManualPortfolio`.

---

## 8. Handler-Level Issues

### 8.1 Error Message Information Leak (LOW)

```go
if strings.Contains(err.Error(), "insufficient") || strings.Contains(err.Error(), "invalid") ...
```

Pattern relies on error message substring matching to determine HTTP status codes. Fragile:
- If the error message changes, a 400 becomes a 500
- Internal error details in the message leak to the client

### 8.2 Missing Auth on Trade Routes (VERIFY)

Need to verify that the trade routes go through `s.authenticatedContext(r)`. The spec shows this in the handler code, but need to verify that the route registration includes auth middleware.

### 8.3 Portfolio Name from URL Path (LOW)

Trade routes extract `portfolioName` from the URL path. Need to verify there's no path traversal — but since it's used as a KV key (not filesystem path), the risk is limited to accessing another user's data. The `authenticatedContext` should scope by user ID.

### 8.4 Trade ID from URL Path — No Validation (LOW)

`tradeID := strings.TrimPrefix(subpath, "trades/")` — no validation on the trade ID format. Passing `trades/../../something` could potentially match unexpected routes if routing logic is complex. Need to verify the route matching doesn't allow this.

---

## 9. Consideration() Method Edge Cases

```go
func (t Trade) Consideration() float64 {
    base := t.Units * t.Price
    if t.Action == TradeActionBuy {
        return base + t.Fees
    }
    return base - t.Fees
}
```

- If `Fees > base` on a sell: `Consideration = negative` → negative proceeds. This is mathematically correct (loss-making trade after fees) but unusual.
- If Action is neither "buy" nor "sell" (invalid action): falls through to sell branch → calculates as a sell. Should validate action before calling.

---

## 10. Stress Test Plan (Post-Implementation)

### Tests to Write

1. **NaN/Inf injection:** AddTrade with NaN units, Inf price, NaN fees
2. **Negative values:** units=-1, price=-1, fees=-1
3. **String bombs:** ticker with 10000 chars, notes with 1MB
4. **Sell validation bypass:** sell more than held, sell with no buys, concurrent sells
5. **Float precision:** 1000 buy/sell cycles, verify final position and P&L
6. **Division by zero:** sell exactly all units, then attempt another sell
7. **Snapshot duplicates:** same ticker twice in positions array
8. **Snapshot negatives:** zero units, negative avg_cost
9. **Portfolio name injection:** whitespace, control chars, very long names
10. **Concurrent trade adds:** goroutine stress test for lost writes
11. **Trade update after sell:** update a buy trade's units to 0 after a sell was made against it
12. **Remove buy trade that has sells against it:** removes the buy, recalculates → negative position?

---

## Stress Test Execution Results (2026-03-04)

**File:** `internal/services/trade/service_stress_test.go`
**Total Tests:** 34
**PASS:** 24 (7 with WARNING annotations)
**FAIL:** 10 (confirmed vulnerabilities)

### FAILURES (require fixes)

| Test | Severity | Finding |
|------|----------|---------|
| TestStress_UpdateBuyUnitsAfterSell | CRITICAL | UpdateTrade created units=-30 (negative position) |
| TestStress_AddTrade_NegativePrice | HIGH | Price -50 accepted, corrupts Consideration() |
| TestStress_AddTrade_NegativeFees | HIGH | Fees -10 accepted, creates free money |
| TestStress_AddTrade_ExtremeUnits | MEDIUM | 1e16 units accepted, exceeds float64 safe range |
| TestStress_AddTrade_ExtremePrice | MEDIUM | 1e16 price accepted, exceeds float64 safe range |
| TestStress_AddTrade_LongTicker | MEDIUM | 10000-char ticker accepted, storage bloat |
| TestStress_AddTrade_LongNotes | MEDIUM | 100KB notes accepted, storage bloat |
| TestStress_AddTrade_LongSourceRef | MEDIUM | 10000-char source_ref accepted |
| TestStress_AddTrade_WhitespaceTicker | MEDIUM | `"   "` accepted as valid ticker |
| TestStress_AddTrade_TickerWithLeadingTrailingSpaces | MEDIUM | `" BHP.AU "` creates separate position from `"BHP.AU"` |

### WARNINGS (passed but flagged concerns)

| Test | Warning |
|------|---------|
| TestStress_SnapshotDuplicateTickers | 2 BHP.AU positions stored (no dedup) |
| TestStress_SnapshotZeroUnits | 0-unit position accepted |
| TestStress_SnapshotNegativeValues | -100 unit position accepted |
| TestStress_SnapshotNegativeAvgCost | avg_cost=-50 accepted |
| TestStress_SnapshotEmptyTicker | Empty ticker in snapshot accepted |
| TestStress_ConcurrentTradeAdds | Lost 13/20 trades in concurrent writes |
| TestStress_AddTrade_EmptyPortfolioName | Empty portfolio name accepted |
| TestStress_AddTrade_LongPortfolioName | 1000-char portfolio name accepted |

### PASSES (confirmed working correctly)

| Test | What it validates |
|------|-------------------|
| TestStress_AddTrade_NaNUnits | NaN caught by json.Marshal (accidental, not validateTrade) |
| TestStress_AddTrade_InfPrice | Inf caught by json.Marshal (accidental, not validateTrade) |
| TestStress_AddTrade_NegativeInfFees | -Inf caught by json.Marshal (accidental) |
| TestStress_AddTrade_NaNFees | NaN caught by json.Marshal (accidental) |
| TestStress_SellWithNoBuys | Correctly rejects sell with 0 position |
| TestStress_SellAfterFullSell | Correctly rejects sell after full liquidation |
| TestStress_DeriveHolding_EmptyTrades | Returns zero values for empty input |
| TestStress_DeriveHolding_ZeroPriceBonus | Bonus shares (price=0) handled correctly |
| TestStress_DeriveHolding_WithCurrentPrice | Market value and unrealized computed correctly |
| TestStress_DeriveHolding_FloatPrecision_ManyBuySellCycles | 100 cycles: no significant drift |
| TestStress_DeriveHolding_LargePositionThenFullSell | 10 partial sells: correct liquidation |
| TestStress_ConcurrentSellRace | Position doesn't go negative (race protected) |
| TestStress_InvalidAction | "short" action rejected |
| TestStress_EmptyAction | Empty action rejected |
| TestStress_Consideration_FeesExceedProceeds | Negative proceeds computed correctly |
| TestStress_Consideration_InvalidAction | Falls through to sell branch (noted) |
| TestStress_TradeBook_TradesForTicker_CaseSensitive | Case-sensitive matching confirmed |
| TestStress_TradeBook_UniqueTickers_Empty | Empty book returns empty tickers |
| TestStress_ListTrades_NegativeOffset | Handled (clamps to 0) |
| TestStress_ListTrades_ZeroLimit | Defaults to 50 |
| TestStress_ListTrades_ExcessiveLimit | Capped at 200 |
| TestStress_AddTrade_NoUserContext | Falls back to default user ID |

### NOTE: NaN/Inf Protection is Accidental

NaN/Inf values pass `validateTrade()` (NaN comparisons return false in Go).
They are caught by `json.Marshal` in `saveTradeBook()`, NOT by validation.
This is **fragile** — the trade is already appended to `tb.Trades` in memory
before the marshal fails. If the caller retries or if there's caching, the
corrupted trade persists in memory. Explicit `math.IsNaN`/`math.IsInf`
checks should be added to `validateTrade()`.

---

## Summary: Findings by Severity (updated with test results)

| # | Severity | Finding | Test Confirmed? | Impl Mitigated? |
|---|----------|---------|-----------------|-----------------|
| 1 | CRITICAL | UpdateTrade creates negative position | YES (units=-30) | NO |
| 2 | HIGH | No NaN/Inf validation (accidental json.Marshal catch) | YES (fragile) | PARTIAL |
| 3 | HIGH | Negative price accepted | YES | NO |
| 4 | HIGH | Negative fees accepted | YES | NO |
| 5 | HIGH | Concurrent sell race → data loss (known pattern) | YES (13/20 lost) | NO |
| 6 | MEDIUM | No string length limits | YES (10KB ticker) | NO |
| 7 | MEDIUM | No max value cap on units/price | YES (1e16 accepted) | NO |
| 8 | MEDIUM | Whitespace ticker creates phantom positions | YES | NO |
| 9 | MEDIUM | Ticker not trimmed → duplicates | YES | NO |
| 10 | MEDIUM | Snapshot: no validation at all | YES (5 warnings) | NO |
| 11 | LOW | Division by zero in DeriveHolding | N/A | YES (guarded) |
| 12 | LOW | Float precision over many cycles | YES | OK (no drift) |
| 13 | LOW | Sell validation (no buys, after full sell) | YES | OK (working) |
