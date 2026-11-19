# orders-svc — Database

## Engine
- Postgres 16 · database `orders` · schema `public`
- Access: pgx 5 + sqlc · read replica: no
- Migrations: golang-migrate, folder `migrations/`

## Tables

### `orders`
One customer order.

| Column | Type | Null | Default | Notes |
|---|---|---|---|---|
| `id` | `bigserial` | no | | primary key |
| `public_id` | `text` | no | | `ord_` + ULID, shown to customers; unique |
| `user_id` | `bigint` | no | | owned by users service, no FK |
| `status` | `order_status` | no | `'pending'` | see enums |
| `total_amount` | `bigint` | no | | minor units |
| `currency` | `char(3)` | no | `'USD'` | ISO 4217 |
| `payment_id` | `text` | yes | | set when payments-svc returns a charge id |
| `created_at` | `timestamptz` | no | `now()` | |
| `updated_at` | `timestamptz` | no | `now()` | trigger-maintained |

Indexes: `idx_orders_user_created (user_id, created_at DESC)` · Unique: `public_id` · Soft delete: none

### `order_items`
One line in an order.

| Column | Type | Null | Default | Notes |
|---|---|---|---|---|
| `id` | `bigserial` | no | | primary key |
| `order_id` | `bigint` | no | | FK → `orders.id`, on delete cascade |
| `sku` | `text` | no | | product SKU |
| `name` | `text` | no | | product name at time of order |
| `unit_price` | `bigint` | no | | minor units |
| `quantity` | `int` | no | | > 0 (check constraint) |

Indexes: `idx_order_items_order (order_id)`

### `order_status_events`
Status history, one row per change.

| Column | Type | Null | Default | Notes |
|---|---|---|---|---|
| `id` | `bigserial` | no | | primary key |
| `order_id` | `bigint` | no | | FK → `orders.id` |
| `from_status` | `order_status` | yes | | null for the first row |
| `to_status` | `order_status` | no | | |
| `reason` | `text` | yes | | e.g. `customer_cancelled` |
| `created_at` | `timestamptz` | no | `now()` | |

## Relations
```
orders 1 ──< order_items
orders 1 ──< order_status_events
```

## Enums and status values
| Field | Values | Meaning |
|---|---|---|
| `order_status` | `pending`, `paid`, `shipped`, `delivered`, `cancelled`, `refunded` | `pending → paid → shipped → delivered`; `pending|paid → cancelled`; `cancelled → refunded` when a paid order's refund succeeds |

## Data owned elsewhere
- `user_id` → users service (not in this example workspace).
- `payment_id` → payments-svc `charges.public_id`.

## Migration log
| Migration | Date | Change |
|---|---|---|
| `000003_order_status_events` | 2026-10-28 | adds status history table |
| `000002_add_payment_id` | 2026-10-14 | adds `orders.payment_id` |
| `000001_init` | 2026-10-02 | `orders`, `order_items`, `order_status` enum |

## Data notes
- Money is always minor units + currency; never floats.
- `order_items.name` and `unit_price` are snapshots; don't join to the catalog for history.
