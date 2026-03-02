# Requirements: Dividend & OHLC Batch (fb_799b5844, fb_a89d4d22, fb_827739dd)

## Feedback Items

| ID | Summary | Scope |
|----|---------|-------|
| fb_799b5844 | Add OHLC candle data to get_stock_data response | Small |
| fb_a89d4d22 | Add ledger-confirmed dividend amount to portfolio response | Small |
| fb_827739dd | Dividend processing foundation — Ticker field + EODHD dividend client | Medium |

## What's IN Scope

1. **OHLC Candles**: Expose historical EOD bars in `get_stock_data` response
2. **Ledger Dividend**: Add `ledger_dividend_return` to Portfolio response
3. **Dividend Foundation**: Add `Ticker` field to CashTransaction + EODHD `GetDividends()` client

## What's OUT of Scope (deferred)

- Dividend matching logic (auto-linking cash dividends to holdings)
- Pending/banked dividend states in cash accounts
- Standalone user design (non-Navexa)
- Dividend enrichment during BUY/SELL processing

---

## Feature 1: OHLC Candle Data (fb_799b5844)

### Root Cause
`StockData` response only includes `PriceData` (latest bar's OHLC). The full `MarketData.EOD []EODBar` array is stored but not surfaced to API clients. Candlestick pattern analysis requires historical OHLC bars.

### Files to Change

**`internal/models/market.go`** — Add Candles field to StockData
- After `Price *PriceData` (line 180), add:
```go
Candles []EODBar `json:"candles,omitempty"` // Historical OHLC bars (most recent first)
```

**`internal/services/market/service.go`** — Populate Candles in GetStockData
- Inside the `if include.Price && len(marketData.EOD) > 0` block (line 480), after the real-time overlay (line 536), add:
```go
// Include historical OHLC candle data (up to 200 bars)
maxCandles := 200
candles := marketData.EOD
if len(candles) > maxCandles {
    candles = candles[:maxCandles]
}
stockData.Candles = candles
```

**`internal/server/catalog.go`** — Update get_stock_data description (line 1005)
- Add to description: "When price is included, also returns `candles` array of historical OHLC bars (up to 200 trading days, most recent first) for candlestick pattern analysis."

### Tests

**Unit test** in `internal/services/market/service_test.go`:
- `TestGetStockData_CandlesPopulated` — verify Candles field is populated with EODBar data when Price is included
- `TestGetStockData_CandlesLimitedTo200` — verify Candles is capped at 200 bars
- `TestGetStockData_CandlesNotIncludedWithoutPrice` — verify Candles is nil when Price is not requested

**Integration test** in `tests/api/`:
- `TestStockData_CandlesField` — verify candles array in get_stock_data response

---

## Feature 2: Ledger Dividend Return (fb_a89d4d22)

### Root Cause
Portfolio has `dividend_return` (from Navexa holdings) but no field showing the confirmed dividend amount from the cash flow ledger. The portal needs both to display "Dividends: $971.47 ($1,906.42)".

### Data Flow
```
Cash flow ledger → ledger.Summary().NetCashByCategory["dividend"] → Portfolio.LedgerDividendReturn
```

### Files to Change

**`internal/models/portfolio.go`** — Add LedgerDividendReturn field to Portfolio struct
- After `DividendReturn` (line 57), add:
```go
LedgerDividendReturn float64 `json:"ledger_dividend_return"` // confirmed dividends from cash flow ledger
```

**`internal/services/portfolio/service.go`** — Populate LedgerDividendReturn
- At line 416, inside the existing `if s.cashflowSvc != nil` block where ledger is already loaded:
```go
if s.cashflowSvc != nil {
    if ledger, err := s.cashflowSvc.GetLedger(ctx, name); err == nil && ledger != nil {
        totalCash = ledger.TotalCashBalance()
        // Get confirmed dividend total from ledger
        summary := ledger.Summary()
        ledgerDividends = summary.NetCashByCategory[string(models.CashCatDividend)]
    }
}
```
- Declare `var ledgerDividends float64` near the other totals (line 399)
- Set `LedgerDividendReturn: ledgerDividends` in the portfolio struct construction (after line 457)

**`internal/server/catalog.go`** — Update get_portfolio description (line 253)
- Add: "Includes `ledger_dividend_return` (confirmed dividends from cash flow ledger, distinct from `dividend_return` which is Navexa-calculated)."

**`internal/common/version.go`** — Bump SchemaVersion "10" → "11"
- Forces re-sync of cached portfolios to pick up the new field

### Tests

**Unit test** in `internal/services/portfolio/service_test.go`:
- `TestSyncPortfolio_LedgerDividendReturn` — verify LedgerDividendReturn populated from ledger
- `TestSyncPortfolio_LedgerDividendReturn_NoLedger` — verify 0 when no cash flow service

**Integration test** in `tests/api/`:
- `TestPortfolioLedgerDividendReturn_FieldPresent` — verify field in response

---

## Feature 3: Dividend Processing Foundation (fb_827739dd)

### 3a: Ticker Field on CashTransaction

Add optional `Ticker` field to `CashTransaction` for dividend attribution. When category is "dividend", the ticker links the cash event to a holding.

**`internal/models/cashflow.go`** — Add Ticker field to CashTransaction struct
- After `Description` (line 64), add:
```go
Ticker string `json:"ticker,omitempty"` // Optional: links dividend to holding (e.g. "BHP.AU")
```

**No validation change needed** — Ticker is optional on all categories. The field is informational.

**`internal/server/catalog.go`** — Update add_cash_transaction and set_cash_transactions descriptions
- Mention the optional ticker field for dividend attribution

### 3b: EODHD Dividend Client

Add `GetDividends()` to the EODHD client. EODHD endpoint: `/div/{ticker}?from=YYYY-MM-DD&to=YYYY-MM-DD&fmt=json`

**`internal/models/market.go`** — Add DividendEvent struct
- After EODBar (line 73), add:
```go
// DividendEvent represents a historical dividend payment from EODHD
type DividendEvent struct {
    Date            time.Time `json:"date"`              // Ex-dividend date
    DeclarationDate string    `json:"declaration_date"`  // When dividend was declared
    RecordDate      string    `json:"record_date"`       // Record date for eligibility
    PaymentDate     string    `json:"payment_date"`      // When dividend is paid
    Value           float64   `json:"value"`             // Dividend per share (split-adjusted)
    UnadjustedValue float64   `json:"unadjusted_value"`  // Raw dividend per share
    Currency        string    `json:"currency"`          // e.g. "AUD", "USD"
    Period          string    `json:"period"`            // e.g. "Quarterly", "Annual"
}
```

**`internal/interfaces/clients.go`** — Add GetDividends to EODHDClient interface
- After GetBulkEOD (line 22), add:
```go
// GetDividends retrieves historical dividend events for a ticker
GetDividends(ctx context.Context, ticker string, from, to time.Time) ([]models.DividendEvent, error)
```

**`internal/clients/eodhd/client.go`** — Implement GetDividends
- Follow the same pattern as `GetEOD()` (lines 300-346). Endpoint: `/div/{ticker}`.
- API response struct:
```go
type dividendResponse struct {
    Date            string  `json:"date"`
    DeclarationDate string  `json:"declarationDate"`
    RecordDate      string  `json:"recordDate"`
    PaymentDate     string  `json:"paymentDate"`
    Value           float64 `json:"value"`
    UnadjustedValue float64 `json:"unadjustedValue"`
    Currency        string  `json:"currency"`
    Period          string  `json:"period"`
}
```
- Map to `[]models.DividendEvent` with date parsing (same `time.Parse("2006-01-02", ...)` pattern).

### Tests

**Unit test** in `internal/clients/eodhd/`:
- `TestGetDividends_ParsesResponse` — verify JSON parsing with mock server

**Integration test** in `tests/api/`:
- `TestCashTransaction_TickerField` — verify ticker field roundtrips on dividend transactions

---

## Implementation Order

1. Feature 1 (OHLC Candles) — independent, no model migration
2. Feature 2 (Ledger Dividend) — needs schema version bump
3. Feature 3a (Ticker field) — simple model addition
4. Feature 3b (EODHD dividend client) — new client method

Schema version bump once (10→11) for both Features 2 and 3a.

## Verification Checklist

- [ ] `go build ./cmd/vire-server/`
- [ ] `go test ./internal/...`
- [ ] `go vet ./...`
- [ ] `golangci-lint run`
- [ ] Integration tests pass against live server
