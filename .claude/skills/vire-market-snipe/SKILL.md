# /market-snipe - Market Snipe Buy Analysis

Find turnaround stock opportunities showing buy signals with good prospects for 10%+ gains.

## Usage
```
/market-snipe [exchange] [options]
```

**Examples:**
- `/market-snipe ASX` - Find top 3 snipe buys on ASX
- `/market-snipe ASX --limit 5` - Get top 5 candidates
- `/market-snipe US --sector Technology` - Filter by sector

## Prerequisites - Auto Build & Start

Before executing the workflow, ensure the MCP server is running with latest code:

### Step 0: Check and Rebuild Container
```bash
cd /home/bobmc/development/vire

# Check if source files are newer than last build
NEEDS_REBUILD=false
if [ ! -f docker/.last_build ]; then
    NEEDS_REBUILD=true
else
    # Check if any go files or go.mod changed since last build
    if find . -name "*.go" -newer docker/.last_build 2>/dev/null | grep -q . || \
       [ go.mod -nt docker/.last_build ] || [ go.sum -nt docker/.last_build ]; then
        NEEDS_REBUILD=true
    fi
fi

# Rebuild if needed
if [ "$NEEDS_REBUILD" = true ]; then
    echo "Code changes detected, rebuilding container..."
    docker compose -f docker/docker-compose.yml build
    touch docker/.last_build
    docker compose -f docker/docker-compose.yml up -d
else
    # Ensure container is running
    if ! docker compose -f docker/docker-compose.yml ps --status running | grep -q vire-mcp; then
        docker compose -f docker/docker-compose.yml up -d
    fi
fi

# Wait for health check
sleep 2
```

Run this bash script before proceeding with the MCP workflow steps.

## Workflow

Execute this workflow using the Vire MCP tools:

### Step 1: Scan Market
```
Use: market_snipe
Parameters:
  - exchange: {exchange code - AU for ASX, US for NYSE/NASDAQ}
  - limit: {specified limit or 3}
  - criteria: [oversold_rsi, near_support, underpriced, accumulating]
  - sector: {specified sector or null}
```

### Step 2: Validate Candidates (for each result)
```
Use: get_stock_data
Parameters:
  - ticker: {candidate ticker}
  - include: [price, fundamentals, signals, news]
```

### Step 3: Generate Analysis
```
Use: detect_signals
Parameters:
  - tickers: [top candidate tickers]
  - signal_types: [rsi, pbas, vli, regime, support_resistance]
```

## Scoring Criteria

Candidates are scored (0-100) based on:

| Criterion | Weight | Description |
|-----------|--------|-------------|
| Oversold RSI | 25% | RSI < 30 |
| Near Support | 20% | Price testing support level |
| PBAS Underpriced | 20% | Business momentum > price momentum |
| Volume Accumulation | 15% | VLI shows institutional buying |
| Regime Shift | 10% | Moving from distribution to accumulation |
| Near 52-Week Low | 10% | Within 10% of yearly low |

## Output Format

For each candidate:

1. **Ticker & Name** with score
2. **Price Table**: Current, Target, Upside %
3. **Bullish Signals** (reasons to buy)
4. **Risk Factors** (what to watch)
5. **AI Analysis** (2-3 sentence summary)

## Response Guidelines

When presenting snipe candidates:
- Emphasize the turnaround thesis for each stock
- Include specific entry and target prices
- Highlight the key catalyst expected to drive upside
- Note the main risk that could invalidate the thesis
- Suggest position sizing based on risk factors
- Never recommend more than the specified limit

## Risk Disclosure

Always include:
> These are speculative turnaround plays. Consider position sizing appropriately and use stop-losses. Past patterns don't guarantee future results.
