# Task Dependencies

This document describes the dependency structure between tasks in the development pipeline.

## Dependency Graph

```
Phase 1 (Implement)
├── Task 1: Implement feature (implementer)
│   └── No dependencies
├── Task 2: Review implementation (reviewer)
│   └── Blocked by: Task 1
└── Task 3: Stress-test (devil's-advocate)
    └── Blocked by: Task 1

Phase 2 (Integration Tests)
├── Task 4: Create integration tests (test-creator)
│   └── Blocked by: Task 2 AND Task 3
└── Task 5: Execute integration tests (test-executor)
    └── Blocked by: Task 4
    └── Feedback loop to implementer on failure

Phase 3 (Verify)
├── Task 6: Build and test (implementer)
│   └── Blocked by: Task 5 (all tests passing)
└── Task 7: Validate deployment (reviewer)
    └── Blocked by: Task 6

Phase 4 (Document)
├── Task 8: Update documentation (implementer)
│   └── Blocked by: Task 7
└── Task 9: Verify documentation (reviewer)
    └── Blocked by: Task 8
```

## Execution Strategy

### Phase 1
- **Sequential**: Run implementer first, wait for completion
- **Parallel**: Run reviewer AND devil's-advocate concurrently after implementer finishes
- **Block**: Phase 2 cannot start until both review tasks complete

### Phase 2
- **Sequential**: Run test-creator, wait for completion
- **Sequential**: Run test-executor after test-creator finishes
- **Feedback Loop**: If tests fail, dispatch implementer with failure details, then re-run test-executor
- **Max Retries**: 3 rounds of test failures before documenting remaining issues

### Phase 3
- **Sequential**: Run implementer (build/test), wait for completion
- **Sequential**: Run reviewer (validate) after implementer finishes
- **Block**: Phase 4 cannot start until validation passes

### Phase 4
- **Sequential**: Run implementer (docs), wait for completion
- **Sequential**: Run reviewer (verify docs) after implementer finishes
- **Complete**: Pipeline finishes when documentation verification passes

## Task Status

Each task has one of four states:

1. **pending** — Task not yet started
2. **in_progress** — Agent actively working on task
3. **completed** — Task finished successfully
4. **blocked** — Waiting for dependencies to complete

## Parallel Execution Limits

- **Max concurrent agents**: 2 (reviewer + devil's-advocate in Phase 1)
- **Reason**: Avoids context confusion and ensures clear task ownership
- **Exception**: Phase 2 feedback loop may have test-executor waiting while implementer fixes

## Timeout Guidelines

| Phase | Task Type | Timeout | Reason |
|-------|-----------|---------|--------|
| 1 | Implement | 600s | Complex implementation may take time |
| 1 | Review | 300s | Code review should be faster |
| 1 | Stress-test | 450s | Security testing needs thoroughness |
| 2 | Test-create | 600s | Writing comprehensive tests |
| 2 | Test-execute | 300s | Running tests + Docker startup |
| 3 | Build/test | 300s | Verification should be quick |
| 3 | Validate | 200s | Final checks |
| 4 | Update docs | 300s | Documentation changes |
| 4 | Verify docs | 200s | Doc review |

## Failure Handling

### Critical Failure (e.g., build fails, security vulnerability)
1. Stop pipeline execution
2. Report issue to user
3. Wait for user decision before continuing

### Non-Critical Failure (e.g., test failure, doc typo)
1. Continue pipeline execution
2. Document issue in summary
3. Create follow-up task for later

### Test Failure Feedback Loop
1. test-executor reports failures to implementer
2. implementer fixes issues
3. test-executor re-runs tests
4. Repeat up to 3 rounds
5. After 3 rounds, document remaining issues and proceed
