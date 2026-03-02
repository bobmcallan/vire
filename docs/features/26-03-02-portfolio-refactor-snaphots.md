# Vire Portfolio Source Architecture

## Refactor: Source-Typed Portfolios & Holdings

**Status:** Design  
**Date:** 2 March 2026  
**Author:** Bob / Claude  

---

## Problem Statement

Vire currently treats all portfolio data as read-only, synced exclusively from Navexa. This creates a hard dependency on a single source and prevents common workflows like:

- Screenshotting a broker app and having an LLM enter the positions into Vire
- Manually recording trades that aren't tracked in Navexa
- Importing trade history from CSV exports
- Managing portfolios that don't exist in Navexa at all
- Correcting or overriding Navexa data when it's wrong or stale

The fix is an architectural change: make the **data source** a first-class concept at both the portfolio and holding level, so `get_portfolio` remains a single unified interface regardless of where the data originated.

---

## Design Principle

**The consumer never cares where the data came from.**

`get_portfolio` returns the same shape whether the portfolio is fully Navexa-synced, fully manual, a one-time screenshot capture, or a hybrid mix. The `source_type` fields are metadata — they inform the system how to sync, refresh, and reconcile, but they don't change the API contract.

---

## Data Model Changes

### Portfolio Level

Add `source_type` to the portfolio entity. This determines the *default* sync behaviour for the portfolio as a whole.

```
Portfolio
├── id                  string
├── name                string
├── source_type         string    ← NEW
│   ├── "navexa"                  fully synced from Navexa (existing behaviour)
│   ├── "manual"                  all holdings managed via MCP endpoints
│   ├── "snapshot"                point-in-time capture (screenshot / bulk import)
│   └── "hybrid"                  Navexa base + manual overrides per-ticker
├── navexa_id           string    (empty for non-Navexa portfolios)
├── currency            string
├── created_at          datetime
└── updated_at          datetime
```

### Holding Level

Each holding carries its own `source_type`, independent of the portfolio's type. A hybrid portfolio will have holdings with mixed source types.

```
Holding
├── ticker              string
├── exchange            string
├── name                string
├── source_type         string    ← NEW
│   ├── "navexa"                  synced from Navexa
│   ├── "manual"                  entered via add_trade
│   ├── "snapshot"                from screenshot OCR / bulk import
│   └── "csv"                     from broker CSV import
├── source_ref          string    ← NEW  e.g. "screenshot:2026-03-02", "csv:commsec_export_001"
├── units               float
├── avg_cost            float
├── current_price       float
├── market_value        float
├── cost_basis          float
├── ... (all existing position/return fields unchanged)
└── trades              []Trade   ← NEW (for manual/snapshot holdings)
```

### Trade Entity (New)

Trades are the atomic unit of portfolio change. For Navexa holdings, trades are fetched from Navexa (as they are today via `get_portfolio_stock`). For manual/snapshot/csv holdings, trades are stored locally in Vire.

```
Trade
├── id                  string    auto-generated, e.g. "tr_a1b2c3d4"
├── portfolio_name      string
├── ticker              string
├── action              string    "buy" or "sell"
├── units               float
├── price               float     per unit, excluding fees
├── fees                float     brokerage / commission
├── date                string    trade date (ISO 8601)
├── settle_date         string    settlement date (optional, typically T+2)
├── source_type         string    "manual", "snapshot", "csv"
├── source_ref          string    free-form provenance tag
├── notes               string
├── created_at          datetime
└── updated_at          datetime
```

**Position derivation:** For manual portfolios, Vire derives the holding's `units`, `avg_cost`, `cost_basis`, `realized_return`, and `unrealized_return` by replaying the trade list for that ticker. This is the same calculation Navexa does internally — Vire just owns it locally for non-Navexa holdings.

---

## Portfolio Assembly Logic

When `get_portfolio` is called, the assembly path depends on `source_type`:

```
get_portfolio(name)
│
├── source_type = "navexa"
│   └── Fetch from Navexa API (existing behaviour, no change)
│       Holdings tagged source_type: "navexa"
│
├── source_type = "manual"
│   └── Aggregate from local trades table
│       Group trades by ticker → derive position for each
│       Holdings tagged source_type: per-trade source
│
├── source_type = "snapshot"
│   └── Return stored snapshot positions directly
│       No trade-level derivation (positions are the source of truth)
│       Holdings tagged source_type: "snapshot"
│
└── source_type = "hybrid"
    ├── Fetch Navexa holdings (base layer)
    ├── Fetch local trades/positions (override layer)
    └── Merge: local takes precedence per-ticker
        If ticker exists locally → use local version
        If ticker only in Navexa → use Navexa version
        Holdings tagged with their actual source
```

The response shape is identical in all cases. `source_type` fields on the portfolio and holdings are additive — existing consumers that don't read them are unaffected.

---

## New MCP Endpoints

### Tier 1 — Core (Enables Screenshot Workflow)

#### `create_portfolio`

Create a new portfolio that is not sourced from Navexa.

```
Parameters:
  name            string    required    Portfolio name
  source_type     string    required    "manual" | "snapshot" | "hybrid"
  currency        string    optional    default "AUD"
  navexa_id       string    optional    only for "hybrid" (links to Navexa portfolio)

Returns:
  Portfolio object with id, name, source_type, created_at
```

#### `add_trade`

Record a single buy or sell transaction.

```
Parameters:
  portfolio_name  string    optional    uses default if not specified
  ticker          string    required    e.g. "BHP.AU"
  action          string    required    "buy" or "sell"
  units           float     required    number of shares/units
  price           float     required    price per unit (excluding fees)
  fees            float     optional    brokerage/commission, default 0
  date            string    required    trade date, ISO 8601
  settle_date     string    optional    settlement date
  source_type     string    optional    default "manual"
  source_ref      string    optional    provenance tag
  notes           string    optional

Returns:
  Trade object with generated id
  Updated holding summary for the ticker

Behaviour:
  - If ticker doesn't exist in portfolio, creates the holding
  - If ticker exists, updates units/avg_cost by replaying all trades
  - Sell validation: cannot sell more units than currently held
  - Auto-resolves ticker suffix (BHP → BHP.AU if portfolio is AUD)
```

#### `snapshot_portfolio`

Bulk-import positions from a screenshot or external source. This is the "paste my whole portfolio" endpoint.

```
Parameters:
  portfolio_name  string    optional
  positions       array     required    array of position objects:
    ├── ticker          string    required
    ├── name            string    optional    company name
    ├── units           float     required
    ├── avg_cost        float     required    average cost per unit
    ├── current_price   float     optional    price at time of snapshot
    ├── market_value    float     optional
    ├── fees_total      float     optional    cumulative brokerage
    └── notes           string    optional
  mode            string    optional    "replace" (default) or "merge"
  source_ref      string    optional    e.g. "commsec:2026-03-02"
  snapshot_date   string    optional    date of the snapshot, default today

Returns:
  Created/updated portfolio summary
  Count of positions added/updated/removed

Behaviour:
  - mode "replace": clears existing non-Navexa holdings, inserts new
  - mode "merge": updates matching tickers, adds new ones, leaves unmatched
  - Each position is stored as a holding with source_type "snapshot"
  - No trade derivation — positions ARE the source of truth
  - If portfolio doesn't exist, creates it with source_type "snapshot"
```

#### `list_trades`

Query trade history with filters.

```
Parameters:
  portfolio_name  string    optional
  ticker          string    optional    filter by specific stock
  action          string    optional    "buy" or "sell"
  date_from       string    optional    ISO 8601
  date_to         string    optional    ISO 8601
  source_type     string    optional    filter by source
  limit           int       optional    default 50, max 200
  offset          int       optional    for pagination

Returns:
  Array of Trade objects
  Total count (for pagination)
  Summary: total buys, total sells, net position per ticker
```

#### `remove_trade`

Delete a trade by ID. Recalculates the holding's position.

```
Parameters:
  id              string    required    trade ID
  portfolio_name  string    optional

Returns:
  Deleted trade object
  Updated holding summary (or holding removed if no trades remain)
```

#### `update_trade`

Amend a trade. Merge semantics — only provided fields are changed.

```
Parameters:
  id              string    required    trade ID
  portfolio_name  string    optional
  units           float     optional
  price           float     optional
  fees            float     optional
  date            string    optional
  notes           string    optional

Returns:
  Updated trade object
  Updated holding summary
```

### Tier 2 — Extended

#### `import_trades_csv`

Bulk import from broker CSV export. Vire recognises common broker formats.

```
Parameters:
  portfolio_name  string    optional
  csv_data        string    required    raw CSV content
  broker          string    optional    "commsec" | "stake" | "selfwealth" | "auto"
  date_from       string    optional    only import trades after this date
  dry_run         bool      optional    validate and return preview without committing

Returns:
  Array of parsed Trade objects
  Validation warnings (unrecognised tickers, date parsing issues)
  Import summary: count by action, total consideration
```

#### `reconcile_portfolio`

Compare holdings from two sources and flag discrepancies. Useful after a screenshot to verify against Navexa.

```
Parameters:
  portfolio_name  string    required
  compare_with    string    optional    "navexa" (default) or another portfolio name
  tolerance_pct   float     optional    ignore differences below this % (default 1%)

Returns:
  Array of discrepancy objects:
    ├── ticker
    ├── field           e.g. "units", "avg_cost", "market_value"
    ├── source_a_value
    ├── source_b_value
    ├── difference_pct
    └── suggested_action
```

#### `set_portfolio_source`

Change a portfolio's source_type. Enables migration paths like Navexa → hybrid → manual.

```
Parameters:
  portfolio_name  string    required
  source_type     string    required
  navexa_id       string    optional    required when setting to "hybrid"

Returns:
  Updated portfolio object
```

---

## Screenshot → Vire Workflow

### Conversation Flow

The LLM (Claude or ChatGPT) acts as the intermediary between the user's screenshot and Vire's MCP endpoints.

```
User: [pastes screenshot of CommSec portfolio]

LLM:  1. Vision model extracts structured data from image
      2. Identifies screenshot type (portfolio summary / trade confirmation / order history)
      3. Asks clarifying questions:
         - Which portfolio? (SMSF / Trading / create new?)
         - Full snapshot or partial update?
         - Replace existing positions or merge?
      4. Calls appropriate Vire endpoint:
         - Portfolio summary screenshot  →  snapshot_portfolio
         - Trade confirmation screenshot →  add_trade
         - Order history screenshot      →  multiple add_trade calls
      5. Confirms what was entered, shows discrepancies if any
```

### Data Points to Extract by Screenshot Type

#### Portfolio Summary (High Level)

Typically a table/list view from a broker app or website.

| Field | Required | Notes |
|-------|----------|-------|
| Ticker / stock code | Yes | Map to ASX code + .AU suffix |
| Company name | Preferred | Helps with ticker disambiguation |
| Units held | Yes | Core position data |
| Average cost | Yes | Per unit, for cost basis calculation |
| Current price | Preferred | For snapshot valuation |
| Market value | Preferred | Can derive from units × price |
| Gain/loss $ | Optional | Can derive from market value − cost basis |
| Gain/loss % | Optional | Redundant but useful for validation |
| Portfolio weight % | Optional | Can derive from market value / total |

This maps to → `snapshot_portfolio` with mode "replace".

#### Individual Stock Detail

A drill-down view for a single holding.

| Field | Required | Notes |
|-------|----------|-------|
| All portfolio summary fields | Yes | As above |
| Trade date(s) | Yes | For trade history reconstruction |
| Buy/sell action | Yes | Per trade |
| Price per unit at trade | Yes | Per trade |
| Units per trade | Yes | Per trade |
| Brokerage/fees | Preferred | Per trade |
| Total consideration | Optional | Can derive |
| Dividend history | Optional | Dates and amounts |
| Franking credits | Optional | Relevant for SMSF |
| DRP participation | Optional | Affects unit count |

This maps to → multiple `add_trade` calls for the trade history, plus dividend recording.

#### Trade Confirmation

A buy/sell confirmation screen or email.

| Field | Required | Notes |
|-------|----------|-------|
| Ticker | Yes | |
| Action (BUY/SELL) | Yes | |
| Date & time | Yes | Trade execution timestamp |
| Units | Yes | |
| Price per unit | Yes | Limit/market price achieved |
| Total consideration | Preferred | For validation against units × price |
| Brokerage | Yes | |
| Settlement date | Optional | Typically T+2 |
| Order reference | Optional | Stored in notes |

This maps to → single `add_trade` call.

#### Broker CSV Export

The most complete and structured source. Common formats:

| Broker | Format Notes |
|--------|-------------|
| CommSec | Date, Reference, Type, Details, Debit, Credit, Balance |
| Stake | Date, Side, Symbol, Quantity, Price, Fees, Total |
| SelfWealth | Trade Date, Settlement Date, Action, Code, Quantity, Price, Brokerage, Total |
| Interactive Brokers | Comprehensive multi-section CSV with trades, dividends, fees, corporate actions |

This maps to → `import_trades_csv` with broker auto-detection.

---

## Existing Endpoint Impact

### No Changes Required

These endpoints work identically regardless of source_type because they consume the unified portfolio/holding shape:

| Endpoint | Why No Change |
|----------|---------------|
| `get_portfolio` | Assembly logic is internal; response shape unchanged |
| `get_portfolio_stock` | Returns position data; source is transparent |
| `portfolio_compliance` | Runs signals on tickers regardless of source |
| `compute_indicators` | Pure market data, no portfolio dependency |
| `get_stock_data` | Market data lookup, source-agnostic |
| `get_quote` | Real-time price, no portfolio dependency |
| `get_portfolio_timeline` | Consumes daily values regardless of source |
| `get_portfolio_plan` | Plan items reference tickers, not holdings directly |
| `get_portfolio_watchlist` | Independent entity |
| All cash transaction endpoints | Cash ledger is already Vire-native |

### Minor Additions

| Endpoint | Change |
|----------|--------|
| `get_portfolio` response | Add `source_type` field to portfolio and each holding |
| `get_portfolio_stock` response | Add `source_type` and `source_ref` to holding; for manual holdings, return local trades instead of Navexa trades |
| `list_portfolios` response | Add `source_type` to each portfolio summary |
| `generate_report` | Support non-Navexa portfolios (skip Navexa sync step if source_type ≠ navexa/hybrid) |
| `get_summary` | Same as generate_report |

---

## Storage Considerations

### Trades Table

New table for locally-managed trades.

```
trades
├── id              string    PK    auto-generated "tr_" prefix
├── portfolio_name  string    FK
├── ticker          string    indexed
├── action          string    "buy" | "sell"
├── units           float
├── price           float
├── fees            float
├── date            string    indexed
├── settle_date     string
├── source_type     string    indexed
├── source_ref      string
├── notes           string
├── created_at      datetime
└── updated_at      datetime
```

Indexes: `(portfolio_name, ticker)`, `(portfolio_name, date)`, `(portfolio_name, source_type)`

### Snapshot Positions Table

For snapshot-type portfolios where positions (not trades) are the source of truth.

```
snapshot_positions
├── id              string    PK
├── portfolio_name  string    FK
├── snapshot_date   string    indexed
├── ticker          string
├── name            string
├── units           float
├── avg_cost        float
├── current_price   float
├── market_value    float
├── fees_total      float
├── source_ref      string
├── notes           string
├── created_at      datetime
└── updated_at      datetime
```

### Portfolio Table Update

Add `source_type` column to existing portfolio storage. Default to "navexa" for all existing portfolios (backward compatible).

---

## Position Derivation (Manual Portfolios)

For manual portfolios, holdings are derived by replaying trades. The calculation follows standard average cost basis:

```
For each ticker in portfolio:
  
  running_units = 0
  running_cost  = 0
  realized_pnl  = 0

  For each trade ordered by date:
    
    If trade.action == "buy":
      running_cost  += (trade.units × trade.price) + trade.fees
      running_units += trade.units

    If trade.action == "sell":
      avg_cost_at_sell = running_cost / running_units
      cost_of_sold     = avg_cost_at_sell × trade.units
      proceeds         = (trade.units × trade.price) - trade.fees
      realized_pnl    += proceeds - cost_of_sold
      running_cost    -= cost_of_sold
      running_units   -= trade.units

  holding.units       = running_units
  holding.avg_cost    = running_cost / running_units  (if units > 0)
  holding.cost_basis  = running_cost
  holding.realized    = realized_pnl
  holding.unrealized  = (current_price × running_units) - running_cost
```

This matches the existing `calculation_method: "average_cost"` used by Navexa-sourced portfolios, so returns are directly comparable.

---

## Hybrid Portfolio Merge Rules

For `source_type: "hybrid"`, the merge strategy is simple and predictable:

1. Fetch Navexa holdings as the base layer
2. Fetch local holdings (from trades or snapshots) as the override layer
3. For each ticker:
   - If ticker exists **only in Navexa** → use Navexa, tag `source_type: "navexa"`
   - If ticker exists **only locally** → use local, tag with its source
   - If ticker exists **in both** → use local version, tag with its source
4. Assemble portfolio totals from the merged set

This means a user can selectively override individual stocks by adding local trades for them, while leaving the rest to Navexa sync.

### Conflict Resolution

The "local wins" rule is intentional. Common scenarios:

| Scenario | Behaviour |
|----------|-----------|
| Navexa has stale data for BHP | User adds manual trades for BHP → local version used |
| User sold a stock but Navexa hasn't synced | User adds sell trade → position shows as closed |
| User wants to track a stock not in Navexa | User adds buy trade → appears in portfolio |
| User wants to revert to Navexa data | Delete local trades for that ticker → Navexa version restored |

---

## Migration Path

### Phase 1: Schema + Core Endpoints

1. Add `source_type` field to portfolio and holding models (default: "navexa")
2. Create trades table and snapshot_positions table
3. Implement `add_trade`, `remove_trade`, `update_trade`, `list_trades`
4. Implement `snapshot_portfolio`
5. Implement `create_portfolio`
6. Update `get_portfolio` assembly to check source_type and route accordingly
7. Update `get_portfolio_stock` to return local trades for non-Navexa holdings

Existing portfolios (SMSF, Strategy, Trading) remain `source_type: "navexa"` — zero impact on current behaviour.

### Phase 2: CSV Import + Reconciliation

1. Implement `import_trades_csv` with broker format detection
2. Implement `reconcile_portfolio` for cross-source comparison
3. Implement `set_portfolio_source` for migration between types

### Phase 3: Extended Sources

1. Direct broker API integrations (Stake, etc.) as additional source types
2. Automatic screenshot parsing pipeline (image → structured data → `snapshot_portfolio`)
3. Scheduled reconciliation jobs

---

## LLM Integration Notes

### What the LLM Already Handles

Claude and ChatGPT vision models can reliably extract tabular data from broker screenshots. The accuracy is high for standard broker UIs (CommSec, Stake, SelfWealth, NAB Trade). The LLM handles:

- OCR of stock codes, prices, unit counts from screenshots
- Disambiguation of ticker symbols (e.g. "BHP" → "BHP.AU")
- Identification of screenshot type (summary vs confirmation vs order history)
- Data validation (cross-checking units × price = market value)

### What the LLM Should Ask

When processing a screenshot, the LLM should gather:

1. **Portfolio target** — Which portfolio does this belong to?
2. **Capture type** — Full portfolio snapshot or individual trade?
3. **Import mode** — Replace all positions or merge with existing?
4. **Broker identification** — Helps with layout-specific parsing
5. **Ambiguity resolution** — "Is $42.10 the average cost or current price?"

### Prompt Engineering for Screenshot Extraction

The system prompt for the extraction step should specify the exact output schema matching the `snapshot_portfolio` or `add_trade` parameter shapes. This ensures the LLM output can be passed directly to the MCP endpoint with minimal transformation.

---

## Summary

| What | Status |
|------|--------|
| Portfolio source_type concept | **New** — schema addition |
| Holding source_type concept | **New** — schema addition |
| Trade entity | **New** — table + CRUD endpoints |
| `get_portfolio` response shape | **Unchanged** — additive fields only |
| Existing Navexa sync | **Unchanged** — default path preserved |
| All signal/compliance/market endpoints | **Unchanged** — source-agnostic |
| Cash ledger | **Unchanged** — already Vire-native |
| Watchlist | **Unchanged** — already Vire-native |

The core insight is that `source_type` is metadata that informs Vire's internal data assembly, not the external API contract. Everything downstream of `get_portfolio` — signals, compliance, reports, timelines — works identically because it consumes the same holding shape regardless of origin.
