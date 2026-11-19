# payments-svc — Architecture

## Purpose
Owns money movement: charges, refunds and saved payment methods. Talks to the card processor (Stripe) and is the source of truth for whether money moved. Publishes payment events.

## Repo
`git@github.com:kitebox/payments-svc.git` · module `github.com/kitebox/payments-svc` · default branch `main`

## Stack
- Language: Go 1.23
- Libraries: grpc-go 1.68, gorm 1.25 (postgres driver), stripe-go v81, nats.go 1.37, testify
- Build: `go build ./...`

## Entrypoint and startup
- Entry: `main.go` → `cmd.Execute()`
- Commands: `serve`, `webhooks` (HTTP server for Stripe webhooks), `migrate up|down`
- Startup: config → Postgres → Stripe client → NATS → gRPC server
- Shutdown: reverse

## Transport
- gRPC `:13002` (internal only, not exposed through the gateway)
- HTTP `:8082` for `POST /webhooks/stripe`, signature checked with the webhook secret

## Middleware / interceptors
Order: recovery → request log → service-to-service auth (`kit/auth`, mTLS identity must be `orders-svc`) → idempotency (reads `idempotency-key` metadata).

## Layering
Pattern: hexagonal.

| Folder | Owns |
|---|---|
| `internal/core` | domain types, ports (interfaces), payment rules |
| `internal/adapters/stripe` | card processor adapter |
| `internal/adapters/postgres` | gorm repositories |
| `internal/adapters/grpc` | gRPC handlers |
| `internal/adapters/nats` | event publisher |

Wiring in `cmd/serve.go`.

## Data stores
Details in [database.md](database.md). Postgres 16 (`payments` db).

## Messaging
- Publishes: `payment.succeeded`, `payment.failed`, `payment.refunded`
- Consumes: none

## Dependencies on other services
- Calls: Stripe API (external)
- Called by: orders-svc (`Charge`, `Refund`)

## Config
- Keys: `postgres.dsn`, `stripe.api_key`, `stripe.webhook_secret`, `nats.url`, `grpc.port`, `http.port`
- Secret files (never read): `config.yml`, `.env`
- Example template: `config.example.yml`

## Commands
```
go build ./...
go run . serve
go run . webhooks
go test ./...
go run . migrate up
```

## Gotchas
- Every `Charge` and `Refund` call needs an `idempotency-key`; the interceptor rejects calls without one.
- Refund amounts can't exceed `charges.amount - charges.refunded_amount` (checked in `internal/core/refund.go`).
