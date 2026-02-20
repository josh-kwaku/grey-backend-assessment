# Architecture & Design Decisions

Multi-currency payment processing system for the Grey backend take-home assessment.

---

## Overview

This system handles two core payment flows: internal transfers between platform users (same-currency and cross-currency) and external payouts to bank accounts via a mock provider. All money movement is recorded through double-entry bookkeeping, and external payouts follow an async webhook-based reconciliation model.

---

## Core Design Decisions

### 1. Double-Entry Ledger

Every payment produces balanced ledger records (debit + credit entries). User balances are materialized on the accounts table, but the ledger is the authoritative record of all money movement.

Double-entry bookkeeping ensures every movement of money creates balanced entries. Money cannot appear or disappear without a corresponding record on both sides.

**Trade-off:** We maintain a materialized `balance` column on accounts, updated atomically alongside ledger inserts within the same DB transaction, so balance reads don't require summing the entire ledger. The risk is balance drift if a bug ever bypasses the ledger path. In production, you'd run periodic reconciliation jobs to verify `SUM(credits) - SUM(debits) = materialized balance` for every account.

### 2. Multi-Currency Wallets

Each user holds separate accounts per currency (USD, EUR, GBP), one per currency, enforced by a unique constraint. This is the natural model for a multi-currency platform where users need distinct balances in each currency.

### 3. FX Conversion via FX Pool Accounts

Cross-currency transfers route through system-owned FX pool accounts. A transfer of 100 USD to EUR creates 4 ledger entries:

1. DEBIT: User A's USD account, 100 USD
2. CREDIT: FX Pool USD account, 100 USD
3. DEBIT: FX Pool EUR account, 92 EUR
4. CREDIT: User B's EUR account, 92 EUR

The idea is that the platform holds pools of each currency. When a cross-currency transfer happens, we debit from one currency pool and credit into another. The FX pool accounts track the platform's currency exposure. Each currency's books balance perfectly: USD debits equal USD credits, EUR debits equal EUR credits.

FX pool accounts are owned by a special "system" user in the users table, so no schema changes were needed. The system user is just a seeded record.

### 3b. Outgoing Clearing Accounts (External Payouts)

The system user also owns "Outgoing" clearing accounts per currency (Outgoing USD, Outgoing EUR, Outgoing GBP). These represent money in transit to external recipients.

In double-entry, every debit needs a credit. When a user sends money externally, the money leaves our system, but the ledger still needs a balanced entry. The outgoing clearing account provides the credit side.

External payout ledger entries:
- Same-currency (100 USD external): 2 entries
  1. DEBIT: User's USD account, 100 USD
  2. CREDIT: Outgoing USD account, 100 USD

- Cross-currency (100 USD to 92 EUR external): 4 entries
  1. DEBIT: User's USD account, 100 USD
  2. CREDIT: FX Pool USD account, 100 USD
  3. DEBIT: FX Pool EUR account, 92 EUR
  4. CREDIT: Outgoing EUR account, 92 EUR

On failure, compensating entries reverse the flow (credit the user back, debit clearing/FX pool back).

System accounts total: 6 accounts owned by the system user:
- FX Pool USD, FX Pool EUR, FX Pool GBP (currency conversion intermediary)
- Outgoing USD, Outgoing EUR, Outgoing GBP (external payout clearing)

All seeded with large initial balances (10M minor units per currency). In production, these would be funded via treasury operations.

### 4. FX Rate Service with Configurable Spread

Exchange rates come from a mock FX rate service (internal module with its own endpoint: `GET /fx/rates?from=USD&to=EUR`). A configurable spread is applied on top of the mid-market rate:

```
effective_rate = mid_market_rate * (1 - spread_percentage)
```

In production this would be backed by a real rate provider. The spread models how fintech platforms typically generate revenue on currency conversion.

**Trade-off:** The FX service is an in-process module, not a separate service. This keeps the system simple but means you can't scale or deploy the rate service independently. For this scope it's the right call. The service boundary is there in code (separate package, interface-driven), so extracting it later would be straightforward.

### 5. Payment States

Payments go through states: `pending` > `processing` > `completed` | `failed` | `reversed`

- Internal transfers are synchronous. Both users are in our system, so the transfer completes (or fails) atomically within a single DB transaction.
- External payouts are asynchronous. The payment is created in `pending` status, submitted to a mock external provider, and the provider calls back via webhook to confirm or reject.

Real bank transfers are inherently asynchronous, so modeling this felt important for a payment system assessment.

### 6. Immediate Debit with Reversal on Failure

For external payouts, the user's balance is debited immediately when the payment is created. If the external provider fails, reversal ledger entries are created to refund the user.

The main reason for immediate debit is preventing double-spend: if we didn't debit upfront, a user with $100 could initiate two $100 payouts while both are pending. The reversal path (compensating transactions) maintains a complete audit trail since we never delete or update ledger entries.

**Trade-off:** The user sees their balance drop immediately, even though the payment hasn't settled yet. A more sophisticated approach would be a hold/capture pattern where the balance shows a "pending" hold that only becomes a real debit on confirmation. We went with immediate debit + reversal for simplicity.

### 7. Idempotency Middleware

All state-mutating endpoints require an `Idempotency-Key` header. Middleware checks for an existing cached response with the same key and replays it on duplicates.

In payment systems, processing a payment twice can have serious consequences. Network retries, client double-clicks, and infrastructure failures make duplicate requests inevitable. The idempotency key is the defense against this.

Behavior:
- First request: process normally, cache the response keyed by idempotency key + user ID
- Duplicate request (same key, same payload): return cached response with `X-Idempotent-Replayed: true`
- Same key, different payload: return `409 Conflict`

Cache entries expire after 24 hours.

**Trade-off:** Idempotency is implemented at the middleware layer (caches full HTTP responses) rather than at the service layer (checks for existing domain objects). The middleware approach is simpler to implement and covers all endpoints uniformly, but it caches serialized JSON rather than domain-level deduplication. If we needed to change the response format without invalidating idempotency keys, the service-layer approach would be more flexible.

### 8. Authentication

Simple JWT authentication with a login endpoint (`POST /auth/login`). No signup/registration flow; users are seeded via DB migration. JWT is validated on all protected endpoints with a 24-hour expiry.

**Trade-off:** No user registration endpoint. Users are pre-seeded with known credentials. This prioritizes payment processing logic over auth scaffolding, which felt appropriate for the scope of this assessment.

### 9. Concurrency Control

Defense-in-depth with three layers:

1. **Pessimistic locking:** `SELECT ... FOR UPDATE` on account rows during payment processing. This is the primary mechanism. The second concurrent transaction blocks until the first commits.
2. **Optimistic locking:** A `version` column on accounts, checked via `WHERE version = $N` on every balance update. If the version doesn't match, the update is rejected with a `VERSION_CONFLICT` error.
3. **Database constraint:** `CHECK (balance >= 0)` as the final safety net.

Pessimistic locking handles the common case by serializing concurrent transactions on the same account. The optimistic version check guards against any code path that might accidentally bypass the `FOR UPDATE` lock. The DB constraint catches anything else.

**Trade-off:** Pessimistic locking creates contention under high concurrent load on the same account. For this scope, correctness matters more than throughput.

### 10. Recipient Identification (Grey Tag)

Internal transfers identify recipients by `unique_name` (grey tag), a user-chosen handle. The sender also specifies the destination currency. The system validates that the recipient has an account in that currency; if not, it returns `ACCOUNT_NOT_FOUND`.

This is based on the grey tag concept from the assessment spec. Users don't know each other's account UUIDs, so a human-readable handle makes more sense.

### 11. Self-Transfers (Cross-Currency Conversion)

Same-account transfers are blocked, but same-user cross-currency transfers are allowed. A user can convert their own USD to EUR by transferring from their USD account to their EUR account. This is a common feature on multi-currency platforms like Wise and Revolut.

### 12. Mock External Provider

The mock external payment provider runs as a separate service in docker-compose, not a goroutine within the main app.

The mock provider:
- Accepts payment requests via HTTP (`POST /process`)
- Simulates a processing delay (1-3 seconds)
- POSTs a webhook callback to the main app with the result (success or failure)
- Signs the webhook payload with HMAC-SHA256 using a shared secret

This mirrors the general pattern of how external payment rails work: submit a request, wait for an async callback.

### 13. Webhook Security (HMAC Verification)

The mock provider signs webhook callbacks with HMAC-SHA256 using a shared secret (configured via env var on both services). The main app verifies the signature before processing.

Without verification, anyone who discovers the webhook URL could POST fake payment confirmations. HMAC-signed webhooks are a common pattern; Stripe and Paystack both use variations of this approach for their webhook delivery.

### 14. Webhook Processing

Incoming webhooks are stored in the `webhook_events` table first, then a background goroutine processor picks them up, updates payment status, and creates the appropriate ledger entries (completion or reversal).

Flow:
1. Mock provider POSTs webhook to `POST /webhooks/provider`
2. Endpoint validates HMAC, inserts into `webhook_events` table (status: `pending`)
3. Background goroutine polls for pending events and processes each one:
   - Success: payment moves to `completed`, completion ledger entries created
   - Failure: payment moves to `failed`, reversal ledger entries created
4. Webhook event marked as `dispatched`

**Trade-off:** The background processor is a goroutine within the main app. In production, this would be a separate worker process or a message queue consumer for better isolation and independent scaling. It also retries indefinitely on failure with no max attempts or dead-letter mechanism.

### 15. Per-Currency Transaction Limits

Configurable maximum transaction amount per currency (e.g., USD: $100,000, EUR: 90,000 EUR, GBP: 80,000 GBP). Transaction limits are a basic risk control and are configurable per currency since limits may differ across jurisdictions.

### 16. Graceful Shutdown

The app handles SIGTERM/SIGINT via `signal.Notify`, stops the webhook processor first (cancel context + WaitGroup), then drains in-flight HTTP requests with a 30-second timeout before exiting.

Killing a payment system mid-transaction could leave payments in an inconsistent state. DB transactions will roll back on connection drop, but in-flight HTTP responses would be lost.

---

## Data Model Decisions

### Payment Destinations

Destination info lives directly on the payments table:
- `dest_account_id` for internal transfers (points to recipient's account)
- `dest_account_number`, `dest_iban`, `dest_swift_bic`, `dest_bank_name` for external payouts

**Trade-off:** This creates nullable columns (dest_account_id is NULL for external payouts, bank fields are NULL for internal transfers). A cleaner approach would be to normalize into a separate `payment_destinations` table, but we went with the simpler schema given the time constraint.

### Account Fields

The accounts table includes `provider`, `provider_ref`, `account_number`, `routing_number`, `iban`, `swift_bic` fields. These represent metadata about how the account was provisioned and aren't directly used in the transfer flow. They're included because the assessment schema references them and they'd be populated by real banking providers.

### Money Representation

All monetary amounts are stored as `bigint` in minor units (cents/pence). Floats are never used for money. `$19.99` is stored as `1999`. Exchange rates use `decimal(20,10)` for precision, and FX math uses `shopspring/decimal` for arbitrary-precision arithmetic.

---

## API Design

### Endpoints

```
# Auth (public)
POST   /api/v1/auth/login                    > JWT token

# Users (authenticated)
GET    /api/v1/users/:id                     > Get user profile

# Accounts (authenticated)
POST   /api/v1/users/:id/accounts            > Create account (wallet) for a currency
GET    /api/v1/users/:id/accounts             > List user's accounts

# Payments (authenticated, idempotency key required)
POST   /api/v1/payments                       > Internal transfer (by grey tag)
POST   /api/v1/payments/external              > External payout
GET    /api/v1/payments/:id                   > Get payment status

# FX (authenticated)
GET    /api/v1/fx/rates                       > Get exchange rate (from, to query params)

# Webhooks (HMAC-authenticated, internal)
POST   /api/v1/webhooks/provider              > Receive mock provider callback

# Health (public)
GET    /health                                > Liveness check
GET    /health/ready                          > Readiness check (DB connectivity)

# Documentation (public)
GET    /docs                                  > Swagger UI (interactive API reference)
GET    /docs/openapi.yaml                     > OpenAPI 3.0 spec
```

### Response Format

All API responses (except `/health` and `/health/ready`) use a standard envelope:

```json
{ "success": true,  "data": { ... }, "error": null }
{ "success": false, "data": null,    "error": { "code": "...", "message": "...", "details": ... } }
```

Error codes are machine-readable constants (e.g., `INSUFFICIENT_FUNDS`, `DUPLICATE_PAYMENT`, `IDEMPOTENCY_CONFLICT`). Health endpoints return flat JSON for load balancer compatibility.

### HTTP Status Codes

- `201` for resource creation
- `202 Accepted` for async external payouts
- `400` for malformed requests
- `409` for idempotency conflicts
- `422` for business rule violations (insufficient funds, frozen account)

---

## Testing

The approach is to test behavior, not implementation, with a focus on quality over coverage metrics.

- **Unit tests:** FX conversion logic, payment validation rules, HMAC verification, JWT generation/validation
- **Integration tests:** Full payment flows (seed user, create account, make payment, verify ledger entries balance) against real Postgres via testcontainers-go
- **Edge case tests:** Insufficient funds, self-transfer rejection, invalid currency, concurrent transfers racing to overdraft, webhook processing with reversals

---

## Technology Choices

| Choice | Rationale |
|--------|-----------|
| Go standard library router (1.22+) | Built-in method + path routing. No external router needed for this scope. |
| `slog` for logging | Standard library structured logging (Go 1.21+). |
| `shopspring/decimal` for FX math | Arbitrary-precision decimal arithmetic for exchange rate calculations. |
| `golang-migrate` | SQL-based migration files, runs as a separate container in docker-compose. |
| `caarlos0/env` | Parses env vars into a typed config struct. 12-factor compliant. |
| `testcontainers-go` | Spins up a fresh Postgres instance per test suite. No shared test database. |
| Manual dependency injection | All wiring in `main.go`. No DI framework. |

---

## Infrastructure

### Docker Compose Services

| Service | Purpose |
|---------|---------|
| `postgres` | PostgreSQL 16 database |
| `app` | Main payment processing API (port 8080) |
| `mock-provider` | Simulated external payment provider (port 8081) |
| `migrate` | Runs golang-migrate on startup, then exits |

The FX rate service is an internal module within `app` (not a separate container) since it uses hardcoded rates with a configurable spread.

### Configuration

All configuration via environment variables, parsed into a typed Go config struct at startup:

| Env Var | Description | Example |
|---------|-------------|---------|
| `DATABASE_URL` | Postgres connection string | `postgres://user:pass@postgres:5432/grey?sslmode=disable` |
| `JWT_SECRET` | HMAC secret for JWT signing | `super-secret-key` |
| `FX_SPREAD_PCT` | FX spread percentage | `0.005` (0.5%) |
| `MOCK_PROVIDER_URL` | URL of mock provider service | `http://mock-provider:8081` |
| `WEBHOOK_SECRET` | Shared HMAC secret for webhook verification | `webhook-shared-secret` |
| `PORT` | App listen port | `8080` |
| `TX_LIMIT_USD` | Max transaction amount in USD cents | `10000000` ($100K) |
| `TX_LIMIT_EUR` | Max transaction amount in EUR cents | `9000000` (90K EUR) |
| `TX_LIMIT_GBP` | Max transaction amount in GBP pence | `8000000` (80K GBP) |

---

## What I'd Improve With More Time

| Area | Current | Improvement |
|------|---------|-------------|
| Payment destinations | Nullable columns on payments table | Normalize into a separate `payment_destinations` table |
| Balance reconciliation | Materialized balance only | Periodic reconciliation job: verify ledger sums match balances |
| FX rate caching | Rates computed per request | Cache with TTL (30s), background refresh |
| Rate limiting | Not implemented | Token bucket per user, per endpoint |
| Audit logging | Payment events table | Dedicated audit_log with IP, user agent, before/after state |
| Webhook processor | Goroutine in main app | Separate worker process or message queue with retry and dead-letter |
| Webhook retry cap | Retries indefinitely on failure | Max attempts, dead-letter after N failures |
| Orphaned payments | No timeout on pending payouts | Sweep job to expire payments stuck pending beyond 24h |
| Auth | JWT login with seeded users | Full auth flow: signup, email verification, refresh tokens |
| Hold pattern | Immediate debit + reversal | Proper hold/capture for external payouts |
| Transaction history | Not implemented (out of scope) | `GET /accounts/:id/transactions` with cursor-based pagination |
| Monitoring | Health endpoints only | Prometheus metrics, OpenTelemetry tracing |
| CI/CD | None | GitHub Actions with lint, test, build pipeline |
| Database | Single Postgres | Read replicas, connection pooling (PgBouncer) |
| Recipient lookup | Grey tag only | Multiple identifiers: email, account ID, grey tag |
