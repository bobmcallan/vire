---
name: vire-architect
description: Vire-specific architectural guardrails derived from the actual codebase. Apply this skill whenever writing, reviewing, modifying, or debugging Vire code — including small fixes, bug investigations, feature additions, data validation, and refactors. This skill encodes Vire's real patterns (handler → service → storage, interface-driven DI, context-based multi-tenancy) so violations are caught before they're introduced. Triggers on any task that touches vire-server source code.
---

# Vire Architect

Mandatory architectural rules for the Vire codebase. Derived from real patterns — every rule references actual files and functions.

## 1. Handler Pattern

Handlers are receiver methods on `Server` in `internal/server/handlers*.go`. They follow a strict sequence:

```
1. RequireMethod(w, r, http.MethodGet)     — validate HTTP method
2. Extract params (path, query, body)       — parse input
3. s.app.XxxService.Method(ctx, ...)       — call service (one call)
4. Post-process response (strip fields)     — filter output
5. WriteJSON(w, statusCode, result)         — return response
```

**Handlers MUST NOT:**
- Contain business logic or calculations
- Write to storage directly
- Call external APIs (Navexa, EODHD, Gemini)
- Validate business rules (that's the service's job)
- Iterate over domain data to compute derived values

**Handlers MAY:**
- Strip sensitive fields before returning (e.g. `portfolio.Holdings[i].Trades = nil`)
- Attach optional non-fatal metadata (e.g. capital performance, advisory fields)
- Compose responses from multiple service calls when the handler is an aggregation endpoint

**Reference**: `internal/server/handlers.go:111-168` (handlePortfolioGet) is the canonical example.

**Violation pattern** — handler computing a value:
```go
// BAD: handler iterates holdings to calculate weight
totalValue := 0.0
for _, h := range portfolio.Holdings {
    totalValue += h.MarketValue
}
h.Weight = h.MarketValue / totalValue

// GOOD: service already computed it
portfolio, _ := s.app.PortfolioService.SyncPortfolio(ctx, name, false)
// portfolio.Holdings[i].Weight is pre-computed
```

## 2. Service Ownership

Each service owns its domain's business logic. No other layer may reimplement it.

| Service | Owns | Key File |
|---------|------|----------|
| **PortfolioService** | Portfolio aggregation, growth timelines, reviews, watchlist analysis | `internal/services/portfolio/service.go` |
| **TradeService** | Trade CRUD, derived holdings computation | `internal/services/trade/service.go` |
| **MarketService** | EOD collection, fundamentals, news, signals, screening | `internal/services/market/service.go` |
| **CashFlowService** | Cash ledger, capital performance, transaction management | `internal/services/cashflow/service.go` |
| **QuoteService** | Real-time quotes (EODHD with ASX fallback) | `internal/services/quote/service.go` |
| **ReportService** | AI-generated portfolio/ticker reports | `internal/services/report/service.go` |

**Check for:**
- Duplicated calculation loops across packages (e.g. growth.go iterating cash transactions instead of calling CashFlowService)
- Business logic in handlers that belongs in the owning service
- Raw data iteration outside the owning service
- Multiple code paths computing the same derived value

**Fix:** Expose a function on the owning service. Have all consumers call it.

## 3. Dependency Injection

Services use two injection patterns to avoid circular dependencies:

**Constructor injection** — hard dependencies (storage, external clients):
```go
func NewService(
    storage interfaces.StorageManager,
    navexa  interfaces.NavexaClient,
    eodhd   interfaces.EODHDClient,
    logger  *common.Logger,
) *Service
```

**Setter injection** — soft dependencies between services:
```go
func (s *Service) SetCashFlowService(svc interfaces.CashFlowService)
func (s *Service) SetTradeService(svc interfaces.TradeService)
```

**Wiring happens in**: `internal/app/app.go` (NewApp constructor).

**Rules:**
- All service dependencies go through interfaces (`internal/interfaces/services.go`)
- Never import a concrete service type from another service package
- Never add a new constructor parameter when a setter would avoid a circular import
- All new services must implement an interface in `internal/interfaces/`

## 4. Interface-First Design

All major components are abstracted behind interfaces in `internal/interfaces/`:

| File | Interfaces |
|------|-----------|
| `services.go` | PortfolioService, MarketService, TradeService, CashFlowService, QuoteService, ReportService, etc. |
| `storage.go` | StorageManager, UserDataStore, MarketDataStorage, SignalStorage, TimelineStore, InternalStore |
| `clients.go` | EODHDClient, NavexaClient, GeminiClient, ASXClient |

**Rules:**
- New services, stores, or clients MUST have an interface in `internal/interfaces/`
- Consumers depend on the interface, never the concrete type
- Test code depends on interfaces for mocking

## 5. Context-Based Multi-Tenancy

User identity is resolved at the middleware layer and carried through context — not passed as function parameters.

**Flow:**
```
bearerTokenMiddleware → validates JWT, loads user from InternalStore
    → userContextMiddleware → resolves preferences from KV (navexa_key, display_currency, portfolios)
        → sets common.UserContext in request context
            → handlers/services call common.UserContextFromContext(ctx)
```

**Key types:**
- `common.UserContext` (`internal/common/userctx.go`) — UserID, Role, Portfolios, DisplayCurrency, NavexaAPIKey
- `common.ResolveUserID(ctx)` — returns UserID or "default" (single-tenant fallback)

**Rules:**
- Never pass user ID as a function parameter when it's available from context
- Services that need the Navexa client must resolve it from context via `resolveNavexaClient(ctx)` — the per-user API key is in the context
- New handler endpoints that need auth MUST rely on the middleware chain, not validate tokens themselves

## 6. Middleware Stack

Applied in `internal/server/middleware.go:340-349`, executed in this order on each request:

```
Recovery → CORS → Bearer Token → User Context → Correlation ID → Logging
```

**Rules:**
- Never bypass the middleware chain for new endpoints
- Auth validation happens in middleware, not in handlers
- New cross-cutting concerns (rate limiting, request validation) go in middleware, not handlers
- The logging middleware logs 4xx as Info, 5xx as Error — match this convention

## 7. Error Handling

**HTTP layer** (`internal/server/helpers.go`):
```go
WriteError(w, http.StatusNotFound, "portfolio not found")
WriteErrorWithCode(w, 400, "invalid ticker", "INVALID_TICKER")
```

**Service layer**: Return `(result, error)`. The handler decides the HTTP status code.

**Rules:**
- Services return errors — they never set HTTP status codes
- Handlers translate service errors to appropriate HTTP status (400/404/500)
- Use `WriteError` or `WriteErrorWithCode` — never write raw JSON error responses
- Non-fatal enrichment failures (advisory fields, capital performance) are logged and skipped, not returned as errors

## 8. Storage Pattern

All persistence goes through `interfaces.StorageManager`, which coordinates:

| Store | Purpose | Key Methods |
|-------|---------|-------------|
| `UserDataStore` | Generic record CRUD (portfolios, trades, plans, strategies) | Get, Put, Delete, List, Query |
| `MarketDataStorage` | Ticker EOD bars, fundamentals | GetMarketData, SaveMarketData |
| `SignalStorage` | Technical indicators | GetSignals, SaveSignals |
| `TimelineStore` | Historical portfolio snapshots | Get/Set/Delete snapshots |
| `InternalStore` | User accounts, user KV, system KV | CRUD users, KV get/set |
| `BlobStore` | Large binary data (filing PDFs, reports) | Get, Put, Delete |

**Implementation**: SurrealDB (`internal/storage/surrealdb/`).

**Rules:**
- Services access storage through `interfaces.StorageManager` — never through concrete SurrealDB types
- New data types that need persistence must use an existing store interface or extend one
- Schema version lives in `internal/common/version.go` — bump it when model changes invalidate cached data

## 9. Configuration

**File**: `internal/common/config.go`
**Format**: TOML (`config/vire-service.toml`) with env var overrides.
**Resolution**: CLI arg → `VIRE_CONFIG` env → `config/vire-service.toml` → defaults.

**Rules:**
- New config fields go in the `Config` struct with a TOML tag
- Secrets (API keys, JWT secret) support env var override
- Relative paths are resolved against the binary directory in `app.go`
- API keys are resolved via `ResolveAPIKey()` (env → KV store → config fallback)

## 10. No Legacy Compatibility

There is 1 server and 1 portal. When a format changes, change it everywhere.

**Never introduce:**
- Deprecated type aliases or backward-compatible shims
- Old-format unmarshallers or dual-format readers
- Compatibility wrappers or migration helpers
- Anything named "legacy", "v1", "old", or "deprecated"

**Flag and remove** any of the above found in existing code.

## 11. Naming Conventions

All field names must follow the canonical naming guide in `.claude/skills/vire-naming/SKILL.md`:
- Pattern: `{domain}_{concept}_{qualifier}` in `snake_case`
- Percentages always suffix `_pct`
- No `cash_*` at portfolio level — use `capital_*`
- Check the migration map for legacy field names that should be replaced

## Applying These Rules

**Before writing code:**
1. Identify which service owns the data you're working with
2. Check if the function you need already exists on that service
3. If not, add it to the owning service — don't compute it in the handler or another service
4. Check `internal/interfaces/` for the interface you need to implement

**Before completing a task:**
1. Verify handlers only parse/call/format — no business logic
2. Grep for duplicated calculation patterns across packages
3. Confirm all new types have interfaces in `internal/interfaces/`
4. Check field names against the naming guide
5. Verify no legacy/compat patterns introduced
6. Update `docs/architecture/` if the system architecture changed
