# /portfolio-review - Portfolio Review Workflow

Review a stock portfolio for overnight movement, trading signals, and actionable recommendations.

## Usage
```
/portfolio-review [portfolio_name] [options]
```

**Examples:**
- `/portfolio-review SMSF` - Review the SMSF portfolio
- `/portfolio-review Personal --news` - Review with news analysis
- `/portfolio-review SMSF --signals rsi,sma` - Focus on specific signals

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

### Step 1: Sync Portfolio (if needed)
```
Use: sync_portfolio
Parameters:
  - portfolio_name: {portfolio_name}
  - force: false
```

### Step 2: Collect Market Data
```
Use: collect_market_data
Parameters:
  - tickers: [extracted from portfolio holdings]
  - include_news: {true if --news flag, otherwise false}
```

### Step 3: Detect Signals
```
Use: detect_signals
Parameters:
  - tickers: [extracted from portfolio holdings]
  - signal_types: {specified signals or all}
```

### Step 4: Generate Review
```
Use: portfolio_review
Parameters:
  - portfolio_name: {portfolio_name}
  - focus_signals: {specified signals or null}
  - include_news: {true if --news flag}
```

## Output Format

The review should include:

1. **Executive Summary** - AI-generated overview of portfolio status
2. **Alerts** - High-priority signals requiring attention
3. **Holdings Table** with:
   - Ticker, Price, Change%, Weight
   - Action (BUY/SELL/HOLD/WATCH)
   - Reason for action
4. **Recommendations** - Actionable next steps

## Key Signals to Monitor

- **RSI Extremes**: >70 overbought (sell signal), <30 oversold (buy signal)
- **SMA Crossovers**: Golden cross (bullish), Death cross (bearish)
- **50-Day SMA Test**: Price approaching 50-day moving average
- **Volume Spikes**: Unusual volume indicating institutional activity
- **Support/Resistance Tests**: Price at key technical levels

## Response Guidelines

When presenting the review:
- Lead with the most important alerts
- Highlight any SELL signals prominently
- Include specific price levels for actions
- Note any overnight moves >2%
- Flag commodity-sensitive stocks if commodities are falling
