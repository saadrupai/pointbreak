# Pointbreak

**Shared memory and a one-command workflow for AI agents in microservice teams.**

AI coding agents forget your system every session. In a microservice codebase that's expensive: the agent rediscovers each service, misses the consumers of an event, and edits code in the wrong layer. Pointbreak gives every service a small, plain-markdown memory and gives the agent one command to start real work across services:

```
/pointbreak orders-svc,payments-svc feature/ORD-142 https://yourco.atlassian.net/browse/ORD-142 ./designs/refund-modal.png "partial refunds, max 3 per order"
```

The agent loads your team's rules, reads each service's architecture, database and recent history, reads the ticket, the design and your instruction, checks the right branch in each repo, writes a plan per service (including every consumer a contract change affects), and waits for approval before touching code.

## Why teams use it

### 1. Low token usage
Without shared memory, every session starts by crawling repos: directory listings, entrypoints, config, migrations, grep after grep, often across several services. Pointbreak replaces that with one `load_context` call that returns a few small, curated files.

- In the bundled [Kitebox example](examples/kitebox/context), one service's full context (architecture, database, features, current task, this month's log) is about **6 KB, roughly 1,500 tokens**. All three services plus the rules and system map are about 28 KB.
- `load_context` takes a `files` filter, so a task that only needs the schema loads only `database.md`.
- Logs are split by month, so history doesn't grow the default load: only the newest month is read.
- `impact` and `search_logs` answer "who consumes this event?" and "how did we do this last time?" by scanning the docs, without opening a single repo file.

### 2. Cache-friendly by design
Prompt caching makes repeated input much cheaper, but only when the start of the prompt stays the same. Pointbreak keeps it stable:

- `/pointbreak-setup` imports the team rules into your workspace `CLAUDE.md` (`@context/README.md`), so they sit in the fixed start of every session, where caching applies.
- `load_context` always returns files in the same order (rules, system map, then each service's architecture, database, features, task, log). Same task, same bytes, same cache prefix.
- The large files (`architecture.md`, `database.md`, `features.md`) change rarely. The parts that change every day (`temp.md`, the current log) are small and come last, so a day of edits doesn't invalidate the big stable block in front of them.

### 3. Context that holds
- **Across sessions:** tomorrow's session resumes from `temp.md` and the log instead of from zero.
- **Across compaction and model switches:** the plan, decisions and next step live in files, not only in the chat history, so a long session that gets summarised (or moves to another agent) loses nothing that matters.
- **Across people:** a teammate picking up your branch reads the same memory you wrote.
- **Across services:** `global-architecture.md` keeps the calls, events and queues between services in one place, so a change in one repo is checked against every consumer.

### 4. Safe by default
- Plans are approved before any code changes; the team picks per-change or per-plan review.
- No commits, pushes, resets or branch switches unless asked. Other branches are read with `git show`, never checked out.
- Secret files are never read. Context files hold names, never values.
- Every write stays inside `context/`. Applied migrations are never edited.

## What's inside a workspace

```
<workspace>/
  CLAUDE.md                    imports context/README.md (added by /pointbreak-setup)
  context/
    README.md                  your team's rules for agents
    global-architecture.md     the service map: calls, events, queues, shared contracts
    orders-svc/
      temp.md                  today's task: goal, plan, progress
      architecture.md          how the service is built and wired
      features.md              what it does, with entry points
      database.md              tables, indexes, relations, enums, migration log
      logs/2026-11.md          finished work, one file per month
    payments-svc/ …
  orders-svc/                  your repos, untouched
  payments-svc/
```

Everything is markdown your team can read, review and edit. Nothing leaves your machine.

## Commands

| Command | Does |
|---|---|
| `/pointbreak-setup` | One-time setup: finds every service repo, creates `context/`, fills each service's docs from its code and migrations, builds the system map, wires the rules into `CLAUDE.md` |
| `/pointbreak <repos> [branches] [ticket / doc / image / "prompt"…]` | Start a task across one or more services |
| `/pointbreak save [repos]` | Checkpoint progress into `temp.md` |
| `/pointbreak end [repos]` | Fold the task into the monthly log and update the service docs |
| `/pointbreak status` | Show today's active tasks |

Full behaviour: [spec/pointbreak-command.md](spec/pointbreak-command.md).

## MCP tools

The `pointbreak-mcp` server (Go, standard library only, single binary) exposes the same operations to any MCP client: Claude Code, Cursor, Codex, Antigravity and others.

| Tool | Does | Writes? |
|---|---|---|
| `init_workspace` | Create `context/` for every repo (never overwrites) | `context/` only |
| `list_services` | Registered services and repos still missing context | no |
| `load_context` | Rules, system map and service memory in one call, stable order | no |
| `start_task` | Add today's task block to a service's `temp.md` | `context/` only |
| `save_progress` | Checkpoint Plan / Done / Next / Decisions / Questions | `context/` only |
| `end_task` | Move a task to the top of the monthly log | `context/` only |
| `task_status` | Today's tasks across services, stale entries flagged | no |
| `search_logs` | Search past work across services | no |
| `impact` | Every service and doc that depends on an event, RPC, table or field | no |

Every tool declares MCP annotations (`readOnlyHint`, `destructiveHint`, `idempotentHint`, `openWorldHint`), so hosts can show what is safe to run.

## Quick start

```bash
# 1. Build the server (Go 1.23+)
go build -o ~/.local/bin/pointbreak-mcp ./cmd/pointbreak-mcp

# 2. Add it to Claude Code for your workspace
cd ~/work/yourco
claude mcp add pointbreak -- pointbreak-mcp --workspace "$PWD"

# 3. Install the commands
mkdir -p .claude/commands
cp /path/to/pointbreak/commands/*.md .claude/commands/

# 4. In Claude Code, run once:
/pointbreak-setup
```

Other MCP clients: point them at `pointbreak-mcp --workspace /path/to/workspace` over stdio.

No MCP? `./scripts/init.sh ~/work/yourco` creates the same `context/` folder, and the commands work with plain file reads.

## Status and roadmap

| Phase | What | Status |
|---|---|---|
| 1 | Context spec, templates, `/pointbreak` and `/pointbreak-setup`, example workspace, init script | ✅ |
| 2 | Go MCP server with 9 tools, tests | ✅ |
| 3 | Claude Code plugin: one-step install, session-start hook that loads the service you're in | next |
| 4 | `sync_database`: generate `database.md` from migrations (golang-migrate, goose, Prisma) | planned |

## Develop

```bash
go vet ./...
go test ./...
go build -o bin/pointbreak-mcp ./cmd/pointbreak-mcp
```

| Path | What |
|---|---|
| `templates/context/` | Templates (embedded in the binary) |
| `commands/` | `/pointbreak` and `/pointbreak-setup` |
| `internal/workspace/` | Reading and writing `context/` |
| `internal/mcp/` | Minimal stdio JSON-RPC MCP server |
| `internal/tools/` | Tool definitions and annotations |
| `cmd/pointbreak-mcp/` | The binary |
| `spec/` | File formats and command behaviour |
| `examples/kitebox/` | A filled-in example workspace |
| `scripts/init.sh` | Shell alternative to `init_workspace` |

## License

MIT © 2026 Saad Rupai
