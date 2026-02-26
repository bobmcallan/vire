# Code Quality Review - Implementation Pending

## Status
- **Implementation Status:** Task #1 in_progress
- **Review Status:** Task #3 pending implementation completion
- **Reviewer:** Claude (reviewer)

## Summary
Code review will proceed once implementation (task #1) completes. This document provides the quality checks and patterns to validate.

---

## Feature 1: Fix `get_stock_data` Include Parameter

### Location
`/home/bobmc/development/vire/internal/server/handlers.go` lines 547-565

### Current Code Issue
```go
includeParam := r.URL.Query().Get("include")  // BUG: only reads FIRST value
```

When MCP sends array-style params: `include=price&include=signals`, only "price" is captured.

### Required Fix Pattern
```go
includeParams := r.URL.Query()["include"]
include := interfaces.StockDataInclude{Price: true, Fundamentals: true, Signals: true, News: true}
if len(includeParams) > 0 {
    include = interfaces.StockDataInclude{}
    for _, param := range includeParams {
        for _, inc := range strings.Split(param, ",") {
            switch strings.TrimSpace(inc) {
            case "price":
                include.Price = true
            // ... etc
```

### Code Quality Checklist
- [ ] Correctly uses `r.URL.Query()["include"]` (returns []string)
- [ ] Outer loop iterates all query param values
- [ ] Inner loop handles comma-separated values within each param
- [ ] Both formats work:
  - Array: `?include=price&include=signals`
  - CSV: `?include=price,signals`
  - Mixed: `?include=price,fundamentals&include=signals`
- [ ] `strings.TrimSpace()` applied to split values
- [ ] Switch statement for 4 cases (price, fundamentals, signals, news)
- [ ] Case-sensitive matching (lowercase)
- [ ] Default all-true behavior preserved if no params
- [ ] No off-by-one errors in loop logic

### Testing Expectations
These tests should exist before implementation validation:

**Unit Tests (in handlers_test.go or new handlers_stock_test.go):**
1. Array format: `?include=price&include=signals` → Price=true, Fundamentals=false, Signals=true, News=false
2. CSV format: `?include=price,signals` → Price=true, Fundamentals=false, Signals=true, News=false
3. Mixed format: `?include=price&include=signals,news` → all true except Fundamentals
4. Invalid value ignored: `?include=price,invalid` → only Price=true (invalid dropped)
5. Empty param: `?include=` → default behavior (all true)
6. No param: (default) → all true
7. Whitespace handling: `?include=price , signals` → trimmed correctly

---

## Feature 2: Add User Context to Feedback

### Affected Files
1. `internal/models/feedback.go`
2. `internal/server/handlers_feedback.go`
3. `internal/interfaces/storage.go`
4. `internal/storage/surrealdb/feedbackstore.go`

### Changes Required

#### 1. Model: `internal/models/feedback.go`

**Add fields to Feedback struct:**
```go
type Feedback struct {
    // ... existing fields ...
    UserID    string `json:"user_id,omitempty"`
    UserName  string `json:"user_name,omitempty"`
    UserEmail string `json:"user_email,omitempty"`
}
```

**Code Quality Checks:**
- [ ] Fields added after `ResolutionNotes`, before timestamps
- [ ] JSON tags use snake_case
- [ ] All three marked `omitempty` (feedback can come from unauthenticated sources)
- [ ] No validation logic in model (validation at handler level)

#### 2. Handler: `internal/server/handlers_feedback.go`

**In `handleFeedbackSubmit()` (around line 102, after building fb):**
```go
fb := &models.Feedback{
    SessionID:     body.SessionID,
    ClientType:    body.ClientType,
    Category:      body.Category,
    Severity:      body.Severity,
    Description:   strings.TrimSpace(body.Description),
    Ticker:        body.Ticker,
    PortfolioName: body.PortfolioName,
    ToolName:      body.ToolName,
}

// NEW: Extract user context if authenticated
if uc := common.UserContextFromContext(r.Context()); uc != nil && uc.UserID != "" {
    fb.UserID = uc.UserID
    // Look up user details
    internalUser, err := s.app.Storage.InternalStore().GetUser(ctx, uc.UserID)
    if err != nil {
        s.logger.Warn().Err(err).Str("userID", uc.UserID).Msg("Failed to look up user for feedback")
    } else if internalUser != nil {
        fb.UserName = internalUser.DisplayName  // or appropriate field
        fb.UserEmail = internalUser.Email
    }
}
```

**In `handleFeedbackUpdate()` (around line 257, before store.Update call):**
```go
// Extract updater identity if authenticated
userID, userName, userEmail := "", "", ""
if uc := common.UserContextFromContext(r.Context()); uc != nil && uc.UserID != "" {
    userID = uc.UserID
    internalUser, err := s.app.Storage.InternalStore().GetUser(ctx, uc.UserID)
    if err != nil {
        s.logger.Warn().Err(err).Str("userID", uc.UserID).Msg("Failed to look up user for feedback update")
    } else if internalUser != nil {
        userName = internalUser.DisplayName
        userEmail = internalUser.Email
    }
}

if err := store.Update(ctx, id, status, body.ResolutionNotes, userID, userName, userEmail); err != nil {
    // ...
}
```

**Code Quality Checks:**
- [ ] Uses `common.UserContextFromContext()` (see pattern in handlers_admin_test.go:20)
- [ ] Checks both `uc != nil && uc.UserID != ""` before using
- [ ] User lookup wrapped in GetUser(), not assumed to exist
- [ ] Errors logged as warnings (non-fatal)
- [ ] Feedback submission doesn't fail if user lookup fails
- [ ] Variables initialized to empty strings as defaults
- [ ] Field assignment follows alphabetical or logical order

#### 3. Interface: `internal/interfaces/storage.go` (line 132)

**Update FeedbackStore.Update() signature:**
```go
type FeedbackStore interface {
    Create(ctx context.Context, fb *models.Feedback) error
    Get(ctx context.Context, id string) (*models.Feedback, error)
    List(ctx context.Context, opts FeedbackListOptions) ([]*models.Feedback, int, error)
    Update(ctx context.Context, id string, status, resolutionNotes string, userID, userName, userEmail string) error  // CHANGED
    BulkUpdateStatus(ctx context.Context, ids []string, status, resolutionNotes string) (int, error)
    Delete(ctx context.Context, id string) error
    Summary(ctx context.Context) (*models.FeedbackSummary, error)
}
```

**Code Quality Checks:**
- [ ] Parameter order: context, id, status, notes, userID, userName, userEmail
- [ ] All user params are strings (not pointers)
- [ ] Method signature updated (not just implementation)

#### 4. Storage: `internal/storage/surrealdb/feedbackstore.go`

**Update feedbackSelectFields constant (line 17-19):**
```go
const feedbackSelectFields = `feedback_id as id, session_id, client_type, category, severity, description,
    ticker, portfolio_name, tool_name, observed_value, expected_value,
    user_id, user_name, user_email,
    status, resolution_notes, created_at, updated_at`
```

**Update Create() method (line 32-78):**
- [ ] Add to SQL SET clause: `user_id = $user_id, user_name = $user_name, user_email = $user_email`
- [ ] Add to vars map: `"user_id": fb.UserID, "user_name": fb.UserName, "user_email": fb.UserEmail`
- [ ] Placement: after tool_name, before status

**Update Update() method signature and implementation (line 198-211):**
```go
func (s *FeedbackStore) Update(ctx context.Context, id string, status, resolutionNotes string, userID, userName, userEmail string) error {
    sql := "UPDATE $rid SET status = $status, resolution_notes = $notes, updated_by_user_id = $user_id, updated_by_user_name = $user_name, updated_by_user_email = $user_email, updated_at = $now"
    vars := map[string]any{
        "rid":      surrealmodels.NewRecordID("mcp_feedback", id),
        "status":   status,
        "notes":    resolutionNotes,
        "user_id":  userID,
        "user_name": userName,
        "user_email": userEmail,
        "now":      time.Now(),
    }
    // ... rest of implementation
}
```

**Code Quality Checks:**
- [ ] SQL column names prefixed with `updated_by_` to avoid confusion with creation
- [ ] New parameters added to vars map with correct types
- [ ] `updated_at` timestamp remains (not the "who" but the "when")
- [ ] No SQL injection (all values parameterized)
- [ ] Compile check at EOF still passes

**BulkUpdateStatus() consideration:**
- [ ] Check if this method needs signature update (spec doesn't mention it)
- [ ] If updated, same pattern as Update()
- [ ] If unchanged, document why in comment

### Testing Expectations

**Unit Tests (handlers_feedback_test.go):**
1. Feedback without auth context → UserID/UserName/UserEmail empty
2. Feedback with auth context → UserID/UserName/UserEmail populated
3. Feedback with auth but GetUser() fails → logged warning, UserID still set, name/email empty
4. Update without auth context → updated_by fields empty
5. Update with auth context → updated_by fields populated

**Integration Tests (test suite):**
1. Submit with X-Vire-User-ID header → lookup user, populate fields
2. Submit without auth → fields empty
3. Update with auth → verify updated_by fields in GET response
4. Verify GET returns user fields correctly

### Pattern Consistency

**UserContext extraction pattern** (from handlers_admin_test.go:20):
```go
uc := common.UserContextFromContext(r.Context())
```

**User lookup pattern** (from common practice):
```go
user, err := s.app.Storage.InternalStore().GetUser(ctx, userID)
if err != nil {
    s.logger.Warn().Err(err).Msg("lookup failed")
}
```

---

## Overall Code Quality Expectations

### General Guidelines
- No panic() — use errors
- No shadowing of outer scope variables
- SQL is parameterized (no string concatenation)
- Struct field order: logical grouping or alphabetical
- Consistent error handling: log + optional return
- Comments for non-obvious logic only

### Test Coverage Expectations
- Unit tests written BEFORE implementation
- Integration tests cover both formats (include param)
- Tests verify both happy path and edge cases
- Error cases documented (bad input, missing data, etc.)

### Documentation
- No code comments needed for straightforward logic
- Method signatures are self-documenting
- SQL field names should match Go struct tags where possible

---

## Review Completion Criteria

This review will be completed and approved when:
1. ✓ All code changes implemented per specification
2. ✓ All unit and integration tests pass
3. ✓ No new linting violations
4. ✓ Backward compatibility preserved (no breaking API changes)
5. ✓ Storage schema matches Go structs
6. ✓ Handler logic doesn't fail on auth context extraction

**Next Steps:** Awaiting task #1 completion
