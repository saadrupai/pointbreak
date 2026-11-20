# Contributing

Thanks for helping. Issues and pull requests are welcome.

## Ground rules
- **No runtime dependencies.** The server uses the Go standard library only. Discuss before adding one.
- **No database or network access.** `guard_test.go` fails the build if a database driver or network package is imported. Schema tracking reads migration files only.
- **Writes stay in `context/`.** Tools may read service repos but never change them.
- **Every MCP tool declares all four annotations** (`readOnlyHint`, `destructiveHint`, `idempotentHint`, `openWorldHint`), truthfully.
- **Tests first.** New behaviour comes with a test that failed before the change.

## Working on the code
```bash
go vet ./...
go test ./...                 # add -race where cgo is available
gofmt -l .                    # must print nothing
go build -o bin/pointbreak-mcp ./cmd/pointbreak-mcp
claude plugin validate .      # if you change the plugin files
```

## Where things live
| Path | What |
|---|---|
| `templates/context/` | Files `init_workspace` creates (embedded in the binary) |
| `internal/workspace/` | Reading and writing `context/` |
| `internal/dbschema/` | Migration replay and Prisma parsing |
| `internal/tools/` | MCP tool definitions |
| `internal/mcp/` | stdio JSON-RPC server |
| `commands/`, `skills/`, `hooks/` | Claude Code plugin parts |
| `spec/` | File formats and command behaviour — update it with any format change |

## Adding a migration format
Add detection and ordering in `internal/dbschema/load.go`, parsing in `sqlparse.go` (or a new file), and a test with a realistic fixture in `dbschema_test.go`. Unknown statements must end up in `Skipped`, never guessed.

## Pull requests
Keep them focused, describe the behaviour change, and update `CHANGELOG.md` under "Unreleased".
