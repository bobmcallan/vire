# Requirements: Phase 1 — Observability (Replace Zerolog with Arbor)

**Date:** 2026-02-10
**Requested:** Implement Phase 1 of docs/performance-plan.md — replace zerolog with arbor logging, add correlation IDs, add get_diagnostics MCP tool, add timing to all handlers.

## Scope
- Replace `github.com/rs/zerolog` with `github.com/ternarybob/arbor` across the codebase
- Rewrite `internal/common/logging.go` factory functions for arbor with console + memory writers
- Fix 2 `.Time()` call sites (convert to `.Str()`)
- Add correlation IDs to all MCP handler entry points
- Add `get_diagnostics` MCP tool that queries arbor's memory writer
- Change default log level from `warn` to `info`
- Add timing instrumentation to all MCP handlers (not just portfolio_review)
- Add API call logging to EODHD client `get()` method

## Out of Scope
- Phase 2 (live prices)
- Phase 3 (concurrency)
- Phase 4 (EODHD technical API)
- Phase 5 (dead code cleanup)

## Approach
Per docs/performance-plan.md Phase 1 and docs/arbor-assessment.md:
- Arbor's ILogEvent interface is 99% API-compatible with zerolog (Str, Int, Float64, Bool, Err, Dur, Msg, Msgf all match)
- 212 of 214 log statements work unchanged; 2 `.Time()` calls convert to `.Str()`
- Constructor injection pattern (`*common.Logger`) stays — wrapper type changes from embedding zerolog.Logger to wrapping arbor.ILogger
- Arbor memory writer replaces need for custom metrics.go — diagnostics tool queries it directly
- Correlation IDs trace MCP requests through service → client → storage layers

## Files Expected to Change
- `go.mod` / `go.sum` — swap zerolog for arbor dependency
- `internal/common/logging.go` — rewrite factory functions
- `internal/clients/eodhd/client.go` — add API call logging to `get()`
- `internal/clients/navexa/client.go` — timing if `.Time()` used here
- `internal/clients/gemini/client.go` — timing if `.Time()` used here
- `cmd/vire-mcp/main.go` — change log level, register diagnostics tool
- `cmd/vire-mcp/handlers.go` — correlation IDs, timing on all handlers, new handleDiagnostics
- ~18 other files importing common.Logger — may need minor type adjustments
