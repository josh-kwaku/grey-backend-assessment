# Sprint Progress

> Updated when sprint/task status changes.

## Overview

| Sprint | Name | Status | Notes |
|--------|------|--------|-------|
| 01 | Project Scaffolding | `done` | Go mod, docker-compose, migrations, config, logging |
| 02 | Domain Models & DB Layer | `done` | All tasks complete |
| 03 | Auth & Seeding | `done` | JWT, seed data, response envelope, middleware, AppError centralization |
| 04 | Account Creation | `done` | Account service, handler, ownership helper, route wiring |
| 05 | Internal Transfer (Same Currency) | `done` | Payment service (sub-package), handler, lock ordering, double-entry ledger |
| 06 | FX & Cross-Currency | `done` | FX rate service, 4-entry ledger, self-conversion, system account helper |
| 07 | External Payout | `done` | Mock provider, webhook, outbox, reversals, defense-in-depth fixes |
| 08 | Idempotency Middleware | `done` | Response caching, replay, conflict detection, per-user scoping |
| 09 | Transaction History | `skipped` | Out of scope per assessment PDF |
| 10 | Tests | `done` | Unit + integration tests (testcontainers) |
| 11 | Polish | `pending` | Health, graceful shutdown, README |

## Detailed Task Tracking

### Sprint 01: Project Scaffolding

- [x] 1.1 Initialize Go module
- [x] 1.2 Create project directory structure
- [x] 1.3 Create configuration loader
- [x] 1.4 Create database migration files
- [x] 1.5 Create Dockerfiles
- [x] 1.6 Create docker-compose.yml
- [x] 1.7 Create minimal main.go entrypoints
- [x] 1.8 Create .env.example
- [x] 1.9 Create logging package and standards (added mid-sprint)

### Sprint 02: Domain Models & DB Layer

- [x] 2.1 Define domain types
- [x] 2.2 Define repository interfaces (consumer-side)
- [x] 2.3 Implement concrete repositories
- [x] 2.4 Create database connection helper

### Sprint 03: Auth & Seeding

- [x] 3.1 Create seed migration
- [x] 3.2 Implement JWT utility
- [x] 3.3 Implement response envelope helpers
- [x] 3.4 Implement auth middleware
- [x] 3.5 Implement login handler
- [x] 3.6 Implement user handler
- [x] 3.7 Implement request tracing middleware
- [x] 3.8 Implement logging middleware
- [x] 3.9 Implement recovery middleware
- [x] 3.10 Wire routes and middleware

### Sprint 04: Account Creation

- [x] 4.1 Implement account service
- [x] 4.2 Implement account handler
- [x] 4.3 Wire routes

### Sprint 05: Internal Transfer (Same Currency)

- [x] 5.1 Implement payment service (CreateInternalTransfer)
- [x] 5.2 Implement payment handler (POST + GET)
- [x] 5.3 Implement lock accounts helper (folded into 5.1)
- [x] 5.4 Wire routes

### Sprint 06: FX & Cross-Currency

- [x] 6.1 Implement FX rate service
- [x] 6.2 Implement FX rate handler
- [x] 6.3 Extend payment service for cross-currency
- [x] 6.4 Add system account lookup helper (folded into 6.3)
- [x] 6.5 Wire FX rate route

### Sprint 07: External Payout

- [x] 7.1 Implement mock provider service
- [x] 7.2 Implement webhook handler
- [x] 7.3 Implement webhook processor
- [x] 7.4 Implement external payout in payment service
- [x] 7.5 Implement mock provider client
- [x] 7.6 Implement external payout handler
- [x] 7.7 Wire routes + start processor

### Sprint 08: Idempotency Middleware

- [x] 8.1 Create idempotency cache table
- [x] 8.2 Implement idempotency repository
- [x] 8.3 Implement idempotency middleware
- [x] 8.4 Remove service-level idempotency checks
- [x] 8.5 Wire middleware
- [~] 8.6 Optional: cleanup goroutine (skipped — not needed for assessment)

### Sprint 09: Transaction History (Skipped)

Out of scope per assessment PDF requirements.

### Sprint 10: Tests

- [x] 10.1 Set up test infrastructure (testcontainers + fixtures)
- [x] 10.2 Unit tests: FX rate service
- [x] 10.3 Unit tests: payment validation
- [x] 10.4 Integration tests: same-currency internal transfer
- [x] 10.5 Integration tests: cross-currency internal transfer
- [x] 10.6 Integration tests: external payout
- [x] 10.7 Unit tests: HMAC webhook verification
- [x] 10.8 Unit tests: JWT auth

### Sprint 11: Polish

- [ ] 11.1 Implement health endpoints
- [ ] 11.2 Implement graceful shutdown
- [ ] 11.3 Review and finalize error codes
- [ ] 11.4 Add secure headers middleware
- [ ] 11.5 Write README.md
- [ ] 11.6 Update ARCHITECTURE.md
- [ ] 11.7 Final docker-compose verification
