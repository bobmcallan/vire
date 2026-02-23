# CBOE Return Calculation — FX Currency Gain Missing

## Issue Summary

For USD-denominated holdings, vire is not capturing the FX (currency) component of total return. This results in an understated total return figure when compared to Navexa.

**CBOE Example:**

| Component | Navexa | Vire |
|-----------|--------|------|
| Capital Gain (AUD) | $2,832.66 | $2,828.00 ✅ |
| Currency Gain (AUD) | $433.08 | $0.00 ❌ |
| **Total Return** | **$3,265.74** | **$2,828.00** |
| **Return %** | **6.91%** | **5.94%** |

The capital gain is essentially correct. The FX gain is not being calculated or surfaced.

---

## Current CBOE Holding JSON

```json
{
  "ticker": "CBOE",
  "exchange": "NYSE",
  "name": "Cboe Global Markets, Inc. Common Stock",
  "units": 124,
  "avg_cost": 272.02717741935487,
  "current_price": 288.185,
  "market_value": 35734.94,
  "net_return": 0,
  "net_return_pct": 5.939782463623622,
  "weight": 13.463419569039168,
  "total_cost": 33731.37,
  "total_invested": 33731.37,
  "realized_net_return": 0,
  "unrealized_net_return": 0,
  "dividend_return": 0,
  "capital_gain_pct": 529.6945503694812,
  "net_return_pct_irr": 0,
  "net_return_pct_twrr": 0,
  "currency": "USD",
  "country": "US",
  "last_updated": "2026-02-22T08:50:08.817495085Z",
  "true_breakeven_price": 272.02717741935487
}
```

*(Note: above is the cached/simplified output where net_return fields were zeroed. The refreshed output below is the live data.)*

```json
{
  "ticker": "CBOE",
  "exchange": "NYSE",
  "name": "Cboe Global Markets, Inc. Common Stock",
  "units": 124,
  "avg_cost": 383.89384338040486,
  "current_price": 406.6963025684448,
  "market_value": 50430.34151848716,
  "net_return": 2827.5049393169625,
  "net_return_pct": 5.939782463623622,
  "weight": 13.463419569039168,
  "total_cost": 47602.836579170194,
  "total_invested": 47602.836579170194,
  "realized_net_return": 0,
  "unrealized_net_return": 2827.5049393169625,
  "dividend_return": 0,
  "capital_gain_pct": 495.3402212688434,
  "net_return_pct_irr": 495.3402212688434,
  "net_return_pct_twrr": 7.275536033353203,
  "currency": "USD",
  "original_currency": "USD",
  "last_updated": "2026-02-22T19:31:35.434486147Z",
  "true_breakeven_price": 383.89384338040486
}
```

---

## Root Cause

The `avg_cost` and `total_cost` fields are stored and returned in AUD (converted at the FX rate at time of purchase). The `current_price` and `market_value` are also converted to AUD at the **current** FX rate.

However, the `unrealized_net_return` is being calculated as:

```
unrealized_net_return = market_value - total_cost
= (units × current_price_USD × current_fx) - (units × avg_cost_USD × purchase_fx)
```

This correctly captures both capital gain and FX gain in aggregate, **but** the result is being further reduced somewhere, as vire reports $2,828 vs Navexa's $3,266 total. The $433 gap is the FX component that is either:

1. Being calculated at the wrong FX rate, or
2. Not being included in the `unrealized_net_return` at all

---

## Required Changes

### 1. Add FX Gain/Loss Fields to Holding

For any holding where `original_currency != "AUD"`, calculate and expose:

```
fx_gain_loss = units × avg_cost_usd × (current_fx - purchase_fx)
capital_gain_loss_aud = units × (current_price_usd - avg_cost_usd) × current_fx
total_return = capital_gain_loss_aud + fx_gain_loss
```

New fields to add to the holding struct:

| Field | Type | Description |
|-------|------|-------------|
| `fx_gain_loss` | float | AUD gain/loss attributable to FX movement |
| `capital_gain_loss_native` | float | Gain/loss in the native currency (USD) |
| `capital_gain_loss_aud` | float | Capital gain converted to AUD at current FX |
| `purchase_fx_rate` | float | FX rate at time of average purchase |
| `current_fx_rate` | float | Current FX rate used for conversion |

### 2. Fix `unrealized_net_return` to Include FX Component

The total `unrealized_net_return` should equal `capital_gain_loss_aud + fx_gain_loss`. Currently it appears to only reflect one component.

### 3. Update Portfolio-Level Totals

`total_unrealized_net_return` at the portfolio level should aggregate the corrected per-holding values, ensuring USD holdings contribute their full AUD return including FX.

### 4. Display in UI / Reports

Where holdings are displayed, surface the breakdown:
- Capital gain (native currency)
- FX gain (AUD)
- Total return (AUD)

This matches how Navexa presents the data and gives a clearer picture of return attribution.

---

## Verification

Once implemented, CBOE should reconcile as follows:

| | Expected |
|---|---|
| Capital Gain AUD | ~$2,833 |
| FX Gain AUD | ~$433 |
| Total Return AUD | ~$3,266 |
| Return % | ~6.91% |
