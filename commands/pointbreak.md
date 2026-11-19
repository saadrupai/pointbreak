---
description: Start, checkpoint or finish a task across one or more microservices using the shared context/ folder
argument-hint: <repos> [branches] [ticket|doc|image|"prompt"…] | save [repos] | end [repos] | status
---

You are running **/pointbreak** in a microservice workspace. The workspace root contains one directory per service repo and a `context/` folder with the team's rules and per-service memory.

Arguments: `$ARGUMENTS`

Follow `context/README.md` as the team's rules for everything below.

If the Pointbreak MCP tools are connected, use them instead of editing context files by hand: `load_context` (step 3, one call for all repos; pass `skip_rules: true` if `context/README.md` is already imported in `CLAUDE.md`), `start_task` (step 6), `save_progress` (plans and checkpoints), `end_task` (finish), `task_status` (status) and `impact` (before any contract change). If `context/README.md` is missing, say so and offer to create the `context/` folder from the Pointbreak templates. Do not continue the task without it.

## Pick the form

- First word is `save` → **Checkpoint.**
- First word is `end` → **Finish.**
- First word is `status` → **Status.**
- Anything else → **Start.**

## Start

Parse the arguments:
- First token: comma-separated repo names. Each must be a directory in the workspace root. If one isn't, stop and list the directories you did find.
- Second token, only if it isn't a URL, an existing file path or quoted text: branches. Either one branch for all repos, `repo=branch` pairs, or `-` for current branches.
- Every remaining token is an input. URLs and ticket keys are tickets or docs. Existing image files are designs or screenshots. Other existing files are specs. Anything else, including quoted text, is the user's instruction.

Echo the parse in one line per repo (repo · branch · inputs), then:

1. Run `date +%Y-%m-%d` for today's date.
2. Read `context/README.md` and note the team settings (review mode, ticket system, contract source).
3. For each repo read `context/<repo>/architecture.md`, `database.md`, `features.md`, `temp.md` and the newest file in `context/<repo>/logs/`. With two or more repos, or any API, event, queue or shared-table change, also read `context/global-architecture.md`. If `context/<repo>/` doesn't exist, create it from the template, filled from the repo's README, code and migrations, with gaps marked `(unverified)`.
4. Branches: run `git -C <repo> branch --show-current`. If the requested branch is checked out, read the working tree. If not, `git -C <repo> fetch`, then read with `git -C <repo> show <branch>:<path>` and `git -C <repo> diff <current>...<branch>`. Never check out, create or switch branches without asking. If the branch exists nowhere, ask whether to create it.
5. Read every input: tickets through a connected ticket tool if there is one (otherwise fetch the URL, or ask for the text), docs and specs as files or fetched pages, images as images. Write down the goal, acceptance criteria, constraints and open questions.
6. Before investigating further, add today's entry to each repo's `context/<repo>/temp.md`:
   `## <date>` then `### branch: <branch> — <title>` with Goal, Inputs and, for multi-repo work, "Also touches: <other repos>".
7. Investigate: start from the entry points in `features.md`, follow the layering in `architecture.md`, check tables in `database.md`. For multi-repo work, trace calls, events and queues across services with `global-architecture.md`. Read only what you need.
8. Write the plan into each `temp.md` entry: changes per repo (file, layer, what, why), migrations, contract changes and their consumers, rollout order across services, tests, risks, open questions.
9. Show a short plan summary and the open questions. **Stop and wait for approval.**
10. After approval, build following the review mode in `context/README.md`. Write unit tests with the code and run them. Keep `temp.md` current at each meaningful step. Report test results as they are.
11. When done, summarise the work and suggest `/pointbreak end`.

## Checkpoint (`save`)

For each repo given, or each repo with an entry dated today: update its `temp.md` entry with Done, Next, Decisions and Open questions. Keep it short. Confirm in one line per repo.

## Finish (`end`)

For each repo given, or each repo with an entry dated today:
1. Summarise the `temp.md` entry into short bullets and insert it at the top of `context/<repo>/logs/<YYYY-MM>.md`, under today's `## <date>` header (create the file and header if missing).
2. If the work changed what `architecture.md`, `features.md`, `database.md` or `global-architecture.md` describe, update them (code facts only, gaps marked `(unverified)`).
3. Reset `temp.md` to the header plus `(none active)`.
4. Report what was logged and which docs changed. Do not commit anything.

## Status

List each service with an entry dated today in its `temp.md`: repo, branch, title, and the last Next item.

## Always

- No commits, pushes, rebases, resets or branch deletions unless the user asks.
- Never read secret files named in `context/README.md` or in a service's `architecture.md`.
- Never edit an applied migration.
- No repository code changes before the plan is approved.
