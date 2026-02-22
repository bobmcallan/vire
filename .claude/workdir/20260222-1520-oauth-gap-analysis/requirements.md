# Requirements: OAuth Provider Gap Analysis

**Date:** 2026-02-22
**Requested:** Verify implementation against docs/authentication/vire-server-oauth-requirements.md, identify gaps

## Scope
- Audit all 4 OAuth endpoints against the requirements spec
- Verify state parameter handling matches spec (or is equivalent/better)
- Verify user mapping strategy (email-based matching, account linking)
- Verify JWT claims format matches portal contract
- Verify error handling and redirect behaviour
- Verify config and env var support
- Verify test coverage against spec's test list
- Identify any deviations and assess whether they're improvements or gaps

## Out of Scope
- Portal-side changes (spec says none required except optional error param check)
- New feature additions beyond what's in the spec

## Approach
Read implementation files against each section of the requirements doc. Produce a gap report.

## Files to Audit
- `internal/server/handlers_auth.go` — OAuth handlers
- `internal/server/handlers_user.go` — user management
- `internal/server/routes.go` — route registration
- `internal/common/config.go` — auth config
- `internal/server/middleware.go` — user context
- `internal/server/handlers_auth_test.go` — auth unit tests
- `tests/api/user_test.go` — integration tests
- `config/vire-service.toml` — config file
