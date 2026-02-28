# Admin API

Endpoints in `internal/server/handlers_admin.go`. Protected by `requireAdmin()` (checks UserContext.Role, falls back to DB lookup).

## Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/admin/users` | GET | List all users (no password hashes) |
| `/api/admin/users/{id}/role` | PATCH | Update role (validates, prevents self-demotion) |
| `/api/admin/jobs` | GET | List jobs (?ticker=, ?status=pending, ?limit=) |
| `/api/admin/jobs/queue` | GET | Pending jobs by priority with count |
| `/api/admin/jobs/enqueue` | POST | Manual enqueue ({job_type, ticker, priority}) |
| `/api/admin/jobs/{id}/priority` | PUT | Set priority (number or "top") |
| `/api/admin/jobs/{id}/cancel` | POST | Cancel pending/running job |
| `/api/admin/stock-index` | GET | List all stock index entries |
| `/api/admin/stock-index` | POST | Add/upsert stock index entry |
| `/api/admin/migrate-cashflow` | POST | Migrate legacy ledger to account-based format (?portfolio_name=X) |
| `/api/admin/ws/jobs` | GET | WebSocket for real-time job events |

Route dispatch: `/api/admin/jobs/{id}/*` via `routeAdminJobs`, `/api/admin/users/{id}/*` via `routeAdminUsers`.

## Stock Index

Shared, user-agnostic registry in SurrealDB (`stock_index` table). Populated by:
- Portfolio sync (source "navexa")
- Admin API (source "manual")

Job manager watcher scans periodically and enqueues for stale components.
