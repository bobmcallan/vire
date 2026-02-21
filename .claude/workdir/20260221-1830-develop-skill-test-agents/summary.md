# Summary: Add test-creator and test-executor agents to develop workflow

**Date:** 2026-02-21
**Status:** Completed

## What Changed

| File | Change |
|------|--------|
| `.claude/skills/develop/SKILL.md` | Added test-creator and test-executor agents; restructured from 3 phases/7 tasks to 4 phases/9 tasks; added feedback loop for test failures |

## Changes Detail

### New Agents (2)

| Agent | Model | Role |
|-------|-------|------|
| test-creator | sonnet | Creates API/data integration tests in `tests/` following `test-create-review` skill |
| test-executor | sonnet | Runs tests following `test-execute` skill; feeds failures back to implementer |

### New Phase (Phase 2 — Integration Tests)

Inserted between Phase 1 (Implement) and the former Phase 2 (Verify):

- **Task: Create API and data integration tests** — owner: test-creator, blocked by review + stress-test
- **Task: Execute integration tests** — owner: test-executor, blocked by test-create

### Feedback Loop

When the test-executor finds failures:
1. Sends failure details to implementer (which tests failed, error output, files to fix)
2. Implementer fixes the issues
3. Test-executor re-runs (max 3 rounds)
4. Task marked complete only when all pass or remaining failures documented

### Updated Sections
- Step 3: 9 tasks, 4 phases (was 7 tasks, 3 phases)
- Step 4: 5 agents spawned (was 3)
- Step 5: Added coordination point for test feedback loop
- Step 6: Completion checklist includes integration tests and test-executor sign-off
- Summary template includes integration test results and feedback rounds

## Notes
- test-creator uses `bypassPermissions` mode (needs to write test files)
- test-executor uses `bypassPermissions` mode (needs to run go test commands)
- Both test agents read test-common + their respective skill files before starting
- test-executor is read-only for test code (never modifies test files)
