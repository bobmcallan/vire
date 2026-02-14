# /stock-report - Detailed Stock/ETF Report

Generate and save a detailed report for a specific stock or ETF to a markdown file.

## Usage
```
/stock-report <ticker> [options]
```

**Arguments:**
- `ticker` (required) - Ticker symbol (e.g., BHP, ACDC, SEMI, NVDA.US)

**Options:**
- `--portfolio <name>` - Portfolio context (default: SMSF)
- `--force` - Force data refresh before generating report

**Examples:**
- `/stock-report ACDC` - Generate ACDC report with SMSF portfolio context
- `/stock-report NVDA.US` - Generate NVDA report (no portfolio context, market data only)
- `/stock-report DFND --force` - Force refresh then generate DFND report
- `/stock-report BHP --portfolio Personal` - Generate BHP report with Personal portfolio context

## CRITICAL RULES

1. **NEVER call `portfolio_review`** — that is the FULL portfolio tool
2. **Do NOT output the report markdown to the screen** — save it to a file only
3. **Show timing stats** — report how long the generation took and where the file was saved

## Workflow

### Step 1: Note start time

Record the current time before making MCP calls.

### Step 2: Force refresh (if --force)

When `--force` is set, first collect fresh data:
```
Use: collect_market_data
Parameters:
  - tickers: [{resolved_ticker}]

Use: detect_signals
Parameters:
  - tickers: [{resolved_ticker}]
```

### Step 3: Get market data

Call `get_stock_data` for comprehensive market analysis:
```
Use: get_stock_data
Parameters:
  - ticker: {resolved_ticker}
  - include: [price, fundamentals, signals, news]
```

> **Note:** `get_stock_data` provides market data, fundamentals, technical signals, news intelligence, company releases (per-filing extracted financials), and company timeline.

> **Note:** When news data exists, the report automatically includes a **News Intelligence** section with AI-powered analysis: overall sentiment, critical summary, key themes, impact assessment (week/month/year), and source credibility ratings. This is cached for 30 days.

> **Note:** The report includes a 3-layer stock assessment:
> - **Company Releases** — per-filing structured data extraction with actual numbers (revenue, profit, margins, contract values, guidance). Each filing is analysed individually by Gemini and cached permanently.
> - **Company Timeline** — structured yearly/quarterly financial history with period-by-period data, key events, and operational metrics (work-on-hand, repeat business rate). Rebuilt when new filings are analysed or every 7 days.
> - **Analyst Consensus** — rating, target price, buy/hold/sell counts from EODHD fundamentals (7-day cache).

### Step 4: Get portfolio position (if in portfolio)

If a portfolio is specified, call `get_portfolio_stock` to get position data:
```
Use: get_portfolio_stock
Parameters:
  - portfolio_name: {portfolio}  (default: "SMSF")
  - ticker: {base_ticker}        (e.g., "BHP" not "BHP.AU")
```

If `get_portfolio_stock` returns an error (ticker not in portfolio), skip this step and proceed with market data only.

> **Note:** `get_portfolio_stock` provides portfolio context: units, avg cost, market value, weight, capital gain, income return, total return, TWRR, and full trade history. This complements the market data from `get_stock_data`.

### Step 5: Save report to file

Combine the outputs into a single markdown file:
- Market data section (from `get_stock_data`)
- Portfolio position section (from `get_portfolio_stock`, if available)

Save the combined markdown to:
```
Path: ./reports/{YYYYMMDD}-{HHMM}-{ticker}.md
Example: ./reports/20260206-1230-CGS.AU.md
```

Create the `reports/` directory if it doesn't exist. Write the complete output as-is to the file.

### Step 6: Show timing stats

Output a brief summary to the user (do NOT include the report content):
```
Stock report generated for {ticker}
  File: ./reports/{filename}
  Time: {elapsed seconds}s
```

## Ticker Resolution

| Input | Resolved Ticker | Base Ticker (for get_portfolio_stock) |
|-------|----------------|---------------------------------------|
| `CGS` | `CGS.AU` | `CGS` |
| `ACDC` | `ACDC.AU` | `ACDC` |
| `BHP.AU` | `BHP.AU` | `BHP` |
| `NVDA.US` | `NVDA.US` | `NVDA` |

If no exchange suffix is provided, append `.AU`.

## Strategy Integration

When a portfolio strategy exists, `portfolio_compliance` includes strategy-aware context in the holding review:
- Action recommendations consider risk appetite thresholds
- Position size alerts flag holdings exceeding strategy limits
- The AI summary includes structured strategy context (risk level, return targets)
