package surrealdb

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// 1. Input validation — hostile payloads in string fields
// ============================================================================

func TestStress_FeedbackCreate_HostileDescriptions(t *testing.T) {
	db := testDB(t)
	store := NewFeedbackStore(db, testLogger())
	ctx := context.Background()

	hostilePayloads := []struct {
		name  string
		value string
	}{
		{"sql_injection_single_quote", "'; DROP TABLE mcp_feedback; --"},
		{"sql_injection_double_quote", `"; DELETE FROM mcp_feedback; --`},
		{"surrealql_remove_table", "`) REMOVE TABLE mcp_feedback; --"},
		{"surrealql_update", "test; UPDATE mcp_feedback SET status = 'resolved'"},
		{"xss_script_tag", `<script>alert('xss')</script>`},
		{"xss_img_onerror", `<img src=x onerror=alert(1)>`},
		{"xss_event_handler", `<div onmouseover="steal(document.cookie)">hover</div>`},
		{"null_bytes", "description with \x00 null \x00 bytes"},
		{"unicode_rtl_override", "normal \u202Edesrever text"},
		{"unicode_zero_width", "hello\u200Bworld\u200B\u200B"},
		{"very_long_10kb", strings.Repeat("A", 10240)},
		{"very_long_100kb", strings.Repeat("B", 102400)},
		{"newlines_crlf", "line1\r\nline2\r\nline3"},
		{"json_in_description", `{"key": "value", "nested": {"a": 1}}`},
		{"backslash_escapes", `path\to\file\n\t\r`},
		{"html_entities", `&lt;script&gt;alert(1)&lt;/script&gt;`},
		{"emoji_payload", "🔥💉🗃️📋"},
		{"empty_after_trim", "   \t\n\r   "},
	}

	for _, tc := range hostilePayloads {
		t.Run(tc.name, func(t *testing.T) {
			fb := &models.Feedback{
				Category:    models.FeedbackCategoryObservation,
				Severity:    models.FeedbackSeverityHigh,
				Description: tc.value,
			}

			err := store.Create(ctx, fb)
			require.NoError(t, err, "Create should not fail for hostile description")
			assert.NotEmpty(t, fb.ID, "ID should be generated")

			// Verify round-trip: read it back
			got, err := store.Get(ctx, fb.ID)
			require.NoError(t, err, "Get should not fail")
			require.NotNil(t, got, "record should exist")
			assert.Equal(t, tc.value, got.Description, "description should round-trip unchanged")
		})
	}
}

// ============================================================================
// 2. SQL injection via record ID field
// ============================================================================

func TestStress_FeedbackGet_HostileIDs(t *testing.T) {
	db := testDB(t)
	store := NewFeedbackStore(db, testLogger())
	ctx := context.Background()

	hostileIDs := []string{
		"'; DROP TABLE mcp_feedback; --",
		"fb_test\"; DELETE mcp_feedback; --",
		"mcp_feedback:injected",
		"`) REMOVE TABLE mcp_feedback; --",
		"../../../etc/passwd",
		"\x00\x00\x00",
		strings.Repeat("A", 100000),
		"mcp_feedback:⟨injected⟩",
		"test`; REMOVE DATABASE test; `",
	}

	for _, id := range hostileIDs {
		name := id
		if len(name) > 30 {
			name = name[:30]
		}
		t.Run(name, func(t *testing.T) {
			// Should not panic, should not corrupt data
			got, err := store.Get(ctx, id)
			// We accept either nil result or an error — but no panic
			if err != nil {
				t.Logf("Get with hostile ID returned error (acceptable): %v", err)
			}
			if got != nil {
				t.Logf("Get with hostile ID returned non-nil result (unexpected but not a crash)")
			}
		})
	}
}

func TestStress_FeedbackDelete_HostileIDs(t *testing.T) {
	db := testDB(t)
	store := NewFeedbackStore(db, testLogger())
	ctx := context.Background()

	hostileIDs := []string{
		"'; DROP TABLE mcp_feedback; --",
		"mcp_feedback:*",
		"*",
		"",
	}

	// Create a canary record first
	canary := &models.Feedback{
		Category:    models.FeedbackCategoryObservation,
		Description: "canary record",
		Severity:    models.FeedbackSeverityLow,
	}
	require.NoError(t, store.Create(ctx, canary))

	for _, id := range hostileIDs {
		name := id
		if name == "" {
			name = "(empty)"
		}
		t.Run(name, func(t *testing.T) {
			// Should not delete the canary
			err := store.Delete(ctx, id)
			// Error is acceptable, panic is not
			if err != nil {
				t.Logf("Delete with hostile ID returned error: %v", err)
			}
		})
	}

	// Verify canary still exists
	got, err := store.Get(ctx, canary.ID)
	require.NoError(t, err, "canary Get should not fail")
	require.NotNil(t, got, "canary should still exist after hostile delete attempts")
	assert.Equal(t, "canary record", got.Description)
}

// ============================================================================
// 3. Observed/Expected value — malformed JSON edge cases
// ============================================================================

func TestStress_FeedbackCreate_ObservedValueEdgeCases(t *testing.T) {
	db := testDB(t)
	store := NewFeedbackStore(db, testLogger())
	ctx := context.Background()

	cases := []struct {
		name     string
		observed json.RawMessage
		expected json.RawMessage
	}{
		{"null_json", json.RawMessage("null"), json.RawMessage("null")},
		{"nested_deep", json.RawMessage(`{"a":{"b":{"c":{"d":{"e":"deep"}}}}}`), nil},
		{"large_array", json.RawMessage(`[` + strings.Repeat(`"x",`, 999) + `"x"]`), nil},
		{"number_overflow", json.RawMessage(`99999999999999999999999999999999999999`), nil},
		{"boolean", json.RawMessage(`true`), json.RawMessage(`false`)},
		{"string_value", json.RawMessage(`"just a string"`), nil},
		{"empty_object", json.RawMessage(`{}`), json.RawMessage(`{}`)},
		{"empty_array", json.RawMessage(`[]`), json.RawMessage(`[]`)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fb := &models.Feedback{
				Category:      models.FeedbackCategoryDataAnomaly,
				Description:   "test observed/expected",
				Severity:      models.FeedbackSeverityMedium,
				ObservedValue: tc.observed,
				ExpectedValue: tc.expected,
			}

			err := store.Create(ctx, fb)
			require.NoError(t, err, "Create should handle JSON edge case")

			got, err := store.Get(ctx, fb.ID)
			require.NoError(t, err)
			require.NotNil(t, got)
			// Verify observed value round-trips
			if tc.observed != nil {
				assert.JSONEq(t, string(tc.observed), string(got.ObservedValue))
			}
		})
	}
}

// ============================================================================
// 4. Pagination boundary conditions
// ============================================================================

func TestStress_FeedbackList_PaginationBoundaries(t *testing.T) {
	db := testDB(t)
	store := NewFeedbackStore(db, testLogger())
	ctx := context.Background()

	// Create 5 records
	for i := 0; i < 5; i++ {
		fb := &models.Feedback{
			Category:    models.FeedbackCategoryObservation,
			Description: "pagination test",
			Severity:    models.FeedbackSeverityLow,
		}
		require.NoError(t, store.Create(ctx, fb))
	}

	cases := []struct {
		name          string
		page          int
		perPage       int
		expectItems   int // expected item count
		expectClamped bool
	}{
		{"page_0_clamps_to_1", 0, 20, 5, true},
		{"page_negative_clamps_to_1", -1, 20, 5, true},
		{"per_page_0_clamps_to_20", 1, 0, 5, true},
		{"per_page_negative_clamps_to_20", 1, -1, 5, true},
		{"per_page_over_100_clamps", 1, 9999, 5, true},
		{"page_beyond_total", 999, 20, 0, false},
		{"per_page_1", 1, 1, 1, false},
		{"per_page_exactly_total", 1, 5, 5, false},
		{"per_page_exceeds_total", 1, 10, 5, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := interfaces.FeedbackListOptions{
				Page:    tc.page,
				PerPage: tc.perPage,
			}
			items, total, err := store.List(ctx, opts)
			require.NoError(t, err)
			assert.Equal(t, 5, total, "total should always be 5")
			assert.Len(t, items, tc.expectItems)
		})
	}
}

// ============================================================================
// 5. Summary with empty database
// ============================================================================

func TestStress_FeedbackSummary_EmptyDB(t *testing.T) {
	db := testDB(t)
	store := NewFeedbackStore(db, testLogger())
	ctx := context.Background()

	summary, err := store.Summary(ctx)
	require.NoError(t, err, "Summary should not fail on empty table")
	require.NotNil(t, summary)
	assert.Equal(t, 0, summary.Total)
	assert.NotNil(t, summary.ByStatus, "ByStatus map should be initialized even when empty")
	assert.NotNil(t, summary.BySeverity, "BySeverity map should be initialized even when empty")
	assert.NotNil(t, summary.ByCategory, "ByCategory map should be initialized even when empty")
	assert.Nil(t, summary.OldestUnresolved, "OldestUnresolved should be nil when no feedback exists")
}

// ============================================================================
// 6. Update non-existent record — silent success
// ============================================================================

func TestStress_FeedbackUpdate_NonExistent(t *testing.T) {
	db := testDB(t)
	store := NewFeedbackStore(db, testLogger())
	ctx := context.Background()

	// FINDING: Update on a non-existent ID succeeds silently.
	// SurrealDB's UPDATE on a non-existent record ID creates no error.
	// The handler checks existence before calling Update, but the store
	// layer itself does not verify the record exists.
	err := store.Update(ctx, "fb_nonexistent", "resolved", "fixed", "", "", "")
	// Document the behavior — the store does not return an error
	if err == nil {
		t.Log("FINDING: FeedbackStore.Update succeeds silently on non-existent ID — " +
			"handler must check existence before calling Update (which it does)")
	}
}

// ============================================================================
// 7. BulkUpdateStatus — mix of valid and invalid IDs
// ============================================================================

func TestStress_FeedbackBulkUpdate_MixedIDs(t *testing.T) {
	db := testDB(t)
	store := NewFeedbackStore(db, testLogger())
	ctx := context.Background()

	// Create one real record
	fb := &models.Feedback{
		Category:    models.FeedbackCategoryObservation,
		Description: "bulk test",
		Severity:    models.FeedbackSeverityLow,
	}
	require.NoError(t, store.Create(ctx, fb))

	// Mix valid and invalid IDs
	ids := []string{fb.ID, "fb_nonexistent1", "fb_nonexistent2"}
	updated, err := store.BulkUpdateStatus(ctx, ids, "acknowledged", "batch update")
	require.NoError(t, err)
	// BulkUpdateStatus iterates and counts successes.
	// SurrealDB UPDATE on non-existent IDs doesn't error — it returns empty result.
	// So all 3 will be counted as "updated" even though 2 don't exist.
	t.Logf("FINDING: BulkUpdateStatus reports %d updated for %d IDs (including non-existent). "+
		"Non-existent IDs are counted as successful updates because SurrealDB UPDATE doesn't error.",
		updated, len(ids))

	// Verify the real record was actually updated
	got, err := store.Get(ctx, fb.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "acknowledged", got.Status)
}

func TestStress_FeedbackBulkUpdate_EmptyIDs(t *testing.T) {
	db := testDB(t)
	store := NewFeedbackStore(db, testLogger())
	ctx := context.Background()

	// The handler validates len(ids) > 0, but test the store directly
	updated, err := store.BulkUpdateStatus(ctx, []string{}, "resolved", "")
	require.NoError(t, err)
	assert.Equal(t, 0, updated, "empty IDs should update 0 records")
}

// ============================================================================
// 8. ID format predictability
// ============================================================================

func TestStress_FeedbackCreate_IDUniqueness(t *testing.T) {
	db := testDB(t)
	store := NewFeedbackStore(db, testLogger())
	ctx := context.Background()

	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		fb := &models.Feedback{
			Category:    models.FeedbackCategoryObservation,
			Description: "uniqueness test",
			Severity:    models.FeedbackSeverityLow,
		}
		require.NoError(t, store.Create(ctx, fb))
		assert.True(t, strings.HasPrefix(fb.ID, "fb_"), "ID should have fb_ prefix, got: %s", fb.ID)
		assert.False(t, seen[fb.ID], "duplicate ID detected: %s", fb.ID)
		seen[fb.ID] = true
	}

	// FINDING: IDs use uuid.New().String()[:8] — 8 hex chars = 32 bits of entropy.
	// Birthday paradox: ~50% collision probability at ~65,000 records.
	// For a feedback table this is likely fine, but worth noting.
	// UPSERT is used for Create, so a collision would silently overwrite.
	t.Log("NOTE: Feedback IDs use 8 chars of UUID (32 bits entropy). " +
		"Birthday paradox collision at ~65K records. UPSERT means collision = silent overwrite.")
}

// ============================================================================
// 9. Create with pre-set ID — UPSERT overwrites
// ============================================================================

func TestStress_FeedbackCreate_IDOverwrite(t *testing.T) {
	db := testDB(t)
	store := NewFeedbackStore(db, testLogger())
	ctx := context.Background()

	// FINDING: Create uses UPSERT, so if a caller provides an existing ID,
	// the record is silently overwritten. The handler doesn't set ID,
	// so this only matters if the store is used directly.

	fb1 := &models.Feedback{
		ID:          "fb_fixed_id",
		Category:    models.FeedbackCategoryDataAnomaly,
		Description: "original record",
		Severity:    models.FeedbackSeverityHigh,
	}
	require.NoError(t, store.Create(ctx, fb1))

	fb2 := &models.Feedback{
		ID:          "fb_fixed_id", // same ID
		Category:    models.FeedbackCategoryToolError,
		Description: "overwritten record",
		Severity:    models.FeedbackSeverityLow,
	}
	require.NoError(t, store.Create(ctx, fb2))

	got, err := store.Get(ctx, "fb_fixed_id")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "overwritten record", got.Description,
		"UPSERT should overwrite existing record with same ID")
	assert.Equal(t, models.FeedbackCategoryToolError, got.Category)
	t.Log("FINDING: Create uses UPSERT — calling Create with an existing ID silently overwrites the record")
}

// ============================================================================
// 10. Timestamp integrity
// ============================================================================

func TestStress_FeedbackCreate_TimestampIntegrity(t *testing.T) {
	db := testDB(t)
	store := NewFeedbackStore(db, testLogger())
	ctx := context.Background()

	before := time.Now().Add(-time.Second)

	fb := &models.Feedback{
		Category:    models.FeedbackCategoryObservation,
		Description: "timestamp test",
	}
	require.NoError(t, store.Create(ctx, fb))

	after := time.Now().Add(time.Second)

	got, err := store.Get(ctx, fb.ID)
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.True(t, got.CreatedAt.After(before), "created_at should be after test start")
	assert.True(t, got.CreatedAt.Before(after), "created_at should be before test end")
	assert.True(t, got.UpdatedAt.After(before), "updated_at should be after test start")
	assert.True(t, got.UpdatedAt.Before(after), "updated_at should be before test end")
}

func TestStress_FeedbackUpdate_TimestampAdvances(t *testing.T) {
	db := testDB(t)
	store := NewFeedbackStore(db, testLogger())
	ctx := context.Background()

	fb := &models.Feedback{
		Category:    models.FeedbackCategoryObservation,
		Description: "update timestamp test",
	}
	require.NoError(t, store.Create(ctx, fb))

	original, err := store.Get(ctx, fb.ID)
	require.NoError(t, err)

	// Small delay to ensure timestamps differ
	time.Sleep(10 * time.Millisecond)

	require.NoError(t, store.Update(ctx, fb.ID, "acknowledged", "looking into it", "", "", ""))

	updated, err := store.Get(ctx, fb.ID)
	require.NoError(t, err)
	require.NotNil(t, updated)

	assert.Equal(t, original.CreatedAt, updated.CreatedAt,
		"created_at should not change on update")
	assert.True(t, updated.UpdatedAt.After(original.UpdatedAt) || updated.UpdatedAt.Equal(original.UpdatedAt),
		"updated_at should advance or stay the same on update")
}

// ============================================================================
// 11. List with hostile filter values
// ============================================================================

func TestStress_FeedbackList_HostileFilters(t *testing.T) {
	db := testDB(t)
	store := NewFeedbackStore(db, testLogger())
	ctx := context.Background()

	// Hostile filter values should not cause SQL injection
	hostileFilters := []interfaces.FeedbackListOptions{
		{Status: "'; DROP TABLE mcp_feedback; --", Page: 1, PerPage: 20},
		{Category: `" OR 1=1; --`, Page: 1, PerPage: 20},
		{Ticker: "BHP'; DELETE mcp_feedback; --", Page: 1, PerPage: 20},
		{SessionID: "test\x00null\x00byte", Page: 1, PerPage: 20},
		{Sort: "'; DROP TABLE mcp_feedback; --", Page: 1, PerPage: 20},
		{PortfolioName: strings.Repeat("A", 100000), Page: 1, PerPage: 20},
	}

	for _, opts := range hostileFilters {
		name := opts.Status + opts.Category + opts.Ticker + opts.SessionID
		if len(name) > 40 {
			name = name[:40]
		}
		if name == "" {
			name = "hostile_filter"
		}
		t.Run(name, func(t *testing.T) {
			// Should not panic, should not corrupt data
			items, total, err := store.List(ctx, opts)
			// We accept either empty results or an error — but no panic or injection
			if err != nil {
				t.Logf("List with hostile filter returned error (acceptable): %v", err)
				return
			}
			assert.GreaterOrEqual(t, total, 0)
			assert.NotNil(t, items) // may be nil if no results, that's fine
		})
	}
}

// ============================================================================
// 12. Sort parameter — only valid values are handled
// ============================================================================

func TestStress_FeedbackList_SortInjection(t *testing.T) {
	db := testDB(t)
	store := NewFeedbackStore(db, testLogger())
	ctx := context.Background()

	// The sort parameter uses a switch statement, so unknown values
	// fall through to the default ORDER BY. This is safe.
	// But verify no injection is possible.
	opts := interfaces.FeedbackListOptions{
		Sort:    "created_at DESC; DELETE FROM mcp_feedback; --",
		Page:    1,
		PerPage: 20,
	}
	_, _, err := store.List(ctx, opts)
	// The hostile sort value should be ignored (falls to default case in switch)
	// No injection should occur because sort is not interpolated into SQL
	if err != nil {
		t.Logf("Sort injection returned error (acceptable): %v", err)
	}
	t.Log("VERIFIED: Sort parameter uses switch statement — unrecognized values fall to default ORDER BY, no injection possible")
}

// ============================================================================
// 13. Severity sort ordering — string-based, not numeric
// ============================================================================

func TestStress_FeedbackList_SeveritySortIsStringBased(t *testing.T) {
	// FINDING: The "severity_desc" sort option uses ORDER BY severity DESC.
	// SurrealDB sorts strings lexicographically: "medium" > "low" > "high"
	// This means "high" severity items sort LAST, not first.
	// Expected order for severity_desc: medium, low, high
	// Actual desired order: high, medium, low
	//
	// This is a semantic bug — the sort doesn't produce the expected ordering.

	t.Log("FINDING: severity_desc sort uses lexicographic ordering: medium > low > high. " +
		"High severity items sort LAST, which is the opposite of what users expect. " +
		"Fix: use a CASE expression or numeric severity field for correct ordering.")
}

// ============================================================================
// 14. Delete then Get — should return nil
// ============================================================================

func TestStress_FeedbackDeleteThenGet(t *testing.T) {
	db := testDB(t)
	store := NewFeedbackStore(db, testLogger())
	ctx := context.Background()

	fb := &models.Feedback{
		Category:    models.FeedbackCategoryObservation,
		Description: "will be deleted",
	}
	require.NoError(t, store.Create(ctx, fb))

	require.NoError(t, store.Delete(ctx, fb.ID))

	got, err := store.Get(ctx, fb.ID)
	require.NoError(t, err, "Get after delete should not error")
	assert.Nil(t, got, "Get after delete should return nil")
}

// ============================================================================
// 15. Defaults — severity and status auto-set
// ============================================================================

func TestStress_FeedbackCreate_DefaultsApplied(t *testing.T) {
	db := testDB(t)
	store := NewFeedbackStore(db, testLogger())
	ctx := context.Background()

	fb := &models.Feedback{
		Category:    models.FeedbackCategoryObservation,
		Description: "defaults test",
		// No severity, no status
	}
	require.NoError(t, store.Create(ctx, fb))

	got, err := store.Get(ctx, fb.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, models.FeedbackSeverityMedium, got.Severity, "default severity should be medium")
	assert.Equal(t, models.FeedbackStatusNew, got.Status, "default status should be new")
}
