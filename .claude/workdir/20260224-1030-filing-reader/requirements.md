# Requirements: Filing Reader MCP Tool (fb_dc200885)

**Date:** 2026-02-24
**Requested:** New `read_filing` MCP tool to extract and return text content from ASX PDF filings stored in Vire's FileStore.

## Context

Vire's filing pipeline already:
1. Scrapes ASX announcements (HTML + Markit API fallback) → `CompanyFiling` structs
2. Downloads PDFs via `downloadASXPDF()` with terms-page handling → stored in FileStore under `filing_pdf` category
3. Extracts text via `extractPDFTextFromBytes()` (unexported, uses `ledongthuc/pdf`) → used only by filing summary prompt builder
4. Generates AI summaries via Gemini → `FilingSummary` structs

The gap: no MCP tool exposes the raw PDF text. Users see filing metadata but can't read the content.

## Scope

**In scope:**
- New `ReadFiling` method on `MarketService` that retrieves PDF text by ticker + document_key
- New `FilingContent` response model with text + metadata
- HTTP endpoint `GET /api/market/stocks/{ticker}/filings/{document_key}`
- MCP tool `read_filing` registered in catalog.go
- Route wiring in `routeMarketStocks`
- Unit tests for the service method
- Integration tests for the API endpoint

**Out of scope:**
- On-demand PDF downloading (if not already cached, return error)
- Changing the filing collection pipeline
- Filing search/filter capabilities
- AI re-summarization

## Approach

### Model (`internal/models/market.go`)

New struct:
```go
type FilingContent struct {
    Ticker         string    `json:"ticker"`
    DocumentKey    string    `json:"document_key"`
    Date           time.Time `json:"date"`
    Headline       string    `json:"headline"`
    Type           string    `json:"type"`
    PriceSensitive bool      `json:"price_sensitive"`
    Relevance      string    `json:"relevance"`
    PDFURL         string    `json:"pdf_url"`
    PDFPath        string    `json:"pdf_path"`
    Text           string    `json:"text"`
    TextLength     int       `json:"text_length"`
    PageCount      int       `json:"page_count"`
}
```

### Service (`internal/services/market/filings.go`)

Export `ExtractPDFTextFromBytes` (rename from unexported) and add a new `ReadFiling` method:

```go
func (s *Service) ReadFiling(ctx context.Context, ticker, documentKey string) (*models.FilingContent, error)
```

Flow:
1. Load MarketData for ticker
2. Find the `CompanyFiling` matching `documentKey`
3. Use `filing.PDFPath` to fetch bytes from `FileStore().GetFile(ctx, "filing_pdf", pdfPath)`
4. Extract text via `ExtractPDFTextFromBytes(data)`
5. Return `FilingContent` with text + filing metadata

### Interface (`internal/interfaces/services.go`)

Add to `MarketService`:
```go
ReadFiling(ctx context.Context, ticker, documentKey string) (*models.FilingContent, error)
```

### Handler (`internal/server/handlers.go`)

New `handleReadFiling(w, r, ticker, documentKey)`:
- GET only
- Validates ticker and documentKey
- Calls `MarketService.ReadFiling`
- Returns JSON response

### Route (`internal/server/routes.go`)

In `routeMarketStocks`, add handling for `/api/market/stocks/{ticker}/filings/{document_key}`:
```go
if strings.HasPrefix(path, ticker+"/filings/") {
    docKey := strings.TrimPrefix(path, ticker+"/filings/")
    s.handleReadFiling(w, r, ticker, docKey)
    return
}
```

### MCP Catalog (`internal/server/catalog.go`)

Register `read_filing` tool:
- Name: `read_filing`
- Description: "Read the text content of an ASX filing/announcement PDF. Returns extracted text, metadata, and source URL. Use document_key from filing data returned by get_stock_data."
- Method: GET
- Path: `/api/market/stocks/{ticker}/filings/{document_key}`
- Params: ticker (path, required), document_key (path, required)

## Files Expected to Change

| File | Change |
|------|--------|
| `internal/models/market.go` | Add `FilingContent` struct |
| `internal/interfaces/services.go` | Add `ReadFiling` to `MarketService` |
| `internal/services/market/filings.go` | Export `ExtractPDFTextFromBytes`, add `ReadFiling` method |
| `internal/server/handlers.go` | Add `handleReadFiling` handler |
| `internal/server/routes.go` | Route `/filings/{document_key}` in `routeMarketStocks` |
| `internal/server/catalog.go` | Register `read_filing` MCP tool |
| `internal/services/market/filings_test.go` | Unit tests for `ReadFiling` and `ExtractPDFTextFromBytes` |
| `tests/api/filing_reader_test.go` | API integration tests |
| `README.md` | Add `read_filing` to MCP tools list |
| `.claude/skills/develop/SKILL.md` | Reference `read_filing` tool |
