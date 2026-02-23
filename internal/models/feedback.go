package models

import (
	"encoding/json"
	"time"
)

// Feedback represents an MCP client observation or data quality issue.
type Feedback struct {
	ID              string          `json:"id"`
	SessionID       string          `json:"session_id"`
	ClientType      string          `json:"client_type"`
	Category        string          `json:"category"`
	Severity        string          `json:"severity"`
	Description     string          `json:"description"`
	Ticker          string          `json:"ticker,omitempty"`
	PortfolioName   string          `json:"portfolio_name,omitempty"`
	ToolName        string          `json:"tool_name,omitempty"`
	ObservedValue   json.RawMessage `json:"observed_value,omitempty"`
	ExpectedValue   json.RawMessage `json:"expected_value,omitempty"`
	Status          string          `json:"status"`
	ResolutionNotes string          `json:"resolution_notes,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// FeedbackSummary provides aggregate counts across feedback entries.
type FeedbackSummary struct {
	Total            int            `json:"total"`
	ByStatus         map[string]int `json:"by_status"`
	BySeverity       map[string]int `json:"by_severity"`
	ByCategory       map[string]int `json:"by_category"`
	OldestUnresolved *time.Time     `json:"oldest_unresolved,omitempty"`
}

// Feedback category constants.
const (
	FeedbackCategoryDataAnomaly      = "data_anomaly"
	FeedbackCategorySyncDelay        = "sync_delay"
	FeedbackCategoryCalculationError = "calculation_error"
	FeedbackCategoryMissingData      = "missing_data"
	FeedbackCategorySchemaChange     = "schema_change"
	FeedbackCategoryToolError        = "tool_error"
	FeedbackCategoryObservation      = "observation"
)

// Feedback severity constants.
const (
	FeedbackSeverityLow    = "low"
	FeedbackSeverityMedium = "medium"
	FeedbackSeverityHigh   = "high"
)

// Feedback status constants.
const (
	FeedbackStatusNew          = "new"
	FeedbackStatusAcknowledged = "acknowledged"
	FeedbackStatusResolved     = "resolved"
	FeedbackStatusDismissed    = "dismissed"
)

// ValidFeedbackCategories is the set of allowed category values.
var ValidFeedbackCategories = map[string]bool{
	FeedbackCategoryDataAnomaly:      true,
	FeedbackCategorySyncDelay:        true,
	FeedbackCategoryCalculationError: true,
	FeedbackCategoryMissingData:      true,
	FeedbackCategorySchemaChange:     true,
	FeedbackCategoryToolError:        true,
	FeedbackCategoryObservation:      true,
}

// ValidFeedbackSeverities is the set of allowed severity values.
var ValidFeedbackSeverities = map[string]bool{
	FeedbackSeverityLow:    true,
	FeedbackSeverityMedium: true,
	FeedbackSeverityHigh:   true,
}

// ValidFeedbackStatuses is the set of allowed status values.
var ValidFeedbackStatuses = map[string]bool{
	FeedbackStatusNew:          true,
	FeedbackStatusAcknowledged: true,
	FeedbackStatusResolved:     true,
	FeedbackStatusDismissed:    true,
}
