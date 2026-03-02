# Vire Filing Processor — OOM Remediation Plan

**Date:** 2026-02-25  
**Author:** Bob  
**Context:** Cold-start OOM on 2×2GB instances when processing 150+ ASX filings at startup

---

## Problem Statement

Cold-start processing of 150+ filings causes a thundering-herd memory spike on 2GB instances. Root causes stack:

- Unbounded goroutine concurrency (one goroutine per filing)
- In-memory PDF buffering (`ioutil.ReadAll` / `bytes.Buffer`)
- Go-side PDF parsing (libraries like `pdfcpu` allocate 10–20× raw file size)
- Raw bytes passed to the Gemini SDK instead of Files API URIs

---

## Priority 1 — Stop the Bleeding *(Do First, Low Risk)*

### 1.1 Set `GOMEMLIMIT`

The single highest-leverage change with **zero code required**. Add to your environment or Dockerfile:

```bash
GOMEMLIMIT=1750MiB
GOGC=50
```

This makes the Go runtime aggressively GC before hitting the OS limit rather than after. Buys immediate headroom while the deeper fixes are implemented.

---

### 1.2 Worker Pool with Hard Concurrency Cap

Replace any unbounded `go processFile(f)` fan-out with a fixed pool. Start conservative — tune upward once memory profiles are validated.

```go
const maxWorkers = 3 // start here for 2GB instances; tune based on avg filing size

func processBatch(filings []Filing) error {
    jobs := make(chan Filing, len(filings))
    var wg sync.WaitGroup
    var firstErr atomic.Value

    for range maxWorkers {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for f := range jobs {
                if err := processOne(f); err != nil {
                    firstErr.Store(err)
                }
                runtime.GC()
                debug.FreeOSMemory()
            }
        }()
    }

    for _, f := range filings {
        jobs <- f
    }
    close(jobs)
    wg.Wait()
    return nil
}
```

> The `runtime.GC()` + `debug.FreeOSMemory()` calls after each filing are heavy-handed but effective during the cold-start window. Remove them once streaming is in place.

---

## Priority 2 — Eliminate In-Memory Buffering *(High Impact)*

### 2.1 Stream Downloads to Disk

Replace `ioutil.ReadAll` / `bytes.Buffer` with streaming writes to a temp file. The file handle becomes the unit of work — not a `[]byte`.

```go
func downloadToTemp(url string) (*os.File, error) {
    resp, err := http.Get(url)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    tmp, err := os.CreateTemp("", "filing-*.pdf")
    if err != nil {
        return nil, err
    }

    if _, err := io.Copy(tmp, resp.Body); err != nil {
        tmp.Close()
        os.Remove(tmp.Name())
        return nil, err
    }

    _, err = tmp.Seek(0, io.SeekStart)
    return tmp, err
}
```

Memory profile per filing drops from `O(file_size)` to effectively zero for the download phase.

---

### 2.2 Upload Raw PDF via Gemini Files API — Skip Go-Side Parsing

Stop extracting text from PDFs in Go entirely. **Gemini 1.5 Flash/Pro reads PDFs natively.** Upload the raw file and pass the URI in the prompt.

```go
func summariseFiling(ctx context.Context, client *genai.Client, f *os.File) (string, error) {
    defer os.Remove(f.Name()) // clean up temp file when done

    uploaded, err := client.UploadFile(ctx, "", f, &genai.UploadFileOptions{
        MIMEType: "application/pdf",
    })
    if err != nil {
        return "", fmt.Errorf("upload: %w", err)
    }
    defer client.DeleteFile(ctx, uploaded.Name) // don't accumulate files in the API

    model := client.GenerativeModel("gemini-1.5-flash")
    resp, err := model.GenerateContent(ctx,
        genai.FileData{URI: uploaded.URI, MIMEType: "application/pdf"},
        genai.Text(`Summarise this ASX filing. Extract:
  - Key announcement
  - Financial impact
  - Sentiment (positive / negative / neutral)
  - Any action items`),
    )
    if err != nil {
        return "", fmt.Errorf("generate: %w", err)
    }

    return resp.Candidates[0].Content.Parts[0].(genai.Text).String(), nil
}
```

This eliminates the largest memory consumer — PDF parsing libraries like `pdfcpu` routinely allocate 10–20× the raw file size during parse.

---

## Priority 3 — Architectural Shift for Cold Start *(Medium Term)*

### 3.1 Decouple Startup from Processing

The filing download/summarisation loop **should not block or run during application startup**. Refactor so that:

1. App starts, serves traffic, and reports healthy **immediately**
2. A background job (or separate worker process) handles the initial ingest
3. Use a persistent queue (even a simple SQLite-backed job table works) so work survives restarts
4. Results are written to the DB as they complete

```
Startup sequence:
  1. App starts  →  seed job table with unfetched filings
  2. HTTP server starts  →  health check passes  →  receives traffic
  3. Background worker drains job table at controlled pace
  4. Summaries written to DB incrementally as each filing completes
```

This prevents the cold-start spike from ever impacting production traffic and allows the worker concurrency to be rate-limited independently of the web tier.

---

### 3.2 Gemini Batch API for Bulk Historical Ingest

For the initial cold-start scenario specifically (150+ filings at once), use the **Batch API** rather than real-time calls:

```
Flow:
  1. Upload all PDFs to Gemini Files API  (streaming, one at a time via worker pool)
  2. Build a .jsonl batch request file referencing their uploaded URIs
  3. Submit single batch job  →  poll for completion
  4. Parse results and write summaries to DB

Benefits:
  - 50% cost reduction vs standard API
  - Zero concurrent Gemini SDK calls on your server during processing
  - Google handles queueing; your app just polls
  - Correct pattern for any catch-up scenario (new deployment, new ticker, historical backfill)
```

---

## Priority 4 — Observability *(Do Alongside the Above)*

You can't tune what you can't measure. Add these before considering the OOM problem solved:

**Expose a memstats endpoint** — wire `runtime.ReadMemStats` to an internal `/debug/memstats` route so you can watch `HeapInuse`, `HeapIdle`, and `Sys` during a controlled cold-start replay.

**Log peak memory per filing** — record `HeapInuse` before and after `processOne()` to identify outlier filings. A 200-page prospectus behaves very differently from a 2-page quarterly update.

**Add a filing size gate** — anything over a configurable threshold (e.g., 5MB) goes to a separate low-concurrency queue with `maxWorkers = 1`.

---

## Memory Profile Comparison

| Stage | Current (Buffering) | Recommended (Streaming) |
|-------|--------------------|-----------------------|
| Download | `ioutil.ReadAll` → `[]byte` in heap | `io.Copy` to temp file on disk |
| PDF Processing | Parse in Go (`pdfcpu`) → 10–20× file size in RAM | Skip entirely — Gemini reads raw PDF |
| SDK Call | Base64 string in prompt body | Files API URI (pointer only) |
| Concurrency | Unbounded goroutines | Fixed worker pool (N=3 to start) |

---

## Implementation Sequence

| Step | Change | Effort | Expected Impact |
|------|--------|--------|-----------------|
| 1 | `GOMEMLIMIT=1750MiB` + `GOGC=50` | Minutes | Reduces OOM frequency immediately |
| 2 | Worker pool capped at 3 | Hours | Eliminates thundering herd |
| 3 | Stream downloads to temp file on disk | Hours | Eliminates download-phase heap spike |
| 4 | Remove Go PDF parser; use Gemini native PDF via Files API | 1 day | Largest single memory reduction |
| 5 | Decouple startup from processing via job table | 1 day | Eliminates cold-start risk entirely |
| 6 | Batch API for historical / catch-up ingest | 2–3 days | Right-sizes cost and memory for bulk runs |
| 7 | Memstats observability + per-filing logging | Hours | Enables ongoing tuning with real data |

> **Steps 1–4** should bring 2GB instances well within safe headroom for ongoing workloads.  
> **Step 5** is the correct long-term architecture regardless of memory constraints.

---

*Generated: 2026-02-25*
