# Namespace Separation & Market Binary

## Context

Vire serves two distinct audiences through a single binary (`vire-server`):

1. **Portfolio users** — sync holdings from Navexa (and in future: Sharesight, CSV upload), review positions, manage strategies/plans/watchlists, generate reports. Requires a broker API key.
2. **Stock evaluation users** — real-time quotes, stock data, technical signals, screening, scanning, filing summaries. No portfolio or broker key required.

These domains share a SurrealDB instance and some external clients (EODHD, Gemini) but are otherwise independent. Market code never calls portfolio code. The dependency is one-way: portfolio reads from market data tables for signals and fundamentals.

This plan separates the domains at two levels:

- **SurrealDB namespaces** — market and portfolio data in separate logical containers within the same instance
- **Binary entry points** — a new `cmd/vire-market` that runs only market/stock services, alongside the existing `cmd/vire-server` that runs everything

---

## Goals

1. Run a market-only server with no portfolio dependencies, no Navexa client, no user data tables
2. Keep the full `vire-server` binary unchanged for users who need both domains
3. Same SurrealDB instance, same credentials, separate namespaces
4. No breaking changes to existing deployments — zero-config defaults must work as before
5. Prepare for future portfolio sources (Sharesight, manual CSV) by decoupling from Navexa-specific assumptions

---

## SurrealDB Namespace Layout

### Current (single namespace)

```
SurrealDB Instance (ws://localhost:8000)
└── Namespace: "vire"
    └── Database: "vire"
        ├── user              (accounts)
        ├── user_kv           (per-user config)
        ├── system_kv         (system config, API keys)
        ├── user_data         (portfolios, strategies, watchlists, plans, cashflows)
        ├── market_data       (EOD, fundamentals, filings, news)
        ├── signals           (technical indicators)
        ├── stock_index       (ticker metadata, timestamps)
        ├── job_queue         (background jobs)
        ├── job_runs          (job run history)
        ├── files             (PDFs, charts)
        └── mcp_feedback      (data quality feedback)
```

### Proposed (two namespaces)

```
SurrealDB Instance (ws://localhost:8000)
├── Namespace: "market"
│   └── Database: "market"
│       ├── market_data       (EOD, fundamentals, filings, news)
│       ├── signals           (technical indicators)
│       ├── stock_index       (ticker metadata, timestamps)
│       ├── job_queue         (market collection jobs)
│       ├── job_runs          (job run history)
│       ├── files             (filing PDFs)
│       ├── mcp_feedback      (market-related feedback)
│       └── system_kv         (market config: EODHD key, Gemini key)
│
└── Namespace: "portfolio"
    └── Database: "portfolio"
        ├── user              (accounts)
        ├── user_kv           (per-user config: Navexa key, preferences)
        ├── system_kv         (portfolio config: default portfolio)
        ├── user_data         (portfolios, strategies, watchlists, plans, cashflows)
        ├── mcp_feedback      (portfolio-related feedback)
        └── files             (portfolio charts)
```

### Key decisions

- **`job_queue` stays in market namespace** — all current jobs are market data collection (EOD, filings, signals). Portfolio sync is request-driven, not queued.
- **`mcp_feedback` exists in both** — feedback is submitted per-request; each service writes to its own namespace. The MCP tool routes to whichever service handles the request.
- **`system_kv` exists in both** — market needs EODHD/Gemini keys; portfolio needs default_portfolio and auth secrets. No cross-namespace reads required.
- **`files` exists in both** — market stores filing PDFs; portfolio stores generated charts.

---

## Binary Entry Points

### `cmd/vire-server` (existing, unchanged behaviour)

Runs everything. Connects to both namespaces. This is the default for users who want the full Vire experience.

```
vire-server
├── Market services (MarketService, SignalService, QuoteService)
├── Portfolio services (PortfolioService, StrategyService, PlanService, WatchlistService, CashFlowService)
├── Report service (orchestrates both domains)
├── Job manager (market data collection)
├── Auth (OAuth, JWT)
└── All REST + MCP endpoints
```

### `cmd/vire-market` (new)

Runs market/stock services only. Connects to market namespace only. No auth, no user data, no Navexa.

```
vire-market
├── Market services (MarketService, SignalService, QuoteService)
├── Job manager (market data collection)
├── REST endpoints (market subset only)
└── MCP endpoints (market subset only)
```

**Excluded from vire-market:**
- PortfolioService, StrategyService, PlanService, WatchlistService, CashFlowService, ReportService
- NavexaClient, InjectNavexaClient
- Auth handlers (OAuth, JWT, user management)
- All `/api/portfolios/*` routes
- All portfolio MCP tools

---

## Config Changes

### Current config structure

```toml
[storage]
address   = "ws://localhost:8000/rpc"
namespace = "vire"
database  = "vire"
```

### Proposed config structure

```toml
[storage]
address   = "ws://localhost:8000/rpc"
username  = "root"
password  = "root"

[storage.market]
namespace = "market"
database  = "market"

[storage.portfolio]
namespace = "portfolio"
database  = "portfolio"
```

**Backward compatibility:** If `[storage.market]` and `[storage.portfolio]` are absent, fall back to `storage.namespace` / `storage.database` (current behaviour). Existing configs continue to work — both domains use the same namespace as today.

**`vire-market` binary** only reads `[storage.market]`. If absent, falls back to top-level `storage.namespace`/`storage.database`.

---

## Storage Layer Changes

### StorageManager split

Currently `NewManager()` creates a single SurrealDB connection and initialises all stores on it. The change introduces two connections (or the same connection selecting different namespace/database pairs).

**Option A: Two connections** (recommended)

```go
type Manager struct {
    marketDB    *surrealdb.DB   // connected to market/market
    portfolioDB *surrealdb.DB   // connected to portfolio/portfolio (nil in vire-market)

    // Market stores (always initialised)
    marketDataStore  *MarketDataStore
    signalStore      *SignalStore
    stockIndexStore  *StockIndexStore
    jobQueueStore    *JobQueueStore
    marketFileStore  *FileStore
    marketFeedback   *FeedbackStore
    marketSystemKV   *SystemKVStore

    // Portfolio stores (nil in vire-market)
    internalStore    *InternalStore
    userDataStore    *UserDataStore
    portfolioFileStore *FileStore
    portfolioFeedback  *FeedbackStore
}
```

SurrealDB's Go driver uses a single websocket per `surrealdb.New()` call, and `db.Use(ns, db)` is connection-scoped. Two connections are cleaner than switching namespace/database on a shared connection.

**Option B: Single connection, namespace switching per query**

The SurrealDB Go driver sets namespace/database at connection level via `db.Use()`. Switching per query would require mutex coordination. Not recommended.

### Interface changes

`StorageManager` already exposes typed sub-stores. The interface doesn't change — callers don't know or care which namespace backs each store. The only change is internal wiring in `NewManager()`.

For `vire-market`, portfolio stores return nil. Services that accept nil stores already handle this (e.g., `MarketService` doesn't use `UserDataStore`).

---

## New App Initialiser

### `internal/app/market.go` (new)

A stripped-down `NewMarketApp()` that initialises only market-domain services:

```go
type MarketApp struct {
    Config       *common.Config
    Logger       *common.Logger
    Storage      interfaces.StorageManager  // market-only stores populated
    EODHDClient  interfaces.EODHDClient
    GeminiClient interfaces.GeminiClient
    QuoteService interfaces.QuoteService
    MarketService interfaces.MarketService
    SignalService interfaces.SignalService
    JobManager   *jobmanager.JobManager
    StartupTime  time.Time
}
```

This avoids touching `NewApp()` — the full server continues to work unchanged.

---

## REST API Route Split

### Market-only routes (served by both binaries)

| Route | Handler | Service |
|---|---|---|
| `GET /api/health` | handleHealth | — |
| `GET /api/version` | handleVersion | — |
| `GET /api/config` | handleConfig | — |
| `GET /api/diagnostics` | handleDiagnostics | — |
| `GET /api/mcp/tools` | handleToolCatalog | — |
| `GET /debug/memstats` | handleMemstats | — |
| `GET /api/market/quote/{ticker}` | handleMarketQuote | QuoteService |
| `GET /api/market/stocks/{ticker}` | handleMarketStocks | MarketService |
| `GET /api/market/stocks/{ticker}/filings/{key}` | handleReadFiling | MarketService |
| `POST /api/market/signals` | handleMarketSignals | SignalService |
| `POST /api/market/collect` | handleMarketCollect | MarketService |
| `GET /api/scan/fields` | handleScanFields | MarketService |
| `POST /api/scan` | handleScan | MarketService |
| `POST /api/screen` | handleScreen | MarketService |
| `POST /api/screen/snipe` | handleScreenSnipe | MarketService |
| `POST /api/screen/funnel` | handleScreenFunnel | MarketService |
| `GET /api/strategies/template` | handleStrategyTemplate | — |
| `GET /api/jobs/status` | handleJobStatus | JobManager |
| `GET /api/reports` | handleReportList | — |
| `POST /api/feedback` | handleFeedbackRoot | FeedbackStore |
| `GET /api/feedback` | handleFeedbackRoot | FeedbackStore |

### Portfolio-only routes (served by vire-server only)

| Route | Handler | Service |
|---|---|---|
| `GET /api/portfolios` | handlePortfolioList | PortfolioService |
| `GET /api/portfolios/{name}` | handlePortfolioGet | PortfolioService |
| `POST /api/portfolios/{name}/sync` | handlePortfolioSync | PortfolioService |
| `POST /api/portfolios/{name}/review` | handlePortfolioReview | PortfolioService |
| `POST /api/portfolios/{name}/report` | handlePortfolioReport | ReportService |
| `GET /api/portfolios/{name}/summary` | handlePortfolioSummary | ReportService |
| `GET /api/portfolios/{name}/stock/{ticker}` | handlePortfolioStock | PortfolioService |
| `GET /api/portfolios/{name}/strategy` | handlePortfolioStrategy | StrategyService |
| `GET /api/portfolios/{name}/plan` | handlePortfolioPlan | PlanService |
| `GET /api/portfolios/{name}/watchlist` | handlePortfolioWatchlist | WatchlistService |
| `POST /api/portfolios/{name}/watchlist/review` | handleWatchlistReview | PortfolioService |
| `GET /api/portfolios/{name}/external-balances` | handleExternalBalances | PortfolioService |
| `GET /api/portfolios/{name}/cashflows` | handleCashFlows | CashFlowService |
| `GET /api/portfolios/{name}/indicators` | handlePortfolioIndicators | PortfolioService |
| `POST /api/portfolios/default` | handlePortfolioDefault | — |
| All `/api/auth/*` | auth handlers | AuthService |
| All `/api/users/*` | user handlers | InternalStore |
| All `/api/admin/*` | admin handlers | Various |

### MCP tool split

**Market tools** (available in both binaries):

`get_version`, `get_config`, `get_diagnostics`, `get_quote`, `get_stock_data`, `read_filing`, `compute_indicators`, `market_scan`, `market_scan_fields`, `strategy_scanner`, `stock_screen`, `list_reports`, `get_strategy_template`, `submit_feedback`, `get_feedback`, `update_feedback`

**Portfolio tools** (vire-server only):

`list_portfolios`, `set_default_portfolio`, `get_portfolio`, `get_portfolio_stock`, `portfolio_compliance`, `generate_report`, `get_summary`, `get_portfolio_indicators`, `get_external_balances`, `set_external_balances`, `add_external_balance`, `remove_external_balance`, `list_cash_transactions`, `add_cash_transaction`, `update_cash_transaction`, `remove_cash_transaction`, `get_capital_performance`, `get_portfolio_watchlist`, `set_portfolio_watchlist`, `add_watchlist_item`, `update_watchlist_item`, `remove_watchlist_item`, `review_watchlist`, `get_portfolio_strategy`, `set_portfolio_strategy`, `delete_portfolio_strategy`, `get_portfolio_plan`, `set_portfolio_plan`, `add_plan_item`, `update_plan_item`, `remove_plan_item`, `check_plan_status`, `list_users`, `update_user_role`

---

## Cross-Domain Reads (Portfolio reading Market data)

When `vire-server` runs both domains, `PortfolioService.ReviewPortfolio()` reads from `MarketDataStorage` and `SignalStorage`. These stores are backed by the market namespace.

The full-server `StorageManager` connects to both namespaces. Market stores use the market connection; portfolio stores use the portfolio connection. `ReviewPortfolio()` calls `storage.MarketDataStorage()` which transparently reads from the market namespace. No cross-namespace SurrealDB queries — just two Go-level connections.

```
vire-server StorageManager
├── marketDB connection → market/market
│   ├── MarketDataStorage
│   ├── SignalStorage
│   ├── StockIndexStore
│   ├── JobQueueStore
│   └── FileStore (market)
│
└── portfolioDB connection → portfolio/portfolio
    ├── InternalStore
    ├── UserDataStore
    └── FileStore (portfolio)
```

---

## Data Migration

Existing single-namespace deployments need a one-time migration to split data into two namespaces.

### Migration strategy

1. **Create new namespaces** — `market/market` and `portfolio/portfolio`
2. **Copy tables** — SurrealQL `INSERT INTO` from old namespace to new
3. **Verify counts** — compare record counts per table
4. **Update config** — switch to new `[storage.market]` / `[storage.portfolio]` config
5. **Drop old namespace** — once verified

### Migration script (SurrealQL)

```sql
-- Run against the SurrealDB instance

-- 1. Create namespaces and databases
DEFINE NAMESPACE IF NOT EXISTS market;
DEFINE NAMESPACE IF NOT EXISTS portfolio;
USE NS market DB market;
DEFINE TABLE market_data; DEFINE TABLE signals; DEFINE TABLE stock_index;
DEFINE TABLE job_queue; DEFINE TABLE job_runs; DEFINE TABLE files;
DEFINE TABLE mcp_feedback; DEFINE TABLE system_kv;

USE NS portfolio DB portfolio;
DEFINE TABLE user; DEFINE TABLE user_kv; DEFINE TABLE system_kv;
DEFINE TABLE user_data; DEFINE TABLE mcp_feedback; DEFINE TABLE files;

-- 2. Copy market data (run from old namespace)
-- This requires scripting — SurrealDB doesn't support cross-namespace INSERT directly.
-- Use the Go migration tool (see below).
```

### Go migration tool

A `cmd/vire-migrate` binary that:
1. Connects to old namespace, reads all records from each table
2. Connects to new namespace, writes records
3. Reports counts and any errors
4. Idempotent (safe to re-run)

---

## Implementation Phases

### Phase 1 — Config + storage split (no new binary yet)

1. Add `[storage.market]` and `[storage.portfolio]` config sections with backward-compatible defaults
2. Refactor `StorageManager` to accept two namespace configs
3. Create two SurrealDB connections in `NewManager()` when separate namespaces are configured
4. Wire stores to appropriate connections
5. Existing `vire-server` works unchanged — both connections, both store sets populated
6. Write migration tool (`cmd/vire-migrate`)

**Verification:** `vire-server` works identically with either single-namespace or dual-namespace config. All existing tests pass.

### Phase 2 — Market app + binary

1. Create `internal/app/market.go` with `NewMarketApp()`
2. Create `internal/server/market_server.go` with market-only route registration
3. Create `cmd/vire-market/main.go`
4. Market binary connects to market namespace only, registers market routes only
5. Tool catalog returns market-only tools when running as `vire-market`

**Verification:** `vire-market` starts, serves market endpoints, returns 404 for portfolio routes. `vire-server` unchanged.

### Phase 3 — Admin routes for market binary

1. Add market-relevant admin routes to `vire-market` (job queue management, stock index)
2. Ensure job manager works with market-only storage
3. Add `/api/admin/jobs/*` routes to market server

### Phase 4 — Future portfolio sources

Once the split is in place, adding new portfolio sources (Sharesight, CSV upload) only touches the portfolio namespace and `vire-server`. The market binary and namespace are completely unaffected.

---

## Files Modified/Created

| File | Change |
|---|---|
| `internal/common/config.go` | Add `StorageMarketConfig`, `StoragePortfolioConfig` to `StorageConfig` |
| `internal/storage/surrealdb/manager.go` | Accept dual namespace config, create two DB connections |
| `internal/storage/manager.go` | Pass through new config |
| `internal/app/market.go` | **New** — `MarketApp` struct and `NewMarketApp()` |
| `internal/server/market_server.go` | **New** — market-only server with route subset |
| `internal/server/catalog.go` | Split tool catalog into market/portfolio subsets |
| `cmd/vire-market/main.go` | **New** — market binary entry point |
| `cmd/vire-migrate/main.go` | **New** — namespace migration tool |
| `config/vire-service.toml.example` | Add `[storage.market]`, `[storage.portfolio]` sections |
| `config/vire-market.toml.example` | **New** — minimal config for market-only deployment |

---

## Risks & Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Two DB connections doubles SurrealDB resource usage | Low — SurrealDB websocket connections are lightweight | Monitor; single-namespace fallback remains available |
| Migration tool corrupts data | High | Idempotent design, dry-run mode, count verification, backup first |
| Route duplication between binaries | Medium | Extract route registration into composable functions, not copy-paste |
| Tool catalog divergence | Low | Single source of truth with filter flag, not two separate catalogs |
| ReportService needs both namespaces | N/A for vire-market | ReportService excluded from market binary; only available in vire-server |
