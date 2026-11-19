# Pointbreak — handoff notes for any AI agent

Read this first. It is kept current so work can continue in any agent (Claude Code, Antigravity, Cursor, Codex). Last updated: 2026-11-20, Phase 3 done (Claude Code plugin) and pushed. Next: Phase 4.

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

## Next steps, in order
1. Phase 4 below. Ask the owner before every push.
2. Phase 4: `sync_database` tool — parse migrations (golang-migrate `*.up.sql`, goose, Prisma) into `database.md` without touching "Data notes".
3. Phase 5: first commit (owner must ask), GitHub repo `saadrupai/pointbreak`, claim on m8ven.ai (badge in README), Agensi listing (free core), LinkedIn post. Add the listings to the owner's published-listings notes.

## Build
```
go vet ./...
go test ./...
go build -o bin/pointbreak-mcp ./cmd/pointbreak-mcp
```
