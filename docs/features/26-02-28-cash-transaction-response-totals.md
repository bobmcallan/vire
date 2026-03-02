# Cash Transaction Response: Server-Side Totals

**Date:** 2026-02-28
**Status:** Implemented — Summary redesigned 2026-03-01 (fb_4e848a89)
**Feedback:** fb_0ac33209, fb_f26501fd, fb_94f33577, fb_22513b54, fb_4e848a89

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
  "accounts": [
    {"name": "Trading", "type": "trading", "is_transactional": true, "balance": 428004.88},
    {"name": "Stake Accumulate", "type": "accumulate", "is_transactional": false, "balance": 50000.00}
  ],
  "summary": {
    "total_cash": 478004.88,
    "transaction_count": 47,
    "by_category": {
      "contribution": 477014.62,
      "dividend": 1019.79,
      "transfer": 0.00,
      "fee": -29.53,
      "other": 0.00
    }
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

### 1. Per-account `balance` field on accounts

Each account object in the `accounts` array now includes a computed `balance` field.
Balance = sum of all signed transaction amounts for that account.

### 2. Redesigned `summary` object (replaces total_credits/total_debits/net_cash_flow)

The old summary (`total_credits`, `total_debits`, `net_cash_flow`) was misleading:
transfers were double-counted, inflating both totals. The new design:

| Field | Type | Description |
|-------|------|-------------|
| `total_cash` | float | Sum of all account balances (= `TotalCashBalance()`) |
| `transaction_count` | int | Total number of transactions |
| `by_category` | object | Net amount per category across all transactions |

`by_category` always contains all 5 keys: `contribution`, `dividend`, `transfer`, `fee`, `other`.
Each value is the sum of signed amounts for that category. Transfer pairs net to zero.

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
total_cash        = SUM(signed_amount) across ALL transactions (= sum of all account balances)
transaction_count = COUNT(transactions)
by_category[cat]  = SUM(signed_amount) WHERE category = cat
```

All 5 categories are always present in `by_category`, even if zero.
Transfer pairs net to zero: the credit and debit entries cancel out exactly.

Important: the summary is computed from the **full dataset**, not from any paginated/filtered subset.

## Portal Impact

Once this is implemented, the portal will:

1. Read `data.summary.total_cash` for net cash display
2. Read `data.summary.by_category.contribution` for total contributions
3. Read `data.summary.by_category.fee` for total fees
4. Read per-account balances from `data.accounts[*].balance`
5. Remove all client-side `filter/reduce` calculation
6. Use amount sign for row colouring instead of type-based classification
7. Display `category` in the TYPE column

## MCP Tool Impact

The `list_cash_transactions` MCP tool description updated to reflect:
- Per-account `balance` fields in the accounts array
- `summary.total_cash` and `summary.by_category` instead of old credit/debit fields

## Test Cases

### Per-account Balances
- Each account object in `accounts` has a `balance` field
- Balance equals sum of signed amounts for that account's transactions
- Zero balance is included (not omitted)

### Summary: total_cash
- `total_cash` equals sum of all account balances
- Paired transfers do not affect `total_cash` (they net to zero)

### Summary: by_category
- All 5 categories always present: `contribution`, `dividend`, `transfer`, `fee`, `other`
- Each value is the signed net for that category
- Transfer pairs net to zero in `by_category.transfer`
- `transaction_count` equals total number of transactions

### Backward compatibility
- Old `total_credits`, `total_debits`, `net_cash_flow` fields removed
- `direction` field absent from transactions (amount sign is source of truth)
- `category` field present (not old `type`)
