# OAuth Troubleshooting Analysis for MCP Integration

**Date:** 2026-02-26
**Status:** Fix In Progress
**Severity:** Critical - Production OAuth not working

## Summary

OAuth is not working when adding the vire MCP server to Claude or ChatGPT:

| Client | Symptom |
|--------|---------|
| **Claude** | OAuth screen does not appear |
| **ChatGPT** | Service adds once, then stops working |
| **Local dev mode** | Works fine with custom URL |

This document analyzes the root causes and tracks the fixes.

## Production Architecture

| Service | URL | Access | Role |
|---------|-----|--------|------|
| **vire-pprod-portal** | `https://vire-pprod-portal.fly.dev` | Public HTTPS | User-facing UI, MCP endpoint, OAuth 2.1 provider |
| **vire-pprod-server** | `http://vire-pprod-server.internal:8080` | Private only | Backend API |
| **vire-pprod-surrealdb** | `ws://vire-pprod-surrealdb.flycast:8000/rpc` | Private only | Database |

**Key insight**: The portal implements the full OAuth 2.1 provider (discovery, authorization, tokens). All OAuth endpoints are publicly accessible.

## Root Cause Analysis

### 1. PRIMARY: Missing 401 + WWW-Authenticate Response (RFC 9728 Non-Compliance)

**Status**: Fix being implemented by portal team

**File**: `vire-portal/internal/mcp/handler.go`

**Problem**: The `/mcp` endpoint never returns 401 for unauthenticated requests. Per RFC 9728 and MCP spec, when an unauthenticated client requests a protected resource, the server MUST:

1. Return `HTTP 401 Unauthorized`
2. Include `WWW-Authenticate` header with `resource_metadata` parameter

**Current behavior**:
```go
// internal/mcp/handler.go:108-111
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    r = h.withUserContext(r)  // Tries to extract user, but doesn't fail if missing
    h.streamable.ServeHTTP(w, r)  // Always delegates, even without auth
}
```

**Expected behavior**:
```go
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    r = h.withUserContext(r)

    if _, ok := FromUserContext(r.Context()); !ok {
        resourceMetadata := fmt.Sprintf("%s://%s/.well-known/oauth-protected-resource",
            scheme, r.Host)
        w.Header().Set("WWW-Authenticate",
            fmt.Sprintf(`Bearer resource_metadata="%s"`, resourceMetadata))
        w.WriteHeader(http.StatusUnauthorized)
        // ... return error JSON
        return
    }
    h.streamable.ServeHTTP(w, r)
}
```

**Impact**: Without the 401 + WWW-Authenticate header:
- Claude Desktop never triggers the OAuth flow (it only starts OAuth after receiving 401)
- ChatGPT makes requests without auth, gets empty results, stops working

**Fix**: Portal team implementing at `vire-portal/.claude/workdir/20260226-1314-oauth-rfc9728-fix/`

### 2. Missing `bearer_methods_supported` in Protected Resource Metadata

**Status**: Fix being implemented by portal team

**File**: `vire-portal/internal/auth/discovery.go`

**Problem**: The protected resource metadata is missing `bearer_methods_supported` field required by RFC 9728.

**Current**:
```json
{
  "resource": "...",
  "authorization_servers": [...],
  "scopes_supported": [...]
}
```

**Expected**:
```json
{
  "resource": "...",
  "authorization_servers": [...],
  "scopes_supported": [...],
  "bearer_methods_supported": ["header"]
}
```

### 3. vire-server: Missing OAuth2 Issuer (Lower Priority)

**Status**: Not yet addressed

**File**: `vire-infra/fly/deploy.sh`

**Problem**: `VIRE_OAUTH2_ISSUER` is not set for vire-server. This affects vire-server's internal OAuth endpoints (for user login via Google/GitHub), not the MCP OAuth flow.

**Note**: This is separate from the MCP OAuth issue. The MCP OAuth flow is handled entirely by the portal.

## OAuth Endpoints (Already Implemented in Portal)

The portal already has all required OAuth endpoints:

| Endpoint | Purpose | Status |
|----------|---------|--------|
| `/.well-known/oauth-protected-resource` | Resource server metadata | Implemented |
| `/.well-known/oauth-authorization-server` | Authorization server metadata | Implemented |
| `/register` | Dynamic client registration | Implemented |
| `/authorize` | Authorization endpoint | Implemented |
| `/token` | Token endpoint | Implemented |

See `vire-portal/internal/server/routes.go` for route definitions.

## Why Dev Mode Works

Dev mode (`/mcp/{encrypted_uid}`) bypasses OAuth entirely by embedding user identity in the URL path. The `DevHandler` decrypts the UID and injects it into the context before delegating to the main handler.

```
Dev Mode Flow:
/mcp/{encrypted_uid} → DevHandler.decryptUID() → Inject UserContext → MCPHandler
```

## Files Being Modified

| Repository | File | Changes | Status |
|------------|------|---------|--------|
| vire-portal | `internal/mcp/handler.go` | Add 401 + WWW-Authenticate response | In Progress |
| vire-portal | `internal/auth/discovery.go` | Add `bearer_methods_supported` | In Progress |
| vire-portal | `internal/mcp/handler_test.go` | Tests for auth requirement | In Progress |

## Verification Steps

After the portal fix is deployed:

### 1. Test 401 Response

```bash
curl -v https://vire-pprod-portal.fly.dev/mcp
```

**Expected**:
```
HTTP/1.1 401 Unauthorized
Www-Authenticate: Bearer resource_metadata="https://vire-pprod-portal.fly.dev/.well-known/oauth-protected-resource"
{"error":"unauthorized","error_description":"Authentication required to access MCP endpoint"}
```

### 2. Test Protected Resource Metadata

```bash
curl https://vire-pprod-portal.fly.dev/.well-known/oauth-protected-resource
```

**Expected**:
```json
{
  "resource": "https://vire-pprod-portal.fly.dev",
  "authorization_servers": ["https://vire-pprod-portal.fly.dev"],
  "scopes_supported": ["portfolio:read", "portfolio:write", "tools:invoke"],
  "bearer_methods_supported": ["header"]
}
```

### 3. Test with Claude/ChatGPT

Add the MCP server and verify:
- OAuth consent screen should appear
- Authorization should complete
- MCP tools should be accessible

## Expected OAuth Flow (After Fix)

```
┌─────────────────┐     ┌─────────────────┐
│  Claude Desktop │     │   Vire Portal   │
└────────┬────────┘     └────────┬────────┘
         │                       │
         │ 1. POST /mcp (no auth)│
         │──────────────────────>│
         │                       │
         │ 2. 401 + WWW-Authenticate:
         │    Bearer resource_metadata=
         │    ".../.well-known/oauth-protected-resource"
         │<──────────────────────│
         │                       │
         │ 3. GET /.well-known/oauth-protected-resource
         │──────────────────────>│
         │                       │
         │ 4. JSON metadata      │
         │<──────────────────────│
         │                       │
         │ 5. OAuth flow...      │
         │                       │
         │ 6. POST /mcp          │
         │    (Bearer token)     │
         │──────────────────────>│
         │                       │
         │ 7. Success response   │
         │<──────────────────────│
```

## Related Documentation

- [Portal: OAuth Troubleshooting Findings](../../vire-portal/docs/authentication/oauth-troubleshooting-findings.md) - Detailed analysis by portal team
- [OAuth Provider Implementation](./features/20260222-oauth-provider-implementation.md) - Original OAuth implementation
- [RFC 9728: OAuth 2.0 Protected Resource Metadata](https://www.rfc-editor.org/rfc/rfc9728)
- [MCP Authorization Specification](https://modelcontextprotocol.io/specification/2025-03-26/basic/authorization)

## Timeline

| Phase | Task | Status |
|-------|------|--------|
| 1 | Portal: Add 401 + WWW-Authenticate to `/mcp` | In Progress |
| 2 | Portal: Add `bearer_methods_supported` | In Progress |
| 3 | Portal: Write tests | In Progress |
| 4 | Deploy portal fix | Pending |
| 5 | Test with Claude/ChatGPT | Pending |
