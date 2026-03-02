# Assessment: Replacing Zerolog with Arbor

**Date:** 2026-02-10
**Status:** Assessment complete

---

## API Compatibility

Arbor's `ILogEvent` interface matches zerolog's fluent API almost exactly:

| Method | Zerolog | Arbor | Compatible? |
|--------|---------|-------|-------------|
| `.Str(key, val)` | Yes | Yes | Drop-in |
| `.Int(key, val)` | Yes | Yes | Drop-in |
| `.Int64(key, val)` | Yes | Yes | Drop-in |
| `.Float64(key, val)` | Yes | Yes | Drop-in |
| `.Bool(key, val)` | Yes | Yes | Drop-in |
| `.Err(err)` | Yes | Yes | Drop-in |
| `.Dur(key, dur)` | Yes | Yes | Drop-in |
| `.Msg(msg)` | Yes | Yes | Drop-in |
| `.Msgf(fmt, args...)` | Yes | Yes | Drop-in |
| `.Info()` | Yes | Yes | Drop-in |
| `.Warn()` | Yes | Yes | Drop-in |
| `.Error()` | Yes | Yes | Drop-in |
| `.Debug()` | Yes | Yes | Drop-in |
| `.Fatal()` | Yes | Yes | Drop-in |
| `.Strs(key, vals)` | Yes | Yes | Drop-in |
| `.Time(key, t)` | Yes | No | Not in arbor ILogEvent |

Vire uses `.Time()` in 2 places only. These would need minor refactoring (format to string, use `.Str()`).

**Verdict: 99% API compatible.** The fluent chain pattern `.Info().Str("k","v").Dur("elapsed", d).Msg("done")` works identically in both libraries.

---

## What Arbor Adds That Zerolog Doesn't

### 1. Correlation IDs — Solves the Request Tracing Problem

Zerolog has no built-in concept of request correlation. Vire's MCP handlers process tool calls but there's no way to trace which log entries belong to which MCP request.

Arbor's `.WithCorrelationId("req-123")` attaches a correlation ID to all log entries from that logger instance. For Vire:
- Each MCP tool call gets a correlation ID
- All downstream service calls, API requests, and storage operations log under that ID
- The diagnostics tool can query logs by correlation ID: "show me everything that happened during this portfolio_review call"

**This directly addresses the "can't diagnose why a request was slow" problem.**

### 2. In-Memory Log Store — Enables the Diagnostics MCP Tool

Zerolog writes to stderr and it's gone. To build the `get_diagnostics` tool from the performance plan, you'd need a custom ring buffer alongside zerolog.

Arbor's memory writer stores logs queryable at runtime:
- `GetMemoryLogs()` — recent entries
- `GetMemoryLogsForCorrelation(id)` — all entries for a specific request
- `GetMemoryLogsWithLimit(n)` — last N entries

This means the diagnostics MCP tool can return actual structured log data, including per-request timing breakdowns, without building a custom metrics layer.

### 3. Channel-Based Log Streaming — Future MCP Endpoint

Arbor's `SetChannel("diagnostics")` registers a named channel that receives log events in batches. A future `stream_diagnostics` MCP tool could subscribe to real-time log output for a specific correlation ID — e.g., "stream logs while this portfolio_review runs."

Not needed immediately, but the capability is there without additional infrastructure.

### 4. Multi-Writer by Default

Zerolog writes to one output (Vire uses stderr via ConsoleWriter). Adding file logging requires manually configuring a multi-writer.

Arbor supports simultaneous writers out of the box:
- Console writer (stderr, human-readable)
- File writer (JSON, with rotation — 500KB default, 20 backups)
- Memory writer (queryable in-memory store)

All active simultaneously. No custom plumbing needed.

### 5. Logger Registry — Cross-Context Access

Arbor's global registry allows retrieving loggers by name without passing them through every constructor. Vire currently passes `*common.Logger` through 20+ constructors. The registry pattern would allow:
```go
arbor.GetLogger("market-service")
```

**However:** Vire's constructor injection is clean and explicit. The registry is optional — arbor supports both patterns. Recommendation: keep constructor injection for testability, use registry only if needed for edge cases.

---

## What Zerolog Has That Arbor Doesn't

### 1. `.Time(key, time.Time)` Method

Arbor's `ILogEvent` doesn't include `.Time()`. Vire uses this in 2 places. Minor — convert to `.Str(key, t.Format(time.RFC3339))`.

### 2. Zero-Allocation Design

Zerolog is specifically engineered for zero-allocation logging — log events don't escape to the heap. This gives zerolog ~100ns per log entry.

Arbor creates a `models.LogEvent` struct, marshals to JSON, dispatches to writers. Direct writers claim ~50-100μs per log. That's 500-1000x slower than zerolog per entry.

**Is this a problem for Vire?** No. Vire logs ~214 statements across all code paths. At 100μs each, even a worst-case scenario logging every statement adds 21ms total — invisible against 5-35 second operations. The bottleneck is EODHD API calls and Gemini, not logging overhead.

### 3. Ecosystem / Community

Zerolog has 11k+ GitHub stars and widespread adoption. Arbor is a personal library. This matters for:
- Documentation: zerolog has extensive docs, blog posts, examples
- Bug fixes: zerolog has active maintenance
- Familiarity: other developers know zerolog

**For Vire as a personal project**, this is irrelevant. You maintain both libraries.

---

## Migration Effort

### What Changes

**`internal/common/logging.go`** — Complete rewrite (small file, 82 lines):
- Replace `zerolog.Logger` embed with `arbor.ILogger`
- Factory functions create arbor loggers with console + memory writers
- Add correlation ID helper for MCP handlers

**All 20 files using logger** — Type change only:
- `*common.Logger` stays as the type (wrapper around arbor instead of zerolog)
- All `.Info().Str().Msg()` chains work without changes
- 2 `.Time()` calls need conversion to `.Str()`

**`cmd/vire-mcp/handlers.go`** — Add correlation IDs:
- Each handler creates a correlated logger: `logger.WithCorrelationId(requestID)`
- Pass correlated logger to service calls
- No change to logging statements themselves

### What Doesn't Change

- All 212 of 214 log statements (identical API)
- Constructor injection pattern
- Service/client code (receives logger, calls same methods)
- Test patterns (silent logger still works)

### Estimated Effort

| Task | Files | Effort |
|------|-------|--------|
| Rewrite `logging.go` | 1 | 30 min |
| Update `go.mod` (add arbor, remove zerolog) | 1 | 5 min |
| Fix 2 `.Time()` calls | 2 | 10 min |
| Add correlation ID to handlers | 1 | 1 hour |
| Wire memory writer into diagnostics tool | 2 | 1 hour |
| Test and verify | — | 1 hour |
| **Total** | **~7** | **~3-4 hours** |

---

## Recommendation

**Replace zerolog with arbor.** The API is compatible enough that migration is mechanical, and arbor provides three capabilities Vire needs that zerolog doesn't:

1. **Correlation IDs** — trace a slow MCP request through all layers
2. **In-memory queryable logs** — the diagnostics MCP tool reads from arbor's memory writer instead of building a custom metrics layer
3. **Multi-writer** — console + file + memory simultaneously without custom code

This collapses Phase 1.1 of the performance plan (build custom `metrics.go`) into the logging migration itself. Arbor's memory writer *is* the metrics store. The diagnostics tool queries arbor directly rather than maintaining a parallel data structure.

### Migration Sequence

1. Add `github.com/ternarybob/arbor` to `go.mod`, remove `github.com/rs/zerolog`
2. Rewrite `internal/common/logging.go` — arbor logger with console + memory writers
3. Fix the 2 `.Time()` calls
4. Add correlation ID creation in MCP handler entry points
5. Build `get_diagnostics` handler that queries arbor's memory writer
6. Verify all existing log output still works

### What This Replaces in the Performance Plan

| Performance Plan Item | Before (zerolog) | After (arbor) |
|-----------------------|-------------------|---------------|
| Phase 1.1: Add `metrics.go` | Custom ring buffer, counters, timing recorder | Arbor memory writer — already built |
| Phase 1.2: Change log level | Same — change `"warn"` to `"info"` | Same |
| Phase 1.3: Add timing to handlers | Same — add `.Dur()` calls | Same, plus correlation IDs for free |
| Phase 1.4: API call logging | Same — add logging to `get()` | Same, correlated to parent request |
| Diagnostics tool data source | Custom `metrics.go` struct | `logger.GetMemoryLogsForCorrelation(id)` |

---

## Risk Assessment

| Risk | Severity | Mitigation |
|------|----------|------------|
| Arbor bug blocks Vire | Low | You own both codebases — fix immediately |
| Performance regression from logging | Negligible | 21ms worst case across 214 log statements |
| Missing `.Time()` method | Trivial | 2 call sites, convert to `.Str()` |
| Memory growth from memory writer | Low | Arbor supports `GetMemoryLogsWithLimit()` — configure retention |
| Breaking change in arbor | Low | Pin version in go.mod |
