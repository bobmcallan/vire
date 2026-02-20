# Summary: Gemini Model Upgrade

**Date:** 2026-02-20
**Status:** Complete

## What changed

Upgraded the Gemini model from `gemini-2.0-flash` to `gemini-2.5-flash` and consolidated the two URL context methods into one.

### Problem

`gemini-2.0-flash` no longer supports URL context features (deprecated, shutdown March 31, 2026). This caused two distinct 400 errors in production:

1. `enrichETF` / `enrichStock` -- FileData URL context returned "Invalid or unsupported file uri"
2. `summarizeFilingBatch` / `generateNewsIntelligence` -- URLContext tool returned "Browse tool is not supported"

### Solution

1. **Model upgrade:** Changed `DefaultModel` to `gemini-2.5-flash` which fully supports the URLContext tool.
2. **Method consolidation:** Removed the deprecated FileData-based `GenerateWithURLContext(ctx, prompt, urls []string)` and the separate `GenerateWithURLContextTool(ctx, prompt)`. Replaced both with a single `GenerateWithURLContext(ctx, prompt, urls ...string)` that uses the URLContext tool config and prepends any provided URLs to the prompt text.

## Files changed

| File | Change |
|------|--------|
| `internal/clients/gemini/client.go` | Model upgrade, removed FileData method, consolidated to single URLContext method with variadic urls |
| `internal/interfaces/clients.go` | Updated `GeminiClient` interface: two methods replaced by one with `urls ...string` |
| `internal/services/market/enrich.go` | Updated `enrichETF` and `enrichStock` callers to pass URL as variadic arg |
| `internal/services/market/filings.go` | Updated `summarizeFilingBatch` to call renamed method |
| `internal/services/market/newsintel.go` | Updated `generateNewsIntelligence` to call renamed method |
| `internal/common/config.go` | Updated default config model to `gemini-2.5-flash` |
| `config/vire-service.toml.example` | Updated example config model to `gemini-2.5-flash` |
| `tests/docker/vire-blank.toml` | Updated test config model to `gemini-2.5-flash` |

## Verification

- `go build ./...` -- clean
- `go vet ./...` -- clean
- `go test ./internal/services/... -count=1` -- all pass
- Server restart and health check -- `{"status":"ok"}`
- No error/fatal/panic entries in server logs after restart

## Review notes

- The `maxURLs` field and `WithMaxURLs` option on the Client struct are now unused (callers only pass 0-1 URLs). Left as-is since it is pre-existing and does not affect correctness.
- The `maxContentSize` field was also unused before this change.
- No test mocks implement the `GeminiClient` interface, so no mock updates were needed.
