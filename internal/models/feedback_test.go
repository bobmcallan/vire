package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidFeedbackCategories(t *testing.T) {
	expected := []string{
		"data_anomaly", "sync_delay", "calculation_error",
		"missing_data", "schema_change", "tool_error", "observation",
	}
	for _, cat := range expected {
		assert.True(t, ValidFeedbackCategories[cat], "expected %q to be a valid category", cat)
	}
	assert.False(t, ValidFeedbackCategories["invalid_category"])
	assert.False(t, ValidFeedbackCategories[""])
	assert.Equal(t, len(expected), len(ValidFeedbackCategories))
}

func TestValidFeedbackSeverities(t *testing.T) {
	expected := []string{"low", "medium", "high"}
	for _, sev := range expected {
		assert.True(t, ValidFeedbackSeverities[sev], "expected %q to be a valid severity", sev)
	}
	assert.False(t, ValidFeedbackSeverities["critical"])
	assert.False(t, ValidFeedbackSeverities[""])
	assert.Equal(t, len(expected), len(ValidFeedbackSeverities))
}

func TestValidFeedbackStatuses(t *testing.T) {
	expected := []string{"new", "acknowledged", "resolved", "dismissed"}
	for _, st := range expected {
		assert.True(t, ValidFeedbackStatuses[st], "expected %q to be a valid status", st)
	}
	assert.False(t, ValidFeedbackStatuses["closed"])
	assert.False(t, ValidFeedbackStatuses[""])
	assert.Equal(t, len(expected), len(ValidFeedbackStatuses))
}

func TestFeedbackConstants(t *testing.T) {
	// Verify constants match the validation maps
	assert.True(t, ValidFeedbackCategories[FeedbackCategoryDataAnomaly])
	assert.True(t, ValidFeedbackCategories[FeedbackCategorySyncDelay])
	assert.True(t, ValidFeedbackCategories[FeedbackCategoryCalculationError])
	assert.True(t, ValidFeedbackCategories[FeedbackCategoryMissingData])
	assert.True(t, ValidFeedbackCategories[FeedbackCategorySchemaChange])
	assert.True(t, ValidFeedbackCategories[FeedbackCategoryToolError])
	assert.True(t, ValidFeedbackCategories[FeedbackCategoryObservation])

	assert.True(t, ValidFeedbackSeverities[FeedbackSeverityLow])
	assert.True(t, ValidFeedbackSeverities[FeedbackSeverityMedium])
	assert.True(t, ValidFeedbackSeverities[FeedbackSeverityHigh])

	assert.True(t, ValidFeedbackStatuses[FeedbackStatusNew])
	assert.True(t, ValidFeedbackStatuses[FeedbackStatusAcknowledged])
	assert.True(t, ValidFeedbackStatuses[FeedbackStatusResolved])
	assert.True(t, ValidFeedbackStatuses[FeedbackStatusDismissed])
}
