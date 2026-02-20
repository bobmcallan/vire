# Requirements: Fix Gemini URL context fallback issues

**Date:** 2026-02-20
**Requested:** Review container logs and fix the fallback issues where URL context calls fail

## Problem

Two distinct fallback errors observed in production container `a873897...`:

### Issue 1: `enrichETF` — FileData URL context (Error 400: Invalid or unsupported file uri)
- `GenerateWithURLContext()` passes ETF product page URLs as `FileData` parts
- Gemini API returns: `Invalid or unsupported file uri: https://www.globalxetfs.com.au/funds/acdc/`
- **Root cause:** `gemini-2.0-flash` no longer supports URL context (deprecated since GA release of URL context tool). The `FileData` approach with HTTP URLs was only supported during experimental phase.

### Issue 2: `summarizeFilingBatch` — URLContext tool (Error 400: Browse tool is not supported)
- `GenerateWithURLContextTool()` uses `URLContext` tool config
- Gemini API returns: `Browse tool is not supported`
- **Root cause:** Same — `gemini-2.0-flash` doesn't support URL context tool. This was removed when URL context went GA.
- This fires for every filing in a batch (~12 times per ticker), making it very noisy.

### Root Cause
`gemini-2.0-flash` is being deprecated (shutdown March 31, 2026) and no longer supports URL context features. The fix is to upgrade to `gemini-2.5-flash` which fully supports both URL context approaches.

## Scope

**In scope:**
- Upgrade default Gemini model from `gemini-2.0-flash` to `gemini-2.5-flash`
- Consolidate the two URL context methods (`GenerateWithURLContext` FileData approach + `GenerateWithURLContextTool`) into the single supported `URLContext` tool approach
- Update `enrichETF` to use the URLContext tool (pass URLs in the prompt, not as FileData)
- Update config and tests

**Out of scope:**
- Changing Gemini API key or billing
- Adding new Gemini features beyond fixing URL context
- Changing prompts/logic beyond what's needed for the method consolidation

## Approach

### 1. Upgrade model (simple)
Change `DefaultModel` from `gemini-2.0-flash` to `gemini-2.5-flash` in `internal/clients/gemini/client.go`.

### 2. Consolidate URL context methods
The `GenerateWithURLContext` (FileData approach) is deprecated. The `GenerateWithURLContextTool` (URLContext tool) is the correct approach.

- Remove `GenerateWithURLContext` (FileData approach)
- Keep and enhance `GenerateWithURLContextTool` to optionally accept URLs to include in the prompt
- Update `enrichETF` to use `GenerateWithURLContextTool`, embedding the ETF URL in the prompt text
- Update the `GeminiClient` interface

### 3. Update callers
- `enrichETF`: switch from `GenerateWithURLContext(prompt, urls)` to `GenerateWithURLContextTool(prompt)` with URL embedded in prompt
- `summarizeFilingBatch`: already uses `GenerateWithURLContextTool` — no change needed

## Files Expected to Change

| File | Change |
|------|--------|
| `internal/clients/gemini/client.go` | Upgrade model, remove FileData method, consolidate |
| `internal/interfaces/clients.go` | Remove `GenerateWithURLContext` from interface |
| `internal/services/market/enrich.go` | Update `enrichETF` to use URLContext tool |
| Tests in `tests/` | Update mock interfaces |
