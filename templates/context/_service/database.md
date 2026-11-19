# <service> — Database

The service's data model as it stands. Source: the migrations (or ORM schema file) only; no database connection. `sync_database` rebuilds the tables, relations, enums and migration log from them. Mark anything unconfirmed `(unverified)`.

## Engine
- `<Postgres 16>` · database `<orders>` · schema `<public>`
- Access: `<gorm | sqlc | pgx | prisma>` · read replica: `<yes | no>`
- Migrations: tool `<golang-migrate | goose | prisma | atlas>`, folder `<migrations/>`

## Tables

### `<table_name>`
<one line: what a row represents>

| Column | Type | Null | Default | Notes |
|---|---|---|---|---|
| `id` | `bigserial` | no | | primary key |
| `<column>` | `<type>` | `<yes/no>` | `<default>` | `<FK → other.id, enum values, units>` |

Indexes: `<idx_name (col1, col2)>` · Unique: `<…>` · Soft delete: `<deleted_at | none>`

## Relations
```
<orders> 1 ──< <order_items> >── 1 <products>
```

## Enums and status values
| Field | Values | Meaning |
|---|---|---|
| `<orders.status>` | `<pending, paid, shipped, cancelled>` | `<state machine notes>` |

## Data owned elsewhere
- `<user_id>` refers to `<users-svc>`; no foreign key across services.

## Migration log
Newest first. One line per migration.

| Migration | Date | Change |
|---|---|---|
| `<000004_add_refunds>` | `<2026-11-20>` | `<adds refunds table>` |

## Data notes
- `<Money is stored in minor units (cents).>`
- `<Big tables, hot paths, retention rules, PII columns.>`
