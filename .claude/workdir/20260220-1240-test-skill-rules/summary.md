# Summary: Test Skill Mandatory Rules & Consolidation

**Date:** 2026-02-20
**Status:** Completed

## What Changed

| File | Change |
|------|--------|
| `.claude/skills/test-common/SKILL.md` | Added Mandatory Rules section (Rules 1-4) at top of file |
| `.claude/skills/test-create-review/SKILL.md` | NEW: Consolidated skill with create, review, and audit actions |
| `.claude/skills/test-execute/SKILL.md` | Added mandatory structure validation step (read-only), report format |
| `.claude/skills/test-create/SKILL.md` | Replaced with redirect to test-create-review |
| `.claude/skills/test-review/SKILL.md` | Replaced with redirect to test-create-review |

## Mandatory Rules (in test-common)

1. **Tests independent of Claude** — executable via `go test`, no AI dependencies
2. **Common containerized setup** — clean per test file, uses `tests/common/` helpers
3. **Results output** — every test writes to `tests/results/{datetime}-{TestName}/`
4. **test-execute is read-only** — validates structure, reports non-compliance, never modifies tests

## Skill Consolidation

- `/test-create` + `/test-review` → `/test-create-review`
- Three actions: `create` (scaffold new), `review` (check and fix), `audit` (report only)
- Old skills redirect to the new consolidated skill

## Notes
- Rules are defined once in test-common, referenced by test-create-review and test-execute (no duplication)
- test-execute now has a mandatory Step 1 that validates structure before running tests
- Non-compliance in test-execute doesn't block execution — it documents and advises `/test-create-review`
