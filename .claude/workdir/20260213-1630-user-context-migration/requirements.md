# Requirements: Move User Context from vire-server to vire-mcp

**Date:** 2026-02-13
**Requested:** Move user-specific configuration (portfolios, display_currency, navexa API key) from vire-server config to vire-mcp, with vire-mcp injecting it as X-Vire-* headers on every request. Preparation for multi-tenant cloud architecture.

## Scope

### In scope
- Add [user] and [navexa] config sections to vire-mcp with 4-tier resolution: CLI flags > env vars > TOML > defaults
- MCP proxy injects X-Vire-Portfolios, X-Vire-Display-Currency, X-Vire-Navexa-Key headers on all requests
- Fix proxy.get() to support custom headers (refactor from httpClient.Get to http.NewRequest)
- Server-side UserContext middleware to extract headers into request context
- Handler updates to resolve from UserContext before config fallback
- Per-request Navexa client creation from header-provided key
- isConfigured() gate — MCP starts but rejects all tool calls with specific error when config is missing
- Remove user-specific values from docker/vire.toml
- Update CORS to allow X-Vire-* headers

### Out of scope
- EODHD/Gemini API key migration (remain server-side)
- Cloud deployment, GCS storage, multi-tenant middleware
- DisplayCurrency consumption in handlers/services (not currently used)

## Approach
Follow the detailed plan in docs/user-context-migration.md. Key design: MCP holds user context, injects as headers, server extracts via middleware with fallback to config. Defaults leave MCP in "not configured" state with specific error messages per missing value.

## Files Expected to Change
- cmd/vire-mcp/main.go — UserConfig/NavexaConfig structs, CLI flags, env overrides
- cmd/vire-mcp/proxy.go — userHeaders, applyUserHeaders(), isConfigured() gate, refactor get()
- internal/common/userctx.go — new: UserContext type, context helpers, resolve functions
- internal/server/middleware.go — userContextMiddleware, CORS update
- internal/server/routes.go — handleConfig uses resolved portfolios/currency
- internal/server/handlers.go — resolvePortfolio and handlePortfolioDefault
- internal/app/app.go — NavexaClientForRequest()
- internal/services/portfolio/service.go — optional NavexaClient override in SyncPortfolio
- docker/vire-mcp.toml — add [user] and [navexa] sections
- docker/vire.toml — remove portfolios, display_currency, navexa api_key
- tests/docker/vire-mcp.toml — add [user] and [navexa] sections
