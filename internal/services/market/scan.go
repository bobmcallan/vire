package market

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

// Scanner executes flexible market scan queries over cached data.
type Scanner struct {
	storage  interfaces.StorageManager
	logger   *common.Logger
	registry *ScanFieldRegistry
}

// NewScanner creates a new scanner.
func NewScanner(storage interfaces.StorageManager, logger *common.Logger) *Scanner {
	return &Scanner{
		storage:  storage,
		logger:   logger,
		registry: NewScanFieldRegistry(),
	}
}

// Fields returns the available scan field definitions.
func (sc *Scanner) Fields() *models.ScanFieldsResponse {
	return sc.registry.FieldsResponse()
}

// Scan executes a market scan query.
func (sc *Scanner) Scan(ctx context.Context, query models.ScanQuery) (*models.ScanResponse, error) {
	start := time.Now()

	// Validate
	if err := sc.validateQuery(&query); err != nil {
		return nil, err
	}

	// Get stock index entries
	entries, err := sc.storage.StockIndexStore().List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list stock index: %w", err)
	}

	// Filter by exchange
	var tickers []string
	for _, entry := range entries {
		if query.Exchange == "ALL" || strings.EqualFold(entry.Exchange, query.Exchange) {
			tickers = append(tickers, entry.Ticker)
		}
	}

	if len(tickers) == 0 {
		return &models.ScanResponse{
			Results: []models.ScanResult{},
			Meta: models.ScanMeta{
				TotalMatched: 0,
				Returned:     0,
				Exchange:     query.Exchange,
				ExecutedAt:   time.Now().UTC(),
				QueryTimeMS:  time.Since(start).Milliseconds(),
			},
		}, nil
	}

	sc.logger.Debug().
		Str("exchange", query.Exchange).
		Int("tickers", len(tickers)).
		Int("fields", len(query.Fields)).
		Int("filters", len(query.Filters)).
		Msg("Executing market scan")

	// Batch load market data and signals
	marketDataList, err := sc.storage.MarketDataStorage().GetMarketDataBatch(ctx, tickers)
	if err != nil {
		return nil, fmt.Errorf("failed to load market data: %w", err)
	}

	signalsList, err := sc.storage.SignalStorage().GetSignalsBatch(ctx, tickers)
	if err != nil {
		sc.logger.Warn().Err(err).Msg("Failed to batch load signals, continuing without")
		signalsList = nil
	}

	// Index signals by ticker for fast lookup
	signalsByTicker := make(map[string]*models.TickerSignals, len(signalsList))
	for _, sig := range signalsList {
		if sig != nil {
			signalsByTicker[sig.Ticker] = sig
		}
	}

	// Parse sort
	sortFields, err := query.ParseScanSort()
	if err != nil {
		return nil, fmt.Errorf("invalid sort: %w", err)
	}

	// Process each ticker: extract fields, apply filters
	var results []models.ScanResult
	for _, md := range marketDataList {
		if md == nil {
			continue
		}
		sig := signalsByTicker[md.Ticker]

		// Evaluate filters
		if !sc.evaluateFilters(query.Filters, md, sig) {
			continue
		}

		// Extract requested fields
		result := make(models.ScanResult, len(query.Fields))
		for _, field := range query.Fields {
			entry := sc.registry.Get(field)
			if entry == nil {
				result[field] = nil
				continue
			}
			result[field] = entry.extractor(md, sig)
		}
		results = append(results, result)
	}

	totalMatched := len(results)

	// Sort
	if len(sortFields) > 0 {
		sc.sortResults(results, sortFields)
	}

	// Limit
	limit := query.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	if len(results) > limit {
		results = results[:limit]
	}

	// Ensure non-nil results array
	if results == nil {
		results = []models.ScanResult{}
	}

	return &models.ScanResponse{
		Results: results,
		Meta: models.ScanMeta{
			TotalMatched: totalMatched,
			Returned:     len(results),
			Exchange:     query.Exchange,
			ExecutedAt:   time.Now().UTC(),
			QueryTimeMS:  time.Since(start).Milliseconds(),
		},
	}, nil
}

// validateQuery validates and applies defaults to a scan query.
func (sc *Scanner) validateQuery(query *models.ScanQuery) error {
	if query.Exchange == "" {
		return fmt.Errorf("exchange is required")
	}
	query.Exchange = strings.ToUpper(query.Exchange)

	if len(query.Fields) == 0 {
		return fmt.Errorf("at least one field is required")
	}

	// Validate field names
	for _, field := range query.Fields {
		if sc.registry.Get(field) == nil {
			return fmt.Errorf("unknown field: %s", field)
		}
	}

	// Validate filter field names
	for _, filter := range query.Filters {
		if err := sc.validateFilter(filter, 0); err != nil {
			return err
		}
	}

	// Validate sort fields
	sortFields, err := query.ParseScanSort()
	if err != nil {
		return fmt.Errorf("invalid sort: %w", err)
	}
	for _, sf := range sortFields {
		entry := sc.registry.Get(sf.Field)
		if entry == nil {
			return fmt.Errorf("unknown sort field: %s", sf.Field)
		}
		if !entry.def.Sortable {
			return fmt.Errorf("field %s is not sortable", sf.Field)
		}
		if sf.Order != "" && sf.Order != "asc" && sf.Order != "desc" {
			return fmt.Errorf("invalid sort order: %s (must be asc or desc)", sf.Order)
		}
	}

	// Apply defaults
	if query.Limit <= 0 {
		query.Limit = 20
	}
	if query.Limit > 50 {
		query.Limit = 50
	}

	return nil
}

const maxFilterDepth = 10

// validateFilter recursively validates a filter and its OR sub-filters.
func (sc *Scanner) validateFilter(f models.ScanFilter, depth int) error {
	if depth > maxFilterDepth {
		return fmt.Errorf("filter nesting exceeds max depth of %d", maxFilterDepth)
	}
	if len(f.Or) > 0 {
		for _, sub := range f.Or {
			if err := sc.validateFilter(sub, depth+1); err != nil {
				return err
			}
		}
		return nil
	}

	if f.Field == "" {
		return fmt.Errorf("filter field is required")
	}
	entry := sc.registry.Get(f.Field)
	if entry == nil {
		return fmt.Errorf("unknown filter field: %s", f.Field)
	}
	if !entry.def.Filterable {
		return fmt.Errorf("field %s is not filterable", f.Field)
	}

	// Validate operator
	validOp := false
	for _, op := range entry.def.Operators {
		if op == f.Op {
			validOp = true
			break
		}
	}
	if !validOp {
		return fmt.Errorf("operator %s not valid for field %s (valid: %v)", f.Op, f.Field, entry.def.Operators)
	}

	return nil
}

// evaluateFilters evaluates all filters against a ticker's data.
// Top-level filters are AND'd. Returns true if all filters pass.
func (sc *Scanner) evaluateFilters(filters []models.ScanFilter, md *models.MarketData, sig *models.TickerSignals) bool {
	for _, filter := range filters {
		if !sc.evaluateFilter(filter, md, sig) {
			return false
		}
	}
	return true
}

// evaluateFilter evaluates a single filter (or OR group).
func (sc *Scanner) evaluateFilter(filter models.ScanFilter, md *models.MarketData, sig *models.TickerSignals) bool {
	// OR group: any sub-filter passing means the group passes
	if len(filter.Or) > 0 {
		for _, sub := range filter.Or {
			if sc.evaluateFilter(sub, md, sig) {
				return true
			}
		}
		return false
	}

	// Get field value
	entry := sc.registry.Get(filter.Field)
	if entry == nil {
		return false
	}
	val := entry.extractor(md, sig)

	return evaluateOp(filter.Op, val, filter.Value)
}

// evaluateOp evaluates a single operator comparison.
func evaluateOp(op string, fieldVal, filterVal interface{}) bool {
	// Null checks
	if op == "is_null" {
		return fieldVal == nil
	}
	if op == "not_null" {
		return fieldVal != nil
	}

	// If field is nil, all other operators fail
	if fieldVal == nil {
		return false
	}

	switch op {
	case "==":
		return compareEqual(fieldVal, filterVal)
	case "!=":
		return !compareEqual(fieldVal, filterVal)
	case "<":
		return compareNumeric(fieldVal, filterVal) < 0
	case "<=":
		return compareNumeric(fieldVal, filterVal) <= 0
	case ">":
		return compareNumeric(fieldVal, filterVal) > 0
	case ">=":
		return compareNumeric(fieldVal, filterVal) >= 0
	case "between":
		return evaluateBetween(fieldVal, filterVal)
	case "in":
		return evaluateIn(fieldVal, filterVal)
	case "not_in":
		return !evaluateIn(fieldVal, filterVal)
	default:
		return false
	}
}

// compareEqual compares two values for equality, handling type coercion.
func compareEqual(a, b interface{}) bool {
	// Handle boolean comparisons
	if aBool, ok := a.(bool); ok {
		bBool, bOk := toBool(b)
		if bOk {
			return aBool == bBool
		}
		return false
	}

	// Handle string comparisons
	if aStr, ok := a.(string); ok {
		if bStr, ok := b.(string); ok {
			return strings.EqualFold(aStr, bStr)
		}
		return false
	}

	// Numeric comparison
	aNum, aOk := toFloat64(a)
	bNum, bOk := toFloat64(b)
	if aOk && bOk {
		return aNum == bNum
	}

	return false
}

// compareNumeric compares two values numerically.
// Returns -1, 0, or 1 like strings.Compare.
func compareNumeric(a, b interface{}) int {
	aNum, aOk := toFloat64(a)
	bNum, bOk := toFloat64(b)
	if !aOk || !bOk {
		return 0
	}
	if aNum < bNum {
		return -1
	}
	if aNum > bNum {
		return 1
	}
	return 0
}

// evaluateBetween checks if fieldVal is between min and max (inclusive).
// filterVal should be a []interface{}{min, max}.
func evaluateBetween(fieldVal, filterVal interface{}) bool {
	arr, ok := filterVal.([]interface{})
	if !ok || len(arr) != 2 {
		return false
	}
	fieldNum, fOk := toFloat64(fieldVal)
	minNum, minOk := toFloat64(arr[0])
	maxNum, maxOk := toFloat64(arr[1])
	if !fOk || !minOk || !maxOk {
		return false
	}
	return fieldNum >= minNum && fieldNum <= maxNum
}

// evaluateIn checks if fieldVal is in the filter value list.
func evaluateIn(fieldVal, filterVal interface{}) bool {
	arr, ok := filterVal.([]interface{})
	if !ok {
		return false
	}
	for _, item := range arr {
		if compareEqual(fieldVal, item) {
			return true
		}
	}
	return false
}

// toFloat64 converts various numeric types to float64.
func toFloat64(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	default:
		return 0, false
	}
}

// toBool converts a value to bool.
func toBool(v interface{}) (bool, bool) {
	switch b := v.(type) {
	case bool:
		return b, true
	default:
		return false, false
	}
}

// sortResults sorts scan results by the specified sort fields.
func (sc *Scanner) sortResults(results []models.ScanResult, sortFields []models.ScanSort) {
	sort.SliceStable(results, func(i, j int) bool {
		for _, sf := range sortFields {
			vi := results[i][sf.Field]
			vj := results[j][sf.Field]

			// Handle nil values: push them to the end
			if vi == nil && vj == nil {
				continue
			}
			if vi == nil {
				return false // nil goes last
			}
			if vj == nil {
				return true // non-nil before nil
			}

			cmp := compareValues(vi, vj)
			if cmp == 0 {
				continue
			}

			if sf.Order == "asc" || sf.Order == "" {
				return cmp < 0
			}
			return cmp > 0
		}
		return false
	})
}

// compareValues compares two non-nil values for sorting.
func compareValues(a, b interface{}) int {
	// Try numeric
	aNum, aOk := toFloat64(a)
	bNum, bOk := toFloat64(b)
	if aOk && bOk {
		if aNum < bNum {
			return -1
		}
		if aNum > bNum {
			return 1
		}
		return 0
	}

	// Try string
	aStr, aOk := a.(string)
	bStr, bOk := b.(string)
	if aOk && bOk {
		return strings.Compare(strings.ToLower(aStr), strings.ToLower(bStr))
	}

	// Try bool (false < true)
	aBool, aOk := a.(bool)
	bBool, bOk := b.(bool)
	if aOk && bOk {
		if !aBool && bBool {
			return -1
		}
		if aBool && !bBool {
			return 1
		}
		return 0
	}

	return 0
}
