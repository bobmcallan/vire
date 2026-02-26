# Directory Structure

| Component | Location |
|-----------|----------|
| Entry Point | `cmd/vire-server/` |
| Application | `internal/app/` |
| Services | `internal/services/` |
| Job Manager | `internal/services/jobmanager/` |
| Clients | `internal/clients/` |
| Models | `internal/models/` |
| Config (code) | `internal/common/config.go` |
| Config (files) | `config/` |
| Signals | `internal/signals/` |
| HTTP Server | `internal/server/` |
| Storage | `internal/storage/` |
| Interfaces | `internal/interfaces/` |
| User Context | `internal/common/userctx.go` |
| Tests | `tests/` |
| Docker | `docker/` |
| Scripts | `scripts/` |
| Skills | `.claude/skills/` |

## Test Layout

```
tests/
├── api/           # API integration tests (Docker-based)
├── common/        # Test infra (containers.go, mocks.go)
├── docker/        # Docker test configs (.env.example)
├── fixtures/      # Test data
├── import/        # Import data (users.json)
└── logs/          # Test output (gitignored)
```

## Key Models

- `internal/models/storage.go` — InternalUser, UserKeyValue, UserRecord
- `internal/models/jobs.go` — StockIndexEntry, Job, JobEvent
- `internal/models/cashflow.go` — CashTransaction, CashFlowLedger, CapitalPerformance
- `internal/models/portfolio.go` — Portfolio, ExternalBalance, PortfolioIndicators
- `internal/models/market.go` — MarketData, QualityAssessment
- `internal/models/oauth.go` — OAuthClient, OAuthCode, OAuthRefreshToken
- `internal/models/watchlist.go` — WatchlistItemReview, WatchlistReview
