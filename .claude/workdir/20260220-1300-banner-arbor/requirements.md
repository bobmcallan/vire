# Requirements: Banner Screen & Arbor Logging Audit

**Date:** 2026-02-20
**Requested:** Add a startup banner similar to quaero and ensure all logging is via arbor.

## Scope

### In Scope
- Create `internal/common/banner.go` with styled startup banner
- Add `github.com/ternarybob/banner` dependency for color/style constants
- Integrate banner into startup sequence (after app init)
- Add shutdown banner
- Audit all logging to confirm arbor is used everywhere
- Add SurrealDB config sections to README.md

### Out of Scope
- Changing arbor library itself
- Modifying logging levels or output formats

## Investigation Findings

### Logging Audit — Already Complete
- Vire already uses arbor exclusively via `internal/common/logging.go`
- `Logger` wraps `arbor.ILogger`
- Zero `fmt.Print*` or `log.*` calls in source (only `fmt.Fprintf(os.Stderr)` in main.go for pre-logger fatal errors, which is correct)
- 214+ arbor log statements across the codebase
- **No changes needed for logging** — already fully arbor-based

### Banner — Critical Constraint
The `github.com/ternarybob/banner` library uses `fmt.Printf` (stdout) internally in `printColorized()` and `PrintTextWithAlignment()`. Vire's logging tests explicitly state:
> stdout IS the JSON-RPC channel — Logger MUST route to stderr only

**Solution:** Build banner strings using the banner library's style/color constants, but write to `os.Stderr` instead of using the library's Print methods. This preserves the visual styling without contaminating stdout.

## Approach

### 1. `internal/common/banner.go`

Create banner functions that write to stderr:
- `PrintBanner(config *Config, logger *Logger)` — startup banner
- `PrintShutdownBanner(logger *Logger)` — shutdown banner

Adapting from quaero but for Vire:
- Title: "VIRE"
- Subtitle: "Investment Research & Portfolio Analysis"
- Key-value: Version, Build, Environment, Service URL, Storage (SurrealDB address)
- Style: Double border, Cyan (to distinguish from quaero's green)
- All output via `fmt.Fprintf(os.Stderr, ...)` instead of `fmt.Printf`
- Also log structured data through arbor

### 2. Integration Points
- `cmd/vire-server/main.go`: Call `PrintBanner` after app init, `PrintShutdownBanner` before exit
- `internal/app/app.go`: Pass config/logger to banner

### 3. README.md Update
Add SurrealDB TOML config section and Docker deployment instructions.

## Files Expected to Change
- `internal/common/banner.go` — NEW
- `cmd/vire-server/main.go` — Add banner calls
- `go.mod` / `go.sum` — Add banner dependency
- `README.md` — Add SurrealDB config/docker sections
