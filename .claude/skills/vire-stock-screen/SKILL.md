# /stock-screen - Quality-Value Stock Screener

Find quality-value stocks with low P/E ratios, consistent returns, and credible news support. Unlike `/market-snipe` (which finds turnaround/oversold plays), this screen targets fundamentally strong companies with proven momentum.

## Usage
```
/stock-screen [exchange] [options]
```

**Examples:**
- `/stock-screen AU` - Screen ASX for quality-value stocks
- `/stock-screen US` - Screen US market
- `/stock-screen US --max-pe 15 --sector Technology` - Tighter criteria
- `/stock-screen AU --min-return 15` - Require higher quarterly returns

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

### Step 1: Screen Market
```
Use: stock_screen
Parameters:
  - exchange: {exchange code - AU for ASX, US for NYSE/NASDAQ}
  - limit: {specified limit or 5}
  - max_pe: {specified max P/E or 20}
  - min_return: {specified min quarterly return or 10}
  - sector: {specified sector or null}
```

### Step 2: Deep-Dive Candidates (for each top result)
```
Use: get_stock_data
Parameters:
  - ticker: {candidate ticker}
  - include: [price, fundamentals, signals, news]
```

### Step 3: Validate Signals
```
Use: detect_signals
Parameters:
  - tickers: [top candidate tickers]
  - signal_types: [rsi, pbas, vli, regime, trend, support_resistance, macd]
```

## Screening Criteria

Candidates must pass ALL hard filters:

| Filter | Requirement | Rationale |
|--------|------------|-----------|
| P/E Ratio | > 0 and <= max_pe (default 20) | Positive earnings, not overvalued |
| EPS | > 0 | Real profitability |
| Market Cap | >= $100M | Not penny stocks |
| Quarterly Returns | >= min_return% annualised, all 3 quarters | Consistent performance |

Then scored (0-100) on soft factors:

| Factor | Weight | Description |
|--------|--------|-------------|
| P/E Quality | 20% | Sweet spot 5-12, reasonable 12-18 |
| Quarterly Return Consistency | 25% | Higher average = higher score |
| Price Trajectory Alignment | 20% | Bullish trend confirms fundamentals |
| Not a Story Stock | 15% | Large cap, real earnings, dividends |
| News Quality | 10% | Credible sources, bullish sentiment |
| Technical Health | 10% | Healthy RSI, bullish MACD, accumulation |

## Output Format

For each candidate:

1. **Ticker & Name** with score and sector
2. **Key Metrics Table**: Price, P/E, EPS, Market Cap, Dividend Yield
3. **Quarterly Returns Table**: Last 3 quarters annualised
4. **Strengths** (why this is a quality-value pick)
5. **Concerns** (what to watch)
6. **AI Analysis** (3-4 sentence assessment)

## Response Guidelines

When presenting screen results:
- Emphasise the fundamental quality (low P/E + real earnings = genuine value)
- Show the consistency of returns across quarters (not a one-off spike)
- Explain how price trajectory confirms the financial outlook
- Note dividend income as a bonus, not the primary thesis
- Highlight that these are NOT speculative or turnaround plays
- Flag any concerns about news quality or volatility
- Suggest using `get_stock_data` for deeper analysis on top picks

## Strategy Integration

If a portfolio strategy is set, the stock screen automatically:
- Adjusts default P/E threshold by risk appetite (conservative=15, moderate=20, aggressive=25) when user doesn't specify `--max-pe`
- Filters out excluded sectors from the strategy (unless user explicitly passes `--sector`)
- Boosts dividend-paying stocks for conservative strategies (+5% score for >3% yield)
- Adds target returns and income requirements to the AI analysis prompt
- Appends a note to results: "*Filtered for your conservative smsf strategy*"

## Risk Disclosure

Always include:
> These results reflect historical screening criteria. Low P/E and past returns do not guarantee future performance. Always conduct independent due diligence before making investment decisions.
