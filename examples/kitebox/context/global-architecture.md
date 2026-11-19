# Kitebox — Global Architecture

> Example workspace. Kitebox is a made-up online shop used to show what filled-in Pointbreak context looks like.

## Overview
Kitebox sells kites online. Customers order through a web app; the gateway forwards requests to three Go services. Orders are the source of truth for what was bought, payments for money movement, notifications for everything sent to customers.

## Diagram
```
web app ──HTTP──▶ gateway ──gRPC──▶ orders-svc ──gRPC──▶ payments-svc
                                        │                    │
                                        └──── NATS events ───┴──▶ notifications-svc ──▶ SMS / email
```

## Services

| Service | Repo dir | Owns | Transport | Port | Context |
|---|---|---|---|---|---|
| `orders-svc` | `orders-svc` | orders, order items, carts | gRPC | 13001 | [context](orders-svc/architecture.md) |
| `payments-svc` | `payments-svc` | charges, refunds, payment methods | gRPC | 13002 | [context](payments-svc/architecture.md) |
| `notifications-svc` | `notifications-svc` | templates, delivery attempts | NATS consumer + gRPC | 13003 | [context](notifications-svc/architecture.md) |

## Communication

**Synchronous calls**

| Caller | Callee | Method | Purpose |
|---|---|---|---|
| gateway | orders-svc | `orders.v1.Orders/*` | all order APIs |
| orders-svc | payments-svc | `payments.v1.Payments/Charge` | charge at checkout |
| orders-svc | payments-svc | `payments.v1.Payments/Refund` | refund a cancelled order |

**Events (NATS JetStream, stream `kitebox`)**

| Subject | Publisher | Consumers | Contract |
|---|---|---|---|
| `order.created` | orders-svc | notifications-svc | `proto/events/v1/order.proto` |
| `order.cancelled` | orders-svc | notifications-svc | `proto/events/v1/order.proto` |
| `payment.succeeded` | payments-svc | orders-svc, notifications-svc | `proto/events/v1/payment.proto` |
| `payment.refunded` | payments-svc | orders-svc, notifications-svc | `proto/events/v1/payment.proto` |

**Queues and jobs**

| Queue | Producer | Worker | Notes |
|---|---|---|---|
| `notify:send` (asynq) | notifications-svc | notifications-svc worker | 5 retries, exponential backoff |

## Shared contracts
- gRPC and event schemas: `kitebox-proto` repo, generated Go code in `github.com/kitebox/proto-go`.
- Shared libs: `github.com/kitebox/kit` (logger, errors, auth interceptor).

## Shared infrastructure
- One Postgres 16 cluster, one database per service.
- NATS JetStream for events, Redis for cache and asynq queues.
- Auth: gateway verifies the customer JWT and forwards `user_id` in gRPC metadata.

## Cross-service rules
- Only the owning service writes to its database.
- Event payloads are versioned protobuf; add fields, never remove or renumber.
- Money is always `int64` minor units plus an ISO currency code.
