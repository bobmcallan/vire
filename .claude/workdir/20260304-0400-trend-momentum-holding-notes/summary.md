# Summary: Trend Momentum Signal & Holding Intelligence Layer

**Status:** completed
**Feedback items:** fb_28c6e0bd, fb_61fba80b

## Changes

| File | Change |
|------|--------|
| `internal/models/signals.go` | Added `TrendMomentum` struct, `TrendMomentumLevel` enum (5-point scale), `SignalTypeTrendMomentum` constant, field on `TickerSignals` |
| `internal/models/holding_notes.go` | NEW: `HoldingNote`, `AssetType`, `LiquidityProfile`, `SignalConfidence`, `PortfolioHoldingNotes` with `NoteMap()`, `FindByTicker()`, `IsStale()`, `DeriveSignalConfidence()` |
| `internal/models/portfolio.go` | Added `HoldingNote`, `SignalConfidence`, `NoteStale` fields to `HoldingReview` |
| `internal/models/watchlist.go` | Added `HoldingNote`, `SignalConfidence`, `NoteStale` fields to `WatchlistItemReview` |
| `internal/interfaces/services.go` | Added `HoldingNoteService` interface (5 methods), `SetHoldingNoteService` to `PortfolioService` |
| `internal/signals/computer.go` | Added `computeTrendMomentum()` + helpers (`priceChangePct`, `avgVolume`, `clampFloat`, `classifyTrendMomentum`, `describeTrendMomentum`) |
| `internal/signals/computer_test.go` | 30+ trend momentum unit tests (edge cases, acceleration, volume confirmation, boundaries) |
| `internal/services/holdingnotes/service.go` | NEW: `HoldingNoteService` implementation (CRUD, UserDataStore subject="holding_notes") |
| `internal/services/portfolio/service.go` | Added `holdingNoteService` field + setter; integrated notes into `ReviewPortfolio`/`ReviewWatchlist`; added trend momentum to `determineAction`/`generateAlerts` |
| `internal/server/handlers.go` | Added `handleHoldingNotes` (GET/PUT), `handleHoldingNoteAdd` (POST), `handleHoldingNoteItem` (PATCH/DELETE) |
| `internal/server/routes.go` | Added `routeHoldingNotes` dispatcher, "notes" case in `routePortfolios` |
| `internal/server/catalog.go` | Added 5 MCP tool definitions (holding_note_get/set/add/update/remove) |
| `internal/server/catalog_test.go` | Updated tool count 70→75 |
| `internal/app/app.go` | Added `HoldingNoteService` field + wiring + `SetHoldingNoteService` injection |
| `internal/services/cashflow/service_test.go` | Added `SetHoldingNoteService` mock stub |
| `internal/services/report/devils_advocate_test.go` | Added `SetHoldingNoteService` mock stub |
| `tests/data/holding_notes_test.go` | NEW: 16 data layer integration tests |
| `tests/api/holding_notes_test.go` | NEW: 11 API integration tests |

## Tests

### Unit Tests
- `internal/signals/` — 87 tests pass (30+ new trend momentum tests)
- `internal/services/portfolio/` — all pass (0.3s)
- `internal/services/cashflow/` — all pass (4.5s)
- `internal/services/report/` — all pass

### Integration Tests
- `tests/data/holding_notes_test.go` — 16/16 pass (6s)
- `tests/api/holding_notes_test.go` — 11 tests created (Docker-dependent)

### Build/Vet
- `go build ./cmd/vire-server/` — PASS
- `go vet ./...` — PASS

## Pre-existing Failures (unchanged)
- `internal/server` — TestRoleEscalation_* (Docker timeout)
- `internal/storage/surrealdb` — TestPurgeCharts
- `internal/app` — test infrastructure (config required)

## How It Works

### fb_28c6e0bd: Trend Momentum Signal
Multi-timeframe composite signal tracking short-term price trajectory:
- **Inputs**: 3/5/10-day price changes, acceleration (rate of change of changes), volume confirmation, support proximity
- **Score**: Weighted composite (-1.0 to +1.0): 3d×0.45 + 5d×0.30 + 10d×0.15 + accel×0.10
- **Classification**: TREND_STRONG_UP / TREND_UP / TREND_FLAT / TREND_DOWN / TREND_STRONG_DOWN
- **Volume confirmation**: Lowers thresholds when 3-day avg volume exceeds 20-day avg by 20%+
- **Integration**: TREND_STRONG_DOWN → EXIT TRIGGER, TREND_DOWN → WATCH in compliance review
- **Alerts**: High severity for strong downtrend, medium for deteriorating

### fb_61fba80b: Holding Intelligence Layer
Per-holding analyst notes with signal confidence derivation:
- **Storage**: UserDataStore subject="holding_notes", single document per portfolio
- **Fields**: ticker, asset_type (ETF/ASX_stock/US_equity), liquidity_profile, thesis, known_behaviours, signal_overrides, stale_days
- **Signal confidence**: ETF → low, ASX_stock+high_liquidity → high, ASX_stock+low_liquidity → medium, US_equity → high
- **Staleness**: Configurable TTL (default 90 days), flagged in compliance review with alert
- **MCP tools**: 5 tools (get/set/add/update/remove) following watchlist pattern
- **Compliance integration**: Notes loaded at review start, attached to each HoldingReview/WatchlistItemReview with confidence and stale flags

## Notes
- Automated weekly review job is OUT OF SCOPE (future enhancement building on notes infrastructure)
- Signal confidence is informational — it doesn't suppress signals, it provides context for Claude's advisory layer
