# Vire Multi-Tenant Architecture Plan

**Date:** 2026-02-11
**Status:** Phase 1 Completed
**Depends on:** docs/rest-api-plan.md (✅ COMPLETED)

---

## Background

Vire started as "Quaero" (Python prototype) and evolved into a Go-based MCP server for personal SMSF portfolio management. The current architecture is single-tenant:

- File-based JSON storage in a local directory
- Hardcoded Navexa API credentials in config
- No authentication on MCP or REST endpoints
- Single Docker deployment on local machine

This document outlines the path from personal tool to multi-user service while maintaining:
1. **Data security** — user data encrypted at rest, no cross-tenant leakage
2. **Horizontal scalability** — multiple server instances, no central database
3. **Deployment flexibility** — same codebase for personal Docker and hosted service

---

## Current Storage Architecture

```
data/
├── portfolios/     # Synced from Navexa (derived)
├── market/         # EODHD price/fundamentals (derived, shared)
├── signals/        # Computed signals (derived)
├── reports/        # Generated reports (derived)
├── strategies/     # User-authored investment rules
├── plans/          # User-authored action items
├── watchlists/     # User-authored watch lists
├── searches/       # Search history (derived)
├── kv/             # Key-value store (mixed)
└── charts/         # Generated chart images (derived)
```

**Observations:**
- User-authored data (strategies, plans, watchlists) is small and infrequently written
- Derived data (portfolios, market, signals, reports) is larger and frequently refreshed
- Market data is shareable across users (same EODHD data for BHP.AU)
- File-based storage works well for single-tenant but lacks atomic operations and encryption

---

## Target Architecture

### Tier 1: Personal (Current + Encryption)

Single user, local Docker deployment. Add optional encryption for data at rest.

```
┌─────────────────────────────────────────────────┐
│  Claude Desktop / Claude Code                    │
└──────────────────┬──────────────────────────────┘
                   │ MCP (stdio or HTTP)
┌──────────────────▼──────────────────────────────┐
│  vire-mcp (MCP proxy)                            │
└──────────────────┬──────────────────────────────┘
                   │ REST
┌──────────────────▼──────────────────────────────┐
│  vire-server                                     │
│  - REST API                                      │
│  - Services (portfolio, market, signals, etc.)  │
│  - File storage with optional encryption        │
└─────────────────────────────────────────────────┘
```

**Changes from current:**
- Add encryption layer to file storage (AES-256-GCM)
- Encryption key derived from user passphrase or stored in config
- No authentication (single user, local network)

### Tier 2: Multi-User (Hosted Service)

Multiple users, remote hosted. Each user has isolated encrypted storage.

```
┌─────────────────────────────────────────────────┐
│  Claude Desktop / Claude Code (User A, B, C...) │
└──────────────────┬──────────────────────────────┘
                   │ MCP over HTTPS + API Key
┌──────────────────▼──────────────────────────────┐
│  vire-mcp (stateless, multiple instances)       │
│  - Validates API key                            │
│  - Extracts user_id from key                    │
│  - Forwards to vire-server with user context    │
└──────────────────┬──────────────────────────────┘
                   │ REST + user_id header
┌──────────────────▼──────────────────────────────┐
│  vire-server (stateless, multiple instances)    │
│  - REST API with user isolation                 │
│  - Per-user encrypted storage                   │
│  - Shared market data cache                     │
└──────────────────┬──────────────────────────────┘
                   │
┌──────────────────▼──────────────────────────────┐
│  Distributed Storage                             │
│  - User data: encrypted blobs (S3/GCS/local)    │
│  - Market data: shared cache (Redis/file)       │
│  - No central SQL database                      │
└─────────────────────────────────────────────────┘
```

---

## Storage Strategy: Encrypted Blob Store

### Why Not a Traditional Database?

| Approach | Pros | Cons |
|----------|------|------|
| PostgreSQL/MySQL | ACID, queries, mature | Central point of failure, scaling complexity, encryption at field level is complex |
| BadgerDB (embedded) | Fast, embedded, Go-native | Single-process only, no horizontal scaling |
| File-based JSON | Simple, portable, human-readable | No atomic operations, no encryption |
| **Encrypted blob store** | Portable, scalable, secure by default | No queries (acceptable for Vire's access patterns) |

**Vire's access patterns:**
- Read/write by key (portfolio name, ticker, user_id)
- No cross-entity queries ("find all users with X")
- Small documents (strategies ~2KB, portfolios ~50KB, market data ~10KB)
- Infrequent writes (strategies change rarely, market data refreshes daily)

These patterns fit a key-value blob store perfectly. No need for SQL.

### Encryption Model

**Per-user encryption key:**
```
user_master_key = HKDF(user_passphrase, salt=user_id, info="vire-storage")
```

**Document encryption:**
```
encrypted_blob = AES-256-GCM(plaintext_json, user_master_key, nonce)
stored_blob = nonce || encrypted_blob || auth_tag
```

**Key management options:**

| Option | Security | UX | Complexity |
|--------|----------|-----|------------|
| User passphrase | High (user controls key) | Must enter on each session | Low |
| Server-managed key | Medium (server can decrypt) | Seamless | Medium |
| HSM/KMS | High (hardware-backed) | Seamless | High |

**Recommendation for Tier 2:** Server-managed keys stored in a secrets manager (AWS Secrets Manager, HashiCorp Vault). User data is encrypted at rest, but the server can decrypt to serve requests. This balances security with usability.

### OAuth/OIDC (Future)

For web UI or third-party integrations, add OAuth 2.0:
- Google/GitHub login for user identity
- Issue Vire API keys linked to OAuth identity
- Revoke keys when OAuth session expires

Not needed for initial multi-tenant — API keys are sufficient for MCP clients.

---

## Storage Backend Options

### Option A: S3-Compatible Object Storage (Recommended)

```
s3://vire-data/
├── users/
│   ├── usr_abc123/
│   │   ├── strategies/SMSF.json.enc
│   │   ├── plans/SMSF.json.enc
│   │   ├── watchlists/SMSF.json.enc
│   │   ├── portfolios/SMSF.json.enc
│   │   └── ...
│   └── usr_def456/
│       └── ...
└── market/                    # Shared, unencrypted
    ├── BHP.AU.json
    ├── CBA.AU.json
    └── ...
```

**Pros:**
- Horizontally scalable (S3 handles it)
- No server state (any vire-server instance can serve any user)
- Built-in durability and replication
- Works with AWS S3, GCS, MinIO, Cloudflare R2

**Cons:**
- Higher latency than local file (~50-100ms per read)
- Requires caching layer for hot data

**Caching strategy:**
- Local file cache on each vire-server instance
- Cache user data on first access, invalidate on write
- Market data cached with TTL (1 hour for EOD, 1 min for real-time)

### Option B: Distributed KV Store (Redis Cluster / DynamoDB)

```
Key pattern: vire:{user_id}:{type}:{name}
Example:     vire:usr_abc123:strategy:SMSF
```

**Pros:**
- Lower latency (~5-10ms)
- Built-in TTL for cache entries
- Atomic operations

**Cons:**
- More operational complexity
- Cost scales with data size
- Redis Cluster requires careful configuration

### Option C: Hybrid (Recommended for Production)

- **Hot data (portfolios, signals, reports):** Redis with TTL
- **Cold data (strategies, plans, watchlists):** S3 with local cache
- **Market data:** Redis (shared across users) with S3 backup

This gives low latency for frequently accessed data while keeping costs manageable.

---

## Implementation Phases

### Phase 1: Storage Abstraction Layer ✅ COMPLETED

**Implemented:** 2026-02-10

Provider-agnostic blob storage interface supporting file (current), GCS (future), and S3 (future) backends.

**Files created:**
- `internal/storage/blob.go` — BlobStore interface and config types
- `internal/storage/file_blob.go` — FileBlobStore implementation
- `internal/storage/factory.go` — NewBlobStore factory function
- `internal/storage/blob_test.go` — Comprehensive tests

**BlobStore interface:**
```go
type BlobStore interface {
    Get(ctx context.Context, key string) ([]byte, error)
    GetReader(ctx context.Context, key string) (io.ReadCloser, error)
    Put(ctx context.Context, key string, data []byte) error
    PutReader(ctx context.Context, key string, r io.Reader, size int64) error
    Delete(ctx context.Context, key string) error
    Exists(ctx context.Context, key string) (bool, error)
    Metadata(ctx context.Context, key string) (*BlobMetadata, error)
    List(ctx context.Context, opts ListOptions) (*ListResult, error)
    Close() error
}
```

**Configuration (config.toml):**
```toml
[storage]
backend = "file"  # "file" (default), "gcs", "s3"

[storage.file]
path = "data"
versions = 5

[storage.gcs]     # Future Phase 2
bucket = ""
prefix = ""
credentials_file = ""

[storage.s3]      # Future Phase 2
bucket = ""
prefix = ""
region = ""
endpoint = ""     # Custom endpoint for S3-compatible stores (MinIO, R2)
access_key = ""
secret_key = ""
```

**Key features:**
- Atomic writes (temp file + rename for file backend)
- Path traversal protection (sanitizeKey)
- Streaming support (GetReader/PutReader for large blobs)
- Metadata with ETags (for conditional operations)
- Prefix-based listing with pagination support

**Backward compatibility:**
- Default backend is "file" — existing deployments work unchanged
- Domain-specific storage (portfolioStorage, etc.) unchanged
- Manager now exposes both blob store and legacy file store

### Phase 2: User Context Propagation

Add user context to all service calls:

```go
type UserContext struct {
    UserID        string
    EncryptionKey []byte  // Derived from user's master key
}

func (s *PortfolioService) GetPortfolio(ctx context.Context, uc *UserContext, name string) (*models.Portfolio, error) {
    key := fmt.Sprintf("%s/portfolios/%s", uc.UserID, name)
    data, err := s.storage.Get(ctx, key)
    // ...
}
```

**Backward compatibility:**
- If `UserContext` is nil or `UserID` is empty, use current behavior (no prefix)
- Personal mode continues to work unchanged

### Phase 3: Authentication Middleware

Add to `internal/server/middleware.go`:

```go
func (s *Server) authMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if s.config.Auth.Enabled {
            apiKey := extractAPIKey(r)
            userCtx, err := s.validateAPIKey(apiKey)
            if err != nil {
                http.Error(w, "Unauthorized", 401)
                return
            }
            ctx := context.WithValue(r.Context(), userContextKey, userCtx)
            next.ServeHTTP(w, r.WithContext(ctx))
        } else {
            // Personal mode — no auth
            next.ServeHTTP(w, r)
        }
    })
}
```

### Phase 4: MCP Proxy Authentication

Update `cmd/vire-mcp/` to:
1. Accept API key from environment or MCP client headers
2. Pass API key to vire-server in `Authorization` header
3. Handle 401 responses gracefully

### Phase 5: Shared Market Data

Market data (prices, fundamentals) is identical for all users. Optimize:

1. **Separate storage path:** `market/` not under `users/{user_id}/`
2. **Shared cache:** Single Redis/file cache for market data
3. **No encryption:** Market data is public, no need to encrypt

This reduces storage costs and improves cache hit rates.

---

## Configuration

### Personal Mode (Default)

```toml
[storage]
backend = "file"
path = "./data"

[storage.encryption]
enabled = false

[auth]
enabled = false
```

### Multi-Tenant Mode

```toml
[storage]
backend = "s3"
bucket = "vire-data"
region = "ap-southeast-2"

[storage.encryption]
enabled = true
key_source = "secrets_manager"  # or "env", "file"
key_arn = "arn:aws:secretsmanager:..."

[storage.cache]
backend = "redis"
url = "redis://localhost:6379"
ttl_seconds = 3600

[auth]
enabled = true
keys_backend = "dynamodb"  # or "redis", "file"
keys_table = "vire-api-keys"
```

---

## Security Considerations

### Data Isolation

- All storage keys prefixed with `user_id`
- No API endpoint returns data without user context
- Market data (shared) contains no PII

### Encryption

- User data encrypted with per-user keys
- Keys never logged or exposed in errors
- Encryption at rest (S3 server-side encryption) as additional layer

### API Key Security

- Keys hashed before storage (SHA-256)
- Keys rotatable without data migration
- Rate limiting per API key
- Audit log of key usage

### Network Security

- HTTPS required for multi-tenant mode
- mTLS optional for service-to-service
- API keys transmitted only in headers, never in URLs

---

## Migration Path

### From Personal to Multi-Tenant

1. **Export:** Dump current file storage to JSON
2. **Create user:** Generate `user_id` and API key
3. **Import:** Upload to S3 under `users/{user_id}/`
4. **Encrypt:** Re-encrypt with user's key
5. **Configure:** Update config to multi-tenant mode
6. **Test:** Verify all data accessible via API key

### Rollback

- Keep file-based storage as fallback
- Config switch between backends
- No schema migration (same JSON format)

---

## Estimated Effort

| Phase | Description | Effort |
|-------|-------------|--------|
| Phase 1 | Storage abstraction + encryption | 2-3 days |
| Phase 2 | User context propagation | 1-2 days |
| Phase 3 | Auth middleware | 1 day |
| Phase 4 | MCP proxy auth | 0.5 day |
| Phase 5 | Shared market data | 1 day |
| **Total** | | **6-8 days** |

---

## Open Questions

1. **Key management for personal mode:** Should personal users set a passphrase, or is unencrypted acceptable for local Docker?

2. **Navexa credentials:** Currently in config. For multi-tenant, each user needs their own Navexa API key. Store encrypted in user data? OAuth flow?

3. **EODHD API key:** Shared across users (service pays) or per-user (user pays)?

4. **Rate limiting:** Per-user limits? Global limits? How to handle burst from AI agents?

5. **Billing:** If hosted service, how to meter usage? Per API call? Per portfolio? Flat subscription?

---

## Summary

The path from personal to multi-tenant:

1. **Abstract storage** — interface over file/S3/Redis backends
2. **Add encryption** — AES-256-GCM with per-user keys
3. **Add authentication** — API keys with user context
4. **Isolate user data** — key prefixes, no cross-tenant access
5. **Share market data** — single cache, no encryption needed

This maintains the "no central database" principle while enabling horizontal scaling and data security. The same codebase serves personal Docker deployments and hosted multi-user service.
