# Requirements: get_summary Fix & Currency on CashAccount

## Feedback Items
- fb_d2eb8712 (MEDIUM): get_summary double-counts — adds holdings value + TotalCash (which includes deposits already converted to holdings). Refactor: add `get_cash_summary` tool, fix `get_summary` totals.
- fb_b6e0e817 (MEDIUM): CashAccount model has no currency field — cannot represent multi-currency cash positions (e.g. USD Wall St account alongside AUD Trading).
- fb_98b90af1: **RESOLVED** — portal MCP catalog auto-refresh already implemented. Out of scope.

## Scope

**In scope:**
1. Add `Currency` field to `CashAccount` struct (default `"AUD"`)
2. Add `Currency` field to `CashAccountUpdate` struct
3. Add `currency` parameter to `update_account` MCP tool catalog entry
4. Update `UpdateAccount` service method to apply currency updates
5. Add `Currency` field to `cashAccountWithBalance` response struct
6. Update all 6 auto-create locations to set default currency `"AUD"`
7. Add new `get_cash_summary` MCP tool (GET endpoint returning cash summary data)
8. Fix `get_summary` double-counting: `review.TotalValue` at `service.go:814` should NOT add `portfolio.TotalCash`
9. Update `CashFlowSummary` to include per-currency totals
10. Unit tests for all changes

**Out of scope:**
- Portal changes (portal updated separately)
- FX conversion of cash balances (balances stored and reported in native currency)
- Historical data repair / migration of existing accounts
- Automatic currency detection

---

## Analysis

### Problem 1: get_summary double-counting (fb_d2eb8712)

**Root cause** — `internal/services/portfolio/service.go:814`:
```go
review.TotalValue = liveTotal + portfolio.TotalCash
```

`portfolio.TotalCash` is the sum of ALL cash transactions (contributions, dividends, etc.). When a user deposits $100k and buys $80k of stock, `TotalCash` still reflects the ledger balance (e.g. $20k remaining), but `portfolio.TotalValue` was already set to `totalValue + totalCash` at line 430. So `ReviewPortfolio` ALREADY receives a portfolio where `TotalValue` includes cash. Then line 814 adds `portfolio.TotalCash` AGAIN to `liveTotal` (which is recomputed from live holding prices, which is just the holdings value).

Actually let me re-examine: `liveTotal` at line 807-811 sums `hr.Holding.MarketValue` for active holdings. This is correct — it's the live equity value. Adding `portfolio.TotalCash` to it gives `holdings + cash = total portfolio value`. This is NOT double-counting if `TotalCash` represents actual remaining cash (not total deposits).

Let me verify: `TotalCashBalance()` at `cashflow.go:139-145` sums ALL `SignedAmount()` across all transactions. For SMSF with $478k deposited and stocks purchased via broker (not tracked as cash debits in the ledger), the ledger balance is $478k because there are no debit transactions for stock purchases. The Navexa integration handles trades separately — the cash ledger only tracks manual deposits/withdrawals/dividends.

**THIS IS THE ACTUAL BUG**: The cash ledger balance ($478k) represents deposits that have already been deployed to buy stocks. The portfolio holdings value ($427k) represents the current market value of those same stocks. Adding them together ($905k) double-counts the capital.

**The fix is NOT to change line 814**. The fix is conceptual: `TotalCash` from the ledger should NOT be treated as "cash on hand". It's a record of cash flows, not a bank balance. The `TotalCashBalance()` function name is misleading.

**However**, for users who DO have actual uninvested cash sitting in their brokerage, the ledger IS the right source. The problem is that contributions should be offset by trade settlements (buys = debits, sells = credits) but this isn't implemented yet (it's marked as "future enhancement" — trade settlement auto-apply to transactional accounts).

**Pragmatic fix for get_summary**: The `get_summary` tool generates a report via `GenerateReport` → `ReviewPortfolio`. The report's `TotalValue` at line 814 should use `liveTotal` ONLY (just holdings market value). Cash information should be presented separately through the new `get_cash_summary` tool. This aligns with the feedback request.

**Changes to line 814**:
```go
// Before:
review.TotalValue = liveTotal + portfolio.TotalCash

// After:
review.TotalValue = liveTotal
```

This means `TotalValue` on `PortfolioReview` represents holdings market value only. The summary report will show the correct holdings-only total.

**Also fix line 430** in `GetPortfolio` — `Portfolio.TotalValue` should also be holdings-only:
```go
// Before:
TotalValue: totalValue + totalCash,

// After:
TotalValue: totalValue,
```

Wait — this would break other things. `Portfolio.TotalValue` is used for weight calculations and other purposes. The JSON field description says "holdings + external balances". Let me check what consumes this...

Actually, looking at this more carefully:
- `Portfolio.TotalValueHoldings` at line 48 = "equity holdings only"
- `Portfolio.TotalValue` at line 49 = "holdings + external balances"
- These are ALREADY separate fields

So the fix for `ReviewPortfolio` line 814 is simple: use `liveTotal` without adding cash:
```go
review.TotalValue = liveTotal
```

And `Portfolio.TotalValue` can stay as `totalValue + totalCash` (it's documented as including external balances). The issue is only in the report summary where `formatReportSummary` shows `review.TotalValue` as the total — this should show holdings-only.

BUT WAIT — if `Portfolio.TotalValue` still includes `totalCash`, and `totalCash = TotalCashBalance()` which is $478k of undeployed deposits, then `get_portfolio` also shows an inflated total. This is the same problem.

**Final decision**:
- `Portfolio.TotalValue` stays as-is (holdings + cash) — it's explicitly documented
- `PortfolioReview.TotalValue` at line 814: change to `liveTotal` (holdings only)
- The `get_summary` report will show the correct holdings-only value
- Add a new `get_cash_summary` tool so cash data is accessible separately
- The `get_portfolio` tool already has `TotalValueHoldings` for holdings-only queries

### Problem 2: Currency on CashAccount (fb_b6e0e817)

Straightforward model change. Add `Currency` field, default to `"AUD"`, propagate through all auto-create locations and the update handler.

For `CashFlowSummary`, change `TotalCash` to a map of currency → balance instead of a single float. This way multi-currency portfolios show correct per-currency totals.

Actually, per the feedback: "Report total_cash per currency (e.g. total_cash: { AUD: 427985.09, USD: 48000 })". Let's do that.

But changing `TotalCash` from `float64` to `map[string]float64` is a breaking change that affects:
- `cashflow.go:103` — Summary() method
- `handlers.go:1680` — response construction
- `service.go:406` — `TotalCashBalance()` is still used for portfolio weight calculation
- Any code that reads `summary.total_cash` as a number

**Approach**: Keep `TotalCash float64` on `CashFlowSummary` (sum of ALL accounts regardless of currency — for backward compat). Add a new field `TotalCashByCurrency map[string]float64` for the per-currency breakdown.

---

## Files to Change

### 1. `internal/models/cashflow.go` — Add Currency to CashAccount

**Line 36-40**: Add Currency field to CashAccount:
```go
type CashAccount struct {
    Name            string `json:"name"`
    Type            string `json:"type"`
    IsTransactional bool   `json:"is_transactional"`
    Currency        string `json:"currency"` // ISO 4217 currency code (e.g. "AUD", "USD"). Default: "AUD"
}
```

**Line 42-47**: Add Currency field to CashAccountUpdate:
```go
type CashAccountUpdate struct {
    Type            string `json:"type,omitempty"`
    IsTransactional *bool  `json:"is_transactional,omitempty"`
    Currency        string `json:"currency,omitempty"` // ISO 4217 currency code
}
```

**Line 101-106**: Add TotalCashByCurrency to CashFlowSummary:
```go
type CashFlowSummary struct {
    TotalCash           float64            `json:"total_cash"`
    TotalCashByCurrency map[string]float64 `json:"total_cash_by_currency"` // Balance per currency
    TransactionCount    int                `json:"transaction_count"`
    ByCategory          map[string]float64 `json:"by_category"`
}
```

**Lines 109-125**: Update Summary() to compute per-currency totals:
```go
func (l *CashFlowLedger) Summary() CashFlowSummary {
    byCategory := make(map[string]float64)
    for _, tx := range l.Transactions {
        byCategory[string(tx.Category)] += tx.Amount
    }
    for _, cat := range []CashCategory{CashCatContribution, CashCatDividend, CashCatTransfer, CashCatFee, CashCatOther} {
        if _, ok := byCategory[string(cat)]; !ok {
            byCategory[string(cat)] = 0
        }
    }

    // Compute per-currency totals
    byCurrency := make(map[string]float64)
    accountCurrency := make(map[string]string)
    for _, a := range l.Accounts {
        cur := a.Currency
        if cur == "" {
            cur = "AUD"
        }
        accountCurrency[a.Name] = cur
    }
    for _, tx := range l.Transactions {
        cur := accountCurrency[tx.Account]
        if cur == "" {
            cur = "AUD"
        }
        byCurrency[cur] += tx.SignedAmount()
    }

    return CashFlowSummary{
        TotalCash:           l.TotalCashBalance(),
        TotalCashByCurrency: byCurrency,
        TransactionCount:    len(l.Transactions),
        ByCategory:          byCategory,
    }
}
```

### 2. `internal/services/cashflow/service.go` — Update auto-create locations + UpdateAccount

**6 auto-create locations** — all need `Currency: "AUD"` added to the `CashAccount` struct literal:

1. **Line 93-96** (GetLedger default): Add `Currency: "AUD"` to default Trading account
2. **Line 111-113** (GetLedger fallback): Add `Currency: "AUD"` to default Trading account
3. **Line 139-142** (AddTransaction auto-create): Add `Currency: "AUD"`
4. **Line 212-220** (AddTransfer auto-create, two locations): Add `Currency: "AUD"` to both from_account and to_account auto-creates
5. **Line 419-423** (SetTransactions auto-create): Add `Currency: "AUD"`
6. **Line 454-456** (ClearLedger default): Add `Currency: "AUD"` to default Trading account

**UpdateAccount method (line 359-389)**: Add currency update logic:
```go
// After the IsTransactional block (line 378-379), add:
if update.Currency != "" {
    acct.Currency = update.Currency
}
```

Also add currency to the log line at line 386:
```go
s.logger.Info().Str("portfolio", portfolioName).Str("account", accountName).
    Str("type", acct.Type).Bool("is_transactional", acct.IsTransactional).
    Str("currency", acct.Currency).
    Msg("Account updated")
```

### 3. `internal/server/handlers.go` — Update response struct + add get_cash_summary handler

**Line 1644-1650**: Add Currency to cashAccountWithBalance:
```go
type cashAccountWithBalance struct {
    Name            string  `json:"name"`
    Type            string  `json:"type"`
    IsTransactional bool    `json:"is_transactional"`
    Currency        string  `json:"currency"`
    Balance         float64 `json:"balance"`
}
```

**Line 1668-1673**: Add Currency to response construction in newCashFlowResponse:
```go
accounts[i] = cashAccountWithBalance{
    Name:            a.Name,
    Type:            a.Type,
    IsTransactional: a.IsTransactional,
    Currency:        a.Currency,
    Balance:         ledger.AccountBalance(a.Name),
}
```

But wait — `a.Currency` might be empty for existing accounts that were created before the currency field was added. We need a fallback:
```go
currency := a.Currency
if currency == "" {
    currency = "AUD"
}
accounts[i] = cashAccountWithBalance{
    Name:            a.Name,
    Type:            a.Type,
    IsTransactional: a.IsTransactional,
    Currency:        currency,
    Balance:         ledger.AccountBalance(a.Name),
}
```

**New handler — handleCashSummary**: Add after `handleCashFlows` (after line 1759):
```go
func (s *Server) handleCashSummary(w http.ResponseWriter, r *http.Request, name string) {
    if !RequireMethod(w, r, http.MethodGet) {
        return
    }
    ctx := r.Context()

    if _, err := s.app.PortfolioService.GetPortfolio(ctx, name); err != nil {
        WriteError(w, http.StatusNotFound, fmt.Sprintf("Portfolio not found: %v", err))
        return
    }
    ledger, err := s.app.CashFlowService.GetLedger(ctx, name)
    if err != nil {
        WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error getting cash summary: %v", err))
        return
    }

    // Build per-account balances with currency
    accounts := make([]cashAccountWithBalance, len(ledger.Accounts))
    for i, a := range ledger.Accounts {
        currency := a.Currency
        if currency == "" {
            currency = "AUD"
        }
        accounts[i] = cashAccountWithBalance{
            Name:            a.Name,
            Type:            a.Type,
            IsTransactional: a.IsTransactional,
            Currency:        currency,
            Balance:         ledger.AccountBalance(a.Name),
        }
    }

    WriteJSON(w, http.StatusOK, map[string]interface{}{
        "portfolio_name": name,
        "accounts":       accounts,
        "summary":        ledger.Summary(),
    })
}
```

### 4. `internal/server/router.go` — Add route for get_cash_summary

Find where `/cash-transactions` route is registered and add a new route:
```go
// Add route for cash summary
// Path: /api/portfolios/{name}/cash-summary
```

We need to find the exact router file and pattern.

### 5. `internal/services/portfolio/service.go` — Fix ReviewPortfolio double-counting

**Line 813-815**: Remove TotalCash addition:
```go
// Before:
if liveTotal > 0 {
    review.TotalValue = liveTotal + portfolio.TotalCash
}

// After:
if liveTotal > 0 {
    review.TotalValue = liveTotal
}
```

This means `PortfolioReview.TotalValue` represents holdings market value only.
The `DayChangePct` calculation at line 817-819 should also use just `liveTotal`:
```go
if review.TotalValue > 0 {
    review.DayChangePct = (dayChange / review.TotalValue) * 100
}
```
This is already correct since it uses `review.TotalValue` which we just set to `liveTotal`.

### 6. `internal/server/catalog.go` — Add get_cash_summary tool + update_account currency

**After the `list_cash_transactions` entry (line 378)**, add new tool:
```go
{
    Name:        "get_cash_summary",
    Description: "FAST: Get cash account summary for a portfolio — account balances with currency, total cash, and category breakdown. No transaction details (use list_cash_transactions for full ledger).",
    Method:      "GET",
    Path:        "/api/portfolios/{portfolio_name}/cash-summary",
    Params: []models.ParamDefinition{
        portfolioParam,
    },
},
```

**Line 572-599**: Add currency parameter to update_account tool:
After the `type` parameter (line 594-596), add:
```go
{
    Name:        "currency",
    Type:        "string",
    Description: "ISO 4217 currency code (e.g. 'AUD', 'USD').",
    In:          "body",
},
```

**Line 574**: Update update_account description:
```go
Description: "Update a cash account's properties (type, is_transactional, currency). All accounts contribute to total_cash.",
```

### 7. `internal/server/handlers.go` — Route the new endpoint

Find where routes are set up. Look for the pattern used for `/cash-transactions`:
```go
// We need to find the router/mux setup and add:
// GET /api/portfolios/{name}/cash-summary → handleCashSummary
```

---

## Router Setup

**File**: `internal/server/routes.go`

The portfolio subpath routing is a switch at line 226+. Add `cash-summary` case before `cash-transactions` (line 239):

```go
// Line 237-240 currently:
case "glossary":
    s.handleGlossary(w, r, name)
case "cash-transactions":
    s.handleCashFlows(w, r, name)

// Add between glossary and cash-transactions:
case "cash-summary":
    s.handleCashSummary(w, r, name)
case "cash-transactions":
    s.handleCashFlows(w, r, name)
```

---

## Unit Tests

### Test: Currency field on CashAccount
**File**: `internal/models/cashflow_test.go` — add or update existing test
```go
func TestSummary_PerCurrencyTotals(t *testing.T) {
    ledger := &CashFlowLedger{
        Accounts: []CashAccount{
            {Name: "Trading", Currency: "AUD"},
            {Name: "Wall St", Currency: "USD"},
        },
        Transactions: []CashTransaction{
            {Account: "Trading", Amount: 100000, Category: CashCatContribution},
            {Account: "Wall St", Amount: 48000, Category: CashCatContribution},
        },
    }
    s := ledger.Summary()
    assert.Equal(t, 100000.0, s.TotalCashByCurrency["AUD"])
    assert.Equal(t, 48000.0, s.TotalCashByCurrency["USD"])
    assert.Equal(t, 148000.0, s.TotalCash) // aggregate (mixed currencies)
}
```

### Test: Currency default fallback
```go
func TestSummary_EmptyCurrencyDefaultsToAUD(t *testing.T) {
    ledger := &CashFlowLedger{
        Accounts: []CashAccount{
            {Name: "Trading"}, // no Currency set
        },
        Transactions: []CashTransaction{
            {Account: "Trading", Amount: 50000, Category: CashCatContribution},
        },
    }
    s := ledger.Summary()
    assert.Equal(t, 50000.0, s.TotalCashByCurrency["AUD"])
}
```

### Test: UpdateAccount with currency
**File**: `internal/services/cashflow/service_test.go` — add test
```go
func TestUpdateAccount_Currency(t *testing.T) {
    // Setup: create ledger with default Trading account
    // Update account currency to "USD"
    // Verify currency is "USD" on the returned ledger
}
```

### Test: ReviewPortfolio TotalValue excludes cash
**File**: `internal/services/portfolio/service_test.go` or similar
- Verify that `PortfolioReview.TotalValue` equals sum of active holding market values only
- No cash added

---

## Integration Points

### Router registration
The implementer MUST find where `handleCashFlows` is dispatched (search for `handleCashFlows` or `cash-transactions` in the router setup) and add a matching entry for `handleCashSummary` on path `/cash-summary`.

### Existing tests
After adding `Currency` to `CashAccount`, any test that constructs `CashAccount{}` literals will still compile (Currency defaults to zero value `""`). The fallback to `"AUD"` in handler and Summary() ensures backward compatibility.

### Catalog test
The catalog count test at `internal/server/catalog_test.go:17` and `:143` expects `52` tools. Increment to `53` for the new `get_cash_summary` tool.

---

## Notes
- `Portfolio.TotalValue` at `service.go:430` is kept as `totalValue + totalCash` — it's documented as "holdings + external balances" and is used for weight calculations. Only `PortfolioReview.TotalValue` changes.
- Existing `CashFlowSummary.TotalCash` remains as-is (aggregate across all currencies). The new `TotalCashByCurrency` field provides the per-currency breakdown.
- No FX conversion is performed — balances are stored and reported in their native account currency.
- Portal will need updating to display currency on account balances and handle the new `get_cash_summary` tool.
