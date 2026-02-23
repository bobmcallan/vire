# Summary: Fix portfolio dashboard display fields

**Date:** 2026-02-22
**Status:** completed

## What Changed

| File | Change |
|------|--------|
| `vire-portal/pages/dashboard.html` | Added TOTAL COST summary item, renamed TOTAL GAIN labels to NET RETURN, updated per-holding columns to use `net_return`/`net_return_pct` with color coding, renamed table headers to Return $/% |
| `docs/portfolio/portfolio_updates.md` | Documented FX conversion, field renames, cache invalidation, and dashboard display |

## Details

The portal dashboard was not displaying portfolio return data correctly after the backend field rename in commit `8054b79` (gain_loss -> net_return). Three fixes applied:

1. **Portfolio summary bar**: Labels changed from "TOTAL GAIN $"/"TOTAL GAIN %" to "NET RETURN $"/"NET RETURN %". Added "TOTAL COST" item between TOTAL VALUE and NET RETURN. The JS (`common.js` lines 253-255) already read the correct backend fields (`total_net_return`, `total_net_return_pct`, `total_cost`).

2. **Holdings table columns**: Changed from `h.total_return_pct` (old, non-existent field) to `h.net_return` and `h.net_return_pct`. Added `gainClass()` color coding (green for positive, red for negative). Table headers renamed from "Gain $"/"Gain %" to "Return $"/"Return %".

3. **Deployment**: Rebuilt portal Docker image (`vire-portal:0.2.35`) and restarted container. Verified dashboard HTML served correctly from container.

## Notes
- No JS changes required — `common.js` already mapped the correct API response fields
- Portal is a separate repo (`vire-portal/`) from the backend (`vire/`)
- CSS classes `.gain-positive` (#2d8a4e) and `.gain-negative` (#a33) were already defined
