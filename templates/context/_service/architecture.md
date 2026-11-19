# <service> — Architecture

Fill this from the repo's README, code and config. Mark anything you could not confirm as `(unverified)`. Delete sections that don't apply.

## Purpose
<What this service owns and why it exists. What it is the source of truth for.>

## Repo
`<git remote>` · module `<module path / package name>` · default branch `<main>`

## Stack
- Language: `<Go 1.23 | Node 22 | Java 21 | …>`
- Framework / key libraries: `<grpc-go, gorm, chi, go-redis, sarama, …>`
- Build: `<go build ./... | npm run build>`

## Entrypoint and startup
- Entry: `<main.go | cmd/server/main.go | src/index.ts>`
- Commands: `<serve | worker | migrate | seed>`
- Startup order: `<config → DB → cache → clients → message bus → server>`
- Shutdown order: `<reverse of startup>`

## Transport
- Listens on: `<gRPC :13001 | HTTP :8080>`
- Public path via gateway: `<…>`
- Health check: `<…>`

## Middleware / interceptors
Order: `<recovery → logging → auth → rate limit → …>`. Auth rules live in `<file>`.

## Layering
Pattern: `<clean architecture | hexagonal | layered MVC>`.

| Folder | Owns |
|---|---|
| `<internal/domain>` | `<entities, interfaces>` |
| `<internal/usecase>` | `<business logic>` |
| `<internal/repository>` | `<DB access>` |
| `<internal/delivery/grpc>` | `<handlers>` |

Dependencies are wired in `<file>`.

## Data stores
Details in [database.md](database.md).
- Primary DB: `<Postgres 16>`
- Cache: `<Redis>`
- Other: `<S3, Elasticsearch>`

## Messaging
- Publishes: `<subjects / topics>`
- Consumes: `<subjects / topics>`
- Background jobs: `<queue names, cron schedules>`

## Dependencies on other services
- Calls: `<service-c: Payments.Charge>`
- Called by: `<gateway, service-b>`

## Config
Names only, never values.
- Config keys / env vars: `<DB_DSN, REDIS_URL, …>`
- Secret files (never read): `<config.yml, .env>`
- Example template: `<config.example.yml>`

## Commands
```
<build>
<run>
<test>
<lint>
<migrate up / new>
```

## Gotchas
- `<Things that surprised someone once.>`
