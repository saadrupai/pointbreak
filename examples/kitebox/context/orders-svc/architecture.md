# orders-svc — Architecture

## Purpose
Owns orders and their life cycle from cart to delivery. Source of truth for what a customer bought and the order's status. Calls payments-svc to charge and refund; publishes order events.

## Repo
`git@github.com:kitebox/orders-svc.git` · module `github.com/kitebox/orders-svc` · default branch `main`

## Stack
- Language: Go 1.23
- Libraries: grpc-go 1.68, pgx 5 + sqlc, go-redis 9, nats.go 1.37, cobra, viper, testify
- Build: `go build ./...`

## Entrypoint and startup
- Entry: `cmd/orders/main.go` (cobra)
- Commands: `serve`, `consume`, `migrate up|down|new`
- Startup: config → Postgres → Redis → payments client → NATS → gRPC server
- Shutdown: gRPC GracefulStop → NATS drain → payments client → Redis → Postgres

## Transport
- gRPC `:13001`, health via `grpc.health.v1`
- Gateway path: `/api/v1/orders/*` → `orders.v1.Orders`

## Middleware / interceptors
Order: recovery → request log → auth (`kit/auth`, reads `user_id`) → validation (protovalidate).

## Layering
Pattern: clean architecture.

| Folder | Owns |
|---|---|
| `internal/domain` | entities (`Order`, `OrderItem`), repository and usecase interfaces |
| `internal/usecase` | business rules: checkout, cancel, status changes |
| `internal/repository/postgres` | sqlc queries in `queries/*.sql`, generated code in `db/` |
| `internal/delivery/grpc` | gRPC handlers and proto mapping |
| `internal/events` | NATS publisher and consumers |

Dependencies are wired in `cmd/orders/serve.go`.

## Data stores
Details in [database.md](database.md). Postgres 16 (`orders` db), Redis for cart cache (TTL 7 days).

## Messaging
- Publishes: `order.created`, `order.cancelled`
- Consumes: `payment.succeeded` (mark paid), `payment.refunded` (mark refunded)

## Dependencies on other services
- Calls: payments-svc `Charge`, `Refund`
- Called by: gateway

## Config
- Keys: `postgres.dsn`, `redis.url`, `nats.url`, `payments.addr`, `grpc.port`
- Secret files (never read): `config.yml`
- Example template: `config.example.yml`

## Commands
```
go build ./...
go run ./cmd/orders serve
go run ./cmd/orders consume
go test ./...
golangci-lint run
go run ./cmd/orders migrate new <name>
go run ./cmd/orders migrate up
sqlc generate
```

## Gotchas
- After editing `queries/*.sql`, run `sqlc generate`; CI fails on a dirty diff.
- Cancel is only allowed in `pending` and `paid`; `shipped` orders go through returns (not built yet).
