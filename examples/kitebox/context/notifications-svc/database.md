# notifications-svc — Database

## Engine
- Postgres 16 · database `notifications` · schema `public`
- Access: pgx 5 (hand-written queries in `internal/store`)
- Migrations: golang-migrate, folder `migrations/`

## Tables

### `delivery_attempts`
One message sent (or tried) to one recipient.

| Column | Type | Null | Default | Notes |
|---|---|---|---|---|
| `id` | `bigserial` | no | | primary key |
| `dedupe_key` | `text` | no | | `<subject>:<event id>:<channel>`; unique, makes consumers idempotent |
| `channel` | `text` | no | | `email` or `sms` |
| `template` | `text` | no | | e.g. `email/refund` |
| `recipient` | `text` | no | | email or phone; PII |
| `status` | `text` | no | `'queued'` | `queued`, `sent`, `failed` |
| `attempts` | `int` | no | `0` | |
| `last_error` | `text` | yes | | provider error text |
| `provider_message_id` | `text` | yes | | SES / Twilio id |
| `created_at` | `timestamptz` | no | `now()` | |
| `sent_at` | `timestamptz` | yes | | |

Indexes: unique `dedupe_key` · `idx_attempts_status_created (status, created_at)`

## Relations
Single table; no foreign keys.

## Enums and status values
| Field | Values | Meaning |
|---|---|---|
| `status` | `queued`, `sent`, `failed` | `failed` only after 5 retries |

## Migration log
| Migration | Date | Change |
|---|---|---|
| `000001_init` | 2026-10-06 | `delivery_attempts` |

## Data notes
- `recipient` is PII; rows older than 180 days are deleted by a nightly job (unverified: job not found in this repo).
