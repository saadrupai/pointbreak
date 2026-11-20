---
description: Set up Pointbreak in a microservice workspace — find every service repo, create context/, and fill each service's docs from its code
argument-hint: [service dirs to include…] [--defaults]
---

You are running **/pointbreak-setup** at the root of a microservice workspace (one directory per service repo). Your job: create a complete, accurate `context/` folder so every future session can start work with `/pointbreak` without rediscovering the system.

Arguments: `$ARGUMENTS`

Work in this order. Read as little as you need: manifests, entrypoints, migrations and API definitions tell you most of what matters. Prefer `grep`/`rg` for patterns over reading whole files.

## 1. Find the services

- If service directories were passed, use those. Otherwise list the workspace's direct subdirectories that contain `.git`.
- Skip `context/`, hidden folders and anything that isn't a repo.
- Show the list in one line and continue unless the user objects.

## 2. Team settings

Unless `--defaults` was passed, ask these in one question set (offer the default first):

| Setting | Default |
|---|---|
| Timezone for dates | the machine's timezone |
| Ticket system | none |
| API contract source | detect (`proto/`, `*.proto`, `openapi*.yaml`, a shared contracts repo) |
| Engineering standards | all on (clean code, TDD, design, errors, contracts, migrations, security, observability, reliability, config, test pyramid) — list any to switch off |
| Database layout | each service has its own database / some services share one / all services share one |

## 3. Create the skeleton

- If the Pointbreak MCP server is connected, call its `init_workspace` tool.
- Otherwise create, without overwriting anything that exists:
  `context/README.md`, `context/STANDARDS.md`, `context/global-architecture.md`, and for each service `context/<svc>/{temp.md, architecture.md, features.md, database.md, logs/<YYYY-MM>.md}` from the Pointbreak templates.
- Write the chosen team settings into the table at the bottom of `context/README.md`.

## 4. Fill each service

Do one service at a time. For each, read in this order and stop when you have enough:

1. `README.md`, then `CLAUDE.md` / `AGENTS.md` / `CONTRIBUTING.md` if present.
2. Manifest: `go.mod`, `package.json`, `pom.xml`, `build.gradle`, `pyproject.toml`, `Cargo.toml`.
3. Entrypoint: `main.go`, `cmd/*/main.go`, `src/index.*`, `src/main.*`, `Application.java`.
4. Wiring: where the server, DB, cache, message bus and clients are created.
5. API surface: `*.proto`, OpenAPI files, route registrations, gRPC service registrations.
6. Messaging: publish and subscribe calls (subjects, topics, queue names), cron schedules.
7. Database: the migrations folder (`migrations/`, `db/migrations/`, `prisma/schema.prisma`, `*.sql`), ORM models if there are no migrations.
8. Config **names**: the example config (`config.example.*`, `.env.example`). Never open `.env`, `config.yml`, `*.pem`, `*.key` or other secret files.
9. CI file for build, test and lint commands.

Then fill:
- `architecture.md` — every section that applies; delete the rest.
- `features.md` — one line per feature with its entry point (endpoint, RPC, event consumer, cron).
- `database.md` — if the Pointbreak MCP server is connected, call `sync_database` instead of writing it by hand:
  - service with its own database: `sync_database(service: "<svc>")` (finds the migrations in the repo);
  - database shared by some services: once, `sync_database(database: "<name>", source: "<folder with its migrations>", users: [<services>])`;
  - one database for every service: `sync_database(database: "<name>", source: "…", users: ["*"])`.
  It reads migration files only and never connects to a database. Without the MCP server, document tables, columns, indexes, relations, enums and a newest-first migration log from the migrations by hand.
- `logs/<YYYY-MM>.md` — header only. Add one entry: `### branch: (setup) — Pointbreak context created`.
- `temp.md` — `(none active)`.

Mark anything you could not confirm in code, config or migrations with `(unverified)`. Never invent facts.

## 5. Build the system map

Fill `context/global-architecture.md` from what you found across all services:
- Services table (owner of what, transport, port).
- Synchronous calls: which service's client calls which service's RPC or endpoint.
- Events: subject or topic, publisher, every consumer.
- Queues and jobs.
- Shared contracts and libraries.

Match publishers to consumers by subject name. List unmatched ones under a short "Unmatched" note, marked `(unverified)`.

## 6. Make the rules load automatically

Show the user this block and, after a yes, add it to the workspace-root `CLAUDE.md` (create the file if missing; if a Pointbreak block already exists, replace only the block):

```
<!-- pointbreak:start -->
## Pointbreak
Team rules for agents: @context/README.md
Per-service memory lives in context/<service>/. Start work with /pointbreak <repos> [branches] [inputs…].
<!-- pointbreak:end -->
```

Importing the rules from `CLAUDE.md` puts them in the stable start of every session, where prompt caching applies. Per-service files stay out of that block and are loaded only when a task needs them.

If the `/pointbreak` command isn't installed (no plugin and no `.claude/commands/pointbreak.md`), offer to install it.

## 7. Report

One table: service · files filled · tables documented · `(unverified)` count. Then the three most useful next steps (usually: review `global-architecture.md`, confirm unverified items, try `/pointbreak <one service> - "…"`).

## Never

- Change any file inside a service repo. Setup writes only to `context/` and, after approval, the workspace `CLAUDE.md`.
- Read secret files.
- Commit, push or switch branches.
