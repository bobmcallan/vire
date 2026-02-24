# Summary: Filing Reader MCP Tool (fb_dc200885)

**Date:** 2026-02-24
**Status:** completed

## What Changed

| File | Change |
|------|--------|
| `internal/models/market.go` | Added `FilingContent` struct (16 lines) |
| `internal/interfaces/services.go` | Added `ReadFiling` to `MarketService` interface |
| `internal/services/market/filings.go` | Exported `ExtractPDFTextFromBytes`, added `ReadFiling` method + `countPDFPages` helper (89 lines) |
| `internal/server/handlers.go` | Added `handleReadFiling` handler (28 lines) |
| `internal/server/routes.go` | Added `/filings/{document_key}` route in `routeMarketStocks` (8 lines) |
| `internal/server/catalog.go` | Registered `read_filing` MCP tool (10 lines) |
| `internal/server/catalog_test.go` | Updated catalog test for new tool |
| `internal/services/market/filings_test.go` | Unit tests for `ExtractPDFTextFromBytes` and `ReadFiling` (197 lines) |
| `internal/services/market/filings_stress_test.go` | Devils-advocate stress tests (777 lines) |
| `internal/services/market/core_stress_test.go` | Updated to use exported `ExtractPDFTextFromBytes` |
| `internal/services/jobmanager/manager_test.go` | Updated mock for new interface method |
| `internal/services/report/devils_advocate_test.go` | Updated mock for new interface method |

## New MCP Tool

| Tool | Method | Path | Description |
|------|--------|------|-------------|
| `read_filing` | GET | `/api/market/stocks/{ticker}/filings/{document_key}` | Read ASX filing PDF text by ticker + document key |

## Tests
- Unit tests: 7 tests in `filings_test.go` (empty data, corrupt data, success, doc key not found, no market data, empty PDF path, file store error)
- Stress tests: 777 lines in `filings_stress_test.go` (security, edge cases, hostile inputs)
- Integration tests: `tests/api/filing_reader_test.go` created
- All unit tests pass, go vet clean

## Documentation Updated
- `README.md` — added `read_filing` to MCP tools table
- `.claude/skills/develop/SKILL.md` — added `ReadFiling` to MarketService methods table and filing reader endpoint

## Devils-Advocate Findings
- Input validation tested for path traversal, injection, special characters
- Panic recovery verified for corrupt PDF data
- Error handling verified for missing files, missing market data

## Notes
- `ExtractPDFTextFromBytes` exported (was unexported) to enable reuse from `ReadFiling`
- `countPDFPages` helper added with independent panic recovery
- Returns 404 when filing or PDF not found, 500 for internal errors
- PDF text truncated at 50k chars (existing behaviour preserved)
- Pre-existing SurrealDB stress test failure unrelated to this feature
