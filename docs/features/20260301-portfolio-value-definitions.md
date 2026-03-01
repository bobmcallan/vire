# Portfolio Value Field Definitions

## Context

Prior to this change, `total_value` in the portfolio response was computed as:
```
total_value = equity_holdings_value + total_cash
```

This double-counted capital already deployed into stocks. The cash ledger tracks cumulative contributions (total deposits minus total withdrawals), not just the uninvested portion. So adding the entire ledger balance to equity overstated the portfolio value.

## Correct Definitions

### `total_value_holdings`
**Definition**: Current market value of all equity holdings.
**Formula**: `sum(units × current_price)` for each holding
**Notes**: Unchanged from previous behavior.

### `total_cost`
**Definition**: Net capital deployed in equities from trade history (buy costs minus sell proceeds), FX-adjusted.
**Formula**: `sum(total_invested - total_proceeds)` for all holdings (open and closed)
**Notes**: Previously computed as `sum(avg_cost × units)` for open positions only — a meaningless cost basis number. Now derived from trade history for all holdings, including closed positions.

### `total_cash`
**Definition**: Sum of all cash account balances (trading, accumulate, term deposits, offset).
**Formula**: `sum(transactions.signed_amount)` across all accounts
**Notes**: This represents the FULL ledger balance, not just uninvested cash. Unchanged.

### `available_cash` (NEW)
**Definition**: Uninvested (available) cash: total cash ledger balance minus net capital locked in equities.
**Formula**: `total_cash - total_cost`
**Notes**: Can be negative when equity has appreciated beyond the ledger balance (e.g., unrealized gains make equity worth more than was invested, while the ledger balance reflects original contributions). When there is no cash ledger configured, `total_cash = 0` and `available_cash = -total_cost`.

### `total_value` (FIXED)
**Definition**: Portfolio value: equity holdings plus available (uninvested) cash.
**Formula**: `total_value_holdings + available_cash`
**Notes**: Previously was `total_value_holdings + total_cash`, which double-counted deployed capital.

### `capital_gain` (NEW)
**Definition**: Overall portfolio gain: total value minus net capital deployed (from cash flow performance).
**Formula**: `total_value - net_capital_deployed`
**Notes**: Only populated when cash flow transactions exist.

### `capital_gain_pct` (NEW)
**Definition**: Overall portfolio gain as a percentage of net capital deployed.
**Formula**: `(capital_gain / net_capital_deployed) × 100`
**Notes**: Only populated when cash flow transactions exist and `net_capital_deployed > 0`.

## Per-Holding `total_proceeds` (NEW)
**Definition**: Sum of all sell proceeds for this holding (units × price − fees).
**Notes**: Used together with `total_invested` to compute portfolio-level `total_cost`.

## Weight Calculation
Holdings weights are computed using the new `total_value` as denominator:
```
weight = holding.market_value / total_value × 100
```
When `available_cash < 0`, weights can exceed 100% per holding (this is mathematically correct when equity has appreciated beyond original capital). When `available_cash > 0`, weights sum to less than 100% (the cash portion is the remainder).

## Example (SMSF)

| Field | Value | Notes |
|---|---|---|
| `total_value_holdings` | $427,561 | Equity market value |
| `total_cash` | $477,985 | Full ledger balance |
| `total_cost` | $426,985 | Net equity capital from trades |
| `available_cash` | $51,000 | `477,985 - 426,985` |
| `total_value` | $478,561 | `427,561 + 51,000` |
| `capital_gain` | computed from perf | vs net capital deployed |
