# <system name> — Global Architecture

The map of the whole system. Read it whenever a task touches more than one service or any shared contract. Keep it short: this is an index, not a design document.

## Overview
<two or three sentences: what the system does and who uses it>

## Diagram
```
<client apps> ──HTTP──▶ <gateway> ──gRPC──▶ <service-a> ──event──▶ <service-b>
                                         └─gRPC──▶ <service-c>
```

## Services

| Service | Repo dir | Owns (source of truth for) | Transport | Port | Context |
|---|---|---|---|---|---|
| `<service-a>` | `<dir>` | `<orders, carts>` | `<gRPC | HTTP>` | `<13001>` | [context](<service-a>/architecture.md) |

## Communication

**Synchronous calls**

| Caller | Callee | Method / endpoint | Purpose |
|---|---|---|---|
| `<service-a>` | `<service-c>` | `<Payments.Charge>` | `<charge an order>` |

**Events**

| Event / subject | Publisher | Consumers | Payload contract |
|---|---|---|---|
| `<order.created>` | `<service-a>` | `<service-b, service-d>` | `<proto/events/order.proto>` |

**Queues and jobs**

| Queue / topic | Producer | Worker | Notes |
|---|---|---|---|
| `<email-send>` | `<service-b>` | `<service-b worker>` | `<retries, DLQ>` |

## Shared contracts
- API definitions: `<proto repo | OpenAPI spec path>`
- Event schemas: `<path>`
- Shared libraries: `<logger, errors, auth middleware>`

## Shared infrastructure
- Databases: `<one per service | shared cluster>`
- Message bus: `<Kafka | NATS | RabbitMQ | SQS>`
- Cache: `<Redis>`
- Auth: `<JWT issued by …, verified by …>`

## Cross-service rules
- `<Only the owning service writes to its tables.>`
- `<Events are versioned; never remove a field, add a new one.>`
