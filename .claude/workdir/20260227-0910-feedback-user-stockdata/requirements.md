# Requirements: Fix get_stock_data include param + Add user context to feedback

## Feature 1: Fix get_stock_data include parameter parsing (fb_e2584b1a)

**Problem:** `get_stock_data` with `include=[price]` returns no price block. The MCP tool catalog defines `include` as `Type: "array"`, but the handler uses `r.URL.Query().Get("include")` which only reads the first value. When MCP clients send `include=price&include=fundamentals` (repeated keys), only the first is read. The comma-separated fallback works but the array format does not.

**Fix in `internal/server/handlers.go` lines 547-565:**
- Replace `r.URL.Query().Get("include")` with `r.URL.Query()["include"]`
- Merge both array-style (`?include=price&include=signals`) and comma-separated (`?include=price,signals`) values
- Parse the merged list with the existing switch block

**Before:**
```go
includeParam := r.URL.Query().Get("include")
include := interfaces.StockDataInclude{...}
if includeParam != "" {
    include = interfaces.StockDataInclude{}
    for _, inc := range strings.Split(includeParam, ",") {
```

**After:**
```go
includeParams := r.URL.Query()["include"]
include := interfaces.StockDataInclude{Price: true, Fundamentals: true, Signals: true, News: true}
if len(includeParams) > 0 {
    include = interfaces.StockDataInclude{}
    for _, param := range includeParams {
        for _, inc := range strings.Split(param, ",") {
```

## Feature 2: Add user_id and user_name/email to feedback create/update

**Problem:** Feedback items don't record who submitted or updated them. Need to capture the authenticated user's identity from the existing UserContext middleware.

### Changes required:

**`internal/models/feedback.go`** — Add 3 fields to Feedback struct:
```go
UserID    string `json:"user_id,omitempty"`
UserName  string `json:"user_name,omitempty"`
UserEmail string `json:"user_email,omitempty"`
```

**`internal/storage/surrealdb/feedbackstore.go`**:
- Add `user_id, user_name, user_email` to `feedbackSelectFields`
- Add fields to `Create()` SQL and vars map
- Add user fields to `Update()` SQL — capture who last updated

**`internal/server/handlers_feedback.go`**:
- In `handleFeedbackSubmit()`: Extract UserContext from `r.Context()` via `common.UserContextFromContext()`. If present, set `fb.UserID = uc.UserID`. Look up `InternalUser` via `store.GetUser()` for name/email.
- In `handleFeedbackUpdate()`: Same pattern — record who performed the update by adding `user_id`, `user_name`, `user_email` to the Update call.

**`internal/interfaces/storage.go`** — Update `FeedbackStore.Update()` signature:
```go
Update(ctx context.Context, id string, status, resolutionNotes string, userID, userName, userEmail string) error
```

**`internal/server/catalog.go`** — No changes needed (user context is automatic from auth, not a tool param).

## Files expected to change

| File | Change |
|------|--------|
| `internal/server/handlers.go` | Fix include param parsing (~5 lines) |
| `internal/models/feedback.go` | Add UserID, UserName, UserEmail fields |
| `internal/storage/surrealdb/feedbackstore.go` | Add user fields to SQL, select, Create, Update |
| `internal/server/handlers_feedback.go` | Extract UserContext in submit + update handlers |
| `internal/interfaces/storage.go` | Update FeedbackStore.Update signature |

## Test expectations

- Unit tests: include param parsing, feedback with/without user context
- Integration tests: submit feedback with auth → verify user fields populated
- Integration tests: update feedback with auth → verify updater fields
