# /stock-report - Detailed Stock/ETF Report

Generate and save a detailed report for a specific stock or ETF to a markdown file.

## Usage
```
/stock-report <ticker> [options]
```

**Arguments:**
- `ticker` (required) - Ticker symbol (e.g., BHP, ACDC, SEMI, NVDA.US)

**Options:**
- `--force` - Force data refresh before generating report

**Examples:**
- `/stock-report ACDC` - Generate ACDC report
- `/stock-report NVDA.US` - Generate NVDA report
- `/stock-report DFND --force` - Force refresh then generate DFND report

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

### Step 3: Get stock data

**Make exactly ONE call:**
```
Use: get_stock_data
Parameters:
  - ticker: {resolved_ticker}
  - include: [price, fundamentals, signals, news]
```

> **Note:** When news data exists, the report automatically includes a **News Intelligence** section with AI-powered analysis: overall sentiment, critical summary, key themes, impact assessment (week/month/year), and source credibility ratings. This is cached for 30 days.

> **Note:** The report also includes a **Company Filings Intelligence** section with AI-analyzed ASX announcements and financial filings. This includes financial health assessment, 10% annual growth assessment, key metrics, year-over-year trends, strategy notes, and risk/positive factors. Filings are cached for 30 days and the intelligence summary for 90 days.

### Step 4: Save report to file

Save the returned markdown to:
```
Path: ./reports/{YYYYMMDD}-{HHMM}-{ticker}.md
Example: ./reports/20260206-1230-CGS.AU.md
```

Create the `reports/` directory if it doesn't exist. Write the complete `get_stock_data` output as-is to the file.

### Step 5: Show timing stats

Output a brief summary to the user (do NOT include the report content):
```
Stock report generated for {ticker}
  File: ./reports/{filename}
  Time: {elapsed seconds}s
```

## Ticker Resolution

| Input | Resolved Ticker |
|-------|----------------|
| `CGS` | `CGS.AU` |
| `ACDC` | `ACDC.AU` |
| `BHP.AU` | `BHP.AU` |
| `NVDA.US` | `NVDA.US` |

If no exchange suffix is provided, append `.AU`.
