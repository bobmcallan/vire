# Requirements: Fix portfolio dashboard display fields

**Date:** 2026-02-22
**Requested:** Three frontend fixes to the portfolio dashboard at /dashboard

## Scope
- Fix total gain $ and % display (currently showing 0 due to backend field rename)
- Add total cost field to portfolio summary
- Fix per-holding net_return and net_return_pct display with color coding
- Out of scope: Backend changes (already fixed in commit a72b335)

## Changes

### 1. Portfolio Summary (dashboard.html)
- Labels already say "TOTAL GAIN $" / "TOTAL GAIN %" — keep or rename to "NET RETURN"
- Add TOTAL COST item between TOTAL VALUE and NET RETURN
- JS already reads `total_net_return` / `total_net_return_pct` correctly (lines 253-254 common.js)

### 2. Per-holding Table (dashboard.html)
- Current: reads `h.total_return_pct` — this field no longer exists in backend response
- Fix: use `h.net_return_pct` for percentage column
- Add: `h.net_return` dollar column with color coding via `gainClass()`

### 3. Files to Change
- `/home/bobmc/development/vire-portal/pages/dashboard.html`
- No JS changes needed (field names already correct in common.js)
