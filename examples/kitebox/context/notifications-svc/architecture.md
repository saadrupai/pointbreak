# notifications-svc — Architecture

## Purpose
Sends every customer message: email and SMS. Turns order and payment events into messages using templates, and records each delivery attempt. Source of truth for what was sent to whom.

## Repo
`git@github.com:kitebox/notifications-svc.git` · module `github.com/kitebox/notifications-svc` · default branch `main`

## Stack
- Language: Go 1.23
- Libraries: nats.go 1.37, asynq 0.25, pgx 5, go-redis 9, `text/template` + `html/template`, testify
- Providers: SES (email), Twilio (SMS)

## Entrypoint and startup
- Entry: `cmd/notifications/main.go`
- Commands: `consume` (NATS consumers), `worker` (asynq), `serve` (gRPC admin API), `migrate up`
- Startup: config → Postgres → Redis → NATS / asynq → providers

## Transport
- NATS durable consumers on stream `kitebox`
- gRPC `:13003` for an internal admin API (resend, list attempts)

## Layering
Pattern: layered.

| Folder | Owns |
|---|---|
| `internal/consumers` | one handler per subject; maps event → message job |
| `internal/templates` | template files under `templates/{email,sms}/` |
| `internal/sender` | SES and Twilio clients behind a `Sender` interface |
| `internal/store` | delivery attempt queries |
| `internal/worker` | asynq task handlers (`notify:send`) |

## Data stores
Details in [database.md](database.md). Postgres 16 (`notifications` db), Redis for asynq.

## Messaging
- Consumes: `order.created`, `order.cancelled`, `payment.succeeded`, `payment.refunded`
- Publishes: none
- Queue: `notify:send`, 5 retries with exponential backoff, then the attempt is marked `failed`

## Dependencies on other services
- Calls: SES, Twilio (external)
- Called by: none (event driven); admin tool uses the gRPC API

## Config
- Keys: `postgres.dsn`, `redis.url`, `nats.url`, `ses.region`, `twilio.account_sid`
- Secret files (never read): `config.yml`
- Example template: `config.example.yml`

## Commands
```
go run ./cmd/notifications consume
go run ./cmd/notifications worker
go test ./...
```

## Gotchas
- Consumers must be idempotent: NATS may redeliver. The `delivery_attempts.dedupe_key` unique index enforces it.
- Unknown event fields are ignored, so adding proto fields is safe; renaming is not.
