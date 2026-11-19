# payments-svc — Current Task

## 2026-11-20
### branch: feature/ORD-142-partial-refunds — Partial refunds
- Ticket: ORD-142
- Inputs: `specs/refunds.md`, `designs/refund-modal.png`, prompt "partial refunds, max 3 per order"
- Also touches: orders-svc, notifications-svc
- Goal: let support refund part of an order, at most 3 refunds per charge, and tell the customer each time.
- Plan:
  - `internal/core/refund.go`: reject a 4th refund on the same charge (`ErrRefundLimitReached`); keep the remaining-amount check.
  - `internal/adapters/grpc/refund.go`: map the new error to `FailedPrecondition`.
  - No migration needed: count from `refunds` where `charge_id = ? AND status <> 'failed'`.
  - `payment.refunded` event: add `refund_amount` and `remaining_amount` fields (new proto fields 7 and 8, nothing removed).
  - Rollout: ship `kitebox-proto` first, then payments-svc, then the consumers that read the new fields.
  - Tests: refund limit, remaining amount, idempotent retry of the same refund.
- Done: plan approved.
- Next: implement `ErrRefundLimitReached` with tests.
- Open questions: should a `failed` refund count toward the limit? (assumed no)
