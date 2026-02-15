# Requirements: User Storage API (Phase 1 — Migration from Portal BadgerDB)

**Date:** 2026-02-15
**Requested:** Move user data storage from vire-portal's BadgerDB to vire-server, exposing REST endpoints for user CRUD, authentication, and bulk import. This is Phase 1 of the migration plan in vire-portal/docs/migration-remove-badger.md.

## Scope

### In scope
- User model (`internal/models/user.go`) with username, email, password_hash, role, navexa_key
- UserStorage interface and file-based implementation (keyed by username, stored in `data/user/users/`)
- REST endpoints: POST/GET/PUT/DELETE `/api/users/{id}`, POST `/api/users/import`
- Auth endpoints: POST `/api/auth/login` (verify credentials, return user data), POST `/api/auth/validate` (placeholder)
- Password hashing with bcrypt (moved from portal's importer)
- Enhance `userContextMiddleware` to resolve navexa_key from user storage when `X-Vire-User-ID` is present (Option B from migration plan)
- User lookups must be performant — accessed every request via middleware
- Unit tests for all new code
- Config additions for `[auth]` section (jwt_secret, token_expiry — placeholder for Phase 2)

### Out of scope
- JWT token generation/validation (stays in portal for now — Phase 2)
- Portal code changes (Phase 2-3)
- OAuth integration
- In-memory caching layer (file reads are fast enough for iteration 1; optimize later if needed)
- Per-user data isolation (namespacing portfolios by user — separate feature)

## Approach

### User Model
New file `internal/models/user.go`:
```go
type User struct {
    Username     string `json:"username"`
    Email        string `json:"email"`
    PasswordHash string `json:"password_hash"`
    Role         string `json:"role"`
    NavexaKey    string `json:"navexa_key,omitempty"`
}
```
- Password is stored as bcrypt hash, never returned in API responses
- NavexaKey is stored in plaintext (same as portal), never returned in GET (only `navexa_key_set` bool + `navexa_key_preview` last-4)
- Username is the primary key (matches portal's badgerhold key)

### UserStorage Interface
Add to `internal/interfaces/storage.go`:
```go
type UserStorage interface {
    GetUser(ctx context.Context, username string) (*models.User, error)
    SaveUser(ctx context.Context, user *models.User) error
    DeleteUser(ctx context.Context, username string) error
    ListUsers(ctx context.Context) ([]string, error)
}
```

### File-based Implementation
Add to `internal/storage/file.go` following the exact same pattern as `portfolioStorage`:
- Directory: `users/` under userStore
- Key: username
- Versioned: false (user records don't need version history)

### Storage Manager
Add `UserStorage()` accessor to `StorageManager` interface and `Manager` struct.

### REST Handlers
New file `internal/server/handlers_user.go`:

| Endpoint | Method | Handler | Notes |
|----------|--------|---------|-------|
| `/api/users` | POST | handleUserCreate | Creates user, bcrypt hash password |
| `/api/users/{id}` | GET | handleUserGet | Returns user without password/navexa_key |
| `/api/users/{id}` | PUT | handleUserUpdate | Merge semantics (only update provided fields) |
| `/api/users/{id}` | DELETE | handleUserDelete | Remove user |
| `/api/users/import` | POST | handleUserImport | Bulk import with bcrypt, skip existing |
| `/api/auth/login` | POST | handleAuthLogin | Verify username + password, return user data |

Response format follows existing pattern:
```json
{"status": "ok", "data": {...}}
```

Error format uses existing `WriteError`:
```json
{"error": "message"}
```

### Middleware Enhancement
Update `userContextMiddleware` in `internal/server/middleware.go`:
- When `X-Vire-User-ID` header is present and `X-Vire-Navexa-Key` is absent
- Look up user from UserStorage
- If found and user has NavexaKey, inject it into UserContext
- This implements Option B from migration plan — server resolves the key internally
- Requires access to UserStorage from middleware (pass via Server struct)

### Routes
Register in `internal/server/routes.go`:
```go
mux.HandleFunc("/api/users/import", s.handleUserImport)
mux.HandleFunc("/api/users/", s.routeUsers)
mux.HandleFunc("/api/users", s.handleUserCreate)
mux.HandleFunc("/api/auth/login", s.handleAuthLogin)
```

### Config
No config changes needed for iteration 1. Auth config (jwt_secret, token_expiry) deferred to Phase 2 when JWT moves to server.

## Files Expected to Change

| File | Change |
|------|--------|
| `internal/models/user.go` | New — User struct |
| `internal/interfaces/storage.go` | Add UserStorage interface, add UserStorage() to StorageManager |
| `internal/storage/file.go` | Add userStorage implementation |
| `internal/storage/manager.go` | Add UserStorage() accessor and initialization |
| `internal/server/handlers_user.go` | New — user CRUD + auth handlers |
| `internal/server/routes.go` | Register new routes |
| `internal/server/middleware.go` | Enhance userContextMiddleware to resolve navexa_key from user storage |
| `internal/server/server.go` | Possibly expose UserStorage for middleware access |
| `go.mod` / `go.sum` | Add `golang.org/x/crypto` for bcrypt |

## Performance Considerations
- User lookups happen every request via middleware (when X-Vire-User-ID present)
- File-based storage uses `os.ReadFile` + `json.Unmarshal` — fast for small JSON files
- Each user file is ~200 bytes — sub-millisecond reads on any storage
- If latency becomes an issue, add sync.Map cache with TTL in a later iteration
