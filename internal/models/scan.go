// Package models defines data structures for Vire
package models

import (
	"encoding/json"
	"time"
)

// ScanQuery represents a market scan request
type ScanQuery struct {
	Exchange     string          `json:"exchange"`
	Filters      []ScanFilter    `json:"filters,omitempty"`
	Fields       []string        `json:"fields"`
	Sort         json.RawMessage `json:"sort,omitempty"` // single ScanSort or []ScanSort
	Limit        int             `json:"limit,omitempty"`
	ForceRefresh bool            `json:"force_refresh,omitempty"`
}

// ScanFilter represents a single filter condition
type ScanFilter struct {
	Field string       `json:"field,omitempty"`
	Op    string       `json:"op,omitempty"`
	Value interface{}  `json:"value,omitempty"`
	Or    []ScanFilter `json:"or,omitempty"` // OR group
}

// ScanSort specifies sort order
type ScanSort struct {
	Field string `json:"field"`
	Order string `json:"order"` // "asc" or "desc"
}

// ScanResult holds one row of scan results
type ScanResult map[string]interface{}

// ScanResponse is the full scan response
type ScanResponse struct {
	Results []ScanResult `json:"results"`
	Meta    ScanMeta     `json:"meta"`
}

// ScanMeta contains query metadata
type ScanMeta struct {
	TotalMatched int       `json:"total_matched"`
	Returned     int       `json:"returned"`
	Exchange     string    `json:"exchange"`
	ExecutedAt   time.Time `json:"executed_at"`
	QueryTimeMS  int64     `json:"query_time_ms"`
}

// ScanFieldDef describes a single scannable field (for introspection)
type ScanFieldDef struct {
	Field       string      `json:"field"`
	Type        string      `json:"type"` // "string", "float", "int", "bool"
	Description string      `json:"description"`
	Filterable  bool        `json:"filterable"`
	Sortable    bool        `json:"sortable"`
	Operators   []string    `json:"operators"`
	Nullable    bool        `json:"nullable,omitempty"`
	Enum        []string    `json:"enum,omitempty"`
	Example     interface{} `json:"example,omitempty"`
	Unit        string      `json:"unit,omitempty"`
}

// ScanFieldGroup groups fields by category
type ScanFieldGroup struct {
	Name   string         `json:"name"`
	Fields []ScanFieldDef `json:"fields"`
}

// ScanFieldsResponse is returned by GET /api/scan/fields
type ScanFieldsResponse struct {
	Version     string           `json:"version"`
	Groups      []ScanFieldGroup `json:"groups"`
	Exchanges   []string         `json:"exchanges"`
	MaxLimit    int              `json:"max_limit"`
	GeneratedAt time.Time        `json:"generated_at"`
}

// ParseScanSort parses the raw Sort JSON into a slice of ScanSort.
// Accepts either a single ScanSort object or an array of ScanSort objects.
func (q *ScanQuery) ParseScanSort() ([]ScanSort, error) {
	if len(q.Sort) == 0 {
		return nil, nil
	}

	// Try array first
	var arr []ScanSort
	if err := json.Unmarshal(q.Sort, &arr); err == nil {
		return arr, nil
	}

	// Try single object
	var single ScanSort
	if err := json.Unmarshal(q.Sort, &single); err != nil {
		return nil, err
	}
	return []ScanSort{single}, nil
}
