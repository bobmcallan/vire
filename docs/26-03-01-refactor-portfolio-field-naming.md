# Portfolio & Cash Field Naming Refactor

**Date:** 2026-03-01
**Status:** Proposal
**Scope:** All portfolio, holding, cash, and capital field names across models, services, handlers, MCP catalog, and glossary

---

## Problem Statement

The current field naming has evolved organically across multiple features (cash ledger, capital performance, FX conversion, timeseries). This has produced:

1. **Same name, different meaning** — `total_value` means "equity + available cash" in Portfolio but "equity only" in TimeSeriesPoint
2. **Different name, same concept** — `total_value`, `current_value`, `current_portfolio_value` all refer to portfolio value
3. **Ambiguous prefixes** — `total_` is overloaded: sometimes it means "sum of all" (total_cash), sometimes "net after sells" (total_cost), sometimes "gross before deductions" (total_invested)
4. **Level confusion** — `total_cost` means different things at portfolio vs holding level
5. **Missing semantic grouping** — cash fields, equity fields, and capital fields are interleaved with no clear hierarchy
6. **Context-dependent names** — fields like `net_return`, `available_cash`, `yesterday_total` require knowledge of which struct they belong to in order to understand them

---

## Naming Convention

Every field name must be **self-explanatory in isolation** — readable without knowing which struct, endpoint, or array it came from.

### Template

```
{qualifier}_{category}_{timescale}_{measure}_{suffix}
```

| Segment | Required | Values | Purpose |
|---|---|---|---|
| **qualifier** | When paired concept exists | `gross`, `net`, `realized`, `unrealized`, `annualized` | Signals what deductions/adjustments are applied |
| **category** | Always | `equity`, `cash`, `capital`, `portfolio` | Domain the field belongs to |
| **timescale** | When historical | `yesterday`, `last_week` | Time reference for snapshot values |
| **measure** | Always | `value`, `cost`, `balance`, `return`, `change`, `flow`, `deployed`, `weight` | What is being measured |
| **suffix** | When applicable | `pct`, `by_currency` | Unit or breakdown type |

### Rules

1. **One name per concept** — the same thing always has the same field name, everywhere
2. **Different names for different things** — if the calculation differs, the name must differ
3. **Gross/net pairing** — when both a pre-deduction and post-deduction value exist, both must carry their qualifier. Neither is "default"
4. **No `total_` prefix** — `total_` is banned. Use `gross_` (sum before deductions) or `net_` (sum after deductions) or omit when no paired concept exists
5. **No implied context** — field names do not rely on the containing struct/endpoint to convey meaning

---

## Current Field Inventory

### Portfolio-Level Fields (Portfolio struct → JSON)

| Current JSON Field | Current Meaning | Ambiguity |
|---|---|---|
| `total_value` | equity holdings + available cash | **HIGH** — name suggests "total of everything" but excludes non-transactional cash. Conflicts with TimeSeriesPoint where `total_value` means equity only |
| `total_value_holdings` | sum of all holding market values (equity only) | Verbose workaround; `_holdings` suffix added to disambiguate from `total_value` |
| `total_cost` | net capital in equities (buys - sells) | **HIGH** — "cost" implies what you paid. At holding level, `total_cost` = avg_cost * units. At portfolio level, it's buys minus sells. Different semantics, same name |
| `total_cash` | sum of all cash account balances | **HIGH** — includes capital already deployed in equities. Is this gross or net? Name gives no signal |
| `available_cash` | total_cash - total_cost | **HIGH** — "available" is vague. Doesn't signal it's a net figure derived from a gross. Standalone, "available cash" could mean anything |
| `total_net_return` | equity gain including dividends | **MEDIUM** — `total_` is redundant. Missing `equity_` scope: is this return on equities, cash, or the whole portfolio? |
| `total_net_return_pct` | return % vs total_cost | Same as above |
| `capital_gain` | total_value - net_capital_deployed | **MEDIUM** — "capital gain" has a specific tax meaning (realised gain on asset sale). This is actually net capital return |
| `capital_gain_pct` | capital_gain / net_capital_deployed * 100 | Same as above |
| `total_realized_net_return` | P&L from sold positions | **MEDIUM** — `total_` redundant, missing `equity_` scope |
| `total_unrealized_net_return` | P&L on open positions | Same |
| `yesterday_total` | portfolio value at yesterday's close | **MEDIUM** — "total" of what? Which value concept? Portfolio? Equity? |
| `yesterday_total_pct` | % change from yesterday | Missing `change` — is this a value or a change? |
| `last_week_total` | portfolio value at last week's close | Same as `yesterday_total` |
| `last_week_total_pct` | % change from last week | Same as `yesterday_total_pct` |
| `yesterday_net_flow` | net cash deposits - withdrawals yesterday | **LOW** — missing `cash_` category |
| `last_week_net_flow` | net cash deposits - withdrawals last week | Same |

### Holding-Level Fields (Holding struct → JSON)

| Current JSON Field | Current Meaning | Ambiguity |
|---|---|---|
| `market_value` | units * current_price | OK — standard financial term |
| `total_cost` | avg_cost * remaining units (remaining cost basis) | **HIGH** — at portfolio level this means buys - sells. Completely different semantics, same name |
| `total_invested` | sum of all buy costs + fees | **MEDIUM** — `total_` doesn't signal whether sells are deducted. This is gross (no sells deducted) |
| `total_proceeds` | sum of all sell proceeds | Same — `total_` doesn't signal this is gross proceeds |
| `net_return` | gain/loss on position | **LOW** — standalone, could be portfolio-level. But structurally nested so acceptable |
| `net_return_pct` | simple return % | OK |
| `capital_gain_pct` | XIRR annualised return (capital only, excl. dividends) | **HIGH** — at portfolio level this means `(value - deployed) / deployed %`. At holding level it's XIRR. Completely different calculation, same name |
| `net_return_pct_irr` | XIRR annualised return (incl. dividends) | **MEDIUM** — `_irr` suffix is jargon. Not self-explanatory |
| `net_return_pct_twrr` | Time-weighted return | **MEDIUM** — `_twrr` suffix is jargon |
| `realized_net_return` | P&L from sold portions | OK |
| `unrealized_net_return` | P&L on remaining position | OK |
| `dividend_return` | dividend income received | OK |
| `true_breakeven_price` | breakeven accounting for realized P&L | OK |
| `yesterday_close` | previous trading day close price | **LOW** — close of what? Price is implied but not stated |
| `yesterday_pct` | % change from yesterday | **LOW** — pct of what? Missing `change` and `price` |
| `last_week_close` | last Friday close price | Same as `yesterday_close` |
| `last_week_pct` | % change from last week | Same as `yesterday_pct` |

### TimeSeriesPoint Fields (timeline/indicators → JSON)

| Current JSON Field | Current Meaning | Ambiguity |
|---|---|---|
| `total_value` | equity holdings value only (NO cash) | **HIGH** — in Portfolio, `total_value` = equity + cash. In TimeSeriesPoint, `total_value` = equity only. Silently different |
| `total_cost` | cost basis of holdings | Consistent with Portfolio but inherits same naming problem |
| `net_return` | equity return (value - cost) | **MEDIUM** — missing `equity_` scope |
| `net_return_pct` | equity return percentage | Same |
| `total_cash` | running cash balance | **HIGH** — gross or net? Same ambiguity as Portfolio |
| `available_cash` | total_cash - total_cost | **HIGH** — same "available" vagueness |
| `total_capital` | total_value + total_cash (everything) | **HIGH** — different name than Portfolio's `total_value` for a similar concept |
| `net_capital_deployed` | cumulative deposits - withdrawals | OK |

### CapitalPerformance Fields (embedded in Portfolio → JSON)

| Current JSON Field | Current Meaning | Ambiguity |
|---|---|---|
| `total_deposited` | sum of positive contributions | **LOW** — `total_` redundant but unambiguous. This is gross deposits |
| `total_withdrawn` | sum of negative contributions (absolute) | Same |
| `net_capital_deployed` | deposited - withdrawn | OK |
| `current_portfolio_value` | equity holdings only (TotalValueHoldings) | **HIGH** — says "portfolio" but is equity only. Third name for this concept |
| `simple_return_pct` | (equity_value - deployed) / deployed * 100 | **MEDIUM** — missing scope. "Simple" contrasts with annualized but doesn't say what it's a return on |
| `annualized_return_pct` | XIRR from trade history | **MEDIUM** — same: return on what? |

### CashFlowSummary Fields (cash endpoints → JSON)

| Current JSON Field | Current Meaning | Ambiguity |
|---|---|---|
| `total_cash` | sum of all account balances | **HIGH** — gross or net? |
| `total_cash_by_currency` | balance per currency | **MEDIUM** — should follow gross naming |
| `by_category` | net amounts by category | OK |

### PortfolioIndicators Fields (indicators endpoint → JSON)

| Current JSON Field | Current Meaning | Ambiguity |
|---|---|---|
| `current_value` | current portfolio value | **HIGH** — third different name for portfolio value |

---

## Ambiguity Summary

### Critical (same name, different meaning across contexts)

| Field | Portfolio | Holding | TimeSeriesPoint | CapitalPerformance |
|---|---|---|---|---|
| `total_value` | equity + net cash | — | equity only | — |
| `total_cost` | net equity capital (buys - sells) | remaining cost basis (avg * units) | cost basis | — |
| `capital_gain_pct` | (value - deployed) / deployed % | XIRR annualised % | — | — |

### Critical (different name, same concept)

| Concept | Names Used |
|---|---|
| Equity holdings value (no cash) | `total_value_holdings` (Portfolio), `total_value` (TimeSeriesPoint), `current_portfolio_value` (CapitalPerformance) |
| Portfolio value | `total_value` (Portfolio), `total_capital` (TimeSeriesPoint), `current_value` (PortfolioIndicators) |
| Cash ledger balance | `total_cash` (Portfolio, CashFlowSummary), unnamed (no standalone field in some contexts) |

---

## Proposed Renames

### Guiding Principles

1. **Self-explanatory** — every field name is unambiguous when read in isolation, outside any struct or endpoint context
2. **Template-driven** — fields follow `{qualifier}_{category}_{timescale}_{measure}_{suffix}`
3. **One name per concept** — the same thing always has the same field name, everywhere
4. **Gross/net explicit** — when a paired concept exists, both sides carry their qualifier. Neither is "default"
5. **No `total_` prefix** — replaced with `gross_` or `net_` as appropriate, or omitted when no pair exists
6. **No backward compatibility** — single-phase rename with portal change advisory

### Portfolio Struct

| Current | Proposed | Template Breakdown |
|---|---|---|
| `total_value_holdings` | `equity_value` | `{equity}_{value}` — market value of all stock holdings |
| `total_value` | `portfolio_value` | `{portfolio}_{value}` — equity + net cash (the whole portfolio) |
| `total_cost` | `net_equity_cost` | `{net}_{equity}_{cost}` — buys minus sells. "Net" because sell proceeds are deducted |
| `total_cash` | `gross_cash_balance` | `{gross}_{cash}_{balance}` — raw ledger total. "Gross" because equity cost is NOT deducted |
| `available_cash` | `net_cash_balance` | `{net}_{cash}_{balance}` — after equity cost deducted. Paired with `gross_cash_balance` |
| `total_net_return` | `net_equity_return` | `{net}_{equity}_{return}` — P&L on equities including dividends |
| `total_net_return_pct` | `net_equity_return_pct` | + `{pct}` suffix |
| `capital_gain` | `net_capital_return` | `{net}_{capital}_{return}` — portfolio value minus capital deployed |
| `capital_gain_pct` | `net_capital_return_pct` | + `{pct}` suffix |
| `total_realized_net_return` | `realized_equity_return` | `{realized}_{equity}_{return}` — P&L from sold positions |
| `total_unrealized_net_return` | `unrealized_equity_return` | `{unrealized}_{equity}_{return}` — P&L on open positions |
| `yesterday_total` | `portfolio_yesterday_value` | `{portfolio}_{yesterday}_{value}` |
| `yesterday_total_pct` | `portfolio_yesterday_change_pct` | `{portfolio}_{yesterday}_{change}_{pct}` |
| `last_week_total` | `portfolio_last_week_value` | `{portfolio}_{last_week}_{value}` |
| `last_week_total_pct` | `portfolio_last_week_change_pct` | `{portfolio}_{last_week}_{change}_{pct}` |
| `yesterday_net_flow` | `net_cash_yesterday_flow` | `{net}_{cash}_{yesterday}_{flow}` — deposits minus withdrawals |
| `last_week_net_flow` | `net_cash_last_week_flow` | `{net}_{cash}_{last_week}_{flow}` |

### Holding Struct

| Current | Proposed | Template Breakdown |
|---|---|---|
| `market_value` | `market_value` | **Keep** — standard financial term, self-explanatory |
| `avg_cost` | `avg_cost` | **Keep** — self-explanatory |
| `current_price` | `current_price` | **Keep** |
| `total_cost` | `cost_basis` | Standard term for remaining position cost. Eliminates collision with portfolio `net_equity_cost` |
| `total_invested` | `gross_invested` | `{gross}_{invested}` — all buy costs, no sells deducted. "Gross" because `cost_basis` (net) exists alongside |
| `total_proceeds` | `gross_proceeds` | `{gross}_{proceeds}` — all sell revenue. Consistent with `gross_invested` |
| `net_return` | `net_return` | **Keep** — within a holding object, unambiguous |
| `net_return_pct` | `net_return_pct` | **Keep** |
| `capital_gain_pct` | `annualized_capital_return_pct` | `{annualized}_{capital}_{return}_{pct}` — XIRR excl. dividends. Self-explanatory, no collision with portfolio |
| `net_return_pct_irr` | `annualized_total_return_pct` | `{annualized}_{total}_{return}_{pct}` — XIRR incl. dividends |
| `net_return_pct_twrr` | `time_weighted_return_pct` | Self-explanatory, no jargon suffix |
| `realized_net_return` | `realized_return` | `{realized}_{return}` — P&L from sold portions |
| `unrealized_net_return` | `unrealized_return` | `{unrealized}_{return}` — P&L on remaining position |
| `dividend_return` | `dividend_return` | **Keep** |
| `true_breakeven_price` | `true_breakeven_price` | **Keep** |
| `yesterday_close` | `yesterday_close_price` | `{yesterday}_{close_price}` — explicitly a price |
| `yesterday_pct` | `yesterday_price_change_pct` | `{yesterday}_{price}_{change}_{pct}` |
| `last_week_close` | `last_week_close_price` | `{last_week}_{close_price}` |
| `last_week_pct` | `last_week_price_change_pct` | `{last_week}_{price}_{change}_{pct}` |
| `weight` | `portfolio_weight_pct` | `{portfolio}_{weight}_{pct}` — weight within what? The portfolio. Is it a percentage? Yes |

### TimeSeriesPoint

| Current | Proposed | Template Breakdown |
|---|---|---|
| `total_value` | `equity_value` | `{equity}_{value}` — consistent with Portfolio |
| `total_cost` | `net_equity_cost` | `{net}_{equity}_{cost}` — consistent with Portfolio |
| `net_return` | `net_equity_return` | `{net}_{equity}_{return}` — consistent with Portfolio |
| `net_return_pct` | `net_equity_return_pct` | + `{pct}` suffix |
| `total_cash` | `gross_cash_balance` | `{gross}_{cash}_{balance}` — consistent with Portfolio |
| `available_cash` | `net_cash_balance` | `{net}_{cash}_{balance}` — consistent with Portfolio |
| `total_capital` | `portfolio_value` | `{portfolio}_{value}` — consistent with Portfolio |
| `net_capital_deployed` | `net_capital_deployed` | **Keep** |
| `holding_count` | `holding_count` | **Keep** |

### CapitalPerformance

| Current | Proposed | Template Breakdown |
|---|---|---|
| `current_portfolio_value` | `equity_value` | `{equity}_{value}` — this field is TotalValueHoldings, not portfolio value. Name now matches |
| `total_deposited` | `gross_capital_deposited` | `{gross}_{capital}_{deposited}` — all deposits, no withdrawals deducted |
| `total_withdrawn` | `gross_capital_withdrawn` | `{gross}_{capital}_{withdrawn}` — all withdrawals |
| `net_capital_deployed` | `net_capital_deployed` | **Keep** — already follows convention |
| `simple_return_pct` | `simple_capital_return_pct` | `{simple}_{capital}_{return}_{pct}` — scope and methodology both explicit |
| `annualized_return_pct` | `annualized_capital_return_pct` | `{annualized}_{capital}_{return}_{pct}` — scope and methodology both explicit |
| `first_transaction_date` | `first_transaction_date` | **Keep** |
| `transaction_count` | `transaction_count` | **Keep** |

### CashFlowSummary

| Current | Proposed | Template Breakdown |
|---|---|---|
| `total_cash` | `gross_cash_balance` | `{gross}_{cash}_{balance}` — consistent everywhere |
| `total_cash_by_currency` | `gross_cash_balance_by_currency` | + `{by_currency}` suffix — explicit, no ambiguity |
| `by_category` | `net_cash_by_category` | `{net}_{cash}_{by_category}` — these are net amounts per category |

### PortfolioIndicators

| Current | Proposed | Template Breakdown |
|---|---|---|
| `current_value` | `portfolio_value` | `{portfolio}_{value}` — consistent with Portfolio |

### PortfolioReview / slimPortfolioReview

| Current | Proposed | Template Breakdown |
|---|---|---|
| `total_value` | `portfolio_value` | Consistent |
| `total_cost` | `net_equity_cost` | Consistent |
| `total_net_return` | `net_equity_return` | Consistent |
| `total_net_return_pct` | `net_equity_return_pct` | Consistent |
| `day_change` | `portfolio_day_change` | `{portfolio}_{day}_{change}` |
| `day_change_pct` | `portfolio_day_change_pct` | + `{pct}` suffix |

### GrowthDataPoint (internal, no JSON)

| Current | Proposed | Rationale |
|---|---|---|
| `TotalValue` | `EquityValue` | Consistent with all API layers |
| `TotalCost` | `NetEquityCost` | Consistent |
| `NetReturn` | `NetEquityReturn` | Consistent |
| `NetReturnPct` | `NetEquityReturnPct` | Consistent |
| `CashBalance` | `GrossCashBalance` | Consistent. Explicit pre-deduction |
| `TotalCapital` | `PortfolioValue` | Consistent |
| `NetDeployed` | `NetCapitalDeployed` | Full name, consistent |

### PortfolioSnapshot / SnapshotHolding (internal, no JSON)

| Current | Proposed | Rationale |
|---|---|---|
| `TotalValue` | `EquityValue` | Consistent |
| `TotalCost` | `NetEquityCost` | Consistent |
| `TotalNetReturn` | `NetEquityReturn` | Consistent |
| `TotalNetReturnPct` | `NetEquityReturnPct` | Consistent |

---

## Field Consistency After Refactor

Every concept has exactly one name, used everywhere it appears.

| Concept | Universal Name | Appears In |
|---|---|---|
| Market value of all equity holdings | `equity_value` | Portfolio, TimeSeriesPoint, CapitalPerformance, PortfolioReview |
| Total portfolio value (equity + net cash) | `portfolio_value` | Portfolio, TimeSeriesPoint, PortfolioIndicators, PortfolioReview |
| Net capital in equities (buys - sells) | `net_equity_cost` | Portfolio, TimeSeriesPoint, PortfolioReview |
| Remaining cost basis for a holding | `cost_basis` | Holding |
| Gross buy costs for a holding | `gross_invested` | Holding |
| Gross sell proceeds for a holding | `gross_proceeds` | Holding |
| Cash ledger balance (pre-deduction) | `gross_cash_balance` | Portfolio, TimeSeriesPoint, CashFlowSummary |
| Cash after equity cost deducted | `net_cash_balance` | Portfolio, TimeSeriesPoint |
| Net P&L on equities | `net_equity_return` | Portfolio, TimeSeriesPoint, PortfolioReview |
| Net P&L on equities (%) | `net_equity_return_pct` | Portfolio, TimeSeriesPoint, PortfolioReview |
| Portfolio value minus capital deployed | `net_capital_return` | Portfolio |
| Portfolio value minus capital deployed (%) | `net_capital_return_pct` | Portfolio |
| P&L from sold equity positions | `realized_equity_return` | Portfolio |
| P&L on open equity positions | `unrealized_equity_return` | Portfolio |
| Cumulative deposits - withdrawals | `net_capital_deployed` | CapitalPerformance, TimeSeriesPoint |
| All deposits (gross) | `gross_capital_deposited` | CapitalPerformance |
| All withdrawals (gross) | `gross_capital_withdrawn` | CapitalPerformance |
| Simple return on capital (%) | `simple_capital_return_pct` | CapitalPerformance |
| XIRR return on capital (%) | `annualized_capital_return_pct` | CapitalPerformance, Holding |
| XIRR total return incl. dividends (%) | `annualized_total_return_pct` | Holding |
| Time-weighted return (%) | `time_weighted_return_pct` | Holding |
| Holding weight in portfolio (%) | `portfolio_weight_pct` | Holding |

---

## Migration Strategy

Single-phase rename — no backward compatibility, no dual-write. Old field names are removed immediately.

### Steps

1. **Rename all Go struct fields and JSON tags** in models
2. **Update all services, handlers, and glossary** to use new names
3. **Update all unit and API tests** to new field names
4. **Update MCP catalog descriptions** to reference new names
5. **Build + test** to verify zero regressions
6. **Advise portal** on field changes (see change advisory below)

### Portal Change Advisory

The following JSON field names change in all API responses. The portal must update any field references before deploying this backend change.

**Portfolio response (`get_portfolio`):**

| Old Field | New Field |
|---|---|
| `total_value_holdings` | `equity_value` |
| `total_value` | `portfolio_value` |
| `total_cost` | `net_equity_cost` |
| `total_cash` | `gross_cash_balance` |
| `available_cash` | `net_cash_balance` |
| `total_net_return` | `net_equity_return` |
| `total_net_return_pct` | `net_equity_return_pct` |
| `capital_gain` | `net_capital_return` |
| `capital_gain_pct` | `net_capital_return_pct` |
| `total_realized_net_return` | `realized_equity_return` |
| `total_unrealized_net_return` | `unrealized_equity_return` |
| `yesterday_total` | `portfolio_yesterday_value` |
| `yesterday_total_pct` | `portfolio_yesterday_change_pct` |
| `last_week_total` | `portfolio_last_week_value` |
| `last_week_total_pct` | `portfolio_last_week_change_pct` |
| `yesterday_net_flow` | `net_cash_yesterday_flow` |
| `last_week_net_flow` | `net_cash_last_week_flow` |

**Holdings array (within `get_portfolio`):**

| Old Field | New Field |
|---|---|
| `total_cost` | `cost_basis` |
| `total_invested` | `gross_invested` |
| `total_proceeds` | `gross_proceeds` |
| `capital_gain_pct` | `annualized_capital_return_pct` |
| `net_return_pct_irr` | `annualized_total_return_pct` |
| `net_return_pct_twrr` | `time_weighted_return_pct` |
| `realized_net_return` | `realized_return` |
| `unrealized_net_return` | `unrealized_return` |
| `weight` | `portfolio_weight_pct` |
| `yesterday_close` | `yesterday_close_price` |
| `yesterday_pct` | `yesterday_price_change_pct` |
| `last_week_close` | `last_week_close_price` |
| `last_week_pct` | `last_week_price_change_pct` |

**Capital performance (embedded in `get_portfolio`):**

| Old Field | New Field |
|---|---|
| `current_portfolio_value` | `equity_value` |
| `total_deposited` | `gross_capital_deposited` |
| `total_withdrawn` | `gross_capital_withdrawn` |
| `simple_return_pct` | `simple_capital_return_pct` |
| `annualized_return_pct` | `annualized_capital_return_pct` |

**Timeline (`get_portfolio_timeline`) and indicators time_series (`get_portfolio_indicators`):**

| Old Field | New Field |
|---|---|
| `total_value` | `equity_value` |
| `total_cost` | `net_equity_cost` |
| `net_return` | `net_equity_return` |
| `net_return_pct` | `net_equity_return_pct` |
| `total_cash` | `gross_cash_balance` |
| `available_cash` | `net_cash_balance` |
| `total_capital` | `portfolio_value` |

**Portfolio indicators (`get_portfolio_indicators`):**

| Old Field | New Field |
|---|---|
| `current_value` | `portfolio_value` |

**Cash endpoints (`get_cash_summary`, `list_cash_transactions`):**

| Old Field | New Field |
|---|---|
| `total_cash` | `gross_cash_balance` |
| `total_cash_by_currency` | `gross_cash_balance_by_currency` |
| `by_category` | `net_cash_by_category` |

**Compliance/review (`portfolio_compliance`):**

| Old Field | New Field |
|---|---|
| `total_value` | `portfolio_value` |
| `total_cost` | `net_equity_cost` |
| `total_net_return` | `net_equity_return` |
| `total_net_return_pct` | `net_equity_return_pct` |
| `day_change` | `portfolio_day_change` |
| `day_change_pct` | `portfolio_day_change_pct` |

---

## MCP & API Endpoint Consolidation

The MCP catalog currently exposes **53 tools**. Several have overlapping data, duplicate response fields, or could be merged without losing functionality. Fewer tools means less ambiguity for LLM clients deciding which tool to call.

### Current Tool Inventory

| Group | Tools | Count |
|---|---|---|
| Portfolio data | `get_portfolio`, `get_portfolio_stock`, `portfolio_compliance`, `generate_report`, `get_summary`, `get_portfolio_indicators`, `get_portfolio_timeline`, `get_capital_performance` | 8 |
| Cash | `get_cash_summary`, `list_cash_transactions`, `set_cash_transactions`, `add_cash_transaction`, `add_cash_transfer`, `update_cash_transaction`, `remove_cash_transaction`, `clear_cash_transactions`, `update_account` | 9 |
| Watchlist | `get_portfolio_watchlist`, `set_portfolio_watchlist`, `add_watchlist_item`, `update_watchlist_item`, `remove_watchlist_item`, `review_watchlist` | 6 |
| Strategy | `get_portfolio_strategy`, `set_portfolio_strategy`, `delete_portfolio_strategy`, `get_strategy_template` | 4 |
| Plan | `get_portfolio_plan`, `set_portfolio_plan`, `add_plan_item`, `update_plan_item`, `remove_plan_item`, `check_plan_status` | 6 |
| Market data | `get_quote`, `get_stock_data`, `read_filing`, `compute_indicators` | 4 |
| Scanning | `market_scan`, `market_scan_fields`, `strategy_scanner`, `stock_screen` | 4 |
| System | `get_version`, `get_config`, `get_diagnostics`, `list_portfolios`, `set_default_portfolio`, `get_glossary`, `list_reports` | 7 |
| Feedback | `get_feedback`, `submit_feedback`, `update_feedback` | 3 |
| Admin | `list_users`, `update_user_role` | 2 |
| **Total** | | **53** |

### Overlap Analysis

#### 1. `get_portfolio_indicators` time_series duplicates `get_portfolio_timeline`

Both return the same `TimeSeriesPoint` array — identical struct, identical data source (`GetDailyGrowth`). The timeline is the primary data; the indicators are derived from it.

| | `get_portfolio_timeline` | `get_portfolio_indicators` |
|---|---|---|
| Purpose | Portfolio value over time (charting, P&L) | Overbought/oversold analysis |
| TimeSeriesPoint data | Yes | Yes (in `time_series` field) |
| RSI / EMA / trend | No | Yes |
| from/to date filtering | Yes | No |
| format downsampling | Yes (daily/weekly/monthly/auto) | No |
| API path | `/api/portfolios/{name}/history` | `/api/portfolios/{name}/indicators` |

**Action**: Keep `get_portfolio_timeline` as the primary timeline tool (renamed from `get_capital_timeline` — it shows portfolio value over time, not just capital). Remove the embedded `time_series` array from `get_portfolio_indicators` — it should return only the computed indicators (RSI, EMA, trend), not duplicate the raw data. Callers needing both call both endpoints. Removes duplicated data, clarifies each tool's purpose.

#### 2. `get_capital_performance` duplicates `get_portfolio` capital_performance

The standalone endpoint returns `CapitalPerformance` — the same struct already embedded in the `get_portfolio` response under `capital_performance`.

| | `get_capital_performance` | `get_portfolio` |
|---|---|---|
| CapitalPerformance data | Yes (entire response) | Yes (embedded `capital_performance` field) |
| Holdings | No | Yes |
| API path | `/api/portfolios/{name}/cash-transactions/performance` | `/api/portfolios/{name}` |

**Action**: Remove `get_capital_performance`. The data is already in `get_portfolio` which is also marked FAST. Removes 1 tool, 1 API route.

#### 3. `get_cash_summary` is a subset of `list_cash_transactions`

The cash summary (account balances, totals, category breakdown) is included in the `list_cash_transactions` response under the `summary` field. The summary endpoint exists as a lightweight alternative.

| | `get_cash_summary` | `list_cash_transactions` |
|---|---|---|
| Account balances | Yes | Yes |
| Summary (totals, by_category) | Yes | Yes |
| Transaction details | No | Yes |
| API path | `/api/portfolios/{name}/cash-summary` | `/api/portfolios/{name}/cash-transactions` |

**Action**: Remove `get_cash_summary`. Add `summary_only=true` query param to `list_cash_transactions` — when set, omits the transactions array and returns only accounts + summary. Same FAST performance, one fewer tool. Removes 1 tool, 1 API route.

#### 4. `strategy_scanner` and `stock_screen` are both stock screeners

Both screen for buy candidates. They differ only in filter type: fundamental vs technical. LLM clients must guess which to use.

| | `stock_screen` | `strategy_scanner` |
|---|---|---|
| Filter type | Fundamental (P/E, earnings, quarterly returns) | Technical (RSI, support, volume, regime) |
| Strategy-aware | Yes (loads portfolio strategy) | Yes (loads portfolio strategy) |
| Exchange filter | Yes | Yes |
| Sector filter | Yes | Yes |
| News option | Yes | Yes |
| API path | `/api/screen` | `/api/screen/snipe` |

**Action**: Merge into `screen_stocks`. Add `mode` param: `fundamental` (current stock_screen behavior) or `technical` (current strategy_scanner behavior). Keep the individual filter params (`max_pe`, `min_return`, `criteria`). Removes 1 tool, 1 API route.

#### 5. `market_scan` overlaps with screeners

`market_scan` is the flexible low-level scanner — arbitrary filters, arbitrary fields. The screeners (`stock_screen`, `strategy_scanner`) are opinionated presets. Three scanning tools confuse LLM clients.

**Action**: Keep `market_scan` + `market_scan_fields` as the power-user flexible scanner. Merge the two screeners (per #4 above). Result: 3 scanning tools → 3 tools (no reduction, but clearer separation: `market_scan` = raw, `screen_stocks` = curated).

#### 6. `generate_report` vs `get_summary` vs `portfolio_compliance`

These three all analyze the portfolio but at different levels:

| | `get_summary` | `portfolio_compliance` | `generate_report` |
|---|---|---|---|
| Speed | FAST (cached) | Medium (computes signals) | SLOW (full resync + signals) |
| Holdings data | No | Yes (with signals) | Yes (with signals) |
| AI summary | Yes | Yes (summary field) | Yes |
| Technical signals | No | Yes | Yes |
| Alerts/recommendations | No | Yes | Yes |
| Sector balance | No | Yes | Yes |
| Generates fresh report | No (reads cache) | No | Yes |

**Action**: Keep all three — they serve genuinely different use cases (quick glance, analysis, full refresh). No consolidation needed.

### Proposed Tool Consolidation Summary

| Action | Tools Affected | Net Change |
|---|---|---|
| Strip `time_series` from `get_portfolio_indicators` | Keep `get_portfolio_timeline` (renamed from `get_capital_timeline`), slim down indicators response | 0 (dedup, no tool change) |
| Remove `get_capital_performance` | Absorbed by `get_portfolio` | -1 |
| Remove `get_cash_summary` | Add `summary_only` param to `list_cash_transactions` | -1 |
| Merge `strategy_scanner` + `stock_screen` → `screen_stocks` | Remove both, add `screen_stocks` | -1 |
| **Total** | | **53 → 50** |

### API Route Changes

| Old Route | New Route / Change |
|---|---|
| `GET /api/portfolios/{name}/indicators` | **Keep** — remove `time_series` array from response (indicators only: RSI, EMA, trend) |
| `GET /api/portfolios/{name}/history` | **Rename** → `GET /api/portfolios/{name}/timeline` — canonical timeline endpoint (tool renamed to `get_portfolio_timeline`) |
| `GET /api/portfolios/{name}/cash-transactions/performance` | **Remove** — data in `GET /api/portfolios/{name}` response |
| `GET /api/portfolios/{name}/cash-summary` | **Remove** — use `GET /api/portfolios/{name}/cash-transactions?summary_only=true` |
| `POST /api/screen` | **Remove** — replaced by `POST /api/screen/stocks` |
| `POST /api/screen/snipe` | **Remove** — replaced by `POST /api/screen/stocks` |
| *(new)* `POST /api/screen/stocks` | Merged screener with `mode` param (fundamental/technical) |

### Portal Impact

Endpoints being removed must be updated in the portal before deployment:

- Any use of `time_series` from `/api/portfolios/{name}/indicators` → use `/api/portfolios/{name}/timeline` instead (the canonical timeline endpoint)
- Any calls to `/api/portfolios/{name}/cash-transactions/performance` → read `capital_performance` from `GET /api/portfolios/{name}` response
- Any calls to `/api/portfolios/{name}/cash-summary` → add `?summary_only=true` to `/api/portfolios/{name}/cash-transactions`
- Any calls to `/api/screen` or `/api/screen/snipe` → use `/api/screen/stocks` with `mode` param

---

## Files Affected

### Field Renames

| File | Changes |
|---|---|
| `internal/models/portfolio.go` | Rename struct fields and JSON tags for Portfolio, Holding, TimeSeriesPoint, GrowthDataPoint, PortfolioReview, PortfolioSnapshot, SnapshotHolding |
| `internal/models/cashflow.go` | Rename CapitalPerformance fields, CashFlowSummary fields |
| `internal/services/portfolio/service.go` | Update field assignments in SyncPortfolio |
| `internal/services/portfolio/indicators.go` | Update GrowthPointsToTimeSeries field mapping |
| `internal/services/portfolio/growth.go` | Update GrowthDataPoint construction |
| `internal/services/portfolio/history.go` | Update snapshot field references |
| `internal/services/cashflow/service.go` | Update CapitalPerformance construction, CashFlowSummary |
| `internal/server/handlers.go` | Update handler field references, slimPortfolioReview, capital return computation |
| `internal/server/glossary.go` | Update all term names, labels, formulas, and examples |
| `internal/server/catalog.go` | Update all MCP tool descriptions |
| `internal/services/portfolio/*_test.go` | Update all test assertions |
| `tests/api/*_test.go` | Update all API test JSON field references |

### Endpoint Consolidation

| File | Changes |
|---|---|
| `internal/server/handlers.go` | Remove `time_series` from indicators response. Remove cash-summary handler. Remove capital-performance handler. Merge stock_screen + strategy_scanner handlers into screen_stocks handler |
| `internal/server/routes.go` | Remove routes for `/cash-summary`, `/cash-transactions/performance`, `/screen`, `/screen/snipe`. Rename `/history` → `/timeline`. Add route for `/screen/stocks`. Add `summary_only` query param to cash-transactions route |
| `internal/server/catalog.go` | Rename `get_capital_timeline` → `get_portfolio_timeline`. Remove `get_capital_performance`, `get_cash_summary`, `stock_screen`, `strategy_scanner` tool definitions. Add `screen_stocks` tool definition. Remove `time_series` reference from `get_portfolio_indicators` description. Add summary_only param to `list_cash_transactions` |
| `internal/services/portfolio/indicators.go` | Remove time_series from PortfolioIndicators response |
| `tests/api/*_test.go` | Update API tests for removed endpoints, new endpoints, and merged params |

---

## Exclusions

**Navexa upstream structs** — `NavexaPortfolio`, `NavexaHolding`, `NavexaPerformance`, `NavexaTrade`, and the Navexa client internals (`performanceHolding`, `performanceTotalReturn`) are excluded from this refactor. These structs mirror the external Navexa API shape and renaming them would create a maintenance burden when reconciling against upstream changes. The boundary between Navexa fields and Vire fields is the `SyncPortfolio` function, which maps Navexa names to Vire names.
