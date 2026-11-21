---
name: pointbreak
description: Work inside a Pointbreak microservice workspace (a folder with context/README.md and one context/<service>/ folder per repo). Use whenever the user asks to build, fix, plan or review something in one or more of its services, asks what a service does or how services talk to each other, or asks what was done before — even if they don't mention Pointbreak.
---

# Working in a Pointbreak workspace

The workspace root holds one directory per service repo and a `context/` folder: the team's rules (`context/README.md`), the system map (`context/global-architecture.md`), and per-service memory (`architecture.md`, `database.md`, `features.md`, `temp.md`, `logs/YYYY-MM.md`).

## Before touching code

1. Follow `context/README.md` — it holds the team's approval rules, git rules and secret files.
2. Load memory for the services involved, in one call when the Pointbreak MCP server is connected:
   `load_context(services: [...], skip_rules: true if CLAUDE.md already imports context/README.md)`.
   Otherwise read the same files directly. For a schema-only question, pass `files: ["database.md"]`.
3. Before changing an API, event, queue or shared table, run `impact(term)` and list every consumer in the plan. `impact` searches docs, not code: confirm consumers in code, especially for services flagged "docs may be out of date".
4. Check the log for prior work: `search_logs(query)` with the ticket key, table or event name.

## Doing the work

- Start with `/pointbreak <repos> [branches] [inputs…]`, or call `start_task` per service.
- Before planning or writing code, read `context/STANDARDS.md` (or `load_context(..., include_standards: true)`) and follow it: tests first (red, green, refactor), clean code, layering, wrapped errors, backward-compatible contracts, safe migrations, no secrets in logs, timeouts and idempotent retries.
- Write the plan into `temp.md` (`save_progress` with `plan`) and get it approved. Then write **one file at a time**: show the diff, say what changed and why, and wait for an explicit "approved" / "okay" for that file before writing it. Approving the plan, or "go on", approves no file. No exceptions, tests included.
- Checkpoint at meaningful steps (`save_progress` with `done` / `next`). The plan in `temp.md` must survive the chat being compacted or another agent taking over.
- When the task is fully done, finish with `end_task(services: [every service it touched], summary: [...])`: the log gets the summary and all details, and each `temp.md` is cleared and update `architecture.md`, `features.md` or `database.md` if the work changed what they describe.

## Rules that always hold

- Context files contain facts from code, config and migrations only. Mark anything unconfirmed `(unverified)`.
- Read other branches with `git show <branch>:<path>`; never check out, commit, push or reset without being asked.
- Never read secret files; context files hold names, never values.
- Never edit an applied migration.

If `context/` doesn't exist yet and the folder has several service repos, suggest `/pointbreak-setup`.
