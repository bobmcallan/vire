# Docs Validation Checklist

## Task #8: Validate docs match implementation

### Phase 1: Verify Model Changes
**File**: `internal/models/portfolio.go`

- [ ] TimeSeriesPoint struct has new fields:
  - `CashBalance float64 json:"cash_balance,omitempty"`
  - `ExternalBalance float64 json:"external_balance,omitempty"`
  - `TotalCapital float64 json:"total_capital,omitempty"`
  - `NetDeployed float64 json:"net_deployed,omitempty"`
- [ ] GrowthDataPoint struct has same fields
- [ ] Portfolio struct has net flow fields:
  - `YesterdayNetFlow float64 json:"yesterday_net_flow,omitempty"`
  - `LastWeekNetFlow float64 json:"last_week_net_flow,omitempty"`

### Phase 2: Verify Service Documentation
**File**: `docs/architecture/services.md`

#### Portfolio Service Section
- [ ] **Capital Allocation Timeline** subsection exists
- [ ] Explains TimeSeriesPoint now includes:
  - cash_balance (running balance)
  - external_balance (accumulate, term deposits)
  - total_capital (value + cash + external)
  - net_deployed (cumulative deposits - withdrawals)
- [ ] **Net Flow Fields** subsection exists
- [ ] Explains Portfolio.YesterdayNetFlow and Portfolio.LastWeekNetFlow
- [ ] Explains computation: sum of (deposits + dividends - withdrawals) for each window
- [ ] **CashFlowService Integration** mentioned:
  - Cash flow ledger required for timeline computation
  - Non-breaking: fields omitted when no cash data
- [ ] **GetDailyGrowth** mentioned:
  - Accepts CashFlowLedger parameter
  - Merges date-sorted transactions in single pass
- [ ] **GetPortfolioIndicators** documented:
  - Loads cash flow ledger
  - Passes to GetDailyGrowth
- [ ] **populateHistoricalValues** documented:
  - Computes yesterday/last-week net flow windows
  - Non-fatal: handles missing cash data gracefully

### Phase 3: Verify Tool Descriptions
**File**: `internal/server/catalog.go`

#### get_portfolio (line 254)
- [ ] Description mentions new fields:
  - `yesterday_net_flow` (net cash yesterday)
  - `last_week_net_flow` (net cash last 7 days)
- [ ] Description unchanged for existing fields

#### get_portfolio_indicators (line 362)
- [ ] Description mentions TimeSeries now includes:
  - `cash_balance`
  - `external_balance`
  - `total_capital`
  - `net_deployed`
- [ ] Description mentions single-pass computation with cash flow ledger

#### get_capital_performance (line 582)
- [ ] Description unchanged (not affected by this feature)

### Phase 4: Cross-Check Model ↔ Docs Alignment

**TimeSeriesPoint fields in docs**:
- [ ] cache_balance ✓
- [ ] external_balance ✓
- [ ] total_capital ✓
- [ ] net_deployed ✓

**Portfolio fields in docs**:
- [ ] yesterday_net_flow ✓
- [ ] last_week_net_flow ✓

### Phase 5: Verify No Stale References

- [ ] No references to removed field names
- [ ] No references to old computation methods (e.g., "derived from..." if changed)
- [ ] No contradictory statements about data sources (cash ledger vs other)
- [ ] No leftover TODO/FIXME comments in docs

### Phase 6: Consistency Checks

- [ ] Field names in docs match JSON tags (snake_case)
- [ ] Go field names in docs match struct definitions (CamelCase)
- [ ] Terminology consistent throughout:
  - "cash_balance" (not "cash")
  - "net_deployed" (not "net flow" without context)
  - "external_balance" (consistent with model)
  - "time series" vs "TimeSeries" used appropriately

## Pass/Fail Criteria

**PASS**: All 30+ checkboxes complete, no stale references, full alignment
**FAIL**: Any broken links, missing sections, contradictions, or misaligned field names

## Notes

- Task #8 is blocked by #7 (build), which is blocked by #6 (tests)
- Implementation in task #1 will be the source of truth
- All new fields use `omitempty` JSON tags (non-breaking change)
