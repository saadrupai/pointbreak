# Engineering standards

Read before planning or writing code (`/pointbreak` loads this file for builds). Mandatory unless the team settings in `context/README.md` switch a section off. Where a standard conflicts with a service's existing style, follow the service and note it in the plan.

## 1. Test-driven development
1. **Red:** write a failing test that states the behaviour (unit test for logic, handler test for transport, integration test for a repository or migration). Run it and see it fail for the right reason.
2. **Green:** write the smallest code that passes it.
3. **Refactor:** clean up names, duplication and structure with the tests green.
4. One behaviour per test; name tests after the behaviour (`TestCancel_PaidOrder_RefundsInFull`). Use table-driven tests for variations.
5. Test through public interfaces; fake dependencies at the port (repository, client, publisher), never mock what you own internally.
6. Every bug fix starts with a test that reproduces the bug.
7. The plan lists the tests before the code. "Done" means the full suite passes and the new tests were seen failing first.

## 2. Clean code
- Names say what, not how: `ChargeOrder`, not `ProcessData2`. No abbreviations a newcomer can't guess.
- Small functions that do one thing; one level of abstraction per function. Early returns over nested `if`s.
- No dead code, no commented-out code, no TODO without a ticket.
- Comments explain why, never what. Public types and functions get a one-line doc comment.
- Keep changes focused: no drive-by refactors in a feature change; propose them separately.
- Follow the language's standard formatter and linter (`gofmt`/`golangci-lint`, `prettier`/`eslint`, …) with zero new warnings.

## 3. Design
- SOLID, applied with judgement: depend on interfaces at layer boundaries, keep business rules free of transport, database and framework types.
- Layers: delivery (HTTP/gRPC/consumer) → usecase (business rules) → repository/client (I/O). Dependencies point inward. Wiring happens in one place (`main`/composition root).
- Prefer composition over inheritance and explicit dependencies over globals or singletons.
- YAGNI: build what the ticket needs; no speculative abstractions or config switches.

## 4. Errors
- Never swallow an error. Wrap with context (`fmt.Errorf("charge order %s: %w", id, err)`); return, don't log-and-return.
- Domain errors are typed (`ErrRefundLimitReached`) and mapped to transport codes in the delivery layer only (gRPC status, HTTP status).
- No panics for expected failures. Recover only at process or request boundaries.
- Error messages to clients never leak internals (SQL, stack traces, hostnames).

## 5. APIs, events and contracts
- Contracts are versioned. Add fields; never remove, rename or renumber them in place. Breaking changes get a new version.
- Validate every input at the edge (types, ranges, required fields, sizes).
- Writes that can be retried are idempotent (idempotency key or natural key + upsert).
- Event consumers are idempotent and tolerate redelivery and out-of-order delivery.
- Lists are paginated (cursor preferred); never return unbounded results.
- Roll out compatible changes in order: consumers tolerant first, then producers.

## 6. Data and migrations
- Every schema change is a new, forward-only migration with a matching down migration where the tool supports it. Never edit an applied migration.
- Migrations are safe on large tables: add nullable columns or defaults without rewrites, create indexes concurrently where supported, backfill in batches, then tighten constraints.
- Use transactions for multi-row business invariants. Keep transactions short; no network calls inside them.
- Index for the queries you add; no N+1 queries; select only the columns you need.
- Money in integer minor units with a currency code. Timestamps in UTC with time zone.

## 7. Security
- Never log or return secrets, tokens or full personal data. Mask PII in logs.
- Parameterised queries only; never build SQL or shell commands from input.
- Authorisation is checked in the service for every operation, not only at the gateway.
- Least privilege for credentials and service-to-service access. Dependencies are pinned and kept patched.

## 8. Observability
- Structured logs (key/value) with a request or trace id, at the right level: errors that need action at `error`, not `info`.
- Metrics for rate, errors and latency on every endpoint and consumer; traces propagate across service calls and events.
- Health and readiness endpoints reflect real dependencies.

## 9. Reliability and concurrency
- Every outbound call has a timeout and a cancellation context; retries use backoff with jitter and a cap, only for safe operations.
- No goroutine/thread/promise without an owner that can stop it; graceful shutdown drains in-flight work.
- Protect shared state explicitly (locks, channels, atomic, or single-writer); run the race detector in tests where available.

## 10. Configuration
- Config from environment or config files, never hard-coded; one typed config struct validated at startup; fail fast on missing values.
- Same build for every environment; only config differs.

## 11. Testing pyramid
- Many fast unit tests (usecases, mappers, validators), fewer integration tests (repositories against a real database in a container, consumers against a real broker), a few contract tests for APIs and events shared with other services.
- Tests are deterministic: injected clocks and ids, no sleeps, no order dependence.

## 12. Review checklist (run before saying done)
- [ ] Tests written first, seen failing, now green; full suite passes
- [ ] Layering and naming follow `architecture.md`
- [ ] Errors wrapped and mapped; no leaked internals
- [ ] Contracts backward compatible; every consumer from `impact` checked
- [ ] Migration safe and recorded in `database.md`
- [ ] Inputs validated; authorisation checked; no secrets or PII in logs
- [ ] Timeouts on outbound calls; idempotent retries
- [ ] Context docs updated (`features.md`, `architecture.md`, `database.md`)
