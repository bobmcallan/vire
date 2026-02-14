# Requirements: Dynamic MCP Tool Catalog Endpoint

**Date:** 2026-02-14
**Requested:** Add `GET /api/mcp/tools` endpoint to vire-server that returns a tool catalog for dynamic registration by vire-portal.

## Context

Architecture is moving from three services to two (see `vire-infra/docs/design-two-service-architecture.md`). The MCP layer (`cmd/vire-mcp/`) will be replaced by `vire-portal`, which dynamically registers tools by fetching a catalog from vire-server. This is migration step 1.

## Scope

### In Scope
- New `GET /api/mcp/tools` endpoint on vire-server
- Returns JSON array of `ToolDefinition` objects describing all 26 active MCP tools
- Each tool definition includes: name, description, HTTP method, path template, parameter definitions
- Parameters include: name, type, description, required, location (path/query/body), default_from
- New model types: `ToolDefinition`, `ParamDefinition` in `internal/models/`
- Route registration in `internal/server/routes.go`
- Handler in `internal/server/handlers.go`
- Unit test for the endpoint
- Integration test

### Out of Scope
- Changes to vire-portal (separate repo)
- OAuth implementation
- X-Vire-* header changes (migration step 2-3)
- Removing cmd/vire-mcp (future step)
- Commented-out/disabled tools (sync_portfolio, rebuild_data, etc.)

## Approach

1. Define `ToolDefinition` and `ParamDefinition` structs (matching design doc schema)
2. Create `internal/server/catalog.go` with `buildToolCatalog()` function that returns the full tool list
3. Add `GET /api/mcp/tools` route and handler
4. Map all 26 active tools from `cmd/vire-mcp/tools.go` to their HTTP equivalents
5. Test the endpoint returns valid JSON matching the schema

## Tool Mapping (26 active tools)

| MCP Tool | Method | Path |
|----------|--------|------|
| get_version | GET | /api/version |
| get_config | GET | /api/config |
| get_diagnostics | GET | /api/diagnostics |
| list_portfolios | GET | /api/portfolios |
| set_default_portfolio | GET/PUT | /api/portfolios/default |
| get_portfolio | GET | /api/portfolios/{portfolio_name} |
| get_portfolio_stock | GET | /api/portfolios/{portfolio_name}/stock/{ticker} |
| portfolio_compliance | POST | /api/portfolios/{portfolio_name}/review |
| generate_report | POST | /api/portfolios/{portfolio_name}/report |
| get_summary | GET | /api/portfolios/{portfolio_name}/summary |
| get_portfolio_strategy | GET | /api/portfolios/{portfolio_name}/strategy |
| set_portfolio_strategy | PUT | /api/portfolios/{portfolio_name}/strategy |
| delete_portfolio_strategy | DELETE | /api/portfolios/{portfolio_name}/strategy |
| get_portfolio_plan | GET | /api/portfolios/{portfolio_name}/plan |
| set_portfolio_plan | PUT | /api/portfolios/{portfolio_name}/plan |
| add_plan_item | POST | /api/portfolios/{portfolio_name}/plan/items |
| update_plan_item | PATCH | /api/portfolios/{portfolio_name}/plan/items/{item_id} |
| remove_plan_item | DELETE | /api/portfolios/{portfolio_name}/plan/items/{item_id} |
| check_plan_status | GET | /api/portfolios/{portfolio_name}/plan/status |
| get_quote | GET | /api/market/quote/{ticker} |
| get_stock_data | GET | /api/market/stocks/{ticker} |
| compute_indicators | POST | /api/market/signals |
| strategy_scanner | POST | /api/screen/snipe |
| stock_screen | POST | /api/screen |
| list_reports | GET | /api/reports |
| get_strategy_template | GET | /api/strategies/template |

## Files Expected to Change
- `internal/models/tools.go` — new file: ToolDefinition, ParamDefinition structs
- `internal/server/catalog.go` — new file: buildToolCatalog()
- `internal/server/routes.go` — add route
- `internal/server/handlers.go` — add handleToolCatalog handler
- `internal/server/catalog_test.go` — unit test
- `test/api/catalog_test.go` — integration test (optional)
- `README.md` — document new endpoint
