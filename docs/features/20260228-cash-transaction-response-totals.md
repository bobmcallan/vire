# Cash Transaction Response: Server-Side Totals

**Date:** 2026-02-28
**Status:** Implemented
**Feedback:** fb_0ac33209, fb_f26501fd, fb_94f33577, fb_22513b54

## Overview

The `GET /api/portfolios/{name}/cash-transactions` endpoint must return server-computed summary totals alongside the transaction list. The portal currently calculates totals client-side from the returned transactions, which is incorrect — pagination means the client may only have a subset of transactions, producing wrong totals.

Additionally, the `direction` field should be removed from cash transactions. The amount sign (+/-) is the source of truth: positive = credit (money in), negative = debit (money out). The direction field is redundant.

## Current Response

```json
{
  "transactions": [
    {
      "id": "ct_abc123",
      "date": "2026-02-24",
      "type": "deposit",
      "direction": "credit",
      "amount": 1080.00,
      "account": "SMSF Trading",
      "description": "Weekly contribution"
    }
  ]
}
```

### Problems

1. **No summary totals** — the portal calculates Total Deposits, Total Withdrawals, and Net Cash Flow by filtering and summing the transaction array client-side. If the response is paginated or truncated, the totals are wrong.

2. **`direction` field is redundant** — amount sign already encodes credit/debit. Positive amount = money in, negative amount = money out. Two fields saying the same thing creates inconsistency risk (fb_94f33577).

3. **`type` field stale** — after the account-based ledger refactor (52f0709), the old type values (`deposit`, `withdrawal`, `transfer_in`, `transfer_out`) no longer match what the server stores. The portal's client-side filter (`credits.includes(t.type)`) matches nothing, so all transactions fall into "withdrawals" and Total Deposits shows $0 (fb_0ac33209).

## Required Response

```json
{
  "summary": {
    "total_credits": 477014.62,
    "total_debits": 618326.00,
    "net_cash_flow": -141311.38,
    "transaction_count": 47
  },
  "transactions": [
    {
      "id": "ct_abc123",
      "date": "2026-02-24",
      "amount": 1080.00,
      "category": "contribution",
      "account": "SMSF Trading",
      "description": "Weekly contribution"
    }
  ]
}
```

## Changes

### 1. Add `summary` object to response

Computed server-side across ALL transactions for the portfolio, regardless of any pagination or filtering applied to the transaction array.

| Field | Type | Description |
|-------|------|-------------|
| `total_credits` | float | Sum of all positive amounts (money in) |
| `total_debits` | float | Sum of all negative amounts (absolute value, shown as positive) |
| `net_cash_flow` | float | `total_credits - total_debits` (positive = net inflow) |
| `transaction_count` | int | Total number of transactions |

### 2. Remove `direction` field from transactions

- Drop the `direction` field from the transaction response object
- Amount sign is the source of truth: `+` = credit, `-` = debit
- The portal will use the amount sign to determine display colour (positive = green, negative = red)

### 3. Remove `type` field, use `category`

The old `type` values (`deposit`, `withdrawal`, `transfer_in`, `transfer_out`, `dividend`) are from the pre-refactor schema. Replace with `category` which reflects the account-based ledger model:

| Category | Description |
|----------|-------------|
| `contribution` | Capital contributed to the portfolio |
| `withdrawal` | Capital withdrawn from the portfolio |
| `transfer` | Movement between accounts within the portfolio |
| `dividend` | Dividend income received |
| `fee` | Fees charged |
| `other` | Uncategorised |

The `category` field is for labelling only — it does not affect the credit/debit classification. The amount sign determines that.

### 4. Add `account` field to transaction

Each transaction belongs to a named account. This field should already be present from the ledger refactor but confirm it is included in the response.

## Summary Calculation Rules

```
total_credits   = SUM(amount) WHERE amount > 0   -- all positive transactions
total_debits    = SUM(ABS(amount)) WHERE amount < 0   -- all negative transactions, shown positive
net_cash_flow   = total_credits - total_debits
```

Important: the summary must be computed from the **full dataset**, not from the paginated/filtered subset returned in `transactions`.

## Portal Impact

Once this is implemented, the portal will:

1. Read `data.summary.total_credits`, `data.summary.total_debits`, `data.summary.net_cash_flow` directly
2. Remove all client-side `filter/reduce` calculation
3. Use amount sign for row colouring instead of type-based classification
4. Display `category` in the TYPE column instead of the old `type`

## MCP Tool Impact

The `list_cash_transactions` MCP tool should also reflect these changes:
- Include `summary` in the tool response
- Remove `direction` from transaction objects in the response
- Use `category` instead of `type`

The `add_cash_transaction` MCP tool (fb_22513b54):
- Remove `direction` parameter — infer from amount sign
- Replace `type` parameter with `category`
- Keep `account` parameter

## Test Cases

### Summary Totals
- Response includes `summary` object with all four fields
- `total_credits` equals sum of all positive amounts across all transactions
- `total_debits` equals sum of absolute value of all negative amounts
- `net_cash_flow` equals `total_credits - total_debits`
- `transaction_count` equals total number of transactions
- Summary is computed from full dataset, not paginated subset

### Direction Removal
- Transaction objects do not contain `direction` field
- Positive amounts represent credits (money in)
- Negative amounts represent debits (money out)

### Category Field
- Transaction objects contain `category` instead of `type`
- Category values are one of: `contribution`, `withdrawal`, `transfer`, `dividend`, `fee`, `other`

### add_cash_transaction
- Accepts amount with sign (positive = credit, negative = debit)
- Does not require or accept `direction` parameter
- Accepts `category` instead of `type`
