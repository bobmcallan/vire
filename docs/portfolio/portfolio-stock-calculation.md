# Vire — P&L Calculation Spec
**Feature:** True Net P&L, Break-even Price & Price Targets
**Status:** Implemented
**Date:** 2026-02-22
**Author:** Bob (via Claude)

---

## Background

During portfolio review it became clear that the existing return % calculations in Vire do not accurately answer the core trading question:

> *"If I sold today, what is my actual profit or loss on this stock?"*

The issue arises when a holding has been partially or fully closed and re-entered — the realised P&L from prior trades must be included to get a true net figure. Without this, stop-loss prices and profit targets anchored to the current open position's avg cost are misleading.

Both Vire and Navexa agree on the **dollar figure** — the disagreement is in the **percentage**, which is a function of what you divide by and how timing is weighted. For price target calculations, the % is largely irrelevant — what matters is the **break-even price** derived from the full trade history.

---

## Problem Statement

### Example: SKS Technologies (SKS.AU)

| Trade | Date | Units | Price | Value |
|-------|------|-------|-------|-------|
| Buy | 24/12/2025 | 4,925 | $4.0248 | $19,825 |
| Sell | 22/01/2026 | 1,333 | $3.7627 | -$5,013 |
| Sell | 27/01/2026 | 819 | $3.680 | -$3,011 |
| Sell | 29/01/2026 | 2,773 | $3.4508 | -$9,566 |
| Buy | 05/02/2026 | 2,511 | $3.980 | $9,997 |
| Buy | 13/02/2026 | 2,456 | $4.070 | $9,999 |

**Current state (at $4.71):**
- Realised P&L (Jan sells): **-$2,235.47**
- Unrealised P&L (open position): **+$3,398.87**
- **Net if sold today: +$1,163.40**

### Current Vire behaviour vs what's needed

| Metric | Current Vire | Required |
|--------|-------------|----------|
| Open position return | 5.82% (gain / open cost) | ✅ Keep — useful context |
| Total return % | 2.92% (net gain / total invested) | ✅ Keep — honest round-trip |
| **Break-even price** | $4.03 (avg cost of open units) | ❌ Wrong — ignores Jan losses |
| **True break-even** | **$4.48** (accounts for realised loss) | ✅ Required |
| **Price targets** | Based on $4.03 | ❌ Misleading |
| **Stop losses** | Based on $4.03 | ❌ Misleading |

---

## Proposed New Fields

### Per Holding (open positions only)

| Field | Formula | Example (SKS) |
|-------|---------|---------------|
| `net_pnl_if_sold_today` | `realized_gain_loss + unrealized_gain_loss` | +$1,163.40 |
| `net_return_pct` | `net_pnl_if_sold_today / total_invested` | +2.92% |
| `true_breakeven_price` | `(open_cost - realized_gain_loss) / units_held` | $4.48 |
| `price_target_15pct` | `true_breakeven_price × 1.15` | $5.15 |
| `stop_loss_5pct` | `true_breakeven_price × 0.95` | $4.26 |
| `stop_loss_10pct` | `true_breakeven_price × 0.90` | $4.03 |
| `stop_loss_15pct` | `true_breakeven_price × 0.85` | $3.81 |

> **Note:** All fields above return `null` when `units_held = 0` (closed positions).

---

## Break-even Price Formula — Detailed

```
true_breakeven_price = (total_cost_of_open_position - realized_gain_loss) / units_held
```

Where:
- `total_cost_of_open_position` = cost basis of currently held units only
- `realized_gain_loss` = net P&L from all prior closed trades on this holding (positive = prior profit, negative = prior loss)
- `units_held` = current open units

### How it handles all scenarios

| Prior trade outcome | Effect on break-even | Rationale |
|--------------------|---------------------|-----------|
| Prior **loss** (e.g. SKS Jan sells) | Break-even price **increases** | Must recover prior loss before net positive |
| Prior **profit** (e.g. took gains, re-entered) | Break-even price **decreases** | Prior profit offsets current position cost |
| No prior trades (simple hold) | Break-even = avg cost | Standard case, no change |
| Multiple cycles over time | All fold in via cumulative `realized_gain_loss` | Naturally handles complex histories |

---

## What This Does NOT Change

- The existing `gain_loss_pct` calculation (open position return) — keep as is
- The existing `total_return_pct` (net gain / total invested) — keep as is
- Navexa TWRR figures — keep as is, useful for portfolio performance reporting
- Closed positions — no price target fields needed, return `null`

---

## Data Availability

All required inputs are **already present in the Vire payload** as of 2026-02-22:

| Required input | Vire field | Status |
|---------------|-----------|--------|
| Open position cost | `total_cost` | ✅ Available |
| Realised P&L | `realized_gain_loss` | ✅ Available |
| Unrealised P&L | `unrealized_gain_loss` | ✅ Available |
| Total capital deployed | `total_invested` | ✅ Available |
| Units held | `units` | ✅ Available |

No new data from Navexa is required. All new fields are **server-side derived calculations**.

---

## Acceptance Criteria

- [x] `true_breakeven_price` is returned for all holdings where `units > 0`
- [x] `true_breakeven_price` returns `null` where `units = 0`
- [x] `net_pnl_if_sold_today` equals `realized_gain_loss + unrealized_gain_loss`
- [x] Price targets and stop losses are calculated from `true_breakeven_price`, not `avg_cost`
- [x] For a simple hold (no prior sells), `true_breakeven_price` equals `avg_cost`
- [x] Prior profits correctly **lower** the break-even price
- [x] Prior losses correctly **raise** the break-even price
- [x] SKS.AU `true_breakeven_price` = **$4.47** at current data (verified via unit test)

---

## Out of Scope

- User-configurable target % thresholds (future feature)
- Portfolio-level break-even aggregation (future feature)
- Navexa TWRR replacement (not needed — serves a different purpose)

---

*Spec derived from live portfolio analysis session, 2026-02-22*
