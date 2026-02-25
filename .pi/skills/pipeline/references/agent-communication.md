# Agent Communication

This document describes how agents communicate during the development pipeline.

## Communication Methods

### Method 1: Dispatcher (Primary Agent) Relay

The primary agent running the pipeline skill acts as a coordinator:

```
User → Pipeline Skill (dispatcher) → implementer → Pipeline Skill → reviewer
                                                          ↓
                                        Pipeline Skill ← devil's-advocate
```

**Pattern:**
1. Dispatcher dispatches agent with task
2. Agent completes task and returns result
3. Dispatcher reads result and decides next action
4. Dispatcher may forward findings to another agent

### Method 2: Direct Agent-to-Agent

Agents communicate directly using the `dispatch_agent` tool:

```json
{
  "agent": "implementer",
  "task": "Fix the following issues found by reviewer:\n\n<issues>"
}
```

**When to use:**
- Simple fix requests
- Clarifying questions
- Test failure notifications

**When NOT to use:**
- Complex multi-agent coordination
- Conflict resolution
- Pipeline state management

## Message Types

### 1. Task Assignment

```json
{
  "type": "task_assignment",
  "task_id": "implement-feature",
  "description": "Implement feature X with tests",
  "context": "See requirements.md for details",
  "files": ["path/to/file1.go", "path/to/file2.go"],
  "constraints": [
    "Follow existing patterns",
    "Add unit tests",
    "No external dependencies"
  ]
}
```

### 2. Review Findings

```json
{
  "type": "review_findings",
  "reviewer": "reviewer",
  "status": "changes_requested",
  "findings": [
    {
      "severity": "high",
      "file": "handlers/feature.go",
      "line": 42,
      "issue": "Missing error handling",
      "recommendation": "Add error check before proceeding"
    }
  ],
  "approval": false
}
```

### 3. Security Issues

```json
{
  "type": "security_issues",
  "source": "devil's-advocate",
  "vulnerabilities": [
    {
      "severity": "critical",
      "type": "sql_injection",
      "location": "services/user.go:123",
      "exploit": "Input ' OR '1'='1 bypasses auth",
      "fix": "Use parameterized queries"
    }
  ]
}
```

### 4. Test Failures

```json
{
  "type": "test_failures",
  "source": "test-executor",
  "round": 2,
  "max_rounds": 3,
  "failures": [
    {
      "test": "TestFeatureIntegration",
      "error": "expected 200, got 500",
      "file": "tests/integration/feature_test.go:42",
      "likely_fix": "Check error handling in handlers/feature.go"
    }
  ],
  "summary": {
    "total": 10,
    "passed": 8,
    "failed": 2
  }
}
```

### 5. Clarification Request

```json
{
  "type": "clarification_request",
  "from": "implementer",
  "question": "Should the feature support pagination?",
  "context": "The requirements mention 'list all items' but don't specify limits",
  "options": [
    "Add pagination (offset/limit)",
    "Add cursor-based pagination",
    "Return all items (no pagination)"
  ]
}
```

## Communication Patterns

### Pattern 1: Sequential Handoff

```
implementer → (completes) → dispatcher → reviewer → (completes) → dispatcher
```

Use for: Phase dependencies where each agent must finish before next starts.

### Pattern 2: Parallel Execution + Merge

```
                → reviewer → (results) →
implementer →                            → dispatcher (merge findings)
                → devil's-advocate → (results) →
```

Use for: Phase 1 review + stress-test running in parallel.

### Pattern 3: Feedback Loop

```
test-executor → (failure) → implementer → (fix) → test-executor → (retry)
                                                              ↓
                                                      (success or max retries)
```

Use for: Test failures requiring fixes before proceeding.

### Pattern 4: Escalation

```
agent → (critical issue) → dispatcher → (user decision) → continue/abort
```

Use for: Critical issues requiring user input (e.g., security vulnerabilities, design decisions).

## Dispatcher Responsibilities

The primary agent (dispatcher) running the pipeline skill is responsible for:

1. **Task Sequencing**: Start tasks in the right order
2. **Dependency Management**: Wait for blocked tasks to complete
3. **Result Routing**: Forward findings to appropriate agents
4. **Conflict Resolution**: Make final decisions when agents disagree
5. **Progress Tracking**: Know which phase and agent is active
6. **Timeout Enforcement**: Stop runaway tasks
7. **Max Retry Enforcement**: Stop after N feedback loop rounds
8. **User Communication**: Report pipeline status to user

## Agent Output Format

All agents should end their responses with structured output:

```markdown
## Summary
<brief description of what was done>

## Changes Made
- file/path: description
- another/file: description

## Issues Found (if applicable)
- Severity: description

## Next Steps (if applicable)
- Recommended actions

## Status
- [ ] Complete / [ ] Needs follow-up / [ ] Blocked
```

## Best Practices

1. **Be specific**: Include file paths, line numbers, exact errors
2. **Be actionable**: Provide clear recommendations, not vague issues
3. **Be concise**: Summarize findings, don't dump entire logs
4. **Use structured data**: JSON for machine-readable messages
5. **Include context**: Why is this issue important?
6. **Suggest fixes**: Don't just find problems, propose solutions
7. **Set expectations**: What needs to happen next?
8. **Avoid redundancy**: Don't repeat what other agents have found

## Example: Test Failure Feedback Loop

### Round 1

**test-executor → dispatcher:**
```json
{
  "type": "test_failures",
  "failures": [
    {
      "test": "TestFeatureAPI",
      "error": "expected 200, got 500",
      "file": "tests/integration/api_test.go:42"
    }
  ]
}
```

**dispatcher → implementer:**
```json
{
  "type": "task_assignment",
  "task_id": "fix-test-failures",
  "description": "Fix test failures from round 1:\n\nTestFeatureAPI: expected 200, got 500 at tests/integration/api_test.go:42\n\nLikely issue: Check error handling in handlers/feature.go",
  "round": 1,
  "max_rounds": 3
}
```

### Round 2

**implementer → dispatcher:**
```markdown
## Fixed Issues
- handlers/feature.go: Added error handling for missing input

## Tests
- Local test passes: go test ./internal/...
```

**dispatcher → test-executor:**
```json
{
  "type": "task_assignment",
  "task_id": "rerun-tests",
  "description": "Re-run integration tests after fixes in round 1",
  "round": 2
}
```

**test-executor → dispatcher:**
```json
{
  "type": "test_results",
  "status": "passed",
  "summary": {
    "total": 10,
    "passed": 10,
    "failed": 0
  }
}
```

**dispatcher → user:**
```markdown
## Phase 2 Complete ✓
- Integration tests: 10/10 passed
- Rounds: 2 (1 failure, fixed in round 2)
- Proceeding to Phase 3...
```
