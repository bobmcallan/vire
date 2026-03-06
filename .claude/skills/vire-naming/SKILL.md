---
name: vire-naming
description: Vire financial field naming style guide and canonical glossary. Use this skill whenever naming, renaming, or defining fields in vire-server (Go structs, JSON keys, API responses) or vire-portal (display labels, tooltips, dashboard metrics). Triggers on any discussion of field names, glossary terms, API schema changes, or dashboard label design in the Vire system.
---

# Vire Field Naming Style Guide & Glossary

This document is the canonical reference for all field names across `vire-server` and `vire-portal`. Both implementations must derive labels, keys, and definitions from this guide. When in conflict, this guide wins.

---

## 1. Naming Convention

### Pattern
```
{domain}_{concept}_{qualifier}
```

All parts use `snake_case`. Fields are **fully qualified** — no abbreviations, no ambiguous shorthand.

### Rules

1. **Domain first** — the financial domain the field belongs to (`equity`, `capital`, `portfolio`, `income`, `indicator`, `holding`)
2. **Concept second** — the specific thing being measured (`holdings`, `contributions`, `return`, `balance`, `change`)
3. **Qualifier last** — narrows the concept (`gross`, `net`, `avg`, `pct`, `simple`, `xirr`, `day`, `week`, `month`)
4. **No redundant qualifiers** — don't repeat the domain in the concept (`capital_capital_gross` ✗)
5. **Percentages always suffix `_pct`** — never `_percent`, never `_rate` for a percentage
6. **Monetary amounts have no suffix** — `equity_holdings_return` is a dollar value; `equity_holdings_return_pct` is the percentage
7. **`_gross` vs `_net`** — gross = before deductions/adjustments; net = after
8. **`cash` is not a field prefix** — cash is a ledger/account concept only. At the portfolio level, uninvested money is `capital_available`

---

## 2. Domain Definitions

### `equity_*`
The asset class. Covers all held and closed equity positions (stocks, ETFs).

| Field | Type | Definition | Formula |
|---|---|---|---|
| `equity_holdings_value` | $ | Current market value of all open equity positions, FX-adjusted to base currency | `sum(units × price)` for all open holdings |
| `equity_holdings_cost` | $ | Net capital deployed into equities (buy costs minus sell proceeds) | `sum(gross_invested - gross_proceeds)` |
| `equity_holdings_return` | $ | Net return on all capital deployed into equities, including realised P&L from closed positions | `equity_holdings_value - equity_holdings_cost` |
| `equity_holdings_return_pct` | % | Equity return as a percentage of cost | `(equity_holdings_return / equity_holdings_cost) × 100` |
| `equity_holdings_realized` | $ | Cumulative profit/loss from fully or partially sold positions | `sum(realized_return)` for all holdings |
| `equity_holdings_unrealized` | $ | Paper profit/loss on remaining open positions | `sum(unrealized_return)` for open holdings |

> **D/W/M movement on `equity_holdings_return`**: Track change in `equity_holdings_value` over the period — not the cumulative return delta. This strips out realised P&L crystallisation events (selldowns) so the movement reflects price action only.

---

### `capital_*`
Money flow and deployment. Covers all capital entering, leaving, and sitting uninvested in the fund. Replaces all prior use of `cash_*` at the portfolio level.

| Field | Type | Definition | Formula |
|---|---|---|---|
| `capital_gross` | $ | Total capital held across all accounts (invested + uninvested) | `sum(all account balances)` |
| `capital_available` | $ | Uninvested capital — not deployed into equities | `capital_gross - equity_holdings_cost` |
| `capital_contributions_gross` | $ | Total capital deposited into the fund (contributions, transfers in, dividends received) | `sum(all credit transactions)` |
| `capital_contributions_net` | $ | Net capital contributed after withdrawals | `capital_contributions_gross - capital_withdrawals_gross` |
| `capital_withdrawals_gross` | $ | Total capital withdrawn from the fund | `sum(all debit transactions)` |
| `capital_return_simple_pct` | % | Simple (non-time-weighted) return on deployed capital | `(portfolio_value - capital_contributions_net) / capital_contributions_net × 100` |
| `capital_return_xirr_pct` | % | Time-weighted annualised return using XIRR, accounting for timing and size of each cash flow | `XIRR(cash_flows, current_value)` |

---

### `portfolio_*`
The aggregate whole. Combines equity positions and uninvested capital into fund-level metrics.

| Field | Type | Definition | Formula |
|---|---|---|---|
| `portfolio_value` | $ | Total fund value: equity holdings plus available capital | `equity_holdings_value + capital_available` |
| `portfolio_return` | $ | Overall fund gain: portfolio value minus net capital contributed | `portfolio_value - capital_contributions_net` |
| `portfolio_return_pct` | % | Overall fund return as a percentage of net capital contributed | `(portfolio_return / capital_contributions_net) × 100` |
| `portfolio_change_day` | % | Portfolio value change since previous close | `(portfolio_value - value_yesterday) / value_yesterday × 100` |
| `portfolio_change_week` | % | Portfolio value change over rolling 7 days | `(portfolio_value - value_7d_ago) / value_7d_ago × 100` |
| `portfolio_change_month` | % | Portfolio value change over rolling 30 days | `(portfolio_value - value_30d_ago) / value_30d_ago × 100` |

---

### `income_*`
Income streams received by or forecast for the fund.

| Field | Type | Definition |
|---|---|---|
| `income_dividends_received` | $ | Confirmed dividend income recorded in the capital ledger |
| `income_dividends_forecast` | $ | Forecasted future dividends based on holdings and declared amounts |

---

### `holding_*`
Per-holding metrics. Used in the holdings table and individual stock views.

| Field | Type | Definition | Formula |
|---|---|---|---|
| `holding_value_market` | $ | Current market value of this position | `units × current_price` |
| `holding_cost_avg` | $ | Average purchase price per unit including brokerage | `total_cost / units` |
| `holding_weight_pct` | % | Position as a proportion of total equity holdings value | `(holding_value_market / equity_holdings_value) × 100` |
| `holding_return_net` | $ | Unrealised gain/loss on this position | `holding_value_market - cost_basis` |
| `holding_return_net_pct` | % | Return as a percentage of total capital invested in this holding | `(holding_return_net / total_invested) × 100` |
| `holding_change_day` | % | Price change since previous close | — |
| `holding_change_week` | % | Price change over rolling 7 days | — |
| `holding_change_month` | % | Price change over rolling 30 days | — |
| `holding_trend` | string | One-line trend indicator from signal engine | e.g. `↑ Uptrend`, `↓ Strong downtrend`, `→ Consolidating` |

---

### `indicator_*`
Technical indicators computed on portfolio value or individual holdings.

| Field | Type | Definition |
|---|---|---|
| `indicator_ema_20` | $ | 20-day Exponential Moving Average — short-term trend |
| `indicator_ema_50` | $ | 50-day Exponential Moving Average — medium-term trend |
| `indicator_ema_200` | $ | 200-day Exponential Moving Average — long-term trend |
| `indicator_rsi` | 0–100 | Relative Strength Index. <30 oversold, >70 overbought |
| `indicator_trend` | string | Overall trend direction from EMA crossovers and RSI |

---

## 3. Display Labels (vire-portal)

Internal field names map to display labels as follows. Labels are title case, concise, and use financial terminology familiar to an SMSF investor.

| Field | Display Label |
|---|---|
| `equity_holdings_value` | Equity Value |
| `equity_holdings_cost` | Cost Basis |
| `equity_holdings_return` | Equity Return |
| `equity_holdings_return_pct` | Equity Return % |
| `equity_holdings_realized` | Realised P&L |
| `equity_holdings_unrealized` | Unrealised P&L |
| `capital_gross` | Gross Capital |
| `capital_available` | Available Capital |
| `capital_contributions_gross` | Contributions (Gross) |
| `capital_contributions_net` | Contributions (Net) |
| `capital_withdrawals_gross` | Withdrawals |
| `capital_return_simple_pct` | Simple Return % |
| `capital_return_xirr_pct` | XIRR Return % |
| `portfolio_value` | Portfolio Value |
| `portfolio_return` | Portfolio Return |
| `portfolio_return_pct` | Portfolio Return % |
| `portfolio_change_day` | D |
| `portfolio_change_week` | W |
| `portfolio_change_month` | M |
| `income_dividends_received` | Dividends Received |
| `income_dividends_forecast` | Dividends Forecast |
| `holding_value_market` | Value |
| `holding_cost_avg` | Avg Cost |
| `holding_weight_pct` | Weight |
| `holding_return_net` | Return |
| `holding_return_net_pct` | Return % |
| `holding_trend` | Trend |

---

## 4. Migration Map (Old → New)

For `vire-server` and `vire-portal` implementors. All legacy field names are deprecated.

| Legacy Field | Canonical Field |
|---|---|
| `equity_value` | `equity_holdings_value` |
| `net_equity_cost` | `equity_holdings_cost` |
| `net_equity_return` | `equity_holdings_return` |
| `net_equity_return_pct` | `equity_holdings_return_pct` |
| `realized_equity_return` | `equity_holdings_realized` |
| `unrealized_equity_return` | `equity_holdings_unrealized` |
| `gross_cash_balance` | `capital_gross` |
| `net_cash_balance` | `capital_available` |
| `gross_capital_deposited` | `capital_contributions_gross` |
| `net_capital_deployed` | `capital_contributions_net` |
| `gross_capital_withdrawn` | `capital_withdrawals_gross` |
| `simple_capital_return_pct` | `capital_return_simple_pct` |
| `annualized_capital_return_pct` | `capital_return_xirr_pct` |
| `net_capital_return` | `portfolio_return` |
| `net_capital_return_pct` | `portfolio_return_pct` |
| `yesterday_change` | `portfolio_change_day` |
| `last_week_change` | `portfolio_change_week` |
| `ledger_dividend_return` | `income_dividends_received` |
| `dividend_forecast` | `income_dividends_forecast` |
| `market_value` | `holding_value_market` |
| `avg_cost` | `holding_cost_avg` |
| `portfolio_weight_pct` | `holding_weight_pct` |
| `net_return` | `holding_return_net` |
| `net_return_pct` | `holding_return_net_pct` |

---

## 5. Anti-Patterns

Avoid these naming patterns in all new fields:

| Anti-Pattern | Reason | Correct Pattern |
|---|---|---|
| `cash_*` at portfolio level | Cash is a ledger term, not a portfolio domain | `capital_*` |
| `net_*` as a prefix | Net of what? Ambiguous without domain context | Qualify fully: `capital_contributions_net` |
| `gross_*` as a prefix | Same — domain must come first | `capital_contributions_gross` |
| `total_*` | Vague — total of what, over what period? | Use domain + concept |
| `*_return` without domain | Which return? Equity? Portfolio? Capital? | Always prefix with domain |
| Abbreviated qualifiers (`pct` is OK, but not `ret`, `val`) | Reduces readability | Spell out concepts, abbreviate only units |
