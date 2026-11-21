# Pointbreak — handoff notes for any AI agent

Read this first. It is kept current so work can continue in any agent (Claude Code, Antigravity, Cursor, Codex). Last updated: 2026-11-20, Phase 4 done and pushed. Next: Phase 5 launch (each step needs the owner's OK).

## What this is
An open-core product for backend teams in microservice workspaces: a `context/` folder of plain-markdown memory per service, a `/pointbreak` command that starts work across services, and a Go MCP server exposing the same operations as tools. Owner: Saad Rupai (Go backend engineer). Plan: free core on GitHub (claim on m8ven.ai for a trust badge), paid "Pro" pack on agensi.io later.

## Hard rules
- Never add Claude/AI attribution (Co-Authored-By, "Generated with…") to commits or PRs.
- **Never publish without the owner's explicit permission for that specific step**: no git push, no GitHub repo, no Agensi/M8ven submissions, no LinkedIn posts. Local work only until approved.
- Do not commit unless the owner asks.
- Repo: https://github.com/saadrupai/pointbreak (public, MIT). Phases 1-2 pushed in 90bcfb1 on 2026-11-20 with the owner's OK. Every later push needs a new OK.
- `~/Downloads/context-baremetal` is someone else's unlicensed bundle: inspiration only, never copy its text.
- Go code: standard library only (zero dependencies). Every MCP tool declares all four annotations (readOnlyHint, destructiveHint, idempotentHint, openWorldHint) — m8ven.ai grades this.
- All writes stay inside `<workspace>/context/`. Service repos are only read.
- Don't publish exact token or cache numbers we haven't measured.

## Layout
| Path | What | State |
|---|---|---|
| `templates/context/` | README rules, global-architecture, `_service/{temp,architecture,features,database}.md`, `_service/logs/YYYY-MM.md` | done |
| `commands/pointbreak.md` | `/pointbreak <repos> [branches] [ticket/doc/image/"prompt"…]` + `save`, `end`, `status` | done |
| `commands/pointbreak-setup.md` | `/pointbreak-setup`: find repos, create context/, fill docs from code, build system map, add `@context/README.md` import block to workspace CLAUDE.md | done |
| `spec/SPEC.md`, `spec/pointbreak-command.md` | file formats, command behaviour | done |
| `examples/kitebox/context/` | filled example: orders-svc, payments-svc, notifications-svc | done |
| `scripts/init.sh` | creates context/ without overwriting; tested | done |
| `go.mod`, `assets.go` | module `github.com/saadrupai/pointbreak`, go 1.23; embeds `all:templates` | done |
| `internal/workspace/workspace.go` | Workspace, CheckName, Repos, Services, Init, NewestLog, LoadContext | done, tested |
| `internal/workspace/tasks.go` | temp.md parse/write, StartTask, SaveProgress, EndTask, appendToLog, stale-day archiving | done, tested |
| `internal/workspace/query.go` | Status, SearchLogs, Impact, ListServices | done, tested |
| `internal/mcp/` | stdio JSON-RPC 2.0 server: initialize, ping, tools/list, tools/call | done, tested |
| `internal/tools/` | 9 tools wired to workspace methods | done, tested |
| `cmd/pointbreak-mcp/main.go` | flags `--workspace` (else env `POINTBREAK_WORKSPACE`, else cwd), `--version` | done, tested |
| tests | workspace + MCP tests | done, tested |

## MCP tools (Phase 2 target)
| Tool | Workspace method | read/write |
|---|---|---|
| `init_workspace` | Init | write (never overwrites; idempotent) |
| `list_services` | ListServices | read |
| `load_context` | LoadContext (services, files, include_global, skip_rules) | read |
| `start_task` | StartTask | write |
| `save_progress` | SaveProgress (the `/pointbreak save` checkpoint) | write |
| `end_task` | EndTask (moves task to log top, resets temp.md) | write, not destructive (content preserved in log) |
| `task_status` | Status | read |
| `search_logs` | SearchLogs | read |
| `impact` | Impact (system map rows + service doc lines mentioning a term) | read |

## Done in Phase 2
- MCP server (stdlib only), 9 tools, all with 4 annotations. Tests pass (`go test ./...`), workspace coverage ~85%.
- Live stdio test against `examples/kitebox` works (impact, task_status).
- Only past dates count as stale (future-dated temp entries are left alone).
- README rewritten: token, cache and context-hold benefits (only measured number used: Kitebox one-service context ≈ 6 KB ≈ 1,500 tokens), MCP install, `/pointbreak-setup`.
- `commands/pointbreak.md` prefers MCP tools when connected.

## Done in Phase 3 (pushed 2026-11-20)
- Repo root is the plugin: `.claude-plugin/{plugin.json,marketplace.json,icon.svg}`, `commands/`, `skills/pointbreak/SKILL.md`, `hooks/{hooks.json,session-start.sh}`, `.mcp.json` → `scripts/mcp-launch.sh` (uses PATH binary or builds from source into bin/).
- `pointbreak-mcp` auto-detects the workspace by walking up to `context/README.md` (`findWorkspace`, tested).
- Hook tested: workspace root, inside a service, multi-repo without context (suggests setup), unrelated folder (silent).
- `claude plugin validate .` passes; only warning is root CLAUDE.md not loaded as plugin context (intended: it's this handoff file).
- Install: `/plugin install pointbreak --marketplace saadrupai/pointbreak`.

## Phase 4 (done, pushed 2026-11-20)
- New tool `sync_database`: replays migrations from a repo (read-only) into a schema and writes it between `<!-- pointbreak:generated:start/end -->` markers. Human sections (Engine notes, Data owned elsewhere, Data notes) are kept. Formats: golang-migrate (`*.up.sql`), goose (`-- +goose Up`), plain `*.sql`, Prisma `schema.prisma`.
- Three database setups:
  1. Own DB per service → `context/<svc>/database.md` (`sync_database(service)`).
  2. Some services share a DB → schema once in `context/_db/<db>.md` (`sync_database(database, source, users)`); each user service gets a `<!-- pointbreak:shared-db:start/end -->` block in its `database.md`: which shared DB, and tables its code references (scanned, marked unverified).
  3. All services one DB → same as 2 with every service as a user.
- `load_context` adds `context/_db/<db>.md` when a service's database.md links it; `impact` also scans `context/_db/*.md`. `_db` starts with `_`, so it is never treated as a service.
- Code: `internal/dbschema/` (model, sqlparse, prisma, load, render), `internal/workspace/database.go`, tool in `internal/tools/tools.go`.

## Phase 4 results
- 10 tools now. 58 tests pass; coverage dbschema 87%, mcp 93%, workspace 89%.
- No DB connection: sync reads migration files / schema.prisma only. `guard_test.go` fails if any non-test code imports database/sql, net, drivers or ORMs. Docs and templates say "migrations only" (DB-access setting removed).
- Edge cases covered: idempotent re-sync, human text kept, hand-written doc needs replace_sections, dry run, broken markers refused, path escapes rejected, CRLF, goose Up/Down, MySQL MODIFY/CHANGE/RENAME TABLE, renames/drops propagate to FKs and indexes, enum ADD VALUE BEFORE/AFTER, dollar-quoted bodies, Prisma @map/@relation/@@index, shared DB for some/all services, pointer blocks refreshed not duplicated, code-scan ignores vendor/migrations/plain words, load_context dedupes services and shared docs, logs without `---` stay newest-first, future-dated temp entries kept.

## v0.3.0 (2026-11-20)
- Engineering standards: `templates/context/README.md` section 7 (TDD 7.1 … review checklist 7.12), wired into /pointbreak, setup, skill, README, Kitebox.
- Owner rule (strict): after the plan is approved and the user says "go on"/"start", EVERY repository file write (code, tests, migrations, config, docs) is shown first (full new file or diff) with what changed and why, and written only after an explicit "approved"/"okay" for that file. One file per approval, no batching, no exceptions. Only `context/` memory files are outside the rule. The old per-plan review mode and the "tests without approval" exception are removed everywhere.
- Owner rule: when a task is fully done, end it in every service it touched: log gets a short summary plus ALL details from temp.md; temp.md cleared. Code: `EndTask` keeps every line under "- Summary:"; `EndTaskIn` ends across services; `end_task` tool takes `services`. 60 tests pass.
- Benchmark `bench/` (fixture `make_fixture.py`, runner `run.py`): first run hit the owner's 5-hour usage limit (resets 1:40 Asia/Dhaka). Completed runs (Sonnet, repeat 0): t1 impact — baseline 7/7, 136.5k input tokens, 16 turns, $0.139, 28s; pointbreak 7/7, 117.0k, 10 turns, $0.132, 25s. t2 schema — baseline 7/7, 64.3k, $0.073; pointbreak 7/7, 78.8k, $0.038 (more tokens but cheaper: cache reads). t3 flow — baseline 10/10, 209.2k, 33 turns, $0.198, 46s; pointbreak errored (limit). Rerun the full set after the reset: `cd bench && python3 run.py --repeats 2`. Note the CLAUDE.md import loads the whole rules file every turn; it grew with standards and approval rules, which costs tokens on small tasks — consider a slim rules option.

## Done for v0.3.0 (items 1-3 of the owner's list)
- Slim rules: `context/README.md` 13.1 KB → 7.3 KB (loaded every turn); standards moved to `context/STANDARDS.md` (6.1 KB), loaded via `load_context(include_standards: true)` or when planning/building. Init and init.sh create it (13 files for 2 services).
- Flyway: `V1__x.sql`, `V1.1__`, `R__` (applied last), `U` ignored, numeric part ordering (1.2 < 1.10), default folder `src/main/resources/db/migration`.
- Enforcement: `hooks/pre-write.sh` (PreToolUse on Edit|Write|MultiEdit|NotebookEdit) returns permissionDecision "ask" for any write outside `<ws>/context/` inside a Pointbreak workspace; paths with `..` always ask; silent elsewhere.
- Basics: `.github/workflows/ci.yml` (gofmt, vet, race tests on Go 1.23 + stable, build, bash -n, shellcheck, JSON), CHANGELOG.md, CONTRIBUTING.md, SECURITY.md, version 0.3.0 in plugin.json, marketplace.json, main.go. README: daily-use origin (author uses it daily at work; no employer named), slim rules, enforcement, Flyway.
- `bench/results/` is gitignored until the full benchmark is rerun after the usage limit resets.

## v0.4.0 (2026-11-21, merged to main and tagged)
From launch feedback (LinkedIn: gotchas per service, stale docs; X: what impact scans):
- Drift warnings: `internal/workspace/drift.go` (`Drift`, `driftFor`, `gitCommitsSince` via injectable `Workspace.CommitsSince`; read-only `git log`, `GIT_CEILING_DIRECTORIES` so it never walks into a parent repo; 6h grace; services without their own `.git` skipped). Shown in `task_status`, `load_context` (comment before the service's files) and `impact`. Tests in `drift_test.go`; `newWS` stubs CommitsSince.
- Gotchas template prompts (source of truth, delivery, ordering, already exists, traps); rules, /pointbreak and skill read them.
- README: impact row says what it searches; new "Docs that warn when they drift" section. CHANGELOG [Unreleased].

## Next steps, in order
1. Launch done 2026-11-20: v0.3.0 pushed + tagged; M8ven Verified Publisher (badge in README, keep it); Agensi listing submitted free (in review). LinkedIn post published 2026-11-20: https://www.linkedin.com/feed/update/urn:li:activity:7529558785229565953/ (links to GitHub, M8ven, Agensi live listing https://www.agensi.io/skills/pointbreak-microservice-context-workflow).
2. Rerun the benchmark after the usage reset, publish method + results.
3. GitHub Actions blocked: owner's GitHub account is locked for billing (jobs never start). CI badge removed from README on 2026-11-20; add it back (`[![ci](https://github.com/saadrupai/pointbreak/actions/workflows/ci.yml/badge.svg)](…/actions/workflows/ci.yml)`) once billing is fixed and a run is green.
2. Phase 4: `sync_database` tool — parse migrations (golang-migrate `*.up.sql`, goose, Prisma) into `database.md` without touching "Data notes".
3. Phase 5: first commit (owner must ask), GitHub repo `saadrupai/pointbreak`, claim on m8ven.ai (badge in README), Agensi listing (free core), LinkedIn post. Add the listings to the owner's published-listings notes.

## Build
```
go vet ./...
go test ./...
go build -o bin/pointbreak-mcp ./cmd/pointbreak-mcp
```
