# Grey Payment Processing System

A backend payment processing system built in Go. Supports internal transfers (same-currency and cross-currency), external payouts to bank accounts via a mock provider, FX rate quoting, and double-entry ledger accounting.

## Quick Start

```bash
docker-compose up --build
```

This starts Postgres, runs migrations, starts the API server on port 8080, and starts the mock payment provider on port 8081. No manual steps needed.

## API Documentation

Once the server is running, interactive Swagger UI is available at:

```
http://localhost:8080/docs
```

The raw OpenAPI spec is at `/docs/openapi.yaml`.

## Test Credentials

Three users are seeded on startup. All passwords are `password123`.

| User    | Email              | Grey Tag  |
|---------|--------------------|-----------|
| Alice   | alice@test.com     | alice     |
| Bob     | bob@test.com       | bob       |
| Charlie | charlie@test.com   | charlie   |

## Architecture

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for detailed design decisions covering:

- Double-entry ledger model
- FX pool and system accounts
- External payout flow with webhook reconciliation
- Idempotency and concurrency control
- Trade-offs and what I'd improve with more time

## Running Tests

```bash
go test ./... -race
```

Integration tests use [testcontainers-go](https://github.com/testcontainers/testcontainers-go) to spin up real Postgres instances. Docker must be running.

## Project Structure

```
cmd/
  api/               Main API server entrypoint
  mock-provider/     Mock external payment provider
internal/
  config/            Environment variable loading
  domain/            Core types, business rules, errors
  service/           Business logic layer
    payment/         Payment service (transfers, payouts, validation)
  repository/        Database access (Postgres)
  handler/           HTTP handlers
  middleware/        Auth, idempotency, logging, security headers
  auth/              JWT utilities
  fx/                FX rate service
  logging/           Structured logging
  testutil/          Test helpers (testcontainers, fixtures)
migrations/          SQL migration files (golang-migrate)
docs/                Architecture docs, OpenAPI spec
docker/              Dockerfiles
```

## Key Design Decisions

- **Money is never a float.** All amounts stored as `int64` minor units. FX math uses `shopspring/decimal`.
- **Double-entry ledger.** Every transfer creates balanced debit/credit entries. The ledger is the source of truth.
- **Idempotency middleware.** POST requests require an `Idempotency-Key` header. Responses are cached and replayed on duplicate requests.
- **Pessimistic + optimistic locking.** `SELECT FOR UPDATE` serializes concurrent transactions on the same account. A version column provides a second layer of defense. A `CHECK (balance >= 0)` constraint is the final safety net.
- **HMAC webhook verification.** Provider callbacks are signed with SHA-256 and verified before processing.
- **Graceful shutdown.** On SIGTERM, the webhook processor stops first, then in-flight HTTP requests are drained with a 30-second timeout.
