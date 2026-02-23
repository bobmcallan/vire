# Requirements: Sync portfolio dashboard labels with API field names

**Date:** 2026-02-23
**Requested:** Align portal dashboard display labels with the backend's net_return field naming and docs/portfolio/portfolio_updates.md specification.

## Scope
- Rename summary labels: "TOTAL GAIN $" → "NET RETURN $", "TOTAL GAIN %" → "NET RETURN %"
- Rename table headers: "Gain $" → "Return $", "Gain %" → "Return %"
- Update portal tests (UI + stress) to expect new labels
- Rebuild and deploy portal Docker image
- Commit portal changes (vire-portal repo)
- Commit documentation (vire repo: docs/portfolio/portfolio_updates.md)
- Out of scope: Backend changes (already committed at a72b335), JS logic changes (already correct)

## Approach
Direct label renames in dashboard.html and corresponding test string updates. No logic changes needed — the Alpine.js data bindings and JS field mappings already use net_return terminology.

## Files Expected to Change

### vire-portal repo
- `pages/dashboard.html` — 4 label renames
- `tests/ui/dashboard_test.go` — update expected label strings
- `internal/handlers/dashboard_stress_test.go` — update expected label strings

### vire repo
- `docs/portfolio/portfolio_updates.md` — already written, just needs committing
