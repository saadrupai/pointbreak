# Engineering Context — rules for AI agents

This folder is the shared memory for a microservice workspace. Every agent session reads this file first. It then reads the context folder of each service it touches. Humans can read and edit everything here too; it is plain markdown.

> Replace `<placeholders>` with your team's values. Delete rules your team does not want. Keep the ones you keep short and enforceable.

## 1. Workspace layout

```
<workspace>/
  context/
    README.md                  this file: rules + workflow
    global-architecture.md     the system map: services, transports, events, shared contracts
    <service>/
      temp.md                  today's task(s) for this service, nothing older
      architecture.md          how the service is built and wired
      features.md              what the service does, one line per feature
      database.md              schema, indexes, relations, migrations, data notes
      logs/
        YYYY-MM.md             one file per month, newest day at the top
  <service-repo-a>/
  <service-repo-b>/
  …
```

One folder under `context/` per service repo. The folder name matches the repo directory name exactly.

## 2. Load order

When a session starts work on one or more services:

1. Read this file.
2. For each service involved, read `architecture.md`, `database.md`, `features.md`, `temp.md`, and the newest file in `logs/` (the lexically last `YYYY-MM.md`). Respect the Gotchas in `architecture.md`. If a service is flagged "docs may be out of date" (its repo has commits newer than its docs), check the code before trusting those docs, and update them as part of the task.
3. If more than one service is involved, or the task touches an API, event, queue or shared table, also read `global-architecture.md`.

Read only what the task needs from the repos themselves. The context files exist so you don't have to rediscover the system each session.

## 3. Task workflow

**Start**
1. Get today's date from the shell (`date +%Y-%m-%d`). Never guess it.
2. Get the branch from the user's command, otherwise `git -C <repo> branch --show-current`, otherwise write `(not recorded)`.
3. Write a stub in each involved service's `temp.md` with the goal, before planning anything.
4. Read the requirement sources (ticket, doc, image, prompt) and the relevant code.
5. Write the plan into the same `temp.md` entry: files to change per repo, approach, tests, risks, and cross-service impact.
6. Ask for approval of the plan. Then build one file at a time, each shown as a diff and approved before it is written (section 6, rule 2).

**During**
- Keep `temp.md` current at natural checkpoints: what is done, what is next, open questions.
- If the plan changes, update it in `temp.md` and say so.

**End** — when the task is fully done (accepted, merged or closed), in **every service the task touched**:
1. Write the task into the top of `logs/<YYYY-MM>.md` (create the month file if it is missing): a short summary first, then all the details from `temp.md` — goal, inputs, plan, what was done, decisions and why, files and migrations changed, tests, open follow-ups.
2. Update `architecture.md`, `features.md` or `database.md` if the work changed what they describe.
3. Clear the task from `temp.md`. When no task is left for today, `temp.md` goes back to `(none active)`.

**`temp.md` holds today only.** A second task on the same day goes under the same date header. On a new date, start the file fresh; yesterday's work is already in the log.

## 4. Entry format

Used in both `temp.md` and the monthly logs:

```
## 2026-11-20
### branch: feature/ORD-142-partial-refunds — Partial refunds for orders
- Ticket: ORD-142
- Goal: …
- Plan / Done / Next: …
```

One `## YYYY-MM-DD` header per day. One `### branch: <branch> — <title>` block per task. Log entries are short bullets: what changed, where (`file:line`, migration name, endpoint, event subject), why, status.

## 5. Sources of truth

- Facts in context files come from the repo's code, config, migrations and docs. Never invent them.
- Anything you could not confirm gets `(unverified)` after it.
- API contracts live in `<proto repo | OpenAPI spec | shared types package>`. Name the location in `global-architecture.md`.
- Database facts come from migrations (or the ORM schema file) only. Pointbreak never connects to a database; it tracks the schema from the files that change it.

## 6. Rules for changing code (mandatory)

Before planning or writing code, also read `context/STANDARDS.md` (TDD, clean code, design, errors, contracts, migrations, security, observability, reliability, config, testing, review checklist). It is kept separate so everyday questions don't load it.


1. **The plan comes first.** No repository file is created, edited, renamed or deleted before the plan in `temp.md` is approved.
2. **Every file write needs its own approval — no exceptions.** Approving the plan, or saying "go on" / "start", approves no file. For each file, one at a time:
   1. Show the exact change: the full content of a new file, a unified diff of an edit, or the path of a file to delete or rename.
   2. Say what changed and why, in the reply (never as comments in the code).
   3. Stop and wait. Write the file only after an explicit "approved", "okay", "yes" or "go ahead" for **that** file.
   4. If the user asks for changes, show the revised diff and wait again.
   This applies to every file in a repository: code, tests, migrations, config, docs, generated files. Never batch several files behind one approval, and never write "just this small one" without it. The only files outside this rule are the agent's own memory in `context/` (`temp.md`, logs, service docs), which it keeps up to date as part of the workflow. With the Pointbreak plugin installed, a pre-write hook makes Claude Code ask before every other write, even in auto-accept modes; keep it installed.
3. **Tests first.** Follow test-driven development (`context/STANDARDS.md` §1): failing test, then code, then refactor. Each test file goes through the same approval as any other file (rule 2). Run the full suite before saying a task is done, and report failures as they are.
4. **Git.** Never commit, push, rebase, reset, force-push or delete branches unless asked. Never switch the branch of a repo with uncommitted changes. To read another branch, use `git show <branch>:<path>` instead of checking it out.
5. **Secrets.** Never read `.env*`, `config.yml`, `*.pem`, key files, or anything listed under "Secret files" in a service's `architecture.md`. Read the example template instead.
6. **Layering.** Follow the layering in the service's `architecture.md`. New code goes in the layer it belongs to.
7. **Contracts.** Changing an API, event, queue message or shared table means checking every consumer listed in `global-architecture.md` first, and listing them in the plan.
8. **Migrations.** Never edit an applied migration. Add a new one. Note it in the service's `database.md`.

## 7. New services

When a repo exists in the workspace without a `context/<service>/` folder, create the five files from the service template, fill them from the repo's README, code and migrations, mark gaps `(unverified)`, and add the service to `global-architecture.md`.

## 8. Team settings

| Setting | Value |
|---|---|
| Timezone for dates | `<e.g. Asia/Dhaka>` |
| Ticket system | `<Jira | Linear | GitHub Issues | none>` |
| Standards switched off (`STANDARDS.md` sections) | `<none | list section numbers, e.g. 8>` |
| API contract source | `<repo or path>` |

Personal preferences (your git identity, model choice, tone) belong in your own agent settings, not in this shared folder.
