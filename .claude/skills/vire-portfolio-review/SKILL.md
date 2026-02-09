# /portfolio-review - Portfolio Review Workflow

Generate a portfolio review report and save it to a file.

## Usage
```
/portfolio-review [portfolio_name] [options]
```

**Options:**
- `--news` - Include news sentiment analysis
- `--noupdate` - Skip data refresh, use cached reports only (returns error if no cached report exists)
- `--force` - Force full regeneration bypassing all caches
- `--signals rsi,sma` - Focus on specific signals

**Examples:**
- `/portfolio-review SMSF` - Smart refresh and review
- `/portfolio-review SMSF --noupdate` - Use cached reports (fast, no API calls)
- `/portfolio-review SMSF --force` - Force full regeneration (ignores all caches)
- `/portfolio-review Personal --news` - Review with news analysis

## Prerequisites - Auto Build & Start

Before executing the workflow, ensure the MCP server is running with latest code:

### Step 0: Build and Start Container
```bash
cd /home/bobmc/development/vire && ./scripts/deploy.sh local
```

Run this bash script before proceeding with the MCP workflow steps.

## CRITICAL RULES

1. **Do NOT output the report markdown to the screen** — save it to a file only
2. **Show timing stats** — report how long the generation took, how many tickers, and where the file was saved

## Workflow

### Step 1: Note start time

Record the current time before making MCP calls.

### Step 2: Generate or Refresh Report

**Default (no flags):** Call `portfolio_review` via the Vire MCP. This auto-generates a fresh report if none exists or the cached report is stale (>1hr).

```
Use: portfolio_review
Parameters:
  - portfolio_name: {portfolio_name}
  - include_news: {true if --news flag, otherwise false}
```

**If `--force` IS set:** Call `generate_report` with `force_refresh=true` first, then `portfolio_review`.

```
Use: generate_report
Parameters:
  - portfolio_name: {portfolio_name}
  - force_refresh: true
  - include_news: {true if --news flag, otherwise false}

Use: portfolio_review
Parameters:
  - portfolio_name: {portfolio_name}
  - include_news: {true if --news flag, otherwise false}
```

**If `--noupdate` IS set:** Call `portfolio_review` directly. If no cached data exists, it will fetch fresh — but the existing smart caching means this will be fast if data was recently collected.

### Step 3: Save Report and Chart to Files

The `portfolio_review` MCP response returns multiple content blocks:
1. **TextContent[0]** — the main portfolio review markdown
2. **ImageContent** — a base64-encoded PNG growth chart (if available, for inline rendering)
3. **TextContent** with `<!-- CHART_FILE:/path/to/chart.png -->` — the saved chart file path (if available)
4. **TextContent** — the portfolio growth table markdown (if available)

Create the `reports/` directory if it doesn't exist.

**Save the chart PNG first** (if the response includes a `<!-- CHART_FILE:... -->` TextContent block):

Extract the file path from the `<!-- CHART_FILE:{path} -->` marker and copy the chart from the Docker container:
```bash
docker cp vire-mcp:{path} ./reports/{YYYYMMDD}-{HHMM}-{portfolio_name_lowercase}-growth.png
```
For example:
```bash
docker cp vire-mcp:/app/data/charts/smsf-growth.png ./reports/20260206-1158-smsf-growth.png
```

**Save the markdown report:**
```
Path: ./reports/{YYYYMMDD}-{HHMM}-{portfolio_name_lowercase}.md
Example: ./reports/20260206-1158-smsf.md
```

Concatenate all text content blocks (excluding the `<!-- CHART_FILE:... -->` marker block) into the `.md` file. After the growth table markdown, append a markdown image reference to the chart PNG so the chart is visible in the report:
```markdown
![Portfolio Growth]({basename}-growth.png)
```
For example, if the report file is `20260206-1158-smsf.md`, append:
```markdown
![Portfolio Growth](20260206-1158-smsf-growth.png)
```

Then append the timing footer (see Step 4).

### Step 4: Append timing footer and show stats

Append a timing footer to the end of the saved report file:
```markdown

---
*Generated in {elapsed seconds}s on {YYYY-MM-DD HH:MM}*
```

Then output a brief summary to the user (do NOT include the report content):
```
Portfolio review generated for {portfolio_name}
  File: ./reports/{filename}
  Tickers: {count} ({list of tickers})
  Time: {elapsed seconds}s
```

Extract the ticker count and list from the report content (look for the holdings tables).

## Smart Caching Behavior

The system uses per-component freshness TTLs to minimize unnecessary API calls:

| Data Type | TTL | Behavior |
|-----------|-----|----------|
| EOD bars (historical) | Immutable | Never re-fetched; only new bars after last stored date |
| Today's EOD bar | 1 hour | Incremental fetch appends to existing data |
| Fundamentals | 7 days | Quarterly data, rarely changes |
| News | 6 hours | Daily news cycle |
| Signals | 1 hour | Recomputed only when EOD data changes |
| Portfolio sync | 1 hour | Re-synced from Navexa when stale (or force=true) |
| Report | 1 hour | Auto-regenerated when stale |

- **Default workflow**: Serves cached report if <1hr old; auto-generates otherwise using smart per-component caching
- **`--force`**: Bypasses all TTLs, re-fetches everything from APIs
- **`--noupdate`**: Uses whatever data is cached, fast
- **Price cross-check**: `sync_portfolio` cross-checks Navexa prices against cached EODHD close prices. If EODHD has a bar from within the last 24 hours with a different price, it uses the EODHD close (Navexa can return stale prices, e.g. Friday's close on Monday). This requires EODHD data to be collected first (via `collect_market_data` or a prior `portfolio_review`)

## Output Format Reference

These templates document the stored report formats. The Go formatters generate this markdown automatically — do NOT manually construct reports.

### Summary Report

Contains: portfolio header, stocks table, ETFs table, portfolio balance (sector allocation, style, concentration risk), AI summary, alerts & recommendations, timing footer.

Note: Individual ETF details and stock fundamentals are NOT included in the portfolio review. Use `get_ticker_report` for per-ticker detail.

## Strategy Integration

If a portfolio strategy is set (via `/strategy` or `set_portfolio_strategy`), the review automatically:
- Adjusts RSI/SMA action thresholds based on risk appetite (conservative = tighter, aggressive = looser)
- Generates alerts when position sizes exceed strategy limits
- Adds strategy-specific recommendations (sector alignment, income targets)
- Includes structured strategy context in the AI summary prompt
- Updates `last_reviewed_at` on the strategy document

## Key Signals to Monitor

- **RSI Extremes**: >70 overbought (sell signal), <30 oversold (buy signal)
- **SMA Crossovers**: Golden cross (bullish), Death cross (bearish)
- **Volume Spikes**: Unusual volume indicating institutional activity
- **Support/Resistance Tests**: Price at key technical levels
