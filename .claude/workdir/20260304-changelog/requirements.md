# Requirements: Changelog System

**Date:** 2026-03-04
**Template:** FeedbackStore pattern (feedbackstore.go, handlers_feedback.go)

## Scope

**IN:**
- Changelog model with ID, service, version/build, markdown content, timestamps, author tracking
- SurrealDB store with CRUD, paginated listing
- HTTP handlers with auth guards (POST = admin or service, PATCH/DELETE = admin, GET = anyone)
- MCP tool catalog entries (4 tools)
- Route registration
- StorageManager integration (ChangelogStore accessor)
- Schema version bump 14 → 15

**OUT:**
- No notification system
- No changelog diffing or versioning of individual entries
- No categories/tags on entries (keep it simple)

## Model

**File:** `internal/models/changelog.go` (NEW)

Follow `internal/models/feedback.go` pattern.

```go
package models

import "time"

// ChangelogEntry represents a single changelog entry for a service.
type ChangelogEntry struct {
	ID             string    `json:"id"`
	Service        string    `json:"service"`                   // e.g. "vire-server", "vire-portal"
	ServiceVersion string    `json:"service_version"`           // e.g. "0.3.153"
	ServiceBuild   string    `json:"service_build,omitempty"`   // e.g. "2026-03-04-14-30-00"
	Content        string    `json:"content"`                   // markdown body
	CreatedByID    string    `json:"created_by_id,omitempty"`   // user/service ID
	CreatedByName  string    `json:"created_by_name,omitempty"` // display name
	UpdatedByID    string    `json:"updated_by_id,omitempty"`
	UpdatedByName  string    `json:"updated_by_name,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
```

No constants or validation maps needed — service is free-form text, content is markdown.

## Store Interface

**File:** `internal/interfaces/storage.go` — add after FeedbackStore (line ~139)

Follow `FeedbackStore` interface pattern (lines 131-139).

```go
// ChangelogStore manages changelog entries.
type ChangelogStore interface {
	Create(ctx context.Context, entry *models.ChangelogEntry) error
	Get(ctx context.Context, id string) (*models.ChangelogEntry, error)
	List(ctx context.Context, opts ChangelogListOptions) ([]*models.ChangelogEntry, int, error)
	Update(ctx context.Context, entry *models.ChangelogEntry) error
	Delete(ctx context.Context, id string) error
}

// ChangelogListOptions configures filtering and pagination for changelog queries.
type ChangelogListOptions struct {
	Service string // filter by service name
	Page    int
	PerPage int
}
```

**StorageManager interface** — add `ChangelogStore() ChangelogStore` accessor after `FeedbackStore()` (line ~21).

## Store Implementation

**File:** `internal/storage/surrealdb/changelogstore.go` (NEW)

Follow `feedbackstore.go` exactly. Key patterns to replicate:

1. **Table name:** `changelog`
2. **ID format:** `"cl_" + uuid.New().String()[:8]`
3. **Select fields alias:** `changelog_id as id` (same pattern as `feedback_id as id`)
4. **Record ID:** `surrealmodels.NewRecordID("changelog", entry.ID)`
5. **Create:** UPSERT with auto-ID, auto-timestamps (follow feedbackstore.go:34-84)
6. **Get:** SELECT by record ID, return nil if not found (follow feedbackstore.go:86-104)
7. **List:** WHERE clause building, count query + data query, pagination (follow feedbackstore.go:106-201)
   - Default sort: `ORDER BY created_at DESC, changelog_id DESC`
   - Only filter: `service` (optional)
   - Pagination: page/perPage with defaults 1/20, max 100
8. **Update:** Merge semantics — only update provided non-empty fields. Different from feedback Update which only handles status/notes. Use this pattern:
   ```go
   func (s *ChangelogStore) Update(ctx context.Context, entry *models.ChangelogEntry) error {
       // Build SET clauses dynamically for non-empty fields
       sets := []string{"updated_at = $now"}
       vars := map[string]any{
           "rid": surrealmodels.NewRecordID("changelog", entry.ID),
           "now": time.Now(),
       }
       if entry.Service != "" {
           sets = append(sets, "service = $service")
           vars["service"] = entry.Service
       }
       if entry.ServiceVersion != "" {
           sets = append(sets, "service_version = $service_version")
           vars["service_version"] = entry.ServiceVersion
       }
       if entry.ServiceBuild != "" {
           sets = append(sets, "service_build = $service_build")
           vars["service_build"] = entry.ServiceBuild
       }
       if entry.Content != "" {
           sets = append(sets, "content = $content")
           vars["content"] = entry.Content
       }
       if entry.UpdatedByID != "" {
           sets = append(sets, "updated_by_id = $updated_by_id")
           vars["updated_by_id"] = entry.UpdatedByID
       }
       if entry.UpdatedByName != "" {
           sets = append(sets, "updated_by_name = $updated_by_name")
           vars["updated_by_name"] = entry.UpdatedByName
       }
       sql := "UPDATE $rid SET " + strings.Join(sets, ", ")
       // ... execute
   }
   ```
9. **Delete:** Same as feedbackstore.go:242-248
10. **Compile-time check:** `var _ interfaces.ChangelogStore = (*ChangelogStore)(nil)`

**Imports:** Same as feedbackstore.go + `"strings"` for Update.

## Manager Integration

**File:** `internal/storage/surrealdb/manager.go`

1. Add field: `changelogStore *ChangelogStore` (after `feedbackStore` at line 26)
2. Add table to define list: `"changelog"` (line 54, in the tables slice)
3. Init store: `m.changelogStore = NewChangelogStore(db, logger)` (after feedbackStore init at line 84)
4. Add accessor method:
   ```go
   func (m *Manager) ChangelogStore() interfaces.ChangelogStore {
       return m.changelogStore
   }
   ```
   (after FeedbackStore() at line 127)

## HTTP Handlers

**File:** `internal/server/handlers_changelog.go` (NEW)

Follow `handlers_feedback.go` patterns exactly.

### handleChangelogRoot — GET/POST /api/changelog

```go
func (s *Server) handleChangelogRoot(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodGet:
        s.handleChangelogList(w, r)
    case http.MethodPost:
        s.handleChangelogCreate(w, r)
    default:
        w.Header().Set("Allow", "GET, POST")
        WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
    }
}
```

### routeChangelog — /api/changelog/{id}

```go
func (s *Server) routeChangelog(w http.ResponseWriter, r *http.Request) {
    id := strings.TrimPrefix(r.URL.Path, "/api/changelog/")
    if id == "" {
        s.handleChangelogRoot(w, r)
        return
    }
    switch r.Method {
    case http.MethodGet:
        s.handleChangelogGet(w, r, id)
    case http.MethodPatch:
        s.handleChangelogUpdate(w, r, id)
    case http.MethodDelete:
        s.handleChangelogDelete(w, r, id)
    default:
        w.Header().Set("Allow", "GET, PATCH, DELETE")
        WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
    }
}
```

### handleChangelogCreate — POST /api/changelog

**Auth:** `requireAdminOrService(w, r)` — admins and portal service user can create.

Request body:
```go
var body struct {
    Service        string `json:"service"`
    ServiceVersion string `json:"service_version"`
    ServiceBuild   string `json:"service_build"`
    Content        string `json:"content"`
}
```

Validation:
- `service` required, max 100 chars
- `content` required (markdown), max 50000 chars
- `service_version` optional, max 50 chars
- `service_build` optional, max 50 chars

Capture creator identity from UserContext (follow handlers_feedback.go:115-121):
```go
if uc := common.UserContextFromContext(ctx); uc != nil && strings.TrimSpace(uc.UserID) != "" {
    entry.CreatedByID = strings.TrimSpace(uc.UserID)
    if user, err := s.app.Storage.InternalStore().GetUser(ctx, entry.CreatedByID); err == nil && user != nil {
        entry.CreatedByName = user.Name
    }
}
```

Response: HTTP 201, `{"id": "cl_xxxx", "entry": <full entry>}`

### handleChangelogList — GET /api/changelog

**Auth:** None (anyone can list).

Query params:
- `service` — filter by service name
- `page` — page number (default 1)
- `per_page` — items per page (default 20, max 100)

Follow handlers_feedback.go:137-210 pagination pattern exactly. Response:
```json
{"items": [...], "total": N, "page": 1, "per_page": 20, "pages": N}
```

### handleChangelogGet — GET /api/changelog/{id}

**Auth:** None. Follow handlers_feedback.go:231-246.

### handleChangelogUpdate — PATCH /api/changelog/{id}

**Auth:** `requireAdmin(w, r)` — only admins can edit.

Request body (all optional, merge semantics):
```go
var body struct {
    Service        string `json:"service"`
    ServiceVersion string `json:"service_version"`
    ServiceBuild   string `json:"service_build"`
    Content        string `json:"content"`
}
```

- Verify entry exists first (return 404 if not)
- Capture updater identity in UpdatedByID/UpdatedByName
- Apply same validation limits as create
- Response: HTTP 200, updated entry

### handleChangelogDelete — DELETE /api/changelog/{id}

**Auth:** `requireAdmin(w, r)` — only admins can delete.
Follow handlers_feedback.go:358-372 exactly. Response: HTTP 204 No Content.

## Routes

**File:** `internal/server/routes.go`

Add after feedback routes (lines 119-120):

```go
// Changelog
mux.HandleFunc("/api/changelog/", s.routeChangelog)
mux.HandleFunc("/api/changelog", s.handleChangelogRoot)
```

## MCP Tool Catalog

**File:** `internal/server/catalog.go`

Add 4 tools after the feedback section. Follow existing naming convention (`changelog_list`, `changelog_add`, `changelog_update`, `changelog_delete`).

```go
// --- Changelog ---
{
    Name:        "changelog_list",
    Description: "List changelog entries, ordered by newest first. Supports pagination and optional service filter.",
    Method:      "GET",
    Path:        "/api/changelog",
    Params: []models.ParamDefinition{
        {Name: "service", Type: "string", Description: "Filter by service name (e.g. 'vire-server', 'vire-portal')", In: "query"},
        {Name: "page", Type: "number", Description: "Page number (default: 1)", In: "query"},
        {Name: "per_page", Type: "number", Description: "Items per page (default: 20, max: 100)", In: "query"},
    },
},
{
    Name:        "changelog_add",
    Description: "Add a changelog entry. Admin or service user required. Content is markdown with date, service, and version info.",
    Method:      "POST",
    Path:        "/api/changelog",
    Params: []models.ParamDefinition{
        {Name: "service", Type: "string", Description: "Service name (e.g. 'vire-server', 'vire-portal')", In: "body", Required: true},
        {Name: "service_version", Type: "string", Description: "Service version (e.g. '0.3.153')", In: "body"},
        {Name: "service_build", Type: "string", Description: "Service build timestamp", In: "body"},
        {Name: "content", Type: "string", Description: "Changelog content in markdown format", In: "body", Required: true},
    },
},
{
    Name:        "changelog_update",
    Description: "Update a changelog entry by ID. Admin access required. Uses merge semantics — only provided fields are changed.",
    Method:      "PATCH",
    Path:        "/api/changelog/{id}",
    Params: []models.ParamDefinition{
        {Name: "id", Type: "string", Description: "Changelog entry ID (e.g. 'cl_1a2b3c4d')", In: "path", Required: true},
        {Name: "service", Type: "string", Description: "Updated service name", In: "body"},
        {Name: "service_version", Type: "string", Description: "Updated service version", In: "body"},
        {Name: "service_build", Type: "string", Description: "Updated service build", In: "body"},
        {Name: "content", Type: "string", Description: "Updated markdown content", In: "body"},
    },
},
{
    Name:        "changelog_delete",
    Description: "Delete a changelog entry by ID. Admin access required.",
    Method:      "DELETE",
    Path:        "/api/changelog/{id}",
    Params: []models.ParamDefinition{
        {Name: "id", Type: "string", Description: "Changelog entry ID (e.g. 'cl_1a2b3c4d')", In: "path", Required: true},
    },
},
```

## Schema Version

**File:** `internal/common/version.go` — line 15

Change: `const SchemaVersion = "14"` → `const SchemaVersion = "15" // changelog`

## Unit Tests

**File:** `internal/storage/surrealdb/changelogstore_test.go` (NEW)

Follow the pattern in `feedbackstore_test.go` if it exists, or write standalone tests. Test:

1. `TestChangelogCreate` — creates entry, verifies ID prefix "cl_", timestamps set
2. `TestChangelogGet` — create then get, verify all fields
3. `TestChangelogGetNotFound` — returns nil, nil for missing ID
4. `TestChangelogList` — create 3 entries, list with default pagination, verify order (newest first)
5. `TestChangelogListFilterByService` — create entries for 2 services, filter by one
6. `TestChangelogListPagination` — create 5 entries, list with per_page=2, verify page counts
7. `TestChangelogUpdate` — create, update content and service_version, verify merge semantics
8. `TestChangelogUpdateNotFound` — update non-existent ID (no error, no-op — SurrealDB behavior)
9. `TestChangelogDelete` — create, delete, verify get returns nil
10. `TestChangelogDeleteNotFound` — delete non-existent ID (no error)

These are SurrealDB integration tests requiring a live DB. If no DB connection is available, skip with `t.Skip("SurrealDB not available")`.

**However** — since this is a SurrealDB-dependent test, the implementer should write a mock-based unit test in the handler file or alongside. The integration tests go to `tests/data/`.

## Handler Tests (via integration tests)

**File:** `tests/data/changelog_test.go` (NEW)

Follow `tests/data/trade_test.go` pattern. Read `.claude/skills/test-common/SKILL.md` and `.claude/skills/test-create-review/SKILL.md`.

Test cases:
1. `TestChangelogCreate` — POST /api/changelog with admin auth, verify 201
2. `TestChangelogCreateAsService` — POST /api/changelog with service user auth, verify 201
3. `TestChangelogCreateUnauthorized` — POST /api/changelog without auth, verify 401/403
4. `TestChangelogList` — GET /api/changelog, verify paginated response
5. `TestChangelogListFilterService` — GET /api/changelog?service=vire-server
6. `TestChangelogGet` — GET /api/changelog/{id}
7. `TestChangelogGetNotFound` — GET /api/changelog/nonexistent, verify 404
8. `TestChangelogUpdate` — PATCH /api/changelog/{id} with admin, verify merge
9. `TestChangelogUpdateUnauthorized` — PATCH without admin, verify 401/403
10. `TestChangelogDelete` — DELETE /api/changelog/{id} with admin, verify 204
11. `TestChangelogDeleteUnauthorized` — DELETE without admin, verify 401/403

## Files Summary

| File | Action | Description |
|------|--------|-------------|
| `internal/models/changelog.go` | NEW | ChangelogEntry struct |
| `internal/interfaces/storage.go` | MODIFY | Add ChangelogStore interface + ChangelogListOptions + StorageManager accessor |
| `internal/storage/surrealdb/changelogstore.go` | NEW | SurrealDB implementation |
| `internal/storage/surrealdb/manager.go` | MODIFY | Add changelogStore field, table define, init, accessor |
| `internal/server/handlers_changelog.go` | NEW | HTTP handlers with auth guards |
| `internal/server/routes.go` | MODIFY | Add changelog route registration |
| `internal/server/catalog.go` | MODIFY | Add 4 MCP tool definitions |
| `internal/common/version.go` | MODIFY | Schema 14 → 15 |
| `tests/data/changelog_test.go` | NEW | Integration tests |
