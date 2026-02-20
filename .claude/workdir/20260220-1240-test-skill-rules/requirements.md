# Requirements: Test Skill Mandatory Rules & Consolidation

**Date:** 2026-02-20
**Requested:** Document mandatory test rules in skill files and consolidate test-create + test-review into test-create-review.

## Scope
- Add mandatory rules to test-common (shared reference, no duplication)
- Consolidate test-create and test-review into test-create-review
- Update test-execute with structure validation rule (read-only)
- Remove old test-create and test-review skills

## Rules to Document
1. Tests executable independently via `go test` (no Claude dependency)
2. Common containerized setup, clean per test file (not necessarily per test)
3. All test files output results to `./tests/results/{datetime}-{test name/group}`
4. test-execute validates structure but NEVER modifies tests
5. test-create-review creates new tests or updates existing to match structure

## Files to Change
- `.claude/skills/test-common/SKILL.md` — Add Mandatory Rules section
- `.claude/skills/test-create-review/SKILL.md` — NEW (consolidated)
- `.claude/skills/test-execute/SKILL.md` — Add structure validation rule
- `.claude/skills/test-create/SKILL.md` — Remove (redirect to test-create-review)
- `.claude/skills/test-review/SKILL.md` — Remove (redirect to test-create-review)
