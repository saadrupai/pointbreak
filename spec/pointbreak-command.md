# `/pointbreak` — command spec

One command to start real work across one or more microservices: load the team's conventions, read the right repos on the right branches, understand the requirement, plan, and then build under the team's review rules.

## Forms

| Form | What it does |
|---|---|
| `/pointbreak <repos> [branches] [inputs…]` | Start a task (the main form) |
| `/pointbreak save [repos]` | Checkpoint: write progress, decisions and next steps into `temp.md` |
| `/pointbreak end [repos]` | Finish: fold `temp.md` into the monthly log, update context docs, reset `temp.md` |
| `/pointbreak status` | Show today's active tasks across all services |

`[repos]` in `save` / `end` defaults to every service with an active `temp.md` entry today.

## Arguments of the start form

### 1. `repos` (required)
Comma-separated service repo names, matching folder names in the workspace and in `context/`.

```
orders-svc
orders-svc,payments-svc,notifications-svc
```

### 2. `branches` (optional)
Either one branch for every repo, or `repo=branch` pairs. `-` means "the repo's current branch". Repos left out use their current branch.

```
feature/ORD-142                                   same branch name in every repo
orders-svc=feature/ORD-142,payments-svc=main      per repo
-                                                 current branch everywhere
```

The second argument is treated as branches only if it doesn't look like an input (a URL, an existing file path, or quoted text).

### 3. `inputs` (optional, any number, any order)
Everything that describes the work. Each token is classified:

| Looks like | Treated as | How it is read |
|---|---|---|
| `https://…/browse/ORD-142`, Linear or GitHub issue URL | Ticket | Ticket connector if one is available, otherwise a web fetch |
| Ticket key like `ORD-142` | Ticket | Ticket connector; asks for a link if none |
| Any other URL | Doc | Doc/drive connector or web fetch |
| Existing file path ending in `.png .jpg .jpeg .webp .gif` | Image (design, screenshot, diagram) | Read as an image |
| Existing file path ending in `.md .txt .pdf .yaml .json .proto .sql` | Doc / spec / contract | Read as a file |
| Quoted text, or anything left over | Prompt: the instruction in the user's own words | Used as-is |

### Examples

```
/pointbreak orders-svc feature/ORD-142 https://yourco.atlassian.net/browse/ORD-142

/pointbreak orders-svc,payments-svc orders-svc=feature/refunds,payments-svc=feature/refunds \
  ./specs/refunds.md ./designs/refund-modal.png "partial refunds, max 3 per order"

/pointbreak notifications-svc - "add a retry with backoff to the SMS sender"
```

## What the start form does

1. **Parse.** Echo back one short line per repo: repo, branch, and the inputs found. Stop and ask only if a repo name doesn't exist.
2. **Date.** Get today's date from the shell.
3. **Conventions.** Read `context/README.md`, including the team settings (review mode, ticket system, contract source).
4. **Context.** For each repo, read `architecture.md`, `database.md`, `features.md`, `temp.md` and the newest log. If there are two or more repos, or a contract is involved, read `global-architecture.md`. A repo with no context folder gets one created from the template first (repo code is only read, never changed, for this).
5. **Branches.** For each repo:
   - Requested branch = current branch → read the working tree.
   - Different branch → `git fetch`, then read files with `git show <branch>:<path>` and compare with `git diff <current>...<branch>`. Never check out without asking.
   - Branch missing locally and on the remote → ask whether to create it.
   - Writing code later on a branch that isn't checked out → ask before switching, and only switch with a clean working tree.
6. **Requirements.** Read every input. Extract the goal, acceptance criteria, constraints and open questions. Images are read for UI states, fields and flows; docs for contracts and rules.
7. **Stub.** Write a `temp.md` entry in each repo's context with the goal and the inputs, before investigating further. In multi-repo tasks, each entry names the other repos involved.
8. **Investigate.** Use `features.md` entry points, the layering in `architecture.md` and the tables in `database.md` to find the code that matters. For multi-repo tasks, trace the flow across services (calls, events, queues) using `global-architecture.md`.
9. **Plan.** Write the plan into each `temp.md` entry:
   - Changes per repo: files, layer, what and why
   - Database: new migrations, index changes, backfills
   - Contracts: API / event / queue changes and every consumer affected
   - Order of rollout across services (for example, consumer tolerant first, then producer)
   - Tests to add or update
   - Risks and open questions
10. **Approve.** Show a short plan summary and the open questions. Wait for approval.
11. **Build.** Implement following the team's review mode (`per-change` or `per-plan`). Write tests with the code. Run them. Keep `temp.md` updated at each meaningful step.
12. **Hand back.** Summarise what was done, test results, and anything left. Suggest `/pointbreak end` when the work is merged or paused for the day.

## What it never does
- Commit, push, rebase, reset or delete branches without being asked.
- Read secret files listed in the rules or in a service's `architecture.md`.
- Edit an applied migration.
- Change code in a repo before the plan is approved.
