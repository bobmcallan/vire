# Summary: Source-Typed Portfolios & Trade Management (Phase 1)

**Status:** completed
**Date:** 2026-03-04
**Schema:** 13 → 14

## Changes

| File | Change |
|------|--------|
| `internal/models/trade.go` | **NEW** — SourceType enum, Trade, TradeBook, SnapshotPosition, DerivedHolding structs |
| `internal/services/trade/service.go` | **NEW** — TradeService: CRUD, position derivation, snapshot import, validation |
| `internal/services/trade/service_test.go` | **NEW** — 62 unit tests (CRUD, derivation, edge cases) |
| `internal/services/trade/service_stress_test.go` | **NEW** — 34 stress tests (validation, concurrency, adversarial inputs) |
| `internal/models/portfolio.go` | Added SourceType to Portfolio + Holding structs |
| `internal/interfaces/services.go` | Added TradeService interface, TradeFilter, CreatePortfolio to PortfolioService |
| `internal/services/portfolio/service.go` | Added SetTradeService, CreatePortfolio, assembleManualPortfolio, assembleSnapshotPortfolio, GetPortfolio routing by source_type |
| `internal/server/handlers_trade.go` | **NEW** — HTTP handlers: portfolio create, trade CRUD, snapshot import |
| `internal/server/routes.go` | Added trade routes + portfolio POST dispatch |
| `internal/server/catalog.go` | Added 6 MCP tool definitions |
| `internal/app/app.go` | Wired TradeService (field, creation, DI injection) |
| `internal/common/version.go` | Schema version 13 → 14 |
| `docs/architecture/26-02-27-services.md` | Added Trade Service section |
| `tests/data/trade_test.go` | **NEW** — 13 data layer integration tests |
| `tests/api/trade_test.go` | **NEW** — 10 API integration tests |
| 3 mock test files | Added CreatePortfolio stub |

## New MCP Tools (6)

| Tool | Method | Path |
|------|--------|------|
| `portfolio_create` | POST | `/api/portfolios` |
| `trade_add` | POST | `/api/portfolios/{name}/trades` |
| `trade_list` | GET | `/api/portfolios/{name}/trades` |
| `trade_update` | PUT | `/api/portfolios/{name}/trades/{id}` |
| `trade_remove` | DELETE | `/api/portfolios/{name}/trades/{id}` |
| `portfolio_snapshot` | POST | `/api/portfolios/{name}/snapshot` |

## Tests

- Unit tests: 62 (service_test.go) — ALL PASS
- Stress tests: 34 (service_stress_test.go) — ALL PASS
- Integration tests: 23 created (13 data + 10 API) — ready for Docker execution
- Full suite: 1,694+/1,698 passing (99.1%, pre-existing failures documented)
- Fix rounds: 1 (validation hardening from devils-advocate)

## Architecture

- TradeService follows CashFlowService pattern (UserDataStore KV, ID generation, full-doc save)
- Position derivation owned by TradeService (average cost basis algorithm)
- Portfolio assembly routes by SourceType: navexa → existing Navexa sync, manual → derive from trades, snapshot → return stored positions
- DI: TradeService injected into PortfolioService via SetTradeService()
- Docs updated: docs/architecture/26-02-27-services.md

## Devils-Advocate

- 10 validation gaps found and fixed (negative price/fees, NaN/Inf, extreme values, string limits, ticker trimming)
- Critical: UpdateTrade negative position bug found and fixed (post-update position replay)
- Concurrent write race documented as known limitation (shared with cashflow pattern)
- All 34 stress tests passing after fixes

## Notes

- Schema bump 13→14 purges cached portfolios/market data on restart
- Manual portfolio current prices default to avg_cost until market data is collected
- Hybrid portfolio assembly deferred to Phase 2
- CSV import deferred to Phase 2
