# Summary: Dynamic MCP Tool Catalog Endpoint

**Date:** 2026-02-14
**Status:** completed

## What Changed

| File | Change |
|------|--------|
| `internal/models/tools.go` | New file: `ToolDefinition` and `ParamDefinition` structs matching design doc schema. |
| `internal/server/catalog.go` | New file: `buildToolCatalog()` returns all 26 active MCP tools with HTTP method, path template, and parameter definitions. Params describe the HTTP contract (not MCP tool interface). |
| `internal/server/catalog_test.go` | New file: 7 tests — tool count assertion (26), known tools verification, param validation, schema correctness. |
| `internal/server/routes.go` | Added `GET /api/mcp/tools` route and `GET /api/portfolios/{name}/stock/{ticker}` route. |
| `internal/server/handlers.go` | Added `handleToolCatalog()` handler (returns catalog as JSON) and `handlePortfolioStock()` handler (returns single holding with trade history). |
| `README.md` | Added `get_portfolio_stock` tool documentation. |

## Tests
- 14 new tests (7 catalog, 7 handler)
- All pass: 19 packages, 0 failures
- `go vet ./...` clean

## Documentation Updated
- `README.md` — `get_portfolio_stock` tool documented

## Review Findings
- **Bug 1 (fixed):** `set_default_portfolio` catalog param was `portfolio_name` but server expects `name` in JSON body. Fixed to `name`.
- **Bug 2 (fixed):** Body params used MCP-style names (`strategy_json`, `plan_json`, `item_json`) instead of actual HTTP body field names. Fixed to describe the HTTP contract: `strategy` (object), `items` (array) + `notes` (string), individual plan item fields.
- Reviewer verified all 26 tools cross-referenced against `cmd/vire-mcp/tools.go`.

## Notes
- This is migration step 1 from the two-service architecture design (`vire-infra/docs/design-two-service-architecture.md`)
- The catalog is static (built at compile time) — no runtime dependencies
- `get_portfolio_stock` now has a dedicated server endpoint (`GET /api/portfolios/{name}/stock/{ticker}`) instead of client-side filtering
- Commented-out/disabled tools are not included in the catalog
- The portal will use this endpoint to dynamically register MCP tools without hardcoding
