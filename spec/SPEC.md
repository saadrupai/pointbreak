# Pointbreak context spec, v0.1

The file formats every Pointbreak tool reads and writes. Humans and agents both edit these files, so the formats stay simple markdown with a few fixed conventions.

## Layout

```
<workspace>/
  context/
    README.md
    global-architecture.md
    <service>/
      temp.md
      architecture.md
      features.md
      database.md
      logs/YYYY-MM.md
  <service-repo>/ …
```

- `<service>` equals the service repo's directory name in the workspace.
- A service is "registered" when `context/<service>/architecture.md` exists.
- Folders starting with `_` (like `_service/` in the templates) are not services.

## Files

| File | Purpose | Written by | Lifetime |
|---|---|---|---|
| `README.md` | Team rules, workflow, settings | Team | Permanent |
| `global-architecture.md` | Service map, calls, events, queues, shared contracts | Agent + team | Permanent, updated when the system changes |
| `<svc>/architecture.md` | How the service is built | Agent from code | Permanent |
| `<svc>/features.md` | What the service does, with entry points | Agent from code | Permanent |
| `<svc>/database.md` | Schema, indexes, relations, enums, migration log | Agent from migrations | Permanent |
| `<svc>/temp.md` | Today's task(s) | Agent | Same calendar day only |
| `<svc>/logs/YYYY-MM.md` | Finished work, one file per month | Agent | Permanent, append at top |

## Entry headers

Shared by `temp.md` and logs.

```
## YYYY-MM-DD
### branch: <branch> — <title>
```

- Exactly one `## YYYY-MM-DD` header per day per file.
- Days are ordered newest first in logs.
- `<branch>` is the branch name, or `(not recorded)`.
- The separator between branch and title is ` — ` (space, em dash, space).

## `temp.md`

```
# <service> — Current Task

## 2026-11-20
### branch: feature/ORD-142 — Partial refunds
- Ticket: ORD-142
- Inputs: specs/refunds.md, designs/refund-modal.png
- Also touches: payments-svc
- Goal: …
- Plan: …
- Done: …
- Next: …
- Open questions: …
```

- When idle the body is exactly `(none active)`.
- A `temp.md` whose date header is not today is stale. Tools must fold it into the log, or ask, before starting a new day.

## Logs

- File name: `logs/YYYY-MM.md`, using the date of the work.
- New day headers go directly under the `---` line, above older days.
- A task block in a log starts with a short summary (what changed, where, why, status), followed by all the details from its `temp.md` entry: goal, inputs, plan, checkpoints, decisions, files and migrations changed, tests and follow-ups. It is written to the log of every service the task touched.

## `database.md`

Sections in this order: Engine, Tables, Relations, Enums and status values, Data owned elsewhere, Migration log, Data notes.

- One `### \`<table>\`` heading per table, each with a column table: Column, Type, Null, Default, Notes.
- The migration log is newest first: Migration, Date, Change.
- Generated content comes from migration files or an ORM schema file only, never from a database connection. It lives between `<!-- pointbreak:generated:start … -->` and `<!-- pointbreak:generated:end -->`; everything outside those markers is human-owned and never rewritten.
- A database shared by several services is documented once in `context/_db/<name>.md`; each service using it has a `<!-- pointbreak:shared-db:start … -->` block in its `database.md` that links it.

## Unverified facts

Any fact not confirmed in code, config, migrations or docs ends with `(unverified)`. Tools that verify a fact later remove the tag.

## Secrets

Context files hold names only, never values: env var names, config keys, secret file names. Tools must refuse to read files listed as secret.
