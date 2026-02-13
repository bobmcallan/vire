# Vire Phase 1: Portfolio Compliance Engine Transformation

## Project Context

Vire is being repositioned from a "Portfolio / Stock Reviewer" to a **Portfolio Compliance Engine** — a rules-based MCP service that checks holdings against user-defined strategy rules. This is a legal and product requirement: the service must provide compliance status against user-authored rules, not financial advice or recommendations.

The core principle: **the user defines the rules, Vire computes compliance**. No output from Vire should constitute a recommendation, opinion, or suggestion to buy, sell, or hold any financial product.

Repository: `github.com/bobmcallan/vire`

---

## Scope — Language & Output Cleanup

This phase covers three areas:

1. MCP tool descriptions and metadata
2. Signal and review output language
3. README and public-facing text

No architectural changes. No new features. This is a language and classification refactor only.

---

## 1. MCP Tool Descriptions

Every MCP tool description is part of the legal surface area — LLMs read these descriptions and use them to frame responses to users. Advisory language in tool descriptions becomes advisory language in LLM output.

### Tools to rename and redescribe

| Current Tool | New Name | Current Description Problem | New Description Direction |
|---|---|---|---|
| `portfolio_review` | `portfolio_compliance` | "Returns buy/sell/hold recommendations" | "Evaluates holdings against user-defined strategy rules. Returns compliance status per holding." |
| `market_snipe` | `strategy_scanner` | "Find turnaround stock opportunities showing buy signals" | "Scan exchange for tickers matching user-defined entry criteria from portfolio strategy." |
| `stock_screen` | `stock_screen` (keep) | "Screen for quality-value stocks" — opinionated | "Filter stocks by quantitative metrics defined in user strategy: P/E, return %, earnings, sector." |
| `detect_signals` | `compute_indicators` | "Detect trading signals" — implies actionability | "Compute technical indicators for specified tickers. Returns raw indicator values." |

### Description language rules

- **Remove**: "buy signals", "sell signals", "opportunities", "recommendations", "actionable", "turnaround", "quality-value", "good upside potential", "oversold stocks with accumulation patterns"
- **Replace with**: "entry criteria met", "exit trigger active", "compliant", "non-compliant", "indicator values", "user-defined thresholds", "strategy filter match"
- **Never use**: "should", "recommend", "opportunity", "attractive", "undervalued", "overvalued"
- **Always frame as**: computation against user rules, not independent assessment

### Specific tool description rewrites

**portfolio_review → portfolio_compliance**
```
Old: "Review a portfolio for signals, overnight movement, and actionable 
      recommendations. Returns a comprehensive analysis of holdings with 
      buy/sell/hold recommendations."

New: "Check portfolio holdings against user-defined strategy rules. Returns 
      compliance status for each holding based on the strategy's entry criteria, 
      exit triggers, position sizing limits, and sector allocation rules. 
      Requires a strategy to be set via set_portfolio_strategy."
```

**market_snipe → strategy_scanner**
```
Old: "Find turnaround stock opportunities showing buy signals. Scans the market 
      for oversold stocks with accumulation patterns and good upside potential."

New: "Scan exchange for tickers that match the entry criteria defined in the 
      user's portfolio strategy. Filters by strategy rules including indicator 
      thresholds, sector preferences, and company filters. Returns matching 
      tickers with their computed indicator values."
```

**stock_screen**
```
Old: "Screen for quality-value stocks with low P/E, positive earnings, consistent 
      quarterly returns (10%+ annualised), bullish price trajectory, and credible 
      news support."

New: "Filter stocks by quantitative criteria from the user's portfolio strategy. 
      Default filters: P/E ratio, earnings, quarterly return percentage, price 
      trend direction. All thresholds are configurable via the strategy's 
      company_filter settings."
```

**detect_signals → compute_indicators**
```
Old: "Detect and compute trading signals for specified tickers. Returns technical 
      indicators, trend classification, and risk flags."

New: "Compute technical indicators for specified tickers. Returns raw indicator 
      values: SMA, RSI, MACD, volume metrics, PBAS, VLI, regime classification, 
      and trend direction. Indicator interpretation is determined by the user's 
      strategy rules."
```

---

## 2. Output Classification Language

### portfolio_review / portfolio_compliance output

Replace the recommendation classification system:

| Current Output | New Output |
|---|---|
| `BUY` | `ENTRY CRITERIA MET` |
| `SELL` | `EXIT TRIGGER ACTIVE` |
| `HOLD` | `COMPLIANT` |
| `STRONG BUY` | `MULTIPLE ENTRY CRITERIA MET` |
| `STRONG SELL` | `MULTIPLE EXIT TRIGGERS ACTIVE` |

Each holding's compliance output should reference the specific strategy rule being tested:

```
Current: "BUY — RSI oversold with accumulation pattern"
New:     "ENTRY CRITERIA MET — RSI (28) below strategy threshold (30). 
          Volume accumulation detected per strategy rule."
```

```
Current: "SELL — below SMA200, bearish momentum"  
New:     "EXIT TRIGGER ACTIVE — Price below SMA200 (strategy exit rule). 
          MACD bearish crossover detected."
```

```
Current: "HOLD — neutral signals"
New:     "COMPLIANT — all strategy rules within tolerance. No entry or 
          exit triggers active."
```

### Signal output language

When `detect_signals` / `compute_indicators` returns data, ensure the output is raw values with factual trend classification, not advisory interpretation:

| Current | New |
|---|---|
| "Bullish signal" | "Upward trend" |
| "Bearish signal" | "Downward trend" |
| "Buy signal detected" | "Entry threshold reached" |
| "Oversold — potential reversal" | "RSI: 28 (below 30 threshold)" |
| "Accumulation phase" | "Volume pattern: increasing on up days" |
| "Risk flag: high" | "Volatility: above strategy max_drawdown threshold" |

### market_snipe / strategy_scanner output

Current output frames results as "opportunities." New output frames them as "strategy matches":

```
Current: "Found 3 turnaround opportunities with buy signals"
New:     "3 tickers match strategy entry criteria"
```

Each result should show which strategy rules the ticker satisfies:

```
Current: "XYZ — oversold, accumulating, near support. Good upside potential."
New:     "XYZ — matches 3/5 entry criteria: RSI below threshold (✓), 
          volume pattern match (✓), price near support level (✓), 
          sector: Technology (✓ allowed), P/E: 14.2 (✓ below max 20)"
```

### stock_screen output

Remove any quality judgments. Present as filter results:

```
Current: "Quality-value stocks found: these companies show strong fundamentals"
New:     "5 tickers pass strategy filters"
```

---

## 3. README and Public-Facing Text

### README.md

```
Current: "Portfolio / Stock reviewer"
New:     "Portfolio Compliance Engine — rules-based MCP service for checking 
          holdings against user-defined investment strategy rules"
```

Update the GitHub repository description to match.

### Add disclaimer to README

Add a section at the top of the README:

```markdown
## Important Notice

Vire is a portfolio compliance computation tool. It evaluates holdings and 
market data against user-defined strategy rules and returns compliance status. 

Vire does not provide financial advice, recommendations, or opinions on any 
financial product. All strategy rules, entry criteria, and exit triggers are 
defined by the user. Outputs reflect computation against those user-defined 
rules only.

Users should seek independent professional advice before making investment 
decisions.
```

---

## 4. Implementation Checklist

Work through these files in order:

### MCP Tool Registration
- [ ] Find where MCP tools are registered (likely `cmd/vire-mcp/` or `internal/mcp/`)
- [ ] Rename `portfolio_review` → `portfolio_compliance`
- [ ] Rename `market_snipe` → `strategy_scanner`  
- [ ] Rename `detect_signals` → `compute_indicators`
- [ ] Update all tool descriptions per the rewrites above
- [ ] Ensure parameter descriptions don't contain advisory language

### Signal/Review Output
- [ ] Find the output formatting code (likely `internal/signals/` or `internal/review/`)
- [ ] Replace BUY/SELL/HOLD classification with ENTRY CRITERIA MET / EXIT TRIGGER ACTIVE / COMPLIANT
- [ ] Update signal descriptions to use factual language (upward/downward trend, not bullish/bearish signal)
- [ ] Ensure market_snipe/strategy_scanner output references specific strategy rules matched
- [ ] Ensure stock_screen output presents results as filter matches, not quality assessments

### Public Text
- [ ] Update README.md with new description and disclaimer
- [ ] Update GitHub repository description
- [ ] Update any comments in code that reference "recommendations" or "advice"

### Validation
- [ ] Run existing tests — renamed tools should not break functionality, only language
- [ ] Verify MCP tool registration works with new names
- [ ] Test portfolio_compliance output format with a real portfolio
- [ ] Test strategy_scanner output references user strategy rules
- [ ] Confirm no output contains: "buy", "sell", "hold", "recommend", "opportunity", "should", "attractive", "undervalued", "overvalued" as advisory language (these words are fine in factual context, e.g. "user's sell trigger" or "strategy buy criteria")

---

## 5. What NOT to Change

- **Do not** change the underlying signal computation logic — the maths stays the same
- **Do not** change the strategy template or strategy storage — these are already user-defined
- **Do not** add new features or tools — this is language cleanup only
- **Do not** change API endpoints, parameter names (except tool names listed above), or data structures beyond the classification labels
- **Do not** remove any computed indicators — all raw data (RSI values, SMA levels, MACD, etc.) should still be returned. Only the interpretive labels change
- **Do not** change `get_portfolio`, `get_summary`, `get_stock_data`, `list_portfolios`, `list_reports`, `generate_report`, `get_config`, `get_diagnostics`, `get_version` — these are data retrieval tools with no advisory language issues

---

## 6. Guiding Principle

If an ASIC auditor read every string in the codebase, would they find language that constitutes a recommendation or opinion intended to influence a financial decision? After this phase, the answer must be **no**. Every output must trace back to a rule the user defined, not a judgment Vire made.