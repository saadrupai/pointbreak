# payments-svc — Database

## Engine
- Postgres 16 · database `payments` · schema `public`
- Access: gorm 1.25 · read replica: no
- Migrations: goose, folder `db/migrations/`

## Tables

### `charges`
One attempt to take money for an order.

| Column | Type | Null | Default | Notes |
|---|---|---|---|---|
| `id` | `bigserial` | no | | primary key |
| `public_id` | `text` | no | | `chg_` + ULID; unique |
| `order_public_id` | `text` | no | | orders-svc `orders.public_id`, no FK |
| `amount` | `bigint` | no | | minor units |
| `refunded_amount` | `bigint` | no | `0` | minor units, ≤ `amount` (check constraint) |
| `currency` | `char(3)` | no | | ISO 4217 |
| `status` | `charge_status` | no | `'pending'` | see enums |
| `stripe_payment_intent_id` | `text` | yes | | |
| `idempotency_key` | `text` | no | | unique |
| `created_at` | `timestamptz` | no | `now()` | |
| `updated_at` | `timestamptz` | no | `now()` | |

Indexes: `idx_charges_order (order_public_id)` · Unique: `public_id`, `idempotency_key`

### `refunds`
One refund against a charge.

| Column | Type | Null | Default | Notes |
|---|---|---|---|---|
| `id` | `bigserial` | no | | primary key |
| `public_id` | `text` | no | | `rfd_` + ULID; unique |
| `charge_id` | `bigint` | no | | FK → `charges.id` |
| `amount` | `bigint` | no | | minor units |
| `status` | `refund_status` | no | `'pending'` | |
| `reason` | `text` | yes | | |
| `stripe_refund_id` | `text` | yes | | |
| `idempotency_key` | `text` | no | | unique |
| `created_at` | `timestamptz` | no | `now()` | |

Indexes: `idx_refunds_charge (charge_id)`

### `payment_methods`
A customer's saved card (tokens only, never card numbers).

| Column | Type | Null | Default | Notes |
|---|---|---|---|---|
| `id` | `bigserial` | no | | primary key |
| `user_id` | `bigint` | no | | users service, no FK |
| `stripe_customer_id` | `text` | no | | |
| `stripe_payment_method_id` | `text` | no | | unique |
| `brand` | `text` | no | | `visa`, `mastercard`, … |
| `last4` | `char(4)` | no | | |
| `deleted_at` | `timestamptz` | yes | | gorm soft delete |

## Relations
```
charges 1 ──< refunds
```

## Enums and status values
| Field | Values | Meaning |
|---|---|---|
| `charge_status` | `pending`, `succeeded`, `failed` | set by Stripe webhook |
| `refund_status` | `pending`, `succeeded`, `failed` | set by Stripe webhook |

## Data owned elsewhere
- `order_public_id` → orders-svc.
- `user_id` → users service.

## Migration log
| Migration | Date | Change |
|---|---|---|
| `20261021_payment_methods` | 2026-10-21 | saved cards |
| `20261003_init` | 2026-10-03 | `charges`, `refunds`, enums |

## Data notes
- No card numbers or CVCs are stored anywhere; Stripe ids only.
- `refunded_amount` is updated in the same transaction as the refund row turning `succeeded`.
