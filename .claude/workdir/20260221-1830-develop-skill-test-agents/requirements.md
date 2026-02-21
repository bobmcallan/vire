# Requirements: Add test-creator and test-executor agents to develop workflow

**Date:** 2026-02-21
**Requested:** Add two new agents to the develop skill: a test-creator that creates API/data integration tests in ./tests/ using .claude/skills/test-create-review, and a test-executor that runs them using .claude/skills/test-execute. Test failures feed back to the implementer for fixes.

## Scope
- Update .claude/skills/develop/SKILL.md task pipeline and agent definitions
- Add test-creator agent (creates tests in ./tests/)
- Add test-executor agent (runs tests, reports results)
- Add feedback loop: failures -> implementer fixes -> re-test

## Approach
- Add Phase 2 (Integration Tests) between existing Phase 1 (Implement) and Phase 2 (Verify)
- Expand from 7 tasks to 9 tasks, 3 agents to 5 agents
- test-creator reads test-create-review skill and creates tests following its patterns
- test-executor reads test-execute skill, runs tests, sends failures to implementer
- Implementer fixes issues, test-executor re-runs until pass

## Files Expected to Change
- `.claude/skills/develop/SKILL.md`
