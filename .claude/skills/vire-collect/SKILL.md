# /collect - Data Collection

Manually trigger data collection for specific tickers or portfolios.

## Usage
```
/collect [tickers...] [options]
/collect --portfolio [name]
```

**Examples:**
- `/collect BHP.AU CBA.AU` - Collect data for specific tickers
- `/collect --portfolio SMSF` - Collect data for all portfolio holdings
- `/collect BHP.AU --news` - Include news articles

## Workflow

### Collect Specific Tickers
```
Use: collect_market_data
Parameters:
  - tickers: {list of tickers}
  - include_news: {true if --news flag}
```

### Collect Portfolio Holdings
```
Step 1: Use sync_portfolio to get holdings
Step 2: Extract tickers from holdings
Step 3: Use collect_market_data with extracted tickers
```

## Data Collected

For each ticker:
- **EOD Data**: 1 year of daily OHLCV data
- **Fundamentals**: P/E, P/B, Market Cap, EPS, Dividend Yield, Beta, Sector
- **News** (optional): Latest 10 news articles with sentiment

After collection, signals are automatically computed:
- Moving averages (SMA20, SMA50, SMA200)
- RSI, MACD, ATR
- Volume analysis
- PBAS, VLI, Regime classification

## Output

Confirmation of collected tickers:
```
# Market Data Collection Complete

**Tickers Collected:** 5

- ✅ BHP.AU
- ✅ CBA.AU
- ✅ CSL.AU
- ✅ WES.AU
- ✅ NAB.AU

Data is now available for analysis.
```

## Best Practices

1. Collect data before running portfolio reviews for freshest signals
2. Use `--news` when you need sentiment analysis
3. Collection respects API rate limits (EODHD: 10 req/sec)
4. Data is cached - subsequent requests use stored data unless stale
