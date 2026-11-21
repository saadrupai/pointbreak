# Changelog

All notable changes to Pointbreak. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow [Semantic Versioning](https://semver.org/).

## [0.4.0] — 2026-11-21

### Added
- Drift warnings: when a service's repo has commits newer than its `architecture.md`, `database.md` or `features.md`, `load_context`, `task_status` and `impact` flag it ("docs may be out of date"), with the commit count and dates. Read-only `git log`; commits within 6 hours of a doc update are treated as the same work; services without their own repo are skipped.
- Service template: the Gotchas section now prompts for source of truth, delivery guarantees, ordering, things that already exist, and traps. The rules and `/pointbreak` read it before planning.

### Changed
- `impact` docs and tool description now say exactly what it searches (system map, shared-DB docs, every service's docs) and that it searches docs, not code.

## [0.3.0] — 2026-11-20

### Added
- `sync_database`: rebuilds database docs from migration files — golang-migrate, goose, Flyway (`V…__`, `R__`, numeric `1.2 < 1.10` ordering), plain SQL and Prisma. Tables, columns, keys, indexes, foreign keys, enums and a newest-first migration log, written between generated markers so human text is kept. Never connects to a database.
- Shared databases: per service, shared by some services, or one for all. Shared schemas live in `context/_db/<name>.md`; each service links it with the tables its code references. `load_context` and `impact` include them.
- Engineering standards in `context/STANDARDS.md`: TDD, clean code, design, errors, contracts, migrations, security, observability, reliability, config, testing pyramid, review checklist.
- Pre-write hook: inside a Pointbreak workspace, every file write outside `context/` makes Claude Code ask for approval, even when edits would be auto-accepted.
- `end_task` takes `services` to finish a task in every service it touched; the log keeps a summary plus all details.
- `load_context` option `include_standards`.
- CI (gofmt, vet, race tests, build, shellcheck), CONTRIBUTING, SECURITY, this changelog.
- Benchmark harness in `bench/` (fixture + runner + answer keys).

### Changed
- Team rules split: a slim `context/README.md` (loaded every turn, 44% smaller) and `context/STANDARDS.md` (loaded for planning and building).
- Every repository file write needs its own approval after the plan is approved; the per-plan review mode and the test-file exception are removed.

### Fixed
- Broken generated markers are refused instead of duplicating content.
- Logs without a `---` separator stay newest first.
- `load_context` loads a repeated service once.
- Future-dated `temp.md` entries are no longer treated as stale.

## [0.2.0] — 2026-11-20
### Added
- Claude Code plugin: manifest, marketplace entry, `pointbreak` skill, session-start hook, MCP launcher.
- Workspace auto-detection from any subfolder.

## [0.1.0] — 2026-11-20
### Added
- Context spec and templates, `/pointbreak` and `/pointbreak-setup`, Kitebox example, `init.sh`.
- Go MCP server with nine tools.

[0.3.0]: https://github.com/saadrupai/pointbreak/compare/6016c13...v0.3.0
