# Plan: Move User Context from vire-server to vire-mcp

## Context

The cloud architecture (`vire-infra/docs/architecture-per-user-deployment.md`) has a gateway/portal that creates per-user MCP proxy instances with user-specific config injected. The shared vire-server receives user context via HTTP headers on every request. This separation allows one vire-server to serve multiple users.

Today, user-specific config (portfolios, display_currency, Navexa API key) lives in `docker/vire.toml` alongside server-level config. To prepare for the cloud architecture while keeping the local Docker workflow intact, move user context to `docker/vire-mcp.toml` and have vire-mcp inject it as `X-Vire-*` headers on every request to vire-server.

**Scope:** portfolios, display_currency, navexa only. EODHD and Gemini keys remain server-side.

---

## What Moves Where

| Field | From (`docker/vire.toml`) | To (`docker/vire-mcp.toml`) | Header |
|-------|---------------------------|------------------------------|--------|
| `portfolios` | Top-level | `[user] portfolios` | `X-Vire-Portfolios` |
| `display_currency` | Top-level | `[user] display_currency` | `X-Vire-Display-Currency` |
| `clients.navexa.api_key` | `[clients.navexa]` | `[navexa] api_key` | `X-Vire-Navexa-Key` |

**Keep in vire.toml:** storage, server, clients.eodhd, clients.gemini, clients.navexa.base_url/rate_limit/timeout, logging. Remove portfolios/display_currency/navexa key from vire.toml — the server no longer has fallback defaults for these. If no headers arrive, requests fail with a clear error.

**Header format:**
- `X-Vire-Portfolios: SMSF` (comma-separated if multiple)
- `X-Vire-Display-Currency: AUD`
- `X-Vire-Navexa-Key: KGliZ...` (raw key)

---

## MCP User Config — Resolution Order

User config values are resolved with the following precedence (highest wins):

| Priority | Source | Example |
|----------|--------|---------|
| 1 (highest) | Command-line flags | `--portfolios SMSF --display-currency AUD --navexa-key xxx` |
| 2 | Environment variables | `VIRE_PORTFOLIOS=SMSF`, `VIRE_DISPLAY_CURRENCY=AUD`, `NAVEXA_API_KEY=xxx` |
| 3 | TOML config file | `docker/vire-mcp.toml` `[user]` and `[navexa]` sections |
| 4 (lowest) | Defaults | portfolios=[] (empty), display_currency="AUD", navexa_key="" (null) |

**Defaults — service starts but rejects requests:**
- `portfolios` = `[]` (empty)
- `display_currency` = `"AUD"`
- `navexa_key` = `""` (null/empty)

The MCP service starts normally regardless of config state — it registers tools and listens for connections. However, every tool request validates the resolved config before making an HTTP call. Missing required values produce a specific error identifying what is not configured:

| Condition | Error |
|-----------|-------|
| `portfolios` is empty | `"user context not configured: no portfolios defined"` |
| `navexa_key` is empty | `"user context not configured: navexa api key not defined"` |
| `display_currency` is empty | `"user context not configured: display currency not defined"` |

Multiple missing values are reported together, e.g.: `"user context not configured: no portfolios defined, navexa api key not defined"`.

This ensures the MCP process is running and ready to receive config (e.g., from a cloud gateway setting env vars or a restart with CLI flags) but does not silently operate without user context. The error message tells the operator exactly which config values need to be provided.

---

## Changes

### 1. MCP Config — add `[user]` and `[navexa]` sections (`cmd/vire-mcp/main.go`)

Add structs to MCP Config:

```go
type UserConfig struct {
    Portfolios      []string `mapstructure:"portfolios"`
    DisplayCurrency string   `mapstructure:"display_currency"`
}

type NavexaConfig struct {
    APIKey string `mapstructure:"api_key"`
}
```

Add to the existing `Config` struct:
```go
type Config struct {
    Server  ServerConfig         `mapstructure:"server"`
    User    UserConfig           `mapstructure:"user"`
    Navexa  NavexaConfig         `mapstructure:"navexa"`
    Logging common.LoggingConfig `mapstructure:"logging"`
}
```

**Defaults:**
```go
viper.SetDefault("user.portfolios", []string{})
viper.SetDefault("user.display_currency", "AUD")
viper.SetDefault("navexa.api_key", "")
```

**Command-line flags:**
```go
flag.String("portfolios", "", "Comma-separated portfolio names")
flag.String("display-currency", "", "Display currency (AUD or USD)")
flag.String("navexa-key", "", "Navexa API key")
```

**Environment overrides** (after viper unmarshal):
```go
if p := os.Getenv("VIRE_PORTFOLIOS"); p != "" {
    cfg.User.Portfolios = strings.Split(p, ",")
}
if dc := os.Getenv("VIRE_DISPLAY_CURRENCY"); dc != "" {
    cfg.User.DisplayCurrency = dc
}
if nk := os.Getenv("NAVEXA_API_KEY"); nk != "" {
    cfg.Navexa.APIKey = nk
}
```

**Flag overrides** (highest priority, after env):
```go
if *portfoliosFlag != "" {
    cfg.User.Portfolios = strings.Split(*portfoliosFlag, ",")
}
if *displayCurrencyFlag != "" {
    cfg.User.DisplayCurrency = *displayCurrencyFlag
}
if *navexaKeyFlag != "" {
    cfg.Navexa.APIKey = *navexaKeyFlag
}
```

### 2. MCP — user context validation and error gate

Add a validation check after config resolution. Collect all missing required values into a single error:

```go
func (p *MCPProxy) isConfigured() error {
    var missing []string
    if p.userHeaders.Get("X-Vire-Portfolios") == "" {
        missing = append(missing, "no portfolios defined")
    }
    if p.userHeaders.Get("X-Vire-Navexa-Key") == "" {
        missing = append(missing, "navexa api key not defined")
    }
    if p.userHeaders.Get("X-Vire-Display-Currency") == "" {
        missing = append(missing, "display currency not defined")
    }
    if len(missing) > 0 {
        return fmt.Errorf("user context not configured: %s", strings.Join(missing, ", "))
    }
    return nil
}
```

Call `isConfigured()` at the top of `get()`, `del()`, `doJSON()`. If it returns an error, return that error immediately without making the HTTP request.

The MCP process starts, registers tools, and listens — but every tool call fails with a specific error identifying the missing config until all required values are provided (via restart with flags/env/TOML, or in the cloud via gateway provisioning).

### 3. MCP Proxy — header injection (`cmd/vire-mcp/proxy.go`)

Add `userHeaders http.Header` field to `MCPProxy`. Populate from `UserConfig` and `NavexaConfig` in constructor:

```go
func NewMCPProxy(serverURL string, logger *common.Logger, userCfg UserConfig, navexaCfg NavexaConfig) *MCPProxy {
    headers := make(http.Header)
    if len(userCfg.Portfolios) > 0 {
        headers.Set("X-Vire-Portfolios", strings.Join(userCfg.Portfolios, ","))
    }
    if userCfg.DisplayCurrency != "" {
        headers.Set("X-Vire-Display-Currency", userCfg.DisplayCurrency)
    }
    if navexaCfg.APIKey != "" {
        headers.Set("X-Vire-Navexa-Key", navexaCfg.APIKey)
    }
    // ...
}
```

Add `applyUserHeaders(req)` method, called from all HTTP methods.

**Fix `get()`** — currently uses `httpClient.Get()` which cannot set headers. Refactor to `http.NewRequest()` + `httpClient.Do()` (matching `del()` and `doJSON()` pattern).

Call `applyUserHeaders()` from:
- `get()` — after refactor
- `del()` — after existing `req.Header.Set("Content-Type", ...)`
- `doJSON()` — after existing `req.Header.Set("Content-Type", ...)` (covers post/put/patch)

### 4. Server — user context type (`internal/common/userctx.go` — new)

```go
type UserContext struct {
    Portfolios      []string
    DisplayCurrency string
    NavexaAPIKey    string
}
```

With `UserContextFromContext(ctx)` and `WithUserContext(ctx, uc)` helpers using a context key.

Add resolution helpers:
- `ResolvePortfolios(ctx, configPortfolios) []string` — UserContext first, config fallback
- `ResolveDisplayCurrency(ctx, configCurrency) string` — UserContext first, config fallback (validate AUD/USD)

### 5. Server — middleware (`internal/server/middleware.go`)

Add `userContextMiddleware` — extracts `X-Vire-*` headers into `UserContext`, stores in request context. Only creates `UserContext` if at least one header is present (absent headers = no UserContext in context).

Insert into `applyMiddleware()` chain after CORS, before correlationID:

```go
func applyMiddleware(handler http.Handler, logger *common.Logger) http.Handler {
    handler = loggingMiddleware(logger)(handler)
    handler = correlationIDMiddleware(handler)
    handler = userContextMiddleware(handler)  // NEW
    handler = corsMiddleware(handler)
    handler = recoveryMiddleware(logger)(handler)
    return handler
}
```

Update CORS `Allow-Headers` to include `X-Vire-Portfolios, X-Vire-Display-Currency, X-Vire-Navexa-Key`.

### 6. Server — handler updates

**`handleConfig`** (`internal/server/routes.go:153`):
- Use `common.ResolvePortfolios(ctx, s.app.Config.Portfolios)` instead of `s.app.Config.Portfolios`
- Add `display_currency` to response using `common.ResolveDisplayCurrency(ctx, s.app.Config.DisplayCurrency)`

**`resolvePortfolio`** (`internal/server/handlers.go:1205`):
- Check `UserContext.Portfolios[0]` before falling back to KV store / config default

**`handlePortfolioDefault`** (`internal/server/handlers.go:151`):
- Check UserContext for default portfolio before config fallback

### 7. Server — per-request Navexa client (`internal/app/app.go`)

The Navexa client is created once at startup and held on `App.NavexaClient`. When a header provides a different key, we need a per-request client.

Add to `App`:
```go
func (a *App) NavexaClientForRequest(ctx context.Context) interfaces.NavexaClient {
    if uc := common.UserContextFromContext(ctx); uc != nil && uc.NavexaAPIKey != "" {
        return navexa.NewClient(uc.NavexaAPIKey,
            navexa.WithLogger(a.Logger),
            navexa.WithRateLimit(a.Config.Clients.Navexa.RateLimit),
        )
    }
    return a.NavexaClient
}
```

Handlers that trigger Navexa operations (`handlePortfolioSync`, `handlePortfolioRebuild`) pass the resolved client to the service. This requires either:
- (a) Adding a `navexaClient` parameter to `SyncPortfolio()`, or
- (b) A `SetNavexaClient` method on the service, or
- (c) Having the service check UserContext itself

**Recommended: (a)** — add an optional `NavexaClient` override parameter. The service already imports `interfaces`, so this is clean. When nil, use the service's default client.

### 8. TOML config updates

**`docker/vire-mcp.toml`** — add:
```toml
[user]
portfolios = ["SMSF"]
display_currency = "AUD"

[navexa]
api_key = "KGliZrS7rOlgwp+i9zZasCQPd+HN2/K57EGNF6j72mI="
```

**`docker/vire.toml`** — remove `portfolios`, `display_currency`, and `clients.navexa.api_key`. Keep `[clients.navexa]` section for base_url/rate_limit/timeout (service-level settings). The server no longer falls back to its own config for user-specific values.

**`tests/docker/vire-mcp.toml`** — add matching `[user]` and `[navexa]` sections for test config.

### 9. Security — never log API keys

- Proxy `applyUserHeaders()` does not log header values
- Server `userContextMiddleware` does not log `NavexaAPIKey`
- Server `loggingMiddleware` already only logs method/path/status — no headers logged

---

## What NOT to Change

- **Domain storage types** — storage separation is already done, unrelated
- **Service layer internals** — services access storage through unchanged interfaces
- **MCP tool handlers** (`cmd/vire-mcp/tools.go`) — they call proxy methods which auto-inject headers
- **EODHD/Gemini clients** — remain server-side, single-instance
- **Docker compose** — volume mounts and networking unchanged

---

## Notable Finding

`DisplayCurrency` is defined in `Config` (`internal/common/config.go:21`) but **not consumed by any handler or service**. The MCP formatters (`cmd/vire-mcp/formatters.go`) hardcode AUD conversion logic. Moving it to MCP config and plumbing it as a header establishes the contract for future use, but there is no current consumer to update on the server side. The `handleConfig` response can include it for visibility.

---

## Files to Modify

| File | Action |
|------|--------|
| `cmd/vire-mcp/main.go` | Add UserConfig/NavexaConfig structs, CLI flags, env overrides, pass to proxy |
| `cmd/vire-mcp/proxy.go` | Add userHeaders field, applyUserHeaders(), isConfigured() gate, refactor get() |
| `internal/common/userctx.go` | **New** — UserContext type, context helpers, resolve functions |
| `internal/server/middleware.go` | Add userContextMiddleware, update CORS, wire into chain |
| `internal/server/routes.go` | Update handleConfig to use resolved portfolios/currency |
| `internal/server/handlers.go` | Update resolvePortfolio and handlePortfolioDefault |
| `internal/app/app.go` | Add NavexaClientForRequest() method |
| `internal/services/portfolio/service.go` | Accept optional NavexaClient override in SyncPortfolio |
| `docker/vire-mcp.toml` | Add `[user]` and `[navexa]` sections |
| `docker/vire.toml` | Remove portfolios, display_currency, navexa api_key |
| `tests/docker/vire-mcp.toml` | Add `[user]` and `[navexa]` sections |

---

## Verification

1. `go test ./...` — all existing tests pass
2. `./scripts/deploy.sh local` — both containers build and start
3. `curl -sf http://localhost:8501/api/health` — server healthy
4. `curl -H "X-Vire-Portfolios: TEST" http://localhost:8501/api/config` — verify config endpoint reflects header value
5. `curl http://localhost:8501/api/config` (no headers) — verify no user context returned
6. MCP tool call via Claude Desktop — verify portfolio operations work end-to-end with headers
7. Start MCP with no config (empty TOML, no flags, no env) — verify it starts but returns "user context not configured" error for all tool calls
8. Start MCP with CLI flags only — verify headers are injected correctly
9. New unit tests:
   - `cmd/vire-mcp/proxy_test.go` — headers sent on all HTTP methods, empty config = no headers, isConfigured() gate
   - `internal/server/middleware_test.go` — header extraction, comma parsing, missing headers
   - `internal/common/userctx_test.go` — context round-trip, resolve helpers with/without UserContext
