# Requirements: Portfolio Async Collection & Job Recovery

**Date:** 2026-02-26
**Requested:** Fix 4 feedback items related to data collection and job management

## Feedback Items

| ID | Category | Description | Status |
|----|----------|-------------|--------|
| fb_c604ed2b | tool_error | SGI.AU timeout - get_stock_data consistently times out | Needs Investigation |
| fb_0a9f2081 | tool_error | Jobs lost during deploy - in-flight jobs not recovered on SIGINT | **Fix Required** |
| fb_d6d1d3bd | observation | get_portfolio should return fast + background collection | **Already Implemented** |
| fb_5184abd7 | observation | Background collection must respect freshness TTL | **Already Implemented** |

## Investigation Findings

### fb_d6d1d3bd & fb_5184abd7 - Already Implemented

The `handlePortfolioGet` function in `internal/server/handlers.go` (lines 141-156) already:
1. Returns portfolio data immediately (line 139)
2. Triggers `EnqueueTickerJobs` in a background goroutine (lines 151-154)

The `EnqueueTickerJobs` method in `internal/services/jobmanager/watcher.go` already:
1. Checks per-component freshness timestamps against TTLs
2. Only enqueues jobs for stale components

**Action:** Mark these feedback items as resolved.

### fb_0a9f2081 - Jobs Lost During Deploy

**Root Cause:** When SIGINT fires during shutdown:
1. The main context is cancelled
2. Jobs that fail try to re-enqueue using the cancelled context
3. `Enqueue(ctx, job)` fails because ctx is cancelled
4. The code falls through to `complete(ctx, job, ...)`
5. `Complete(ctx, ...)` also fails because ctx is cancelled
6. The job is left in "running" state with no recovery

**Code locations:**
- `internal/services/jobmanager/manager.go:218` - Re-enqueue fails
- `internal/services/jobmanager/queue.go:51` - Complete fails

**Fix Approach:** Use a detached context (not tied to shutdown) for cleanup operations:
- When the main context is cancelled, create a new context with a short timeout (e.g., 5s)
- Use this cleanup context for re-enqueue and complete operations
- This allows jobs to be properly re-queued or marked as failed even during shutdown

### fb_c604ed2b - SGI.AU Timeout

**Investigation Needed:** The EODHD client has a 30-second default timeout. The timeout could be caused by:
1. EODHD API being slow for this specific ticker
2. MCP tool timeout being shorter than HTTP timeout
3. A hang in data processing

**Initial Fix:** This appears to be an upstream issue. Will investigate further and potentially add per-request timeout handling or skip logic for problematic tickers.

## Scope

### In Scope
- Fix job recovery during shutdown (fb_0a9f2081)
- Mark already-implemented items as resolved (fb_d6d1d3bd, fb_5184abd7)
- Investigate SGI.AU timeout (fb_c604ed2b)

### Out of Scope
- Changes to EODHD client timeout handling (unless root cause is identified)
- Changes to portfolio sync logic (already working correctly)

## Files Expected to Change

| File | Change |
|------|--------|
| `internal/services/jobmanager/manager.go` | Add cleanup context for shutdown operations |
| `internal/services/jobmanager/queue.go` | Update complete() to accept optional cleanup context |
| `internal/services/jobmanager/shutdown.go` | New file - graceful shutdown with job recovery |

## Approach

### Phase 1: Job Recovery Fix

1. Add a `shutdownContext` that provides a short window (5-10s) for cleanup after main context is cancelled
2. Modify `processLoop` to use this context for re-enqueue and complete operations
3. Add a `RecoverRunningJobs` function called on startup to reset any orphaned "running" jobs to "pending"

### Phase 2: Verification

1. Unit tests for shutdown context handling
2. Integration test simulating SIGINT during job execution
3. Verify jobs are recovered on restart

### Phase 3: Documentation

1. Update feedback items with resolution notes
2. Update SKILL.md if shutdown behavior changes

## Acceptance Criteria

- [ ] Jobs interrupted by SIGINT are either re-queued or marked as failed
- [ ] No jobs left in "running" state after shutdown
- [ ] Recovery function on startup resets orphaned jobs
- [ ] Unit tests pass
- [ ] Integration tests pass
- [ ] Feedback items updated with resolution
