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

### Step 5: Save Report

After the review is generated, save the output as a directory of markdown files:

```
Directory: /home/bobmc/development/vire/reports/{YYYYMMDD-HHMM}-{portfolioname}/
Files:
  summary.md        # Portfolio overview, holdings tables, balance, alerts
  {TICKER}.md       # One file per holding with full detail
```

Create the report directory (and `reports/` parent if needed). Use the current date/time and lowercase portfolio name. For example: `reports/20260205-1430-smsf/`.

#### summary.md

```markdown
# Portfolio Review: {NAME}

**Date:** {date}
**Total Value:** ${value}
**Total Cost:** ${cost}
**Total Gain:** ${gain} ({gain%})
**Day Change:** ${dayChange} ({dayChange%})

## Holdings

### Stocks

| Symbol | Weight | Avg Buy | Qty | Price | Value | Capital Gain % | Total Return | Total Return % | Action |
|--------|--------|---------|-----|-------|-------|----------------|--------------|----------------|--------|
| ... | ... | ... | ... | ... | ... | ... | ... | ... | ... |
| **Stocks Total** | | | | | **${subtotal}** | | **${return}** | **${return%}** | |

### ETFs

| Symbol | Weight | Avg Buy | Qty | Price | Value | Capital Gain % | Total Return | Total Return % | Action |
|--------|--------|---------|-----|-------|-------|----------------|--------------|----------------|--------|
| ... | ... | ... | ... | ... | ... | ... | ... | ... | ... |
| **ETFs Total** | | | | | **${subtotal}** | | **${return}** | **${return%}** | |

**Portfolio Total:** ${total} | **Total Return:** ${return} ({return%})

## Portfolio Balance

### Sector Allocation
{sector allocation table}

### Portfolio Style
{style table}

**Concentration Risk:** {level}
**Analysis:** {commentary}

## Summary

{AI-generated overview}

## Alerts & Recommendations

### Alerts
{alert list}

### Recommendations
{recommendation list}
```

Do NOT include ETF Details or Stock Fundamentals in summary.md — those go in per-ticker files.

#### {TICKER}.md — ETF template

```markdown
# {TICKER} - {Full Name}

**Action:** {BUY/SELL/HOLD/WATCH} | **Reason:** {brief rationale}

## About

{Fund description — objective and strategy}

## Position

| Metric | Value |
|--------|-------|
| Weight | {weight}% |
| Avg Buy | ${avgBuy} |
| Quantity | {qty} |
| Price | ${price} |
| Value | ${value} |
| Capital Gain | {capGain%} |
| Total Return | ${totalReturn} ({totalReturn%}) |

## Fund Metrics

| Metric | Value |
|--------|-------|
| Beta | {beta} |
| Expense Ratio | {expenseRatio}% |
| Management Style | {style} |

## Top Holdings

| Holding | Weight |
|---------|--------|
| ... | ...% |

## Sector Breakdown

| Sector | Weight |
|--------|--------|
| ... | ...% |

## Country Exposure

| Country | Weight |
|---------|--------|
| ... | ...% |

## Technical Signals

| Signal | Value | Status |
|--------|-------|--------|
| Trend | {trend} | {bullish/bearish/neutral} |
| SMA 20 | ${sma20} | {above/below} |
| SMA 50 | ${sma50} | {above/below} |
| SMA 200 | ${sma200} | {above/below} |
| RSI | {rsi} | {overbought/oversold/neutral} |
| MACD | {macd} | {signal} |
| Volume | {vol} | {normal/unusual Nx avg} |
| PBAS | {pbas} | {tight/wide} |
| VLI | {vli} | {status} |
| Regime | {regime} | {trending/mean-reverting/random} |
| Support | ${support} | |
| Resistance | ${resistance} | |

## Risk Flags

{list of risk flags, or "None" if clean}
```

#### {TICKER}.md — Stock template

```markdown
# {TICKER} - {Company Name}

**Action:** {BUY/SELL/HOLD/WATCH} | **Reason:** {brief rationale}

**Sector:** {sector} | **Industry:** {industry}

## About

{Company description}

## Position

| Metric | Value |
|--------|-------|
| Weight | {weight}% |
| Avg Buy | ${avgBuy} |
| Quantity | {qty} |
| Price | ${price} |
| Value | ${value} |
| Capital Gain | {capGain%} |
| Total Return | ${totalReturn} ({totalReturn%}) |

## Fundamentals

| Metric | Value |
|--------|-------|
| Market Cap | ${marketCap} |
| P/E Ratio | {pe} |
| P/B Ratio | {pb} |
| EPS | ${eps} |
| Dividend Yield | {divYield}% |
| Beta | {beta} |

## Technical Signals

| Signal | Value | Status |
|--------|-------|--------|
| Trend | {trend} | {bullish/bearish/neutral} |
| SMA 20 | ${sma20} | {above/below} |
| SMA 50 | ${sma50} | {above/below} |
| SMA 200 | ${sma200} | {above/below} |
| RSI | {rsi} | {overbought/oversold/neutral} |
| MACD | {macd} | {signal} |
| Volume | {vol} | {normal/unusual Nx avg} |
| PBAS | {pbas} | {tight/wide} |
| VLI | {vli} | {status} |
| Regime | {regime} | {trending/mean-reverting/random} |
| Support | ${support} | |
| Resistance | ${resistance} | |

## Risk Flags

{list of risk flags, or "None" if clean}
```

## Output Format Notes

- Omit table sections where data is unavailable (e.g., no sector breakdown for PMGOLD)
- Technical Signals: populate from the detect_signals response; leave blank or "N/A" if a signal was not computed
- Risk Flags: extract from the alerts in the portfolio_review response for this ticker
- The Action and Reason in per-ticker files should match the Action column in summary.md

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
