# Vire MCP Feedback Channel — Requirements

## Overview

MCP clients (Claude desktop, Claude CLI, agents) interact with vire during live sessions and are well-positioned to observe data quality issues, calculation anomalies, sync delays, and behavioural inconsistencies that a human user may not notice. This document defines a lightweight feedback channel that allows MCP clients to report these observations back to vire in real time.

The feedback channel is not a bug tracker — it is a **structured observation stream** from the AI layer to the data layer, enabling vire to improve data quality, surface sync issues, and build a record of anomalies over time.

---

## Design Principles

1. **Non-blocking.** Feedback is fire-and-forget. Claude never waits for a response before continuing.
2. **Low friction.** A single tool call. No ceremony, no required fields beyond category and description.
3. **Structured but flexible.** A small set of typed categories with a free-form observation field.
4. **Actionable.** Each feedback item should be specific enough for a developer to investigate without additional context.
5. **Session-scoped.** Feedback is associated with the session in which it was generated, enabling replay and correlation.
6. **Not a chat.** Feedback flows one way — Claude → vire. Vire does not respond to feedback in the MCP channel.

---

## Feedback Categories

| Category | When to Use |
|----------|-------------|
| `data_anomaly` | A field value that appears incorrect, impossible, or inconsistent with other fields |
| `sync_delay` | Data appears stale — cached values not reflecting known recent events (trades, prices) |
| `calculation_error` | A computed field (return %, break-even, gain/loss) produces a result that doesn't reconcile |
| `missing_data` | An expected field is null or absent for a ticker/holding where it should exist |
| `schema_change` | A field name, type, or structure appears to have changed unexpectedly |
| `tool_error` | A tool call returned an error, timeout, or empty response |
| `observation` | General qualitative observation that doesn't fit another category |

---

## MCP Tool Definition

```json
{
  "name": "submit_feedback",
  "description": "Submit an observation or data quality issue to vire from the current MCP session. Use this when you detect anomalies, calculation errors, stale data, missing fields, or any other issue worth recording. Fire-and-forget — do not wait for a response.",
  "parameters": {
    "category": {
      "type": "string",
      "enum": ["data_anomaly", "sync_delay", "calculation_error", "missing_data", "schema_change", "tool_error", "observation"],
      "required": true
    },
    "description": {
      "type": "string",
      "description": "Plain English description of the observation. Be specific — include field names, values observed, values expected, and any relevant context.",
      "required": true
    },
    "ticker": {
      "type": "string",
      "description": "Ticker symbol if the feedback relates to a specific holding (e.g. 'GNP', 'SEMI.AU')",
      "required": false
    },
    "portfolio_name": {
      "type": "string",
      "description": "Portfolio name if relevant",
      "required": false
    },
    "tool_name": {
      "type": "string",
      "description": "The vire tool that produced the anomalous data (e.g. 'get_portfolio_stock', 'get_stock_data')",
      "required": false
    },
    "observed_value": {
      "type": "any",
      "description": "The actual value observed (field value, error message, response snippet)",
      "required": false
    },
    "expected_value": {
      "type": "any",
      "description": "What the value should have been, if known",
      "required": false
    },
    "severity": {
      "type": "string",
      "enum": ["low", "medium", "high"],
      "description": "Impact severity. High = incorrect data shown to user. Medium = degraded analysis quality. Low = cosmetic or minor.",
      "required": false,
      "default": "medium"
    }
  }
}
```

---

## API Endpoint

> **Implementation note:** vire-server uses `/api/feedback` (not `/api/v1/feedback`) to match the existing route pattern. The vire-portal REST API section below retains `/api/v1/` for portal-specific endpoints.

```
POST /api/feedback
Content-Type: application/json

{
  "session_id": "auto-injected by MCP gateway",
  "client_type": "claude-desktop | claude-cli | claude-api",
  "timestamp": "2026-02-23T10:00:00Z",
  "category": "calculation_error",
  "description": "GNP net_return_pct showing -19.45% immediately after a new trade was synced. Units field shows 5,594 but trades array contains 4 buys totalling 7,470 units. Return figures inconsistent with trade data — likely mid-sync state.",
  "ticker": "GNP",
  "portfolio_name": "SMSF",
  "tool_name": "get_portfolio_stock",
  "observed_value": { "units": 5594, "net_return_pct": -19.45 },
  "expected_value": { "units": 7470, "net_return_pct": 7.4 },
  "severity": "high"
}
```

### Response

Always `202 Accepted` — no payload. Claude does not process the response.

```json
{ "accepted": true, "feedback_id": "fb_20260223_abc123" }
```

---

## Real Session Examples

These are feedback items that would have been submitted during today's session:

### 1. GNP Mid-Sync State
```
category: calculation_error
severity: high
ticker: GNP
tool: get_portfolio_stock
description: After new trade sync, units field shows 5,594 but trades array
contains 4 entries totalling 7,470 units. net_return_pct -19.45% and
true_breakeven_price $9.25 are inconsistent with trade data. Portfolio
appears to be in a partial sync state — holdings count not yet updated
while trades array already reflects new trade.
observed: units=5594, net_return_pct=-19.45, breakeven=9.25
expected: units=7470, net_return_pct=~7.4, breakeven=~6.93
```

### 2. Cached Portfolio Zeros
```
category: data_anomaly
severity: high
tool: get_portfolio
description: Initial get_portfolio call returned all net_return,
realized_net_return, and unrealized_net_return fields as 0 for all
holdings. Force refresh resolved the issue. Cached response appears
to strip return fields entirely rather than serving stale values.
```

### 3. CBOE FX Gap
```
category: calculation_error
severity: medium
ticker: CBOE
tool: get_portfolio_stock
description: unrealized_net_return $2,828 does not match Navexa total
return of $3,266. Difference of $433 corresponds to FX currency gain
not captured in vire return calculation. Capital gain component
matches ($2,833 vs $2,833) but FX gain is absent from vire output.
```

### 4. SRG Return Inconsistency
```
category: data_anomaly
severity: medium
ticker: SRG
tool: get_portfolio_stock
description: net_return_pct showing +17.29% but unrealized position
is -4.5% on current price vs avg_cost. The positive net_return appears
to include realised gains from prior partial sells, but the
true_breakeven_price of $1.99 and capital_gain_pct of 446% suggest
incorrect base cost calculation.
```

---

## Database Table — `mcp_feedback`

Feedback is persisted in a dedicated table in the vire database. The table is append-only — items are never updated or deleted by the system, only by explicit admin action via the API.

### Schema

| Column | Type | Nullable | Description |
|--------|------|----------|-------------|
| `id` | UUID | No | Primary key, auto-generated |
| `session_id` | VARCHAR(128) | No | MCP session identifier, injected by vire-portal gateway |
| `client_type` | VARCHAR(32) | No | `claude-desktop`, `claude-cli`, `claude-api` |
| `category` | VARCHAR(32) | No | One of the seven feedback categories |
| `severity` | VARCHAR(8) | No | `low`, `medium`, `high` — defaults to `medium` |
| `description` | TEXT | No | Plain English observation from Claude |
| `ticker` | VARCHAR(16) | Yes | Ticker symbol if applicable |
| `portfolio_name` | VARCHAR(64) | Yes | Portfolio name if applicable |
| `tool_name` | VARCHAR(64) | Yes | Vire tool that produced the data |
| `observed_value` | JSONB | Yes | Raw observed value(s) as JSON |
| `expected_value` | JSONB | Yes | Expected value(s) as JSON |
| `status` | VARCHAR(16) | No | `new`, `acknowledged`, `resolved`, `dismissed` — defaults to `new` |
| `resolution_notes` | TEXT | Yes | Admin notes on resolution, populated via API |
| `created_at` | TIMESTAMPTZ | No | Server timestamp at receipt |
| `updated_at` | TIMESTAMPTZ | No | Last status change timestamp |

### Indexes

- Primary key on `id`
- Index on `session_id` — for session replay and correlation
- Index on `(category, severity, status)` — for filtered list queries
- Index on `ticker` — for per-holding feedback lookup
- Index on `created_at DESC` — for chronological listing

---

## REST API — vire-portal

All endpoints are under `/api/v1/feedback` and are served by vire-portal. Authentication follows the existing vire-portal auth model.

### Submit Feedback (MCP Gateway — internal)

Receives feedback from the MCP gateway on behalf of Claude. Not exposed publicly — internal vire-portal route only.

```
POST /api/v1/feedback
```

Request body matches the MCP tool parameters plus gateway-injected fields (`session_id`, `client_type`, `timestamp`). Returns `202 Accepted` with the generated `id`.

---

### List Feedback

Returns a paginated list of feedback items, with optional filters. Used by the vire-portal admin UI and the `get_diagnostics` MCP tool.

```
GET /api/v1/feedback
```

**Query parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `status` | string | Filter by status: `new`, `acknowledged`, `resolved`, `dismissed` |
| `severity` | string | Filter by severity: `low`, `medium`, `high` |
| `category` | string | Filter by category |
| `ticker` | string | Filter by ticker symbol |
| `portfolio_name` | string | Filter by portfolio |
| `session_id` | string | Filter by session — returns all feedback from a single session |
| `since` | ISO datetime | Items created after this time |
| `before` | ISO datetime | Items created before this time |
| `page` | int | Page number, 1-based (default: 1) |
| `per_page` | int | Items per page, max 100 (default: 20) |
| `sort` | string | `created_at_desc` (default), `created_at_asc`, `severity_desc` |

**Response:**

```
{
  "items": [ ...feedback objects... ],
  "total": 47,
  "page": 1,
  "per_page": 20,
  "pages": 3
}
```

---

### Get Single Feedback Item

```
GET /api/v1/feedback/:id
```

Returns the full feedback record including all fields.

---

### Update Feedback Status

Used by admin to triage and track resolution. The only mutable fields are `status` and `resolution_notes`.

```
PATCH /api/v1/feedback/:id
```

**Request body:**

| Field | Type | Description |
|-------|------|-------------|
| `status` | string | New status: `acknowledged`, `resolved`, `dismissed` |
| `resolution_notes` | string | Free-form notes on what was done |

**Response:** Updated feedback object with new `status`, `resolution_notes`, and `updated_at`.

---

### Bulk Update Status

Update status on multiple items at once — useful for mass-acknowledging a batch of related items.

```
PATCH /api/v1/feedback/bulk
```

**Request body:**

| Field | Type | Description |
|-------|------|-------------|
| `ids` | array of UUID | Items to update |
| `status` | string | New status for all items |
| `resolution_notes` | string | Optional shared note |

---

### Delete Feedback Item

Hard delete — for removing noise or test entries. Admin only.

```
DELETE /api/v1/feedback/:id
```

Returns `204 No Content`.

---

### Summary / Stats

Returns aggregate counts grouped by category, severity, and status. Used for the vire-portal dashboard widget.

```
GET /api/v1/feedback/summary
```

**Response:**

```
{
  "total": 47,
  "by_status": {
    "new": 12,
    "acknowledged": 18,
    "resolved": 14,
    "dismissed": 3
  },
  "by_severity": {
    "high": 8,
    "medium": 24,
    "low": 15
  },
  "by_category": {
    "calculation_error": 11,
    "sync_delay": 9,
    "data_anomaly": 14,
    "tool_error": 6,
    "missing_data": 3,
    "observation": 3,
    "schema_change": 1
  },
  "oldest_unresolved": "2026-02-23T10:00:00Z"
}
```

---

## Existing Diagnostics MCP Tool Extension

The `get_diagnostics` MCP tool is extended to optionally include recent feedback, allowing Claude to check for known issues during a session.

**New parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `include_feedback` | bool | Include recent feedback in diagnostics output |
| `feedback_since` | ISO datetime | Only include feedback after this time |
| `feedback_severity` | string | Minimum severity to include |
| `feedback_status` | string | Filter by status, e.g. `new` to see unresolved items |

---

## vire-portal UI — Feedback Page

A dedicated feedback page in vire-portal provides admin visibility and triage. Minimum requirements:

- Table view of all feedback items, sortable and filterable by all query parameters above
- Inline status update — change status and add resolution notes without leaving the table
- Bulk select and bulk status update
- Per-item detail view showing all fields including `observed_value` and `expected_value` JSON
- Summary stats widget on the main dashboard showing unresolved high-severity item count
- Session view — click a `session_id` to see all feedback from that session in chronological order, giving full context of what Claude observed during the session

---

## When Claude Should Submit Feedback

Claude should submit feedback **during the session** when it detects:

- A field value that contradicts another field in the same response
- A return % that cannot be reconciled with the trade history provided
- A tool returning an empty response or error
- Data that appears stale relative to known recent events (e.g. new trade just placed but position not updated)
- A field that was present in a previous call but absent in a subsequent call
- Any value the user directly identifies as incorrect

Claude should **not** submit feedback for:
- Market movements or price changes (not a data error)
- Strategic disagreements (not a data quality issue)
- Slow response times unless it resulted in an error or timeout

---

## Out of Scope (v1)

- Two-way feedback (vire responding to Claude in-session)
- Automated remediation triggered by feedback
- Export to external issue trackers (GitHub Issues, Jira)
- Feedback from non-MCP sources (web portal, Navexa webhooks)
