# Plan: Replace Zerolog with Arbor (Phase 1 Observability)

## Investigation Summary

### Current State
- `internal/common/logging.go` (82 lines): wraps `zerolog.Logger` in a `Logger` struct with 4 factory functions
- 19 files use `*common.Logger` via constructor injection
- 1 file imports `zerolog` directly: `internal/common/logging.go`
- 2 `.Time()` call sites:
  - `internal/services/market/service.go:73` — `s.logger.Debug().Str("ticker", ticker).Time("from", fromDate).Msg(...)`
  - `internal/services/portfolio/snapshot.go:87` — `s.logger.Info().Str("name", name).Time("asOf", asOf).Msg(...)`
- Default log level is `"warn"` (set in `cmd/vire-mcp/main.go:65`)
- Only `handlePortfolioReview` has phase timing; other handlers have no timing

### Arbor API Compatibility Confirmed
- `arbor.NewLogger()` creates a fresh `ILogger`
- `.WithConsoleWriter(config)`, `.WithMemoryWriter(config)` — register writers, return forked logger
- `.WithLevelFromString(level)` — sets level from string ("debug", "info", etc.)
- `.WithCorrelationId(id)` — sets correlation ID, returns forked logger (auto-generates UUID if empty)
- `.Copy()` — creates independent fork (tree-like)
- `.Info()/.Warn()/.Error()/.Debug()/.Fatal()` — return `ILogEvent` with fluent `.Str()/.Int()/.Dur()/.Err()/.Msg()/.Msgf()` methods
- `.GetMemoryLogsForCorrelation(id)` — returns `map[string]string`
- `.GetMemoryLogsWithLimit(n)` — returns last N entries
- **No `.Time()` on ILogEvent** — must use `.Str(key, t.Format(time.RFC3339))`
- `WriterConfiguration` struct: `Type`, `Writer` (io.Writer), `Level`, `TimeFormat`, `OutputType`

### Key Design Decision: Logger Wrapper
The current `Logger` struct embeds `zerolog.Logger`, which means all zerolog methods are promoted. Arbor's `ILogger` is an interface, not a struct. The new `Logger` will embed `arbor.ILogger` to maintain the same promotion pattern. This preserves the `logger.Info().Str(...).Msg(...)` pattern across all 19 consumer files without changes.

---

## Implementation Steps

### Step 1: Rewrite `internal/common/logging.go`

Replace the 82-line file. New structure:

```go
package common

import (
    "os"
    "github.com/ternarybob/arbor"
    "github.com/ternarybob/arbor/models"
)

type Logger struct {
    arbor.ILogger
}

func NewLogger(level string) *Logger {
    arborLogger := arbor.NewLogger().
        WithConsoleWriter(models.WriterConfiguration{
            Type:       models.LogWriterTypeConsole,
            Writer:     os.Stderr,
            TimeFormat: "2006-01-02T15:04:05Z07:00",
        }).
        WithMemoryWriter(models.WriterConfiguration{
            Type: models.LogWriterTypeMemory,
        }).
        WithLevelFromString(level)

    return &Logger{ILogger: arborLogger}
}

func NewLoggerWithOutput(level string, w io.Writer) *Logger {
    arborLogger := arbor.NewLogger().
        WithConsoleWriter(models.WriterConfiguration{
            Type:       models.LogWriterTypeConsole,
            Writer:     w,
            TimeFormat: "2006-01-02T15:04:05Z07:00",
        }).
        WithMemoryWriter(models.WriterConfiguration{
            Type: models.LogWriterTypeMemory,
        }).
        WithLevelFromString(level)

    return &Logger{ILogger: arborLogger}
}

func NewDefaultLogger() *Logger {
    return NewLogger("info")
}

func NewSilentLogger() *Logger {
    // Arbor with disabled level — all events suppressed
    arborLogger := arbor.NewLogger().WithLevelFromString("disabled")
    return &Logger{ILogger: arborLogger}
}

// WithCorrelationId returns a new Logger with a correlation ID set.
// Used by MCP handlers to trace a request through all layers.
func (l *Logger) WithCorrelationId(id string) *Logger {
    return &Logger{ILogger: l.ILogger.WithCorrelationId(id)}
}
```

Key points:
- `Logger` embeds `arbor.ILogger` (interface) so `.Info()`, `.Warn()`, etc. are all promoted
- Console writer writes to `os.Stderr` (same as current)
- Memory writer enabled by default for diagnostics
- `WithCorrelationId` returns `*Logger` (not `ILogger`) to stay in the typed wrapper
- `NewSilentLogger` uses `"disabled"` level — arbor suppresses all events

### Step 2: Update `go.mod`

- Add `github.com/ternarybob/arbor` (latest)
- Remove `github.com/rs/zerolog`
- Run `go mod tidy`

### Step 3: Fix 2 `.Time()` call sites

**`internal/services/market/service.go:73`:**
```go
// Before:
s.logger.Debug().Str("ticker", ticker).Time("from", fromDate).Msg("Incremental EOD fetch")
// After:
s.logger.Debug().Str("ticker", ticker).Str("from", fromDate.Format(time.RFC3339)).Msg("Incremental EOD fetch")
```

**`internal/services/portfolio/snapshot.go:87`:**
```go
// Before:
s.logger.Info().Str("name", name).Time("asOf", asOf).Msg("Building portfolio snapshot")
// After:
s.logger.Info().Str("name", name).Str("asOf", asOf.Format(time.RFC3339)).Msg("Building portfolio snapshot")
```

### Step 4: Change default log level

**`cmd/vire-mcp/main.go:65`:**
```go
// Before:
logger := common.NewLogger("warn")
// After:
logger := common.NewLogger("info")
```

### Step 5: Add correlation IDs to MCP handlers

In `cmd/vire-mcp/handlers.go`, add a helper function and use it at the top of each handler:

```go
func generateRequestID() string {
    return fmt.Sprintf("mcp-%d", time.Now().UnixNano())
}
```

Pattern for each handler (example: handlePortfolioReview):
```go
func handlePortfolioReview(..., logger *common.Logger) server.ToolHandlerFunc {
    return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        reqLogger := logger.WithCorrelationId(generateRequestID())
        handlerStart := time.Now()
        // ... use reqLogger instead of logger for all logging in this handler ...
    }
}
```

This applies to all ~38 handlers. Each handler gets a correlated logger that traces through to service calls (since services receive `*common.Logger` in their constructors, we need to pass `reqLogger` only for handler-level logging; service constructors keep the base logger). For full end-to-end correlation, services would need to accept a logger per request — but that's a larger refactor. For Phase 1, correlation IDs on handler-level logging is sufficient to trace which handler was slow.

### Step 6: Add timing to all handlers

Currently only `handlePortfolioReview` has `handlerStart` / elapsed logging. Add this pattern to all other handlers:

```go
handlerStart := time.Now()
// ... handler body ...
reqLogger.Info().Dur("elapsed", time.Since(handlerStart)).Str("tool", "tool_name").Msg("Handler complete")
```

Handlers to add timing to (those that currently lack it):
- All handlers listed in the grep output (~38 total); most are simple but the slow ones matter most:
  `handleMarketSnipe`, `handleStockScreen`, `handleFunnelScreen`, `handleGetStockData`,
  `handleCollectMarketData`, `handleDetectSignals`, `handleGenerateReport`,
  `handleGenerateTickerReport`, `handlePortfolioReview` (already has it),
  `handleRebuildData`, `handleSyncPortfolio`

### Step 7: Add API call logging to EODHD client

In `internal/clients/eodhd/client.go`, add logging to the `get()` method:
```go
func (c *Client) get(ctx context.Context, path string, params url.Values) ([]byte, error) {
    start := time.Now()
    // ... existing HTTP call ...
    c.logger.Info().
        Str("method", "GET").
        Str("path", path).
        Int("status", resp.StatusCode).
        Dur("elapsed", time.Since(start)).
        Msg("EODHD API call")
    // ...
}
```

### Step 8: Add `get_diagnostics` MCP tool

New handler `handleDiagnostics` in `cmd/vire-mcp/handlers.go`:
- Queries `logger.GetMemoryLogsWithLimit(100)` for recent log entries
- Queries `logger.GetMemoryLogsForCorrelation(id)` if a correlation ID is provided
- Reports service uptime (track startup time in a package var)
- Reports version, config summary
- Reports recent errors (filter memory logs for error-level entries)

New tool registration in `cmd/vire-mcp/main.go`:
```go
mcpServer.AddTool(createGetDiagnosticsTool(), handleDiagnostics(logger, startupTime))
```

Tool parameters:
- `correlation_id` (optional string) — if provided, returns logs for that specific request
- `limit` (optional int, default 50) — max recent log entries to return

---

## Files Modified

| File | Change |
|------|--------|
| `go.mod` | Add arbor, remove zerolog |
| `internal/common/logging.go` | Complete rewrite (arbor wrapper) |
| `internal/services/market/service.go` | Fix `.Time()` -> `.Str()` (1 line) |
| `internal/services/portfolio/snapshot.go` | Fix `.Time()` -> `.Str()` (1 line) |
| `cmd/vire-mcp/main.go` | Change log level to "info", register diagnostics tool, pass startupTime |
| `cmd/vire-mcp/handlers.go` | Add correlation IDs + timing to all handlers, add handleDiagnostics |
| `internal/clients/eodhd/client.go` | Add API call logging to `get()` method |

**No changes needed in the other 18 files** that use `*common.Logger` — the wrapper type stays the same, the fluent API is compatible, and constructor injection is unchanged.

---

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Arbor's global writer registry (writers registered globally, not per-instance) | Acceptable for Vire — single logger instance. NewSilentLogger uses "disabled" level, not separate writers. |
| Memory writer growth | Arbor has built-in TTL cleanup (10-min default retention). Acceptable for diagnostics. |
| `NewSilentLogger` with "disabled" may still register writers | Will test; if needed, create bare `arbor.NewLogger()` without any writers. |
| Test loggers creating memory writers | `NewSilentLogger` should avoid registering writers for test isolation. Will verify. |

---

## Test Strategy (for task #4)

1. Unit test `NewLogger` — verify it returns non-nil, `.Info().Str().Msg()` doesn't panic
2. Unit test `NewSilentLogger` — verify it discards output
3. Unit test `WithCorrelationId` — verify returned logger has correlation context
4. Unit test `GetMemoryLogsWithLimit` — verify logs are queryable after writing
5. Compile test — `go build ./...` passes (proves all 212 log statements compile)
6. Existing tests pass — `go test ./internal/...`
