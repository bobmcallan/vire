# Plan: Chart PNG File Saving

## Problem
When `portfolio_review` MCP tool returns an ImageContent block with base64 PNG, Claude Code cannot programmatically extract the raw base64 bytes to save as a file. The server should save the chart PNG to the data volume and return the file path.

## Proposed Changes (5 files)

### 1. `internal/interfaces/storage.go` — Add DataPath to StorageManager interface
Add `DataPath() string` to the `StorageManager` interface so handlers can determine where to write files.

### 2. `internal/storage/manager.go` — Implement DataPath
Add `DataPath()` method returning `m.fs.basePath`. This exposes the resolved data directory path (e.g. `/app/data` in Docker).

### 3. `internal/storage/file.go` — Add `charts` subdirectory
Add `"charts"` to the `subdirectories` slice so it is auto-created on startup alongside `portfolios/`, `market/`, etc.

### 4. `cmd/vire-mcp/handlers.go` — Save chart file and return path in TextContent
In `handlePortfolioReview`, after rendering chart PNG bytes (line ~119):
- Build path: `filepath.Join(storage.DataPath(), "charts", strings.ToLower(portfolioName)+"-growth.png")`
- Write PNG bytes with `os.WriteFile(path, pngBytes, 0644)`
- Log a warning if the write fails (non-fatal — the ImageContent still works)
- Append a TextContent block with the marker: `<!-- CHART_FILE:{path} -->`
- Keep the existing ImageContent block for Claude Desktop inline rendering

### 5. `.claude/skills/vire-portfolio-review/SKILL.md` — Update chart saving instructions
Update Step 3 to:
- Look for the `<!-- CHART_FILE:... -->` text content block in the MCP response
- Use `docker cp vire-mcp:{path} ./reports/{filename}-growth.png` to copy the chart from the container
- Remove the old instruction to base64-decode the ImageContent block

## Design Decisions
- **Marker format**: `<!-- CHART_FILE:... -->` is consistent with the existing `<!-- CHART_DATA -->` marker in `handleGetPortfolioHistory` (handlers.go:195)
- **Overwrite each run**: Chart file is derived data, overwritten with latest. No versioning needed.
- **Both ImageContent and TextContent**: Backward compatible — Claude Desktop sees the inline image, Claude Code reads the file path.
- **Direct `os.WriteFile`**: Binary PNG file, not JSON — no need for FileStore's JSON read/write machinery.
- **`charts/` in subdirectories**: Auto-created on startup, consistent with other data directories.
- **Lowercase portfolio name in filename**: Normalizes for consistent file naming.
- **Non-fatal write failure**: If chart file save fails, log a warning and continue (the base64 ImageContent still works).
